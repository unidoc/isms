package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
// Returns the entity ID of the created/updated entity for changelog linkage.
type SuggestionApplyFunc func(ctx context.Context, tx pgx.Tx, s *Server, orgID int, suggestion *db.Suggestion, actor string) (entityID string, err error)

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
	if sg.EntityType == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "entity_type is required")
	}
	if sg.SuggestionType == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "suggestion_type is required")
	}
	if sg.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
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

	// Auto-populate payload title/description from suggestion fields for web UI suggestions
	if sg.Payload != nil {
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
		if _, ok := p["description"]; !ok && sg.Rationale != "" {
			p["description"] = sg.Rationale
		}
		sg.Payload, _ = json.Marshal(p)
	}

	// Snapshot entity_updated_at for stale detection
	if sg.EntityID != "" {
		sg.EntityUpdatedAt = s.db.GetEntityUpdatedAt(ctx, orgID, sg.EntityType, sg.EntityID)
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

func (s *Server) handleListEntitySuggestions(c echo.Context) error {
	orgID := getOrgID(c)

	filters := db.SuggestionFilters{
		Status:          c.QueryParam("status"),
		EntityType:      c.QueryParam("entity_type"),
		EntityID:        c.QueryParam("entity_id"),
		SuggestedBy:     c.QueryParam("suggested_by"),
		SuggestedByType: c.QueryParam("suggested_by_type"),
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
	return c.JSON(http.StatusOK, map[string]interface{}{"data": suggestions})
}

func (s *Server) handleGetEntitySuggestion(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	sg, err := s.db.GetSuggestion(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "suggestion not found")
	}

	// Attach stale info if entity has changed
	resp := map[string]interface{}{"data": sg}
	if sg.EntityID != "" && sg.EntityUpdatedAt != nil && sg.Status != "applied" && sg.Status != "rejected" {
		entityIDInt, _ := parseEntityID(sg.EntityID)
		if entityIDInt > 0 {
			changes, _ := s.db.EntityChangesAfter(c.Request().Context(), orgID, sg.EntityType, entityIDInt, *sg.EntityUpdatedAt)
			if len(changes) > 0 {
				resp["stale"] = true
				resp["stale_changes"] = changes
			}
		}
	}

	return c.JSON(http.StatusOK, resp)
}

func (s *Server) handleUpdateEntitySuggestion(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)
	role := getRole(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	existing, err := s.db.GetSuggestion(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "suggestion not found")
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

	var update db.Suggestion
	if err := c.Bind(&update); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	update.ID = id
	if update.Title == "" {
		update.Title = existing.Title
	}
	if update.Payload == nil {
		update.Payload = existing.Payload
	}

	if err := s.db.UpdateSuggestion(ctx, orgID, &update); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	existing, err := s.db.GetSuggestion(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "suggestion not found")
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
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	newStatus, err := s.db.ClaimSuggestion(ctx, orgID, id, actor)
	if err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"status": newStatus})
}

func (s *Server) handleApplyEntitySuggestion(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	sg, err := s.db.GetSuggestion(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "suggestion not found")
	}
	if sg.Status != "open" && sg.Status != "in_review" {
		return echo.NewHTTPError(http.StatusConflict, "suggestion is in terminal state: "+sg.Status)
	}

	// Check for force flag if stale
	var body struct {
		Force bool `json:"force"`
	}
	_ = c.Bind(&body)

	if sg.EntityID != "" && sg.EntityUpdatedAt != nil && !body.Force {
		entityIDInt, _ := parseEntityID(sg.EntityID)
		if entityIDInt > 0 {
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
		var err error
		appliedEntityID, err = handler(ctx, tx, s, orgID, sg, actor)
		if err != nil {
			return fmt.Errorf("apply: %w", err)
		}

		// Changelog linking suggestion to entity
		if appliedEntityID != "" {
			entityIDInt, _ := parseEntityID(appliedEntityID)
			if err := db.LogChangeTx(ctx, tx, orgID, &db.ChangelogEntry{
				EntityType: sg.EntityType,
				EntityID:   entityIDInt,
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
		// with the allowed list); a DB constraint violation maps to 400 via
		// pgxHTTPError; anything else is a genuine 500.
		var he *echo.HTTPError
		if errors.As(txErr, &he) {
			return he
		}
		return pgxHTTPError(txErr)
	}

	// Post-commit: activity log + notifications (non-transactional, OK to fail)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "suggestion_applied",
		Detail: fmt.Sprintf("Applied suggestion #%d: %s → %s %s", sg.ID, sg.Title, sg.EntityType, appliedEntityID),
	})
	s.notifySuggestionResolved(ctx, orgID, sg, "applied",
		fmt.Sprintf("Your suggestion \"%s\" was applied by %s → %s %s", sg.Title, actor, sg.EntityType, appliedEntityID))

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
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(body.Reason) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "reason is required")
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
		s.notifySuggestionResolved(ctx, orgID, sg, "rejected",
			fmt.Sprintf("Your suggestion \"%s\" was rejected by %s: %s", sg.Title, actor, body.Reason))
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "rejected"})
}

