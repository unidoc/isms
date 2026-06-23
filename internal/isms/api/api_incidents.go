package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// --- Request DTOs ---
// Pointer fields use *string / *bool / **db.Epoch so an explicit empty body can clear,
// and an absent body leaves the existing value alone.

type incidentCreateRequest struct {
	Title               string           `json:"title"`
	Description         string           `json:"description"`
	Severity            string           `json:"severity"`
	Status              string           `json:"status"`
	AffectsC            bool             `json:"affects_c"`
	AffectsI            bool             `json:"affects_i"`
	AffectsA            bool             `json:"affects_a"`
	IncidentType        string           `json:"incident_type"`
	Source              string           `json:"source"`
	Notes               string           `json:"notes"`
	DataBreach          bool             `json:"data_breach"`
	GDPRRole            string           `json:"gdpr_role"`
	AuthorityNotified   string           `json:"authority_notified"`
	AuthorityNotifiedAt *db.Epoch        `json:"authority_notified_at"`
	SubjectsNotified    string           `json:"subjects_notified"`
	SubjectsNotifiedAt  *db.Epoch        `json:"subjects_notified_at"`
	Reporter            string           `json:"reporter"`
	Assignee            string           `json:"assignee"`
	DetectedAt          db.Epoch         `json:"detected_at"`
	RootCause           string           `json:"root_cause"`
	LessonsLearned      string           `json:"lessons_learned"`
	References          []ReferenceInput `json:"references"`
}

// incidentUpdateRequest is the API contract for updating an incident. nil = leave alone.
// Status, when present, is routed through UpdateIncidentStatus so closure
// metadata (contained_at, resolved_at, closed_at) is cleared correctly on
// reverse transitions — never inline-set via UpdateIncident.
type incidentUpdateRequest struct {
	Title               *string    `json:"title"`
	Description         *string    `json:"description"`
	Severity            *string    `json:"severity"`
	AffectsC            *bool      `json:"affects_c"`
	AffectsI            *bool      `json:"affects_i"`
	AffectsA            *bool      `json:"affects_a"`
	IncidentType        *string    `json:"incident_type"`
	Source              *string    `json:"source"`
	Status              *string    `json:"status"`
	Notes               *string    `json:"notes"`
	DataBreach          *bool      `json:"data_breach"`
	GDPRRole            *string    `json:"gdpr_role"`
	AuthorityNotified   *string    `json:"authority_notified"`
	AuthorityNotifiedAt **db.Epoch `json:"authority_notified_at"`
	SubjectsNotified    *string    `json:"subjects_notified"`
	SubjectsNotifiedAt  **db.Epoch `json:"subjects_notified_at"`
	Assignee            *string    `json:"assignee"`
	RootCause           *string    `json:"root_cause"`
	LessonsLearned      *string    `json:"lessons_learned"`
}

