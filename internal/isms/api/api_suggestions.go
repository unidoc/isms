package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// ═══════════════════════════════════════════════════════════════════════
// APPLY REGISTRY
// ═══════════════════════════════════════════════════════════════════════

// SuggestionApplyFunc applies a suggestion payload within a transaction.
// All entity mutations must use the provided pgx.Tx for atomicity.
// Returns the created/updated entity's display identifier (stored on the
// suggestion) and its primary key (for the changelog row). The key comes from
// the handler, not from the identifier: a create handler's row is uncommitted, so
// a pool-side lookup would not see it, and the number in "RISK-3" is the per-org
// sequence, not the key (#334).
type SuggestionApplyFunc func(ctx context.Context, tx pgx.Tx, s *Server, orgID int, suggestion *db.Suggestion, actor string) (identifier string, entityID int64, err error)

// applyRegistry maps entity_type:suggestion_type to handler functions.
var applyRegistry = map[string]SuggestionApplyFunc{}

func registerApplyHandler(entityType, suggestionType string, fn SuggestionApplyFunc) {
	applyRegistry[entityType+":"+suggestionType] = fn
}

func getApplyHandler(entityType, suggestionType string) SuggestionApplyFunc {
	if fn, ok := applyRegistry[entityType+":"+suggestionType]; ok {
		return fn
	}
	return nil
}

func init() {
	// Risk handlers
	registerApplyHandler("risk", "create", applyRiskCreate)
	registerApplyHandler("risk", "reassess", applyRiskReading) // reassess = reading
	registerApplyHandler("risk", "update", applyRiskUpdate)

	// Incident handlers
	registerApplyHandler("incident", "create", applyIncidentCreate)
	registerApplyHandler("incident", "update", applyIncidentUpdate)
	registerApplyHandler("incident", "link", applyIncidentLink)

	// Supplier handlers
	registerApplyHandler("supplier", "create", applySupplierCreate)
	registerApplyHandler("supplier", "update", applySupplierUpdate)
	registerApplyHandler("supplier", "reassess", applySupplierReviewSuggestion) // reassess = review for suppliers

	// Legal handlers
	registerApplyHandler("legal_requirement", "create", applyLegalCreate)
	registerApplyHandler("legal_requirement", "update", applyLegalUpdate)

	// Change request handlers
	registerApplyHandler("change_request", "create", applyChangeCreate)
	registerApplyHandler("change_request", "update", applyChangeUpdate)

	// Corrective action handlers
	registerApplyHandler("corrective_action", "create", applyCorrActiveCreate)
	registerApplyHandler("corrective_action", "update", applyCorrActiveUpdate)

	// Task handlers
	registerApplyHandler("task", "create", applyTaskCreate)
	registerApplyHandler("task", "update", applyTaskUpdate)

	// Objective handlers
	registerApplyHandler("program", "create", applyProgramCreate)
	registerApplyHandler("objective", "create", applyObjectiveCreate)
	registerApplyHandler("objective", "update", applyObjectiveUpdate)

	// System handlers
	registerApplyHandler("system", "create", applySystemCreate)
	registerApplyHandler("system", "update", applySystemUpdate)

	// Asset handlers
	registerApplyHandler("asset", "create", applyAssetCreate)
	registerApplyHandler("asset", "update", applyAssetUpdate)

	// Audit finding handlers
	registerApplyHandler("audit_finding", "create", applyAuditFindingCreate)
	registerApplyHandler("audit_finding", "update", applyAuditFindingUpdate)

	// Reading handlers
	registerApplyHandler("risk", "reading", applyRiskReading)
	registerApplyHandler("legal_requirement", "reading", applyLegalReading)

	// Review handlers (supplier review, access review, asset review)
	registerApplyHandler("supplier", "review", applySupplierReviewSuggestion)
	registerApplyHandler("system", "review", applyAccessReviewSuggestion)
	registerApplyHandler("asset", "review", applyAssetReviewSuggestion)
}

// ═══════════════════════════════════════════════════════════════════════
// API HANDLERS
// ═══════════════════════════════════════════════════════════════════════

func (s *Server) handleCreateEntitySuggestion(c echo.Context) error {
	// Contributors and managers create suggestions; readers are read-only (#23).
	// Explicit guard (not just the role middleware) so this can't silently open
	// if the middleware's reader-exception list ever changes.
	if err := requireRole(c, "admin", "manager", "contributor"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	var sg db.Suggestion
	if err := c.Bind(&sg); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// entity_updated_at is server-computed only — below, once the entity is
	// resolved. Without this reset, a client-sent value survives whenever
	// entity_id is empty (the snapshot step is then skipped entirely).
	sg.EntityUpdatedAt = nil
	if sg.EntityType == "" {
		return apiError(http.StatusBadRequest, CodeRequired, Field("entity_type"))
	}
	if sg.SuggestionType == "" {
		return apiError(http.StatusBadRequest, CodeRequired, Field("suggestion_type"))
	}
	if sg.Title == "" {
		return apiError(http.StatusBadRequest, CodeRequired, Field("title"))
	}
	// Types that act on an existing record need it named, or apply can only fail (#200).
	switch sg.SuggestionType {
	case "update", "reassess", "reading", "review", "link":
		if strings.TrimSpace(sg.EntityID) == "" {
			return apiError(http.StatusBadRequest, CodeRequired, Field("entity_id"))
		}
	}

	// Verify apply handler exists for this combination
	if getApplyHandler(sg.EntityType, sg.SuggestionType) == nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("no apply handler for %s:%s", sg.EntityType, sg.SuggestionType))
	}

	sg.SuggestedBy = actor
	if sg.SuggestedByType == "" {
		sg.SuggestedByType = "user"
	}

	// Auto-populate payload title/description from suggestion fields for web UI
	// suggestions. Never for "update": its proposed values live under "fields",
	// and top-level keys would be refused below as the wrong shape (#298).
	if sg.Payload != nil && sg.SuggestionType != "update" {
		var p map[string]interface{}
		json.Unmarshal(sg.Payload, &p)
		if p == nil {
			p = map[string]interface{}{}
		}
		// Suppliers, systems, and assets use "name" not "title"
		switch sg.EntityType {
		case "supplier", "system", "asset":
			if _, ok := p["name"]; !ok && sg.Title != "" {
				p["name"] = sg.Title
			}
		default:
			if _, ok := p["title"]; !ok && sg.Title != "" {
				p["title"] = sg.Title
			}
		}
		// Suppliers have no description (services text lives in notes).
		if _, ok := p["description"]; !ok && sg.Rationale != "" && sg.EntityType != "supplier" {
			p["description"] = sg.Rationale
		}
		sg.Payload, _ = json.Marshal(p)
	}

	// Refuse a payload the apply handler would silently drop (#298, #200).
	if err := validateSuggestionPayload(sg.EntityType, sg.SuggestionType, sg.Payload); err != nil {
		return err
	}
	if err := s.checkObjectiveSuggestionProgram(ctx, orgID, sg.EntityType, sg.SuggestionType, sg.Payload); err != nil {
		return err
	}

	// Snapshot entity_updated_at for stale detection, resolving entity_id (numeric
	// id or per-type identifier, e.g. "ASSET-1") to a primary key the same way
	// the stale check itself does. A resolve failure or a nil snapshot (entity
	// not found, wrong org, ...) is logged but not fatal: the suggestion is still
	// created, same as before this resolve step existed — it's just unchecked.
	if sg.EntityID != "" {
		entityPK, _, err := s.resolveEntityID(ctx, orgID, sg.EntityType, sg.EntityID)
		if err != nil || entityPK <= 0 {
			log.Printf("suggestion create: resolving entity for stale snapshot (org %d, %s %q): %v",
				orgID, sg.EntityType, sg.EntityID, err)
		} else {
			sg.EntityUpdatedAt = s.db.EntityStaleSnapshot(ctx, orgID, sg.EntityType, entityPK)
			if sg.EntityUpdatedAt == nil {
				log.Printf("suggestion create: no stale snapshot for entity (org %d, %s %q, pk %d)",
					orgID, sg.EntityType, sg.EntityID, entityPK)
			}
		}
	}

	if err := s.db.CreateSuggestion(ctx, orgID, &sg); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "suggestion_created",
		Detail: fmt.Sprintf("Suggestion: %s (%s %s)", sg.Title, sg.SuggestionType, sg.EntityType),
	})
	s.notifySuggestionCreated(ctx, orgID, &sg)

	return c.JSON(http.StatusCreated, sg)
}

// taskSuggestionHidden reports whether a suggestion targets a private task the
// caller may not see (mirrors canViewTask). Non-task suggestions and CanSeeAll
// callers are never hidden. Keeps suggestions raised on a private task out of the
// open /suggestions list and single-get for unrelated readers/contributors (#178).
func (s *Server) taskSuggestionHidden(c echo.Context, orgID int, sg *db.Suggestion) bool {
	if sg.EntityType != "task" || sg.EntityID == "" || taskViewer(c).CanSeeAll {
		return false
	}
	var t *db.Task
	if id, err := strconv.ParseInt(sg.EntityID, 10, 64); err == nil {
		t, _ = s.db.GetTask(c.Request().Context(), orgID, id)
	} else {
		t, _ = s.db.GetTaskByIdentifier(c.Request().Context(), orgID, sg.EntityID)
	}
	return t != nil && !canViewTask(c, t)
}

