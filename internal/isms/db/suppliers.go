package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
	ExternalID      string `json:"external_id,omitempty"`
	// Public-facing free text for the trust center subprocessor list (#522).
	// Stored as typed and trimmed on write; the public name falls back to Name
	// only where the list is built, never in the stored value.
	PublicName         string `json:"public_name"`
	PublicPurpose      string `json:"public_purpose"`
	ProcessingLocation string `json:"processing_location"`
	DataCategories     string `json:"data_categories"`
}

// Valid supplier types.
var SupplierTypes = []string{"cloud", "saas", "consulting", "hosting", "infrastructure", "software", "contractor", "other"}

// Valid criticality levels.
var CriticalityLevels = []string{"low", "medium", "high", "critical"}

// Valid supplier lifecycle statuses.
var SupplierStatuses = []string{"active", "under_review", "suspended", "terminated"}

// SupplierReviewCycles resolves the org's supplier review cycles. Exported because
// every supplier write path outside this package (api readings, suggestion apply,
// the manager cron) must resolve real cycles — passing nil there would silently
// ignore the org's configuration, which is the bug the risk readings path has.
func (d *DB) SupplierReviewCycles(ctx context.Context, orgID int) map[string]int {
	return d.reviewCyclesFor(ctx, orgID, "supplier_review_cycle_")
}

// CalculateNextReview sets next_review from the supplier's criticality.
// cycles maps criticality level → months; pass nil to use reviewCycleDefaults
// (1/3/6/12). Resolve a real map with DB.SupplierReviewCycles — do not pass nil
// from outside this package, or the org's configured cycles are ignored (#43).
//
// A caller-supplied date survives because the write paths re-apply it after
// this runs (UpdateSupplier / UpdateSupplierTx take explicitNextReview;
// CreateSupplier keeps a pre-set NextReview).
func (s *Supplier) CalculateNextReview(cycles map[string]int) {
	if cycles == nil {
		cycles = reviewCycleDefaults
	}
	months, ok := cycles[s.Criticality]
	if !ok {
		months = 12
	}
	base := time.Now().UTC()
	if s.LastReview != nil && !s.LastReview.IsZero() {
		base = s.LastReview.Time
	}
	s.NextReview = &Epoch{Time: base.AddDate(0, months, 0)}
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
		COALESCE(notes, ''), created_at, updated_at,
		COALESCE(external_id, ''),
		public_name, public_purpose, processing_location, data_categories`

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
		&s.Notes, &s.CreatedAt, &s.UpdatedAt,
		&s.ExternalID,
		&s.PublicName, &s.PublicPurpose, &s.ProcessingLocation, &s.DataCategories)
}

// trimPublicFields normalises the public-facing fields before a write, so the
// stored value and the projection compared in the UPDATE SQL are the trimmed one.
func (s *Supplier) trimPublicFields() {
	s.PublicName = strings.TrimSpace(s.PublicName)
	s.PublicPurpose = strings.TrimSpace(s.PublicPurpose)
	s.ProcessingLocation = strings.TrimSpace(s.ProcessingLocation)
	s.DataCategories = strings.TrimSpace(s.DataCategories)
}

// supplierPublicChangedSQL is the SET clause that moves trust_public_changed_at
// only when the visible projection changes. In an UPDATE the bare column names
// read the OLD row, so the left side is the stored projection and the right side
// the bound parameters ($2 name, $19..$22 the public fields). Shared by
// UpdateSupplier and UpdateSupplierTx; computed in SQL so concurrent writers
// compare against the committed row, and no trigger is involved.
const supplierPublicChangedSQL = `trust_public_changed_at = CASE WHEN
			ROW(COALESCE(NULLIF(public_name, ''), name), public_purpose, processing_location, data_categories)
			IS DISTINCT FROM
			ROW(COALESCE(NULLIF($19::text, ''), $2::text), $20::text, $21::text, $22::text)
		THEN now() ELSE trust_public_changed_at END`

func (d *DB) CreateSupplier(ctx context.Context, orgID int, s *Supplier) error {
	s.OrganizationID = orgID
	s.trimPublicFields()
	// An explicit next_review on create wins over the calculated one (#202).
	explicitNextReview := s.NextReview
	s.CalculateNextReview(d.SupplierReviewCycles(ctx, orgID))
	if explicitNextReview != nil {
		s.NextReview = explicitNextReview
	}
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
			last_review, next_review, notes, external_id,
			public_name, public_purpose, processing_location, data_categories,
			trust_public_changed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8,
			$9,
			CASE WHEN $10 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $10) END,
			$11,
			$12, $13, $14,
			$15, $16, $17, $18,
			$19, $20, $21, $22,
			now())
		RETURNING id, created_at, updated_at
	`, orgID, s.Identifier, s.Name, s.SupplierType, s.Criticality,
		s.DataAccess, nilIfEmpty(s.Contact), nilIfEmpty(s.ContractRef),
		nilIfEmpty(s.Status), s.Owner, s.ContractExpiry,
		s.Confidentiality, s.Integrity, s.Availability,
		s.LastReview, s.NextReview, nilIfEmpty(s.Notes),
		nilIfEmpty(strings.TrimSpace(s.ExternalID)),
		s.PublicName, s.PublicPurpose, s.ProcessingLocation, s.DataCategories,
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
		where += fmt.Sprintf(` AND (name ILIKE $%d OR identifier ILIKE $%d OR COALESCE(notes,'') ILIKE $%d OR COALESCE(external_id,'') ILIKE $%d)`, idx, idx, idx, idx)
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

// UpdateSupplier updates a supplier.
//
// explicitNextReview, when non-nil, is a caller-supplied date that wins over the
// criticality-derived one. CalculateNextReview always recomputes NextReview, so
// the override has to come after it (#202). Pass nil to keep the derived date.
func (d *DB) UpdateSupplier(ctx context.Context, orgID int, s *Supplier, explicitNextReview *Epoch) error {
	// Same statement as UpdateSupplierTx (it also hides the supplier's trust
	// center item when the supplier is no longer eligible), so every writer,
	// this one included, commits the update and the hide together (#522).
	cycles := d.SupplierReviewCycles(ctx, orgID)
	return d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		return UpdateSupplierTx(ctx, tx, orgID, s, cycles, explicitNextReview)
	})
}

func (d *DB) DeleteSupplier(ctx context.Context, orgID int, id int64) error {
	return d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE suppliers SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		return hideSubprocessorForSupplierTx(ctx, tx, orgID, id, TrustHiddenDeleted, "system")
	})
}

// SupplierActive reports whether a supplier with this status is still in
// service: everything but terminated (suspended and under_review suppliers are
// still our subprocessors). One definition for the trust center subprocessor
// list and, later, the overdue-review filter (#453). supplierActiveSQL is its
// SQL twin; the two must agree.
func SupplierActive(status string) bool { return status != "terminated" }

// supplierActiveSQL is SupplierActive for a suppliers row in a query.
const supplierActiveSQL = `COALESCE(status, 'active') <> 'terminated'`

// SupplierTrustIneligibleReason says why a supplier cannot be a published
// subprocessor: "terminated" (no longer active, which wins), "no_data_access",
// or "" when it can. A deleted supplier is handled by the callers that know the
// row is gone.
func SupplierTrustIneligibleReason(s *Supplier) string {
	switch {
	case !SupplierActive(s.Status):
		return TrustHiddenTerminated
	case !s.DataAccess:
		return TrustHiddenNoDataAccess
	}
	return ""
}
