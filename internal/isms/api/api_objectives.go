package api

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// --- Request DTOs ---

type programCreateRequest struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Notes       string `json:"notes"`
	Owner       string `json:"owner"`
}

type programUpdateRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Notes       *string `json:"notes"`
	Owner       *string `json:"owner"`
}

type objectiveCreateRequest struct {
	ProgramID         int64            `json:"program_id"`
	Title             string           `json:"title"`
	Description       string           `json:"description"`
	Owner             string           `json:"owner"`
	Source            string           `json:"source"`
	MeasurementMethod string           `json:"measurement_method"`
	TargetValue       *float64         `json:"target_value"`
	TargetOperator    string           `json:"target_operator"`
	Unit              string           `json:"unit"`
	WindowSeconds     *int             `json:"window_seconds"`
	GraceSeconds      int              `json:"grace_seconds"`
	CheckinCycle      int              `json:"checkin_cycle"`
	Status            string           `json:"status"`
	StartedAt         *db.Epoch        `json:"started_at"`
	Notes             string           `json:"notes"`
	References        []ReferenceInput `json:"references"`
}

type objectiveUpdateRequest struct {
	Title             *string    `json:"title"`
	Description       *string    `json:"description"`
	Owner             *string    `json:"owner"`
	Source            *string    `json:"source"`
	MeasurementMethod *string    `json:"measurement_method"`
	TargetValue       **float64  `json:"target_value"`
	TargetOperator    *string    `json:"target_operator"`
	Unit              *string    `json:"unit"`
	WindowSeconds     **int      `json:"window_seconds"`
	GraceSeconds      *int       `json:"grace_seconds"`
	CheckinCycle      *int       `json:"checkin_cycle"`
	Status            *string    `json:"status"`
	StartedAt         **db.Epoch `json:"started_at"`
	Notes             *string    `json:"notes"`
}

type checkinCreateRequest struct {
	OccurredAt   db.Epoch `json:"occurred_at"`
	Success      *bool    `json:"success"`
	ValueNumeric *float64 `json:"value_numeric"`
	Message      string   `json:"message"`
	PublicNote   string   `json:"public_note"`
}

type checkinUpdateRequest struct {
	OccurredAt   *db.Epoch `json:"occurred_at"`
	Success      **bool    `json:"success"`
	ValueNumeric **float64 `json:"value_numeric"`
	Message      *string   `json:"message"`
	PublicNote   *string   `json:"public_note"`
}

// ═══════════════════════════════════════════════════════════════════════
// PROGRAMS
// ═══════════════════════════════════════════════════════════════════════

func (s *Server) handleListPrograms(c echo.Context) error {
	orgID := getOrgID(c)
	programs, err := s.db.ListPrograms(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": programs})
}

func (s *Server) handleCreateProgram(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	var req programCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	p := db.Program{
		Key:         strings.ToUpper(strings.TrimSpace(req.Key)),
		Title:       req.Title,
		Description: req.Description,
		Notes:       req.Notes,
		Owner:       req.Owner,
	}

	if p.Key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}
	if p.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}

	if err := s.db.CreateProgram(ctx, orgID, &p); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "program",
		EntityID:   p.ID,
		Action:     "create",
		ChangedBy:  user,
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "program_created",
		Detail: fmt.Sprintf("Program %s: %s", p.Key, p.Title),
	})

	return c.JSON(http.StatusCreated, p)
}

func (s *Server) handleGetProgram(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	p, err := s.db.GetProgram(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "program not found")
	}
	return c.JSON(http.StatusOK, p)
}

func (s *Server) handleUpdateProgram(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	old, err := s.db.GetProgram(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "program not found")
	}

	var req programUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// programs.key is immutable after creation — preserve old.
	p := *old
	p.ID = id
	if req.Title != nil {
		if *req.Title == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "title cannot be empty")
		}
		p.Title = *req.Title
	}
	if req.Description != nil {
		p.Description = *req.Description
	}
	if req.Notes != nil {
		p.Notes = *req.Notes
	}
	if req.Owner != nil {
		p.Owner = *req.Owner
	}

	if err := s.db.UpdateProgram(ctx, orgID, &p); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	after, _ := s.db.GetProgram(ctx, orgID, id)
	if after == nil {
		after = &p
	}
	diffs := db.DiffFields("program", id, user, "", old.ToChangeMap(), after.ToChangeMap())
	_ = s.db.LogChanges(ctx, orgID, diffs)

	return c.JSON(http.StatusOK, after)
}

