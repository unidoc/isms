package db

import (
	"context"
)

// ReviewAssignment tracks who needs to review a given review request.
type ReviewAssignment struct {
	ID             int    `json:"id"`
	OrganizationID int    `json:"organization_id"`
	ReviewID       int    `json:"review_id"`
	Reviewer       string `json:"reviewer"`
	Status         string `json:"status"`
	ReviewedAt     *Epoch `json:"reviewed_at,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
}

func (d *DB) AddReviewAssignment(ctx context.Context, orgID int, a *ReviewAssignment) error {
	a.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO review_assignments (organization_id, review_id, reviewer_id, status)
		VALUES ($1, $2, (SELECT id FROM users WHERE email = $3), $4)
		RETURNING id, created_at
	`, orgID, a.ReviewID, a.Reviewer, a.Status,
	).Scan(&a.ID, &a.CreatedAt)
}

func (d *DB) ListAssignmentsForReview(ctx context.Context, orgID int, reviewID int) ([]ReviewAssignment, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT ra.id, ra.organization_id, ra.review_id, u.email, ra.status, ra.reviewed_at, ra.created_at
		FROM review_assignments ra JOIN users u ON u.id = ra.reviewer_id
		WHERE ra.organization_id = $1 AND ra.review_id = $2
		ORDER BY ra.created_at
	`, orgID, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []ReviewAssignment
	for rows.Next() {
		var a ReviewAssignment
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.ReviewID, &a.Reviewer, &a.Status, &a.ReviewedAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}

func (d *DB) ListAssignmentsForReviewer(ctx context.Context, orgID int, reviewer string, status string) ([]ReviewAssignment, error) {
	query := `SELECT ra.id, ra.organization_id, ra.review_id, u.email, ra.status, ra.reviewed_at, ra.created_at
		FROM review_assignments ra JOIN users u ON u.id = ra.reviewer_id
		WHERE ra.organization_id = $1 AND u.email = $2`
	args := []interface{}{orgID, reviewer}
	if status != "" {
		query += ` AND ra.status = $3`
		args = append(args, status)
	}
	query += ` ORDER BY ra.created_at DESC`

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []ReviewAssignment
	for rows.Next() {
		var a ReviewAssignment
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.ReviewID, &a.Reviewer, &a.Status, &a.ReviewedAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}

func (d *DB) UpdateAssignmentStatus(ctx context.Context, orgID int, id int, status string) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE review_assignments SET status = $2, reviewed_at = now() WHERE id = $1 AND organization_id = $3
	`, id, status, orgID)
	return err
}

// ResetAssignmentsForReview sets all assignments for a review back to "pending" and clears reviewed_at.
func (d *DB) ResetAssignmentsForReview(ctx context.Context, orgID int, reviewID int) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE review_assignments SET status = 'pending', reviewed_at = NULL WHERE review_id = $1 AND organization_id = $2
	`, reviewID, orgID)
	return err
}
