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

// canonLine is one row of the non-canonical table.
type canonLine struct{ Org, Stored, Canonical, Action string }

var gap = regexp.MustCompile(` {2,}`)

// split divides a Check report into the broken table, the non-canonical table
// and the summary lines of each. The two tables are told apart by their header.
func split(out string) (broken []reportLine, canon []canonLine, brokenSums, canonSums []string) {
	inCanon := false
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		switch {
		case strings.HasPrefix(line, "ORG ") && strings.Contains(line, "STORED"):
			inCanon = true
		case strings.HasPrefix(line, "ORG "):
			inCanon = false
		case strings.HasPrefix(line, "deleted ") || strings.Contains(line, " broken pair") || strings.Contains(line, " 0 broken"):
			brokenSums = append(brokenSums, line)
		case strings.HasPrefix(line, "rewrote ") || strings.HasPrefix(line, "merged ") ||
			strings.Contains(line, "non-canonical") || strings.Contains(line, "comment mention"):
			canonSums = append(canonSums, line)
		default:
			f := gap.Split(strings.TrimSpace(line), -1)
			if len(f) != 4 {
				continue
			}
			if inCanon {
				canon = append(canon, canonLine{f[0], f[1], f[2], f[3]})
			} else {
				broken = append(broken, reportLine{f[0], f[1], f[2], f[3]})
			}
		}
	}
	return
}