func (s *Server) handleListIncidents(c echo.Context) error {
	orgID := getOrgID(c)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	params := db.IncidentListParams{
		Page:         page,
		Limit:        limit,
		Sort:         c.QueryParam("sort"),
		Search:       c.QueryParam("q"),
		Status:       c.QueryParam("status"),
		Severity:     c.QueryParam("severity"),
		IncidentType: c.QueryParam("incident_type"),
		Assignee:     c.QueryParam("assignee"),
	}
	items, total, err := s.db.PaginatedIncidents(c.Request().Context(), orgID, params)
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

func (s *Server) handleCreateIncident(c echo.Context) error {
	if err := requireRole(c, "admin", "manager", "contributor"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	var req incidentCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	inc := db.Incident{
		Title:               req.Title,
		Description:         req.Description,
		Severity:            req.Severity,
		Status:              req.Status,
		AffectsC:            req.AffectsC,
		AffectsI:            req.AffectsI,
		AffectsA:            req.AffectsA,
		IncidentType:        req.IncidentType,
		Source:              req.Source,
		Notes:               req.Notes,
		DataBreach:          req.DataBreach,
		GDPRRole:            req.GDPRRole,
		AuthorityNotified:   req.AuthorityNotified,
		AuthorityNotifiedAt: req.AuthorityNotifiedAt,
		SubjectsNotified:    req.SubjectsNotified,
		SubjectsNotifiedAt:  req.SubjectsNotifiedAt,
		Reporter:            req.Reporter,
		Assignee:            req.Assignee,
		DetectedAt:          req.DetectedAt,
		RootCause:           req.RootCause,
		LessonsLearned:      req.LessonsLearned,
	}

	// Server-side overwrites for system-managed fields.
	if inc.Reporter == "" {
		inc.Reporter = getUserEmail(c)
	}
	if inc.Assignee == "" {
		inc.Assignee = inc.Reporter
	}
	if inc.Status == "" {
		inc.Status = "open"
	}
	if inc.Severity == "" {
		inc.Severity = "medium"
	}
	if inc.IncidentType == "" {
		inc.IncidentType = "event"
	}
	if inc.Source == "" {
		inc.Source = "internal"
	}
	if inc.AuthorityNotified == "" {
		inc.AuthorityNotified = "not_required"
	}
	if inc.SubjectsNotified == "" {
		inc.SubjectsNotified = "not_required"
	}
	if inc.DetectedAt.IsZero() {
		inc.DetectedAt = db.EpochNow()
	}

	// Seed Notes with timeline template if empty.
	if inc.Notes == "" {
		displayName := inc.Reporter
		if u, err := s.db.GetUserByEmail(ctx, inc.Reporter); err == nil && u != nil && u.Name != "" {
			displayName = u.Name
		}
		ts := inc.DetectedAt.Format("2006-01-02 15:04")
		inc.Notes = fmt.Sprintf("## Timeline\n\n- %s — Incident raised by %s\n", ts, displayName)
	}

	if err := validateEnum("status", inc.Status, db.IncidentStatuses); err != nil {
		return err
	}
	if err := validateEnum("severity", inc.Severity, db.IncidentSeverities); err != nil {
		return err
	}
	if err := validateEnum("incident_type", inc.IncidentType, db.IncidentTypes); err != nil {
		return err
	}
	if err := validateEnum("source", inc.Source, db.IncidentSources); err != nil {
		return err
	}
	if err := validateEnum("gdpr_role", inc.GDPRRole, db.GDPRRoles); err != nil {
		return err
	}
	if err := validateEnum("authority_notified", inc.AuthorityNotified, db.AuthorityNotifyVals); err != nil {
		return err
	}
	if err := validateEnum("subjects_notified", inc.SubjectsNotified, db.AuthorityNotifyVals); err != nil {
		return err
	}
	if err := s.validateOrgMember(c, inc.Assignee); err != nil {
		return err
	}

	if err := s.db.CreateIncident(ctx, orgID, &inc); err != nil {
		return pgxHTTPError(err)
	}

	s.createReferencesForEntity(ctx, orgID, "incident", inc.Identifier, inc.Reporter, req.References)
	// Re-read so caller gets the canonical record (with assignee verified via FK).
	if out, err := s.db.GetIncident(ctx, orgID, inc.ID); err == nil {
		inc = *out
	}

	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "incident",
		EntityID:   int64(inc.ID),
		Action:     "create",
		ChangedBy:  inc.Reporter,
	})

	s.searchUpsert(orgID, "incident", inc.Identifier, inc.Title, inc.Identifier+" "+inc.Title+" "+inc.Description)

	// Notify via all channels
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  inc.Reporter,
		Action: "incident_created",
		Detail: fmt.Sprintf("[%s] %s: %s", strings.ToUpper(inc.Severity), inc.Title, inc.Description),
	})

	// Create in-app notifications for all managers/admins
	members, _ := s.db.ListOrgUsers(ctx, orgID)
	for _, m := range members {
		if m.Role == "admin" || m.Role == "manager" {
			s.db.CreateNotification(ctx, orgID, &db.Notification{
				RecipientID: m.ID,
				Title:       fmt.Sprintf("New %s incident: %s", inc.Severity, inc.Title),
				Body:        inc.Description,
				Link:        "/incidents",
			})
		}
	}

	// Email assignee if set
	if inc.Assignee != "" && s.mailer.Enabled() {
		s.mailer.SendBranded(inc.Assignee,
			fmt.Sprintf("Incident [%s]: %s", strings.ToUpper(inc.Severity), inc.Title),
			inc.Description, s.orgMail(ctx, orgID).Branding)
	}

	return c.JSON(http.StatusCreated, inc)
}

func (s *Server) handleGetIncident(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid incident id")
	}
	inc, err := s.db.GetIncident(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "incident not found")
	}
	return c.JSON(http.StatusOK, inc)
}

