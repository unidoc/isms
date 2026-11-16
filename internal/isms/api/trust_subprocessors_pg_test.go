package api

// Postgres-gated tests for the trust center subprocessor list (#522): publish
// from the Suppliers register, the hide rules, the read-time check, the public
// projection and "last changed". Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TrustSubprocessor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

const (
	spBase  = "/api/v1/trust-center/subprocessors"
	spNotes = "SECRET-NOTES-0xC0FFEE"
	spCRef  = "CONTRACT-REF-7731"
)

type spFx struct {
	t   *testing.T
	s   *Server
	o   trustOrg
	mgr string
}

func newSPFx(t *testing.T, tag string) *spFx {
	t.Helper()
	s := trustServer(t)
	o := trustNewOrg(t, s, tag, true)
	f := &spFx{t: t, s: s, o: o, mgr: "mgr@" + tag + ".test"}
	seedReviewUser(t, s, o.id, f.mgr, "manager")
	return f
}

func (f *spFx) call(h func(echo.Context) error, method, path, body string, params ...string) tdResp {
	f.t.Helper()
	c, rec := reviewCtxForPath(f.o.id, method, path, body, "manager", f.mgr)
	if len(params) > 0 {
		var names, vals []string
		for i := 0; i+1 < len(params); i += 2 {
			names, vals = append(names, params[i]), append(vals, params[i+1])
		}
		c.SetParamNames(names...)
		c.SetParamValues(vals...)
	}
	if err := h(c); err != nil {
		he, ok := err.(*echo.HTTPError)
		if !ok {
			f.t.Fatalf("handler error: %v", err)
		}
		b, _ := json.Marshal(he.Message)
		return tdResp{he.Code, string(b)}
	}
	return tdResp{rec.Code, rec.Body.String()}
}

func (f *spFx) supplier(name string, mod func(*db.Supplier)) *db.Supplier {
	f.t.Helper()
	sp := &db.Supplier{Name: name, SupplierType: "cloud", Criticality: "low", Status: "active", DataAccess: true,
		Notes: spNotes, ContractRef: spCRef, Contact: "secret-contact@example.com"}
	if mod != nil {
		mod(sp)
	}
	if err := f.s.db.CreateSupplier(context.Background(), f.o.id, sp); err != nil {
		f.t.Fatalf("CreateSupplier %s: %v", name, err)
	}
	return sp
}

func (f *spFx) publish(id int64) tdResp {
	return f.call(f.s.handlePublishTrustSubprocessor, http.MethodPost, spBase, fmt.Sprintf(`{"supplier_id":%d}`, id))
}

func (f *spFx) publishOK(id int64) {
	f.t.Helper()
	if r := f.publish(id); r.code != http.StatusCreated {
		f.t.Fatalf("publish %d: %d %s", id, r.code, r.body)
	}
}

func (f *spFx) put(id int64, body string) {
	f.t.Helper()
	r := f.call(f.s.handleUpdateSupplier, http.MethodPut, fmt.Sprintf("/api/v1/suppliers/%d", id), body, "id", fmt.Sprint(id))
	if r.code != http.StatusOK {
		f.t.Fatalf("PUT supplier %d %s: %d %s", id, body, r.code, r.body)
	}
}

func (f *spFx) publicBody() string {
	f.t.Helper()
	r := trustDo(f.s, trustTestBase, http.MethodGet, "/api/v1/trust/"+f.o.slug+"/items", "")
	if r.code != http.StatusOK {
		f.t.Fatalf("public items: %d %s", r.code, r.body)
	}
	return r.body
}

func (f *spFx) publicRows() []map[string]any {
	f.t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal([]byte(f.publicBody()), &rows); err != nil {
		f.t.Fatal(err)
	}
	return rows
}

func (f *spFx) publicNames() []string {
	var names []string
	for _, r := range f.publicRows() {
		if r["kind"] == "subprocessor" {
			names = append(names, r["name"].(string))
		}
	}
	return names
}

// stored reads the item straight from the database, with none of the read-time
// checks, so a test sees what a writer left behind.
func (f *spFx) stored(id int64) *db.TrustCenterItem {
	f.t.Helper()
	it, err := f.s.db.GetTrustCenterItemByReference(context.Background(), f.o.id, db.TrustKindSubprocessor, fmt.Sprint(id))
	if err != nil {
		f.t.Fatalf("stored item for supplier %d: %v", id, err)
	}
	return it
}

