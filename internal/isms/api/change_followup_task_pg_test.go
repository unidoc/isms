package api

import (
	"context"
	"encoding/json"
	"net/http"
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

// Regression for #478: reopening an old follow-up task while the change already
// has a newer live one broke idx_tasks_change_request_live, and the raw
// Postgres message reached the user.

// liveConflictFixture is a change with an old follow-up task in oldStatus and a
// newer open one, made by approving, finishing the task, sending the change
// back and approving again.
func liveConflictFixture(t *testing.T, s *Server, slug, oldStatus string) (orgID int, cr *db.ChangeRequest, old, live db.Task) {
	t.Helper()
	orgID = newTestOrg(t, s, slug)
	cr = newProposedChange(t, s, orgID, "admin@"+slug+".test")
	setChangeStatus(t, s, orgID, cr.ID, "approved")
	old = onlyFollowup(t, s, orgID, cr)
	setTaskStatus(t, s, orgID, old.ID, oldStatus)
	setChangeStatus(t, s, orgID, cr.ID, "proposed")
	setChangeStatus(t, s, orgID, cr.ID, "approved")
	for _, tk := range followupTasks(t, s, orgID, cr) {
		if tk.ID != old.ID {
			live = tk
		}
	}
	if live.ID == 0 || live.Status != "open" {
		t.Fatalf("fixture: want a new open task beside %s, got %+v", old.Identifier, followupTasks(t, s, orgID, cr))
	}
	return orgID, cr, old, live
}

func wantLiveConflict(t *testing.T, err error, cr *db.ChangeRequest) {
	t.Helper()
	he := wantHTTPStatus(t, err, http.StatusConflict)
	eb, ok := he.Message.(errorBody)
	if !ok || eb.Code != CodeChangeTaskAlreadyLive {
		t.Fatalf("expected %s, got %#v", CodeChangeTaskAlreadyLive, he.Message)
	}
	if eb.Params[ParamRef] != cr.Identifier {
		t.Errorf("params = %#v, want ref=%q", eb.Params, cr.Identifier)
	}
	if want := cr.Identifier + " already has an open implementation task"; eb.Message != want {
		t.Errorf("message = %q, want %q", eb.Message, want)
	}
	if strings.Contains(eb.Message, "idx_tasks_change_request_live") || strings.Contains(eb.Message, "duplicate key") {
		t.Errorf("message leaks the constraint: %q", eb.Message)
	}
}

// wantOnlyLive checks the refused write changed nothing.
func wantOnlyLive(t *testing.T, s *Server, orgID int, cr *db.ChangeRequest, old, live db.Task, oldStatus string) {
	t.Helper()
	for _, tk := range followupTasks(t, s, orgID, cr) {
		switch tk.ID {
		case old.ID:
			wantTaskStatus(t, tk, oldStatus)
		case live.ID:
			wantTaskStatus(t, tk, "open")
		default:
			t.Errorf("unexpected task %s", tk.Identifier)
		}
	}
}

func TestChangeReopenOldTaskViaStatusHandlerIsRefused(t *testing.T) {
	for _, target := range []string{"open", "in_progress"} {
		t.Run(target, func(t *testing.T) {
			s := testServer(t)
			orgID, cr, old, live := liveConflictFixture(t, s, "change-live-status-"+strings.ReplaceAll(target, "_", "-"), "done")
			c, _ := ctxFor(orgID, http.MethodPut, itoa(int(old.ID)), `{"status":"`+target+`"}`)
			wantLiveConflict(t, s.handleUpdateTaskStatus(c), cr)
			wantOnlyLive(t, s, orgID, cr, old, live, "done")
		})
	}
}

func TestChangeReopenOldTaskViaUpdateHandlerIsRefused(t *testing.T) {
	s := testServer(t)
	orgID, cr, old, live := liveConflictFixture(t, s, "change-live-update", "done")
	c, _ := ctxFor(orgID, http.MethodPut, itoa(int(old.ID)), `{"status":"open"}`)
	wantLiveConflict(t, s.handleUpdateTask(c), cr)
	wantOnlyLive(t, s, orgID, cr, old, live, "done")
}

func TestChangeReopenOldTaskViaSuggestionApplyIsRefused(t *testing.T) {
	s := testServer(t)
	orgID, cr, old, live := liveConflictFixture(t, s, "change-live-suggestion", "done")
	contractTestUser(t, s, orgID, suggestionChangelogActor, "manager")
	sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:     "task",
		EntityID:       old.Identifier,
		SuggestionType: "update",
		Payload:        json.RawMessage(`{"fields":{"status":"open"}}`),
	})
	c, _ := suggestionCtx(orgID, http.MethodPost, sg.ID)
	wantLiveConflict(t, s.handleApplyEntitySuggestion(c), cr)
	wantOnlyLive(t, s, orgID, cr, old, live, "done")
}

func TestChangeReopenCancelledSiblingIsRefused(t *testing.T) {
	s := testServer(t)
	orgID, cr, old, live := liveConflictFixture(t, s, "change-live-cancelled", "done")
	setTaskStatus(t, s, orgID, old.ID, "cancelled")
	c, _ := ctxFor(orgID, http.MethodPut, itoa(int(old.ID)), `{"status":"open"}`)
	wantLiveConflict(t, s.handleUpdateTaskStatus(c), cr)
	wantOnlyLive(t, s, orgID, cr, old, live, "cancelled")
}

// Control: with no live sibling the same reopen still works.
func TestChangeReopenDoneTaskWithoutLiveSiblingSucceeds(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-live-control")
	cr := newProposedChange(t, s, orgID, "admin@change-live-control.test")
	setChangeStatus(t, s, orgID, cr.ID, "approved")
	tk := onlyFollowup(t, s, orgID, cr)
	setTaskStatus(t, s, orgID, tk.ID, "done")

	c, rec := ctxFor(orgID, http.MethodPut, itoa(int(tk.ID)), `{"status":"open"}`)
	if err := s.handleUpdateTaskStatus(c); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	wantTaskStatus(t, onlyFollowup(t, s, orgID, cr), "open")
}
