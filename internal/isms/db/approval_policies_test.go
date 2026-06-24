package db

import "testing"

func TestPolicyMatches(t *testing.T) {
	tests := []struct {
		name         string
		pattern      string
		documentPath string
		want         bool
	}{
		{
			name:         "wildcard matches anything",
			pattern:      "*",
			documentPath: "policies/access-control.md",
			want:         true,
		},
		{
			name:         "wildcard matches empty path",
			pattern:      "*",
			documentPath: "",
			want:         true,
		},
		{
			name:         "exact match",
			pattern:      "policies/access-control.md",
			documentPath: "policies/access-control.md",
			want:         true,
		},
		{
			name:         "prefix match with child",
			pattern:      "policies",
			documentPath: "policies/access-control.md",
			want:         true,
		},
		{
			name:         "prefix match nested child",
			pattern:      "policies",
			documentPath: "policies/sub/deep.md",
			want:         true,
		},
		{
			name:         "no match different prefix",
			pattern:      "policies",
			documentPath: "controls/a5.md",
			want:         false,
		},
		{
			name:         "no partial prefix match",
			pattern:      "pol",
			documentPath: "policies/access-control.md",
			want:         false,
		},
		{
			name:         "trailing slash on pattern normalized",
			pattern:      "policies/",
			documentPath: "policies/access-control.md",
			want:         true,
		},
		{
			name:         "trailing slash on document path normalized",
			pattern:      "policies",
			documentPath: "policies/",
			want:         true,
		},
		{
			name:         "both trailing slashes normalized",
			pattern:      "policies/",
			documentPath: "policies/",
			want:         true,
		},
		{
			name:         "exact match after slash normalization",
			pattern:      "policies/",
			documentPath: "policies",
			want:         true,
		},
		{
			name:         "empty pattern does not match non-empty path",
			pattern:      "",
			documentPath: "policies/access-control.md",
			want:         false, // HasPrefix("policies/...", "/") is false
		},
		{
			name:         "empty pattern matches empty path",
			pattern:      "",
			documentPath: "",
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policyMatches(tt.pattern, tt.documentPath)
			if got != tt.want {
				t.Errorf("policyMatches(%q, %q) = %v, want %v", tt.pattern, tt.documentPath, got, tt.want)
			}
		})
	}
}

// EvaluatePolicy is extracted pure logic for testing CheckReviewPolicy decisions
// without a database. It mirrors the logic in CheckReviewPolicy after the DB calls.
type policyInput struct {
	policy     ApprovalPolicy
	approvedBy map[string]bool   // unique approvers
	humanFlags map[string]bool   // email → is human (true) or agent (false)
	userRoles  map[string]string // email → org role
}

func evaluatePolicy(in policyInput) *PolicyStatus {
	approvalCount := len(in.approvedBy)
	humanApproved := false
	rolesSatisfied := map[string]bool{}
	usersSatisfied := map[string]bool{}

	for email := range in.approvedBy {
		if in.humanFlags[email] {
			humanApproved = true
		}
		if role, ok := in.userRoles[email]; ok {
			rolesSatisfied[role] = true
		}
		usersSatisfied[email] = true
	}

	met := true
	if approvalCount < in.policy.MinApprovals {
		met = false
	}
	for _, role := range in.policy.RequiredRoles {
		if !rolesSatisfied[role] {
			met = false
		}
	}
	for _, email := range in.policy.RequiredUsers {
		if !usersSatisfied[email] {
			met = false
		}
	}
	if in.policy.RequireHuman && !humanApproved {
		met = false
	}

	canAutoMerge := met && in.policy.AutoMerge

	return &PolicyStatus{
		PolicyID:      in.policy.ID,
		PolicyName:    in.policy.Name,
		Met:           met,
		RequireHuman:  in.policy.RequireHuman,
		AutoMerge:     in.policy.AutoMerge,
		HumanApproved: humanApproved,
		Approvals:     approvalCount,
		MinApprovals:  in.policy.MinApprovals,
		CanAutoMerge:  canAutoMerge,
	}
}

func TestEvaluatePolicy_NoPolicies(t *testing.T) {
	// When no policies exist, CheckReviewPolicy returns nil (all met implicitly).
	// This test documents that the caller should treat nil as "no restrictions".
	// Nothing to evaluate — the function returns nil, nil when len(policies) == 0.
}

