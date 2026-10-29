package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// digestMember resolves the caller's own membership in the current org. The
// digest preference is always the caller's: no route takes a user id.
func (s *Server) digestMember(c echo.Context) (orgID, userID int, err error) {
	email := getUserEmail(c)
	orgID = getOrgID(c)
	if email == "" || orgID == 0 {
		return 0, 0, apiError(http.StatusUnauthorized, CodeNotFound, Entity("user"))
	}
	u, err := s.db.GetUserByEmail(c.Request().Context(), email)
	if err != nil || u == nil {
		return 0, 0, apiError(http.StatusUnauthorized, CodeNotFound, Entity("user"))
	}
	return orgID, u.ID, nil
}

// handleGetMyDigest returns whether the caller gets the weekly digest for the
// current organization.
func (s *Server) handleGetMyDigest(c echo.Context) error {
	orgID, userID, err := s.digestMember(c)
	if err != nil {
		return err
	}
	enabled, err := s.db.GetDigestEnabled(c.Request().Context(), orgID, userID)
	if err != nil {
		return apiError(http.StatusNotFound, CodeNotFound, Entity("user"))
	}
	return c.JSON(http.StatusOK, map[string]bool{"enabled": enabled})
}

// handleSetMyDigest turns the caller's weekly digest for the current
// organization on or off. Every role may do this for themselves.
func (s *Server) handleSetMyDigest(c echo.Context) error {
	orgID, userID, err := s.digestMember(c)
	if err != nil {
		return err
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil || req.Enabled == nil {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	if err := s.db.SetDigestEnabled(c.Request().Context(), orgID, userID, *req.Enabled); err != nil {
		return apiError(http.StatusNotFound, CodeNotFound, Entity("user"))
	}
	return c.JSON(http.StatusOK, map[string]bool{"enabled": *req.Enabled})
}
