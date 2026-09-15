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
		wantStore string // expected stored value; "" means the key is cleared/reverted
	}{
		{"plain valid value", "supplier_review_cycle_critical", "6", "6"},
		{"surrounding whitespace is trimmed", "supplier_review_cycle_high", " 6 ", "6"},
		{"empty reverts to the registry default", "supplier_review_cycle_medium", "", ""},
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
		})
	}
}
