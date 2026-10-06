package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// TestReferenceEntityExists is a DB-free unit test: for the sequence-identifier
// types, a bare number is rejected by the shape gate before any lookup runs, so
// a nil receiver is safe to call referenceEntityExists on.
func TestReferenceEntityExists(t *testing.T) {
	var s *Server
	for _, tc := range []struct {
		entityType, entityID string
	}{
		{"change_request", "5"},
		{"task", "7"},
		{"legal_requirement", "5"},
		{"risk", ""},
	} {
		if got := s.referenceEntityExists(context.Background(), 1, db.TaskViewer{}, tc.entityType, tc.entityID); got {
			t.Errorf("referenceEntityExists(%q, %q) = true, want false", tc.entityType, tc.entityID)
		}
	}
}

// ctxForCreateReference builds an echo context for POST /api/v1/references with
// a JSON body, setting the same context keys ctxFor (in id_resolution_pg_test.go)
// sets — that helper hardcodes an asset URL, so this is a small local variant.
func ctxForCreateReference(orgID int, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/references", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("org_id", orgID)
	c.Set("user_role", "admin")
	c.Set("user_email", "admin@references-create.test")
	return c, rec
}

// TestCreateReferenceValidatesIDs is the live-Postgres regression for #341:
// POST /api/v1/references used to accept a target_id that does not resolve
// (e.g. a raw row id, or an identifier from the wrong sequence) and store a
// pair of rows whose title can never resolve.
//
// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays green
// without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...
func TestCreateReferenceValidatesIDs(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "refs-create")

	risk := &db.Risk{Title: "risk for reference create test", RiskType: db.RiskTypes[0], Origin: db.RiskOrigins[0], Status: db.RiskStatuses[0]}
	if err := s.db.CreateRisk(ctx, orgID, risk); err != nil {
		t.Fatalf("CreateRisk: %v", err)
	}
	legal := &db.LegalRequirement{Title: "legal for reference create test", Jurisdiction: "EU", Category: "privacy", Status: "open"}
	if err := s.db.CreateLegalRequirement(ctx, orgID, legal); err != nil {
		t.Fatalf("CreateLegalRequirement: %v", err)
	}

	post := func(sourceType, sourceID, targetType, targetID string) error {
		body, err := json.Marshal(map[string]string{
			"source_type": sourceType,
			"source_id":   sourceID,
			"target_type": targetType,
			"target_id":   targetID,
		})
		if err != nil {
			t.Fatalf("marshaling request body: %v", err)
		}
		c, _ := ctxForCreateReference(orgID, string(body))
		return s.handleCreateReference(c)
	}

	// Case 1: a valid reference between a risk and a legal requirement, both
	// addressed by their per-org identifier, must succeed.
	if err := post("risk", risk.Identifier, "legal_requirement", legal.Identifier); err != nil {
		t.Fatalf("valid reference (risk -> legal_requirement) was rejected: %v", err)
	}
	refs, err := s.db.ListAllReferencesForEntity(ctx, orgID, "risk", risk.Identifier)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity: %v", err)
	}
	if len(refs) == 0 {
		t.Fatalf("expected at least one reference row for risk %s after a valid create", risk.Identifier)
	}
	if title := s.resolveEntityTitle(ctx, orgID, db.TaskViewer{}, "legal_requirement", legal.Identifier); title != legal.Title {
		t.Errorf("resolveEntityTitle(legal_requirement, %s) = %q, want %q", legal.Identifier, title, legal.Title)
	}

	assertRejected := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: expected an error, got none", name)
		}
		var he *echo.HTTPError
		if !errors.As(err, &he) {
			t.Fatalf("%s: error is %T, want *echo.HTTPError: %v", name, err, err)
		}
		if he.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want %d", name, he.Code, http.StatusBadRequest)
		}
	}

	// Case 2: the raw row id instead of the per-org identifier must be rejected.
	assertRejected("raw row id as target_id",
		post("risk", risk.Identifier, "legal_requirement", strconv.FormatInt(legal.ID, 10)))

	// Case 3: an identifier that does not resolve to any row must be rejected.
	assertRejected("nonexistent target identifier",
		post("risk", risk.Identifier, "legal_requirement", "LEGAL-99999"))

	// Case 4: an identifier that does not resolve on the source side must be rejected too.
	assertRejected("nonexistent source identifier",
		post("risk", "RISK-99999", "legal_requirement", legal.Identifier))

	// None of cases 2-4 should have written a row: only the pair from case 1
	// exists — stored as two rows (forward risk->legal, reverse legal->risk),
	// both of which match a lookup by the legal requirement's identifier.
	after, err := s.db.ListAllReferencesForEntity(ctx, orgID, "legal_requirement", legal.Identifier)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity after rejected calls: %v", err)
	}
	if len(after) != 2 {
		t.Errorf("legal requirement %s has %d reference rows after rejected calls, want 2 (only the valid pair from case 1)", legal.Identifier, len(after))
	}
}

