package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// ═══════════════════════════════════════════════════════════════════════
// RISK READINGS
// ═══════════════════════════════════════════════════════════════════════

func (s *Server) handleListRiskReadings(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	riskID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid risk id")
	}
	readings, err := s.db.ListEntityReadings(ctx, orgID, "risk", riskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if readings == nil {
		readings = []db.EntityReading{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": readings})
}

func (s *Server) handleCreateRiskReading(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	riskID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid risk id")
	}

	var req struct {
		db.EntityReading
		NextReview string `json:"next_review,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	reading := req.EntityReading
	reading.EntityType = "risk"
	reading.EntityID = riskID
	actor := getUserEmail(c)
	if reading.AssessedBy == "" {
		reading.AssessedBy = actor
	}

	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.CreateEntityReadingTx(ctx, tx, orgID, &reading); err != nil {
			return fmt.Errorf("create reading: %w", err)
		}
		// Write-through: update parent risk
		return writeRiskFromReading(ctx, tx, s, orgID, riskID, &reading, actor, req.NextReview)
	})
	if txErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, txErr.Error())
	}

	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "risk",
		EntityID:   riskID,
		Action:     "reading",
		ChangedBy:  actor,
		Reason:     fmt.Sprintf("Reading #%d recorded", reading.ID),
	})
	return c.JSON(http.StatusCreated, reading)
}

// writeRiskFromReading updates the parent risk with values from a reading.
// The first reading for a risk is treated as the inherent (pre-controls) assessment.
// Subsequent readings update only current/residual values.
// nextReview, if provided, sets the risk's next review date (YYYY-MM-DD).
func writeRiskFromReading(ctx context.Context, tx pgx.Tx, s *Server, orgID int, riskID int64, r *db.EntityReading, actor string, nextReview string) error {
	risk, err := s.db.GetRisk(ctx, orgID, riskID)
	if err != nil {
		return fmt.Errorf("risk %d not found: %w", riskID, err)
	}

	// First reading → also set inherent baseline
	firstReading := risk.InherentLikelihood == nil && risk.InherentImpact == nil
	if firstReading {
		if r.CurrentLikelihood != nil {
			risk.InherentLikelihood = r.CurrentLikelihood
		}
		if r.CurrentImpact != nil {
			risk.InherentImpact = r.CurrentImpact
		}
		if r.Confidentiality != nil {
			risk.InherentConfidentialityImpact = r.Confidentiality
		}
		if r.Integrity != nil {
			risk.InherentIntegrityImpact = r.Integrity
		}
		if r.Availability != nil {
			risk.InherentAvailabilityImpact = r.Availability
		}
	}

	// Always update current/residual
	if r.CurrentLikelihood != nil {
		risk.CurrentLikelihood = r.CurrentLikelihood
	}
	if r.CurrentImpact != nil {
		risk.CurrentImpact = r.CurrentImpact
	}
	if r.Confidentiality != nil {
		risk.ConfidentialityImpact = r.Confidentiality
	}
	if r.Integrity != nil {
		risk.IntegrityImpact = r.Integrity
	}
	if r.Availability != nil {
		risk.AvailabilityImpact = r.Availability
	}
	if r.Status != "" {
		risk.Status = r.Status
	}
	if r.Treatment != "" {
		risk.Treatment = r.Treatment
	}

	// Recompute score and level
	risk.CalculateScore(nil)

	// Set last_review to now (canonical assessment log lives in entity_readings)
	now := db.NewEpoch(time.Now())
	risk.LastReview = &now

	// Set next review date from explicit input, or compute from level
	if nextReview != "" {
		if t, err := time.Parse("2006-01-02", nextReview); err == nil {
			e := db.NewEpoch(t)
			risk.NextReview = &e
		}
	} else {
		risk.CalculateReviewDate(nil)
	}

	if err := db.UpdateRiskTx(ctx, tx, orgID, risk); err != nil {
		return err
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════════════
// ASSET READINGS
// ═══════════════════════════════════════════════════════════════════════

func (s *Server) handleListAssetReadings(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	assetID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid asset id")
	}
	readings, err := s.db.ListEntityReadings(ctx, orgID, "asset", assetID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if readings == nil {
		readings = []db.EntityReading{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": readings})
}

func (s *Server) handleCreateAssetReading(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	assetID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid asset id")
	}

	var req struct {
		db.EntityReading
		NextReview string `json:"next_review,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	reading := req.EntityReading
	reading.EntityType = "asset"
	reading.EntityID = assetID
	actor := getUserEmail(c)
	if reading.AssessedBy == "" {
		reading.AssessedBy = actor
	}

	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.CreateEntityReadingTx(ctx, tx, orgID, &reading); err != nil {
			return fmt.Errorf("create reading: %w", err)
		}
		return writeAssetFromReading(ctx, tx, s, orgID, assetID, &reading, actor, req.NextReview)
	})
	if txErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, txErr.Error())
	}

	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "asset",
		EntityID:   assetID,
		Action:     "reading",
		ChangedBy:  actor,
		Reason:     fmt.Sprintf("Reading #%d recorded", reading.ID),
	})
	return c.JSON(http.StatusCreated, reading)
}

