package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// --- Request DTOs ---
// Pointer fields use *string / **db.Epoch / **int so an explicit empty body can
// clear, and an absent body leaves the existing value alone.

type correctiveActionCreateRequest struct {
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Source      string           `json:"source"`
	Severity    string           `json:"severity"`
	Status      string           `json:"status"`
	Assignee    string           `json:"assignee"`
	DueDate     *db.Epoch        `json:"due_date"`
	RootCause   string           `json:"root_cause"`
	Notes       string           `json:"notes"`
	References  []ReferenceInput `json:"references"`
}

// correctiveActionUpdateRequest is the API contract. nil = leave alone.
// Status, when present, is routed through UpdateCorrectiveActionStatus so the
// resolved_at / resolved_by_id closure metadata is set/cleared correctly.
type correctiveActionUpdateRequest struct {
	Title       *string    `json:"title"`
	Description *string    `json:"description"`
	Source      *string    `json:"source"`
	Severity    *string    `json:"severity"`
	Status      *string    `json:"status"`
	Assignee    *string    `json:"assignee"`
	DueDate     **db.Epoch `json:"due_date"`
	RootCause   *string    `json:"root_cause"`
	Notes       *string    `json:"notes"`
}

func (s *Server) handleListCorrectiveActions(c echo.Context) error {
	orgID := getOrgID(c)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	params := db.CorrectiveActionListParams{
		Page:     page,
		Limit:    limit,
		Sort:     c.QueryParam("sort"),
		Search:   c.QueryParam("q"),
		Status:   c.QueryParam("status"),
		Severity: c.QueryParam("severity"),
		Source:   c.QueryParam("source"),
		Assignee: c.QueryParam("assignee"),
	}
	items, total, err := s.db.PaginatedCorrectiveActions(c.Request().Context(), orgID, params)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Limit < 1 {
		params.Limit = 50
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":      items,
		"total":     total,
		"page":      params.Page,
		"page_size": params.Limit,
	})
}

func (s *Server) handleCreateCorrectiveAction(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	var req correctiveActionCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ca := db.CorrectiveAction{
		Title:       req.Title,
		Description: req.Description,
		Source:      req.Source,
		Severity:    req.Severity,
		Status:      req.Status,
		Assignee:    req.Assignee,
		DueDate:     req.DueDate,
		RootCause:   req.RootCause,
		Notes:       req.Notes,
	}
	// Server-side overwrites for system-managed fields. Body values for these
	// are intentionally ignored so clients cannot spoof identity or timestamps.
	ca.CreatedBy = getUserEmail(c)
	if ca.Assignee == "" {
		ca.Assignee = ca.CreatedBy
	}
	// Server-side create defaults — shared with suggestion-apply so a CA starts
	// in the same state regardless of write path (#26).
	applyCorrectiveActionDefaults(&ca)
	if err := validateEnum("status", ca.Status, db.CorrectiveActionStatuses); err != nil {
		return err
	}
	if err := validateEnum("severity", ca.Severity, db.CorrectiveActionSeverities); err != nil {
		return err
	}
	if err := validateEnum("source", ca.Source, db.CorrectiveActionSources); err != nil {
		return err
	}
	if err := s.validateOrgMember(c, ca.Assignee); err != nil {
		return err
	}

	if err := s.db.CreateCorrectiveAction(ctx, orgID, &ca); err != nil {
		return pgxHTTPError(err)
	}

	s.createReferencesForEntity(ctx, orgID, "corrective_action", ca.Identifier, ca.CreatedBy, req.References)

	// Re-read so caller gets the canonical record (with assignee FK confirmed).
	if out, err := s.db.GetCorrectiveAction(ctx, orgID, ca.ID); err == nil {
		ca = *out
	}

	s.logChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "corrective_action",
		EntityID:   int64(ca.ID),
		Action:     "create",
		ChangedBy:  ca.CreatedBy,
	})

	s.searchUpsert(orgID, "corrective_action", ca.Identifier, ca.Title, ca.Identifier+" "+ca.Title+" "+ca.Description)

	// Log activity + notify
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  ca.CreatedBy,
		Action: "corrective_action_created",
		Detail: fmt.Sprintf("[%s] %s: %s", ca.Severity, ca.Title, ca.Description),
	})

	// Notify assignee if set
	if ca.Assignee != "" {
		s.db.CreateNotificationByEmail(ctx, orgID, ca.Assignee,
			fmt.Sprintf("Corrective action assigned: %s", ca.Title),
			ca.Description, "/corrective-actions")
		if s.mailer.Enabled() {
			s.mailer.SendBranded(ca.Assignee,
				fmt.Sprintf("Corrective Action: %s", ca.Title),
				ca.Description, s.orgMail(ctx, orgID).Branding)
		}
	}

	return c.JSON(http.StatusCreated, ca)
}

func (s *Server) handleGetCorrectiveAction(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid corrective action id")
	}
	ca, err := s.db.GetCorrectiveAction(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "corrective action not found")
	}
	return c.JSON(http.StatusOK, ca)
}

