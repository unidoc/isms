package db

import "context"

// AssetReview records a periodic asset assessment.
type AssetReview struct {
	ID                     int64  `json:"id"`
	OrganizationID         int    `json:"organization_id"`
	AssetID                int64  `json:"asset_id"`
	Outcome                string `json:"outcome"`
	ClassificationVerified bool   `json:"classification_verified"`
	OwnershipVerified      bool   `json:"ownership_verified"`
	Notes                  string `json:"notes,omitempty"`
	ReviewedBy             string `json:"reviewed_by"`
	CreatedAt              Epoch  `json:"created_at"`
}

func (d *DB) CreateAssetReview(ctx context.Context, orgID int, ar *AssetReview) error {
	ar.OrganizationID = orgID
	if ar.Outcome == "" {
		ar.Outcome = "satisfactory"
	}
	err := d.pool.QueryRow(ctx, `
		INSERT INTO asset_reviews (organization_id, asset_id, outcome,
			classification_verified, ownership_verified, notes,
			reviewed_by, reviewed_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, (SELECT id FROM users WHERE email = $7))
		RETURNING id, created_at
	`, orgID, ar.AssetID, ar.Outcome,
		ar.ClassificationVerified, ar.OwnershipVerified,
		nilIfEmpty(ar.Notes), ar.ReviewedBy,
	).Scan(&ar.ID, &ar.CreatedAt)
	if err != nil {
		return err
	}

	// Auto-update asset's last_review.
	asset, err := d.GetAsset(ctx, orgID, ar.AssetID)
	if err == nil {
		now := EpochNow()
		asset.LastReview = &now
		_ = d.UpdateAsset(ctx, orgID, asset)
	}
	return nil
}

func (d *DB) ListAssetReviews(ctx context.Context, orgID int, assetID int64) ([]AssetReview, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, asset_id, outcome,
			classification_verified, ownership_verified,
			COALESCE(notes, ''), reviewed_by, created_at
		FROM asset_reviews
		WHERE organization_id = $1 AND asset_id = $2
		ORDER BY created_at DESC
	`, orgID, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []AssetReview
	for rows.Next() {
		var ar AssetReview
		if err := rows.Scan(&ar.ID, &ar.OrganizationID, &ar.AssetID, &ar.Outcome,
			&ar.ClassificationVerified, &ar.OwnershipVerified,
			&ar.Notes, &ar.ReviewedBy, &ar.CreatedAt); err != nil {
			return nil, err
		}
		reviews = append(reviews, ar)
	}
	return reviews, nil
}
