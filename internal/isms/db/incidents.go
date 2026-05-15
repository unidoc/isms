package db

import (
	"context"
	"fmt"
	"strings"
)

// Allowed enum values for incident fields. Mirrors schema CHECK constraints.
var (
	IncidentStatuses    = []string{"draft", "open", "investigating", "contained", "resolved", "closed"}
	IncidentSeverities  = []string{"critical", "high", "medium", "low"}
	IncidentTypes       = []string{"incident", "event", "weakness"}
	IncidentSources     = []string{"internal", "external", "internal and external"}
	GDPRRoles           = []string{"controller", "processor"}
	AuthorityNotifyVals = []string{"not_required", "pending", "notified"}
)

// IncidentListParams specifies filtering, sorting, and pagination for the incident register.
type IncidentListParams struct {
	Page         int
	Limit        int
	Sort         string
	Search       string
	Status       string
	Severity     string
	IncidentType string
	Assignee     string
}

// incidentSortable maps client-facing sort keys to actual SQL expressions.
var incidentSortable = map[string]string{
	"title":    "title",
	"severity": "CASE severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END",
	"status":   "status",
	"detected": "detected_at",
	"created":  "created_at",
	"updated":  "updated_at",
}

const incidentSelectCols = `incidents.id, incidents.organization_id, identifier, title, description, severity, incidents.status,
	affects_c, affects_i, affects_a,
	incident_type, source,
	COALESCE(notes, ''), data_breach, COALESCE(gdpr_role, ''),
	authority_notified, authority_notified_at,
	subjects_notified, subjects_notified_at,
	reporter,
	COALESCE((SELECT email FROM users WHERE id = incidents.assignee_id), ''),
	detected_at, contained_at, resolved_at, closed_at,
	COALESCE(root_cause, ''), COALESCE(lessons_learned, ''),
	incidents.created_at, incidents.updated_at`

// Incident represents a security or operational incident.
type Incident struct {
	ID                  int    `json:"id"`
	OrganizationID      int    `json:"organization_id"`
	Identifier          string `json:"identifier"`
	Title               string `json:"title"`
	Description         string `json:"description"`
	Severity            string `json:"severity"`
	Status              string `json:"status"`
	AffectsC            bool   `json:"affects_c"`
	AffectsI            bool   `json:"affects_i"`
	AffectsA            bool   `json:"affects_a"`
	IncidentType        string `json:"incident_type"`
	Source              string `json:"source"`
	Notes               string `json:"notes,omitempty"`
	DataBreach          bool   `json:"data_breach"`
	GDPRRole            string `json:"gdpr_role,omitempty"`
	AuthorityNotified   string `json:"authority_notified"`
	AuthorityNotifiedAt *Epoch `json:"authority_notified_at,omitempty"`
	SubjectsNotified    string `json:"subjects_notified"`
	SubjectsNotifiedAt  *Epoch `json:"subjects_notified_at,omitempty"`
	Reporter            string `json:"reporter"`
	Assignee            string `json:"assignee,omitempty"`
	DetectedAt          Epoch  `json:"detected_at"`
	ContainedAt         *Epoch `json:"contained_at,omitempty"`
	ResolvedAt          *Epoch `json:"resolved_at,omitempty"`
	ClosedAt            *Epoch `json:"closed_at,omitempty"`
	RootCause           string `json:"root_cause,omitempty"`
	LessonsLearned      string `json:"lessons_learned,omitempty"`
	CreatedAt           Epoch  `json:"created_at"`
	UpdatedAt           Epoch  `json:"updated_at"`
}

