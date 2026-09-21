package db

import (
	"context"
	"testing"
	"time"
)

// Requires a migrated Postgres — see notifications_roundtrip_test.go's header
// for how to stand one up. Skipped when ISMS_TEST_DATABASE_URL is unset.
//
// Proves that reading endpoints (#288, #289) honour explicit next_review dates
// and respect the organization's configured review cycles. Both issues stem from
// the Tx update helpers recomputing next_review unconditionally, with three of
// them using hardcoded nil cycles.
//
// Regression test for #288 (explicit date discarded) and #289 (configured cycles ignored).
func TestReadingsNextReviewRoundTrip(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "readingsnextreview")

	t.Run("Risk explicit date", func(t *testing.T) {
		likelihood := 2
		impact := 3
		risk := &Risk{
			Title:             "test risk",
			RiskType:          RiskTypes[0],
			Origin:            RiskOrigins[0],
			Status:            RiskStatuses[0],
			Treatment:         "mitigate",
			CurrentLikelihood: &likelihood,
			CurrentImpact:     &impact,
		}
		if err := d.CreateRisk(ctx, orgID, risk); err != nil {
			t.Fatalf("CreateRisk: %v", err)
		}

		// Update with an explicit date that no cycle would produce
		explicitDate := NewEpoch(time.Date(2031, time.February, 14, 0, 0, 0, 0, time.UTC))
		risk.NextReview = &explicitDate

		// Perform the update in a transaction
		tx, err := d.Pool().Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := UpdateRiskTx(ctx, tx, orgID, risk, nil, &explicitDate); err != nil {
			tx.Rollback(ctx)
			t.Fatalf("UpdateRiskTx: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("Commit: %v", err)
		}

		// Read back and verify the exact date was stored
		stored, err := d.GetRisk(ctx, orgID, risk.ID)
		if err != nil {
			t.Fatalf("GetRisk: %v", err)
		}
		if stored.NextReview == nil {
			t.Errorf("NextReview is nil, want explicit date")
		} else {
			got := stored.NextReview.Time.UTC().Format("2006-01-02")
			want := explicitDate.Time.UTC().Format("2006-01-02")
			if got != want {
				t.Errorf("NextReview = %q, want explicit date %q", got, want)
			}
		}
	})

	t.Run("Risk configured cycles honoured", func(t *testing.T) {
		likelihood := 2
		impact := 3
		risk := &Risk{
			Title:             "test risk cycles",
			RiskType:          RiskTypes[0],
			Origin:            RiskOrigins[0],
			Status:            RiskStatuses[0],
			Treatment:         "mitigate",
			CurrentLikelihood: &likelihood,
			CurrentImpact:     &impact,
		}
		if err := d.CreateRisk(ctx, orgID, risk); err != nil {
			t.Fatalf("CreateRisk: %v", err)
		}

		// Set all four risk_review_cycle_* keys to non-default values
		settingsToSet := map[string]string{
			"risk_review_cycle_critical": "2",
			"risk_review_cycle_high":     "4",
			"risk_review_cycle_medium":   "8",
			"risk_review_cycle_low":      "24",
		}
		for key, val := range settingsToSet {
			if err := d.SetOrgSetting(ctx, orgID, key, val); err != nil {
				t.Fatalf("SetOrgSetting %s: %v", key, err)
			}
		}

		// Get the configured cycles
		configuredCycles := d.RiskReviewCycles(ctx, orgID)

		// Set LastReview to avoid timing issues near day boundaries when comparing dates
		now := NewEpoch(time.Now())
		risk.LastReview = &now

		// Perform the update with configured cycles and no explicit date
		riskCopy := *risk
		tx, err := d.Pool().Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := UpdateRiskTx(ctx, tx, orgID, &riskCopy, configuredCycles, nil); err != nil {
			tx.Rollback(ctx)
			t.Fatalf("UpdateRiskTx: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("Commit: %v", err)
		}

		// Read back the stored value
		stored, err := d.GetRisk(ctx, orgID, riskCopy.ID)
		if err != nil {
			t.Fatalf("GetRisk: %v", err)
		}
		if stored.NextReview == nil {
			t.Errorf("NextReview is nil, want computed value")
			return
		}
		storedDateStr := stored.NextReview.Time.UTC().Format("2006-01-02")

		// Compute the expected date with configured cycles using the stored risk's data
		// This ensures we use the exact same LastReview that was persisted
		expectedWithConfig := &Risk{
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
		expectedDateStr := expectedWithConfig.NextReview.Time.UTC().Format("2006-01-02")

		// Compute the date with nil (defaults) for comparison
		defaultCompute := &Risk{
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
		defaultDateStr := defaultCompute.NextReview.Time.UTC().Format("2006-01-02")

		// Both assertions from the plan
		if storedDateStr != expectedDateStr {
			t.Errorf("stored date %q != configured cycles computation %q", storedDateStr, expectedDateStr)
		}
		if storedDateStr == defaultDateStr {
			// This would mean the configured cycles were NOT used
			t.Errorf("stored date %q should differ from default cycles computation %q", storedDateStr, defaultDateStr)
		}
	})

	t.Run("LegalRequirement explicit date", func(t *testing.T) {
		likelihood := 2
		impact := 3
		lr := &LegalRequirement{
			Title:             "test legal",
			Jurisdiction:      "US",
			Category:          LegalCategories[0],
			Status:            LegalStatuses[0],
			CurrentLikelihood: &likelihood,
			CurrentImpact:     &impact,
		}
		if err := d.CreateLegalRequirement(ctx, orgID, lr); err != nil {
			t.Fatalf("CreateLegalRequirement: %v", err)
		}

		// Update with an explicit date that no cycle would produce
		explicitDate := NewEpoch(time.Date(2031, time.March, 15, 0, 0, 0, 0, time.UTC))
		lr.NextReview = &explicitDate

		// Perform the update in a transaction
		tx, err := d.Pool().Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := UpdateLegalRequirementTx(ctx, tx, orgID, lr, nil, &explicitDate); err != nil {
			tx.Rollback(ctx)
			t.Fatalf("UpdateLegalRequirementTx: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("Commit: %v", err)
		}

		// Read back and verify the exact date was stored
		stored, err := d.GetLegalRequirement(ctx, orgID, lr.ID)
		if err != nil {
			t.Fatalf("GetLegalRequirement: %v", err)
		}
		if stored.NextReview == nil {
			t.Errorf("NextReview is nil, want explicit date")
		} else {
			got := stored.NextReview.Time.UTC().Format("2006-01-02")
			want := explicitDate.Time.UTC().Format("2006-01-02")
			if got != want {
				t.Errorf("NextReview = %q, want explicit date %q", got, want)
			}
		}
	})

	t.Run("LegalRequirement configured cycles honoured", func(t *testing.T) {
		likelihood := 2
		impact := 3
		lr := &LegalRequirement{
			Title:             "test legal cycles",
			Jurisdiction:      "UK",
			Category:          LegalCategories[0],
			Status:            LegalStatuses[0],
			CurrentLikelihood: &likelihood,
			CurrentImpact:     &impact,
		}
		if err := d.CreateLegalRequirement(ctx, orgID, lr); err != nil {
			t.Fatalf("CreateLegalRequirement: %v", err)
		}

		// LegalRequirements use the same risk_review_cycle_* keys
		// Set all four keys (if not already set from the Risk test above)
		settingsToSet := map[string]string{
			"risk_review_cycle_critical": "2",
			"risk_review_cycle_high":     "4",
			"risk_review_cycle_medium":   "8",
			"risk_review_cycle_low":      "24",
		}
		for key, val := range settingsToSet {
			if err := d.SetOrgSetting(ctx, orgID, key, val); err != nil {
				t.Fatalf("SetOrgSetting %s: %v", key, err)
			}
		}

		// Get the configured cycles
		configuredCycles := d.RiskReviewCycles(ctx, orgID)

		// Set LastReview to avoid timing issues near day boundaries when comparing dates
		now := NewEpoch(time.Now())
		lr.LastReview = &now

		// Perform the update with configured cycles and no explicit date
		lrCopy := *lr
		tx, err := d.Pool().Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := UpdateLegalRequirementTx(ctx, tx, orgID, &lrCopy, configuredCycles, nil); err != nil {
			tx.Rollback(ctx)
			t.Fatalf("UpdateLegalRequirementTx: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("Commit: %v", err)
		}

		// Read back the stored value
		stored, err := d.GetLegalRequirement(ctx, orgID, lrCopy.ID)
		if err != nil {
			t.Fatalf("GetLegalRequirement: %v", err)
		}
		if stored.NextReview == nil {
			t.Errorf("NextReview is nil, want computed value")
			return
		}
		storedDateStr := stored.NextReview.Time.UTC().Format("2006-01-02")

		// Compute the expected date with configured cycles using the stored legal req's data
		// This ensures we use the exact same LastReview that was persisted
		expectedWithConfig := &LegalRequirement{
			Title:             stored.Title,
			Jurisdiction:      stored.Jurisdiction,
			Category:          stored.Category,
			Status:            stored.Status,
			CurrentLikelihood: stored.CurrentLikelihood,
			CurrentImpact:     stored.CurrentImpact,
			LastReview:        stored.LastReview,
			CurrentScore:      stored.CurrentScore,
			CurrentLevel:      stored.CurrentLevel,
		}
		expectedWithConfig.CalculateRiskScore(configuredCycles)
		expectedDateStr := expectedWithConfig.NextReview.Time.UTC().Format("2006-01-02")

		// Compute the date with nil (defaults) for comparison
		defaultCompute := &LegalRequirement{
			Title:             stored.Title,
			Jurisdiction:      stored.Jurisdiction,
			Category:          stored.Category,
			Status:            stored.Status,
			CurrentLikelihood: stored.CurrentLikelihood,
			CurrentImpact:     stored.CurrentImpact,
			LastReview:        stored.LastReview,
			CurrentScore:      stored.CurrentScore,
			CurrentLevel:      stored.CurrentLevel,
		}
		defaultCompute.CalculateRiskScore(nil)
		defaultDateStr := defaultCompute.NextReview.Time.UTC().Format("2006-01-02")

		// Both assertions from the plan.
		// Note: LegalRequirement.CalculateReviewDate always uses time.Now() (not LastReview),
		// so the stored and fresh computations might be at different wall-clock seconds,
		// potentially crossing a day boundary. We check they're within 2 days of each other
		// (to handle midnight boundaries) and that configured differs from default.
		storedTime, _ := time.Parse("2006-01-02", storedDateStr)
		expectedTime, _ := time.Parse("2006-01-02", expectedDateStr)
		daysDiff := int(storedTime.Sub(expectedTime).Hours() / 24)
		if daysDiff < 0 {
			daysDiff = -daysDiff
		}
		if daysDiff > 1 {
			t.Errorf("stored date %q and configured cycles computation %q differ by %d days (should be ≤1)", storedDateStr, expectedDateStr, daysDiff)
		}
		if storedDateStr == defaultDateStr {
			// This would mean the configured cycles were NOT used
			t.Errorf("stored date %q should differ from default cycles computation %q", storedDateStr, defaultDateStr)
		}
	})

	t.Run("Supplier explicit date", func(t *testing.T) {
		sup := &Supplier{
			Name:         "test supplier",
			SupplierType: SupplierTypes[0],
			Criticality:  CriticalityLevels[0],
			Status:       SupplierStatuses[0],
		}
		if err := d.CreateSupplier(ctx, orgID, sup); err != nil {
			t.Fatalf("CreateSupplier: %v", err)
		}

		// Update with an explicit date that no cycle would produce
		explicitDate := NewEpoch(time.Date(2031, time.April, 20, 0, 0, 0, 0, time.UTC))
		sup.NextReview = &explicitDate

		// Perform the update in a transaction
		tx, err := d.Pool().Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		cycles := d.SupplierReviewCycles(ctx, orgID)
		if err := UpdateSupplierTx(ctx, tx, orgID, sup, cycles, &explicitDate); err != nil {
			tx.Rollback(ctx)
			t.Fatalf("UpdateSupplierTx: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("Commit: %v", err)
		}

		// Read back and verify the exact date was stored
		stored, err := d.GetSupplier(ctx, orgID, sup.ID)
		if err != nil {
			t.Fatalf("GetSupplier: %v", err)
		}
		if stored.NextReview == nil {
			t.Errorf("NextReview is nil, want explicit date")
		} else {
			got := stored.NextReview.Time.UTC().Format("2006-01-02")
			want := explicitDate.Time.UTC().Format("2006-01-02")
			if got != want {
				t.Errorf("NextReview = %q, want explicit date %q", got, want)
			}
		}
	})

	t.Run("System explicit date", func(t *testing.T) {
		sys := &System{
			Name:           "test system",
			Classification: SystemClassifications[0],
			Criticality:    CriticalityLevels[0],
			Status:         SystemStatuses[0],
		}
		if err := d.CreateSystem(ctx, orgID, sys); err != nil {
			t.Fatalf("CreateSystem: %v", err)
		}

		// Update with an explicit date that no cycle would produce
		explicitDate := NewEpoch(time.Date(2031, time.May, 25, 0, 0, 0, 0, time.UTC))
		sys.NextReview = &explicitDate

		// Perform the update in a transaction
		tx, err := d.Pool().Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := UpdateSystemTx(ctx, tx, orgID, sys, &explicitDate); err != nil {
			tx.Rollback(ctx)
			t.Fatalf("UpdateSystemTx: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("Commit: %v", err)
		}

		// Read back and verify the exact date was stored
		stored, err := d.GetSystem(ctx, orgID, sys.ID)
		if err != nil {
			t.Fatalf("GetSystem: %v", err)
		}
		if stored.NextReview == nil {
			t.Errorf("NextReview is nil, want explicit date")
		} else {
			got := stored.NextReview.Time.UTC().Format("2006-01-02")
			want := explicitDate.Time.UTC().Format("2006-01-02")
			if got != want {
				t.Errorf("NextReview = %q, want explicit date %q", got, want)
			}
		}
	})
}
