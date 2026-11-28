package api

// Postgres-gated tests for archiving the versions of documents that are gone
// (a deleted document's versions are never served, listed, exported or counted
// again, and a new document that reuses the id starts clean), and for never
// archiving a live document's. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run DocArchive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/jackc/pgx/v5"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/license"
	"isms.sh/internal/isms/store"
)

func (f *tdFx) versionRows(doc string) (active, archived int) {
	f.t.Helper()
	rawQuery(f.t, `SELECT count(*) FILTER (WHERE archived_at IS NULL), count(*) FILTER (WHERE archived_at IS NOT NULL)
		FROM document_versions WHERE organization_id = $1 AND document_id = $2`, []any{f.o.id, doc}, &active, &archived)
	return
}

func (f *tdFx) deleteDocument(doc string) {
	f.t.Helper()
	f.mustOK(f.call(f.s.handleDeleteDocument, http.MethodDelete, "/api/v1/documents/"+doc, "", "manager", f.mgr, "docId", doc), "delete "+doc)
}

func (f *tdFx) createDocument(doc, title string) tdResp {
	body := fmt.Sprintf(`{"folder":"test","filename":"%s.md","document_id":"%s","title":"%s"}`, doc, doc, title)
	return f.call(f.s.handleCreateDocument, http.MethodPost, "/api/v1/documents", body, "manager", f.mgr, "", "")
}

// the document at a given id, as a fixture of its own
func (f *tdFx) doc(id string) *tdFx { g := *f; g.docID = id; return &g }

// A deleted document's id is reused: nothing of the old document is served.
func TestDocArchiveReusedIdServesNothingOld(t *testing.T) {
	f := newTDFx(t, "da1", "hr-pol")
	f.s.exportCaps = func() license.Capabilities {
		return license.Capabilities{PDF: license.SourceMetered, DOCX: license.SourceMetered}
	}
	f.save("# HR\n\nOLD APPROVED TEXT\n")
	f.approve()
	pubID := f.publishOK("public")
	oldRows, err := f.s.db.ListVersions(context.Background(), f.o.id, "hr-pol")
	if err != nil || len(oldRows) != 1 {
		t.Fatalf("old versions = %v %v", oldRows, err)
	}
	old := oldRows[0]
	if f.publicDoc(pubID).HTML == "" {
		t.Fatal("precondition")
	}

	f.deleteDocument("hr-pol")
	if a, ar := f.versionRows("hr-pol"); a != 0 || ar != 1 {
		t.Errorf("after delete: %d active, %d archived; want 0 and 1 (nothing is deleted)", a, ar)
	}
	f.mustOK(f.createDocument("hr-pol", "New HR"), "recreate")

	// The new draft has no approved version, so it cannot be published, and the
	// public page serves nothing of the old document.
	if r := f.publish("public", "manager", f.mgr); r.code != http.StatusConflict || !strings.Contains(r.body, CodeTrustNoApprovedVersion) {
		t.Fatalf("publish the recreated draft = %d %s", r.code, r.body)
	}
	f.assertNotFound(f.public(pubID), "the old public id")
	if body := f.items().body; strings.Contains(body, "OLD APPROVED TEXT") || strings.Contains(body, "HR") {
		t.Errorf("items = %s", body)
	}

	// Version history lists only the new document's versions.
	r := f.docCall(f.s.handleListVersions, http.MethodGet, "", "manager", f.mgr)
	if r.code != http.StatusOK || strings.Contains(r.body, old.CommitHash) || strings.Contains(r.body, `"id":`+fmt.Sprint(old.ID)+`,`) {
		t.Errorf("history = %d %s", r.code, r.body)
	}
	// The old version cannot be exported, by label or by row id.
	for _, q := range []string{"version=" + old.Version, fmt.Sprintf("version_id=%d", old.ID)} {
		r := f.call(f.s.handleExportDocument, http.MethodGet, "/api/v1/documents/hr-pol/export?format=docx&"+q, "", "manager", f.mgr, "docId", "hr-pol")
		if r.code != http.StatusNotFound || !strings.Contains(r.body, "export_version_not_found") {
			t.Errorf("export %s = %d %s", q, r.code, r.body)
		}
	}

	// Approved again at the SAME label: a new active row; the archived row is untouched.
	f.save("# HR\n\nNEW TEXT\n")
	f.approve()
	if a, ar := f.versionRows("hr-pol"); a != 1 || ar != 1 {
		t.Errorf("after approving the new document: %d active, %d archived", a, ar)
	}
	var commit string
	rawQuery(t, `SELECT commit_hash FROM document_versions WHERE id = $1`, []any{old.ID}, &commit)
	if commit != old.CommitHash {
		t.Error("the archived row was overwritten")
	}
	id := f.publishOK("public")
	d := f.publicDoc(id)
	if !strings.Contains(d.HTML, "NEW TEXT") || strings.Contains(d.HTML, "OLD APPROVED") {
		t.Errorf("served = %s", d.HTML)
	}
}

// A document that is gone yields no overdue review, and one that is live does.
func TestDocArchiveDeletedDocumentIsNotOverdue(t *testing.T) {
	f := newTDFx(t, "da2", "gone-pol")
	f.save("text\n")
	f.approve()
	live := f.withDoc("live-pol", "Live")
	live.save("text\n")
	live.approve()
	rawExec(t, `UPDATE document_versions SET created_at = now() - interval '5 years' WHERE organization_id = $1`, f.o.id)
	overdue := func() map[string]bool {
		items, err := f.s.db.OverdueDocumentReviews(context.Background(), f.o.id)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, it := range items {
			m[it.EntityID] = true
		}
		return m
	}
	if m := overdue(); !m["gone-pol"] || !m["live-pol"] {
		t.Fatalf("precondition: %v", m)
	}
	f.deleteDocument("gone-pol")
	if m := overdue(); m["gone-pol"] || !m["live-pol"] {
		t.Errorf("after delete: %v", m)
	}
}

// NDA: deleting the approved NDA and creating a draft with its id fails closed.
func TestDocArchiveNDAFailsClosedWhenItsDocumentIsRecreated(t *testing.T) {
	g := newNDAFx(t, "da3")
	g.setNDA(g.nda.docID)
	n := g.wantNDA(t, g.cookie)
	if r := g.accept(n.ContentHash, g.cookie); r.code != http.StatusOK {
		t.Fatalf("accept = %d %s", r.code, r.body)
	}
	g.wantOpen(t, g.cookie, "accepted")

	g.nda.deleteDocument(g.nda.docID)
	g.nda.mustOK(g.nda.createDocument(g.nda.docID, "New NDA draft"), "recreate")

	g.wantLocked(t, g.cookie, "NDA document recreated as a draft")
	if st := g.visitorNDA(t, g.cookie); st["status"] != "unavailable" {
		t.Errorf("visitor nda = %v, want unavailable", st)
	}
	if r := g.getNDA(g.cookie); r.code != http.StatusNotFound {
		t.Errorf("GET nda = %d %s, want the old text gone", r.code, r.body)
	}
	g.wantPublicOpen(t, g.cookie, "public items are unaffected")
}

