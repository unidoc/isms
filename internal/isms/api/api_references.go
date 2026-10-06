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
	// display id (#350); an audit or finding likewise by row id while rows are
	// stored as AUDIT-<id> / FIND-<id> (#366). Canonicalise the query id so those rows are found. If it
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

// lookupEntity looks up an entity by type and ID, and returns its display
// title, its canonical reference ID, and whether it was found. For most types
// the canonical ID is entityID unchanged. Programs and objectives can be
// addressed by row id or by key / display id (#350); both forms resolve, but
// the canonical ID returned is always the key / display id, since that is
// the form the UI reads and writes and the form every reference must be
// stored in. Audits and audit findings likewise resolve by row id or by
// AUDIT-<id> / FIND-<id>, and canonicalise to the prefixed form (#366). Callers that write references (canonicalReferenceID and its
// callers) must store the returned canonicalID, never the input entityID.
func (s *Server) lookupEntity(ctx context.Context, orgID int, viewer db.TaskViewer, entityType, entityID string) (title, canonicalID string, found bool) {
	// References store per-org identifiers (e.g. "RISK-12", "INC-3") — both
	// the UI and createReferencesForEntity write that format. Resolve by
	// identifier, never by numeric row id: the numeric part of an identifier
	// and the row id are different sequences and diverge in multi-org DBs.
	switch entityType {
	case "risk":
		r, err := s.db.GetRiskByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, entityID, false
		}
		return r.Title, entityID, true

	case "legal_requirement":
		l, err := s.db.GetLegalRequirementByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, entityID, false
		}
		return l.Title, entityID, true

	case "asset":
		a, err := s.db.GetAssetByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, entityID, false
		}
		return a.Name, entityID, true

	case "supplier":
		sup, err := s.db.GetSupplierByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, entityID, false
		}
		return sup.Name, entityID, true

	case "system":
		sys, err := s.db.GetSystemByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, entityID, false
		}
		return sys.Name, entityID, true

	case "incident":
		inc, err := s.db.GetIncidentByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, entityID, false
		}
		return inc.Title, entityID, true

	case "corrective_action":
		ca, err := s.db.GetCorrectiveActionByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, entityID, false
		}
		return ca.Title, entityID, true

	case "objective":
		id, err := strconv.ParseInt(entityID, 10, 64)
		if err != nil {
			// Try by display_id (e.g. "ISMS-1")
			o, err := s.db.GetObjectiveByDisplayID(ctx, orgID, entityID)
			if err != nil {
				return entityID, entityID, false
			}
			return o.Title, o.DisplayID, true
		}
		o, err := s.db.GetObjective(ctx, orgID, id)
		if err != nil {
			return entityID, entityID, false
		}
		return o.Title, o.DisplayID, true

	case "program":
		id, err := strconv.ParseInt(entityID, 10, 64)
		if err != nil {
			// Try by key (e.g. "ISMS")
			p, err := s.db.GetProgramByKey(ctx, orgID, entityID)
			if err != nil {
				return entityID, entityID, false
			}
			return p.Title, p.Key, true
		}
		p, err := s.db.GetProgram(ctx, orgID, id)
		if err != nil {
			return entityID, entityID, false
		}
		return p.Title, p.Key, true

	case "document":
		st, err := s.storeForOrg(ctx, orgID)
		if err != nil {
			return entityID, entityID, false
		}
		if docPath := st.FindDocumentByID(entityID); docPath != "" {
			if doc, err := st.LoadDocument(docPath); err == nil {
				if doc.Frontmatter.Title != "" {
					return doc.Frontmatter.Title, entityID, true
				}
				return entityID, entityID, true
			}
			return entityID, entityID, false
		}
		return entityID, entityID, false

	case "audit":
		// AUDIT-/FIND- are built from the row id (audits and audit_findings have
		// no identifier column), so stripping is correct here — see api_audit.go.
		// Both "5" and "AUDIT-5" resolve, but references are stored as AUDIT-<id>
		// only, or the two spellings are two links that never meet (#366).
		id, err := strconv.Atoi(stripPrefix(entityID, "AUDIT-"))
		if err != nil {
			return entityID, entityID, false
		}
		a, err := s.db.GetAudit(ctx, orgID, id)
		if err != nil {
			return entityID, entityID, false
		}
		return a.Title, fmt.Sprintf("AUDIT-%d", a.ID), true

	case "audit_finding":
		// FIND-<id> is the form db.SoftDeleteAuditFinding's linked-CA guard and
		// the create-from-finding flow use; a bare row id is canonicalised to it.
		id, err := strconv.Atoi(stripPrefix(entityID, "FIND-"))
		if err != nil {
			return entityID, entityID, false
		}
		f, err := s.db.GetAuditFinding(ctx, orgID, int64(id))
		if err != nil {
			return entityID, entityID, false
		}
		return f.Title, fmt.Sprintf("FIND-%d", f.ID), true

	case "change_request":
		// CR- identifiers come from the per-org sequence, so the suffix is not
		// the primary key — stripping it named a different change request's
		// title on the reference chip (#201, and the warning in db/changes.go).
		id, err := s.resolveChangeID(ctx, orgID, entityID)
		if err != nil {
			return entityID, entityID, false
		}
		cr, err := s.db.GetChangeRequest(ctx, orgID, int(id))
		if err != nil {
			return entityID, entityID, false
		}
		return cr.Title, entityID, true

	case "task":
		// As with CR- above: TASK- is a per-org sequence, not the primary key.
		id, err := s.resolveTaskID(ctx, orgID, entityID)
		if err != nil {
			return entityID, entityID, false
		}
		t, err := s.db.GetTask(ctx, orgID, id)
		if err != nil {
			return entityID, entityID, false
		}
		// Don't leak a private task's title to someone who may not see it — fall
		// back to the identifier (mirrors db.TaskViewer's rule).
		if t.Private && !viewer.CanSeeAll && t.Assignee != viewer.Email && t.CreatedBy != viewer.Email {
			return entityID, entityID, true
		}
		return t.Title, entityID, true

	default:
		return entityID, entityID, false
	}
}

// sequenceIdentifierTypes are the reference types whose display identifier comes
// from a per-org sequence. Its numeric suffix is not the primary key (#201), so a
// bare number is never a valid reference id for them — even where the underlying
// resolver (resolveTaskID, resolveChangeID) would accept one as a row id.
var sequenceIdentifierTypes = map[string]bool{
	"risk": true, "legal_requirement": true, "asset": true, "supplier": true,
	"system": true, "incident": true, "corrective_action": true,
	"change_request": true, "task": true,
}

// canonicalReferenceID resolves entityID to the form a reference must store
// it in, and reports whether it names a real entity of entityType in the org.
// For a program or objective this is the key / display id, even when
// entityID was the numeric row id (#350); for an audit or audit finding it is
// AUDIT-<id> / FIND-<id> (#366); every other type returns entityID unchanged. Writers must store the returned id, never the input entityID.
func (s *Server) canonicalReferenceID(ctx context.Context, orgID int, viewer db.TaskViewer, entityType, entityID string) (string, bool) {
	if sequenceIdentifierTypes[entityType] && !hasIdentifierShape(entityID) {
		return entityID, false
	}
	_, canonicalID, found := s.lookupEntity(ctx, orgID, viewer, entityType, entityID)
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
