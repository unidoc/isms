package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Suggestion represents a proposed change to an operational entity.
type Suggestion struct {
	ID              int64           `json:"id"`
	OrganizationID  int             `json:"organization_id"`
	EntityType      string          `json:"entity_type"`
	EntityID        string          `json:"entity_id,omitempty"`
	SuggestionType  string          `json:"suggestion_type"`
	Title           string          `json:"title"`
	Payload         json.RawMessage `json:"payload"`
	Rationale       string          `json:"rationale,omitempty"`
	SourceRefs      json.RawMessage `json:"source_refs,omitempty"`
	EntityUpdatedAt *Epoch          `json:"entity_updated_at,omitempty"`
	Status          string          `json:"status"`
	SuggestedBy     string          `json:"suggested_by"`
	SuggestedByType string          `json:"suggested_by_type"`
	ReviewedBy      string          `json:"reviewed_by,omitempty"`
	ReviewedAt      *Epoch          `json:"reviewed_at,omitempty"`
	AppliedAt       *Epoch          `json:"applied_at,omitempty"`
	AppliedEntityID string          `json:"applied_entity_id,omitempty"`
	RejectReason    string          `json:"reject_reason,omitempty"`
	CreatedAt       Epoch           `json:"created_at"`
	UpdatedAt       Epoch           `json:"updated_at"`
}

const suggestionSelectCols = `
	id, organization_id, entity_type, COALESCE(entity_id, ''),
	suggestion_type, title, payload, COALESCE(rationale, ''),
	source_refs, entity_updated_at,
	status, suggested_by, suggested_by_type,
	COALESCE(reviewed_by, ''), reviewed_at, applied_at,
	COALESCE(applied_entity_id, ''), COALESCE(reject_reason, ''),
	created_at, updated_at`

func scanSuggestion(scanner interface {
	Scan(dest ...interface{}) error
}, s *Suggestion) error {
	return scanner.Scan(
		&s.ID, &s.OrganizationID, &s.EntityType, &s.EntityID,
		&s.SuggestionType, &s.Title, &s.Payload, &s.Rationale,
		&s.SourceRefs, &s.EntityUpdatedAt,
		&s.Status, &s.SuggestedBy, &s.SuggestedByType,
		&s.ReviewedBy, &s.ReviewedAt, &s.AppliedAt,
		&s.AppliedEntityID, &s.RejectReason,
		&s.CreatedAt, &s.UpdatedAt,
	)
}

