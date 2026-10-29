package db

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// overdueTasksFixture seeds one overdue item of every kind in a fresh org.
// Dates are set by Postgres (CURRENT_DATE +/- n), never from Go.
type overdueTasksFixture struct {
	d         *DB
	orgID     int
	owner     string // owns the risk, system, contract supplier and legal requirement
	fallback  string // a second real user, for ownerless items
	ownerless *Supplier
	contract  *Supplier
}

func newOverdueTasksFixture(t *testing.T, name string) *overdueTasksFixture {
	t.Helper()
	d := testDB(t)
	ctx := context.Background()
	fallback := fmt.Sprintf("%s-fallback@notifications-roundtrip.test", name)
	// Registered first, so it runs last: the org (and its tasks, which reference
	// this user) must be gone before the user can be.
	t.Cleanup(func() { _, _ = d.pool.Exec(ctx, `DELETE FROM users WHERE email = $1`, fallback) })
	orgID, owner := newTestOrgUser(t, d, name)
	// A second user, seeded here so the test does not lean on anything another
	// test left behind.
	if _, err := d.pool.Exec(ctx, `INSERT INTO users (email, name) VALUES ($1, $1)`, fallback); err != nil {
		t.Fatalf("creating fallback user: %v", err)
	}
	f := &overdueTasksFixture{d: d, orgID: orgID, owner: owner, fallback: fallback}
	own := func(table string, id int64) {
		if _, err := d.pool.Exec(ctx, fmt.Sprintf(`UPDATE %s SET owner_id = (SELECT id FROM users WHERE email = $1) WHERE id = $2`, table), owner, id); err != nil {
			t.Fatalf("owning %s %d: %v", table, id, err)
		}
	}
	late := func(table string, id int64, days int) {
		if _, err := d.pool.Exec(ctx, fmt.Sprintf(`UPDATE %s SET next_review = CURRENT_DATE - $1::int WHERE id = $2`, table), days, id); err != nil {
			t.Fatalf("setting next_review on %s: %v", table, err)
		}
	}

	risk := &Risk{Title: "Overdue risk", RiskType: RiskTypes[0], Origin: RiskOrigins[0], Status: RiskStatuses[0]}
	if err := d.CreateRisk(ctx, orgID, risk); err != nil {
		t.Fatalf("CreateRisk: %v", err)
	}
	own("risks", risk.ID)
	late("risks", risk.ID, 20)

	f.ownerless = newTestSupplier(t, d, orgID, "Ownerless Supplier")
	if _, err := d.pool.Exec(ctx, `UPDATE suppliers SET owner_id = NULL WHERE id = $1`, f.ownerless.ID); err != nil {
		t.Fatalf("clearing owner: %v", err)
	}
	late("suppliers", f.ownerless.ID, 10)

	sys := &System{Name: "Overdue System", Classification: "internal", Criticality: "high", Status: "active"}
	if err := d.CreateSystem(ctx, orgID, sys); err != nil {
		t.Fatalf("CreateSystem: %v", err)
	}
	own("systems", sys.ID)
	late("systems", sys.ID, 5)

	legal := &LegalRequirement{Title: "Overdue legal", Jurisdiction: "EU", Category: "privacy", Status: "open"}
	if err := d.CreateLegalRequirement(ctx, orgID, legal); err != nil {
		t.Fatalf("CreateLegalRequirement: %v", err)
	}
	own("legal_requirements", legal.ID)
	late("legal_requirements", legal.ID, 3)

	// An expiring contract on a supplier with an owner, whose own review is not due.
	f.contract = newTestSupplier(t, d, orgID, "Contract Supplier")
	own("suppliers", f.contract.ID)
	offsetDays(t, d, f.contract.ID, 10)
	// An evidence link on the same supplier that expires in 12 days.
	if _, err := d.pool.Exec(ctx, `
		INSERT INTO external_links (organization_id, entity_type, entity_id, url, label, link_type, valid_until, created_by)
		VALUES ($1, 'supplier', $2, 'https://trust.example/iso', 'ISO 27001 certificate', 'certificate', CURRENT_DATE + 12, $3)`,
		orgID, f.contract.ID, owner); err != nil {
		t.Fatalf("seeding external link: %v", err)
	}

	// A document past its 12-month review cycle (owner snapshot on the version).
	if _, err := d.pool.Exec(ctx, `
		INSERT INTO document_versions (organization_id, document_id, version, commit_hash, file_path, owner, review_cycle_months, created_by, created_at)
		VALUES ($1, 'overdue-doc-1', '1.0', 'abc', 'documents/x.md', $2, 12, $2, now() - interval '14 months')`, orgID, owner); err != nil {
		t.Fatalf("seeding document version: %v", err)
	}

	// An active objective whose check-in cycle (1 month) lapsed 3 months ago.
	prog := &Program{Key: "OT" + strings.ToUpper(name[len(name)-3:]), Title: "Overdue tasks program"}
	if err := d.CreateProgram(ctx, orgID, prog); err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	obj := &Objective{ProgramID: prog.ID, Title: "Overdue objective", Status: "active", CheckinCycle: 1, Owner: owner}
	if err := d.CreateObjective(ctx, orgID, obj); err != nil {
		t.Fatalf("CreateObjective: %v", err)
	}
	if _, err := d.pool.Exec(ctx, `UPDATE objectives SET started_at = now() - interval '3 months' WHERE id = $1`, obj.ID); err != nil {
		t.Fatalf("aging objective: %v", err)
	}
	return f
}

