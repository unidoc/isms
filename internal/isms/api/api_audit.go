package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// --- Request DTOs ---
// Pointer fields use *string / **db.Epoch so an explicit empty body can clear,
// and an absent body leaves the existing value alone.

type auditProgrammeCreateRequest struct {
	Title       string `json:"title"`
	Year        int    `json:"year"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Notes       string `json:"notes"`
}

type auditProgrammeUpdateRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Notes       *string `json:"notes"`
	Status      *string `json:"status"`
}

type auditCreateRequest struct {
	ProgrammeID *int      `json:"programme_id"`
	Title       string    `json:"title"`
	Scope       string    `json:"scope"`
	Auditor     string    `json:"auditor"`
	AuditType   string    `json:"audit_type"`
	Status      string    `json:"status"`
	PlannedDate *db.Epoch `json:"planned_date"`
	EndDate     *db.Epoch `json:"end_date"`
	Notes       string    `json:"notes"`
}

// auditUpdateRequest is the API contract for updating an audit. nil = leave alone.
// Status, when present, is routed through UpdateAuditStatus so it goes through
// the same code path used by tools/CLI.
type auditUpdateRequest struct {
	Title       *string    `json:"title"`
	Scope       *string    `json:"scope"`
	Auditor     *string    `json:"auditor"`
	AuditType   *string    `json:"audit_type"`
	Status      *string    `json:"status"`
	Summary     *string    `json:"summary"`
	Notes       *string    `json:"notes"`
	PlannedDate **db.Epoch `json:"planned_date"`
	EndDate     **db.Epoch `json:"end_date"`
}

type auditItemCreateRequest struct {
	ItemID   string `json:"item_id"`
	ItemType string `json:"item_type"`
	Title    string `json:"title"`
	Result   string `json:"result"`
	Evidence string `json:"evidence"`
	Notes    string `json:"notes"`
}

type auditItemUpdateRequest struct {
	Result   *string `json:"result"`
	Evidence *string `json:"evidence"`
	Notes    *string `json:"notes"`
}

type auditFindingCreateRequest struct {
	AuditID     int       `json:"audit_id"`
	AuditItemID *int      `json:"audit_item_id"`
	FindingType string    `json:"finding_type"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	DueDate     *db.Epoch `json:"due_date"`
	Owner       string    `json:"owner"`
}

type auditFindingUpdateRequest struct {
	Title       *string    `json:"title"`
	Description *string    `json:"description"`
	Owner       *string    `json:"owner"`
	DueDate     **db.Epoch `json:"due_date"`
	Status      *string    `json:"status"`
}

// --- Audit Programmes ---

func (s *Server) handleListAuditProgrammes(c echo.Context) error {
	orgID := getOrgID(c)
	programmes, err := s.db.ListAuditProgrammes(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": programmes})
}

func (s *Server) handleCreateAuditProgramme(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	var req auditProgrammeCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}
	if req.Year < 1900 || req.Year > 2200 {
		return echo.NewHTTPError(http.StatusBadRequest, "year must be between 1900 and 2200")
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if !db.AuditProgrammeStatuses[req.Status] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid status: "+req.Status)
	}
	p := db.AuditProgramme{
		Title:       req.Title,
		Year:        req.Year,
		Description: req.Description,
		Status:      req.Status,
		Notes:       req.Notes,
		CreatedBy:   getUserEmail(c),
	}
	if err := s.db.CreateAuditProgramme(c.Request().Context(), orgID, &p); err != nil {
		return pgxHTTPError(err)
	}
	s.logAndNotify(c.Request().Context(), orgID, &db.Activity{
		Actor:  getUserEmail(c),
		Action: "audit_programme_created",
		Detail: fmt.Sprintf("Created audit programme: %s (%d)", p.Title, p.Year),
	})
	return c.JSON(http.StatusCreated, p)
}

func (s *Server) handleGetAuditProgramme(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid programme id")
	}
	p, err := s.db.GetAuditProgramme(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "programme not found")
	}
	return c.JSON(http.StatusOK, p)
}