func (d *DB) CreateSuggestion(ctx context.Context, orgID int, s *Suggestion) error {
	s.OrganizationID = orgID
	if s.Status == "" {
		s.Status = "open"
	}
	if s.SuggestedByType == "" {
		s.SuggestedByType = "user"
	}
	if s.Payload == nil {
		s.Payload = json.RawMessage(`{}`)
	}
	return d.pool.QueryRow(ctx, `
		INSERT INTO suggestions (
			organization_id, entity_type, entity_id, suggestion_type,
			title, payload, rationale, source_refs, entity_updated_at,
			status, suggested_by, suggested_by_user_id, suggested_by_type
		) VALUES (
			$1, $2, NULLIF($3, ''), $4,
			$5, $6, NULLIF($7, ''), $8, $9,
			$10, $11, (SELECT id FROM users WHERE email = $11), $12
		)
		RETURNING id, created_at, updated_at
	`, orgID, s.EntityType, s.EntityID, s.SuggestionType,
		s.Title, s.Payload, s.Rationale, s.SourceRefs, s.EntityUpdatedAt,
		s.Status, s.SuggestedBy, s.SuggestedByType,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

func (d *DB) GetSuggestion(ctx context.Context, orgID int, id int64) (*Suggestion, error) {
	var s Suggestion
	err := d.pool.QueryRow(ctx,
		`SELECT `+suggestionSelectCols+` FROM suggestions WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	).Scan(
		&s.ID, &s.OrganizationID, &s.EntityType, &s.EntityID,
		&s.SuggestionType, &s.Title, &s.Payload, &s.Rationale,
		&s.SourceRefs, &s.EntityUpdatedAt,
		&s.Status, &s.SuggestedBy, &s.SuggestedByType,
		&s.ReviewedBy, &s.ReviewedAt, &s.AppliedAt,
		&s.AppliedEntityID, &s.RejectReason,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSuggestions returns suggestions with optional filters.
func (d *DB) ListSuggestions(ctx context.Context, orgID int, filters SuggestionFilters) ([]Suggestion, error) {
	query := `SELECT ` + suggestionSelectCols + ` FROM suggestions WHERE organization_id = $1`
	args := []interface{}{orgID}
	n := 1

	if filters.Status != "" {
		n++
		query += fmt.Sprintf(` AND status = $%d`, n)
		args = append(args, filters.Status)
	}
	if filters.EntityType != "" {
		n++
		query += fmt.Sprintf(` AND entity_type = $%d`, n)
		args = append(args, filters.EntityType)
	}
	if filters.EntityID != "" {
		n++
		query += fmt.Sprintf(` AND entity_id = $%d`, n)
		args = append(args, filters.EntityID)
	}
	if filters.SuggestedBy != "" {
		n++
		query += fmt.Sprintf(` AND suggested_by = $%d`, n)
		args = append(args, filters.SuggestedBy)
	}
	if filters.SuggestedByType != "" {
		n++
		query += fmt.Sprintf(` AND suggested_by_type = $%d`, n)
		args = append(args, filters.SuggestedByType)
	}

	query += ` ORDER BY created_at DESC`
	if filters.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, filters.Limit)
	} else {
		query += ` LIMIT 100`
	}

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suggestions []Suggestion
	for rows.Next() {
		var s Suggestion
		if err := scanSuggestion(rows, &s); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, s)
	}
	return suggestions, nil
}

type SuggestionFilters struct {
	Status          string
	EntityType      string
	EntityID        string
	SuggestedBy     string
	SuggestedByType string
	Limit           int
}

// UpdateSuggestion updates editable fields on an open or in_review suggestion.
func (d *DB) UpdateSuggestion(ctx context.Context, orgID int, s *Suggestion) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE suggestions SET
			title = $2, payload = $3, rationale = NULLIF($4, ''),
			source_refs = $5, updated_at = now()
		WHERE id = $1 AND organization_id = $6
			AND status IN ('open', 'in_review')
	`, s.ID, s.Title, s.Payload, s.Rationale, s.SourceRefs, orgID)
	return err
}

// DeleteSuggestion hard-deletes a non-terminal suggestion.
func (d *DB) DeleteSuggestion(ctx context.Context, orgID int, id int64) error {
	tag, err := d.pool.Exec(ctx, `
		DELETE FROM suggestions
		WHERE id = $1 AND organization_id = $2
			AND status IN ('open', 'in_review', 'withdrawn')
	`, id, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("suggestion not found or in terminal state")
	}
	return nil
}

// ClaimSuggestion toggles between open and in_review.
func (d *DB) ClaimSuggestion(ctx context.Context, orgID int, id int64, reviewerEmail string) (string, error) {
	var newStatus string
	err := d.pool.QueryRow(ctx, `
		UPDATE suggestions SET
			status = CASE status WHEN 'open' THEN 'in_review' WHEN 'in_review' THEN 'open' END,
			reviewed_by = CASE status WHEN 'open' THEN $3 ELSE NULL END,
			reviewed_by_user_id = CASE status WHEN 'open' THEN (SELECT id FROM users WHERE email = $3) ELSE NULL END,
			updated_at = now()
		WHERE id = $1 AND organization_id = $2
			AND status IN ('open', 'in_review')
		RETURNING status
	`, id, orgID, reviewerEmail).Scan(&newStatus)
	if err != nil {
		return "", fmt.Errorf("suggestion not found or not claimable: %w", err)
	}
	return newStatus, nil
}

// ApplySuggestion marks a suggestion as applied (pool-based, used outside WithOrgTx context).
func (d *DB) ApplySuggestion(ctx context.Context, orgID int, id int64, reviewerEmail string, appliedEntityID string) error {
	tag, err := d.pool.Exec(ctx, `
		UPDATE suggestions SET
			status = 'applied',
			reviewed_by = $3,
			reviewed_by_user_id = (SELECT id FROM users WHERE email = $3),
			reviewed_at = now(),
			applied_at = now(),
			applied_entity_id = NULLIF($4, ''),
			updated_at = now()
		WHERE id = $1 AND organization_id = $2
			AND status IN ('open', 'in_review')
	`, id, orgID, reviewerEmail, appliedEntityID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("suggestion not found or already in terminal state")
	}
	return nil
}

// RejectEntitySuggestion marks an entity suggestion as rejected with a reason.
func (d *DB) RejectEntitySuggestion(ctx context.Context, orgID int, id int64, reviewerEmail, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("reject_reason is required")
	}
	tag, err := d.pool.Exec(ctx, `
		UPDATE suggestions SET
			status = 'rejected',
			reviewed_by = $3,
			reviewed_by_user_id = (SELECT id FROM users WHERE email = $3),
			reviewed_at = now(),
			reject_reason = $4,
			updated_at = now()
		WHERE id = $1 AND organization_id = $2
			AND status IN ('open', 'in_review')
	`, id, orgID, reviewerEmail, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("suggestion not found or already in terminal state")
	}
	return nil
}

// WithdrawSuggestion allows the original author to withdraw their suggestion.
func (d *DB) WithdrawSuggestion(ctx context.Context, orgID int, id int64, authorEmail string) error {
	tag, err := d.pool.Exec(ctx, `
		UPDATE suggestions SET
			status = 'withdrawn',
			updated_at = now()
		WHERE id = $1 AND organization_id = $2
			AND status = 'open'
			AND suggested_by = $3
	`, id, orgID, authorEmail)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("suggestion not found, not open, or not yours")
	}
	return nil
}

