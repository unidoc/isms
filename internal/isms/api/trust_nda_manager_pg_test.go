package api

// The manager side of the click-through NDA (#525): the NDA fields of the
// trust center settings and the NDA status of each visitor in the list. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run NDAMgr

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type mgrNDA struct {
	Status          string `json:"status"`
	Version         string `json:"version"`
	ContentHash     string `json:"content_hash"`
	AcceptedAt      *int64 `json:"accepted_at"`
	AcceptedVersion string `json:"accepted_version"`
}

type mgrNDAVisitor struct {
	Email string  `json:"email"`
	NDA   *mgrNDA `json:"nda"`
}

func (g *ndaFx) mgrVisitorNDA(t *testing.T) map[string]mgrNDA {
	t.Helper()
	r := g.mustOK(g.mgrCall(g.s.handleListTrustVisitors, http.MethodGet, "/api/v1/trust-center/visitors", ""), "list visitors")
	var rows []mgrNDAVisitor
	if err := json.Unmarshal([]byte(r.body), &rows); err != nil {
		t.Fatal(err)
	}
	out := map[string]mgrNDA{}
	for _, v := range rows {
		if v.NDA == nil {
			t.Fatalf("visitor %s has no nda: %s", v.Email, r.body)
		}
		out[v.Email] = *v.NDA
	}
	return out
}

func (g *ndaFx) ndaChangelog(t *testing.T, field string) []string {
	t.Helper()
	var out []string
	for _, e := range g.settingsChangelog() {
		if e.Field == field {
			o, n := "", ""
			if e.OldValue != nil {
				o = *e.OldValue
			}
			if e.NewValue != nil {
				n = *e.NewValue
			}
			out = append(out, o+" -> "+n)
		}
	}
	return out
}

