package api

// Postgres-gated tests for the trust center manager endpoints (#520): settings,
// the item list and its order. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TrustMgr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

const tmSettings = "/api/v1/trust-center/settings"
const tmItems = "/api/v1/trust-center/items"
const tmOrder = "/api/v1/trust-center/items/order"

func (f *tdFx) mgrCall(h func(echo.Context) error, method, path, body string) tdResp {
	return f.call(h, method, path, body, "manager", f.mgr, "", "")
}

func (f *tdFx) getSettings() trustSettingsJSON {
	f.t.Helper()
	r := f.mustOK(f.mgrCall(f.s.handleGetTrustSettings, http.MethodGet, tmSettings, ""), "get settings")
	return decodeSettings(f.t, r)
}

func decodeSettings(t *testing.T, r tdResp) trustSettingsJSON {
	t.Helper()
	var s trustSettingsJSON
	if err := json.Unmarshal([]byte(r.body), &s); err != nil {
		t.Fatalf("decode settings %q: %v", r.body, err)
	}
	return s
}

func (f *tdFx) putSettings(body string) tdResp {
	return f.mgrCall(f.s.handlePutTrustSettings, http.MethodPut, tmSettings, body)
}

func (f *tdFx) mgrItems() []trustManagerItem {
	f.t.Helper()
	r := f.mustOK(f.mgrCall(f.s.handleListTrustItems, http.MethodGet, tmItems, ""), "list items")
	var items []trustManagerItem
	if err := json.Unmarshal([]byte(r.body), &items); err != nil {
		f.t.Fatalf("decode items %q: %v", r.body, err)
	}
	return items
}

// withDoc returns a fixture sharing this org and store whose document is docID,
// created in the repository.
func (f *tdFx) withDoc(docID, title string) *tdFx {
	f.t.Helper()
	g := *f
	g.docID = docID
	body := fmt.Sprintf(`{"folder":"test","filename":"%s.md","document_id":"%s","title":"%s"}`, docID, docID, title)
	f.mustOK(f.call(f.s.handleCreateDocument, http.MethodPost, "/api/v1/documents", body, "manager", f.mgr, "", ""), "create "+docID)
	return &g
}

func codeOf(r tdResp) string {
	if strings.Contains(r.body, `"code":"`) {
		i := strings.Index(r.body, `"code":"`) + len(`"code":"`)
		return r.body[i : i+strings.Index(r.body[i:], `"`)]
	}
	return ""
}

func TestTrustMgrSettingsGetDefaultsAndWritesNothing(t *testing.T) {
	f := newTDFx(t, "tms0", "pol-s0")
	o2 := trustNewOrg(t, f.s, "tms0b", false)
	f.o = o2 // an org that never configured its trust center
	s := f.getSettings()
	if s.Enabled || s.Title != "" || s.OverviewDocumentID != nil || s.SecurityContact != "" ||
		s.DefaultAccessLevel != "gated" || !s.UpdatedAt.IsZero() || s.UpdatedBy != nil {
		t.Errorf("defaults = %+v", s)
	}
	if r := f.mgrCall(f.s.handleGetTrustSettings, http.MethodGet, tmSettings, ""); !strings.Contains(r.body, `"updated_at":null`) || !strings.Contains(r.body, `"overview_document_id":null`) {
		t.Errorf("body %s", r.body)
	}
	// Reading created no row, and an empty PUT does not either.
	f.mustOK(f.putSettings(`{}`), "empty put")
	if s := f.getSettings(); !s.UpdatedAt.IsZero() {
		t.Errorf("updated_at set after reads and an empty PUT: %+v", s)
	}
	if cl := f.settingsChangelog(); len(cl) != 0 {
		t.Errorf("changelog = %+v", cl)
	}
}

