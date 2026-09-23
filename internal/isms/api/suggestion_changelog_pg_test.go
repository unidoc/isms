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

// Regression for #334: applying a suggestion wrote its suggestion_applied row
// against the number in the display identifier ("RISK-3" → entity_id 3) instead
// of the entity's primary key, and the stale check read the same wrong entity.
// As in id_resolution_pg_test.go, the bug only shows with two organizations:
// org A's risks take the low primary keys, so org B's RISK-n is never PK n.
//
// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays green
// without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...

const suggestionChangelogActor = "manager@suggestion-changelog.test"

// newSuggestionChangelogOrgs returns org B and its three risks, after giving
// org A two risks of its own so B's identifiers and primary keys disagree.
func newSuggestionChangelogOrgs(t *testing.T, s *Server) (int, []*db.Risk) {
	t.Helper()
	orgA := newTestOrg(t, s, "suggestion-changelog-a")
	orgB := newTestOrg(t, s, "suggestion-changelog-b")
	for i := 0; i < 2; i++ {
		newTestRisk(t, s, orgA, fmt.Sprintf("org A risk %d", i+1))
	}
	var risks []*db.Risk
	for i := 0; i < 3; i++ {
		risks = append(risks, newTestRisk(t, s, orgB, fmt.Sprintf("org B risk %d", i+1)))
	}
	if r := risks[2]; r.Identifier != "RISK-3" || r.ID == 3 {
		t.Fatalf("setup: org B's third risk is %s with id %d — want RISK-3 on a primary key other than 3", r.Identifier, r.ID)
	}
	return orgB, risks
}

func newTestRisk(t *testing.T, s *Server, orgID int, title string) *db.Risk {
	t.Helper()
	likelihood, impact := 2, 3
	r := &db.Risk{
		Title:             title,
		RiskType:          db.RiskTypes[0],
		Origin:            db.RiskOrigins[0],
		Status:            db.RiskStatuses[0],
		Treatment:         "mitigate",
		CurrentLikelihood: &likelihood,
		CurrentImpact:     &impact,
	}
	if err := s.db.CreateRisk(context.Background(), orgID, r); err != nil {
		t.Fatalf("CreateRisk in org %d: %v", orgID, err)
	}
	return r
}

func newTestSuggestion(t *testing.T, s *Server, orgID int, sg *db.Suggestion) *db.Suggestion {
	t.Helper()
	sg.Title = "suggestion-changelog " + sg.SuggestionType
	sg.SuggestedBy = suggestionChangelogActor
	if err := s.db.CreateSuggestion(context.Background(), orgID, sg); err != nil {
		t.Fatalf("CreateSuggestion: %v", err)
	}
	return sg
}

func suggestionCtx(orgID int, method string, id int64) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, "/", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", id))
	c.Set("org_id", orgID)
	c.Set("user_role", "manager")
	c.Set("user_email", suggestionChangelogActor)
	return c, rec
}

func applySuggestion(t *testing.T, s *Server, orgID int, id int64) {
	t.Helper()
	c, rec := suggestionCtx(orgID, http.MethodPost, id)
	if err := s.handleApplyEntitySuggestion(c); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"applied"`) {
		t.Fatalf("apply: status %d, body %s", rec.Code, rec.Body.String())
	}
}

// appliedRows counts the suggestion_applied rows for sg on one risk.
func appliedRows(t *testing.T, s *Server, orgID int, riskID int64, sg *db.Suggestion) int {
	t.Helper()
	entries, err := s.db.ListEntityChangelog(context.Background(), orgID, "risk", riskID)
	if err != nil {
		t.Fatalf("ListEntityChangelog(risk %d): %v", riskID, err)
	}
	want := fmt.Sprintf("Applied suggestion #%d:", sg.ID)
	n := 0
	for _, e := range entries {
		if e.Action == "suggestion_applied" && strings.HasPrefix(e.Reason, want) {
			n++
		}
	}
	return n
}

func TestSuggestionAppliedChangelogLandsOnEntity(t *testing.T) {
	s := testServer(t)
	orgID, risks := newSuggestionChangelogOrgs(t, s)
	target := risks[2]

	sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:     "risk",
		EntityID:       target.Identifier,
		SuggestionType: "update",
		Payload:        json.RawMessage(`{"fields":{"notes":"updated by suggestion"}}`),
	})
	applySuggestion(t, s, orgID, sg.ID)

	if n := appliedRows(t, s, orgID, target.ID, sg); n != 1 {
		t.Errorf("%s (id %d) has %d suggestion_applied rows for suggestion #%d, want 1", target.Identifier, target.ID, n, sg.ID)
	}
	for _, r := range risks[:2] {
		if n := appliedRows(t, s, orgID, r.ID, sg); n != 0 {
			t.Errorf("%s (id %d) has %d suggestion_applied rows for a suggestion applied to %s", r.Identifier, r.ID, n, target.Identifier)
		}
	}
}

// A create handler's row is still uncommitted when the changelog row is
// written, so its primary key has to come from the handler, not a lookup.
func TestSuggestionAppliedChangelogOnCreatedEntity(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID, _ := newSuggestionChangelogOrgs(t, s)

	sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:     "risk",
		SuggestionType: "create",
		Payload:        json.RawMessage(`{"title":"risk created by suggestion"}`),
	})
	applySuggestion(t, s, orgID, sg.ID)

	got, err := s.db.GetSuggestion(ctx, orgID, sg.ID)
	if err != nil {
		t.Fatalf("GetSuggestion: %v", err)
	}
	created, err := s.db.GetRiskByIdentifier(ctx, orgID, got.AppliedEntityID)
	if err != nil {
		t.Fatalf("created risk %q: %v", got.AppliedEntityID, err)
	}
	if n := appliedRows(t, s, orgID, created.ID, sg); n != 1 {
		t.Errorf("created %s (id %d) has %d suggestion_applied rows, want 1", created.Identifier, created.ID, n)
	}
}

// The stale check compares the entity's changelog against the snapshot taken
// when the suggestion was made; it has to read the suggestion's own entity.
func TestSuggestionStaleCheckReadsEntity(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID, risks := newSuggestionChangelogOrgs(t, s)
	target := risks[2]

	snapshot := db.Epoch{Time: time.Now().Add(-time.Hour)}
	sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:      "risk",
		EntityID:        target.Identifier,
		SuggestionType:  "update",
		Payload:         json.RawMessage(`{"fields":{"notes":"stale suggestion"}}`),
		EntityUpdatedAt: &snapshot,
	})
	if err := s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "risk",
		EntityID:   target.ID,
		Action:     "update",
		Field:      "notes",
		ChangedBy:  suggestionChangelogActor,
	}); err != nil {
		t.Fatalf("LogChange: %v", err)
	}

	c, rec := suggestionCtx(orgID, http.MethodGet, sg.ID)
	if err := s.handleGetEntitySuggestion(c); err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(rec.Body.String(), `"stale":true`) {
		t.Errorf("GET: %s changed after the snapshot but the suggestion is not marked stale: %s", target.Identifier, rec.Body.String())
	}

	c, rec = suggestionCtx(orgID, http.MethodPost, sg.ID)
	if err := s.handleApplyEntitySuggestion(c); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(rec.Body.String(), `"stale":true`) {
		t.Errorf("apply without force: want the stale response, got %d %s", rec.Code, rec.Body.String())
	}
}
