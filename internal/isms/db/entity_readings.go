package db

import (
	"context"
	"time"
)

// EntityReading represents a periodic assessment record for a risk, legal requirement, or supplier.
type EntityReading struct {
	ID                int64  `json:"id"`
	OrganizationID    int    `json:"organization_id"`
	EntityType        string `json:"entity_type"`
	EntityID          int64  `json:"entity_id"`
	CurrentLikelihood *int   `json:"current_likelihood"`
	CurrentImpact     *int   `json:"current_impact"`
	Confidentiality   *int   `json:"confidentiality"`
	Integrity         *int   `json:"integrity"`
	Availability      *int   `json:"availability"`
	Status            string `json:"status,omitempty"`
	Treatment         string `json:"treatment,omitempty"`
	Notes             string `json:"notes,omitempty"`
	AssessedBy        string `json:"assessed_by"`
	AssessedByUserID  *int   `json:"assessed_by_user_id,omitempty"`
	CreatedAt         Epoch  `json:"created_at"`
}

func (d *DB) CreateEntityReading(ctx context.Context, orgID int, r *EntityReading) error {
	r.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO entity_readings (organization_id, entity_type, entity_id,
			current_likelihood, current_impact, confidentiality, integrity, availability,
			status, treatment, notes,
			assessed_by, assessed_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, (SELECT id FROM users WHERE email = $12))
		RETURNING id, created_at
	`, orgID, r.EntityType, r.EntityID,
		r.CurrentLikelihood, r.CurrentImpact, r.Confidentiality, r.Integrity, r.Availability,
		nilIfEmpty(r.Status), nilIfEmpty(r.Treatment), nilIfEmpty(r.Notes),
		r.AssessedBy,
	).Scan(&r.ID, &r.CreatedAt)
}

// CreateEntityReadingAt inserts a reading with an explicit created_at, the
// same backdating pattern LogActivityAt uses — for seeding demo/historical
// data where a reading needs to land on a specific past date rather than
// now(), e.g. so a risk's assessment trend spans real calendar time instead
// of landing in a single microsecond-apart burst at seed time.
func (d *DB) CreateEntityReadingAt(ctx context.Context, orgID int, r *EntityReading, at time.Time) error {
	r.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO entity_readings (organization_id, entity_type, entity_id,
			current_likelihood, current_impact, confidentiality, integrity, availability,
			status, treatment, notes,
			assessed_by, assessed_by_user_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, (SELECT id FROM users WHERE email = $12), $13)
		RETURNING id, created_at
	`, orgID, r.EntityType, r.EntityID,
		r.CurrentLikelihood, r.CurrentImpact, r.Confidentiality, r.Integrity, r.Availability,
		nilIfEmpty(r.Status), nilIfEmpty(r.Treatment), nilIfEmpty(r.Notes),
		r.AssessedBy, at,
	).Scan(&r.ID, &r.CreatedAt)
}

func (d *DB) ListEntityReadings(ctx context.Context, orgID int, entityType string, entityID int64) ([]EntityReading, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, entity_type, entity_id,
			current_likelihood, current_impact, confidentiality, integrity, availability,
			COALESCE(status, ''), COALESCE(treatment, ''),
			COALESCE(notes, ''), assessed_by, assessed_by_user_id, created_at
		FROM entity_readings
		WHERE organization_id = $1 AND entity_type = $2 AND entity_id = $3
		ORDER BY created_at DESC
	`, orgID, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var readings []EntityReading
	for rows.Next() {
		var r EntityReading
		if err := rows.Scan(&r.ID, &r.OrganizationID, &r.EntityType, &r.EntityID,
			&r.CurrentLikelihood, &r.CurrentImpact, &r.Confidentiality, &r.Integrity, &r.Availability,
			&r.Status, &r.Treatment,
			&r.Notes, &r.AssessedBy, &r.AssessedByUserID, &r.CreatedAt); err != nil {
			return nil, err
		}
		readings = append(readings, r)
	}
	return readings, nil
}

func (d *DB) GetEntityReading(ctx context.Context, orgID int, id int64) (*EntityReading, error) {
	var r EntityReading
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, entity_type, entity_id,
			current_likelihood, current_impact, confidentiality, integrity, availability,
			COALESCE(status, ''), COALESCE(treatment, ''),
			COALESCE(notes, ''), assessed_by, assessed_by_user_id, created_at
		FROM entity_readings
		WHERE id = $1 AND organization_id = $2
	`, id, orgID).Scan(&r.ID, &r.OrganizationID, &r.EntityType, &r.EntityID,
		&r.CurrentLikelihood, &r.CurrentImpact, &r.Confidentiality, &r.Integrity, &r.Availability,
		&r.Status, &r.Treatment,
		&r.Notes, &r.AssessedBy, &r.AssessedByUserID, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}