func (f *spFx) changed() *time.Time {
	f.t.Helper()
	ts, err := f.s.db.TrustSubprocessorsChangedAt(context.Background(), f.o.id)
	if err != nil {
		f.t.Fatal(err)
	}
	return ts
}

func (f *spFx) managerList() (changed *int64, rows map[string]trustSubprocessorJSON, order []string) {
	f.t.Helper()
	r := f.call(f.s.handleListTrustSubprocessors, http.MethodGet, spBase, "")
	if r.code != http.StatusOK {
		f.t.Fatalf("manager list: %d %s", r.code, r.body)
	}
	var out struct {
		ChangedAt *int64                  `json:"changed_at"`
		Suppliers []trustSubprocessorJSON `json:"suppliers"`
	}
	if err := json.Unmarshal([]byte(r.body), &out); err != nil {
		f.t.Fatal(err)
	}
	rows = map[string]trustSubprocessorJSON{}
	for _, sp := range out.Suppliers {
		rows[sp.Name] = sp
		order = append(order, sp.Name)
	}
	return out.ChangedAt, rows, order
}

func spSortedKeys(m map[string]any) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	sort.Strings(k)
	return k
}

// a, b, d: exactly nine keys, no leak, the name fallback, every non-terminated
// status listed.
func TestTrustSubprocessorPublicProjection(t *testing.T) {
	f := newSPFx(t, "sp-proj")
	plain := f.supplier("Plain Cloud Ltd", nil)
	named := f.supplier("Internal Name", func(s *db.Supplier) {
		s.PublicName = "  Legal Entity GmbH "
		s.PublicPurpose, s.ProcessingLocation, s.DataCategories = "Cloud hosting", "EU (Frankfurt)", "Name, email address"
	})
	suspended := f.supplier("Suspended Co", func(s *db.Supplier) { s.Status = "suspended" })
	review := f.supplier("Review Co", func(s *db.Supplier) { s.Status = "under_review" })
	for _, s := range []*db.Supplier{plain, named, suspended, review} {
		f.publishOK(s.ID)
	}
	body := f.publicBody()
	for _, bad := range []string{spNotes, spCRef, "secret-contact", plain.Identifier, named.Identifier, "supplier_id", "Internal Name"} {
		if strings.Contains(body, bad) {
			t.Errorf("public items leak %q: %s", bad, body)
		}
	}
	want := []string{"access_level", "data_categories", "id", "kind", "location", "name", "purpose", "section", "sort_order"}
	var byName = map[string]map[string]any{}
	for _, r := range f.publicRows() {
		if got := spSortedKeys(r); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("public row keys = %v, want %v", got, want)
		}
		if r["access_level"] != "public" || r["kind"] != "subprocessor" {
			t.Errorf("row = %v", r)
		}
		byName[r["name"].(string)] = r
	}
	// b: fallback and override, trimmed.
	if byName["Plain Cloud Ltd"] == nil {
		t.Errorf("empty public_name should fall back to the supplier name: %v", f.publicNames())
	}
	if r := byName["Legal Entity GmbH"]; r == nil || r["purpose"] != "Cloud hosting" || r["location"] != "EU (Frankfurt)" || r["data_categories"] != "Name, email address" {
		t.Errorf("set public_name row = %v", byName)
	}
	if p := byName["Plain Cloud Ltd"]; p != nil && (p["purpose"] != "" || p["location"] != "" || p["data_categories"] != "") {
		t.Errorf("empty texts must be empty strings, got %v", p)
	}
	// d: suspended and under_review are still listed.
	if byName["Suspended Co"] == nil || byName["Review Co"] == nil {
		t.Errorf("suspended and under_review suppliers must be listed: %v", f.publicNames())
	}
}

