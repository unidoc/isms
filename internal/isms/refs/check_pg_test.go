package refs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/jackc/pgx/v5"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

// These tests run Check against a real database and real bare repos. The
// per-type matrix of the rule is covered through the API; here the point is the
// report, the counts, --fix, and that a lookup that failed is never treated as
// a broken reference.
//
// Check is only run with OrgSlug set: the database is shared with other test
// packages, and a fix across every org would delete their deliberately broken
// rows.

type checkOrg struct {
	ID   int
	Slug string
}

func newCheckOrg(t *testing.T, d *db.DB, name string) checkOrg {
	t.Helper()
	slug := fmt.Sprintf("%s-%d", name, os.Getpid())
	org := &db.Organization{Name: slug, Slug: slug, RepoPath: "/tmp/none-" + slug}
	if err := d.CreateOrganization(context.Background(), org); err != nil {
		t.Fatalf("creating org %s: %v", name, err)
	}
	t.Cleanup(func() { _ = d.DeleteOrganization(context.Background(), org.ID) })
	return checkOrg{ID: org.ID, Slug: slug}
}

type seeder struct {
	t   *testing.T
	d   *db.DB
	org checkOrg
}

func (s seeder) risk(title string) *db.Risk {
	s.t.Helper()
	r := &db.Risk{Title: title, RiskType: db.RiskTypes[0], Origin: db.RiskOrigins[0], Status: db.RiskStatuses[0]}
	if err := s.d.CreateRisk(context.Background(), s.org.ID, r); err != nil {
		s.t.Fatalf("CreateRisk: %v", err)
	}
	return r
}

func (s seeder) legal(title string) *db.LegalRequirement {
	s.t.Helper()
	l := &db.LegalRequirement{Title: title, Jurisdiction: "EU", Category: "privacy", Status: "open"}
	if err := s.d.CreateLegalRequirement(context.Background(), s.org.ID, l); err != nil {
		s.t.Fatalf("CreateLegalRequirement: %v", err)
	}
	return l
}

func (s seeder) incident(title string) *db.Incident {
	s.t.Helper()
	i := &db.Incident{Title: title, Description: "x", Severity: "low", Status: "open", IncidentType: "event",
		DetectedAt: db.Epoch{Time: timeNow()}, AuthorityNotified: "not_required", SubjectsNotified: "not_required",
		Source: "internal", Reporter: "reporter@check.test"}
	if err := s.d.CreateIncident(context.Background(), s.org.ID, i); err != nil {
		s.t.Fatalf("CreateIncident: %v", err)
	}
	return i
}

// user makes the org's one member, whom tasks and change requests need.
func (s seeder) user() string {
	s.t.Helper()
	email := fmt.Sprintf("member-%d-%d@check.test", s.org.ID, os.Getpid())
	u := &db.User{Email: email, Name: email, Active: true}
	if err := s.d.UpsertUser(context.Background(), u); err != nil {
		s.t.Fatalf("UpsertUser: %v", err)
	}
	if err := s.d.AddOrgMember(context.Background(), s.org.ID, u.ID, "contributor"); err != nil {
		s.t.Fatalf("AddOrgMember: %v", err)
	}
	return email
}

func (s seeder) task(title string) *db.Task {
	s.t.Helper()
	email := s.user()
	tk := &db.Task{Title: title, TaskType: "general", Assignee: email, CreatedBy: email, Status: "open", Priority: "medium"}
	if err := s.d.CreateTask(context.Background(), s.org.ID, tk); err != nil {
		s.t.Fatalf("CreateTask: %v", err)
	}
	return tk
}

func (s seeder) change(title string) *db.ChangeRequest {
	s.t.Helper()
	cr := &db.ChangeRequest{Title: title, Description: "d", Justification: "j", Priority: "medium",
		Category: "other", RiskLevel: "low", RollbackPlan: "revert", RequestedBy: s.user(), Status: "proposed"}
	if err := s.d.CreateChangeRequest(context.Background(), s.org.ID, cr); err != nil {
		s.t.Fatalf("CreateChangeRequest: %v", err)
	}
	return cr
}

