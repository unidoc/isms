package api

// Postgres-gated tests for publishing documents to the trust center (#518).
// Versions, merges, confirms, retires and deletes are all driven through the
// real handlers, so document_versions rows come from production code. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TrustDoc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

type tdFx struct {
	t     *testing.T
	s     *Server
	o     trustOrg
	st    *store.Store
	docID string
	admin string
	mgr   string
	contr string
	read  string
	rev   string
}

func newTDFx(t *testing.T, tag, docID string) *tdFx {
	t.Helper()
	s := trustServer(t)
	o := trustNewOrg(t, s, tag, true)
	seedRepoForOrg(t, s, o.id, docID)
	f := &tdFx{t: t, s: s, o: o, docID: docID,
		admin: "admin@" + tag + ".test", mgr: "mgr@" + tag + ".test", contr: "contr@" + tag + ".test",
		read: "read@" + tag + ".test", rev: "rev@" + tag + ".test"}
	seedReviewUser(t, s, o.id, f.admin, "admin")
	seedReviewUser(t, s, o.id, f.mgr, "manager")
	seedReviewUser(t, s, o.id, f.contr, "contributor")
	seedReviewUser(t, s, o.id, f.read, "reader")
	seedReviewUser(t, s, o.id, f.rev, "contributor")
	st, err := s.storeForOrg(context.Background(), o.id)
	if err != nil {
		t.Fatal(err)
	}
	f.st = st
	return f
}

type tdResp struct {
	code int
	body string
}

// call runs a handler directly, flattening an *echo.HTTPError into a response.
func (f *tdFx) call(h func(echo.Context) error, method, path, body, role, email string, pn, pv string) tdResp {
	f.t.Helper()
	c, rec := reviewCtxForPath(f.o.id, method, path, body, role, email)
	if pn != "" {
		c.SetParamNames(pn)
		c.SetParamValues(pv)
	}
	if err := h(c); err != nil {
		var he *echo.HTTPError
		if !errors.As(err, &he) {
			f.t.Fatalf("handler error: %v", err)
		}
		b, _ := json.Marshal(he.Message)
		return tdResp{he.Code, string(b)}
	}
	return tdResp{rec.Code, rec.Body.String()}
}

func (f *tdFx) mustOK(r tdResp, what string) tdResp {
	f.t.Helper()
	if r.code < 200 || r.code > 299 {
		f.t.Fatalf("%s: %d %s", what, r.code, r.body)
	}
	return r
}

func (f *tdFx) docCall(h func(echo.Context) error, method, body, role, email string) tdResp {
	return f.call(h, method, "/api/v1/documents/"+f.docID, body, role, email, "docId", f.docID)
}

func (f *tdFx) save(content string) {
	f.t.Helper()
	b, _ := json.Marshal(map[string]string{"content": content})
	f.mustOK(f.docCall(f.s.handleUpdateDocumentContent, http.MethodPut, string(b), "admin", f.admin), "save")
}

// approve sends the document for review, approves and merges it.
func (f *tdFx) approve() {
	f.t.Helper()
	b, _ := json.Marshal(map[string]any{"reviewers": []string{f.rev}})
	r := f.mustOK(f.docCall(f.s.handleReviewSend, http.MethodPost, string(b), "admin", f.admin), "send")
	var resp struct {
		ReviewID int `json:"review_id"`
	}
	if err := json.Unmarshal([]byte(r.body), &resp); err != nil || resp.ReviewID == 0 {
		f.t.Fatalf("send response %s: %v", r.body, err)
	}
	id := strconv.Itoa(resp.ReviewID)
	f.mustOK(f.call(f.s.handleReviewApprove, http.MethodPost, "/api/v1/reviews/"+id+"/approve",
		`{"decision":"approved","comment":"ok"}`, "contributor", f.rev, "id", id), "approve")
	f.mustOK(f.call(f.s.handleMergeReview, http.MethodPost, "/api/v1/reviews/"+id+"/merge", `{}`, "admin", f.admin, "id", id), "merge")
}

