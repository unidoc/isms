package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// pgExecer is satisfied by both *pgxpool.Pool and pgx.Tx, so a pool-based DB
// method and its tx variant can share one query body (no duplicated SQL).
type pgExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// --- Status / type allowlists ---

var (
	AuditProgrammeStatuses = map[string]bool{"draft": true, "active": true, "closed": true}
	AuditStatuses          = map[string]bool{"planned": true, "in_progress": true, "completed": true}
	AuditTypes             = map[string]bool{"internal": true, "external": true, "surveillance": true, "certification": true, "recertification": true}
	AuditItemResults       = map[string]bool{"not_assessed": true, "conforming": true, "minor_nc": true, "major_nc": true, "observation": true, "opportunity": true}
	AuditFindingStatuses   = map[string]bool{"open": true, "closed": true}
	AuditFindingTypes      = map[string]bool{"major_nc": true, "minor_nc": true, "observation": true, "opportunity": true}
)

// --- Audit Programmes ---

type AuditProgramme struct {
	ID             int    `json:"id"`
	OrganizationID int    `json:"organization_id"`
	Title          string `json:"title"`
	Year           int    `json:"year"`
	Description    string `json:"description,omitempty"`
	Status         string `json:"status"`
	Notes          string `json:"notes,omitempty"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      Epoch  `json:"created_at"`
	UpdatedAt      Epoch  `json:"updated_at"`
	// Computed
	AuditCount int `json:"audit_count,omitempty"`
}

func (d *DB) CreateAuditProgramme(ctx context.Context, orgID int, p *AuditProgramme) error {
	p.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO audit_programmes (organization_id, title, year, description, status, notes, created_by, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, (SELECT id FROM users WHERE email = $7))
		RETURNING id, created_at, updated_at
	`, orgID, p.Title, p.Year, nilIfEmpty(p.Description), p.Status, nilIfEmpty(p.Notes), p.CreatedBy,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

const auditProgrammeCols = `id, organization_id, title, year, COALESCE(description, ''), status, COALESCE(notes, ''), created_by, created_at, updated_at,
	(SELECT COUNT(*) FROM audits a WHERE a.programme_id = audit_programmes.id)`

func scanAuditProgramme(r interface {
	Scan(...interface{}) error
}, p *AuditProgramme) error {
	return r.Scan(&p.ID, &p.OrganizationID, &p.Title, &p.Year, &p.Description, &p.Status, &p.Notes, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.AuditCount)
}

func (d *DB) ListAuditProgrammes(ctx context.Context, orgID int) ([]AuditProgramme, error) {
	rows, err := d.pool.Query(ctx, `SELECT `+auditProgrammeCols+` FROM audit_programmes WHERE organization_id = $1 ORDER BY year DESC, created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	programmes := []AuditProgramme{}
	for rows.Next() {
		var p AuditProgramme
		if err := scanAuditProgramme(rows, &p); err != nil {
			return nil, err
		}
		programmes = append(programmes, p)
	}
	return programmes, nil
}

func (d *DB) GetAuditProgramme(ctx context.Context, orgID int, id int) (*AuditProgramme, error) {
	var p AuditProgramme
	err := scanAuditProgramme(d.pool.QueryRow(ctx, `SELECT `+auditProgrammeCols+` FROM audit_programmes WHERE id = $1 AND organization_id = $2`, id, orgID), &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *DB) UpdateAuditProgramme(ctx context.Context, orgID, id int, title, description, notes, status *string) error {
	sets := []string{"updated_at = now()"}
	args := []interface{}{id, orgID}
	idx := 3
	add := func(col string, v *string) {
		if v == nil {
			return
		}
		sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
		args = append(args, nilIfEmpty(*v))
		idx++
	}
	add("title", title)
	add("description", description)
	add("notes", notes)
	add("status", status)
	if len(sets) == 1 {
		return nil
	}
	q := `UPDATE audit_programmes SET ` + strings.Join(sets, ", ") + ` WHERE id = $1 AND organization_id = $2`
	_, err := d.pool.Exec(ctx, q, args...)
	return err
}

// AuditProgrammeHasAudits returns true if the programme has any audits (used to gate delete).
func (d *DB) AuditProgrammeHasAudits(ctx context.Context, orgID, id int) (bool, error) {
	var n int
	err := d.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audits WHERE organization_id = $1 AND programme_id = $2`, orgID, id).Scan(&n)
	return n > 0, err
}

func (d *DB) DeleteAuditProgramme(ctx context.Context, orgID, id int) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM audit_programmes WHERE id = $1 AND organization_id = $2`, id, orgID)
	return err
}

// --- Audits ---

type Audit struct {
	ID             int    `json:"id"`
	OrganizationID int    `json:"organization_id"`
	ProgrammeID    *int   `json:"programme_id,omitempty"`
	Title          string `json:"title"`
	Scope          string `json:"scope"`
	Auditor        string `json:"auditor"`
	AuditType      string `json:"audit_type"`
	Status         string `json:"status"`
	PlannedDate    *Epoch `json:"planned_date,omitempty"`
	EndDate        *Epoch `json:"end_date,omitempty"`
	StartedAt      *Epoch `json:"started_at,omitempty"`
	CompletedAt    *Epoch `json:"completed_at,omitempty"`
	Summary        string `json:"summary,omitempty"`
	Notes          string `json:"notes,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
	UpdatedAt      Epoch  `json:"updated_at"`
	// Computed
	ItemCount    int `json:"item_count,omitempty"`
	FindingCount int `json:"finding_count,omitempty"`
	OpenFindings int `json:"open_findings,omitempty"`
}

