package api

import (
	"testing"
	"time"

	"isms.sh/internal/isms/db"
)

// TestRequestedNextReview pins which update requests count as choosing a
// next_review (#202). An echo of the stored date must not count: clients that
// GET a record and PUT the whole body back would otherwise pin a stale date.
func TestRequestedNextReview(t *testing.T) {
	stored := db.NewEpoch(time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC))
	sameDayLater := db.NewEpoch(time.Date(2027, 3, 1, 15, 30, 0, 0, time.UTC))
	other := db.NewEpoch(time.Date(2031, 2, 14, 0, 0, 0, 0, time.UTC))
	var nullDate *db.Epoch

	ptr := func(e db.Epoch) **db.Epoch { p := &e; return &p }

	cases := []struct {
		name   string
		sent   **db.Epoch
		stored *db.Epoch
		want   string
	}{
		{"absent", nil, &stored, ""},
		{"null", &nullDate, &stored, ""},
		{"echo of the stored date", ptr(stored), &stored, ""},
		{"same day, different time", ptr(sameDayLater), &stored, ""},
		{"a different date", ptr(other), &stored, "2031-02-14"},
		{"a date when nothing is stored", ptr(other), nil, "2031-02-14"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := requestedNextReview(tc.sent, tc.stored)
			gotStr := ""
			if got != nil {
				gotStr = got.Time.UTC().Format("2006-01-02")
			}
			if gotStr != tc.want {
				t.Fatalf("requestedNextReview = %q, want %q", gotStr, tc.want)
			}
		})
	}
}