func (f *tdFx) publish(level string, role, email string) tdResp {
	body := "{}"
	if level != "" {
		body = `{"access_level":"` + level + `"}`
	}
	return f.docCall(f.s.handlePublishDocumentTrust, http.MethodPost, body, role, email)
}

func (f *tdFx) publishOK(level string) string {
	f.t.Helper()
	r := f.publish(level, "manager", f.mgr)
	if r.code != http.StatusCreated && r.code != http.StatusOK {
		f.t.Fatalf("publish: %d %s", r.code, r.body)
	}
	return decodeState(f.t, r).ID
}

func decodeState(t *testing.T, r tdResp) trustDocumentState {
	t.Helper()
	var st trustDocumentState
	if err := json.Unmarshal([]byte(r.body), &st); err != nil {
		t.Fatalf("decode state %q: %v", r.body, err)
	}
	return st
}

func (f *tdFx) state() trustDocumentState {
	f.t.Helper()
	return decodeState(f.t, f.mustOK(f.docCall(f.s.handleGetDocumentTrust, http.MethodGet, "", "manager", f.mgr), "state"))
}

func (f *tdFx) public(id string) trustResp {
	return trustDo(f.s, trustTestBase, http.MethodGet, "/api/v1/trust/"+f.o.slug+"/documents/"+id, "")
}

func (f *tdFx) items() trustResp {
	return trustDo(f.s, trustTestBase, http.MethodGet, "/api/v1/trust/"+f.o.slug+"/items", "")
}

func (f *tdFx) item() *db.TrustCenterItem {
	f.t.Helper()
	it, err := f.s.db.GetTrustCenterItemByReference(context.Background(), f.o.id, db.TrustKindDocument, f.docID)
	if err != nil {
		f.t.Fatalf("item row: %v", err)
	}
	return it
}

type tdPublicDoc struct {
	ID, Title, Version, ContentHash, HTML string
}

func (f *tdFx) publicDoc(id string) tdPublicDoc {
	f.t.Helper()
	r := f.public(id)
	if r.code != http.StatusOK {
		f.t.Fatalf("public doc: %d %s", r.code, r.body)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(r.body), &raw); err != nil {
		f.t.Fatal(err)
	}
	if len(raw) != 6 {
		f.t.Errorf("public document must have exactly id,title,version,approved_at,content_hash,html: %s", r.body)
	}
	for _, k := range []string{"id", "title", "version", "approved_at", "content_hash", "html"} {
		if _, ok := raw[k]; !ok {
			f.t.Errorf("public document lacks %q: %s", k, r.body)
		}
	}
	var d tdPublicDoc
	d.ID, _ = raw["id"].(string)
	d.Title, _ = raw["title"].(string)
	d.Version, _ = raw["version"].(string)
	d.ContentHash, _ = raw["content_hash"].(string)
	d.HTML, _ = raw["html"].(string)
	return d
}

func (f *tdFx) assertNotFound(r trustResp, why string) {
	f.t.Helper()
	if r.code != http.StatusNotFound || r.body != trustDo(f.s, trustTestBase, http.MethodGet, "/api/v1/trust/nope-"+f.o.slug, "").body {
		f.t.Fatalf("%s: want the one 404 body, got %d %s", why, r.code, r.body)
	}
}

func (f *tdFx) listedIDs() []string {
	f.t.Helper()
	var ids []string
	for _, it := range trustItems(f.t, f.items()) {
		ids = append(ids, it.ID)
	}
	return ids
}

