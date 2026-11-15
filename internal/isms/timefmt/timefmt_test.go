package timefmt

import (
	"testing"
	"time"

	"isms.sh/internal/isms/db"
)

func withZone(t *testing.T, offset int) {
	t.Helper()
	prev := time.Local
	time.Local = time.FixedZone("host", offset)
	t.Cleanup(func() { time.Local = prev })
}

var zones = []struct {
	name   string
	offset int
}{{"west", -5 * 3600}, {"east", 7 * 3600}}

func TestDayIsTheUTCDayInAnyZone(t *testing.T) {
	for _, z := range zones {
		t.Run(z.name, func(t *testing.T) {
			withZone(t, z.offset)
			// As the API decoder produces it, and as a local-zone time would.
			e := db.NewEpoch(time.Unix(1767225600, 0)) // 2026-01-01 00:00 UTC, in the host zone
			if got := Day(e); got != "2026-01-01" {
				t.Errorf("Day = %q, want 2026-01-01", got)
			}
			if got := Day(&e); got != "2026-01-01" {
				t.Errorf("Day(ptr) = %q, want 2026-01-01", got)
			}
		})
	}
}

func TestLocalTimeIsTheViewersZone(t *testing.T) {
	at := db.NewEpoch(time.Date(2026, 11, 15, 9, 51, 0, 0, time.UTC))
	for _, c := range []struct {
		name   string
		offset int
		time   string
		day    string
	}{
		{"west", -5 * 3600, "2026-11-15 04:51", "2026-11-15"},
		{"east", 7 * 3600, "2026-11-15 16:51", "2026-11-15"},
	} {
		t.Run(c.name, func(t *testing.T) {
			withZone(t, c.offset)
			if got := LocalTime(at); got != c.time {
				t.Errorf("LocalTime = %q, want %q", got, c.time)
			}
			if got := LocalDay(&at); got != c.day {
				t.Errorf("LocalDay = %q, want %q", got, c.day)
			}
		})
	}
	// A timestamp near UTC midnight lands on the local date.
	withZone(t, -5*3600)
	late := db.NewEpoch(time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC))
	if got := LocalDay(late); got != "2025-12-31" {
		t.Errorf("LocalDay near midnight = %q, want 2025-12-31", got)
	}
}

func TestNilAndZeroAreEmpty(t *testing.T) {
	var nilEpoch *db.Epoch
	var zero db.Epoch
	for name, got := range map[string]string{
		"Day nil": Day(nilEpoch), "Day zero": Day(zero), "Day zero ptr": Day(&zero),
		"LocalDay nil": LocalDay(nilEpoch), "LocalDay zero": LocalDay(zero),
		"LocalTime nil": LocalTime(nilEpoch), "LocalTime zero": LocalTime(zero),
	} {
		if got != "" {
			t.Errorf("%s = %q, want empty", name, got)
		}
	}
}

func TestIsPastDueCountsOnTheUTCDay(t *testing.T) {
	due := func(day string) db.Epoch {
		d, err := time.Parse("2006-01-02", day)
		if err != nil {
			t.Fatal(err)
		}
		return db.NewEpoch(d)
	}
	lateToday := time.Date(2026, 11, 15, 23, 30, 0, 0, time.UTC)
	justAfter := time.Date(2026, 11, 16, 0, 10, 0, 0, time.UTC)
	for _, z := range zones {
		t.Run(z.name, func(t *testing.T) {
			withZone(t, z.offset)
			// The host's own clock reads differently from UTC; the answer must not.
			if isPastDueAt(due("2026-11-15"), lateToday.In(time.Local)) {
				t.Error("due today (UTC) reported past due")
			}
			if !isPastDueAt(due("2026-11-14"), lateToday.In(time.Local)) {
				t.Error("due yesterday (UTC) not reported past due")
			}
			if !isPastDueAt(due("2026-11-15"), justAfter.In(time.Local)) {
				t.Error("due day ended 10 minutes ago, not reported past due")
			}
			d := due("2026-11-15")
			if isPastDueAt(&d, lateToday.In(time.Local)) {
				t.Error("pointer: due today reported past due")
			}
		})
	}
	var nilEpoch *db.Epoch
	if IsPastDue(nilEpoch) || IsPastDue(db.Epoch{}) {
		t.Error("nil or zero reported past due")
	}
}