func TestTrustMgrSettingsPartialUpdateAndChangelog(t *testing.T) {
	f := newTDFx(t, "tms1", "pol-s1")
	f.o = trustNewOrg(t, f.s, "tms1b", false)

	r := f.mustOK(f.putSettings(`{"enabled":true,"title":"Acme Trust","security_contact":"sec@acme.test"}`), "put")
	s := decodeSettings(t, r)
	if !s.Enabled || s.Title != "Acme Trust" || s.SecurityContact != "sec@acme.test" || s.DefaultAccessLevel != "gated" ||
		s.UpdatedAt.IsZero() || s.UpdatedBy == nil || *s.UpdatedBy != f.mgr {
		t.Fatalf("after put = %+v", s)
	}
	cl := f.settingsChangelog()
	got := map[string][2]string{}
	for _, e := range cl {
		if e.Action != "update" || e.ChangedBy != f.mgr || e.OldValue == nil || e.NewValue == nil {
			t.Errorf("entry %+v", e)
			continue
		}
		got[e.Field] = [2]string{*e.OldValue, *e.NewValue}
	}
	want := map[string][2]string{
		"enabled": {"false", "true"}, "title": {"", "Acme Trust"}, "security_contact": {"", "sec@acme.test"},
	}
	if len(cl) != 3 || fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("changelog = %v (%d rows), want %v", got, len(cl), want)
	}

	// Absent keys are unchanged; one field changes, one row is written.
	f.mustOK(f.putSettings(`{"default_access_level":"public"}`), "level")
	s = f.getSettings()
	if !s.Enabled || s.Title != "Acme Trust" || s.DefaultAccessLevel != "public" {
		t.Errorf("partial update clobbered others: %+v", s)
	}
	if n := len(f.settingsChangelog()); n != 4 {
		t.Errorf("changelog rows = %d, want 4", n)
	}

	// Writing the same values again is a no-op: no new row, updated_at kept.
	before := f.getSettings().UpdatedAt
	f.mustOK(f.putSettings(`{"enabled":true,"title":"Acme Trust","default_access_level":"public"}`), "same")
	if n := len(f.settingsChangelog()); n != 4 {
		t.Errorf("no-op PUT wrote changelog rows: %d", n)
	}
	if after := f.getSettings().UpdatedAt; !after.Equal(before.Time) {
		t.Errorf("no-op PUT moved updated_at %v -> %v", before, after)
	}
	// An empty string is a value for title (it clears it), and is logged.
	f.mustOK(f.putSettings(`{"title":""}`), "clear title")
	if f.getSettings().Title != "" || len(f.settingsChangelog()) != 5 {
		t.Error("empty title did not clear and log")
	}
}

func TestTrustMgrSettingsValidation(t *testing.T) {
	f := newTDFx(t, "tms2", "pol-s2")
	f.o = trustNewOrg(t, f.s, "tms2b", false)
	for name, body := range map[string]string{
		"level":   `{"default_access_level":"secret"}`,
		"title":   `{"title":"` + strings.Repeat("x", 201) + `"}`,
		"contact": `{"security_contact":"` + strings.Repeat("x", 321) + `"}`,
		"badjson": `{"enabled":"yes"}`,
	} {
		if r := f.putSettings(body); r.code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, r.code, r.body)
		}
	}
	// Nothing was written by a rejected request, even next to a valid key.
	if r := f.putSettings(`{"enabled":true,"default_access_level":"secret"}`); r.code != 400 {
		t.Errorf("mixed = %d", r.code)
	}
	if f.getSettings().Enabled || len(f.settingsChangelog()) != 0 {
		t.Error("a rejected PUT changed something")
	}
	if r := f.putSettings(`{"title":"` + strings.Repeat("x", 200) + `","security_contact":"` + strings.Repeat("y", 320) + `"}`); r.code != 200 {
		t.Errorf("boundary lengths = %d %s", r.code, r.body)
	}
}

