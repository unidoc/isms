package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Allowed enum values for legal requirement fields. Mirrors schema CHECK constraints.
var (
	LegalStatuses   = []string{"draft", "open", "closed"}
	LegalTreatments = []string{"mitigate", "accept", "transfer", "avoid"}
	LegalCategories = []string{"privacy", "security", "sector", "contractual", "other"}
)

// LegalListParams specifies filtering, sorting, and pagination for the legal register.
type LegalListParams struct {
	Page     int    // 1-indexed; defaults to 1
	Limit    int    // page size; defaults to 50, capped at 200
	Sort     string // field name with optional "-" prefix for descending: "title", "-score", "-updated"
	Search   string // matches title, description, identifier
	Level    string // current_level filter
	Category string
	Status   string
}

// legalSortable maps client-facing sort keys to actual SQL expressions (whitelist to prevent injection).
var legalSortable = map[string]string{
	"title":      "title",
	"identifier": "identifier",
	"score":      "COALESCE(current_score, 0)",
	"level":      "current_level",
	"updated":    "updated_at",
}

// LegalRequirement represents an entry in the legal register.
type LegalRequirement struct {
	ID             int64  `json:"id"`
	Identifier     string `json:"identifier"`
	OrganizationID int    `json:"organization_id"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	Jurisdiction   string `json:"jurisdiction"`
	Category       string `json:"category"`
	Reference      string `json:"reference,omitempty"`
	URL            string `json:"url,omitempty"`
	Status         string `json:"status"`
	Owner          string `json:"owner,omitempty"`
	LastReview     *Epoch `json:"last_review,omitempty"`
	NextReview     *Epoch `json:"next_review,omitempty"`
	Notes          string `json:"notes,omitempty"`
	// Risk assessment (nil = not assessed)
	CurrentLikelihood *int   `json:"current_likelihood"`
	CurrentImpact     *int   `json:"current_impact"`
	CurrentScore      *int   `json:"current_score"`
	CurrentLevel      string `json:"current_level"`
	Treatment         string `json:"treatment"`
	TreatmentPlan     string `json:"treatment_plan,omitempty"`
	TargetLikelihood  *int   `json:"target_likelihood,omitempty"`
	TargetImpact      *int   `json:"target_impact,omitempty"`
	Completion        int    `json:"completion"`
	// Assessment lifecycle: see entity_readings table for the canonical assessment log
	CreatedAt Epoch `json:"created_at"`
	UpdatedAt Epoch `json:"updated_at"`
}

// CalculateReviewDate sets next_review based on current_level (risk-driven).
// When level is unknown, defaults to 12 months from now.
// This is just a default suggestion — users can always override the date manually.
// cycles maps level name → months; pass nil to use built-in defaults from reviewCycleDefaults.
func (lr *LegalRequirement) CalculateReviewDate(cycles map[string]int) {
	if cycles == nil {
		cycles = reviewCycleDefaults
	}
	months := 12
	if lr.CurrentLevel != "" {
		if m, ok := cycles[lr.CurrentLevel]; ok {
			months = m
		}
	}
	next := time.Now().AddDate(0, months, 0)
	lr.NextReview = &Epoch{Time: next}
}

// CalculateRiskScore computes current_score and current_level for a legal requirement.
// If either input is nil, output is nil (not assessed).
// cycles maps level name → review months; pass nil to use built-in defaults.
func (lr *LegalRequirement) CalculateRiskScore(cycles map[string]int) {
	if lr.CurrentLikelihood != nil && lr.CurrentImpact != nil {
		s := *lr.CurrentLikelihood * *lr.CurrentImpact
		lr.CurrentScore = &s
		lr.CurrentLevel = legalScoreToLevel(s)
	} else {
		lr.CurrentScore = nil
		lr.CurrentLevel = ""
	}

	// Auto-calculate review date from current level
	lr.CalculateReviewDate(cycles)
}

func legalScoreToLevel(score int) string {
	switch {
	case score >= 16:
		return "critical"
	case score >= 10:
		return "high"
	case score >= 5:
		return "medium"
	default:
		return "low"
	}
}

// columns shared across INSERT/SELECT
const legalCols = `title, description, jurisdiction, category,
	reference, url, status, owner_id,
	last_review, next_review, notes,
	current_likelihood, current_impact, current_score, current_level, treatment, treatment_plan,
	target_likelihood, target_impact, completion`

const legalSelectCols = `id, identifier, organization_id, title, COALESCE(description, ''), jurisdiction, category,
	COALESCE(reference, ''), COALESCE(url, ''),
	status,
	COALESCE((SELECT email FROM users WHERE id = legal_requirements.owner_id), ''),
	last_review, next_review,
	COALESCE(notes, ''),
	current_likelihood, current_impact, current_score, COALESCE(current_level, ''),
	COALESCE(treatment, ''), COALESCE(treatment_plan, ''),
	target_likelihood, target_impact, COALESCE(completion, 0),
	created_at, updated_at`

func scanLegal(scanner interface {
	Scan(dest ...interface{}) error
}, lr *LegalRequirement) error {
	return scanner.Scan(
		&lr.ID, &lr.Identifier, &lr.OrganizationID, &lr.Title, &lr.Description,
		&lr.Jurisdiction, &lr.Category, &lr.Reference, &lr.URL,
		&lr.Status, &lr.Owner,
		&lr.LastReview, &lr.NextReview, &lr.Notes,
		&lr.CurrentLikelihood, &lr.CurrentImpact, &lr.CurrentScore, &lr.CurrentLevel,
		&lr.Treatment, &lr.TreatmentPlan,
		&lr.TargetLikelihood, &lr.TargetImpact, &lr.Completion,
		&lr.CreatedAt, &lr.UpdatedAt,
	)
}

func (d *DB) CreateLegalRequirement(ctx context.Context, orgID int, lr *LegalRequirement) error {
	lr.OrganizationID = orgID
	lr.CalculateRiskScore(d.riskReviewCycles(ctx, orgID))
	ident, err := d.NextIdentifier(ctx, orgID, "legal_requirement")
	if err != nil {
		return err
	}
	lr.Identifier = ident
	// columns: title($3), description($4), jurisdiction($5), category($6),
	//   reference($7), url($8), status($9),
	//   owner_id ← email $10,
	//   last_review($11), next_review($12), notes($13),
	//   current_likelihood($14), current_impact($15), current_score($16), current_level($17),
	//   treatment($18), treatment_plan($19),
	//   target_likelihood($20), target_impact($21), completion($22)
	return d.pool.QueryRow(ctx, `
		INSERT INTO legal_requirements (organization_id, identifier, `+legalCols+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
			CASE WHEN $10 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $10) END,
			$11, $12, $13,
			$14, $15, $16, $17, $18, $19,
			$20, $21, $22)
		RETURNING id, created_at, updated_at
	`, orgID, lr.Identifier, lr.Title, nilIfEmpty(lr.Description), lr.Jurisdiction, lr.Category,
		nilIfEmpty(lr.Reference), nilIfEmpty(lr.URL),
		lr.Status,
		lr.Owner,
		lr.LastReview, lr.NextReview,
		nilIfEmpty(lr.Notes),
		lr.CurrentLikelihood, lr.CurrentImpact, lr.CurrentScore, nilIfEmpty(lr.CurrentLevel), nilIfEmpty(lr.Treatment), nilIfEmpty(lr.TreatmentPlan),
		lr.TargetLikelihood, lr.TargetImpact, lr.Completion,
	).Scan(&lr.ID, &lr.CreatedAt, &lr.UpdatedAt)
}

