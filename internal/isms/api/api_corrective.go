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
// Update fields are *string, or Optional[T] for nullable dates and numbers. A
// nil *string or an Optional that was not Set leaves the stored value alone;
// an empty string or a null Optional clears it.

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
	ExternalID  string           `json:"external_id"`
	References  []ReferenceInput `json:"references"`
}

// correctiveActionUpdateRequest is the API contract. nil / not Set = leave
// alone; null clears due_date.
// Status, when present, goes through enforceCorrectiveActionWriteTx, which
// stamps resolved_at / resolved_by_id on a transition to resolved.
type correctiveActionUpdateRequest struct {
	Title       *string            `json:"title"`
	Description *string            `json:"description"`
	Source      *string            `json:"source"`
	Severity    *string            `json:"severity"`
	Status      *string            `json:"status"`
	Assignee    *string            `json:"assignee"`
	DueDate     Optional[db.Epoch] `json:"due_date"`
	RootCause   *string            `json:"root_cause"`
	Notes       *string            `json:"notes"`
	ExternalID  *string            `json:"external_id"`
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
		ExternalID:  req.ExternalID,
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
	if err := validateCorrectiveActionCreate(&ca); err != nil {
		return err
	}
	if err := s.validateOrgMember(c, ca.Assignee); err != nil {
		return err
	}

	refs, err := s.validateReferenceInputs(ctx, orgID, taskViewer(c), req.References)
	if err != nil {
		return err
	}
	if err := s.db.CreateCorrectiveAction(ctx, orgID, &ca); err != nil {
		return pgxHTTPError(err)
	}

	s.createReferencesForEntity(ctx, orgID, "corrective_action", ca.Identifier, ca.CreatedBy, refs)

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
		// No BodyKey: the body is the action's own description, org-authored
		// content rather than product copy, and must never be translated.
		s.db.CreateNotificationContentByEmail(ctx, orgID, ca.Assignee, db.NotificationContent{
			Title:    fmt.Sprintf("Corrective action assigned: %s", ca.Title),
			TitleKey: NotifyKeyCAAssigned,
			Body:     ca.Description,
			Params:   map[string]any{"title": ca.Title},
			Link:     "/corrective-actions",
		})
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
	id, err := s.resolveCorrectiveActionID(c.Request().Context(), orgID, c.Param("id"))
	if errors.Is(err, errInvalidID) {
		return errInvalidEntityID("corrective_action")
	} else if err != nil {
		return errNotFound("corrective_action")
	}
	ca, err := s.db.GetCorrectiveAction(c.Request().Context(), orgID, id)
	if err != nil {
		return errNotFound("corrective_action")
	}
	return c.JSON(http.StatusOK, ca)
}

// prepareCorrectiveActionUpdate validates req and returns old with req merged in.
// Shared by handleUpdateCorrectiveAction and the suggestion apply handler so PUT
// and apply accept and write exactly the same fields (#200).
func (s *Server) prepareCorrectiveActionUpdate(ctx context.Context, orgID int, old *db.CorrectiveAction, req *correctiveActionUpdateRequest) (db.CorrectiveAction, error) {
	updated := *old
	if req.Severity != nil {
		if err := validateEnum("severity", *req.Severity, db.CorrectiveActionSeverities); err != nil {
			return db.CorrectiveAction{}, err
		}
	}
	if req.Source != nil {
		if err := validateEnum("source", *req.Source, db.CorrectiveActionSources); err != nil {
			return db.CorrectiveAction{}, err
		}
	}
	if req.Status != nil {
		if err := validateEnum("status", *req.Status, db.CorrectiveActionStatuses); err != nil {
			return db.CorrectiveAction{}, err
		}
	}
	if req.Assignee != nil && *req.Assignee != "" {
		if err := s.validateOrgMemberIn(ctx, orgID, *req.Assignee); err != nil {
			return db.CorrectiveAction{}, err
		}
	}

	// Status transitions flow through the unified write path below (open-task
	// guard + resolved_at/by) — the same enforced function suggestion-apply uses
	// (#26). Top-level requireRole(admin,manager) already gates status changes.
	if req.Status != nil {
		updated.Status = *req.Status
	}

	// Apply pointer-based partial update onto existing record.
	if req.Title != nil {
		updated.Title = *req.Title
	}
	if req.Description != nil {
		updated.Description = *req.Description
	}
	if req.Source != nil {
		updated.Source = *req.Source
	}
	if req.Severity != nil {
		updated.Severity = *req.Severity
	}
	if req.Assignee != nil {
		updated.Assignee = *req.Assignee
	}
	if req.DueDate.Set {
		updated.DueDate = req.DueDate.Value
	}
	if req.RootCause != nil {
		updated.RootCause = *req.RootCause
	}
	if req.Notes != nil {
		updated.Notes = *req.Notes
	}
	if req.ExternalID != nil {
		updated.ExternalID = *req.ExternalID
	}

	return updated, nil
}

