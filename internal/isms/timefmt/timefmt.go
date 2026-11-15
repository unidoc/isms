// Package timefmt formats db.Epoch values for people. A calendar date (a DATE
// column such as a due or review date) is a UTC date and is shown on its UTC
// day in every zone; a timestamp (a TIMESTAMPTZ column such as created_at) is
// an instant and is shown in the viewer's local zone (#503).
package timefmt

import (
	"time"

	"isms.sh/internal/isms/db"
)

// Epoch is the set of types the helpers accept: a value or a pointer.
type Epoch interface{ db.Epoch | *db.Epoch }

func toTime[E Epoch](e E) (time.Time, bool) {
	var t time.Time
	switch v := any(e).(type) {
	case db.Epoch:
		t = v.Time
	case *db.Epoch:
		if v == nil {
			return t, false
		}
		t = v.Time
	}
	return t, !t.IsZero()
}

// Day formats a DATE column (due, review, planned, expiry dates) as the UTC
// calendar day, 2006-01-02. Empty for nil or zero. Never use it for a
// timestamp.
func Day[E Epoch](e E) string {
	t, ok := toTime(e)
	if !ok {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// LocalDay formats a TIMESTAMPTZ column (created, started, completed) as the
// viewer's local date, 2006-01-02. Empty for nil or zero.
func LocalDay[E Epoch](e E) string {
	t, ok := toTime(e)
	if !ok {
		return ""
	}
	return t.Local().Format("2006-01-02")
}

// LocalTime formats a TIMESTAMPTZ column as the viewer's local date and time,
// 2006-01-02 15:04. Empty for nil or zero.
func LocalTime[E Epoch](e E) string {
	t, ok := toTime(e)
	if !ok {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}