// c, plus the manager 404/400 paths.
func TestTrustSubprocessorPublishRefusals(t *testing.T) {
	f := newSPFx(t, "sp-refuse")
	other := newSPFx(t, "sp-refuse-other")
	terminated := f.supplier("Terminated Co", func(s *db.Supplier) { s.Status = "terminated" })
	noAccess := f.supplier("No Access Co", func(s *db.Supplier) { s.DataAccess = false })
	both := f.supplier("Both Co", func(s *db.Supplier) { s.Status = "terminated"; s.DataAccess = false })
	ok := f.supplier("Fine Co", nil)
	foreign := other.supplier("Foreign Co", nil)
	deleted := f.supplier("Deleted Co", nil)
	if err := f.s.db.DeleteSupplier(context.Background(), f.o.id, deleted.ID); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		id     int64
		status int
		code   string
		value  string
	}{
		{"terminated", terminated.ID, 409, CodeTrustSubprocessorIneligible, "terminated"},
		{"no data access", noAccess.ID, 409, CodeTrustSubprocessorIneligible, "no_data_access"},
		{"terminated wins", both.ID, 409, CodeTrustSubprocessorIneligible, "terminated"},
		{"other org", foreign.ID, 404, CodeNotFound, ""},
		{"deleted", deleted.ID, 404, CodeNotFound, ""},
		{"unknown", 99999999, 404, CodeNotFound, ""},
	} {
		r := f.publish(tc.id)
		if r.code != tc.status || codeOf(r) != tc.code || (tc.value != "" && !strings.Contains(r.body, `"value":"`+tc.value+`"`)) {
			t.Errorf("%s: got %d %s, want %d %s value %q", tc.name, r.code, r.body, tc.status, tc.code, tc.value)
		}
	}
	// Non-public level, missing id.
	r := f.call(f.s.handlePublishTrustSubprocessor, http.MethodPost, spBase, fmt.Sprintf(`{"supplier_id":%d,"access_level":"gated"}`, ok.ID))
	if r.code != 400 || codeOf(r) != CodeInvalidRequest {
		t.Errorf("gated level: %d %s", r.code, r.body)
	}
	if r := f.call(f.s.handlePublishTrustSubprocessor, http.MethodPost, spBase, `{}`); r.code != 400 {
		t.Errorf("no supplier_id: %d %s", r.code, r.body)
	}
	// "public" is accepted; twice is a 409.
	if r := f.call(f.s.handlePublishTrustSubprocessor, http.MethodPost, spBase, fmt.Sprintf(`{"supplier_id":%d,"access_level":"public"}`, ok.ID)); r.code != 201 {
		t.Fatalf("publish: %d %s", r.code, r.body)
	}
	if r := f.publish(ok.ID); r.code != 409 || codeOf(r) != CodeTrustSubprocessorAlreadyPublished {
		t.Errorf("twice: %d %s", r.code, r.body)
	}
	// Role check.
	c, _ := reviewCtxForPath(f.o.id, http.MethodPost, spBase, fmt.Sprintf(`{"supplier_id":%d}`, ok.ID), "contributor", f.mgr)
	if err := f.s.handlePublishTrustSubprocessor(c); err == nil {
		t.Error("a contributor must not publish")
	}
	// Nothing leaked by the refusals.
	if got := f.publicNames(); len(got) != 1 || got[0] != "Fine Co" {
		t.Errorf("public names = %v", got)
	}
}