func (d *DB) GetLegalRequirement(ctx context.Context, orgID int, id int64) (*LegalRequirement, error) {
	var lr LegalRequirement
	err := scanLegal(d.pool.QueryRow(ctx, `
		SELECT `+legalSelectCols+`
		FROM legal_requirements WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID), &lr)
	if err != nil {
		return nil, err
	}
	return &lr, nil
}

func (d *DB) ListLegalRequirements(ctx context.Context, orgID int, _ string) ([]LegalRequirement, error) {
	query := `SELECT ` + legalSelectCols + `
		FROM legal_requirements WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY title ASC`

	rows, err := d.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []LegalRequirement{}
	for rows.Next() {
		var lr LegalRequirement
		if err := scanLegal(rows, &lr); err != nil {
			return nil, err
		}
		items = append(items, lr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// LegalStats are aggregate counts across the entire register, independent of pagination.
type LegalStats struct {
	Total       int `json:"total"`
	Critical    int `json:"critical"`
	High        int `json:"high"`
	Medium      int `json:"medium"`
	Low         int `json:"low"`
	NotAssessed int `json:"not_assessed"`
	Open        int `json:"open"`
	Closed      int `json:"closed"`
	Draft       int `json:"draft"`
}

// LegalStats returns counts by level and status for the org.
func (d *DB) LegalStats(ctx context.Context, orgID int) (*LegalStats, error) {
	var s LegalStats
	err := d.pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE current_level = 'critical'),
			count(*) FILTER (WHERE current_level = 'high'),
			count(*) FILTER (WHERE current_level = 'medium'),
			count(*) FILTER (WHERE current_level = 'low'),
			count(*) FILTER (WHERE current_score IS NULL OR current_score = 0),
			count(*) FILTER (WHERE status = 'open'),
			count(*) FILTER (WHERE status = 'closed'),
			count(*) FILTER (WHERE status = 'draft')
		FROM legal_requirements
		WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&s.Total, &s.Critical, &s.High, &s.Medium, &s.Low, &s.NotAssessed, &s.Open, &s.Closed, &s.Draft)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PaginatedLegalRequirements returns a filtered/sorted/paginated slice of legal requirements
// along with the total matching count (before pagination).
func (d *DB) PaginatedLegalRequirements(ctx context.Context, orgID int, p LegalListParams) ([]LegalRequirement, int, error) {
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
		where += fmt.Sprintf(` AND (title ILIKE $%d OR COALESCE(description,'') ILIKE $%d OR identifier ILIKE $%d)`, idx, idx, idx)
		args = append(args, "%"+p.Search+"%")
		idx++
	}
	if p.Level != "" {
		where += fmt.Sprintf(` AND current_level = $%d`, idx)
		args = append(args, p.Level)
		idx++
	}
	if p.Category != "" {
		where += fmt.Sprintf(` AND category = $%d`, idx)
		args = append(args, p.Category)
		idx++
	}
	if p.Status != "" {
		where += fmt.Sprintf(` AND status = $%d`, idx)
		args = append(args, p.Status)
		idx++
	}

	var total int
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM legal_requirements`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortDir := "ASC"
	sortKey := strings.TrimPrefix(p.Sort, "-")
	if strings.HasPrefix(p.Sort, "-") {
		sortDir = "DESC"
	}
	sortField, ok := legalSortable[sortKey]
	if !ok {
		sortField = "title"
		sortDir = "ASC"
	}

	offset := (p.Page - 1) * p.Limit
	args = append(args, p.Limit, offset)

	query := `SELECT ` + legalSelectCols + ` FROM legal_requirements` + where +
		` ORDER BY ` + sortField + ` ` + sortDir + `, identifier ` + sortDir +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, idx, idx+1)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []LegalRequirement
	for rows.Next() {
		var lr LegalRequirement
		if err := scanLegal(rows, &lr); err != nil {
			return nil, 0, err
		}
		items = append(items, lr)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if items == nil {
		items = []LegalRequirement{}
	}
	return items, total, nil
}

func (d *DB) GetLegalRequirementByIdentifier(ctx context.Context, orgID int, identifier string) (*LegalRequirement, error) {
	var id int64
	err := d.pool.QueryRow(ctx, `SELECT id FROM legal_requirements WHERE organization_id = $1 AND identifier = $2 AND deleted_at IS NULL`, orgID, identifier).Scan(&id)
	if err != nil {
		return nil, err
	}
	return d.GetLegalRequirement(ctx, orgID, id)
}

func (d *DB) UpdateLegalRequirement(ctx context.Context, orgID int, lr *LegalRequirement) error {
	lr.CalculateRiskScore(d.riskReviewCycles(ctx, orgID))
	_, err := d.pool.Exec(ctx, `
		UPDATE legal_requirements SET title = $2, description = $3, jurisdiction = $4, category = $5,
			reference = $6, url = $7,
			status = $8,
			owner_id = CASE WHEN $9 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $9) END,
			last_review = $10, next_review = $11, notes = $12,
			current_likelihood = $13, current_impact = $14, current_score = $15, current_level = $16, treatment = $17, treatment_plan = $18,
			target_likelihood = $19, target_impact = $20, completion = $21,
			updated_at = now()
		WHERE id = $1 AND organization_id = $22 AND deleted_at IS NULL
	`, lr.ID, lr.Title, nilIfEmpty(lr.Description), lr.Jurisdiction, lr.Category,
		nilIfEmpty(lr.Reference), nilIfEmpty(lr.URL),
		lr.Status,
		lr.Owner,
		lr.LastReview, lr.NextReview,
		nilIfEmpty(lr.Notes),
		lr.CurrentLikelihood, lr.CurrentImpact, lr.CurrentScore, nilIfEmpty(lr.CurrentLevel), nilIfEmpty(lr.Treatment), nilIfEmpty(lr.TreatmentPlan),
		lr.TargetLikelihood, lr.TargetImpact, lr.Completion,
		orgID)
	return err
}

func (d *DB) DeleteLegalRequirement(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `UPDATE legal_requirements SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
	return err
}