func (s *Server) handleDeleteProgram(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	if err := s.db.DeleteProgram(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "program",
		EntityID:   id,
		Action:     "delete",
		ChangedBy:  user,
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "program_deleted",
		Detail: fmt.Sprintf("Program %d deleted", id),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// ═══════════════════════════════════════════════════════════════════════
// OBJECTIVES
// ═══════════════════════════════════════════════════════════════════════

func (s *Server) handleListObjectives(c echo.Context) error {
	orgID := getOrgID(c)
	var programID int64
	if v := c.QueryParam("program_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			programID = n
		}
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	params := db.ObjectiveListParams{
		Page:      page,
		Limit:     limit,
		Sort:      c.QueryParam("sort"),
		Search:    c.QueryParam("q"),
		Status:    c.QueryParam("status"),
		ProgramID: programID,
		Owner:     c.QueryParam("owner"),
	}
	items, total, err := s.db.PaginatedObjectives(c.Request().Context(), orgID, params)
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

func (s *Server) handleObjectiveStats(c echo.Context) error {
	orgID := getOrgID(c)
	stats, err := s.db.ObjectiveStats(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) handleCreateObjective(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	var req objectiveCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.ProgramID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "program_id is required")
	}
	if req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}
	if err := validateEnum("status", req.Status, db.ObjectiveStatuses); err != nil {
		return err
	}
	if err := validateEnum("target_operator", req.TargetOperator, db.ObjectiveTargetOperators); err != nil {
		return err
	}
	// Cross-org parent guard: confirm the program belongs to this org.
	if _, err := s.db.GetProgram(ctx, orgID, req.ProgramID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "program not found in this organization")
	}
	o := db.Objective{
		ProgramID:         req.ProgramID,
		Title:             req.Title,
		Description:       req.Description,
		Owner:             req.Owner,
		Source:            req.Source,
		MeasurementMethod: req.MeasurementMethod,
		TargetValue:       req.TargetValue,
		TargetOperator:    req.TargetOperator,
		Unit:              req.Unit,
		WindowSeconds:     req.WindowSeconds,
		GraceSeconds:      req.GraceSeconds,
		CheckinCycle:      req.CheckinCycle,
		Status:            req.Status,
		StartedAt:         req.StartedAt,
		Notes:             req.Notes,
	}

	if err := s.db.CreateObjective(ctx, orgID, &o); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	s.createReferencesForEntity(ctx, orgID, "objective", o.DisplayID, user, req.References)

	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "objective",
		EntityID:   o.ID,
		Action:     "create",
		ChangedBy:  user,
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "objective_created",
		Detail: fmt.Sprintf("%s: %s", o.DisplayID, o.Title),
	})

	return c.JSON(http.StatusCreated, o)
}

func (s *Server) handleGetObjective(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	o, err := s.db.GetObjective(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "objective not found")
	}
	return c.JSON(http.StatusOK, o)
}

func (s *Server) handleUpdateObjective(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	old, err := s.db.GetObjective(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "objective not found")
	}

	var req objectiveUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Status != nil {
		if err := validateEnum("status", *req.Status, db.ObjectiveStatuses); err != nil {
			return err
		}
	}
	if req.TargetOperator != nil {
		if err := validateEnum("target_operator", *req.TargetOperator, db.ObjectiveTargetOperators); err != nil {
			return err
		}
	}
	// Apply pointer-based partial update onto the existing record.
	o := *old
	o.ID = id
	if req.Title != nil {
		if *req.Title == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "title cannot be empty")
		}
		o.Title = *req.Title
	}
	if req.Description != nil {
		o.Description = *req.Description
	}
	if req.Owner != nil {
		o.Owner = *req.Owner
	}
	if req.Source != nil {
		o.Source = *req.Source
	}
	if req.MeasurementMethod != nil {
		o.MeasurementMethod = *req.MeasurementMethod
	}
	if req.TargetValue != nil {
		o.TargetValue = *req.TargetValue
	}
	if req.TargetOperator != nil {
		o.TargetOperator = *req.TargetOperator
	}
	if req.Unit != nil {
		o.Unit = *req.Unit
	}
	if req.WindowSeconds != nil {
		o.WindowSeconds = *req.WindowSeconds
	}
	if req.GraceSeconds != nil {
		o.GraceSeconds = *req.GraceSeconds
	}
	if req.CheckinCycle != nil && *req.CheckinCycle > 0 {
		o.CheckinCycle = *req.CheckinCycle
	}
	if req.Status != nil {
		o.Status = *req.Status
	}
	if req.StartedAt != nil {
		o.StartedAt = *req.StartedAt
	}
	if req.Notes != nil {
		o.Notes = *req.Notes
	}

	if err := s.db.UpdateObjective(ctx, orgID, &o); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	after, _ := s.db.GetObjective(ctx, orgID, id)
	if after == nil {
		after = &o
	}
	diffs := db.DiffFields("objective", id, user, "", old.ToChangeMap(), after.ToChangeMap())
	_ = s.db.LogChanges(ctx, orgID, diffs)

	return c.JSON(http.StatusOK, after)
}

