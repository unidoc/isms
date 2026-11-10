package api

// The trust center read API is served without a session, so these tests drive
// the real chain a request takes (OrgResolverMiddleware, AuthMiddleware,
// RoleMiddleware, the registered routes) with Host headers, the way a browser
// reaches it, rather than setting org_id by hand. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TrustPublic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

const trustTestBase = "isms.test"

type trustOrg struct {
	id     int
	slug   string
	domain string
}

// trustServer returns a Server whose echo instance carries the production
// middleware order for a deployment with subdomain routing on.
func trustServer(t *testing.T) *Server {
	t.Helper()
	s := testServer(t)
	s.secret = "trust-public-test-secret-0123456789abcdef"
	s.echo = echo.New()
	s.echo.Use(OrgResolverMiddleware(s.db, trustTestBase, true))
	s.echo.Use(AuthMiddleware(AuthConfig{DB: s.db, Secret: s.secret}))
	s.echo.Use(s.RoleMiddleware())
	s.routes()
	return s
}

// trustNewOrg creates an organization with a custom domain. When enabled, its
// trust center is switched on.
func trustNewOrg(t *testing.T, s *Server, tag string, enabled bool) trustOrg {
	t.Helper()
	n := time.Now().UnixNano()
	slug := fmt.Sprintf("tcp-%s-%d", tag, n)
	domain := fmt.Sprintf("trust-%s-%d.example.test", tag, n)
	org := &db.Organization{Name: "Org " + tag, Slug: slug, RepoPath: "/tmp/none", Domain: &domain}
	if err := s.db.CreateOrganization(context.Background(), org); err != nil {
		t.Fatalf("create org %s: %v", tag, err)
	}
	t.Cleanup(func() { _ = s.db.DeleteOrganization(context.Background(), org.ID) })
	if enabled {
		st := &db.TrustCenterSettings{Enabled: true, Title: "Trust " + tag, SecurityContact: tag + "@sec.test", DefaultAccessLevel: db.TrustAccessGated}
		if err := s.db.UpsertTrustCenterSettings(context.Background(), org.ID, st, "mgr@example.com"); err != nil {
			t.Fatalf("settings %s: %v", tag, err)
		}
	}
	return trustOrg{id: org.ID, slug: slug, domain: domain}
}

func trustPublishItem(t *testing.T, s *Server, o trustOrg, kind, ref, level string) *db.TrustCenterItem {
	t.Helper()
	it := &db.TrustCenterItem{Kind: kind, Reference: ref, AccessLevel: level, Section: "policies", SortOrder: 1}
	if err := s.db.PublishTrustCenterItem(context.Background(), o.id, it, "mgr@example.com"); err != nil {
		t.Fatalf("publish %s: %v", ref, err)
	}
	return it
}

type trustResp struct {
	code int
	body string
	hdr  http.Header
}

func trustDo(s *Server, host, method, path, bearer string) trustResp {
	req := httptest.NewRequest(method, "http://"+host+path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	return trustResp{code: rec.Code, body: rec.Body.String(), hdr: rec.Header()}
}

type trustItemJSON struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Section     string `json:"section"`
	SortOrder   int    `json:"sort_order"`
	AccessLevel string `json:"access_level"`
}

func trustItems(t *testing.T, r trustResp) []trustItemJSON {
	t.Helper()
	if r.code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", r.code, r.body)
	}
	var items []trustItemJSON
	if err := json.Unmarshal([]byte(r.body), &items); err != nil {
		t.Fatalf("decode %q: %v", r.body, err)
	}
	return items
}

// modes returns, for org o, the Host a request uses in each routing mode.
func trustModes(o trustOrg) map[string]string {
	return map[string]string{
		"path":      trustTestBase,
		"subdomain": o.slug + "." + trustTestBase,
		"custom":    o.domain,
	}
}

