package db

import (
	"context"
	"fmt"
	"strings"
)

// Allowed enum values for task fields. Mirrors schema CHECK constraints.
var (
	TaskStatuses   = []string{"open", "in_progress", "done", "cancelled"}
	TaskPriorities = []string{"critical", "high", "medium", "low"}
	TaskTypes      = []string{"general", "review", "incident_followup", "audit_followup", "ca_followup", "change_followup", "onboarding", "offboarding", "training", "other"}
)

// TaskListParams specifies filtering, sorting, and pagination for tasks.
// Cross-entity links (incident, document, control, etc.) live in entity_references —
// query the references API to find tasks linked to a given source entity.
type TaskListParams struct {
	Page     int
	Limit    int
	Sort     string
	Search   string
	Status   string
	Priority string
	TaskType string
	Assignee string
}

var taskSortable = map[string]string{
	"title":    "t.title",
	"priority": "CASE t.priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END",
	"status":   "t.status",
	"due":      "t.due_date",
	"created":  "t.created_at",
	"updated":  "t.updated_at",
}

const taskSelectCols = `t.id, t.organization_id, t.identifier, t.title, COALESCE(t.description,''), t.task_type,
	(SELECT email FROM users WHERE id = t.assignee_id), t.created_by, t.status, t.priority, t.due_date, t.completed_at, t.recurrence_days, COALESCE(t.notes,''), t.created_at, t.updated_at`

// Task represents a work item in the ISMS.
type Task struct {
	ID             int    `json:"id"`
	OrganizationID int    `json:"organization_id"`
	Identifier     string `json:"identifier"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	TaskType       string `json:"task_type"`
	Assignee       string `json:"assignee"`
	CreatedBy      string `json:"created_by"`
	Status         string `json:"status"`
	Priority       string `json:"priority"`
	DueDate        *Epoch `json:"due_date,omitempty"`
	CompletedAt    *Epoch `json:"completed_at,omitempty"`
	RecurrenceDays *int   `json:"recurrence_days,omitempty"`
	Notes          string `json:"notes,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
	UpdatedAt      Epoch  `json:"updated_at"`
}

