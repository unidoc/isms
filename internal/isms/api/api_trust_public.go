package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/i18n"
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
	// DefaultLocale is the organization's default, never a guess from the
	// request: the page negotiates the visitor's language itself.
	DefaultLocale string `json:"default_locale"`
	ShowPoweredBy bool   `json:"show_powered_by"`
	// Overview is the served version of the overview document, or null.
	Overview *trustPublicOverviewDocument `json:"overview"`
}

// trustPublicOverviewDocument is the overview text of the trust center page.
type trustPublicOverviewDocument struct {
	Version     string `json:"version"`
	ApprovedAt  int64  `json:"approved_at"` // epoch seconds
	ContentHash string `json:"content_hash"`
	HTML        string `json:"html"`
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
	// Documents only, for every access level: which version visitors are
	// served. Never the body.
	Version     string `json:"version,omitempty"`
	ApprovedAt  int64  `json:"approved_at,omitempty"` // epoch seconds
	ContentHash string `json:"content_hash,omitempty"`
	// Certifications only. Title is the label. url (a link certification) and
	// file_name / file_size (a file certification) are set only at access level
	// public: a gated or restricted row shows what it is, never where it is.
	LinkType   string `json:"link_type,omitempty"`
	Issuer     string `json:"issuer,omitempty"`
	ValidFrom  string `json:"valid_from,omitempty"`
	ValidUntil string `json:"valid_until,omitempty"`
	State      string `json:"state,omitempty"` // current | expiring
	Source     string `json:"source,omitempty"`
	URL        string `json:"url,omitempty"`
	FileName   string `json:"file_name,omitempty"`
	FileSize   int64  `json:"file_size,omitempty"`
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
	var docID, certID string
	switch {
	case slug == "":
		return trustPublicNotFound(c)
	case len(parts) == 1:
	case len(parts) == 2 && parts[1] == "items":
		items = true
	case len(parts) == 3 && parts[1] == "documents" && parts[2] != "":
		docID = parts[2]
	case len(parts) == 4 && parts[1] == "certifications" && parts[2] != "" && parts[3] == "file":
		certID = parts[2]
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

	switch {
	case certID != "":
		return s.trustPublicCertificationFile(c, org, certID)
	case docID != "":
		return s.trustPublicDocument(c, org, docID)
	case items:
		return s.trustPublicItems(c, org)
	}
	return s.trustPublicOverview(c, org, settings)
}

// The three answers below are shared with the authenticated preview
// (trust_preview.go): they take the organization and its settings from the
// caller and never look at whether the trust center is on, so a visitor and a
// previewing manager are served by the same code.

// trustPublicOverview answers GET /api/v1/trust/:slug.
func (s *Server) trustPublicOverview(c echo.Context, org *db.Organization, settings *db.TrustCenterSettings) error {
	ctx := c.Request().Context()
	color, err := s.db.GetOrgSetting(ctx, org.ID, "branding_color")
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return trustPublicError(c, err)
	}
	// Same rules as /config: the platform switch, then the org setting.
	showPoweredBy := !s.hidePoweredBy
	if val, err := s.db.GetOrgSetting(ctx, org.ID, "show_powered_by"); err == nil && val != "" {
		showPoweredBy = val == "true" || val == "1"
	}
	orgLocale, _ := s.db.GetOrgSetting(ctx, org.ID, "default_locale")
	return c.JSON(http.StatusOK, trustPublicOverview{
		Title:           settings.Title,
		SecurityContact: settings.SecurityContact,
		BrandingName:    s.db.OrgBrandName(ctx, org),
		BrandingColor:   color,
		DefaultLocale:   i18n.Resolve("", orgLocale),
		ShowPoweredBy:   showPoweredBy,
		Overview:        s.trustOverviewDocument(ctx, org, settings),
	})
}

// trustPublicItems answers GET /api/v1/trust/:slug/items.
func (s *Server) trustPublicItems(c echo.Context, org *db.Organization) error {
	ctx := c.Request().Context()
	rows, err := s.db.ListPublishedTrustCenterItems(ctx, org.ID)
	if err != nil {
		return trustPublicError(c, err)
	}
	docs, err := s.trustServableDocuments(ctx, org.ID, rows)
	if err != nil {
		return trustPublicError(c, err)
	}
	servable, err := s.db.ListServableTrustCertifications(ctx, org.ID)
	if err != nil {
		return trustPublicError(c, err)
	}
	certs := make(map[string]*db.TrustCertification, len(servable))
	for i := range servable {
		certs[strconv.FormatInt(servable[i].ID, 10)] = &servable[i]
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
			pi.Version = info.Served.Version
			pi.ApprovedAt = info.Served.ApprovedAt.Unix()
			pi.ContentHash = info.Served.ContentHash
		}
		if it.Kind == db.TrustKindCertification {
			// Only a certification the servable query returned goes out: an
			// expired, hidden or missing one is skipped, never emitted bare.
			cert := certs[it.Reference]
			if cert == nil || cert.Item == nil || cert.Item.ID != it.ID {
				continue
			}
			pi.AccessLevel = cert.Item.AccessLevel
			pi.Title, pi.LinkType, pi.Issuer, pi.State, pi.Source = cert.Label, cert.LinkType, cert.Issuer, cert.State, trustCertSource(cert)
			if cert.ValidFrom != nil {
				pi.ValidFrom = *cert.ValidFrom
			}
			if cert.ValidUntil != nil {
				pi.ValidUntil = *cert.ValidUntil
			}
			if cert.Item.AccessLevel == db.TrustAccessPublic {
				pi.URL, pi.FileName, pi.FileSize = cert.URL, cert.FileName, cert.FileSize
			}
		}
		out = append(out, pi)
	}
	return c.JSON(http.StatusOK, out)
}