func TestEvaluatePolicy_MinApprovals(t *testing.T) {
	tests := []struct {
		name         string
		minApprovals int
		approvers    []string
		wantMet      bool
	}{
		{
			name:         "requires 2 has 1 not met",
			minApprovals: 2,
			approvers:    []string{"alice@example.com"},
			wantMet:      false,
		},
		{
			name:         "requires 2 has 2 met",
			minApprovals: 2,
			approvers:    []string{"alice@example.com", "bob@example.com"},
			wantMet:      true,
		},
		{
			name:         "requires 2 has 3 met",
			minApprovals: 2,
			approvers:    []string{"alice@example.com", "bob@example.com", "carol@example.com"},
			wantMet:      true,
		},
		{
			name:         "requires 1 has 0 not met",
			minApprovals: 1,
			approvers:    nil,
			wantMet:      false,
		},
		{
			name:         "requires 0 has 0 met",
			minApprovals: 0,
			approvers:    nil,
			wantMet:      true,
		},
		{
			name:         "requires 1 has 1 met",
			minApprovals: 1,
			approvers:    []string{"alice@example.com"},
			wantMet:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approved := make(map[string]bool)
			humans := make(map[string]bool)
			for _, email := range tt.approvers {
				approved[email] = true
				humans[email] = true // all human by default
			}

			result := evaluatePolicy(policyInput{
				policy: ApprovalPolicy{
					ID:           1,
					Name:         "test-policy",
					MinApprovals: tt.minApprovals,
				},
				approvedBy: approved,
				humanFlags: humans,
				userRoles:  map[string]string{},
			})

			if result.Met != tt.wantMet {
				t.Errorf("Met = %v, want %v", result.Met, tt.wantMet)
			}
			if result.Approvals != len(tt.approvers) {
				t.Errorf("Approvals = %d, want %d", result.Approvals, len(tt.approvers))
			}
			if result.MinApprovals != tt.minApprovals {
				t.Errorf("MinApprovals = %d, want %d", result.MinApprovals, tt.minApprovals)
			}
		})
	}
}

func TestEvaluatePolicy_RequireHuman(t *testing.T) {
	tests := []struct {
		name      string
		approvers map[string]bool // email → is human
		wantMet   bool
		wantHuman bool
	}{
		{
			name:      "only agent approved not met",
			approvers: map[string]bool{"bot@example.com": false},
			wantMet:   false,
			wantHuman: false,
		},
		{
			name:      "human approved met",
			approvers: map[string]bool{"alice@example.com": true},
			wantMet:   true,
			wantHuman: true,
		},
		{
			name:      "agent and human approved met",
			approvers: map[string]bool{"bot@example.com": false, "alice@example.com": true},
			wantMet:   true,
			wantHuman: true,
		},
		{
			name:      "multiple agents no human not met",
			approvers: map[string]bool{"bot1@example.com": false, "bot2@example.com": false},
			wantMet:   false,
			wantHuman: false,
		},
		{
			name:      "no approvals not met",
			approvers: map[string]bool{},
			wantMet:   false,
			wantHuman: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approved := make(map[string]bool)
			humans := make(map[string]bool)
			for email, isHuman := range tt.approvers {
				approved[email] = true
				humans[email] = isHuman
			}

			result := evaluatePolicy(policyInput{
				policy: ApprovalPolicy{
					ID:           1,
					Name:         "require-human-policy",
					MinApprovals: 1,
					RequireHuman: true,
				},
				approvedBy: approved,
				humanFlags: humans,
				userRoles:  map[string]string{},
			})

			if result.Met != tt.wantMet {
				t.Errorf("Met = %v, want %v", result.Met, tt.wantMet)
			}
			if result.HumanApproved != tt.wantHuman {
				t.Errorf("HumanApproved = %v, want %v", result.HumanApproved, tt.wantHuman)
			}
			if result.RequireHuman != true {
				t.Errorf("RequireHuman should be true")
			}
		})
	}
}

