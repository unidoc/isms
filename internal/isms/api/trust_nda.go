package api

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// Click-through NDA for gated trust center items (#525). When the organization
// has set an NDA document, a gated item needs a signed-in visitor who accepted
// the NDA's current approved version (the served version, by content hash) or
// whose email domain is on the skip list as it is right now. With no NDA set
// nothing changes. An NDA that is set but cannot be served (never approved,
// retired, deleted, or a served version without a content hash) fails closed:
// gated items are refused for everyone and the setting is left alone.

// NDA status of a signed-in visitor.
const (
	trustNDANone        = "none"        // no NDA set
	trustNDARequired    = "required"    // the current version is not accepted
	trustNDAAccepted    = "accepted"    // the current version is accepted
	trustNDASkipped     = "skipped"     // the email domain is on the skip list
	trustNDAUnavailable = "unavailable" // set, but nothing can be served
)

// trustNDACurrent is the NDA version visitors are asked to accept.
type trustNDACurrent struct {
	DocumentID  string
	VersionID   int64
	Version     string
	ApprovedAt  db.Epoch
	ContentHash string
	Title       string
	Body        string // markdown, frontmatter stripped
}

// trustPublicNDA is the response of GET /api/v1/trust/:slug/nda, and the
// "nda" member of the trust_nda_outdated answer.
type trustPublicNDA struct {
	Version     string `json:"version"`
	ApprovedAt  int64  `json:"approved_at"` // epoch seconds
	ContentHash string `json:"content_hash"`
	Title       string `json:"title"`
	HTML        string `json:"html"`
	// Company is the visitor's company, for "on behalf of <company>".
	Company string `json:"company"`
}

