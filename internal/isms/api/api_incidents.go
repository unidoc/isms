package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// --- Request DTOs ---
// Update fields are *string / *bool, or Optional[T] for nullable dates. A nil
// pointer or an Optional that was not Set leaves the stored value alone; an
// empty string or a null Optional clears it.

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
	ExternalID          string           `json:"external_id"`
	References          []ReferenceInput `json:"references"`
}

// incidentUpdateRequest is the API contract for updating an incident. nil / not Set =
// leave alone; null clears authority_notified_at and subjects_notified_at.
// Status, when present, goes through enforceIncidentWriteTx, whose
// SetIncidentLifecycleTx stamps or clears closure metadata (contained_at,
// resolved_at, closed_at) on forward and reverse transitions.
type incidentUpdateRequest struct {
	Title               *string            `json:"title"`
	Description         *string            `json:"description"`
	Severity            *string            `json:"severity"`
	AffectsC            *bool              `json:"affects_c"`
	AffectsI            *bool              `json:"affects_i"`
	AffectsA            *bool              `json:"affects_a"`
	IncidentType        *string            `json:"incident_type"`
	Source              *string            `json:"source"`
	Status              *string            `json:"status"`
	Notes               *string            `json:"notes"`
	DataBreach          *bool              `json:"data_breach"`
	GDPRRole            *string            `json:"gdpr_role"`
	AuthorityNotified   *string            `json:"authority_notified"`
	AuthorityNotifiedAt Optional[db.Epoch] `json:"authority_notified_at"`
	SubjectsNotified    *string            `json:"subjects_notified"`
	SubjectsNotifiedAt  Optional[db.Epoch] `json:"subjects_notified_at"`
	Assignee            *string            `json:"assignee"`
	RootCause           *string            `json:"root_cause"`
	LessonsLearned      *string            `json:"lessons_learned"`
	ExternalID          *string            `json:"external_id"`
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
	if err := requireRole(c, "admin", "manager"); err != nil {
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
		ExternalID:          req.ExternalID,
	}

	s.applyIncidentDefaults(ctx, orgID, &inc, getUserEmail(c))
	if err := validateIncidentCreate(&inc); err != nil {
		return err
	}
	if err := s.validateOrgMember(c, inc.Assignee); err != nil {
		return err
	}

	refs, err := s.validateReferenceInputs(ctx, orgID, taskViewer(c), req.References)
	if err != nil {
		return err
	}
	if err := s.db.CreateIncident(ctx, orgID, &inc); err != nil {
		return pgxHTTPError(err)
	}

	s.createReferencesForEntity(ctx, orgID, "incident", inc.Identifier, inc.Reporter, refs)
	// Re-read so caller gets the canonical record (with assignee verified via FK).
	if out, err := s.db.GetIncident(ctx, orgID, inc.ID); err == nil {
		inc = *out
	}

	s.logChange(ctx, orgID, &db.ChangelogEntry{
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
			// No BodyKey: the body is the reporter's own description of the
			// incident, org content rather than product copy. `severity` is an
			// enum and is resolved through the shared enum keys before
			// interpolation.
			s.db.CreateNotification(ctx, orgID, &db.Notification{
				RecipientID: m.ID,
				Title:       fmt.Sprintf("New %s incident: %s", inc.Severity, inc.Title),
				TitleKey:    NotifyKeyIncidentNew,
				Body:        inc.Description,
				Params:      map[string]any{"severity": inc.Severity, "title": inc.Title},
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
	id, err := s.resolveIncidentID(c.Request().Context(), orgID, c.Param("id"))
	if errors.Is(err, errInvalidID) {
		return errInvalidEntityID("incident")
	} else if err != nil {
		return errNotFound("incident")
	}
	inc, err := s.db.GetIncident(c.Request().Context(), orgID, id)
	if err != nil {
		return errNotFound("incident")
	}
	return c.JSON(http.StatusOK, inc)
}

// prepareIncidentUpdate validates req and returns old with req merged in. Shared
// by handleUpdateIncident and the suggestion apply handler so PUT and apply
// accept and write exactly the same fields (#200).
func (s *Server) prepareIncidentUpdate(ctx context.Context, orgID int, old *db.Incident, req *incidentUpdateRequest) (db.Incident, error) {
	updated := *old
	if req.Severity != nil {
		if err := validateEnum("severity", *req.Severity, db.IncidentSeverities); err != nil {
			return db.Incident{}, err
		}
	}
	if req.Status != nil {
		if err := validateEnum("status", *req.Status, db.IncidentStatuses); err != nil {
			return db.Incident{}, err
		}
	}
	if req.IncidentType != nil {
		if err := validateEnum("incident_type", *req.IncidentType, db.IncidentTypes); err != nil {
			return db.Incident{}, err
		}
	}
	if req.Source != nil {
		if err := validateEnum("source", *req.Source, db.IncidentSources); err != nil {
			return db.Incident{}, err
		}
	}
	if req.GDPRRole != nil {
		if err := validateEnum("gdpr_role", *req.GDPRRole, db.GDPRRoles); err != nil {
			return db.Incident{}, err
		}
	}
	if req.AuthorityNotified != nil {
		if err := validateEnum("authority_notified", *req.AuthorityNotified, db.AuthorityNotifyVals); err != nil {
			return db.Incident{}, err
		}
	}
	if req.SubjectsNotified != nil {
		if err := validateEnum("subjects_notified", *req.SubjectsNotified, db.AuthorityNotifyVals); err != nil {
			return db.Incident{}, err
		}
	}
	if req.Assignee != nil && *req.Assignee != "" {
		if err := s.validateOrgMemberIn(ctx, orgID, *req.Assignee); err != nil {
			return db.Incident{}, err
		}
	}

	// Status transitions flow through the unified write path below (open-CA guard
	// + lifecycle timestamps) — the same enforced function suggestion-apply uses
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
	if req.Severity != nil {
		updated.Severity = *req.Severity
	}
	if req.AffectsC != nil {
		updated.AffectsC = *req.AffectsC
	}
	if req.AffectsI != nil {
		updated.AffectsI = *req.AffectsI
	}
	if req.AffectsA != nil {
		updated.AffectsA = *req.AffectsA
	}
	if req.IncidentType != nil {
		updated.IncidentType = *req.IncidentType
	}
	if req.Source != nil {
		updated.Source = *req.Source
	}
	if req.Notes != nil {
		updated.Notes = *req.Notes
	}
	if req.DataBreach != nil {
		updated.DataBreach = *req.DataBreach
	}
	if req.GDPRRole != nil {
		updated.GDPRRole = *req.GDPRRole
	}
	if req.AuthorityNotified != nil {
		updated.AuthorityNotified = *req.AuthorityNotified
	}
	if req.AuthorityNotifiedAt.Set {
		updated.AuthorityNotifiedAt = req.AuthorityNotifiedAt.Value
	}
	if req.SubjectsNotified != nil {
		updated.SubjectsNotified = *req.SubjectsNotified
	}
	if req.SubjectsNotifiedAt.Set {
		updated.SubjectsNotifiedAt = req.SubjectsNotifiedAt.Value
	}
	if req.Assignee != nil {
		updated.Assignee = *req.Assignee
	}
	if req.RootCause != nil {
		updated.RootCause = *req.RootCause
	}
	if req.LessonsLearned != nil {
		updated.LessonsLearned = *req.LessonsLearned
	}
	if req.ExternalID != nil {
		updated.ExternalID = *req.ExternalID
	}

	return updated, nil
}

func (s *Server) handleUpdateIncident(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := s.resolveIncidentID(c.Request().Context(), orgID, c.Param("id"))
	if errors.Is(err, errInvalidID) {
		return errInvalidEntityID("incident")
	} else if err != nil {
		return errNotFound("incident")
	}

	// Get existing incident first
	ctx := c.Request().Context()
	existing, err := s.db.GetIncident(ctx, orgID, id)
	if err != nil {
		return errNotFound("incident")
	}
	prevStatus := existing.Status
	// Snapshot BEFORE the request is applied onto existing below (#196):
	// ToChangeMap returns a fresh map, so later field assignments can't touch it.
	oldMap := existing.ToChangeMap()

	var req incidentUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	updated, err := s.prepareIncidentUpdate(ctx, orgID, existing, &req)
	if err != nil {
		return err
	}
	existing = &updated

	existing.ID = id
	// Single enforced incident write path (#26): open-CA guard on resolve/close
	// + lifecycle timestamps, shared verbatim with suggestion-apply. The
	// changelog is written in the same transaction, diffed against the row as
	// stored, so the change and its history commit or fail together (#196).
	var after *db.Incident
	if err := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := enforceIncidentWriteTx(ctx, tx, orgID, existing, prevStatus); err != nil {
			return err
		}
		var err error
		if after, err = db.GetIncidentTx(ctx, tx, orgID, id); err != nil {
			return err
		}
		changes := db.DiffFields("incident", int64(id), getUserEmail(c), c.QueryParam("reason"), oldMap, after.ToChangeMap())
		return db.LogChangesTx(ctx, tx, orgID, changes)
	}); err != nil {
		var oce openCAsLinkedError
		if errors.As(err, &oce) {
			return echo.NewHTTPError(http.StatusConflict, oce.Error())
		}
		return pgxHTTPError(err)
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
	if err := requireRole(c, "admin", "manager", "contributor"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := s.resolveIncidentID(c.Request().Context(), orgID, c.Param("id"))
	if errors.Is(err, errInvalidID) {
		return errInvalidEntityID("incident")
	} else if err != nil {
		return errNotFound("incident")
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
	actor := getUserEmail(c)

	// Same enforced write path as PUT /incidents/:id and suggestion-apply (#26):
	// transactional open-CA guard + lifecycle timestamps, with the changelog
	// written in the same transaction (#196). The row is locked and re-read
	// first, so the ownership check applies to the row being written and a
	// concurrent reassignment or edit can't slip in between (#409).
	var after *db.Incident
	if err := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.LockIncidentTx(ctx, tx, orgID, id); err != nil {
			return err
		}
		cur, err := db.GetIncidentTx(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		// The assignee may move their own incident through every status, closed
		// included (#409); the open-CA guard still applies.
		if !canActOnAssignment(c, cur.Assignee) {
			return echo.NewHTTPError(http.StatusForbidden, "contributors can only change the status of incidents assigned to them")
		}
		// A closed incident is signed off: its assignee may reopen it (#409), but
		// not rewrite its root cause or lessons learned while it stays closed.
		if !isManagerRole(c) && cur.Status == "closed" && req.Status == "closed" &&
			(req.RootCause != "" || req.LessonsLearned != "") {
			return echo.NewHTTPError(http.StatusConflict, "incident is closed")
		}
		prevStatus := cur.Status
		oldMap := cur.ToChangeMap()

		cur.Status = req.Status
		// Empty means "leave the current value alone", the contract the old
		// single-statement UPDATE had (COALESCE(NULLIF($4, ''), root_cause)).
		if req.RootCause != "" {
			cur.RootCause = req.RootCause
		}
		if req.LessonsLearned != "" {
			cur.LessonsLearned = req.LessonsLearned
		}
		cur.ID = id

		if err := enforceIncidentWriteTx(ctx, tx, orgID, cur, prevStatus); err != nil {
			return err
		}
		a, err := db.GetIncidentTx(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		after = a
		changes := db.DiffFields("incident", int64(id), actor, c.QueryParam("reason"), oldMap, after.ToChangeMap())
		return db.LogChangesTx(ctx, tx, orgID, changes)
	}); err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he
		}
		var oce openCAsLinkedError
		if errors.As(err, &oce) {
			return echo.NewHTTPError(http.StatusConflict, oce.Error())
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return errNotFound("incident")
		}
		return pgxHTTPError(err)
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "incident_status_changed",
		Detail: fmt.Sprintf("Incident #%d status changed to %s", id, req.Status),
	})

	// On resolve/close, notify relevant people
	if req.Status == "resolved" || req.Status == "closed" {
		if inc := after; inc != nil {
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
				s.db.CreateNotificationContentByEmail(ctx, orgID, r, db.NotificationContent{
					Title:    fmt.Sprintf("Incident %s: %s", req.Status, inc.Title),
					TitleKey: NotifyKeyIncidentStatus,
					Body:     fmt.Sprintf("Incident #%d has been %s", id, req.Status),
					BodyKey:  NotifyKeyIncidentStatusBody,
					Params:   map[string]any{"status": req.Status, "title": inc.Title, "id": id},
					Link:     "/incidents",
				})
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"status": req.Status})
}

// incidentProgressRequest is the narrow update the assignee may make on their
// own incident (#409): what caused it, what was learned and working notes.
// The wide PUT stays manager/admin, since it also rewrites title, severity,
// assignee and the regulatory notification fields.
type incidentProgressRequest struct {
	RootCause      *string `json:"root_cause"`
	LessonsLearned *string `json:"lessons_learned"`
	Notes          *string `json:"notes"`
}

func (s *Server) handleUpdateIncidentProgress(c echo.Context) error {
	if err := requireRole(c, "admin", "manager", "contributor"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := s.resolveIncidentID(c.Request().Context(), orgID, c.Param("id"))
	if errors.Is(err, errInvalidID) {
		return errInvalidEntityID("incident")
	} else if err != nil {
		return errNotFound("incident")
	}

	var req incidentProgressRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.RootCause == nil && req.LessonsLearned == nil && req.Notes == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "root_cause, lessons_learned or notes is required")
	}

	ctx := c.Request().Context()
	actor := getUserEmail(c)

	// Lock, re-read, check ownership and write in one transaction, so the check
	// applies to the row being written and a concurrent reassignment or edit
	// can't be overwritten (#409). Same enforced write path as the status route
	// and the wide PUT (#26), with the status unchanged, so neither the open-CA
	// guard nor the lifecycle stamps run; the changelog is written in the same
	// transaction (#196).
	var after *db.Incident
	if err := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.LockIncidentTx(ctx, tx, orgID, id); err != nil {
			return err
		}
		cur, err := db.GetIncidentTx(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		if !canActOnAssignment(c, cur.Assignee) {
			return echo.NewHTTPError(http.StatusForbidden, "contributors can only update incidents assigned to them")
		}
		// Closed is read-only to the assignee until they reopen it (#409).
		if !isManagerRole(c) && cur.Status == "closed" {
			return echo.NewHTTPError(http.StatusConflict, "incident is closed")
		}
		prevStatus := cur.Status
		oldMap := cur.ToChangeMap()
		if req.RootCause != nil {
			cur.RootCause = *req.RootCause
		}
		if req.LessonsLearned != nil {
			cur.LessonsLearned = *req.LessonsLearned
		}
		if req.Notes != nil {
			cur.Notes = *req.Notes
		}
		cur.ID = id
		if err := enforceIncidentWriteTx(ctx, tx, orgID, cur, prevStatus); err != nil {
			return err
		}
		a, err := db.GetIncidentTx(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		after = a
		changes := db.DiffFields("incident", int64(id), actor, c.QueryParam("reason"), oldMap, after.ToChangeMap())
		return db.LogChangesTx(ctx, tx, orgID, changes)
	}); err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return errNotFound("incident")
		}
		return pgxHTTPError(err)
	}

	s.searchUpsert(orgID, "incident", after.Identifier, after.Title, after.Identifier+" "+after.Title+" "+after.Description)

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "incident_updated",
		Detail: fmt.Sprintf("Incident #%d updated: %s", id, after.Title),
	})

	return c.JSON(http.StatusOK, after)
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
	id, err := s.resolveIncidentID(c.Request().Context(), orgID, c.Param("id"))
	if errors.Is(err, errInvalidID) {
		return errInvalidEntityID("incident")
	} else if err != nil {
		return errNotFound("incident")
	}

	inc, err := s.db.GetIncident(ctx, orgID, id)
	if err != nil || inc == nil {
		return errNotFound("incident")
	}

	if err := s.db.DeleteIncident(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	s.logChange(ctx, orgID, &db.ChangelogEntry{
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
