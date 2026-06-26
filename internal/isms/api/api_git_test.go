package api

import "testing"

func TestReviewRefViolation(t *testing.T) {
	const master = "refs/heads/master"
	const reviewRef = "refs/heads/review/2"
	const archived = "refs/reviews/5"

	// Fast-forward iff new == "ff" (test stand-in for "old is an ancestor of new").
	ffOnly := func(old, new string) (bool, error) { return new == "ff", nil }

	cases := []struct {
		name          string
		before, after map[string]string
		wantViolation bool
	}{
		{
			name:   "fast-forward of a normal branch is allowed",
			before: map[string]string{master: "a"},
			after:  map[string]string{master: "ff"},
		},
		{
			name:   "unchanged refs are allowed",
			before: map[string]string{master: "a", reviewRef: "r"},
			after:  map[string]string{master: "a", reviewRef: "r"},
		},
		{
			name:          "non-fast-forward (history rewrite) is rejected",
			before:        map[string]string{master: "a"},
			after:         map[string]string{master: "rewritten"},
			wantViolation: true,
		},
		{
			name:          "changing a review/* ref via push is rejected",
			before:        map[string]string{master: "a", reviewRef: "r"},
			after:         map[string]string{master: "a", reviewRef: "ff"},
			wantViolation: true,
		},
		{
			name:          "creating a review/* ref via push is rejected",
			before:        map[string]string{master: "a"},
			after:         map[string]string{master: "a", reviewRef: "new"},
			wantViolation: true,
		},
		{
			name:          "writing an archived refs/reviews/* ref is rejected",
			before:        map[string]string{master: "a"},
			after:         map[string]string{master: "a", archived: "x"},
			wantViolation: true,
		},
		{
			name:          "deleting a ref is rejected",
			before:        map[string]string{master: "a", reviewRef: "r"},
			after:         map[string]string{master: "a"},
			wantViolation: true,
		},
	}

	for _, tc := range cases {
		got := reviewRefViolation(tc.before, tc.after, ffOnly)
		if tc.wantViolation && got == "" {
			t.Errorf("%s: expected a violation, got none", tc.name)
		}
		if !tc.wantViolation && got != "" {
			t.Errorf("%s: expected no violation, got %q", tc.name, got)
		}
	}
}
