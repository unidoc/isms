package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Allowed enum values for system fields. Mirrors schema CHECK constraints.
var (
	SystemStatuses        = []string{"active", "under_review", "decommissioned"}
	SystemCriticalities   = []string{"low", "medium", "high", "critical"}
	SystemClassifications = []string{"public", "internal", "confidential", "restricted"}
)

// SystemListParams specifies filtering, sorting, and pagination for the systems register.
type SystemListParams struct {
	Page        int
	Limit       int
	Sort        string // "name", "-updated", "identifier", "criticality", "status"
	Search      string
	Department  string
	Criticality string
	Status      string
	Owner       string
	SupplierID  int64 // reverse lookup: systems linked to this supplier
}

var systemSortable = map[string]string{
	"name":        "name",
	"identifier":  "identifier",
	"criticality": "criticality",
	"status":      "status",
	"updated":     "updated_at",
}

// SystemStats holds aggregate counts independent of pagination.
type SystemStats struct {
	Total          int `json:"total"`
	Active         int `json:"active"`
	UnderReview    int `json:"under_review"`
	Decommissioned int `json:"decommissioned"`
	Critical       int `json:"critical"`
	High           int `json:"high"`
	Medium         int `json:"medium"`
	Low            int `json:"low"`
}

// System represents an IT system in the systems register.
type System struct {
	ID             int64  `json:"id"`
	Identifier     string `json:"identifier"`
	OrganizationID int    `json:"organization_id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	SupplierID     *int64 `json:"supplier_id,omitempty"`
	Department     string `json:"department,omitempty"`
	// Purpose lives in description (## Purpose heading)
	Classification  string `json:"classification"`
	Criticality     string `json:"criticality"`
	Status          string `json:"status"`
	RPOHours        int    `json:"rpo_hours"`
	RTOHours        int    `json:"rto_hours"`
	Confidentiality *int   `json:"confidentiality"`
	Integrity       *int   `json:"integrity"`
	Availability    *int   `json:"availability"`
	// AuthMethod lives in notes (## Access control heading)
	LastReview *Epoch `json:"last_review,omitempty"`
	NextReview *Epoch `json:"next_review,omitempty"`
	Owner      string `json:"owner,omitempty"`
	Notes      string `json:"notes,omitempty"`
	CreatedAt  Epoch  `json:"created_at"`
	UpdatedAt  Epoch  `json:"updated_at"`
}

// ToChangeMap returns a map of field names to string values for changelog diffing.
func (sys *System) ToChangeMap() map[string]string {
	supplierID := ""
	if sys.SupplierID != nil {
		supplierID = strconv.FormatInt(*sys.SupplierID, 10)
	}
	return map[string]string{
		"name":            sys.Name,
		"description":     sys.Description,
		"supplier_id":     supplierID,
		"department":      sys.Department,
		"classification":  sys.Classification,
		"criticality":     sys.Criticality,
		"status":          sys.Status,
		"rpo_hours":       strconv.Itoa(sys.RPOHours),
		"rto_hours":       strconv.Itoa(sys.RTOHours),
		"confidentiality": intPtrStr(sys.Confidentiality),
		"integrity":       intPtrStr(sys.Integrity),
		"availability":    intPtrStr(sys.Availability),
		"last_review":     epochToString(sys.LastReview),
		"next_review":     epochToString(sys.NextReview),
		"owner":           sys.Owner,
		"notes":           sys.Notes,
	}
}

// systemReviewMonths derives review cycle (months) from criticality.
// Mirrors the supplier pattern: critical=1, high=3, medium=6, low/unset=12.
func systemReviewMonths(criticality string) int {
	switch criticality {
	case "critical":
		return 1
	case "high":
		return 3
	case "medium":
		return 6
	default:
		return 12
	}
}

// CalculateNextReview sets next_review based on criticality.
// Cycle is purely derived — users override the date through readings/access reviews.
func (sys *System) CalculateNextReview() {
	months := systemReviewMonths(sys.Criticality)
	base := time.Now()
	if sys.LastReview != nil && !sys.LastReview.IsZero() {
		base = sys.LastReview.Time
	}
	next := base.AddDate(0, months, 0)
	sys.NextReview = &Epoch{Time: next}
}

// RPO/RTO classification helpers.
var CriticalityRPO = map[string]string{
	"critical": "<=4 hours",
	"high":     "<=12 hours",
	"medium":   "<=24 hours",
	"low":      "<=7 days",
}

var CriticalityRTO = map[string]string{
	"critical": "<=4 hours",
	"high":     "<=12 hours",
	"medium":   "<=48 hours",
	"low":      "<=7 days",
}

// Shared column lists — keep INSERT/SELECT/UPDATE in lock-step.
const systemSelectCols = `id, organization_id, identifier, name, COALESCE(description, ''),
		supplier_id, COALESCE(department, ''),
		classification, criticality, COALESCE(status, 'active'),
		rpo_hours, rto_hours,
		confidentiality, integrity, availability,
		last_review, next_review,
		COALESCE((SELECT email FROM users WHERE id = systems.owner_id), ''),
		COALESCE(notes, ''), created_at, updated_at`

func scanSystem(scanner interface {
	Scan(dest ...interface{}) error
}, sys *System) error {
	return scanner.Scan(&sys.ID, &sys.OrganizationID, &sys.Identifier, &sys.Name, &sys.Description,
		&sys.SupplierID, &sys.Department,
		&sys.Classification, &sys.Criticality, &sys.Status,
		&sys.RPOHours, &sys.RTOHours,
		&sys.Confidentiality, &sys.Integrity, &sys.Availability,
		&sys.LastReview, &sys.NextReview,
		&sys.Owner,
		&sys.Notes, &sys.CreatedAt, &sys.UpdatedAt)
}

func (d *DB) CreateSystem(ctx context.Context, orgID int, sys *System) error {
	sys.OrganizationID = orgID
	sys.CalculateNextReview()
	ident, err := d.NextIdentifier(ctx, orgID, "system")
	if err != nil {
		return err
	}
	sys.Identifier = ident
	if sys.Status == "" {
		sys.Status = "active"
	}
	return d.pool.QueryRow(ctx, `
		INSERT INTO systems (organization_id, identifier, name, description, supplier_id, department,
			classification, criticality, status, rpo_hours, rto_hours,
			confidentiality, integrity, availability,
			last_review, next_review,
			owner_id, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
			CASE WHEN $17 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $17) END,
			$18)
		RETURNING id, created_at, updated_at
	`, orgID, sys.Identifier, sys.Name, nilIfEmpty(sys.Description), sys.SupplierID,
		nilIfEmpty(sys.Department),
		sys.Classification, sys.Criticality, sys.Status,
		sys.RPOHours, sys.RTOHours,
		sys.Confidentiality, sys.Integrity, sys.Availability,
		sys.LastReview, sys.NextReview,
		sys.Owner, nilIfEmpty(sys.Notes),
	).Scan(&sys.ID, &sys.CreatedAt, &sys.UpdatedAt)
}

func (d *DB) GetSystem(ctx context.Context, orgID int, id int64) (*System, error) {
	var sys System
	row := d.pool.QueryRow(ctx, `
		SELECT `+systemSelectCols+`
		FROM systems WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID)
	if err := scanSystem(row, &sys); err != nil {
		return nil, err
	}
	return &sys, nil
}

