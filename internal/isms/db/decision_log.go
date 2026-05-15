package db

import (
	"context"
)

// DecisionRecord is an immutable audit record created when a review is approved, merged, or closed.
type DecisionRecord struct {
	ID             int    `json:"id"`
	OrganizationID int    `json:"organization_id"`
	ReviewID       *int   `json:"review_id,omitempty"`
	DocumentID     string `json:"document_id"`
	Decision       string `json:"decision"` // approved, changes_requested, merged, closed
	DecidedBy      string `json:"decided_by"`
	DecidedByID    *int   `json:"decided_by_id,omitempty"`
	CommitRef      string `json:"commit_ref,omitempty"`
	Version        string `json:"version,omitempty"`
	Comment        string `json:"comment,omitempty"`
	ContentHash    string `json:"content_hash,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
}

// CreateDecisionRecord inserts an immutable decision record.
func (d *DB) CreateDecisionRecord(ctx context.Context, orgID int, rec *DecisionRecord) error {
	rec.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO decision_log (organization_id, review_id, document_id, decision, decided_by, decided_by_id, commit_ref, version, comment, content_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`, orgID, rec.ReviewID, rec.DocumentID, rec.Decision, rec.DecidedBy, rec.DecidedByID, rec.CommitRef, rec.Version, rec.Comment, rec.ContentHash,
	).Scan(&rec.ID, &rec.CreatedAt)
}

// ListDecisionRecords returns all decision records for a document, newest first.
func (d *DB) ListDecisionRecords(ctx context.Context, orgID int, documentID string) ([]DecisionRecord, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, review_id, document_id, decision, decided_by, decided_by_id,
		       COALESCE(commit_ref, ''), COALESCE(version, ''), COALESCE(comment, ''), COALESCE(content_hash, ''), created_at
		FROM decision_log WHERE organization_id = $1 AND document_id = $2
		ORDER BY created_at DESC
	`, orgID, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []DecisionRecord
	for rows.Next() {
		var r DecisionRecord
		if err := rows.Scan(&r.ID, &r.OrganizationID, &r.ReviewID, &r.DocumentID, &r.Decision, &r.DecidedBy, &r.DecidedByID,
			&r.CommitRef, &r.Version, &r.Comment, &r.ContentHash, &r.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

// GetReviewDecisions returns all decision records for a specific review, in chronological order.
func (d *DB) GetReviewDecisions(ctx context.Context, orgID int, reviewID int) ([]DecisionRecord, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, review_id, document_id, decision, decided_by, decided_by_id,
		       COALESCE(commit_ref, ''), COALESCE(version, ''), COALESCE(comment, ''), COALESCE(content_hash, ''), created_at
		FROM decision_log WHERE organization_id = $1 AND review_id = $2
		ORDER BY created_at ASC
	`, orgID, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []DecisionRecord
	for rows.Next() {
		var r DecisionRecord
		if err := rows.Scan(&r.ID, &r.OrganizationID, &r.ReviewID, &r.DocumentID, &r.Decision, &r.DecidedBy, &r.DecidedByID,
			&r.CommitRef, &r.Version, &r.Comment, &r.ContentHash, &r.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}
