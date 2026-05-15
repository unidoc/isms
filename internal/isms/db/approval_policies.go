package db

import (
	"context"
	"fmt"
	"strings"
)

// ApprovalPolicy defines an enforced review policy for documents matching a path pattern.
type ApprovalPolicy struct {
	ID             int      `json:"id"`
	OrganizationID int      `json:"organization_id"`
	Name           string   `json:"name"`
	PathPattern    string   `json:"path_pattern"`
	MinApprovals   int      `json:"min_approvals"`
	RequiredRoles  []string `json:"required_roles"`
	RequiredUsers  []string `json:"required_users"`
	RequireHuman   bool     `json:"require_human"`
	AutoMerge      bool     `json:"auto_merge"`
	Active         bool     `json:"active"`
	CreatedAt      Epoch    `json:"created_at"`
	UpdatedAt      Epoch    `json:"updated_at"`
}

func (d *DB) CreateApprovalPolicy(ctx context.Context, orgID int, p *ApprovalPolicy) error {
	p.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO approval_policies (organization_id, name, path_pattern, min_approvals, required_roles, required_users, require_human, auto_merge, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`, orgID, p.Name, p.PathPattern, p.MinApprovals, p.RequiredRoles, p.RequiredUsers, p.RequireHuman, p.AutoMerge, p.Active,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (d *DB) ListApprovalPolicies(ctx context.Context, orgID int) ([]ApprovalPolicy, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, name, path_pattern, min_approvals, required_roles, required_users, require_human, auto_merge, active, created_at, updated_at
		FROM approval_policies WHERE organization_id = $1
		ORDER BY name ASC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []ApprovalPolicy
	for rows.Next() {
		var p ApprovalPolicy
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.PathPattern, &p.MinApprovals, &p.RequiredRoles, &p.RequiredUsers, &p.RequireHuman, &p.AutoMerge, &p.Active, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, nil
}

func (d *DB) GetApprovalPolicy(ctx context.Context, orgID int, id int) (*ApprovalPolicy, error) {
	var p ApprovalPolicy
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, path_pattern, min_approvals, required_roles, required_users, require_human, auto_merge, active, created_at, updated_at
		FROM approval_policies WHERE id = $1 AND organization_id = $2
	`, id, orgID).Scan(&p.ID, &p.OrganizationID, &p.Name, &p.PathPattern, &p.MinApprovals, &p.RequiredRoles, &p.RequiredUsers, &p.RequireHuman, &p.AutoMerge, &p.Active, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *DB) UpdateApprovalPolicy(ctx context.Context, orgID int, p *ApprovalPolicy) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE approval_policies
		SET name = $3, path_pattern = $4, min_approvals = $5, required_roles = $6, required_users = $7,
			require_human = $8, auto_merge = $9, active = $10, updated_at = now()
		WHERE id = $1 AND organization_id = $2
	`, p.ID, orgID, p.Name, p.PathPattern, p.MinApprovals, p.RequiredRoles, p.RequiredUsers, p.RequireHuman, p.AutoMerge, p.Active)
	return err
}

func (d *DB) DeleteApprovalPolicy(ctx context.Context, orgID int, id int) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM approval_policies WHERE id = $1 AND organization_id = $2`, id, orgID)
	return err
}

// GetPoliciesForDocument returns all active policies whose path_pattern matches the given document path.
// A pattern of "*" matches everything. Otherwise the pattern is matched as a prefix of the document path.
func (d *DB) GetPoliciesForDocument(ctx context.Context, orgID int, documentPath string) ([]ApprovalPolicy, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, name, path_pattern, min_approvals, required_roles, required_users, require_human, auto_merge, active, created_at, updated_at
		FROM approval_policies
		WHERE organization_id = $1 AND active = true
		ORDER BY path_pattern DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matching []ApprovalPolicy
	for rows.Next() {
		var p ApprovalPolicy
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.PathPattern, &p.MinApprovals, &p.RequiredRoles, &p.RequiredUsers, &p.RequireHuman, &p.AutoMerge, &p.Active, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if policyMatches(p.PathPattern, documentPath) {
			matching = append(matching, p)
		}
	}
	return matching, nil
}

