package db

import (
	"context"
	"testing"
)

// Requires a migrated Postgres — see notifications_roundtrip_test.go's header
// for how to stand one up. Skipped when ISMS_TEST_DATABASE_URL is unset.
//
// Proves the custom_fields JSONB column round-trips through CreateRisk /
// GetRisk / UpdateRisk, not just that ValidateCustomFieldValues decides
// correctly in memory — riskSelectCols/scanRisk is a positional scan, so a
// column added out of order is exactly the class of bug that compiles and
// passes every unit test while corrupting reads.
func TestRiskCustomFieldsRoundTrip(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "riskcustomfields")

	r := &Risk{
		Title:    "Vendor risk",
		RiskType: "threat",
		Origin:   "external",
		Owner:    email,
		Status:   "open",
		CustomFields: map[string]any{
			"vendor": "Acme",
			"cost":   float64(500),
		},
	}
	if err := d.CreateRisk(ctx, orgID, r); err != nil {
		t.Fatalf("CreateRisk: %v", err)
	}

	got, err := d.GetRisk(ctx, orgID, r.ID)
	if err != nil {
		t.Fatalf("GetRisk: %v", err)
	}
	if got.CustomFields["vendor"] != "Acme" {
		t.Errorf("vendor = %v, want Acme", got.CustomFields["vendor"])
	}
	if got.CustomFields["cost"] != float64(500) {
		t.Errorf("cost = %v, want 500", got.CustomFields["cost"])
	}
	if got.Title != "Vendor risk" {
		t.Errorf("unrelated field Title corrupted: got %q", got.Title)
	}

	// Update one value, confirm the JSONB persisted and unrelated fields survived.
	updated := *got
	updated.CustomFields = map[string]any{"vendor": "Acme", "cost": float64(750)}
	updated.Notes = "reviewed"
	if err := d.UpdateRisk(ctx, orgID, &updated); err != nil {
		t.Fatalf("UpdateRisk: %v", err)
	}

	after, err := d.GetRisk(ctx, orgID, r.ID)
	if err != nil {
		t.Fatalf("GetRisk after update: %v", err)
	}
	if after.CustomFields["cost"] != float64(750) {
		t.Errorf("cost after update = %v, want 750", after.CustomFields["cost"])
	}
	if after.CustomFields["vendor"] != "Acme" {
		t.Errorf("vendor after update = %v, want Acme (unchanged)", after.CustomFields["vendor"])
	}
	if after.Notes != "reviewed" {
		t.Errorf("Notes = %q, want reviewed", after.Notes)
	}
	if after.Title != "Vendor risk" {
		t.Errorf("Title corrupted after update: got %q", after.Title)
	}
}
