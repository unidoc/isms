package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"isms.sh/internal/isms/db"
)

// Requires a migrated Postgres; skipped otherwise, same as change_status_transition_pg_test.go:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...
//
// Regression for #422: the "Implement CR-n" task created on approval was tied
// to its change by title only, so revoking the approval left it open and the
// next approval created a second one. Tasks are counted by task_type and title
// prefix so these tests need nothing beyond the handlers.

func followupTasks(t *testing.T, s *Server, orgID int, cr *db.ChangeRequest) []db.Task {
	t.Helper()
	all, err := s.db.ListTasks(context.Background(), orgID, db.TaskViewer{CanSeeAll: true}, "", "", 0)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	var out []db.Task
	for _, tk := range all {
		if tk.TaskType == "change_followup" && strings.HasPrefix(tk.Title, "Implement "+cr.Identifier+":") {
			out = append(out, tk)
		}
	}
	return out
}

// onlyFollowup fails unless the change has exactly one follow-up task.
func onlyFollowup(t *testing.T, s *Server, orgID int, cr *db.ChangeRequest) db.Task {
	t.Helper()
	got := followupTasks(t, s, orgID, cr)
	if len(got) != 1 {
		t.Fatalf("%s has %d follow-up tasks, want exactly 1: %+v", cr.Identifier, len(got), got)
	}
	return got[0]
}

func wantTaskStatus(t *testing.T, tk db.Task, want string) {
	t.Helper()
	if tk.Status != want {
		t.Errorf("task %s status = %q, want %q", tk.Identifier, tk.Status, want)
	}
}

func setTaskStatus(t *testing.T, s *Server, orgID int, id int64, status string) {
	t.Helper()
	if err := s.db.UpdateTaskStatus(context.Background(), orgID, id, status); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
}

func TestChangeApprovalCreatesOneFollowupTask(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-followup-control")
	cr := newProposedChange(t, s, orgID, "admin@change-followup-control.test")

	setChangeStatus(t, s, orgID, cr.ID, "approved")
	tk := onlyFollowup(t, s, orgID, cr)
	wantTaskStatus(t, tk, "open")
	if tk.Assignee != cr.RequestedBy {
		t.Errorf("assignee = %q, want the requester %q", tk.Assignee, cr.RequestedBy)
	}

	// An assignee on the change wins over the requester.
	other := newProposedChange(t, s, orgID, "admin@change-followup-control.test")
	other.AssignedTo = "owner@change-followup-control.test"
	contractTestUser(t, s, orgID, other.AssignedTo, "contributor")
	if err := s.db.UpdateChangeRequest(context.Background(), orgID, other.ID, other); err != nil {
		t.Fatalf("UpdateChangeRequest: %v", err)
	}
	setChangeStatus(t, s, orgID, other.ID, "approved")
	if got := onlyFollowup(t, s, orgID, other); got.Assignee != other.AssignedTo {
		t.Errorf("assignee = %q, want the change's assignee %q", got.Assignee, other.AssignedTo)
	}
}

func TestChangeSendBackAndReapproveKeepsOneTask(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-followup-reapprove")
	cr := newProposedChange(t, s, orgID, "admin@change-followup-reapprove.test")

	setChangeStatus(t, s, orgID, cr.ID, "approved")
	first := onlyFollowup(t, s, orgID, cr)

	setChangeStatus(t, s, orgID, cr.ID, "proposed")
	wantTaskStatus(t, onlyFollowup(t, s, orgID, cr), "cancelled")

	setChangeStatus(t, s, orgID, cr.ID, "approved")
	again := onlyFollowup(t, s, orgID, cr)
	if again.ID != first.ID {
		t.Errorf("re-approval used task %d, want the original %d reopened", again.ID, first.ID)
	}
	wantTaskStatus(t, again, "open")
	if again.CompletedAt != nil {
		t.Errorf("completed_at = %v, want cleared", again.CompletedAt)
	}
	if again.DueDate == nil {
		t.Errorf("due date missing after reopen")
	}
}

func TestChangeRejectionCancelsFollowupTask(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-followup-reject")
	cr := newProposedChange(t, s, orgID, "admin@change-followup-reject.test")

	setChangeStatus(t, s, orgID, cr.ID, "approved")
	setChangeStatus(t, s, orgID, cr.ID, "rejected")
	wantTaskStatus(t, onlyFollowup(t, s, orgID, cr), "cancelled")
}

