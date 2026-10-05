package db

import (
	"context"
)

// EntityReference represents a cross-reference link between two entities.
type EntityReference struct {
	ID             int64  `json:"id"`
	OrganizationID int    `json:"-"`
	SourceType     string `json:"source_type"`
	SourceID       string `json:"source_id"`
	TargetType     string `json:"target_type"`
	TargetID       string `json:"target_id"`
	Title          string `json:"title,omitempty"` // resolved display name (populated by API, not stored)
	CreatedBy      string `json:"created_by,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
	// Origin is who created the link: ReferenceOriginManual (the Links tab and
	// every other writer; the default when empty) or ReferenceOriginComment (a
	// #IDENT mention, removed again with its comment, #194).
	Origin string `json:"origin,omitempty"`
}

const (
	ReferenceOriginManual  = "manual"
	ReferenceOriginComment = "comment"
)

// referenceUpsertSQL inserts a reference, or returns the existing row for the
// pair. A manual write on an existing comment-made pair upgrades it to manual,
// so deleting the comment never removes a link someone also added by hand; a
// comment write never downgrades a manual pair.
const referenceUpsertSQL = `
	INSERT INTO entity_references (organization_id, source_type, source_id, target_type, target_id, created_by, created_by_user_id, origin)
	VALUES ($1, $2, $3, $4, $5, $6, (SELECT id FROM users WHERE email = $6), $7)
	ON CONFLICT (organization_id, source_type, source_id, target_type, target_id)
	DO UPDATE SET origin = CASE WHEN EXCLUDED.origin = 'manual' THEN 'manual' ELSE entity_references.origin END
	RETURNING id, created_at, origin`

func upsertReference(ctx context.Context, q rowQuerier, orgID int, ref *EntityReference) error {
	ref.OrganizationID = orgID
	if ref.Origin == "" {
		ref.Origin = ReferenceOriginManual
	}
	return q.QueryRow(ctx, referenceUpsertSQL,
		orgID, ref.SourceType, ref.SourceID, ref.TargetType, ref.TargetID, nilIfEmpty(ref.CreatedBy), ref.Origin,
	).Scan(&ref.ID, &ref.CreatedAt, &ref.Origin)
}

// CreateReference inserts a new entity reference, or returns the existing one.
func (d *DB) CreateReference(ctx context.Context, orgID int, ref *EntityReference) error {
	return upsertReference(ctx, d.pool, orgID, ref)
}

// GetReference returns a single reference by ID.
func (d *DB) GetReference(ctx context.Context, orgID int, id int64) (*EntityReference, error) {
	var r EntityReference
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, source_type, source_id, target_type, target_id,
			COALESCE(created_by, ''), created_at, origin
		FROM entity_references WHERE id = $1 AND organization_id = $2
	`, id, orgID).Scan(&r.ID, &r.OrganizationID, &r.SourceType, &r.SourceID,
		&r.TargetType, &r.TargetID, &r.CreatedBy, &r.CreatedAt, &r.Origin)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// DeleteReference removes a reference by ID.
func (d *DB) DeleteReference(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM entity_references WHERE id = $1 AND organization_id = $2`, id, orgID)
	return err
}

// DeleteReferencePair removes both directions of a bidirectional reference.
func (d *DB) DeleteReferencePair(ctx context.Context, orgID int, sourceType, sourceID, targetType, targetID string) error {
	_, err := d.pool.Exec(ctx, `
		DELETE FROM entity_references
		WHERE organization_id = $1
			AND (
				(source_type = $2 AND source_id = $3 AND target_type = $4 AND target_id = $5)
				OR
				(source_type = $4 AND source_id = $5 AND target_type = $2 AND target_id = $3)
			)
	`, orgID, sourceType, sourceID, targetType, targetID)
	return err
}

// ListReferencesFrom returns all references where the given entity is the source.
func (d *DB) ListReferencesFrom(ctx context.Context, orgID int, sourceType, sourceID string) ([]EntityReference, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, source_type, source_id, target_type, target_id,
			COALESCE(created_by, ''), created_at, origin
		FROM entity_references
		WHERE organization_id = $1 AND source_type = $2 AND source_id = $3
		ORDER BY created_at
	`, orgID, sourceType, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReferences(rows)
}

// ListReferencesTo returns all references where the given entity is the target.
func (d *DB) ListReferencesTo(ctx context.Context, orgID int, targetType, targetID string) ([]EntityReference, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, source_type, source_id, target_type, target_id,
			COALESCE(created_by, ''), created_at, origin
		FROM entity_references
		WHERE organization_id = $1 AND target_type = $2 AND target_id = $3
		ORDER BY created_at
	`, orgID, targetType, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReferences(rows)
}

// ListAllReferencesForEntity returns all references where the entity is either source or target.
func (d *DB) ListAllReferencesForEntity(ctx context.Context, orgID int, entityType, entityID string) ([]EntityReference, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, source_type, source_id, target_type, target_id,
			COALESCE(created_by, ''), created_at, origin
		FROM entity_references
		WHERE organization_id = $1
			AND ((source_type = $2 AND source_id = $3) OR (target_type = $2 AND target_id = $3))
		ORDER BY created_at
	`, orgID, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReferences(rows)
}

func scanReferences(rows interface {
	Next() bool
	Scan(dest ...interface{}) error
}) ([]EntityReference, error) {
	var refs []EntityReference
	for rows.Next() {
		var r EntityReference
		if err := rows.Scan(&r.ID, &r.OrganizationID, &r.SourceType, &r.SourceID,
			&r.TargetType, &r.TargetID, &r.CreatedBy, &r.CreatedAt, &r.Origin); err != nil {
			return nil, err
		}
		refs = append(refs, r)
	}
	return refs, nil
}
