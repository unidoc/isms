package db

import (
	"context"
	"testing"
)

// Requires a migrated Postgres — see notifications_roundtrip_test.go's header
// for how to stand one up. Skipped when ISMS_TEST_DATABASE_URL is unset.
//
// Regression test for #363: the Risks and Legal summary strips read their
// Critical/High counts from RiskStats and LegalStats, which counted closed
// items by level, so a treated and closed critical risk still counted as a
// critical one. Level counts are live exposure only; total and the status
// counts still cover every item.
func TestRegisterStatsLevelCountsExcludeClosed(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "registerstats")

	// Likelihood × impact: 16 and 25 are critical, 15 high, 6 medium.
	items := []struct {
		status             string
		likelihood, impact int
	}{
		{"open", 4, 4},
		{"draft", 3, 5},
		{"open", 2, 3},
		{"closed", 4, 4},
		{"closed", 5, 5},
		{"closed", 3, 5},
	}

	t.Run("risks", func(t *testing.T) {
		for _, it := range items {
			l, i := it.likelihood, it.impact
			r := &Risk{Title: "stats " + it.status, RiskType: RiskTypes[0], Origin: RiskOrigins[0],
				Status: it.status, Treatment: "mitigate", CurrentLikelihood: &l, CurrentImpact: &i}
			if err := d.CreateRisk(ctx, orgID, r); err != nil {
				t.Fatalf("CreateRisk: %v", err)
			}
		}
		s, err := d.RiskStats(ctx, orgID)
		if err != nil {
			t.Fatalf("RiskStats: %v", err)
		}
		want := RiskStats{Total: 6, Critical: 1, High: 1, Medium: 1, Low: 0, Open: 2, Closed: 3, Draft: 1}
		if *s != want {
			t.Errorf("RiskStats = %+v, want %+v", *s, want)
		}
	})

	t.Run("legal requirements", func(t *testing.T) {
		for _, it := range items {
			l, i := it.likelihood, it.impact
			lr := &LegalRequirement{Title: "stats " + it.status, Jurisdiction: "EU", Category: "privacy",
				Status: it.status, CurrentLikelihood: &l, CurrentImpact: &i}
			if err := d.CreateLegalRequirement(ctx, orgID, lr); err != nil {
				t.Fatalf("CreateLegalRequirement: %v", err)
			}
		}
		s, err := d.LegalStats(ctx, orgID)
		if err != nil {
			t.Fatalf("LegalStats: %v", err)
		}
		want := LegalStats{Total: 6, Critical: 1, High: 1, Medium: 1, Low: 0, NotAssessed: 0, Open: 2, Closed: 3, Draft: 1}
		if *s != want {
			t.Errorf("LegalStats = %+v, want %+v", *s, want)
		}
	})
}

// #425 F5: RiskStats had no bucket for "accepted" risks — added alongside
// making the status reachable (#414), so the stat strip's status breakdown
// doesn't silently undercount an org's total once risks can be accepted.
func TestRiskStatsCountsAccepted(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "riskstats-accepted")

	l, i := 2, 2
	for _, status := range []string{"open", "accepted", "accepted", "closed"} {
		r := &Risk{Title: "stats " + status, RiskType: RiskTypes[0], Origin: RiskOrigins[0],
			Status: status, Treatment: "mitigate", CurrentLikelihood: &l, CurrentImpact: &i}
		if err := d.CreateRisk(ctx, orgID, r); err != nil {
			t.Fatalf("CreateRisk: %v", err)
		}
	}
	s, err := d.RiskStats(ctx, orgID)
	if err != nil {
		t.Fatalf("RiskStats: %v", err)
	}
	want := RiskStats{Total: 4, Low: 3, Open: 1, Accepted: 2, Closed: 1}
	if *s != want {
		t.Errorf("RiskStats = %+v, want %+v", *s, want)
	}
}