func TestTrustPublicItemsPerOrgInEveryMode(t *testing.T) {
	s := trustServer(t)
	a := trustNewOrg(t, s, "a", true)
	b := trustNewOrg(t, s, "b", true)
	ia := trustPublishItem(t, s, a, db.TrustKindCertification, "doc-a", db.TrustAccessPublic)
	trustPublishItem(t, s, a, db.TrustKindFAQ, "faq-a", db.TrustAccessGated)
	trustPublishItem(t, s, b, db.TrustKindCertification, "doc-b", db.TrustAccessPublic)

	for mode, host := range trustModes(a) {
		t.Run(mode, func(t *testing.T) {
			r := trustDo(s, host, http.MethodGet, "/api/v1/trust/"+a.slug+"/items", "")
			items := trustItems(t, r) // no credentials: g, never a 401
			if len(items) != 2 {
				t.Fatalf("items = %d, want 2 (A's only): %s", len(items), r.body)
			}
			var sawGated bool
			for _, it := range items {
				if it.AccessLevel == db.TrustAccessGated {
					sawGated = true
				}
			}
			if !sawGated {
				t.Error("gated item must be listed so the page can show it locked")
			}
			if strings.Contains(r.body, "doc-b") || strings.Contains(r.body, "doc-a") || strings.Contains(r.body, "mgr@example.com") {
				t.Errorf("response leaks reference or publisher: %s", r.body)
			}
			if strings.Contains(r.body, fmt.Sprintf(`"id":%d`, ia.ID)) {
				t.Errorf("response exposes the serial id: %s", r.body)
			}
			var found bool
			for _, it := range items {
				found = found || it.ID == ia.PublicID
			}
			if !found {
				t.Errorf("public_id %s missing: %s", ia.PublicID, r.body)
			}
			if cc := r.hdr.Get("Cache-Control"); cc != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", cc)
			}
		})
	}
}

func TestTrustPublicOverview(t *testing.T) {
	s := trustServer(t)
	a := trustNewOrg(t, s, "ov", true)
	for mode, host := range trustModes(a) {
		r := trustDo(s, host, http.MethodGet, "/api/v1/trust/"+a.slug, "")
		if r.code != http.StatusOK {
			t.Fatalf("%s: status %d body %s", mode, r.code, r.body)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(r.body), &got); err != nil {
			t.Fatal(err)
		}
		if got["title"] != "Trust ov" || got["security_contact"] != "ov@sec.test" || got["branding_name"] != "Org ov" {
			t.Errorf("%s: body = %s", mode, r.body)
		}
		if _, ok := got["branding_color"]; !ok || len(got) != 4 {
			t.Errorf("%s: want exactly title, security_contact, branding_name, branding_color: %s", mode, r.body)
		}
		if !strings.HasPrefix(r.hdr.Get("Content-Type"), "application/json") {
			t.Errorf("content type = %q", r.hdr.Get("Content-Type"))
		}
	}
}

func TestTrustPublicEmptyListIsArray(t *testing.T) {
	s := trustServer(t)
	a := trustNewOrg(t, s, "empty", true)
	r := trustDo(s, trustTestBase, http.MethodGet, "/api/v1/trust/"+a.slug+"/items", "")
	if r.code != http.StatusOK || strings.TrimSpace(r.body) != "[]" {
		t.Errorf("status %d body %q, want 200 []", r.code, r.body)
	}
}

func TestTrustPublicHostMustMatchSlug(t *testing.T) {
	s := trustServer(t)
	a := trustNewOrg(t, s, "ha", true)
	b := trustNewOrg(t, s, "hb", true)
	trustPublishItem(t, s, b, db.TrustKindCertification, "doc-b", db.TrustAccessPublic)

	want := trustDo(s, trustTestBase, http.MethodGet, "/api/v1/trust/no-such-org-xyz", "")
	for _, host := range []string{a.slug + "." + trustTestBase, a.domain} {
		for _, suffix := range []string{"", "/items"} {
			r := trustDo(s, host, http.MethodGet, "/api/v1/trust/"+b.slug+suffix, "")
			if r.code != http.StatusNotFound || r.body != want.body {
				t.Errorf("host %s path %q: status %d body %q, want 404 %q", host, suffix, r.code, r.body, want.body)
			}
		}
	}
	// B's own hosts still work.
	for mode, host := range trustModes(b) {
		if r := trustDo(s, host, http.MethodGet, "/api/v1/trust/"+b.slug+"/items", ""); r.code != http.StatusOK {
			t.Errorf("%s: B's own host gives %d", mode, r.code)
		}
	}
}