func (s *Server) handleListEntitySuggestions(c echo.Context) error {
	orgID := getOrgID(c)

	involving, err := involvingParam(c)
	if err != nil {
		return err
	}
	filters := db.SuggestionFilters{
		Status:             c.QueryParam("status"),
		EntityType:         c.QueryParam("entity_type"),
		EntityID:           c.QueryParam("entity_id"),
		SuggestedBy:        c.QueryParam("suggested_by"),
		SuggestedByType:    c.QueryParam("suggested_by_type"),
		Involving:          involving,
		InvolvingCanReview: involvingCanApprove(c),
	}
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filters.Limit = n
		}
	}

	suggestions, err := s.db.ListSuggestions(c.Request().Context(), orgID, filters)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	// Drop suggestions raised on a private task the caller may not see (#178 review).
	// taskSuggestionHidden short-circuits for managers/admins, so this is a no-op
	// (no extra DB hits) for CanSeeAll viewers.
	kept := make([]db.Suggestion, 0, len(suggestions))
	for i := range suggestions {
		if s.taskSuggestionHidden(c, orgID, &suggestions[i]) {
			continue
		}
		kept = append(kept, suggestions[i])
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": kept})
}

func (s *Server) handleGetEntitySuggestion(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidID)
	}

	sg, err := s.db.GetSuggestion(c.Request().Context(), orgID, id)
	if err != nil {
		return apiError(http.StatusNotFound, CodeNotFound, Entity("suggestion"))
	}
	// A suggestion on a private task is 404 for someone who can't see the task (#178).
	if s.taskSuggestionHidden(c, orgID, sg) {
		return apiError(http.StatusNotFound, CodeNotFound, Entity("suggestion"))
	}

	// Attach stale info if entity has changed
	resp := map[string]interface{}{"data": sg}
	if sg.EntityID != "" && sg.EntityUpdatedAt != nil && sg.Status != "applied" && sg.Status != "rejected" {
		entityIDInt, _, err := s.resolveEntityID(c.Request().Context(), orgID, sg.EntityType, sg.EntityID)
		if err == nil && entityIDInt > 0 {
			changes, _ := s.db.EntityChangesAfter(c.Request().Context(), orgID, sg.EntityType, entityIDInt, *sg.EntityUpdatedAt)
			if len(changes) > 0 {
				resp["stale"] = true
				resp["stale_changes"] = changes
			}
		}
	}

	return c.JSON(http.StatusOK, resp)
}

// updateSuggestionRequest is the editable surface of PUT /suggestions/:id.
// Pointer fields distinguish "field absent" (keep the stored value) from
// "field sent empty" (an intentional clear) — see #204.
type updateSuggestionRequest struct {
	Title      *string         `json:"title"`
	Payload    json.RawMessage `json:"payload"`
	Rationale  *string         `json:"rationale"`
	SourceRefs json.RawMessage `json:"source_refs"`
}

// normalizeRawJSON collapses an explicit JSON null to a nil RawMessage so the
// column is stored as SQL NULL instead of the literal string "null".
func normalizeRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}

func (s *Server) handleUpdateEntitySuggestion(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)
	role := getRole(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidID)
	}

	existing, err := s.db.GetSuggestion(ctx, orgID, id)
	if err != nil {
		return apiError(http.StatusNotFound, CodeNotFound, Entity("suggestion"))
	}

	// RBAC: contributor can edit own open only, manager/admin can edit any open/in_review
	if role == "reader" {
		return echo.NewHTTPError(http.StatusForbidden, "readers cannot edit suggestions")
	}
	if role == "contributor" {
		if existing.SuggestedBy != actor {
			return echo.NewHTTPError(http.StatusForbidden, "contributors can only edit their own suggestions")
		}
		if existing.Status != "open" {
			return echo.NewHTTPError(http.StatusForbidden, "contributors can only edit open suggestions")
		}
	}

	var req updateSuggestionRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Only the four editable columns are taken from the request; everything else
	// on the row is server-owned. Absent fields keep their current value so a
	// partial edit cannot blank what it did not mention (#204).
	update := db.Suggestion{
		ID:         id,
		Title:      existing.Title,
		Payload:    existing.Payload,
		Rationale:  existing.Rationale,
		SourceRefs: existing.SourceRefs,
	}
	// A blank title is never meaningful, so empty is treated as "unchanged"
	// rather than as a clear.
	if req.Title != nil && *req.Title != "" {
		update.Title = *req.Title
	}
	if payload := normalizeRawJSON(req.Payload); payload != nil {
		// Same check as create (#298, #200): an edit must not turn a suggestion
		// into one whose apply would silently drop what it proposes.
		if err := validateSuggestionPayload(existing.EntityType, existing.SuggestionType, payload); err != nil {
			return err
		}
		if err := s.checkObjectiveSuggestionProgram(ctx, orgID, existing.EntityType, existing.SuggestionType, payload); err != nil {
			return err
		}
		update.Payload = payload
	}
	// rationale and source_refs are optional free-form fields: sending them
	// explicitly as "" / [] / null clears them, omitting them keeps them.
	if req.Rationale != nil {
		update.Rationale = *req.Rationale
	}
	if req.SourceRefs != nil {
		update.SourceRefs = normalizeRawJSON(req.SourceRefs)
	}

	if err := s.db.UpdateSuggestion(ctx, orgID, &update); err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleDeleteEntitySuggestion(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)
	role := getRole(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidID)
	}

	existing, err := s.db.GetSuggestion(ctx, orgID, id)
	if err != nil {
		return apiError(http.StatusNotFound, CodeNotFound, Entity("suggestion"))
	}

	// RBAC: author can delete own open/withdrawn, manager/admin can delete non-terminal
	isAuthor := existing.SuggestedBy == actor
	isManager := role == "admin" || role == "manager"

	if !isAuthor && !isManager {
		return echo.NewHTTPError(http.StatusForbidden, "not authorized to delete this suggestion")
	}
	if isAuthor && !isManager && existing.Status != "open" && existing.Status != "withdrawn" {
		return echo.NewHTTPError(http.StatusForbidden, "can only delete your own open or withdrawn suggestions")
	}

	if err := s.db.DeleteSuggestion(ctx, orgID, id); err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "suggestion_deleted",
		Detail: fmt.Sprintf("Suggestion #%d: %s", id, existing.Title),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleClaimEntitySuggestion(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidID)
	}

	newStatus, err := s.db.ClaimSuggestion(ctx, orgID, id, actor)
	if err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"status": newStatus})
}

// applyPostCommitKey carries a queue of side-effect callbacks that must run only
// AFTER the apply transaction commits. Apply handlers use registerApplyPostCommit
// for effects that touch the pool directly or send mail (e.g. auto-created tasks),
// which must not fire inside WithOrgTx — mirroring how the HTTP handlers run their
// post-commit work. Keeps the HTTP and suggestion-apply paths behaving identically.
type applyPostCommitKey struct{}

// registerApplyPostCommit queues fn to run after the enclosing apply tx commits.
// A no-op if the context carries no queue (defensive; the apply path always sets one).
func registerApplyPostCommit(ctx context.Context, fn func()) {
	if q, ok := ctx.Value(applyPostCommitKey{}).(*[]func()); ok {
		*q = append(*q, fn)
	}
}

