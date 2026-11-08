package api

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// The trust center read API is the one part of the server that answers people
// who are not signed in, so it is deliberately small and strict:
//
//   - AuthMiddleware skips authentication for this prefix (see trustPublicPath),
//     which means nothing here may read a user, role or token from the request.
//     A bearer header is ignored, not validated.
//   - Isolation does not come from a session. The organization is the one named
//     by the slug in the path, and when the request arrived on a tenant
//     subdomain or custom domain (OrgResolverMiddleware set org_id) the host's
//     organization must be the same one, so acme.example.com can never serve
//     another tenant's page.
//   - Every "no" answer is the same 404 body: unknown or deleted organization,
//     trust center off or never set up, host/slug mismatch, unknown path and
//     any method other than GET. A visitor cannot tell the cases apart.
//   - A 500 carries a fixed message. The real error goes to the log.
//   - Responses are never cacheable, so unpublishing takes effect immediately.

const trustPublicPrefix = "/api/v1/trust"

// trustNotFound is the single body behind every 404 under the prefix. It is
// built once so the bytes cannot differ between cases.
var trustNotFound = apiError(http.StatusNotFound, CodeNotFound, Entity("item"))

// trustPublicPath reports whether AuthMiddleware may skip authentication for
// path. The skip is only for a path that is already in its canonical form (a
// trailing slash aside): a path with "..", ".", or empty segments might be
// normalized by a proxy in front of us into a protected route, so it takes the
// normal authentication route instead.
func trustPublicPath(path string) bool {
	if path != trustPublicPrefix && !strings.HasPrefix(path, trustPublicPrefix+"/") {
		return false
	}
	rest := strings.TrimSuffix(strings.TrimPrefix(path, trustPublicPrefix), "/")
	if rest == "" {
		return true
	}
	for _, seg := range strings.Split(rest[1:], "/") {
		if seg == "" || seg == "." || seg == ".." || strings.Contains(seg, "\\") {
			return false
		}
	}
	return true
}

// trustPublicOverview is the response of GET /api/v1/trust/:slug.
type trustPublicOverview struct {
	Title           string `json:"title"`
	SecurityContact string `json:"security_contact"`
	BrandingName    string `json:"branding_name"`
	BrandingColor   string `json:"branding_color"`
}

// trustPublicItem is one element of the GET /api/v1/trust/:slug/items array.
// ID is the item's public UUID; the serial id, the reference, the publisher
// and the timestamps stay internal.
type trustPublicItem struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Section     string `json:"section"`
	SortOrder   int    `json:"sort_order"`
	AccessLevel string `json:"access_level"`
}

func trustPublicNotFound(c echo.Context) error {
	return c.JSON(http.StatusNotFound, trustNotFound.Message)
}

// trustPublicError logs err with enough context to find the request and sends
// the fixed 500 body.
func trustPublicError(c echo.Context, err error) error {
	req := c.Request()
	id := req.Header.Get(echo.HeaderXRequestID)
	if id == "" {
		id = c.Response().Header().Get(echo.HeaderXRequestID)
	}
	log.Printf("trust center public API: %s %s request_id=%q: %v", req.Method, req.URL.Path, id, err)
	return c.JSON(http.StatusInternalServerError, map[string]string{"message": "internal server error"})
}

// handleTrustPublic serves everything under /api/v1/trust. It is registered
// for every method so that the only two answers are the two GET endpoints and
// the one 404.
func (s *Server) handleTrustPublic(c echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")

	req := c.Request()
	if req.Method != http.MethodGet || !strings.HasPrefix(req.URL.Path, trustPublicPrefix+"/") {
		return trustPublicNotFound(c)
	}
	parts := strings.Split(strings.TrimPrefix(req.URL.Path, trustPublicPrefix+"/"), "/")
	slug := parts[0]
	var items bool
	switch {
	case slug == "":
		return trustPublicNotFound(c)
	case len(parts) == 1:
	case len(parts) == 2 && parts[1] == "items":
		items = true
	default:
		return trustPublicNotFound(c)
	}

	ctx := req.Context()
	org, err := s.db.GetOrganizationBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return trustPublicNotFound(c)
	}
	if err != nil {
		return trustPublicError(c, err)
	}
	// Set by OrgResolverMiddleware from the Host header only (this route never
	// carries a session): a tenant subdomain or custom domain pins the request
	// to that organization.
	if hostOrg, ok := c.Get("org_id").(int); ok && hostOrg != org.ID {
		return trustPublicNotFound(c)
	}

	settings, err := s.db.GetTrustCenterSettings(ctx, org.ID)
	if err != nil {
		return trustPublicError(c, err)
	}
	if !settings.Enabled {
		return trustPublicNotFound(c)
	}

	if items {
		rows, err := s.db.ListPublishedTrustCenterItems(ctx, org.ID)
		if err != nil {
			return trustPublicError(c, err)
		}
		out := make([]trustPublicItem, 0, len(rows))
		for _, it := range rows {
			out = append(out, trustPublicItem{
				ID: it.PublicID, Kind: it.Kind, Section: it.Section,
				SortOrder: it.SortOrder, AccessLevel: it.AccessLevel,
			})
		}
		return c.JSON(http.StatusOK, out)
	}

	color, err := s.db.GetOrgSetting(ctx, org.ID, "branding_color")
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return trustPublicError(c, err)
	}
	return c.JSON(http.StatusOK, trustPublicOverview{
		Title:           settings.Title,
		SecurityContact: settings.SecurityContact,
		BrandingName:    s.db.OrgBrandName(ctx, org),
		BrandingColor:   color,
	})
}
