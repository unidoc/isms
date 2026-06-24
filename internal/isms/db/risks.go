package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RiskListParams specifies filtering, sorting, and pagination for the risk register.
type RiskListParams struct {
	Page     int
	Limit    int
	Sort     string // "title", "-score", "-updated", "identifier"
	Search   string
	Level    string
	Category string
	Status   string
	Owner    string
}

var riskSortable = map[string]string{
	"title":      "title",
	"identifier": "identifier",
	"score":      "COALESCE(current_score, 0)",
	"level":      "current_level",
	"updated":    "updated_at",
}

// Risk represents a single risk entry in the register.
type Risk struct {
	ID             int64  `json:"id"`
	OrganizationID int    `json:"organization_id"`
	Identifier     string `json:"identifier"` // RISK-001
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	RiskType       string `json:"risk_type"`
	Origin         string `json:"origin"`
	Category       string `json:"category,omitempty"`
	// PotentialConsequences was a column; now lives in description (## Potential consequences)

	// Current/residual assessment (nil = not assessed)
	CurrentLikelihood *int   `json:"current_likelihood"`
	CurrentImpact     *int   `json:"current_impact"`
	CurrentScore      *int   `json:"current_score"`
	CurrentLevel      string `json:"current_level"`

	// CIA impact (nil = not assessed)
	ConfidentialityImpact *int `json:"confidentiality_impact"`
	IntegrityImpact       *int `json:"integrity_impact"`
	AvailabilityImpact    *int `json:"availability_impact"`

	// Inherent assessment
	InherentLikelihood            *int `json:"inherent_likelihood,omitempty"`
	InherentImpact                *int `json:"inherent_impact,omitempty"`
	InherentScore                 *int `json:"inherent_score,omitempty"`
	InherentConfidentialityImpact *int `json:"inherent_confidentiality_impact,omitempty"`
	InherentIntegrityImpact       *int `json:"inherent_integrity_impact,omitempty"`
	InherentAvailabilityImpact    *int `json:"inherent_availability_impact,omitempty"`

	// Target assessment
	TargetLikelihood *int   `json:"target_likelihood,omitempty"`
	TargetImpact     *int   `json:"target_impact,omitempty"`
	TargetScore      *int   `json:"target_score,omitempty"`
	TargetLevel      string `json:"target_level,omitempty"`

	// Treatment
	Treatment        string `json:"treatment"`
	TreatmentPlan    string `json:"treatment_plan,omitempty"`
	TreatmentDueDate *Epoch `json:"treatment_due_date,omitempty"`

	// Assessment lifecycle (entity_readings is the canonical assessment log)
	AcceptedAt   *Epoch `json:"accepted_at,omitempty"`
	AcceptedByID *int   `json:"accepted_by_id,omitempty"`

	// Ownership
	Owner      string `json:"owner,omitempty"`
	Status     string `json:"status"`
	LastReview *Epoch `json:"last_review,omitempty"`
	NextReview *Epoch `json:"next_review,omitempty"`
	Notes      string `json:"notes,omitempty"`

	CreatedAt Epoch `json:"created_at"`
	UpdatedAt Epoch `json:"updated_at"`
}