// trustOverviewDocument serves the overview document like a published one: the
// served version, never HEAD, through the same renderer and link policy. It is
// nil when nothing can be served, and then the rest of the page still works: an
// error is logged and never fails the request or clears the setting. A document
// that is retired at HEAD, or cleanly not found, clears the setting.
func (s *Server) trustOverviewDocument(ctx context.Context, org *db.Organization, settings *db.TrustCenterSettings) *trustPublicOverviewDocument {
	if settings.OverviewDocumentID == nil || *settings.OverviewDocumentID == "" {
		return nil
	}
	docID := strings.ToLower(*settings.OverviewDocumentID)
	st, err := s.storeForOrg(ctx, org.ID)
	if err != nil {
		log.Printf("trust center: org %d overview document %s: %v", org.ID, docID, err)
		return nil
	}
	head, err := s.trustDocumentHead(ctx, st, org.ID, docID)
	if err != nil {
		log.Printf("trust center: org %d overview document %s: %v", org.ID, docID, err)
		return nil
	}
	if reason := trustHideReason(head); reason != "" {
		s.clearTrustOverviewForDocument(ctx, org.ID, docID, reason, "system")
		return nil
	}
	served, err := s.trustServed(ctx, st, org.ID, docID)
	if err != nil {
		log.Printf("trust center: org %d overview document %s: %v", org.ID, docID, err)
		return nil
	}
	if served == nil {
		return nil
	}
	rows, err := s.db.ListPublishedTrustCenterItems(ctx, org.ID)
	if err == nil {
		var docs map[string]*trustDocInfo
		if docs, err = s.trustServableDocuments(ctx, org.ID, rows); err == nil {
			var html string
			if html, err = renderTrustMarkdown([]byte(served.Body), s.trustLinkPolicyFor(org, docs)); err == nil {
				return &trustPublicOverviewDocument{
					Version: served.Version, ApprovedAt: served.ApprovedAt.Unix(),
					ContentHash: served.ContentHash, HTML: html,
				}
			}
		}
	}
	log.Printf("trust center: org %d overview document %s: %v", org.ID, docID, err)
	return nil
}

// trustLinkPolicyFor is the link policy for text served to visitors of org:
// a link to a published document of the same trust center is allowed.
func (s *Server) trustLinkPolicyFor(org *db.Organization, docs map[string]*trustDocInfo) trustLinkPolicy {
	policy := trustLinkPolicy{ApexHost: s.apexHost, Slug: org.Slug, Docs: map[string]string{}}
	if org.Domain != nil {
		policy.OrgHost = *org.Domain
	}
	for ref, d := range docs {
		policy.Docs[ref] = d.Item.PublicID
	}
	return policy
}

// trustPublicDocument answers GET /api/v1/trust/:slug/documents/:id. Every case
// that cannot be served is the same 404: unknown id, another organization's
// id, a hidden item, a retired or deleted document, no approved version, and
// (until visitor sign-in exists) any access level but public.
func (s *Server) trustPublicDocument(c echo.Context, org *db.Organization, publicID string) error {
	ctx := c.Request().Context()
	rows, err := s.db.ListPublishedTrustCenterItems(ctx, org.ID)
	if err != nil {
		return trustPublicError(c, err)
	}
	docs, err := s.trustServableDocuments(ctx, org.ID, rows)
	if err != nil {
		return trustPublicError(c, err)
	}
	var info *trustDocInfo
	for _, it := range rows {
		if it.Kind == db.TrustKindDocument && it.PublicID == strings.ToLower(publicID) {
			info = docs[it.Reference]
		}
	}
	if info == nil || info.Item.AccessLevel != db.TrustAccessPublic {
		return trustPublicNotFound(c)
	}
	html, err := renderTrustMarkdown([]byte(info.Served.Body), s.trustLinkPolicyFor(org, docs))
	if err != nil {
		return trustPublicError(c, err)
	}
	return c.JSON(http.StatusOK, trustPublicDocument{
		ID: info.Item.PublicID, Title: info.Served.Title, Version: info.Served.Version,
		ApprovedAt: info.Served.ApprovedAt.Unix(), ContentHash: info.Served.ContentHash, HTML: html,
	})
}