func TestTrustPublicOneNotFoundBody(t *testing.T) {
	s := trustServer(t)
	a := trustNewOrg(t, s, "nf", true)
	b := trustNewOrg(t, s, "nfb", true)
	disabled := trustNewOrg(t, s, "dis", true)
	noRow := trustNewOrg(t, s, "norow", false)
	gone := trustNewOrg(t, s, "gone", true)
	trustPublishItem(t, s, a, db.TrustKindCertification, "doc-a", db.TrustAccessPublic)
	if err := s.db.UpsertTrustCenterSettings(context.Background(), disabled.id,
		&db.TrustCenterSettings{Enabled: false, DefaultAccessLevel: db.TrustAccessGated}, "m"); err != nil {
		t.Fatal(err)
	}
	if err := s.db.DeleteOrganization(context.Background(), gone.id); err != nil {
		t.Fatal(err)
	}

	type tc struct{ name, host, method, path string }
	cases := []tc{
		{"unknown slug", trustTestBase, "GET", "/api/v1/trust/no-such-org-xyz"},
		{"unknown slug items", trustTestBase, "GET", "/api/v1/trust/no-such-org-xyz/items"},
		{"disabled", trustTestBase, "GET", "/api/v1/trust/" + disabled.slug},
		{"disabled items", trustTestBase, "GET", "/api/v1/trust/" + disabled.slug + "/items"},
		{"no settings row", trustTestBase, "GET", "/api/v1/trust/" + noRow.slug},
		{"deleted org", trustTestBase, "GET", "/api/v1/trust/" + gone.slug + "/items"},
		{"host mismatch subdomain", b.slug + "." + trustTestBase, "GET", "/api/v1/trust/" + a.slug + "/items"},
		{"host mismatch domain", b.domain, "GET", "/api/v1/trust/" + a.slug},
		{"unknown sub-path", trustTestBase, "GET", "/api/v1/trust/" + a.slug + "/whatever"},
		{"deeper sub-path", trustTestBase, "GET", "/api/v1/trust/" + a.slug + "/items/extra"},
		{"trailing slash", trustTestBase, "GET", "/api/v1/trust/" + a.slug + "/"},
		{"empty slug", trustTestBase, "GET", "/api/v1/trust/"},
		{"bare prefix", trustTestBase, "GET", "/api/v1/trust"},
		{"POST items", trustTestBase, "POST", "/api/v1/trust/" + a.slug + "/items"},
		{"POST overview", trustTestBase, "POST", "/api/v1/trust/" + a.slug},
		{"PUT items", trustTestBase, "PUT", "/api/v1/trust/" + a.slug + "/items"},
		{"DELETE overview", trustTestBase, "DELETE", "/api/v1/trust/" + a.slug},
		{"PATCH items", trustTestBase, "PATCH", "/api/v1/trust/" + a.slug + "/items"},
		{"POST unknown slug", trustTestBase, "POST", "/api/v1/trust/no-such-org-xyz"},
	}
	var want string
	for i, c := range cases {
		r := trustDo(s, c.host, c.method, c.path, "")
		if r.code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404; body %s", c.name, r.code, r.body)
			continue
		}
		if i == 0 {
			want = r.body
			var b struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			}
			if err := json.Unmarshal([]byte(want), &b); err != nil || b.Message == "" || b.Code == "" {
				t.Fatalf("reference 404 body %q is not an apiError body", want)
			}
		}
		if r.body != want {
			t.Errorf("%s: body %q differs from %q", c.name, r.body, want)
		}
		if cc := r.hdr.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("%s: Cache-Control = %q", c.name, cc)
		}
		if ct := r.hdr.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: content type %q", c.name, ct)
		}
	}
}

