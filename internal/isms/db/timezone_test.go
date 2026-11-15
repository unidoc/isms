package db

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Requires a migrated Postgres (ISMS_TEST_DATABASE_URL); skipped otherwise.
//
// #503: every session must run in UTC whatever the connection string or the
// Postgres server's TimeZone says, or CURRENT_DATE and date casts move with
// the host's configuration.
func TestPoolSessionsRunInUTC(t *testing.T) {
	url := os.Getenv("ISMS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ISMS_TEST_DATABASE_URL not set - skipping database round-trip test")
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	// pgx passes unknown URL parameters on as runtime parameters, kept as
	// spelled; Postgres matches the name case-insensitively.
	for _, spelling := range []string{"timezone", "TimeZone", "TIMEZONE"} {
		t.Run(spelling, func(t *testing.T) {
			ctx := context.Background()
			d, err := New(ctx, url+sep+"pool_max_conns=40&"+spelling+"=America/New_York")
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			t.Cleanup(d.Close)

			// 30 distinct connections: a stray second key made the winner
			// vary per connection, so one session proves nothing.
			var conns []*pgxpool.Conn
			t.Cleanup(func() {
				for _, c := range conns {
					c.Release()
				}
			})
			bad := 0
			for i := 0; i < 30; i++ {
				c, err := d.pool.Acquire(ctx)
				if err != nil {
					t.Fatal(err)
				}
				conns = append(conns, c)
				var tz string
				if err := c.QueryRow(ctx, `SHOW timezone`).Scan(&tz); err != nil {
					t.Fatal(err)
				}
				if tz != "UTC" {
					bad++
				}
			}
			if bad != 0 {
				t.Errorf("%d of 30 connections ran outside UTC although the connection string asked for America/New_York", bad)
			}
			var same bool
			if err := d.pool.QueryRow(ctx,
				`SELECT CURRENT_DATE = (now() AT TIME ZONE 'UTC')::date`).Scan(&same); err != nil {
				t.Fatal(err)
			}
			if !same {
				t.Error("CURRENT_DATE is not the UTC date")
			}
		})
	}
}

// A DATE written from a Go time must land on the time's UTC calendar day,
// whatever zone the host runs in.
func TestDateColumnTakesUTCDayFromHostLocalTime(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "tz-date-roundtrip")

	for _, z := range hostZones {
		t.Run(z.name, func(t *testing.T) {
			withLocalZone(t, z.name, z.offset)
			// 23:30 on Jan 1 local; the UTC day differs on both hosts (west:
			// 04:30 Jan 2, east: 16:30 Jan 1 is the same day, so use 03:30
			// there to cross midnight the other way).
			hour := 23
			if z.offset > 0 {
				hour = 3
			}
			local := time.Date(2026, 1, 1, hour, 30, 0, 0, time.Local)
			want := local.UTC().Format("2006-01-02")
			if want == local.Format("2006-01-02") {
				t.Fatalf("test setup: local and UTC day are both %s", want)
			}

			e := NewEpoch(local)
			risk := &Risk{Title: "tz " + z.name, RiskType: RiskTypes[0], Origin: RiskOrigins[0], Status: RiskStatuses[0], LastReview: &e}
			if err := d.CreateRisk(ctx, orgID, risk); err != nil {
				t.Fatalf("CreateRisk: %v", err)
			}
			got, err := d.GetRisk(ctx, orgID, risk.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.LastReview == nil {
				t.Fatal("last_review was not stored")
			}
			if day := got.LastReview.Time.UTC().Format("2006-01-02"); day != want {
				t.Errorf("last_review = %s, want the UTC day %s", day, want)
			}
		})
	}
}

// A record read back from Postgres must diff clean against the same values
// decoded from JSON: pgx scans TIMESTAMPTZ in time.Local, JSON epochs are UTC,
// and the changelog formatted each in its own zone, so a host west of UTC
// logged a false change on an unchanged planned_at.
func TestChangeDiffIsZoneFree(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, actor := newTestOrgUser(t, d, "tz-change-diff")
	withLocalZone(t, "west", -5*3600)

	var planned Epoch
	if err := json.Unmarshal([]byte(`1767225600`), &planned); err != nil { // 2026-01-01 00:00 UTC
		t.Fatal(err)
	}
	cr := &ChangeRequest{
		Title: "diff", Description: "d", Priority: "medium", Category: "other",
		RiskLevel: "low", RequestedBy: actor, Status: "proposed", PlannedAt: &planned,
	}
	if err := d.CreateChangeRequest(ctx, orgID, cr); err != nil {
		t.Fatalf("CreateChangeRequest: %v", err)
	}
	read, err := d.GetChangeRequest(ctx, orgID, cr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.PlannedAt == nil {
		t.Fatal("planned_at not stored")
	}
	diffs := DiffFields("change", int64(cr.ID), actor, "", read.ToChangeMap(), cr.ToChangeMap())
	if len(diffs) != 0 {
		t.Errorf("unchanged record logged %d false change(s): %+v", len(diffs), diffs)
	}
}
