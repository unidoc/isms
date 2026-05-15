package db

import (
	"context"
	"fmt"
)

// ToChangeMap returns the user-editable fields for changelog comparison.
func (c *Checkin) ToChangeMap() map[string]string {
	m := map[string]string{
		"message":     c.Message,
		"public_note": c.PublicNote,
	}
	if c.Success != nil {
		m["success"] = fmt.Sprintf("%v", *c.Success)
	}
	if c.ValueNumeric != nil {
		m["value_numeric"] = fmt.Sprintf("%g", *c.ValueNumeric)
	}
	if !c.OccurredAt.IsZero() {
		m["occurred_at"] = c.OccurredAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return m
}

// Checkin is a time-series measurement for an objective.
type Checkin struct {
	ID             int64      `json:"id"`
	OrganizationID int        `json:"organization_id"`
	ObjectiveID    int64      `json:"objective_id"`
	OccurredAt     Epoch    `json:"occurred_at"`
	RecordedAt     Epoch    `json:"recorded_at"`
	CreatedBy      string   `json:"created_by,omitempty"`
	Success        *bool    `json:"success"`
	ValueNumeric   *float64 `json:"value_numeric,omitempty"`
	Message        string   `json:"message,omitempty"`
	PublicNote     string   `json:"public_note,omitempty"`
	CreatedAt      Epoch    `json:"created_at"`
}

func (d *DB) CreateCheckin(ctx context.Context, orgID int, c *Checkin) error {
	c.OrganizationID = orgID
	if c.OccurredAt.IsZero() {
		c.OccurredAt = EpochNow()
	}
	return d.pool.QueryRow(ctx, `
		INSERT INTO checkins (organization_id, objective_id, occurred_at, recorded_at,
			created_by, created_by_user_id, success, value_numeric, message, public_note)
		VALUES ($1, $2, $3, now(), $4, (SELECT id FROM users WHERE email = $4), $5, $6, $7, $8)
		RETURNING id, recorded_at, created_at
	`, orgID, c.ObjectiveID, c.OccurredAt,
		nilIfEmpty(c.CreatedBy), c.Success, c.ValueNumeric,
		nilIfEmpty(c.Message), nilIfEmpty(c.PublicNote),
	).Scan(&c.ID, &c.RecordedAt, &c.CreatedAt)
}

func (d *DB) GetCheckin(ctx context.Context, orgID int, id int64) (*Checkin, error) {
	var c Checkin
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, objective_id, occurred_at, recorded_at,
			COALESCE(created_by, ''), success, value_numeric,
			COALESCE(message, ''), COALESCE(public_note, ''), created_at
		FROM checkins WHERE id = $1 AND organization_id = $2
	`, id, orgID).Scan(&c.ID, &c.OrganizationID, &c.ObjectiveID, &c.OccurredAt, &c.RecordedAt,
		&c.CreatedBy, &c.Success, &c.ValueNumeric,
		&c.Message, &c.PublicNote, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (d *DB) ListCheckins(ctx context.Context, orgID int, objectiveID int64, limit, offset int) ([]Checkin, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, objective_id, occurred_at, recorded_at,
			COALESCE(created_by, ''), success, value_numeric,
			COALESCE(message, ''), COALESCE(public_note, ''), created_at
		FROM checkins
		WHERE organization_id = $1 AND objective_id = $2
		ORDER BY occurred_at DESC
		LIMIT $3 OFFSET $4
	`, orgID, objectiveID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	checkins := []Checkin{}
	for rows.Next() {
		var c Checkin
		if err := rows.Scan(&c.ID, &c.OrganizationID, &c.ObjectiveID, &c.OccurredAt, &c.RecordedAt,
			&c.CreatedBy, &c.Success, &c.ValueNumeric,
			&c.Message, &c.PublicNote, &c.CreatedAt); err != nil {
			return nil, err
		}
		checkins = append(checkins, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return checkins, nil
}

func (d *DB) UpdateCheckin(ctx context.Context, orgID int, c *Checkin) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE checkins SET
			occurred_at = $2, success = $3, value_numeric = $4,
			message = $5, public_note = $6
		WHERE id = $1 AND organization_id = $7
	`, c.ID, c.OccurredAt, c.Success, c.ValueNumeric,
		nilIfEmpty(c.Message), nilIfEmpty(c.PublicNote), orgID)
	return err
}

func (d *DB) DeleteCheckin(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM checkins WHERE id = $1 AND organization_id = $2`, id, orgID)
	return err
}
