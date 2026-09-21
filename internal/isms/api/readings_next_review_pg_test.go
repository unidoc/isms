package api

import (
	"context"
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
// Proves that the readings endpoints honour explicit next_review dates (#288) and
// respect the organization's configured review cycles (#289).
func TestReadingsNextReviewEndToEnd(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "readings-next-review")

	// Helper to build an echo context for POST /api/v1/risks/:id/readings
	ctxForReading := func(riskID int64, body string) (echo.Context, *httptest.ResponseRecorder) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/risks/%d/readings", riskID), strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", riskID))
		c.Set("org_id", orgID)
		c.Set("user_role", "manager")
		c.Set("user_email", "manager@readings-next-review.test")
		return c, rec
	}

	t.Run("explicit next_review date survives", func(t *testing.T) {
		// Create a fresh risk for this test
		likelihood := 2
		impact := 3
		freshRisk := &db.Risk{
			Title:             "risk for explicit date test",
			RiskType:          db.RiskTypes[0],
			Origin:            db.RiskOrigins[0],
			Status:            db.RiskStatuses[0],
			Treatment:         "mitigate",
			CurrentLikelihood: &likelihood,
			CurrentImpact:     &impact,
		}
		if err := s.db.CreateRisk(ctx, orgID, freshRisk); err != nil {
			t.Fatalf("CreateRisk: %v", err)
		}

		// Build the reading request with explicit next_review date
		readingBody := `{
			"current_likelihood": 2,
			"current_impact": 3,
			"assessed_by": "manager@readings-next-review.test",
			"next_review": "2031-02-14"
		}`

		c, rec := ctxForReading(freshRisk.ID, readingBody)
		if err := s.handleCreateRiskReading(c); err != nil {
			t.Fatalf("handleCreateRiskReading: %v", err)
		}

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body.String())
		}

		// Read back the risk and verify the explicit date was stored
		stored, err := s.db.GetRisk(ctx, orgID, freshRisk.ID)
		if err != nil {
			t.Fatalf("GetRisk: %v", err)
		}

		if stored.NextReview == nil {
			t.Errorf("NextReview is nil after reading with explicit date")
			return
		}

		gotDate := stored.NextReview.Time.UTC().Format("2006-01-02")
		wantDate := "2031-02-14"

		if gotDate != wantDate {
			t.Errorf("NextReview = %q, want explicit date %q", gotDate, wantDate)
		}
	})

	t.Run("configured cycles are honoured", func(t *testing.T) {
		// Create a fresh risk for this test
		likelihood := 2
		impact := 3
		now := db.NewEpoch(time.Now())
		freshRisk := &db.Risk{
			Title:             "risk for configured cycles test",
			RiskType:          db.RiskTypes[0],
			Origin:            db.RiskOrigins[0],
			Status:            db.RiskStatuses[0],
			Treatment:         "mitigate",
			CurrentLikelihood: &likelihood,
			CurrentImpact:     &impact,
			LastReview:        &now,
		}
		if err := s.db.CreateRisk(ctx, orgID, freshRisk); err != nil {
			t.Fatalf("CreateRisk: %v", err)
		}

		// Configure custom cycle values
		settingsToSet := map[string]string{
			"risk_review_cycle_critical": "2",
			"risk_review_cycle_high":     "4",
			"risk_review_cycle_medium":   "8",
			"risk_review_cycle_low":      "24",
		}
		for key, val := range settingsToSet {
			if err := s.db.SetOrgSetting(ctx, orgID, key, val); err != nil {
				t.Fatalf("SetOrgSetting %s: %v", key, err)
			}
		}

		// Build the reading request WITHOUT next_review (should derive from configured cycles)
		readingBody := `{
			"current_likelihood": 2,
			"current_impact": 3,
			"assessed_by": "manager@readings-next-review.test"
		}`

		c, rec := ctxForReading(freshRisk.ID, readingBody)
		if err := s.handleCreateRiskReading(c); err != nil {
			t.Fatalf("handleCreateRiskReading: %v", err)
		}

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body.String())
		}

		// Read back the risk and verify the date derives from configured cycles
		stored, err := s.db.GetRisk(ctx, orgID, freshRisk.ID)
		if err != nil {
			t.Fatalf("GetRisk: %v", err)
		}

		if stored.NextReview == nil {
			t.Errorf("NextReview is nil after reading without explicit date")
			return
		}

		storedDate := stored.NextReview.Time.UTC().Format("2006-01-02")

		// Compute the expected date with configured cycles using the stored risk's data
		// This ensures we use the exact same LastReview that was persisted
		configuredCycles := s.db.RiskReviewCycles(ctx, orgID)
		expectedWithConfig := &db.Risk{
			Title:             stored.Title,
			RiskType:          stored.RiskType,
			Origin:            stored.Origin,
			Status:            stored.Status,
			Treatment:         stored.Treatment,
			CurrentLikelihood: stored.CurrentLikelihood,
			CurrentImpact:     stored.CurrentImpact,
			LastReview:        stored.LastReview,
			CurrentScore:      stored.CurrentScore,
			CurrentLevel:      stored.CurrentLevel,
		}
		expectedWithConfig.CalculateScore(configuredCycles)
		expectedWithConfigDate := expectedWithConfig.NextReview.Time.UTC().Format("2006-01-02")

		// Compute the date with defaults (nil cycles)
		defaultCompute := &db.Risk{
			Title:             stored.Title,
			RiskType:          stored.RiskType,
			Origin:            stored.Origin,
			Status:            stored.Status,
			Treatment:         stored.Treatment,
			CurrentLikelihood: stored.CurrentLikelihood,
			CurrentImpact:     stored.CurrentImpact,
			LastReview:        stored.LastReview,
			CurrentScore:      stored.CurrentScore,
			CurrentLevel:      stored.CurrentLevel,
		}
		defaultCompute.CalculateScore(nil)
		defaultDate := defaultCompute.NextReview.Time.UTC().Format("2006-01-02")

		// Verify both assertions: stored matches configured, differs from default
		if storedDate != expectedWithConfigDate {
			t.Errorf("stored date %q != configured cycles computation %q", storedDate, expectedWithConfigDate)
		}
		if storedDate == defaultDate {
			t.Errorf("stored date %q should differ from default cycles computation %q", storedDate, defaultDate)
		}
	})
}