// parse returns the broken table and the broken-section summary lines.
func parse(out string) (rows []reportLine, summaries []string) {
	rows, _, summaries, _ = split(out)
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

	// Valid and canonical.
	a.link("risk", aRisk.Identifier, "legal_requirement", aLegal.Identifier, true)
	a.link("incident", aInc.Identifier, "task", aTask.Identifier, true)
	a.link("change_request", aCR.Identifier, "document", "doc-1", true)
	a.link("risk", aRisk.Identifier, "program", "PA", true)
	a.link("risk", aRisk.Identifier, "objective", aObj.DisplayID, true)
	a.link("task", aTask.Identifier, "audit", fmt.Sprintf("AUDIT-%d", aAudit.ID), true)
	// Resolve, but stored under the other spelling: two get rewritten, the audit
	// one is a duplicate of the canonical row above and is merged into it.
	a.link("incident", aInc.Identifier, "program", strconv.FormatInt(aProg.ID, 10), true)
	a.link("incident", aInc.Identifier, "objective", strconv.FormatInt(aObj.ID, 10), true)
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

	aProgID, aObjID := strconv.FormatInt(aProg.ID, 10), strconv.FormatInt(aObj.ID, 10)
	aAuditNum := strconv.Itoa(aAudit.ID)
	aLegalRowID := strconv.FormatInt(aLegal.ID, 10)
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
		_, canon, _, canonSums := split(out)
		wantCanon := []canonLine{
			{orgA.Slug, "incident " + aInc.Identifier + " -> program " + aProgID, "incident " + aInc.Identifier + " -> program PA", "rewrite"},
			{orgA.Slug, "incident " + aInc.Identifier + " -> objective " + aObjID, "incident " + aInc.Identifier + " -> objective " + aObj.DisplayID, "rewrite"},
			{orgA.Slug, "task " + aTask.Identifier + " -> audit " + aAuditNum, "task " + aTask.Identifier + " -> audit AUDIT-" + aAuditNum, "merge (duplicate)"},
		}
		if fmt.Sprint(canon) != fmt.Sprint(wantCanon) {
			t.Errorf("non-canonical rows:\n got %v\nwant %v\nfull output:\n%s", canon, wantCanon, out)
		}
		wantCanonSums := []string{orgA.Slug + ": 3 non-canonical pairs (6 rows), 1 duplicate", "total: 3 non-canonical pairs (6 rows), 1 duplicate"}
		if fmt.Sprint(canonSums) != fmt.Sprint(wantCanonSums) {
			t.Errorf("non-canonical summaries = %q, want %q", canonSums, wantCanonSums)
		}
		if res.NonCanonical != 3 || res.NonCanonicalRows != 6 || res.Duplicates != 1 || res.Fixed != 0 {
			t.Errorf("result = %+v, want 3 non-canonical (6 rows), 1 duplicate, 0 fixed", res)
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

	t.Run("fix deletes the broken rows, canonicalises the rest, and a second check is clean", func(t *testing.T) {
		rowsBefore := refRows(t, d, orgA.ID)
		res, _, _, out := run(orgA.Slug, true)
		if res.Broken != 5 || res.Deleted != 5 || res.DeletedRows != 9 {
			t.Errorf("result = %+v, want 5 broken, 5 deleted (9 rows)", res)
		}
		if res.Fixed != 3 || res.FixedRows != 6 || res.FixedComments != 0 {
			t.Errorf("result = %+v, want 3 canonical pairs fixed (6 rows)", res)
		}
		for _, w := range []string{
			orgA.Slug + ": deleted 5 broken pairs (9 rows)", "total: deleted 5 broken pairs (9 rows)",
			orgA.Slug + ": fixed 3 non-canonical pairs (6 rows)", "total: fixed 3 non-canonical pairs (6 rows)",
			"rewrote " + orgA.Slug + " incident " + aInc.Identifier + " -> program " + aProgID + " as incident " + aInc.Identifier + " -> program PA (2 rows)",
			"merged " + orgA.Slug + " task " + aTask.Identifier + " -> audit " + aAuditNum + " into task " + aTask.Identifier + " -> audit AUDIT-" + aAuditNum + " (2 rows)",
		} {
			if !strings.Contains(out, w) {
				t.Errorf("output lacks %q:\n%s", w, out)
			}
		}
		if !strings.Contains(out, "deleted "+orgA.Slug+" task "+aTask.Identifier+" -> asset ASSET-999 (1 row)") {
			t.Errorf("no per-pair deletion line for the one-direction row:\n%s", out)
		}

		// The broken pairs' 9 rows are gone, the audit duplicate left 2 rows
		// fewer, and nothing else was deleted.
		rowsAfter := refRows(t, d, orgA.ID)
		if want := len(rowsBefore) - 9 - 2; len(rowsAfter) != want {
			t.Errorf("row count %d -> %d, want %d", len(rowsBefore), len(rowsAfter), want)
		}
		after := map[int64]string{}
		for _, r := range rowsAfter {
			after[r.ID] = fmt.Sprintf("%s %s -> %s %s", r.SourceType, r.SourceID, r.TargetType, r.TargetID)
		}
		for _, r := range rowsBefore {
			broken := strings.Contains(fmt.Sprint(r), "LEGAL-99999") || strings.Contains(fmt.Sprint(r), "ASSET-999") ||
				strings.Contains(fmt.Sprint(r), "no-such-doc") || strings.Contains(fmt.Sprint(r), aDeleted.Identifier) ||
				(r.SourceType == "legal_requirement" && r.SourceID == aLegalRowID) || (r.TargetType == "legal_requirement" && r.TargetID == aLegalRowID)
			if _, ok := after[r.ID]; broken && ok {
				t.Errorf("broken row %d survived the fix", r.ID)
			}
		}
		// A rewritten row keeps its id and now reads canonically.
		for _, c := range []struct {
			aType, aID, bType, bID, want string
		}{
			{"incident", aInc.Identifier, "program", aProgID, "incident " + aInc.Identifier + " -> program PA"},
			{"program", aProgID, "incident", aInc.Identifier, "program PA -> incident " + aInc.Identifier},
			{"incident", aInc.Identifier, "objective", aObjID, "incident " + aInc.Identifier + " -> objective " + aObj.DisplayID},
			{"objective", aObjID, "incident", aInc.Identifier, "objective " + aObj.DisplayID + " -> incident " + aInc.Identifier},
		} {
			id := findRow(t, rowsBefore, c.aType, c.aID, c.bType, c.bID)
			if after[id] != c.want {
				t.Errorf("row %d reads %q, want %q", id, after[id], c.want)
			}
		}
		// The audit duplicate: the canonical rows stay, the bare ones are gone.
		for _, c := range [][4]string{
			{"task", aTask.Identifier, "audit", aAuditNum}, {"audit", aAuditNum, "task", aTask.Identifier},
		} {
			if id := findRow(t, rowsBefore, c[0], c[1], c[2], c[3]); after[id] != "" {
				t.Errorf("bare audit row %d survived: %q", id, after[id])
			}
		}
		var auditRows []string
		for _, v := range after {
			if strings.Contains(v, "audit ") {
				auditRows = append(auditRows, v)
			}
		}
		sort.Strings(auditRows)
		wantAudit := []string{
			"audit AUDIT-" + aAuditNum + " -> task " + aTask.Identifier,
			"task " + aTask.Identifier + " -> audit AUDIT-" + aAuditNum,
		}
		if fmt.Sprint(auditRows) != fmt.Sprint(wantAudit) {
			t.Errorf("audit rows = %v, want %v", auditRows, wantAudit)
		}

		res2, rows2, _, out2 := run(orgA.Slug, false)
		if res2.Broken != 0 || res2.Unchecked != 0 || res2.NonCanonical != 0 || res2.NonCanonicalComments != 0 || len(rows2) != 0 ||
			!strings.Contains(out2, "0 broken pairs (0 rows): every reference resolves") ||
			!strings.Contains(out2, "0 non-canonical pairs (0 rows): every reference uses its canonical id") {
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

// refRows is the org's whole entity_references table, ordered by row id.
func refRows(t *testing.T, d *db.DB, orgID int) []db.EntityReference {
	t.Helper()
	var rows []db.EntityReference
	err := d.WithOrgTx(context.Background(), orgID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = db.ListOrgReferencesTx(ctx, tx, orgID)
		return err
	})
	if err != nil {
		t.Fatalf("reading references: %v", err)
	}
	return rows
}

func findRow(t *testing.T, rows []db.EntityReference, st, sid, tt, tid string) int64 {
	t.Helper()
	for _, r := range rows {
		if r.SourceType == st && r.SourceID == sid && r.TargetType == tt && r.TargetID == tid {
			return r.ID
		}
	}
	t.Fatalf("no row %s %s -> %s %s", st, sid, tt, tid)
	return 0
}

func execSQL(t *testing.T, d *db.DB, orgID int, sql string, args ...any) {
	t.Helper()
	err := d.WithOrgTx(context.Background(), orgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// setOrigin sets the origin of both directions of a pair, as stored.
func (s seeder) setOrigin(aType, aID, bType, bID, origin string) {
	s.t.Helper()
	execSQL(s.t, s.d, s.org.ID, `
		UPDATE entity_references SET origin = $5
		WHERE organization_id = $6
		  AND ((source_type = $1 AND source_id = $2 AND target_type = $3 AND target_id = $4)
		    OR (source_type = $3 AND source_id = $4 AND target_type = $1 AND target_id = $2))`,
		aType, aID, bType, bID, origin, s.org.ID)
}

// commentMention inserts a comment_references row exactly as spelled.
func (s seeder) commentMention(commentType string, commentID int64, aType, aID, bType, bID string) {
	s.t.Helper()
	execSQL(s.t, s.d, s.org.ID, `
		INSERT INTO comment_references (organization_id, comment_type, comment_id, source_type, source_id, target_type, target_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, s.org.ID, commentType, commentID, aType, aID, bType, bID)
}

func commentMentions(t *testing.T, d *db.DB, orgID int, commentID int64) []string {
	t.Helper()
	var out []string
	err := d.WithOrgTx(context.Background(), orgID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT source_type, source_id, target_type, target_id FROM comment_references
			WHERE organization_id = $1 AND comment_id = $2 ORDER BY id`, orgID, commentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a, b, c, e string
			if err := rows.Scan(&a, &b, &c, &e); err != nil {
				return err
			}
			out = append(out, fmt.Sprintf("%s %s -> %s %s", a, b, c, e))
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("reading comment mentions: %v", err)
	}
	return out
}

func timeNow() time.Time { return time.Now() }

const (
	cleanBroken = "0 broken pairs (0 rows): every reference resolves"
	cleanCanon  = "0 non-canonical pairs (0 rows): every reference uses its canonical id"
)

// TestCheckCanonical covers links that resolve but are stored under the other
// spelling of the same record: report, rewrite, merge into an existing
// canonical row, and the origin and comment_references that go with them.
func TestCheckCanonical(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	open := func(db.Organization) (*store.Store, error) { return bareRepo(t), nil }
	run := func(org checkOrg, fix bool) (CheckResult, string) {
		t.Helper()
		var buf bytes.Buffer
		res, err := Check(ctx, d, &buf, CheckOptions{OrgSlug: org.Slug, Fix: fix, OpenStore: open})
		if err != nil {
			t.Fatalf("Check(%s, fix=%v): %v", org.Slug, fix, err)
		}
		return res, buf.String()
	}
	has := func(t *testing.T, out string, want ...string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(out, w) {
				t.Errorf("output lacks %q:\n%s", w, out)
			}
		}
	}
	str := func(n int64) string { return strconv.FormatInt(n, 10) }

	t.Run("report lists rewrite and merge and changes nothing", func(t *testing.T) {
		org := newCheckOrg(t, d, "can-report")
		s := seeder{t, d, org}
		prog := s.program("PA")
		inc, risk, audit := s.incident("i"), s.risk("r"), s.audit()
		s.link("incident", inc.Identifier, "program", str(prog.ID), true)
		s.link("risk", risk.Identifier, "audit", strconv.Itoa(audit.ID), true)
		s.link("risk", risk.Identifier, "audit", fmt.Sprintf("AUDIT-%d", audit.ID), true)
		before := rowSet(t, d, org.ID)

		res, out := run(org, false)
		_, canon, brokenSums, canonSums := split(out)
		want := []canonLine{
			{org.Slug, "incident " + inc.Identifier + " -> program " + str(prog.ID), "incident " + inc.Identifier + " -> program PA", "rewrite"},
			{org.Slug, fmt.Sprintf("risk %s -> audit %d", risk.Identifier, audit.ID), fmt.Sprintf("risk %s -> audit AUDIT-%d", risk.Identifier, audit.ID), "merge (duplicate)"},
		}
		if fmt.Sprint(canon) != fmt.Sprint(want) {
			t.Errorf("rows:\n got %v\nwant %v\n%s", canon, want, out)
		}
		wantSums := []string{org.Slug + ": 2 non-canonical pairs (4 rows), 1 duplicate", "total: 2 non-canonical pairs (4 rows), 1 duplicate"}
		if fmt.Sprint(canonSums) != fmt.Sprint(wantSums) {
			t.Errorf("summaries = %q, want %q", canonSums, wantSums)
		}
		if fmt.Sprint(brokenSums) != fmt.Sprint([]string{cleanBroken}) {
			t.Errorf("broken section = %q, want the clean line only", brokenSums)
		}
		if hdr := regexp.MustCompile(`ORG +STORED`).FindStringIndex(out); hdr == nil || strings.Index(out, cleanBroken) > hdr[0] {
			t.Errorf("canonical section must come after the broken section:\n%s", out)
		}
		if res.NonCanonical != 2 || res.NonCanonicalRows != 4 || res.Duplicates != 1 || res.Broken != 0 || res.Fixed != 0 {
			t.Errorf("result = %+v", res)
		}
		if got := rowSet(t, d, org.ID); fmt.Sprint(got) != fmt.Sprint(before) {
			t.Errorf("a read-only check changed the rows:\nbefore %v\nafter  %v", before, got)
		}
	})

	t.Run("fix rewrites in place, merges, and handles a one-direction twin", func(t *testing.T) {
		org := newCheckOrg(t, d, "can-fix")
		s := seeder{t, d, org}
		prog := s.program("PA")
		obj := s.objective(prog.ID)
		inc, risk, audit, task := s.incident("i"), s.risk("r"), s.audit(), s.task("t")
		s.link("incident", inc.Identifier, "program", str(prog.ID), true) // rewrite
		s.link("risk", risk.Identifier, "audit", strconv.Itoa(audit.ID), true)
		s.link("risk", risk.Identifier, "audit", fmt.Sprintf("AUDIT-%d", audit.ID), true) // merge, both directions
		s.link("task", task.Identifier, "objective", str(obj.ID), true)
		s.link("task", task.Identifier, "objective", obj.DisplayID, false) // canonical twin in one direction only
		before := refRows(t, d, org.ID)

		res, out := run(org, true)
		if res.NonCanonical != 3 || res.NonCanonicalRows != 6 || res.Duplicates != 2 || res.Fixed != 3 || res.FixedRows != 6 {
			t.Errorf("result = %+v, want 3 pairs (6 rows), 2 duplicates, all fixed", res)
		}
		has(t, out,
			"rewrote "+org.Slug+" incident "+inc.Identifier+" -> program "+str(prog.ID)+" as incident "+inc.Identifier+" -> program PA (2 rows)",
			fmt.Sprintf("merged %s risk %s -> audit %d into risk %s -> audit AUDIT-%d (2 rows)", org.Slug, risk.Identifier, audit.ID, risk.Identifier, audit.ID),
			"merged "+org.Slug+" task "+task.Identifier+" -> objective "+str(obj.ID)+" into task "+task.Identifier+" -> objective "+obj.DisplayID+" (2 rows)",
			org.Slug+": fixed 3 non-canonical pairs (6 rows)", "total: fixed 3 non-canonical pairs (6 rows)")
		_, canon, _, _ := split(out)
		wantTwin := canonLine{org.Slug, "task " + task.Identifier + " -> objective " + str(obj.ID), "task " + task.Identifier + " -> objective " + obj.DisplayID, "merge (duplicate)"}
		if len(canon) != 3 || canon[2] != wantTwin {
			t.Errorf("rows = %v, want the third to be %v", canon, wantTwin)
		}

		after := map[int64]string{}
		for _, r := range refRows(t, d, org.ID) {
			after[r.ID] = fmt.Sprintf("%s %s -> %s %s", r.SourceType, r.SourceID, r.TargetType, r.TargetID)
		}
		// Rewrite: same row ids.
		for _, c := range [][5]string{
			{"incident", inc.Identifier, "program", str(prog.ID), "incident " + inc.Identifier + " -> program PA"},
			{"program", str(prog.ID), "incident", inc.Identifier, "program PA -> incident " + inc.Identifier},
			// One-direction twin: the old A->B row is deleted, the old B->A row rewritten.
			{"objective", str(obj.ID), "task", task.Identifier, "objective " + obj.DisplayID + " -> task " + task.Identifier},
		} {
			id := findRow(t, before, c[0], c[1], c[2], c[3])
			if after[id] != c[4] {
				t.Errorf("row %d reads %q, want %q", id, after[id], c[4])
			}
		}
		// Merge: the old rows are gone.
		for _, c := range [][4]string{
			{"risk", risk.Identifier, "audit", strconv.Itoa(audit.ID)}, {"audit", strconv.Itoa(audit.ID), "risk", risk.Identifier},
			{"task", task.Identifier, "objective", str(obj.ID)},
		} {
			if id := findRow(t, before, c[0], c[1], c[2], c[3]); after[id] != "" {
				t.Errorf("old row %d survived as %q", id, after[id])
			}
		}
		// The canonical row that already existed is untouched.
		canonID := findRow(t, before, "task", task.Identifier, "objective", obj.DisplayID)
		if after[canonID] != "task "+task.Identifier+" -> objective "+obj.DisplayID {
			t.Errorf("canonical row %d = %q", canonID, after[canonID])
		}
		if len(after) != len(before)-3 {
			t.Errorf("row count %d -> %d, want -3", len(before), len(after))
		}
		if res2, out2 := run(org, false); res2.NonCanonical != 0 || !strings.Contains(out2, cleanCanon) {
			t.Errorf("second check not clean: %+v\n%s", res2, out2)
		}
	})

	t.Run("merge keeps manual when either row was manual", func(t *testing.T) {
		org := newCheckOrg(t, d, "can-origin")
		s := seeder{t, d, org}
		prog := s.program("PA")
		pid := str(prog.ID)
		r1, r2, r3 := s.risk("r1"), s.risk("r2"), s.risk("r3")
		for _, c := range []struct {
			risk       *db.Risk
			old, canon string
		}{{r1, "manual", "comment"}, {r2, "comment", "comment"}, {r3, "comment", "manual"}} {
			s.link("risk", c.risk.Identifier, "program", pid, true)
			s.link("risk", c.risk.Identifier, "program", "PA", true)
			s.setOrigin("risk", c.risk.Identifier, "program", pid, c.old)
			// The canonical rows come second, so set theirs by exact spelling.
			execSQL(t, d, org.ID, `UPDATE entity_references SET origin = $1 WHERE organization_id = $2
				AND ((source_type = 'risk' AND source_id = $3 AND target_id = 'PA') OR (source_type = 'program' AND source_id = 'PA' AND target_id = $3))`,
				c.canon, org.ID, c.risk.Identifier)
		}
		res, out := run(org, true)
		if res.Fixed != 3 || res.Duplicates != 3 {
			t.Errorf("result = %+v\n%s", res, out)
		}
		got := map[string]string{}
		for _, r := range refRows(t, d, org.ID) {
			if r.SourceType == "risk" {
				got[r.SourceID+" -> "+r.TargetID] = r.Origin
			}
		}
		want := map[string]string{
			r1.Identifier + " -> PA": "manual", r2.Identifier + " -> PA": "comment", r3.Identifier + " -> PA": "manual",
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("origins after merge = %v, want %v", got, want)
		}
	})

	t.Run("two spellings of one pair with no canonical row yet: rewrite then merge", func(t *testing.T) {
		org := newCheckOrg(t, d, "can-twice")
		s := seeder{t, d, org}
		prog := s.program("PA")
		obj := s.objective(prog.ID)
		s.link("objective", str(obj.ID), "program", str(prog.ID), true)
		s.link("objective", obj.DisplayID, "program", str(prog.ID), true)

		res, out := run(org, false)
		_, canon, _, _ := split(out)
		want := []canonLine{
			{org.Slug, "objective " + str(obj.ID) + " -> program " + str(prog.ID), "objective " + obj.DisplayID + " -> program PA", "rewrite"},
			{org.Slug, "objective " + obj.DisplayID + " -> program " + str(prog.ID), "objective " + obj.DisplayID + " -> program PA", "merge (duplicate)"},
		}
		if fmt.Sprint(canon) != fmt.Sprint(want) {
			t.Errorf("rows:\n got %v\nwant %v\n%s", canon, want, out)
		}
		if res.NonCanonical != 2 || res.Duplicates != 1 {
			t.Errorf("result = %+v", res)
		}
		res, out = run(org, true)
		if res.Fixed != 2 || res.FixedRows != 4 {
			t.Errorf("fix result = %+v\n%s", res, out)
		}
		var got []string
		for _, r := range refRows(t, d, org.ID) {
			got = append(got, fmt.Sprintf("%s %s -> %s %s", r.SourceType, r.SourceID, r.TargetType, r.TargetID))
		}
		sort.Strings(got)
		wantRows := []string{"objective " + obj.DisplayID + " -> program PA", "program PA -> objective " + obj.DisplayID}
		if fmt.Sprint(got) != fmt.Sprint(wantRows) {
			t.Errorf("rows after fix = %v, want %v", got, wantRows)
		}
	})

	t.Run("comment mentions in the old spelling are reported and rewritten", func(t *testing.T) {
		org := newCheckOrg(t, d, "can-comment")
		s := seeder{t, d, org}
		prog := s.program("PA")
		pid := str(prog.ID)
		r1, r2 := s.risk("r1"), s.risk("r2")
		// r1: a comment-only link under the old spelling.
		s.link("risk", r1.Identifier, "program", pid, true)
		s.setOrigin("risk", r1.Identifier, "program", pid, "comment")
		s.commentMention("entity_comment", 4242, "risk", r1.Identifier, "program", pid)
		// r2: the same comment already holds the canonical mention too.
		s.link("risk", r2.Identifier, "program", pid, true)
		s.link("risk", r2.Identifier, "program", "PA", true)
		s.setOrigin("risk", r2.Identifier, "program", pid, "comment")
		s.setOrigin("risk", r2.Identifier, "program", "PA", "comment")
		s.commentMention("entity_comment", 4243, "risk", r2.Identifier, "program", pid)
		s.commentMention("entity_comment", 4243, "risk", r2.Identifier, "program", "PA")

		res, out := run(org, false)
		_, _, _, canonSums := split(out)
		has(t, out, org.Slug+": 2 comment mention rows use a non-canonical id", "total: 2 comment mention rows use a non-canonical id")
		if strings.Contains(out, "mention rows uses") {
			t.Errorf("plural verb must agree:\n%s", out)
		}
		if res.NonCanonicalComments != 2 || res.NonCanonical != 2 {
			t.Errorf("result = %+v\n%v", res, canonSums)
		}
		if got := commentMentions(t, d, org.ID, 4242); fmt.Sprint(got) != fmt.Sprint([]string{"risk " + r1.Identifier + " -> program " + pid}) {
			t.Errorf("a read-only check changed the mentions: %v", got)
		}

		res, out = run(org, true)
		if res.FixedComments != 2 {
			t.Errorf("fix result = %+v\n%s", res, out)
		}
		has(t, out, org.Slug+": fixed 2 comment mention rows")
		if got := commentMentions(t, d, org.ID, 4242); fmt.Sprint(got) != fmt.Sprint([]string{"risk " + r1.Identifier + " -> program PA"}) {
			t.Errorf("mention not rewritten: %v", got)
		}
		if got := commentMentions(t, d, org.ID, 4243); fmt.Sprint(got) != fmt.Sprint([]string{"risk " + r2.Identifier + " -> program PA"}) {
			t.Errorf("duplicate mention not removed: %v", got)
		}
		if res2, out2 := run(org, false); res2.NonCanonical != 0 || res2.NonCanonicalComments != 0 || !strings.Contains(out2, cleanCanon) {
			t.Errorf("second check not clean: %+v\n%s", res2, out2)
		}

		// Deleting comment 4242 now finds, and removes, the rewritten link.
		err := d.WithOrgTx(ctx, org.ID, func(ctx context.Context, tx pgx.Tx) error {
			return db.RemoveCommentReferencesTx(ctx, tx, org.ID, "entity_comment", 4242)
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range refRows(t, d, org.ID) {
			if r.SourceID == r1.Identifier || r.TargetID == r1.Identifier {
				t.Errorf("comment-only link survived its comment: %+v", r)
			}
		}
	})

	t.Run("a single comment mention row reads in the singular", func(t *testing.T) {
		org := newCheckOrg(t, d, "can-single")
		s := seeder{t, d, org}
		prog := s.program("PA")
		risk := s.risk("r")
		s.link("risk", risk.Identifier, "program", str(prog.ID), true)
		s.commentMention("entity_comment", 4244, "risk", risk.Identifier, "program", str(prog.ID))
		_, out := run(org, false)
		has(t, out, org.Slug+": 1 comment mention row uses a non-canonical id", "total: 1 comment mention row uses a non-canonical id")
		_, out = run(org, true)
		has(t, out, org.Slug+": fixed 1 comment mention row")
	})

	t.Run("a broken pair is only broken, even if its healthy end is non-canonical", func(t *testing.T) {
		org := newCheckOrg(t, d, "can-broken")
		s := seeder{t, d, org}
		prog := s.program("PA")
		s.link("program", str(prog.ID), "risk", "RISK-99999", true)

		res, out := run(org, false)
		rows, canon, _, _ := split(out)
		if len(rows) != 1 || rows[0].Reason != "not found" || len(canon) != 0 {
			t.Errorf("broken %v, canonical %v\n%s", rows, canon, out)
		}
		has(t, out, cleanCanon)
		if res.Broken != 1 || res.NonCanonical != 0 {
			t.Errorf("result = %+v", res)
		}
		res, out = run(org, true)
		if res.Deleted != 1 || res.DeletedRows != 2 || res.Fixed != 0 || strings.Contains(out, "rewrote") {
			t.Errorf("fix result = %+v\n%s", res, out)
		}
		if left := rowSet(t, d, org.ID); len(left) != 0 {
			t.Errorf("rows left: %v", left)
		}
	})

	t.Run("a clean org says so in both sections", func(t *testing.T) {
		org := newCheckOrg(t, d, "can-clean")
		s := seeder{t, d, org}
		prog := s.program("PA")
		risk := s.risk("r")
		s.link("risk", risk.Identifier, "program", "PA", true)
		_ = prog
		for _, fix := range []bool{false, true} {
			res, out := run(org, fix)
			if res != (CheckResult{}) {
				t.Errorf("fix=%v: result = %+v, want zero", fix, res)
			}
			if out != cleanBroken+"\n"+cleanCanon+"\n" {
				t.Errorf("fix=%v: output =\n%s", fix, out)
			}
		}
	})
}
