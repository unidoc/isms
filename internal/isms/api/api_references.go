package api

import (
	"context"
	"fmt"
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

// createReferencesForEntity creates bidirectional references for a newly created entity.
// Called by create handlers that accept a "references" field in the request body.
func (s *Server) createReferencesForEntity(ctx context.Context, orgID int, sourceType, sourceID, actor string, refs []ReferenceInput) {
	for _, r := range refs {
		if r.Type == "" || r.ID == "" {
			continue
		}
		fwd := &db.EntityReference{SourceType: sourceType, SourceID: sourceID, TargetType: r.Type, TargetID: r.ID, CreatedBy: actor}
		_ = s.db.CreateReference(ctx, orgID, fwd)
		rev := &db.EntityReference{SourceType: r.Type, SourceID: r.ID, TargetType: sourceType, TargetID: sourceID, CreatedBy: actor}
		_ = s.db.CreateReference(ctx, orgID, rev)
	}
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
	refs, err := s.db.ListAllReferencesForEntity(ctx, orgID, entityType, entityID)
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
		// Determine the "other" entity to resolve title for
		otherType, otherID := r.TargetType, r.TargetID
		if r.TargetType == entityType && r.TargetID == entityID {
			otherType, otherID = r.SourceType, r.SourceID
		}
		pairKey := otherType + ":" + otherID
		if seen[pairKey] {
			continue
		}
		seen[pairKey] = true
		rwt := refWithTitle{EntityReference: r}
		rwt.Title = s.resolveEntityTitle(ctx, orgID, taskViewer(c), otherType, otherID)
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
	viewer := taskViewer(c)
	if !s.referenceEntityExists(ctx, orgID, viewer, req.SourceType, req.SourceID) {
		return apiError(http.StatusBadRequest, CodeNotFoundInOrg, Entity(req.SourceType))
	}
	if !s.referenceEntityExists(ctx, orgID, viewer, req.TargetType, req.TargetID) {
		return apiError(http.StatusBadRequest, CodeNotFoundInOrg, Entity(req.TargetType))
	}

	// Create both forward and reverse references atomically with RLS
	fwd := &db.EntityReference{
		SourceType: req.SourceType,
		SourceID:   req.SourceID,
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		CreatedBy:  email,
	}
	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.CreateReferenceTx(ctx, tx, orgID, fwd); err != nil {
			return err
		}
		rev := &db.EntityReference{
			SourceType: req.TargetType,
			SourceID:   req.TargetID,
			TargetType: req.SourceType,
			TargetID:   req.SourceID,
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
// and reports whether the entity was found.
func (s *Server) lookupEntityTitle(ctx context.Context, orgID int, viewer db.TaskViewer, entityType, entityID string) (string, bool) {
	// References store per-org identifiers (e.g. "RISK-12", "INC-3") — both
	// the UI and createReferencesForEntity write that format. Resolve by
	// identifier, never by numeric row id: the numeric part of an identifier
	// and the row id are different sequences and diverge in multi-org DBs.
	switch entityType {
	case "risk":
		r, err := s.db.GetRiskByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, false
		}
		return r.Title, true

	case "legal_requirement":
		l, err := s.db.GetLegalRequirementByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, false
		}
		return l.Title, true

	case "asset":
		a, err := s.db.GetAssetByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, false
		}
		return a.Name, true

	case "supplier":
		sup, err := s.db.GetSupplierByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, false
		}
		return sup.Name, true

	case "system":
		sys, err := s.db.GetSystemByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, false
		}
		return sys.Name, true

	case "incident":
		inc, err := s.db.GetIncidentByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, false
		}
		return inc.Title, true

	case "corrective_action":
		ca, err := s.db.GetCorrectiveActionByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return entityID, false
		}
		return ca.Title, true

	case "objective":
		id, err := strconv.ParseInt(entityID, 10, 64)
		if err != nil {
			// Try by display_id (e.g. "ISMS-1")
			o, err := s.db.GetObjectiveByDisplayID(ctx, orgID, entityID)
			if err != nil {
				return entityID, false
			}
			return o.Title, true
		}
		o, err := s.db.GetObjective(ctx, orgID, id)
		if err != nil {
			return entityID, false
		}
		return o.Title, true

	case "program":
		id, err := strconv.ParseInt(entityID, 10, 64)
		if err != nil {
			// Try by key (e.g. "ISMS")
			p, err := s.db.GetProgramByKey(ctx, orgID, entityID)
			if err != nil {
				return entityID, false
			}
			return p.Title, true
		}
		p, err := s.db.GetProgram(ctx, orgID, id)
		if err != nil {
			return entityID, false
		}
		return p.Title, true

	case "document":
		st, err := s.storeForOrg(ctx, orgID)
		if err != nil {
			return entityID, false
		}
		if docPath := st.FindDocumentByID(entityID); docPath != "" {
			if doc, err := st.LoadDocument(docPath); err == nil {
				if doc.Frontmatter.Title != "" {
					return doc.Frontmatter.Title, true
				}
				return entityID, true
			}
			return entityID, false
		}
		return entityID, false

	case "audit":
		// AUDIT-/FIND- are built from the row id (audits and audit_findings have
		// no identifier column), so stripping is correct here — see api_audit.go.
		id, err := strconv.Atoi(stripPrefix(entityID, "AUDIT-"))
		if err != nil {
			return entityID, false
		}
		a, err := s.db.GetAudit(ctx, orgID, id)
		if err != nil {
			return entityID, false
		}
		return a.Title, true

	case "audit_finding":
		id, err := strconv.Atoi(stripPrefix(entityID, "FIND-"))
		if err != nil {
			return entityID, false
		}
		f, err := s.db.GetAuditFinding(ctx, orgID, int64(id))
		if err != nil {
			return entityID, false
		}
		return f.Title, true

	case "change_request":
		// CR- identifiers come from the per-org sequence, so the suffix is not
		// the primary key — stripping it named a different change request's
		// title on the reference chip (#201, and the warning in db/changes.go).
		id, err := s.resolveChangeID(ctx, orgID, entityID)
		if err != nil {
			return entityID, false
		}
		cr, err := s.db.GetChangeRequest(ctx, orgID, int(id))
		if err != nil {
			return entityID, false
		}
		return cr.Title, true

	case "task":
		// As with CR- above: TASK- is a per-org sequence, not the primary key.
		id, err := s.resolveTaskID(ctx, orgID, entityID)
		if err != nil {
			return entityID, false
		}
		t, err := s.db.GetTask(ctx, orgID, id)
		if err != nil {
			return entityID, false
		}
		// Don't leak a private task's title to someone who may not see it — fall
		// back to the identifier (mirrors db.TaskViewer's rule).
		if t.Private && !viewer.CanSeeAll && t.Assignee != viewer.Email && t.CreatedBy != viewer.Email {
			return entityID, true
		}
		return t.Title, true

	default:
		return entityID, false
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

// referenceEntityExists reports whether entityID names a real entity of
// entityType in the org, in the form references store (see resolveEntityTitle).
func (s *Server) referenceEntityExists(ctx context.Context, orgID int, viewer db.TaskViewer, entityType, entityID string) bool {
	if sequenceIdentifierTypes[entityType] && !hasIdentifierShape(entityID) {
		return false
	}
	_, found := s.lookupEntityTitle(ctx, orgID, viewer, entityType, entityID)
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
