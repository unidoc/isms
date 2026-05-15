package api

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// supplierReviewCreateRequest is the body schema for POST /suppliers/:id/reviews.
// Server-managed fields (SupplierID, ReviewedBy, OrganizationID, ID, CreatedAt)
// are deliberately omitted — they are set from the URL param, JWT, and DB.
type supplierReviewCreateRequest struct {
	Outcome                string `json:"outcome"`
	CertificationsVerified bool   `json:"certifications_verified"`
	DataHandlingVerified   bool   `json:"data_handling_verified"`
	SLAMet                 bool   `json:"sla_met"`
	Notes                  string `json:"notes"`
}

func (s *Server) handleCreateSupplierReview(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid supplier id")
	}

	// Verify supplier exists in this org (cross-org safety).
	sup, err := s.db.GetSupplier(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "supplier not found")
	}

	var req supplierReviewCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Notes == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "notes are required — describe what was reviewed and confirmed")
	}

	sr := db.SupplierReview{
		SupplierID:             id,
		ReviewedBy:             actor,
		Outcome:                req.Outcome,
		CertificationsVerified: req.CertificationsVerified,
		DataHandlingVerified:   req.DataHandlingVerified,
		SLAMet:                 req.SLAMet,
		Notes:                  req.Notes,
	}

	if err := s.db.CreateSupplierReview(ctx, orgID, &sr); err != nil {
		return pgxHTTPError(err)
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "supplier_reviewed",
		Detail: "Supplier review completed for " + sup.Name + ": " + sr.Outcome,
	})

	return c.JSON(http.StatusCreated, sr)
}

func (s *Server) handleListSupplierReviews(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid supplier id")
	}

	reviews, err := s.db.ListSupplierReviews(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if reviews == nil {
		reviews = []db.SupplierReview{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": reviews})
}