func (s seeder) program(key string) *db.Program {
	s.t.Helper()
	p := &db.Program{Key: key, Title: "program " + key}
	if err := s.d.CreateProgram(context.Background(), s.org.ID, p); err != nil {
		s.t.Fatalf("CreateProgram: %v", err)
	}
	return p
}

func (s seeder) objective(programID int64) *db.Objective {
	s.t.Helper()
	o := &db.Objective{ProgramID: programID, Title: "objective"}
	if err := s.d.CreateObjective(context.Background(), s.org.ID, o); err != nil {
		s.t.Fatalf("CreateObjective: %v", err)
	}
	return o
}

func (s seeder) audit() *db.Audit {
	s.t.Helper()
	a := &db.Audit{Title: "audit", AuditType: "internal", Status: "planned"}
	if err := s.d.CreateAudit(context.Background(), s.org.ID, a); err != nil {
		s.t.Fatalf("CreateAudit: %v", err)
	}
	return a
}

// link writes the reference rows without any validation, as old data was
// written. both=false leaves only the a -> b direction.
func (s seeder) link(aType, aID, bType, bID string, both bool) {
	s.t.Helper()
	ctx := context.Background()
	r := &db.EntityReference{SourceType: aType, SourceID: aID, TargetType: bType, TargetID: bID}
	if err := s.d.CreateReference(ctx, s.org.ID, r); err != nil {
		s.t.Fatalf("CreateReference: %v", err)
	}
	if both {
		r2 := &db.EntityReference{SourceType: bType, SourceID: bID, TargetType: aType, TargetID: aID}
		if err := s.d.CreateReference(ctx, s.org.ID, r2); err != nil {
			s.t.Fatalf("CreateReference: %v", err)
		}
	}
}

func bareRepo(t *testing.T) *store.Store {
	t.Helper()
	repoPath := filepath.Join(t.TempDir(), "org.git")
	if _, err := git.PlainInit(repoPath, true); err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	st, err := store.NewBare(repoPath)
	if err != nil {
		t.Fatalf("NewBare: %v", err)
	}
	return st
}

func repoWithDoc(t *testing.T, docID string) *store.Store {
	t.Helper()
	st := bareRepo(t)
	content := "---\ndocument_id: " + docID + "\ntitle: A document\nstatus: draft\n---\nbody\n"
	abs := filepath.Join(st.Root(), "documents", "test", docID+".md")
	if _, err := st.CommitFile(abs, []byte(content), "Test User", "test@example.com", "add "+docID); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	return st
}

// rowSet is the org's whole entity_references table as sorted strings.
func rowSet(t *testing.T, d *db.DB, orgID int) []string {
	t.Helper()
	var out []string
	err := d.WithOrgTx(context.Background(), orgID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := db.ListOrgReferencesTx(ctx, tx, orgID)
		for _, r := range rows {
			out = append(out, fmt.Sprintf("%d %s %s -> %s %s", r.ID, r.SourceType, r.SourceID, r.TargetType, r.TargetID))
		}
		return err
	})
	if err != nil {
		t.Fatalf("reading references: %v", err)
	}
	sort.Strings(out)
	return out
}

type reportLine struct{ Org, Source, Target, Reason string }

var gap = regexp.MustCompile(` {2,}`)

// parse splits a Check report into its table rows and its summary lines.
func parse(out string) (rows []reportLine, summaries []string) {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		switch {
		case strings.HasPrefix(line, "ORG "):
		case strings.HasPrefix(line, "deleted "):
			summaries = append(summaries, line)
		case strings.Contains(line, " broken pair") || strings.Contains(line, " 0 broken"):
			summaries = append(summaries, line)
		default:
			f := gap.Split(strings.TrimSpace(line), -1)
			if len(f) == 4 {
				rows = append(rows, reportLine{f[0], f[1], f[2], f[3]})
			}
		}
	}
	return rows, summaries
}