func TestTrustPublicUnpublishTakesEffect(t *testing.T) {
	s := trustServer(t)
	a := trustNewOrg(t, s, "unp", true)
	keep := trustPublishItem(t, s, a, db.TrustKindCertification, "keep", db.TrustAccessPublic)
	drop := trustPublishItem(t, s, a, db.TrustKindCertification, "drop", db.TrustAccessPublic)

	path := "/api/v1/trust/" + a.slug + "/items"
	if got := trustItems(t, trustDo(s, trustTestBase, "GET", path, "")); len(got) != 2 {
		t.Fatalf("before = %d items", len(got))
	}
	if err := s.db.UnpublishTrustCenterItem(context.Background(), a.id, drop.ID, "mgr@example.com"); err != nil {
		t.Fatal(err)
	}
	got := trustItems(t, trustDo(s, trustTestBase, "GET", path, ""))
	if len(got) != 1 || got[0].ID != keep.PublicID {
		t.Errorf("after unpublish = %+v, want only %s", got, keep.PublicID)
	}
	// Turning the trust center off hides everything at once.
	if err := s.db.UpsertTrustCenterSettings(context.Background(), a.id,
		&db.TrustCenterSettings{Enabled: false, DefaultAccessLevel: db.TrustAccessGated}, "m"); err != nil {
		t.Fatal(err)
	}
	if r := trustDo(s, trustTestBase, "GET", path, ""); r.code != http.StatusNotFound {
		t.Errorf("after disable: %d", r.code)
	}
}

func TestTrustPublicIgnoresBearerOfAnotherOrg(t *testing.T) {
	s := trustServer(t)
	a := trustNewOrg(t, s, "bea", true)
	b := trustNewOrg(t, s, "beb", true)
	trustPublishItem(t, s, a, db.TrustKindCertification, "doc-a", db.TrustAccessPublic)
	trustPublishItem(t, s, b, db.TrustKindCertification, "doc-b", db.TrustAccessPublic)
	trustPublishItem(t, s, b, db.TrustKindFAQ, "faq-b", db.TrustAccessPublic)

	u := contractTestUser(t, s, b.id, fmt.Sprintf("tcp-bearer-%d@example.com", time.Now().UnixNano()), "manager")
	tok, err := s.createSessionJWT(u, b.id, "manager", b.slug, "Org beb")
	if err != nil {
		t.Fatal(err)
	}
	for mode, host := range trustModes(a) {
		anon := trustDo(s, host, "GET", "/api/v1/trust/"+a.slug+"/items", "")
		with := trustDo(s, host, "GET", "/api/v1/trust/"+a.slug+"/items", tok)
		if with.code != http.StatusOK || with.body != anon.body {
			t.Errorf("%s: bearer changed the result: %d %q vs %q", mode, with.code, with.body, anon.body)
		}
	}
	// A garbage bearer must not turn the public route into a 401 either.
	if r := trustDo(s, trustTestBase, "GET", "/api/v1/trust/"+a.slug, "isms_not-a-real-key"); r.code != http.StatusOK {
		t.Errorf("bogus API key gives %d, want 200", r.code)
	}
}

func TestTrustPublicDBFailureIsGeneric500(t *testing.T) {
	good := testServer(t)
	a := trustNewOrg(t, good, "f", true)

	// A second Server over a closed pool: every query fails with pgx text.
	bad := trustServer(t)
	closed, err := db.New(context.Background(), os.Getenv("ISMS_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	bad.db = closed

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	for _, path := range []string{"/api/v1/trust/" + a.slug, "/api/v1/trust/" + a.slug + "/items"} {
		r := trustDo(bad, trustTestBase, "GET", path, "")
		if r.code != http.StatusInternalServerError {
			t.Fatalf("%s: status %d, want 500; body %s", path, r.code, r.body)
		}
		for _, leak := range []string{"closed", "pool", "pgx", "SELECT", "organizations", "sql"} {
			if strings.Contains(strings.ToLower(r.body), strings.ToLower(leak)) {
				t.Errorf("%s: 500 body leaks %q: %s", path, leak, r.body)
			}
		}
		if r.hdr.Get("Cache-Control") != "no-store" {
			t.Errorf("%s: 500 lacks Cache-Control no-store", path)
		}
	}
	logged := buf.String()
	if !strings.Contains(logged, "closed pool") {
		t.Errorf("real error missing from log: %q", logged)
	}
}
