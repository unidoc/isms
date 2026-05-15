package db

import (
	"context"
	"fmt"
	"strings"
)

// AssetListParams specifies filtering, sorting, and pagination for the asset register.
type AssetListParams struct {
	Page   int
	Limit  int
	Sort   string // "name", "-updated", "identifier", "type", "status"
	Search string
	Type   string
	Status string
	Owner  string
}

var assetSortable = map[string]string{
	"name":       "name",
	"identifier": "identifier",
	"type":       "asset_type",
	"asset_type": "asset_type",
	"status":     "status",
	"updated":    "updated_at",
}

// AssetStats holds aggregate counts independent of pagination.
type AssetStats struct {
	Total    int `json:"total"`
	Draft    int `json:"draft"`
	Open     int `json:"open"`
	Archived int `json:"archived"`
	Critical int `json:"critical"` // any CIA == 5
}

// Asset represents an information asset in the register.
type Asset struct {
	ID              int64  `json:"id"`
	OrganizationID  int    `json:"organization_id"`
	Identifier      string `json:"identifier"` // AST-001
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	AssetType       string `json:"asset_type"`
	Status          string `json:"status"`
	Owner           string `json:"owner,omitempty"`
	PrimaryLocation string `json:"primary_location,omitempty"`
	Confidentiality *int   `json:"confidentiality"`
	Integrity       *int   `json:"integrity"`
	Availability    *int   `json:"availability"`
	LastReview      *Epoch `json:"last_review,omitempty"`
	NextReview      *Epoch `json:"next_review,omitempty"`
	Notes           string `json:"notes,omitempty"`
	CreatedAt       Epoch  `json:"created_at"`
	UpdatedAt       Epoch  `json:"updated_at"`
}

// Valid asset types.
var AssetTypes = []string{
	"infrastructure", "processing_devices", "software", "financial_info",
	"personal_data", "ipr", "sales_marketing", "processing_facility",
	"products_services", "supply_chain", "system", "network", "service", "other",
}

// CIALevelNames maps CIA integer ratings to display names.
var CIALevelNames = map[int]string{
	0: "Not Assessed",
	1: "Insignificant",
	2: "Minor",
	3: "Moderate",
	4: "Major",
	5: "Severe",
}

// Valid asset statuses.
var AssetStatuses = []string{"draft", "open", "archived"}

// Valid primary location options.
var PrimaryLocations = []string{"company_office", "third_party_dc", "on_person", "everywhere", "other"}

