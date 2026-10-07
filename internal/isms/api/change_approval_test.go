package api

import (
	"testing"
	"time"

	"isms.sh/internal/isms/db"
)

func TestChangeSubstanceChanged(t *testing.T) {
	planned := db.NewEpoch(time.Now())
	base := func() *db.ChangeRequest {
		return &db.ChangeRequest{
			Type: "change", Title: "t", Description: "d", Justification: "j",
			Priority: "medium", Category: "other", RiskLevel: "low", RollbackPlan: "r",
		}
	}
	substance := map[string]func(*db.ChangeRequest){
		"description":   func(c *db.ChangeRequest) { c.Description = "x" },
		"justification": func(c *db.ChangeRequest) { c.Justification = "x" },
		"risk_level":    func(c *db.ChangeRequest) { c.RiskLevel = "critical" },
		"rollback_plan": func(c *db.ChangeRequest) { c.RollbackPlan = "x" },
		"type":          func(c *db.ChangeRequest) { c.Type = "access_request" },
		"category":      func(c *db.ChangeRequest) { c.Category = "x" },
		"priority":      func(c *db.ChangeRequest) { c.Priority = "high" },
	}
	for name, mutate := range substance {
		t.Run(name, func(t *testing.T) {
			updated := base()
			mutate(updated)
			if !changeSubstanceChanged(base(), updated) {
				t.Errorf("changing %s should count as substance", name)
			}
		})
	}
	other := map[string]func(*db.ChangeRequest){
		"title":       func(c *db.ChangeRequest) { c.Title = "x" },
		"notes":       func(c *db.ChangeRequest) { c.Notes = "x" },
		"assigned_to": func(c *db.ChangeRequest) { c.AssignedTo = "x@y.test" },
		"planned_at":  func(c *db.ChangeRequest) { c.PlannedAt = &planned },
	}
	for name, mutate := range other {
		t.Run(name, func(t *testing.T) {
			updated := base()
			mutate(updated)
			if changeSubstanceChanged(base(), updated) {
				t.Errorf("changing %s should not count as substance", name)
			}
		})
	}
}

func TestChangeApprovalWithdrawn(t *testing.T) {
	approvedAt := db.NewEpoch(time.Now())
	cases := []struct {
		name   string
		status string
		stamp  bool
		edit   func(*db.ChangeRequest)
		target string
		want   bool
	}{
		{"approved, description, stays approved", "approved", true, func(c *db.ChangeRequest) { c.Description = "new" }, "approved", true},
		{"approved, description, to in_progress", "approved", true, func(c *db.ChangeRequest) { c.Description = "new" }, "in_progress", true},
		{"in_progress, risk_level, stays", "in_progress", true, func(c *db.ChangeRequest) { c.RiskLevel = "critical" }, "in_progress", true},
		{"approved, only title", "approved", true, func(c *db.ChangeRequest) { c.Title = "new" }, "approved", false},
		{"approved, description, to rejected", "approved", true, func(c *db.ChangeRequest) { c.Description = "new" }, "rejected", false},
		{"proposed, description, to approved", "proposed", false, func(c *db.ChangeRequest) { c.Description = "new" }, "approved", false},
		{"in_progress without stamp", "in_progress", false, func(c *db.ChangeRequest) { c.Description = "new" }, "in_progress", false},
		{"implemented, description", "implemented", true, func(c *db.ChangeRequest) { c.Description = "new" }, "implemented", false},
		{"closed, description", "closed", true, func(c *db.ChangeRequest) { c.Description = "new" }, "closed", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := &db.ChangeRequest{Status: tc.status, Description: "d", Title: "t", RiskLevel: "low"}
			if tc.stamp {
				old.ApprovedAt = &approvedAt
			}
			updated := *old
			tc.edit(&updated)
			if got := changeApprovalWithdrawn(old, &updated, tc.target); got != tc.want {
				t.Errorf("changeApprovalWithdrawn = %v, want %v", got, tc.want)
			}
		})
	}
}