func TestChangeInProgressThenSendBackCancelsFollowupTask(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-followup-inprogress")
	cr := newProposedChange(t, s, orgID, "admin@change-followup-inprogress.test")

	setChangeStatus(t, s, orgID, cr.ID, "approved")
	setChangeStatus(t, s, orgID, cr.ID, "in_progress")
	// Starting the work keeps the approval, so the task stays as it is.
	tk := onlyFollowup(t, s, orgID, cr)
	wantTaskStatus(t, tk, "open")
	setTaskStatus(t, s, orgID, tk.ID, "in_progress")

	setChangeStatus(t, s, orgID, cr.ID, "proposed")
	wantTaskStatus(t, onlyFollowup(t, s, orgID, cr), "cancelled")
}

func TestChangeDoneFollowupTaskIsLeftAloneOnReapproval(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-followup-done")
	cr := newProposedChange(t, s, orgID, "admin@change-followup-done.test")

	setChangeStatus(t, s, orgID, cr.ID, "approved")
	done := onlyFollowup(t, s, orgID, cr)
	setTaskStatus(t, s, orgID, done.ID, "done")

	setChangeStatus(t, s, orgID, cr.ID, "proposed")
	if got := onlyFollowup(t, s, orgID, cr); got.Status != "done" {
		t.Fatalf("done task was touched by the send-back: status = %q", got.Status)
	}
	setChangeStatus(t, s, orgID, cr.ID, "approved")

	got := followupTasks(t, s, orgID, cr)
	if len(got) != 2 {
		t.Fatalf("%d follow-up tasks, want 2 (the done one plus a new one): %+v", len(got), got)
	}
	open := 0
	for _, tk := range got {
		switch {
		case tk.ID == done.ID:
			wantTaskStatus(t, tk, "done")
		case tk.Status == "open":
			open++
		default:
			t.Errorf("unexpected task %s status %q", tk.Identifier, tk.Status)
		}
	}
	if open != 1 {
		t.Errorf("%d open tasks, want 1", open)
	}
}

func TestChangeEditFormWithdrawalAndReapprovalFollowTask(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-followup-form")
	cr := newApprovedChange(t, s, orgID)
	first := onlyFollowup(t, s, orgID, cr)
	wantTaskStatus(t, first, "open")

	// A substance edit withdraws the approval (#197): back to proposed.
	if got := putChange(t, s, orgID, cr.ID, `{"description":"rewritten substantially"}`); got.Status != "proposed" {
		t.Fatalf("status = %q, want proposed", got.Status)
	}
	wantTaskStatus(t, onlyFollowup(t, s, orgID, cr), "cancelled")

	putChange(t, s, orgID, cr.ID, `{"status":"approved"}`)
	again := onlyFollowup(t, s, orgID, cr)
	if again.ID != first.ID {
		t.Errorf("edit-form approval used task %d, want the original %d reopened", again.ID, first.ID)
	}
	wantTaskStatus(t, again, "open")
}

func TestChangeSuggestionApplyFollowsFollowupTask(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-followup-suggestion")
	contractTestUser(t, s, orgID, suggestionChangelogActor, "manager")
	cr := newProposedChange(t, s, orgID, "admin@change-followup-suggestion.test")

	apply := func(status string) {
		t.Helper()
		sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
			EntityType:     "change_request",
			EntityID:       cr.Identifier,
			SuggestionType: "update",
			Payload:        json.RawMessage(`{"fields":{"status":"` + status + `"}}`),
		})
		applySuggestion(t, s, orgID, sg.ID)
	}

	apply("approved")
	first := onlyFollowup(t, s, orgID, cr)
	wantTaskStatus(t, first, "open")

	apply("proposed")
	wantTaskStatus(t, onlyFollowup(t, s, orgID, cr), "cancelled")

	apply("approved")
	again := onlyFollowup(t, s, orgID, cr)
	if again.ID != first.ID {
		t.Errorf("suggestion approval used task %d, want the original %d reopened", again.ID, first.ID)
	}
	wantTaskStatus(t, again, "open")
}

func TestChangeRevokingOneChangeLeavesAnothersTaskAlone(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-followup-two")
	a := newProposedChange(t, s, orgID, "admin@change-followup-two.test")
	b := newProposedChange(t, s, orgID, "admin@change-followup-two.test")

	setChangeStatus(t, s, orgID, a.ID, "approved")
	setChangeStatus(t, s, orgID, b.ID, "approved")
	setChangeStatus(t, s, orgID, a.ID, "proposed")

	wantTaskStatus(t, onlyFollowup(t, s, orgID, a), "cancelled")
	wantTaskStatus(t, onlyFollowup(t, s, orgID, b), "open")
}
