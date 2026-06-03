package db

import (
	"context"
	"fmt"
	"strings"
)

// Allowed enum values for corrective actions.  Mirror the schema CHECK constraints
// so the API layer can validate input via validateEnum() and return 400 instead of
// the database returning 23514.
var (
	CorrectiveActionStatuses   = []string{"todo", "assessment", "awaiting_approval", "implementation", "monitoring", "resolved"}
	CorrectiveActionSeverities = []string{"major_nc", "minor_nc", "observation", "opportunity"}
	CorrectiveActionSources    = []string{"internal_audit", "external_audit", "risk_assessment", "security_incident", "objective", "feedback", "other"}
)

// CorrectiveActionListParams specifies filtering, sorting, and pagination.
// Cross-entity links (incident, audit_finding, risk, etc.) live in entity_references —
// query the references API to find CAs linked to a given source entity.
type CorrectiveActionListParams struct {
	Page     int
	Limit    int
	Sort     string
	Search   string
	Status   string
	Severity string
	Source   string
	Assignee string
}

var correctiveActionSortable = map[string]string{
	"title":    "title",
	"severity": "CASE severity WHEN 'major_nc' THEN 1 WHEN 'minor_nc' THEN 2 WHEN 'observation' THEN 3 ELSE 4 END",
	"status":   "status",
	"due":      "due_date",
	"created":  "created_at",
	"updated":  "updated_at",
}

const correctiveActionSelectCols = `id, organization_id, identifier, title, description, source, severity, status,
	COALESCE((SELECT email FROM users WHERE id = corrective_actions.assignee_id), ''), created_by, due_date,
	COALESCE(root_cause, ''),
	COALESCE(notes, ''),
	resolved_at, COALESCE((SELECT email FROM users WHERE id = corrective_actions.resolved_by_id), ''),
	created_at, updated_at`

// CorrectiveAction represents a corrective action / nonconformity record.
type CorrectiveAction struct {
	ID             int    `json:"id"`
	OrganizationID int    `json:"organization_id"`
	Identifier     string `json:"identifier"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Source         string `json:"source"`   // internal_audit, external_audit, risk_assessment, security_incident, objective, feedback, other
	Severity       string `json:"severity"` // major_nc, minor_nc, observation, opportunity
	Status         string `json:"status"`   // todo, assessment, awaiting_approval, implementation, monitoring, resolved
	Assignee       string `json:"assignee,omitempty"`
	CreatedBy      string `json:"created_by"`
	DueDate        *Epoch `json:"due_date,omitempty"`
	RootCause      string `json:"root_cause,omitempty"`
	Notes          string `json:"notes,omitempty"`
	ResolvedAt     *Epoch `json:"resolved_at,omitempty"`
	ResolvedBy     string `json:"resolved_by,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
	UpdatedAt      Epoch  `json:"updated_at"`
}

func (d *DB) CreateCorrectiveAction(ctx context.Context, orgID int, ca *CorrectiveAction) error {
	ca.OrganizationID = orgID
	ident, err := d.NextIdentifier(ctx, orgID, "corrective_action")
	if err != nil {
		return err
	}
	ca.Identifier = ident
	return d.pool.QueryRow(ctx, `
		INSERT INTO corrective_actions (organization_id, identifier, title, description, source, severity, status,
			assignee_id, created_by, created_by_user_id, due_date, root_cause, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7,
			CASE WHEN $8 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $8) END,
			$9, (SELECT id FROM users WHERE email = $9), $10, $11, $12)
		RETURNING id, created_at, updated_at
	`, orgID, ca.Identifier, ca.Title, ca.Description, ca.Source, ca.Severity, ca.Status,
		ca.Assignee, ca.CreatedBy, ca.DueDate,
		nilIfEmpty(ca.RootCause),
		nilIfEmpty(ca.Notes),
	).Scan(&ca.ID, &ca.CreatedAt, &ca.UpdatedAt)
}