func (f *overdueTasksFixture) taskByPrefix(t *testing.T, res *CreatedReviewTasks, prefix string) Task {
	t.Helper()
	for _, task := range res.Created {
		if strings.HasPrefix(task.Title, prefix) {
			return task
		}
	}
	t.Fatalf("no created task starts with %q; created: %v", prefix, titles(res.Created))
	return Task{}
}

func titles(ts []Task) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Title)
	}
	return out
}

func TestCreateOverdueReviewTasksCreatesEveryKind(t *testing.T) {
	f := newOverdueTasksFixture(t, "ovtasks-all")
	ctx := context.Background()

	res, err := f.d.CreateOverdueReviewTasks(ctx, f.orgID, f.owner, f.fallback)
	if err != nil {
		t.Fatalf("CreateOverdueReviewTasks: %v", err)
	}
	const want = 8 // risk, supplier, system, legal, contract renewal, link renewal, document, objective
	if len(res.Created) != want || res.Total != want || res.Failed != 0 || res.Unassigned != 0 || res.Skipped != 0 {
		t.Fatalf("created %d total %d failed %d unassigned %d skipped %d, want %d/%d/0/0/0: %v",
			len(res.Created), res.Total, res.Failed, res.Unassigned, res.Skipped, want, want, titles(res.Created))
	}
	for _, task := range res.Created {
		if task.TaskType != "review" {
			t.Errorf("%q has type %q, want review", task.Title, task.TaskType)
		}
		if task.ID == 0 {
			t.Errorf("%q was not stored", task.Title)
		}
	}

	owned := map[string]string{
		"Risk review: Overdue risk": f.owner, "Access review: Overdue System": f.owner,
		"Legal review: Overdue legal": f.owner, "Contract renewal: Contract Supplier": f.owner,
		"Document review: ": f.owner, "Objective check-in: Overdue objective": f.owner,
		// The evidence link is renewed by its supplier's owner.
		"Renewal: ISO 27001 certificate": f.owner,
		// No owner: the fallback.
		"Supplier review: Ownerless Supplier": f.fallback,
	}
	for prefix, assignee := range owned {
		if got := f.taskByPrefix(t, res, prefix).Assignee; got != assignee {
			t.Errorf("%s: assignee %q, want %q", prefix, got, assignee)
		}
	}

	renewal := f.taskByPrefix(t, res, "Contract renewal: ")
	if want := "Contract renewal: Contract Supplier (" + f.contract.Identifier + ")"; renewal.Title != want {
		t.Errorf("renewal title %q, want %q", renewal.Title, want)
	}
	var sameDay bool
	if err := f.d.pool.QueryRow(ctx, `SELECT due_date::date = (SELECT contract_expiry FROM suppliers WHERE id = $2) FROM tasks WHERE id = $1`,
		renewal.ID, f.contract.ID).Scan(&sameDay); err != nil || !sameDay {
		t.Errorf("renewal due date is not the contract expiry (err %v)", err)
	}

	link := f.taskByPrefix(t, res, "Renewal: ")
	if want := "Renewal: ISO 27001 certificate - Contract Supplier (" + f.contract.Identifier + ")"; link.Title != want {
		t.Errorf("link renewal title %q, want %q", link.Title, want)
	}
	if err := f.d.pool.QueryRow(ctx, `SELECT due_date::date = CURRENT_DATE + 12 FROM tasks WHERE id = $1`, link.ID).Scan(&sameDay); err != nil || !sameDay {
		t.Errorf("link renewal due date is not the link's valid_until (err %v)", err)
	}

	// Second run: nothing new, everything skipped.
	again, err := f.d.CreateOverdueReviewTasks(ctx, f.orgID, f.owner, f.fallback)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(again.Created) != 0 || again.Skipped != want || again.Failed != 0 {
		t.Errorf("second run created %d skipped %d failed %d, want 0/%d/0", len(again.Created), again.Skipped, again.Failed, want)
	}
}