// assetReviewMonths derives review cycle (months) from the highest CIA value.
// Mirrors the frontend ReadingsPanel logic and the level-based REVIEW_CYCLES
// used for risk: critical=1, high=3, medium=6, low/unset=12.
func assetReviewMonths(c, i, av *int) int {
	max := 0
	for _, v := range []*int{c, i, av} {
		if v != nil && *v > max {
			max = *v
		}
	}
	switch max {
	case 5:
		return 1
	case 4:
		return 3
	case 3:
		return 6
	default:
		return 12
	}
}

// writeAssetFromReading updates the parent asset's CIA values from a reading.
// nextReview, if provided, overrides the auto-computed CIA-based review date.
func writeAssetFromReading(ctx context.Context, tx pgx.Tx, s *Server, orgID int, assetID int64, r *db.EntityReading, actor string, nextReview string) error {
	a, err := s.db.GetAsset(ctx, orgID, assetID)
	if err != nil {
		return fmt.Errorf("asset %d not found: %w", assetID, err)
	}
	if r.Confidentiality != nil {
		a.Confidentiality = r.Confidentiality
	}
	if r.Integrity != nil {
		a.Integrity = r.Integrity
	}
	if r.Availability != nil {
		a.Availability = r.Availability
	}
	// Stamp last_review to now (canonical log lives in entity_readings)
	nowEpoch := db.NewEpoch(time.Now())
	a.LastReview = &nowEpoch
	// Next review: explicit user input wins, otherwise derive from current CIA severity.
	if nextReview != "" {
		if t, err := time.Parse("2006-01-02", nextReview); err == nil {
			e := db.NewEpoch(t)
			a.NextReview = &e
		}
	} else {
		months := assetReviewMonths(a.Confidentiality, a.Integrity, a.Availability)
		next := time.Now().AddDate(0, months, 0)
		e := db.NewEpoch(next)
		a.NextReview = &e
	}
	return db.UpdateAssetTx(ctx, tx, orgID, a)
}

// ═══════════════════════════════════════════════════════════════════════
// LEGAL READINGS
// ═══════════════════════════════════════════════════════════════════════

func (s *Server) handleListLegalReadings(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	legalID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid legal id")
	}
	readings, err := s.db.ListEntityReadings(ctx, orgID, "legal_requirement", legalID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if readings == nil {
		readings = []db.EntityReading{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": readings})
}

func (s *Server) handleCreateLegalReading(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	legalID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid legal id")
	}

	var req struct {
		db.EntityReading
		NextReview string `json:"next_review,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	reading := req.EntityReading
	reading.EntityType = "legal_requirement"
	reading.EntityID = legalID
	actor := getUserEmail(c)
	if reading.AssessedBy == "" {
		reading.AssessedBy = actor
	}

	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.CreateEntityReadingTx(ctx, tx, orgID, &reading); err != nil {
			return fmt.Errorf("create reading: %w", err)
		}
		// Write-through: update parent legal requirement
		return writeLegalFromReading(ctx, tx, s, orgID, int(legalID), &reading, actor, req.NextReview)
	})
	if txErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, txErr.Error())
	}

	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "legal_requirement",
		EntityID:   legalID,
		Action:     "reading",
		ChangedBy:  actor,
		Reason:     fmt.Sprintf("Reading #%d recorded", reading.ID),
	})
	return c.JSON(http.StatusCreated, reading)
}

// writeLegalFromReading updates the parent legal requirement with values from a reading.
// CIA / inherent baselines have moved off legal_requirements (they live on linked risks).
// Each reading just updates current likelihood/impact and recomputes score.
// nextReview, if provided, sets the legal requirement's next review date (YYYY-MM-DD).
func writeLegalFromReading(ctx context.Context, tx pgx.Tx, s *Server, orgID int, legalID int, r *db.EntityReading, actor string, nextReview string) error {
	lr, err := s.db.GetLegalRequirement(ctx, orgID, legalID)
	if err != nil {
		return fmt.Errorf("legal requirement %d not found: %w", legalID, err)
	}

	if r.CurrentLikelihood != nil {
		lr.CurrentLikelihood = r.CurrentLikelihood
	}
	if r.CurrentImpact != nil {
		lr.CurrentImpact = r.CurrentImpact
	}

	// Recompute score and level
	lr.CalculateRiskScore(nil)

	// Stamp last_review to now (canonical log lives in entity_readings)
	now := db.NewEpoch(time.Now())
	lr.LastReview = &now
	// Set next review date from explicit input, or compute from level
	if nextReview != "" {
		if t, err := time.Parse("2006-01-02", nextReview); err == nil {
			e := db.NewEpoch(t)
			lr.NextReview = &e
		}
	} else {
		lr.CalculateReviewDate(nil)
	}

	return db.UpdateLegalRequirementTx(ctx, tx, orgID, lr)
}

// ═══════════════════════════════════════════════════════════════════════
// SUPPLIER READINGS
// ═══════════════════════════════════════════════════════════════════════

func (s *Server) handleListSupplierReadings(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	supplierID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid supplier id")
	}
	readings, err := s.db.ListEntityReadings(ctx, orgID, "supplier", supplierID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if readings == nil {
		readings = []db.EntityReading{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": readings})
}

func (s *Server) handleCreateSupplierReading(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	supplierID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid supplier id")
	}

	var req struct {
		db.EntityReading
		NextReview string `json:"next_review,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	reading := req.EntityReading
	reading.EntityType = "supplier"
	reading.EntityID = supplierID
	actor := getUserEmail(c)
	if reading.AssessedBy == "" {
		reading.AssessedBy = actor
	}

	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.CreateEntityReadingTx(ctx, tx, orgID, &reading); err != nil {
			return fmt.Errorf("create reading: %w", err)
		}
		return writeSupplierFromReading(ctx, tx, s, orgID, supplierID, &reading, actor, req.NextReview)
	})
	if txErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, txErr.Error())
	}

	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "supplier",
		EntityID:   supplierID,
		Action:     "reading",
		ChangedBy:  actor,
		Reason:     fmt.Sprintf("Reading #%d recorded", reading.ID),
	})
	return c.JSON(http.StatusCreated, reading)
}

