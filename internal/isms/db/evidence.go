package db

import (
	"context"
)

// Evidence is an S3-backed file attachment on a checkin.
type Evidence struct {
	ID             int64     `json:"id"`
	OrganizationID int       `json:"organization_id"`
	CheckinID      int64     `json:"checkin_id"`
	Title          string    `json:"title"`
	ObjectKey      string    `json:"object_key"`
	ContentType    string    `json:"content_type"`
	SizeBytes      *int64    `json:"size_bytes,omitempty"`
	SHA256         string `json:"sha256,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
}

func (d *DB) CreateEvidence(ctx context.Context, orgID int, e *Evidence) error {
	e.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO checkin_evidence (organization_id, checkin_id, title, object_key, content_type, size_bytes, sha256)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`, orgID, e.CheckinID, e.Title, e.ObjectKey, e.ContentType, e.SizeBytes, nilIfEmpty(e.SHA256),
	).Scan(&e.ID, &e.CreatedAt)
}

func (d *DB) GetEvidence(ctx context.Context, orgID int, id int64) (*Evidence, error) {
	var e Evidence
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, checkin_id, title, object_key, content_type,
			size_bytes, COALESCE(sha256, ''), created_at
		FROM checkin_evidence WHERE id = $1 AND organization_id = $2
	`, id, orgID).Scan(&e.ID, &e.OrganizationID, &e.CheckinID, &e.Title, &e.ObjectKey,
		&e.ContentType, &e.SizeBytes, &e.SHA256, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (d *DB) ListEvidence(ctx context.Context, orgID int, checkinID int64) ([]Evidence, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, checkin_id, title, object_key, content_type,
			size_bytes, COALESCE(sha256, ''), created_at
		FROM checkin_evidence
		WHERE organization_id = $1 AND checkin_id = $2
		ORDER BY created_at
	`, orgID, checkinID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var evidence []Evidence
	for rows.Next() {
		var e Evidence
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.CheckinID, &e.Title, &e.ObjectKey,
			&e.ContentType, &e.SizeBytes, &e.SHA256, &e.CreatedAt); err != nil {
			return nil, err
		}
		evidence = append(evidence, e)
	}
	return evidence, nil
}

func (d *DB) DeleteEvidence(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM checkin_evidence WHERE id = $1 AND organization_id = $2`, id, orgID)
	return err
}