// a. The approved text stays served while HEAD holds a newer draft.
func TestTrustDocServesLastApprovedNotHead(t *testing.T) {
	f := newTDFx(t, "tda", "pol-a")
	f.save("# Policy\n\nApproved text v1.\n")
	f.approve()
	id := f.publishOK("public")
	v1 := f.publicDoc(id)
	if !strings.Contains(v1.HTML, "Approved text v1.") || v1.Title != "Test Doc" || v1.ContentHash == "" {
		t.Fatalf("v1 = %+v", v1)
	}

	f.save("# Policy\n\nSECRET DRAFT TEXT.\n") // approved -> new draft version
	st := f.state()
	if st.DocumentStatus != "draft" || st.ServedVersion == nil || st.ServedVersion.Version != v1.Version {
		t.Fatalf("state = %+v, want served %s while HEAD is draft", st, v1.Version)
	}
	if st.DocumentVersion == v1.Version {
		t.Errorf("HEAD version %s should differ from served %s", st.DocumentVersion, v1.Version)
	}
	now := f.publicDoc(id)
	if strings.Contains(now.HTML, "SECRET DRAFT") || !strings.Contains(now.HTML, "Approved text v1.") || now.ContentHash != v1.ContentHash || now.Version != v1.Version {
		t.Errorf("public changed after a draft edit: %+v", now)
	}
}

// b. A document with no approved version cannot be published.
func TestTrustDocPublishNeverApprovedIs409(t *testing.T) {
	f := newTDFx(t, "tdb", "pol-b")
	r := f.publish("public", "manager", f.mgr)
	if r.code != http.StatusConflict || !strings.Contains(r.body, CodeTrustNoApprovedVersion) {
		t.Fatalf("publish = %d %s", r.code, r.body)
	}
	st := f.state()
	if st.Published || st.CanPublish || st.CannotPublishReason != "no_approved_version" || st.ServedVersion != nil {
		t.Errorf("state = %+v", st)
	}
	if _, err := f.s.db.GetTrustCenterItemByReference(context.Background(), f.o.id, db.TrustKindDocument, f.docID); err == nil {
		t.Error("a refused publish must leave no item")
	}
	if r := f.docCall(f.s.handleGetDocumentTrust, http.MethodGet, "", "manager", f.mgr); r.code != 200 {
		t.Fatal(r)
	}
	missing := f.call(f.s.handleGetDocumentTrust, http.MethodGet, "/x", "", "manager", f.mgr, "docId", "no-such-doc")
	if missing.code != http.StatusNotFound {
		t.Errorf("unknown document = %d, want 404", missing.code)
	}
}

// c. Merging the next version changes what visitors get, with no republish.
func TestTrustDocMergeNewVersionServedWithoutRepublish(t *testing.T) {
	f := newTDFx(t, "tdc", "pol-c")
	f.save("v1 text\n")
	f.approve()
	id := f.publishOK("public")
	v1 := f.publicDoc(id)
	f.save("v2 text\n")
	f.approve()
	v2 := f.publicDoc(id)
	if !strings.Contains(v2.HTML, "v2 text") || v2.Version == v1.Version || v2.ContentHash == v1.ContentHash {
		t.Fatalf("v1 %+v v2 %+v", v1, v2)
	}
	if got := f.state(); got.ServedVersion.Version != v2.Version || got.DocumentStatus != "approved" {
		t.Errorf("state = %+v", got)
	}
	if r := f.publish("public", "manager", f.mgr); r.code != http.StatusConflict || !strings.Contains(r.body, CodeTrustAlreadyPublished) {
		t.Errorf("second publish = %d %s", r.code, r.body)
	}
}

// d. Retired and deleted documents leave the trust center and stay out.
func TestTrustDocRetireHidesUntilPublishedAgain(t *testing.T) {
	f := newTDFx(t, "tdd", "pol-d")
	f.save("text\n")
	f.approve()
	id := f.publishOK("public")
	f.publicDoc(id)

	f.mustOK(f.docCall(f.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"retired"}}`, "manager", f.mgr), "retire")
	f.assertNotFound(f.public(id), "retired")
	if len(f.listedIDs()) != 0 {
		t.Errorf("retired document still listed: %s", f.items().body)
	}
	if st := f.state(); st.Published || st.HiddenReason != "retired" || st.CanPublish || st.CannotPublishReason != "retired" {
		t.Errorf("state after retire = %+v", st)
	}
	if r := f.publish("public", "manager", f.mgr); r.code != http.StatusConflict || !strings.Contains(r.body, CodeTrustDocumentRetired) {
		t.Errorf("publish while retired = %d %s", r.code, r.body)
	}

	f.mustOK(f.docCall(f.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"approved"}}`, "manager", f.mgr), "un-retire")
	f.assertNotFound(f.public(id), "un-retired but not republished")
	if len(f.listedIDs()) != 0 {
		t.Error("un-retired document came back in the list")
	}
	r := f.publish("public", "manager", f.mgr)
	if r.code != http.StatusOK {
		t.Fatalf("republish = %d %s", r.code, r.body)
	}
	if got := decodeState(t, r); got.ID != id || !got.Published || got.HiddenReason != "" {
		t.Errorf("republish state = %+v", got)
	}
	if d := f.publicDoc(id); !strings.Contains(d.HTML, "text") {
		t.Errorf("after republish: %+v", d)
	}
}