func TestTrustMgrOverviewValidation(t *testing.T) {
	f := newTDFx(t, "tms3", "pol-s3")
	// A draft with no approved version is allowed; the id is stored lowercase.
	r := f.mustOK(f.putSettings(`{"overview_document_id":"POL-S3"}`), "draft overview")
	if s := decodeSettings(t, r); s.OverviewDocumentID == nil || *s.OverviewDocumentID != "pol-s3" {
		t.Fatalf("overview = %v", s.OverviewDocumentID)
	}
	cl := f.settingsChangelog()
	if len(cl) != 1 || cl[0].Field != "overview_document_id" || *cl[0].OldValue != "" || *cl[0].NewValue != "pol-s3" {
		t.Errorf("changelog = %+v", cl)
	}
	if r := f.putSettings(`{"overview_document_id":"pol-s3"}`); r.code != 200 || len(f.settingsChangelog()) != 1 {
		t.Errorf("same overview again = %d, changelog %d", r.code, len(f.settingsChangelog()))
	}
	// Unknown document: 404, setting unchanged.
	if r := f.putSettings(`{"overview_document_id":"no-such-doc"}`); r.code != http.StatusNotFound {
		t.Errorf("unknown = %d %s", r.code, r.body)
	}
	// null clears, and logs the clear with an empty new value.
	r = f.mustOK(f.putSettings(`{"overview_document_id":null}`), "null clears")
	if s := decodeSettings(t, r); s.OverviewDocumentID != nil {
		t.Errorf("null did not clear: %+v", s)
	}
	cl = f.settingsChangelog()
	if len(cl) != 2 || *cl[0].OldValue != "pol-s3" || *cl[0].NewValue != "" {
		t.Errorf("changelog = %+v", cl)
	}
	// Retired: 409 trust_document_retired. (Retiring also clears the setting.)
	f.mustOK(f.putSettings(`{"overview_document_id":"pol-s3"}`), "set again")
	f.mustOK(f.docCall(f.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"retired"}}`, "manager", f.mgr), "retire")
	if r := f.putSettings(`{"overview_document_id":"pol-s3"}`); r.code != http.StatusConflict || codeOf(r) != CodeTrustDocumentRetired {
		t.Errorf("retired = %d %s", r.code, r.body)
	}
	if got := f.overviewSetting(); got != nil {
		t.Errorf("retired document is still the overview: %v", *got)
	}
}

func TestTrustMgrOverviewEmptyStringClears(t *testing.T) {
	f := newTDFx(t, "tms4", "pol-s4")
	f.mustOK(f.putSettings(`{"overview_document_id":"pol-s4"}`), "set")
	f.mustOK(f.putSettings(`{"overview_document_id":""}`), "empty clears")
	if f.overviewSetting() != nil {
		t.Error("empty string did not clear")
	}
	n := len(f.settingsChangelog())
	f.mustOK(f.putSettings(`{"overview_document_id":null}`), "clear again")
	if len(f.settingsChangelog()) != n {
		t.Error("clearing an unset overview wrote a changelog row")
	}
}

func TestTrustMgrItemsListStatusesAndOrder(t *testing.T) {
	f := newTDFx(t, "tmi", "pol-pub")
	f.save("# Pub\n\nv1.\n")
	f.approve()
	pubID := f.publishOK("public")

	drf := f.withDoc("pol-drf", "Draft In Progress")
	drf.save("v1\n")
	drf.approve()
	drfID := drf.publishOK("gated")
	drf.save("v2 edit\n") // HEAD now a newer draft

	ret := f.withDoc("pol-ret", "Retired One")
	ret.save("x\n")
	ret.approve()
	ret.publishOK("gated")
	ret.mustOK(ret.docCall(ret.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"retired"}}`, "manager", ret.mgr), "retire")

	del := f.withDoc("pol-del", "Deleted One")
	del.save("x\n")
	del.approve()
	del.publishOK("gated")
	del.mustOK(del.docCall(del.s.handleDeleteDocument, http.MethodDelete, "", "manager", del.mgr), "delete")

	// Never approved: a row exists but there is no served version.
	nev := f.withDoc("pol-nev", "Never Approved")
	if err := f.s.db.PublishTrustCenterItem(context.Background(), f.o.id,
		&db.TrustCenterItem{Kind: db.TrustKindDocument, Reference: "pol-nev", AccessLevel: db.TrustAccessGated}, f.mgr); err != nil {
		t.Fatal(err)
	}
	_ = nev

	// A recorded version whose commit cannot be read: status error, list intact.
	f.withDoc("pol-bad", "Bad One")
	if err := f.s.db.RecordVersion(context.Background(), f.o.id, &db.DocumentVersion{DocumentID: "pol-bad", Version: "9.9",
		FilePath: "documents/test/pol-bad.md", CommitHash: "0000000000000000000000000000000000000000", CreatedBy: f.admin}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.db.PublishTrustCenterItem(context.Background(), f.o.id,
		&db.TrustCenterItem{Kind: db.TrustKindDocument, Reference: "pol-bad", AccessLevel: db.TrustAccessGated}, f.mgr); err != nil {
		t.Fatal(err)
	}

	items := f.mgrItems()
	if len(items) != 6 {
		t.Fatalf("items = %d, want 6: %+v", len(items), items)
	}
	by := map[string]trustManagerItem{}
	for i, it := range items {
		if it.Document == nil || it.Kind != "document" {
			t.Fatalf("item %d = %+v", i, it)
		}
		by[it.Document.DocumentID] = it
		if i > 0 && items[i-1].SortOrder > it.SortOrder {
			t.Errorf("not ordered by sort_order: %+v", items)
		}
	}
	// New publishes append: the sort order follows the publish order.
	var order []string
	for _, it := range items {
		order = append(order, it.Document.DocumentID)
	}
	if got := strings.Join(order, ","); got != "pol-pub,pol-drf,pol-ret,pol-del,pol-nev,pol-bad" {
		t.Errorf("order = %s", got)
	}
	check := func(id, status, title string) trustManagerItem {
		t.Helper()
		it, ok := by[id]
		if !ok {
			t.Fatalf("no item for %s", id)
		}
		if it.Document.Status != status {
			t.Errorf("%s status = %q, want %q (%+v)", id, it.Document.Status, status, it.Document)
		}
		if it.Document.Title != title {
			t.Errorf("%s title = %q, want %q", id, it.Document.Title, title)
		}
		return it
	}
	p := check("pol-pub", "published", "Test Doc")
	if p.ID != pubID || p.AccessLevel != "public" || p.HiddenReason != "" || p.PublishedAt == 0 ||
		p.Document.ServedVersion == nil || p.Document.ServedVersion.ContentHash == "" ||
		p.Document.DocumentStatus != "approved" || p.Document.DocumentVersion != p.Document.ServedVersion.Version {
		t.Errorf("published row = %+v / %+v", p, p.Document)
	}
	d := check("pol-drf", "draft_in_progress", "Draft In Progress")
	if d.ID != drfID || d.Document.DocumentStatus != "draft" || d.Document.ServedVersion == nil ||
		d.Document.DocumentVersion == d.Document.ServedVersion.Version {
		t.Errorf("draft row = %+v / %+v", d, d.Document)
	}
	r := check("pol-ret", "hidden", "Retired One")
	if r.HiddenReason != "retired" || r.Document.DocumentStatus != "retired" {
		t.Errorf("retired row = %+v / %+v", r, r.Document)
	}
	// Deleted: the HEAD title is gone, the served title is used.
	x := check("pol-del", "hidden", "Deleted One")
	if x.HiddenReason != "deleted" || x.Document.ServedVersion == nil {
		t.Errorf("deleted row = %+v / %+v", x, x.Document)
	}
	n := check("pol-nev", "cannot_publish", "Never Approved")
	if n.Document.ServedVersion != nil {
		t.Errorf("never approved served = %+v", n.Document.ServedVersion)
	}
	check("pol-bad", "error", "Bad One")
}