// TestValidateReferenceInputs is DB-free, like TestReferenceEntityExists: every
// rejected case fails before any lookup runs, so a nil receiver is safe.
func TestValidateReferenceInputs(t *testing.T) {
	var s *Server
	ctx := context.Background()

	if _, err := s.validateReferenceInputs(ctx, 1, db.TaskViewer{}, nil); err != nil {
		t.Errorf("no references: got %v, want nil", err)
	}
	if _, err := s.validateReferenceInputs(ctx, 1, db.TaskViewer{}, []ReferenceInput{{}}); err != nil {
		t.Errorf("an all-blank entry is skipped: got %v, want nil", err)
	}
	for _, refs := range [][]ReferenceInput{
		{{Type: "legal_requirement", ID: "5"}},
		{{Type: "asset", ID: ""}},
		{{Type: "", ID: "ASSET-1"}},
		{{}, {Type: "risk", ID: "12"}},
	} {
		_, err := s.validateReferenceInputs(ctx, 1, db.TaskViewer{}, refs)
		var he *echo.HTTPError
		if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
			t.Errorf("validateReferenceInputs(%+v) = %v, want a 400", refs, err)
		}
	}
}

// TestCreateEntityValidatesReferences is the live-Postgres regression for #351:
// references sent with a create request used to be stored without checking the
// target exists. A bad one must now fail the request before the entity is
// created. All create handlers share validateReferenceInputs, so one is tested.
func TestCreateEntityValidatesReferences(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "refs-on-create")
	// The HTTP create path now requires the owner (defaulted to the caller) to be an org member.
	seedReviewUser(t, s, orgID, "admin@custom-fields.test", "admin")

	risk := &db.Risk{Title: "risk for create-references test", RiskType: db.RiskTypes[0], Origin: db.RiskOrigins[0], Status: db.RiskStatuses[0]}
	if err := s.db.CreateRisk(ctx, orgID, risk); err != nil {
		t.Fatalf("CreateRisk: %v", err)
	}

	create := func(title, targetID string) error {
		body, err := json.Marshal(map[string]any{
			"title":        title,
			"jurisdiction": "EU",
			"category":     "privacy",
			"references":   []map[string]string{{"type": "risk", "id": targetID}},
		})
		if err != nil {
			t.Fatalf("marshaling request body: %v", err)
		}
		c, _ := ctxForPath(orgID, http.MethodPost, "/api/v1/legal", string(body), "admin")
		return s.handleCreateLegal(c)
	}
	legalCount := func() int {
		t.Helper()
		all, err := s.db.ListLegalRequirements(ctx, orgID, "")
		if err != nil {
			t.Fatalf("ListLegalRequirements: %v", err)
		}
		return len(all)
	}

	for name, targetID := range map[string]string{
		"raw row id":             strconv.FormatInt(risk.ID, 10),
		"nonexistent identifier": "RISK-99999",
	} {
		err := create("rejected: "+name, targetID)
		var he *echo.HTTPError
		if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
			t.Errorf("%s: err = %v, want a 400", name, err)
		}
	}
	if n := legalCount(); n != 0 {
		t.Errorf("%d legal requirements exist after rejected creates, want 0", n)
	}

	if err := create("accepted", risk.Identifier); err != nil {
		t.Fatalf("create with a valid reference was rejected: %v", err)
	}
	if n := legalCount(); n != 1 {
		t.Fatalf("%d legal requirements exist after a valid create, want 1", n)
	}
	refs, err := s.db.ListAllReferencesForEntity(ctx, orgID, "risk", risk.Identifier)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity: %v", err)
	}
	if len(refs) != 2 {
		t.Errorf("risk %s has %d reference rows, want 2 (forward and reverse)", risk.Identifier, len(refs))
	}
}
