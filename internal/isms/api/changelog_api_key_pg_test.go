package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays green
// without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...
//
// Regression for #335: entity_changelog.api_key_id was always NULL, even for
// changes made with a personal API key, because nothing read the key id the
// auth middleware stores on the echo context. These tests go through the
// real AuthMiddleware (not a hand-set api_key_id) so they exercise the wiring
// gap itself, not just the read-back.

const changelogAPIKeyActor = "manager@changelog-api-key.test"

// changelogAPIKeyFixture is a throwaway org, member and API key for a
// changelog-api-key test.
type changelogAPIKeyFixture struct {
	orgID    int
	email    string
	rawToken string
	keyID    int
}

// newChangelogAPIKeyFixture creates an org, a manager member and an API key
// scoped to that org, so a request bearing the raw token authenticates as
// that member through AuthMiddleware.
func newChangelogAPIKeyFixture(t *testing.T, s *Server) changelogAPIKeyFixture {
	t.Helper()
	ctx := context.Background()
	orgID := newTestOrg(t, s, "changelog-api-key")

	email := fmt.Sprintf("%s.%d", changelogAPIKeyActor, os.Getpid())
	u := &db.User{Email: email, Name: email, Active: true}
	if err := s.db.UpsertUser(ctx, u); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := s.db.AddOrgMember(ctx, orgID, u.ID, "manager"); err != nil {
		t.Fatalf("AddOrgMember: %v", err)
	}

	raw := fmt.Sprintf("isms_test335_%d_%s", os.Getpid(), t.Name())
	key, err := s.db.CreateAPIKey(ctx, "changelog-api-key test", sha256Hash(raw), email, "read-write", &orgID, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	return changelogAPIKeyFixture{orgID: orgID, email: email, rawToken: raw, keyID: key.ID}
}

// apiKeyRequest builds an echo context carrying a bearer API key, as
// OrgResolverMiddleware and AuthMiddleware would see it in production, and
// runs it through the real AuthMiddleware before calling handler.
func apiKeyRequest(t *testing.T, s *Server, f changelogAPIKeyFixture, method, id, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	// The path must start with /api/ — AuthMiddleware treats anything else as a
	// static asset and skips auth entirely, which would defeat these tests.
	req := httptest.NewRequest(method, "/api/v1/changelog-api-key-test", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+f.rawToken)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	// Mimics OrgResolverMiddleware, which runs before AuthMiddleware in production.
	c.Set("org_id", f.orgID)
	return c, rec
}

func runThroughAuthMiddleware(t *testing.T, s *Server, c echo.Context, handler echo.HandlerFunc) error {
	t.Helper()
	return AuthMiddleware(AuthConfig{DB: s.db})(handler)(c)
}

// TestChangelogRecordsAPIKeyForReading is the acceptance case from #335: a
// change made with an API key must record that key's id in
// entity_changelog.api_key_id.
func TestChangelogRecordsAPIKeyForReading(t *testing.T) {
	s := testServer(t)
	f := newChangelogAPIKeyFixture(t, s)
	risk := newTestRisk(t, s, f.orgID, "risk for api key changelog test")

	c, rec := apiKeyRequest(t, s, f, http.MethodPost, fmt.Sprintf("%d", risk.ID), `{"current_likelihood":2,"current_impact":3}`)
	if err := runThroughAuthMiddleware(t, s, c, s.handleCreateRiskReading); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body.String())
	}

	entries, err := s.db.ListEntityChangelog(context.Background(), f.orgID, "risk", risk.ID)
	if err != nil {
		t.Fatalf("ListEntityChangelog: %v", err)
	}
	var found *db.ChangelogEntry
	for i := range entries {
		if entries[i].Action == "reading" {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no changelog row with action \"reading\" for risk %d (got %d rows)", risk.ID, len(entries))
	}
	if found.APIKeyID == nil || *found.APIKeyID != f.keyID {
		t.Errorf("api_key_id = %v, want %d", found.APIKeyID, f.keyID)
	}
	if found.ChangedBy != f.email {
		t.Errorf("changed_by = %q, want %q", found.ChangedBy, f.email)
	}
}

// TestChangelogSessionWriteHasNoAPIKey pins the other half of the schema
// comment: a write made without an API key (a JWT/browser session, mimicked
// here the way readings_changelog_pg_test.go does) must leave api_key_id NULL.
func TestChangelogSessionWriteHasNoAPIKey(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "changelog-session")
	risk := newTestRisk(t, s, orgID, "risk for session changelog test")

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"current_likelihood":2,"current_impact":3}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", risk.ID))
	c.Set("org_id", orgID)
	c.Set("user_role", "manager")
	c.Set("user_email", changelogAPIKeyActor)

	if err := s.handleCreateRiskReading(c); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body.String())
	}

	entries, err := s.db.ListEntityChangelog(context.Background(), orgID, "risk", risk.ID)
	if err != nil {
		t.Fatalf("ListEntityChangelog: %v", err)
	}
	var found *db.ChangelogEntry
	for i := range entries {
		if entries[i].Action == "reading" {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no changelog row with action \"reading\" for risk %d (got %d rows)", risk.ID, len(entries))
	}
	if found.APIKeyID != nil {
		t.Errorf("api_key_id = %d, want nil for a session write", *found.APIKeyID)
	}
}

// TestChangelogRecordsAPIKeyForSuggestionApply covers the in-transaction
// inserts: every changelog row written by applying a suggestion through an
// API key — the field diff row(s) and the suggestion_applied row — must carry
// that key's id.
func TestChangelogRecordsAPIKeyForSuggestionApply(t *testing.T) {
	s := testServer(t)
	f := newChangelogAPIKeyFixture(t, s)
	risk := newTestRisk(t, s, f.orgID, "risk for api key suggestion apply test")

	sg := newTestSuggestion(t, s, f.orgID, &db.Suggestion{
		EntityType:     "risk",
		EntityID:       risk.Identifier,
		SuggestionType: "update",
		Payload:        []byte(`{"fields":{"notes":"updated by api key suggestion"}}`),
	})

	c, rec := apiKeyRequest(t, s, f, http.MethodPost, fmt.Sprintf("%d", sg.ID), `{}`)
	if err := runThroughAuthMiddleware(t, s, c, s.handleApplyEntitySuggestion); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"applied"`) {
		t.Fatalf("apply: status %d, body %s", rec.Code, rec.Body.String())
	}

	entries, err := s.db.ListEntityChangelog(context.Background(), f.orgID, "risk", risk.ID)
	if err != nil {
		t.Fatalf("ListEntityChangelog: %v", err)
	}

	sawSuggestionApplied := false
	sawFieldDiff := false
	for _, e := range entries {
		switch e.Action {
		case "suggestion_applied":
			sawSuggestionApplied = true
		case "update":
			sawFieldDiff = true
		default:
			continue
		}
		if e.APIKeyID == nil || *e.APIKeyID != f.keyID {
			t.Errorf("row action=%q: api_key_id = %v, want %d", e.Action, e.APIKeyID, f.keyID)
		}
	}
	if !sawSuggestionApplied {
		t.Errorf("no changelog row with action \"suggestion_applied\" for risk %d", risk.ID)
	}
	if !sawFieldDiff {
		t.Errorf("no field diff changelog row for risk %d", risk.ID)
	}
}