func (s *Server) handleApplyEntitySuggestion(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	// Callbacks registered by apply handlers for side effects that must run only
	// after the tx commits (task auto-creation, mail). Drained below on success.
	var postCommit []func()
	ctx = context.WithValue(ctx, applyPostCommitKey{}, &postCommit)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidID)
	}

	sg, err := s.db.GetSuggestion(ctx, orgID, id)
	if err != nil {
		return apiError(http.StatusNotFound, CodeNotFound, Entity("suggestion"))
	}
	if sg.Status != "open" && sg.Status != "in_review" {
		return echo.NewHTTPError(http.StatusConflict, "suggestion is in terminal state: "+sg.Status)
	}

	// An update is only "applied" if it proposes values the handler writes
	// (#298, #200). Checked here too, not only at create, because suggestions
	// stored before this check can hold any payload. The suggestion stays open.
	if sg.SuggestionType == "update" {
		if err := validateUpdatePayload(sg.EntityType, sg.Payload, true); err != nil {
			return err
		}
	}

	// Check for force flag if stale
	var body struct {
		Force bool `json:"force"`
	}
	_ = c.Bind(&body)

	if sg.EntityID != "" && sg.EntityUpdatedAt != nil && !body.Force {
		entityIDInt, _, err := s.resolveEntityID(ctx, orgID, sg.EntityType, sg.EntityID)
		if err == nil && entityIDInt > 0 {
			changes, _ := s.db.EntityChangesAfter(ctx, orgID, sg.EntityType, entityIDInt, *sg.EntityUpdatedAt)
			if len(changes) > 0 {
				return c.JSON(http.StatusOK, map[string]interface{}{
					"stale":         true,
					"stale_changes": changes,
					"message":       "Entity has changed since this suggestion was created. Re-submit with force=true to apply anyway.",
				})
			}
		}
	}

	// Dispatch to apply handler
	handler := getApplyHandler(sg.EntityType, sg.SuggestionType)
	if handler == nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("no apply handler for %s:%s", sg.EntityType, sg.SuggestionType))
	}

	// Atomic: entity mutation + changelog + suggestion mark — all in one transaction with RLS
	var appliedEntityID string
	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var appliedID int64
		var err error
		appliedEntityID, appliedID, err = handler(ctx, tx, s, orgID, sg, actor)
		if err != nil {
			return fmt.Errorf("apply: %w", err)
		}

		// Changelog linking suggestion to entity
		if appliedID > 0 {
			if err := db.LogChangeTx(ctx, tx, orgID, &db.ChangelogEntry{
				EntityType: sg.EntityType,
				EntityID:   appliedID,
				Action:     "suggestion_applied",
				ChangedBy:  actor,
				Reason:     fmt.Sprintf("Applied suggestion #%d: %s", sg.ID, sg.Title),
			}); err != nil {
				return fmt.Errorf("changelog: %w", err)
			}
		}

		// Mark suggestion as applied
		if err := db.ApplySuggestionTx(ctx, tx, orgID, id, actor, appliedEntityID); err != nil {
			return fmt.Errorf("mark applied: %w", err)
		}

		return nil
	})
	if txErr != nil {
		// Preserve an actionable status: validation returns *echo.HTTPError (400
		// with the allowed list); the incident open-CA guard is a 409, the same
		// status the direct incident endpoint returns for the same rule; a DB
		// constraint violation or db.ValidationError maps via pgxHTTPError;
		// anything else is a genuine 500.
		var he *echo.HTTPError
		if errors.As(txErr, &he) {
			return he
		}
		var oce openCAsLinkedError
		if errors.As(txErr, &oce) {
			return echo.NewHTTPError(http.StatusConflict, oce.Error())
		}
		var ote openTasksLinkedError
		if errors.As(txErr, &ote) {
			return echo.NewHTTPError(http.StatusConflict, ote.Error())
		}
		return pgxHTTPError(txErr)
	}

	// Post-commit: run side effects handlers deferred until the tx committed
	// (auto-created tasks, mail), then the activity log + notifications. All
	// non-transactional and OK to fail — the entity mutation is already durable.
	for _, fn := range postCommit {
		fn()
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "suggestion_applied",
		Detail: fmt.Sprintf("Applied suggestion #%d: %s → %s %s", sg.ID, sg.Title, sg.EntityType, appliedEntityID),
	})
	s.notifySuggestionResolved(ctx, orgID, sg, "applied",
		fmt.Sprintf("Your suggestion \"%s\" was applied by %s → %s %s", sg.Title, actor, sg.EntityType, appliedEntityID),
		NotifyKeySuggestionAppliedBody,
		map[string]any{"title": sg.Title, "actor": actor, "entity": sg.EntityType, "id": appliedEntityID})

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":            "applied",
		"applied_entity_id": appliedEntityID,
	})
}

func (s *Server) handleRejectEntitySuggestion(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidID)
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(body.Reason) == "" {
		return apiError(http.StatusBadRequest, CodeRequired, Field("reason"))
	}

	if err := s.db.RejectEntitySuggestion(ctx, orgID, id, actor, body.Reason); err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}

	sg, _ := s.db.GetSuggestion(ctx, orgID, id)
	title := fmt.Sprintf("#%d", id)
	if sg != nil {
		title = sg.Title
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "suggestion_rejected",
		Detail: fmt.Sprintf("Rejected suggestion: %s — %s", title, body.Reason),
	})
	if sg != nil {
		// `reason` is the reviewer's own words — verbatim, like a review note.
		s.notifySuggestionResolved(ctx, orgID, sg, "rejected",
			fmt.Sprintf("Your suggestion \"%s\" was rejected by %s: %s", sg.Title, actor, body.Reason),
			NotifyKeySuggestionRejectedBody,
			map[string]any{"title": sg.Title, "actor": actor, "reason": body.Reason})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "rejected"})
}

func (s *Server) handleWithdrawEntitySuggestion(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidID)
	}

	if err := s.db.WithdrawSuggestion(ctx, orgID, id, actor); err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "withdrawn"})
}

// ═══════════════════════════════════════════════════════════════════════
// HELPERS
// ═══════════════════════════════════════════════════════════════════════

func getRole(c echo.Context) string {
	role, _ := c.Get("user_role").(string)
	return role
}

// notifySuggestionCreated sends notifications to entity owner and org managers.
func (s *Server) notifySuggestionCreated(ctx context.Context, orgID int, sg *db.Suggestion) {
	link := fmt.Sprintf("/inbox/suggestions?id=%d", sg.ID)
	body := fmt.Sprintf("%s suggested: %s (%s %s)", sg.SuggestedBy, sg.Title, sg.SuggestionType, sg.EntityType)

	// suggestion_type and entity are enum values, so they are resolved through
	// the shared enum/entity keys before interpolation — splicing the English
	// "risk" into a translated frame yields half-translated output.
	params := map[string]any{
		"actor":           sg.SuggestedBy,
		"title":           sg.Title,
		"suggestion_type": sg.SuggestionType,
		"entity":          sg.EntityType,
	}

	// Notify managers
	users, _ := s.db.ListOrgUsers(ctx, orgID)
	for _, u := range users {
		if (u.Role == "admin" || u.Role == "manager") && u.Email != sg.SuggestedBy {
			_ = s.db.CreateNotificationContentByEmail(ctx, orgID, u.Email, db.NotificationContent{
				Title:    "New suggestion",
				TitleKey: NotifyKeySuggestionNew,
				Body:     body,
				BodyKey:  NotifyKeySuggestionNewBody,
				Params:   params,
				Link:     link,
			})
		}
	}
}