func (s *Server) handleUpdateAuditProgramme(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid programme id")
	}
	var req auditProgrammeUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Status != nil && !db.AuditProgrammeStatuses[*req.Status] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid status: "+*req.Status)
	}
	if err := s.db.UpdateAuditProgramme(c.Request().Context(), orgID, id, req.Title, req.Description, req.Notes, req.Status); err != nil {
		return pgxHTTPError(err)
	}
	p, err := s.db.GetAuditProgramme(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "programme not found")
	}
	s.logAndNotify(c.Request().Context(), orgID, &db.Activity{
		Actor:  getUserEmail(c),
		Action: "audit_programme_updated",
		Detail: fmt.Sprintf("Programme #%d updated", id),
	})
	return c.JSON(http.StatusOK, p)
}

func (s *Server) handleDeleteAuditProgramme(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid programme id")
	}
	hasAudits, err := s.db.AuditProgrammeHasAudits(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if hasAudits {
		return echo.NewHTTPError(http.StatusConflict, "cannot delete programme with existing audits")
	}
	if err := s.db.DeleteAuditProgramme(c.Request().Context(), orgID, id); err != nil {
		return pgxHTTPError(err)
	}
	s.logAndNotify(c.Request().Context(), orgID, &db.Activity{
		Actor:  getUserEmail(c),
		Action: "audit_programme_deleted",
		Detail: fmt.Sprintf("Programme #%d deleted", id),
	})
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Audits ---

func (s *Server) handleListAudits(c echo.Context) error {
	orgID := getOrgID(c)
	var pid *int
	if v := c.QueryParam("programme_id"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid programme_id")
		}
		pid = &id
	}
	audits, err := s.db.ListAudits(c.Request().Context(), orgID, pid)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": audits})
}

func (s *Server) handleCreateAudit(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	var req auditCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}
	// Scope and auditor are optional at create time — fill via the edit modal.
	// Schema permits null auditor_id and empty scope.
	if req.AuditType == "" {
		req.AuditType = "internal"
	}
	if !db.AuditTypes[req.AuditType] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid audit_type: "+req.AuditType)
	}
	if req.Status == "" {
		req.Status = "planned"
	}
	if !db.AuditStatuses[req.Status] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid status: "+req.Status)
	}
	if err := s.validateOrgMember(c, req.Auditor); err != nil {
		return err
	}
	if req.ProgrammeID != nil {
		if _, err := s.db.GetAuditProgramme(ctx, orgID, *req.ProgrammeID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "programme not found in this organization")
		}
	}

	a := db.Audit{
		ProgrammeID: req.ProgrammeID,
		Title:       req.Title,
		Scope:       req.Scope,
		Auditor:     req.Auditor,
		AuditType:   req.AuditType,
		Status:      req.Status,
		PlannedDate: req.PlannedDate,
		EndDate:     req.EndDate,
		Notes:       req.Notes,
	}
	if err := s.db.CreateAudit(ctx, orgID, &a); err != nil {
		return pgxHTTPError(err)
	}
	// Re-read so we return canonical state (auditor email confirmed via FK).
	out, err := s.db.GetAudit(ctx, orgID, a.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	s.logChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "audit",
		EntityID:   int64(out.ID),
		Action:     "create",
		ChangedBy:  getUserEmail(c),
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  getUserEmail(c),
		Action: "audit_created",
		Detail: fmt.Sprintf("Created audit: %s", out.Title),
	})
	return c.JSON(http.StatusCreated, out)
}

func (s *Server) handleGetAudit(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid audit id")
	}
	audit, err := s.db.GetAudit(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "audit not found")
	}
	return c.JSON(http.StatusOK, audit)
}