func (s *Server) handleWithdrawEntitySuggestion(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
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

func parseEntityID(s string) (int64, error) {
	// Strip prefix like "RISK-", "INC-", etc.
	parts := strings.SplitN(s, "-", 2)
	if len(parts) == 2 {
		if n, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			return n, nil
		}
	}
	return strconv.ParseInt(s, 10, 64)
}

// notifySuggestionCreated sends notifications to entity owner and org managers.
func (s *Server) notifySuggestionCreated(ctx context.Context, orgID int, sg *db.Suggestion) {
	link := fmt.Sprintf("/inbox/suggestions?id=%d", sg.ID)
	body := fmt.Sprintf("%s suggested: %s (%s %s)", sg.SuggestedBy, sg.Title, sg.SuggestionType, sg.EntityType)

	// Notify managers
	users, _ := s.db.ListOrgUsers(ctx, orgID)
	for _, u := range users {
		if (u.Role == "admin" || u.Role == "manager") && u.Email != sg.SuggestedBy {
			_ = s.db.CreateNotificationByEmail(ctx, orgID, u.Email, "New suggestion", body, link)
		}
	}
}

// notifySuggestionResolved sends notification to the original suggester.
func (s *Server) notifySuggestionResolved(ctx context.Context, orgID int, sg *db.Suggestion, action, detail string) {
	link := fmt.Sprintf("/inbox/suggestions?id=%d", sg.ID)
	title := fmt.Sprintf("Suggestion %s", action)
	_ = s.db.CreateNotificationByEmail(ctx, orgID, sg.SuggestedBy, title, detail, link)
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: RISKS
// ═══════════════════════════════════════════════════════════════════════

func applyRiskCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Title             string `json:"title"`
		Description       string `json:"description"`
		RiskType          string `json:"risk_type"`
		Origin            string `json:"origin"`
		Category          string `json:"category"`
		CurrentLikelihood *int   `json:"current_likelihood"`
		CurrentImpact     *int   `json:"current_impact"`
		TreatmentPlan     string `json:"treatment_plan"`
		Treatment         string `json:"treatment"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid risk payload: %w", err)
	}
	if payload.Title == "" {
		return "", fmt.Errorf("title is required in risk payload")
	}

	risk := db.Risk{
		Title:             payload.Title,
		Description:       payload.Description,
		RiskType:          payload.RiskType,
		Origin:            payload.Origin,
		Category:          payload.Category,
		CurrentLikelihood: payload.CurrentLikelihood,
		CurrentImpact:     payload.CurrentImpact,
		TreatmentPlan:     payload.TreatmentPlan,
		Treatment:         payload.Treatment,
		Status:            "open",
		Owner:             actor,
	}
	// Defaults for required validation fields when web UI sends minimal payload
	if risk.RiskType == "" {
		risk.RiskType = "threat"
	}
	if risk.Origin == "" {
		risk.Origin = "internal"
	}
	if risk.Category == "" {
		risk.Category = "technology"
	}
	if risk.CurrentLikelihood == nil {
		l := 3
		risk.CurrentLikelihood = &l
	}
	if risk.CurrentImpact == nil {
		i := 3
		risk.CurrentImpact = &i
	}
	// Seed description with section headings when empty
	// (potential_consequences column was folded into description).
	if risk.Description == "" {
		risk.Description = "## Description\n\n\n\n## Potential consequences\n\n"
	}

	if err := db.CreateRiskTx(ctx, tx, orgID, &risk); err != nil {
		return "", err
	}

	return risk.Identifier, nil
}

func applyRiskReassess(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		CurrentLikelihood *int   `json:"current_likelihood"`
		CurrentImpact     *int   `json:"current_impact"`
		Reason            string `json:"reason"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid reassess payload: %w", err)
	}

	risk, err := s.db.GetRiskByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", fmt.Errorf("risk %s not found: %w", sg.EntityID, err)
	}

	old := risk.ToChangeMap()
	if payload.CurrentLikelihood != nil {
		risk.CurrentLikelihood = payload.CurrentLikelihood
	}
	if payload.CurrentImpact != nil {
		risk.CurrentImpact = payload.CurrentImpact
	}
	risk.CalculateScore(nil)

	if err := db.UpdateRiskTx(ctx, tx, orgID, risk); err != nil {
		return "", err
	}

	diffs := db.DiffFields("risk", int64(risk.ID), actor, fmt.Sprintf("suggestion #%d: %s", sg.ID, payload.Reason), old, risk.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", err
	}

	return risk.Identifier, nil
}

func applyRiskUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid update payload: %w", err)
	}

	risk, err := s.db.GetRiskByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", fmt.Errorf("risk %s not found: %w", sg.EntityID, err)
	}

	old := risk.ToChangeMap()

	if v, ok := payload.Fields["owner"]; ok {
		if s, ok := v.(string); ok {
			risk.Owner = s
		}
	}
	if v, ok := payload.Fields["status"]; ok {
		if s, ok := v.(string); ok {
			risk.Status = s
		}
	}
	if v, ok := payload.Fields["treatment"]; ok {
		if s, ok := v.(string); ok {
			risk.Treatment = s
		}
	}
	if v, ok := payload.Fields["treatment_plan"]; ok {
		if s, ok := v.(string); ok {
			risk.TreatmentPlan = s
		}
	}
	if v, ok := payload.Fields["notes"]; ok {
		if s, ok := v.(string); ok {
			risk.Notes = s
		}
	}

	if err := db.UpdateRiskTx(ctx, tx, orgID, risk); err != nil {
		return "", err
	}

	diffs := db.DiffFields("risk", int64(risk.ID), actor, fmt.Sprintf("suggestion #%d", sg.ID), old, risk.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", err
	}

	return risk.Identifier, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: INCIDENTS
// ═══════════════════════════════════════════════════════════════════════

func applyIncidentCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Title           string   `json:"title"`
		Summary         string   `json:"summary"`
		Description     string   `json:"description"`
		Severity        string   `json:"severity"`
		AffectsC        bool     `json:"affects_c"`
		AffectsI        bool     `json:"affects_i"`
		AffectsA        bool     `json:"affects_a"`
		AffectedSystems []string `json:"affected_systems"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid incident payload: %w", err)
	}
	if payload.Title == "" {
		return "", fmt.Errorf("title is required in incident payload")
	}

	inc := db.Incident{
		Title:       payload.Title,
		Description: payload.Description,
		Severity:    payload.Severity,
		AffectsC:    payload.AffectsC,
		AffectsI:    payload.AffectsI,
		AffectsA:    payload.AffectsA,
		Status:      "open",
		Assignee:    actor,
	}
	// Defaults for required fields when web UI sends minimal payload
	if inc.Severity == "" {
		inc.Severity = "medium"
	}
	if inc.IncidentType == "" {
		inc.IncidentType = "event"
	}
	if inc.Source == "" {
		inc.Source = "internal"
	}
	if inc.Reporter == "" {
		inc.Reporter = actor
	}

	if err := db.CreateIncidentTx(ctx, tx, orgID, &inc); err != nil {
		return "", err
	}

	return inc.Identifier, nil
}

func applyIncidentUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid update payload: %w", err)
	}

	incID, parseErr := parseEntityID(sg.EntityID)
	if parseErr != nil {
		return "", fmt.Errorf("invalid incident ID %s: %w", sg.EntityID, parseErr)
	}
	inc, err := s.db.GetIncident(ctx, orgID, int(incID))
	if err != nil {
		return "", fmt.Errorf("incident %s not found: %w", sg.EntityID, err)
	}

	old := inc.ToChangeMap()
	prevStatus := inc.Status

	if v, ok := payload.Fields["severity"]; ok {
		if sv, ok := v.(string); ok {
			inc.Severity = sv
		}
	}
	if v, ok := payload.Fields["status"]; ok {
		if sv, ok := v.(string); ok {
			inc.Status = sv
		}
	}
	if v, ok := payload.Fields["assignee"]; ok {
		if sv, ok := v.(string); ok {
			inc.Assignee = sv
		}
	}
	if v, ok := payload.Fields["root_cause"]; ok {
		if sv, ok := v.(string); ok {
			inc.RootCause = sv
		}
	}
	if v, ok := payload.Fields["affects_c"]; ok {
		if bv, ok := v.(bool); ok {
			inc.AffectsC = bv
		}
	}
	if v, ok := payload.Fields["affects_i"]; ok {
		if bv, ok := v.(bool); ok {
			inc.AffectsI = bv
		}
	}
	if v, ok := payload.Fields["affects_a"]; ok {
		if bv, ok := v.(bool); ok {
			inc.AffectsA = bv
		}
	}

	// Unified write path (#26): same open-CA guard + lifecycle timestamps the
	// HTTP handler enforces — previously suggestion-apply bypassed both.
	if err := enforceIncidentWriteTx(ctx, tx, orgID, inc, prevStatus); err != nil {
		return "", err
	}

	diffs := db.DiffFields("incident", int64(inc.ID), actor, fmt.Sprintf("suggestion #%d", sg.ID), old, inc.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", err
	}

	return inc.Identifier, nil
}

func applyIncidentLink(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Links []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"links"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid link payload: %w", err)
	}

	var linked int
	for _, link := range payload.Links {
		if err := db.CreateReferenceTx(ctx, tx, orgID, &db.EntityReference{
			SourceType: "incident",
			SourceID:   sg.EntityID,
			TargetType: link.Type,
			TargetID:   link.ID,
		}); err != nil {
			return "", fmt.Errorf("failed to link %s %s: %w", link.Type, link.ID, err)
		}
		linked++
	}
	if linked == 0 {
		return "", fmt.Errorf("no links in payload")
	}

	return sg.EntityID, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: SUPPLIERS
// ═══════════════════════════════════════════════════════════════════════

func applySupplierCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Name         string `json:"name"`
		SupplierType string `json:"supplier_type"`
		Criticality  string `json:"criticality"`
		Owner        string `json:"owner"`
		Notes        string `json:"notes"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid supplier payload: %w", err)
	}
	if payload.Name == "" {
		return "", fmt.Errorf("name is required in supplier payload")
	}

	sup := db.Supplier{
		Name:         payload.Name,
		SupplierType: payload.SupplierType,
		Criticality:  payload.Criticality,
		Owner:        payload.Owner,
		Notes:        payload.Notes,
	}
	applySupplierDefaults(&sup, actor)
	if err := validateSupplierCreate(&sup); err != nil {
		return "", err
	}

	if err := db.CreateSupplierTx(ctx, tx, orgID, &sup); err != nil {
		return "", err
	}

	return sup.Identifier, nil
}

func applySupplierUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid update payload: %w", err)
	}

	sup, err := s.db.GetSupplierByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", fmt.Errorf("supplier %s not found: %w", sg.EntityID, err)
	}

	old := sup.ToChangeMap()

	if v, ok := payload.Fields["criticality"]; ok {
		if sv, ok := v.(string); ok {
			sup.Criticality = sv
		}
	}
	if v, ok := payload.Fields["notes"]; ok {
		if sv, ok := v.(string); ok {
			sup.Notes = sv
		}
	}
	if v, ok := payload.Fields["status"]; ok {
		if sv, ok := v.(string); ok {
			sup.Status = sv
		}
	}

	if err := db.UpdateSupplierTx(ctx, tx, orgID, sup); err != nil {
		return "", err
	}

	diffs := db.DiffFields("supplier", sup.ID, actor, fmt.Sprintf("suggestion #%d", sg.ID), old, sup.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", err
	}

	return sup.Identifier, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: LEGAL
// ═══════════════════════════════════════════════════════════════════════

func applyLegalCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Title        string `json:"title"`
		Description  string `json:"description"`
		Jurisdiction string `json:"jurisdiction"`
		Category     string `json:"category"`
		Owner        string `json:"owner"`
		Notes        string `json:"notes"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid legal payload: %w", err)
	}
	if payload.Title == "" {
		return "", fmt.Errorf("title is required in legal payload")
	}

	lr := db.LegalRequirement{
		Title:        payload.Title,
		Description:  payload.Description,
		Jurisdiction: payload.Jurisdiction,
		Category:     payload.Category,
		Owner:        payload.Owner,
		Notes:        payload.Notes,
	}
	applyLegalDefaults(&lr, actor)
	if err := validateLegalCreate(&lr); err != nil {
		return "", err
	}

	if err := db.CreateLegalRequirementTx(ctx, tx, orgID, &lr); err != nil {
		return "", err
	}

	return lr.Identifier, nil
}

func applyLegalUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid update payload: %w", err)
	}

	lr, err := s.db.GetLegalRequirementByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", fmt.Errorf("legal requirement %s not found: %w", sg.EntityID, err)
	}

	old := lr.ToChangeMap()

	if v, ok := payload.Fields["owner"]; ok {
		if sv, ok := v.(string); ok {
			lr.Owner = sv
		}
	}
	if v, ok := payload.Fields["notes"]; ok {
		if sv, ok := v.(string); ok {
			lr.Notes = sv
		}
	}

	if err := db.UpdateLegalRequirementTx(ctx, tx, orgID, lr); err != nil {
		return "", err
	}

	diffs := db.DiffFields("legal_requirement", int64(lr.ID), actor, fmt.Sprintf("suggestion #%d", sg.ID), old, lr.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", err
	}

	return lr.Identifier, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: CHANGE REQUESTS
// ═══════════════════════════════════════════════════════════════════════

func applyChangeCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Title         string `json:"title"`
		Description   string `json:"description"`
		Justification string `json:"justification"`
		Priority      string `json:"priority"`
		Category      string `json:"category"`
		RiskLevel     string `json:"risk_level"`
		RollbackPlan  string `json:"rollback_plan"`
		AssignedTo    string `json:"assigned_to"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid change payload: %w", err)
	}
	if payload.Title == "" {
		return "", fmt.Errorf("title is required")
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
		return "", err
	}
	return cr.Identifier, nil
}

func applyChangeUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid update payload: %w", err)
	}
	idInt, _ := parseEntityID(sg.EntityID)
	cr, err := s.db.GetChangeRequest(ctx, orgID, int(idInt))
	if err != nil {
		return "", fmt.Errorf("change request %s not found: %w", sg.EntityID, err)
	}
	if v, ok := payload.Fields["type"]; ok {
		if sv, ok := v.(string); ok {
			if err := validateEnum("type", sv, db.ChangeTypes); err != nil {
				return "", err
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
	if err := db.UpdateChangeRequestTx(ctx, tx, orgID, cr.ID, cr); err != nil {
		return "", err
	}
	return cr.Identifier, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: CORRECTIVE ACTIONS
// ═══════════════════════════════════════════════════════════════════════

func applyCorrActiveCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Source      string `json:"source"`
		Severity    string `json:"severity"`
		Assignee    string `json:"assignee"`
		Notes       string `json:"notes"`
		RootCause   string `json:"root_cause"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid CA payload: %w", err)
	}
	if payload.Title == "" {
		return "", fmt.Errorf("title is required")
	}
	ca := db.CorrectiveAction{
		Title: payload.Title, Description: payload.Description,
		Source: payload.Source, Severity: payload.Severity,
		Assignee: payload.Assignee, CreatedBy: actor,
		Notes: payload.Notes, RootCause: payload.RootCause,
	}
	if ca.Assignee == "" {
		ca.Assignee = actor
	}
	// Same server-side defaults as the HTTP create handler (#26) — previously
	// suggestion-apply seeded a different starting state.
	applyCorrectiveActionDefaults(&ca)
	if err := db.CreateCorrectiveActionTx(ctx, tx, orgID, &ca); err != nil {
		return "", err
	}
	return ca.Identifier, nil
}

func applyCorrActiveUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid update payload: %w", err)
	}
	idInt, _ := parseEntityID(sg.EntityID)
	ca, err := s.db.GetCorrectiveAction(ctx, orgID, int(idInt))
	if err != nil {
		return "", fmt.Errorf("corrective action %s not found: %w", sg.EntityID, err)
	}
	old := ca.ToChangeMap()
	prevStatus := ca.Status
	if v, ok := payload.Fields["assignee"]; ok {
		if sv, ok := v.(string); ok {
			ca.Assignee = sv
		}
	}
	if v, ok := payload.Fields["status"]; ok {
		if sv, ok := v.(string); ok {
			ca.Status = sv
		}
	}
	if v, ok := payload.Fields["root_cause"]; ok {
		if sv, ok := v.(string); ok {
			ca.RootCause = sv
		}
	}
	if v, ok := payload.Fields["notes"]; ok {
		if sv, ok := v.(string); ok {
			ca.Notes = sv
		}
	}
	// Unified write path (#26): same open-task guard + resolved_at/by the HTTP
	// handler enforces — previously suggestion-apply bypassed both.
	if err := enforceCorrectiveActionWriteTx(ctx, tx, orgID, ca, prevStatus, actor); err != nil {
		return "", err
	}
	diffs := db.DiffFields("corrective_action", int64(ca.ID), actor, fmt.Sprintf("suggestion #%d", sg.ID), old, ca.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", err
	}
	return ca.Identifier, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: TASKS
// ═══════════════════════════════════════════════════════════════════════

func applyTaskCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Assignee    string `json:"assignee"`
		Priority    string `json:"priority"`
		TaskType    string `json:"task_type"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid task payload: %w", err)
	}
	if payload.Title == "" {
		return "", fmt.Errorf("title is required")
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
		return "", err
	}
	return t.Identifier, nil
}

func applyTaskUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid update payload: %w", err)
	}
	idInt, _ := parseEntityID(sg.EntityID)
	t, err := s.db.GetTask(ctx, orgID, int(idInt))
	if err != nil {
		return "", fmt.Errorf("task %s not found: %w", sg.EntityID, err)
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
		return "", err
	}
	diffs := db.DiffFields("task", int64(t.ID), actor, fmt.Sprintf("suggestion #%d", sg.ID), old, t.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", err
	}
	return t.Identifier, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: OBJECTIVES