func (s *Server) handleDeleteObjective(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	if err := s.db.DeleteObjective(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "objective",
		EntityID:   id,
		Action:     "delete",
		ChangedBy:  user,
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "objective_deleted",
		Detail: fmt.Sprintf("Objective %d deleted", id),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleArchiveObjective(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	before, _ := s.db.GetObjective(ctx, orgID, id)
	oldStatus := ""
	if before != nil {
		oldStatus = before.Status
	}

	if err := s.db.ArchiveObjective(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	newStatus := "archived"
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "objective",
		EntityID:   id,
		Action:     "update",
		Field:      "status",
		OldValue:   &oldStatus,
		NewValue:   &newStatus,
		ChangedBy:  user,
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "objective_archived",
		Detail: fmt.Sprintf("Objective %d archived", id),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "archived"})
}

func (s *Server) handleUnarchiveObjective(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	before, _ := s.db.GetObjective(ctx, orgID, id)
	oldStatus := ""
	if before != nil {
		oldStatus = before.Status
	}

	if err := s.db.UnarchiveObjective(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	after, _ := s.db.GetObjective(ctx, orgID, id)
	newStatus := ""
	if after != nil {
		newStatus = after.Status
	}
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "objective",
		EntityID:   id,
		Action:     "update",
		Field:      "status",
		OldValue:   &oldStatus,
		NewValue:   &newStatus,
		ChangedBy:  user,
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "objective_unarchived",
		Detail: fmt.Sprintf("Objective %d unarchived", id),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "unarchived"})
}

// ═══════════════════════════════════════════════════════════════════════
// CHECKINS
// ═══════════════════════════════════════════════════════════════════════

func (s *Server) handleListCheckins(c echo.Context) error {
	orgID := getOrgID(c)
	objectiveID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid objective id")
	}

	limit := 50
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	offset := 0
	if v := c.QueryParam("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	checkins, err := s.db.ListCheckins(c.Request().Context(), orgID, objectiveID, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": checkins})
}

func (s *Server) handleCreateCheckin(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	objectiveID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid objective id")
	}

	var req checkinCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// Verify the objective belongs to this org before creating a checkin against it.
	if _, err := s.db.GetObjective(ctx, orgID, objectiveID); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "objective not found in this organization")
	}
	ci := db.Checkin{
		ObjectiveID:  objectiveID,
		OccurredAt:   req.OccurredAt,
		Success:      req.Success,
		ValueNumeric: req.ValueNumeric,
		Message:      req.Message,
		PublicNote:   req.PublicNote,
		CreatedBy:    getUserEmail(c),
	}

	if err := s.db.CreateCheckin(ctx, orgID, &ci); err != nil {
		return pgxHTTPError(err)
	}

	// Look up objective for display_id in log
	obj, _ := s.db.GetObjective(ctx, orgID, objectiveID)
	displayID := fmt.Sprintf("%d", objectiveID)
	if obj != nil {
		displayID = obj.DisplayID
	}

	user := getUserEmail(c)
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "checkin",
		EntityID:   ci.ID,
		Action:     "create",
		ChangedBy:  user,
	})
	detail := fmt.Sprintf("Checkin on %s", displayID)
	if ci.ValueNumeric != nil {
		detail += fmt.Sprintf(": value=%g", *ci.ValueNumeric)
	}
	if ci.Success != nil {
		if *ci.Success {
			detail += " (pass)"
		} else {
			detail += " (fail)"
		}
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "checkin_created",
		Detail: detail,
	})

	return c.JSON(http.StatusCreated, ci)
}