func TestTrustMgrItemsEmptyIsArrayAndClassification(t *testing.T) {
	f := newTDFx(t, "tmi2", "pol-cls")
	if r := f.mgrCall(f.s.handleListTrustItems, http.MethodGet, tmItems, ""); r.code != 200 || strings.TrimSpace(r.body) != "[]" {
		t.Fatalf("empty list = %d %q", r.code, r.body)
	}
	abs := f.st.FindDocumentByID(f.docID)
	raw := "---\ndocument_id: pol-cls\ntitle: Classified\nversion: \"1.0\"\nstatus: draft\nclassification: confidential\n---\nbody\n"
	if _, err := f.st.CommitFile(abs, []byte(raw), "Seed", "seed@example.com", "seed"); err != nil {
		t.Fatal(err)
	}
	f.approve()
	f.publishOK("gated")
	items := f.mgrItems()
	if len(items) != 1 || items[0].Document.Classification != "confidential" || items[0].Document.Title != "Classified" {
		t.Errorf("items = %+v", items)
	}
}

func TestTrustMgrOrder(t *testing.T) {
	f := newTDFx(t, "tmo", "pol-o1")
	ids := map[string]string{}
	for _, d := range []string{"pol-o1", "pol-o2", "pol-o3"} {
		g := f // the fixture's own document is pol-o1
		if d != "pol-o1" {
			g = f.withDoc(d, "Doc "+d)
		}
		g.save("x\n")
		g.approve()
		ids[d] = g.publishOK("public")
	}
	body := func(kind string, ids ...string) string {
		b, _ := json.Marshal(map[string]any{"kind": kind, "ids": ids})
		return string(b)
	}
	put := func(b string) tdResp { return f.mgrCall(f.s.handleReorderTrustItems, http.MethodPut, tmOrder, b) }
	list := func() string {
		var out []string
		for _, it := range f.mgrItems() {
			out = append(out, fmt.Sprintf("%s=%d", it.Document.DocumentID, it.SortOrder))
		}
		return strings.Join(out, ",")
	}
	if got := list(); got != "pol-o1=1,pol-o2=2,pol-o3=3" {
		t.Fatalf("initial = %s", got)
	}
	r := f.mustOK(put(body("document", ids["pol-o3"], ids["pol-o1"], ids["pol-o2"])), "reorder")
	var echoed []trustManagerItem
	if err := json.Unmarshal([]byte(r.body), &echoed); err != nil || len(echoed) != 3 || echoed[0].Document.DocumentID != "pol-o3" {
		t.Errorf("response must be the new list: %s", r.body)
	}
	if got := list(); got != "pol-o3=1,pol-o1=2,pol-o2=3" {
		t.Errorf("after reorder = %s", got)
	}
	// The public API follows.
	var pub []string
	for _, it := range trustItems(t, f.items()) {
		pub = append(pub, fmt.Sprint(it.SortOrder))
	}
	if strings.Join(pub, ",") != "1,2,3" {
		t.Errorf("public sort orders = %v", pub)
	}
	if n := len(f.settingsChangelog()); n != 0 {
		t.Errorf("reorder wrote settings changelog: %d", n)
	}

	stale := func(why, b string) {
		t.Helper()
		if r := put(b); r.code != http.StatusConflict || codeOf(r) != CodeTrustOrderStale {
			t.Errorf("%s = %d %s, want 409 trust_order_stale", why, r.code, r.body)
		}
	}
	stale("missing id", body("document", ids["pol-o1"], ids["pol-o2"]))
	stale("extra unknown id", body("document", ids["pol-o1"], ids["pol-o2"], ids["pol-o3"], "00000000-0000-0000-0000-000000000000"))
	stale("duplicate", body("document", ids["pol-o1"], ids["pol-o1"], ids["pol-o2"]))
	stale("duplicate full length", body("document", ids["pol-o1"], ids["pol-o1"], ids["pol-o2"], ids["pol-o3"]))
	stale("not a uuid", body("document", "x", "y", "z"))
	stale("empty", body("document"))
	// Another organization's item id.
	other := newTDFx(t, "tmo2", "pol-z")
	other.save("x\n")
	other.approve()
	otherID := other.publishOK("public")
	stale("other org id", body("document", ids["pol-o1"], ids["pol-o2"], otherID))
	if r := put(body("bogus", ids["pol-o1"])); r.code != http.StatusBadRequest {
		t.Errorf("unknown kind = %d %s", r.code, r.body)
	}
	if r := put(`{"kind":"document","ids":"nope"}`); r.code != http.StatusBadRequest {
		t.Errorf("bad json = %d", r.code)
	}
	if got := list(); got != "pol-o3=1,pol-o1=2,pol-o2=3" {
		t.Errorf("rejected requests changed the order: %s", got)
	}
	// Hidden items are part of the set: retire one, it must still be listed.
	f.mustOK(f.withDocCall("pol-o2", f.s.handleUpdateDocumentMetadata, `{"fields":{"status":"retired"}}`), "retire")
	f.mgrItems() // hides on read
	stale("hidden one omitted", body("document", ids["pol-o3"], ids["pol-o1"]))
	f.mustOK(put(body("document", ids["pol-o2"], ids["pol-o3"], ids["pol-o1"])), "with hidden")
	if got := list(); got != "pol-o2=1,pol-o3=2,pol-o1=3" {
		t.Errorf("hidden item order = %s", got)
	}
	// A new publish lands after the highest value, republish keeps its place.
	n := f.withDoc("pol-o4", "Doc 4")
	n.save("x\n")
	n.approve()
	n.publishOK("public")
	if got := list(); got != "pol-o2=1,pol-o3=2,pol-o1=3,pol-o4=4" {
		t.Errorf("append = %s", got)
	}
	f.mustOK(f.withDocCall("pol-o2", f.s.handleUpdateDocumentMetadata, `{"fields":{"status":"approved"}}`), "unretire")
	g := *f
	g.docID = "pol-o2"
	g.mustOK(g.publish("public", "manager", g.mgr), "republish")
	if got := list(); got != "pol-o2=1,pol-o3=2,pol-o1=3,pol-o4=4" {
		t.Errorf("republish moved the item: %s", got)
	}
}

