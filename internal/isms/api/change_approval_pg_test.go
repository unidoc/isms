package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"isms.sh/internal/isms/db"
)

// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays green
// without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...
//
// Regression for #197: approving a change and then starting work must keep the
// approval, while rewriting the approved content must withdraw it (status back
// to proposed, approval cleared, one changelog row carrying the reason).

const changeApprovalActor = "admin@id-resolution.test"

func newApprovedChange(t *testing.T, s *Server, orgID int) *db.ChangeRequest {
	t.Helper()
	// requested_by_id is NOT NULL, so the actor must exist as a user.
	contractTestUser(t, s, orgID, changeApprovalActor, "admin")
	cr := &db.ChangeRequest{
		Title: "approval change", Description: "original", Justification: "because",
		Priority: "medium", Category: "other", RiskLevel: "low", RollbackPlan: "revert",
		RequestedBy: changeApprovalActor, Status: "proposed",
	}
	if err := s.db.CreateChangeRequest(context.Background(), orgID, cr); err != nil {
		t.Fatalf("CreateChangeRequest: %v", err)
	}
	return setChangeStatus(t, s, orgID, cr.ID, "approved")
}

func setChangeStatus(t *testing.T, s *Server, orgID, id int, status string) *db.ChangeRequest {
	t.Helper()
	c, rec := ctxFor(orgID, http.MethodPut, itoa(id), `{"status":"`+status+`"}`)
	if err := s.handleUpdateChangeStatus(c); err != nil {
		t.Fatalf("status %s: %v", status, err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %s: code %d, body %s", status, rec.Code, rec.Body.String())
	}
	// The status endpoint answers with just {"status": ...}; reload the row.
	got, err := s.db.GetChangeRequest(context.Background(), orgID, id)
	if err != nil {
		t.Fatalf("GetChangeRequest: %v", err)
	}
	return got
}

func putChange(t *testing.T, s *Server, orgID, id int, body string) *db.ChangeRequest {
	t.Helper()
	c, rec := ctxFor(orgID, http.MethodPut, itoa(id), body)
	if err := s.handleUpdateChange(c); err != nil {
		t.Fatalf("PUT change: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT change: code %d, body %s", rec.Code, rec.Body.String())
	}
	return decodeChange(t, rec.Body.Bytes())
}

func decodeChange(t *testing.T, b []byte) *db.ChangeRequest {
	t.Helper()
	var cr db.ChangeRequest
	if err := json.Unmarshal(b, &cr); err != nil {
		t.Fatalf("decoding change: %v (%s)", err, b)
	}
	return &cr
}

func assertApprovalKept(t *testing.T, got *db.ChangeRequest, want *db.ChangeRequest) {
	t.Helper()
	if got.ApprovedBy != want.ApprovedBy || got.ApprovedAt == nil || want.ApprovedAt == nil ||
		got.ApprovedAt.Time.Unix() != want.ApprovedAt.Time.Unix() {
		t.Errorf("approval changed: by=%q at=%v, want by=%q at=%v", got.ApprovedBy, got.ApprovedAt, want.ApprovedBy, want.ApprovedAt)
	}
}

func assertApprovalCleared(t *testing.T, got *db.ChangeRequest) {
	t.Helper()
	if got.Status != "proposed" || got.ApprovedBy != "" || got.ApprovedAt != nil {
		t.Errorf("status=%q approved_by=%q approved_at=%v, want proposed with no approval", got.Status, got.ApprovedBy, got.ApprovedAt)
	}
}

func TestChangeApprovalSurvivesStartingWork(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-approval-start")
	cr := newApprovedChange(t, s, orgID)

	got := putChange(t, s, orgID, cr.ID, `{"status":"in_progress"}`)
	if got.Status != "in_progress" {
		t.Fatalf("status = %q, want in_progress", got.Status)
	}
	assertApprovalKept(t, got, cr)
}

func TestChangeRewriteWithdrawsApproval(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-approval-rewrite")
	cr := newApprovedChange(t, s, orgID)

	got := putChange(t, s, orgID, cr.ID, `{"description":"rewritten","risk_level":"critical"}`)
	assertApprovalCleared(t, got)

	entries, err := s.db.ListEntityChangelog(context.Background(), orgID, "change_request", int64(cr.ID))
	if err != nil {
		t.Fatalf("ListEntityChangelog: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "update" && e.Field == "status" && e.OldValue != nil && *e.OldValue == "approved" &&
			e.NewValue != nil && *e.NewValue == "proposed" {
			found = true
			if e.Reason != changeApprovalWithdrawnReason {
				t.Errorf("reason = %q, want %q", e.Reason, changeApprovalWithdrawnReason)
			}
		}
	}
	if !found {
		t.Errorf("no status approved -> proposed changelog row (got %d rows)", len(entries))
	}
}

// The web edit form always sends every field, including the unchanged status.
func TestChangeFullFormSave(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-approval-form")
	cr := newApprovedChange(t, s, orgID)

	form := func(title, justification string) string {
		b, _ := json.Marshal(map[string]any{
			"type": "change", "title": title, "description": "original", "justification": justification,
			"priority": "medium", "category": "other", "risk_level": "low", "rollback_plan": "revert",
			"notes": "", "assigned_to": "", "status": "approved",
		})
		return string(b)
	}

	got := putChange(t, s, orgID, cr.ID, form("new title only", "because"))
	if got.Status != "approved" || got.Title != "new title only" {
		t.Fatalf("title-only save: status=%q title=%q, want approved / new title only", got.Status, got.Title)
	}
	assertApprovalKept(t, got, cr)

	got = putChange(t, s, orgID, cr.ID, form("new title only", "a different justification"))
	assertApprovalCleared(t, got)
}

func TestChangeRewriteWhileInProgressWithdrawsApproval(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-approval-inprogress")
	cr := newApprovedChange(t, s, orgID)
	setChangeStatus(t, s, orgID, cr.ID, "in_progress")

	got := putChange(t, s, orgID, cr.ID, `{"rollback_plan":"new"}`)
	assertApprovalCleared(t, got)
}

func TestChangeRewriteAfterImplementedKeepsApproval(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-approval-implemented")
	cr := newApprovedChange(t, s, orgID)
	setChangeStatus(t, s, orgID, cr.ID, "implemented")

	got := putChange(t, s, orgID, cr.ID, `{"description":"after the fact"}`)
	if got.Status != "implemented" || got.Description != "after the fact" {
		t.Errorf("status=%q description=%q, want implemented / saved description", got.Status, got.Description)
	}
	assertApprovalKept(t, got, cr)
}

func TestChangeSuggestionRewriteWithdrawsApproval(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-approval-suggestion")
	cr := newApprovedChange(t, s, orgID)

	sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:     "change_request",
		EntityID:       cr.Identifier,
		SuggestionType: "update",
		Payload:        json.RawMessage(`{"fields":{"description":"via suggestion"}}`),
	})
	applySuggestion(t, s, orgID, sg.ID)

	got, err := s.db.GetChangeRequest(context.Background(), orgID, cr.ID)
	if err != nil {
		t.Fatalf("GetChangeRequest: %v", err)
	}
	assertApprovalCleared(t, got)
	if got.Description != "via suggestion" {
		t.Errorf("description = %q, want the suggestion's value", got.Description)
	}

	entries, err := s.db.ListEntityChangelog(context.Background(), orgID, "change_request", int64(cr.ID))
	if err != nil {
		t.Fatalf("ListEntityChangelog: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Field == "status" && strings.Contains(e.Reason, "suggestion #") && strings.Contains(e.Reason, changeApprovalWithdrawnReason) {
			found = true
		}
	}
	if !found {
		t.Errorf("no status changelog row with a suggestion + withdrawal reason (got %d rows)", len(entries))
	}
}