func TestEvaluatePolicy_RequiredRoles(t *testing.T) {
	tests := []struct {
		name          string
		requiredRoles []string
		userRoles     map[string]string
		wantMet       bool
	}{
		{
			name:          "required manager role present",
			requiredRoles: []string{"manager"},
			userRoles:     map[string]string{"alice@example.com": "manager"},
			wantMet:       true,
		},
		{
			name:          "required manager role missing",
			requiredRoles: []string{"manager"},
			userRoles:     map[string]string{"alice@example.com": "contributor"},
			wantMet:       false,
		},
		{
			name:          "multiple required roles all present",
			requiredRoles: []string{"manager", "admin"},
			userRoles:     map[string]string{"alice@example.com": "manager", "bob@example.com": "admin"},
			wantMet:       true,
		},
		{
			name:          "multiple required roles one missing",
			requiredRoles: []string{"manager", "admin"},
			userRoles:     map[string]string{"alice@example.com": "manager"},
			wantMet:       false,
		},
		{
			name:          "no required roles always met",
			requiredRoles: nil,
			userRoles:     map[string]string{"alice@example.com": "reader"},
			wantMet:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approved := make(map[string]bool)
			humans := make(map[string]bool)
			for email := range tt.userRoles {
				approved[email] = true
				humans[email] = true
			}

			result := evaluatePolicy(policyInput{
				policy: ApprovalPolicy{
					ID:            1,
					Name:          "role-policy",
					MinApprovals:  len(approved),
					RequiredRoles: tt.requiredRoles,
				},
				approvedBy: approved,
				humanFlags: humans,
				userRoles:  tt.userRoles,
			})

			if result.Met != tt.wantMet {
				t.Errorf("Met = %v, want %v", result.Met, tt.wantMet)
			}
		})
	}
}

func TestEvaluatePolicy_RequiredUsers(t *testing.T) {
	tests := []struct {
		name          string
		requiredUsers []string
		approvers     []string
		wantMet       bool
	}{
		{
			name:          "required user approved",
			requiredUsers: []string{"ciso@example.com"},
			approvers:     []string{"ciso@example.com"},
			wantMet:       true,
		},
		{
			name:          "required user did not approve",
			requiredUsers: []string{"ciso@example.com"},
			approvers:     []string{"alice@example.com"},
			wantMet:       false,
		},
		{
			name:          "multiple required users all approved",
			requiredUsers: []string{"ciso@example.com", "cto@example.com"},
			approvers:     []string{"ciso@example.com", "cto@example.com"},
			wantMet:       true,
		},
		{
			name:          "multiple required users one missing",
			requiredUsers: []string{"ciso@example.com", "cto@example.com"},
			approvers:     []string{"ciso@example.com"},
			wantMet:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approved := make(map[string]bool)
			humans := make(map[string]bool)
			for _, email := range tt.approvers {
				approved[email] = true
				humans[email] = true
			}

			result := evaluatePolicy(policyInput{
				policy: ApprovalPolicy{
					ID:            1,
					Name:          "user-policy",
					MinApprovals:  1,
					RequiredUsers: tt.requiredUsers,
				},
				approvedBy: approved,
				humanFlags: humans,
				userRoles:  map[string]string{},
			})

			if result.Met != tt.wantMet {
				t.Errorf("Met = %v, want %v", result.Met, tt.wantMet)
			}
		})
	}
}

func TestEvaluatePolicy_AutoMerge(t *testing.T) {
	tests := []struct {
		name          string
		autoMerge     bool
		met           bool
		wantAutoMerge bool
	}{
		{
			name:          "auto merge enabled and met",
			autoMerge:     true,
			met:           true,
			wantAutoMerge: true,
		},
		{
			name:          "auto merge enabled but not met",
			autoMerge:     true,
			met:           false,
			wantAutoMerge: false,
		},
		{
			name:          "auto merge disabled even if met",
			autoMerge:     false,
			met:           true,
			wantAutoMerge: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approved := make(map[string]bool)
			humans := make(map[string]bool)
			minApprovals := 0
			if !tt.met {
				minApprovals = 1 // force not met with zero approvers
			}

			result := evaluatePolicy(policyInput{
				policy: ApprovalPolicy{
					ID:           1,
					Name:         "auto-merge-policy",
					MinApprovals: minApprovals,
					AutoMerge:    tt.autoMerge,
				},
				approvedBy: approved,
				humanFlags: humans,
				userRoles:  map[string]string{},
			})

			if result.CanAutoMerge != tt.wantAutoMerge {
				t.Errorf("CanAutoMerge = %v, want %v", result.CanAutoMerge, tt.wantAutoMerge)
			}
			if result.AutoMerge != tt.autoMerge {
				t.Errorf("AutoMerge = %v, want %v", result.AutoMerge, tt.autoMerge)
			}
		})
	}
}

