package api

// A signed-in trust center visitor reads gated documents and certifications (#524); restricted
// ones stay locked for everyone, and the manager preview stays the anonymous
// view. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TrustGated

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"isms.sh/internal/isms/db"
)

func TestTrustViewerMayRead(t *testing.T) {
	v := &db.TrustVisitor{ID: 1, Email: "a@b.test"}
	cases := []struct {
		level string
		v     *db.TrustVisitor
		want  bool
	}{
		{db.TrustAccessPublic, nil, true}, {db.TrustAccessPublic, v, true},
		{db.TrustAccessGated, nil, false}, {db.TrustAccessGated, v, true},
		{db.TrustAccessRestricted, nil, false}, {db.TrustAccessRestricted, v, false},
		{"", nil, false}, {"", v, false}, {"secret", v, false}, {"PUBLIC", v, false},
	}
	for _, c := range cases {
		if got := trustViewerMayRead(c.level, c.v); got != c.want {
			t.Errorf("trustViewerMayRead(%q, visitor=%v) = %v, want %v", c.level, c.v != nil, got, c.want)
		}
	}
}

// gatedFx is an organization with a gated, a restricted and a public document
// published, a fake SMTP sink and a signed-in visitor.
type gatedFx struct {
	f, gatedFx              *tdFx
	e                       visitorEnv
	gatedID, restrID, pubID string
	cookie                  string
}

func newGatedFx(t *testing.T, tag string) *gatedFx {
	t.Helper()
	purgeTestLinks(t)
	t.Setenv("ISMS_BASE_URL", "http://"+trustTestBase)
	t.Setenv("ISMS_RATE_LIMIT", "")
	t.Setenv("ISMS_TRUST_RATE_LIMIT", "100000")
	f := newTDFx(t, tag, "pol-pub")
	sink := newSMTPSink(t)
	f.s.mailer = sink.mailer()
	g := &gatedFx{f: f, e: visitorEnv{s: f.s, sink: sink, org: f.o}}
	publish := func(fx *tdFx, body, level string) string {
		fx.save("# T\n\n" + body + "\n")
		fx.approve()
		return fx.publishOK(level)
	}
	g.pubID = publish(f, "public body text", "public")
	g.gatedFx = f.withDoc("pol-gated", "Gated Doc")
	g.gatedID = publish(g.gatedFx, "gated body text", "gated")
	g.restrID = publish(f.withDoc("pol-restr", "Restricted Doc"), "restricted body text", "restricted")
	g.cookie = g.e.signIn(t, "reader@auditco.test")
	return g
}

func (g *gatedFx) doc(id string, mods ...func(*http.Request)) trustResp {
	return vdo(g.f.s, http.MethodGet, g.e.path("/documents/"+id), "", mods...)
}

func (g *gatedFx) wantNotFound(t *testing.T, r trustResp, why string) {
	t.Helper()
	g.f.assertNotFound(r, why)
	if strings.Contains(r.body, "body text") {
		t.Errorf("%s: a body leaked: %s", why, r.body)
	}
}

func TestTrustGatedDocumentAccess(t *testing.T) {
	g := newGatedFx(t, "a3acc")

	// a. anonymous
	g.wantNotFound(t, g.doc(g.gatedID), "anonymous, gated")
	// b. signed in
	r := g.doc(g.gatedID, withCookie(g.cookie))
	if r.code != http.StatusOK || !strings.Contains(r.body, "gated body text") || !strings.Contains(r.body, `"html"`) {
		t.Fatalf("visitor, gated = %d %s", r.code, r.body)
	}
	if r.hdr.Get("Cache-Control") != "no-store" {
		t.Errorf("gated answer Cache-Control = %q, want no-store", r.hdr.Get("Cache-Control"))
	}
	// the next anonymous request is still refused
	g.wantNotFound(t, g.doc(g.gatedID), "anonymous after a visitor read")
	// c. restricted stays locked
	g.wantNotFound(t, g.doc(g.restrID, withCookie(g.cookie)), "visitor, restricted")
	// d. public
	if r := g.doc(g.pubID, withCookie(g.cookie)); r.code != http.StatusOK || !strings.Contains(r.body, "public body text") {
		t.Errorf("visitor, public = %d %s", r.code, r.body)
	}
	if r := g.doc(g.pubID); r.code != http.StatusOK {
		t.Errorf("anonymous, public = %d", r.code)
	}
	// a Bearer header is not a visitor credential
	g.wantNotFound(t, g.doc(g.gatedID, withBearer(g.cookie)), "visitor token as Bearer")
	// the items list carries no per-viewer document fields and no bodies (only a
	// certification's file and link depend on the viewer)
	items := vdo(g.f.s, http.MethodGet, g.e.path("/items"), "", withCookie(g.cookie))
	anon := vdo(g.f.s, http.MethodGet, g.e.path("/items"), "")
	if items.body != anon.body || strings.Contains(items.body, "body text") {
		t.Errorf("items differ for a visitor:\n%s\n%s", items.body, anon.body)
	}
}