// tablesWithDeletedAt lists tables that have a deleted_at column.
var tablesWithDeletedAt = map[string]bool{
	"risks":              true,
	"suppliers":          true,
	"assets":             true,
	"systems":            true,
	"legal_requirements": true,
	"incidents":          true,
	"tasks":              true,
	"change_requests":    true,
	"corrective_actions": true,
	"audit_findings":     true,
	"objectives":         true,
	"programs":           true,
}

// GetEntityUpdatedAt returns the updated_at timestamp for an entity, used for stale detection.
func (d *DB) GetEntityUpdatedAt(ctx context.Context, orgID int, entityType, entityID string) *Epoch {
	table := entityTypeToTable(entityType)
	if table == "" {
		return nil
	}
	idCol := "id"
	if entityType == "risk" || entityType == "supplier" {
		idCol = "identifier"
	}
	var updatedAt Epoch
	query := fmt.Sprintf(`SELECT updated_at FROM %s WHERE organization_id = $1 AND %s = $2`, table, idCol)
	if tablesWithDeletedAt[table] {
		query += ` AND deleted_at IS NULL`
	}
	if err := d.pool.QueryRow(ctx, query, orgID, entityID).Scan(&updatedAt); err != nil {
		return nil
	}
	return &updatedAt
}

func entityTypeToTable(entityType string) string {
	switch entityType {
	case "risk":
		return "risks"
	case "supplier":
		return "suppliers"
	case "incident":
		return "incidents"
	case "legal_requirement":
		return "legal_requirements"
	case "change_request":
		return "change_requests"
	case "corrective_action":
		return "corrective_actions"
	case "objective":
		return "objectives"
	case "task":
		return "tasks"
	case "system":
		return "systems"
	case "asset":
		return "assets"
	case "audit_finding":
		return "audit_findings"
	case "program":
		return "programs"
	case "checkin":
		return "checkins"
	case "access_review":
		return "access_reviews"
	default:
		return ""
	}
}

// EntityChangesAfter returns changelog entries for an entity after a given time.
// Used for stale detection on suggestions.
func (d *DB) EntityChangesAfter(ctx context.Context, orgID int, entityType string, entityID int64, after Epoch) ([]ChangelogEntry, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, entity_type, entity_id, action,
			COALESCE(field, ''), old_value, new_value,
			changed_by, api_key_id, COALESCE(reason, ''), created_at
		FROM entity_changelog
		WHERE organization_id = $1 AND entity_type = $2 AND entity_id = $3
			AND created_at > $4
		ORDER BY created_at ASC
	`, orgID, entityType, entityID, after.Time)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanChangelog(rows)
}
