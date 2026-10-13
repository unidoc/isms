package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/refs"
)

// ReferenceInput is a reference to create alongside an entity.
type ReferenceInput struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// validateReferenceInputs applies the POST /references rule to the "references"
// field of a create request: every target must resolve, in the form references
// store (#341, #351). Create handlers call it before creating the entity, so a
// bad entry fails the request with nothing written. Entries with both fields
// blank are skipped, as createReferencesForEntity skips them.
//
// It returns a new slice with each ID replaced by its canonical form (#350):
// a program or objective sent as a row id comes back as its key / display id,
// so createReferencesForEntity — called next, once the entity exists — never
// sees the raw input. Returning a new slice rather than mutating refs in
// place makes it hard for a caller to keep using the raw input by accident.
func (s *Server) validateReferenceInputs(ctx context.Context, orgID int, viewer db.TaskViewer, refs []ReferenceInput) ([]ReferenceInput, error) {
	canonical := make([]ReferenceInput, len(refs))
	for i, r := range refs {
		if r.Type == "" && r.ID == "" {
			canonical[i] = r
			continue
		}
		if r.Type == "" || r.ID == "" {
			return nil, apiError(http.StatusBadRequest, CodeInvalidRequest)
		}
		id, found := s.canonicalReferenceID(ctx, orgID, viewer, r.Type, r.ID)
		if !found {
			return nil, apiError(http.StatusBadRequest, CodeNotFoundInOrg, Entity(r.Type))
		}
		canonical[i] = ReferenceInput{Type: r.Type, ID: id}
	}
	return canonical, nil
}

// createReferencesForEntity creates bidirectional references for a newly created entity.
// Called by create handlers that accept a "references" field in the request body,
// after validateReferenceInputs has accepted it. The entity already exists by
// then, so a failed write is logged rather than failing the request.
func (s *Server) createReferencesForEntity(ctx context.Context, orgID int, sourceType, sourceID, actor string, refs []ReferenceInput) {
	for _, r := range refs {
		if r.Type == "" || r.ID == "" {
			continue
		}
		err := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
			fwd := &db.EntityReference{SourceType: sourceType, SourceID: sourceID, TargetType: r.Type, TargetID: r.ID, CreatedBy: actor}
			if err := db.CreateReferenceTx(ctx, tx, orgID, fwd); err != nil {
				return err
			}
			rev := &db.EntityReference{SourceType: r.Type, SourceID: r.ID, TargetType: sourceType, TargetID: sourceID, CreatedBy: actor}
			return db.CreateReferenceTx(ctx, tx, orgID, rev)
		})
		if err != nil {
			log.Printf("references: linking %s %s to %s %s failed: %v", sourceType, sourceID, r.Type, r.ID, err)
		}
	}
}

// createReferencesTx is createReferencesForEntity inside an existing transaction
// (suggestion apply, #200): both directions, and a failure aborts the apply
// instead of being logged.
func (s *Server) createReferencesTx(ctx context.Context, tx pgx.Tx, orgID int, sourceType, sourceID, actor string, refs []ReferenceInput) error {
	for _, r := range refs {
		if r.Type == "" || r.ID == "" {
			continue
		}
		fwd := &db.EntityReference{SourceType: sourceType, SourceID: sourceID, TargetType: r.Type, TargetID: r.ID, CreatedBy: actor}
		if err := db.CreateReferenceTx(ctx, tx, orgID, fwd); err != nil {
			return err
		}
		rev := &db.EntityReference{SourceType: r.Type, SourceID: r.ID, TargetType: sourceType, TargetID: sourceID, CreatedBy: actor}
		if err := db.CreateReferenceTx(ctx, tx, orgID, rev); err != nil {
			return err
		}
	}
	return nil
}