func (d *DB) CreateAsset(ctx context.Context, orgID int, a *Asset) error {
	a.OrganizationID = orgID
	ident, err := d.NextIdentifier(ctx, orgID, "asset")
	if err != nil {
		return err
	}
	a.Identifier = ident
	return d.pool.QueryRow(ctx, `
		INSERT INTO assets (organization_id, identifier, name, description, asset_type, status,
			owner_id, primary_location, confidentiality, integrity, availability,
			last_review, next_review, notes)
		VALUES ($1, $2, $3, $4, $5, $6,
			CASE WHEN $7 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $7) END,
			$8, $9, $10, $11,
			$12, $13, $14)
		RETURNING id, created_at, updated_at
	`, orgID, a.Identifier, a.Name, nilIfEmpty(a.Description), a.AssetType, a.Status,
		a.Owner, nilIfEmpty(a.PrimaryLocation),
		a.Confidentiality, a.Integrity, a.Availability,
		a.LastReview, a.NextReview, nilIfEmpty(a.Notes),
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

const assetSelectCols = `id, organization_id, identifier, name, COALESCE(description, ''), asset_type, status,
		COALESCE((SELECT email FROM users WHERE id = assets.owner_id), ''), COALESCE(primary_location, ''),
		confidentiality, integrity, availability,
		last_review, next_review,
		COALESCE(notes, ''),
		created_at, updated_at`

func scanAsset(scanner interface{ Scan(dest ...interface{}) error }, a *Asset) error {
	return scanner.Scan(&a.ID, &a.OrganizationID, &a.Identifier, &a.Name, &a.Description,
		&a.AssetType, &a.Status, &a.Owner, &a.PrimaryLocation,
		&a.Confidentiality, &a.Integrity, &a.Availability,
		&a.LastReview, &a.NextReview,
		&a.Notes, &a.CreatedAt, &a.UpdatedAt)
}

func (d *DB) GetAsset(ctx context.Context, orgID int, id int64) (*Asset, error) {
	var a Asset
	err := scanAsset(d.pool.QueryRow(ctx, `
		SELECT `+assetSelectCols+`
		FROM assets WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID), &a)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (d *DB) GetAssetByIdentifier(ctx context.Context, orgID int, identifier string) (*Asset, error) {
	var a Asset
	err := scanAsset(d.pool.QueryRow(ctx, `
		SELECT `+assetSelectCols+`
		FROM assets WHERE identifier = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, identifier, orgID), &a)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (d *DB) ListAssets(ctx context.Context, orgID int) ([]Asset, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT `+assetSelectCols+`
		FROM assets WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY identifier
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	assets := []Asset{}
	for rows.Next() {
		var a Asset
		if err := scanAsset(rows, &a); err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return assets, nil
}

// AssetStats returns counts by status for the org.
func (d *DB) AssetStats(ctx context.Context, orgID int) (*AssetStats, error) {
	var s AssetStats
	err := d.pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE status = 'draft'),
			count(*) FILTER (WHERE status = 'open'),
			count(*) FILTER (WHERE status = 'archived'),
			count(*) FILTER (WHERE confidentiality = 5 OR integrity = 5 OR availability = 5)
		FROM assets
		WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&s.Total, &s.Draft, &s.Open, &s.Archived, &s.Critical)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PaginatedAssets returns a filtered/sorted/paginated slice plus total count.
func (d *DB) PaginatedAssets(ctx context.Context, orgID int, p AssetListParams) ([]Asset, int, error) {
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
	if p.Type != "" {
		where += fmt.Sprintf(` AND asset_type = $%d`, idx)
		args = append(args, p.Type)
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
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM assets`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortDir := "ASC"
	sortKey := strings.TrimPrefix(p.Sort, "-")
	if strings.HasPrefix(p.Sort, "-") {
		sortDir = "DESC"
	}
	sortField, ok := assetSortable[sortKey]
	if !ok {
		sortField = "identifier"
		sortDir = "ASC"
	}

	offset := (p.Page - 1) * p.Limit
	args = append(args, p.Limit, offset)

	query := `SELECT ` + assetSelectCols +
		` FROM assets` + where +
		` ORDER BY ` + sortField + ` ` + sortDir + `, identifier ` + sortDir +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, idx, idx+1)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var assets []Asset
	for rows.Next() {
		var a Asset
		if err := scanAsset(rows, &a); err != nil {
			return nil, 0, err
		}
		assets = append(assets, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if assets == nil {
		assets = []Asset{}
	}
	return assets, total, nil
}

func (d *DB) UpdateAsset(ctx context.Context, orgID int, a *Asset) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE assets SET name = $2, description = $3, asset_type = $4, status = $5,
			owner_id = CASE WHEN $6 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $6) END,
			primary_location = $7,
			confidentiality = $8, integrity = $9, availability = $10,
			last_review = $11, next_review = $12,
			notes = $13, updated_at = now()
		WHERE id = $1 AND organization_id = $14 AND deleted_at IS NULL
	`, a.ID, a.Name, nilIfEmpty(a.Description), a.AssetType, a.Status,
		a.Owner, nilIfEmpty(a.PrimaryLocation),
		a.Confidentiality, a.Integrity, a.Availability,
		a.LastReview, a.NextReview,
		nilIfEmpty(a.Notes), orgID)
	return err
}

func (d *DB) DeleteAsset(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `UPDATE assets SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
	return err
}