func (d *DB) CreateIncident(ctx context.Context, orgID int, inc *Incident) error {
	inc.OrganizationID = orgID
	ident, err := d.NextIdentifier(ctx, orgID, "incident")
	if err != nil {
		return err
	}
	inc.Identifier = ident
	return d.pool.QueryRow(ctx, `
		INSERT INTO incidents (organization_id, identifier, title, description, severity, status,
			affects_c, affects_i, affects_a,
			incident_type, source, notes, data_breach, gdpr_role,
			authority_notified, subjects_notified,
			reporter, reporter_user_id, assignee_id, detected_at,
			root_cause, lessons_learned)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16,
			$17, (SELECT id FROM users WHERE email = $17), (SELECT id FROM users WHERE email = $18), $19, $20, $21)
		RETURNING id, created_at, updated_at
	`, orgID, inc.Identifier, inc.Title, inc.Description, inc.Severity, inc.Status,
		inc.AffectsC, inc.AffectsI, inc.AffectsA,
		inc.IncidentType, inc.Source, nilIfEmpty(inc.Notes), inc.DataBreach, nilIfEmpty(inc.GDPRRole),
		inc.AuthorityNotified, inc.SubjectsNotified,
		inc.Reporter, inc.Assignee,
		inc.DetectedAt, nilIfEmpty(inc.RootCause), nilIfEmpty(inc.LessonsLearned),
	).Scan(&inc.ID, &inc.CreatedAt, &inc.UpdatedAt)
}

func (d *DB) GetIncident(ctx context.Context, orgID int, id int) (*Incident, error) {
	var inc Incident
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, identifier, title, description, severity, status,
			affects_c, affects_i, affects_a,
			incident_type, source,
			COALESCE(notes, ''), data_breach, COALESCE(gdpr_role, ''),
			authority_notified, authority_notified_at,
			subjects_notified, subjects_notified_at,
			reporter,
			COALESCE((SELECT email FROM users WHERE id = incidents.assignee_id), ''),
			detected_at, contained_at, resolved_at, closed_at,
			COALESCE(root_cause, ''), COALESCE(lessons_learned, ''),
			created_at, updated_at
		FROM incidents WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID).Scan(&inc.ID, &inc.OrganizationID, &inc.Identifier, &inc.Title, &inc.Description,
		&inc.Severity, &inc.Status,
		&inc.AffectsC, &inc.AffectsI, &inc.AffectsA,
		&inc.IncidentType, &inc.Source,
		&inc.Notes, &inc.DataBreach, &inc.GDPRRole,
		&inc.AuthorityNotified, &inc.AuthorityNotifiedAt,
		&inc.SubjectsNotified, &inc.SubjectsNotifiedAt,
		&inc.Reporter, &inc.Assignee,
		&inc.DetectedAt, &inc.ContainedAt, &inc.ResolvedAt, &inc.ClosedAt,
		&inc.RootCause, &inc.LessonsLearned,
		&inc.CreatedAt, &inc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

func (d *DB) ListIncidents(ctx context.Context, orgID int, status, severity string, limit int) ([]Incident, error) {
	query := `SELECT ` + incidentSelectCols + `
		FROM incidents WHERE organization_id = $1 AND deleted_at IS NULL`
	args := []interface{}{orgID}
	n := 1
	if status != "" {
		n++
		query += fmt.Sprintf(` AND status = $%d`, n)
		args = append(args, status)
	}
	if severity != "" {
		n++
		query += fmt.Sprintf(` AND severity = $%d`, n)
		args = append(args, severity)
	}
	query += ` ORDER BY CASE severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END, created_at DESC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	incidents := []Incident{}
	for rows.Next() {
		var inc Incident
		if err := rows.Scan(&inc.ID, &inc.OrganizationID, &inc.Identifier, &inc.Title, &inc.Description,
			&inc.Severity, &inc.Status,
			&inc.AffectsC, &inc.AffectsI, &inc.AffectsA,
			&inc.IncidentType, &inc.Source,
			&inc.Notes, &inc.DataBreach, &inc.GDPRRole,
			&inc.AuthorityNotified, &inc.AuthorityNotifiedAt,
			&inc.SubjectsNotified, &inc.SubjectsNotifiedAt,
			&inc.Reporter, &inc.Assignee,
			&inc.DetectedAt, &inc.ContainedAt, &inc.ResolvedAt, &inc.ClosedAt,
			&inc.RootCause, &inc.LessonsLearned,
			&inc.CreatedAt, &inc.UpdatedAt); err != nil {
			return nil, err
		}
		incidents = append(incidents, inc)
	}
	return incidents, nil
}

func (d *DB) UpdateIncident(ctx context.Context, orgID int, inc *Incident) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE incidents SET title = $2, description = $3, severity = $4,
			affects_c = $5, affects_i = $6, affects_a = $7,
			incident_type = $8, source = $9,
			notes = $10, data_breach = $11, gdpr_role = $12,
			authority_notified = $13, authority_notified_at = $14,
			subjects_notified = $15, subjects_notified_at = $16,
			assignee_id = (SELECT id FROM users WHERE email = $17),
			root_cause = $18, lessons_learned = $19,
			updated_at = now()
		WHERE id = $1 AND organization_id = $20 AND deleted_at IS NULL
	`, inc.ID, inc.Title, inc.Description, inc.Severity,
		inc.AffectsC, inc.AffectsI, inc.AffectsA,
		inc.IncidentType, inc.Source,
		nilIfEmpty(inc.Notes), inc.DataBreach, nilIfEmpty(inc.GDPRRole),
		inc.AuthorityNotified, inc.AuthorityNotifiedAt,
		inc.SubjectsNotified, inc.SubjectsNotifiedAt,
		nilIfEmpty(inc.Assignee),
		nilIfEmpty(inc.RootCause), nilIfEmpty(inc.LessonsLearned),
		orgID)
	return err
}

