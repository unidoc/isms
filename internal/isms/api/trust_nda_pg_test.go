package api

// Click-through NDA for gated trust center items (#525). With an NDA document
// set, a gated item needs a signed-in visitor who accepted the NDA's current
// approved version (or whose email domain is on the skip list); with none set
// nothing changes. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run NDA

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"isms.sh/internal/isms/db"
)

// ndaFx is an organization with a gated and a public document, a gated and a
// public certification file, an approved NDA document and a signed-in visitor.
type ndaFx struct {
	*certFx
	e                  visitorEnv
	nda                *tdFx // the NDA document (the fixture's first document)
	gatedDoc, publicID string
	gatedCert, pubCert string
	cookie             string
}

func newNDAFx(t *testing.T, tag string) *ndaFx {
	t.Helper()
	purgeTestLinks(t)
	t.Setenv("ISMS_BASE_URL", "http://"+trustTestBase)
	t.Setenv("ISMS_RATE_LIMIT", "")
	t.Setenv("ISMS_TRUST_RATE_LIMIT", "100000")
	f := newCertFx(t, tag)
	sink := newSMTPSink(t)
	f.s.mailer = sink.mailer()
	g := &ndaFx{certFx: f, e: visitorEnv{s: f.s, sink: sink, org: f.o}, nda: f.tdFx}
	g.nda.save("# NDA\n\nYou shall keep the secrets, version one.\n")
	g.nda.approve()

	publish := func(fx *tdFx, body, level string) string {
		fx.save("# T\n\n" + body + "\n")
		fx.approve()
		return fx.publishOK(level)
	}
	g.gatedDoc = publish(f.withDoc("gated-doc", "Gated Doc"), "gated body text", "gated")
	g.publicID = publish(f.withDoc("public-doc", "Public Doc"), "public body text", "public")
	g.gatedCert = f.mkFile("Gated file", certPDF, map[string]string{"publish": "true", "access_level": "gated"}).Item.ID
	g.pubCert = f.mkFile("Public file", certPDF, map[string]string{"publish": "true", "access_level": "public"}).Item.ID
	g.cookie = g.e.signIn(t, "reader@auditco.test")
	return g
}

// uniqueEmail is an address no earlier run used: the per-address link cap counts
// across runs, and purgeTestLinks only clears the auditco.test domain.
func uniqueEmail(domain string) string {
	return fmt.Sprintf("u%d@%s", time.Now().UnixNano(), domain)
}

func (g *ndaFx) patch(p db.TrustCenterSettingsPatch) {
	g.t.Helper()
	if _, err := g.s.db.UpdateTrustCenterSettings(context.Background(), g.o.id, p, g.mgr); err != nil {
		g.t.Fatal(err)
	}
}

func (g *ndaFx) setNDA(docID string) {
	g.t.Helper()
	var p *string
	if docID != "" {
		p = &docID
	}
	g.patch(db.TrustCenterSettingsPatch{SetNDA: true, NDADocumentID: p})
}

func (g *ndaFx) setSkip(domains ...string) {
	g.t.Helper()
	g.patch(db.TrustCenterSettingsPatch{SetNDASkipDomains: true, NDASkipDomains: domains})
}

func (g *ndaFx) settings() *db.TrustCenterSettings {
	g.t.Helper()
	st, err := g.s.db.GetTrustCenterSettings(context.Background(), g.o.id)
	if err != nil {
		g.t.Fatal(err)
	}
	return st
}

func (g *ndaFx) doc(id, cookie string) trustResp {
	var mods []func(*http.Request)
	if cookie != "" {
		mods = append(mods, withCookie(cookie))
	}
	return vdo(g.s, http.MethodGet, g.e.path("/documents/"+id), "", mods...)
}

func (g *ndaFx) certFile(id, cookie string) trustResp {
	var mods []func(*http.Request)
	if cookie != "" {
		mods = append(mods, withCookie(cookie))
	}
	return vdo(g.s, http.MethodGet, g.e.path("/certifications/"+id+"/file"), "", mods...)
}