func TestCreateOverdueReviewTasksOwnerlessWithoutFallback(t *testing.T) {
	f := newOverdueTasksFixture(t, "ovtasks-nofb")
	res, err := f.d.CreateOverdueReviewTasks(context.Background(), f.orgID, "system", "")
	if err != nil {
		t.Fatalf("CreateOverdueReviewTasks: %v", err)
	}
	if res.Unassigned != 1 || res.Failed != 0 || len(res.Created) != 7 {
		t.Errorf("unassigned %d failed %d created %d, want 1/0/7", res.Unassigned, res.Failed, len(res.Created))
	}
	for _, task := range res.Created {
		if strings.Contains(task.Title, "Ownerless Supplier") {
			t.Errorf("a task was created for the ownerless supplier: %q", task.Title)
		}
	}
}

func TestCreateOverdueReviewTasksCountsFailures(t *testing.T) {
	f := newOverdueTasksFixture(t, "ovtasks-fail")
	// The fallback is not a user, so the ownerless supplier's insert fails.
	res, err := f.d.CreateOverdueReviewTasks(context.Background(), f.orgID, f.owner, "nobody@not-a-user.test")
	if err != nil {
		t.Fatalf("CreateOverdueReviewTasks: %v", err)
	}
	if res.Failed != 1 || res.Unassigned != 0 || len(res.Created) != 7 {
		t.Errorf("failed %d unassigned %d created %d, want 1/0/7", res.Failed, res.Unassigned, len(res.Created))
	}
}

func TestOverdueSummaryReportsSupplierAndSystemOwners(t *testing.T) {
	f := newOverdueTasksFixture(t, "ovtasks-owner")
	sum, err := f.d.GetOverdueSummary(context.Background(), f.orgID, TaskViewer{CanSeeAll: true})
	if err != nil {
		t.Fatalf("GetOverdueSummary: %v", err)
	}
	if len(sum.SupplierContracts) != 1 || sum.SupplierContracts[0].Owner != f.owner {
		t.Errorf("contract owner: %+v, want %q", sum.SupplierContracts, f.owner)
	}
	if len(sum.ExternalLinks) != 1 || sum.ExternalLinks[0].Owner != f.owner {
		t.Errorf("external link owner: %+v, want %q", sum.ExternalLinks, f.owner)
	}
	if len(sum.Systems) != 1 || sum.Systems[0].Owner != f.owner {
		t.Errorf("system owner: %+v, want %q", sum.Systems, f.owner)
	}
	if len(sum.Suppliers) != 1 || sum.Suppliers[0].Owner != "" {
		t.Errorf("ownerless supplier owner: %+v, want empty", sum.Suppliers)
	}
}