// No false archive: edits, a review merge and a metadata change of a live document.
func TestDocArchiveLiveDocumentKeepsItsVersions(t *testing.T) {
	f := newTDFx(t, "da4", "live-pol")
	f.save("v1\n")
	f.approve()
	a0, _ := f.versionRows("live-pol")
	f.save("v2 draft\n")
	f.mustOK(f.docCall(f.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"classification":"internal","owner":"x@y.test"}}`, "manager", f.mgr), "metadata")
	f.approve()
	other := f.withDoc("other-pol", "Other") // a second document created next to it
	_ = other
	if a, ar := f.versionRows("live-pol"); a < a0 || ar != 0 {
		t.Errorf("%d active (was %d), %d archived: a live document lost history", a, a0, ar)
	}
	// Creating and deleting another document touches only that id.
	other.save("x\n")
	other.approve()
	other.deleteDocument("other-pol")
	if a, ar := f.versionRows("live-pol"); ar != 0 || a < a0 {
		t.Errorf("deleting another document archived this one: %d active, %d archived", a, ar)
	}
	if a, ar := f.versionRows("other-pol"); a != 0 || ar != 1 {
		t.Errorf("deleted document: %d active, %d archived", a, ar)
	}
}

// A HEAD that cannot be read archives nothing, whichever way it is asked.
func TestDocArchiveUnreadableHeadArchivesNothing(t *testing.T) {
	f := newTDFx(t, "da5", "pol-x")
	f.save("text\n")
	f.approve()
	ctx := context.Background()
	st := f.st

	// Break HEAD: point the branch at a commit that does not exist.
	sym, err := os.ReadFile(filepath.Join(st.Root(), "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	head := filepath.Join(st.Root(), strings.TrimSpace(strings.TrimPrefix(string(sym), "ref:")))
	good, err := os.ReadFile(head)
	if err != nil {
		t.Skipf("cannot reach the branch ref: %v", err)
	}
	if err := os.WriteFile(head, []byte("0123456789012345678901234567890123456789\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	restore := func() { _ = os.WriteFile(head, good, 0o644) }
	defer restore()

	if _, err := st.DocumentIDsAtHead(); err == nil {
		t.Fatal("DocumentIDsAtHead must fail on a broken HEAD")
	}
	if err := f.s.archiveIfAbsent(ctx, f.o.id, st, []string{"pol-x"}, "test"); err == nil {
		t.Error("archiveIfAbsent did not fail")
	}
	bogus := "0123456789012345678901234567890123456789"
	if err := f.s.archiveBetween(ctx, f.o.id, st, bogus, bogus, nil, "test"); err == nil {
		t.Error("archiveBetween did not fail")
	}
	if err := f.s.archiveAfterPush(ctx, f.o.id, st, pushBase{hash: bogus}); err == nil {
		t.Error("archiveAfterPush did not fail")
	}
	if err := f.s.archiveDeleted(ctx, f.o.id, st, "pol-x", bogus); err == nil {
		t.Error("archiveDeleted did not fail")
	}
	// A create cannot tell whether its id is free and does not write. (No
	// assertion distinguishes this from the code before the fix: a branch that
	// points at a missing commit fails the write itself there too. What the guard
	// adds is that nothing is archived first, asserted below.)
	if r := f.createDocument("fresh-id", "Fresh"); r.code != http.StatusInternalServerError {
		t.Errorf("create with an unreadable HEAD = %d %s", r.code, r.body)
	}
	if a, ar := f.versionRows("pol-x"); a != 1 || ar != 0 {
		t.Errorf("a read error archived: %d active, %d archived", a, ar)
	}
	restore()
	// The idempotent guard: absent AND readable archives; present does not.
	if err := f.s.archiveIfAbsent(ctx, f.o.id, st, []string{"pol-x"}, "test"); err != nil {
		t.Fatal(err)
	}
	if a, ar := f.versionRows("pol-x"); a != 1 || ar != 0 {
		t.Errorf("archiving an id that is present: %d active, %d archived", a, ar)
	}
}

// ---- templates -------------------------------------------------------------

func writeTemplate(t *testing.T, name string, ids ...string) {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.yaml"), []byte("id: "+name+"\nname: T\nversion: \"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		raw := "---\ndocument_id: " + id + "\ntitle: " + id + "\nversion: \"1.0\"\nstatus: draft\n---\nTemplate text for " + id + "\n"
		if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ISMS_TEMPLATE_PATH", base)
}

func (f *tdFx) addTemplate(name string) {
	f.t.Helper()
	f.mustOK(f.call(f.s.handleAddTemplate, http.MethodPost, "/api/v1/templates", `{"template":"`+name+`"}`, "manager", f.mgr, "", ""), "add template")
	f.reopenStore() // the handler drops the cached store
}

// reopenStore registers a fresh store for the organization's repository.
func (f *tdFx) reopenStore() {
	f.t.Helper()
	st, err := store.NewBare(f.st.Root())
	if err != nil {
		f.t.Fatal(err)
	}
	f.s.stores.Store(f.o.id, st)
	f.st = st
}

// Adding a template again, over documents that are approved, archives nothing;
// adding it after a document was deleted starts that document clean.
func TestDocArchiveTemplates(t *testing.T) {
	writeTemplate(t, "tpl196", "tpl-a", "tpl-b")
	f := newTDFx(t, "da6", "base-doc")
	f.addTemplate("tpl196")
	a := f.doc("tpl-a")
	a.save("approved A\n")
	a.approve()
	b := f.doc("tpl-b")
	b.save("approved B\n")
	b.approve()

	f.addTemplate("tpl196") // a second scaffold over existing documents
	for _, id := range []string{"tpl-a", "tpl-b"} {
		if act, arch := f.versionRows(id); act != 1 || arch != 0 {
			t.Errorf("re-scaffold: %s has %d active, %d archived", id, act, arch)
		}
	}

	// A document deleted before archiving existed: its versions are still active.
	a.deleteDocument("tpl-a") // archived by the delete
	rawExec(t, `UPDATE document_versions SET archived_at = NULL WHERE organization_id = $1 AND document_id = 'tpl-a'`, f.o.id)
	f.addTemplate("tpl196")
	if act, arch := f.versionRows("tpl-a"); act != 0 || arch != 1 {
		t.Errorf("re-adding a deleted template document: %d active, %d archived", act, arch)
	}
	if act, arch := f.versionRows("tpl-b"); act != 1 || arch != 0 {
		t.Errorf("the other template document: %d active, %d archived", act, arch)
	}

	// Removing the template archives the ids that leave.
	f.mustOK(f.call(f.s.handleRemoveTemplate, http.MethodDelete, "/api/v1/templates/tpl196", "", "manager", f.mgr, "name", "tpl196"), "remove template")
	f.reopenStore()
	if act, arch := f.versionRows("tpl-b"); act != 0 || arch != 1 {
		t.Errorf("after removing the template: %d active, %d archived", act, arch)
	}
}

// ---- git push --------------------------------------------------------------

type pushFx struct {
	*tdFx
	srv     *httptest.Server
	jwt     string
	repo    string
	cloneAt string // a scratch directory to run git from
}

func newPushFx(t *testing.T, tag, docID string) *pushFx {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	f := newTDFx(t, tag, docID)
	u := contractTestUser(t, f.s, f.o.id, "pusher-"+tag+"@push.test", "manager")
	jwt, err := f.s.createSessionJWT(u, f.o.id, "manager", f.o.slug, "Org")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(f.s.echo)
	t.Cleanup(srv.Close)
	return &pushFx{tdFx: f, srv: srv, jwt: jwt, repo: f.st.Root(), cloneAt: t.TempDir()}
}

func (p *pushFx) git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "http.extraHeader=Authorization: Bearer " + p.jwt,
		"-c", "user.name=Pusher", "-c", "user.email=pusher@push.test", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (p *pushFx) clone() string {
	p.t.Helper()
	org, err := p.s.db.GetOrganization(context.Background(), p.o.id)
	if err != nil {
		p.t.Fatal(err)
	}
	dir := filepath.Join(p.t.TempDir(), "clone")
	if out, err := p.git(p.cloneAt, "clone", p.srv.URL+"/git/"+org.UUID, dir); err != nil {
		p.t.Fatalf("clone: %v\n%s", err, out)
	}
	return dir
}

// reopen re-registers the store: a push drops the cached one.
func (p *pushFx) reopen() {
	p.t.Helper()
	p.reopenStore()
}

func writeDoc(t *testing.T, path, id, body string) {
	t.Helper()
	raw := "---\ndocument_id: " + id + "\ntitle: " + id + "\nversion: \"1.0\"\nstatus: draft\n---\n" + body + "\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDocArchiveGitPush(t *testing.T) {
	p := newPushFx(t, "da7", "base-doc")
	for _, id := range []string{"push-del", "push-keep", "push-move", "push-dup"} {
		d := p.withDoc(id, id)
		d.save("approved " + id + "\n")
		d.approve()
	}
	dir := p.clone()
	docs := filepath.Join(dir, "documents", "test")
	push := func() (string, error) { return p.git(dir, "push", "origin", "HEAD") }
	must := func(out string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}

	// 1. A rejected push (a duplicate document id) is reset: nothing is archived,
	// even though the same push removes a document.
	must(p.git(dir, "rm", "-q", "documents/test/push-keep.md"))
	writeDoc(t, filepath.Join(docs, "dup-a.md"), "dup-id", "x")
	writeDoc(t, filepath.Join(docs, "dup-b.md"), "dup-id", "y")
	must(p.git(dir, "add", "-A"))
	must(p.git(dir, "commit", "-q", "-m", "bad push"))
	if out, err := push(); err == nil {
		t.Fatalf("the duplicate-id push was accepted\n%s", out)
	}
	p.reopen()
	if a, ar := p.versionRows("push-keep"); a != 1 || ar != 0 {
		t.Errorf("rejected push: push-keep has %d active, %d archived", a, ar)
	}
	if p.st.FindDocumentByID("push-keep") == "" {
		t.Fatal("the rejected push was not reset")
	}
	must(p.git(dir, "reset", "-q", "--hard", "HEAD~1"))

	// 2. Delete + re-add of one id in one push (a move) is an edit.
	must(p.git(dir, "mv", "documents/test/push-move.md", "documents/test/push-moved.md"))
	must(p.git(dir, "commit", "-q", "-m", "move"))
	must(push())
	p.reopen()
	if a, ar := p.versionRows("push-move"); a != 1 || ar != 0 {
		t.Errorf("a move archived: %d active, %d archived", a, ar)
	}

	// 3. An accepted push that removes a document archives it.
	must(p.git(dir, "rm", "-q", "documents/test/push-del.md"))
	must(p.git(dir, "commit", "-q", "-m", "delete"))
	must(push())
	p.reopen()
	if a, ar := p.versionRows("push-del"); a != 0 || ar != 1 {
		t.Errorf("accepted deletion: %d active, %d archived", a, ar)
	}
	if a, ar := p.versionRows("push-keep"); a != 1 || ar != 0 {
		t.Errorf("another document: %d active, %d archived", a, ar)
	}
	// ... and recreating it through the API serves none of the old text.
	p.mustOK(p.createDocument("push-del", "Recreated"), "recreate")
	r := p.doc("push-del").publish("public", "manager", p.mgr)
	if r.code != http.StatusConflict || !strings.Contains(r.body, CodeTrustNoApprovedVersion) {
		t.Errorf("publish after the push deletion = %d %s", r.code, r.body)
	}

	// 4. An id that appears through a push starts with no history: versions left
	// by a document deleted before archiving existed are archived.
	if err := p.s.db.RecordVersion(context.Background(), p.o.id, &db.DocumentVersion{DocumentID: "ghost-doc", Version: "1.0", CommitHash: "c", FilePath: "documents/test/ghost-doc.md", CreatedBy: "x@y.test"}); err != nil {
		t.Fatal(err)
	}
	writeDoc(t, filepath.Join(docs, "ghost-doc.md"), "ghost-doc", "new")
	must(p.git(dir, "add", "-A"))
	must(p.git(dir, "commit", "-q", "-m", "ghost"))
	must(p.git(dir, "pull", "-q", "--rebase", "origin", "main"))
	must(push())
	p.reopen()
	if a, ar := p.versionRows("ghost-doc"); a != 0 || ar != 1 {
		t.Errorf("an id that appeared: %d active, %d archived", a, ar)
	}
}

func TestDocArchiveTheDiffsAreExact(t *testing.T) {
	before := map[string]bool{"a": true, "b": true, "c": true}
	after := map[string]bool{"b": true, "c": true, "d": true}
	if got := fmt.Sprint(idsAbsentFrom(before, after)); got != "[a]" {
		t.Errorf("vanished = %s", got)
	}
	if got := fmt.Sprint(idsAbsentFrom(after, before)); got != "[d]" {
		t.Errorf("appeared = %s", got)
	}
}

// ---- the safety brake and the template scope ------------------------------------

// Removing a template archives only the template's own documents that left.
func TestDocArchiveTemplateRemovalIsScoped(t *testing.T) {
	writeTemplate(t, "tpl196s", "ts-a")
	f := newTDFx(t, "da8", "base-doc")
	f.addTemplate("tpl196s")
	a := f.doc("ts-a")
	a.save("a\n")
	a.approve()
	// An organization's own document in the template's folder, and one elsewhere.
	madeBody := `{"folder":"tpl196s","filename":"org-made.md","document_id":"org-made","title":"Org made"}`
	f.mustOK(f.call(f.s.handleCreateDocument, http.MethodPost, "/api/v1/documents", madeBody, "manager", f.mgr, "", ""), "create in the template folder")
	made := f.doc("org-made")
	made.save("m\n")
	made.approve()
	f.mustOK(f.call(f.s.handleRemoveTemplate, http.MethodDelete, "/api/v1/templates/tpl196s", "", "manager", f.mgr, "name", "tpl196s"), "remove template")
	f.reopenStore()
	if act, arch := f.versionRows("ts-a"); act != 0 || arch != 1 {
		t.Errorf("the template's document: %d active, %d archived", act, arch)
	}
	// Out of the brief's scope: the removal deleted the organization's own document
	// too, but only the template's ids are archived (its versions are archived when
	// the id is next created).
	if act, arch := f.versionRows("org-made"); act != 1 || arch != 0 {
		t.Errorf("the organization's own document: %d active, %d archived", act, arch)
	}
	if act, _ := f.versionRows("base-doc"); act != 0 {
		t.Errorf("base-doc has %d versions", act)
	}
}

// ---- reviews of a document that is gone -------------------------------------

func (f *tdFx) sendAndApprove() string {
	f.t.Helper()
	b, _ := jsonMarshal(map[string]any{"reviewers": []string{f.rev}})
	r := f.mustOK(f.docCall(f.s.handleReviewSend, http.MethodPost, string(b), "admin", f.admin), "send")
	id := reviewIDOf(f.t, r.body)
	f.mustOK(f.call(f.s.handleReviewApprove, http.MethodPost, "/api/v1/reviews/"+id+"/approve",
		`{"decision":"approved","comment":"ok"}`, "contributor", f.rev, "id", id), "approve")
	return id
}

func (f *tdFx) mergeReviewID(id string) tdResp {
	return f.call(f.s.handleMergeReview, http.MethodPost, "/api/v1/reviews/"+id+"/merge", `{}`, "admin", f.admin, "id", id)
}

func (f *tdFx) reviewStatus(id string) string {
	f.t.Helper()
	var st string
	rawQuery(f.t, `SELECT status FROM reviews WHERE id = $1::int`, []any{id}, &st)
	return st
}

func TestDocArchiveReviewOfADeletedDocumentCannotBeMerged(t *testing.T) {
	f := newTDFx(t, "da9", "rev-pol")
	f.save("first text\n")
	f.approve() // a version, so the id has history
	f.save("reviewed text\n")
	rid := f.sendAndApprove()
	if st := f.reviewStatus(rid); st != "approved" {
		t.Fatalf("precondition: review is %s", st)
	}

	f.deleteDocument("rev-pol")
	if st := f.reviewStatus(rid); st != "closed" {
		t.Errorf("the review of the deleted document is %s, want closed", st)
	}
	var n int
	rawQuery(t, `SELECT count(*) FROM activity WHERE organization_id = $1 AND review_id = $2::int AND action = 'review_status_changed' AND detail LIKE '%no longer exists%'`, []any{f.o.id, rid}, &n)
	if n != 1 {
		t.Errorf("close activity rows = %d", n)
	}

	// The id is reused in another folder; the review cannot be merged into it.
	body := `{"folder":"elsewhere","filename":"renamed.md","document_id":"rev-pol","title":"Other incarnation"}`
	f.mustOK(f.call(f.s.handleCreateDocument, http.MethodPost, "/api/v1/documents", body, "manager", f.mgr, "", ""), "recreate elsewhere")
	if r := f.mergeReviewID(rid); r.code < 400 {
		t.Errorf("merge of a closed review = %d %s", r.code, r.body)
	}

	// A review that was never closed (it models a state from before archiving
	// existed), with its sent_head INTACT, is refused by the guard: a version of
	// the id was archived after the review was created.
	rawExec(t, `UPDATE reviews SET status = 'approved' WHERE id = $1::int`, rid)
	r := f.mergeReviewID(rid)
	if r.code != http.StatusConflict || !strings.Contains(r.body, "modified since") {
		t.Fatalf("merge of a reopened review into another incarnation = %d %s", r.code, r.body)
	}
	if a, ar := f.versionRows("rev-pol"); a != 0 || ar != 1 {
		t.Errorf("versions: %d active, %d archived; the merge wrote a row", a, ar)
	}
	if st := f.reviewStatus(rid); st != "approved" {
		t.Errorf("a refused merge changed the review to %s", st)
	}
	doc, err := f.st.LoadDocument(f.st.FindDocumentByID("rev-pol"))
	if err != nil || doc.Frontmatter.Status != "draft" {
		t.Errorf("the recreated draft is now %+v (%v)", doc.Frontmatter.Status, err)
	}
}

// The same, for a document deleted and created again before archiving existed:
// nothing was ever archived, so only the content check, by id, can tell.
func TestDocArchivePreFixRecreationIsRefusedByTheContentCheck(t *testing.T) {
	f := newTDFx(t, "da9b", "pre-pol")
	f.save("first\n")
	f.approve()
	f.save("reviewed\n")
	rid := f.sendAndApprove()
	// Delete and recreate straight through the store: no archive, no review closed.
	abs := f.st.FindDocumentByID("pre-pol")
	if _, err := f.st.DeleteFile(abs, "T", "t@example.test", "delete (before the fix)"); err != nil {
		t.Fatal(err)
	}
	raw := "---\ndocument_id: pre-pol\ntitle: Other\nversion: \"0.1\"\nstatus: draft\n---\nnew incarnation\n"
	if _, err := f.st.CommitFile(filepath.Join(f.st.Root(), "documents", "elsewhere", "renamed.md"), []byte(raw), "T", "t@example.test", "recreate"); err != nil {
		t.Fatal(err)
	}
	if a, ar := f.versionRows("pre-pol"); a != 1 || ar != 0 {
		t.Fatalf("precondition: %d active, %d archived", a, ar)
	}
	if st := f.reviewStatus(rid); st != "approved" {
		t.Fatalf("precondition: %s", st)
	}
	r := f.mergeReviewID(rid)
	if r.code != http.StatusConflict || !strings.Contains(r.body, "modified since") {
		t.Fatalf("merge = %d %s", r.code, r.body)
	}
	if a, ar := f.versionRows("pre-pol"); a != 1 || ar != 0 {
		t.Errorf("the refused merge changed the versions: %d active, %d archived", a, ar)
	}
	doc, _ := f.st.LoadDocument(f.st.FindDocumentByID("pre-pol"))
	if doc == nil || doc.Frontmatter.Status != "draft" {
		t.Errorf("the recreated draft changed: %+v", doc)
	}
}

// A closed or merged review is not brought back by a decision.
func TestDocArchiveApprovingAFinishedReviewIsRefused(t *testing.T) {
	f := newTDFx(t, "da9c", "fin-pol")
	f.save("text\n")
	rid := f.sendAndApprove()
	f.mustOK(f.call(f.s.handleUpdateReviewStatus, http.MethodPut, "/api/v1/reviews/"+rid+"/status", `{"status":"closed"}`, "manager", f.mgr, "id", rid), "close")
	for _, decision := range []string{"approved", "changes_requested"} {
		r := f.call(f.s.handleReviewApprove, http.MethodPost, "/api/v1/reviews/"+rid+"/approve",
			`{"decision":"`+decision+`","comment":"again"}`, "contributor", f.rev, "id", rid)
		if r.code != http.StatusBadRequest || !strings.Contains(r.body, CodeReviewWrongStatus) {
			t.Errorf("%s on a closed review = %d %s", decision, r.code, r.body)
		}
		if st := f.reviewStatus(rid); st != "closed" {
			t.Errorf("a decision changed the closed review to %s", st)
		}
	}
	// The database refuses it as well, whatever the handler checked: a close that
	// raced with a change is not undone.
	err := f.s.db.UpdateReviewStatusFrom(context.Background(), f.o.id, atoi(t, rid), "approved", db.LiveReviewStatuses)
	if !errors.Is(err, db.ErrReviewStatusChanged) {
		t.Errorf("UpdateReviewStatusFrom on a closed review = %v", err)
	}
	if st := f.reviewStatus(rid); st != "closed" {
		t.Errorf("the conditional update changed the status to %s", st)
	}
	// The transaction variants (forward, resubmit, approve) refuse it as well.
	txErr := f.s.db.WithOrgTx(context.Background(), f.o.id, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.UpdateReviewStatusFromTx(ctx, tx, f.o.id, atoi(t, rid), "open", db.LiveReviewStatuses); !errors.Is(err, db.ErrReviewStatusChanged) {
			t.Errorf("UpdateReviewStatusFromTx = %v", err)
		}
		if err := db.ResubmitReviewTx(ctx, tx, f.o.id, atoi(t, rid), "0123456789012345678901234567890123456789"); !errors.Is(err, db.ErrReviewStatusChanged) {
			t.Errorf("ResubmitReviewTx = %v", err)
		}
		return nil
	})
	if txErr != nil {
		t.Fatal(txErr)
	}
	if st := f.reviewStatus(rid); st != "closed" {
		t.Errorf("a transaction variant changed the status to %s", st)
	}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	var n int
	if _, err := fmt.Sscan(s, &n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A live document's review still merges: the guard is not a false refusal.
func TestDocArchiveIncarnationGuardAllowsALiveReview(t *testing.T) {
	f := newTDFx(t, "da10", "live-rev")
	f.save("live text\n")
	rid := f.sendAndApprove()
	if r := f.mergeReviewID(rid); r.code != http.StatusOK {
		t.Fatalf("merge = %d %s", r.code, r.body)
	}
	if a, ar := f.versionRows("live-rev"); a != 1 || ar != 0 {
		t.Errorf("versions: %d active, %d archived", a, ar)
	}
}

// ---- POST /documents never overwrites a file --------------------------------------

func TestDocArchiveCreateRefusesAnExistingPath(t *testing.T) {
	f := newTDFx(t, "da11", "base-doc")
	other := f.withDoc("owner-of-path", "Owner")
	other.save("OWNER TEXT\n")
	body := `{"folder":"test","filename":"owner-of-path.md","document_id":"another-id","title":"Intruder"}`
	r := f.call(f.s.handleCreateDocument, http.MethodPost, "/api/v1/documents", body, "manager", f.mgr, "", "")
	if r.code != http.StatusConflict {
		t.Fatalf("create over an existing file = %d %s", r.code, r.body)
	}
	doc, err := f.st.LoadDocument(f.st.FindDocumentByID("owner-of-path"))
	if err != nil || !strings.Contains(doc.Body, "OWNER TEXT") {
		t.Errorf("the existing document was overwritten: %v %+v", err, doc)
	}
	if f.st.FindDocumentByID("another-id") != "" {
		t.Error("a document was created")
	}
}

// ---- overdue reviews of a recreated id ----------------------------------------------

func TestDocArchiveOverdueDoesNotUseAnOldIncarnationsReview(t *testing.T) {
	f := newTDFx(t, "da12", "old-pol")
	f.save("text\n")
	f.approve()
	f.deleteDocument("old-pol")
	f.mustOK(f.createDocument("old-pol", "New incarnation"), "recreate")
	// The new document has a version of its own, recorded without a review.
	ver := &db.DocumentVersion{DocumentID: "old-pol", Version: "0.1", CommitHash: "c", FilePath: "documents/test/old-pol.md", CreatedBy: f.mgr}
	if err := f.s.db.RecordVersion(context.Background(), f.o.id, ver); err != nil {
		t.Fatal(err)
	}
	rawExec(t, `UPDATE document_versions SET created_at = now() - interval '5 years' WHERE id = $1`, ver.ID)
	rawExec(t, `UPDATE reviews SET updated_at = now() - interval '6 years' WHERE organization_id = $1 AND document_id = 'old-pol'`, f.o.id)
	items, err := f.s.db.OverdueDocumentReviews(context.Background(), f.o.id)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, it := range items {
		if it.EntityID == "old-pol" {
			found = true
			if it.Title != "old-pol" {
				t.Errorf("title = %q: the old incarnation's review title leaked", it.Title)
			}
		}
	}
	if !found {
		t.Fatal("the recreated document is not overdue")
	}
}

// ---- pushes ------------------------------------------------------------------------------

func TestDocArchiveRejectedMultiCommitPushLeavesHeadAsItWas(t *testing.T) {
	p := newPushFx(t, "da13", "base-doc")
	for _, id := range []string{"mc-del", "mc-keep"} {
		d := p.withDoc(id, id)
		d.save("approved " + id + "\n")
		d.approve()
	}
	pre, _, err := p.st.DefaultBranchHash()
	if err != nil {
		t.Fatal(err)
	}
	dir := p.clone()
	docs := filepath.Join(dir, "documents", "test")
	must := func(out string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	// Commit 1 is valid and removes a document; commit 2 is invalid.
	must(p.git(dir, "rm", "-q", "documents/test/mc-del.md"))
	must(p.git(dir, "commit", "-q", "-m", "valid first commit"))
	writeDoc(t, filepath.Join(docs, "dup-a.md"), "dup-id", "x")
	writeDoc(t, filepath.Join(docs, "dup-b.md"), "dup-id", "y")
	must(p.git(dir, "add", "-A"))
	must(p.git(dir, "commit", "-q", "-m", "invalid second commit"))
	if out, err := p.git(dir, "push", "origin", "HEAD"); err == nil {
		t.Fatalf("the push was accepted\n%s", out)
	}
	p.reopen()
	post, _, err := p.st.DefaultBranchHash()
	if err != nil || post != pre {
		t.Errorf("HEAD after a rejected 2-commit push = %s, want the pre-push %s (%v)", post, pre, err)
	}
	if p.st.FindDocumentByID("mc-del") == "" {
		t.Error("the first commit of the rejected push stayed")
	}
	for _, id := range []string{"mc-del", "mc-keep"} {
		if a, ar := p.versionRows(id); a != 1 || ar != 0 {
			t.Errorf("%s: %d active, %d archived after a rejected push", id, a, ar)
		}
	}
}

// A push that adds many files into a repository that already has a pack is read
// through a store opened after the push.
func TestDocArchiveBigPushIsSeen(t *testing.T) {
	p := newPushFx(t, "da14", "base-doc")
	d := p.withDoc("bp-seed", "seed")
	d.save("seed\n")
	d.approve()
	// Leftovers of documents that were deleted before archiving existed.
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("bulk-%d", i)
		if err := p.s.db.RecordVersion(context.Background(), p.o.id, &db.DocumentVersion{DocumentID: id, Version: "1.0", CommitHash: "c", FilePath: "documents/test/" + id + ".md", CreatedBy: "x@y.test"}); err != nil {
			t.Fatal(err)
		}
	}
	// Put what exists into a pack, and make the server's store read it.
	if out, err := exec.Command("git", "-C", p.repo, "gc", "-q").CombinedOutput(); err != nil {
		t.Skipf("git gc: %v %s", err, out)
	}
	// The server's cached store is left as it was: after the gc it cannot read
	// the packed objects, which is the situation the push handler must cope with.

	dir := p.clone()
	for i := 0; i < 160; i++ {
		writeDoc(t, filepath.Join(dir, "documents", "bulk", fmt.Sprintf("bulk-%d.md", i)), fmt.Sprintf("bulk-%d", i), strings.Repeat("filler text ", 30)+fmt.Sprint(i))
	}
	if out, err := p.git(dir, "add", "-A"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, err := p.git(dir, "commit", "-q", "-m", "bulk"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, err := p.git(dir, "push", "origin", "HEAD"); err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}
	p.reopen()
	for i := 0; i < 3; i++ {
		if a, ar := p.versionRows(fmt.Sprintf("bulk-%d", i)); a != 0 || ar != 1 {
			t.Errorf("bulk-%d: %d active, %d archived: the push's new ids were not seen", i, a, ar)
		}
	}
	if a, ar := p.versionRows("bp-seed"); a != 1 || ar != 0 {
		t.Errorf("bp-seed: %d active, %d archived", a, ar)
	}
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func reviewIDOf(t *testing.T, body string) string {
	t.Helper()
	var resp struct {
		ReviewID int `json:"review_id"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.ReviewID == 0 {
		t.Fatalf("review response %s: %v", body, err)
	}
	return fmt.Sprint(resp.ReviewID)
}