// trustVisitorNDA is the "nda" member of the visitor responses.
type trustVisitorNDA struct {
	Status      string `json:"status"`
	Version     string `json:"version,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
}

// Why a set NDA cannot be served (trustNDAState), for the manager's warning.
const (
	trustNDANotApproved = "not_approved" // no approved version yet
	trustNDARetired     = "retired"
	trustNDADeleted     = "deleted"
	trustNDAError       = "error" // unreadable, or a served version without a content hash
)

// trustNDAStatusFor is the one mapping from the facts about a visitor to their
// NDA status, shared by the public routes and the manager's visitor list so the
// two cannot drift from the access rule: gated is open exactly for none,
// accepted and skipped. cur is nil when the NDA is set but cannot be served.
func trustNDAStatusFor(set bool, cur *trustNDACurrent, acceptedCurrent, skipListed bool) string {
	switch {
	case !set:
		return trustNDANone
	case cur == nil:
		return trustNDAUnavailable
	case acceptedCurrent:
		return trustNDAAccepted
	case skipListed:
		return trustNDASkipped
	}
	return trustNDARequired
}

// trustNDAState resolves the NDA of an organization. set is whether the
// settings name one; cur is nil when it is set but cannot be served, and
// reason then says why. Any read failure counts as "cannot be served": it is
// logged and fails closed, and it never clears the setting.
func (s *Server) trustNDAState(ctx context.Context, orgID int, settings *db.TrustCenterSettings) (cur *trustNDACurrent, set bool, reason string) {
	if settings.NDADocumentID == nil || *settings.NDADocumentID == "" {
		return nil, false, ""
	}
	docID := strings.ToLower(*settings.NDADocumentID)
	fail := func(err error) (*trustNDACurrent, bool, string) {
		log.Printf("trust center: org %d NDA document %s: %v", orgID, docID, err)
		return nil, true, trustNDAError
	}
	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return fail(err)
	}
	head, err := s.trustDocumentHead(ctx, st, orgID, docID)
	if err != nil {
		return fail(err)
	}
	switch trustHideReason(head) {
	case db.TrustHiddenDeleted:
		return nil, true, trustNDADeleted
	case db.TrustHiddenRetired:
		return nil, true, trustNDARetired
	}
	served, err := s.trustServed(ctx, st, orgID, docID)
	if err != nil {
		return fail(err)
	}
	if served == nil {
		return nil, true, trustNDANotApproved
	}
	// An empty hash could never be told apart from "not accepted".
	if served.ContentHash == "" {
		return nil, true, trustNDAError
	}
	return &trustNDACurrent{
		DocumentID: docID, VersionID: int64(served.VersionID), Version: served.Version,
		ApprovedAt: served.ApprovedAt, ContentHash: served.ContentHash,
		Title: served.Title, Body: served.Body,
	}, true, ""
}

// trustNDAPayload renders the NDA for a visitor, like a published document.
func (s *Server) trustNDAPayload(ctx context.Context, org *db.Organization, cur *trustNDACurrent, company string) (*trustPublicNDA, error) {
	rows, err := s.db.ListPublishedTrustCenterItems(ctx, org.ID)
	if err != nil {
		return nil, err
	}
	docs, err := s.trustServableDocuments(ctx, org.ID, rows)
	if err != nil {
		return nil, err
	}
	html, err := renderTrustMarkdown([]byte(cur.Body), s.trustLinkPolicyFor(org, docs))
	if err != nil {
		return nil, err
	}
	return &trustPublicNDA{
		Version: cur.Version, ApprovedAt: cur.ApprovedAt.Unix(), ContentHash: cur.ContentHash,
		Title: cur.Title, HTML: html, Company: company,
	}, nil
}

// trustViewer is a signed-in visitor together with what the NDA says about
// them. The NDA is resolved on first use and at most once per request, so a
// request that never reaches a gated item does not read the NDA document or
// write a skip row. A nil *trustViewer is an anonymous viewer.
type trustViewer struct {
	Visitor *db.TrustVisitor

	s        *Server
	c        echo.Context
	org      *db.Organization
	settings *db.TrustCenterSettings

	done    bool
	status  string
	current *trustNDACurrent
	err     error
}

// newTrustViewer wraps the visitor from the cookie; nil stays nil.
func (s *Server) newTrustViewer(c echo.Context, org *db.Organization, settings *db.TrustCenterSettings, v *db.TrustVisitor) *trustViewer {
	if v == nil {
		return nil
	}
	return &trustViewer{Visitor: v, s: s, c: c, org: org, settings: settings}
}

// resolve works out the NDA status once. The skip list is read as it is now;
// the domain_skip row it writes is a log, never a grant.
func (tv *trustViewer) resolve() {
	if tv.done {
		return
	}
	tv.done = true
	tv.status = trustNDARequired // anything unresolved stays closed
	ctx := tv.c.Request().Context()
	cur, set, _ := tv.s.trustNDAState(ctx, tv.org.ID, tv.settings)
	tv.current = cur
	var accepted bool
	if set && cur != nil {
		var err error
		if accepted, err = tv.s.db.HasTrustNDAAccepted(ctx, tv.org.ID, tv.Visitor.ID, cur.ContentHash); err != nil {
			tv.err = err
			return
		}
	}
	status := trustNDAStatusFor(set, cur, accepted, tv.settings.NDASkips(tv.Visitor.Email))
	if status == trustNDASkipped {
		_, err := tv.s.db.RecordTrustNDAAcceptance(ctx, tv.org.ID, &db.TrustNDAAcceptance{
			VisitorID: tv.Visitor.ID, DocumentID: cur.DocumentID, VersionID: cur.VersionID,
			Version: cur.Version, ContentHash: cur.ContentHash, Method: db.TrustNDADomainSkip,
			Company: tv.Visitor.Company, IP: tv.c.RealIP(), UserAgent: tv.c.Request().UserAgent(),
		})
		if err != nil {
			tv.err = err
			return
		}
	}
	tv.status = status
}

// satisfied reports whether the visitor has met the NDA step (or there is none).
func (tv *trustViewer) satisfied() bool {
	tv.resolve()
	if tv.err != nil {
		return false
	}
	switch tv.status {
	case trustNDANone, trustNDAAccepted, trustNDASkipped:
		return true
	}
	return false
}

// Err is a real failure met while resolving the NDA, which the caller answers
// with a 500 instead of a 404. Safe on a nil viewer.
func (tv *trustViewer) Err() error {
	if tv == nil {
		return nil
	}
	return tv.err
}

// info is the "nda" member of the visitor responses.
func (tv *trustViewer) info() (*trustVisitorNDA, error) {
	tv.resolve()
	if tv.err != nil {
		return nil, tv.err
	}
	out := &trustVisitorNDA{Status: tv.status}
	if tv.current != nil && tv.status != trustNDANone && tv.status != trustNDAUnavailable {
		out.Version, out.ContentHash = tv.current.Version, tv.current.ContentHash
	}
	return out, nil
}

// trustPublicNDAGet answers GET /api/v1/trust/:slug/nda: the current NDA for a
// signed-in visitor, the standard 404 otherwise (not signed in, no NDA set, or
// the NDA cannot be served).
func (s *Server) trustPublicNDAGet(c echo.Context, org *db.Organization, settings *db.TrustCenterSettings) error {
	v, err := s.trustVisitorFromRequest(c, org.ID)
	if err != nil {
		return trustPublicError(c, err)
	}
	if v == nil {
		return trustPublicNotFound(c)
	}
	cur, set, _ := s.trustNDAState(c.Request().Context(), org.ID, settings)
	if !set || cur == nil {
		return trustPublicNotFound(c)
	}
	payload, err := s.trustNDAPayload(c.Request().Context(), org, cur, v.Company)
	if err != nil {
		return trustPublicError(c, err)
	}
	return c.JSON(http.StatusOK, payload)
}

// trustNDAAccept answers POST /nda/accept: record the acceptance of the
// current version. A hash that is not the current one (empty included) is a 409
// carrying the current NDA, and writes nothing.
func (s *Server) trustNDAAccept(c echo.Context, org *db.Organization, settings *db.TrustCenterSettings, body trustVisitorBody) error {
	ctx := c.Request().Context()
	v, err := s.trustVisitorFromRequest(c, org.ID)
	if err != nil {
		return trustPublicError(c, err)
	}
	if v == nil {
		return trustPublicNotFound(c)
	}
	cur, set, _ := s.trustNDAState(ctx, org.ID, settings)
	if !set || cur == nil {
		return trustPublicNotFound(c)
	}
	if body.ContentHash == "" || body.ContentHash != cur.ContentHash {
		payload, err := s.trustNDAPayload(ctx, org, cur, v.Company)
		if err != nil {
			return trustPublicError(c, err)
		}
		e := apiError(http.StatusConflict, CodeTrustNDAOutdated)
		eb, _ := e.Message.(errorBody)
		return c.JSON(http.StatusConflict, struct {
			errorBody
			NDA *trustPublicNDA `json:"nda"`
		}{eb, payload})
	}
	if _, err := s.db.RecordTrustNDAAcceptance(ctx, org.ID, &db.TrustNDAAcceptance{
		VisitorID: v.ID, DocumentID: cur.DocumentID, VersionID: cur.VersionID,
		Version: cur.Version, ContentHash: cur.ContentHash, Method: db.TrustNDAAccepted,
		Company: v.Company, IP: c.RealIP(), UserAgent: c.Request().UserAgent(),
	}); err != nil {
		return trustPublicError(c, err)
	}
	tv := s.newTrustViewer(c, org, settings, v)
	nda, err := tv.info()
	if err != nil {
		return trustPublicError(c, err)
	}
	return trustVisitorResponse(c, v, nda)
}
