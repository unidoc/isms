package db

import (
	"testing"
	"time"
)

// #308: system review cycle length used to be a hard-coded per-criticality
// switch with no org override. It is now driven by an org-resolved
// map[string]int, exactly like Supplier.CalculateNextReview (#43). nil means
// "use the built-in 1/3/6/12 defaults", a level missing from a partial org map
// or an unknown/empty criticality falls back to 12, and last_review (when set
// and non-zero) is the base date instead of now().

func TestSystemCalculateNextReview(t *testing.T) {
	tests := []struct {
		name        string
		criticality string
		cycles      map[string]int
		wantMonths  int // months to add to the base date for the expected result
	}{
		{name: "nil defaults critical", criticality: "critical", cycles: nil, wantMonths: 1},
		{name: "nil defaults high", criticality: "high", cycles: nil, wantMonths: 3},
		{name: "nil defaults medium", criticality: "medium", cycles: nil, wantMonths: 6},
		{name: "nil defaults low", criticality: "low", cycles: nil, wantMonths: 12},
		{name: "custom org map honoured", criticality: "high", cycles: map[string]int{"high": 2}, wantMonths: 2},
		{name: "custom org map honoured for critical", criticality: "critical", cycles: map[string]int{"critical": 2}, wantMonths: 2},
		{name: "level missing from partial map falls back to 12", criticality: "high", cycles: map[string]int{"critical": 2}, wantMonths: 12},
		{name: "unknown criticality falls back to 12", criticality: "", cycles: nil, wantMonths: 12},
		{name: "garbage criticality falls back to 12", criticality: "urgent", cycles: nil, wantMonths: 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := time.Now()
			sys := &System{Criticality: tt.criticality}
			sys.CalculateNextReview(tt.cycles)

			want := base.AddDate(0, tt.wantMonths, 0).Format("2006-01-02")
			got := sys.NextReview.Time.Format("2006-01-02")
			if got != want {
				t.Errorf("NextReview=%s, want %s (base %s + %d months)", got, want, base.Format("2006-01-02"), tt.wantMonths)
			}
		})
	}
}

func TestSystemCalculateNextReviewBaseDate(t *testing.T) {
	t.Run("base is last_review when set", func(t *testing.T) {
		last := NewEpoch(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC))
		sys := &System{Criticality: "low", LastReview: &last}
		sys.CalculateNextReview(nil)

		want := "2025-01-15"
		got := sys.NextReview.Time.Format("2006-01-02")
		if got != want {
			t.Errorf("NextReview=%s, want %s", got, want)
		}
	})

	t.Run("configured cycle counts from last_review", func(t *testing.T) {
		last := NewEpoch(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC))
		sys := &System{Criticality: "high", LastReview: &last}
		sys.CalculateNextReview(map[string]int{"high": 2})

		want := "2024-03-15"
		got := sys.NextReview.Time.Format("2006-01-02")
		if got != want {
			t.Errorf("NextReview=%s, want %s", got, want)
		}
	})

	t.Run("base is now when last_review is zero", func(t *testing.T) {
		base := time.Now()
		sys := &System{Criticality: "low", LastReview: &Epoch{}}
		sys.CalculateNextReview(nil)

		want := base.AddDate(0, 12, 0).Format("2006-01-02")
		got := sys.NextReview.Time.Format("2006-01-02")
		if got != want {
			t.Errorf("NextReview=%s, want %s (base now, not zero last_review)", got, want)
		}
	})
}