func (s *Server) handleUpdateAudit(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid audit id")
	}
	ctx := c.Request().Context()
	before, err := s.db.GetAudit(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "audit not found")
	}

	var req auditUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.AuditType != nil && !db.AuditTypes[*req.AuditType] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid audit_type: "+*req.AuditType)
	}
	if req.Status != nil && !db.AuditStatuses[*req.Status] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid status: "+*req.Status)
	}
	if req.Auditor != nil && *req.Auditor != "" {
		if err := s.validateOrgMember(c, *req.Auditor); err != nil {
			return err
		}
	}
	// Route status changes through the dedicated transition function.
	if req.Status != nil && *req.Status != before.Status {
		if err := s.db.UpdateAuditStatus(ctx, orgID, id, *req.Status); err != nil {
			return pgxHTTPError(err)
		}
	}
	if err := s.db.UpdateAudit(ctx, orgID, id, req.Title, req.Scope, req.Auditor, req.AuditType, req.Summary, req.Notes, req.PlannedDate, req.EndDate); err != nil {
		return pgxHTTPError(err)
	}
	after, err := s.db.GetAudit(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "audit not found")
	}
	actor := getUserEmail(c)
	if changes := db.DiffFields("audit", int64(id), actor, c.QueryParam("reason"), before.ToChangeMap(), after.ToChangeMap()); len(changes) > 0 {
		s.logChanges(ctx, orgID, changes)
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "audit_updated",
		Detail: fmt.Sprintf("Audit #%d updated", id),
	})
	return c.JSON(http.StatusOK, after)
}

func (s *Server) handleUpdateAuditStatus(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid audit id")
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !db.AuditStatuses[req.Status] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid status: "+req.Status)
	}
	ctx := c.Request().Context()
	before, err := s.db.GetAudit(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "audit not found")
	}
	if err := s.db.UpdateAuditStatus(ctx, orgID, id, req.Status); err != nil {
		return pgxHTTPError(err)
	}
	after, err := s.db.GetAudit(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	actor := getUserEmail(c)
	if changes := db.DiffFields("audit", int64(id), actor, "", before.ToChangeMap(), after.ToChangeMap()); len(changes) > 0 {
		s.logChanges(ctx, orgID, changes)
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "audit_status_changed",
		Detail: fmt.Sprintf("Audit #%d status changed to %s", id, req.Status),
	})
	return c.JSON(http.StatusOK, map[string]string{"status": req.Status})
}

// --- Audit Calendar ---

func (s *Server) handleAuditCalendar(c echo.Context) error {
	orgID := getOrgID(c)
	yearStr := c.QueryParam("year")
	if yearStr == "" {
		yearStr = strconv.Itoa(time.Now().Year())
	}
	yearInt, err := strconv.Atoi(yearStr)
	if err != nil || yearInt < 1900 || yearInt > 2200 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid year")
	}

	scheduled, err := s.db.ListAuditsForYear(c.Request().Context(), orgID, yearInt)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	unscheduled, err := s.db.ListUnscheduledAudits(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	type MonthData struct {
		Month  int        `json:"month"`
		Name   string     `json:"name"`
		Audits []db.Audit `json:"audits"`
	}

	monthNames := []string{
		"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December",
	}
	months := make([]MonthData, 12)
	for i := 0; i < 12; i++ {
		months[i] = MonthData{Month: i + 1, Name: monthNames[i], Audits: []db.Audit{}}
	}
	for _, a := range scheduled {
		if a.PlannedDate != nil {
			m := int(a.PlannedDate.Month()) - 1
			if m >= 0 && m < 12 {
				months[m].Audits = append(months[m].Audits, a)
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"year":        yearInt,
		"months":      months,
		"unscheduled": unscheduled,
	})
}

// --- Audit Items ---

func (s *Server) handleListAuditItems(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid audit id")
	}
	if exists, err := s.db.AuditExists(c.Request().Context(), orgID, id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	} else if !exists {
		return echo.NewHTTPError(http.StatusNotFound, "audit not found")
	}
	items, err := s.db.ListAuditItems(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": items})
}

func (s *Server) handleCreateAuditItem(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	auditID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid audit id")
	}
	if exists, err := s.db.AuditExists(c.Request().Context(), orgID, auditID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	} else if !exists {
		return echo.NewHTTPError(http.StatusNotFound, "audit not found")
	}
	var req auditItemCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}
	if req.Result == "" {
		req.Result = "not_assessed"
	}
	if !db.AuditItemResults[req.Result] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid result: "+req.Result)
	}
	item := &db.AuditItem{
		AuditID:  auditID,
		ItemID:   req.ItemID,
		ItemType: req.ItemType,
		Title:    req.Title,
		Result:   req.Result,
		Evidence: req.Evidence,
		Notes:    req.Notes,
	}
	if err := s.db.AddAuditItem(c.Request().Context(), orgID, item); err != nil {
		return pgxHTTPError(err)
	}
	return c.JSON(http.StatusCreated, item)
}

