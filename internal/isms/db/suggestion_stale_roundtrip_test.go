package db

import (
	"context"
	"testing"
	"time"
)

// Requires a migrated Postgres — see notifications_roundtrip_test.go's header
// for how to stand one up. Skipped when ISMS_TEST_DATABASE_URL is unset.
//
// Regression for #338: EntityStaleSnapshot must read the same clock
// EntityChangesAfter compares against (entity_changelog.created_at), not
// updated_at alone — register handlers write the entity, then log the
// changelog row as a separate statement, so the row's created_at always lands
// after updated_at. A snapshot of plain updated_at would make
// EntityChangesAfter see the entity's own latest row as a change every time.
//
// Also covers #339: the snapshot is looked up by primary key, the same key
// EntityChangesAfter (and s.resolveEntityID) use — no identifier-vs-id column
// switch to get wrong.
func newTestAsset(t *testing.T, d *DB, orgID int, name string) (assetID int64, updatedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := d.pool.QueryRow(ctx, `
		INSERT INTO assets (organization_id, identifier, name)
		VALUES ($1, $2, $3)
		RETURNING id, updated_at
	`, orgID, "ASSET-"+name, name).Scan(&assetID, &updatedAt); err != nil {
		t.Fatalf("creating asset: %v", err)
	}
	return assetID, updatedAt
}

// insertChangelogAt writes an entity_changelog row with an explicit
// created_at, bypassing LogChange (which always uses now()). The stale
// snapshot boundary is a strict '>', so tests exercise it with an explicit
// offset rather than relying on two statements landing in different
// transaction-clock ticks (inside one test's statements they would not).
func insertChangelogAt(t *testing.T, d *DB, orgID int, entityType string, entityID int64, changedBy string, createdAt time.Time) {
	t.Helper()
	ctx := context.Background()
	if _, err := d.pool.Exec(ctx, `
		INSERT INTO entity_changelog (organization_id, entity_type, entity_id, action, changed_by, created_at)
		VALUES ($1, $2, $3, 'update', $4, $5)
	`, orgID, entityType, entityID, changedBy, createdAt); err != nil {
		t.Fatalf("inserting changelog row: %v", err)
	}
}

func TestEntityStaleSnapshotUsesNewestChangelogRow(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "stale-snapshot-changelog")

	assetID, updatedAt := newTestAsset(t, d, orgID, "changelog-newer")
	rowCreatedAt := updatedAt.Add(1 * time.Millisecond)
	insertChangelogAt(t, d, orgID, "asset", assetID, email, rowCreatedAt)

	snapshot := d.EntityStaleSnapshot(ctx, orgID, "asset", assetID)
	if snapshot == nil {
		t.Fatal("snapshot = nil, want the changelog row's created_at")
	}
	if !snapshot.Time.Equal(rowCreatedAt) {
		t.Errorf("snapshot = %v, want the changelog row's created_at %v", snapshot.Time, rowCreatedAt)
	}

	// This is the #338 invariant at the layer it breaks: a snapshot taken at
	// (or after) the entity's own latest changelog row must not itself show up
	// as a change.
	changes, err := d.EntityChangesAfter(ctx, orgID, "asset", assetID, *snapshot)
	if err != nil {
		t.Fatalf("EntityChangesAfter: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("EntityChangesAfter(snapshot) = %d rows, want 0 (entity's own row counted as a change): %+v", len(changes), changes)
	}
}

func TestEntityStaleSnapshotFallsBackToUpdatedAtWithNoChangelog(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "stale-snapshot-no-changelog")

	assetID, updatedAt := newTestAsset(t, d, orgID, "no-changelog")

	snapshot := d.EntityStaleSnapshot(ctx, orgID, "asset", assetID)
	if snapshot == nil {
		t.Fatal("snapshot = nil, want updated_at (GREATEST falls back when there are no changelog rows)")
	}
	if !snapshot.Time.Equal(updatedAt) {
		t.Errorf("snapshot = %v, want updated_at %v", snapshot.Time, updatedAt)
	}
}

func TestEntityStaleSnapshotNilCases(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "stale-snapshot-nil-cases")
	otherOrgID, _ := newTestOrgUser(t, d, "stale-snapshot-nil-cases-other")

	assetID, _ := newTestAsset(t, d, orgID, "nil-cases")

	if got := d.EntityStaleSnapshot(ctx, orgID, "asset", 999999999); got != nil {
		t.Errorf("nonexistent PK: snapshot = %v, want nil", got.Time)
	}
	if got := d.EntityStaleSnapshot(ctx, otherOrgID, "asset", assetID); got != nil {
		t.Errorf("another org's PK: snapshot = %v, want nil", got.Time)
	}

	if _, err := d.pool.Exec(ctx, `UPDATE assets SET deleted_at = now() WHERE id = $1`, assetID); err != nil {
		t.Fatalf("soft-deleting asset: %v", err)
	}
	if got := d.EntityStaleSnapshot(ctx, orgID, "asset", assetID); got != nil {
		t.Errorf("soft-deleted entity: snapshot = %v, want nil", got.Time)
	}
}

func TestEntityChangesAfterSeesRowLoggedAfterSnapshot(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "stale-snapshot-later-row")

	assetID, updatedAt := newTestAsset(t, d, orgID, "later-row")

	snapshot := d.EntityStaleSnapshot(ctx, orgID, "asset", assetID)
	if snapshot == nil {
		t.Fatal("snapshot = nil, want updated_at")
	}

	laterRow := updatedAt.Add(1 * time.Hour)
	insertChangelogAt(t, d, orgID, "asset", assetID, email, laterRow)

	changes, err := d.EntityChangesAfter(ctx, orgID, "asset", assetID, *snapshot)
	if err != nil {
		t.Fatalf("EntityChangesAfter: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("EntityChangesAfter = %d rows, want 1: %+v", len(changes), changes)
	}
	if !changes[0].CreatedAt.Time.Equal(laterRow) {
		t.Errorf("change created_at = %v, want %v", changes[0].CreatedAt.Time, laterRow)
	}
}