// handleListReferences returns all references for an entity (both directions).
func (s *Server) handleListReferences(c echo.Context) error {
	orgID := getOrgID(c)
	entityType := c.QueryParam("type")
	entityID := c.QueryParam("id")
	if entityType == "" || entityID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "type and id query params required")
	}

	ctx := c.Request().Context()

	// A program or objective may be asked for by row id (e.g. MCP's entity_id, or
	// the program page's numeric id) while every row is stored under its key /
	// display id (#350); an audit, finding or programme likewise by row id while
	// rows are stored as AUDIT-<id> / FIND-<id> / AP-<id> (#366, #394). Canonicalise the query id so those rows are found. If it
	// doesn't resolve — a soft-deleted entity, or a legacy raw-id row from before
	// this fix — fall back to the literal id so those rows still list.
	lookupID := entityID
	viewer := taskViewer(c)
	if canonicalID, found := s.canonicalReferenceID(ctx, orgID, viewer, entityType, entityID); found {
		lookupID = canonicalID
	}

	refs, err := s.db.ListAllReferencesForEntity(ctx, orgID, entityType, lookupID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Resolve titles for each reference and normalize: return the "other" side.
	// Dedup: bidirectional storage means both A->B and B->A exist; keep one per pair.
	// Subtype: for documents, surface the frontmatter type (control/policy/clause/etc)
	// so the UI can label the badge by document role instead of generic "DOC".
	type refWithTitle struct {
		db.EntityReference
		Title   string `json:"title"`
		Subtype string `json:"subtype,omitempty"`
	}
	seen := make(map[string]bool)
	result := make([]refWithTitle, 0, len(refs))
	for _, r := range refs {
		// Determine the "other" entity to resolve title for. Rows came back
		// matched against lookupID (the canonical form), so compare against that,
		// not the literal query id.
		otherType, otherID := r.TargetType, r.TargetID
		if r.TargetType == entityType && r.TargetID == lookupID {
			otherType, otherID = r.SourceType, r.SourceID
		}
		pairKey := otherType + ":" + otherID
		if seen[pairKey] {
			continue
		}
		seen[pairKey] = true
		rwt := refWithTitle{EntityReference: r}
		rwt.Title = s.resolveEntityTitle(ctx, orgID, viewer, otherType, otherID)
		if otherType == "document" {
			rwt.Subtype = s.resolveDocumentSubtype(ctx, orgID, otherID)
		}
		result = append(result, rwt)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"data": result})
}

// handleCreateReference creates a reference and its reverse (bidirectional).
func (s *Server) handleCreateReference(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	email := getUserEmail(c)

	var req struct {
		SourceType string `json:"source_type"`
		SourceID   string `json:"source_id"`
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
	}
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	if req.SourceType == "" || req.SourceID == "" || req.TargetType == "" || req.TargetID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "source_type, source_id, target_type, target_id required")
	}

	ctx := c.Request().Context()

	// Both sides are checked: the reverse row makes the source a target too, and a
	// reference that does not resolve is stored permanently with a raw-id title (#341).
	// A program or objective can be either side, so both are canonicalised to key /
	// display id before the write, not just checked for existence (#350).
	viewer := taskViewer(c)
	sourceID, found := s.canonicalReferenceID(ctx, orgID, viewer, req.SourceType, req.SourceID)
	if !found {
		return apiError(http.StatusBadRequest, CodeNotFoundInOrg, Entity(req.SourceType))
	}
	targetID, found := s.canonicalReferenceID(ctx, orgID, viewer, req.TargetType, req.TargetID)
	if !found {
		return apiError(http.StatusBadRequest, CodeNotFoundInOrg, Entity(req.TargetType))
	}

	// Create both forward and reverse references atomically with RLS
	fwd := &db.EntityReference{
		SourceType: req.SourceType,
		SourceID:   sourceID,
		TargetType: req.TargetType,
		TargetID:   targetID,
		CreatedBy:  email,
	}
	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.CreateReferenceTx(ctx, tx, orgID, fwd); err != nil {
			return err
		}
		rev := &db.EntityReference{
			SourceType: req.TargetType,
			SourceID:   targetID,
			TargetType: req.SourceType,
			TargetID:   sourceID,
			CreatedBy:  email,
		}
		return db.CreateReferenceTx(ctx, tx, orgID, rev)
	})
	if txErr != nil {
		// Duplicates are absorbed by ON CONFLICT DO UPDATE inside CreateReferenceTx,
		// so any error here is a real one (CHECK violation, FK, etc.) — surface it.
		return pgxHTTPError(txErr)
	}

	return c.JSON(http.StatusCreated, fwd)
}