func (d *DB) FindTaskByTitle(ctx context.Context, orgID int, title string) (*Task, error) {
	var t Task
	err := d.pool.QueryRow(ctx, `SELECT id FROM tasks WHERE organization_id = $1 AND title = $2 AND status != 'done' AND status != 'cancelled' AND deleted_at IS NULL LIMIT 1`, orgID, title).Scan(&t.ID)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (d *DB) CreateTask(ctx context.Context, orgID int, t *Task) error {
	t.OrganizationID = orgID
	ident, err := d.NextIdentifier(ctx, orgID, "task")
	if err != nil {
		return err
	}
	t.Identifier = ident
	return d.pool.QueryRow(ctx, `
		INSERT INTO tasks (organization_id, identifier, title, description, task_type, assignee_id, created_by, created_by_user_id, status, priority, due_date, recurrence_days, notes)
		VALUES ($1, $2, $3, $4, $5, (SELECT id FROM users WHERE email = $6), $7, (SELECT id FROM users WHERE email = $7), $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at
	`, orgID, t.Identifier, t.Title, t.Description, t.TaskType,
		t.Assignee, t.CreatedBy, t.Status, t.Priority, t.DueDate, t.RecurrenceDays, nilIfEmpty(t.Notes),
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func (d *DB) ListTasks(ctx context.Context, orgID int, assignee, status string, limit int) ([]Task, error) {
	query := `SELECT ` + taskSelectCols + `
		FROM tasks t WHERE t.organization_id = $1 AND t.deleted_at IS NULL`
	args := []interface{}{orgID}
	n := 1
	if assignee != "" {
		n++
		query += fmt.Sprintf(` AND t.assignee_id = (SELECT id FROM users WHERE email = $%d)`, n)
		args = append(args, assignee)
	}
	if status != "" {
		n++
		query += fmt.Sprintf(` AND t.status = $%d`, n)
		args = append(args, status)
	}
	query += ` ORDER BY CASE t.priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END, t.due_date ASC NULLS LAST`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.OrganizationID, &t.Identifier, &t.Title, &t.Description, &t.TaskType,
			&t.Assignee, &t.CreatedBy, &t.Status, &t.Priority, &t.DueDate, &t.CompletedAt, &t.RecurrenceDays,
			&t.Notes, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (d *DB) UpdateTaskStatus(ctx context.Context, orgID int, id int, status string) error {
	query := `UPDATE tasks SET status = $2, updated_at = now()`
	if status == "done" {
		query += `, completed_at = now()`
	} else {
		// Clear completed_at when task reverts from done (or any non-done status)
		query += `, completed_at = NULL`
	}
	query += ` WHERE id = $1 AND organization_id = $3 AND deleted_at IS NULL`
	_, err := d.pool.Exec(ctx, query, id, status, orgID)
	return err
}

func (d *DB) UpdateTask(ctx context.Context, orgID int, t *Task) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE tasks SET
			title = $2, description = $3,
			assignee_id = CASE WHEN $4 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $4) END,
			priority = $5, due_date = $6, task_type = $7, status = $8,
			completed_at = CASE
				WHEN $8 = 'done' AND completed_at IS NULL THEN now()
				WHEN $8 != 'done' THEN NULL
				ELSE completed_at
			END,
			notes = $9,
			updated_at = now()
		WHERE id = $1 AND organization_id = $10 AND deleted_at IS NULL
	`, t.ID, t.Title, nilIfEmpty(t.Description), t.Assignee,
		t.Priority, t.DueDate, t.TaskType, t.Status, nilIfEmpty(t.Notes), orgID)
	return err
}

func (d *DB) DeleteTask(ctx context.Context, orgID int, id int) error {
	_, err := d.pool.Exec(ctx, `UPDATE tasks SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
	return err
}

func (t *Task) ToChangeMap() map[string]string {
	m := map[string]string{
		"title":       t.Title,
		"description": t.Description,
		"assignee":    t.Assignee,
		"priority":    t.Priority,
		"status":      t.Status,
		"task_type":   t.TaskType,
		"notes":       t.Notes,
	}
	if t.DueDate != nil {
		m["due_date"] = t.DueDate.Format("2006-01-02")
	}
	return m
}

func (d *DB) GetTask(ctx context.Context, orgID int, id int) (*Task, error) {
	var t Task
	err := d.pool.QueryRow(ctx, `
		SELECT `+taskSelectCols+`
		FROM tasks t WHERE t.id = $1 AND t.organization_id = $2 AND t.deleted_at IS NULL
	`, id, orgID).Scan(&t.ID, &t.OrganizationID, &t.Identifier, &t.Title, &t.Description, &t.TaskType,
		&t.Assignee, &t.CreatedBy, &t.Status, &t.Priority, &t.DueDate, &t.CompletedAt, &t.RecurrenceDays,
		&t.Notes, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// OverdueTasks returns tasks past their due date that aren't done/cancelled.
func (d *DB) OverdueTasks(ctx context.Context, orgID int) ([]Task, error) {
	return d.ListTasksWhere(ctx, orgID, `status NOT IN ('done','cancelled') AND due_date < now()`, 100)
}

func (d *DB) ListTasksWhere(ctx context.Context, orgID int, where string, limit int) ([]Task, error) {
	query := fmt.Sprintf(`SELECT `+taskSelectCols+`
		FROM tasks t WHERE t.organization_id = $1 AND t.deleted_at IS NULL AND %s ORDER BY t.due_date ASC NULLS LAST LIMIT %d`, where, limit)

	rows, err := d.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.OrganizationID, &t.Identifier, &t.Title, &t.Description, &t.TaskType,
			&t.Assignee, &t.CreatedBy, &t.Status, &t.Priority, &t.DueDate, &t.CompletedAt, &t.RecurrenceDays,
			&t.Notes, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

// TaskStats are aggregate counts across the entire register.
type TaskStats struct {
	Total      int `json:"total"`
	Open       int `json:"open"`
	InProgress int `json:"in_progress"`
	Done       int `json:"done"`
	Cancelled  int `json:"cancelled"`
	Critical   int `json:"critical"`
	High       int `json:"high"`
	Medium     int `json:"medium"`
	Low        int `json:"low"`
}

// TaskStats returns counts by status and priority for the org.
func (d *DB) TaskStats(ctx context.Context, orgID int) (*TaskStats, error) {
	var s TaskStats
	err := d.pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE status = 'open'),
			count(*) FILTER (WHERE status = 'in_progress'),
			count(*) FILTER (WHERE status = 'done'),
			count(*) FILTER (WHERE status = 'cancelled'),
			count(*) FILTER (WHERE priority = 'critical'),
			count(*) FILTER (WHERE priority = 'high'),
			count(*) FILTER (WHERE priority = 'medium'),
			count(*) FILTER (WHERE priority = 'low')
		FROM tasks
		WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&s.Total, &s.Open, &s.InProgress, &s.Done, &s.Cancelled,
		&s.Critical, &s.High, &s.Medium, &s.Low)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PaginatedTasks returns a filtered/sorted/paginated slice of tasks plus total count.
func (d *DB) PaginatedTasks(ctx context.Context, orgID int, p TaskListParams) ([]Task, int, error) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Limit < 1 {
		p.Limit = 50
	}
	if p.Limit > 200 {
		p.Limit = 200
	}

	where := ` WHERE t.organization_id = $1 AND t.deleted_at IS NULL`
	args := []interface{}{orgID}
	idx := 2
	if p.Search != "" {
		where += fmt.Sprintf(` AND (t.title ILIKE $%d OR COALESCE(t.description,'') ILIKE $%d)`, idx, idx)
		args = append(args, "%"+p.Search+"%")
		idx++
	}
	if p.Status != "" {
		where += fmt.Sprintf(` AND t.status = $%d`, idx)
		args = append(args, p.Status)
		idx++
	}
	if p.Priority != "" {
		where += fmt.Sprintf(` AND t.priority = $%d`, idx)
		args = append(args, p.Priority)
		idx++
	}
	if p.TaskType != "" {
		where += fmt.Sprintf(` AND t.task_type = $%d`, idx)
		args = append(args, p.TaskType)
		idx++
	}
	if p.Assignee != "" {
		where += fmt.Sprintf(` AND t.assignee_id = (SELECT id FROM users WHERE email = $%d)`, idx)
		args = append(args, p.Assignee)
		idx++
	}

	var total int
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM tasks t`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortDir := "ASC"
	sortKey := strings.TrimPrefix(p.Sort, "-")
	if strings.HasPrefix(p.Sort, "-") {
		sortDir = "DESC"
	}
	sortField, ok := taskSortable[sortKey]
	if !ok {
		// default: priority asc, due_date asc nulls last
		sortField = "CASE t.priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END"
		sortDir = "ASC"
	}

	offset := (p.Page - 1) * p.Limit
	args = append(args, p.Limit, offset)

	query := `SELECT ` + taskSelectCols + ` FROM tasks t` + where +
		` ORDER BY ` + sortField + ` ` + sortDir + `, t.due_date ASC NULLS LAST, t.id DESC` +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, idx, idx+1)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.OrganizationID, &t.Identifier, &t.Title, &t.Description, &t.TaskType,
			&t.Assignee, &t.CreatedBy, &t.Status, &t.Priority, &t.DueDate, &t.CompletedAt, &t.RecurrenceDays,
			&t.Notes, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, t)
	}
	if tasks == nil {
		tasks = []Task{}
	}
	return tasks, total, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