func (d *DB) GetCorrectiveAction(ctx context.Context, orgID int, id int) (*CorrectiveAction, error) {
	var ca CorrectiveAction
	err := d.pool.QueryRow(ctx, `
		SELECT `+correctiveActionSelectCols+`
		FROM corrective_actions WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID).Scan(&ca.ID, &ca.OrganizationID, &ca.Identifier, &ca.Title, &ca.Description,
		&ca.Source, &ca.Severity, &ca.Status,
		&ca.Assignee, &ca.CreatedBy, &ca.DueDate,
		&ca.RootCause,
		&ca.Notes,
		&ca.ResolvedAt, &ca.ResolvedBy,
		&ca.CreatedAt, &ca.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &ca, nil
}

// GetCorrectiveActionByIdentifier resolves a CA by its per-org identifier
// (e.g. "CA-7") — the canonical ID format used in entity_references.
func (d *DB) GetCorrectiveActionByIdentifier(ctx context.Context, orgID int, identifier string) (*CorrectiveAction, error) {
	var ca CorrectiveAction
	err := d.pool.QueryRow(ctx, `
		SELECT `+correctiveActionSelectCols+`
		FROM corrective_actions WHERE identifier = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, identifier, orgID).Scan(&ca.ID, &ca.OrganizationID, &ca.Identifier, &ca.Title, &ca.Description,
		&ca.Source, &ca.Severity, &ca.Status,
		&ca.Assignee, &ca.CreatedBy, &ca.DueDate,
		&ca.RootCause,
		&ca.Notes,
		&ca.ResolvedAt, &ca.ResolvedBy,
		&ca.CreatedAt, &ca.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &ca, nil
}

func (d *DB) ListCorrectiveActions(ctx context.Context, orgID int, status, severity, assignee string, limit int) ([]CorrectiveAction, error) {
	query := `SELECT ` + correctiveActionSelectCols + `
		FROM corrective_actions WHERE organization_id = $1 AND deleted_at IS NULL`
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
	if assignee != "" {
		n++
		query += fmt.Sprintf(` AND assignee_id = (SELECT id FROM users WHERE email = $%d)`, n)
		args = append(args, assignee)
	}
	query += ` ORDER BY CASE severity WHEN 'major_nc' THEN 1 WHEN 'minor_nc' THEN 2 WHEN 'observation' THEN 3 ELSE 4 END, created_at DESC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []CorrectiveAction
	for rows.Next() {
		var ca CorrectiveAction
		if err := rows.Scan(&ca.ID, &ca.OrganizationID, &ca.Identifier, &ca.Title, &ca.Description,
			&ca.Source, &ca.Severity, &ca.Status,
			&ca.Assignee, &ca.CreatedBy, &ca.DueDate,
			&ca.RootCause,
			&ca.Notes,
			&ca.ResolvedAt, &ca.ResolvedBy,
			&ca.CreatedAt, &ca.UpdatedAt); err != nil {
			return nil, err
		}
		actions = append(actions, ca)
	}
	return actions, nil
}

func (d *DB) UpdateCorrectiveAction(ctx context.Context, orgID int, ca *CorrectiveAction) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE corrective_actions SET title = $2, description = $3, source = $4, severity = $5,
			assignee_id = CASE WHEN $6 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $6) END, due_date = $7,
			root_cause = $8,
			notes = $9, updated_at = now()
		WHERE id = $1 AND organization_id = $10 AND deleted_at IS NULL
	`, ca.ID, ca.Title, ca.Description, ca.Source, ca.Severity,
		ca.Assignee, ca.DueDate,
		nilIfEmpty(ca.RootCause),
		nilIfEmpty(ca.Notes), orgID)
	return err
}

func (d *DB) UpdateCorrectiveActionStatus(ctx context.Context, orgID int, id int, status, actor string) error {
	if status == "resolved" {
		_, err := d.pool.Exec(ctx, `
			UPDATE corrective_actions SET status = $2, resolved_at = now(), resolved_by_id = (SELECT id FROM users WHERE email = $4), updated_at = now()
			WHERE id = $1 AND organization_id = $3 AND deleted_at IS NULL
		`, id, status, orgID, actor)
		return err
	}
	_, err := d.pool.Exec(ctx, `
		UPDATE corrective_actions SET status = $2, updated_at = now()
		WHERE id = $1 AND organization_id = $3 AND deleted_at IS NULL
	`, id, status, orgID)
	return err
}

// CountOpenCAsByIncident returns the number of corrective actions linked to an incident
// (via entity_references) that are not yet resolved.
// CountOpenCAsByIncident counts unresolved corrective actions linked to the
// incident via entity_references. incidentIdentifier is the per-org
// identifier (e.g. "INC-12") — the canonical format references are stored in.
func (d *DB) CountOpenCAsByIncident(ctx context.Context, orgID int, incidentIdentifier string) (int, error) {
	var n int
	err := d.pool.QueryRow(ctx, `
		SELECT count(DISTINCT ca.id) FROM corrective_actions ca
		JOIN entity_references r ON r.organization_id = ca.organization_id
		WHERE ca.organization_id = $1
		  AND ca.status != 'resolved'
		  AND ca.deleted_at IS NULL
		  AND (
		    (r.source_type = 'corrective_action' AND r.source_id = ca.identifier
		      AND r.target_type = 'incident' AND r.target_id = $2)
		    OR
		    (r.target_type = 'corrective_action' AND r.target_id = ca.identifier
		      AND r.source_type = 'incident' AND r.source_id = $2)
		  )
	`, orgID, incidentIdentifier).Scan(&n)
	return n, err
}

