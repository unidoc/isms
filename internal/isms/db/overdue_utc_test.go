package db

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Requires a migrated Postgres (ISMS_TEST_DATABASE_URL); skipped otherwise.
//
// #503: one overdue rule everywhere. An item is overdue only after its due day
// has ended, counted on the UTC day: due yesterday (UTC) is overdue, due today
// (UTC) is not. The host zone must not move the cutoff.

// hostOffsetOffUTCDay picks a host zone whose calendar day is not the UTC day
// right now, so the test means something whenever it runs: UTC-12 differs while
// the UTC hour is below 12, UTC+14 from hour 10 on. No clock seam exists.
func hostOffsetOffUTCDay() int {
	if time.Now().UTC().Hour() < 12 {
		return -12 * 3600
	}
	return 14 * 3600
}

func utcDay(offsetDays int) string {
	return time.Now().UTC().AddDate(0, 0, offsetDays).Format("2006-01-02")
}

func hasID(items []OverdueItem, id string) bool {
	for _, it := range items {
		if it.EntityID == id {
			return true
		}
	}
	return false
}

func TestOverdueRuleCountsOnTheUTCDay(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, actor := newTestOrgUser(t, d, "overdue-utc")

	mkTask := func(title, day string) *Task {
		due, err := time.Parse("2006-01-02", day)
		if err != nil {
			t.Fatal(err)
		}
		e := NewEpoch(due)
		tk := &Task{Title: title, TaskType: "change_followup", Assignee: actor, CreatedBy: actor, Status: "open", Priority: "medium", DueDate: &e}
		if err := d.CreateTask(ctx, orgID, tk); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		return tk
	}
	mkRisk := func(title, day string) *Risk {
		r := &Risk{Title: title, RiskType: RiskTypes[0], Origin: RiskOrigins[0], Status: RiskStatuses[0]}
		if err := d.CreateRisk(ctx, orgID, r); err != nil {
			t.Fatalf("CreateRisk: %v", err)
		}
		if _, err := d.pool.Exec(ctx, `UPDATE risks SET next_review = $1::date WHERE id = $2`, day, r.ID); err != nil {
			t.Fatal(err)
		}
		return r
	}
	taskYesterday := mkTask("due yesterday", utcDay(-1))
	taskToday := mkTask("due today", utcDay(0))
	riskYesterday := mkRisk("review yesterday", utcDay(-1))
	riskToday := mkRisk("review today", utcDay(0))

	// A contract expiring tomorrow (UTC) is 1 day away.
	sup := newTestSupplier(t, d, orgID, "Contract Tomorrow")
	if _, err := d.pool.Exec(ctx, `UPDATE suppliers SET contract_expiry = $1::date WHERE id = $2`, utcDay(1), sup.ID); err != nil {
		t.Fatal(err)
	}

	prev := time.Local
	time.Local = time.FixedZone("host", hostOffsetOffUTCDay())
	t.Cleanup(func() { time.Local = prev })
	if time.Now().Format("2006-01-02") == time.Now().UTC().Format("2006-01-02") {
		t.Fatal("test setup: local and UTC day are equal")
	}

	sum, err := d.GetOverdueSummary(ctx, orgID, TaskViewer{CanSeeAll: true})
	if err != nil {
		t.Fatalf("GetOverdueSummary: %v", err)
	}
	if !hasID(sum.Tasks, fmt.Sprint(taskYesterday.ID)) {
		t.Errorf("task due yesterday (UTC) missing from overdue summary")
	}
	if hasID(sum.Tasks, fmt.Sprint(taskToday.ID)) {
		t.Errorf("task due today (UTC) is in the overdue summary; it is overdue only after the day ends")
	}
	if !hasID(sum.Risks, riskYesterday.Identifier) {
		t.Errorf("risk reviewed due yesterday (UTC) missing from overdue summary")
	}
	if hasID(sum.Risks, riskToday.Identifier) {
		t.Errorf("risk due today (UTC) is in the overdue summary")
	}
	for _, it := range sum.Risks {
		if it.EntityID == riskYesterday.Identifier && it.DaysLate != 1 {
			t.Errorf("risk due yesterday: days_late = %d, want 1", it.DaysLate)
		}
	}
	for _, it := range sum.Tasks {
		if it.EntityID == fmt.Sprint(taskYesterday.ID) && it.DaysLate != 1 {
			t.Errorf("task due yesterday: days_late = %d, want 1", it.DaysLate)
		}
	}
	found := false
	for _, it := range sum.SupplierContracts {
		if it.EntityID == sup.Identifier {
			found = true
			if it.DaysLate != -1 || it.State != "expiring" {
				t.Errorf("contract expiring tomorrow: days_late=%d state=%q, want -1 (days_until 1) and expiring", it.DaysLate, it.State)
			}
		}
	}
	if !found {
		t.Errorf("contract expiring tomorrow missing from summary")
	}

	tasks, err := d.OverdueTasks(ctx, orgID, TaskViewer{CanSeeAll: true})
	if err != nil {
		t.Fatalf("OverdueTasks: %v", err)
	}
	var gotY, gotT bool
	for _, tk := range tasks {
		gotY = gotY || tk.ID == taskYesterday.ID
		gotT = gotT || tk.ID == taskToday.ID
	}
	if !gotY || gotT {
		t.Errorf("OverdueTasks: yesterday listed=%v (want true), today listed=%v (want false)", gotY, gotT)
	}
}