func (s *Server) handleUpdateCorrectiveAction(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := s.resolveCorrectiveActionID(c.Request().Context(), orgID, c.Param("id"))
	if errors.Is(err, errInvalidID) {
		return errInvalidEntityID("corrective_action")
	} else if err != nil {
		return errNotFound("corrective_action")
	}

	ctx := c.Request().Context()
	existing, err := s.db.GetCorrectiveAction(ctx, orgID, id)
	if err != nil {
		return errNotFound("corrective_action")
	}
	prevStatus := existing.Status
	// Snapshot BEFORE the request is applied onto existing below (#196):
	// ToChangeMap returns a fresh map, so later field assignments can't touch it.
	oldMap := existing.ToChangeMap()

	var req correctiveActionUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	updated, err := s.prepareCorrectiveActionUpdate(ctx, orgID, existing, &req)
	if err != nil {
		return err
	}
	existing = &updated

	existing.ID = id
	// Single enforced CA write path (#26): open-task guard on resolve +
	// resolved_at/by, shared verbatim with suggestion-apply. The changelog is
	// written in the same transaction, diffed against the row as stored (not
	// the request), so the change and its history commit or fail together (#196).
	var after *db.CorrectiveAction
	if err := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := enforceCorrectiveActionWriteTx(ctx, tx, orgID, existing, prevStatus, getUserEmail(c)); err != nil {
			return err
		}
		var err error
		if after, err = db.GetCorrectiveActionTx(ctx, tx, orgID, id); err != nil {
			return err
		}
		changes := db.DiffFields("corrective_action", int64(id), getUserEmail(c), c.QueryParam("reason"), oldMap, after.ToChangeMap())
		return db.LogChangesTx(ctx, tx, orgID, changes)
	}); err != nil {
		var ote openTasksLinkedError
		if errors.As(err, &ote) {
			return echo.NewHTTPError(http.StatusConflict, ote.Error())
		}
		return pgxHTTPError(err)
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
	if err := requireRole(c, "admin", "manager", "contributor"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := s.resolveCorrectiveActionID(c.Request().Context(), orgID, c.Param("id"))
	if errors.Is(err, errInvalidID) {
		return errInvalidEntityID("corrective_action")
	} else if err != nil {
		return errNotFound("corrective_action")
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

	// Same enforced write path as PUT /corrective-actions/:id and suggestion-apply
	// (#26): transactional open-task guard + resolved_at/by on →resolved, with
	// the changelog written in the same transaction (#196). The row is locked and
	// re-read first, so the ownership check applies to the row being written and
	// a concurrent reassignment or edit can't slip in between (#203).
	var after *db.CorrectiveAction
	if err := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.LockCorrectiveActionTx(ctx, tx, orgID, id); err != nil {
			return err
		}
		cur, err := db.GetCorrectiveActionTx(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		// The assignee may move their own corrective action through every status,
		// resolved included (#203); the open-task guard still applies.
		if !canActOnAssignment(c, cur.Assignee) {
			return echo.NewHTTPError(http.StatusForbidden, "contributors can only change the status of corrective actions assigned to them")
		}
		prevStatus := cur.Status
		oldMap := cur.ToChangeMap()
		cur.Status = req.Status
		cur.ID = id
		if err := enforceCorrectiveActionWriteTx(ctx, tx, orgID, cur, prevStatus, actor); err != nil {
			return err
		}
		a, err := db.GetCorrectiveActionTx(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		after = a
		changes := db.DiffFields("corrective_action", int64(id), actor, c.QueryParam("reason"), oldMap, after.ToChangeMap())
		return db.LogChangesTx(ctx, tx, orgID, changes)
	}); err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he
		}
		var ote openTasksLinkedError
		if errors.As(err, &ote) {
			return echo.NewHTTPError(http.StatusConflict, ote.Error())
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return errNotFound("corrective_action")
		}
		return pgxHTTPError(err)
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "corrective_action_status_changed",
		Detail: fmt.Sprintf("Corrective action #%d status changed to %s", id, req.Status),
	})

	// On resolve, notify created_by
	if req.Status == "resolved" {
		s.db.CreateNotificationContentByEmail(ctx, orgID, after.CreatedBy, db.NotificationContent{
			Title:    fmt.Sprintf("Corrective action resolved: %s", after.Title),
			TitleKey: NotifyKeyCAResolved,
			Body:     fmt.Sprintf("Corrective action #%d has been resolved by %s", id, actor),
			BodyKey:  NotifyKeyCAResolvedBody,
			Params:   map[string]any{"title": after.Title, "id": id, "actor": actor},
			Link:     "/corrective-actions",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": req.Status})
}

// correctiveActionProgressRequest is the narrow update the assignee may make on
// their own corrective action (#203): what caused it and what was done. The wide
// PUT stays manager/admin, since it also rewrites title, severity, source and
// assignee.
type correctiveActionProgressRequest struct {
	RootCause *string `json:"root_cause"`
	Notes     *string `json:"notes"`
}

func (s *Server) handleUpdateCorrectiveActionProgress(c echo.Context) error {
	if err := requireRole(c, "admin", "manager", "contributor"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := s.resolveCorrectiveActionID(c.Request().Context(), orgID, c.Param("id"))
	if errors.Is(err, errInvalidID) {
		return errInvalidEntityID("corrective_action")
	} else if err != nil {
		return errNotFound("corrective_action")
	}

	var req correctiveActionProgressRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.RootCause == nil && req.Notes == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "root_cause or notes is required")
	}

	ctx := c.Request().Context()
	actor := getUserEmail(c)

	// Lock, re-read, check ownership and write in one transaction, so the check
	// applies to the row being written and a concurrent reassignment or edit
	// can't be overwritten (#203). Same enforced write path as the status route
	// and the wide PUT (#26), with the status unchanged, so neither the open-task
	// guard nor the resolved stamp runs; the changelog is written in the same
	// transaction (#196).
	var after *db.CorrectiveAction
	if err := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.LockCorrectiveActionTx(ctx, tx, orgID, id); err != nil {
			return err
		}
		cur, err := db.GetCorrectiveActionTx(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		if !canActOnAssignment(c, cur.Assignee) {
			return echo.NewHTTPError(http.StatusForbidden, "contributors can only update corrective actions assigned to them")
		}
		prevStatus := cur.Status
		oldMap := cur.ToChangeMap()
		if req.RootCause != nil {
			cur.RootCause = *req.RootCause
		}
		if req.Notes != nil {
			cur.Notes = *req.Notes
		}
		cur.ID = id
		if err := enforceCorrectiveActionWriteTx(ctx, tx, orgID, cur, prevStatus, actor); err != nil {
			return err
		}
		a, err := db.GetCorrectiveActionTx(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		after = a
		changes := db.DiffFields("corrective_action", int64(id), actor, c.QueryParam("reason"), oldMap, after.ToChangeMap())
		return db.LogChangesTx(ctx, tx, orgID, changes)
	}); err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return errNotFound("corrective_action")
		}
		return pgxHTTPError(err)
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "corrective_action_updated",
		Detail: fmt.Sprintf("Corrective action #%d updated: %s", id, after.Title),
	})

	return c.JSON(http.StatusOK, after)
}

func (s *Server) handleDeleteCorrectiveAction(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := s.resolveCorrectiveActionID(c.Request().Context(), orgID, c.Param("id"))
	if errors.Is(err, errInvalidID) {
		return errInvalidEntityID("corrective_action")
	} else if err != nil {
		return errNotFound("corrective_action")
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
