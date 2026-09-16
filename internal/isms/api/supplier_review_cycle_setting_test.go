package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

// #43: the settings PUT used to accept any text for supplier_review_cycle_*
// (and risk_review_cycle_*), so a typo silently degraded to the 12-month
// fallback with no signal (§1.7). The guard is shared between both prefixes,
// so one risk_review_cycle_* case is included to pin that.

// TestAdminUpdateSettingRejectsBadReviewCycle covers the reject path, which
// needs no DB: the guard runs before SetOrgSetting is reached, and a nil
// *db.DB proves the bad value was never touched (same technique as
// TestAdminUpdateSettingRejectsMalformedRiskCategories).
func TestAdminUpdateSettingRejectsBadReviewCycle(t *testing.T) {
	s := &Server{}

	for _, tc := range []struct {
		name string
		key  string
		val  string
	}{
		{"zero", "supplier_review_cycle_critical", "0"},
		{"negative", "supplier_review_cycle_high", "-3"},
		{"not a number", "supplier_review_cycle_medium", "abc"},
		{"fractional", "supplier_review_cycle_low", "1.5"},
		{"over the 120 ceiling", "supplier_review_cycle_critical", "999"},
		{"shared guard also covers risk_review_cycle_", "risk_review_cycle_high", "abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"key":"` + tc.key + `","value":"` + tc.val + `"}`
			c, _ := newSettingsContext(body)
			err := s.handleAdminUpdateSetting(c)
			if err == nil {
				t.Fatalf("expected a 400 for %s=%q, got nil error (value would have been stored)", tc.key, tc.val)
			}
			he, ok := err.(*echo.HTTPError)
			if !ok {
				t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
			}
			if he.Code != http.StatusBadRequest {
				t.Errorf("status=%d, want 400 (%v)", he.Code, he.Message)
			}
		})
	}
}

// TestAdminUpdateSettingAcceptsValidReviewCycle covers the accept path, which
// needs a real DB because it falls through to SetOrgSetting. Requires
// ISMS_TEST_DATABASE_URL — skipped otherwise, via testServer(t).
func TestAdminUpdateSettingAcceptsValidReviewCycle(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "supplier-review-cycle-setting")

	for _, tc := range []struct {
		name      string
		key       string
		val       string
		wantStore string // expected value from GetOrgSetting (registry default when cleared)
		wantNoRow bool   // when true, assert no organization_settings row exists for the key
	}{
		{"plain valid value", "supplier_review_cycle_critical", "6", "6", false},
		{"surrounding whitespace is trimmed", "supplier_review_cycle_high", " 6 ", "6", false},
		// By this point critical and high are already configured above, so the
		// review-cycle map is non-empty — this is the masking condition from
		// #43: if the fix regressed to storing '' instead of deleting the row,
		// GetOrgSetting would return "" (not the registry default "6") but the
		// old assertion style (comparing only to "") would not have caught
		// that regression, since it also happened to expect "". Comparing to
		// the actual registry default value catches it either way.
		{"empty reverts to the registry default", "supplier_review_cycle_medium", "", "6", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"key":"` + tc.key + `","value":"` + tc.val + `"}`
			c, rec := ctxForPath(orgID, http.MethodPut, "/api/v1/admin/settings", body, "admin")
			if err := s.handleAdminUpdateSetting(c); err != nil {
				t.Fatalf("handleAdminUpdateSetting(%s=%q): unexpected error %v", tc.key, tc.val, err)
			}
			if rec.Code != 0 && rec.Code != http.StatusOK {
				t.Errorf("status=%d, want 200", rec.Code)
			}
			got, err := s.db.GetOrgSetting(context.Background(), orgID, tc.key)
			if err != nil {
				t.Fatalf("GetOrgSetting: %v", err)
			}
			if got != tc.wantStore {
				t.Errorf("stored value=%q, want %q", got, tc.wantStore)
			}
			if tc.wantNoRow {
				// A 200 response and GetOrgSetting()=="" are not enough proof:
				// SetOrgSetting("") would ALSO produce both, because
				// COALESCE(os.value, s.default_value, '') can't tell an
				// explicitly-empty row from a missing one (that's the bug).
				// Assert the row itself is gone.
				var count int
				err := s.db.Pool().QueryRow(context.Background(),
					`SELECT count(*) FROM organization_settings WHERE organization_id = $1 AND setting_key = $2`,
					orgID, tc.key).Scan(&count)
				if err != nil {
					t.Fatalf("querying organization_settings: %v", err)
				}
				if count != 0 {
					t.Errorf("organization_settings row for %s still present after clearing (count=%d); "+
						"SetOrgSetting(\"\") stores a real empty-string row that COALESCE never falls through, "+
						"masking the registry default forever", tc.key, count)
				}
			}
		})
	}
}
