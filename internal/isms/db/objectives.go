package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Allowed enum values for objective fields. Mirrors schema CHECK constraints.
var (
	ObjectiveStatuses        = []string{"draft", "active", "at_risk", "paused", "complete"}
	ObjectiveTargetOperators = []string{"gte", "lte", "eq", "gt", "lt"}
)

// ObjectiveListParams specifies filtering, sorting, and pagination.
type ObjectiveListParams struct {
	Page      int
	Limit     int
	Sort      string
	Search    string
	Status    string
	ProgramID int64
	Owner     string
}

var objectiveSortable = map[string]string{
	"title":     "title",
	"display":   "display_id",
	"status":    "status",
	"created":   "created_at",
	"updated":   "updated_at",
}

const objectiveSelectCols = `id, organization_id, program_id, display_id, seq_number,
	title, COALESCE(description, ''), COALESCE((SELECT email FROM users WHERE id = objectives.owner_id), ''),
	COALESCE(source, ''), COALESCE(measurement_method, ''),
	target_value, target_operator, COALESCE(unit, ''),
	window_seconds, grace_seconds, checkin_cycle, status,
	started_at, archived_at, COALESCE(notes, ''), created_at, updated_at`

// Objective is a measurable ISMS objective within a program.
type Objective struct {
	ID                int64      `json:"id"`
	OrganizationID    int        `json:"organization_id"`
	ProgramID         int64      `json:"program_id"`
	DisplayID         string     `json:"display_id"`
	SeqNumber         int        `json:"seq_number"`
	Title             string     `json:"title"`
	Description       string     `json:"description,omitempty"`
	Owner             string     `json:"owner,omitempty"`
	Source            string     `json:"source,omitempty"`
	MeasurementMethod string     `json:"measurement_method,omitempty"`
	TargetValue       *float64   `json:"target_value,omitempty"`
	TargetOperator    string     `json:"target_operator"`
	Unit              string     `json:"unit,omitempty"`
	WindowSeconds     *int       `json:"window_seconds,omitempty"`
	GraceSeconds      int        `json:"grace_seconds"`
	CheckinCycle      int        `json:"checkin_cycle"`
	Status            string     `json:"status"`
	StartedAt         *Epoch `json:"started_at,omitempty"`
	ArchivedAt        *Epoch `json:"archived_at,omitempty"`
	Notes             string `json:"notes,omitempty"`
	CreatedAt         Epoch  `json:"created_at"`
	UpdatedAt         Epoch  `json:"updated_at"`
}