// ═══════════════════════════════════════════════════════════════════════

func applyObjectiveCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Title             string   `json:"title"`
		Description       string   `json:"description"`
		ProgramID         int64    `json:"program_id"`
		Owner             string   `json:"owner"`
		MeasurementMethod string   `json:"measurement_method"`
		TargetValue       *float64 `json:"target_value"`
		Unit              string   `json:"unit"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid objective payload: %w", err)
	}
	if payload.Title == "" {
		return "", fmt.Errorf("title is required")
	}
	// If no program_id supplied, use the first program in the org
	if payload.ProgramID == 0 {
		progs, err := s.db.ListPrograms(ctx, orgID)
		if err != nil || len(progs) == 0 {
			return "", fmt.Errorf("program_id is required and no default program exists")
		}
		payload.ProgramID = progs[0].ID
	}
	o := db.Objective{
		Title:             payload.Title,
		Description:       payload.Description,
		ProgramID:         payload.ProgramID,
		Owner:             payload.Owner,
		MeasurementMethod: payload.MeasurementMethod,
		TargetValue:       payload.TargetValue,
		Unit:              payload.Unit,
	}
	applyObjectiveDefaults(&o, actor)
	if err := validateObjectiveCreate(&o); err != nil {
		return "", err
	}
	if err := db.CreateObjectiveTx(ctx, tx, orgID, &o); err != nil {
		return "", err
	}
	return o.DisplayID, nil
}

func applyObjectiveUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid update payload: %w", err)
	}
	idInt, _ := parseEntityID(sg.EntityID)
	o, err := s.db.GetObjective(ctx, orgID, idInt)
	if err != nil {
		return "", fmt.Errorf("objective %s not found: %w", sg.EntityID, err)
	}
	old := o.ToChangeMap()
	if v, ok := payload.Fields["title"]; ok {
		if sv, ok := v.(string); ok {
			o.Title = sv
		}
	}
	if v, ok := payload.Fields["description"]; ok {
		if sv, ok := v.(string); ok {
			o.Description = sv
		}
	}
	if v, ok := payload.Fields["owner"]; ok {
		if sv, ok := v.(string); ok {
			o.Owner = sv
		}
	}
	if v, ok := payload.Fields["measurement_method"]; ok {
		if sv, ok := v.(string); ok {
			o.MeasurementMethod = sv
		}
	}
	if v, ok := payload.Fields["status"]; ok {
		if sv, ok := v.(string); ok {
			o.Status = sv
		}
	}
	if err := db.UpdateObjectiveTx(ctx, tx, orgID, o); err != nil {
		return "", err
	}
	diffs := db.DiffFields("objective", o.ID, actor, fmt.Sprintf("suggestion #%d", sg.ID), old, o.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", err
	}
	return o.DisplayID, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: SYSTEMS
// ═══════════════════════════════════════════════════════════════════════

func applySystemCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Name           string `json:"name"`
		Title          string `json:"title"` // alias: web UI may send title instead of name
		Description    string `json:"description"`
		Classification string `json:"classification"`
		Criticality    string `json:"criticality"`
		Department     string `json:"department"`
		Owner          string `json:"owner"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid system payload: %w", err)
	}
	// Systems use "name" not "title" — accept either
	if payload.Name == "" {
		payload.Name = payload.Title
	}
	if payload.Name == "" {
		return "", fmt.Errorf("name is required in system payload")
	}
	sys := db.System{
		Name:           payload.Name,
		Description:    payload.Description,
		Classification: payload.Classification,
		Criticality:    payload.Criticality,
		Department:     payload.Department,
		Owner:          payload.Owner,
	}
	applySystemDefaults(&sys, actor)
	if err := validateSystemCreate(&sys); err != nil {
		return "", err
	}
	if err := db.CreateSystemTx(ctx, tx, orgID, &sys); err != nil {
		return "", err
	}
	return sys.Identifier, nil
}

func applySystemUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid update payload: %w", err)
	}
	idInt, _ := parseEntityID(sg.EntityID)
	sys, err := s.db.GetSystem(ctx, orgID, idInt)
	if err != nil {
		return "", fmt.Errorf("system %s not found: %w", sg.EntityID, err)
	}
	old := sys.ToChangeMap()
	if v, ok := payload.Fields["name"]; ok {
		if sv, ok := v.(string); ok {
			sys.Name = sv
		}
	}
	if v, ok := payload.Fields["criticality"]; ok {
		if sv, ok := v.(string); ok {
			sys.Criticality = sv
		}
	}
	if v, ok := payload.Fields["classification"]; ok {
		if sv, ok := v.(string); ok {
			sys.Classification = sv
		}
	}
	if v, ok := payload.Fields["owner"]; ok {
		if sv, ok := v.(string); ok {
			sys.Owner = sv
		}
	}
	if v, ok := payload.Fields["notes"]; ok {
		if sv, ok := v.(string); ok {
			sys.Notes = sv
		}
	}
	if v, ok := payload.Fields["department"]; ok {
		if sv, ok := v.(string); ok {
			sys.Department = sv
		}
	}
	if v, ok := payload.Fields["status"]; ok {
		if sv, ok := v.(string); ok {
			sys.Status = sv
		}
	}
	if err := db.UpdateSystemTx(ctx, tx, orgID, sys); err != nil {
		return "", err
	}
	diffs := db.DiffFields("system", sys.ID, actor, fmt.Sprintf("suggestion #%d", sg.ID), old, sys.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", err
	}
	return sys.Identifier, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: ASSETS
// ═══════════════════════════════════════════════════════════════════════

func applyAssetCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Name        string `json:"name"`
		Title       string `json:"title"` // alias: web UI may send title instead of name
		Description string `json:"description"`
		AssetType   string `json:"asset_type"`
		Owner       string `json:"owner"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid asset payload: %w", err)
	}
	// Assets use "name" not "title" — accept either
	if payload.Name == "" {
		payload.Name = payload.Title
	}
	if payload.Name == "" {
		return "", fmt.Errorf("name is required in asset payload")
	}
	a := db.Asset{
		Name:        payload.Name,
		Description: payload.Description,
		AssetType:   payload.AssetType,
		Owner:       payload.Owner,
	}
	applyAssetDefaults(&a, actor)
	if err := validateAssetCreate(&a); err != nil {
		return "", err
	}
	if err := db.CreateAssetTx(ctx, tx, orgID, &a); err != nil {
		return "", err
	}
	return a.Identifier, nil
}

func applyAssetUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid update payload: %w", err)
	}
	idInt, _ := parseEntityID(sg.EntityID)
	asset, err := s.db.GetAsset(ctx, orgID, idInt)
	if err != nil {
		return "", fmt.Errorf("asset %s not found: %w", sg.EntityID, err)
	}
	old := asset.ToChangeMap()
	if v, ok := payload.Fields["name"]; ok {
		if sv, ok := v.(string); ok {
			asset.Name = sv
		}
	}
	if v, ok := payload.Fields["owner"]; ok {
		if sv, ok := v.(string); ok {
			asset.Owner = sv
		}
	}
	if v, ok := payload.Fields["status"]; ok {
		if sv, ok := v.(string); ok {
			asset.Status = sv
		}
	}
	if v, ok := payload.Fields["notes"]; ok {
		if sv, ok := v.(string); ok {
			asset.Notes = sv
		}
	}
	if v, ok := payload.Fields["asset_type"]; ok {
		if sv, ok := v.(string); ok {
			asset.AssetType = sv
		}
	}
	if err := db.UpdateAssetTx(ctx, tx, orgID, asset); err != nil {
		return "", err
	}
	diffs := db.DiffFields("asset", asset.ID, actor, fmt.Sprintf("suggestion #%d", sg.ID), old, asset.ToChangeMap())
	if err := db.LogChangesTx(ctx, tx, orgID, diffs); err != nil {
		return "", err
	}
	return asset.Identifier, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: AUDIT FINDINGS
// ═══════════════════════════════════════════════════════════════════════

func applyAuditFindingCreate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		AuditID     int    `json:"audit_id"`
		FindingType string `json:"finding_type"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid finding payload: %w", err)
	}
	if payload.Title == "" {
		return "", fmt.Errorf("title is required")
	}
	if payload.AuditID == 0 {
		return "", fmt.Errorf("audit_id is required")
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
		return "", err
	}
	return fmt.Sprintf("%d", f.ID), nil
}