func (d *DB) DeleteIncident(ctx context.Context, orgID int, id int) error {
	_, err := d.pool.Exec(ctx, `UPDATE incidents SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
	return err
}

func (d *DB) UpdateIncidentStatus(ctx context.Context, orgID int, id int, status string) error {
	return d.UpdateIncidentStatusWithDetails(ctx, orgID, id, status, "", "")
}

// UpdateIncidentStatusWithDetails performs the status update and (optionally) sets
// root_cause / lessons_learned in a single SET clause. Combining these lets us
// avoid the 1-3 sequential UPDATE pattern that previously lived in the API tx,
// which churned through the connection pool and could partially apply on error.
//
// Empty rootCause / lessonsLearned leave the existing values unchanged.
// Returns an error if no rows were affected (incident not found / already deleted).
func (d *DB) UpdateIncidentStatusWithDetails(ctx context.Context, orgID, id int, status, rootCause, lessonsLearned string) error {
	// Lifecycle: draft → open → investigating → contained → resolved → closed.
	// Forward transitions stamp the relevant timestamp; reopens clear timestamps
	// for stages AT OR AFTER the new state so closure metadata reflects reality.
	query := `UPDATE incidents SET status = $2, updated_at = now()`
	switch status {
	case "draft", "open":
		// Reopen: clear all closure timestamps.
		query += `, contained_at = NULL, resolved_at = NULL, closed_at = NULL`
	case "investigating":
		// Reopen from contained/resolved/closed: clear those timestamps.
		query += `, contained_at = NULL, resolved_at = NULL, closed_at = NULL`
	case "contained":
		query += `, contained_at = COALESCE(contained_at, now()), resolved_at = NULL, closed_at = NULL`
	case "resolved":
		query += `, resolved_at = COALESCE(resolved_at, now()), closed_at = NULL`
	case "closed":
		query += `, closed_at = COALESCE(closed_at, now())`
	}
	// Use COALESCE on the parameters: empty string means "leave existing value alone".
	query += `, root_cause = COALESCE(NULLIF($4, ''), root_cause),
		lessons_learned = COALESCE(NULLIF($5, ''), lessons_learned)`
	query += ` WHERE id = $1 AND organization_id = $3 AND deleted_at IS NULL`
	tag, err := d.pool.Exec(ctx, query, id, status, orgID, rootCause, lessonsLearned)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("incident %d not found in org %d", id, orgID)
	}
	return nil
}

// IncidentStats are aggregate counts across the entire register, independent of pagination.
type IncidentStats struct {
	Total         int `json:"total"`
	Critical      int `json:"critical"`
	High          int `json:"high"`
	Medium        int `json:"medium"`
	Low           int `json:"low"`
	Draft         int `json:"draft"`
	Open          int `json:"open"`
	Investigating int `json:"investigating"`
	Contained     int `json:"contained"`
	Resolved      int `json:"resolved"`
	Closed        int `json:"closed"`
}

// IncidentStats returns counts by severity and status for the org.
func (d *DB) IncidentStats(ctx context.Context, orgID int) (*IncidentStats, error) {
	var s IncidentStats
	err := d.pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE severity = 'critical'),
			count(*) FILTER (WHERE severity = 'high'),
			count(*) FILTER (WHERE severity = 'medium'),
			count(*) FILTER (WHERE severity = 'low'),
			count(*) FILTER (WHERE status = 'draft'),
			count(*) FILTER (WHERE status = 'open'),
			count(*) FILTER (WHERE status = 'investigating'),
			count(*) FILTER (WHERE status = 'contained'),
			count(*) FILTER (WHERE status = 'resolved'),
			count(*) FILTER (WHERE status = 'closed')
		FROM incidents
		WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&s.Total, &s.Critical, &s.High, &s.Medium, &s.Low,
		&s.Draft, &s.Open, &s.Investigating, &s.Contained, &s.Resolved, &s.Closed)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PaginatedIncidents returns a filtered/sorted/paginated slice of incidents plus total count.
func (d *DB) PaginatedIncidents(ctx context.Context, orgID int, p IncidentListParams) ([]Incident, int, error) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Limit < 1 {
		p.Limit = 50
	}
	if p.Limit > 200 {
		p.Limit = 200
	}

	where := ` WHERE organization_id = $1 AND deleted_at IS NULL`
	args := []interface{}{orgID}
	idx := 2
	if p.Search != "" {
		where += fmt.Sprintf(` AND (title ILIKE $%d OR description ILIKE $%d)`, idx, idx)
		args = append(args, "%"+p.Search+"%")
		idx++
	}
	if p.Status != "" {
		where += fmt.Sprintf(` AND status = $%d`, idx)
		args = append(args, p.Status)
		idx++
	}
	if p.Severity != "" {
		where += fmt.Sprintf(` AND severity = $%d`, idx)
		args = append(args, p.Severity)
		idx++
	}
	if p.IncidentType != "" {
		where += fmt.Sprintf(` AND incident_type = $%d`, idx)
		args = append(args, p.IncidentType)
		idx++
	}
	if p.Assignee != "" {
		where += fmt.Sprintf(` AND assignee_id = (SELECT id FROM users WHERE email = $%d)`, idx)
		args = append(args, p.Assignee)
		idx++
	}

	var total int
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM incidents`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortDir := "DESC"
	sortKey := strings.TrimPrefix(p.Sort, "-")
	if !strings.HasPrefix(p.Sort, "-") && p.Sort != "" {
		sortDir = "ASC"
	}
	sortField, ok := incidentSortable[sortKey]
	if !ok {
		// default: severity then created desc (matches ListIncidents)
		sortField = "CASE severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END"
		sortDir = "ASC"
	}

	offset := (p.Page - 1) * p.Limit
	args = append(args, p.Limit, offset)

	query := `SELECT ` + incidentSelectCols + ` FROM incidents` + where +
		` ORDER BY ` + sortField + ` ` + sortDir + `, incidents.created_at DESC` +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, idx, idx+1)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var incidents []Incident
	for rows.Next() {
		var inc Incident
		if err := rows.Scan(&inc.ID, &inc.OrganizationID, &inc.Identifier, &inc.Title, &inc.Description,
			&inc.Severity, &inc.Status,
			&inc.AffectsC, &inc.AffectsI, &inc.AffectsA,
			&inc.IncidentType, &inc.Source,
			&inc.Notes, &inc.DataBreach, &inc.GDPRRole,
			&inc.AuthorityNotified, &inc.AuthorityNotifiedAt,
			&inc.SubjectsNotified, &inc.SubjectsNotifiedAt,
			&inc.Reporter, &inc.Assignee,
			&inc.DetectedAt, &inc.ContainedAt, &inc.ResolvedAt, &inc.ClosedAt,
			&inc.RootCause, &inc.LessonsLearned,
			&inc.CreatedAt, &inc.UpdatedAt); err != nil {
			return nil, 0, err
		}
		incidents = append(incidents, inc)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if incidents == nil {
		incidents = []Incident{}
	}
	return incidents, total, nil
}