// e. Another organization's session does not open this organization's document.
func TestTrustGatedDocumentOtherOrgCookie(t *testing.T) {
	g := newGatedFx(t, "a3org")
	orgB := trustNewOrg(t, g.f.s, "a3orgb", true)
	cookieB := visitorEnv{s: g.f.s, sink: g.e.sink, org: orgB}.signIn(t, "reader@auditco.test")
	g.wantNotFound(t, g.doc(g.gatedID, withCookie(cookieB)), "org B visitor cookie")
	if r := g.doc(g.gatedID, withCookie(g.cookie)); r.code != http.StatusOK {
		t.Errorf("own cookie = %d", r.code)
	}
}

// f. Sign-out and a block both end access at once.
func TestTrustGatedDocumentEndsOnSignOutAndBlock(t *testing.T) {
	g := newGatedFx(t, "a3end")
	c2 := g.e.signIn(t, "second@auditco.test")
	if r := g.doc(g.gatedID, withCookie(c2)); r.code != http.StatusOK {
		t.Fatalf("second visitor = %d", r.code)
	}

	vdo(g.f.s, http.MethodPost, g.e.path("/visitor/sign-out"), "", withCookie(g.cookie))
	g.wantNotFound(t, g.doc(g.gatedID, withCookie(g.cookie)), "after sign-out")

	v, err := g.f.s.db.GetTrustVisitorByEmail(context.Background(), g.f.o.id, "second@auditco.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.f.s.db.SetTrustVisitorBlocked(context.Background(), g.f.o.id, v.ID, true, "mgr@example.com"); err != nil {
		t.Fatal(err)
	}
	g.wantNotFound(t, g.doc(g.gatedID, withCookie(c2)), "after a block")
}

