package api

// A signed-in trust center visitor reads gated documents (#524); restricted
// ones stay locked for everyone, and the manager preview stays the anonymous
// view. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TrustGated

import (
	"context"
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
	// the items list carries no per-viewer fields and no bodies
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
