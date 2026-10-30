package db

import (
	"context"
	"testing"
	"time"
)

// Requires a migrated Postgres — see notifications_roundtrip_test.go's header
// for how to stand one up. Skipped when ISMS_TEST_DATABASE_URL is unset.
//
// Proves the settings-registry seed for system_review_cycle_* (#308) resolves
// through SystemReviewCycles/GetOrgSetting, and that CreateSystem and
// UpdateSystem use the org's configured cycle rather than the built-in one.
func TestSystemReviewCyclesRoundTrip(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "systemreviewcycles")

	// A fixed last_review keeps the expected dates independent of today's date.
	last := NewEpoch(time.Date(2030, 3, 10, 0, 0, 0, 0, time.UTC))
	day := func(e *Epoch) string { return e.Time.UTC().Format("2006-01-02") }

	// 1. Untouched org: the registry defaults resolve to 1/3/6/12 with no
	// organization_settings rows.
	cycles := d.SystemReviewCycles(ctx, orgID)
	for level, want := range map[string]int{"critical": 1, "high": 3, "medium": 6, "low": 12} {
		sys := &System{Criticality: level, LastReview: &last}
		sys.CalculateNextReview(cycles)
		wantDate := last.Time.AddDate(0, want, 0).Format("2006-01-02")
		if got := day(sys.NextReview); got != wantDate {
			t.Errorf("untouched org, level %s: got %s, want %d months -> %s", level, got, want, wantDate)
		}
	}

	// 2. A configured system_review_cycle_high drives CreateSystem and
	// UpdateSystem, and only for that level.
	if err := d.SetOrgSetting(ctx, orgID, "system_review_cycle_high", "2"); err != nil {
		t.Fatalf("SetOrgSetting: %v", err)
	}
	if got := d.SystemReviewCycles(ctx, orgID)["high"]; got != 2 {
		t.Fatalf("SystemReviewCycles high after setting 2: got %d", got)
	}
	sys := &System{Name: "cycle system", Classification: "internal", Criticality: "high", Status: "active", LastReview: &last}
	if err := d.CreateSystem(ctx, orgID, sys); err != nil {
		t.Fatalf("CreateSystem: %v", err)
	}
	stored, err := d.GetSystem(ctx, orgID, sys.ID)
	if err != nil {
		t.Fatalf("GetSystem: %v", err)
	}
	if got, want := day(stored.NextReview), "2030-05-10"; got != want {
		t.Errorf("CreateSystem next_review = %s, want %s", got, want)
	}
	low := &System{Name: "low system", Classification: "internal", Criticality: "low", Status: "active", LastReview: &last}
	if err := d.CreateSystem(ctx, orgID, low); err != nil {
		t.Fatalf("CreateSystem low: %v", err)
	}
	if storedLow, err := d.GetSystem(ctx, orgID, low.ID); err != nil {
		t.Fatalf("GetSystem low: %v", err)
	} else if got, want := day(storedLow.NextReview), "2031-03-10"; got != want {
		t.Errorf("unconfigured level next_review = %s, want %s", got, want)
	}

	stored.Notes = "edited"
	stored.NextReview = nil
	if err := d.UpdateSystem(ctx, orgID, stored, nil); err != nil {
		t.Fatalf("UpdateSystem: %v", err)
	}
	if again, err := d.GetSystem(ctx, orgID, sys.ID); err != nil {
		t.Fatalf("GetSystem: %v", err)
	} else if got, want := day(again.NextReview), "2030-05-10"; got != want {
		t.Errorf("UpdateSystem next_review = %s, want %s", got, want)
	}

	// 3. DeleteOrgSetting on one level reverts THAT level to its registry
	// default while another level stays configured (the masking condition from
	// #43: with only one level configured, an empty map short-circuits to the
	// built-in defaults whatever the clear did).
	if err := d.SetOrgSetting(ctx, orgID, "system_review_cycle_critical", "2"); err != nil {
		t.Fatalf("SetOrgSetting: %v", err)
	}
	if err := d.DeleteOrgSetting(ctx, orgID, "system_review_cycle_high"); err != nil {
		t.Fatalf("DeleteOrgSetting: %v", err)
	}
	cycles = d.SystemReviewCycles(ctx, orgID)
	if cycles == nil || cycles["critical"] != 2 {
		t.Fatalf("expected critical:2 to remain configured after clearing high, got %v", cycles)
	}
	if got := cycles["high"]; got != 3 {
		t.Errorf("cleared system_review_cycle_high: cycles[\"high\"]=%d, want registry default 3 (full map: %v)", got, cycles)
	}
}
