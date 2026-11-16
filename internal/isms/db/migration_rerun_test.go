package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The runner records migrations by file name, and every append to the unreleased
// v1.0.0 file renames it, so a database that already ran an earlier name runs
// the whole file again on top of rows written since. The file must therefore
// never re-add a CHECK with a narrower value set than the rows it can meet: one
// violated row rolls back the whole file and the server will not boot.
//
// This applies the file a second time, the way Migrate does (one Exec of the
// whole file), over a database holding a row for every value the widened CHECKs
// allow. Requires a migrated Postgres, like the other tests here.
func TestV100MigrationReRunsOnPopulatedData(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*_v1.0.0.sql"))
	if err != nil || len(files) != 1 {
		t.Fatalf("want exactly one *_v1.0.0.sql, got %v (%v)", files, err)
	}
	sqlText, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	orgID, _ := newTestOrgUser(t, d, "migration-rerun")

	for _, et := range []string{"trust_center_item", "trust_center_settings", "trust_center_certification"} {
		if _, err := d.pool.Exec(ctx, `
			INSERT INTO entity_changelog (organization_id, entity_type, entity_id, action, changed_by)
			VALUES ($1, $2, 1, 'update', 'migration-test@example.test')`, orgID, et); err != nil {
			t.Fatalf("precondition: entity_changelog row for %s: %v (is the test database migrated?)", et, err)
		}
	}
	for i, reason := range []string{"retired", "deleted", "terminated", "no_data_access"} {
		if _, err := d.pool.Exec(ctx, `
			INSERT INTO trust_center_items (organization_id, kind, reference, access_level, published_by, hidden_at, hidden_reason)
			VALUES ($1, 'subprocessor', $2, 'public', 'migration-test@example.test', now(), $3)`,
			orgID, fmt.Sprint(900+i), reason); err != nil {
			t.Fatalf("precondition: item hidden as %s: %v", reason, err)
		}
	}

	if _, err := d.pool.Exec(ctx, string(sqlText)); err != nil {
		t.Fatalf("re-running %s on a populated database: %v", filepath.Base(files[0]), err)
	}
}
