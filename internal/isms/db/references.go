package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
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

// ListOrgReferencesTx returns every reference of the org, oldest id first. It
// runs inside the caller's WithOrgTx so row-level security applies.
func ListOrgReferencesTx(ctx context.Context, tx pgx.Tx, orgID int) ([]EntityReference, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, organization_id, source_type, source_id, target_type, target_id,
			COALESCE(created_by, ''), created_at, origin
		FROM entity_references
		WHERE organization_id = $1
		ORDER BY id
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs, err := scanReferences(rows)
	if err != nil {
		return nil, err
	}
	return refs, rows.Err()
}

// DeleteReferencesTx removes the given reference rows of the org and returns how
// many were deleted. Deleting by row id, not by pair, removes exactly the rows a
// report listed even if a new one appeared since.
func DeleteReferencesTx(ctx context.Context, tx pgx.Tx, orgID int, ids []int64) (int64, error) {
	tag, err := tx.Exec(ctx, `DELETE FROM entity_references WHERE organization_id = $1 AND id = ANY($2)`, orgID, ids)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ReferenceFix is what CanonicalizeReferenceTx did to a row.
type ReferenceFix int

const (
	ReferenceGone      ReferenceFix = iota // the row no longer exists; nothing done
	ReferenceRewritten                     // the row now carries the canonical ids
	ReferenceMerged                        // the canonical row existed; the old row was deleted
)

// CanonicalizeReferenceTx moves row id to the canonical tuple. It decides from
// the current state: if a row with the canonical tuple already exists it wins
// (and becomes 'manual' when the old row was), and the old row is deleted;
// otherwise the old row is updated in place, keeping its id, created_at,
// created_by and origin. A row deleted since the scan is left alone.
func CanonicalizeReferenceTx(ctx context.Context, tx pgx.Tx, orgID int, id int64, sourceType, sourceID, targetType, targetID string) (ReferenceFix, error) {
	var origin string
	err := tx.QueryRow(ctx, `SELECT origin FROM entity_references WHERE organization_id = $1 AND id = $2 FOR UPDATE`, orgID, id).Scan(&origin)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReferenceGone, nil
	}
	if err != nil {
		return 0, err
	}
	var keptID int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM entity_references
		WHERE organization_id = $1 AND source_type = $2 AND source_id = $3 AND target_type = $4 AND target_id = $5
		FOR UPDATE
	`, orgID, sourceType, sourceID, targetType, targetID).Scan(&keptID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		_, err = tx.Exec(ctx, `
			UPDATE entity_references SET source_id = $3, target_id = $4
			WHERE organization_id = $1 AND id = $2
		`, orgID, id, sourceID, targetID)
		return ReferenceRewritten, err
	case err != nil:
		return 0, err
	case keptID == id:
		return ReferenceGone, nil
	}
	if origin == ReferenceOriginManual {
		if _, err := tx.Exec(ctx, `UPDATE entity_references SET origin = 'manual' WHERE organization_id = $1 AND id = $2`, orgID, keptID); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM entity_references WHERE organization_id = $1 AND id = $2`, orgID, id); err != nil {
		return 0, err
	}
	return ReferenceMerged, nil
}

// identifierTables maps a reference type to the table whose `identifier` column
// holds its per-org identifier. Every one has a deleted_at column.
var identifierTables = map[string]string{
	"risk": "risks", "legal_requirement": "legal_requirements", "asset": "assets",
	"supplier": "suppliers", "system": "systems", "incident": "incidents",
	"corrective_action": "corrective_actions", "change_request": "change_requests",
	"task": "tasks",
}

// SoftDeletedEntityExists reports whether a soft-deleted record answers to
// entityID in the forms a reference may use (the same forms the live lookup
// accepts), so a reference to it can be told from one to a record that never
// existed. Documents live in git and are never soft-deleted, so the answer for
// them, and for an unknown type, is false.
func (d *DB) SoftDeletedEntityExists(ctx context.Context, orgID int, entityType, entityID string) (bool, error) {
	var query string
	var arg any = entityID
	if table, ok := identifierTables[entityType]; ok {
		query = fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE organization_id = $1 AND identifier = $2 AND deleted_at IS NOT NULL)`, table)
	} else {
		switch entityType {
		case "objective", "program":
			table, col := "objectives", "display_id"
			if entityType == "program" {
				table, col = "programs", "key"
			}
			if n, err := strconv.ParseInt(entityID, 10, 64); err == nil {
				col, arg = "id", n
			}
			query = fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE organization_id = $1 AND %s = $2 AND deleted_at IS NOT NULL)`, table, col)
		case "audit", "audit_finding", "audit_programme":
			prefix, table := "AUDIT-", "audits"
			if entityType == "audit_finding" {
				prefix, table = "FIND-", "audit_findings"
			} else if entityType == "audit_programme" {
				prefix, table = "AP-", "audit_programmes"
			}
			n, err := strconv.Atoi(strings.TrimPrefix(entityID, prefix))
			if err != nil {
				return false, nil
			}
			arg = n
			query = fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE organization_id = $1 AND id = $2 AND deleted_at IS NOT NULL)`, table)
		default:
			return false, nil
		}
	}
	var exists bool
	if err := d.pool.QueryRow(ctx, query, orgID, arg).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
