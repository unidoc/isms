package db

import (
	"testing"
)

// #213/#216: risk custom fields mirror the risk_categories mechanism —
// ParseCustomFieldDefs is the single gate in front of the risk_custom_fields
// setting, used both by the admin settings PUT (failure -> 400) and by
// CustomFieldDefsFor (failure -> empty slice, never blocking risk creation).

func TestParseCustomFieldDefsValid(t *testing.T) {
	raw := `[{"key":"vendor","label":"Vendor","type":"text"},{"key":"severity","label":"Severity","type":"select","required":true,"options":["Low","High"]}]`
	got, err := ParseCustomFieldDefs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2 (%+v)", len(got), got)
	}
	if got[1].Required != true || len(got[1].Options) != 2 {
		t.Errorf("got[1]=%+v, want required=true, 2 options", got[1])
	}
}

func TestParseCustomFieldDefsEmptyArrayAccepted(t *testing.T) {
	got, err := ParseCustomFieldDefs(`[]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len=%d, want 0", len(got))
	}
}

func TestParseCustomFieldDefsMalformedJSON(t *testing.T) {
	if _, err := ParseCustomFieldDefs(`not json`); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseCustomFieldDefsNonArray(t *testing.T) {
	if _, err := ParseCustomFieldDefs(`{"key":"x"}`); err == nil {
		t.Fatal("expected error for non-array")
	}
}

func TestParseCustomFieldDefsOverCap(t *testing.T) {
	raw := `[`
	for i := 0; i < maxCustomFields+1; i++ {
		if i > 0 {
			raw += ","
		}
		raw += `{"key":"f` + itoa(i) + `","label":"F","type":"text"}`
	}
	raw += `]`
	if _, err := ParseCustomFieldDefs(raw); err == nil {
		t.Fatal("expected error for over cap")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestParseCustomFieldDefsBadSlug(t *testing.T) {
	cases := []string{
		`[{"key":"Vendor","label":"Vendor","type":"text"}]`, // uppercase
		`[{"key":"vendor name","label":"V","type":"text"}]`, // spaces
		`[{"key":"_vendor","label":"V","type":"text"}]`,     // leading underscore
	}
	for _, raw := range cases {
		if _, err := ParseCustomFieldDefs(raw); err == nil {
			t.Errorf("expected error for %s", raw)
		}
	}
}

func TestParseCustomFieldDefsOverLongKeyLabel(t *testing.T) {
	longKey := ""
	for i := 0; i < maxCustomFieldKey+1; i++ {
		longKey += "a"
	}
	if _, err := ParseCustomFieldDefs(`[{"key":"` + longKey + `","label":"L","type":"text"}]`); err == nil {
		t.Error("expected error for over-long key")
	}
	longLabel := ""
	for i := 0; i < maxCustomFieldLabel+1; i++ {
		longLabel += "a"
	}
	if _, err := ParseCustomFieldDefs(`[{"key":"k","label":"` + longLabel + `","type":"text"}]`); err == nil {
		t.Error("expected error for over-long label")
	}
}

func TestParseCustomFieldDefsBlankLabel(t *testing.T) {
	if _, err := ParseCustomFieldDefs(`[{"key":"k","label":"  ","type":"text"}]`); err == nil {
		t.Error("expected error for blank label")
	}
}

func TestParseCustomFieldDefsUnknownType(t *testing.T) {
	if _, err := ParseCustomFieldDefs(`[{"key":"k","label":"L","type":"bogus"}]`); err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestParseCustomFieldDefsSelectNoOptions(t *testing.T) {
	if _, err := ParseCustomFieldDefs(`[{"key":"k","label":"L","type":"select"}]`); err == nil {
		t.Error("expected error for select with no options")
	}
}

func TestParseCustomFieldDefsSelectBadOptions(t *testing.T) {
	cases := []string{
		`[{"key":"k","label":"L","type":"select","options":[""]}]`,      // blank option
		`[{"key":"k","label":"L","type":"select","options":["A","a"]}]`, // duplicate (case-insensitive)
	}
	for _, raw := range cases {
		if _, err := ParseCustomFieldDefs(raw); err == nil {
			t.Errorf("expected error for %s", raw)
		}
	}
	longOpt := ""
	for i := 0; i < maxCustomFieldOption+1; i++ {
		longOpt += "a"
	}
	if _, err := ParseCustomFieldDefs(`[{"key":"k","label":"L","type":"select","options":["` + longOpt + `"]}]`); err == nil {
		t.Error("expected error for over-long option")
	}
}

func TestParseCustomFieldDefsNonSelectWithOptions(t *testing.T) {
	if _, err := ParseCustomFieldDefs(`[{"key":"k","label":"L","type":"text","options":["A"]}]`); err == nil {
		t.Error("expected error: options only allowed for select")
	}
}

func TestParseCustomFieldDefsDuplicateKeysCaseInsensitive(t *testing.T) {
	if _, err := ParseCustomFieldDefs(`[{"key":"vendor","label":"A","type":"text"},{"key":"VENDOR","label":"B","type":"text"}]`); err == nil {
		t.Error("expected error for duplicate key differing only in case")
	}
}

func TestParseCustomFieldDefsAcceptedBoundaries(t *testing.T) {
	key := ""
	for i := 0; i < maxCustomFieldKey; i++ {
		key += "a"
	}
	label := ""
	for i := 0; i < maxCustomFieldLabel; i++ {
		label += "a"
	}
	raw := `[{"key":"` + key + `","label":"` + label + `","type":"text"}]`
	if _, err := ParseCustomFieldDefs(raw); err != nil {
		t.Errorf("unexpected error at boundary: %v", err)
	}
}

func TestValidateCustomFieldValues(t *testing.T) {
	defs := []CustomFieldDef{
		{Key: "vendor", Label: "Vendor", Type: "text"},
		{Key: "cost", Label: "Cost", Type: "number"},
		{Key: "due", Label: "Due", Type: "date"},
		{Key: "severity", Label: "Severity", Type: "select", Required: true, Options: []string{"Low", "High"}},
	}

	if err := ValidateCustomFieldValues(defs, map[string]any{"unknown": "x"}, false); err == nil {
		t.Error("expected error for unknown key")
	}

	// happy paths
	values := map[string]any{
		"vendor":   "Acme",
		"cost":     float64(100),
		"due":      "2026-01-01",
		"severity": "High",
	}
	if err := ValidateCustomFieldValues(defs, values, true); err != nil {
		t.Errorf("unexpected error on happy path: %v", err)
	}

	// wrong type per type
	if err := ValidateCustomFieldValues(defs, map[string]any{"cost": "100"}, false); err == nil {
		t.Error("expected error: numeric string rejected for number")
	}
	if err := ValidateCustomFieldValues(defs, map[string]any{"due": "not-a-date"}, false); err == nil {
		t.Error("expected error: bad date string")
	}
	if err := ValidateCustomFieldValues(defs, map[string]any{"severity": "Medium"}, false); err == nil {
		t.Error("expected error: off-list select value")
	}

	// empty/nil allowed for non-required field
	if err := ValidateCustomFieldValues(defs, map[string]any{"vendor": ""}, false); err != nil {
		t.Errorf("unexpected error: empty string should be allowed: %v", err)
	}
	if err := ValidateCustomFieldValues(defs, map[string]any{"vendor": nil}, false); err != nil {
		t.Errorf("unexpected error: nil should be allowed: %v", err)
	}

	// checkRequired=true, required field missing/empty rejected
	if err := ValidateCustomFieldValues(defs, map[string]any{}, true); err == nil {
		t.Error("expected error: required field missing")
	}
	if err := ValidateCustomFieldValues(defs, map[string]any{"severity": ""}, true); err == nil {
		t.Error("expected error: required field empty")
	}
	// present accepted
	if err := ValidateCustomFieldValues(defs, map[string]any{"severity": "Low"}, true); err != nil {
		t.Errorf("unexpected error: required field present: %v", err)
	}

	// checkRequired=false never rejects a missing required field (suggestion-apply carve-out)
	if err := ValidateCustomFieldValues(defs, map[string]any{}, false); err != nil {
		t.Errorf("unexpected error: checkRequired=false must never reject a missing required field: %v", err)
	}
}

func TestNormalizeCustomFieldValues(t *testing.T) {
	defs := []CustomFieldDef{{Key: "vendor", Label: "Vendor", Type: "text"}}
	in := map[string]any{
		"vendor":  "  Acme  ",
		"cleared": nil,
		"empty":   "",
		"orphan":  "still here", // no matching def
	}
	out := NormalizeCustomFieldValues(defs, in)
	if out == nil {
		t.Fatal("must never return nil")
	}
	if v, ok := out["vendor"]; !ok || v != "Acme" {
		t.Errorf("vendor=%v, want trimmed Acme", v)
	}
	if _, ok := out["cleared"]; ok {
		t.Error("nil value must be dropped")
	}
	if _, ok := out["empty"]; ok {
		t.Error("empty string must be dropped")
	}
	if v, ok := out["orphan"]; !ok || v != "still here" {
		t.Errorf("orphan key must survive even with no matching def, got %v (ok=%v)", v, ok)
	}
}

func TestRequiredCustomFieldsSatisfiedIgnoresOrphanKeys(t *testing.T) {
	// Regression: a risk holding a value for a field whose definition was since
	// deleted (orphan-not-cascade) must not fail a required-only check just
	// because that orphan key has no matching def. RequiredCustomFieldsSatisfied
	// is used for exactly this over the whole merged map in handleUpdateRisk,
	// separately from the unknown-key check ValidateCustomFieldValues does over
	// changed values only.
	defs := []CustomFieldDef{
		{Key: "priority", Label: "Priority", Type: "text", Required: true},
	}
	values := map[string]any{
		"priority":       "High",
		"reviewer_notes": "orphaned, no def anymore",
	}
	if err := RequiredCustomFieldsSatisfied(defs, values); err != nil {
		t.Fatalf("unexpected error with orphan key present: %v", err)
	}
}

func TestRequiredCustomFieldsSatisfiedRejectsMissing(t *testing.T) {
	defs := []CustomFieldDef{
		{Key: "priority", Label: "Priority", Type: "text", Required: true},
	}
	if err := RequiredCustomFieldsSatisfied(defs, map[string]any{}); err == nil {
		t.Fatal("expected error for missing required field")
	}
	if err := RequiredCustomFieldsSatisfied(defs, map[string]any{"priority": "  "}); err == nil {
		t.Fatal("expected error for blank required field")
	}
}

func TestCustomFieldDefsForFallback(t *testing.T) {
	// No DB wired up here; exercise the parse-failure fallback path directly
	// via ParseCustomFieldDefs, which CustomFieldDefsFor delegates to for the
	// "stored value is invalid" branch.
	if _, err := ParseCustomFieldDefs(`not json`); err == nil {
		t.Fatal("expected parse error")
	}
	// The DB-backed fallback (empty slice, nil error) is exercised by the
	// Postgres round-trip tests, which are gated on ISMS_TEST_DATABASE_URL.
}