func (s *Server) handleDeleteAuditItem(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid item id")
	}
	if err := s.db.DeleteAuditItem(c.Request().Context(), orgID, id); err != nil {
		return pgxHTTPError(err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleUpdateAuditItem(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid item id")
	}
	var req auditItemUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Result != nil && !db.AuditItemResults[*req.Result] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid result: "+*req.Result)
	}
	if err := s.db.UpdateAuditItem(c.Request().Context(), orgID, id, req.Result, req.Evidence, req.Notes, getUserEmail(c)); err != nil {
		return pgxHTTPError(err)
	}
	out, err := s.db.GetAuditItem(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "item not found")
	}
	if req.Result != nil {
		s.logAndNotify(c.Request().Context(), orgID, &db.Activity{
			Actor:  getUserEmail(c),
			Action: "audit_item_assessed",
			Detail: fmt.Sprintf("Audit item #%d assessed: %s", id, *req.Result),
		})
	}
	return c.JSON(http.StatusOK, out)
}

// --- Audit Findings ---

func (s *Server) handleListAuditFindingsForAudit(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid audit id")
	}
	if exists, err := s.db.AuditExists(c.Request().Context(), orgID, id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	} else if !exists {
		return echo.NewHTTPError(http.StatusNotFound, "audit not found")
	}
	findings, err := s.db.ListAuditFindings(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": findings})
}

// handlePaginatedAuditFindings is the new server-side aggregate list endpoint
// that replaces the previous N+1 client-side filtering on the frontend.
func (s *Server) handlePaginatedAuditFindings(c echo.Context) error {
	orgID := getOrgID(c)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	auditID, _ := strconv.Atoi(c.QueryParam("audit_id"))
	progID, _ := strconv.Atoi(c.QueryParam("programme_id"))

	params := db.AuditFindingListParams{
		Page:        page,
		Limit:       limit,
		Sort:        c.QueryParam("sort"),
		Search:      c.QueryParam("q"),
		Status:      c.QueryParam("status"),
		Type:        c.QueryParam("type"),
		AuditID:     auditID,
		ProgrammeID: progID,
		Owner:       c.QueryParam("owner"),
		OverdueOnly: c.QueryParam("overdue") == "true",
	}
	if params.Status != "" && !db.AuditFindingStatuses[params.Status] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid status filter")
	}
	if params.Type != "" && !db.AuditFindingTypes[params.Type] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid type filter")
	}
	items, total, err := s.db.PaginatedAuditFindings(c.Request().Context(), orgID, params)
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

func (s *Server) handleGetAuditFinding(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid finding id")
	}
	f, err := s.db.GetAuditFinding(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "finding not found")
	}
	return c.JSON(http.StatusOK, f)
}

func (s *Server) handleAddAuditFinding(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	var req auditFindingCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.AuditID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "audit_id is required")
	}
	if req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}
	// Description is optional at create time — fill via the edit modal.
	if !db.AuditFindingTypes[req.FindingType] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid finding_type: "+req.FindingType)
	}
	// Cross-org: confirm the audit belongs to this org before insert.
	if exists, err := s.db.AuditExists(ctx, orgID, req.AuditID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	} else if !exists {
		return echo.NewHTTPError(http.StatusNotFound, "audit not found in this organization")
	}
	if req.Owner != "" {
		if err := s.validateOrgMember(c, req.Owner); err != nil {
			return err
		}
	}

	// Seed description with ## Corrective Action heading if not provided.
	desc := req.Description
	if desc == "" {
		desc = "## Corrective Action\n\n"
	}
	f := db.AuditFinding{
		AuditID:     req.AuditID,
		AuditItemID: req.AuditItemID,
		FindingType: req.FindingType,
		Title:       req.Title,
		Description: desc,
		Status:      "open",
		DueDate:     req.DueDate,
		Owner:       req.Owner,
	}
	if err := s.db.AddAuditFinding(ctx, orgID, &f); err != nil {
		return pgxHTTPError(err)
	}
	out, err := s.db.GetAuditFinding(ctx, orgID, f.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	s.logChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "audit_finding",
		EntityID:   int64(out.ID),
		Action:     "create",
		ChangedBy:  getUserEmail(c),
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  getUserEmail(c),
		Action: "audit_finding_created",
		Detail: fmt.Sprintf("Finding: %s (%s)", out.Title, out.FindingType),
	})
	return c.JSON(http.StatusCreated, out)
}

