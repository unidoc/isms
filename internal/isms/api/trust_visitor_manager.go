package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// Manager endpoints for trust center visitors (#524). Visitor actions (revoke,
// block, unblock) write no entity_changelog rows: a visitor is not an entity of
// the register, and the trust center access log records them. Every endpoint is
// admin or manager only and the organization is the session's, so one cannot
// reach another organization's visitor; an id that is unknown, foreign or not a
// number is the same 404.

type trustManagerVisitor struct {
	ID         int64     `json:"id"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	Company    string    `json:"company"`
	VerifiedAt *db.Epoch `json:"verified_at"`
	FirstSeen  db.Epoch  `json:"first_seen"`
	LastSeen   db.Epoch  `json:"last_seen"`
	Blocked    bool      `json:"blocked"`
	BlockedAt  *db.Epoch `json:"blocked_at"`
	BlockedBy  *string   `json:"blocked_by"`
	// ActiveSessions counts sessions that are neither revoked nor expired.
	ActiveSessions int `json:"active_sessions"`
}

type trustVisitorBlockRequest struct {
	Blocked *bool `json:"blocked"`
}

func trustVisitorRow(v *db.TrustVisitor) trustManagerVisitor {
	return trustManagerVisitor{
		ID: v.ID, Email: v.Email, Name: v.Name, Company: v.Company, VerifiedAt: v.VerifiedAt,
		FirstSeen: v.FirstSeen, LastSeen: v.LastSeen, Blocked: v.Blocked, BlockedAt: v.BlockedAt,
		BlockedBy: v.BlockedBy, ActiveSessions: v.ActiveSessions,
	}
}

// GET /api/v1/trust-center/visitors
func (s *Server) handleListTrustVisitors(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	vs, err := s.db.ListTrustVisitors(c.Request().Context(), getOrgID(c))
	if err != nil {
		return trustInternalError(c, "listing visitors", err)
	}
	out := make([]trustManagerVisitor, 0, len(vs))
	for i := range vs {
		out = append(out, trustVisitorRow(&vs[i]))
	}
	return c.JSON(http.StatusOK, out)
}

// trustVisitorFromParam resolves :id to one of the session organization's
// visitors, or the 404.
func (s *Server) trustVisitorFromParam(c echo.Context) (*db.TrustVisitor, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, errNotFound("visitor")
	}
	v, err := s.db.GetTrustVisitor(c.Request().Context(), getOrgID(c), id)
	if errors.Is(err, db.ErrTrustVisitorNotFound) {
		return nil, errNotFound("visitor")
	}
	if err != nil {
		return nil, trustInternalError(c, "reading visitor", err)
	}
	return v, nil
}

// POST /api/v1/trust-center/visitors/:id/revoke-sessions
func (s *Server) handleRevokeTrustVisitorSessions(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	v, err := s.trustVisitorFromParam(c)
	if err != nil {
		return err
	}
	n, err := s.db.RevokeTrustVisitorSessions(c.Request().Context(), getOrgID(c), v.ID)
	if err != nil {
		return trustInternalError(c, "revoking visitor sessions", err)
	}
	return c.JSON(http.StatusOK, map[string]int{"revoked": n})
}

// PUT /api/v1/trust-center/visitors/:id/blocked
func (s *Server) handleBlockTrustVisitor(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	v, err := s.trustVisitorFromParam(c)
	if err != nil {
		return err
	}
	var req trustVisitorBlockRequest
	if err := c.Bind(&req); err != nil || req.Blocked == nil {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	out, err := s.db.SetTrustVisitorBlocked(c.Request().Context(), getOrgID(c), v.ID, *req.Blocked, getUserEmail(c))
	if errors.Is(err, db.ErrTrustVisitorNotFound) {
		return errNotFound("visitor")
	}
	if err != nil {
		return trustInternalError(c, "updating visitor", err)
	}
	return c.JSON(http.StatusOK, trustVisitorRow(out))
}