func (s *Server) handleUpdateIncident(c echo.Context) error {
	if err := requireRole(c, "admin", "manager", "contributor"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid incident id")
	}

	// Get existing incident first
	ctx := c.Request().Context()
	existing, err := s.db.GetIncident(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "incident not found")
	}

	var req incidentUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Severity != nil {
		if err := validateEnum("severity", *req.Severity, db.IncidentSeverities); err != nil {
			return err
		}
	}
	if req.Status != nil {
		if err := validateEnum("status", *req.Status, db.IncidentStatuses); err != nil {
			return err
		}
	}
	if req.IncidentType != nil {
		if err := validateEnum("incident_type", *req.IncidentType, db.IncidentTypes); err != nil {
			return err
		}
	}
	if req.Source != nil {
		if err := validateEnum("source", *req.Source, db.IncidentSources); err != nil {
			return err
		}
	}
	if req.GDPRRole != nil {
		if err := validateEnum("gdpr_role", *req.GDPRRole, db.GDPRRoles); err != nil {
			return err
		}
	}
	if req.AuthorityNotified != nil {
		if err := validateEnum("authority_notified", *req.AuthorityNotified, db.AuthorityNotifyVals); err != nil {
			return err
		}
	}
	if req.SubjectsNotified != nil {
		if err := validateEnum("subjects_notified", *req.SubjectsNotified, db.AuthorityNotifyVals); err != nil {
			return err
		}
	}
	if req.Assignee != nil && *req.Assignee != "" {
		if err := s.validateOrgMember(c, *req.Assignee); err != nil {
			return err
		}
	}

	// Route status changes through the dedicated transition function so that
	// contained_at / resolved_at / closed_at are cleared correctly on reverse transitions.
	if req.Status != nil && *req.Status != existing.Status {
		// Changing status is a manager/admin action. The dedicated status
		// endpoint already enforces this; the general edit endpoint must apply
		// the same rule so it can't be used as an RBAC bypass (#24).
		if err := requireRole(c, "admin", "manager"); err != nil {
			return err
		}
		// Same rule as the dedicated status endpoint: cannot resolve/close an
		// incident with open corrective actions still linked.
		if *req.Status == "closed" || *req.Status == "resolved" {
			if n, err := s.db.CountOpenCAsByIncident(ctx, orgID, existing.Identifier); err == nil && n > 0 {
				return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("cannot %s incident: %d open corrective action(s) still linked", statusVerb(*req.Status), n))
			}
		}
		if err := s.db.UpdateIncidentStatus(ctx, orgID, id, *req.Status); err != nil {
			return pgxHTTPError(err)
		}
		// Re-load so subsequent UpdateIncident writes against the new state.
		existing, err = s.db.GetIncident(ctx, orgID, id)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "incident not found")
		}
	}

	// Apply pointer-based partial update onto existing record.
	if req.Title != nil {
		existing.Title = *req.Title
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Severity != nil {
		existing.Severity = *req.Severity
	}
	if req.AffectsC != nil {
		existing.AffectsC = *req.AffectsC
	}
	if req.AffectsI != nil {
		existing.AffectsI = *req.AffectsI
	}
	if req.AffectsA != nil {
		existing.AffectsA = *req.AffectsA
	}
	if req.IncidentType != nil {
		existing.IncidentType = *req.IncidentType
	}
	if req.Source != nil {
		existing.Source = *req.Source
	}
	if req.Notes != nil {
		existing.Notes = *req.Notes
	}
	if req.DataBreach != nil {
		existing.DataBreach = *req.DataBreach
	}
	if req.GDPRRole != nil {
		existing.GDPRRole = *req.GDPRRole
	}
	if req.AuthorityNotified != nil {
		existing.AuthorityNotified = *req.AuthorityNotified
	}
	if req.AuthorityNotifiedAt != nil {
		existing.AuthorityNotifiedAt = *req.AuthorityNotifiedAt
	}
	if req.SubjectsNotified != nil {
		existing.SubjectsNotified = *req.SubjectsNotified
	}
	if req.SubjectsNotifiedAt != nil {
		existing.SubjectsNotifiedAt = *req.SubjectsNotifiedAt
	}
	if req.Assignee != nil {
		existing.Assignee = *req.Assignee
	}
	if req.RootCause != nil {
		existing.RootCause = *req.RootCause
	}
	if req.LessonsLearned != nil {
		existing.LessonsLearned = *req.LessonsLearned
	}

	oldMap := existing.ToChangeMap()
	existing.ID = id
	if err := s.db.UpdateIncident(ctx, orgID, existing); err != nil {
		return pgxHTTPError(err)
	}

	after, _ := s.db.GetIncident(ctx, orgID, id)
	if after != nil {
		actor := getUserEmail(c)
		reason := c.QueryParam("reason")
		changes := db.DiffFields("incident", int64(id), actor, reason, oldMap, after.ToChangeMap())
		if len(changes) > 0 {
			_ = s.db.LogChanges(ctx, orgID, changes)
		}
	}

	s.searchUpsert(orgID, "incident", existing.Identifier, existing.Title, existing.Identifier+" "+existing.Title+" "+existing.Description)

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  getUserEmail(c),
		Action: "incident_updated",
		Detail: fmt.Sprintf("Incident #%d updated: %s", id, existing.Title),
	})

	if after != nil {
		return c.JSON(http.StatusOK, after)
	}
	return c.JSON(http.StatusOK, existing)
}