func (s *Server) handleUpdateAuditFinding(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid finding id")
	}
	ctx := c.Request().Context()

	before, err := s.db.GetAuditFinding(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "finding not found")
	}

	var req auditFindingUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Status != nil && !db.AuditFindingStatuses[*req.Status] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid status: "+*req.Status)
	}
	if req.Owner != nil && *req.Owner != "" {
		if err := s.validateOrgMember(c, *req.Owner); err != nil {
			return err
		}
	}
	actor := getUserEmail(c)

	// Apply field updates and (optionally) status update atomically per RLS.
	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, _ pgx.Tx) error {
		if req.Title != nil || req.Description != nil || req.Owner != nil || req.DueDate != nil {
			if err := s.db.UpdateAuditFindingPartial(ctx, orgID, id, req.Title, req.Description, req.Owner, req.DueDate); err != nil {
				return err
			}
		}
		if req.Status != nil {
			if err := s.db.SetAuditFindingStatus(ctx, orgID, id, *req.Status, actor); err != nil {
				return err
			}
		}
		return nil
	})
	if txErr != nil {
		return pgxHTTPError(txErr)
	}

	after, err := s.db.GetAuditFinding(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if changes := db.DiffFields("audit_finding", int64(id), actor, c.QueryParam("reason"), before.ToChangeMap(), after.ToChangeMap()); len(changes) > 0 {
		s.logChanges(ctx, orgID, changes)
	}
	if req.Status != nil && before.Status != after.Status {
		s.logAndNotify(ctx, orgID, &db.Activity{
			Actor:  actor,
			Action: "audit_finding_status_changed",
			Detail: fmt.Sprintf("Finding #%d status changed to %s", id, after.Status),
		})
	}
	return c.JSON(http.StatusOK, after)
}

func (s *Server) handleUpdateAuditFindingStatus(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid finding id")
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !db.AuditFindingStatuses[req.Status] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid status: "+req.Status)
	}
	ctx := c.Request().Context()
	before, err := s.db.GetAuditFinding(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "finding not found")
	}
	actor := getUserEmail(c)
	if err := s.db.SetAuditFindingStatus(ctx, orgID, id, req.Status, actor); err != nil {
		return pgxHTTPError(err)
	}
	after, err := s.db.GetAuditFinding(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if changes := db.DiffFields("audit_finding", int64(id), actor, "", before.ToChangeMap(), after.ToChangeMap()); len(changes) > 0 {
		s.logChanges(ctx, orgID, changes)
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "audit_finding_status_changed",
		Detail: fmt.Sprintf("Finding #%d status changed to %s", id, req.Status),
	})
	return c.JSON(http.StatusOK, map[string]string{"status": req.Status})
}

func (s *Server) handleDeleteAuditFinding(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid finding id")
	}
	ctx := c.Request().Context()
	if _, err := s.db.GetAuditFinding(ctx, orgID, id); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "finding not found")
	}
	if err := s.db.SoftDeleteAuditFinding(ctx, orgID, id); err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}
	s.logChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "audit_finding",
		EntityID:   int64(id),
		Action:     "delete",
		ChangedBy:  getUserEmail(c),
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  getUserEmail(c),
		Action: "audit_finding_deleted",
		Detail: fmt.Sprintf("Finding #%d deleted", id),
	})
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
