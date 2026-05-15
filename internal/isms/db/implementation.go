package db

import (
	"context"
	"fmt"
)

// ImplementationStatus tracks the implementation state of an ISMS item.
type ImplementationStatus struct {
	ID             int        `json:"id"`
	OrganizationID int        `json:"organization_id"`
	ItemID         string     `json:"item_id"`
	ItemType       string     `json:"item_type"`
	Status         string     `json:"status"`
	Owner          string     `json:"owner,omitempty"`
	TargetDate     *Epoch `json:"target_date,omitempty"`
	Notes          string `json:"notes,omitempty"`
	UpdatedAt      Epoch  `json:"updated_at"`
}

func (d *DB) UpsertImplementationStatus(ctx context.Context, orgID int, s *ImplementationStatus) error {
	s.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO implementation_status (organization_id, item_id, item_type, status, owner_id, target_date, notes)
		VALUES ($1, $2, $3, $4,
			CASE WHEN $5 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $5) END,
			$6, $7)
		ON CONFLICT (organization_id, item_type, item_id) DO UPDATE SET
			status = EXCLUDED.status,
			owner_id = EXCLUDED.owner_id,
			target_date = EXCLUDED.target_date,
			notes = EXCLUDED.notes,
			updated_at = now()
		RETURNING id, updated_at
	`, orgID, s.ItemID, s.ItemType, s.Status, s.Owner, s.TargetDate, nilIfEmpty(s.Notes),
	).Scan(&s.ID, &s.UpdatedAt)
}

func (d *DB) GetImplementationStatus(ctx context.Context, orgID int, itemID string) (*ImplementationStatus, error) {
	var s ImplementationStatus
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, item_id, item_type, status, COALESCE((SELECT email FROM users WHERE id = implementation_status.owner_id), ''), target_date, COALESCE(notes, ''), updated_at
		FROM implementation_status WHERE organization_id = $1 AND item_id = $2
	`, orgID, itemID).Scan(&s.ID, &s.OrganizationID, &s.ItemID, &s.ItemType, &s.Status, &s.Owner, &s.TargetDate, &s.Notes, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *DB) ListImplementationStatus(ctx context.Context, orgID int, itemType, status string) ([]ImplementationStatus, error) {
	query := `SELECT id, organization_id, item_id, item_type, status, COALESCE((SELECT email FROM users WHERE id = implementation_status.owner_id), ''), target_date, COALESCE(notes, ''), updated_at
		FROM implementation_status WHERE organization_id = $1`
	args := []interface{}{orgID}
	n := 1
	if itemType != "" {
		n++
		query += fmt.Sprintf(` AND item_type = $%d`, n)
		args = append(args, itemType)
	}
	if status != "" {
		n++
		query += fmt.Sprintf(` AND status = $%d`, n)
		args = append(args, status)
	}
	query += ` ORDER BY item_id`

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ImplementationStatus
	for rows.Next() {
		var s ImplementationStatus
		if err := rows.Scan(&s.ID, &s.OrganizationID, &s.ItemID, &s.ItemType, &s.Status, &s.Owner, &s.TargetDate, &s.Notes, &s.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, nil
}

// ImplementationProgress returns aggregate counts for the progress dashboard.
func (d *DB) ImplementationProgress(ctx context.Context, orgID int) (total, notStarted, inProgress, implemented, verified int, err error) {
	err = d.pool.QueryRow(ctx, `
		SELECT COUNT(*),
			COUNT(*) FILTER (WHERE status = 'not_started'),
			COUNT(*) FILTER (WHERE status = 'in_progress'),
			COUNT(*) FILTER (WHERE status = 'implemented'),
			COUNT(*) FILTER (WHERE status = 'verified')
		FROM implementation_status WHERE organization_id = $1
	`, orgID).Scan(&total, &notStarted, &inProgress, &implemented, &verified)
	return
}

// ImplementationProgressByType returns aggregate counts filtered by item type.
func (d *DB) ImplementationProgressByType(ctx context.Context, orgID int, itemType string) (total, notStarted, inProgress, implemented, verified int, err error) {
	err = d.pool.QueryRow(ctx, `
		SELECT COUNT(*),
			COUNT(*) FILTER (WHERE status = 'not_started'),
			COUNT(*) FILTER (WHERE status = 'in_progress'),
			COUNT(*) FILTER (WHERE status = 'implemented'),
			COUNT(*) FILTER (WHERE status = 'verified')
		FROM implementation_status WHERE organization_id = $1 AND item_type = $2
	`, orgID, itemType).Scan(&total, &notStarted, &inProgress, &implemented, &verified)
	return
}