func TestTrustDocDeleteHidesAndRecreateDoesNotResurrect(t *testing.T) {
	f := newTDFx(t, "tdx", "pol-x")
	f.save("text\n")
	f.approve()
	id := f.publishOK("public")
	f.publicDoc(id)

	f.mustOK(f.docCall(f.s.handleDeleteDocument, http.MethodDelete, "", "manager", f.mgr), "delete")
	f.assertNotFound(f.public(id), "deleted")
	if len(f.listedIDs()) != 0 {
		t.Error("deleted document still listed")
	}
	if it := f.item(); it.HiddenAt == nil || it.HiddenReason != "deleted" {
		t.Errorf("row after delete = %+v", it)
	}

	body := `{"folder":"test","filename":"pol-x.md","document_id":"pol-x","title":"Reborn"}`
	f.mustOK(f.call(f.s.handleCreateDocument, http.MethodPost, "/api/v1/documents", body, "manager", f.mgr, "", ""), "recreate")
	f.assertNotFound(f.public(id), "re-created with the same id")
	if st := f.state(); st.Published || st.HiddenReason != "deleted" {
		t.Errorf("state after recreate = %+v", st)
	}
}

// The read path notices a deletion that did not go through the handler, which
// also proves the document index is not serving a stale answer.
func TestTrustDocReadPathDetectsDeleteBehindTheHandlersBack(t *testing.T) {
	f := newTDFx(t, "tdr", "pol-r")
	f.save("text\n")
	f.approve()
	id := f.publishOK("public")
	f.publicDoc(id) // warms the document index

	abs := f.st.FindDocumentByID(f.docID)
	if abs == "" {
		t.Fatal("document not found before delete")
	}
	if _, err := f.st.DeleteFile(abs, "x", "x@example.com", "docs: delete"); err != nil {
		t.Fatal(err)
	}
	if it := f.item(); it.HiddenAt != nil {
		t.Fatal("precondition: nothing hid the item yet")
	}
	f.assertNotFound(f.public(id), "deleted at HEAD")
	if it := f.item(); it.HiddenAt == nil || it.HiddenReason != "deleted" {
		t.Errorf("read path did not hide the item: %+v", it)
	}
	cl, _ := f.s.db.ListEntityChangelog(context.Background(), f.o.id, "trust_center_item", f.item().ID)
	if len(cl) < 2 || cl[0].Field != "hidden" || cl[0].ChangedBy != "system" {
		t.Errorf("changelog = %+v", cl)
	}
}

