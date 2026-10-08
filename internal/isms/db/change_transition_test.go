package db

import "testing"

// TestChangeStatusTransitionAllowed pins the full adjacency table (#423): one
// row per (from, to) pair across every combination of ChangeStatuses, so a
// change to the table is a visible diff here rather than a silent widening or
// narrowing of what the API accepts.
func TestChangeStatusTransitionAllowed(t *testing.T) {
	allowed := map[[2]string]bool{
		{"proposed", "approved"}: true,
		{"proposed", "rejected"}: true,
		{"proposed", "closed"}:   true, // withdrawn before any decision

		{"approved", "in_progress"}: true,
		{"approved", "implemented"}: true, // skip in_progress
		{"approved", "rejected"}:    true,
		{"approved", "proposed"}:    true, // sent back, approval withdrawn (#197)

		{"in_progress", "implemented"}: true,
		{"in_progress", "rejected"}:    true,
		{"in_progress", "proposed"}:    true, // sent back, approval withdrawn (#197)

		{"implemented", "closed"}:      true,
		{"implemented", "in_progress"}: true, // reopened; implemented_at and approval survive (#197)
	}

	for _, from := range ChangeStatuses {
		for _, to := range ChangeStatuses {
			t.Run(from+"->"+to, func(t *testing.T) {
				want := from == to || allowed[[2]string{from, to}]
				got := ChangeStatusTransitionAllowed(from, to)
				if got != want {
					t.Errorf("ChangeStatusTransitionAllowed(%q, %q) = %v, want %v", from, to, got, want)
				}
			})
		}
	}
}

// TestChangeStatusTransitionAllowedSameStatusAlwaysOK pins that a no-op
// "transition" is never rejected, even from a terminal status — closing an
// already-closed change, or re-approving an already-approved one, must not
// 409 just because nothing in the adjacency table lists it.
func TestChangeStatusTransitionAllowedSameStatusAlwaysOK(t *testing.T) {
	for _, s := range ChangeStatuses {
		if !ChangeStatusTransitionAllowed(s, s) {
			t.Errorf("ChangeStatusTransitionAllowed(%q, %q) = false, want true (same-status is always a no-op)", s, s)
		}
	}
}

// TestChangeStatusTransitionAllowedTerminalStatuses pins that rejected and
// closed have no way out through this function — not even to each other, and
// not back to any earlier status. An admin wanting to undo one files a new
// change request instead (see the PR description for #423: this is a
// deliberate, disclosed trade-off, not an oversight).
func TestChangeStatusTransitionAllowedTerminalStatuses(t *testing.T) {
	for _, from := range []string{"rejected", "closed"} {
		for _, to := range ChangeStatuses {
			if to == from {
				continue
			}
			if ChangeStatusTransitionAllowed(from, to) {
				t.Errorf("ChangeStatusTransitionAllowed(%q, %q) = true, want false (%q is terminal)", from, to, from)
			}
		}
	}
}
