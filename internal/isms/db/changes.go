package db

import (
	"context"
	"fmt"
	"strings"
)

// Allowed enum values for change request fields. Mirrors schema CHECK constraints.
var (
	ChangeStatuses   = []string{"proposed", "approved", "rejected", "in_progress", "implemented", "closed"}
	ChangePriorities = []string{"low", "medium", "high", "critical"}
	ChangeRiskLevels = []string{"low", "medium", "high", "critical"}
	ChangeCategories = []string{"process", "technology", "people", "documentation", "infrastructure", "other"}
	// A change request is a normal change or an access request — same approval
	// flow; access-request specifics live in notes, no extra structured fields.
	ChangeTypes = []string{"change", "access_request"}
)

// ChangeRequestListParams specifies filtering, sorting, and pagination for the change register.
type ChangeRequestListParams struct {
	Page     int
	Limit    int
	Sort     string
	Search   string
	Status   string
	Priority string
	Category string
	Assignee string
}

var changeRequestSortable = map[string]string{
	"title":    "title",
	"priority": "CASE priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END",
	"status":   "status",
	"created":  "created_at",
	"updated":  "updated_at",
}

const changeRequestSelectCols = `change_requests.id, change_requests.organization_id, identifier, title, description, COALESCE(justification, ''),
	priority, category, risk_level, COALESCE(rollback_plan, ''), COALESCE(notes, ''),
	(SELECT email FROM users WHERE id = change_requests.requested_by_id),
	COALESCE((SELECT email FROM users WHERE id = change_requests.assigned_to_id), ''),
	status, COALESCE(approved_by, ''),
	approved_at, planned_at, implemented_at, change_requests.created_at, change_requests.updated_at,
	change_requests.type`

// ChangeRequest represents a change management record.
type ChangeRequest struct {
	ID             int    `json:"id"`
	OrganizationID int    `json:"organization_id"`
	Identifier     string `json:"identifier"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Justification  string `json:"justification,omitempty"`
	Priority       string `json:"priority"`
	Category       string `json:"category"`
	RiskLevel      string `json:"risk_level"`
	RollbackPlan   string `json:"rollback_plan,omitempty"`
	Notes          string `json:"notes,omitempty"`
	RequestedBy    string `json:"requested_by"`
	AssignedTo     string `json:"assigned_to,omitempty"`
	Status         string `json:"status"`
	ApprovedBy     string `json:"approved_by,omitempty"`
	ApprovedAt     *Epoch `json:"approved_at,omitempty"`
	PlannedAt      *Epoch `json:"planned_at,omitempty"`
	ImplementedAt  *Epoch `json:"implemented_at,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
	UpdatedAt      Epoch  `json:"updated_at"`
}

func (d *DB) CreateChangeRequest(ctx context.Context, orgID int, cr *ChangeRequest) error {
	cr.OrganizationID = orgID
	if cr.Type == "" {
		cr.Type = "change"
	}
	ident, err := d.NextIdentifier(ctx, orgID, "change_request")
	if err != nil {
		return err
	}
	cr.Identifier = ident
	return d.pool.QueryRow(ctx, `
		INSERT INTO change_requests (organization_id, identifier, title, description, justification, priority, category, risk_level, rollback_plan, notes, requested_by_id, assigned_to_id, status, planned_at, type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, (SELECT id FROM users WHERE email = $11),
			(SELECT id FROM users WHERE email = $12), $13, $14, $15)
		RETURNING id, created_at, updated_at
	`, orgID, cr.Identifier, cr.Title, cr.Description, nilIfEmpty(cr.Justification),
		cr.Priority, cr.Category, cr.RiskLevel, nilIfEmpty(cr.RollbackPlan), nilIfEmpty(cr.Notes),
		cr.RequestedBy, nilIfEmpty(cr.AssignedTo), cr.Status, cr.PlannedAt, cr.Type,
	).Scan(&cr.ID, &cr.CreatedAt, &cr.UpdatedAt)
}