func applyAuditFindingUpdate(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Fields map[string]interface{} `json:"fields"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid update payload: %w", err)
	}
	idInt, _ := parseEntityID(sg.EntityID)
	for field, val := range payload.Fields {
		// Status transitions go through the shared closure-metadata path (same as
		// the HTTP handler) — a plain field write would skip closed_at/closed_by.
		if field == "status" {
			sv, ok := val.(string)
			if !ok {
				return "", fmt.Errorf("field status: expected a string, got %T", val)
			}
			if !db.AuditFindingStatuses[sv] {
				return "", fmt.Errorf("invalid status: %s", sv)
			}
			if err := db.SetAuditFindingStatusTx(ctx, tx, orgID, int(idInt), sv, actor); err != nil {
				return "", err
			}
			continue
		}
		sv, ok := val.(string)
		if !ok {
			continue
		}
		if err := db.UpdateAuditFindingFieldTx(ctx, tx, orgID, int(idInt), field, sv); err != nil {
			return "", fmt.Errorf("updating field %s: %w", field, err)
		}
	}
	return sg.EntityID, nil
}

// ═══════════════════════════════════════════════════════════════════════
// APPLY HANDLERS: READINGS
// ═══════════════════════════════════════════════════════════════════════

func applyRiskReading(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var reading db.EntityReading
	if err := json.Unmarshal(sg.Payload, &reading); err != nil {
		return "", fmt.Errorf("invalid reading payload: %w", err)
	}

	risk, err := s.db.GetRiskByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", fmt.Errorf("risk %s not found: %w", sg.EntityID, err)
	}

	reading.EntityType = "risk"
	reading.EntityID = risk.ID
	reading.AssessedBy = actor

	if err := db.CreateEntityReadingTx(ctx, tx, orgID, &reading); err != nil {
		return "", fmt.Errorf("create reading: %w", err)
	}
	if err := writeRiskFromReading(ctx, tx, s, orgID, risk.ID, &reading, actor, ""); err != nil {
		return "", err
	}
	return risk.Identifier, nil
}

func applyLegalReading(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var reading db.EntityReading
	if err := json.Unmarshal(sg.Payload, &reading); err != nil {
		return "", fmt.Errorf("invalid reading payload: %w", err)
	}

	lr, err := s.db.GetLegalRequirementByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", fmt.Errorf("legal requirement %s not found: %w", sg.EntityID, err)
	}

	reading.EntityType = "legal_requirement"
	reading.EntityID = int64(lr.ID)
	reading.AssessedBy = actor

	if err := db.CreateEntityReadingTx(ctx, tx, orgID, &reading); err != nil {
		return "", fmt.Errorf("create reading: %w", err)
	}
	if err := writeLegalFromReading(ctx, tx, s, orgID, lr.ID, &reading, actor, ""); err != nil {
		return "", err
	}
	return lr.Identifier, nil
}

// applySupplierReviewSuggestion creates a supplier review from a suggestion.
func applySupplierReviewSuggestion(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Outcome                string `json:"outcome"`
		CertificationsVerified bool   `json:"certifications_verified"`
		DataHandlingVerified   bool   `json:"data_handling_verified"`
		SLAMet                 bool   `json:"sla_met"`
		Notes                  string `json:"notes"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid review payload: %w", err)
	}

	sup, err := s.db.GetSupplierByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", fmt.Errorf("supplier %s not found: %w", sg.EntityID, err)
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
		return "", fmt.Errorf("create supplier review: %w", err)
	}
	return sup.Identifier, nil
}

// applyAccessReviewSuggestion creates an access review for a system from a suggestion.
func applyAccessReviewSuggestion(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		UsersAdded   int    `json:"users_added"`
		UsersRemoved int    `json:"users_removed"`
		Notes        string `json:"notes"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid access review payload: %w", err)
	}

	sys, err := s.db.GetSystemByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", fmt.Errorf("system %s not found: %w", sg.EntityID, err)
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
		return "", fmt.Errorf("create access review: %w", err)
	}
	return sys.Identifier, nil
}

// applyAssetReviewSuggestion creates an asset review from a suggestion.
func applyAssetReviewSuggestion(ctx context.Context, tx pgx.Tx, s *Server, orgID int, sg *db.Suggestion, actor string) (string, error) {
	var payload struct {
		Outcome                string `json:"outcome"`
		ClassificationVerified bool   `json:"classification_verified"`
		OwnershipVerified      bool   `json:"ownership_verified"`
		Notes                  string `json:"notes"`
	}
	if err := json.Unmarshal(sg.Payload, &payload); err != nil {
		return "", fmt.Errorf("invalid asset review payload: %w", err)
	}

	asset, err := s.db.GetAssetByIdentifier(ctx, orgID, sg.EntityID)
	if err != nil {
		return "", fmt.Errorf("asset %s not found: %w", sg.EntityID, err)
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
		return "", fmt.Errorf("create asset review: %w", err)
	}
	return asset.Identifier, nil
}
