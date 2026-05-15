package db

import (
	"context"
)

// AccessReview represents a periodic access review for a system.
type AccessReview struct {
	ID             int64     `json:"id"`
	OrganizationID int       `json:"organization_id"`
	SystemID       int64     `json:"system_id"`
	ReviewedAt     Epoch  `json:"reviewed_at"`
	ReviewedBy     string `json:"reviewed_by"`
	UsersAdded     int    `json:"users_added"`
	UsersRemoved   int    `json:"users_removed"`
	Notes          string `json:"notes,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
}

func (d *DB) CreateAccessReview(ctx context.Context, orgID int, ar *AccessReview) error {
	ar.OrganizationID = orgID
	err := d.pool.QueryRow(ctx, `
		INSERT INTO access_reviews (organization_id, system_id, reviewed_at, reviewed_by, reviewed_by_user_id,
			users_added, users_removed, notes)
		VALUES ($1, $2, $3, $4, (SELECT id FROM users WHERE email = $4), $5, $6, $7)
		RETURNING id, created_at
	`, orgID, ar.SystemID, ar.ReviewedAt, ar.ReviewedBy,
		ar.UsersAdded, ar.UsersRemoved, nilIfEmpty(ar.Notes),
	).Scan(&ar.ID, &ar.CreatedAt)
	if err != nil {
		return err
	}

	// Auto-update parent system's last_review and next_review.
	sys, err := d.GetSystem(ctx, orgID, ar.SystemID)
	if err == nil {
		sys.LastReview = &ar.ReviewedAt
		sys.CalculateNextReview()
		_ = d.UpdateSystem(ctx, orgID, sys)
	}
	return nil
}

func (d *DB) ListAccessReviews(ctx context.Context, orgID int, systemID int64) ([]AccessReview, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, system_id, reviewed_at, reviewed_by,
			users_added, users_removed, COALESCE(notes, ''), created_at
		FROM access_reviews
		WHERE organization_id = $1 AND system_id = $2
		ORDER BY reviewed_at DESC
	`, orgID, systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []AccessReview
	for rows.Next() {
		var ar AccessReview
		if err := rows.Scan(&ar.ID, &ar.OrganizationID, &ar.SystemID,
			&ar.ReviewedAt, &ar.ReviewedBy,
			&ar.UsersAdded, &ar.UsersRemoved, &ar.Notes, &ar.CreatedAt); err != nil {
			return nil, err
		}
		reviews = append(reviews, ar)
	}
	return reviews, nil
}

func (d *DB) DeleteAccessReview(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM access_reviews WHERE id = $1 AND organization_id = $2`, id, orgID)
	return err
}