func (s *Server) handleUpdateCorrectiveAction(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid corrective action id")
	}

	ctx := c.Request().Context()
	existing, err := s.db.GetCorrectiveAction(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "corrective action not found")
	}
	prevStatus := existing.Status

	var req correctiveActionUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Severity != nil {
		if err := validateEnum("severity", *req.Severity, db.CorrectiveActionSeverities); err != nil {
			return err
		}
	}
	if req.Source != nil {
		if err := validateEnum("source", *req.Source, db.CorrectiveActionSources); err != nil {
			return err
		}
	}
	if req.Status != nil {
		if err := validateEnum("status", *req.Status, db.CorrectiveActionStatuses); err != nil {
			return err
		}
	}
	if req.Assignee != nil && *req.Assignee != "" {
		if err := s.validateOrgMember(c, *req.Assignee); err != nil {
			return err
		}
	}

	// Status transitions flow through the unified write path below (open-task
	// guard + resolved_at/by) — the same enforced function suggestion-apply uses
	// (#26). Top-level requireRole(admin,manager) already gates status changes.
	if req.Status != nil {
		existing.Status = *req.Status
	}

	// Apply pointer-based partial update onto existing record.
	if req.Title != nil {
		existing.Title = *req.Title
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Source != nil {
		existing.Source = *req.Source
	}
	if req.Severity != nil {
		existing.Severity = *req.Severity
	}
	if req.Assignee != nil {
		existing.Assignee = *req.Assignee
	}
	if req.DueDate != nil {
		existing.DueDate = *req.DueDate
	}
	if req.RootCause != nil {
		existing.RootCause = *req.RootCause
	}
	if req.Notes != nil {
		existing.Notes = *req.Notes
	}

	oldMap := existing.ToChangeMap()
	existing.ID = id
	// Single enforced CA write path (#26): open-task guard on resolve +
	// resolved_at/by, shared verbatim with suggestion-apply.
	if err := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		return enforceCorrectiveActionWriteTx(ctx, tx, orgID, existing, prevStatus, getUserEmail(c))
	}); err != nil {
		var ote openTasksLinkedError
		if errors.As(err, &ote) {
			return echo.NewHTTPError(http.StatusConflict, ote.Error())
		}
		return pgxHTTPError(err)
	}

	// Re-read so the response is canonical (assignee FK confirmed, updated_at, etc.).
	after, _ := s.db.GetCorrectiveAction(ctx, orgID, id)
	if after != nil {
		actor := getUserEmail(c)
		reason := c.QueryParam("reason")
		changes := db.DiffFields("corrective_action", int64(id), actor, reason, oldMap, after.ToChangeMap())
		if len(changes) > 0 {
			s.logChanges(ctx, orgID, changes)
		}
	}

	s.searchUpsert(orgID, "corrective_action", existing.Identifier, existing.Title, existing.Identifier+" "+existing.Title+" "+existing.Description)

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  getUserEmail(c),
		Action: "corrective_action_updated",
		Detail: fmt.Sprintf("Corrective action #%d updated: %s", id, existing.Title),
	})

	if after != nil {
		return c.JSON(http.StatusOK, after)
	}
	return c.JSON(http.StatusOK, existing)
}

func (s *Server) handleUpdateCorrectiveActionStatus(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid corrective action id")
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := validateEnum("status", req.Status, db.CorrectiveActionStatuses); err != nil {
		return err
	}

	ctx := c.Request().Context()
	actor := getUserEmail(c)

	// Block resolving if there are still-open implementation tasks linked to this CA.
	if req.Status == "resolved" {
		existing, err := s.db.GetCorrectiveAction(ctx, orgID, id)
		if err != nil || existing == nil {
			return echo.NewHTTPError(http.StatusNotFound, "corrective action not found")
		}
		// An empty identifier is corrupt data — the open-task query can't
		// match anything, so skipping would silently disable enforcement.
		if existing.Identifier == "" {
			return echo.NewHTTPError(http.StatusInternalServerError, "corrective action has no identifier")
		}
		if n, err := s.db.CountOpenTasksByCA(ctx, orgID, existing.Identifier); err == nil && n > 0 {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("cannot resolve %s: %d open implementation task(s) still linked", existing.Identifier, n))
		}
	}

	if err := s.db.UpdateCorrectiveActionStatus(ctx, orgID, id, req.Status, actor); err != nil {
		return pgxHTTPError(err)
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "corrective_action_status_changed",
		Detail: fmt.Sprintf("Corrective action #%d status changed to %s", id, req.Status),
	})

	// On resolve, notify created_by
	if req.Status == "resolved" {
		ca, err := s.db.GetCorrectiveAction(ctx, orgID, id)
		if err == nil {
			s.db.CreateNotificationByEmail(ctx, orgID, ca.CreatedBy,
				fmt.Sprintf("Corrective action resolved: %s", ca.Title),
				fmt.Sprintf("Corrective action #%d has been resolved by %s", id, actor),
				"/corrective-actions")
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"status": req.Status})
}

func (s *Server) handleDeleteCorrectiveAction(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid corrective action id")
	}

	ctx := c.Request().Context()
	old, _ := s.db.GetCorrectiveAction(ctx, orgID, id)
	if err := s.db.DeleteCorrectiveAction(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}

	if old != nil {
		s.logChange(ctx, orgID, &db.ChangelogEntry{
			EntityType: "corrective_action",
			EntityID:   int64(old.ID),
			Action:     "delete",
			ChangedBy:  getUserEmail(c),
		})
	}

	identifier := ""
	if old != nil {
		identifier = old.Identifier
	}
	s.searchRemove(orgID, "corrective_action", identifier)

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  getUserEmail(c),
		Action: "corrective_action_deleted",
		Detail: fmt.Sprintf("%s deleted", identifier),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleCorrectiveActionStats(c echo.Context) error {
	orgID := getOrgID(c)
	stats, err := s.db.CorrectiveActionStats(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}