const auditCols = `a.id, a.organization_id, a.programme_id, a.title, a.scope,
	COALESCE((SELECT email FROM users WHERE id = a.auditor_id), ''),
	COALESCE(a.audit_type, 'internal'), a.status,
	a.planned_date, a.end_date, a.started_at, a.completed_at, COALESCE(a.summary, ''), COALESCE(a.notes, ''),
	a.created_at, a.updated_at,
	(SELECT COUNT(*) FROM audit_items ai WHERE ai.audit_id = a.id),
	(SELECT COUNT(*) FROM audit_findings af WHERE af.audit_id = a.id AND af.deleted_at IS NULL),
	(SELECT COUNT(*) FROM audit_findings af WHERE af.audit_id = a.id AND af.deleted_at IS NULL AND af.status = 'open')`

func scanAudit(r interface {
	Scan(...interface{}) error
}, a *Audit) error {
	return r.Scan(&a.ID, &a.OrganizationID, &a.ProgrammeID, &a.Title, &a.Scope, &a.Auditor,
		&a.AuditType, &a.Status,
		&a.PlannedDate, &a.EndDate, &a.StartedAt, &a.CompletedAt, &a.Summary, &a.Notes,
		&a.CreatedAt, &a.UpdatedAt, &a.ItemCount, &a.FindingCount, &a.OpenFindings)
}

// ToChangeMap exposes the audit fields used by the entity changelog.
func (a *Audit) ToChangeMap() map[string]string {
	planned := ""
	if a.PlannedDate != nil {
		planned = a.PlannedDate.String()
	}
	end := ""
	if a.EndDate != nil {
		end = a.EndDate.String()
	}
	return map[string]string{
		"title":        a.Title,
		"scope":        a.Scope,
		"auditor":      a.Auditor,
		"audit_type":   a.AuditType,
		"status":       a.Status,
		"summary":      a.Summary,
		"notes":        a.Notes,
		"planned_date": planned,
		"end_date":     end,
	}
}