// gatedCertListed returns the gated certification's list entry for a viewer.
func (g *ndaFx) gatedCertListed(cookie string) map[string]any {
	g.t.Helper()
	var mods []func(*http.Request)
	if cookie != "" {
		mods = append(mods, withCookie(cookie))
	}
	r := vdo(g.s, http.MethodGet, g.e.path("/items"), "", mods...)
	if r.code != http.StatusOK {
		g.t.Fatalf("items = %d %s", r.code, r.body)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(r.body), &list); err != nil {
		g.t.Fatal(err)
	}
	for _, it := range list {
		if it["id"] == g.gatedCert {
			return it
		}
	}
	g.t.Fatalf("gated certification not listed: %s", r.body)
	return nil
}

func (g *ndaFx) wantLocked(t *testing.T, cookie, why string) {
	t.Helper()
	r := g.doc(g.gatedDoc, cookie)
	g.assertNotFound(r, why+": gated document")
	if strings.Contains(r.body, "gated body text") {
		t.Errorf("%s: a body leaked: %s", why, r.body)
	}
	g.assertNotFound(g.certFile(g.gatedCert, cookie), why+": gated certification file")
	it := g.gatedCertListed(cookie)
	for _, k := range []string{"url", "file_name", "file_size"} {
		if _, ok := it[k]; ok {
			t.Errorf("%s: items list carries %s: %v", why, k, it)
		}
	}
}

func (g *ndaFx) wantOpen(t *testing.T, cookie, why string) {
	t.Helper()
	if r := g.doc(g.gatedDoc, cookie); r.code != http.StatusOK || !strings.Contains(r.body, "gated body text") {
		t.Errorf("%s: gated document = %d %s", why, r.code, r.body)
	}
	if r := g.certFile(g.gatedCert, cookie); r.code != http.StatusOK || r.body != string(certPDF) {
		t.Errorf("%s: gated certification file = %d", why, r.code)
	}
	if it := g.gatedCertListed(cookie); it["file_name"] != "Gated file.pdf" {
		t.Errorf("%s: items list = %v", why, it)
	}
}

func (g *ndaFx) wantPublicOpen(t *testing.T, cookie, why string) {
	t.Helper()
	if r := g.doc(g.publicID, cookie); r.code != http.StatusOK || !strings.Contains(r.body, "public body text") {
		t.Errorf("%s: public document = %d %s", why, r.code, r.body)
	}
	if r := g.certFile(g.pubCert, cookie); r.code != http.StatusOK {
		t.Errorf("%s: public certification file = %d", why, r.code)
	}
}

type ndaJSON struct {
	Version     string `json:"version"`
	ApprovedAt  int64  `json:"approved_at"`
	ContentHash string `json:"content_hash"`
	Title       string `json:"title"`
	HTML        string `json:"html"`
	Company     string `json:"company"`
}

func (g *ndaFx) getNDA(cookie string) trustResp {
	var mods []func(*http.Request)
	if cookie != "" {
		mods = append(mods, withCookie(cookie))
	}
	return vdo(g.s, http.MethodGet, g.e.path("/nda"), "", mods...)
}

