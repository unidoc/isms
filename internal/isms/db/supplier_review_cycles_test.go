package db

import (
	"testing"
	"time"
)

// #43: supplier review cycle length used to be a hard-coded per-criticality
// switch. It is now driven by an org-resolved map[string]int, exactly like
// Risk.CalculateReviewDate. Every case here
// pins a semantic that must survive that change: unknown/empty criticality
// and a criticality missing from a partial org map both fall back to 12
// months (not an error, not zero), nil means "use the built-in 1/3/6/12
// defaults", and last_review (when set and non-zero) is the base date
// instead of now(). Losing any of these silently reschedules real supplier
// reviews.

func TestReviewCycleDefaultsContract(t *testing.T) {
	// AC 2 requires existing orgs to see zero behaviour change. That is only
	// true if these defaults never move — pin them so a future edit to
	// reviewCycleDefaults (shared with risks) is caught here too.
	want := map[string]int{"critical": 1, "high": 3, "medium": 6, "low": 12}
	if len(reviewCycleDefaults) != len(want) {
		t.Fatalf("reviewCycleDefaults=%v, want %v", reviewCycleDefaults, want)
	}
	for level, months := range want {
		if reviewCycleDefaults[level] != months {
			t.Errorf("reviewCycleDefaults[%q]=%d, want %d", level, reviewCycleDefaults[level], months)
		}
	}
}

func TestSupplierCalculateNextReview(t *testing.T) {
	tests := []struct {
		name        string
		criticality string
		cycles      map[string]int
		lastReview  *Epoch
		wantMonths  int // months to add to the base date for the expected result
	}{
		{name: "nil defaults critical", criticality: "critical", cycles: nil, wantMonths: 1},
		{name: "nil defaults high", criticality: "high", cycles: nil, wantMonths: 3},
		{name: "nil defaults medium", criticality: "medium", cycles: nil, wantMonths: 6},
		{name: "nil defaults low", criticality: "low", cycles: nil, wantMonths: 12},
		{name: "custom org map honoured", criticality: "critical", cycles: map[string]int{"critical": 2}, wantMonths: 2},
		{name: "level missing from partial map falls back to 12", criticality: "high", cycles: map[string]int{"critical": 2}, wantMonths: 12},
		{name: "unknown criticality falls back to 12", criticality: "", cycles: nil, wantMonths: 12},
		{name: "garbage criticality falls back to 12", criticality: "urgent", cycles: nil, wantMonths: 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := time.Now()
			s := &Supplier{Criticality: tt.criticality}
			s.CalculateNextReview(tt.cycles)

			want := base.AddDate(0, tt.wantMonths, 0).Format("2006-01-02")
			got := s.NextReview.Time.Format("2006-01-02")
			if got != want {
				t.Errorf("NextReview=%s, want %s (base %s + %d months)", got, want, base.Format("2006-01-02"), tt.wantMonths)
			}
		})
	}
}

func TestSupplierCalculateNextReviewBaseDate(t *testing.T) {
	t.Run("base is last_review when set", func(t *testing.T) {
		last := NewEpoch(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC))
		s := &Supplier{Criticality: "low", LastReview: &last}
		s.CalculateNextReview(nil)

		want := "2025-01-15"
		got := s.NextReview.Time.Format("2006-01-02")
		if got != want {
			t.Errorf("NextReview=%s, want %s", got, want)
		}
	})

	t.Run("base is now when last_review is zero", func(t *testing.T) {
		base := time.Now()
		zero := &Epoch{}
		s := &Supplier{Criticality: "low", LastReview: zero}
		s.CalculateNextReview(nil)

		want := base.AddDate(0, 12, 0).Format("2006-01-02")
		got := s.NextReview.Time.Format("2006-01-02")
		if got != want {
			t.Errorf("NextReview=%s, want %s (base now, not zero last_review)", got, want)
		}
	})
}
