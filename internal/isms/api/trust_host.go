package api

import (
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

// trustHostKey is the echo context key OrgResolverMiddleware sets when the
// request arrived on an organization's trust domain.
const trustHostKey = "trust_host"

func isTrustHost(c echo.Context) bool {
	v, _ := c.Get(trustHostKey).(bool)
	return v
}

// useHostResolution mounts the org resolver and the trust-host gate, in that
// order. Production and the tests both call it so the chain cannot drift.
func (s *Server) useHostResolution(baseDomain string, subdomainRouting bool) {
	s.echo.Use(OrgResolverMiddleware(s.db, baseDomain, subdomainRouting))
	s.echo.Use(s.trustHostGate())
}

// spaFileSystem is the SPA's file tree: webDir (disk) before webFS (embedded),
// nil when neither is set.
func (s *Server) spaFileSystem() fs.FS {
	if s.webDir != "" {
		return os.DirFS(s.webDir)
	}
	if s.webFS != nil {
		return s.webFS
	}
	return nil
}

// trustHostPage reports whether path is one of the SPA pages a trust host
// serves. Keep in step with buildTrustHostRoutes in web/src/trustRoutes.js:
// "/", "/sign-in" and "/documents/<id>" (exactly one non-empty segment). The
// SPA side is the same three shapes.
func trustHostPage(path string) bool {
	switch path {
	case "/", "/sign-in":
		return true
	}
	if id, ok := strings.CutPrefix(path, "/documents/"); ok {
		return id != "" && !strings.Contains(id, "/")
	}
	return false
}

// trustHostBranding are the branding files a trust host may serve.
func trustHostBranding(path string) bool {
	return path == "/branding/logo" || path == "/branding/favicon.ico" || path == "/favicon.ico"
}

// trustHostStaticFile reports whether path names an existing file of the SPA
// (assets, favicon.svg, ...). The path must be clean: no "..", no empty segment.
func trustHostStaticFile(spa fs.FS, path string) bool {
	if spa == nil || !strings.HasPrefix(path, "/") {
		return false
	}
	name := path[1:]
	if !fs.ValidPath(name) || name == "." {
		return false
	}
	info, err := fs.Stat(spa, name)
	return err == nil && !info.IsDir()
}

// trustHostAllowed decides whether a request on a trust host is answered at all.
func trustHostAllowed(method, path string, spa fs.FS) bool {
	// The whole public trust API, any method: the handler has its own method
	// rules and pins the organization to the host. Never a string prefix: the
	// member API /api/v1/trust-center shares it.
	if trustPublicPath(path) {
		return true
	}
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	return trustHostBranding(path) || trustHostPage(path) || trustHostStaticFile(spa, path)
}

// trustHostGate answers 404 to everything a trust host does not serve, before
// any handler runs. It acts only when the resolver marked the request.
func (s *Server) trustHostGate() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !isTrustHost(c) {
				return next(c)
			}
			if !trustHostAllowed(c.Request().Method, c.Request().URL.Path, s.spaFileSystem()) {
				return c.String(http.StatusNotFound, "not found\n")
			}
			return next(c)
		}
	}
}
