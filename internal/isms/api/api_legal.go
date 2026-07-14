package api

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// --- Request DTOs ---

type legalCreateRequest struct {
	Title             string           `json:"title"`
	Description       string           `json:"description"`
	Jurisdiction      string           `json:"jurisdiction"`
	Category          string           `json:"category"`
	Reference         string           `json:"reference"`
	URL               string           `json:"url"`
	Status            string           `json:"status"`
	Owner             string           `json:"owner"`
	LastReview        *db.Epoch        `json:"last_review"`
	NextReview        *db.Epoch        `json:"next_review"`
	Notes             string           `json:"notes"`
	CurrentLikelihood *int             `json:"current_likelihood"`
	CurrentImpact     *int             `json:"current_impact"`
	Treatment         string           `json:"treatment"`
	TreatmentPlan     string           `json:"treatment_plan"`
	TargetLikelihood  *int             `json:"target_likelihood"`
	TargetImpact      *int             `json:"target_impact"`
	Completion        int              `json:"completion"`
	References        []ReferenceInput `json:"references"`
}

type legalUpdateRequest struct {
	Title             *string    `json:"title"`
	Description       *string    `json:"description"`
	Jurisdiction      *string    `json:"jurisdiction"`
	Category          *string    `json:"category"`
	Reference         *string    `json:"reference"`
	URL               *string    `json:"url"`
	Status            *string    `json:"status"`
	Owner             *string    `json:"owner"`
	LastReview        **db.Epoch `json:"last_review"`
	NextReview        **db.Epoch `json:"next_review"`
	Notes             *string    `json:"notes"`
	CurrentLikelihood **int      `json:"current_likelihood"`
	CurrentImpact     **int      `json:"current_impact"`
	Treatment         *string    `json:"treatment"`
	TreatmentPlan     *string    `json:"treatment_plan"`
	TargetLikelihood  **int      `json:"target_likelihood"`
	TargetImpact      **int      `json:"target_impact"`
	Completion        *int       `json:"completion"`
}

func (s *Server) handleLegalStats(c echo.Context) error {
	orgID := getOrgID(c)
	stats, err := s.db.LegalStats(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) handleListLegal(c echo.Context) error {
	orgID := getOrgID(c)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	params := db.LegalListParams{
		Page:     page,
		Limit:    limit,
		Sort:     c.QueryParam("sort"),
		Search:   c.QueryParam("q"),
		Level:    c.QueryParam("level"),
		Category: c.QueryParam("category"),
		Status:   c.QueryParam("status"),
	}
	items, total, err := s.db.PaginatedLegalRequirements(c.Request().Context(), orgID, params)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Limit < 1 {
		params.Limit = 50
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":      items,
		"total":     total,
		"page":      params.Page,
		"page_size": params.Limit,
	})
}

func (s *Server) handleCreateLegal(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)

	var req legalCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	lr := db.LegalRequirement{
		Title:             req.Title,
		Description:       req.Description,
		Jurisdiction:      req.Jurisdiction,
		Category:          req.Category,
		Reference:         req.Reference,
		URL:               req.URL,
		Status:            req.Status,
		Owner:             req.Owner,
		LastReview:        req.LastReview,
		NextReview:        req.NextReview,
		Notes:             req.Notes,
		CurrentLikelihood: req.CurrentLikelihood,
		CurrentImpact:     req.CurrentImpact,
		Treatment:         req.Treatment,
		TreatmentPlan:     req.TreatmentPlan,
		TargetLikelihood:  req.TargetLikelihood,
		TargetImpact:      req.TargetImpact,
		Completion:        req.Completion,
	}

	applyLegalDefaults(&lr, getUserEmail(c))
	if err := validateLegalCreate(&lr); err != nil {
		return err
	}

	ctx := c.Request().Context()
	if err := s.db.CreateLegalRequirement(ctx, orgID, &lr); err != nil {
		return pgxHTTPError(err)
	}

	actor := getUserEmail(c)
	s.createReferencesForEntity(ctx, orgID, "legal_requirement", lr.Identifier, actor, req.References)

	s.logChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "legal_requirement",
		EntityID:   int64(lr.ID),
		Action:     "create",
		ChangedBy:  actor,
	})

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "legal_requirement_created",
		Detail: lr.Title,
	})

	s.searchUpsert(orgID, "legal_requirement", lr.Identifier, lr.Title, lr.Identifier+" "+lr.Title+" "+lr.Description+" "+lr.Jurisdiction)

	return c.JSON(http.StatusCreated, lr)
}