// e. Frontmatter identities never reach visitors.
func TestTrustDocNoIdentityLeak(t *testing.T) {
	f := newTDFx(t, "tde", "pol-e")
	abs := f.st.FindDocumentByID(f.docID)
	raw := "---\ndocument_id: pol-e\ntitle: Leak Check\nversion: \"1.0\"\nstatus: draft\n" +
		"author: Zelda Zephyr <zelda.author@leak.test>\nowner: owen.owner@leak.test\nreviewer: Rita Reviewer rita.rev@leak.test\n" +
		"approved_by: Aldo Approver aldo.appr@leak.test\nchangelog:\n  - version: \"0.9\"\n    date: 2025-01-01\n    author: Cleo Changelog cleo.chg@leak.test\n    approved_by: Boris Boss boris.boss@leak.test\n    description: first\n---\n# Heading\n\nBody without names.\n"
	if _, err := f.st.CommitFile(abs, []byte(raw), "Seed", "seed@example.com", "seed leak fixture"); err != nil {
		t.Fatal(err)
	}
	f.approve()
	id := f.publishOK("public")
	r := f.public(id)
	items := f.items()
	if r.code != 200 {
		t.Fatalf("public = %d %s", r.code, r.body)
	}
	email := regexp.MustCompile(`[\w.+-]+@[\w-]+\.`)
	for name, body := range map[string]string{"document": r.body, "items": items.body} {
		for _, bad := range []string{"Zelda", "Zephyr", "owen", "Rita", "Aldo", "Cleo", "Boris", "leak.test", "author", "owner", "reviewer", "approved_by", "changelog", f.admin, f.rev, f.mgr} {
			if strings.Contains(body, bad) {
				t.Errorf("%s response contains %q: %s", name, bad, body)
			}
		}
		if email.MatchString(body) {
			t.Errorf("%s response contains an email address: %s", name, body)
		}
	}
	if d := f.publicDoc(id); d.Title != "Leak Check" || !strings.Contains(d.HTML, "Body without names.") || strings.Contains(d.HTML, "document_id") {
		t.Errorf("doc = %+v", d)
	}
}

// f. Internal links, end to end.
func TestTrustDocInternalLinksEndToEnd(t *testing.T) {
	f := newTDFx(t, "tdf", "pol-f")
	// A second document, published, to link to; and one that is not.
	other := &tdFx{t: t, s: f.s, o: f.o, st: f.st, docID: "pol-pub", admin: f.admin, mgr: f.mgr, rev: f.rev}
	body := `{"folder":"test","filename":"pol-pub.md","document_id":"pol-pub","title":"Published Other"}`
	f.mustOK(f.call(f.s.handleCreateDocument, http.MethodPost, "/api/v1/documents", body, "manager", f.mgr, "", ""), "create other")
	other.approve()
	otherID := other.publishOK("gated")
	f.save("See [unpublished](/documents/pol-secret), [published](/documents/pol-pub) and [risk](/risks/RISK-12).\n")
	f.approve()
	id := f.publishOK("public")
	d := f.publicDoc(id)
	want := `<a data-trust-document="` + otherID + `">published</a>`
	if !strings.Contains(d.HTML, want) {
		t.Errorf("html lacks %s:\n%s", want, d.HTML)
	}
	for _, bad := range []string{"pol-secret", "RISK-12", "/documents/", "/risks/", "href"} {
		if strings.Contains(d.HTML, bad) {
			t.Errorf("html contains %q:\n%s", bad, d.HTML)
		}
	}
	if !strings.Contains(d.HTML, "unpublished") || !strings.Contains(d.HTML, "risk") {
		t.Errorf("link text lost:\n%s", d.HTML)
	}
}

// g. Only admins and managers use the manager endpoints.
func TestTrustDocManagerEndpointRoles(t *testing.T) {
	f := newTDFx(t, "tdg", "pol-g")
	f.save("text\n")
	f.approve()
	for _, who := range []struct{ role, email string }{{"reader", f.read}, {"contributor", f.contr}} {
		for name, h := range map[string]struct {
			fn     func(echo.Context) error
			method string
		}{
			"GET": {f.s.handleGetDocumentTrust, http.MethodGet}, "POST": {f.s.handlePublishDocumentTrust, http.MethodPost},
			"DELETE": {f.s.handleUnpublishDocumentTrust, http.MethodDelete},
		} {
			if r := f.docCall(h.fn, h.method, "{}", who.role, who.email); r.code != http.StatusForbidden {
				t.Errorf("%s %s = %d %s, want 403", who.role, name, r.code, r.body)
			}
		}
	}
	if _, err := f.s.db.GetTrustCenterItemByReference(context.Background(), f.o.id, db.TrustKindDocument, f.docID); err == nil {
		t.Fatal("a forbidden publish created an item")
	}
	for _, who := range []struct{ role, email string }{{"manager", f.mgr}, {"admin", f.admin}} {
		f.mustOK(f.docCall(f.s.handleGetDocumentTrust, http.MethodGet, "", who.role, who.email), who.role+" GET")
		r := f.publish("public", who.role, who.email)
		if r.code != http.StatusCreated {
			t.Fatalf("%s POST = %d %s", who.role, r.code, r.body)
		}
		if r := f.docCall(f.s.handleUnpublishDocumentTrust, http.MethodDelete, "", who.role, who.email); r.code != http.StatusNoContent {
			t.Errorf("%s DELETE = %d %s", who.role, r.code, r.body)
		}
		if r := f.docCall(f.s.handleUnpublishDocumentTrust, http.MethodDelete, "", who.role, who.email); r.code != http.StatusNotFound {
			t.Errorf("%s second DELETE = %d, want 404", who.role, r.code)
		}
	}
}

