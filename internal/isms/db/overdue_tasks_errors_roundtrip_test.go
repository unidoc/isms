package db

import (
	"context"
	"strings"
	"testing"
)

func orgTaskCount(t *testing.T, d *DB, orgID int) int {
	t.Helper()
	var n int
	if err := d.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM tasks WHERE organization_id = $1`, orgID).Scan(&n); err != nil {
		t.Fatalf("counting tasks: %v", err)
	}
	return n
}

// A failing "list open tasks" query must stop the run before anything is
// created. Continuing with an empty dedup map would create a duplicate of every
// task that already exists.
func TestCreateOverdueReviewTasksStopsWhenOpenTasksCannotBeListed(t *testing.T) {
	f := newOverdueTasksFixture(t, "ovt-listerr")
	ctx := context.Background()
	first, err := f.d.CreateOverdueReviewTasks(ctx, f.orgID, f.owner, f.fallback)
	if err != nil || len(first.Created) == 0 {
		t.Fatalf("seeding open tasks: created %d, err %v", len(first.Created), err)
	}
	before := orgTaskCount(t, f.d, f.orgID)

	// The view has no completed_at: ListTasksWhere selects it and fails, while the
	// overdue summary's tasks query and the task INSERT never touch it.
	shadow := shadowTableDB(t, "tasks", `s.id, s.organization_id, s.identifier, s.title, s.description, s.task_type,
		s.assignee_id, s.created_by, s.created_by_user_id, s.status, s.priority, s.due_date, s.recurrence_days,
		s.notes, s.created_at, s.updated_at, s.deleted_at, s.private, s.change_request_id`)

	res, err := shadow.CreateOverdueReviewTasks(ctx, f.orgID, f.owner, f.fallback)
	if err == nil {
		t.Errorf("want an error when the open tasks cannot be listed, got result created=%d skipped=%d failed=%d",
			len(res.Created), res.Skipped, res.Failed)
	} else if !strings.Contains(err.Error(), "overdue") {
		t.Errorf("error should be in the overdue family, got %q", err)
	}
	if after := orgTaskCount(t, f.d, f.orgID); after != before {
		t.Errorf("tasks in the org went from %d to %d: duplicates were created", before, after)
	}
}

func TestCreateOverdueReviewTasksStopsWhenDocumentReviewsFail(t *testing.T) {
	f := newOverdueTasksFixture(t, "ovt-docerr")
	// No review_cycle_months: OverdueDocumentReviews fails, nothing else does.
	shadow := shadowTableDB(t, "document_versions",
		`s.organization_id, s.document_id, s.version, s.created_by, s.created_at, s.owner`)

	res, err := shadow.CreateOverdueReviewTasks(context.Background(), f.orgID, f.owner, f.fallback)
	if err == nil {
		t.Errorf("want an error when document reviews cannot be read, got created=%d", len(res.Created))
	}
	if n := orgTaskCount(t, f.d, f.orgID); n != 0 {
		t.Errorf("want no tasks created, got %d", n)
	}
}

func TestCreateOverdueReviewTasksStopsWhenObjectiveCheckinsFail(t *testing.T) {
	f := newOverdueTasksFixture(t, "ovt-objerr")
	// No checkin_cycle: OverdueObjectiveCheckins fails, nothing else does.
	shadow := shadowTableDB(t, "objectives",
		`s.id, s.display_id, s.title, s.owner_id, s.started_at, s.created_at, s.organization_id, s.status, s.deleted_at`)

	res, err := shadow.CreateOverdueReviewTasks(context.Background(), f.orgID, f.owner, f.fallback)
	if err == nil {
		t.Errorf("want an error when objective check-ins cannot be read, got created=%d", len(res.Created))
	}
	if n := orgTaskCount(t, f.d, f.orgID); n != 0 {
		t.Errorf("want no tasks created, got %d", n)
	}
}

// Scan and iteration failures inside the two readers must surface too, not
// just query failures.
func TestOverdueReadersReturnScanAndIterationErrors(t *testing.T) {
	f := newOverdueTasksFixture(t, "ovt-readers")
	ctx := context.Background()
	docView := func(version, owner string) *DB {
		return shadowTableDB(t, "document_versions", `s.organization_id, s.document_id, `+version+` AS version, s.created_by, s.created_at, `+owner+` AS owner, s.review_cycle_months`)
	}
	if _, err := docView(`ARRAY[1]`, `s.owner`).OverdueDocumentReviews(ctx, f.orgID); err == nil {
		t.Error("OverdueDocumentReviews: want an error on a Scan failure")
	}
	if _, err := docView(`s.version`, `(1 / (s.review_cycle_months - s.review_cycle_months))::text`).OverdueDocumentReviews(ctx, f.orgID); err == nil {
		t.Error("OverdueDocumentReviews: want an error on an iteration failure")
	}
	objView := func(title string) *DB {
		return shadowTableDB(t, "objectives", `s.id, s.display_id, `+title+` AS title, s.owner_id, s.checkin_cycle, s.started_at, s.created_at, s.organization_id, s.status, s.deleted_at`)
	}
	if _, err := objView(`ARRAY[1]`).OverdueObjectiveCheckins(ctx, f.orgID); err == nil {
		t.Error("OverdueObjectiveCheckins: want an error on a Scan failure")
	}
	if _, err := objView(`(1 / (s.id - s.id))::text`).OverdueObjectiveCheckins(ctx, f.orgID); err == nil {
		t.Error("OverdueObjectiveCheckins: want an error on an iteration failure")
	}
}