// e: each writer hides at once with the right reason; restoring keeps it hidden
// (D3); republishing brings back the SAME public id.
func TestTrustSubprocessorHideOnWriters(t *testing.T) {
	f := newSPFx(t, "sp-hide")
	cases := []struct {
		name   string
		reason string
		do     func(sp *db.Supplier)
		undo   func(sp *db.Supplier)
	}{
		{"terminate via PUT", db.TrustHiddenTerminated,
			func(sp *db.Supplier) { f.put(sp.ID, `{"status":"terminated"}`) },
			func(sp *db.Supplier) { f.put(sp.ID, `{"status":"active"}`) }},
		{"data_access off via suggestion apply", db.TrustHiddenNoDataAccess,
			func(sp *db.Supplier) {
				sg := newTestSuggestion(t, f.s, f.o.id, &db.Suggestion{
					EntityType: "supplier", EntityID: sp.Identifier, SuggestionType: "update",
					Payload: json.RawMessage(`{"fields":{"data_access":false}}`)})
				applySuggestion(t, f.s, f.o.id, sg.ID)
			},
			func(sp *db.Supplier) { f.put(sp.ID, `{"data_access":true}`) }},
		{"delete", db.TrustHiddenDeleted,
			func(sp *db.Supplier) {
				r := f.call(f.s.handleDeleteSupplier, http.MethodDelete, fmt.Sprintf("/api/v1/suppliers/%d", sp.ID), "", "id", fmt.Sprint(sp.ID))
				if r.code != 200 {
					t.Fatalf("delete: %d %s", r.code, r.body)
				}
			},
			nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sp := f.supplier("Writer "+tc.name, nil)
			f.publishOK(sp.ID)
			pubID := f.stored(sp.ID).PublicID
			if got := f.publicNames(); !spContains(got, sp.Name) {
				t.Fatalf("precondition: %v", got)
			}
			tc.do(sp)
			// The stored state, before any read path could hide it.
			it := f.stored(sp.ID)
			if it.HiddenAt == nil || it.HiddenReason != tc.reason {
				t.Fatalf("stored hidden = %v reason %q, want hidden with %q", it.HiddenAt, it.HiddenReason, tc.reason)
			}
			if got := f.publicNames(); spContains(got, sp.Name) {
				t.Fatalf("still public after %s: %v", tc.name, got)
			}
			if tc.undo == nil {
				return
			}
			tc.undo(sp)
			if it := f.stored(sp.ID); it.HiddenAt == nil || it.HiddenReason != tc.reason {
				t.Fatalf("restoring must not republish: hidden = %v %q", it.HiddenAt, it.HiddenReason)
			}
			if got := f.publicNames(); spContains(got, sp.Name) {
				t.Fatalf("restored supplier reappeared by itself: %v", got)
			}
			f.publishOK(sp.ID)
			if it := f.stored(sp.ID); it.HiddenAt != nil || it.PublicID != pubID {
				t.Fatalf("republish: hidden %v, public id %q want %q", it.HiddenAt, it.PublicID, pubID)
			}
			if got := f.publicNames(); !spContains(got, sp.Name) {
				t.Fatalf("republished supplier not listed: %v", got)
			}
		})
	}
}

