package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// #522: the four public supplier fields (public_name, public_purpose,
// processing_location, data_categories) go through every writer: POST, PUT and
// suggestion apply store the same trimmed values, a PUT that omits them leaves
// them alone, and reviews/readings never blank them or move
// trust_public_changed_at.
//
// Requires a migrated Postgres; skipped otherwise:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...

const publicFieldsActor = "admin@supplier-public.test"

func putSupplier(t *testing.T, s *Server, orgID int, id int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	c, rec := ctxFor(orgID, http.MethodPut, fmt.Sprintf("%d", id), body)
	c.Set("user_email", publicFieldsActor)
	if err := s.handleUpdateSupplier(c); err != nil {
		return errRecorder(err)
	}
	return rec
}

// errRecorder turns a handler's returned echo error into a recorder so tests
// can assert the status the client would see.
func errRecorder(err error) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	if he, ok := err.(*echo.HTTPError); ok {
		rec.Code = he.Code
		rec.Body.WriteString(fmt.Sprint(he.Message))
		return rec
	}
	rec.Code = http.StatusInternalServerError
	rec.Body.WriteString(err.Error())
	return rec
}

func publicFieldsOf(sp *db.Supplier) [4]string {
	return [4]string{sp.PublicName, sp.PublicPurpose, sp.ProcessingLocation, sp.DataCategories}
}

func newPublicFieldsServer(t *testing.T, name string) (*Server, int) {
	t.Helper()
	s := testServer(t)
	orgID := newTestOrg(t, s, name)
	contractTestUser(t, s, orgID, publicFieldsActor, "admin")
	return s, orgID
}