// h. Confirming a document that was edited back to a draft records a version
// row for the draft; visitors still get the approved text.
func TestTrustDocConfirmOnDraftDoesNotServeDraft(t *testing.T) {
	f := newTDFx(t, "tdh", "pol-h")
	f.save("approved v1 text\n")
	f.approve()
	id := f.publishOK("public")
	v1 := f.publicDoc(id)
	f.save("UNAPPROVED DRAFT\n")
	f.mustOK(f.docCall(f.s.handleConfirmDocumentReview, http.MethodPost, `{"comment":"annual"}`, "admin", f.admin), "confirm")

	latest, err := f.s.db.LatestVersion(context.Background(), f.o.id, f.docID)
	if err != nil {
		t.Fatal(err)
	}
	head, _ := f.st.HeadHash()
	if latest.CommitHash != head || latest.Version == v1.Version {
		t.Fatalf("precondition: confirm should have recorded the draft as the newest version, got %+v (head %s)", latest, head)
	}
	d := f.publicDoc(id)
	if strings.Contains(d.HTML, "UNAPPROVED") || !strings.Contains(d.HTML, "approved v1 text") || d.ContentHash != v1.ContentHash || d.Version != v1.Version {
		t.Errorf("public = %+v, want v1 %+v", d, v1)
	}
	if st := f.state(); st.ServedVersion.Version != v1.Version || st.DocumentStatus != "draft" {
		t.Errorf("state = %+v", st)
	}
}

// i. Isolation between organizations, and gated content.
func TestTrustDocCrossOrgAndGated(t *testing.T) {
	a := newTDFx(t, "tdia", "pol-i")
	b := newTDFx(t, "tdib", "pol-i")
	for _, f := range []*tdFx{a, b} {
		f.save("text of " + f.o.slug + "\n")
		f.approve()
	}
	idA := a.publishOK("public")
	idB := b.publishOK("public")
	a.publicDoc(idA)
	// Org A's slug with org B's item id is the same 404 as an unknown id.
	a.assertNotFound(a.public(idB), "other org's id")
	a.assertNotFound(a.public("00000000-0000-0000-0000-000000000000"), "unknown id")
	a.assertNotFound(a.public("not-a-uuid"), "malformed id")
	if strings.Contains(a.items().body, idB) {
		t.Error("org A's list contains org B's item")
	}

	// Gated: no content, but listed with its title.
	f := newTDFx(t, "tdig", "pol-ig")
	f.save("gated text\n")
	f.approve()
	id := f.publishOK("gated")
	f.assertNotFound(f.public(id), "gated")
	var found bool
	for _, it := range trustItems(t, f.items()) {
		if it.ID == id {
			found = true
		}
	}
	if !found || !strings.Contains(f.items().body, `"title":"Test Doc"`) || !strings.Contains(f.items().body, `"access_level":"gated"`) {
		t.Errorf("gated item must be listed with its title: %s", f.items().body)
	}
	if strings.Contains(f.items().body, "gated text") {
		t.Error("list leaks content")
	}
}

