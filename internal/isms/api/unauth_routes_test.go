package api

// Inventory of every route that answers without credentials. AuthMiddleware
// lets a request through with no Authorization header only on a hard-coded
// path list, so a new public route (or a path-matching slip in that list)
// changes what the server exposes to the internet. This test builds the real
// route table, sends each route through the real AuthMiddleware with no
// credentials, and compares the routes that reach their handler against a
// committed list. Adding a public route means editing the list on purpose.
//
// No database is involved: without a bearer token AuthMiddleware never reads
// one, which the test pins by passing a nil DB (a lookup would panic).

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// unauthRoutes is the committed golden list, "METHOD path" with parameters
// substituted by the sample value used in the test.
var unauthRoutes = []string{
	"GET /api/v1/auth/cf-session",
	"GET /api/v1/config",
	"GET /api/v1/auth/oidc/authorize",
	"GET /api/v1/auth/oidc/callback",
	"GET /api/v1/auth/oidc/providers",
	"GET /branding/favicon.ico",
	"GET /branding/logo",
	"GET /favicon.ico",
	"GET /healthz",
	"GET /privacy",
	"GET /terms",
	"POST /api/v1/auth/forgot-password",
	"POST /api/v1/auth/login",
	"POST /api/v1/auth/passkey/login/begin",
	"POST /api/v1/auth/passkey/login/complete",
	"POST /api/v1/auth/signup",
	"POST /api/v1/auth/verify-email",
	"POST /api/v1/auth/verify-email-change",
	// Trust center public read API (#517): one handler registered for every
	// method, answering the same 404 for all but the two GET endpoints.
	"CONNECT /api/v1/trust",
	"CONNECT /api/v1/trust/sample",
	"DELETE /api/v1/trust",
	"DELETE /api/v1/trust/sample",
	"GET /api/v1/trust",
	"GET /api/v1/trust/sample",
	"HEAD /api/v1/trust",
	"HEAD /api/v1/trust/sample",
	"OPTIONS /api/v1/trust",
	"OPTIONS /api/v1/trust/sample",
	"PATCH /api/v1/trust",
	"PATCH /api/v1/trust/sample",
	"POST /api/v1/trust",
	"POST /api/v1/trust/sample",
	"PROPFIND /api/v1/trust",
	"PROPFIND /api/v1/trust/sample",
	"PUT /api/v1/trust",
	"PUT /api/v1/trust/sample",
	"REPORT /api/v1/trust",
	"REPORT /api/v1/trust/sample",
	"TRACE /api/v1/trust",
	"TRACE /api/v1/trust/sample",
}

func sampleRoutePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		switch {
		case strings.HasPrefix(part, ":"):
			parts[i] = "sample"
		case part == "*":
			parts[i] = "sample"
		}
	}
	return strings.Join(parts, "/")
}

func TestUnauthenticatedRouteInventory(t *testing.T) {
	s := &Server{echo: echo.New(), searchIndex: NewSearchIndex()}
	s.routes()

	probe := echo.New()
	reached := map[string]bool{}
	for _, r := range s.echo.Routes() {
		path := sampleRoutePath(r.Path)
		req := httptest.NewRequest(r.Method, path, nil)
		rec := httptest.NewRecorder()
		c := probe.NewContext(req, rec)
		mw := AuthMiddleware(AuthConfig{Secret: "inventory-test-secret-0123456789abcdef"})
		h := mw(func(c echo.Context) error {
			reached[r.Method+" "+path] = true
			return nil
		})
		if err := h(c); err != nil {
			// A rejection (401) is the expected outcome for protected routes.
			if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: unexpected error %v", r.Method, path, err)
			}
		}
	}

	var got []string
	for k := range reached {
		got = append(got, k)
	}
	sort.Strings(got)
	want := append([]string(nil), unauthRoutes...)
	sort.Strings(want)

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("routes reachable without credentials changed.\n got:\n  %s\nwant:\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// A path that only looks like it is under the trust center prefix must not
// inherit its authentication skip. Without credentials it has to be refused by
// AuthMiddleware before any handler (the trust handler included, which would
// otherwise reach a nil database here) runs.
func TestTrustPrefixDoesNotShieldTraversal(t *testing.T) {
	s := &Server{echo: echo.New(), searchIndex: NewSearchIndex()}
	s.echo.Use(AuthMiddleware(AuthConfig{Secret: "inventory-test-secret-0123456789abcdef"}))
	s.routes()

	for _, target := range []string{
		"/api/v1/trust/../risks",
		"/api/v1/trust/..%2Frisks",
		"/api/v1/trust/%2e%2e/risks",
		"/api/v1/trust/./../risks",
		"/api/v1/trust//../risks",
		"/api/v1/trust/acme/../../risks",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		s.echo.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401; body %s", target, rec.Code, rec.Body.String())
		}
	}
}

func TestTrustPublicPath(t *testing.T) {
	for path, want := range map[string]bool{
		"/api/v1/trust":                 true,
		"/api/v1/trust/":                true,
		"/api/v1/trust/acme":            true,
		"/api/v1/trust/acme/":           true,
		"/api/v1/trust/acme/items":      true,
		"/api/v1/trust/acme/whatever/x": true,
		"/api/v1/trusted":               false,
		"/api/v1/trustx/acme":           false,
		"/api/v1/risks":                 false,
		"/api/v1/trust/../risks":        false,
		"/api/v1/trust/./acme":          false,
		"/api/v1/trust//acme":           false,
		"/api/v1/trust/acme/..":         false,
		"/api/v1/trust/..":              false,
		"/api/v1/trust/a\\..\\risks":    false,
	} {
		if got := trustPublicPath(path); got != want {
			t.Errorf("trustPublicPath(%q) = %v, want %v", path, got, want)
		}
	}
}
