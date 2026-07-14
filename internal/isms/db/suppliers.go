package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SupplierListParams specifies filtering, sorting, and pagination for the supplier register.
type SupplierListParams struct {
	Page        int
	Limit       int
	Sort        string // "name", "-updated", "identifier", "criticality", "status"
	Search      string
	Type        string
	Criticality string
	Status      string
	Owner       string
}

var supplierSortable = map[string]string{
	"name":        "name",
	"identifier":  "identifier",
	"criticality": "criticality",
	"status":      "status",
	"updated":     "updated_at",
}

// SupplierStats holds aggregate counts independent of pagination.
type SupplierStats struct {
	Total       int `json:"total"`
	Active      int `json:"active"`
	UnderReview int `json:"under_review"`
	Suspended   int `json:"suspended"`
	Terminated  int `json:"terminated"`
	Critical    int `json:"critical"`
	High        int `json:"high"`
	Medium      int `json:"medium"`
	Low         int `json:"low"`
}

// Supplier represents a supplier in the register.
type Supplier struct {
	ID             int64  `json:"id"`
	OrganizationID int    `json:"organization_id"`
	Identifier     string `json:"identifier"` // SUPPLIER-001
	Name           string `json:"name"`
	SupplierType   string `json:"supplier_type"`
	Criticality    string `json:"criticality"`
	// Services description lives in notes (## Services heading)
	DataAccess     bool   `json:"data_access"`
	Contact        string `json:"contact,omitempty"`
	ContractRef    string `json:"contract_ref,omitempty"`
	Status         string `json:"status"`
	OwnerID        *int   `json:"owner_id,omitempty"`
	Owner          string `json:"owner,omitempty"` // resolved email
	ContractExpiry *Epoch `json:"contract_expiry,omitempty"`
	// AssessmentStatus is derivable from last_review/next_review and supplier_reviews — no column.
	Confidentiality *int   `json:"confidentiality"`
	Integrity       *int   `json:"integrity"`
	Availability    *int   `json:"availability"`
	LastReview      *Epoch `json:"last_review,omitempty"`
	NextReview      *Epoch `json:"next_review,omitempty"`
	Notes           string `json:"notes,omitempty"`
	CreatedAt       Epoch  `json:"created_at"`
	UpdatedAt       Epoch  `json:"updated_at"`
}

// Valid supplier types.
var SupplierTypes = []string{"cloud", "saas", "consulting", "hosting", "infrastructure", "software", "contractor", "other"}

// Valid criticality levels.
var CriticalityLevels = []string{"low", "medium", "high", "critical"}

// Valid supplier lifecycle statuses.
var SupplierStatuses = []string{"active", "under_review", "suspended", "terminated"}

