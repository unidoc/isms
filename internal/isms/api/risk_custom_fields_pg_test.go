package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Postgres-gated handler tests for risk custom fields (#213/#216). Requires a
// migrated Postgres — see id_resolution_pg_test.go's testServer for setup.
// Skipped when ISMS_TEST_DATABASE_URL is unset.

func ctxForPath(orgID int, method, path, body, role string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("org_id", orgID)
	c.Set("user_role", role)
	c.Set("user_email", "admin@custom-fields.test")
	return c, rec
}

func setCustomFieldDefs(t *testing.T, s *Server, orgID int, raw string) {
	t.Helper()
	if err := s.db.SetOrgSetting(context.Background(), orgID, "risk_custom_fields", raw); err != nil {
		t.Fatalf("seeding risk_custom_fields: %v", err)
	}
}

func TestHandleListRiskCustomFieldsReachableByNonAdmin(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "cf-list")

	c, rec := ctxForPath(orgID, http.MethodGet, "/api/v1/risks/custom-fields", "", "reader")
	if err := s.handleListRiskCustomFields(c); err != nil {
		t.Fatalf("handleListRiskCustomFields: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	var got []db.CustomFieldDef
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected [] when unset, got %+v", got)
	}
}

func TestSettingsPutRejectsMalformedCustomFields(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "cf-malformed")

	c, rec := ctxForPath(orgID, http.MethodPut, "/api/v1/admin/settings",
		`{"key":"risk_custom_fields","value":"not json"}`, "admin")
	err := s.handleAdminUpdateSetting(c)
	if err == nil {
		t.Fatal("expected an error for malformed risk_custom_fields payload")
	}
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Errorf("err = %v, want *echo.HTTPError 400", err)
	}
	_ = rec

	// Stored value must remain unchanged (empty/unset).
	raw, _ := s.db.GetOrgSetting(context.Background(), orgID, "risk_custom_fields")
	if strings.TrimSpace(raw) != "" {
		t.Errorf("stored value changed despite rejected PUT: %q", raw)
	}
}

func TestCreateRiskWithUnknownCustomFieldKeyRejected(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "cf-create-unknown")
	setCustomFieldDefs(t, s, orgID, `[{"key":"vendor","label":"Vendor","type":"text"}]`)

	body := `{"title":"R","owner":"admin@custom-fields.test","risk_type":"threat","origin":"internal","current_likelihood":3,"current_impact":3,"custom_fields":{"nope":"x"}}`
	c, rec := ctxForPath(orgID, http.MethodPost, "/api/v1/risks", body, "admin")
	err := s.handleAddRisk(c)
	if err == nil {
		t.Fatal("expected an error for an unknown custom field key")
	}
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Errorf("err = %v, want *echo.HTTPError 400", err)
	}
	_ = rec
}

// The regression that matters most: editing a risk whose select option was
// later removed must still succeed. Validating unchanged values would make
// such a risk permanently uneditable.
func TestEditRiskWithOrphanedSelectValueSucceeds(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "cf-orphan-edit")
	setCustomFieldDefs(t, s, orgID, `[{"key":"severity","label":"Severity","type":"select","options":["Low","High"]}]`)

	risk := &db.Risk{
		Title: "Orphan test", RiskType: "threat", Origin: "internal",
		Owner: "admin@custom-fields.test", Status: "open",
		CustomFields: map[string]any{"severity": "Low"},
	}
	if err := s.db.CreateRisk(ctx, orgID, risk); err != nil {
		t.Fatalf("CreateRisk: %v", err)
	}

	// The org also defines a required field after the risk was created, and
	// removes "Low" as an option on severity — only "High" remains valid now.
	// Both together reproduce the real edit-form shape: the whole custom_fields
	// object PUT back unchanged, including the now-orphaned "severity" value,
	// alongside a newly-required field that must still be checked.
	setCustomFieldDefs(t, s, orgID, `[
		{"key":"severity","label":"Severity","type":"select","options":["High"]},
		{"key":"priority","label":"Priority","type":"text","required":true}
	]`)

	// PUT the whole form back unchanged except notes, exactly like the web edit
	// form does — custom_fields is resent verbatim, including the orphaned
	// "severity" value and satisfying the newly-required "priority" field.
	body := `{"notes":"edited","custom_fields":{"severity":"Low","priority":"P1"}}`
	c, rec := ctxForPath(orgID, http.MethodPut, "/api/v1/risks/"+risk.Identifier, body, "admin")
	c.SetParamNames("id")
	c.SetParamValues(risk.Identifier)
	if err := s.handleUpdateRisk(c); err != nil {
		t.Fatalf("handleUpdateRisk must succeed on an orphaned (unchanged) select value: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}

	after, err := s.db.GetRisk(ctx, orgID, risk.ID)
	if err != nil {
		t.Fatalf("GetRisk: %v", err)
	}
	if after.CustomFields["severity"] != "Low" {
		t.Errorf("orphaned value must survive the edit, got %v", after.CustomFields["severity"])
	}
	if after.Notes != "edited" {
		t.Errorf("Notes = %q, want edited", after.Notes)
	}
}

func TestSuggestionApplyWithInvalidCustomValueRejectedNoRiskCreated(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "cf-suggestion-invalid")
	setCustomFieldDefs(t, s, orgID, `[{"key":"severity","label":"Severity","type":"select","options":["Low","High"]}]`)

	before, err := s.db.ListRisks(ctx, orgID)
	if err != nil {
		t.Fatalf("ListRisks before: %v", err)
	}
	countBefore := len(before)

	payload := []byte(`{"title":"Bad risk","custom_fields":{"severity":"Medium"}}`)
	sg := &db.Suggestion{OrganizationID: orgID, EntityType: "risk", SuggestionType: "create", Payload: payload}

	tx, err := s.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	_, _, applyErr := applyRiskCreate(ctx, tx, s, orgID, sg, "admin@custom-fields.test")
	if applyErr == nil {
		t.Fatal("expected an error for an invalid custom field value")
	}

	after, err := s.db.ListRisks(ctx, orgID)
	if err != nil {
		t.Fatalf("ListRisks after: %v", err)
	}
	if len(after) != countBefore {
		t.Errorf("risk count changed from %d to %d — a row was created despite the rejected apply", countBefore, len(after))
	}
}

// Agents cannot fill out a form, so applyRiskCreate deliberately skips the
// required-field check (checkRequired=false). A suggestion that omits a
// required custom field must still succeed.
func TestSuggestionApplyRiskCreateSucceedsWithMissingRequiredField(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "cf-suggestion-missing-required")
	setCustomFieldDefs(t, s, orgID, `[{"key":"priority","label":"Priority","type":"text","required":true}]`)

	payload := []byte(`{"title":"Agent-created risk","custom_fields":{}}`)
	sg := &db.Suggestion{OrganizationID: orgID, EntityType: "risk", SuggestionType: "create", Payload: payload}

	tx, err := s.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	identifier, _, applyErr := applyRiskCreate(ctx, tx, s, orgID, sg, "admin@custom-fields.test")
	if applyErr != nil {
		t.Fatalf("applyRiskCreate must succeed despite a missing required custom field: %v", applyErr)
	}
	if identifier == "" {
		t.Error("expected a risk identifier to be returned")
	}
}