// CountOpenTasksByCA returns the number of tasks linked to a CA via task_type='ca_followup'
// and notes/description mentioning the CA identifier — best-effort heuristic since tasks
// don't have a CA foreign key.
func (d *DB) CountOpenTasksByCA(ctx context.Context, orgID int, caIdentifier string) (int, error) {
	var n int
	err := d.pool.QueryRow(ctx,
		`SELECT count(*) FROM tasks WHERE organization_id = $1
		   AND task_type = 'ca_followup'
		   AND status NOT IN ('done','cancelled')
		   AND deleted_at IS NULL
		   AND (title LIKE $2 OR COALESCE(description,'') LIKE $2)`,
		orgID, "%"+caIdentifier+"%").Scan(&n)
	return n, err
}

func (d *DB) DeleteCorrectiveAction(ctx context.Context, orgID int, id int) error {
	_, err := d.pool.Exec(ctx, `UPDATE corrective_actions SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
	return err
}

// CorrectiveActionStats are aggregate counts across the entire register.
type CorrectiveActionStats struct {
	Total            int `json:"total"`
	Todo             int `json:"todo"`
	Assessment       int `json:"assessment"`
	AwaitingApproval int `json:"awaiting_approval"`
	Implementation   int `json:"implementation"`
	Monitoring       int `json:"monitoring"`
	Resolved         int `json:"resolved"`
	MajorNC          int `json:"major_nc"`
	MinorNC          int `json:"minor_nc"`
	Observation      int `json:"observation"`
	Opportunity      int `json:"opportunity"`
}

// CorrectiveActionStats returns counts by status and severity for the org.
func (d *DB) CorrectiveActionStats(ctx context.Context, orgID int) (*CorrectiveActionStats, error) {
	var s CorrectiveActionStats
	err := d.pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE status = 'todo'),
			count(*) FILTER (WHERE status = 'assessment'),
			count(*) FILTER (WHERE status = 'awaiting_approval'),
			count(*) FILTER (WHERE status = 'implementation'),
			count(*) FILTER (WHERE status = 'monitoring'),
			count(*) FILTER (WHERE status = 'resolved'),
			count(*) FILTER (WHERE severity = 'major_nc'),
			count(*) FILTER (WHERE severity = 'minor_nc'),
			count(*) FILTER (WHERE severity = 'observation'),
			count(*) FILTER (WHERE severity = 'opportunity')
		FROM corrective_actions
		WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&s.Total, &s.Todo, &s.Assessment, &s.AwaitingApproval, &s.Implementation,
		&s.Monitoring, &s.Resolved, &s.MajorNC, &s.MinorNC, &s.Observation, &s.Opportunity)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PaginatedCorrectiveActions returns a filtered/sorted/paginated slice plus total count.
func (d *DB) PaginatedCorrectiveActions(ctx context.Context, orgID int, p CorrectiveActionListParams) ([]CorrectiveAction, int, error) {
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
	if p.Source != "" {
		where += fmt.Sprintf(` AND source = $%d`, idx)
		args = append(args, p.Source)
		idx++
	}
	if p.Assignee != "" {
		where += fmt.Sprintf(` AND assignee_id = (SELECT id FROM users WHERE email = $%d)`, idx)
		args = append(args, p.Assignee)
		idx++
	}

	var total int
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM corrective_actions`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortDir := "DESC"
	sortKey := strings.TrimPrefix(p.Sort, "-")
	if !strings.HasPrefix(p.Sort, "-") && p.Sort != "" {
		sortDir = "ASC"
	}
	sortField, ok := correctiveActionSortable[sortKey]
	if !ok {
		// default: severity asc, then created desc
		sortField = "CASE severity WHEN 'major_nc' THEN 1 WHEN 'minor_nc' THEN 2 WHEN 'observation' THEN 3 ELSE 4 END"
		sortDir = "ASC"
	}

	offset := (p.Page - 1) * p.Limit
	args = append(args, p.Limit, offset)

	query := `SELECT ` + correctiveActionSelectCols + ` FROM corrective_actions` + where +
		` ORDER BY ` + sortField + ` ` + sortDir + `, created_at DESC` +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, idx, idx+1)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var actions []CorrectiveAction
	for rows.Next() {
		var ca CorrectiveAction
		if err := rows.Scan(&ca.ID, &ca.OrganizationID, &ca.Identifier, &ca.Title, &ca.Description,
			&ca.Source, &ca.Severity, &ca.Status,
			&ca.Assignee, &ca.CreatedBy, &ca.DueDate,
			&ca.RootCause,
			&ca.Notes,
			&ca.ResolvedAt, &ca.ResolvedBy,
			&ca.CreatedAt, &ca.UpdatedAt); err != nil {
			return nil, 0, err
		}
		actions = append(actions, ca)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if actions == nil {
		actions = []CorrectiveAction{}
	}
	return actions, total, nil
}