// g. A retired or hidden gated document is not served even to a visitor.
func TestTrustGatedDocumentRetiredIsNotFound(t *testing.T) {
	g := newGatedFx(t, "a3ret")
	gf := g.gatedFx
	if r := g.doc(g.gatedID, withCookie(g.cookie)); r.code != http.StatusOK {
		t.Fatalf("before retire = %d", r.code)
	}
	gf.mustOK(gf.docCall(gf.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"retired"}}`, "manager", gf.mgr), "retire")
	g.wantNotFound(t, g.doc(g.gatedID, withCookie(g.cookie)), "retired gated document")
	g.wantNotFound(t, g.doc("00000000-0000-0000-0000-000000000000", withCookie(g.cookie)), "unknown id")
}

// h. The preview is the anonymous view: a manager who also carries a valid
// visitor cookie still gets the locked answer.
func TestTrustGatedPreviewIgnoresTheVisitorCookie(t *testing.T) {
	g := newGatedFx(t, "a3prev")
	u := contractTestUser(t, g.f.s, g.f.o.id, "mgr-a3prev@vis.test", "manager")
	jwt, err := g.f.s.createSessionJWT(u, g.f.o.id, "manager", g.f.o.slug, "Org")
	if err != nil {
		t.Fatal(err)
	}
	p := "/api/v1/trust-center/preview/documents/"
	r := vdo(g.f.s, http.MethodGet, p+g.gatedID, "", withBearer(jwt), withCookie(g.cookie))
	g.wantNotFound(t, r, "preview of a gated document with a visitor cookie")
	if r := vdo(g.f.s, http.MethodGet, p+g.pubID, "", withBearer(jwt), withCookie(g.cookie)); r.code != http.StatusOK {
		t.Errorf("preview of a public document = %d", r.code)
	}
}

// gatedCertFx is an organization with certifications (a blob store) and a
// signed-in visitor, for the gated certification rule.
type gatedCertFx struct {
	*certFx
	e      visitorEnv
	cookie string
}

func newGatedCertFx(t *testing.T, tag string) *gatedCertFx {
	t.Helper()
	purgeTestLinks(t)
	t.Setenv("ISMS_BASE_URL", "http://"+trustTestBase)
	t.Setenv("ISMS_RATE_LIMIT", "")
	t.Setenv("ISMS_TRUST_RATE_LIMIT", "100000")
	f := newCertFx(t, tag)
	sink := newSMTPSink(t)
	f.s.mailer = sink.mailer()
	g := &gatedCertFx{certFx: f, e: visitorEnv{s: f.s, sink: sink, org: f.o}}
	g.cookie = g.e.signIn(t, "reader@auditco.test")
	return g
}

func (g *gatedCertFx) file(itemID string, mods ...func(*http.Request)) trustResp {
	return vdo(g.s, http.MethodGet, g.e.path("/certifications/"+itemID+"/file"), "", mods...)
}

func (g *gatedCertFx) items(mods ...func(*http.Request)) map[string]map[string]any {
	g.t.Helper()
	r := vdo(g.s, http.MethodGet, g.e.path("/items"), "", mods...)
	if r.code != http.StatusOK {
		g.t.Fatalf("items = %d %s", r.code, r.body)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(r.body), &list); err != nil {
		g.t.Fatal(err)
	}
	out := map[string]map[string]any{}
	for _, it := range list {
		if it["kind"] == "certification" {
			out[it["title"].(string)] = it
		}
	}
	return out
}

// A signed-in visitor downloads a gated certification's file and sees a gated
// link; anonymous viewers, restricted items and expired ones stay as before.
func TestTrustGatedCertificationAccess(t *testing.T) {
	g := newGatedCertFx(t, "a3cert")
	gf := g.mkFile("Gated file", certPDF, map[string]string{"publish": "true", "access_level": "gated"})
	g.mkLink("Gated link", `,"publish":true,"access_level":"gated"`)
	rf := g.mkFile("Restricted file", certPDF, map[string]string{"publish": "true", "access_level": "restricted"})
	g.mkLink("Restricted link", `,"publish":true,"access_level":"restricted"`)
	pf := g.mkFile("Public file", certPDF, map[string]string{"publish": "true", "access_level": "public"})

	// The file route.
	if r := g.file(gf.Item.ID); r.code != http.StatusNotFound {
		t.Errorf("anonymous, gated file = %d", r.code)
	}
	r := g.file(gf.Item.ID, withCookie(g.cookie))
	if r.code != http.StatusOK || r.body != string(certPDF) {
		t.Fatalf("visitor, gated file = %d", r.code)
	}
	if r.hdr.Get("Cache-Control") != "no-store" || !strings.HasPrefix(r.hdr.Get("Content-Disposition"), "attachment") {
		t.Errorf("gated file headers = %v", r.hdr)
	}
	if r := g.file(gf.Item.ID); r.code != http.StatusNotFound {
		t.Errorf("anonymous after a visitor download = %d", r.code)
	}
	if r := g.file(gf.Item.ID, withBearer(g.cookie)); r.code != http.StatusNotFound {
		t.Errorf("visitor token as Bearer = %d", r.code)
	}
	if r := g.file(rf.Item.ID, withCookie(g.cookie)); r.code != http.StatusNotFound {
		t.Errorf("visitor, restricted file = %d", r.code)
	}
	for _, mods := range [][]func(*http.Request){nil, {withCookie(g.cookie)}} {
		if r := g.file(pf.Item.ID, mods...); r.code != http.StatusOK {
			t.Errorf("public file (cookie %v) = %d", mods != nil, r.code)
		}
	}

	// The items list: a gated certification's file and link only for the visitor.
	anon, vis := g.items(), g.items(withCookie(g.cookie))
	for _, title := range []string{"Gated file", "Gated link", "Restricted file", "Restricted link"} {
		for _, k := range []string{"url", "file_name", "file_size"} {
			if _, ok := anon[title][k]; ok {
				t.Errorf("anonymous %s carries %s: %v", title, k, anon[title])
			}
		}
	}
	if vis["Gated file"]["file_name"] != "Gated file.pdf" || vis["Gated file"]["file_size"] != float64(len(certPDF)) {
		t.Errorf("visitor, gated file item = %v", vis["Gated file"])
	}
	if vis["Gated link"]["url"] != "https://example.com/r" {
		t.Errorf("visitor, gated link item = %v", vis["Gated link"])
	}
	for _, title := range []string{"Restricted file", "Restricted link"} {
		for _, k := range []string{"url", "file_name", "file_size"} {
			if _, ok := vis[title][k]; ok {
				t.Errorf("visitor, %s carries %s: %v", title, k, vis[title])
			}
		}
	}
	if mustJSON(anon["Public file"]) != mustJSON(vis["Public file"]) {
		t.Errorf("public item differs for a visitor:\n%v\n%v", anon["Public file"], vis["Public file"])
	}

	// Expired: gone for the visitor too.
	setUntil(t, gf.ID, intp(-1))
	if r := g.file(gf.Item.ID, withCookie(g.cookie)); r.code != http.StatusNotFound {
		t.Errorf("visitor, expired gated file = %d", r.code)
	}
	if _, ok := g.items(withCookie(g.cookie))["Gated file"]; ok {
		t.Errorf("expired gated certification still listed")
	}
}

// Another organization's session, a sign-out and a block all end access.
func TestTrustGatedCertificationEndsWithTheSession(t *testing.T) {
	g := newGatedCertFx(t, "a3certend")
	gf := g.mkFile("Gated file", certPDF, map[string]string{"publish": "true", "access_level": "gated"})
	orgB := trustNewOrg(t, g.s, "a3certendb", true)
	cookieB := visitorEnv{s: g.s, sink: g.e.sink, org: orgB}.signIn(t, "reader@auditco.test")
	if r := g.file(gf.Item.ID, withCookie(cookieB)); r.code != http.StatusNotFound {
		t.Errorf("org B visitor cookie = %d", r.code)
	}
	if _, ok := g.items(withCookie(cookieB))["Gated file"]["file_name"]; ok {
		t.Errorf("org B visitor sees the gated file name")
	}

	c2 := g.e.signIn(t, "second@auditco.test")
	if r := g.file(gf.Item.ID, withCookie(c2)); r.code != http.StatusOK {
		t.Fatalf("second visitor = %d", r.code)
	}
	vdo(g.s, http.MethodPost, g.e.path("/visitor/sign-out"), "", withCookie(g.cookie))
	if r := g.file(gf.Item.ID, withCookie(g.cookie)); r.code != http.StatusNotFound {
		t.Errorf("after sign-out = %d", r.code)
	}
	if _, ok := g.items(withCookie(g.cookie))["Gated file"]["file_name"]; ok {
		t.Errorf("signed-out visitor still sees the gated file name")
	}
	v, err := g.s.db.GetTrustVisitorByEmail(context.Background(), g.o.id, "second@auditco.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.s.db.SetTrustVisitorBlocked(context.Background(), g.o.id, v.ID, true, "mgr@example.com"); err != nil {
		t.Fatal(err)
	}
	if r := g.file(gf.Item.ID, withCookie(c2)); r.code != http.StatusNotFound {
		t.Errorf("after a block = %d", r.code)
	}
}

// The preview is the anonymous view: a manager who also carries a valid visitor
// cookie gets no gated file and no gated file details.
func TestTrustGatedCertificationPreviewIgnoresTheVisitorCookie(t *testing.T) {
	g := newGatedCertFx(t, "a3certprev")
	gf := g.mkFile("Gated file", certPDF, map[string]string{"publish": "true", "access_level": "gated"})
	u := contractTestUser(t, g.s, g.o.id, "mgr-a3certprev@vis.test", "manager")
	jwt, err := g.s.createSessionJWT(u, g.o.id, "manager", g.o.slug, "Org")
	if err != nil {
		t.Fatal(err)
	}
	r := vdo(g.s, http.MethodGet, "/api/v1/trust-center/preview/certifications/"+gf.Item.ID+"/file", "", withBearer(jwt), withCookie(g.cookie))
	if r.code != http.StatusNotFound {
		t.Errorf("preview of a gated file with a visitor cookie = %d", r.code)
	}
	r = vdo(g.s, http.MethodGet, "/api/v1/trust-center/preview/items", "", withBearer(jwt), withCookie(g.cookie))
	if r.code != http.StatusOK || strings.Contains(r.body, "Gated file.pdf") {
		t.Errorf("preview items with a visitor cookie = %d %s", r.code, r.body)
	}
}