func (g *ndaFx) wantNDA(t *testing.T, cookie string) ndaJSON {
	t.Helper()
	r := g.getNDA(cookie)
	if r.code != http.StatusOK {
		t.Fatalf("GET nda = %d %s", r.code, r.body)
	}
	if r.hdr.Get("Cache-Control") != "no-store" {
		t.Errorf("GET nda Cache-Control = %q", r.hdr.Get("Cache-Control"))
	}
	var n ndaJSON
	if err := json.Unmarshal([]byte(r.body), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (g *ndaFx) accept(hash, cookie string) trustResp {
	var mods []func(*http.Request)
	if cookie != "" {
		mods = append(mods, withCookie(cookie))
	}
	return vdo(g.s, http.MethodPost, g.e.path("/nda/accept"), fmt.Sprintf(`{"content_hash":%q}`, hash), mods...)
}

// visitorNDA is the "nda" object of GET /visitor, or nil when it is null.
func (g *ndaFx) visitorNDA(t *testing.T, cookie string) map[string]any {
	t.Helper()
	r := vdo(g.s, http.MethodGet, g.e.path("/visitor"), "", withCookie(cookie))
	if r.code != http.StatusOK {
		t.Fatalf("GET visitor = %d %s", r.code, r.body)
	}
	var b struct {
		NDA map[string]any `json:"nda"`
	}
	if err := json.Unmarshal([]byte(r.body), &b); err != nil {
		t.Fatal(err)
	}
	return b.NDA
}

func (g *ndaFx) ndaRows(t *testing.T, method string) int {
	t.Helper()
	var n int
	rawQuery(t, `SELECT count(*) FROM trust_nda_acceptances WHERE organization_id = $1 AND method = $2`,
		[]any{g.o.id, method}, &n)
	return n
}

func (g *ndaFx) ndaRowsFor(t *testing.T, email, method string) int {
	t.Helper()
	var n int
	rawQuery(t, `SELECT count(*) FROM trust_nda_acceptances a JOIN trust_visitors v
		ON v.id = a.visitor_id AND v.organization_id = a.organization_id
		WHERE a.organization_id = $1 AND v.email = $2 AND a.method = $3`, []any{g.o.id, email, method}, &n)
	return n
}

func (g *ndaFx) overviewNDARequired(t *testing.T) bool {
	t.Helper()
	r := vdo(g.s, http.MethodGet, g.e.path(""), "")
	var o map[string]any
	if err := json.Unmarshal([]byte(r.body), &o); err != nil || r.code != http.StatusOK {
		t.Fatalf("overview = %d %s (%v)", r.code, r.body, err)
	}
	v, ok := o["nda_required"].(bool)
	if !ok {
		t.Fatalf("overview has no boolean nda_required: %s", r.body)
	}
	return v
}

// a. A signed-in visitor who has not accepted the NDA reads no gated item;
// public items stay open.
func TestTrustNDAGatedNeedsAcceptance(t *testing.T) {
	g := newNDAFx(t, "nda-a")
	g.wantOpen(t, g.cookie, "before an NDA is set")
	g.setNDA(g.nda.docID)
	g.wantLocked(t, g.cookie, "signed in, NDA not accepted")
	g.wantLocked(t, "", "anonymous")
	g.wantPublicOpen(t, g.cookie, "signed in, NDA not accepted")
	g.wantPublicOpen(t, "", "anonymous")
}

// b. The NDA text and hash; accepting the hash opens gated items and stores a
// row that matches the served document_versions row.
func TestTrustNDAAcceptOpensGatedAndStoresTheVersion(t *testing.T) {
	g := newNDAFx(t, "nda-b")
	g.setNDA(g.nda.docID)
	n := g.wantNDA(t, g.cookie)
	if n.Version == "" || n.ContentHash == "" || n.ApprovedAt == 0 || n.Title != "Test Doc" ||
		!strings.Contains(n.HTML, "keep the secrets") || n.Company != "Audit Co" {
		t.Fatalf("GET nda = %+v", n)
	}
	var verID int64
	var verLabel, verHash string
	rawQuery(t, `SELECT id, version, content_hash FROM document_versions
		WHERE organization_id = $1 AND document_id = $2 ORDER BY created_at DESC, id DESC LIMIT 1`,
		[]any{g.o.id, g.nda.docID}, &verID, &verLabel, &verHash)
	if n.ContentHash != verHash || n.Version != verLabel {
		t.Errorf("GET nda %q/%q is not the served row %q/%q", n.Version, n.ContentHash, verLabel, verHash)
	}
	if st := g.visitorNDA(t, g.cookie); st["status"] != "required" || st["content_hash"] != n.ContentHash {
		t.Errorf("visitor nda before accepting = %v", st)
	}

	r := g.accept(n.ContentHash, g.cookie)
	if r.code != http.StatusOK {
		t.Fatalf("accept = %d %s", r.code, r.body)
	}
	if r.hdr.Get("Cache-Control") != "no-store" {
		t.Errorf("accept Cache-Control = %q", r.hdr.Get("Cache-Control"))
	}
	g.wantOpen(t, g.cookie, "after accepting")
	g.wantPublicOpen(t, g.cookie, "after accepting")
	g.wantLocked(t, "", "anonymous after another visitor accepted")
	if st := g.visitorNDA(t, g.cookie); st["status"] != "accepted" || st["version"] != verLabel {
		t.Errorf("visitor nda after accepting = %v", st)
	}

	var docID, version, hash, company, ip, ua, method, commit, path string
	var versionID int64
	rawQuery(t, `SELECT document_id, version_id, version, content_hash, method, company, ip, user_agent, commit_hash, file_path
		FROM trust_nda_acceptances WHERE organization_id = $1`, []any{g.o.id},
		&docID, &versionID, &version, &hash, &method, &company, &ip, &ua, &commit, &path)
	g.wantProvable(t, commit, path, hash)
	if docID != g.nda.docID || versionID != verID || version != verLabel || hash != verHash ||
		method != "accepted" || company != "Audit Co" {
		t.Errorf("row = doc %q version_id %d version %q hash %q method %q company %q", docID, versionID, version, hash, method, company)
	}
	if ip == "" {
		t.Errorf("row has no ip")
	}
	var okBody struct {
		NDA map[string]any `json:"nda"`
	}
	if err := json.Unmarshal([]byte(r.body), &okBody); err != nil || okBody.NDA["status"] != "accepted" || okBody.NDA["content_hash"] != n.ContentHash {
		t.Errorf("accept answer nda = %s", r.body)
	}
	// Accepting again is fine and writes nothing new.
	if r := g.accept(n.ContentHash, g.cookie); r.code != http.StatusOK {
		t.Errorf("second accept = %d %s", r.code, r.body)
	}
	if n := g.ndaRows(t, "accepted"); n != 1 {
		t.Errorf("accepted rows = %d, want 1", n)
	}
}

// wantProvable checks that an acceptance row's commit and path lead to the exact
// bytes whose hash it stores, and that they are the served version's: the text a
// visitor accepted stays provable from git even if the version row is overwritten.
func (g *ndaFx) wantProvable(t *testing.T, commit, path, hash string) {
	t.Helper()
	served, err := g.s.trustServed(context.Background(), g.nda.st, g.o.id, g.nda.docID)
	if err != nil || served == nil {
		t.Fatalf("served version: %v %v", served, err)
	}
	if commit == "" || path == "" || commit != served.CommitHash || path != served.FilePath {
		t.Errorf("row commit/path = %q %q, want the served %q %q", commit, path, served.CommitHash, served.FilePath)
	}
	raw, err := g.nda.st.ReadFileAtRef(commit, path)
	if err != nil {
		t.Fatalf("reading %s at %s: %v", path, commit, err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != hash {
		t.Errorf("sha256 of the committed file = %x, row content_hash = %s", sum, hash)
	}
}

// c. A newly approved NDA version locks the visitor out again until they accept it.
func TestTrustNDANewVersionNeedsAcceptingAgain(t *testing.T) {
	g := newNDAFx(t, "nda-c")
	g.setNDA(g.nda.docID)
	v1 := g.wantNDA(t, g.cookie)
	if r := g.accept(v1.ContentHash, g.cookie); r.code != http.StatusOK {
		t.Fatalf("accept v1 = %d %s", r.code, r.body)
	}
	g.wantOpen(t, g.cookie, "v1 accepted")

	g.nda.save("# NDA\n\nYou shall keep the secrets, and the new ones too.\n")
	// An unapproved draft does not change what is served.
	g.wantOpen(t, g.cookie, "draft saved, not approved")
	g.nda.approve()
	v2 := g.wantNDA(t, g.cookie)
	if v2.ContentHash == v1.ContentHash || !strings.Contains(v2.HTML, "new ones") {
		t.Fatalf("approving did not serve a new version: %+v vs %+v", v1, v2)
	}
	g.wantLocked(t, g.cookie, "new NDA version approved")
	g.wantPublicOpen(t, g.cookie, "new NDA version approved")
	if st := g.visitorNDA(t, g.cookie); st["status"] != "required" || st["content_hash"] != v2.ContentHash {
		t.Errorf("visitor nda after a new version = %v", st)
	}
	if r := g.accept(v2.ContentHash, g.cookie); r.code != http.StatusOK {
		t.Fatalf("accept v2 = %d %s", r.code, r.body)
	}
	g.wantOpen(t, g.cookie, "v2 accepted")
	if n := g.ndaRows(t, "accepted"); n != 2 {
		t.Errorf("accepted rows = %d, want one per version", n)
	}
}

// d. Accepting a hash that is not the current one is a 409 that carries the
// current NDA and writes nothing.
func TestTrustNDAAcceptWithAnOldOrEmptyHashIs409(t *testing.T) {
	g := newNDAFx(t, "nda-d")
	g.setNDA(g.nda.docID)
	v1 := g.wantNDA(t, g.cookie)
	g.nda.save("# NDA\n\nSecond text.\n")
	g.nda.approve()
	v2 := g.wantNDA(t, g.cookie)

	for _, hash := range []string{v1.ContentHash, "", "deadbeef"} {
		r := g.accept(hash, g.cookie)
		if r.code != http.StatusConflict || codeOfBody(r) != CodeTrustNDAOutdated {
			t.Fatalf("accept %q = %d %s, want 409 %s", hash, r.code, r.body, CodeTrustNDAOutdated)
		}
		var b struct {
			NDA ndaJSON `json:"nda"`
		}
		if err := json.Unmarshal([]byte(r.body), &b); err != nil || b.NDA.ContentHash != v2.ContentHash || !strings.Contains(b.NDA.HTML, "Second text.") {
			t.Errorf("409 body does not carry the current NDA: %s (%v)", r.body, err)
		}
	}
	// A body with no content_hash at all.
	if r := vdo(g.s, http.MethodPost, g.e.path("/nda/accept"), `{}`, withCookie(g.cookie)); r.code != http.StatusConflict {
		t.Errorf("accept {} = %d %s", r.code, r.body)
	}
	if n := g.ndaRows(t, "accepted"); n != 0 {
		t.Errorf("rows written by refused accepts: %d", n)
	}
	g.wantLocked(t, g.cookie, "after refused accepts")

	// Not JSON: 400, like the other visitor POSTs.
	for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", ""} {
		req := httptest.NewRequest(http.MethodPost, "http://"+trustTestBase+g.e.path("/nda/accept"), strings.NewReader("content_hash="+v2.ContentHash))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		req.AddCookie(&http.Cookie{Name: "isms_trust_visitor", Value: g.cookie})
		rec := httptest.NewRecorder()
		g.s.echo.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("accept as %q = %d, want 400", ct, rec.Code)
		}
	}
	if r := vdo(g.s, http.MethodPost, g.e.path("/nda/accept"), `{not json`, withCookie(g.cookie)); r.code != http.StatusBadRequest {
		t.Errorf("accept with broken JSON = %d", r.code)
	}
	if n := g.ndaRows(t, "accepted"); n != 0 {
		t.Errorf("rows written by refused accepts: %d", n)
	}
	// Only POST accepts; GET /nda/accept and POST /nda are not routes.
	for _, c := range []struct{ method, rest string }{
		{http.MethodGet, "/nda/accept"}, {http.MethodPost, "/nda"}, {http.MethodPut, "/nda/accept"},
		{http.MethodPost, "/nda/accept/x"}, {http.MethodDelete, "/nda"}, {http.MethodGet, "/nda/x"},
	} {
		r := vdo(g.s, c.method, g.e.path(c.rest), `{}`, withCookie(g.cookie))
		g.assertNotFound(r, c.method+" "+c.rest)
	}
}

// e. The skip list: an exact domain, a wildcard, live evaluation and the log row.
func TestTrustNDASkipDomains(t *testing.T) {
	g := newNDAFx(t, "nda-e")
	g.setNDA(g.nda.docID)
	g.setSkip("example.com")

	exact := uniqueEmail("example.com")
	exactCookie := g.e.signIn(t, exact)
	sub := uniqueEmail("sub.example.com")
	subCookie := g.e.signIn(t, sub)

	g.wantOpen(t, exactCookie, "example.com is on the list")
	g.wantOpen(t, exactCookie, "second read")
	if n := g.ndaRowsFor(t, exact, "domain_skip"); n != 1 {
		t.Errorf("domain_skip rows after repeated reads = %d, want 1", n)
	}
	if st := g.visitorNDA(t, exactCookie); st["status"] != "skipped" {
		t.Errorf("visitor nda for a skipped domain = %v", st)
	}
	// example.com does not cover its subdomains.
	g.wantLocked(t, subCookie, "sub.example.com with only example.com listed")
	if n := g.ndaRowsFor(t, sub, "domain_skip"); n != 0 {
		t.Errorf("domain_skip rows for an unlisted visitor = %d", n)
	}
	g.wantLocked(t, g.cookie, "an unlisted domain")

	// A wildcard covers subdomains at every depth, never the domain itself.
	g.setSkip("*.example.com")
	deep := uniqueEmail("x.sub.example.com")
	deepCookie := g.e.signIn(t, deep)
	g.wantOpen(t, subCookie, "*.example.com, one level")
	g.wantOpen(t, deepCookie, "*.example.com, two levels")
	g.wantLocked(t, exactCookie, "*.example.com does not cover example.com")
	if n := g.ndaRowsFor(t, sub, "domain_skip"); n != 1 {
		t.Errorf("domain_skip rows for the wildcard match = %d", n)
	}

	// Removing the domain revokes the skip at once; the log row does not grant.
	g.setSkip()
	g.wantLocked(t, subCookie, "domain removed from the list")
	g.wantLocked(t, deepCookie, "domain removed from the list")

	// A skipped visitor can still accept, and gets an accepted row too.
	g.setSkip("*.example.com")
	g.wantOpen(t, subCookie, "listed again")
	n := g.wantNDA(t, subCookie)
	if r := g.accept(n.ContentHash, subCookie); r.code != http.StatusOK {
		t.Fatalf("a skipped visitor accepting = %d %s", r.code, r.body)
	}
	if a, s := g.ndaRowsFor(t, sub, "accepted"), g.ndaRowsFor(t, sub, "domain_skip"); a != 1 || s != 1 {
		t.Errorf("rows for the skipped visitor who accepted: accepted %d, domain_skip %d", a, s)
	}
	// Having accepted, access survives the domain being removed.
	g.setSkip()
	g.wantOpen(t, subCookie, "accepted, then removed from the list")
	if st := g.visitorNDA(t, subCookie); st["status"] != "accepted" {
		t.Errorf("visitor nda = %v", st)
	}
}

// f. No NDA set: gated is sign-in only. With one: nda_required.
func TestTrustNDAUnsetChangesNothing(t *testing.T) {
	g := newNDAFx(t, "nda-f")
	if g.overviewNDARequired(t) {
		t.Errorf("nda_required is true with no NDA set")
	}
	g.wantOpen(t, g.cookie, "no NDA set")
	g.wantLocked(t, "", "no NDA set, anonymous")
	if st := g.visitorNDA(t, g.cookie); st["status"] != "none" {
		t.Errorf("visitor nda with no NDA = %v", st)
	}
	if r := g.getNDA(g.cookie); r.code != http.StatusNotFound {
		t.Errorf("GET nda with no NDA = %d %s", r.code, r.body)
	}
	if r := g.accept("x", g.cookie); r.code != http.StatusNotFound {
		t.Errorf("accept with no NDA = %d %s", r.code, r.body)
	}
	if n := g.ndaRows(t, "accepted") + g.ndaRows(t, "domain_skip"); n != 0 {
		t.Errorf("rows written with no NDA: %d", n)
	}

	g.setNDA(g.nda.docID)
	if !g.overviewNDARequired(t) {
		t.Errorf("nda_required is false with an approved NDA set")
	}
	g.setNDA("")
	if g.overviewNDARequired(t) {
		t.Errorf("nda_required is true after clearing the NDA")
	}
	g.wantOpen(t, g.cookie, "NDA cleared")
}

// g. An NDA that is set but cannot be served fails closed and stays set.
func TestTrustNDAUnavailableFailsClosed(t *testing.T) {
	g := newNDAFx(t, "nda-g")
	retired := g.withDoc("nda-retired", "Retired NDA")
	retired.save("# NDA\n\nretired nda text\n")
	retired.approve()
	draft := g.withDoc("nda-draft", "Draft NDA")
	draft.save("# NDA\n\nnever approved\n")
	deleted := g.withDoc("nda-deleted", "Deleted NDA")
	deleted.save("# NDA\n\ndeleted nda text\n")
	deleted.approve()
	emptyHash := g.withDoc("nda-nohash", "No hash NDA")
	emptyHash.save("# NDA\n\nno hash nda text\n")
	emptyHash.approve()

	mgr := func(f *tdFx, h string) {
		f.mustOK(f.docCall(f.s.handleUpdateDocumentMetadata, http.MethodPut, h, "manager", f.mgr), "update")
	}
	mgr(retired, `{"fields":{"status":"retired"}}`)
	deleted.mustOK(deleted.docCall(deleted.s.handleDeleteDocument, http.MethodDelete, "", "manager", deleted.mgr), "delete")
	rawExec(t, `UPDATE document_versions SET content_hash = '' WHERE organization_id = $1 AND document_id = $2`, g.o.id, emptyHash.docID)

	for _, c := range []struct{ name, docID string }{
		{"never approved", draft.docID}, {"retired", retired.docID}, {"deleted", deleted.docID}, {"empty content hash", emptyHash.docID},
	} {
		g.setNDA(c.docID)
		// A skip-listed visitor is locked out too: nothing can be satisfied.
		g.setSkip("auditco.test")
		g.wantLocked(t, g.cookie, c.name)
		g.wantPublicOpen(t, g.cookie, c.name)
		if r := g.getNDA(g.cookie); r.code != http.StatusNotFound {
			t.Errorf("%s: GET nda = %d %s", c.name, r.code, r.body)
		}
		for _, hash := range []string{"", "x"} {
			if r := g.accept(hash, g.cookie); r.code != http.StatusNotFound {
				t.Errorf("%s: accept %q = %d %s", c.name, hash, r.code, r.body)
			}
		}
		if !g.overviewNDARequired(t) {
			t.Errorf("%s: nda_required is false", c.name)
		}
		if st := g.visitorNDA(t, g.cookie); st["status"] != "unavailable" {
			t.Errorf("%s: visitor nda = %v", c.name, st)
		}
		if got := g.settings().NDADocumentID; got == nil || *got != c.docID {
			t.Errorf("%s: the setting was changed to %v", c.name, got)
		}
		if n := g.ndaRows(t, "accepted") + g.ndaRows(t, "domain_skip"); n != 0 {
			t.Errorf("%s: %d rows written", c.name, n)
		}
	}
	// A NDA that names a document that does not exist at all.
	g.setNDA("no-such-document")
	g.wantLocked(t, g.cookie, "unknown document")

	// Approving the draft one makes it available again.
	g.setSkip()
	g.setNDA(draft.docID)
	draft.approve()
	g.wantLocked(t, g.cookie, "draft approved, not yet accepted")
	n := g.wantNDA(t, g.cookie)
	if r := g.accept(n.ContentHash, g.cookie); r.code != http.StatusOK {
		t.Fatalf("accept = %d %s", r.code, r.body)
	}
	g.wantOpen(t, g.cookie, "approved NDA accepted")
}

// h. Not signed in, or signed in somewhere else: the one 404.
func TestTrustNDAEndpointsAreForSignedInVisitorsOnly(t *testing.T) {
	g := newNDAFx(t, "nda-h")
	g.setNDA(g.nda.docID)
	n := g.wantNDA(t, g.cookie)
	orgB := trustNewOrg(t, g.s, "nda-hb", true)
	cookieB := visitorEnv{s: g.s, sink: g.e.sink, org: orgB}.signIn(t, "reader@auditco.test")

	for _, c := range []struct{ name, cookie string }{{"no cookie", ""}, {"other organization's cookie", cookieB}, {"garbage cookie", "nope"}} {
		g.assertNotFound(g.getNDA(c.cookie), "GET nda, "+c.name)
		g.assertNotFound(g.accept(n.ContentHash, c.cookie), "accept, "+c.name)
	}
	g.assertNotFound(vdo(g.s, http.MethodGet, g.e.path("/nda"), "", withBearer(g.cookie)), "GET nda, visitor token as Bearer")
	if n := g.ndaRows(t, "accepted"); n != 0 {
		t.Errorf("rows written for refused accepts: %d", n)
	}
	// The trust center off: the same 404.
	off := trustNewOrg(t, g.s, "nda-hoff", false)
	r := vdo(g.s, http.MethodGet, "/api/v1/trust/"+off.slug+"/nda", "", withCookie(g.cookie))
	g.assertNotFound(r, "trust center off")
	// Another organization's cookie is not accepted for acceptance either.
	if _, err := g.s.db.GetTrustCenterSettings(context.Background(), orgB.id); err != nil {
		t.Fatal(err)
	}
	if r := vdo(g.s, http.MethodGet, "/api/v1/trust/"+orgB.slug+"/nda", "", withCookie(g.cookie)); r.code != http.StatusNotFound {
		t.Errorf("org A cookie at org B = %d", r.code)
	}
}

// i. The manager preview is the anonymous view: it ignores the visitor cookie
// and its acceptance.
func TestTrustNDAPreviewStaysAnonymous(t *testing.T) {
	g := newNDAFx(t, "nda-i")
	g.setNDA(g.nda.docID)
	n := g.wantNDA(t, g.cookie)
	if r := g.accept(n.ContentHash, g.cookie); r.code != http.StatusOK {
		t.Fatalf("accept = %d %s", r.code, r.body)
	}
	g.wantOpen(t, g.cookie, "accepted")

	u := contractTestUser(t, g.s, g.o.id, "mgr-nda-i@vis.test", "manager")
	jwt, err := g.s.createSessionJWT(u, g.o.id, "manager", g.o.slug, "Org")
	if err != nil {
		t.Fatal(err)
	}
	p := "/api/v1/trust-center/preview"
	g.assertNotFound(vdo(g.s, http.MethodGet, p+"/documents/"+g.gatedDoc, "", withBearer(jwt), withCookie(g.cookie)), "preview of a gated document")
	g.assertNotFound(vdo(g.s, http.MethodGet, p+"/certifications/"+g.gatedCert+"/file", "", withBearer(jwt), withCookie(g.cookie)), "preview of a gated file")
	r := vdo(g.s, http.MethodGet, p+"/items", "", withBearer(jwt), withCookie(g.cookie))
	if r.code != http.StatusOK || strings.Contains(r.body, "Gated file.pdf") {
		t.Errorf("preview items = %d %s", r.code, r.body)
	}
	// The preview exposes no NDA routes.
	for _, rest := range []string{"/nda", "/nda/accept"} {
		if r := vdo(g.s, http.MethodGet, p+rest, "", withBearer(jwt), withCookie(g.cookie)); r.code == http.StatusOK {
			t.Errorf("preview%s answered 200", rest)
		}
	}
}

// The overview document is public whatever the NDA says: it is served to an
// anonymous viewer by the anonymous rule.
func TestTrustNDADoesNotChangeTheOverviewRule(t *testing.T) {
	g := newNDAFx(t, "nda-ov")
	ov := g.withDoc("ov-doc", "Overview")
	ov.save("# O\n\nthe overview text\n")
	ov.approve()
	g.setOverview(ov.docID)
	g.setNDA(g.nda.docID)
	o, raw := g.overviewResp()
	if o.Overview == nil || !strings.Contains(o.Overview.HTML, "the overview text") {
		t.Errorf("overview = %s", raw)
	}
}

// The visitor responses report the status and write no skip row; only a gated
// read that the skip list lets in does, once.
func TestTrustNDAVisitorResponsesWriteNoSkipRow(t *testing.T) {
	g := newNDAFx(t, "nda-skiplog")
	g.setNDA(g.nda.docID)
	g.setSkip("example.com")
	email := uniqueEmail("example.com")
	cookie := g.e.signIn(t, email)
	for i := 0; i < 3; i++ {
		if st := g.visitorNDA(t, cookie); st["status"] != "skipped" {
			t.Fatalf("visitor nda = %v", st)
		}
	}
	if n := g.ndaRowsFor(t, email, "domain_skip"); n != 0 {
		t.Errorf("GET visitor x3 wrote %d skip rows", n)
	}
	g.wantPublicOpen(t, cookie, "skip-listed, public reads")
	if n := g.ndaRowsFor(t, email, "domain_skip"); n != 0 {
		t.Errorf("public reads wrote %d skip rows", n)
	}
	g.wantOpen(t, cookie, "skip-listed")
	g.wantOpen(t, cookie, "skip-listed, again")
	if n := g.ndaRowsFor(t, email, "domain_skip"); n != 1 {
		t.Errorf("gated reads wrote %d skip rows, want 1", n)
	}
	var commit, path, hash string
	rawQuery(t, `SELECT commit_hash, file_path, content_hash FROM trust_nda_acceptances a JOIN trust_visitors v
		ON v.id = a.visitor_id WHERE a.organization_id = $1 AND v.email = $2 AND a.method = 'domain_skip'`, []any{g.o.id, email}, &commit, &path, &hash)
	g.wantProvable(t, commit, path, hash)
}
