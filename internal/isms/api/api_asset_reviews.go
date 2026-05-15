package api

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// assetReviewCreateRequest is the body schema for POST /assets/:id/reviews.
// Server-managed fields (AssetID, ReviewedBy, OrganizationID, ID, CreatedAt)
// are deliberately omitted — they are set from the URL param, JWT, and DB.
type assetReviewCreateRequest struct {
	Outcome                string `json:"outcome"`
	ClassificationVerified bool   `json:"classification_verified"`
	OwnershipVerified      bool   `json:"ownership_verified"`
	Notes                  string `json:"notes"`
}

func (s *Server) handleCreateAssetReview(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid asset id")
	}

	// Verify asset exists in this org (cross-org safety).
	asset, err := s.db.GetAsset(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}

	var req assetReviewCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Notes == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "notes are required — describe what was reviewed and confirmed")
	}

	ar := db.AssetReview{
		AssetID:                id,
		ReviewedBy:             actor,
		Outcome:                req.Outcome,
		ClassificationVerified: req.ClassificationVerified,
		OwnershipVerified:      req.OwnershipVerified,
		Notes:                  req.Notes,
	}

	if err := s.db.CreateAssetReview(ctx, orgID, &ar); err != nil {
		return pgxHTTPError(err)
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "asset_reviewed",
		Detail: "Asset review completed for " + asset.Name + ": " + ar.Outcome,
	})

	return c.JSON(http.StatusCreated, ar)
}

func (s *Server) handleListAssetReviews(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid asset id")
	}

	reviews, err := s.db.ListAssetReviews(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if reviews == nil {
		reviews = []db.AssetReview{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": reviews})
}
