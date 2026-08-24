package db

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// The keyed-notification columns (#212) are only half-testable in memory: the
// unit tests next door prove what keyedColumns() decides, not that the decision
// survives the INSERT and comes back off the SELECT intact. The scan is
// positional over eleven columns and the params column is JSONB, so a column
// reordered in the query or a type the driver refuses is exactly the class of
// regression that compiles, passes every unit test, and corrupts the inbox.
//
// Requires a migrated Postgres. Set ISMS_TEST_DATABASE_URL to run; skipped
// otherwise so `go test ./...` stays green on a machine with no database:
//
//	createdb isms_dbtest && for f in migrations/*.sql; do psql -f $f ...; done
//	ISMS_TEST_DATABASE_URL="postgres://isms:isms@localhost:5432/isms_dbtest?sslmode=disable" go test ./internal/isms/db/...
func testDB(t *testing.T) *DB {
	t.Helper()
	url := os.Getenv("ISMS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ISMS_TEST_DATABASE_URL not set — skipping database round-trip test")
	}
	d, err := New(context.Background(), url)
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}
	t.Cleanup(d.pool.Close)
	return d
}

// newTestOrgUser creates a throwaway org and member, returning the org id and
// the user's email. Every row it creates is removed on cleanup, so the suite can
// run repeatedly against the same database.
func newTestOrgUser(t *testing.T, d *DB, name string) (int, string) {
	t.Helper()
	ctx := context.Background()
	email := fmt.Sprintf("%s@notifications-roundtrip.test", name)

	var orgID int
	if err := d.pool.QueryRow(ctx,
		`INSERT INTO organizations (name, slug, repo_path) VALUES ($1, $1, '/tmp/none') RETURNING id`,
		name,
	).Scan(&orgID); err != nil {
		t.Fatalf("creating org: %v", err)
	}
	var userID int
	if err := d.pool.QueryRow(ctx,
		`INSERT INTO users (email, name) VALUES ($1, $1) RETURNING id`, email,
	).Scan(&userID); err != nil {
		t.Fatalf("creating user: %v", err)
	}
	t.Cleanup(func() {
		// notifications cascade with the org; the user is referenced by them, so
		// it has to go last.
		_, _ = d.pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = d.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})
	return orgID, email
}

func TestNotificationKeysRoundTripThroughPostgres(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "keyed-roundtrip")

	c := NotificationContent{
		Title:    "Review: Access Control Policy v2",
		TitleKey: "notifications.review_requested",
		Body:     "alice@example.com wants to publish Access Control Policy v2 and requested your review\n\nNote: check section 4",
		BodyKey:  "notifications.review_requested.body_with_note",
		Params: map[string]any{
			"actor":   "alice@example.com",
			"title":   "Access Control Policy",
			"version": "2",
			"round":   3,
			// User-authored text: quotes, an ampersand and a newline all have to
			// survive the JSONB round-trip byte for byte.
			"note": "check \"section 4\" & the annex\nthanks",
		},
		Link: "/reviews/1",
	}
	if err := d.CreateNotificationContentByEmail(ctx, orgID, email, c); err != nil {
		t.Fatalf("creating notification: %v", err)
	}

	got := listOne(t, d, ctx, orgID, email)
	if got.Title != c.Title || got.Body != c.Body || got.Link != c.Link {
		t.Errorf("English fallback not preserved:\n got %q / %q / %q\nwant %q / %q / %q",
			got.Title, got.Body, got.Link, c.Title, c.Body, c.Link)
	}
	if got.TitleKey != c.TitleKey || got.BodyKey != c.BodyKey {
		t.Errorf("keys = %q / %q, want %q / %q", got.TitleKey, got.BodyKey, c.TitleKey, c.BodyKey)
	}
	if got.Params["note"] != c.Params["note"] {
		t.Errorf("note not verbatim:\n got %q\nwant %q", got.Params["note"], c.Params["note"])
	}
	if got.Params["actor"] != "alice@example.com" || got.Params["version"] != "2" {
		t.Errorf("params lost: %#v", got.Params)
	}
	// JSONB numbers come back through encoding/json as float64. The renderer
	// interpolates them as-is, so this is the contract, not an accident.
	if got.Params["round"] != float64(3) {
		t.Errorf("round = %#v, want float64(3)", got.Params["round"])
	}
	// The row must be readable by the JSON the API actually ships.
	var wire map[string]any
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if wire["title_key"] != c.TitleKey {
		t.Errorf("wire title_key = %v, want %q", wire["title_key"], c.TitleKey)
	}
}

