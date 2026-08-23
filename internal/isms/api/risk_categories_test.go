package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// #213: risk categories are per-org. These tests cover the parts of the API
// layer that are reachable without a Postgres instance — this repo has no
// DB-backed test harness, so anything that reads or writes organization_settings
// is exercised at the boundary only (see the notes on each test).

// newSettingsContext builds an echo context for handleAdminUpdateSetting with
// the org id where getOrgID looks for it.
func newSettingsContext(body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("org_id", 1)
	c.Set("user_email", "admin@example.com")
	return c, rec
}

// TestAdminUpdateSettingRejectsMalformedRiskCategories: the settings PUT used to
// accept arbitrary text for any key, so an admin could store a blob that broke
// risk create/edit org-wide. The handler must now reject a malformed
// risk_categories payload before it reaches SetOrgSetting.
//
// The server is built with a nil *db.DB on purpose: reaching the store would
// panic, so a clean 400 is positive proof the stored value was never touched.
func TestAdminUpdateSettingRejectsMalformedRiskCategories(t *testing.T) {
	s := &Server{}

	for _, tc := range []struct {
		name string
		body string
	}{
		{"truncated JSON", `{"key":"risk_categories","value":"[{\"key\":\"a\","}`},
		{"not an array", `{"key":"risk_categories","value":"{\"key\":\"a\",\"label\":\"A\"}"}`},
		{"empty array", `{"key":"risk_categories","value":"[]"}`},
		{"uppercase key", `{"key":"risk_categories","value":"[{\"key\":\"Bad\",\"label\":\"Bad\"}]"}`},
		{"blank label", `{"key":"risk_categories","value":"[{\"key\":\"ok\",\"label\":\"  \"}]"}`},
		{"duplicate keys differing in case", `{"key":"risk_categories","value":"[{\"key\":\"ok\",\"label\":\"A\"},{\"key\":\"OK\",\"label\":\"B\"}]"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newSettingsContext(tc.body)
			err := s.handleAdminUpdateSetting(c)
			if err == nil {
				t.Fatal("expected a 400, got nil error (value would have been stored)")
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

// A missing key is still rejected up front, and an unrelated key is NOT run
// through the risk-category validator (that would break every other setting).
// The unrelated-key case must not be added here: it falls through to
// SetOrgSetting and needs a real DB.
func TestAdminUpdateSettingRequiresKey(t *testing.T) {
	s := &Server{}
	c, _ := newSettingsContext(`{"key":"","value":"whatever"}`)
	err := s.handleAdminUpdateSetting(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("expected a 400 HTTPError, got %T %v", err, err)
	}
}

// TestRiskCategoryAllowedSetSemantics locks in the validation contract shared by
// risk create (server.go), risk update (server.go) and suggestion apply
// (api_suggestions.go). All three call validateEnum with the keys of the org's
// configured categories, which is what riskCategoryKeys returns; the allowed-set
// is built here directly because riskCategoryKeys needs a live DB.
func TestRiskCategoryAllowedSetSemantics(t *testing.T) {
	// Unset setting → RiskCategoriesFor falls back to the defaults, so the
	// allowed-set is the 8 built-in keys.
	defaults := keysOf(db.DefaultRiskCategories())

	for _, tc := range []struct {
		name    string
		allowed []string
		value   string
		wantErr bool
	}{
		{"default category with the setting unset", defaults, "technology", false},
		{"another default category", defaults, "quality_operations", false},
		{"empty category is always accepted (uncategorised)", defaults, "", false},
		{"custom key rejected while the setting is unset", defaults, "cloud_saas", true},
		{"unknown key rejected", defaults, "not_a_category", true},
		{"case mismatch rejected — keys are exact", defaults, "Technology", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEnum("category", tc.value, tc.allowed)
			assertEnum(t, err, tc.wantErr)
		})
	}

	// Once a custom list is configured, its keys are the allowed-set and the old
	// defaults are no longer selectable (removal orphans, it does not cascade).
	custom, err := db.ParseRiskCategories(`[{"key":"cloud_saas","label":"Cloud / SaaS"},{"key":"people_process","label":"Our People"}]`)
	if err != nil {
		t.Fatalf("fixture failed to parse: %v", err)
	}
	customKeys := keysOf(custom)

	for _, tc := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"configured custom key accepted", "cloud_saas", false},
		{"retained default key accepted", "people_process", false},
		{"empty still accepted", "", false},
		{"default key dropped from the custom list is rejected", "technology", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertEnum(t, validateEnum("category", tc.value, customKeys), tc.wantErr)
		})
	}
}

func keysOf(cats []db.RiskCategory) []string {
	keys := make([]string, 0, len(cats))
	for _, c := range cats {
		keys = append(keys, c.Key)
	}
	return keys
}

func assertEnum(t *testing.T, err error, wantErr bool) {
	t.Helper()
	if wantErr {
		he, ok := err.(*echo.HTTPError)
		if !ok {
			t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
		}
		if he.Code != http.StatusBadRequest {
			t.Errorf("status=%d, want 400", he.Code)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
