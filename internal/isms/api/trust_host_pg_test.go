package api

// The trust domain (#528): an organization's trust center served on its own
// host. These drive the real chain (resolver, gate, auth, routes, SPA fallback)
// with Host headers. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TrustHost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/blob"
	"isms.sh/internal/isms/i18n"
)

const trustHostIndex = `<!doctype html><html><head><title>x</title></head><body>app</body></html>`

type trustHostEnv struct {
	s      *Server
	a, b   trustOrg
	trust  string // A's trust domain
	mgrTok string // A manager bearer
	bTok   string // B manager bearer
	logoA  string
	logoB  string
}

func trustHostSetup(t *testing.T) trustHostEnv {
	t.Helper()
	// Like trustServer, but with an SPA file tree set before the routes register.
	s := testServer(t)
	s.secret = "trust-public-test-secret-0123456789abcdef"
	s.webFS = fstest.MapFS{
		"index.html":    {Data: []byte(trustHostIndex)},
		"assets/app.js": {Data: []byte("console.log(1)")},
		"favicon.svg":   {Data: []byte("<svg/>")},
	}
	s.subdomainRouting = true
	s.echo = echo.New()
	s.useHostResolution(trustTestBase, true)
	s.echo.Use(AuthMiddleware(AuthConfig{DB: s.db, Secret: s.secret}))
	s.echo.Use(s.RoleMiddleware())
	store := blob.NewLocal(t.TempDir())
	s.blobs = store
	s.routes()
	a := trustNewOrg(t, s, "tha", true)
	b := trustNewOrg(t, s, "thb", true)
	e := trustHostEnv{s: s, a: a, b: b, trust: fmt.Sprintf("trust-%d.trusthost.test", time.Now().UnixNano()), logoA: "<svg>logo-a</svg>", logoB: "<svg>logo-b</svg>"}
	ctx := context.Background()
	if err := s.db.SetOrganizationTrustDomain(ctx, a.id, &e.trust); err != nil {
		t.Fatal(err)
	}
	for _, o := range []struct {
		o    trustOrg
		logo string
	}{{a, e.logoA}, {b, e.logoB}} {
		org, err := s.db.GetOrganization(ctx, o.o.id)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Put(ctx, org.UUID, "branding/logo.svg", []byte(o.logo)); err != nil {
			t.Fatal(err)
		}
	}
	tok := func(o trustOrg) string {
		u := contractTestUser(t, s, o.id, fmt.Sprintf("th-%s-%d@example.com", o.slug, time.Now().UnixNano()), "manager")
		j, err := s.createSessionJWT(u, o.id, "manager", o.slug, "Org")
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	e.mgrTok, e.bTok = tok(a), tok(b)
	return e
}

func TestTrustHostGate(t *testing.T) {
	e := trustHostSetup(t)
	api := "/api/v1/trust/" + e.a.slug
	allowed := []struct{ method, path string }{
		{"GET", api},
		{"GET", api + "/items"},
		{"GET", "/"},
		{"GET", "/sign-in"},
		{"GET", "/documents/x"},
		{"GET", "/assets/app.js"},
		{"GET", "/favicon.svg"},
		{"GET", "/index.html"},
		{"GET", "/branding/logo"},
	}
	for _, c := range allowed {
		if r := trustDo(e.s, e.trust, c.method, c.path, ""); r.code != http.StatusOK {
			t.Errorf("%s %s = %d, want 200 (%s)", c.method, c.path, r.code, r.body)
		}
	}
	denied := []struct{ method, path, bearer string }{
		{"GET", "/git/00000000-0000-0000-0000-000000000000/info/refs", ""},
		{"GET", "/api/v1/trust-center/settings", e.mgrTok},
		{"GET", "/api/v1/trust-centerx", ""},
		{"POST", "/api/v1/auth/login", ""},
		{"GET", "/api/v1/config", ""},
		{"GET", "/api/v1/auth/oidc/providers", ""},
		{"GET", "/api/openapi.yaml", ""},
		{"GET", "/docs", ""},
		{"GET", "/healthz", ""},
		{"GET", "/terms", ""},
		{"GET", "/privacy", ""},
		{"GET", "/login", ""},
		{"GET", "/trust", ""},
		{"GET", "/trust/sign-in", ""},
		{"GET", "/" + e.a.slug + "/trust", ""},
		{"GET", "/" + e.a.slug + "/dashboard", ""},
		{"POST", "/", ""},
		{"POST", "/sign-in", ""},
		{"PUT", "/documents/x", ""},
		{"POST", "/branding/logo", ""},
		{"GET", "/documents", ""},
		{"GET", "/documents/", ""},
		{"GET", "/documents/a/b", ""},
		{"GET", "/assets/", ""},
		{"GET", "/assets/missing.js", ""},
		{"GET", "/assets/../index.html", ""},
		{"GET", "/%2e%2e/etc/passwd", ""},
		{"GET", "/branding/other", ""},
	}
	for _, c := range denied {
		r := trustDo(e.s, e.trust, c.method, c.path, c.bearer)
		if r.code != http.StatusNotFound || r.body != "not found\n" {
			t.Errorf("%s %s = %d %q, want the gate's 404", c.method, c.path, r.code, r.body)
		}
	}
}

func TestTrustHostServesTheOrgsOwnPages(t *testing.T) {
	e := trustHostSetup(t)
	r := trustDo(e.s, e.trust, "GET", "/", "")
	want := `<meta name="isms-trust-host" content="` + e.a.slug + `">`
	if !strings.Contains(r.body, want) {
		t.Errorf("index lacks %s:\n%s", want, r.body)
	}
	for _, p := range []string{"/sign-in", "/documents/abc", "/index.html"} {
		if r := trustDo(e.s, e.trust, "GET", p, ""); !strings.Contains(r.body, want) {
			t.Errorf("%s lacks the trust-host meta", p)
		}
	}
	if r := trustDo(e.s, e.trust, "GET", "/documents/abc?go-get=1", ""); strings.Contains(r.body, "go-import") {
		t.Error("the go-get vanity page is served on a trust host")
	}
	if r := trustDo(e.s, e.trust, "GET", "/assets/app.js", ""); r.body != "console.log(1)" {
		t.Errorf("static file body = %q", r.body)
	}
	if r := trustDo(e.s, e.trust, "GET", "/branding/logo", ""); r.body != e.logoA {
		t.Errorf("logo = %q, want A's", r.body)
	}
}

func TestTrustHostCaseAndPortAreIgnored(t *testing.T) {
	e := trustHostSetup(t)
	for _, host := range []string{strings.ToUpper(e.trust), e.trust + ":8443", strings.ToUpper(e.trust) + ":1"} {
		if r := trustDo(e.s, host, "GET", "/", ""); !strings.Contains(r.body, `isms-trust-host" content="`+e.a.slug) {
			t.Errorf("host %q: / = %d, no trust meta", host, r.code)
		}
		if r := trustDo(e.s, host, "GET", "/login", ""); r.code != http.StatusNotFound {
			t.Errorf("host %q: /login = %d, want 404", host, r.code)
		}
		if r := trustDo(e.s, host, "GET", "/api/v1/trust/"+e.a.slug, ""); r.code != http.StatusOK {
			t.Errorf("host %q: trust API = %d", host, r.code)
		}
	}
}

func TestTrustHostForeignSlugIs404(t *testing.T) {
	e := trustHostSetup(t)
	for _, p := range []string{"", "/items"} {
		if r := trustDo(e.s, e.trust, "GET", "/api/v1/trust/"+e.b.slug+p, ""); r.code != http.StatusNotFound {
			t.Errorf("org B's slug on A's trust host %q = %d, want 404", p, r.code)
		}
	}
}

func TestTrustHostBearerOfAnotherOrgDoesNotSwitchTheOrg(t *testing.T) {
	e := trustHostSetup(t)
	for _, tok := range []string{e.bTok, e.mgrTok} {
		r := trustDo(e.s, e.trust, "GET", "/branding/logo", tok)
		if r.body != e.logoA {
			t.Errorf("logo with a bearer = %q, want A's logo", r.body)
		}
	}
	// control: on B's own app host the same bearer is honoured (soft auth intact).
	if r := trustDo(e.s, trustTestBase, "GET", "/branding/logo", e.bTok); r.body != e.logoB {
		t.Errorf("control: soft auth on the apex = %q, want B's logo", r.body)
	}
}

func TestTrustHostAppHostControls(t *testing.T) {
	e := trustHostSetup(t)
	for name, host := range map[string]string{"app domain": e.a.domain, "app domain upper": strings.ToUpper(e.a.domain)} {
		if r := trustDo(e.s, host, "GET", "/login", ""); r.code != http.StatusOK || strings.Contains(r.body, "isms-trust-host") {
			t.Errorf("%s: /login = %d, trust meta present=%v", name, r.code, strings.Contains(r.body, "isms-trust-host"))
		}
		r := trustDo(e.s, host, "GET", "/api/v1/config", "")
		var cfg struct {
			Org struct {
				Slug string `json:"slug"`
			} `json:"organization"`
		}
		if r.code != http.StatusOK || !strings.Contains(r.body, e.a.slug) {
			t.Errorf("%s: /config = %d, no slug %s in %s", name, r.code, e.a.slug, r.body)
		}
		_ = cfg
		if r := trustDo(e.s, host, "GET", "/trust", ""); strings.Contains(r.body, "isms-trust-host") {
			t.Errorf("%s: /trust carries the trust-host meta", name)
		}
	}
	// apex index: no trust-host meta, same bytes as the previous output
	want := strings.Replace(trustHostIndex, "</head>", `<meta name="isms-apex" content="`+trustTestBase+`"><meta name="isms-subdomain-routing" content="true"></head>`, 1)
	e.s.apexHost = trustTestBase
	if r := trustDo(e.s, trustTestBase, "GET", "/", ""); r.body != want {
		t.Errorf("apex index changed:\n got %q\nwant %q", r.body, want)
	}
	// the app host keeps serving the trust page and API
	for name, host := range trustModes(e.a) {
		if r := trustDo(e.s, host, "GET", "/api/v1/trust/"+e.a.slug, ""); r.code != http.StatusOK {
			t.Errorf("%s: trust API = %d", name, r.code)
		}
	}
	if r := trustDo(e.s, trustTestBase, "GET", "/"+e.a.slug+"/trust", ""); r.code != http.StatusOK {
		t.Errorf("path-mode trust page = %d", r.code)
	}
}

func TestTrustHostChangeAndClearApplyImmediately(t *testing.T) {
	e := trustHostSetup(t)
	ctx := context.Background()
	if r := trustDo(e.s, e.trust, "GET", "/login", ""); r.code != http.StatusNotFound {
		t.Fatalf("before: %d", r.code)
	}
	next := fmt.Sprintf("trust2-%d.trusthost.test", time.Now().UnixNano())
	if err := e.s.db.SetOrganizationTrustDomain(ctx, e.a.id, &next); err != nil {
		t.Fatal(err)
	}
	if r := trustDo(e.s, next, "GET", "/login", ""); r.code != http.StatusNotFound {
		t.Errorf("new host /login = %d, want the gate's 404", r.code)
	}
	if r := trustDo(e.s, next, "GET", "/api/v1/trust/"+e.a.slug, ""); r.code != http.StatusOK {
		t.Errorf("new host trust API = %d", r.code)
	}
	// the old host is no longer a trust host: it falls to the landing context
	if r := trustDo(e.s, e.trust, "GET", "/api/v1/trust/"+e.a.slug, ""); strings.Contains(r.body, "isms-trust-host") || r.code == http.StatusInternalServerError {
		t.Errorf("old host still treated as trust host: %d", r.code)
	}
	if r := trustDo(e.s, e.trust, "GET", "/login", ""); r.code != http.StatusOK {
		t.Errorf("old host /login = %d, want the app's SPA (200)", r.code)
	}
	if err := e.s.db.SetOrganizationTrustDomain(ctx, e.a.id, nil); err != nil {
		t.Fatal(err)
	}
	if r := trustDo(e.s, next, "GET", "/login", ""); r.code != http.StatusOK {
		t.Errorf("cleared host /login = %d, want the app's SPA (200)", r.code)
	}
}

// A database error during the host lookup must not fall through to app routing.
func TestTrustHostLookupErrorFailsClosed(t *testing.T) {
	s := testServer(t)
	dead := testServer(t)
	dead.db.Close() // every query on this pool now errors
	s.db = dead.db
	s.webFS = fstest.MapFS{"index.html": {Data: []byte(trustHostIndex)}}
	s.echo = echo.New()
	s.useHostResolution(trustTestBase, true)
	s.routes()
	for _, p := range []string{"/", "/login", "/api/v1/config", "/git/x/info/refs"} {
		req := httptest.NewRequest(http.MethodGet, "http://trust.example.test"+p, nil)
		rec := httptest.NewRecorder()
		s.echo.ServeHTTP(rec, req)
		if rec.Code != http.StatusInternalServerError || rec.Body.String() != "internal error\n" {
			t.Errorf("%s on a host whose lookup failed = %d %q, want the fixed 500", p, rec.Code, rec.Body.String())
		}
	}
}

func TestTrustHostSignInLinkAndCookie(t *testing.T) {
	e := visitorSetup(t)
	trust := fmt.Sprintf("trust-%d.visitor.test", time.Now().UnixNano())
	if err := e.s.db.SetOrganizationTrustDomain(context.Background(), e.org.id, &trust); err != nil {
		t.Fatal(err)
	}
	post := func(host, body string) trustResp {
		req := httptest.NewRequest(http.MethodPost, "http://"+host+e.path("/visitor/link"), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.s.echo.ServeHTTP(rec, req)
		return trustResp{code: rec.Code, body: rec.Body.String(), hdr: rec.Header()}
	}
	n := 0
	link := func(host string) string {
		n++
		if r := post(host, `{"email":"ada@auditco.test"}`); r.code != http.StatusAccepted {
			t.Fatalf("link on %s = %d %s", host, r.code, r.body)
		}
		return waitSinkMail(t, e.sink, n)[n-1].Body
	}
	if body := link(strings.ToUpper(trust) + ":8443"); !strings.Contains(body, "http://"+trust+"/sign-in#visitor_token=") {
		t.Errorf("trust-host mail lacks the root sign-in link:\n%s", body)
	}
	if body := link("evil.test"); !strings.Contains(body, "http://"+trustTestBase+"/"+e.org.slug+"/trust/sign-in#visitor_token=") || strings.Contains(body, "evil.test") {
		t.Errorf("forged host got a non-path link:\n%s", body)
	}
	if body := link(trustTestBase); !strings.Contains(body, "http://"+trustTestBase+"/"+e.org.slug+"/trust/sign-in#") {
		t.Errorf("path form changed:\n%s", body)
	}

	// redeem on the trust host: host-only cookie scoped to the org's API path
	tok := tokenFromMail(t, e.sink.messages()[0])
	req := httptest.NewRequest(http.MethodPost, "http://"+trust+e.path("/visitor/session"), strings.NewReader(`{"token":"`+tok+`","name":"Ada","company":"Audit Co"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.s.echo.ServeHTTP(rec, req)
	c := visitorCookieOf(t, trustResp{code: rec.Code, hdr: rec.Header()})
	if rec.Code != http.StatusOK || c == nil {
		t.Fatalf("session on trust host = %d %s", rec.Code, rec.Body.String())
	}
	if c.Domain != "" || c.Path != "/api/v1/trust/"+e.org.slug {
		t.Errorf("cookie Domain=%q Path=%q, want host-only and /api/v1/trust/%s", c.Domain, c.Path, e.org.slug)
	}
}

func TestTrustHostSettingsExposeTheDomainReadOnly(t *testing.T) {
	e := mgrSetup(t)
	get := func() map[string]any {
		r := e.do("manager", http.MethodGet, "/settings", "")
		var m map[string]any
		if r.code != http.StatusOK || json.Unmarshal([]byte(r.body), &m) != nil {
			t.Fatalf("GET settings = %d %s", r.code, r.body)
		}
		return m
	}
	if v, ok := get()["trust_domain"]; !ok || v != nil {
		t.Errorf("unset trust_domain = %v (present %v), want JSON null", v, ok)
	}
	trust := fmt.Sprintf("trust-%d.settings.test", time.Now().UnixNano())
	if err := e.s.db.SetOrganizationTrustDomain(context.Background(), e.org.id, &trust); err != nil {
		t.Fatal(err)
	}
	if v := get()["trust_domain"]; v != trust {
		t.Errorf("trust_domain = %v, want %q", v, trust)
	}
	r := e.do("manager", http.MethodPut, "/settings", `{"title":"New","trust_domain":"evil.example.test"}`)
	if r.code != http.StatusOK {
		t.Fatalf("PUT = %d %s", r.code, r.body)
	}
	m := get()
	if m["trust_domain"] != trust || m["title"] != "New" {
		t.Errorf("after PUT: trust_domain=%v title=%v", m["trust_domain"], m["title"])
	}
	if err := e.s.db.SetOrganizationTrustDomain(context.Background(), e.org.id, nil); err != nil {
		t.Fatal(err)
	}
	if v, ok := get()["trust_domain"]; !ok || v != nil {
		t.Errorf("cleared trust_domain = %v, want null", v)
	}
}

// The gate decision needs no database.
func TestTrustHostAllowedDecision(t *testing.T) {
	spa := fstest.MapFS{"index.html": {Data: []byte("x")}, "assets/a.js": {Data: []byte("x")}}
	cases := []struct {
		method, path string
		want         bool
	}{
		{"GET", "/", true}, {"HEAD", "/sign-in", true}, {"GET", "/documents/x", true},
		{"GET", "/assets/a.js", true}, {"GET", "/api/v1/trust/acme/items", true}, {"POST", "/api/v1/trust/acme/visitor/link", true},
		{"GET", "/branding/logo", true}, {"GET", "/favicon.ico", true},
		{"POST", "/", false}, {"GET", "/api/v1/trust-center/items", false}, {"GET", "/api/v1/config", false},
		{"GET", "/documents/a/b", false}, {"GET", "/documents/", false}, {"GET", "/trust", false},
		{"GET", "/assets/../index.html", false}, {"GET", "/assets", false}, {"GET", "//assets/a.js", false},
		{"GET", "/login", false}, {"GET", "/git/x", false},
	}
	for _, c := range cases {
		if got := trustHostAllowed(c.method, c.path, spa); got != c.want {
			t.Errorf("trustHostAllowed(%s %s) = %v, want %v", c.method, c.path, got, c.want)
		}
	}
	if trustHostAllowed("GET", "/assets/a.js", nil) {
		t.Error("a static file is allowed without an SPA filesystem")
	}
}

// The public overview carries the supported locales, so the page can build its
// language picker on a host that does not serve /api/v1/config.
func TestTrustPublicOverviewCarriesSupportedLocales(t *testing.T) {
	e := trustHostSetup(t)
	r := trustDo(e.s, e.trust, "GET", "/api/v1/trust/"+e.a.slug, "")
	var o struct {
		Locales []i18n.Locale `json:"locales"`
	}
	if r.code != http.StatusOK || json.Unmarshal([]byte(r.body), &o) != nil {
		t.Fatalf("overview = %d %s", r.code, r.body)
	}
	want := i18n.Supported()
	if len(o.Locales) == 0 || len(o.Locales) != len(want) {
		t.Fatalf("locales = %v, want %v", o.Locales, want)
	}
	for i := range want {
		if o.Locales[i] != want[i] {
			t.Errorf("locales[%d] = %v, want %v", i, o.Locales[i], want[i])
		}
	}
}
