package db

import "context"

// SupplierReview records a periodic supplier assessment.
type SupplierReview struct {
	ID                     int64  `json:"id"`
	OrganizationID         int    `json:"organization_id"`
	SupplierID             int64  `json:"supplier_id"`
	Outcome                string `json:"outcome"` // satisfactory, concerns, unsatisfactory
	CertificationsVerified bool   `json:"certifications_verified"`
	DataHandlingVerified   bool   `json:"data_handling_verified"`
	SLAMet                 bool   `json:"sla_met"`
	Notes                  string `json:"notes,omitempty"`
	ReviewedBy             string `json:"reviewed_by"`
	CreatedAt              Epoch  `json:"created_at"`
}

func (d *DB) CreateSupplierReview(ctx context.Context, orgID int, sr *SupplierReview) error {
	sr.OrganizationID = orgID
	if sr.Outcome == "" {
		sr.Outcome = "satisfactory"
	}
	err := d.pool.QueryRow(ctx, `
		INSERT INTO supplier_reviews (organization_id, supplier_id, outcome,
			certifications_verified, data_handling_verified, sla_met, notes,
			reviewed_by, reviewed_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, (SELECT id FROM users WHERE email = $8))
		RETURNING id, created_at
	`, orgID, sr.SupplierID, sr.Outcome,
		sr.CertificationsVerified, sr.DataHandlingVerified, sr.SLAMet,
		nilIfEmpty(sr.Notes), sr.ReviewedBy,
	).Scan(&sr.ID, &sr.CreatedAt)
	if err != nil {
		return err
	}

	// Auto-update supplier's last_review and next_review.
	sup, err := d.GetSupplier(ctx, orgID, sr.SupplierID)
	if err == nil {
		now := EpochNow()
		sup.LastReview = &now
		sup.CalculateNextReview()
		if sr.Outcome == "unsatisfactory" {
			sup.Status = "under_review"
		}
		_ = d.UpdateSupplier(ctx, orgID, sup)
	}
	return nil
}

func (d *DB) ListSupplierReviews(ctx context.Context, orgID int, supplierID int64) ([]SupplierReview, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, supplier_id, outcome,
			certifications_verified, data_handling_verified, sla_met,
			COALESCE(notes, ''), reviewed_by, created_at
		FROM supplier_reviews
		WHERE organization_id = $1 AND supplier_id = $2
		ORDER BY created_at DESC
	`, orgID, supplierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []SupplierReview
	for rows.Next() {
		var sr SupplierReview
		if err := rows.Scan(&sr.ID, &sr.OrganizationID, &sr.SupplierID, &sr.Outcome,
			&sr.CertificationsVerified, &sr.DataHandlingVerified, &sr.SLAMet,
			&sr.Notes, &sr.ReviewedBy, &sr.CreatedAt); err != nil {
			return nil, err
		}
		reviews = append(reviews, sr)
	}
	return reviews, nil
}