func (d *DB) GetChangeRequest(ctx context.Context, orgID int, id int) (*ChangeRequest, error) {
	var cr ChangeRequest
	err := d.pool.QueryRow(ctx, `
		SELECT `+changeRequestSelectCols+`
		FROM change_requests WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID).Scan(&cr.ID, &cr.OrganizationID, &cr.Identifier, &cr.Title, &cr.Description, &cr.Justification,
		&cr.Priority, &cr.Category, &cr.RiskLevel, &cr.RollbackPlan, &cr.Notes,
		&cr.RequestedBy, &cr.AssignedTo, &cr.Status, &cr.ApprovedBy,
		&cr.ApprovedAt, &cr.PlannedAt, &cr.ImplementedAt, &cr.CreatedAt, &cr.UpdatedAt, &cr.Type)
	if err != nil {
		return nil, err
	}
	return &cr, nil
}

func (d *DB) ListChangeRequests(ctx context.Context, orgID int, status string, limit int) ([]ChangeRequest, error) {
	query := `SELECT ` + changeRequestSelectCols + `
		FROM change_requests WHERE organization_id = $1 AND deleted_at IS NULL`
	args := []interface{}{orgID}
	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	crs := []ChangeRequest{}
	for rows.Next() {
		var cr ChangeRequest
		if err := rows.Scan(&cr.ID, &cr.OrganizationID, &cr.Identifier, &cr.Title, &cr.Description, &cr.Justification,
			&cr.Priority, &cr.Category, &cr.RiskLevel, &cr.RollbackPlan, &cr.Notes,
			&cr.RequestedBy, &cr.AssignedTo, &cr.Status, &cr.ApprovedBy,
			&cr.ApprovedAt, &cr.PlannedAt, &cr.ImplementedAt, &cr.CreatedAt, &cr.UpdatedAt, &cr.Type); err != nil {
			return nil, err
		}
		crs = append(crs, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return crs, nil
}

func (d *DB) UpdateChangeRequest(ctx context.Context, orgID int, id int, cr *ChangeRequest) error {
	if cr.Type == "" {
		cr.Type = "change"
	}
	_, err := d.pool.Exec(ctx, `
		UPDATE change_requests SET title = $2, description = $3, justification = $4,
			priority = $5, category = $6, risk_level = $7, rollback_plan = $8, notes = $9,
			assigned_to_id = (SELECT id FROM users WHERE email = $10),
			planned_at = $11, type = $13,
			updated_at = now()
		WHERE id = $1 AND organization_id = $12 AND deleted_at IS NULL
	`, id, cr.Title, cr.Description, nilIfEmpty(cr.Justification),
		cr.Priority, cr.Category, cr.RiskLevel, nilIfEmpty(cr.RollbackPlan), nilIfEmpty(cr.Notes),
		nilIfEmpty(cr.AssignedTo), cr.PlannedAt, orgID, cr.Type)
	return err
}

func (d *DB) DeleteChangeRequest(ctx context.Context, orgID int, id int) error {
	_, err := d.pool.Exec(ctx, `UPDATE change_requests SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
	return err
}

func (d *DB) UpdateChangeRequestStatus(ctx context.Context, orgID int, id int, status, approvedBy string) error {
	// Clear closure metadata when transitioning to states that no longer warrant it.
	// approved_at / approved_by[_user_id] survive on approved → implemented → closed,
	// but are cleared when going back to proposed / in_progress / rejected.
	// implemented_at survives only on implemented → closed; cleared otherwise.
	clearApproved := status == "proposed" || status == "in_progress" || status == "rejected"
	clearImplemented := status != "implemented" && status != "closed"

	query := `UPDATE change_requests SET status = $2, updated_at = now()`
	args := []interface{}{id, status}
	if status == "approved" && approvedBy != "" {
		query += `, approved_by = $3, approved_by_user_id = (SELECT id FROM users WHERE email = $3), approved_at = now()`
		args = append(args, approvedBy)
		if clearImplemented {
			query += `, implemented_at = NULL`
		}
		query += ` WHERE id = $1 AND organization_id = $4 AND deleted_at IS NULL`
		args = append(args, orgID)
	} else if status == "implemented" {
		query += `, implemented_at = COALESCE(implemented_at, now())`
		query += ` WHERE id = $1 AND organization_id = $3 AND deleted_at IS NULL`
		args = append(args, orgID)
	} else {
		// in_progress, rejected, closed, proposed
		if clearApproved {
			query += `, approved_at = NULL, approved_by = NULL, approved_by_user_id = NULL`
		}
		if clearImplemented {
			query += `, implemented_at = NULL`
		}
		query += ` WHERE id = $1 AND organization_id = $3 AND deleted_at IS NULL`
		args = append(args, orgID)
	}
	_, err := d.pool.Exec(ctx, query, args...)
	return err
}

func (cr *ChangeRequest) ToChangeMap() map[string]string {
	return map[string]string{
		"type":          cr.Type,
		"title":         cr.Title,
		"description":   cr.Description,
		"justification": cr.Justification,
		"priority":      cr.Priority,
		"category":      cr.Category,
		"risk_level":    cr.RiskLevel,
		"rollback_plan": cr.RollbackPlan,
		"notes":         cr.Notes,
		"assigned_to":   cr.AssignedTo,
		"status":        cr.Status,
		"planned_at":    epochToString(cr.PlannedAt),
	}
}

// ChangeRequestStats are aggregate counts across the entire register.
type ChangeRequestStats struct {
	Total       int `json:"total"`
	Proposed    int `json:"proposed"`
	Approved    int `json:"approved"`
	Rejected    int `json:"rejected"`
	InProgress  int `json:"in_progress"`
	Implemented int `json:"implemented"`
	Closed      int `json:"closed"`
}

// ChangeRequestStats returns counts by status for the org.
func (d *DB) ChangeRequestStats(ctx context.Context, orgID int) (*ChangeRequestStats, error) {
	var s ChangeRequestStats
	err := d.pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE status = 'proposed'),
			count(*) FILTER (WHERE status = 'approved'),
			count(*) FILTER (WHERE status = 'rejected'),
			count(*) FILTER (WHERE status = 'in_progress'),
			count(*) FILTER (WHERE status = 'implemented'),
			count(*) FILTER (WHERE status = 'closed')
		FROM change_requests
		WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&s.Total, &s.Proposed, &s.Approved, &s.Rejected, &s.InProgress, &s.Implemented, &s.Closed)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PaginatedChangeRequests returns a filtered/sorted/paginated slice of change requests plus total count.