func (f *tdFx) withDocCall(docID string, h func(echo.Context) error, body string) tdResp {
	return f.call(h, http.MethodPut, "/api/v1/documents/"+docID, body, "manager", f.mgr, "docId", docID)
}

func TestTrustMgrEndpointRolesAndAuth(t *testing.T) {
	f := newTDFx(t, "tmr", "pol-r1")
	type ep struct {
		name, method, path, body string
		h                        func(echo.Context) error
	}
	eps := []ep{
		{"GET settings", http.MethodGet, tmSettings, "", f.s.handleGetTrustSettings},
		{"PUT settings", http.MethodPut, tmSettings, `{"enabled":true}`, f.s.handlePutTrustSettings},
		{"GET items", http.MethodGet, tmItems, "", f.s.handleListTrustItems},
		{"PUT order", http.MethodPut, tmOrder, `{"kind":"document","ids":[]}`, f.s.handleReorderTrustItems},
	}
	for _, e := range eps {
		for _, who := range []struct{ role, email string }{{"reader", f.read}, {"contributor", f.contr}} {
			if r := f.call(e.h, e.method, e.path, e.body, who.role, who.email, "", ""); r.code != http.StatusForbidden {
				t.Errorf("%s as %s = %d %s, want 403", e.name, who.role, r.code, r.body)
			}
		}
		for _, who := range []struct{ role, email string }{{"manager", f.mgr}, {"admin", f.admin}} {
			if r := f.call(e.h, e.method, e.path, e.body, who.role, who.email, "", ""); r.code != http.StatusOK {
				t.Errorf("%s as %s = %d %s, want 200", e.name, who.role, r.code, r.body)
			}
		}
		// Through the real router, with no token: the /trust-center prefix is
		// authenticated (only /trust and /trust/* are anonymous).
		req := newTMReq(e.method, e.path, e.body)
		rec := serveTM(f.s, req)
		if rec != http.StatusUnauthorized {
			t.Errorf("%s with no token = %d, want 401", e.name, rec)
		}
	}
}

func newTMReq(method, path, body string) *http.Request {
	req, _ := http.NewRequest(method, "http://"+trustTestBase+path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return req
}

func serveTM(s *Server, req *http.Request) int {
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	return rec.Code
}