// notifySuggestionResolved sends notification to the original suggester.
//
// bodyKey and bodyParams come from the caller because applied and rejected say
// genuinely different things — one names the entity it landed on, the other
// carries the reviewer's reason. The title frame is shared, with `action` as a
// translatable param.
func (s *Server) notifySuggestionResolved(ctx context.Context, orgID int, sg *db.Suggestion, action, detail, bodyKey string, bodyParams map[string]any) {
	link := fmt.Sprintf("/inbox/suggestions?id=%d", sg.ID)
	title := fmt.Sprintf("Suggestion %s", action)
	params := map[string]any{"action": action}
	for k, v := range bodyParams {
		params[k] = v
	}
	_ = s.db.CreateNotificationContentByEmail(ctx, orgID, sg.SuggestedBy, db.NotificationContent{
		Title:    title,
		TitleKey: NotifyKeySuggestionResolved,
		Body:     detail,
		BodyKey:  bodyKey,
		Params:   params,
		Link:     link,
	})
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: RISKS
// ═══════════════════════════════════════════════════════════════════════

// riskCreatePayload is the payload of a risk:create suggestion: the POST /risks
// body (#200). Decoded strictly.
type riskCreatePayload struct {
	riskCreateRequest
}

func applyRiskCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var p riskCreatePayload
	if err := decodeSuggestionPayload(sg.Payload, &p); err != nil {
		return "", 0, err
	}
	if p.Title == "" {
		return "", 0, errRequired("title")
	}
	risk := db.Risk{
		Title:                         p.Title,
		Description:                   p.Description,
		RiskType:                      p.RiskType,
		Origin:                        p.Origin,
		Category:                      p.Category,
		CustomFields:                  p.CustomFields,
		CurrentLikelihood:             p.CurrentLikelihood,
		CurrentImpact:                 p.CurrentImpact,
		ConfidentialityImpact:         p.ConfidentialityImpact,
		IntegrityImpact:               p.IntegrityImpact,
		AvailabilityImpact:            p.AvailabilityImpact,
		InherentLikelihood:            p.InherentLikelihood,
		InherentImpact:                p.InherentImpact,
		InherentConfidentialityImpact: p.InherentConfidentialityImpact,
		InherentIntegrityImpact:       p.InherentIntegrityImpact,
		InherentAvailabilityImpact:    p.InherentAvailabilityImpact,
		TargetLikelihood:              p.TargetLikelihood,
		TargetImpact:                  p.TargetImpact,
		Treatment:                     p.Treatment,
		TreatmentPlan:                 p.TreatmentPlan,
		TreatmentDueDate:              p.TreatmentDueDate,
		Owner:                         p.Owner,
		Status:                        p.Status,
		LastReview:                    p.LastReview,
		NextReview:                    p.NextReview,
		Notes:                         p.Notes,
		ExternalID:                    p.ExternalID,
	}
	applyRiskDefaults(&risk, actor)
	// Apply-only default: an agent proposing a risk without a score gets the
	// mid-point rather than an unscored risk.
	if risk.CurrentLikelihood == nil {
		l := 3
		risk.CurrentLikelihood = &l
	}
	if risk.CurrentImpact == nil {
		i := 3
		risk.CurrentImpact = &i
	}
	// No default for Category: the column is nullable and empty is a valid
	// "uncategorised". Picking the first configured category would be
	// order-dependent and surprising.
	if err := validateRiskCreate(&risk, s.riskCategoryKeys(ctx, orgID)); err != nil {
		return "", 0, err
	}
	defs := s.customFieldDefs(ctx, orgID)
	// checkRequired=false, deliberately: agents cannot fill out a form, so a
	// required custom field must never block an agent-created risk. See
	// "Required fields are in scope" in the implementation plan for #213/#216.
	if err := db.ValidateCustomFieldValues(defs, p.CustomFields, false); err != nil {
		return "", 0, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	risk.CustomFields = db.NormalizeCustomFieldValues(defs, p.CustomFields)
	// Only an owner the payload names is checked; the defaulted owner is the
	// acting user, who is a member by construction.
	if err := s.validateOrgMemberIn(ctx, orgID, p.Owner); err != nil {
		return "", 0, err
	}
	refs, err := s.validateReferenceInputs(ctx, orgID, db.TaskViewer{Email: actor, CanSeeAll: true}, p.References)
	if err != nil {
		return "", 0, err
	}
	if err := db.CreateRiskTx(ctx, tx, orgID, &risk, s.db.RiskReviewCycles(ctx, orgID)); err != nil {
		return "", 0, err
	}
	if err := s.createReferencesTx(ctx, tx, orgID, "risk", risk.Identifier, actor, refs); err != nil {
		return "", 0, err
	}
	if err := db.LogChangeTx(ctx, tx, orgID, &db.ChangelogEntry{
		EntityType: "risk", EntityID: risk.ID, Action: "create", ChangedBy: actor,
	}); err != nil {
		return "", 0, err
	}
	return risk.Identifier, risk.ID, nil
}

func applyRiskReassess(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var payload struct {
		CurrentLikelihood *int   `json:"current_likelihood"`
		CurrentImpact     *int   `json:"current_impact"`
		Reason            string `json:"reason"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", 0, fmt.Errorf("invalid reassess payload: %w", err)
	}

	risk, err := s.db.GetRiskByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, fmt.Errorf("risk %s not found: %w", sg.EntityID, err)
	}

	old := risk.ToChangeMap()
	if payload.CurrentLikelihood != nil {
		risk.CurrentLikelihood = payload.CurrentLikelihood
	}
	if payload.CurrentImpact != nil {
		risk.CurrentImpact = payload.CurrentImpact
	}
	if err := db.UpdateRiskTx(ctx, tx, orgID, risk, s.db.RiskReviewCycles(ctx, orgID), nil); err != nil {
		return "", 0, err
	}

	diffs := db.DiffFields("risk", int64(risk.ID), actor, fmt.Sprintf("suggestion #%d: %s", sg.ID, payload.Reason), old, risk.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", 0, err
	}

	return risk.Identifier, risk.ID, nil
}

func applyRiskUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var req riskUpdateRequest
	if err := decodeUpdateFields(sg.EntityType, sg.Payload, &req); err != nil {
		return "", 0, err
	}
	id, err := s.resolveRiskID(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, errNotFound("risk")
	}
	old, err := s.db.GetRisk(ctx, orgID, id)
	if err != nil {
		return "", 0, errNotFound("risk")
	}
	updated, explicitNextReview, err := s.prepareRiskUpdate(ctx, orgID, old, &req, actor)
	if err != nil {
		return "", 0, err
	}
	updated.ID = id
	if err := db.UpdateRiskTx(ctx, tx, orgID, &updated, s.db.RiskReviewCycles(ctx, orgID), explicitNextReview); err != nil {
		return "", 0, err
	}
	defs := s.customFieldDefs(ctx, orgID)
	diffs := db.DiffFields("risk", id, actor, fmt.Sprintf("suggestion #%d", sg.ID), old.ToChangeMap(defs...), updated.ToChangeMap(defs...))
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", 0, err
	}
	return updated.Identifier, updated.ID, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: INCIDENTS
// ═══════════════════════════════════════════════════════════════════════

// incidentCreatePayload is the payload of an incident:create suggestion: the
// POST /incidents body (#200). "summary" and "affected_systems" are no longer
// accepted: they were decoded and silently discarded. Decoded strictly.
type incidentCreatePayload struct {
	incidentCreateRequest
}

func applyIncidentCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var p incidentCreatePayload
	if err := decodeSuggestionPayload(sg.Payload, &p); err != nil {
		return "", 0, err
	}
	if p.Title == "" {
		return "", 0, errRequired("title")
	}
	inc := db.Incident{
		Title:               p.Title,
		Description:         p.Description,
		Severity:            p.Severity,
		Status:              p.Status,
		AffectsC:            p.AffectsC,
		AffectsI:            p.AffectsI,
		AffectsA:            p.AffectsA,
		IncidentType:        p.IncidentType,
		Source:              p.Source,
		Notes:               p.Notes,
		DataBreach:          p.DataBreach,
		GDPRRole:            p.GDPRRole,
		AuthorityNotified:   p.AuthorityNotified,
		AuthorityNotifiedAt: p.AuthorityNotifiedAt,
		SubjectsNotified:    p.SubjectsNotified,
		SubjectsNotifiedAt:  p.SubjectsNotifiedAt,
		Reporter:            p.Reporter,
		Assignee:            p.Assignee,
		DetectedAt:          p.DetectedAt,
		RootCause:           p.RootCause,
		LessonsLearned:      p.LessonsLearned,
		ExternalID:          p.ExternalID,
	}
	s.applyIncidentDefaults(ctx, orgID, &inc, actor)
	if err := validateIncidentCreate(&inc); err != nil {
		return "", 0, err
	}
	// Only an assignee the payload names is checked; the defaulted one is the
	// acting user, who is a member by construction.
	if err := s.validateOrgMemberIn(ctx, orgID, p.Assignee); err != nil {
		return "", 0, err
	}
	refs, err := s.validateReferenceInputs(ctx, orgID, db.TaskViewer{Email: actor, CanSeeAll: true}, p.References)
	if err != nil {
		return "", 0, err
	}
	if err := db.CreateIncidentTx(ctx, tx, orgID, &inc); err != nil {
		return "", 0, err
	}
	if err := s.createReferencesTx(ctx, tx, orgID, "incident", inc.Identifier, inc.Reporter, refs); err != nil {
		return "", 0, err
	}
	if err := db.LogChangeTx(ctx, tx, orgID, &db.ChangelogEntry{
		EntityType: "incident", EntityID: int64(inc.ID), Action: "create", ChangedBy: inc.Reporter,
	}); err != nil {
		return "", 0, err
	}
	return inc.Identifier, inc.ID, nil
}

func applyIncidentUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var req incidentUpdateRequest
	if err := decodeUpdateFields(sg.EntityType, sg.Payload, &req); err != nil {
		return "", 0, err
	}
	incID, err := s.resolveIncidentID(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, errNotFound("incident")
	}
	old, err := s.db.GetIncident(ctx, orgID, incID)
	if err != nil {
		return "", 0, errNotFound("incident")
	}
	updated, err := s.prepareIncidentUpdate(ctx, orgID, old, &req)
	if err != nil {
		return "", 0, err
	}
	updated.ID = incID

	// Unified write path (#26): same open-CA guard + lifecycle timestamps the
	// HTTP handler enforces.
	if err := enforceIncidentWriteTx(ctx, tx, orgID, &updated, old.Status); err != nil {
		return "", 0, err
	}
	after, err := db.GetIncidentTx(ctx, tx, orgID, incID)
	if err != nil {
		return "", 0, err
	}
	diffs := db.DiffFields("incident", int64(incID), actor, fmt.Sprintf("suggestion #%d", sg.ID), old.ToChangeMap(), after.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", 0, err
	}
	return after.Identifier, after.ID, nil
}

func applyIncidentLink(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var payload struct {
		Links []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"links"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", 0, fmt.Errorf("invalid link payload: %w", err)
	}
	incID, err := s.resolveIncidentID(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, fmt.Errorf("incident %s not found: %w", sg.EntityID, err)
	}

	// Store the source under the incident's identifier, as POST /references
	// requires: sg.EntityID may be the numeric row id, which resolveIncidentID
	// accepts but which does not resolve as a reference (#341, #346).
	inc, err := s.db.GetIncident(ctx, orgID, incID)
	if err != nil {
		return "", 0, fmt.Errorf("incident %s not found: %w", sg.EntityID, err)
	}
	viewer := db.TaskViewer{Email: actor, CanSeeAll: true} // apply is manager/admin-only

	var linked int
	for _, link := range payload.Links {
		// Store the canonical id (key / display id for program and objective), not
		// the raw one: link.ID may be a numeric row id (#350).
		targetID, found := s.canonicalReferenceID(ctx, orgID, viewer, link.Type, link.ID)
		if !found {
			return "", 0, apiError(http.StatusBadRequest, CodeNotFoundInOrg, Entity(link.Type))
		}
		if err := db.CreateReferenceTx(ctx, tx, orgID, &db.EntityReference{
			SourceType: "incident",
			SourceID:   inc.Identifier,
			TargetType: link.Type,
			TargetID:   targetID,
		}); err != nil {
			return "", 0, fmt.Errorf("failed to link %s %s: %w", link.Type, targetID, err)
		}
		linked++
	}
	if linked == 0 {
		return "", 0, fmt.Errorf("no links in payload")
	}

	return sg.EntityID, incID, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: SUPPLIERS
// ═══════════════════════════════════════════════════════════════════════

// supplierCreatePayload is the payload of a supplier:create suggestion: the POST
// /suppliers body (#200). Decoded strictly.
type supplierCreatePayload struct {
	supplierCreateRequest
	// Suppliers have no description; the services text lives in notes. Accepted
	// and ignored: SuggestNewButton and the server's rationale copy both send it,
	// and suggestions stored before #298 carry it.
	Description string `json:"description"`
}

func applySupplierCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var p supplierCreatePayload
	if err := decodeSuggestionPayload(sg.Payload, &p); err != nil {
		return "", 0, err
	}
	if p.Name == "" {
		return "", 0, errRequired("name")
	}
	sup := db.Supplier{
		Name:            p.Name,
		SupplierType:    p.SupplierType,
		Criticality:     p.Criticality,
		DataAccess:      p.DataAccess,
		Contact:         p.Contact,
		ContractRef:     p.ContractRef,
		Status:          p.Status,
		Owner:           p.Owner,
		ContractExpiry:  p.ContractExpiry,
		Confidentiality: p.Confidentiality,
		Integrity:       p.Integrity,
		Availability:    p.Availability,
		LastReview:      p.LastReview,
		NextReview:      p.NextReview,
		Notes:           p.Notes,
		ExternalID:      p.ExternalID,
	}
	applySupplierDefaults(&sup, actor)
	if err := validateSupplierCreate(&sup); err != nil {
		return "", 0, err
	}
	refs, err := s.validateReferenceInputs(ctx, orgID, db.TaskViewer{Email: actor, CanSeeAll: true}, p.References)
	if err != nil {
		return "", 0, err
	}
	if err := db.CreateSupplierTx(ctx, tx, orgID, &sup, s.db.SupplierReviewCycles(ctx, orgID)); err != nil {
		return "", 0, err
	}
	if err := s.createReferencesTx(ctx, tx, orgID, "supplier", sup.Identifier, actor, refs); err != nil {
		return "", 0, err
	}
	if err := db.LogChangeTx(ctx, tx, orgID, &db.ChangelogEntry{
		EntityType: "supplier", EntityID: sup.ID, Action: "create", ChangedBy: actor,
	}); err != nil {
		return "", 0, err
	}
	return sup.Identifier, sup.ID, nil
}

func applySupplierUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var req supplierUpdateRequest
	if err := decodeUpdateFields(sg.EntityType, sg.Payload, &req); err != nil {
		return "", 0, err
	}
	id, err := s.resolveSupplierID(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, errNotFound("supplier")
	}
	old, err := s.db.GetSupplier(ctx, orgID, id)
	if err != nil {
		return "", 0, errNotFound("supplier")
	}
	updated, explicitNextReview, err := s.prepareSupplierUpdate(ctx, orgID, old, &req)
	if err != nil {
		return "", 0, err
	}
	updated.ID = id
	if err := db.UpdateSupplierTx(ctx, tx, orgID, &updated, s.db.SupplierReviewCycles(ctx, orgID), explicitNextReview); err != nil {
		return "", 0, err
	}
	diffs := db.DiffFields("supplier", id, actor, fmt.Sprintf("suggestion #%d", sg.ID), old.ToChangeMap(), updated.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", 0, err
	}
	return updated.Identifier, updated.ID, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: LEGAL
// ═══════════════════════════════════════════════════════════════════════

// legalCreatePayload is the payload of a legal_requirement:create suggestion: the
// POST /legal body (#200). Decoded strictly.
type legalCreatePayload struct {
	legalCreateRequest
}

func applyLegalCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var p legalCreatePayload
	if err := decodeSuggestionPayload(sg.Payload, &p); err != nil {
		return "", 0, err
	}
	if p.Title == "" {
		return "", 0, errRequired("title")
	}
	lr := db.LegalRequirement{
		Title:             p.Title,
		Description:       p.Description,
		Jurisdiction:      p.Jurisdiction,
		Category:          p.Category,
		Reference:         p.Reference,
		URL:               p.URL,
		Status:            p.Status,
		Owner:             p.Owner,
		LastReview:        p.LastReview,
		NextReview:        p.NextReview,
		Notes:             p.Notes,
		CurrentLikelihood: p.CurrentLikelihood,
		CurrentImpact:     p.CurrentImpact,
		Treatment:         p.Treatment,
		TreatmentPlan:     p.TreatmentPlan,
		TargetLikelihood:  p.TargetLikelihood,
		TargetImpact:      p.TargetImpact,
		Completion:        p.Completion,
		ExternalID:        p.ExternalID,
	}
	applyLegalDefaults(&lr, actor)
	if err := validateLegalCreate(&lr); err != nil {
		return "", 0, err
	}
	refs, err := s.validateReferenceInputs(ctx, orgID, db.TaskViewer{Email: actor, CanSeeAll: true}, p.References)
	if err != nil {
		return "", 0, err
	}
	if err := db.CreateLegalRequirementTx(ctx, tx, orgID, &lr, s.db.RiskReviewCycles(ctx, orgID)); err != nil {
		return "", 0, err
	}
	if err := s.createReferencesTx(ctx, tx, orgID, "legal_requirement", lr.Identifier, actor, refs); err != nil {
		return "", 0, err
	}
	if err := db.LogChangeTx(ctx, tx, orgID, &db.ChangelogEntry{
		EntityType: "legal_requirement", EntityID: int64(lr.ID), Action: "create", ChangedBy: actor,
	}); err != nil {
		return "", 0, err
	}
	return lr.Identifier, lr.ID, nil
}

func applyLegalUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var req legalUpdateRequest
	if err := decodeUpdateFields(sg.EntityType, sg.Payload, &req); err != nil {
		return "", 0, err
	}
	id, err := s.resolveLegalID(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, errNotFound("legal_requirement")
	}
	old, err := s.db.GetLegalRequirement(ctx, orgID, id)
	if err != nil {
		return "", 0, errNotFound("legal_requirement")
	}
	updated, explicitNextReview, err := s.prepareLegalUpdate(ctx, orgID, old, &req)
	if err != nil {
		return "", 0, err
	}
	updated.ID = id
	if err := db.UpdateLegalRequirementTx(ctx, tx, orgID, &updated, s.db.RiskReviewCycles(ctx, orgID), explicitNextReview); err != nil {
		return "", 0, err
	}
	diffs := db.DiffFields("legal_requirement", id, actor, fmt.Sprintf("suggestion #%d", sg.ID), old.ToChangeMap(), updated.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", 0, err
	}
	return updated.Identifier, updated.ID, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: CHANGE REQUESTS
// ═══════════════════════════════════════════════════════════════════════

// changeCreatePayload is the payload of a change_request:create suggestion. Decoded strictly,
// so a key not listed here is refused instead of silently dropped (#200).
type changeCreatePayload struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	Justification string `json:"justification"`
	Priority      string `json:"priority"`
	Category      string `json:"category"`
	RiskLevel     string `json:"risk_level"`
	RollbackPlan  string `json:"rollback_plan"`
	AssignedTo    string `json:"assigned_to"`
}

func applyChangeCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var payload changeCreatePayload
	if err := decodeSuggestionPayload(sg.Payload, &payload); err != nil {
		return "", 0, fmt.Errorf("invalid change payload: %w", err)
	}
	if payload.Title == "" {
		return "", 0, fmt.Errorf("title is required")
	}
	cr := db.ChangeRequest{
		Title: payload.Title, Description: payload.Description,
		Justification: payload.Justification,
		Priority:      payload.Priority, Category: payload.Category,
		RiskLevel: payload.RiskLevel, RollbackPlan: payload.RollbackPlan,
		RequestedBy: actor, AssignedTo: payload.AssignedTo, Status: "proposed",
	}
	if cr.Status == "" {
		cr.Status = "proposed"
	}
	if cr.Priority == "" {
		cr.Priority = "medium"
	}
	if cr.Category == "" {
		cr.Category = "process"
	}
	if cr.RiskLevel == "" {
		cr.RiskLevel = "low"
	}
	if err := db.CreateChangeRequestTx(ctx, tx, orgID, &cr); err != nil {
		return "", 0, err
	}
	return cr.Identifier, int64(cr.ID), nil
}

func applyChangeUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", 0, fmt.Errorf("invalid update payload: %w", err)
	}
	id, err := s.resolveChangeID(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, fmt.Errorf("change request %s not found: %w", sg.EntityID, err)
	}
	cr, err := s.db.GetChangeRequest(ctx, orgID, int(id))
	if err != nil {
		return "", 0, fmt.Errorf("change request %s not found: %w", sg.EntityID, err)
	}
	if v, ok := payload.Fields["type"]; ok {
		if sv, ok := v.(string); ok {
			if err := validateEnum("type", sv, db.ChangeTypes); err != nil {
				return "", 0, err
			}
			cr.Type = sv
		}
	}
	if v, ok := payload.Fields["priority"]; ok {
		if sv, ok := v.(string); ok {
			cr.Priority = sv
		}
	}
	if v, ok := payload.Fields["risk_level"]; ok {
		if sv, ok := v.(string); ok {
			cr.RiskLevel = sv
		}
	}
	if v, ok := payload.Fields["rollback_plan"]; ok {
		if sv, ok := v.(string); ok {
			cr.RollbackPlan = sv
		}
	}
	if v, ok := payload.Fields["assigned_to"]; ok {
		if sv, ok := v.(string); ok {
			cr.AssignedTo = sv
		}
	}
	// Status transitions go through the shared enforced path so approved_at/by and
	// implemented_at are derived exactly as the HTTP handler does — a plain field
	// write (UpdateChangeRequestTx doesn't touch status) would skip that metadata.
	if v, ok := payload.Fields["status"]; ok {
		if sv, ok := v.(string); ok && sv != cr.Status {
			if err := validateEnum("status", sv, db.ChangeStatuses); err != nil {
				return "", 0, err
			}
			if err := db.UpdateChangeRequestStatusTx(ctx, tx, orgID, cr.ID, sv, actor); err != nil {
				return "", 0, err
			}
			cr.Status = sv
			// The HTTP status paths auto-create the "Implement <CR>" task on
			// approval; keep suggestion-apply identical (#26 acceptance criterion).
			// Deferred to post-commit — createChangeFollowupTask writes via the pool
			// and sends mail, neither safe inside this transaction.
			if sv == "approved" {
				registerApplyPostCommit(ctx, func() {
					s.createChangeFollowupTask(context.Background(), orgID, cr, actor)
				})
			}
		}
	}
	if err := db.UpdateChangeRequestTx(ctx, tx, orgID, cr.ID, cr); err != nil {
		return "", 0, err
	}
	return cr.Identifier, int64(cr.ID), nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: CORRECTIVE ACTIONS
// ═══════════════════════════════════════════════════════════════════════

// correctiveActionCreatePayload is the payload of a corrective_action:create
// suggestion: the POST /corrective-actions body (#200). Decoded strictly.
type correctiveActionCreatePayload struct {
	correctiveActionCreateRequest
}

func applyCorrActiveCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var p correctiveActionCreatePayload
	if err := decodeSuggestionPayload(sg.Payload, &p); err != nil {
		return "", 0, err
	}
	if p.Title == "" {
		return "", 0, errRequired("title")
	}
	ca := db.CorrectiveAction{
		Title:       p.Title,
		Description: p.Description,
		Source:      p.Source,
		Severity:    p.Severity,
		Status:      p.Status,
		Assignee:    p.Assignee,
		DueDate:     p.DueDate,
		RootCause:   p.RootCause,
		Notes:       p.Notes,
		ExternalID:  p.ExternalID,
	}
	ca.CreatedBy = actor
	if ca.Assignee == "" {
		ca.Assignee = ca.CreatedBy
	}
	// Same server-side defaults as the HTTP create handler (#26).
	applyCorrectiveActionDefaults(&ca)
	if err := validateCorrectiveActionCreate(&ca); err != nil {
		return "", 0, err
	}
	// Only an assignee the payload names is checked; the defaulted one is the
	// acting user, who is a member by construction.
	if err := s.validateOrgMemberIn(ctx, orgID, p.Assignee); err != nil {
		return "", 0, err
	}
	refs, err := s.validateReferenceInputs(ctx, orgID, db.TaskViewer{Email: actor, CanSeeAll: true}, p.References)
	if err != nil {
		return "", 0, err
	}
	if err := db.CreateCorrectiveActionTx(ctx, tx, orgID, &ca); err != nil {
		return "", 0, err
	}
	if err := s.createReferencesTx(ctx, tx, orgID, "corrective_action", ca.Identifier, ca.CreatedBy, refs); err != nil {
		return "", 0, err
	}
	if err := db.LogChangeTx(ctx, tx, orgID, &db.ChangelogEntry{
		EntityType: "corrective_action", EntityID: int64(ca.ID), Action: "create", ChangedBy: ca.CreatedBy,
	}); err != nil {
		return "", 0, err
	}
	return ca.Identifier, ca.ID, nil
}

func applyCorrActiveUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var req correctiveActionUpdateRequest
	if err := decodeUpdateFields(sg.EntityType, sg.Payload, &req); err != nil {
		return "", 0, err
	}
	caID, err := s.resolveCorrectiveActionID(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, errNotFound("corrective_action")
	}
	old, err := s.db.GetCorrectiveAction(ctx, orgID, caID)
	if err != nil {
		return "", 0, errNotFound("corrective_action")
	}
	updated, err := s.prepareCorrectiveActionUpdate(ctx, orgID, old, &req)
	if err != nil {
		return "", 0, err
	}
	updated.ID = caID
	// Unified write path (#26): open-task guard + resolved_at/by.
	if err := enforceCorrectiveActionWriteTx(ctx, tx, orgID, &updated, old.Status, actor); err != nil {
		return "", 0, err
	}
	after, err := db.GetCorrectiveActionTx(ctx, tx, orgID, caID)
	if err != nil {
		return "", 0, err
	}
	diffs := db.DiffFields("corrective_action", int64(caID), actor, fmt.Sprintf("suggestion #%d", sg.ID), old.ToChangeMap(), after.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", 0, err
	}
	return after.Identifier, after.ID, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: TASKS
// ═══════════════════════════════════════════════════════════════════════

// taskCreatePayload is the payload of a task:create suggestion. Decoded strictly,
// so a key not listed here is refused instead of silently dropped (#200).
type taskCreatePayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Assignee    string `json:"assignee"`
	Priority    string `json:"priority"`
	TaskType    string `json:"task_type"`
}

func applyTaskCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var payload taskCreatePayload
	if err := decodeSuggestionPayload(sg.Payload, &payload); err != nil {
		return "", 0, fmt.Errorf("invalid task payload: %w", err)
	}
	if payload.Title == "" {
		return "", 0, fmt.Errorf("title is required")
	}
	t := db.Task{
		Title: payload.Title, Description: payload.Description,
		Assignee: payload.Assignee, CreatedBy: actor,
		Priority: payload.Priority, TaskType: payload.TaskType, Status: "open",
	}
	if t.Priority == "" {
		t.Priority = "medium"
	}
	if t.TaskType == "" {
		t.TaskType = "general"
	}
	// tasks.assignee_id is NOT NULL; default to the applier when the suggestion carries none.
	if t.Assignee == "" {
		t.Assignee = actor
	}
	if err := db.CreateTaskTx(ctx, tx, orgID, &t); err != nil {
		return "", 0, err
	}
	return t.Identifier, t.ID, nil
}

func applyTaskUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", 0, fmt.Errorf("invalid update payload: %w", err)
	}
	taskID, err := s.resolveTaskID(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, fmt.Errorf("task %s not found: %w", sg.EntityID, err)
	}
	t, err := s.db.GetTask(ctx, orgID, taskID)
	if err != nil {
		return "", 0, fmt.Errorf("task %s not found: %w", sg.EntityID, err)
	}
	old := t.ToChangeMap()
	if v, ok := payload.Fields["assignee"]; ok {
		if sv, ok := v.(string); ok {
			t.Assignee = sv
		}
	}
	if v, ok := payload.Fields["priority"]; ok {
		if sv, ok := v.(string); ok {
			t.Priority = sv
		}
	}
	if v, ok := payload.Fields["title"]; ok {
		if sv, ok := v.(string); ok {
			t.Title = sv
		}
	}
	if v, ok := payload.Fields["status"]; ok {
		if sv, ok := v.(string); ok {
			t.Status = sv
		}
	}
	if err := db.UpdateTaskTx(ctx, tx, orgID, t); err != nil {
		return "", 0, err
	}
	diffs := db.DiffFields("task", int64(t.ID), actor, fmt.Sprintf("suggestion #%d", sg.ID), old, t.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", 0, err
	}
	return t.Identifier, t.ID, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: OBJECTIVES
// ═══════════════════════════════════════════════════════════════════════

// objectiveCreatePayload is the payload of an objective:create suggestion: the
// POST /objectives body (#200), plus program_key so an agent can name the
// program by its key. Decoded strictly.
type objectiveCreatePayload struct {
	objectiveCreateRequest
	ProgramKey string `json:"program_key"`
}

// resolveSuggestedProgram picks the program an objective suggestion lands in.
// A named program (program_id or program_key) must exist in this org. With none
// named, a lone program is used; several is an error naming their keys. With no
// program at all it is an error when requireOne is set (apply), and a zero id
// otherwise (create time, because a program:create suggestion may be pending).
func (s *Server) resolveSuggestedProgram(ctx context.Context, orgID int, programID int64, programKey string, requireOne bool) (int64, error) {
	key := strings.ToUpper(strings.TrimSpace(programKey))
	var byID, byKey *db.Program
	if programID != 0 {
		p, err := s.db.GetProgram(ctx, orgID, programID)
		if err != nil {
			return 0, apiError(http.StatusBadRequest, CodeNotFoundInOrg, Entity("program"))
		}
		byID = p
	}
	if key != "" {
		p, err := s.db.GetProgramByKey(ctx, orgID, key)
		if err != nil {
			return 0, apiError(http.StatusBadRequest, CodeNotFoundInOrg, Entity("program"))
		}
		byKey = p
	}
	switch {
	case byID != nil && byKey != nil:
		if byID.ID != byKey.ID {
			return 0, echo.NewHTTPError(http.StatusBadRequest, "program_id and program_key name different programs")
		}
		return byID.ID, nil
	case byID != nil:
		return byID.ID, nil
	case byKey != nil:
		return byKey.ID, nil
	}
	progs, err := s.db.ListPrograms(ctx, orgID)
	if err != nil {
		return 0, err
	}
	switch len(progs) {
	case 0:
		if requireOne {
			return 0, echo.NewHTTPError(http.StatusBadRequest,
				"no program exists yet: create one first (a program:create suggestion or POST /programs)")
		}
		return 0, nil
	case 1:
		return progs[0].ID, nil
	}
	keys := make([]string, 0, len(progs))
	for _, p := range progs {
		keys = append(keys, p.Key)
	}
	sort.Strings(keys)
	return 0, echo.NewHTTPError(http.StatusBadRequest,
		"program_id or program_key is required: this organization has several programs ("+strings.Join(keys, ", ")+")")
}

// checkObjectiveSuggestionProgram is the create/edit-time program check for an
// objective:create suggestion (the one DB-backed check at that stage).
func (s *Server) checkObjectiveSuggestionProgram(ctx context.Context, orgID int, entityType, suggestionType string, payload json.RawMessage) error {
	if entityType != "objective" || suggestionType != "create" {
		return nil
	}
	var p objectiveCreatePayload
	if err := decodeSuggestionPayload(payload, &p); err != nil {
		return err
	}
	_, err := s.resolveSuggestedProgram(ctx, orgID, p.ProgramID, p.ProgramKey, false)
	return err
}

func applyObjectiveCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var p objectiveCreatePayload
	if err := decodeSuggestionPayload(sg.Payload, &p); err != nil {
		return "", 0, err
	}
	if p.Title == "" {
		return "", 0, errRequired("title")
	}
	programID, err := s.resolveSuggestedProgram(ctx, orgID, p.ProgramID, p.ProgramKey, true)
	if err != nil {
		return "", 0, err
	}
	o := db.Objective{
		ProgramID:         programID,
		Title:             p.Title,
		Description:       p.Description,
		Owner:             p.Owner,
		Source:            p.Source,
		MeasurementMethod: p.MeasurementMethod,
		TargetValue:       p.TargetValue,
		TargetOperator:    p.TargetOperator,
		Unit:              p.Unit,
		WindowSeconds:     p.WindowSeconds,
		GraceSeconds:      p.GraceSeconds,
		CheckinCycle:      p.CheckinCycle,
		Status:            p.Status,
		StartedAt:         p.StartedAt,
		Notes:             p.Notes,
	}
	applyObjectiveDefaults(&o, actor)
	if err := validateObjectiveCreate(&o); err != nil {
		return "", 0, err
	}
	refs, err := s.validateReferenceInputs(ctx, orgID, db.TaskViewer{Email: actor, CanSeeAll: true}, p.References)
	if err != nil {
		return "", 0, err
	}
	if err := db.CreateObjectiveTx(ctx, tx, orgID, &o); err != nil {
		return "", 0, err
	}
	if err := s.createReferencesTx(ctx, tx, orgID, "objective", o.DisplayID, actor, refs); err != nil {
		return "", 0, err
	}
	if err := db.LogChangeTx(ctx, tx, orgID, &db.ChangelogEntry{
		EntityType: "objective", EntityID: o.ID, Action: "create", ChangedBy: actor,
	}); err != nil {
		return "", 0, err
	}
	return o.DisplayID, o.ID, nil
}

func applyObjectiveUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var req objectiveUpdateRequest
	if err := decodeUpdateFields(sg.EntityType, sg.Payload, &req); err != nil {
		return "", 0, err
	}
	id, err := s.resolveObjectiveID(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, errNotFound("objective")
	}
	old, err := s.db.GetObjective(ctx, orgID, id)
	if err != nil {
		return "", 0, errNotFound("objective")
	}
	updated, err := s.prepareObjectiveUpdate(ctx, orgID, old, &req)
	if err != nil {
		return "", 0, err
	}
	updated.ID = id
	if err := db.UpdateObjectiveTx(ctx, tx, orgID, &updated); err != nil {
		return "", 0, err
	}
	diffs := db.DiffFields("objective", id, actor, fmt.Sprintf("suggestion #%d", sg.ID), old.ToChangeMap(), updated.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", 0, err
	}
	return updated.DisplayID, updated.ID, nil
}

// programCreatePayload is the payload of a program:create suggestion: the POST
// /programs body (#200). Decoded strictly.
type programCreatePayload struct {
	programCreateRequest
}

func applyProgramCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var req programCreatePayload
	if err := decodeSuggestionPayload(sg.Payload, &req); err != nil {
		return "", 0, err
	}
	pr := db.Program{
		Key:         strings.ToUpper(strings.TrimSpace(req.Key)),
		Title:       req.Title,
		Description: req.Description,
		Notes:       req.Notes,
		Owner:       req.Owner,
	}
	if pr.Key == "" {
		return "", 0, errRequired("key")
	}
	if pr.Title == "" {
		return "", 0, errRequired("title")
	}
	if err := db.CreateProgramTx(ctx, tx, orgID, &pr); err != nil {
		return "", 0, err
	}
	if err := db.LogChangeTx(ctx, tx, orgID, &db.ChangelogEntry{
		EntityType: "program", EntityID: pr.ID, Action: "create", ChangedBy: actor,
	}); err != nil {
		return "", 0, err
	}
	return pr.Identifier, pr.ID, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: SYSTEMS
// ═══════════════════════════════════════════════════════════════════════

// systemCreatePayload is the payload of a system:create suggestion: the POST
// /systems body (#200), plus "title", which SuggestNewButton.vue sends for every
// entity and is used as the name when "name" is empty. Decoded strictly.
type systemCreatePayload struct {
	systemCreateRequest
	Title string `json:"title"`
}

func applySystemCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var p systemCreatePayload
	if err := decodeSuggestionPayload(sg.Payload, &p); err != nil {
		return "", 0, err
	}
	if p.Name == "" {
		p.Name = p.Title
	}
	if p.Name == "" {
		return "", 0, errRequired("name")
	}
	sys := db.System{
		Name:            p.Name,
		Description:     p.Description,
		SupplierID:      p.SupplierID,
		Department:      p.Department,
		Classification:  p.Classification,
		Criticality:     p.Criticality,
		Status:          p.Status,
		RPOHours:        p.RPOHours,
		RTOHours:        p.RTOHours,
		Confidentiality: p.Confidentiality,
		Integrity:       p.Integrity,
		Availability:    p.Availability,
		LastReview:      p.LastReview,
		NextReview:      p.NextReview,
		Owner:           p.Owner,
		Notes:           p.Notes,
		ExternalID:      p.ExternalID,
	}
	applySystemDefaults(&sys, actor)
	if err := validateSystemCreate(&sys); err != nil {
		return "", 0, err
	}
	if err := s.validateSystemSupplier(ctx, orgID, &sys); err != nil {
		return "", 0, err
	}
	refs, err := s.validateReferenceInputs(ctx, orgID, db.TaskViewer{Email: actor, CanSeeAll: true}, p.References)
	if err != nil {
		return "", 0, err
	}
	if err := db.CreateSystemTx(ctx, tx, orgID, &sys); err != nil {
		return "", 0, err
	}
	if err := s.createReferencesTx(ctx, tx, orgID, "system", sys.Identifier, actor, refs); err != nil {
		return "", 0, err
	}
	if err := db.LogChangeTx(ctx, tx, orgID, &db.ChangelogEntry{
		EntityType: "system", EntityID: sys.ID, Action: "create", ChangedBy: actor,
	}); err != nil {
		return "", 0, err
	}
	return sys.Identifier, sys.ID, nil
}

func applySystemUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var req systemUpdateRequest
	if err := decodeUpdateFields(sg.EntityType, sg.Payload, &req); err != nil {
		return "", 0, err
	}
	id, err := s.resolveSystemID(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, errNotFound("system")
	}
	old, err := s.db.GetSystem(ctx, orgID, id)
	if err != nil {
		return "", 0, errNotFound("system")
	}
	updated, explicitNextReview, err := s.prepareSystemUpdate(ctx, orgID, old, &req)
	if err != nil {
		return "", 0, err
	}
	updated.ID = id
	if err := db.UpdateSystemTx(ctx, tx, orgID, &updated, explicitNextReview); err != nil {
		return "", 0, err
	}
	diffs := db.DiffFields("system", id, actor, fmt.Sprintf("suggestion #%d", sg.ID), old.ToChangeMap(), updated.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", 0, err
	}
	return updated.Identifier, updated.ID, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: ASSETS
// ═══════════════════════════════════════════════════════════════════════

// assetCreatePayload is the payload of an asset:create suggestion: the POST
// /assets body (#200), plus "title", which SuggestNewButton.vue sends for every
// entity and is used as the name when "name" is empty. Decoded strictly.
type assetCreatePayload struct {
	assetCreateRequest
	Title string `json:"title"`
}

func applyAssetCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var p assetCreatePayload
	if err := decodeSuggestionPayload(sg.Payload, &p); err != nil {
		return "", 0, err
	}
	if p.Name == "" {
		p.Name = p.Title
	}
	if p.Name == "" {
		return "", 0, errRequired("name")
	}
	a := db.Asset{
		Name:            p.Name,
		Description:     p.Description,
		AssetType:       p.AssetType,
		Status:          p.Status,
		Owner:           p.Owner,
		PrimaryLocation: p.PrimaryLocation,
		Confidentiality: p.Confidentiality,
		Integrity:       p.Integrity,
		Availability:    p.Availability,
		LastReview:      p.LastReview,
		NextReview:      p.NextReview,
		Notes:           p.Notes,
		ExternalID:      p.ExternalID,
	}
	applyAssetDefaults(&a, actor)
	if err := validateAssetCreate(&a); err != nil {
		return "", 0, err
	}
	refs, err := s.validateReferenceInputs(ctx, orgID, db.TaskViewer{Email: actor, CanSeeAll: true}, p.References)
	if err != nil {
		return "", 0, err
	}
	if err := db.CreateAssetTx(ctx, tx, orgID, &a); err != nil {
		return "", 0, err
	}
	if err := s.createReferencesTx(ctx, tx, orgID, "asset", a.Identifier, actor, refs); err != nil {
		return "", 0, err
	}
	if err := db.LogChangeTx(ctx, tx, orgID, &db.ChangelogEntry{
		EntityType: "asset", EntityID: a.ID, Action: "create", ChangedBy: actor,
	}); err != nil {
		return "", 0, err
	}
	return a.Identifier, a.ID, nil
}

func applyAssetUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var req assetUpdateRequest
	if err := decodeUpdateFields(sg.EntityType, sg.Payload, &req); err != nil {
		return "", 0, err
	}
	id, err := s.resolveAssetID(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, errNotFound("asset")
	}
	old, err := s.db.GetAsset(ctx, orgID, id)
	if err != nil {
		return "", 0, errNotFound("asset")
	}
	updated, err := s.prepareAssetUpdate(ctx, orgID, old, &req)
	if err != nil {
		return "", 0, err
	}
	updated.ID = id
	if err := db.UpdateAssetTx(ctx, tx, orgID, &updated); err != nil {
		return "", 0, err
	}
	diffs := db.DiffFields("asset", id, actor, fmt.Sprintf("suggestion #%d", sg.ID), old.ToChangeMap(), updated.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", 0, err
	}
	return updated.Identifier, updated.ID, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: AUDIT FINDINGS
// ═══════════════════════════════════════════════════════════════════════

// auditFindingCreatePayload is the payload of an audit_finding:create suggestion. Decoded strictly,
// so a key not listed here is refused instead of silently dropped (#200).
type auditFindingCreatePayload struct {
	AuditID     int    `json:"audit_id"`
	FindingType string `json:"finding_type"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func applyAuditFindingCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var payload auditFindingCreatePayload
	if err := decodeSuggestionPayload(sg.Payload, &payload); err != nil {
		return "", 0, fmt.Errorf("invalid finding payload: %w", err)
	}
	if payload.Title == "" {
		return "", 0, fmt.Errorf("title is required")
	}
	if payload.AuditID == 0 {
		return "", 0, fmt.Errorf("audit_id is required")
	}
	// Seed description with ## Corrective Action heading when empty
	// (corrective_action column was folded into description).
	desc := payload.Description
	if desc == "" {
		desc = "## Corrective Action\n\n"
	}
	f := db.AuditFinding{
		AuditID:     payload.AuditID,
		FindingType: payload.FindingType,
		Title:       payload.Title,
		Description: desc,
		Status:      "open",
	}
	if f.FindingType == "" {
		f.FindingType = "observation"
	}
	if err := db.AddAuditFindingTx(ctx, tx, orgID, &f); err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%d", f.ID), f.ID, nil
}

func applyAuditFindingUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", 0, fmt.Errorf("invalid update payload: %w", err)
	}
	// A finding's display id is built from its primary key (db/audits.go), so
	// the strip is exact here — the one place it is (see entityIDResolvers).
	idInt, err := strconv.ParseInt(stripPrefix(sg.EntityID, "FIND-"), 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("audit finding %s: invalid id", sg.EntityID)
	}
	for field, val := range payload.Fields {
		// Status transitions go through the shared closure-metadata path (same as
		// the HTTP handler) — a plain field write would skip closed_at/closed_by.
		if field == "status" {
			sv, ok := val.(string)
			if !ok {
				return "", 0, fmt.Errorf("field status: expected a string, got %T", val)
			}
			if !db.AuditFindingStatuses[sv] {
				return "", 0, fmt.Errorf("invalid status: %s", sv)
			}
			if err := db.SetAuditFindingStatusTx(ctx, tx, orgID, idInt, sv, actor); err != nil {
				return "", 0, err
			}
			continue
		}
		sv, ok := val.(string)
		if !ok {
			continue
		}
		if err := db.UpdateAuditFindingFieldTx(ctx, tx, orgID, int(idInt), field, sv); err != nil {
			return "", 0, fmt.Errorf("updating field %s: %w", field, err)
		}
	}
	return sg.EntityID, idInt, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: READINGS
// ═══════════════════════════════════════════════════════════════════════

func applyRiskReading(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var reading db.EntityReading
	if err := json.Unmarshal(sg.Payload, &reading); err != nil {
		return "", 0, fmt.Errorf("invalid reading payload: %w", err)
	}

	risk, err := s.db.GetRiskByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, fmt.Errorf("risk %s not found: %w", sg.EntityID, err)
	}

	reading.EntityType = "risk"
	reading.EntityID = risk.ID
	reading.AssessedBy = actor

	if err := db.CreateEntityReadingTx(ctx, tx, orgID, &reading); err != nil {
		return "", 0, fmt.Errorf("create reading: %w", err)
	}
	if err := writeRiskFromReading(ctx, tx, s, orgID, risk.ID, &reading, actor, ""); err != nil {
		return "", 0, err
	}
	return risk.Identifier, risk.ID, nil
}