func spContains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// f: an item left visible for an ineligible supplier by a writer that bypassed
// the hook is skipped, then hidden.
func TestTrustSubprocessorReadTimeCheck(t *testing.T) {
	f := newSPFx(t, "sp-read")
	term := f.supplier("Bypassed Terminated", func(s *db.Supplier) { s.Status = "terminated" })
	noAcc := f.supplier("Bypassed No Access", func(s *db.Supplier) { s.DataAccess = false })
	gone := f.supplier("Bypassed Deleted", nil)
	live := f.supplier("Live Co", nil)
	f.publishOK(live.ID)
	for _, sp := range []*db.Supplier{term, noAcc, gone} {
		trustPublishItem(t, f.s, f.o, db.TrustKindSubprocessor, fmt.Sprint(sp.ID), db.TrustAccessPublic)
	}
	// Soft-delete with SQL, as a writer that skipped DeleteSupplier would.
	if _, err := f.s.db.Pool().Exec(context.Background(), `UPDATE suppliers SET deleted_at = now() WHERE id = $1`, gone.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.publicNames(); len(got) != 1 || got[0] != "Live Co" {
		t.Fatalf("public names = %v, want only Live Co", got)
	}
	for sp, reason := range map[*db.Supplier]string{term: "terminated", noAcc: "no_data_access", gone: "deleted"} {
		if it := f.stored(sp.ID); it.HiddenAt == nil || it.HiddenReason != reason {
			t.Errorf("%s: stored hidden %v reason %q, want %q", sp.Name, it.HiddenAt, it.HiddenReason, reason)
		}
	}
}

// g: what moves "last changed" and what does not.
func TestTrustSubprocessorLastChanged(t *testing.T) {
	f := newSPFx(t, "sp-changed")
	a := f.supplier("Alpha Ltd", nil)
	b := f.supplier("Beta Ltd", nil)
	if ts := f.changed(); ts != nil {
		t.Fatalf("nothing published yet, changed = %v", ts)
	}
	last := time.Time{}
	step := func(label string, wantMove bool, do func()) {
		t.Helper()
		time.Sleep(25 * time.Millisecond)
		do()
		ts := f.changed()
		if ts == nil {
			t.Fatalf("%s: changed is nil", label)
		}
		if moved := ts.After(last); moved != wantMove {
			t.Fatalf("%s: moved = %v, want %v (before %v, after %v)", label, moved, wantMove, last, *ts)
		}
		last = *ts
	}
	step("publish A", true, func() { f.publishOK(a.ID) })
	step("notes edit of a published supplier", false, func() { f.put(a.ID, `{"notes":"changed notes"}`) })
	step("criticality edit", false, func() { f.put(a.ID, `{"criticality":"high"}`) })
	step("public edit of an UNPUBLISHED supplier", false, func() { f.put(b.ID, `{"public_purpose":"Payments"}`) })
	step("public field edit of a published supplier", true, func() { f.put(a.ID, `{"public_purpose":"Hosting"}`) })
	step("name edit, public_name empty", true, func() { f.put(a.ID, `{"name":"Alpha Renamed Ltd"}`) })
	step("publish B", true, func() { f.publishOK(b.ID) })
	step("reorder that changes the order", true, func() { f.reorder(b, a) })
	step("reorder with the same order", false, func() { f.reorder(b, a) })
	step("hide by status", true, func() { f.put(a.ID, `{"status":"terminated"}`) })
	step("unpublish", true, func() {
		r := f.call(f.s.handleUnpublishTrustSubprocessor, http.MethodDelete, spBase+"/"+fmt.Sprint(b.ID), "", "supplierId", fmt.Sprint(b.ID))
		if r.code != 204 {
			t.Fatalf("unpublish: %d %s", r.code, r.body)
		}
	})
	// The unpublish stamp left the other settings alone.
	st, err := f.s.db.GetTrustCenterSettings(context.Background(), f.o.id)
	if err != nil || !st.Enabled || st.Title != "Trust sp-changed" || st.DefaultAccessLevel != db.TrustAccessGated {
		t.Errorf("settings after unpublish: %+v %v", st, err)
	}
	// Not published: 404.
	r := f.call(f.s.handleUnpublishTrustSubprocessor, http.MethodDelete, spBase+"/"+fmt.Sprint(b.ID), "", "supplierId", fmt.Sprint(b.ID))
	if r.code != 404 {
		t.Errorf("unpublish twice: %d %s", r.code, r.body)
	}
}

func (f *spFx) reorder(first, second *db.Supplier) {
	f.t.Helper()
	ids := []string{f.stored(first.ID).PublicID, f.stored(second.ID).PublicID}
	body, _ := json.Marshal(map[string]any{"kind": "subprocessor", "ids": ids})
	if r := f.call(f.s.handleReorderTrustItems, http.MethodPut, tmOrder, string(body)); r.code != 200 {
		f.t.Fatalf("reorder: %d %s", r.code, r.body)
	}
}

// h: subprocessors are always public.
func TestTrustSubprocessorPatchGuard(t *testing.T) {
	f := newSPFx(t, "sp-patch")
	sp := f.supplier("Patch Co", nil)
	f.publishOK(sp.ID)
	pub := f.stored(sp.ID).PublicID
	for _, lvl := range []string{"gated", "restricted", "bogus"} {
		r := f.call(f.s.handlePatchTrustItem, http.MethodPatch, tmItems+"/"+pub, fmt.Sprintf(`{"access_level":%q}`, lvl), "id", pub)
		if r.code != 400 || codeOf(r) != CodeInvalidRequest {
			t.Errorf("PATCH %s: %d %s", lvl, r.code, r.body)
		}
	}
	if it := f.stored(sp.ID); it.AccessLevel != db.TrustAccessPublic {
		t.Errorf("level = %s", it.AccessLevel)
	}
	r := f.call(f.s.handlePatchTrustItem, http.MethodPatch, tmItems+"/"+pub, `{"access_level":"public"}`, "id", pub)
	if r.code != 200 || !strings.Contains(r.body, `"subprocessor":{`) {
		t.Errorf("PATCH public: %d %s", r.code, r.body)
	}
}

// i: preview equals public for subprocessors; overview carries the stamp.
func TestTrustSubprocessorPreviewAndOverview(t *testing.T) {
	f := newSPFx(t, "sp-preview")
	overview := func() map[string]any {
		r := trustDo(f.s, trustTestBase, http.MethodGet, "/api/v1/trust/"+f.o.slug, "")
		var m map[string]any
		_ = json.Unmarshal([]byte(r.body), &m)
		return m
	}
	if _, ok := overview()["subprocessors_changed_at"]; ok {
		t.Error("overview has subprocessors_changed_at before anything was published")
	}
	sp := f.supplier("Preview Co", func(s *db.Supplier) { s.PublicPurpose = "Email delivery" })
	f.publishOK(sp.ID)
	if v, ok := overview()["subprocessors_changed_at"].(float64); !ok || v <= 0 {
		t.Errorf("overview subprocessors_changed_at = %v", overview()["subprocessors_changed_at"])
	}
	r := f.call(f.s.handleTrustPreviewOverview, http.MethodGet, "/api/v1/trust-center/preview", "")
	var pm map[string]any
	_ = json.Unmarshal([]byte(r.body), &pm)
	if v, ok := pm["subprocessors_changed_at"].(float64); !ok || v <= 0 {
		t.Errorf("preview overview subprocessors_changed_at = %v (%s)", pm["subprocessors_changed_at"], r.body)
	}
	pr := f.call(f.s.handleTrustPreviewItems, http.MethodGet, "/api/v1/trust-center/preview/items", "")
	if pr.code != 200 {
		t.Fatalf("preview items: %d %s", pr.code, pr.body)
	}
	var pub, prev []map[string]any
	_ = json.Unmarshal([]byte(f.publicBody()), &pub)
	_ = json.Unmarshal([]byte(pr.body), &prev)
	pj, _ := json.Marshal(pub)
	vj, _ := json.Marshal(prev)
	if string(pj) != string(vj) || len(pub) != 1 || pub[0]["purpose"] != "Email delivery" {
		t.Errorf("preview %s != public %s", vj, pj)
	}
}

// j: the manager list.
func TestTrustSubprocessorManagerList(t *testing.T) {
	f := newSPFx(t, "sp-mgr")
	active := f.supplier("A Active", nil)
	termd := f.supplier("B Terminated", nil)
	f.supplier("C NoAccess Unpublished", func(s *db.Supplier) { s.DataAccess = false })
	delWith := f.supplier("D Deleted", nil)
	f.supplier("E Candidate", nil)
	f.publishOK(termd.ID) // published first, so it sorts first
	f.publishOK(active.ID)
	f.publishOK(delWith.ID)
	f.put(termd.ID, `{"status":"terminated"}`)
	if err := f.s.db.DeleteSupplier(context.Background(), f.o.id, delWith.ID); err != nil {
		t.Fatal(err)
	}
	other := newSPFx(t, "sp-mgr-other")
	other.supplier("Foreign", nil)

	changed, rows, order := f.managerList()
	if changed == nil || *changed <= 0 {
		t.Errorf("changed_at = %v", changed)
	}
	if _, ok := rows["C NoAccess Unpublished"]; ok {
		t.Error("an unpublished supplier without data access must not be listed")
	}
	if _, ok := rows["Foreign"]; ok {
		t.Error("another organization's supplier is listed")
	}
	if got := strings.Join(order, "|"); got != "B Terminated|A Active|D Deleted|E Candidate" {
		t.Errorf("order = %s", got)
	}
	a := rows["A Active"]
	if !a.Eligible || a.IneligibleReason != "" || a.Item == nil || a.Item.HiddenReason != "" || a.Item.ID == "" {
		t.Errorf("active published: %+v", a)
	}
	b := rows["B Terminated"]
	if b.Eligible || b.IneligibleReason != "terminated" || b.Item == nil || b.Item.HiddenReason != "terminated" {
		t.Errorf("terminated published: %+v", b)
	}
	d := rows["D Deleted"]
	if d.Eligible || !d.Deleted || d.IneligibleReason != "deleted" || d.Item == nil || d.Item.HiddenReason != "deleted" {
		t.Errorf("deleted with item: %+v", d)
	}
	e := rows["E Candidate"]
	if !e.Eligible || e.Item != nil {
		t.Errorf("candidate: %+v", e)
	}

	// The items list carries the subprocessor object, with the name fallback.
	r := f.call(f.s.handleListTrustItems, http.MethodGet, tmItems, "")
	var items []trustManagerItem
	if err := json.Unmarshal([]byte(r.body), &items); err != nil || len(items) != 3 {
		t.Fatalf("items: %d %s %v", r.code, r.body, err)
	}
	for _, it := range items {
		if it.Subprocessor == nil || it.Kind != "subprocessor" {
			t.Fatalf("row %+v lacks the subprocessor object", it)
		}
		if it.Subprocessor.Name == "A Active" && (it.Subprocessor.PublicName != "A Active" || !it.Subprocessor.DetailsMissing) {
			t.Errorf("A Active: %+v", it.Subprocessor)
		}
	}
}