// policyMatches checks whether a path_pattern matches a document path.
// "*" matches everything; otherwise the pattern is a prefix match (with normalization).
func policyMatches(pattern, documentPath string) bool {
	if pattern == "*" {
		return true
	}
	// Normalize: strip trailing slashes for consistent prefix matching
	pattern = strings.TrimRight(pattern, "/")
	documentPath = strings.TrimRight(documentPath, "/")
	if documentPath == pattern {
		return true
	}
	return strings.HasPrefix(documentPath, pattern+"/")
}

// PolicyStatus describes whether a review meets its policy requirements.
type PolicyStatus struct {
	PolicyID      int    `json:"policy_id"`
	PolicyName    string `json:"policy_name"`
	Met           bool   `json:"met"`
	RequireHuman  bool   `json:"require_human"`
	AutoMerge     bool   `json:"auto_merge"`
	HumanApproved bool   `json:"human_approved"`
	Approvals     int    `json:"approvals"`
	MinApprovals  int    `json:"min_approvals"`
	CanAutoMerge  bool   `json:"can_auto_merge"`
}

// CheckReviewPolicy checks if a review meets the approval policy for its document.
// Uses both assignments AND approval records — admin/manager can approve without assignment.
func (d *DB) CheckReviewPolicy(ctx context.Context, orgID int, documentPath string, assignments []ReviewAssignment, approvals ...Approval) (*PolicyStatus, error) {
	policies, err := d.GetPoliciesForDocument(ctx, orgID, documentPath)
	if err != nil || len(policies) == 0 {
		return nil, err
	}

	// Use the most specific (first) matching policy
	policy := policies[0]

	approvedBy := map[string]bool{} // track unique approvers

	// Count from assignments (assigned reviewers)
	for _, a := range assignments {
		if a.Status == "approved" {
			approvedBy[a.Reviewer] = true
		}
	}

	// Count from approval records (admin/manager who approved without assignment)
	for _, a := range approvals {
		if a.Decision == "approved" {
			approvedBy[strings.ToLower(a.ApprovedBy)] = true
		}
	}

	approvalCount := len(approvedBy)
	humanApproved := false
	rolesSatisfied := map[string]bool{}
	usersSatisfied := map[string]bool{}

	for email := range approvedBy {
		var isAgent bool
		_ = d.pool.QueryRow(ctx, `SELECT COALESCE(is_agent, false) FROM users WHERE lower(email) = lower($1)`, email).Scan(&isAgent)
		if !isAgent {
			humanApproved = true
		}
		if role, err := d.GetUserRoleByEmail(ctx, orgID, email); err == nil {
			rolesSatisfied[role] = true
		}
		usersSatisfied[email] = true
	}

	// Check all requirements
	met := true
	if approvalCount < policy.MinApprovals {
		met = false
	}
	for _, role := range policy.RequiredRoles {
		if !rolesSatisfied[role] {
			met = false
		}
	}
	for _, email := range policy.RequiredUsers {
		if !usersSatisfied[email] {
			met = false
		}
	}
	if policy.RequireHuman && !humanApproved {
		met = false
	}

	canAutoMerge := met && policy.AutoMerge

	return &PolicyStatus{
		PolicyID:      policy.ID,
		PolicyName:    policy.Name,
		Met:           met,
		RequireHuman:  policy.RequireHuman,
		AutoMerge:     policy.AutoMerge,
		HumanApproved: humanApproved,
		Approvals:     approvalCount,
		MinApprovals:  policy.MinApprovals,
		CanAutoMerge:  canAutoMerge,
	}, nil
}

// GetUserRoleByEmail returns the org role for a user identified by email.
// Returns empty string and error if the user is not a member.
func (d *DB) GetUserRoleByEmail(ctx context.Context, orgID int, email string) (string, error) {
	var role string
	err := d.pool.QueryRow(ctx, `
		SELECT om.role FROM organization_members om
		JOIN users u ON u.id = om.user_id
		WHERE om.organization_id = $1 AND lower(u.email) = lower($2)
	`, orgID, email).Scan(&role)
	if err != nil {
		return "", fmt.Errorf("user role lookup: %w", err)
	}
	return role, nil
}