func applyLegalReading(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var reading db.EntityReading
	if err := json.Unmarshal(sg.Payload, &reading); err != nil {
		return "", 0, fmt.Errorf("invalid reading payload: %w", err)
	}

	lr, err := s.db.GetLegalRequirementByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, fmt.Errorf("legal requirement %s not found: %w", sg.EntityID, err)
	}

	reading.EntityType = "legal_requirement"
	reading.EntityID = int64(lr.ID)
	reading.AssessedBy = actor

	if err := db.CreateEntityReadingTx(ctx, tx, orgID, &reading); err != nil {
		return "", 0, fmt.Errorf("create reading: %w", err)
	}
	if err := writeLegalFromReading(ctx, tx, s, orgID, int(lr.ID), &reading, actor, ""); err != nil {
		return "", 0, err
	}
	return lr.Identifier, lr.ID, nil
}

// applySupplierReviewSuggestion creates a supplier review from a suggestion.
func applySupplierReviewSuggestion(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var payload struct {
		Outcome                string `json:"outcome"`
		CertificationsVerified bool   `json:"certifications_verified"`
		DataHandlingVerified   bool   `json:"data_handling_verified"`
		SLAMet                 bool   `json:"sla_met"`
		Notes                  string `json:"notes"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", 0, fmt.Errorf("invalid review payload: %w", err)
	}

	sup, err := s.db.GetSupplierByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, fmt.Errorf("supplier %s not found: %w", sg.EntityID, err)
	}

	if payload.Outcome == "" {
		payload.Outcome = "satisfactory"
	}
	if payload.Notes == "" {
		payload.Notes = sg.Title + ": " + sg.Rationale
	}

	sr := &db.SupplierReview{
		SupplierID:             sup.ID,
		Outcome:                payload.Outcome,
		CertificationsVerified: payload.CertificationsVerified,
		DataHandlingVerified:   payload.DataHandlingVerified,
		SLAMet:                 payload.SLAMet,
		Notes:                  payload.Notes,
		ReviewedBy:             actor,
	}

	// Use pool (not tx) since SupplierReview auto-updates parent
	if err := s.db.CreateSupplierReview(ctx, orgID, sr); err != nil {
		return "", 0, fmt.Errorf("create supplier review: %w", err)
	}
	return sup.Identifier, sup.ID, nil
}

// applyAccessReviewSuggestion creates an access review for a system from a suggestion.
func applyAccessReviewSuggestion(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var payload struct {
		UsersAdded   int    `json:"users_added"`
		UsersRemoved int    `json:"users_removed"`
		Notes        string `json:"notes"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", 0, fmt.Errorf("invalid access review payload: %w", err)
	}

	sys, err := s.db.GetSystemByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, fmt.Errorf("system %s not found: %w", sg.EntityID, err)
	}

	if payload.Notes == "" {
		payload.Notes = sg.Title + ": " + sg.Rationale
	}

	ar := &db.AccessReview{
		SystemID:     sys.ID,
		ReviewedAt:   db.EpochNow(),
		ReviewedBy:   actor,
		UsersAdded:   payload.UsersAdded,
		UsersRemoved: payload.UsersRemoved,
		Notes:        payload.Notes,
	}

	if err := s.db.CreateAccessReview(ctx, orgID, ar); err != nil {
		return "", 0, fmt.Errorf("create access review: %w", err)
	}
	return sys.Identifier, sys.ID, nil
}