func (s *Server) handleUpdateCheckin(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	existing, err := s.db.GetCheckin(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "checkin not found")
	}

	var req checkinUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ci := *existing
	ci.ID = id
	ci.ObjectiveID = existing.ObjectiveID
	if req.OccurredAt != nil && !req.OccurredAt.IsZero() {
		ci.OccurredAt = *req.OccurredAt
	}
	if req.Success != nil {
		ci.Success = *req.Success
	}
	if req.ValueNumeric != nil {
		ci.ValueNumeric = *req.ValueNumeric
	}
	if req.Message != nil {
		ci.Message = *req.Message
	}
	if req.PublicNote != nil {
		ci.PublicNote = *req.PublicNote
	}

	if err := s.db.UpdateCheckin(ctx, orgID, &ci); err != nil {
		return pgxHTTPError(err)
	}

	after, _ := s.db.GetCheckin(ctx, orgID, id)
	if after == nil {
		after = &ci
	}

	user := getUserEmail(c)
	if changes := db.DiffFields("checkin", id, user, "", existing.ToChangeMap(), after.ToChangeMap()); len(changes) > 0 {
		_ = s.db.LogChanges(ctx, orgID, changes)
	}

	objLabel := fmt.Sprintf("#%d", existing.ObjectiveID)
	if obj, _ := s.db.GetObjective(ctx, orgID, existing.ObjectiveID); obj != nil {
		objLabel = obj.DisplayID
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "checkin_updated",
		Detail: fmt.Sprintf("Checkin #%d updated for objective %s", id, objLabel),
	})

	return c.JSON(http.StatusOK, after)
}

func (s *Server) handleDeleteCheckin(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	if err := s.db.DeleteCheckin(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "checkin",
		EntityID:   id,
		Action:     "delete",
		ChangedBy:  user,
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// ═══════════════════════════════════════════════════════════════════════
// EVIDENCE (S3-backed file attachments)
// ═══════════════════════════════════════════════════════════════════════

func (s *Server) handleUploadEvidence(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	checkinID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid checkin id")
	}

	// Verify checkin exists and belongs to org
	_, err = s.db.GetCheckin(ctx, orgID, checkinID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "checkin not found")
	}

	// Get org UUID for S3 key
	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "organization not found")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}

	title := c.FormValue("title")
	if title == "" {
		title = file.Filename
	}

	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}
	defer src.Close()

	// Read file content to compute hash and get size
	content, err := io.ReadAll(src)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file content")
	}

	hash := sha256.Sum256(content)
	hashStr := fmt.Sprintf("%x", hash)
	size := int64(len(content))

	ext := filepath.Ext(file.Filename)
	objectKey := fmt.Sprintf("evidence/%d/%s%s", checkinID, uuid.New().String(), ext)
	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	if err := s.blobs.PutStream(ctx, org.UUID, objectKey, contentType, bytes.NewReader(content)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to upload: "+err.Error())
	}

	// Record in database
	ev := db.Evidence{
		CheckinID:   checkinID,
		Title:       title,
		ObjectKey:   objectKey,
		ContentType: contentType,
		SizeBytes:   &size,
		SHA256:      hashStr,
	}
	if err := s.db.CreateEvidence(ctx, orgID, &ev); err != nil {
		_ = s.blobs.Delete(ctx, org.UUID, objectKey)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	user := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "evidence_uploaded",
		Detail: fmt.Sprintf("Evidence '%s' uploaded for checkin %d", title, checkinID),
	})

	return c.JSON(http.StatusCreated, ev)
}

func (s *Server) handleListEvidence(c echo.Context) error {
	orgID := getOrgID(c)
	checkinID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid checkin id")
	}

	evidence, err := s.db.ListEvidence(c.Request().Context(), orgID, checkinID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": evidence})
}

func (s *Server) handleDownloadEvidence(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "organization not found")
	}

	ev, err := s.db.GetEvidence(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "evidence not found")
	}

	// S3 backend: return presigned URL. Local backend: serve file directly.
	url, err := s.blobs.URL(ctx, org.UUID, ev.ObjectKey, 15*time.Minute)
	if err == nil && url != "" {
		return c.JSON(http.StatusOK, map[string]string{
			"url":          url,
			"title":        ev.Title,
			"content_type": ev.ContentType,
		})
	}

	// Local backend: read and serve directly
	data, err := s.blobs.Get(ctx, org.UUID, ev.ObjectKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "evidence file not found")
	}
	ct := ev.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, ev.Title))
	return c.Blob(http.StatusOK, ct, data)
}

func (s *Server) handleDeleteEvidence(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}

	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "organization not found")
	}

	ev, err := s.db.GetEvidence(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "evidence not found")
	}

	_ = s.blobs.Delete(ctx, org.UUID, ev.ObjectKey)

	// Delete from database
	if err := s.db.DeleteEvidence(ctx, orgID, id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	user := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "evidence_deleted",
		Detail: fmt.Sprintf("Evidence '%s' deleted", ev.Title),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