func TestUnkeyedNotificationRoundTripsAsLegacyRow(t *testing.T) {
	// The legacy helper and the agent path write no keys. Those rows must come
	// back with the three columns SQL NULL and absent from the JSON entirely, or
	// clients that predate the columns see fields they do not expect.
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "legacy-roundtrip")

	if err := d.CreateNotificationByEmail(ctx, orgID, email,
		"AI review escalated", "Needs a human decision", "/inbox"); err != nil {
		t.Fatalf("creating notification: %v", err)
	}

	var titleKey, bodyKey, params *string
	if err := d.pool.QueryRow(ctx,
		`SELECT title_key, body_key, params::text FROM notifications WHERE organization_id = $1`, orgID,
	).Scan(&titleKey, &bodyKey, &params); err != nil {
		t.Fatalf("reading columns: %v", err)
	}
	if titleKey != nil || bodyKey != nil || params != nil {
		t.Errorf("want all three columns NULL, got %v / %v / %v", titleKey, bodyKey, params)
	}

	got := listOne(t, d, ctx, orgID, email)
	if got.TitleKey != "" || got.BodyKey != "" || got.Params != nil {
		t.Errorf("unkeyed row read back keyed: %+v", got)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, absent := range []string{"title_key", "body_key", "params"} {
		if _, present := wire[absent]; present {
			t.Errorf("%s present in JSON for a legacy row: %s", absent, raw)
		}
	}
}

func TestUnmarshalableParamsStoreAnEnglishOnlyRow(t *testing.T) {
	// The review finding this test exists for: params that fail to marshal used
	// to store NULL params alongside live keys, so the client resolved a frame it
	// could not fill instead of falling back to the English title. Keys and
	// params travel together, all the way to the column.
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "unmarshalable-roundtrip")

	err := d.CreateNotificationContentByEmail(ctx, orgID, email, NotificationContent{
		Title:    "Review: Access Control Policy v2",
		TitleKey: "notifications.review_requested",
		Body:     "alice@example.com requested your review",
		BodyKey:  "notifications.review_requested.body",
		Params:   map[string]any{"actor": func() {}},
		Link:     "/reviews/1",
	})
	if err != nil {
		t.Fatalf("the notification must still be delivered: %v", err)
	}

	got := listOne(t, d, ctx, orgID, email)
	if got.Title == "" {
		t.Error("English title lost — the row must still render")
	}
	if got.TitleKey != "" || got.BodyKey != "" {
		t.Errorf("keys survived without params: title_key=%q body_key=%q", got.TitleKey, got.BodyKey)
	}
	if got.Params != nil {
		t.Errorf("params = %#v, want nil", got.Params)
	}
}

func listOne(t *testing.T, d *DB, ctx context.Context, orgID int, email string) Notification {
	t.Helper()
	var userID int
	if err := d.pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&userID); err != nil {
		t.Fatalf("looking up user: %v", err)
	}
	ns, err := d.ListNotifications(ctx, orgID, userID, false, 0)
	if err != nil {
		t.Fatalf("listing notifications: %v", err)
	}
	if len(ns) != 1 {
		t.Fatalf("got %d notifications, want exactly 1", len(ns))
	}
	return ns[0]
}
