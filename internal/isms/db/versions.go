package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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
		ON CONFLICT (organization_id, document_id, version) WHERE archived_at IS NULL DO UPDATE SET
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

// Archived versions. A deleted document keeps its rows for audit, marked
// archived_at; every reader below ignores them, and RecordVersion's key is the
// partial unique index over the active rows, so a new document that reuses the
// id never reads, inherits or overwrites them.

// ArchiveDocumentVersions archives the active versions of a document id
// (case-insensitive) and returns how many it archived. It is idempotent. The
// caller must have established that the document is absent from the repository:
// an archived history is not served again.
func (d *DB) ArchiveDocumentVersions(ctx context.Context, orgID int, documentID string) (int64, error) {
	tag, err := d.pool.Exec(ctx, `
		UPDATE document_versions SET archived_at = now()
		WHERE organization_id = $1 AND lower(document_id) = lower($2) AND archived_at IS NULL
	`, orgID, documentID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ArchivedVersionsSince reports whether a version of the document id was
// archived after the given time, that is, whether the id went through a
// disappearance since then (a review created at that time belongs to an earlier
// incarnation of the document).
func (d *DB) ArchivedVersionsSince(ctx context.Context, orgID int, documentID string, since time.Time) (bool, error) {
	var has bool
	err := d.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM document_versions
			WHERE organization_id = $1 AND lower(document_id) = lower($2) AND archived_at > $3)
	`, orgID, documentID, since).Scan(&has)
	return has, err
}

// ArchiveDocumentAndCloseReviews archives the active versions of a document id
// and closes its live reviews in ONE transaction: either both happen or neither.
// It returns the number of versions archived and the ids of the reviews closed.
func (d *DB) ArchiveDocumentAndCloseReviews(ctx context.Context, orgID int, documentID string) (archived int64, closed []int, err error) {
	return d.ArchiveDocumentAndCloseReviewsBefore(ctx, orgID, documentID, time.Now().Add(24*time.Hour))
}

// ArchiveDocumentAndCloseReviewsBefore is ArchiveDocumentAndCloseReviews limited
// to versions and reviews created before the given time: a job that decided an id
// is gone by reading the repository at some moment must not archive what was
// created after it.
func (d *DB) ArchiveDocumentAndCloseReviewsBefore(ctx context.Context, orgID int, documentID string, before time.Time) (archived int64, closed []int, err error) {
	err = d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE document_versions SET archived_at = now()
			WHERE organization_id = $1 AND lower(document_id) = lower($2) AND archived_at IS NULL AND created_at < $3
		`, orgID, documentID, before)
		if err != nil {
			return err
		}
		archived = tag.RowsAffected()
		rows, err := tx.Query(ctx, `
			UPDATE reviews SET status = 'closed', updated_at = now()
			WHERE organization_id = $1 AND lower(document_id) = lower($2)
			  AND status IN ('open', 'changes_requested', 'approved') AND created_at < $3
			RETURNING id
		`, orgID, documentID, before)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				return err
			}
			closed = append(closed, id)
		}
		return rows.Err()
	})
	return archived, closed, err
}

// ListVersions returns all recorded versions of a document, newest first.
func (d *DB) ListVersions(ctx context.Context, orgID int, documentID string) ([]DocumentVersion, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, document_id, version, commit_hash, file_path, COALESCE(content_hash, ''), COALESCE(message, ''), COALESCE(owner, ''), review_cycle_months, created_by, created_at
		FROM document_versions WHERE organization_id = $1 AND document_id = $2 AND archived_at IS NULL
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
		FROM document_versions WHERE organization_id = $1 AND document_id = $2 AND version = $3 AND archived_at IS NULL
	`, orgID, documentID, version).Scan(&v.ID, &v.OrganizationID, &v.DocumentID, &v.Version, &v.CommitHash, &v.FilePath, &v.ContentHash, &v.Message, &v.Owner, &v.ReviewCycleMonths, &v.CreatedBy, &v.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("version %s of %s not found: %w", version, documentID, err)
	}
	return &v, nil
}

// GetVersionByID returns one version of a document by its row id. The row must
// belong to the given organization and document, so an id from another document
// or organization is simply not found (pgx.ErrNoRows). A version with an empty
// label can only be addressed this way.
func (d *DB) GetVersionByID(ctx context.Context, orgID int, documentID string, id int) (*DocumentVersion, error) {
	var v DocumentVersion
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, document_id, version, commit_hash, file_path, COALESCE(content_hash, ''), COALESCE(message, ''), COALESCE(owner, ''), review_cycle_months, created_by, created_at
		FROM document_versions WHERE id = $1 AND organization_id = $2 AND document_id = $3 AND archived_at IS NULL
	`, id, orgID, documentID).Scan(&v.ID, &v.OrganizationID, &v.DocumentID, &v.Version, &v.CommitHash, &v.FilePath, &v.ContentHash, &v.Message, &v.Owner, &v.ReviewCycleMonths, &v.CreatedBy, &v.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("version %d of %s not found: %w", id, documentID, err)
	}
	return &v, nil
}

// LatestVersion returns the most recent version of a document.
func (d *DB) LatestVersion(ctx context.Context, orgID int, documentID string) (*DocumentVersion, error) {
	var v DocumentVersion
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, document_id, version, commit_hash, file_path, COALESCE(content_hash, ''), COALESCE(message, ''), COALESCE(owner, ''), review_cycle_months, created_by, created_at
		FROM document_versions WHERE organization_id = $1 AND document_id = $2 AND archived_at IS NULL
		ORDER BY created_at DESC LIMIT 1
	`, orgID, documentID).Scan(&v.ID, &v.OrganizationID, &v.DocumentID, &v.Version, &v.CommitHash, &v.FilePath, &v.ContentHash, &v.Message, &v.Owner, &v.ReviewCycleMonths, &v.CreatedBy, &v.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("no versions found for %s: %w", documentID, err)
	}
	return &v, nil
}

// ActiveVersionDocumentIDs returns the distinct document ids (lowercase) that
// have active versions created before the given time.
func (d *DB) ActiveVersionDocumentIDs(ctx context.Context, orgID int, before time.Time) ([]string, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT DISTINCT lower(document_id) FROM document_versions
		WHERE organization_id = $1 AND archived_at IS NULL AND created_at < $2 ORDER BY 1
	`, orgID, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// VersionBackfillDone reports whether the one-time archiving of the versions of
// documents deleted before archiving existed has completed for the organization.
func (d *DB) VersionBackfillDone(ctx context.Context, orgID int) (bool, error) {
	var done bool
	err := d.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM document_version_backfill WHERE organization_id = $1)`, orgID).Scan(&done)
	return done, err
}

// MarkVersionBackfillDone records that the backfill completed.
func (d *DB) MarkVersionBackfillDone(ctx context.Context, orgID int) error {
	_, err := d.pool.Exec(ctx, `INSERT INTO document_version_backfill (organization_id) VALUES ($1) ON CONFLICT DO NOTHING`, orgID)
	return err
}

// Now is the database's clock. created_at columns are set by the database, so a
// time that is compared with them must come from the same clock, not from the
// application host's.
func (d *DB) Now(ctx context.Context) (time.Time, error) {
	var t time.Time
	err := d.pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&t)
	return t, err
}