func (d *DB) CreateObjective(ctx context.Context, orgID int, o *Objective) error {
	o.OrganizationID = orgID

	// Get program key for display_id generation
	prog, err := d.GetProgram(ctx, orgID, o.ProgramID)
	if err != nil {
		return fmt.Errorf("program not found: %w", err)
	}

	// Get next seq number for this program
	var maxSeq int
	_ = d.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(seq_number), 0) FROM objectives WHERE program_id = $1 AND deleted_at IS NULL`,
		o.ProgramID).Scan(&maxSeq)
	o.SeqNumber = maxSeq + 1
	o.DisplayID = fmt.Sprintf("%s-%d", prog.Key, o.SeqNumber)

	if o.TargetOperator == "" {
		o.TargetOperator = "gte"
	}
	if o.Status == "" {
		o.Status = "draft"
	}

	if o.CheckinCycle <= 0 {
		o.CheckinCycle = 12
	}

	return d.pool.QueryRow(ctx, `
		INSERT INTO objectives (organization_id, program_id, display_id, seq_number,
			title, description, owner_id, source, measurement_method,
			target_value, target_operator, unit, window_seconds, grace_seconds,
			checkin_cycle, status, started_at, notes)
		VALUES ($1, $2, $3, $4, $5, $6, (SELECT id FROM users WHERE email = $7), $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING id, created_at, updated_at
	`, orgID, o.ProgramID, o.DisplayID, o.SeqNumber,
		o.Title, nilIfEmpty(o.Description), nilIfEmpty(o.Owner),
		nilIfEmpty(o.Source), nilIfEmpty(o.MeasurementMethod),
		o.TargetValue, o.TargetOperator, nilIfEmpty(o.Unit),
		o.WindowSeconds, o.GraceSeconds, o.CheckinCycle, o.Status, o.StartedAt, nilIfEmpty(o.Notes),
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
}

func (d *DB) GetObjective(ctx context.Context, orgID int, id int64) (*Objective, error) {
	var o Objective
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, program_id, display_id, seq_number,
			title, COALESCE(description, ''), COALESCE((SELECT email FROM users WHERE id = objectives.owner_id), ''),
			COALESCE(source, ''), COALESCE(measurement_method, ''),
			target_value, target_operator, COALESCE(unit, ''),
			window_seconds, grace_seconds, checkin_cycle, status,
			started_at, archived_at, COALESCE(notes, ''), created_at, updated_at
		FROM objectives WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID).Scan(&o.ID, &o.OrganizationID, &o.ProgramID, &o.DisplayID, &o.SeqNumber,
		&o.Title, &o.Description, &o.Owner,
		&o.Source, &o.MeasurementMethod,
		&o.TargetValue, &o.TargetOperator, &o.Unit,
		&o.WindowSeconds, &o.GraceSeconds, &o.CheckinCycle, &o.Status,
		&o.StartedAt, &o.ArchivedAt, &o.Notes, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (d *DB) GetObjectiveByDisplayID(ctx context.Context, orgID int, displayID string) (*Objective, error) {
	var o Objective
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, program_id, display_id, seq_number,
			title, COALESCE(description, ''), COALESCE((SELECT email FROM users WHERE id = objectives.owner_id), ''),
			COALESCE(source, ''), COALESCE(measurement_method, ''),
			target_value, target_operator, COALESCE(unit, ''),
			window_seconds, grace_seconds, checkin_cycle, status,
			started_at, archived_at, COALESCE(notes, ''), created_at, updated_at
		FROM objectives WHERE display_id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, displayID, orgID).Scan(&o.ID, &o.OrganizationID, &o.ProgramID, &o.DisplayID, &o.SeqNumber,
		&o.Title, &o.Description, &o.Owner,
		&o.Source, &o.MeasurementMethod,
		&o.TargetValue, &o.TargetOperator, &o.Unit,
		&o.WindowSeconds, &o.GraceSeconds, &o.CheckinCycle, &o.Status,
		&o.StartedAt, &o.ArchivedAt, &o.Notes, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (d *DB) ListObjectives(ctx context.Context, orgID int, programID int64, status string) ([]Objective, error) {
	query := `
		SELECT id, organization_id, program_id, display_id, seq_number,
			title, COALESCE(description, ''), COALESCE((SELECT email FROM users WHERE id = objectives.owner_id), ''),
			COALESCE(source, ''), COALESCE(measurement_method, ''),
			target_value, target_operator, COALESCE(unit, ''),
			window_seconds, grace_seconds, checkin_cycle, status,
			started_at, archived_at, COALESCE(notes, ''), created_at, updated_at
		FROM objectives WHERE organization_id = $1 AND deleted_at IS NULL`
	args := []interface{}{orgID}
	argN := 2

	if programID > 0 {
		query += fmt.Sprintf(" AND program_id = $%d", argN)
		args = append(args, programID)
		argN++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, status)
		argN++
	}
	query += " ORDER BY display_id"

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	objectives := []Objective{}
	for rows.Next() {
		var o Objective
		if err := rows.Scan(&o.ID, &o.OrganizationID, &o.ProgramID, &o.DisplayID, &o.SeqNumber,
			&o.Title, &o.Description, &o.Owner,
			&o.Source, &o.MeasurementMethod,
			&o.TargetValue, &o.TargetOperator, &o.Unit,
			&o.WindowSeconds, &o.GraceSeconds, &o.CheckinCycle, &o.Status,
			&o.StartedAt, &o.ArchivedAt, &o.Notes, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		objectives = append(objectives, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return objectives, nil
}

func (d *DB) UpdateObjective(ctx context.Context, orgID int, o *Objective) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE objectives SET
			title = $2, description = $3, owner_id = CASE WHEN $4 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $4) END,
			source = $5, measurement_method = $6,
			target_value = $7, target_operator = $8, unit = $9,
			window_seconds = $10, grace_seconds = $11, checkin_cycle = $12, status = $13,
			started_at = $14, notes = $15, updated_at = now()
		WHERE id = $1 AND organization_id = $16 AND deleted_at IS NULL
	`, o.ID, o.Title, nilIfEmpty(o.Description), o.Owner,
		nilIfEmpty(o.Source), nilIfEmpty(o.MeasurementMethod),
		o.TargetValue, o.TargetOperator, nilIfEmpty(o.Unit),
		o.WindowSeconds, o.GraceSeconds, o.CheckinCycle, o.Status,
		o.StartedAt, nilIfEmpty(o.Notes), orgID)
	return err
}