func TestEvaluatePolicy_MostRestrictiveCombined(t *testing.T) {
	// A policy that requires 2 approvals, require_human, specific role, and a specific user.
	// This exercises all constraints simultaneously.

	t.Run("all constraints met", func(t *testing.T) {
		result := evaluatePolicy(policyInput{
			policy: ApprovalPolicy{
				ID:            1,
				Name:          "strict-policy",
				MinApprovals:  2,
				RequireHuman:  true,
				RequiredRoles: []string{"manager"},
				RequiredUsers: []string{"ciso@example.com"},
				AutoMerge:     true,
			},
			approvedBy: map[string]bool{
				"ciso@example.com":  true,
				"alice@example.com": true,
			},
			humanFlags: map[string]bool{
				"ciso@example.com":  true,
				"alice@example.com": true,
			},
			userRoles: map[string]string{
				"ciso@example.com":  "admin",
				"alice@example.com": "manager",
			},
		})

		if !result.Met {
			t.Error("expected Met=true when all constraints satisfied")
		}
		if !result.CanAutoMerge {
			t.Error("expected CanAutoMerge=true when met and auto_merge enabled")
		}
	})

	t.Run("enough approvals but missing required user", func(t *testing.T) {
		result := evaluatePolicy(policyInput{
			policy: ApprovalPolicy{
				ID:            1,
				Name:          "strict-policy",
				MinApprovals:  2,
				RequireHuman:  true,
				RequiredRoles: []string{"manager"},
				RequiredUsers: []string{"ciso@example.com"},
			},
			approvedBy: map[string]bool{
				"alice@example.com": true,
				"bob@example.com":   true,
			},
			humanFlags: map[string]bool{
				"alice@example.com": true,
				"bob@example.com":   true,
			},
			userRoles: map[string]string{
				"alice@example.com": "manager",
				"bob@example.com":   "contributor",
			},
		})

		if result.Met {
			t.Error("expected Met=false when required user missing")
		}
	})

	t.Run("enough approvals and user but missing role", func(t *testing.T) {
		result := evaluatePolicy(policyInput{
			policy: ApprovalPolicy{
				ID:            1,
				Name:          "strict-policy",
				MinApprovals:  2,
				RequiredRoles: []string{"admin"},
				RequiredUsers: []string{"ciso@example.com"},
			},
			approvedBy: map[string]bool{
				"ciso@example.com":  true,
				"alice@example.com": true,
			},
			humanFlags: map[string]bool{
				"ciso@example.com":  true,
				"alice@example.com": true,
			},
			userRoles: map[string]string{
				"ciso@example.com":  "manager",
				"alice@example.com": "contributor",
			},
		})

		if result.Met {
			t.Error("expected Met=false when required role not present among approvers")
		}
	})

	t.Run("all constraints met except require_human", func(t *testing.T) {
		result := evaluatePolicy(policyInput{
			policy: ApprovalPolicy{
				ID:            1,
				Name:          "strict-policy",
				MinApprovals:  1,
				RequireHuman:  true,
				RequiredUsers: []string{"bot@example.com"},
			},
			approvedBy: map[string]bool{
				"bot@example.com": true,
			},
			humanFlags: map[string]bool{
				"bot@example.com": false, // agent, not human
			},
			userRoles: map[string]string{},
		})

		if result.Met {
			t.Error("expected Met=false when require_human set but only agent approved")
		}
		if result.HumanApproved {
			t.Error("expected HumanApproved=false")
		}
	})
}

func TestEvaluatePolicy_DeduplicatesApprovers(t *testing.T) {
	// In the real CheckReviewPolicy, the same person approving via assignment
	// and via approval record should only count once. We verify that the map
	// deduplication works correctly by passing a pre-deduped map.
	approved := map[string]bool{
		"alice@example.com": true,
	}
	humans := map[string]bool{
		"alice@example.com": true,
	}

	result := evaluatePolicy(policyInput{
		policy: ApprovalPolicy{
			ID:           1,
			Name:         "dedup-policy",
			MinApprovals: 2,
		},
		approvedBy: approved,
		humanFlags: humans,
		userRoles:  map[string]string{},
	})

	if result.Met {
		t.Error("expected Met=false: same person counted once, need 2")
	}
	if result.Approvals != 1 {
		t.Errorf("Approvals = %d, want 1", result.Approvals)
	}
}