// statusVerb maps a target status to the verb for "cannot <verb> …" error
// messages. Intentionally scoped to the terminal statuses guarded by the
// open-CA rule ("resolved", "closed") — extend the switch before adding
// callers with other statuses.
func statusVerb(status string) string {
	switch status {
	case "resolved":
		return "resolve"
	case "closed":
		return "close"
	default:
		return status
	}
}

func (s *Server) handleUpdateIncidentStatus(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid incident id")
	}

	var req struct {
		Status         string `json:"status"`
		RootCause      string `json:"root_cause,omitempty"`
		LessonsLearned string `json:"lessons_learned,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := validateEnum("status", req.Status, db.IncidentStatuses); err != nil {
		return err
	}

	ctx := c.Request().Context()

	// Block closing/resolving if there are still-open corrective actions linked to this incident.
	if req.Status == "closed" || req.Status == "resolved" {
		existing, err := s.db.GetIncident(ctx, orgID, id)
		if err != nil || existing == nil {
			return echo.NewHTTPError(http.StatusNotFound, "incident not found")
		}
		if n, err := s.db.CountOpenCAsByIncident(ctx, orgID, existing.Identifier); err == nil && n > 0 {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("cannot %s incident: %d open corrective action(s) still linked", statusVerb(req.Status), n))
		}
	}

	// Status update + optional field updates in a single SQL statement.
	if err := s.db.UpdateIncidentStatusWithDetails(ctx, orgID, id, req.Status, req.RootCause, req.LessonsLearned); err != nil {
		return pgxHTTPError(err)
	}

	actor := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "incident_status_changed",
		Detail: fmt.Sprintf("Incident #%d status changed to %s", id, req.Status),
	})

	// On resolve/close, notify relevant people
	if req.Status == "resolved" || req.Status == "closed" {
		inc, err := s.db.GetIncident(ctx, orgID, id)
		if err == nil {
			// Log root cause / lessons learned in activity if set
			if inc.RootCause != "" && req.Status == "resolved" {
				s.logAndNotify(ctx, orgID, &db.Activity{
					Actor:  actor,
					Action: "incident_root_cause",
					Detail: fmt.Sprintf("Incident #%d root cause: %s", id, inc.RootCause),
				})
			}
			if inc.LessonsLearned != "" && req.Status == "closed" {
				s.logAndNotify(ctx, orgID, &db.Activity{
					Actor:  actor,
					Action: "incident_lessons_learned",
					Detail: fmt.Sprintf("Incident #%d lessons learned: %s", id, inc.LessonsLearned),
				})
			}

			// Notify reporter and assignee
			recipients := []string{inc.Reporter}
			if inc.Assignee != "" && inc.Assignee != inc.Reporter {
				recipients = append(recipients, inc.Assignee)
			}
			for _, r := range recipients {
				s.db.CreateNotificationByEmail(ctx, orgID, r,
					fmt.Sprintf("Incident %s: %s", req.Status, inc.Title),
					fmt.Sprintf("Incident #%d has been %s", id, req.Status),
					"/incidents")
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"status": req.Status})
}

func (s *Server) handleIncidentStats(c echo.Context) error {
	orgID := getOrgID(c)
	stats, err := s.db.IncidentStats(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) handleDeleteIncident(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid incident id")
	}

	inc, err := s.db.GetIncident(ctx, orgID, id)
	if err != nil || inc == nil {
		return echo.NewHTTPError(http.StatusNotFound, "incident not found")
	}

	if err := s.db.DeleteIncident(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "incident",
		EntityID:   int64(id),
		Action:     "delete",
		ChangedBy:  user,
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "incident_deleted",
		Detail: fmt.Sprintf("%s: %s", inc.Identifier, inc.Title),
	})

	s.searchRemove(orgID, "incident", inc.Identifier)

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
