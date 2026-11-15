package db

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEpochUnmarshalJSONStrictTypes(t *testing.T) {
	for _, in := range []string{`true`, `{}`, `[]`} {
		var e Epoch
		if err := json.Unmarshal([]byte(in), &e); err == nil {
			t.Errorf("%s: want error, got nil", in)
		}
	}
	for _, in := range []string{`1893456000`, `"2030-01-01"`, `""`, `null`} {
		var e Epoch
		if err := json.Unmarshal([]byte(in), &e); err != nil {
			t.Errorf("%s: unexpected error %v", in, err)
		}
	}
}

// withLocalZone swaps time.Local for the test and restores it afterwards. CI
// runs in UTC, which hid #503: a test has to force a non-UTC host itself. Tests
// that call this must not be parallel.
func withLocalZone(t *testing.T, name string, offsetSeconds int) {
	t.Helper()
	prev := time.Local
	time.Local = time.FixedZone(name, offsetSeconds)
	t.Cleanup(func() { time.Local = prev })
}

var hostZones = []struct {
	name   string
	offset int
}{
	{"west", -5 * 3600},
	{"east", 7 * 3600},
}

// 1767225600 is 2026-01-01T00:00:00Z.
func TestEpochUnmarshalNumberIsUTCOnAnyHost(t *testing.T) {
	for _, z := range hostZones {
		t.Run(z.name, func(t *testing.T) {
			withLocalZone(t, z.name, z.offset)
			var e Epoch
			if err := json.Unmarshal([]byte(`1767225600`), &e); err != nil {
				t.Fatal(err)
			}
			if got := e.Time.Format("2006-01-02"); got != "2026-01-01" {
				t.Errorf("calendar day of decoded epoch = %s, want 2026-01-01", got)
			}
			if e.Time.Location() != time.UTC {
				t.Errorf("decoded location = %v, want UTC", e.Time.Location())
			}
		})
	}
}

func TestEpochValueIsUTC(t *testing.T) {
	for _, z := range hostZones {
		t.Run(z.name, func(t *testing.T) {
			withLocalZone(t, z.name, z.offset)
			// 23:30 local on Jan 1 is a different UTC day on both hosts.
			local := time.Date(2026, 1, 1, 23, 30, 0, 0, time.Local)
			v, err := NewEpoch(local).Value()
			if err != nil {
				t.Fatal(err)
			}
			got, ok := v.(time.Time)
			if !ok {
				t.Fatalf("Value() = %T, want time.Time", v)
			}
			if got.Location() != time.UTC {
				t.Errorf("Value() location = %v, want UTC", got.Location())
			}
			if !got.Equal(local) {
				t.Errorf("Value() = %v, not the same instant as %v", got, local)
			}
		})
	}
}

func TestEpochStringForms(t *testing.T) {
	withLocalZone(t, "west", -5*3600)
	cases := []struct{ in, day string }{
		{`"2026-01-01"`, "2026-01-01"},
		// An offset moves the instant, and calendar dates are UTC dates: this
		// used to store 2026-01-01 and now stores the UTC day.
		{`"2026-01-01T00:00:00+07:00"`, "2025-12-31"},
		{`"2026-01-01T00:00:00Z"`, "2026-01-01"},
	}
	for _, c := range cases {
		var e Epoch
		if err := json.Unmarshal([]byte(c.in), &e); err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		v, err := e.Value()
		if err != nil {
			t.Fatal(err)
		}
		if got := v.(time.Time).Format("2006-01-02"); got != c.day {
			t.Errorf("%s: stored day = %s, want %s", c.in, got, c.day)
		}
	}
}
