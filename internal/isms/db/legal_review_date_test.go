package db

import (
	"testing"
	"time"
)

func TestLegalRequirementCalculateReviewDateAnchor(t *testing.T) {
	lastReview := NewEpoch(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC))

	tests := []struct {
		name string
		lr   LegalRequirement
		want string
	}{
		{
			name: "last_review with no level defaults to 12 months",
			lr:   LegalRequirement{LastReview: &lastReview},
			want: "2025-01-15",
		},
		{
			name: "last_review with critical level uses the critical cycle",
			lr:   LegalRequirement{LastReview: &lastReview, CurrentLevel: "critical"},
			want: "2024-02-15",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.lr.CalculateReviewDate(nil)
			if tc.lr.NextReview == nil {
				t.Fatal("NextReview is nil")
			}
			if got := tc.lr.NextReview.Time.UTC().Format("2006-01-02"); got != tc.want {
				t.Errorf("next_review = %s, want %s", got, tc.want)
			}
		})
	}

	t.Run("no last_review counts from now", func(t *testing.T) {
		var lr LegalRequirement
		lr.CalculateReviewDate(nil)
		if lr.NextReview == nil {
			t.Fatal("NextReview is nil")
		}
		want := time.Now().AddDate(0, 12, 0)
		if diff := lr.NextReview.Time.Sub(want); diff < -time.Minute || diff > time.Minute {
			t.Errorf("next_review = %v, want within a minute of %v", lr.NextReview.Time, want)
		}
	})
}