// supplierReviewMonths derives review cycle (months) from criticality.
// Mirrors the risk level mapping: critical=1, high=3, medium=6, low/unset=12.
func supplierReviewMonths(criticality string) int {
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

// CalculateNextReview sets next_review based on criticality (when not already set).
// Cycle is purely derived — users override the date through readings/reviews.
func (s *Supplier) CalculateNextReview() {
	months := supplierReviewMonths(s.Criticality)
	base := time.Now()
	if s.LastReview != nil && !s.LastReview.IsZero() {
		base = s.LastReview.Time
	}
	next := base.AddDate(0, months, 0)
	s.NextReview = &Epoch{Time: next}
}

// Shared column lists — keep INSERT/SELECT/UPDATE in lock-step.
const supplierSelectCols = `id, organization_id, identifier, name, supplier_type, criticality,
		data_access,
		COALESCE(contact, ''), COALESCE(contract_ref, ''),
		COALESCE(status, 'active'), owner_id,
		COALESCE((SELECT email FROM users WHERE id = suppliers.owner_id), ''),
		contract_expiry,
		confidentiality, integrity, availability,
		last_review, next_review,
		COALESCE(notes, ''), created_at, updated_at`

func scanSupplier(scanner interface {
	Scan(dest ...interface{}) error
}, s *Supplier) error {
	return scanner.Scan(&s.ID, &s.OrganizationID, &s.Identifier, &s.Name, &s.SupplierType, &s.Criticality,
		&s.DataAccess,
		&s.Contact, &s.ContractRef,
		&s.Status, &s.OwnerID, &s.Owner,
		&s.ContractExpiry,
		&s.Confidentiality, &s.Integrity, &s.Availability,
		&s.LastReview, &s.NextReview,
		&s.Notes, &s.CreatedAt, &s.UpdatedAt)
}

func (d *DB) CreateSupplier(ctx context.Context, orgID int, s *Supplier) error {
	s.OrganizationID = orgID
	s.CalculateNextReview()
	ident, err := d.NextIdentifier(ctx, orgID, "supplier")
	if err != nil {
		return err
	}
	s.Identifier = ident
	return d.pool.QueryRow(ctx, `
		INSERT INTO suppliers (organization_id, identifier, name, supplier_type, criticality,
			data_access, contact, contract_ref,
			status, owner_id, contract_expiry,
			confidentiality, integrity, availability,
			last_review, next_review, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8,
			$9,
			CASE WHEN $10 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $10) END,
			$11,
			$12, $13, $14,
			$15, $16, $17)
		RETURNING id, created_at, updated_at
	`, orgID, s.Identifier, s.Name, s.SupplierType, s.Criticality,
		s.DataAccess, nilIfEmpty(s.Contact), nilIfEmpty(s.ContractRef),
		nilIfEmpty(s.Status), s.Owner, s.ContractExpiry,
		s.Confidentiality, s.Integrity, s.Availability,
		s.LastReview, s.NextReview, nilIfEmpty(s.Notes),
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

func (d *DB) GetSupplier(ctx context.Context, orgID int, id int64) (*Supplier, error) {
	var s Supplier
	row := d.pool.QueryRow(ctx, `
		SELECT `+supplierSelectCols+`
		FROM suppliers WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID)
	if err := scanSupplier(row, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *DB) ListSuppliers(ctx context.Context, orgID int) ([]Supplier, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT `+supplierSelectCols+`
		FROM suppliers WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY identifier
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	suppliers := []Supplier{}
	for rows.Next() {
		var s Supplier
		if err := scanSupplier(rows, &s); err != nil {
			return nil, err
		}
		suppliers = append(suppliers, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return suppliers, nil
}

// SupplierStats returns counts by criticality and status for the org.
func (d *DB) SupplierStats(ctx context.Context, orgID int) (*SupplierStats, error) {
	var s SupplierStats
	err := d.pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE status = 'active'),
			count(*) FILTER (WHERE status = 'under_review'),
			count(*) FILTER (WHERE status = 'suspended'),
			count(*) FILTER (WHERE status = 'terminated'),
			count(*) FILTER (WHERE criticality = 'critical'),
			count(*) FILTER (WHERE criticality = 'high'),
			count(*) FILTER (WHERE criticality = 'medium'),
			count(*) FILTER (WHERE criticality = 'low')
		FROM suppliers
		WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&s.Total, &s.Active, &s.UnderReview, &s.Suspended, &s.Terminated,
		&s.Critical, &s.High, &s.Medium, &s.Low)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PaginatedSuppliers returns a filtered/sorted/paginated slice plus total count.
func (d *DB) PaginatedSuppliers(ctx context.Context, orgID int, p SupplierListParams) ([]Supplier, int, error) {
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
		where += fmt.Sprintf(` AND (name ILIKE $%d OR identifier ILIKE $%d OR COALESCE(notes,'') ILIKE $%d)`, idx, idx, idx)
		args = append(args, "%"+p.Search+"%")
		idx++
	}
	if p.Type != "" {
		where += fmt.Sprintf(` AND supplier_type = $%d`, idx)
		args = append(args, p.Type)
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

	var total int
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM suppliers`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortDir := "ASC"
	sortKey := strings.TrimPrefix(p.Sort, "-")
	if strings.HasPrefix(p.Sort, "-") {
		sortDir = "DESC"
	}
	sortField, ok := supplierSortable[sortKey]
	if !ok {
		sortField = "identifier"
		sortDir = "ASC"
	}

	offset := (p.Page - 1) * p.Limit
	args = append(args, p.Limit, offset)

	query := `SELECT ` + supplierSelectCols + ` FROM suppliers` + where +
		` ORDER BY ` + sortField + ` ` + sortDir + `, identifier ` + sortDir +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, idx, idx+1)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var suppliers []Supplier
	for rows.Next() {
		var s Supplier
		if err := scanSupplier(rows, &s); err != nil {
			return nil, 0, err
		}
		suppliers = append(suppliers, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if suppliers == nil {
		suppliers = []Supplier{}
	}
	return suppliers, total, nil
}

func (d *DB) GetSupplierByIdentifier(ctx context.Context, orgID int, identifier string) (*Supplier, error) {
	var id int64
	err := d.pool.QueryRow(ctx, `SELECT id FROM suppliers WHERE organization_id = $1 AND identifier = $2 AND deleted_at IS NULL`, orgID, identifier).Scan(&id)
	if err != nil {
		return nil, err
	}
	return d.GetSupplier(ctx, orgID, id)
}

func (d *DB) UpdateSupplier(ctx context.Context, orgID int, s *Supplier) error {
	s.CalculateNextReview()
	_, err := d.pool.Exec(ctx, `
		UPDATE suppliers SET name = $2, supplier_type = $3, criticality = $4,
			data_access = $5, contact = $6, contract_ref = $7,
			status = $8,
			owner_id = CASE WHEN $9 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $9) END,
			contract_expiry = $10,
			confidentiality = $11, integrity = $12, availability = $13,
			last_review = $14, next_review = $15,
			notes = $16, updated_at = now()
		WHERE id = $1 AND organization_id = $17 AND deleted_at IS NULL
	`, s.ID, s.Name, s.SupplierType, s.Criticality,
		s.DataAccess, nilIfEmpty(s.Contact), nilIfEmpty(s.ContractRef),
		nilIfEmpty(s.Status), s.Owner, s.ContractExpiry,
		s.Confidentiality, s.Integrity, s.Availability,
		s.LastReview, s.NextReview,
		nilIfEmpty(s.Notes), orgID)
	return err
}

func (d *DB) DeleteSupplier(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `UPDATE suppliers SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
	return err
}
