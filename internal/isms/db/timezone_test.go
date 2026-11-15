package db

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
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
	// pgx passes unknown URL parameters on as runtime parameters.
	d, err := New(context.Background(), url+sep+"timezone=America/New_York")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(d.Close)

	var tz string
	if err := d.pool.QueryRow(context.Background(), `SHOW timezone`).Scan(&tz); err != nil {
		t.Fatal(err)
	}
	if tz != "UTC" {
		t.Errorf("SHOW timezone = %q, want UTC even though the connection string asked for America/New_York", tz)
	}
	var same bool
	if err := d.pool.QueryRow(context.Background(),
		`SELECT CURRENT_DATE = (now() AT TIME ZONE 'UTC')::date`).Scan(&same); err != nil {
		t.Fatal(err)
	}
	if !same {
		t.Error("CURRENT_DATE is not the UTC date")
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
