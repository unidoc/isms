package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// --- Approval Policy CRUD (admin routes) ---

func (s *Server) handleAdminListPolicies(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	policies, err := s.db.ListApprovalPolicies(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if policies == nil {
		policies = []db.ApprovalPolicy{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": policies})
}

func (s *Server) handleAdminCreatePolicy(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	var p db.ApprovalPolicy
	if err := c.Bind(&p); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// Server-side overwrites: never trust the body for identity / org scope.
	p.ID = 0
	p.OrganizationID = orgID
	if p.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if p.PathPattern == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path_pattern is required")
	}
	p.Active = true // new policies are always active
	if p.MinApprovals < 1 {
		p.MinApprovals = 1
	}
	if p.RequiredRoles == nil {
		p.RequiredRoles = []string{}
	}
	if p.RequiredUsers == nil {
		p.RequiredUsers = []string{}
	}

	if err := s.db.CreateApprovalPolicy(ctx, orgID, &p); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	actor := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "policy_created",
		Detail: fmt.Sprintf("Approval policy %q created (pattern: %s, min: %d)", p.Name, p.PathPattern, p.MinApprovals),
	})

	return c.JSON(http.StatusCreated, p)
}

func (s *Server) handleAdminUpdatePolicy(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid policy id")
	}

	existing, err := s.db.GetApprovalPolicy(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "policy not found")
	}

	// Start from existing policy to preserve bool fields not sent in request
	p := *existing
	if err := c.Bind(&p); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// Server-side overwrites: never trust the body for identity / org scope.
	// A client cannot move a policy to another org via JSON.
	p.ID = existing.ID
	p.OrganizationID = orgID
	if p.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if p.PathPattern == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path_pattern is required")
	}
	if p.MinApprovals < 1 {
		p.MinApprovals = 1
	}
	if p.RequiredRoles == nil {
		p.RequiredRoles = []string{}
	}
	if p.RequiredUsers == nil {
		p.RequiredUsers = []string{}
	}

	if err := s.db.UpdateApprovalPolicy(ctx, orgID, &p); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	actor := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "policy_updated",
		Detail: fmt.Sprintf("Approval policy %q updated", p.Name),
	})

	return c.JSON(http.StatusOK, p)
}

func (s *Server) handleAdminDeletePolicy(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid policy id")
	}

	existing, err := s.db.GetApprovalPolicy(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "policy not found")
	}

	if err := s.db.DeleteApprovalPolicy(ctx, orgID, id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	actor := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "policy_deleted",
		Detail: fmt.Sprintf("Approval policy %q deleted", existing.Name),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Policy status for a review ---

// PolicyViolation describes a single policy that is not satisfied.
type PolicyViolation struct {
	PolicyID     int      `json:"policy_id"`
	PolicyName   string   `json:"policy_name"`
	PathPattern  string   `json:"path_pattern"`
	Satisfied    bool     `json:"satisfied"`
	RequireHuman bool     `json:"require_human"`
	AutoMerge    bool     `json:"auto_merge"`
	Details      []string `json:"details,omitempty"`
}

// handleReviewPolicyStatus returns the policy compliance status for a review.
func (s *Server) handleReviewPolicyStatus(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}

	review, err := s.db.GetReview(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "review not found")
	}

	// Get the document path for policy matching
	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "opening repository")
	}
	docPath := resolveDocPathFromStore(st, review.DocumentID)
	if docPath == "" {
		docPath = review.DocumentID // fallback to document_id itself
	}

	// Canonical policy check — same source of truth as auto-merge
	assignments, _ := s.db.ListAssignmentsForReview(ctx, orgID, id)
	approvals, _ := s.db.ApprovalsForReview(ctx, orgID, id)
	policyResult, _ := s.db.CheckReviewPolicy(ctx, orgID, docPath, assignments, approvals...)

	// No policies = no requirements = satisfied
	allSatisfied := policyResult == nil || policyResult.Met
	canAutoMerge := policyResult != nil && policyResult.CanAutoMerge

	// Build policy detail from canonical result
	var results []PolicyViolation
	if policyResult != nil {
		v := PolicyViolation{
			PolicyID:     policyResult.PolicyID,
			PolicyName:   policyResult.PolicyName,
			PathPattern:  docPath,
			Satisfied:    policyResult.Met,
			RequireHuman: policyResult.RequireHuman,
			AutoMerge:    policyResult.AutoMerge,
		}
		if policyResult.Approvals < policyResult.MinApprovals {
			v.Details = append(v.Details, fmt.Sprintf("need %d approval(s), have %d", policyResult.MinApprovals, policyResult.Approvals))
		}
		if policyResult.RequireHuman && !policyResult.HumanApproved {
			v.Details = append(v.Details, "at least one human approval is required")
		}
		results = append(results, v)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"review_id":      id,
		"document_id":    review.DocumentID,
		"document_path":  docPath,
		"policies":       results,
		"all_satisfied":  allSatisfied,
		"can_auto_merge": canAutoMerge,
	})
}
