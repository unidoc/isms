package api

import (
	"strings"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// OrgResolverMiddleware resolves the organization from the request host or path.
// It runs BEFORE auth middleware and sets org_id + org_slug on the context.
//
// Resolution order:
//  1. Skip /git/ paths (UUID-based, handled by git handler)
//  2. Subdomain: acme.isms.sh → slug "acme" (only when subdomainRouting is enabled)
//  3. Custom domain: isms.unidoc.io → lookup by domain column
//  4. Path-based: /acme/dashboard → slug "acme", rewrite to /dashboard
//  5. No org → set "landing" flag (root domain / landing page)
func OrgResolverMiddleware(database *db.DB, baseDomain string, subdomainRouting bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			host := c.Request().Host
			// Remove port if present
			hostname := host
			if i := strings.LastIndex(host, ":"); i > 0 {
				hostname = host[:i]
			}

			path := c.Request().URL.Path

			// 1. Skip git URLs (org resolved by UUID in handler)
			if strings.HasPrefix(path, "/git/") {
				return next(c)
			}

			// 2. Check subdomain: acme.isms.sh → slug = "acme".
			// Only when this deployment serves tenant orgs on wildcard subdomains.
			// Disabling subdomainRouting (demo / dev) means subdomain hosts are
			// not recognised as org carriers — path-based resolution only.
			if subdomainRouting && baseDomain != "" && strings.HasSuffix(hostname, "."+baseDomain) {
				slug := strings.TrimSuffix(hostname, "."+baseDomain)
				if slug != "" && !strings.Contains(slug, ".") {
					org, err := database.GetOrganizationBySlug(c.Request().Context(), slug)
					if err == nil {
						c.Set("org_id", org.ID)
						c.Set("org_slug", org.Slug)
						return next(c)
					}
				}
			}

			// 3. Check custom domain
			if hostname != baseDomain && hostname != "localhost" && !strings.HasSuffix(hostname, "."+baseDomain) {
				org, err := database.GetOrganizationByDomain(c.Request().Context(), hostname)
				if err == nil {
					c.Set("org_id", org.ID)
					c.Set("org_slug", org.Slug)
					return next(c)
				}
			}

			// 4. Check path-based org: /acme/dashboard → org=acme, rewrite to /dashboard
			// Only for non-API, non-git, non-static-asset paths
			if !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/git/") &&
				!strings.HasPrefix(path, "/healthz") && !strings.HasPrefix(path, "/branding/") {
				parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)
				if len(parts) >= 1 && parts[0] != "" {
					slug := parts[0]
					// Skip obvious static file extensions
					if !looksLikeStaticFile(slug) {
						org, err := database.GetOrganizationBySlug(c.Request().Context(), slug)
						if err == nil {
							c.Set("org_id", org.ID)
							c.Set("org_slug", org.Slug)
							// Rewrite path: /acme/dashboard → /dashboard
							newPath := "/"
							if len(parts) > 1 && parts[1] != "" {
								newPath = "/" + parts[1]
							}
							c.Request().URL.Path = newPath
							return next(c)
						}
					}
				}
			}

			// 5. No org resolved — root/landing page context
			c.Set("landing", true)
			return next(c)
		}
	}
}

// looksLikeStaticFile returns true if the path segment looks like a static asset.
func looksLikeStaticFile(s string) bool {
	for _, ext := range []string{".js", ".css", ".png", ".jpg", ".svg", ".ico", ".woff", ".woff2", ".ttf", ".map", ".html"} {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}
