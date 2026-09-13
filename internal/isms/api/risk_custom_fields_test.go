package api

import (
	"testing"
)

// changedCustomFields backs the "validate only changed entries" rule in
// handleUpdateRisk (#213/#216) — see riskCategoryNeedsValidation for the
// category-level version of the same reasoning.

func TestChangedCustomFieldsDetectsRealChanges(t *testing.T) {
	prev := map[string]any{"vendor": "Acme", "cost": float64(10)}
	next := map[string]any{"vendor": "Other", "cost": float64(10)}
	got := changedCustomFields(prev, next)
	if _, ok := got["vendor"]; !ok {
		t.Error("vendor changed and must be reported")
	}
	if _, ok := got["cost"]; ok {
		t.Error("cost is unchanged and must not be reported")
	}
}

// The round-trip trap: a float64(3) written by Go code and a JSON-decoded 3
// (also float64 in Go, but this test guards the fmt.Sprint comparison path
// generically) must NOT count as changed.
func TestChangedCustomFieldsNoFalsePositiveOnNumberRoundTrip(t *testing.T) {
	prev := map[string]any{"cost": float64(3)}
	next := map[string]any{"cost": float64(3)}
	got := changedCustomFields(prev, next)
	if len(got) != 0 {
		t.Errorf("expected no changes, got %+v", got)
	}
}

func TestChangedCustomFieldsNewKeyCounted(t *testing.T) {
	prev := map[string]any{}
	next := map[string]any{"vendor": "Acme"}
	got := changedCustomFields(prev, next)
	if _, ok := got["vendor"]; !ok {
		t.Error("a key present in next but absent from prev must be reported as changed")
	}
}
