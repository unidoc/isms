package db

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// #213: risk categories became per-org data stored as a JSON array in the
// risk_categories setting. ParseRiskCategories is the single gate in front of
// that value — it is used both by the admin settings PUT (which turns a failure
// into a 400) and by RiskCategoriesFor (which turns a failure into "use the
// defaults"). Every constraint it enforces is load-bearing: a payload that slips
// through here becomes the allowed-set for risk create/update org-wide.

func TestParseRiskCategoriesValid(t *testing.T) {
	raw := `[{"key":"people_process","label":"People & Process"},{"key":"cloud_saas","label":"  Cloud / SaaS  "}]`
	got, err := ParseRiskCategories(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2 (%+v)", len(got), got)
	}
	if got[0] != (RiskCategory{Key: "people_process", Label: "People & Process"}) {
		t.Errorf("got[0]=%+v, want people_process/People & Process", got[0])
	}
	// Labels are trimmed on the way in — the stored value is what pickers render,
	// so leading/trailing whitespace must not survive a round-trip.
	if got[1] != (RiskCategory{Key: "cloud_saas", Label: "Cloud / SaaS"}) {
		t.Errorf("got[1]=%+v, want cloud_saas/\"Cloud / SaaS\" (label trimmed)", got[1])
	}
	// Order is preserved: the admin editor's ordering is the display ordering.
	if got[0].Key != "people_process" || got[1].Key != "cloud_saas" {
		t.Errorf("input order not preserved: %+v", got)
	}
}

func TestParseRiskCategoriesRejects(t *testing.T) {
	// 51 entries — one over the cap.
	tooMany := make([]RiskCategory, 51)
	for i := range tooMany {
		tooMany[i] = RiskCategory{Key: "cat_" + string(rune('a'+i%26)) + strings.Repeat("x", i), Label: "L"}
	}
	tooManyJSON, err := json.Marshal(tooMany)
	if err != nil {
		t.Fatalf("marshalling fixture: %v", err)
	}

	longKey := strings.Repeat("k", 65)    // 65 > 64
	longLabel := strings.Repeat("é", 101) // 101 runes > 100 (bytes would be 202)
	okKey := strings.Repeat("k", 64)      // exactly at the cap — must pass
	okLabel := strings.Repeat("é", 100)   // exactly at the cap — must pass

	cases := []struct {
		name string
		raw  string
	}{
		{"malformed JSON", `[{"key":"a","label":`},
		{"not an array", `{"key":"a","label":"A"}`},
		{"empty string", ``},
		{"empty array", `[]`},
		{"null", `null`},
		{"more than 50 entries", string(tooManyJSON)},
		{"uppercase key", `[{"key":"People_Process","label":"People"}]`},
		{"key with spaces", `[{"key":"people process","label":"People"}]`},
		{"leading underscore key", `[{"key":"_people","label":"People"}]`},
		{"trailing underscore key", `[{"key":"people_","label":"People"}]`},
		{"double underscore key", `[{"key":"people__process","label":"People"}]`},
		{"hyphenated key", `[{"key":"people-process","label":"People"}]`},
		{"empty key", `[{"key":"","label":"People"}]`},
		{"missing key", `[{"label":"People"}]`},
		{"empty label", `[{"key":"people","label":""}]`},
		{"whitespace-only label", `[{"key":"people","label":"   \t "}]`},
		{"missing label", `[{"key":"people"}]`},
		{"over-long key", `[{"key":"` + longKey + `","label":"People"}]`},
		{"over-long label", `[{"key":"people","label":"` + longLabel + `"}]`},
		{"duplicate keys", `[{"key":"people","label":"A"},{"key":"people","label":"B"}]`},
		// Keys are compared case-insensitively even though uppercase keys are
		// rejected outright — the dedupe must not be the only thing standing
		// between two rows that would collide after normalisation.
		{"duplicate keys differing only in case", `[{"key":"people","label":"A"},{"key":"PEOPLE","label":"B"}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRiskCategories(tc.raw)
			if err == nil {
				t.Fatalf("expected an error, got %+v", got)
			}
			if got != nil {
				t.Errorf("expected nil categories alongside the error, got %+v", got)
			}
		})
	}

	// Boundary values that must be accepted, so the caps are off-by-one-proof.
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"key exactly 64 chars", `[{"key":"` + okKey + `","label":"K"}]`},
		{"label exactly 100 runes", `[{"key":"people","label":"` + okLabel + `"}]`},
		{"exactly 50 entries", string(mustJSON(t, tooMany[:50]))},
		{"digits in key", `[{"key":"iso_27001_2022","label":"ISO 27001"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseRiskCategories(tc.raw); err != nil {
				t.Errorf("expected acceptance, got error: %v", err)
			}
		})
	}
}

func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshalling fixture: %v", err)
	}
	return b
}

func TestDefaultRiskCategories(t *testing.T) {
	got := DefaultRiskCategories()
	if len(got) != 8 {
		t.Fatalf("len=%d, want the 8 built-in categories", len(got))
	}
	// The default list must itself satisfy the validator — otherwise an admin
	// could never save a lightly-edited copy of it back.
	raw := mustJSON(t, got)
	if _, err := ParseRiskCategories(string(raw)); err != nil {
		t.Errorf("defaults fail their own validation: %v", err)
	}
	// Keys must stay in lockstep with the legacy RiskCategories slice: risks
	// already store these strings, so a rename here is a silent data migration.
	if len(RiskCategories) != len(got) {
		t.Fatalf("RiskCategories has %d keys, DefaultRiskCategories has %d", len(RiskCategories), len(got))
	}
	for i, c := range got {
		if c.Key != RiskCategories[i] {
			t.Errorf("key %d: DefaultRiskCategories=%q, RiskCategories=%q", i, c.Key, RiskCategories[i])
		}
		if strings.TrimSpace(c.Label) == "" {
			t.Errorf("key %q has an empty label", c.Key)
		}
	}
}

// TestRiskCategoriesForFallsBackOnReadError covers the fallback that keeps a
// broken setting from bricking the risk register: when the setting cannot be
// read, RiskCategoriesFor must return the defaults and NO error to the caller
// (riskCategoryKeys discards the error, so a non-nil error would silently
// produce an empty allowed-set and reject every category).
//
// The pool is built against an address nothing listens on. pgxpool.New is lazy,
// so construction succeeds and the failure lands on the query — exactly the
// shape of a real read failure. This does not reach the network beyond a
// connection-refused on loopback.
func TestRiskCategoriesForFallsBackOnReadError(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/nodb?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("building pool: %v", err)
	}
	defer pool.Close()
	d := &DB{pool: pool}

	cats, err := d.RiskCategoriesFor(context.Background(), 1)
	if err != nil {
		t.Fatalf("RiskCategoriesFor returned an error to the caller: %v", err)
	}
	if len(cats) == 0 {
		t.Fatal("returned an empty slice — callers use this as the allowed-set")
	}
	want := DefaultRiskCategories()
	if len(cats) != len(want) {
		t.Fatalf("len=%d, want %d defaults", len(cats), len(want))
	}
	for i := range want {
		if cats[i] != want[i] {
			t.Errorf("cats[%d]=%+v, want %+v", i, cats[i], want[i])
		}
	}
}
