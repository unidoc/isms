package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Requires a migrated Postgres; skipped when ISMS_TEST_DATABASE_URL is unset
// (see notifications_roundtrip_test.go for how to stand one up).
//
// Covers the schema behind #422: tasks.change_request_id, its tenant-safe
// foreign key, the one-live-task-per-change index, and the finder the API uses
// to decide between reopening and creating.

// wantPgCode fails unless err is a Postgres error with the given SQLSTATE, so a
// test cannot pass on some unrelated constraint.
func wantPgCode(t *testing.T, err error, code string) {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != code {
		t.Fatalf("err = %v, want SQLSTATE %s", err, code)
	}
}

func newLinkedTask(t *testing.T, d *DB, orgID, changeID int, actor, status string) *Task {
	t.Helper()
	cr := changeID
	tk := &Task{
		Title: "Implement", TaskType: "change_followup", Assignee: actor, CreatedBy: actor,
		Status: status, Priority: "medium", ChangeRequestID: &cr,
	}
	if err := d.CreateTask(context.Background(), orgID, tk); err != nil {
		t.Fatalf("CreateTask(%s): %v", status, err)
	}
	return tk
}

func TestFindChangeFollowupTaskPrefersLiveThenCancelledThenDone(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, actor := newTestOrgUser(t, d, "change-followup-finder")
	cr := newTestChange(t, d, orgID, actor)

	if got, err := d.FindChangeFollowupTask(ctx, orgID, cr.ID); err != nil || got != nil {
		t.Fatalf("no linked task: got %v, err %v, want nil, nil", got, err)
	}

	// An unlinked task with the same type must never be returned.
	if err := d.CreateTask(ctx, orgID, &Task{Title: "unlinked", TaskType: "change_followup", Assignee: actor, CreatedBy: actor, Status: "open", Priority: "medium"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	oldDone := newLinkedTask(t, d, orgID, cr.ID, actor, "done")
	newDone := newLinkedTask(t, d, orgID, cr.ID, actor, "done")
	if got, _ := d.FindChangeFollowupTask(ctx, orgID, cr.ID); got == nil || got.ID != newDone.ID {
		t.Fatalf("only done tasks: got %+v, want the newest (%d, not %d)", got, newDone.ID, oldDone.ID)
	}

	cancelled := newLinkedTask(t, d, orgID, cr.ID, actor, "cancelled")
	if got, _ := d.FindChangeFollowupTask(ctx, orgID, cr.ID); got == nil || got.ID != cancelled.ID {
		t.Fatalf("done + cancelled: got %+v, want the cancelled one (%d)", got, cancelled.ID)
	}

	live := newLinkedTask(t, d, orgID, cr.ID, actor, "in_progress")
	got, _ := d.FindChangeFollowupTask(ctx, orgID, cr.ID)
	if got == nil || got.ID != live.ID {
		t.Fatalf("live + cancelled + done: got %+v, want the live one (%d)", got, live.ID)
	}
	if got.Status != "in_progress" || got.Assignee != actor || got.Title != "Implement" {
		t.Errorf("finder returned wrong columns: %+v", got)
	}

	// A soft-deleted task is not a task any more.
	if err := d.DeleteTask(ctx, orgID, live.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if got, _ := d.FindChangeFollowupTask(ctx, orgID, cr.ID); got == nil || got.ID != cancelled.ID {
		t.Fatalf("live deleted: got %+v, want the cancelled one (%d)", got, cancelled.ID)
	}
}

func TestChangeFollowupLiveTaskIsUniquePerChange(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, actor := newTestOrgUser(t, d, "change-followup-unique")
	cr := newTestChange(t, d, orgID, actor)
	other := newTestChange(t, d, orgID, actor)

	live := newLinkedTask(t, d, orgID, cr.ID, actor, "open")

	cid := cr.ID
	second := &Task{Title: "dup", TaskType: "change_followup", Assignee: actor, CreatedBy: actor, Status: "in_progress", Priority: "medium", ChangeRequestID: &cid}
	wantPgCode(t, d.CreateTask(ctx, orgID, second), "23505")

	// History beside a live task, and a live task on another change, are fine.
	newLinkedTask(t, d, orgID, cr.ID, actor, "cancelled")
	newLinkedTask(t, d, orgID, cr.ID, actor, "done")
	newLinkedTask(t, d, orgID, other.ID, actor, "open")

	// Once the live one is soft-deleted, a new live one is allowed again.
	if err := d.DeleteTask(ctx, orgID, live.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	newLinkedTask(t, d, orgID, cr.ID, actor, "open")
}

func TestChangeFollowupTaskRefusesAnotherOrgsChange(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgA, actorA := newTestOrgUser(t, d, "change-followup-fk-a")
	orgB, actorB := newTestOrgUser(t, d, "change-followup-fk-b")
	crB := newTestChange(t, d, orgB, actorB)

	cid := crB.ID
	tk := &Task{Title: "cross-org", TaskType: "change_followup", Assignee: actorA, CreatedBy: actorA, Status: "open", Priority: "medium", ChangeRequestID: &cid}
	wantPgCode(t, d.CreateTask(ctx, orgA, tk), "23503")
}