// writeSupplierFromReading updates the parent supplier's CIA classification + last/next review
// from a reading. nextReview, if provided, overrides the criticality-derived next review date.
func writeSupplierFromReading(ctx context.Context, tx pgx.Tx, s *Server, orgID int, supplierID int64, r *db.EntityReading, actor string, nextReview string) error {
	sup, err := s.db.GetSupplier(ctx, orgID, supplierID)
	if err != nil {
		return fmt.Errorf("supplier %d not found: %w", supplierID, err)
	}
	if r.Confidentiality != nil {
		sup.Confidentiality = r.Confidentiality
	}
	if r.Integrity != nil {
		sup.Integrity = r.Integrity
	}
	if r.Availability != nil {
		sup.Availability = r.Availability
	}

	now := db.NewEpoch(time.Now())
	sup.LastReview = &now

	// Next review: explicit user input wins, otherwise derive from criticality.
	if nextReview != "" {
		if t, err := time.Parse("2006-01-02", nextReview); err == nil {
			e := db.NewEpoch(t)
			sup.NextReview = &e
		}
	} else {
		sup.CalculateNextReview()
	}
	_ = actor
	return db.UpdateSupplierTx(ctx, tx, orgID, sup)
}

// ═══════════════════════════════════════════════════════════════════════
// SYSTEM READINGS
// ═══════════════════════════════════════════════════════════════════════

func (s *Server) handleListSystemReadings(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	systemID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid system id")
	}
	readings, err := s.db.ListEntityReadings(ctx, orgID, "system", systemID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if readings == nil {
		readings = []db.EntityReading{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": readings})
}

func (s *Server) handleCreateSystemReading(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	systemID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid system id")
	}

	var req struct {
		db.EntityReading
		NextReview string `json:"next_review,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	reading := req.EntityReading
	reading.EntityType = "system"
	reading.EntityID = systemID
	actor := getUserEmail(c)
	if reading.AssessedBy == "" {
		reading.AssessedBy = actor
	}

	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.CreateEntityReadingTx(ctx, tx, orgID, &reading); err != nil {
			return fmt.Errorf("create reading: %w", err)
		}
		return writeSystemFromReading(ctx, tx, s, orgID, systemID, &reading, actor, req.NextReview)
	})
	if txErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, txErr.Error())
	}

	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "system",
		EntityID:   systemID,
		Action:     "reading",
		ChangedBy:  actor,
		Reason:     fmt.Sprintf("Reading #%d recorded", reading.ID),
	})
	return c.JSON(http.StatusCreated, reading)
}

// writeSystemFromReading updates the parent system's CIA classification + last/next review
// from a reading. nextReview, if provided, overrides the criticality-derived next review date.
func writeSystemFromReading(ctx context.Context, tx pgx.Tx, s *Server, orgID int, systemID int64, r *db.EntityReading, actor string, nextReview string) error {
	sys, err := s.db.GetSystem(ctx, orgID, systemID)
	if err != nil {
		return fmt.Errorf("system %d not found: %w", systemID, err)
	}
	if r.Confidentiality != nil {
		sys.Confidentiality = r.Confidentiality
	}
	if r.Integrity != nil {
		sys.Integrity = r.Integrity
	}
	if r.Availability != nil {
		sys.Availability = r.Availability
	}

	now := db.NewEpoch(time.Now())
	sys.LastReview = &now

	// Next review: explicit user input wins, otherwise derive from criticality.
	if nextReview != "" {
		if t, err := time.Parse("2006-01-02", nextReview); err == nil {
			e := db.NewEpoch(t)
			sys.NextReview = &e
		}
	} else {
		sys.CalculateNextReview()
	}
	_ = actor
	return db.UpdateSystemTx(ctx, tx, orgID, sys)
}