func (d *DB) CreateAudit(ctx context.Context, orgID int, a *Audit) error {
	a.OrganizationID = orgID
	if a.AuditType == "" {
		a.AuditType = "internal"
	}
	return d.pool.QueryRow(ctx, `
		INSERT INTO audits (organization_id, programme_id, title, scope, auditor_id, audit_type, status, planned_date, end_date, notes)
		VALUES ($1, $2, $3, $4, (SELECT id FROM users WHERE email = $5), $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at
	`, orgID, a.ProgrammeID, a.Title, a.Scope, a.Auditor, a.AuditType, a.Status, a.PlannedDate, a.EndDate, nilIfEmpty(a.Notes),
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

// AuditExists checks that an audit id belongs to the given org.
func (d *DB) AuditExists(ctx context.Context, orgID, id int) (bool, error) {
	var n int
	err := d.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audits WHERE id = $1 AND organization_id = $2`, id, orgID).Scan(&n)
	return n > 0, err
}

func (d *DB) ListAudits(ctx context.Context, orgID int, programmeID *int) ([]Audit, error) {
	query := `SELECT ` + auditCols + ` FROM audits a WHERE a.organization_id = $1`
	args := []interface{}{orgID}
	if programmeID != nil {
		query += ` AND a.programme_id = $2`
		args = append(args, *programmeID)
	}
	query += ` ORDER BY a.created_at DESC`

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	audits := []Audit{}
	for rows.Next() {
		var a Audit
		if err := scanAudit(rows, &a); err != nil {
			return nil, err
		}
		audits = append(audits, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return audits, nil
}

func (d *DB) GetAudit(ctx context.Context, orgID int, id int) (*Audit, error) {
	var a Audit
	err := scanAudit(d.pool.QueryRow(ctx, `SELECT `+auditCols+` FROM audits a WHERE a.id = $1 AND a.organization_id = $2`, id, orgID), &a)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (d *DB) ListAuditsForYear(ctx context.Context, orgID int, year int) ([]Audit, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT `+auditCols+`
		FROM audits a
		WHERE a.organization_id = $1
		  AND a.planned_date IS NOT NULL
		  AND a.planned_date >= make_date($2, 1, 1)
		  AND a.planned_date <  make_date($2 + 1, 1, 1)
		ORDER BY a.planned_date ASC
	`, orgID, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	audits := []Audit{}
	for rows.Next() {
		var a Audit
		if err := scanAudit(rows, &a); err != nil {
			return nil, err
		}
		audits = append(audits, a)
	}
	return audits, nil
}

// ListUnscheduledAudits returns audits with no planned_date (calendar's "Unscheduled" bucket).
func (d *DB) ListUnscheduledAudits(ctx context.Context, orgID int) ([]Audit, error) {
	rows, err := d.pool.Query(ctx, `SELECT `+auditCols+` FROM audits a WHERE a.organization_id = $1 AND a.planned_date IS NULL ORDER BY a.created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	audits := []Audit{}
	for rows.Next() {
		var a Audit
		if err := scanAudit(rows, &a); err != nil {
			return nil, err
		}
		audits = append(audits, a)
	}
	return audits, nil
}

func (d *DB) UpdateAuditStatus(ctx context.Context, orgID int, id int, status string) error {
	// Stamp started_at on first transition to in_progress; completed_at on first transition to completed.
	// On revert (e.g. completed -> in_progress), clear completed_at so the data stays truthful.
	switch status {
	case "in_progress":
		_, err := d.pool.Exec(ctx, `
			UPDATE audits SET status = $2, updated_at = now(),
				started_at = COALESCE(started_at, now()),
				completed_at = NULL
			WHERE id = $1 AND organization_id = $3`, id, status, orgID)
		return err
	case "completed":
		_, err := d.pool.Exec(ctx, `
			UPDATE audits SET status = $2, updated_at = now(),
				completed_at = COALESCE(completed_at, now())
			WHERE id = $1 AND organization_id = $3`, id, status, orgID)
		return err
	default:
		_, err := d.pool.Exec(ctx, `
			UPDATE audits SET status = $2, updated_at = now(),
				started_at = NULL, completed_at = NULL
			WHERE id = $1 AND organization_id = $3`, id, status, orgID)
		return err
	}
}

// UpdateAudit applies a partial update with pointer semantics — nil = leave alone, "" = clear.
func (d *DB) UpdateAudit(ctx context.Context, orgID, id int, title, scope, auditor, auditType, summary, notes *string, plannedDate, endDate **Epoch) error {
	sets := []string{"updated_at = now()"}
	args := []interface{}{id, orgID}
	idx := 3
	addStr := func(col string, v *string, allowEmpty bool) {
		if v == nil {
			return
		}
		sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
		if allowEmpty {
			args = append(args, *v)
		} else {
			args = append(args, nilIfEmpty(*v))
		}
		idx++
	}
	addStr("title", title, true)
	addStr("scope", scope, true)
	if auditor != nil {
		sets = append(sets, fmt.Sprintf("auditor_id = (SELECT id FROM users WHERE email = $%d)", idx))
		args = append(args, *auditor)
		idx++
	}
	addStr("audit_type", auditType, true)
	addStr("summary", summary, false)
	addStr("notes", notes, false)
	if plannedDate != nil {
		sets = append(sets, fmt.Sprintf("planned_date = $%d", idx))
		args = append(args, *plannedDate)
		idx++
	}
	if endDate != nil {
		sets = append(sets, fmt.Sprintf("end_date = $%d", idx))
		args = append(args, *endDate)
		idx++
	}
	if len(sets) == 1 {
		return nil
	}
	q := `UPDATE audits SET ` + strings.Join(sets, ", ") + ` WHERE id = $1 AND organization_id = $2`
	_, err := d.pool.Exec(ctx, q, args...)
	return err
}

func (d *DB) UpdateAuditSummary(ctx context.Context, orgID int, id int, summary string) error {
	_, err := d.pool.Exec(ctx, `UPDATE audits SET summary = $2, updated_at = now() WHERE id = $1 AND organization_id = $3`, id, summary, orgID)
	return err
}

// --- Audit Items ---

type AuditItem struct {
	ID             int    `json:"id"`
	OrganizationID int    `json:"organization_id"`
	AuditID        int    `json:"audit_id"`
	ItemID         string `json:"item_id"`
	ItemType       string `json:"item_type"`
	Title          string `json:"title"`
	Result         string `json:"result"`
	Evidence       string `json:"evidence,omitempty"`
	Notes          string `json:"notes,omitempty"`
	AssessedAt     *Epoch `json:"assessed_at,omitempty"`
	AssessedBy     string `json:"assessed_by,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
	UpdatedAt      Epoch  `json:"updated_at"`
}

const auditItemCols = `id, organization_id, audit_id, item_id, item_type, title, result,
	COALESCE(evidence, ''), COALESCE(notes, ''),
	assessed_at, COALESCE((SELECT email FROM users WHERE id = audit_items.assessed_by_user_id), ''),
	created_at, updated_at`

func scanAuditItem(r interface {
	Scan(...interface{}) error
}, item *AuditItem) error {
	return r.Scan(&item.ID, &item.OrganizationID, &item.AuditID, &item.ItemID, &item.ItemType, &item.Title,
		&item.Result, &item.Evidence, &item.Notes, &item.AssessedAt, &item.AssessedBy, &item.CreatedAt, &item.UpdatedAt)
}

func (d *DB) AddAuditItem(ctx context.Context, orgID int, item *AuditItem) error {
	item.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO audit_items (organization_id, audit_id, item_id, item_type, title, result)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, orgID, item.AuditID, item.ItemID, item.ItemType, item.Title, item.Result,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
}

func (d *DB) DeleteAuditItem(ctx context.Context, orgID int, id int) error {
	_, err := d.pool.Exec(ctx,
		`DELETE FROM audit_items WHERE id = $1 AND organization_id = $2`, id, orgID)
	return err
}

func (d *DB) ListAuditItems(ctx context.Context, orgID int, auditID int) ([]AuditItem, error) {
	rows, err := d.pool.Query(ctx, `SELECT `+auditItemCols+` FROM audit_items WHERE organization_id = $1 AND audit_id = $2 ORDER BY item_id ASC`, orgID, auditID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []AuditItem{}
	for rows.Next() {
		var item AuditItem
		if err := scanAuditItem(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (d *DB) GetAuditItem(ctx context.Context, orgID int, id int) (*AuditItem, error) {
	var item AuditItem
	err := scanAuditItem(d.pool.QueryRow(ctx, `SELECT `+auditItemCols+` FROM audit_items WHERE id = $1 AND organization_id = $2`, id, orgID), &item)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// UpdateAuditItem applies a partial update; pointer semantics so empty string can clear.
// Stamps assessed_at + assessed_by_user_id whenever result transitions to a non-default.
func (d *DB) UpdateAuditItem(ctx context.Context, orgID, id int, result, evidence, notes *string, assessor string) error {
	sets := []string{"updated_at = now()"}
	args := []interface{}{id, orgID}
	idx := 3
	if result != nil {
		sets = append(sets, fmt.Sprintf("result = $%d", idx))
		args = append(args, *result)
		idx++
		if *result != "not_assessed" {
			sets = append(sets, "assessed_at = now()")
			if assessor != "" {
				sets = append(sets, fmt.Sprintf("assessed_by_user_id = (SELECT id FROM users WHERE email = $%d)", idx))
				args = append(args, assessor)
				idx++
			}
		}
	}
	if evidence != nil {
		sets = append(sets, fmt.Sprintf("evidence = $%d", idx))
		args = append(args, nilIfEmpty(*evidence))
		idx++
	}
	if notes != nil {
		sets = append(sets, fmt.Sprintf("notes = $%d", idx))
		args = append(args, nilIfEmpty(*notes))
		idx++
	}
	if len(sets) == 1 {
		return nil
	}
	q := `UPDATE audit_items SET ` + strings.Join(sets, ", ") + ` WHERE id = $1 AND organization_id = $2`
	_, err := d.pool.Exec(ctx, q, args...)
	return err
}

// --- Audit Findings ---

type AuditFinding struct {
	ID             int    `json:"id"`
	OrganizationID int    `json:"organization_id"`
	AuditID        int    `json:"audit_id"`
	AuditTitle     string `json:"audit_title,omitempty"`
	AuditItemID    *int   `json:"audit_item_id,omitempty"`
	FindingType    string `json:"finding_type"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	// CorrectiveAction lives in description (## Corrective Action heading).
	// item_id text removed — audit_item_id FK + entity_references handle linkage.
	Status    string `json:"status"`
	DueDate   *Epoch `json:"due_date,omitempty"`
	Owner     string `json:"owner,omitempty"`
	ClosedAt  *Epoch `json:"closed_at,omitempty"`
	ClosedBy  string `json:"closed_by,omitempty"`
	CreatedAt Epoch  `json:"created_at"`
	UpdatedAt Epoch  `json:"updated_at"`
}

const auditFindingCols = `f.id, f.organization_id, f.audit_id,
	COALESCE((SELECT title FROM audits a2 WHERE a2.id = f.audit_id), ''),
	f.audit_item_id, f.finding_type, f.title, f.description,
	f.status,
	f.due_date,
	COALESCE((SELECT email FROM users WHERE id = f.owner_id), ''),
	f.closed_at, COALESCE(f.closed_by, ''),
	f.created_at, f.updated_at`

func scanAuditFinding(r interface {
	Scan(...interface{}) error
}, f *AuditFinding) error {
	return r.Scan(&f.ID, &f.OrganizationID, &f.AuditID, &f.AuditTitle, &f.AuditItemID, &f.FindingType, &f.Title, &f.Description,
		&f.Status,
		&f.DueDate, &f.Owner, &f.ClosedAt, &f.ClosedBy, &f.CreatedAt, &f.UpdatedAt)
}

// ToChangeMap exposes the finding fields used by the entity changelog.
func (f *AuditFinding) ToChangeMap() map[string]string {
	due := ""
	if f.DueDate != nil {
		due = f.DueDate.String()
	}
	return map[string]string{
		"finding_type": f.FindingType,
		"title":        f.Title,
		"description":  f.Description,
		"status":       f.Status,
		"due_date":     due,
		"owner":        f.Owner,
	}
}

func (d *DB) AddAuditFinding(ctx context.Context, orgID int, f *AuditFinding) error {
	f.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO audit_findings (organization_id, audit_id, audit_item_id, finding_type, title, description,
			status, due_date, owner_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8,
			CASE WHEN $9 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $9) END)
		RETURNING id, created_at, updated_at
	`, orgID, f.AuditID, f.AuditItemID, f.FindingType, f.Title, f.Description,
		f.Status, f.DueDate, f.Owner,
	).Scan(&f.ID, &f.CreatedAt, &f.UpdatedAt)
}

func (d *DB) GetAuditFinding(ctx context.Context, orgID, id int) (*AuditFinding, error) {
	var f AuditFinding
	err := scanAuditFinding(d.pool.QueryRow(ctx, `SELECT `+auditFindingCols+` FROM audit_findings f WHERE f.id = $1 AND f.organization_id = $2 AND f.deleted_at IS NULL`, id, orgID), &f)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (d *DB) ListAuditFindings(ctx context.Context, orgID int, auditID int) ([]AuditFinding, error) {
	rows, err := d.pool.Query(ctx, `SELECT `+auditFindingCols+` FROM audit_findings f WHERE f.organization_id = $1 AND f.audit_id = $2 AND f.deleted_at IS NULL ORDER BY f.created_at ASC`, orgID, auditID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	findings := []AuditFinding{}
	for rows.Next() {
		var f AuditFinding
		if err := scanAuditFinding(rows, &f); err != nil {
			return nil, err
		}
		findings = append(findings, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return findings, nil
}

// --- Server-side findings list (filter / search / sort / pagination) ---

type AuditFindingListParams struct {
	Page        int
	Limit       int
	Sort        string
	Search      string
	Status      string
	Type        string
	AuditID     int
	ProgrammeID int
	Owner       string
	OverdueOnly bool
}

var auditFindingSortable = map[string]string{
	"created": "f.created_at",
	"updated": "f.updated_at",
	"due":     "f.due_date",
	"title":   "f.title",
	"status":  "f.status",
	"type":    "CASE f.finding_type WHEN 'major_nc' THEN 1 WHEN 'minor_nc' THEN 2 WHEN 'observation' THEN 3 ELSE 4 END",
}

func (d *DB) PaginatedAuditFindings(ctx context.Context, orgID int, p AuditFindingListParams) ([]AuditFinding, int, error) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Limit < 1 {
		p.Limit = 50
	}
	if p.Limit > 200 {
		p.Limit = 200
	}

	where := ` WHERE f.organization_id = $1 AND f.deleted_at IS NULL`
	args := []interface{}{orgID}
	idx := 2
	if p.Search != "" {
		where += fmt.Sprintf(` AND (f.title ILIKE $%d OR f.description ILIKE $%d)`, idx, idx)
		args = append(args, "%"+p.Search+"%")
		idx++
	}
	if p.Status != "" {
		where += fmt.Sprintf(` AND f.status = $%d`, idx)
		args = append(args, p.Status)
		idx++
	}
	if p.Type != "" {
		where += fmt.Sprintf(` AND f.finding_type = $%d`, idx)
		args = append(args, p.Type)
		idx++
	}
	if p.AuditID > 0 {
		where += fmt.Sprintf(` AND f.audit_id = $%d`, idx)
		args = append(args, p.AuditID)
		idx++
	}
	if p.ProgrammeID > 0 {
		where += fmt.Sprintf(` AND f.audit_id IN (SELECT id FROM audits WHERE organization_id = $1 AND programme_id = $%d)`, idx)
		args = append(args, p.ProgrammeID)
		idx++
	}
	if p.Owner != "" {
		where += fmt.Sprintf(` AND f.owner_id = (SELECT id FROM users WHERE email = $%d)`, idx)
		args = append(args, p.Owner)
		idx++
	}
	if p.OverdueOnly {
		where += ` AND f.status = 'open' AND f.due_date IS NOT NULL AND f.due_date < CURRENT_DATE`
	}

	var total int
	if err := d.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_findings f`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortDir := "DESC"
	sortKey := strings.TrimPrefix(p.Sort, "-")
	if !strings.HasPrefix(p.Sort, "-") && p.Sort != "" {
		sortDir = "ASC"
	}
	sortField, ok := auditFindingSortable[sortKey]
	if !ok {
		// Default: open first (status='open' before 'closed'), severity asc, then created desc.
		sortField = "CASE f.status WHEN 'open' THEN 0 ELSE 1 END, CASE f.finding_type WHEN 'major_nc' THEN 1 WHEN 'minor_nc' THEN 2 WHEN 'observation' THEN 3 ELSE 4 END"
		sortDir = "ASC"
	}

	offset := (p.Page - 1) * p.Limit
	args = append(args, p.Limit, offset)
	limitIdx, offsetIdx := idx, idx+1

	q := `SELECT ` + auditFindingCols + ` FROM audit_findings f` + where +
		` ORDER BY ` + sortField + ` ` + sortDir + `, f.created_at DESC` +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, limitIdx, offsetIdx)

	rows, err := d.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	findings := []AuditFinding{}
	for rows.Next() {
		var f AuditFinding
		if err := scanAuditFinding(rows, &f); err != nil {
			return nil, 0, err
		}
		findings = append(findings, f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return findings, total, nil
}

// UpdateAuditFindingPartial applies a partial update with pointer semantics.
// nil = leave alone, non-nil = set (empty string clears, except for required fields).
// Corrective action content is folded into description (## Corrective Action heading).
func (d *DB) UpdateAuditFindingPartial(ctx context.Context, orgID, id int, title, description, owner *string, dueDate **Epoch) error {
	sets := []string{"updated_at = now()"}
	args := []interface{}{id, orgID}
	idx := 3
	if title != nil {
		if *title == "" {
			return fmt.Errorf("title cannot be empty")
		}
		sets = append(sets, fmt.Sprintf("title = $%d", idx))
		args = append(args, *title)
		idx++
	}
	if description != nil {
		if *description == "" {
			return fmt.Errorf("description cannot be empty")
		}
		sets = append(sets, fmt.Sprintf("description = $%d", idx))
		args = append(args, *description)
		idx++
	}
	if owner != nil {
		sets = append(sets, fmt.Sprintf("owner_id = CASE WHEN $%d = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $%d) END", idx, idx))
		args = append(args, *owner)
		idx++
	}
	if dueDate != nil {
		sets = append(sets, fmt.Sprintf("due_date = $%d", idx))
		args = append(args, *dueDate)
		idx++
	}
	if len(sets) == 1 {
		return nil
	}
	q := `UPDATE audit_findings SET ` + strings.Join(sets, ", ") + ` WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`
	res, err := d.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("finding not found")
	}
	return nil
}

// SetAuditFindingStatus moves a finding open <-> closed and keeps closure metadata consistent.
// On close: stamps closed_at + closed_by.  On reopen: clears closed_at, closed_by, closed_by_user_id.
func (d *DB) SetAuditFindingStatus(ctx context.Context, orgID, id int, status, closedBy string) error {
	return setAuditFindingStatus(ctx, d.pool, orgID, id, status, closedBy)
}

// setAuditFindingStatus is the shared core: moves a finding open<->closed and
// keeps closure metadata consistent (closed_at/closed_by on close, cleared on
// reopen). Errors if no row matched (nonexistent or soft-deleted), so a status
// apply can't be recorded as "applied" for a mutation that touched nothing.
// Runs against the pool (DB method) or a tx (SetAuditFindingStatusTx) — one body.
func setAuditFindingStatus(ctx context.Context, e pgExecer, orgID, id int, status, closedBy string) error {
	var (
		tag pgconn.CommandTag
		err error
	)
	if status == "closed" {
		tag, err = e.Exec(ctx, `
			UPDATE audit_findings
			   SET status = 'closed',
			       closed_at = COALESCE(closed_at, now()),
			       closed_by = $2,
			       closed_by_user_id = (SELECT id FROM users WHERE email = $2),
			       updated_at = now()
			 WHERE id = $1 AND organization_id = $3 AND deleted_at IS NULL`, id, closedBy, orgID)
	} else {
		tag, err = e.Exec(ctx, `
			UPDATE audit_findings
			   SET status = $2,
			       closed_at = NULL,
			       closed_by = NULL,
			       closed_by_user_id = NULL,
			       updated_at = now()
			 WHERE id = $1 AND organization_id = $3 AND deleted_at IS NULL`, id, status, orgID)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("audit finding %d not found", id)
	}
	return nil
}

// SoftDeleteAuditFinding marks a finding as deleted.  Refuses if the finding has
// linked corrective actions (open or closed) reachable via entity_references.
func (d *DB) SoftDeleteAuditFinding(ctx context.Context, orgID, id int) error {
	findingID := fmt.Sprintf("FIND-%d", id)
	var n int
	err := d.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM corrective_actions ca
		WHERE ca.organization_id = $1
		  AND ca.deleted_at IS NULL
		  AND EXISTS (
		    SELECT 1 FROM entity_references r
		    WHERE r.organization_id = $1 AND (
		      (r.source_type = 'corrective_action' AND r.source_id = ca.identifier
		         AND r.target_type = 'audit_finding' AND r.target_id = $2)
		      OR
		      (r.target_type = 'corrective_action' AND r.target_id = ca.identifier
		         AND r.source_type = 'audit_finding' AND r.source_id = $2)
		    )
		  )
	`, orgID, findingID).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("cannot delete finding: %d corrective action(s) still linked", n)
	}
	_, err = d.pool.Exec(ctx, `UPDATE audit_findings SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
	return err
}

// AuditStats returns counts per result type for a given audit.
func (d *DB) AuditStats(ctx context.Context, orgID int, auditID int) (map[string]int, error) {
	rows, err := d.pool.Query(ctx, `SELECT result, COUNT(*) FROM audit_items WHERE organization_id = $1 AND audit_id = $2 GROUP BY result`, orgID, auditID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := map[string]int{}
	for rows.Next() {
		var result string
		var count int
		if err := rows.Scan(&result, &count); err != nil {
			return nil, err
		}
		stats[result] = count
	}
	return stats, nil
}