func TestSupplierPutStoresPublicFields(t *testing.T) {
	s, orgID := newPublicFieldsServer(t, "supplier-public-put")
	ctx := context.Background()
	sup := &db.Supplier{Name: "Acme", SupplierType: "cloud", Criticality: "low", Status: "active"}
	if err := s.db.CreateSupplier(ctx, orgID, sup); err != nil {
		t.Fatal(err)
	}

	rec := putSupplier(t, s, orgID, sup.ID, `{"public_name":"  Acme GmbH ","public_purpose":" Cloud hosting ","processing_location":"EU (Frankfurt)","data_categories":"Name, email address"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"public_name": "Acme GmbH", "public_purpose": "Cloud hosting", "processing_location": "EU (Frankfurt)", "data_categories": "Name, email address"} {
		if resp[k] != want {
			t.Errorf("PUT response %s = %v, want %q", k, resp[k], want)
		}
	}
	if _, leaked := resp["trust_public_changed_at"]; leaked {
		t.Error("trust_public_changed_at must not be in the supplier JSON")
	}
	want := [4]string{"Acme GmbH", "Cloud hosting", "EU (Frankfurt)", "Name, email address"}
	got, _ := s.db.GetSupplier(ctx, orgID, sup.ID)
	if publicFieldsOf(got) != want {
		t.Fatalf("stored %v, want %v", publicFieldsOf(got), want)
	}

	// Stored as typed: no fallback of public_name to name in the stored value.
	if got.Name != "Acme" {
		t.Errorf("name changed to %q", got.Name)
	}

	// A PUT without the keys (what an older CLI sends) leaves them unchanged.
	if rec := putSupplier(t, s, orgID, sup.ID, `{"notes":"only notes"}`); rec.Code != http.StatusOK {
		t.Fatalf("partial PUT status = %d: %s", rec.Code, rec.Body.String())
	}
	got, _ = s.db.GetSupplier(ctx, orgID, sup.ID)
	if publicFieldsOf(got) != want || got.Notes != "only notes" {
		t.Fatalf("partial PUT: fields %v notes %q, want %v and the new notes", publicFieldsOf(got), got.Notes, want)
	}

	// "" clears one field and only that one.
	if rec := putSupplier(t, s, orgID, sup.ID, `{"public_purpose":""}`); rec.Code != http.StatusOK {
		t.Fatalf("clear PUT status = %d: %s", rec.Code, rec.Body.String())
	}
	got, _ = s.db.GetSupplier(ctx, orgID, sup.ID)
	if want2 := [4]string{"Acme GmbH", "", "EU (Frankfurt)", "Name, email address"}; publicFieldsOf(got) != want2 {
		t.Fatalf("after clearing purpose: %v, want %v", publicFieldsOf(got), want2)
	}

	// The whole-object round trip an older client does (GET body sent back as PUT).
	c, rec2 := ctxFor(orgID, http.MethodGet, fmt.Sprintf("%d", sup.ID), "")
	if err := s.handleGetSupplier(c); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec2.Body.String(), `"public_name":"Acme GmbH"`) {
		t.Fatalf("GET lacks public_name: %s", rec2.Body.String())
	}
	if rec := putSupplier(t, s, orgID, sup.ID, rec2.Body.String()); rec.Code != http.StatusOK {
		t.Fatalf("round-trip PUT status = %d: %s", rec.Code, rec.Body.String())
	}
	after, _ := s.db.GetSupplier(ctx, orgID, sup.ID)
	if publicFieldsOf(after) != publicFieldsOf(got) {
		t.Fatalf("round trip changed %v -> %v", publicFieldsOf(got), publicFieldsOf(after))
	}
}

func TestSupplierPublicFieldLengthLimits(t *testing.T) {
	s, orgID := newPublicFieldsServer(t, "supplier-public-limits")
	ctx := context.Background()
	sup := &db.Supplier{Name: "Acme", SupplierType: "cloud", Criticality: "low", Status: "active"}
	if err := s.db.CreateSupplier(ctx, orgID, sup); err != nil {
		t.Fatal(err)
	}
	limits := map[string]int{"public_name": 200, "public_purpose": 1000, "processing_location": 200, "data_categories": 1000}
	for field, max := range limits {
		// Characters, not bytes: a multi-byte rune at the limit is fine.
		ok := strings.Repeat("é", max)
		if rec := putSupplier(t, s, orgID, sup.ID, fmt.Sprintf(`{%q:%q}`, field, ok)); rec.Code != http.StatusOK {
			t.Errorf("PUT %s at %d chars: status %d, want 200: %s", field, max, rec.Code, rec.Body.String())
		}
		// Whitespace padding does not count: the value is trimmed first.
		if rec := putSupplier(t, s, orgID, sup.ID, fmt.Sprintf(`{%q:%q}`, field, "  "+ok+"  ")); rec.Code != http.StatusOK {
			t.Errorf("PUT %s padded: status %d, want 200: %s", field, rec.Code, rec.Body.String())
		}
		long := strings.Repeat("x", max+1)
		rec := putSupplier(t, s, orgID, sup.ID, fmt.Sprintf(`{%q:%q}`, field, long))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), field) {
			t.Errorf("PUT %s at %d chars: status %d body %q, want 400 naming the field", field, max+1, rec.Code, rec.Body.String())
		}
		c, crec := ctxForCreate(orgID, fmt.Sprintf(`{"name":"N","supplier_type":"cloud","criticality":"low",%q:%q}`, field, long))
		c.Set("user_email", publicFieldsActor)
		err := s.handleAddSupplier(c)
		if err != nil {
			crec = errRecorder(err)
		}
		if crec.Code != http.StatusBadRequest {
			t.Errorf("POST %s at %d chars: status %d, want 400", field, max+1, crec.Code)
		}
	}
}

func TestSupplierCreateStoresPublicFields(t *testing.T) {
	s, orgID := newPublicFieldsServer(t, "supplier-public-create")
	c, rec := ctxForCreate(orgID, `{"name":"Acme","supplier_type":"cloud","criticality":"low","public_name":" Acme GmbH ","public_purpose":"Hosting","processing_location":"EU","data_categories":"Email"}`)
	c.Set("user_email", publicFieldsActor)
	if err := s.handleAddSupplier(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var out db.Supplier
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if want := [4]string{"Acme GmbH", "Hosting", "EU", "Email"}; publicFieldsOf(&out) != want {
		t.Fatalf("response %v, want %v", publicFieldsOf(&out), want)
	}
	got, _ := s.db.GetSupplier(context.Background(), orgID, out.ID)
	if want := [4]string{"Acme GmbH", "Hosting", "EU", "Email"}; publicFieldsOf(got) != want {
		t.Fatalf("stored %v, want %v", publicFieldsOf(got), want)
	}
}

// Suggestion apply stores exactly what PUT stores (#200 parity).
func TestSupplierSuggestionApplyStoresPublicFields(t *testing.T) {
	s, orgID := newPublicFieldsServer(t, "supplier-public-suggest")
	ctx := context.Background()

	id := applySuggestionID(t, s, orgID, &db.Suggestion{
		EntityType: "supplier", SuggestionType: "create",
		Payload: json.RawMessage(`{"name":"Acme","supplier_type":"cloud","criticality":"low","public_name":" Acme GmbH ","public_purpose":"Hosting","processing_location":"EU","data_categories":"Email"}`),
	})
	var created *db.Supplier
	list, _ := s.db.ListSuppliers(ctx, orgID)
	for i := range list {
		if list[i].Identifier == id {
			created = &list[i]
		}
	}
	if created == nil {
		t.Fatalf("created supplier %q not found", id)
	}
	if want := [4]string{"Acme GmbH", "Hosting", "EU", "Email"}; publicFieldsOf(created) != want {
		t.Fatalf("create via suggestion stored %v, want %v", publicFieldsOf(created), want)
	}

	sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType: "supplier", EntityID: created.Identifier, SuggestionType: "update",
		Payload: json.RawMessage(`{"fields":{"public_name":"  New Ltd ","data_categories":""}}`),
	})
	applySuggestion(t, s, orgID, sg.ID)
	got, _ := s.db.GetSupplier(ctx, orgID, created.ID)
	if want := [4]string{"New Ltd", "Hosting", "EU", ""}; publicFieldsOf(got) != want {
		t.Fatalf("update via suggestion stored %v, want %v", publicFieldsOf(got), want)
	}

	// Over-length is refused at suggestion creation, not stored.
	bad := &db.Suggestion{EntityType: "supplier", EntityID: created.Identifier, SuggestionType: "update",
		Title: "t", SuggestedBy: suggestionChangelogActor,
		Payload: json.RawMessage(fmt.Sprintf(`{"fields":{"public_name":%q}}`, strings.Repeat("x", 201)))}
	sg2 := newTestSuggestion(t, s, orgID, bad)
	c, rec := suggestionCtx(orgID, http.MethodPost, sg2.ID)
	err := s.handleApplyEntitySuggestion(c)
	if err == nil && rec.Code < 400 {
		t.Fatalf("applying an over-length public_name succeeded: %d %s", rec.Code, rec.Body.String())
	}
	got, _ = s.db.GetSupplier(ctx, orgID, created.ID)
	if got.PublicName != "New Ltd" {
		t.Fatalf("over-length apply changed public_name to %q", got.PublicName)
	}
}

// A supplier review and a reading rewrite the supplier row; neither may blank the
// public fields or move the visible-projection timestamp.
func TestSupplierReviewAndReadingKeepPublicFields(t *testing.T) {
	s, orgID := newPublicFieldsServer(t, "supplier-public-controls")
	ctx := context.Background()
	sup := &db.Supplier{Name: "Acme", SupplierType: "cloud", Criticality: "low", Status: "active",
		PublicName: "Acme GmbH", PublicPurpose: "Hosting", ProcessingLocation: "EU", DataCategories: "Email"}
	if err := s.db.CreateSupplier(ctx, orgID, sup); err != nil {
		t.Fatal(err)
	}
	changedAt := func() time.Time {
		var ts time.Time
		if err := s.db.Pool().QueryRow(ctx, `SELECT trust_public_changed_at FROM suppliers WHERE id = $1`, sup.ID).Scan(&ts); err != nil {
			t.Fatal(err)
		}
		return ts
	}
	before := changedAt()
	want := publicFieldsOf(sup)
	time.Sleep(30 * time.Millisecond)

	post := func(handler func(echo.Context) error, body string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", sup.ID))
		c.Set("org_id", orgID)
		c.Set("user_role", "manager")
		c.Set("user_email", publicFieldsActor)
		if err := handler(c); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
	}
	check := func(label string) {
		t.Helper()
		got, _ := s.db.GetSupplier(ctx, orgID, sup.ID)
		if publicFieldsOf(got) != want {
			t.Fatalf("%s: public fields %v, want %v", label, publicFieldsOf(got), want)
		}
		if after := changedAt(); !after.Equal(before) {
			t.Fatalf("%s moved trust_public_changed_at: %v -> %v", label, before, after)
		}
	}

	post(s.handleCreateSupplierReview, `{"outcome":"unsatisfactory","notes":"checked"}`)
	check("review")
	post(s.handleCreateSupplierReading, `{"confidentiality":4,"integrity":3,"availability":5}`)
	check("reading")
}
