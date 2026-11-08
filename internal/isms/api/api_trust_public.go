package api

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
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
	// Title is the served version's title; set for documents only.
	Title string `json:"title,omitempty"`
}

// trustPublicDocument is the response of GET /api/v1/trust/:slug/documents/:id.
// It is deliberately only these fields: the document's frontmatter holds staff
// names and emails and never goes out.
type trustPublicDocument struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Version     string `json:"version"`
	ApprovedAt  int64  `json:"approved_at"` // epoch seconds
	ContentHash string `json:"content_hash"`
	HTML        string `json:"html"`
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
	var docID string
	switch {
	case slug == "":
		return trustPublicNotFound(c)
	case len(parts) == 1:
	case len(parts) == 2 && parts[1] == "items":
		items = true
	case len(parts) == 3 && parts[1] == "documents" && parts[2] != "":
		docID = parts[2]
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

	if items || docID != "" {
		rows, err := s.db.ListPublishedTrustCenterItems(ctx, org.ID)
		if err != nil {
			return trustPublicError(c, err)
		}
		docs, err := s.trustServableDocuments(ctx, org.ID, rows)
		if err != nil {
			return trustPublicError(c, err)
		}
		if docID != "" {
			return s.trustPublicDocument(c, org, rows, docs, docID)
		}
		out := make([]trustPublicItem, 0, len(rows))
		for _, it := range rows {
			if it.HiddenAt != nil {
				continue
			}
			pi := trustPublicItem{
				ID: it.PublicID, Kind: it.Kind, Section: it.Section,
				SortOrder: it.SortOrder, AccessLevel: it.AccessLevel,
			}
			if it.Kind == db.TrustKindDocument {
				info := docs[it.Reference]
				if info == nil {
					continue
				}
				pi.Title = info.Served.Title
			}
			out = append(out, pi)
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

// trustPublicDocument answers GET /api/v1/trust/:slug/documents/:id. Every case
// that cannot be served is the same 404: unknown id, another organization's
// id, a hidden item, a retired or deleted document, no approved version, and
// (until visitor sign-in exists) any access level but public.
func (s *Server) trustPublicDocument(c echo.Context, org *db.Organization, rows []db.TrustCenterItem, docs map[string]*trustDocInfo, publicID string) error {
	var info *trustDocInfo
	for _, it := range rows {
		if it.Kind == db.TrustKindDocument && it.PublicID == strings.ToLower(publicID) {
			info = docs[it.Reference]
		}
	}
	if info == nil || info.Item.AccessLevel != db.TrustAccessPublic {
		return trustPublicNotFound(c)
	}
	policy := trustLinkPolicy{ApexHost: s.apexHost, Slug: org.Slug, Docs: map[string]string{}}
	if org.Domain != nil {
		policy.OrgHost = *org.Domain
	}
	for ref, d := range docs {
		policy.Docs[ref] = d.Item.PublicID
	}
	html, err := renderTrustMarkdown([]byte(info.Served.Body), policy)
	if err != nil {
		return trustPublicError(c, err)
	}
	return c.JSON(http.StatusOK, trustPublicDocument{
		ID: info.Item.PublicID, Title: info.Served.Title, Version: info.Served.Version,
		ApprovedAt: info.Served.ApprovedAt.Unix(), ContentHash: info.Served.ContentHash, HTML: html,
	})
}