// applyAssetReviewSuggestion creates an asset review from a suggestion.
func applyAssetReviewSuggestion(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, int64, error) {
	var payload struct {
		Outcome                string `json:"outcome"`
		ClassificationVerified bool   `json:"classification_verified"`
		OwnershipVerified      bool   `json:"ownership_verified"`
		Notes                  string `json:"notes"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", 0, fmt.Errorf("invalid asset review payload: %w", err)
	}

	asset, err := s.db.GetAssetByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", 0, fmt.Errorf("asset %s not found: %w", sg.EntityID, err)
	}

	if payload.Outcome == "" {
		payload.Outcome = "satisfactory"
	}
	if payload.Notes == "" {
		payload.Notes = sg.Title + ": " + sg.Rationale
	}

	ar := &db.AssetReview{
		AssetID:                asset.ID,
		Outcome:                payload.Outcome,
		ClassificationVerified: payload.ClassificationVerified,
		OwnershipVerified:      payload.OwnershipVerified,
		Notes:                  payload.Notes,
		ReviewedBy:             actor,
	}

	if err := s.db.CreateAssetReview(ctx, orgID, ar); err != nil {
		return "", 0, fmt.Errorf("create asset review: %w", err)
	}
	return asset.Identifier, asset.ID, nil
}