// intVal safely dereferences *int, returning 0 if nil.
func intVal(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// intPtr returns a pointer to an int.
func intPtr(v int) *int { return &v }

// Validate checks required fields.
func (r *Risk) Validate() error {
	if r.Title == "" {
		return fmt.Errorf("title is required")
	}
	if r.CurrentLikelihood != nil && (*r.CurrentLikelihood < 0 || *r.CurrentLikelihood > 5) {
		return fmt.Errorf("current_likelihood must be 0-5")
	}
	if r.CurrentImpact != nil && (*r.CurrentImpact < 0 || *r.CurrentImpact > 5) {
		return fmt.Errorf("current_impact must be 0-5")
	}
	validTypes := map[string]bool{"threat": true, "opportunity": true}
	if r.RiskType == "" || !validTypes[r.RiskType] {
		return fmt.Errorf("risk_type is required (threat or opportunity)")
	}
	validOrigins := map[string]bool{"internal": true, "external": true, "internal and external": true}
	if r.Origin == "" || !validOrigins[r.Origin] {
		return fmt.Errorf("origin is required (internal, external, or internal and external)")
	}
	validStatuses := map[string]bool{"draft": true, "open": true, "closed": true}
	if r.Status == "" || !validStatuses[r.Status] {
		return fmt.Errorf("status is required (draft, open, or closed)")
	}
	return nil
}

// reviewCycleDefaults are fallback months per level if org settings are not loaded.
var reviewCycleDefaults = map[string]int{
	"critical": 1,
	"high":     3,
	"medium":   6,
	"low":      12,
}

// CalculateReviewDate sets next_review based on current_level and review cycle months.
// cycles maps level name → months; pass nil to use built-in defaults.
func (r *Risk) CalculateReviewDate(cycles map[string]int) {
	if r.CurrentLevel == "" {
		return
	}
	if cycles == nil {
		cycles = reviewCycleDefaults
	}
	months, ok := cycles[r.CurrentLevel]
	if !ok {
		months = 12
	}
	base := time.Now()
	if r.LastReview != nil && !r.LastReview.IsZero() {
		base = r.LastReview.Time
	}
	next := base.AddDate(0, months, 0)
	r.NextReview = &Epoch{Time: next}
}

// CalculateScore computes current_score and current_level from current_likelihood and current_impact.
// If either input is nil, output is nil (not assessed).
// cycles maps level name → review months; pass nil to use built-in defaults.
func (r *Risk) CalculateScore(cycles map[string]int) {
	if r.CurrentLikelihood != nil && r.CurrentImpact != nil {
		s := *r.CurrentLikelihood * *r.CurrentImpact
		r.CurrentScore = &s
		r.CurrentLevel = ScoreToLevel(s)
	} else {
		r.CurrentScore = nil
		r.CurrentLevel = ""
	}

	if r.InherentLikelihood != nil && r.InherentImpact != nil {
		s := *r.InherentLikelihood * *r.InherentImpact
		r.InherentScore = &s
	} else {
		r.InherentScore = nil
	}

	if r.TargetLikelihood != nil && r.TargetImpact != nil {
		s := *r.TargetLikelihood * *r.TargetImpact
		r.TargetScore = &s
		r.TargetLevel = ScoreToLevel(s)
	} else {
		r.TargetScore = nil
		r.TargetLevel = ""
	}

	// Auto-calculate review date from current level
	r.CalculateReviewDate(cycles)
}

// ScoreToLevel maps a risk score to its level.
func ScoreToLevel(score int) string {
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

// Valid treatment options (empty string = not decided, maps to NULL in DB).
var TreatmentOptions = []string{"mitigate", "accept", "transfer", "avoid"}

// Valid risk statuses.
var RiskStatuses = []string{"draft", "open", "closed"}

// RiskCategories defines valid risk categories.
var RiskCategories = []string{
	"people_process", "technology", "third_party", "legal_regulatory",
	"physical_environmental", "business_continuity", "governance", "quality_operations",
}

// RiskTypes defines valid risk types.
var RiskTypes = []string{"threat", "opportunity"}

// RiskOrigins defines valid risk origins.
var RiskOrigins = []string{"internal", "external", "internal and external"}

// riskReviewCycles fetches the per-level review cycle settings for an org.
// Returns a map of level → months (e.g. "critical" → 1).
func (d *DB) riskReviewCycles(ctx context.Context, orgID int) map[string]int {
	cycles := make(map[string]int, 4)
	for _, level := range []string{"critical", "high", "medium", "low"} {
		key := "risk_review_cycle_" + level
		if val, err := d.GetOrgSetting(ctx, orgID, key); err == nil && val != "" {
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cycles[level] = n
			}
		}
	}
	if len(cycles) == 0 {
		return nil // use built-in defaults
	}
	return cycles
}

func (d *DB) CreateRisk(ctx context.Context, orgID int, r *Risk) error {
	r.OrganizationID = orgID
	if err := r.Validate(); err != nil {
		return err
	}
	r.CalculateScore(d.riskReviewCycles(ctx, orgID))
	ident, err := d.NextIdentifier(ctx, orgID, "risk")
	if err != nil {
		return err
	}
	r.Identifier = ident
	return d.pool.QueryRow(ctx, `
		INSERT INTO risks (organization_id, identifier, title, description, risk_type, origin, category,
			current_likelihood, current_impact, current_score, current_level,
			confidentiality_impact, integrity_impact, availability_impact,
			inherent_likelihood, inherent_impact, inherent_score,
			inherent_confidentiality_impact, inherent_integrity_impact, inherent_availability_impact,
			target_likelihood, target_impact, target_score, target_level,
			treatment, treatment_plan, treatment_due_date,
			accepted_at, accepted_by_id,
			owner_id, status, last_review, next_review, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16, $17, $18, $19, $20, $21,
			$22, $23, $24,
			$25, $26, $27,
			$28, $29,
			(SELECT id FROM users WHERE email = $30), $31, $32, $33, $34)
		RETURNING id, created_at, updated_at
	`, orgID, r.Identifier, r.Title, nilIfEmpty(r.Description), r.RiskType, r.Origin, nilIfEmpty(r.Category),
		r.CurrentLikelihood, r.CurrentImpact, r.CurrentScore, nilIfEmpty(r.CurrentLevel),
		r.ConfidentialityImpact, r.IntegrityImpact, r.AvailabilityImpact,
		r.InherentLikelihood, r.InherentImpact, r.InherentScore,
		r.InherentConfidentialityImpact, r.InherentIntegrityImpact, r.InherentAvailabilityImpact,
		r.TargetLikelihood, r.TargetImpact, r.TargetScore, nilIfEmpty(r.TargetLevel),
		nilIfEmpty(r.Treatment), nilIfEmpty(r.TreatmentPlan), r.TreatmentDueDate,
		r.AcceptedAt, r.AcceptedByID,
		r.Owner, r.Status, r.LastReview, r.NextReview, nilIfEmpty(r.Notes),
	).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
}

// riskSelectCols centralizes SELECT columns so all risk reads stay in lock-step.
const riskSelectCols = `id, organization_id, identifier, title, COALESCE(description, ''),
		risk_type, origin, COALESCE(category, ''),
		current_likelihood, current_impact, current_score, COALESCE(current_level, ''),
		confidentiality_impact, integrity_impact, availability_impact,
		inherent_likelihood, inherent_impact, inherent_score,
		inherent_confidentiality_impact, inherent_integrity_impact, inherent_availability_impact,
		target_likelihood, target_impact, target_score, COALESCE(target_level, ''),
		COALESCE(treatment, ''), COALESCE(treatment_plan, ''),
		treatment_due_date,
		accepted_at, accepted_by_id,
		COALESCE((SELECT email FROM users WHERE id = risks.owner_id), ''), status, last_review, next_review,
		COALESCE(notes, ''), created_at, updated_at`

func scanRisk(scanner interface {
	Scan(dest ...interface{}) error
}, r *Risk) error {
	return scanner.Scan(&r.ID, &r.OrganizationID, &r.Identifier, &r.Title, &r.Description,
		&r.RiskType, &r.Origin, &r.Category,
		&r.CurrentLikelihood, &r.CurrentImpact, &r.CurrentScore, &r.CurrentLevel,
		&r.ConfidentialityImpact, &r.IntegrityImpact, &r.AvailabilityImpact,
		&r.InherentLikelihood, &r.InherentImpact, &r.InherentScore,
		&r.InherentConfidentialityImpact, &r.InherentIntegrityImpact, &r.InherentAvailabilityImpact,
		&r.TargetLikelihood, &r.TargetImpact, &r.TargetScore, &r.TargetLevel,
		&r.Treatment, &r.TreatmentPlan,
		&r.TreatmentDueDate,
		&r.AcceptedAt, &r.AcceptedByID,
		&r.Owner, &r.Status, &r.LastReview, &r.NextReview,
		&r.Notes, &r.CreatedAt, &r.UpdatedAt)
}

func (d *DB) GetRisk(ctx context.Context, orgID int, id int64) (*Risk, error) {
	var r Risk
	err := scanRisk(d.pool.QueryRow(ctx, `
		SELECT `+riskSelectCols+`
		FROM risks WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID), &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) GetRiskByIdentifier(ctx context.Context, orgID int, identifier string) (*Risk, error) {
	var id int64
	err := d.pool.QueryRow(ctx, `SELECT id FROM risks WHERE organization_id = $1 AND identifier = $2 AND deleted_at IS NULL`, orgID, identifier).Scan(&id)
	if err != nil {
		return nil, err
	}
	return d.GetRisk(ctx, orgID, id)
}

func (d *DB) ListRisks(ctx context.Context, orgID int) ([]Risk, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT `+riskSelectCols+`
		FROM risks WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY COALESCE(current_score, 0) DESC, identifier
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	risks := []Risk{}
	for rows.Next() {
		var r Risk
		if err := scanRisk(rows, &r); err != nil {
			return nil, err
		}
		risks = append(risks, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return risks, nil
}

// RiskStats are aggregate counts across the entire register, independent of pagination.
type RiskStats struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Open     int `json:"open"`
	Closed   int `json:"closed"`
	Draft    int `json:"draft"`
}

// RiskStats returns counts by level and status for the org.
func (d *DB) RiskStats(ctx context.Context, orgID int) (*RiskStats, error) {
	var s RiskStats
	err := d.pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE current_level = 'critical'),
			count(*) FILTER (WHERE current_level = 'high'),
			count(*) FILTER (WHERE current_level = 'medium'),
			count(*) FILTER (WHERE current_level = 'low'),
			count(*) FILTER (WHERE status = 'open'),
			count(*) FILTER (WHERE status = 'closed'),
			count(*) FILTER (WHERE status = 'draft')
		FROM risks
		WHERE organization_id = $1 AND deleted_at IS NULL
	`, orgID).Scan(&s.Total, &s.Critical, &s.High, &s.Medium, &s.Low, &s.Open, &s.Closed, &s.Draft)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PaginatedRisks returns a filtered/sorted/paginated slice of risks plus total count.
func (d *DB) PaginatedRisks(ctx context.Context, orgID int, p RiskListParams) ([]Risk, int, error) {
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
	if p.Owner != "" {
		where += fmt.Sprintf(` AND owner_id = (SELECT id FROM users WHERE email = $%d)`, idx)
		args = append(args, p.Owner)
		idx++
	}

	var total int
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM risks`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortDir := "DESC"
	sortKey := strings.TrimPrefix(p.Sort, "-")
	if !strings.HasPrefix(p.Sort, "-") && p.Sort != "" {
		sortDir = "ASC"
	}
	sortField, ok := riskSortable[sortKey]
	if !ok {
		// default: highest score first
		sortField = "COALESCE(current_score, 0)"
		sortDir = "DESC"
	}

	offset := (p.Page - 1) * p.Limit
	args = append(args, p.Limit, offset)

	query := `SELECT ` + riskSelectCols +
		` FROM risks` + where +
		` ORDER BY ` + sortField + ` ` + sortDir + `, identifier ` + sortDir +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, idx, idx+1)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var risks []Risk
	for rows.Next() {
		var r Risk
		if err := scanRisk(rows, &r); err != nil {
			return nil, 0, err
		}
		risks = append(risks, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if risks == nil {
		risks = []Risk{}
	}
	return risks, total, nil
}

func (d *DB) UpdateRisk(ctx context.Context, orgID int, r *Risk) error {
	r.CalculateScore(d.riskReviewCycles(ctx, orgID))
	_, err := d.pool.Exec(ctx, `
		UPDATE risks SET title = $2, description = $3, risk_type = $4, origin = $5, category = $6,
			current_likelihood = $7, current_impact = $8, current_score = $9, current_level = $10,
			confidentiality_impact = $11, integrity_impact = $12, availability_impact = $13,
			inherent_likelihood = $14, inherent_impact = $15, inherent_score = $16,
			inherent_confidentiality_impact = $17, inherent_integrity_impact = $18, inherent_availability_impact = $19,
			target_likelihood = $20, target_impact = $21, target_score = $22, target_level = $23,
			treatment = $24, treatment_plan = $25, treatment_due_date = $26,
			accepted_at = $27, accepted_by_id = $28,
			owner_id = (SELECT id FROM users WHERE email = $29), status = $30, last_review = $31, next_review = $32,
			notes = $33, updated_at = now()
		WHERE id = $1 AND organization_id = $34 AND deleted_at IS NULL
	`, r.ID, r.Title, nilIfEmpty(r.Description), r.RiskType, r.Origin, nilIfEmpty(r.Category),
		r.CurrentLikelihood, r.CurrentImpact, r.CurrentScore, nilIfEmpty(r.CurrentLevel),
		r.ConfidentialityImpact, r.IntegrityImpact, r.AvailabilityImpact,
		r.InherentLikelihood, r.InherentImpact, r.InherentScore,
		r.InherentConfidentialityImpact, r.InherentIntegrityImpact, r.InherentAvailabilityImpact,
		r.TargetLikelihood, r.TargetImpact, r.TargetScore, nilIfEmpty(r.TargetLevel),
		nilIfEmpty(r.Treatment), nilIfEmpty(r.TreatmentPlan), r.TreatmentDueDate,
		r.AcceptedAt, r.AcceptedByID,
		nilIfEmpty(r.Owner), r.Status, r.LastReview, r.NextReview,
		nilIfEmpty(r.Notes), orgID)
	return err
}

func (d *DB) DeleteRisk(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `UPDATE risks SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
	return err
}
