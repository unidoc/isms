package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// #290: GetOrgSetting and GetOrgSettings resolve with
// COALESCE(os.value, s.default_value, empty-string). Before this fix,
// clearing a setting through the admin settings endpoint called
// SetOrgSetting(ctx, orgID, key, "") for every key except the
// supplier/risk review cycles, which stores a real empty-string row. An
// empty string is non-NULL, so COALESCE never falls through to
// s.default_value: the setting is pinned to empty forever
// instead of reverting to the registry default. The fix generalises the
// existing empty-means-delete branch (previously review-cycle only) to
// every key in handleAdminUpdateSetting.
//
// Run with: ISMS_TEST_DATABASE_URL=postgres://... go test ./internal/isms/api/... -run TestAdminClearSetting -v
//
// Each case asserts three things, not just the resolved value, because a
// 200 response and GetOrgSetting()=="" are not enough proof: SetOrgSetting("")
// would ALSO produce both for a NULL-default key, and would produce a wrong
// non-default value for a non-NULL-default key. The row count is the only
// thing that actually distinguishes "reverted to default" from "stored an
// explicit empty string", so every case checks organization_settings
// directly in addition to the two read paths.
func TestAdminClearSettingRevertsToDefault(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "setting-clear")

	// Masking guard (from the issue): keep a different key configured while
	// each case below clears its own key, so GetOrgSettings never resolves
	// against an empty organization_settings table. If deleting a row for
	// one key accidentally deleted rows for other keys too, this would still
	// have something to catch it on.
	putSetting(t, s, orgID, "task_default_private", "true")

	for _, tc := range []struct {
		name      string
		key       string
		setVal    string // stored first, to prove the row existed before clearing
		clearVal  string // then this value is sent to clear it
		wantValue string // expected value from GetOrgSetting/GetOrgSettings after clearing
	}{
		// The headline regression: a non-NULL default that used to be shadowed
		// by a stored '' row.
		{"risk_appetite: non-NULL default was shadowed", "risk_appetite", "5", "", "9"},
		{"show_powered_by: boolean default was shadowed", "show_powered_by", "false", "", "true"},
		// NULL-default keys: the row is gone either way, but this pins that
		// the generalised branch doesn't regress them.
		{"branding_footer: NULL default, row still removed", "branding_footer", "Acme", "", ""},
		// Sensitive key: previously stored as an encrypted empty string; now
		// there is simply no row.
		{"slack_webhook: sensitive key, row still removed", "slack_webhook", "https://hooks.example.test/x", "", ""},
		// Whitespace-only counts as cleared too (strings.TrimSpace in the fix).
		{"risk_appetite: whitespace-only also clears", "risk_appetite", "5", "   ", "9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			putSetting(t, s, orgID, tc.key, tc.setVal)
			got, err := s.db.GetOrgSetting(context.Background(), orgID, tc.key)
			if err != nil {
				t.Fatalf("GetOrgSetting after set: %v", err)
			}
			if got != tc.setVal {
				t.Fatalf("stored value=%q, want %q (the set didn't take, clearing wouldn't prove anything)", got, tc.setVal)
			}

			putSetting(t, s, orgID, tc.key, tc.clearVal)

			got, err = s.db.GetOrgSetting(context.Background(), orgID, tc.key)
			if err != nil {
				t.Fatalf("GetOrgSetting after clear: %v", err)
			}
			if got != tc.wantValue {
				t.Errorf("GetOrgSetting(%s)=%q after clearing, want %q", tc.key, got, tc.wantValue)
			}

			var count int
			if err := s.db.Pool().QueryRow(context.Background(),
				`SELECT count(*) FROM organization_settings WHERE organization_id = $1 AND setting_key = $2`,
				orgID, tc.key).Scan(&count); err != nil {
				t.Fatalf("querying organization_settings: %v", err)
			}
			if count != 0 {
				t.Errorf("organization_settings row for %s still present after clearing (count=%d)", tc.key, count)
			}

			settings, err := s.db.GetOrgSettings(context.Background(), orgID)
			if err != nil {
				t.Fatalf("GetOrgSettings: %v", err)
			}
			found := false
			for _, os := range settings {
				if os.Key == tc.key {
					found = true
					if os.Value != tc.wantValue {
						t.Errorf("GetOrgSettings entry for %s=%q, want %q", tc.key, os.Value, tc.wantValue)
					}
					break
				}
			}
			if !found {
				t.Fatalf("GetOrgSettings: no entry for key %q", tc.key)
			}
		})
	}

	// The masking guard itself: task_default_private must still be exactly
	// what it was set to, with its row intact, after every case above
	// cleared a different key.
	got, err := s.db.GetOrgSetting(context.Background(), orgID, "task_default_private")
	if err != nil {
		t.Fatalf("GetOrgSetting(task_default_private): %v", err)
	}
	if got != "true" {
		t.Errorf("task_default_private=%q after unrelated clears, want %q (masking guard failed)", got, "true")
	}
	var count int
	if err := s.db.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM organization_settings WHERE organization_id = $1 AND setting_key = $2`,
		orgID, "task_default_private").Scan(&count); err != nil {
		t.Fatalf("querying organization_settings for task_default_private: %v", err)
	}
	if count != 1 {
		t.Errorf("organization_settings row count for task_default_private=%d, want 1 (masking guard failed)", count)
	}
}

// putSetting PUTs {key, value} through handleAdminUpdateSetting and fails
// the test on any error or non-200 response. JSON-encoded rather than
// string-concatenated because setting values here contain characters
// (slashes, colons) that would need escaping in a hand-built body.
func putSetting(t *testing.T, s *Server, orgID int, key, value string) {
	t.Helper()

	body, err := json.Marshal(map[string]string{"key": key, "value": value})
	if err != nil {
		t.Fatalf("marshaling request body for %s: %v", key, err)
	}

	c, rec := ctxForPath(orgID, http.MethodPut, "/api/v1/admin/settings", string(body), "admin")
	if err := s.handleAdminUpdateSetting(c); err != nil {
		t.Fatalf("handleAdminUpdateSetting(%s=%q): unexpected error %v", key, value, err)
	}
	if rec.Code != 0 && rec.Code != http.StatusOK {
		t.Fatalf("handleAdminUpdateSetting(%s=%q): status=%d, want 200", key, value, rec.Code)
	}
}