// j. Retiring through the metadata handler hides the item immediately.
func TestTrustDocRetireHandlerHidesImmediately(t *testing.T) {
	f := newTDFx(t, "tdj", "pol-j")
	f.save("text\n")
	f.approve()
	f.publishOK("public")
	if f.item().HiddenAt != nil {
		t.Fatal("precondition")
	}
	f.mustOK(f.docCall(f.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"retired"}}`, "manager", f.mgr), "retire")
	it := f.item() // read straight from the row: no public read, no state call
	if it.HiddenAt == nil || it.HiddenReason != "retired" {
		t.Errorf("row = %+v, want hidden/retired", it)
	}
	cl, _ := f.s.db.ListEntityChangelog(context.Background(), f.o.id, "trust_center_item", it.ID)
	if len(cl) < 2 || cl[0].Field != "hidden" || cl[0].ChangedBy != f.mgr {
		t.Errorf("changelog = %+v", cl)
	}
}

func TestTrustDocPublishDefaultsAndValidation(t *testing.T) {
	f := newTDFx(t, "tdk", "pol-k")
	f.save("text\n")
	f.approve()
	if r := f.publish("secret", "manager", f.mgr); r.code != http.StatusBadRequest {
		t.Errorf("bad level = %d %s", r.code, r.body)
	}
	r := f.publish("", "manager", f.mgr)
	if r.code != http.StatusCreated {
		t.Fatalf("publish = %d %s", r.code, r.body)
	}
	if st := decodeState(t, r); st.AccessLevel != db.TrustAccessGated || !st.Published || st.ServedVersion == nil || st.ServedVersion.ApprovedAt == 0 {
		t.Errorf("state = %+v (settings default is gated)", st)
	}
	if it := f.item(); it.Reference != "pol-k" || it.Kind != db.TrustKindDocument {
		t.Errorf("item = %+v", it)
	}
}

var _ = httptest.NewRecorder

// One document whose recorded version cannot be read must not take the whole
// trust page down: the list and the other documents keep working, and the
// broken one is simply not served (and not hidden: an error is never a hide).
func TestTrustDocUnreadableDocumentDoesNotBreakOthers(t *testing.T) {
	f := newTDFx(t, "tdl", "pol-good")
	f.save("good text\n")
	f.approve()
	goodID := f.publishOK("public")

	body := `{"folder":"test","filename":"pol-bad.md","document_id":"pol-bad","title":"Bad"}`
	f.mustOK(f.call(f.s.handleCreateDocument, http.MethodPost, "/api/v1/documents", body, "manager", f.mgr, "", ""), "create bad")
	ctx := context.Background()
	v := &db.DocumentVersion{DocumentID: "pol-bad", Version: "9.9", FilePath: "documents/test/pol-bad.md",
		CommitHash: "0000000000000000000000000000000000000000", CreatedBy: f.admin}
	if err := f.s.db.RecordVersion(ctx, f.o.id, v); err != nil {
		t.Fatal(err)
	}
	bad := &db.TrustCenterItem{Kind: db.TrustKindDocument, Reference: "pol-bad", AccessLevel: db.TrustAccessPublic}
	if err := f.s.db.PublishTrustCenterItem(ctx, f.o.id, bad, f.mgr); err != nil {
		t.Fatal(err)
	}

	r := f.items()
	ids := f.listedIDs()
	if r.code != http.StatusOK || len(ids) != 1 || ids[0] != goodID {
		t.Fatalf("items = %d %s, want 200 with only the good document", r.code, r.body)
	}
	if d := f.publicDoc(goodID); !strings.Contains(d.HTML, "good text") {
		t.Errorf("good document = %+v", d)
	}
	f.assertNotFound(f.public(bad.PublicID), "unreadable document")
	got, err := f.s.db.GetTrustCenterItemByReference(ctx, f.o.id, db.TrustKindDocument, "pol-bad")
	if err != nil || got.HiddenAt != nil {
		t.Errorf("an error must not hide the item: %+v %v", got, err)
	}
}
