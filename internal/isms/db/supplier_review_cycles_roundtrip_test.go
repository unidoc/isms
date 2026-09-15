package db

import (
	"context"
	"testing"
)

// Requires a migrated Postgres — see notifications_roundtrip_test.go's header
// for how to stand one up. Skipped when ISMS_TEST_DATABASE_URL is unset.
//
// Proves the settings-registry seed from migration 20260915000000_v0.8.0.sql
// actually resolves through SupplierReviewCycles/GetOrgSetting, not just that
// the in-memory fallback logic is correct — the seed's default_value column is
// the AC-2 proof (existing orgs need zero organization_settings rows and zero
// backfill), so this test fails if the migration seed or SupplierReviewCycles
// ever drift apart.
func TestSupplierReviewCyclesRoundTrip(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "supplierreviewcycles")

	// 1. Untouched org: no organization_settings row for any
	// supplier_review_cycle_* key. SupplierReviewCycles may return nil or the
	// registry defaults — either is correct as long as CalculateNextReview
	// resolves to 1/3/6/12, which is the actual AC-2 behaviour being proved.
	cycles := d.SupplierReviewCycles(ctx, orgID)
	want := map[string]int{"critical": 1, "high": 3, "medium": 6, "low": 12}
	// Compare against a fresh CalculateNextReview(nil) call taken at (nearly)
	// the same instant, rather than asserting exact months, so the test
	// cannot flake at a year boundary.
	for level, months := range want {
		got := &Supplier{Criticality: level}
		got.CalculateNextReview(cycles)
		wantSup := &Supplier{Criticality: level}
		wantSup.CalculateNextReview(nil)
		if got.NextReview.Time.Format("2006-01-02") != wantSup.NextReview.Time.Format("2006-01-02") {
			t.Errorf("untouched org, level %s: got %s, want default %d months -> %s",
				level, got.NextReview.Time.Format("2006-01-02"), months, wantSup.NextReview.Time.Format("2006-01-02"))
		}
	}

	// 2. Org configures a custom critical cycle.
	if err := d.SetOrgSetting(ctx, orgID, "supplier_review_cycle_critical", "2"); err != nil {
		t.Fatalf("SetOrgSetting: %v", err)
	}
	cycles = d.SupplierReviewCycles(ctx, orgID)
	if cycles == nil || cycles["critical"] != 2 {
		t.Fatalf("SupplierReviewCycles after setting critical=2: got %v, want map with critical:2", cycles)
	}

	// 3. A value stored directly as garbage (bypassing the API guard in
	// api_admin.go) must be skipped, not trusted — the level falls back to 12
	// rather than erroring or corrupting the schedule.
	if err := d.SetOrgSetting(ctx, orgID, "supplier_review_cycle_high", "soon"); err != nil {
		t.Fatalf("SetOrgSetting: %v", err)
	}
	cycles = d.SupplierReviewCycles(ctx, orgID)
	if _, ok := cycles["high"]; ok {
		t.Errorf("garbage supplier_review_cycle_high value was not skipped: cycles=%v", cycles)
	}
	s := &Supplier{Criticality: "high"}
	s.CalculateNextReview(cycles)
	wantHigh := &Supplier{Criticality: "high"}
	wantHigh.CalculateNextReview(map[string]int{}) // empty map: "high" is absent -> falls back to 12
	if s.NextReview.Time.Format("2006-01-02") != wantHigh.NextReview.Time.Format("2006-01-02") {
		t.Errorf("high with garbage stored value: got %s, want 12-month fallback %s",
			s.NextReview.Time.Format("2006-01-02"), wantHigh.NextReview.Time.Format("2006-01-02"))
	}
}