// handleDeleteReference deletes a reference and its reverse (bidirectional).
func (s *Server) handleDeleteReference(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidID)
	}

	ctx := c.Request().Context()

	// Look up the reference so we can delete both directions.
	ref, err := s.db.GetReference(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "reference not found")
	}

	// Delete both the forward and reverse rows.
	if err := s.db.DeleteReferencePair(ctx, orgID, ref.SourceType, ref.SourceID, ref.TargetType, ref.TargetID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// resolveDocumentSubtype returns the document's frontmatter type (e.g. "control",
// "policy", "clause", "procedure") so the UI can label references by document role
// rather than the generic "document" wire-type. Returns "" if the doc has no type
// or can't be loaded.
func (s *Server) resolveDocumentSubtype(ctx context.Context, orgID int, docID string) string {
	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return ""
	}
	path := st.FindDocumentByID(docID)
	if path == "" {
		return ""
	}
	doc, err := st.LoadDocument(path)
	if err != nil {
		return ""
	}
	return doc.Frontmatter.Type
}

// resolveEntityTitle looks up the display name for an entity by type and ID,
// falling back to the ID itself when the entity cannot be resolved.
func (s *Server) resolveEntityTitle(ctx context.Context, orgID int, viewer db.TaskViewer, entityType, entityID string) string {
	title, _ := s.lookupEntityTitle(ctx, orgID, viewer, entityType, entityID)
	return title
}

// lookupEntityTitle looks up the display name for an entity by type and ID,
// and reports whether the entity was found. It is a thin wrapper around
// lookupEntity for callers that only need the title.
func (s *Server) lookupEntityTitle(ctx context.Context, orgID int, viewer db.TaskViewer, entityType, entityID string) (string, bool) {
	title, _, found := s.lookupEntity(ctx, orgID, viewer, entityType, entityID)
	return title, found
}

// lookupEntity is the API's view of refs.Resolver.Lookup: the entity's display
// title, its canonical reference ID and whether it was found. A failed lookup
// (as opposed to a missing record) is reported as not found, which is right for
// a request that must reject or fall back to the raw id; the CLI is the caller
// that needs the difference. See refs.Resolver.Lookup for the rule.
func (s *Server) lookupEntity(ctx context.Context, orgID int, viewer db.TaskViewer, entityType, entityID string) (title, canonicalID string, found bool) {
	title, canonicalID, found, err := s.refResolver().Lookup(ctx, orgID, viewer, entityType, entityID)
	if err != nil {
		return entityID, entityID, false
	}
	return title, canonicalID, found
}

// refResolver builds the shared reference rule over the server's database and
// per-org stores.
func (s *Server) refResolver() *refs.Resolver {
	return &refs.Resolver{DB: s.db, Store: s.storeForOrg}
}

// canonicalReferenceID resolves entityID to the form a reference must store
// it in, and reports whether it names a real entity of entityType in the org
// (see refs.Resolver.Canonical). Writers must store the returned id, never the
// input entityID.
func (s *Server) canonicalReferenceID(ctx context.Context, orgID int, viewer db.TaskViewer, entityType, entityID string) (string, bool) {
	// The shape gate runs here too, before anything touches s, so a bare number
	// for a sequence type is rejected without a database.
	if refs.SequenceIdentifierTypes[entityType] && !refs.HasIdentifierShape(entityID) {
		return entityID, false
	}
	canonicalID, found, err := s.refResolver().Canonical(ctx, orgID, viewer, entityType, entityID)
	if err != nil {
		return entityID, false
	}
	return canonicalID, found
}

// referenceEntityExists reports whether entityID names a real entity of
// entityType in the org, in a form references accept (see lookupEntity). It
// is a bool wrapper around canonicalReferenceID for callers that only need
// to validate, not store, the id.
func (s *Server) referenceEntityExists(ctx context.Context, orgID int, viewer db.TaskViewer, entityType, entityID string) bool {
	_, found := s.canonicalReferenceID(ctx, orgID, viewer, entityType, entityID)
	return found
}

// parseEntityNumericID strips a prefix like "RISK-" from "RISK-12" and returns 12.
func parseEntityNumericID(id, prefix string) (int64, error) {
	s := stripPrefix(id, prefix)
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid entity ID %q", id)
	}
	return n, nil
}

// stripPrefix removes a prefix if present, otherwise returns the original string.
func stripPrefix(s, prefix string) string {
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}