// ---- review round 3 ---------------------------------------------------------------

// The appearance leg of a push is refused when the "before" side cannot be read,
// or reads as no documents in a repository that had commits.
func TestDocArchiveAppearanceLegNeedsATrustworthyBefore(t *testing.T) {
	f := newTDFx(t, "da15", "pol-y")
	f.save("text\n")
	f.approve()
	// Another document's leftover version: its id is at HEAD, and absent "before".
	ghost := f.withDoc("ghost-y", "Ghost")
	_ = ghost
	if err := f.s.db.RecordVersion(context.Background(), f.o.id, &db.DocumentVersion{DocumentID: "ghost-y", Version: "1.0", CommitHash: "c", FilePath: "documents/test/ghost-y.md", CreatedBy: "x@y.test"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	bogus := "0123456789012345678901234567890123456789"
	if err := f.s.archiveAfterPush(ctx, f.o.id, f.st, pushBase{hash: bogus}); err == nil {
		t.Error("a before that cannot be read was accepted")
	}
	// A base commit that has no documents although the repository had commits.
	dir := filepath.Join(t.TempDir(), "empty-before.git")
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatal(err)
	}
	st, err := store.NewBare(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CommitFile(filepath.Join(dir, "README.md"), []byte("# readme\n"), "T", "t@example.test", "init"); err != nil {
		t.Fatal(err)
	}
	readme, _, _ := st.DefaultBranchHash()
	raw := "---\ndocument_id: ghost-y\ntitle: G\nstatus: draft\n---\nx\n"
	if _, err := st.CommitFile(filepath.Join(dir, "documents", "t", "ghost-y.md"), []byte(raw), "T", "t@example.test", "doc"); err != nil {
		t.Fatal(err)
	}
	// A base commit that really has no documents is a true empty set: the id that
	// appears is archived (a document deleted before this existed).
	if err := f.s.archiveAfterPush(ctx, f.o.id, st, pushBase{hash: readme}); err != nil {
		t.Errorf("a true empty before: %v", err)
	}
	if a, ar := f.versionRows("ghost-y"); a != 0 || ar != 1 {
		t.Errorf("ghost-y: %d active, %d archived", a, ar)
	}
}

// When the store cannot be reopened after a push the push still completes and
// nothing is archived; archiving through the locked store would hang the org.
func TestDocArchivePushWithoutAFreshStoreSkipsArchiving(t *testing.T) {
	p := newPushFx(t, "da16", "base-doc")
	d := p.withDoc("dl-doc", "dl")
	d.save("first\n")
	d.approve() // a version
	d.save("text\n")
	rid := d.sendAndApprove()
	dir := p.clone()
	defer setOpenFreshStore(func(string) (*store.Store, error) { return nil, fmt.Errorf("cannot open") })()
	done := make(chan error, 1)
	go func() {
		out, err := p.git(dir, "rm", "-q", "documents/test/dl-doc.md")
		if err == nil {
			if out, err = p.git(dir, "commit", "-q", "-m", "delete"); err == nil {
				out, err = p.git(dir, "push", "origin", "HEAD")
			}
		}
		if err != nil {
			err = fmt.Errorf("%v: %s", err, out)
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the push hung")
	}
	p.reopen()
	if a, ar := p.versionRows("dl-doc"); a != 1 || ar != 0 {
		t.Errorf("without a fresh store: %d active, %d archived", a, ar)
	}
	if st := p.reviewStatus(rid); st != "approved" {
		t.Errorf("the review is %s", st)
	}
}

// A live document that a push moved between send and merge still merges.
func TestDocArchiveMovedLiveDocumentStillMerges(t *testing.T) {
	p := newPushFx(t, "da17", "base-doc")
	d := p.withDoc("mv-live", "mv")
	d.save("moving text\n")
	rid := d.sendAndApprove()
	dir := p.clone()
	must := func(out string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	must(p.git(dir, "mv", "documents/test/mv-live.md", "documents/test/mv-live-renamed.md"))
	must(p.git(dir, "commit", "-q", "-m", "rename"))
	must(p.git(dir, "push", "origin", "HEAD"))
	p.reopen()
	if st := p.reviewStatus(rid); st != "approved" {
		t.Fatalf("the push closed a live review: %s", st)
	}
	if r := p.doc("mv-live").mergeReviewID(rid); r.code != http.StatusOK {
		t.Fatalf("merge of a moved live document = %d %s", r.code, r.body)
	}
	if a, ar := p.versionRows("mv-live"); a != 1 || ar != 0 {
		t.Errorf("versions: %d active, %d archived", a, ar)
	}
}

// Forwarding must not open a finished review.
func TestDocArchiveForwardingAFinishedReviewIsRefused(t *testing.T) {
	f := newTDFx(t, "da18", "fw-pol")
	f.save("text\n")
	rid := f.sendAndApprove()
	f.deleteDocument("fw-pol")
	if st := f.reviewStatus(rid); st != "closed" {
		t.Fatalf("precondition: %s", st)
	}
	body := fmt.Sprintf(`{"reviewers":[%q]}`, f.rev)
	r := f.call(f.s.handleForwardReview, http.MethodPost, "/api/v1/reviews/"+rid+"/forward", body, "manager", f.mgr, "id", rid)
	if r.code != http.StatusBadRequest || !strings.Contains(r.body, CodeReviewWrongStatus) {
		t.Errorf("forward a closed review = %d %s", r.code, r.body)
	}
	if st := f.reviewStatus(rid); st != "closed" {
		t.Errorf("forwarding reopened the review: %s", st)
	}
}

// Removing the only template leaves no documents; its ids are archived and
// their reviews closed, because the empty commit is read strictly.
func TestDocArchiveRemovingTheOnlyTemplate(t *testing.T) {
	writeTemplate(t, "tpl196o", "to-a")
	f := newTDFx(t, "da19", "base-doc")
	f.addTemplate("tpl196o")
	a := f.doc("to-a")
	a.save("a\n")
	rid := a.sendAndApprove()
	a.mergeReviewID(rid) // a version, then a new review that stays open
	a.save("a edited\n")
	rid2 := a.sendAndApprove()
	f.deleteDocument("base-doc") // now the template's document is the only one
	f.mustOK(f.call(f.s.handleRemoveTemplate, http.MethodDelete, "/api/v1/templates/tpl196o", "", "manager", f.mgr, "name", "tpl196o"), "remove template")
	f.reopenStore()
	if act, arch := f.versionRows("to-a"); act != 0 || arch < 1 {
		t.Errorf("after removing the only template: %d active, %d archived", act, arch)
	}
	if st := f.reviewStatus(rid2); st != "closed" {
		t.Errorf("the review is %s, want closed", st)
	}
}

// A push that removes every document archives them all and closes their
// reviews; a later push that adds them back archives nothing that is live.
func TestDocArchivePushRemovingEveryDocument(t *testing.T) {
	p := newPushFx(t, "da20", "base-doc")
	ids := []string{"all-a", "all-b"}
	var rids []string
	for _, id := range ids {
		d := p.withDoc(id, id)
		d.save("first " + id + "\n")
		d.approve()
		d.save("second " + id + "\n")
		rids = append(rids, d.sendAndApprove())
	}
	dir := p.clone()
	must := func(out string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	must(p.git(dir, "rm", "-rq", "documents"))
	must(p.git(dir, "commit", "-q", "-m", "remove everything"))
	must(p.git(dir, "push", "origin", "HEAD"))
	p.reopen()
	for i, id := range ids {
		if a, ar := p.versionRows(id); a != 0 || ar != 1 {
			t.Errorf("%s after the push: %d active, %d archived", id, a, ar)
		}
		if st := p.reviewStatus(rids[i]); st != "closed" {
			t.Errorf("review of %s is %s, want closed", id, st)
		}
	}
	// A document created in between is live; the push that adds the old ids back
	// archives nothing that is live.
	live := p.withDoc("live-between", "live")
	live.save("live\n")
	live.approve()
	must(p.git(dir, "pull", "-q", "--rebase", "origin", "main"))
	for _, id := range ids {
		writeDoc(t, filepath.Join(dir, "documents", "test", id+".md"), id, "added back")
	}
	must(p.git(dir, "add", "-A"))
	must(p.git(dir, "commit", "-q", "-m", "add back"))
	must(p.git(dir, "push", "origin", "HEAD"))
	p.reopen()
	if a, ar := p.versionRows("live-between"); a != 1 || ar != 0 {
		t.Errorf("a live document: %d active, %d archived", a, ar)
	}
	for _, id := range ids {
		if a, ar := p.versionRows(id); a != 0 || ar != 1 {
			t.Errorf("%s after being added back: %d active, %d archived", id, a, ar)
		}
	}
}

// ---- merge commits ---------------------------------------------------------------------

// A user with plain git merges and pushes. The server's own commits (here, the
// one made when the review was sent) are then the merge commit's SECOND parent,
// and a review sent before the push must still merge.
func TestDocArchiveReviewSentBeforeAMergeCommitPushStillMerges(t *testing.T) {
	p := newPushFx(t, "da21", "base-doc")
	d := p.withDoc("mg-live", "mg")
	d.save("first\n")
	d.approve()
	dir := p.clone() // before the review is sent
	d.save("reviewed\n")
	rid := d.sendAndApprove() // the server commits after the clone
	must := func(out string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	writeDoc(t, filepath.Join(dir, "documents", "test", "local-only.md"), "local-only", "local")
	must(p.git(dir, "add", "-A"))
	must(p.git(dir, "commit", "-q", "-m", "local work"))
	must(p.git(dir, "pull", "-q", "--no-rebase", "--no-edit", "origin", "main"))
	if out, _ := p.git(dir, "rev-list", "--min-parents=2", "-n", "1", "HEAD"); strings.TrimSpace(out) == "" {
		t.Fatal("no merge commit was made")
	}
	must(p.git(dir, "push", "origin", "HEAD"))
	p.reopen()
	if st := p.reviewStatus(rid); st != "approved" {
		t.Fatalf("the push changed the review to %s", st)
	}
	if r := p.doc("mg-live").mergeReviewID(rid); r.code != http.StatusOK {
		t.Fatalf("merge of a review sent before a merge-commit push = %d %s", r.code, r.body)
	}
	if a, ar := p.versionRows("mg-live"); a != 2 || ar != 0 {
		t.Errorf("versions: %d active, %d archived", a, ar)
	}
}

// A delete and re-create on a side branch, brought in by a merge-commit push: the
// push archives nothing (both ends have the id), and the content check, by id,
// refuses the merge.
func TestDocArchiveSideBranchDeleteAndRecreateIsRefused(t *testing.T) {
	p := newPushFx(t, "da22", "base-doc")
	d := p.withDoc("sb-doc", "sb")
	d.save("first\n")
	d.approve()
	d.save("reviewed\n")
	rid := d.sendAndApprove()
	dir := p.clone()
	must := func(out string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	must(p.git(dir, "checkout", "-q", "-b", "side"))
	must(p.git(dir, "rm", "-q", "documents/test/sb-doc.md"))
	must(p.git(dir, "commit", "-q", "-m", "side: delete"))
	writeDoc(t, filepath.Join(dir, "documents", "elsewhere", "sb-doc.md"), "sb-doc", "another incarnation")
	must(p.git(dir, "add", "-A"))
	must(p.git(dir, "commit", "-q", "-m", "side: create again"))
	must(p.git(dir, "checkout", "-q", "-"))
	writeDoc(t, filepath.Join(dir, "documents", "test", "main-work.md"), "main-work", "work")
	must(p.git(dir, "add", "-A"))
	must(p.git(dir, "commit", "-q", "-m", "work on main"))
	must(p.git(dir, "merge", "-q", "--no-ff", "-m", "merge side", "side"))
	must(p.git(dir, "push", "origin", "HEAD"))
	p.reopen()
	// The endpoints both have the id, so the push archived nothing and closed nothing.
	if st := p.reviewStatus(rid); st != "approved" {
		t.Fatalf("precondition: the review is %s", st)
	}
	r := p.doc("sb-doc").mergeReviewID(rid)
	if r.code != http.StatusConflict || !strings.Contains(r.body, "modified since") {
		t.Fatalf("merge across a side-branch deletion = %d %s", r.code, r.body)
	}
}

// setReviewStatusForTest puts a review in any status, scoped to its organization,
// through the conditional update (every status is allowed to start from).
func setReviewStatusForTest(ctx context.Context, d *db.DB, orgID, id int, status string) error {
	return d.UpdateReviewStatusFrom(ctx, orgID, id, status, []string{"open", "changes_requested", "approved", "merged", "closed"})
}

// ---- the checks, one at a time (round 6) ---------------------------------------------

func (f *tdFx) sentHeadOf(rid string) string {
	f.t.Helper()
	var h string
	rawQuery(f.t, `SELECT sent_head FROM reviews WHERE id = $1::int`, []any{rid}, &h)
	return h
}

// Check (a) alone: the document is deleted and created again with exactly the
// bytes it had when the review was sent, so the content check passes, and the
// review still cannot be merged because the id's versions were archived after it.
func TestDocArchiveArchivedVersionsAloneRefuseTheMerge(t *testing.T) {
	f := newTDFx(t, "da23", "only-a")
	f.save("first\n")
	f.approve()
	f.save("reviewed\n")
	rid := f.sendAndApprove()
	abs := f.st.FindDocumentByID("only-a")
	sent, err := f.st.ReadFileAtRef(f.sentHeadOf(rid), strings.TrimPrefix(abs, f.st.Root()+"/"))
	if err != nil {
		t.Fatal(err)
	}
	f.deleteDocument("only-a")
	if _, err := f.st.CommitFile(abs, sent, "T", "t@example.test", "created again with the same bytes"); err != nil {
		t.Fatal(err)
	}
	rawExec(t, `UPDATE reviews SET status = 'approved' WHERE id = $1::int`, rid) // a review that was not closed
	r := f.mergeReviewID(rid)
	if r.code != http.StatusConflict || !strings.Contains(r.body, "created again after the review was sent") {
		t.Fatalf("merge = %d %s", r.code, r.body)
	}
}

// A review with no record of the commit it was sent from reaches its own refusal.
func TestDocArchiveReviewWithNoBaseIsRefused(t *testing.T) {
	f := newTDFx(t, "da24", "no-base")
	f.save("text\n")
	rid := f.sendAndApprove()
	rawExec(t, `UPDATE reviews SET sent_head = '', commit_hash = '' WHERE id = $1::int`, rid)
	r := f.mergeReviewID(rid)
	if r.code != http.StatusConflict || !strings.Contains(r.body, "no record of the commit") {
		t.Fatalf("merge = %d %s", r.code, r.body)
	}
}

// The document is deleted and created again at a new path in ONE commit, pushed:
// nothing is archived, the old path reads "not found", and only locating the
// document by id at the review's base tells the files apart.
func TestDocArchiveDeleteAndRecreateAtANewPathInOneCommit(t *testing.T) {
	p := newPushFx(t, "da25", "base-doc")
	d := p.withDoc("one-commit", "oc")
	d.save("first\n")
	d.approve()
	d.save("reviewed\n")
	rid := d.sendAndApprove()
	dir := p.clone()
	must := func(out string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	must(p.git(dir, "rm", "-q", "documents/test/one-commit.md"))
	writeDoc(t, filepath.Join(dir, "documents", "elsewhere", "other-name.md"), "one-commit", "UNREVIEWED CONTENT")
	must(p.git(dir, "add", "-A"))
	must(p.git(dir, "commit", "-q", "-m", "delete and recreate elsewhere"))
	must(p.git(dir, "push", "origin", "HEAD"))
	p.reopen()
	if st := p.reviewStatus(rid); st != "approved" {
		t.Fatalf("precondition: %s", st)
	}
	r := p.doc("one-commit").mergeReviewID(rid)
	if r.code != http.StatusConflict || !strings.Contains(r.body, "modified since") {
		t.Fatalf("merge = %d %s", r.code, r.body)
	}
	doc, _ := p.st.LoadDocument(p.st.FindDocumentByID("one-commit"))
	if doc == nil || doc.Frontmatter.Status == "approved" {
		t.Errorf("the unreviewed document was approved: %+v", doc)
	}
}

// Moved by a push AND edited: refused, like any edit made after the review was
// sent (the file at the base, found by id, differs from the current one).
func TestDocArchiveMovedAndEditedIsRefused(t *testing.T) {
	p := newPushFx(t, "da26", "base-doc")
	d := p.withDoc("mv-edit", "me")
	d.save("moving\n")
	rid := d.sendAndApprove()
	dir := p.clone()
	must := func(out string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	must(p.git(dir, "mv", "documents/test/mv-edit.md", "documents/test/mv-edit-renamed.md"))
	must(p.git(dir, "commit", "-q", "-m", "rename"))
	raw, _ := os.ReadFile(filepath.Join(dir, "documents", "test", "mv-edit-renamed.md"))
	if err := os.WriteFile(filepath.Join(dir, "documents", "test", "mv-edit-renamed.md"), append(raw, []byte("\nAN EDIT\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	must(p.git(dir, "commit", "-qam", "edit"))
	must(p.git(dir, "push", "origin", "HEAD"))
	p.reopen()
	if r := p.doc("mv-edit").mergeReviewID(rid); r.code != http.StatusConflict || !strings.Contains(r.body, "changed after the review was sent") {
		t.Fatalf("merge of a moved and edited document = %d %s", r.code, r.body)
	}
}

// ---- the backfill ------------------------------------------------------------------------

func TestDocArchiveBackfill(t *testing.T) {
	f := newTDFx(t, "da27", "live-bf")
	f.save("live\n")
	f.approve()
	gone := f.withDoc("gone-bf", "gone")
	gone.save("first\n")
	gone.approve()
	gone.save("second\n")
	rid := gone.sendAndApprove()
	// Deleted before archiving existed: through the store, so nothing is archived.
	if _, err := f.st.DeleteFile(f.st.FindDocumentByID("gone-bf"), "T", "t@example.test", "delete (before the fix)"); err != nil {
		t.Fatal(err)
	}
	if a, ar := f.versionRows("gone-bf"); a != 1 || ar != 0 {
		t.Fatalf("precondition: %d active, %d archived", a, ar)
	}
	ctx := context.Background()
	n, err := f.s.backfillOrgVersions(ctx, f.o.id)
	if err != nil || n != 1 {
		t.Fatalf("backfill = %d, %v", n, err)
	}
	if a, ar := f.versionRows("gone-bf"); a != 0 || ar != 1 {
		t.Errorf("gone-bf after the backfill: %d active, %d archived", a, ar)
	}
	if st := f.reviewStatus(rid); st != "closed" {
		t.Errorf("the review is %s, want closed", st)
	}
	if a, ar := f.versionRows("live-bf"); a != 1 || ar != 0 {
		t.Errorf("a live document: %d active, %d archived", a, ar)
	}
	if done, _ := f.s.db.VersionBackfillDone(ctx, f.o.id); !done {
		t.Error("completion was not recorded")
	}
	// Once: a document deleted later is not touched by a second run.
	if _, err := f.st.DeleteFile(f.st.FindDocumentByID("live-bf"), "T", "t@example.test", "delete later"); err != nil {
		t.Fatal(err)
	}
	if n, err := f.s.backfillOrgVersions(ctx, f.o.id); err != nil || n != 0 {
		t.Errorf("second run = %d, %v", n, err)
	}
	if a, ar := f.versionRows("live-bf"); a != 1 || ar != 0 {
		t.Errorf("the second run archived: %d active, %d archived", a, ar)
	}
}

// A document created and approved after the repository was read is live.
func TestDocArchiveBackfillLeavesWhatWasCreatedMeanwhile(t *testing.T) {
	f := newTDFx(t, "da28", "base-doc")
	ctx := context.Background()
	readAt := time.Now()
	time.Sleep(20 * time.Millisecond)
	d := f.withDoc("late-doc", "late")
	d.save("x\n")
	d.approve()
	if err := f.s.archiveIDsBefore(ctx, f.o.id, f.st, []string{"late-doc"}, readAt, "test"); err != nil {
		t.Fatal(err)
	}
	if a, ar := f.versionRows("late-doc"); a != 1 || ar != 0 {
		t.Errorf("a document created after the read: %d active, %d archived", a, ar)
	}
}

// A read error skips the organization and records nothing.
func TestDocArchiveBackfillReadErrorMarksNothing(t *testing.T) {
	f := newTDFx(t, "da29", "live-be")
	f.save("x\n")
	f.approve()
	sym, _ := os.ReadFile(filepath.Join(f.st.Root(), "HEAD"))
	head := filepath.Join(f.st.Root(), strings.TrimSpace(strings.TrimPrefix(string(sym), "ref:")))
	good, err := os.ReadFile(head)
	if err != nil {
		t.Skip(err)
	}
	if err := os.WriteFile(head, []byte("0123456789012345678901234567890123456789\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.WriteFile(head, good, 0o644)
	ctx := context.Background()
	if n, err := f.s.backfillOrgVersions(ctx, f.o.id); err == nil {
		t.Errorf("a read error was not reported (%d)", n)
	}
	if done, _ := f.s.db.VersionBackfillDone(ctx, f.o.id); done {
		t.Error("a read error was recorded as done")
	}
	if a, ar := f.versionRows("live-be"); a != 1 || ar != 0 {
		t.Errorf("a read error archived: %d active, %d archived", a, ar)
	}
}

// The backfill compares versions with a time from the database clock taken
// before the repository is read; a version created after it is never archived,
// whatever the application clock says.
func TestDocArchiveBackfillUsesTheDatabaseClock(t *testing.T) {
	f := newTDFx(t, "da30", "live-bc")
	f.save("x\n")
	f.approve()
	// Deleted before archiving existed.
	if _, err := f.st.DeleteFile(f.st.FindDocumentByID("live-bc"), "T", "t@example.test", "delete"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dbNow, err := f.s.db.Now(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// A clock from before the version was created: it is newer than the read.
	f.s.backfillClock = func(context.Context) (time.Time, error) { return dbNow.Add(-time.Hour), nil }
	if n, err := f.s.backfillOrgVersions(ctx, f.o.id); err != nil || n != 0 {
		t.Fatalf("backfill with an early read time = %d, %v", n, err)
	}
	if a, ar := f.versionRows("live-bc"); a != 1 || ar != 0 {
		t.Errorf("a version created after the read time: %d active, %d archived", a, ar)
	}
	// That run recorded completion; clear this organization's marker and run again
	// with the real database clock.
	rawExec(t, `DELETE FROM document_version_backfill WHERE organization_id = $1`, f.o.id)
	f.s.backfillClock = nil
	if n, err := f.s.backfillOrgVersions(ctx, f.o.id); err != nil || n != 1 {
		t.Fatalf("backfill with the database clock = %d, %v", n, err)
	}
	if a, ar := f.versionRows("live-bc"); a != 0 || ar != 1 {
		t.Errorf("after the real run: %d active, %d archived", a, ar)
	}
}

// A repository with no commits next to recorded versions is skipped, not read as
// "everything was deleted".
func TestDocArchiveBackfillSkipsAnEmptyRepositoryWithVersions(t *testing.T) {
	f := newTDFx(t, "da31", "live-em")
	f.save("x\n")
	f.approve()
	// The organization's repository is replaced by one with no commits.
	dir := filepath.Join(t.TempDir(), "fresh.git")
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatal(err)
	}
	empty, err := store.NewBare(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.s.stores.Store(f.o.id, empty)
	ctx := context.Background()
	n, err := f.s.backfillOrgVersions(ctx, f.o.id)
	if err == nil || !strings.Contains(err.Error(), "no commits") || n != 0 {
		t.Fatalf("backfill = %d, %v; want a skip", n, err)
	}
	if done, _ := f.s.db.VersionBackfillDone(ctx, f.o.id); done {
		t.Error("the skipped organization was recorded as done")
	}
	if a, ar := f.versionRows("live-em"); a != 1 || ar != 0 {
		t.Errorf("a skipped organization: %d active, %d archived", a, ar)
	}
	// An empty repository with no versions is simply done.
	other := newTDFx(t, "da32", "x-doc")
	other.s.stores.Store(other.o.id, empty)
	if n, err := other.s.backfillOrgVersions(ctx, other.o.id); err != nil || n != 0 {
		t.Errorf("an empty repository with no versions = %d, %v", n, err)
	}
}
