package api

import (
	"context"
	"errors"
	"log"
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
	// NDA is where the visitor stands with the organization's NDA right now.
	NDA trustManagerVisitorNDA `json:"nda"`
}

// trustManagerVisitorNDA mirrors the access rule (trustNDAStatusFor).
// Version and ContentHash are the current NDA's (empty for none and
// unavailable). AcceptedAt and AcceptedVersion describe the visitor's own click
// through: the one for the current version when there is one, else their latest
// (an older version); null / empty when they never accepted.
type trustManagerVisitorNDA struct {
	Status          string    `json:"status"`
	Version         string    `json:"version"`
	ContentHash     string    `json:"content_hash"`
	AcceptedAt      *db.Epoch `json:"accepted_at"`
	AcceptedVersion string    `json:"accepted_version"`
}

type trustVisitorBlockRequest struct {
	Blocked *bool `json:"blocked"`
}

func trustVisitorRow(v *db.TrustVisitor, nda trustManagerVisitorNDA) trustManagerVisitor {
	return trustManagerVisitor{
		ID: v.ID, Email: v.Email, Name: v.Name, Company: v.Company, VerifiedAt: v.VerifiedAt,
		FirstSeen: v.FirstSeen, LastSeen: v.LastSeen, Blocked: v.Blocked, BlockedAt: v.BlockedAt,
		BlockedBy: v.BlockedBy, ActiveSessions: v.ActiveSessions, NDA: nda,
	}
}

// trustVisitorNDAs works out the NDA status of every visitor in vs with one
// resolution of the NDA and one query for the acceptances, so a long list costs
// no more git reads than a short one. It is a read: unlike a visitor's own
// request it writes no skip row. The status comes from trustNDAStatusFor, with
// the skip list as it is now.
func (s *Server) trustVisitorNDAs(ctx context.Context, orgID int, vs []db.TrustVisitor) (map[int64]trustManagerVisitorNDA, error) {
	settings, err := s.db.GetTrustCenterSettings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	cur, set, _ := s.trustNDAState(ctx, orgID, settings)
	accepted := map[int64][]db.TrustNDAAcceptance{} // per visitor, oldest first
	if set {
		rows, err := s.db.ListTrustNDAAccepted(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, a := range rows {
			accepted[a.VisitorID] = append(accepted[a.VisitorID], a)
		}
	}
	out := make(map[int64]trustManagerVisitorNDA, len(vs))
	for _, v := range vs {
		var match, latest *db.TrustNDAAcceptance
		mine := accepted[v.ID]
		for i := range mine {
			latest = &mine[i]
			if cur != nil && mine[i].ContentHash == cur.ContentHash {
				match = &mine[i]
			}
		}
		n := trustManagerVisitorNDA{Status: trustNDAStatusFor(set, cur, match != nil, settings.NDASkips(v.Email))}
		if cur != nil {
			n.Version, n.ContentHash = cur.Version, cur.ContentHash
		}
		if match != nil {
			latest = match
		}
		if latest != nil {
			at := latest.CreatedAt
			n.AcceptedAt, n.AcceptedVersion = &at, latest.Version
		}
		out[v.ID] = n
	}
	return out, nil
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
	ndas, err := s.trustVisitorNDAs(c.Request().Context(), getOrgID(c), vs)
	if err != nil {
		return trustInternalError(c, "reading NDA status", err)
	}
	out := make([]trustManagerVisitor, 0, len(vs))
	for i := range vs {
		out = append(out, trustVisitorRow(&vs[i], ndas[vs[i].ID]))
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
	// The block has been written: a failure to read the NDA status is logged and
	// shown as "unavailable", never a 500.
	nda := trustManagerVisitorNDA{Status: trustNDAUnavailable}
	if ndas, err := s.trustVisitorNDAs(c.Request().Context(), getOrgID(c), []db.TrustVisitor{*out}); err != nil {
		log.Printf("trust center: visitor %d NDA status: %v", out.ID, err)
	} else {
		nda = ndas[out.ID]
	}
	return c.JSON(http.StatusOK, trustVisitorRow(out, nda))
}