func (d *DB) DeleteObjective(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `UPDATE objectives SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
	return err
}

func (d *DB) ArchiveObjective(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE objectives SET archived_at = now(), updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID)
	return err
}

func (d *DB) UnarchiveObjective(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE objectives SET archived_at = NULL, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID)
	return err
}

// ObjectiveStats are aggregate counts across the entire register.
type ObjectiveStats struct {
	Total    int `json:"total"`
	Draft    int `json:"draft"`
	Active   int `json:"active"`
	AtRisk   int `json:"at_risk"`
	Paused   int `json:"paused"`
	Complete int `json:"complete"`
	Archived int `json:"archived"`
}

// ObjectiveStats returns counts by status for the org.
func (d *DB) ObjectiveStats(ctx context.Context, orgID int) (*ObjectiveStats, error) {
	var s ObjectiveStats
	err := d.pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE status = 'draft' AND archived_at IS NULL),
			count(*) FILTER (WHERE status = 'active' AND archived_at IS NULL),
			count(*) FILTER (WHERE status = 'at_risk' AND archived_at IS NULL),
			count(*) FILTER (WHERE status = 'paused' AND archived_at IS NULL),
			count(*) FILTER (WHERE status = 'complete' AND archived_at IS NULL),
			count(*) FILTER (WHERE archived_at IS NOT NULL)
		FROM objectives
		WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&s.Total, &s.Draft, &s.Active, &s.AtRisk, &s.Paused, &s.Complete, &s.Archived)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PaginatedObjectives returns a filtered/sorted/paginated slice plus total count.
func (d *DB) PaginatedObjectives(ctx context.Context, orgID int, p ObjectiveListParams) ([]Objective, int, error) {
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
		where += fmt.Sprintf(` AND (title ILIKE $%d OR COALESCE(description,'') ILIKE $%d OR display_id ILIKE $%d)`, idx, idx, idx)
		args = append(args, "%"+p.Search+"%")
		idx++
	}
	if p.Status != "" {
		where += fmt.Sprintf(` AND status = $%d`, idx)
		args = append(args, p.Status)
		idx++
	}
	if p.ProgramID > 0 {
		where += fmt.Sprintf(` AND program_id = $%d`, idx)
		args = append(args, p.ProgramID)
		idx++
	}
	if p.Owner != "" {
		where += fmt.Sprintf(` AND owner_id = (SELECT id FROM users WHERE email = $%d)`, idx)
		args = append(args, p.Owner)
		idx++
	}

	var total int
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM objectives`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortDir := "ASC"
	sortKey := strings.TrimPrefix(p.Sort, "-")
	if strings.HasPrefix(p.Sort, "-") {
		sortDir = "DESC"
	}
	sortField, ok := objectiveSortable[sortKey]
	if !ok {
		sortField = "display_id"
		sortDir = "ASC"
	}

	offset := (p.Page - 1) * p.Limit
	args = append(args, p.Limit, offset)

	query := `SELECT ` + objectiveSelectCols + ` FROM objectives` + where +
		` ORDER BY ` + sortField + ` ` + sortDir + `, id ` + sortDir +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, idx, idx+1)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var objectives []Objective
	for rows.Next() {
		var o Objective
		if err := rows.Scan(&o.ID, &o.OrganizationID, &o.ProgramID, &o.DisplayID, &o.SeqNumber,
			&o.Title, &o.Description, &o.Owner,
			&o.Source, &o.MeasurementMethod,
			&o.TargetValue, &o.TargetOperator, &o.Unit,
			&o.WindowSeconds, &o.GraceSeconds, &o.CheckinCycle, &o.Status,
			&o.StartedAt, &o.ArchivedAt, &o.Notes, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, 0, err
		}
		objectives = append(objectives, o)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if objectives == nil {
		objectives = []Objective{}
	}
	return objectives, total, nil
}

func (o *Objective) ToChangeMap() map[string]string {
	m := map[string]string{
		"title":              o.Title,
		"description":        o.Description,
		"owner":              o.Owner,
		"source":             o.Source,
		"measurement_method": o.MeasurementMethod,
		"target_operator":    o.TargetOperator,
		"unit":               o.Unit,
		"grace_seconds":      strconv.Itoa(o.GraceSeconds),
		"checkin_cycle":      strconv.Itoa(o.CheckinCycle),
		"status":             o.Status,
		"notes":              o.Notes,
	}
	if o.TargetValue != nil {
		m["target_value"] = fmt.Sprintf("%g", *o.TargetValue)
	}
	if o.WindowSeconds != nil {
		m["window_seconds"] = strconv.Itoa(*o.WindowSeconds)
	}
	return m
}