func (s *Server) handleGetLegal(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid legal requirement id")
	}
	lr, err := s.db.GetLegalRequirement(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "legal requirement not found")
	}
	return c.JSON(http.StatusOK, lr)
}

func (s *Server) handleUpdateLegal(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid legal requirement id")
	}

	ctx := c.Request().Context()
	existing, err := s.db.GetLegalRequirement(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "legal requirement not found")
	}

	var req legalUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Status != nil {
		if err := validateEnum("status", *req.Status, db.LegalStatuses); err != nil {
			return err
		}
	}
	if req.Treatment != nil {
		if err := validateEnum("treatment", *req.Treatment, db.LegalTreatments); err != nil {
			return err
		}
	}
	if req.Category != nil {
		if err := validateEnum("category", *req.Category, db.LegalCategories); err != nil {
			return err
		}
	}
	if req.Owner != nil && *req.Owner != "" {
		if err := s.validateOrgMember(c, *req.Owner); err != nil {
			return err
		}
	}

	if req.Title != nil {
		existing.Title = *req.Title
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Jurisdiction != nil {
		existing.Jurisdiction = *req.Jurisdiction
	}
	if req.Category != nil {
		existing.Category = *req.Category
	}
	if req.Reference != nil {
		existing.Reference = *req.Reference
	}
	if req.URL != nil {
		existing.URL = *req.URL
	}
	if req.Owner != nil {
		existing.Owner = *req.Owner
	}
	if req.LastReview != nil {
		existing.LastReview = *req.LastReview
	}
	if req.NextReview != nil {
		existing.NextReview = *req.NextReview
	}
	if req.Notes != nil {
		existing.Notes = *req.Notes
	}
	if req.CurrentLikelihood != nil {
		existing.CurrentLikelihood = *req.CurrentLikelihood
	}
	if req.CurrentImpact != nil {
		existing.CurrentImpact = *req.CurrentImpact
	}
	if req.Treatment != nil {
		existing.Treatment = *req.Treatment
	}
	if req.TreatmentPlan != nil {
		existing.TreatmentPlan = *req.TreatmentPlan
	}
	if req.Status != nil {
		existing.Status = *req.Status
	}
	if req.TargetLikelihood != nil {
		existing.TargetLikelihood = *req.TargetLikelihood
	}
	if req.TargetImpact != nil {
		existing.TargetImpact = *req.TargetImpact
	}
	if req.Completion != nil {
		existing.Completion = *req.Completion
	}

	oldMap := existing.ToChangeMap()
	existing.ID = id
	// UpdateLegalRequirement recomputes inherent/current scores internally.
	if err := s.db.UpdateLegalRequirement(ctx, orgID, existing); err != nil {
		return pgxHTTPError(err)
	}

	after, _ := s.db.GetLegalRequirement(ctx, orgID, id)
	if after != nil {
		actor := getUserEmail(c)
		reason := c.QueryParam("reason")
		changes := db.DiffFields("legal_requirement", int64(id), actor, reason, oldMap, after.ToChangeMap())
		if len(changes) > 0 {
			s.logChanges(ctx, orgID, changes)
		}
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  getUserEmail(c),
		Action: "legal_requirement_updated",
		Detail: existing.Title,
	})

	s.searchUpsert(orgID, "legal_requirement", existing.Identifier, existing.Title, existing.Identifier+" "+existing.Title+" "+existing.Description+" "+existing.Jurisdiction)

	if after != nil {
		return c.JSON(http.StatusOK, after)
	}
	return c.JSON(http.StatusOK, existing)
}

func (s *Server) handleDeleteLegal(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid legal requirement id")
	}

	ctx := c.Request().Context()

	// Get title for activity log before deleting
	lr, _ := s.db.GetLegalRequirement(ctx, orgID, id)
	title := ""
	if lr != nil {
		title = lr.Title
	}

	if err := s.db.DeleteLegalRequirement(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}

	if lr != nil {
		s.logChange(ctx, orgID, &db.ChangelogEntry{
			EntityType: "legal_requirement",
			EntityID:   int64(lr.ID),
			Action:     "delete",
			ChangedBy:  getUserEmail(c),
		})
		s.searchRemove(orgID, "legal_requirement", lr.Identifier)
	}

	if title != "" {
		s.logAndNotify(ctx, orgID, &db.Activity{
			Actor:  getUserEmail(c),
			Action: "legal_requirement_deleted",
			Detail: title,
		})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
