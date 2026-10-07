package db

import "testing"

func TestChangeStatusClears(t *testing.T) {
	cases := []struct {
		status      string
		approval    bool
		implemented bool
	}{
		{"proposed", true, true},
		{"rejected", true, true},
		// approved stamps a fresh approval rather than clearing one.
		{"approved", false, true},
		{"in_progress", false, false},
		{"implemented", false, false},
		{"closed", false, false},
	}
	if len(cases) != len(ChangeStatuses) {
		t.Fatalf("table covers %d statuses, ChangeStatuses has %d", len(cases), len(ChangeStatuses))
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			approval, implemented := ChangeStatusClears(tc.status)
			if approval != tc.approval || implemented != tc.implemented {
				t.Errorf("ChangeStatusClears(%q) = (%v, %v), want (%v, %v)",
					tc.status, approval, implemented, tc.approval, tc.implemented)
			}
		})
	}
}
