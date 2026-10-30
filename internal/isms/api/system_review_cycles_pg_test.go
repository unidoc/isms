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

// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays green
// without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...
//
// Regression for #308: system review-cycle length was a hard-coded 1/3/6/12
// switch. An org that sets system_review_cycle_high must see that cycle on
// every path that writes a system's next_review, the same as suppliers (#43).

const systemCycleUser = "admin@system-review-cycles.test"

// systemCycleCtx builds an echo context with an optional :id path parameter.
func systemCycleCtx(orgID int, method, param, body string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, "/api/v1/systems/"+param, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if param != "" {
		c.SetParamNames("id")
		c.SetParamValues(param)
	}
	c.Set("org_id", orgID)
	c.Set("user_role", "admin")
	c.Set("user_email", systemCycleUser)
	return c, rec
}

func TestSystemReviewCycleDrivesEveryWritePath(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "system-review-cycles")
	contractTestUser(t, s, orgID, systemCycleUser, "admin")

	if err := s.db.SetOrgSetting(ctx, orgID, "system_review_cycle_high", "2"); err != nil {
		t.Fatalf("SetOrgSetting: %v", err)
	}

	// Dates are counted from a fixed last_review where the path allows it, so
	// the expectation does not depend on today's date. 3 months (the old
	// hard-coded value) would be 2030-06-10.
	const lastReview = "2030-03-10"
	const wantFromLast = "2030-05-10"

	stored := func(t *testing.T, id int64) *db.System {
		t.Helper()
		sys, err := s.db.GetSystem(ctx, orgID, id)
		if err != nil {
			t.Fatalf("GetSystem: %v", err)
		}
		return sys
	}
	// Paths that count from now() accept the UTC or the local calendar day,
	// since the date is stored without a time of day.
	assertTwoMonthsFromNow := func(t *testing.T, got *db.Epoch) {
		t.Helper()
		now := time.Now().AddDate(0, 2, 0)
		if g := utcDate(got); g != now.UTC().Format("2006-01-02") && g != now.Format("2006-01-02") {
			t.Fatalf("next_review = %s, want today + 2 months (%s)", g, now.UTC().Format("2006-01-02"))
		}
	}
	newHigh := func(t *testing.T, name string) *db.System {
		t.Helper()
		lr := db.NewEpoch(time.Date(2030, 3, 10, 0, 0, 0, 0, time.UTC))
		sys := &db.System{Name: name, Classification: "internal", Criticality: "high", Status: "active", LastReview: &lr}
		if err := s.db.CreateSystem(ctx, orgID, sys); err != nil {
			t.Fatalf("CreateSystem: %v", err)
		}
		return sys
	}

	t.Run("create", func(t *testing.T) {
		body := fmt.Sprintf(`{"name":"create","classification":"internal","criticality":"high","status":"active","last_review":%q}`, lastReview)
		c, rec := systemCycleCtx(orgID, http.MethodPost, "", body)
		if err := s.handleCreateSystem(c); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body.String())
		}
		var created struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == 0 {
			t.Fatalf("decode created id: %v; body %s", err, rec.Body.String())
		}
		if got := utcDate(stored(t, created.ID).NextReview); got != wantFromLast {
			t.Fatalf("next_review = %s, want %s", got, wantFromLast)
		}
	})

	t.Run("create without last_review counts from today", func(t *testing.T) {
		body := `{"name":"create now","classification":"internal","criticality":"high","status":"active"}`
		c, rec := systemCycleCtx(orgID, http.MethodPost, "", body)
		if err := s.handleCreateSystem(c); err != nil {
			t.Fatalf("handler: %v", err)
		}
		var created struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == 0 {
			t.Fatalf("decode created id: %v; body %s", err, rec.Body.String())
		}
		assertTwoMonthsFromNow(t, stored(t, created.ID).NextReview)
	})

	t.Run("create keeps an explicit date", func(t *testing.T) {
		body := fmt.Sprintf(`{"name":"explicit","classification":"internal","criticality":"high","status":"active","next_review":%q}`, explicitReviewDate)
		c, rec := systemCycleCtx(orgID, http.MethodPost, "", body)
		if err := s.handleCreateSystem(c); err != nil {
			t.Fatalf("handler: %v", err)
		}
		var created struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == 0 {
			t.Fatalf("decode created id: %v; body %s", err, rec.Body.String())
		}
		if got := utcDate(stored(t, created.ID).NextReview); got != explicitReviewDate {
			t.Fatalf("next_review = %s, want the explicit %s", got, explicitReviewDate)
		}
	})

	t.Run("update", func(t *testing.T) {
		sys := newHigh(t, "update")
		// Move it off the configured date first so the update has to recompute.
		c, rec := systemCycleCtx(orgID, http.MethodPut, fmt.Sprintf("%d", sys.ID), fmt.Sprintf(`{"next_review":%q}`, explicitReviewDate))
		if err := s.handleUpdateSystem(c); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("setup update: %v (status %d, %s)", err, rec.Code, rec.Body.String())
		}
		c, rec = systemCycleCtx(orgID, http.MethodPut, fmt.Sprintf("%d", sys.ID), `{"notes":"edited"}`)
		if err := s.handleUpdateSystem(c); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("update: %v (status %d, %s)", err, rec.Code, rec.Body.String())
		}
		if got := utcDate(stored(t, sys.ID).NextReview); got != wantFromLast {
			t.Fatalf("next_review = %s, want %s", got, wantFromLast)
		}
	})

	t.Run("update keeps an explicit date", func(t *testing.T) {
		sys := newHigh(t, "update explicit")
		c, rec := systemCycleCtx(orgID, http.MethodPut, fmt.Sprintf("%d", sys.ID), fmt.Sprintf(`{"next_review":%q}`, explicitReviewDate))
		if err := s.handleUpdateSystem(c); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("update: %v (status %d, %s)", err, rec.Code, rec.Body.String())
		}
		if got := utcDate(stored(t, sys.ID).NextReview); got != explicitReviewDate {
			t.Fatalf("next_review = %s, want the explicit %s", got, explicitReviewDate)
		}
	})

	t.Run("reading", func(t *testing.T) {
		sys := newHigh(t, "reading")
		c, rec := systemCycleCtx(orgID, http.MethodPost, fmt.Sprintf("%d/readings", sys.ID), `{"confidentiality":2,"integrity":2,"availability":2}`)
		c.SetParamValues(fmt.Sprintf("%d", sys.ID))
		if err := s.handleCreateSystemReading(c); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body.String())
		}
		// A reading sets last_review to now.
		assertTwoMonthsFromNow(t, stored(t, sys.ID).NextReview)
	})

	t.Run("access review", func(t *testing.T) {
		sys := newHigh(t, "access review")
		ar := &db.AccessReview{SystemID: sys.ID, ReviewedAt: db.NewEpoch(time.Date(2030, 4, 20, 0, 0, 0, 0, time.UTC)), ReviewedBy: systemCycleUser}
		if err := s.db.CreateAccessReview(ctx, orgID, ar); err != nil {
			t.Fatalf("CreateAccessReview: %v", err)
		}
		if got, want := utcDate(stored(t, sys.ID).NextReview), "2030-06-20"; got != want {
			t.Fatalf("next_review = %s, want %s", got, want)
		}
	})

	t.Run("suggestion apply create", func(t *testing.T) {
		sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
			EntityType:     "system",
			SuggestionType: "create",
			Payload:        json.RawMessage(fmt.Sprintf(`{"name":"suggested","classification":"internal","criticality":"high","last_review":%q}`, lastReview)),
		})
		applySuggestionAs(t, s, orgID, sg.ID)
		applied, err := s.db.GetSuggestion(ctx, orgID, sg.ID)
		if err != nil {
			t.Fatalf("GetSuggestion: %v", err)
		}
		id, err := s.resolveSystemID(ctx, orgID, applied.AppliedEntityID)
		if err != nil {
			t.Fatalf("resolve %q: %v", applied.AppliedEntityID, err)
		}
		if got := utcDate(stored(t, id).NextReview); got != wantFromLast {
			t.Fatalf("next_review = %s, want %s", got, wantFromLast)
		}
	})

	t.Run("suggestion apply update", func(t *testing.T) {
		sys := newHigh(t, "suggestion update")
		c, rec := systemCycleCtx(orgID, http.MethodPut, fmt.Sprintf("%d", sys.ID), fmt.Sprintf(`{"next_review":%q}`, explicitReviewDate))
		if err := s.handleUpdateSystem(c); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("setup update: %v (status %d, %s)", err, rec.Code, rec.Body.String())
		}
		sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
			EntityType:     "system",
			EntityID:       sys.Identifier,
			SuggestionType: "update",
			Payload:        json.RawMessage(`{"fields":{"notes":"updated by suggestion"}}`),
		})
		applySuggestionAs(t, s, orgID, sg.ID)
		if got := utcDate(stored(t, sys.ID).NextReview); got != wantFromLast {
			t.Fatalf("next_review = %s, want %s", got, wantFromLast)
		}
	})
}

// applySuggestionAs applies a suggestion as a manager the test seeded itself.
func applySuggestionAs(t *testing.T, s *Server, orgID int, id int64) {
	t.Helper()
	contractTestUser(t, s, orgID, suggestionChangelogActor, "manager")
	applySuggestion(t, s, orgID, id)
}