func TestTrustNDAMgrSettings(t *testing.T) {
	g := newNDAFx(t, "ndam-s")
	s := g.getSettings()
	if s.NDADocumentID != nil || s.NDASkipDomains == nil || len(s.NDASkipDomains) != 0 || s.NDAStatus != "none" {
		t.Fatalf("defaults = %+v", s)
	}
	if r := g.mgrCall(g.s.handleGetTrustSettings, http.MethodGet, tmSettings, ""); !jsonHas(r.body, `"nda_skip_domains":[]`) || !jsonHas(r.body, `"nda_document_id":null`) {
		t.Errorf("default body = %s", r.body)
	}

	// An unknown document is a 404, a deleted one too, a retired one a 409.
	if r := g.putSettings(`{"nda_document_id":"no-such-doc"}`); r.code != http.StatusNotFound {
		t.Errorf("unknown document = %d %s", r.code, r.body)
	}
	retired := g.withDoc("nda-m-retired", "Retired")
	retired.save("# R\n\ntext\n")
	retired.approve()
	retired.mustOK(retired.docCall(retired.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"retired"}}`, "manager", retired.mgr), "retire")
	if r := g.putSettings(`{"nda_document_id":"nda-m-retired"}`); r.code != http.StatusConflict || codeOf(r) != CodeTrustDocumentRetired {
		t.Errorf("retired document = %d %s", r.code, r.body)
	}
	gone := g.withDoc("nda-m-gone", "Gone")
	gone.mustOK(gone.docCall(gone.s.handleDeleteDocument, http.MethodDelete, "", "manager", gone.mgr), "delete")
	if r := g.putSettings(`{"nda_document_id":"nda-m-gone"}`); r.code != http.StatusNotFound {
		t.Errorf("deleted document = %d %s", r.code, r.body)
	}
	if s := g.getSettings(); s.NDADocumentID != nil {
		t.Errorf("a refused PUT changed the setting: %+v", s)
	}

	// A draft that was never approved is allowed and reads unavailable.
	draft := g.withDoc("nda-m-draft", "Draft")
	draft.save("# D\n\ntext\n")
	r := g.mustOK(g.putSettings(`{"nda_document_id":"NDA-M-Draft"}`), "set draft")
	s = decodeSettings(t, r)
	if s.NDADocumentID == nil || *s.NDADocumentID != "nda-m-draft" || s.NDAStatus != "unavailable" || s.NDAUnavailableReason != "not_approved" || s.NDAVersion != "" {
		t.Errorf("draft NDA = %+v", s)
	}
	// An approved one reads approved with its version, even if also a published item.
	draft.approve()
	pub := draft.publishOK("public")
	if pub == "" {
		t.Fatal("publish")
	}
	s = g.getSettings()
	if s.NDAStatus != "approved" || s.NDAVersion == "" || s.NDAApprovedAt == 0 || s.NDAUnavailableReason != "" {
		t.Errorf("approved NDA = %+v", s)
	}
	s = decodeSettings(t, g.mustOK(g.putSettings(`{"nda_document_id":"`+g.nda.docID+`"}`), "set approved"))
	if s.NDAStatus != "approved" || s.NDAVersion == "" {
		t.Errorf("approved NDA = %+v", s)
	}

	// Retiring or deleting the NDA document never clears the setting.
	g.nda.mustOK(g.nda.docCall(g.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"retired"}}`, "manager", g.mgr), "retire")
	if s := g.getSettings(); s.NDADocumentID == nil || s.NDAStatus != "unavailable" || s.NDAUnavailableReason != "retired" {
		t.Errorf("after retiring: %+v", s)
	}
	g.nda.mustOK(g.nda.docCall(g.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"approved"}}`, "manager", g.mgr), "un-retire")
	g.nda.mustOK(g.nda.docCall(g.s.handleDeleteDocument, http.MethodDelete, "", "manager", g.mgr), "delete")
	if s := g.getSettings(); s.NDADocumentID == nil || s.NDAStatus != "unavailable" || s.NDAUnavailableReason != "deleted" {
		t.Errorf("after deleting: %+v", s)
	}

	// Clearing: null and "" both clear; an absent key leaves it alone.
	g.mustOK(g.putSettings(`{"title":"x"}`), "unrelated PUT")
	if s := g.getSettings(); s.NDADocumentID == nil {
		t.Errorf("a PUT without the key cleared the NDA")
	}
	for _, body := range []string{`{"nda_document_id":null}`, `{"nda_document_id":""}`} {
		g.mustOK(g.putSettings(`{"nda_document_id":"`+draft.docID+`"}`), "set")
		s = decodeSettings(t, g.mustOK(g.putSettings(body), "clear "+body))
		if s.NDADocumentID != nil || s.NDAStatus != "none" {
			t.Errorf("%s: %+v", body, s)
		}
	}
	if got := g.ndaChangelog(t, "nda_document_id"); len(got) == 0 || got[len(got)-1] != " -> nda-m-draft" || got[0] != "nda-m-draft -> " {
		t.Errorf("nda_document_id changelog = %v", got)
	}
}

func jsonHas(body, frag string) bool { return strings.Contains(body, frag) }

func TestTrustNDAMgrSkipDomains(t *testing.T) {
	g := newNDAFx(t, "ndam-d")
	s := decodeSettings(t, g.mustOK(g.putSettings(`{"nda_skip_domains":[" Example.COM ","*.Sub.Example.com","@b.example.org","example.com"]}`), "set"))
	if want := []string{"*.sub.example.com", "b.example.org", "example.com"}; !slices.Equal(s.NDASkipDomains, want) {
		t.Errorf("normalised list = %v, want %v", s.NDASkipDomains, want)
	}
	if got := g.settings().NDASkipDomains; !slices.Equal(got, s.NDASkipDomains) {
		t.Errorf("stored = %v", got)
	}
	// The same list again writes no changelog row.
	g.mustOK(g.putSettings(`{"nda_skip_domains":["example.com","b.example.org","*.sub.example.com"]}`), "same")
	if got := g.ndaChangelog(t, "nda_skip_domains"); len(got) != 1 || got[0] != " -> *.sub.example.com,b.example.org,example.com" {
		t.Errorf("changelog = %v", got)
	}

	// Refused entries: a 400 and nothing changed.
	for _, bad := range []string{"*.", "a@b.com", "*example.com", "*.*.com", "*.com", "*", "", "localhost", "a b.com", "x/y.com", ".example.com", "a..com"} {
		body, _ := json.Marshal(map[string]any{"nda_skip_domains": []string{"ok.example.com", bad}})
		if r := g.putSettings(string(body)); r.code != http.StatusBadRequest || codeOf(r) != CodeInvalidRequest {
			t.Errorf("entry %q = %d %s, want 400 invalid_request", bad, r.code, r.body)
		}
	}
	if got := g.settings().NDASkipDomains; !slices.Equal(got, s.NDASkipDomains) {
		t.Errorf("a refused PUT changed the list: %v", got)
	}
	if r := g.putSettings(`{"nda_skip_domains":"example.com"}`); r.code != http.StatusBadRequest {
		t.Errorf("a string instead of a list = %d", r.code)
	}

	// An absent key leaves it alone; [] and null clear it.
	g.mustOK(g.putSettings(`{"title":"y"}`), "unrelated")
	if len(g.settings().NDASkipDomains) != 3 {
		t.Errorf("an unrelated PUT changed the list")
	}
	for _, body := range []string{`{"nda_skip_domains":[]}`, `{"nda_skip_domains":null}`} {
		g.mustOK(g.putSettings(`{"nda_skip_domains":["example.com"]}`), "set")
		s = decodeSettings(t, g.mustOK(g.putSettings(body), "clear"))
		if s.NDASkipDomains == nil || len(s.NDASkipDomains) != 0 {
			t.Errorf("%s: list = %#v", body, s.NDASkipDomains)
		}
	}
}

func TestTrustNDAMgrSettingsRoles(t *testing.T) {
	g := newNDAFx(t, "ndam-r")
	for _, who := range []struct{ role, email string }{{"reader", g.read}, {"contributor", g.contr}} {
		r := g.call(g.s.handlePutTrustSettings, http.MethodPut, tmSettings, `{"nda_document_id":"`+g.nda.docID+`","nda_skip_domains":["a.example.com"]}`, who.role, who.email, "", "")
		if r.code != http.StatusForbidden {
			t.Errorf("PUT as %s = %d", who.role, r.code)
		}
		r = g.call(g.s.handleGetTrustSettings, http.MethodGet, tmSettings, "", who.role, who.email, "", "")
		if r.code != http.StatusForbidden {
			t.Errorf("GET as %s = %d", who.role, r.code)
		}
		r = g.call(g.s.handleListTrustVisitors, http.MethodGet, "/api/v1/trust-center/visitors", "", who.role, who.email, "", "")
		if r.code != http.StatusForbidden {
			t.Errorf("visitors as %s = %d", who.role, r.code)
		}
	}
	if s := g.settings(); s.NDADocumentID != nil || len(s.NDASkipDomains) != 0 {
		t.Errorf("a forbidden PUT changed the settings: %+v", s)
	}
}

// The visitors list mirrors the access rule: none / required / accepted /
// skipped / unavailable, from the current NDA and the current skip list.
func TestTrustNDAMgrVisitorStatuses(t *testing.T) {
	g := newNDAFx(t, "ndam-v")
	const reader = "reader@auditco.test"
	if got := g.mgrVisitorNDA(t)[reader]; got.Status != "none" || got.Version != "" || got.ContentHash != "" || got.AcceptedAt != nil {
		t.Errorf("no NDA set: %+v", got)
	}

	g.setNDA(g.nda.docID)
	acc := "acc@auditco.test"
	accCookie := g.e.signIn(t, acc)
	skip := uniqueEmail("example.com")
	skipCookie := g.e.signIn(t, skip)
	v1 := g.wantNDA(t, accCookie)
	if r := g.accept(v1.ContentHash, accCookie); r.code != http.StatusOK {
		t.Fatalf("accept = %d %s", r.code, r.body)
	}

	st := g.mgrVisitorNDA(t)
	if got := st[reader]; got.Status != "required" || got.ContentHash != v1.ContentHash || got.Version != v1.Version || got.AcceptedAt != nil || got.AcceptedVersion != "" {
		t.Errorf("reader = %+v", got)
	}
	if got := st[acc]; got.Status != "accepted" || got.ContentHash != v1.ContentHash || got.AcceptedAt == nil || *got.AcceptedAt == 0 || got.AcceptedVersion != v1.Version {
		t.Errorf("accepted visitor = %+v", got)
	}
	if got := st[skip]; got.Status != "required" {
		t.Errorf("a domain that is not listed yet = %+v", got)
	}
	// The list does not write skip rows.
	g.setSkip("example.com")
	if got := g.mgrVisitorNDA(t)[skip]; got.Status != "skipped" || got.AcceptedAt != nil {
		t.Errorf("skipped visitor = %+v", got)
	}
	if n := g.ndaRows(t, "domain_skip"); n != 0 {
		t.Errorf("the manager list wrote %d skip rows", n)
	}
	// A skip row left by an earlier list is not a grant: the CURRENT list rules.
	g.wantOpen(t, skipCookie, "skip-listed")
	if n := g.ndaRows(t, "domain_skip"); n != 1 {
		t.Fatalf("skip rows = %d", n)
	}
	g.setSkip()
	if got := g.mgrVisitorNDA(t)[skip]; got.Status != "required" {
		t.Errorf("stale skip row counted: %+v", got)
	}

	// A new approved version: the accepted visitor needs to accept again and
	// carries the version they did accept.
	g.nda.save("# NDA\n\nNew text.\n")
	g.nda.approve()
	v2 := g.wantNDA(t, accCookie)
	got := g.mgrVisitorNDA(t)[acc]
	if got.Status != "required" || got.ContentHash != v2.ContentHash || got.AcceptedVersion != v1.Version || got.AcceptedAt == nil {
		t.Errorf("after a new version: %+v", got)
	}
	if r := g.accept(v2.ContentHash, accCookie); r.code != http.StatusOK {
		t.Fatalf("accept v2 = %d", r.code)
	}
	if got := g.mgrVisitorNDA(t)[acc]; got.Status != "accepted" || got.AcceptedVersion != v2.Version {
		t.Errorf("after re-accepting: %+v", got)
	}
	// The status agrees with the public rule for every visitor.
	for email, cookie := range map[string]string{reader: g.cookie, acc: accCookie, skip: skipCookie} {
		pub := g.visitorNDA(t, cookie)["status"]
		if mgr := g.mgrVisitorNDA(t)[email].Status; mgr != pub {
			t.Errorf("%s: manager list says %q, public visitor says %q", email, mgr, pub)
		}
	}

	// Unavailable: set, but nothing can be served.
	g.nda.mustOK(g.nda.docCall(g.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"retired"}}`, "manager", g.mgr), "retire")
	for email, got := range g.mgrVisitorNDA(t) {
		if got.Status != "unavailable" || got.ContentHash != "" {
			t.Errorf("%s with an unavailable NDA = %+v", email, got)
		}
	}
	// Blocking a visitor returns the same row shape.
	v, err := g.s.db.GetTrustVisitorByEmail(t.Context(), g.o.id, acc)
	if err != nil {
		t.Fatal(err)
	}
	r := g.mustOK(g.call(g.s.handleBlockTrustVisitor, http.MethodPut, "/api/v1/trust-center/visitors/1/blocked", `{"blocked":true}`, "manager", g.mgr, "id", strconv.FormatInt(v.ID, 10)), "block")
	var row mgrNDAVisitor
	if err := json.Unmarshal([]byte(r.body), &row); err != nil || row.NDA == nil || row.NDA.Status != "unavailable" {
		t.Errorf("block response nda = %s", r.body)
	}
}