func (d *DB) GetSystemByIdentifier(ctx context.Context, orgID int, identifier string) (*System, error) {
	var sys System
	row := d.pool.QueryRow(ctx, `
		SELECT `+systemSelectCols+`
		FROM systems WHERE identifier = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, identifier, orgID)
	if err := scanSystem(row, &sys); err != nil {
		return nil, err
	}
	return &sys, nil
}

func (d *DB) ListSystems(ctx context.Context, orgID int) ([]System, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT `+systemSelectCols+`
		FROM systems WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY identifier
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	systems := []System{}
	for rows.Next() {
		var sys System
		if err := scanSystem(rows, &sys); err != nil {
			return nil, err
		}
		systems = append(systems, sys)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return systems, nil
}

// SystemStats returns counts by criticality and status for the org.
func (d *DB) SystemStats(ctx context.Context, orgID int) (*SystemStats, error) {
	var s SystemStats
	err := d.pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE status = 'active'),
			count(*) FILTER (WHERE status = 'under_review'),
			count(*) FILTER (WHERE status = 'decommissioned'),
			count(*) FILTER (WHERE criticality = 'critical'),
			count(*) FILTER (WHERE criticality = 'high'),
			count(*) FILTER (WHERE criticality = 'medium'),
			count(*) FILTER (WHERE criticality = 'low')
		FROM systems
		WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&s.Total, &s.Active, &s.UnderReview, &s.Decommissioned,
		&s.Critical, &s.High, &s.Medium, &s.Low)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PaginatedSystems returns a filtered/sorted/paginated slice plus total count.
func (d *DB) PaginatedSystems(ctx context.Context, orgID int, p SystemListParams) ([]System, int, error) {
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
		where += fmt.Sprintf(` AND (name ILIKE $%d OR COALESCE(description,'') ILIKE $%d OR identifier ILIKE $%d)`, idx, idx, idx)
		args = append(args, "%"+p.Search+"%")
		idx++
	}
	if p.Department != "" {
		where += fmt.Sprintf(` AND department = $%d`, idx)
		args = append(args, p.Department)
		idx++
	}
	if p.Criticality != "" {
		where += fmt.Sprintf(` AND criticality = $%d`, idx)
		args = append(args, p.Criticality)
		idx++
	}
	if p.Status != "" {
		where += fmt.Sprintf(` AND status = $%d`, idx)
		args = append(args, p.Status)
		idx++
	}
	if p.Owner != "" {
		where += fmt.Sprintf(` AND owner_id = (SELECT id FROM users WHERE email = $%d)`, idx)
		args = append(args, p.Owner)
		idx++
	}
	if p.SupplierID > 0 {
		where += fmt.Sprintf(` AND supplier_id = $%d`, idx)
		args = append(args, p.SupplierID)
		idx++
	}

	var total int
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM systems`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortDir := "ASC"
	sortKey := strings.TrimPrefix(p.Sort, "-")
	if strings.HasPrefix(p.Sort, "-") {
		sortDir = "DESC"
	}
	sortField, ok := systemSortable[sortKey]
	if !ok {
		sortField = "identifier"
		sortDir = "ASC"
	}

	offset := (p.Page - 1) * p.Limit
	args = append(args, p.Limit, offset)

	query := `SELECT ` + systemSelectCols + ` FROM systems` + where +
		` ORDER BY ` + sortField + ` ` + sortDir + `, identifier ` + sortDir +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, idx, idx+1)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var systems []System
	for rows.Next() {
		var sys System
		if err := scanSystem(rows, &sys); err != nil {
			return nil, 0, err
		}
		systems = append(systems, sys)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if systems == nil {
		systems = []System{}
	}
	return systems, total, nil
}

func (d *DB) UpdateSystem(ctx context.Context, orgID int, sys *System) error {
	sys.CalculateNextReview()
	if sys.Status == "" {
		sys.Status = "active"
	}
	_, err := d.pool.Exec(ctx, `
		UPDATE systems SET name = $2, description = $3, supplier_id = $4,
			department = $5, classification = $6, criticality = $7,
			status = $8,
			rpo_hours = $9, rto_hours = $10,
			confidentiality = $11, integrity = $12, availability = $13,
			last_review = $14, next_review = $15,
			owner_id = CASE WHEN $16 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $16) END,
			notes = $17, updated_at = now()
		WHERE id = $1 AND organization_id = $18 AND deleted_at IS NULL
	`, sys.ID, sys.Name, nilIfEmpty(sys.Description), sys.SupplierID,
		nilIfEmpty(sys.Department),
		sys.Classification, sys.Criticality, sys.Status,
		sys.RPOHours, sys.RTOHours,
		sys.Confidentiality, sys.Integrity, sys.Availability,
		sys.LastReview, sys.NextReview,
		sys.Owner, nilIfEmpty(sys.Notes), orgID)
	return err
}

func (d *DB) DeleteSystem(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `UPDATE systems SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
	return err
}
