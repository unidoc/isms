package api

import (
	"testing"
	"time"
)

var (
	mondayEarly = time.Date(2026, 10, 26, 7, 59, 0, 0, time.UTC)
	mondayOpen  = time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	wednesday   = time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
)

func TestDigestPeriod(t *testing.T) {
	for _, tc := range []struct {
		now    time.Time
		period string
		due    bool
	}{
		{mondayEarly, "2026-W44", false},
		{mondayOpen, "2026-W44", true},
		{wednesday, "2026-W44", true},
		{time.Date(2026, 11, 1, 23, 59, 0, 0, time.UTC), "2026-W44", true}, // Sunday
		{time.Date(2026, 11, 2, 7, 59, 0, 0, time.UTC), "2026-W45", false},
		{time.Date(2026, 12, 31, 12, 0, 0, 0, time.UTC), "2026-W53", true},                    // Thursday, ISO year with 53 weeks
		{time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC), "2026-W53", true},                      // Friday: ISO year differs from calendar year
		{time.Date(2026, 10, 26, 8, 0, 0, 0, time.FixedZone("x", 2*3600)), "2026-W44", false}, // 06:00 UTC
	} {
		period, due := digestPeriod(tc.now)
		if period != tc.period || due != tc.due {
			t.Errorf("digestPeriod(%s) = %s, %v, want %s, %v", tc.now.Format(time.RFC3339), period, due, tc.period, tc.due)
		}
	}
}