func sortLines(l []reportLine) {
	sort.Slice(l, func(i, j int) bool {
		return fmt.Sprint(l[i]) < fmt.Sprint(l[j])
	})
}

func TestCheck(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	// B first: its per-org sequences and primary keys then differ from A's, so
	// a row id and an identifier cannot be confused by luck.
	orgB := newCheckOrg(t, d, "chk-b")
	orgA := newCheckOrg(t, d, "chk-a")
	orgC := newCheckOrg(t, d, "chk-c")
	b, a, c := seeder{t, d, orgB}, seeder{t, d, orgA}, seeder{t, d, orgC}

	b.legal("b legal 1")
	b.legal("b legal 2")
	bRisk, bLegal := b.risk("b risk"), b.legal("b legal")
	b.link("risk", bRisk.Identifier, "legal_requirement", bLegal.Identifier, true) // valid
	b.link("risk", bRisk.Identifier, "document", "anything", true)                 // empty repo: not found

	aRisk, aLegal := a.risk("a risk"), a.legal("a legal")
	aInc, aTask := a.incident("a incident"), a.task("a task")
	aCR := a.change("a change")
	aProg := a.program("PA")
	aObj := a.objective(aProg.ID)
	aAudit := a.audit()
	aDeleted := a.risk("a risk to delete")

	// Valid.
	a.link("risk", aRisk.Identifier, "legal_requirement", aLegal.Identifier, true)
	a.link("incident", aInc.Identifier, "task", aTask.Identifier, true)
	a.link("change_request", aCR.Identifier, "document", "doc-1", true)
	a.link("risk", aRisk.Identifier, "program", "PA", true)
	a.link("incident", aInc.Identifier, "program", strconv.FormatInt(aProg.ID, 10), true)
	a.link("risk", aRisk.Identifier, "objective", aObj.DisplayID, true)
	a.link("incident", aInc.Identifier, "objective", strconv.FormatInt(aObj.ID, 10), true)
	a.link("task", aTask.Identifier, "audit", fmt.Sprintf("AUDIT-%d", aAudit.ID), true)
	a.link("task", aTask.Identifier, "audit", strconv.Itoa(aAudit.ID), true)
	// Broken.
	a.link("incident", aInc.Identifier, "legal_requirement", strconv.FormatInt(aLegal.ID, 10), true)
	a.link("incident", aInc.Identifier, "legal_requirement", "LEGAL-99999", true)
	a.link("incident", aInc.Identifier, "risk", aDeleted.Identifier, true)
	a.link("task", aTask.Identifier, "asset", "ASSET-999", false)
	a.link("risk", aRisk.Identifier, "document", "no-such-doc", true)
	if err := d.DeleteRisk(ctx, orgA.ID, aDeleted.ID); err != nil {
		t.Fatalf("DeleteRisk: %v", err)
	}

	cRisk := c.risk("c risk")
	c.link("risk", cRisk.Identifier, "document", "doc-c", true)

	repoErr := errors.New("opening bare repo /nowhere: repository does not exist")
	open := func(org db.Organization) (*store.Store, error) {
		switch org.ID {
		case orgA.ID:
			return repoWithDoc(t, "doc-1"), nil
		case orgB.ID:
			return bareRepo(t), nil
		case orgC.ID:
			return nil, repoErr
		}
		return nil, errors.New("not a test org")
	}
	run := func(slug string, fix bool) (CheckResult, []reportLine, []string, string) {
		t.Helper()
		var buf bytes.Buffer
		res, err := Check(ctx, d, &buf, CheckOptions{OrgSlug: slug, Fix: fix, OpenStore: open})
		if err != nil {
			t.Fatalf("Check(%s, fix=%v): %v", slug, fix, err)
		}
		rows, sums := parse(buf.String())
		return res, rows, sums, buf.String()
	}

	wantA := []reportLine{
		{orgA.Slug, "incident " + aInc.Identifier, "legal_requirement " + strconv.FormatInt(aLegal.ID, 10), "bare number, not an identifier"},
		{orgA.Slug, "incident " + aInc.Identifier, "legal_requirement LEGAL-99999", "not found"},
		{orgA.Slug, "incident " + aInc.Identifier, "risk " + aDeleted.Identifier, "record deleted"},
		{orgA.Slug, "task " + aTask.Identifier, "asset ASSET-999", "not found"},
		{orgA.Slug, "risk " + aRisk.Identifier, "document no-such-doc", "not found"},
	}

	beforeA, beforeB, beforeC := rowSet(t, d, orgA.ID), rowSet(t, d, orgB.ID), rowSet(t, d, orgC.ID)

	t.Run("report lists exactly the broken pairs and changes nothing", func(t *testing.T) {
		res, rows, sums, out := run(orgA.Slug, false)
		sortLines(rows)
		want := append([]reportLine(nil), wantA...)
		sortLines(want)
		if fmt.Sprint(rows) != fmt.Sprint(want) {
			t.Errorf("report rows:\n got %v\nwant %v\nfull output:\n%s", rows, want, out)
		}
		if res.Broken != 5 || res.BrokenRows != 9 || res.Unchecked != 0 || res.Deleted != 0 {
			t.Errorf("result = %+v, want 5 broken (9 rows), 0 unchecked, 0 deleted", res)
		}
		wantSums := []string{orgA.Slug + ": 5 broken pairs (9 rows)", "total: 5 broken pairs (9 rows)"}
		if fmt.Sprint(sums) != fmt.Sprint(wantSums) {
			t.Errorf("summaries = %q, want %q", sums, wantSums)
		}
		if got := rowSet(t, d, orgA.ID); fmt.Sprint(got) != fmt.Sprint(beforeA) {
			t.Errorf("a read-only check changed the rows:\nbefore %v\nafter  %v", beforeA, got)
		}
	})

	t.Run("org B: an empty repo means the document is not there", func(t *testing.T) {
		res, rows, sums, out := run(orgB.Slug, false)
		want := []reportLine{{orgB.Slug, "risk " + bRisk.Identifier, "document anything", "not found"}}
		if fmt.Sprint(rows) != fmt.Sprint(want) {
			t.Errorf("rows = %v, want %v\n%s", rows, want, out)
		}
		if res.Broken != 1 || res.BrokenRows != 2 {
			t.Errorf("result = %+v", res)
		}
		if len(sums) != 2 || sums[1] != "total: 1 broken pair (2 rows)" {
			t.Errorf("summaries = %q", sums)
		}
	})

	t.Run("org C: a repo that will not open is unchecked, not broken", func(t *testing.T) {
		res, rows, sums, out := run(orgC.Slug, false)
		if len(rows) != 1 || rows[0].Source != "risk "+cRisk.Identifier || rows[0].Target != "document doc-c" ||
			!strings.HasPrefix(rows[0].Reason, "unchecked: ") || !strings.Contains(rows[0].Reason, "repository does not exist") {
			t.Errorf("rows = %v\n%s", rows, out)
		}
		if res.Broken != 0 || res.Unchecked != 1 {
			t.Errorf("result = %+v, want 0 broken, 1 unchecked", res)
		}
		if len(sums) != 2 || sums[0] != orgC.Slug+": 0 broken pairs (0 rows), 1 unchecked" || sums[1] != "total: 0 broken pairs (0 rows), 1 unchecked" {
			t.Errorf("summaries = %q", sums)
		}
	})

	t.Run("all orgs: the report covers every org that has something to report", func(t *testing.T) {
		var buf bytes.Buffer
		if _, err := Check(ctx, d, &buf, CheckOptions{OpenStore: open}); err != nil {
			t.Fatal(err)
		}
		rows, _ := parse(buf.String())
		got := map[string]int{}
		for _, r := range rows {
			got[r.Org]++
		}
		if got[orgA.Slug] != 5 || got[orgB.Slug] != 1 || got[orgC.Slug] != 1 {
			t.Errorf("rows per org = %v, want A 5, B 1, C 1", got)
		}
	})

	t.Run("unknown slug is an error", func(t *testing.T) {
		_, err := Check(ctx, d, &bytes.Buffer{}, CheckOptions{OrgSlug: "no-such-org-slug", OpenStore: open})
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("err = %v, want an organization not found error", err)
		}
	})

	t.Run("fix deletes exactly the broken rows, and a second check is clean", func(t *testing.T) {
		res, _, sums, out := run(orgA.Slug, true)
		if res.Broken != 5 || res.Deleted != 5 || res.DeletedRows != 9 {
			t.Errorf("result = %+v, want 5 broken, 5 deleted (9 rows)", res)
		}
		for _, w := range []string{orgA.Slug + ": deleted 5 broken pairs (9 rows)", "total: deleted 5 broken pairs (9 rows)"} {
			if !strings.Contains(out, w) {
				t.Errorf("output lacks %q:\n%s", w, out)
			}
		}
		if !strings.Contains(out, "deleted "+orgA.Slug+" task "+aTask.Identifier+" -> asset ASSET-999 (1 row)") {
			t.Errorf("no per-pair deletion line for the one-direction row:\n%s", out)
		}
		_ = sums

		// Compare by full row set: only the broken pairs' rows are gone.
		afterA := rowSet(t, d, orgA.ID)
		gone := map[string]bool{}
		for _, row := range beforeA {
			gone[row] = true
		}
		for _, row := range afterA {
			delete(gone, row)
		}
		if len(gone) != 9 {
			t.Errorf("%d rows removed, want 9: %v", len(gone), gone)
		}
		for row := range gone {
			if strings.Contains(row, "legal_requirement "+aLegal.Identifier) || strings.Contains(row, "program") ||
				strings.Contains(row, "objective") || strings.Contains(row, "audit") || strings.Contains(row, "document doc-1") {
				t.Errorf("a valid row was deleted: %s", row)
			}
		}
		if len(afterA) != len(beforeA)-9 {
			t.Errorf("row count %d -> %d, want -9", len(beforeA), len(afterA))
		}

		res2, rows2, _, out2 := run(orgA.Slug, false)
		if res2.Broken != 0 || res2.Unchecked != 0 || len(rows2) != 0 || !strings.Contains(out2, "0 broken") {
			t.Errorf("second check not clean: %+v\n%s", res2, out2)
		}
	})

	t.Run("fix leaves an unchecked pair alone and still reports it", func(t *testing.T) {
		res, rows, _, out := run(orgC.Slug, true)
		if res.Deleted != 0 || res.Unchecked != 1 {
			t.Errorf("result = %+v, want nothing deleted, 1 unchecked", res)
		}
		if len(rows) != 1 || !strings.HasPrefix(rows[0].Reason, "unchecked: ") {
			t.Errorf("rows = %v\n%s", rows, out)
		}
		if got := rowSet(t, d, orgC.ID); fmt.Sprint(got) != fmt.Sprint(beforeC) {
			t.Errorf("--fix touched the unchecked org:\nbefore %v\nafter  %v", beforeC, got)
		}
	})

	t.Run("fix on B deletes B's broken pair only", func(t *testing.T) {
		res, _, _, out := run(orgB.Slug, true)
		if res.Deleted != 1 || res.DeletedRows != 2 {
			t.Errorf("result = %+v", res)
		}
		for _, w := range []string{orgB.Slug + ": deleted 1 broken pair (2 rows)", "total: deleted 1 broken pair (2 rows)"} {
			if !strings.Contains(out, w) {
				t.Errorf("output lacks %q:\n%s", w, out)
			}
		}
		if got := rowSet(t, d, orgB.ID); len(got) != len(beforeB)-2 {
			t.Errorf("B rows %d -> %d, want -2", len(beforeB), len(got))
		}
	})
}

func timeNow() time.Time { return time.Now() }