func (d *DB) PaginatedChangeRequests(ctx context.Context, orgID int, p ChangeRequestListParams) ([]ChangeRequest, int, error) {
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
	if p.Priority != "" {
		where += fmt.Sprintf(` AND priority = $%d`, idx)
		args = append(args, p.Priority)
		idx++
	}
	if p.Category != "" {
		where += fmt.Sprintf(` AND category = $%d`, idx)
		args = append(args, p.Category)
		idx++
	}
	if p.Assignee != "" {
		where += fmt.Sprintf(` AND assigned_to_id = (SELECT id FROM users WHERE email = $%d)`, idx)
		args = append(args, p.Assignee)
		idx++
	}

	var total int
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM change_requests`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortDir := "DESC"
	sortKey := strings.TrimPrefix(p.Sort, "-")
	if !strings.HasPrefix(p.Sort, "-") && p.Sort != "" {
		sortDir = "ASC"
	}
	sortField, ok := changeRequestSortable[sortKey]
	if !ok {
		sortField = "updated_at"
		sortDir = "DESC"
	}

	offset := (p.Page - 1) * p.Limit
	args = append(args, p.Limit, offset)

	query := `SELECT ` + changeRequestSelectCols + ` FROM change_requests` + where +
		` ORDER BY ` + sortField + ` ` + sortDir + `, change_requests.id DESC` +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, idx, idx+1)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var crs []ChangeRequest
	for rows.Next() {
		var cr ChangeRequest
		if err := rows.Scan(&cr.ID, &cr.OrganizationID, &cr.Identifier, &cr.Title, &cr.Description, &cr.Justification,
			&cr.Priority, &cr.Category, &cr.RiskLevel, &cr.RollbackPlan, &cr.Notes,
			&cr.RequestedBy, &cr.AssignedTo, &cr.Status, &cr.ApprovedBy,
			&cr.ApprovedAt, &cr.PlannedAt, &cr.ImplementedAt, &cr.CreatedAt, &cr.UpdatedAt, &cr.Type); err != nil {
			return nil, 0, err
		}
		crs = append(crs, cr)
	}
	if crs == nil {
		crs = []ChangeRequest{}
	}
	return crs, total, nil
}
