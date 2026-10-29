package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// shadowTableDB returns a DB whose own connection pool sees a view named after
// table in a private schema ahead of public.<table>. The overdue queries use
// unqualified table names, so only this pool is affected: the shared public
// schema is never altered, which keeps the parallel db/refs/api test packages
// safe. viewSelect is the SELECT list (over public.<table> s) of the view.
func shadowTableDB(t *testing.T, table, viewSelect string) *DB {
	t.Helper()
	ctx := context.Background()
	url := os.Getenv("ISMS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ISMS_TEST_DATABASE_URL not set — skipping database round-trip test")
	}
	schema := fmt.Sprintf("overdue_shadow_%d", time.Now().UnixNano())

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}
	defer admin.Close()
	for _, stmt := range []string{
		`CREATE SCHEMA ` + schema,
		`CREATE VIEW ` + schema + `.` + table + ` AS SELECT ` + viewSelect + ` FROM public.` + table + ` s`,
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			_, _ = admin.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parsing test database url: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ", public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connecting shadow pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		c, err := pgxpool.New(ctx, url)
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = c.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})
	return &DB{pool: pool}
}

// seedOverdueOrg creates an org with one expired supplier contract (read by an
// early query of the summary) and one overdue system (the query the shadow view
// intercepts), so a swallowed error would show up as a partial summary.
func seedOverdueOrg(t *testing.T, d *DB, name string) int {
	t.Helper()
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, name)
	sup := newTestSupplier(t, d, orgID, name+" Co")
	offsetDays(t, d, sup.ID, -5)
	if _, err := d.pool.Exec(ctx, `
		INSERT INTO systems (organization_id, identifier, name, next_review)
		VALUES ($1, 'SYSTEM-1', 'Late System', CURRENT_DATE - 3)`, orgID); err != nil {
		t.Fatalf("seeding system: %v", err)
	}
	return orgID
}

func TestGetOverdueSummaryControl(t *testing.T) {
	d := testDB(t)
	orgID := seedOverdueOrg(t, d, "overdue-ctl")

	summary, err := d.GetOverdueSummary(context.Background(), orgID, TaskViewer{CanSeeAll: true})
	if err != nil {
		t.Fatalf("GetOverdueSummary: %v", err)
	}
	if len(summary.SupplierContracts) != 1 || len(summary.Systems) != 1 || summary.TotalCount != 2 {
		t.Fatalf("want 1 contract + 1 system (total 2), got %d contracts, %d systems, total %d",
			len(summary.SupplierContracts), len(summary.Systems), summary.TotalCount)
	}
}

func TestGetOverdueSummaryReturnsQueryError(t *testing.T) {
	d := testDB(t)
	orgID := seedOverdueOrg(t, d, "overdue-qerr")
	// No next_review column: the systems query fails when it is prepared.
	shadow := shadowTableDB(t, "systems", `s.id, s.organization_id, s.identifier, s.name, s.criticality, s.owner_id, s.deleted_at`)

	summary, err := shadow.GetOverdueSummary(context.Background(), orgID, TaskViewer{CanSeeAll: true})
	if err == nil {
		t.Fatalf("want an error when the systems query fails, got summary %+v", summary)
	}
	if summary != nil {
		t.Errorf("want a nil summary alongside the error, got %+v", summary)
	}
	if !strings.Contains(err.Error(), "systems") {
		t.Errorf("error should name the failing query, got %q", err)
	}
}

func TestGetOverdueSummaryReturnsScanError(t *testing.T) {
	d := testDB(t)
	orgID := seedOverdueOrg(t, d, "overdue-scan")
	// criticality is an integer array here, which cannot be scanned into the string
	// the loop scans it into: the query runs, rows.Scan fails.
	shadow := shadowTableDB(t, "systems", `s.id, s.organization_id, s.identifier, s.name, ARRAY[1] AS criticality, s.next_review, s.owner_id, s.deleted_at`)

	summary, err := shadow.GetOverdueSummary(context.Background(), orgID, TaskViewer{CanSeeAll: true})
	if err == nil {
		t.Fatalf("want an error when scanning a systems row fails, got summary %+v", summary)
	}
	if summary != nil {
		t.Errorf("want a nil summary alongside the error, got %+v", summary)
	}
}

func TestGetOverdueSummaryReturnsRowsErr(t *testing.T) {
	d := testDB(t)
	orgID := seedOverdueOrg(t, d, "overdue-rowserr")
	// Division by zero while the server produces the first row: the query is
	// accepted, Next() returns false without ever reaching Scan, and the failure
	// is only visible through rows.Err().
	shadow := shadowTableDB(t, "systems", `s.id, s.organization_id, s.identifier, s.name, (1 / (s.id - s.id))::text AS criticality, s.next_review, s.owner_id, s.deleted_at`)

	summary, err := shadow.GetOverdueSummary(context.Background(), orgID, TaskViewer{CanSeeAll: true})
	if err == nil {
		t.Fatalf("want an error when iterating the systems rows fails, got summary %+v", summary)
	}
	if summary != nil {
		t.Errorf("want a nil summary alongside the error, got %+v", summary)
	}
}
