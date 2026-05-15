package db

import (
	"context"
	"fmt"
)

// DocumentVersion records a snapshot of a document at a specific git commit.
type DocumentVersion struct {
	ID                int    `json:"id"`
	OrganizationID    int    `json:"organization_id"`
	DocumentID        string `json:"document_id"`
	Version           string `json:"version"`
	CommitHash        string `json:"commit_hash"`
	FilePath          string `json:"file_path"`
	ContentHash       string `json:"content_hash,omitempty"`
	Message           string `json:"message,omitempty"`
	Owner             string `json:"owner,omitempty"`
	ReviewCycleMonths *int   `json:"review_cycle_months,omitempty"`
	CreatedBy         string `json:"created_by"`
	CreatedAt         Epoch  `json:"created_at"`
}

// RecordVersion inserts a new document version snapshot.
func (d *DB) RecordVersion(ctx context.Context, orgID int, v *DocumentVersion) error {
	v.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO document_versions (organization_id, document_id, version, commit_hash, file_path, content_hash, message, owner, review_cycle_months, created_by, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, (SELECT id FROM users WHERE email = $10))
		ON CONFLICT (organization_id, document_id, version) DO UPDATE SET
			commit_hash = EXCLUDED.commit_hash,
			file_path = EXCLUDED.file_path,
			content_hash = EXCLUDED.content_hash,
			message = EXCLUDED.message,
			owner = EXCLUDED.owner,
			review_cycle_months = EXCLUDED.review_cycle_months,
			created_by = EXCLUDED.created_by,
			created_by_user_id = EXCLUDED.created_by_user_id
		RETURNING id, created_at
	`, orgID, v.DocumentID, v.Version, v.CommitHash, v.FilePath, v.ContentHash, v.Message,
		nilIfEmpty(v.Owner), v.ReviewCycleMonths, v.CreatedBy,
	).Scan(&v.ID, &v.CreatedAt)
}

// ListVersions returns all recorded versions of a document, newest first.
func (d *DB) ListVersions(ctx context.Context, orgID int, documentID string) ([]DocumentVersion, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, document_id, version, commit_hash, file_path, COALESCE(content_hash, ''), COALESCE(message, ''), COALESCE(owner, ''), review_cycle_months, created_by, created_at
		FROM document_versions WHERE organization_id = $1 AND document_id = $2
		ORDER BY created_at DESC
	`, orgID, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []DocumentVersion
	for rows.Next() {
		var v DocumentVersion
		if err := rows.Scan(&v.ID, &v.OrganizationID, &v.DocumentID, &v.Version, &v.CommitHash, &v.FilePath, &v.ContentHash, &v.Message, &v.Owner, &v.ReviewCycleMonths, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, nil
}

// GetVersion returns a specific version of a document.
func (d *DB) GetVersion(ctx context.Context, orgID int, documentID, version string) (*DocumentVersion, error) {
	var v DocumentVersion
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, document_id, version, commit_hash, file_path, COALESCE(content_hash, ''), COALESCE(message, ''), COALESCE(owner, ''), review_cycle_months, created_by, created_at
		FROM document_versions WHERE organization_id = $1 AND document_id = $2 AND version = $3
	`, orgID, documentID, version).Scan(&v.ID, &v.OrganizationID, &v.DocumentID, &v.Version, &v.CommitHash, &v.FilePath, &v.ContentHash, &v.Message, &v.Owner, &v.ReviewCycleMonths, &v.CreatedBy, &v.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("version %s of %s not found: %w", version, documentID, err)
	}
	return &v, nil
}

// LatestVersion returns the most recent version of a document.
func (d *DB) LatestVersion(ctx context.Context, orgID int, documentID string) (*DocumentVersion, error) {
	var v DocumentVersion
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, document_id, version, commit_hash, file_path, COALESCE(content_hash, ''), COALESCE(message, ''), COALESCE(owner, ''), review_cycle_months, created_by, created_at
		FROM document_versions WHERE organization_id = $1 AND document_id = $2
		ORDER BY created_at DESC LIMIT 1
	`, orgID, documentID).Scan(&v.ID, &v.OrganizationID, &v.DocumentID, &v.Version, &v.CommitHash, &v.FilePath, &v.ContentHash, &v.Message, &v.Owner, &v.ReviewCycleMonths, &v.CreatedBy, &v.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("no versions found for %s: %w", documentID, err)
	}
	return &v, nil
}
