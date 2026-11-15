package api

import (
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// The preview answers a signed-in admin or manager with exactly what an
// anonymous visitor would get from /api/v1/trust/:slug if the trust center were
// on: the same builders, the same hidden-item skipping, the same locked-item
// handling and the same 404. The one difference is that the "enabled" setting is
// ignored. The organization is the session's, never a slug from the request, so
// a manager can only preview their own. The anonymous handler stays
// session-blind; nothing here is reachable without a token.

// trustPreviewOrg resolves the session's organization and settings after the
// role check. The responses are never cacheable, like the public ones.
func (s *Server) trustPreviewOrg(c echo.Context) (*db.Organization, *db.TrustCenterSettings, error) {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return nil, nil, err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	ctx := c.Request().Context()
	org, err := s.db.GetOrganization(ctx, getOrgID(c))
	if err != nil {
		return nil, nil, trustInternalError(c, "reading organization", err)
	}
	settings, err := s.db.GetTrustCenterSettings(ctx, org.ID)
	if err != nil {
		return nil, nil, trustInternalError(c, "reading settings", err)
	}
	return org, settings, nil
}

// GET /api/v1/trust-center/preview
func (s *Server) handleTrustPreviewOverview(c echo.Context) error {
	org, settings, err := s.trustPreviewOrg(c)
	if err != nil {
		return err
	}
	return s.trustPublicOverview(c, org, settings)
}

// GET /api/v1/trust-center/preview/items
func (s *Server) handleTrustPreviewItems(c echo.Context) error {
	org, _, err := s.trustPreviewOrg(c)
	if err != nil {
		return err
	}
	return s.trustPublicItems(c, org)
}

// GET /api/v1/trust-center/preview/documents/:id
func (s *Server) handleTrustPreviewDocument(c echo.Context) error {
	org, _, err := s.trustPreviewOrg(c)
	if err != nil {
		return err
	}
	return s.trustPublicDocument(c, org, c.Param("id"))
}
