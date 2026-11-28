package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	netmail "net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
	"isms.sh/internal/isms/db"
)

// Trust center visitor sign-in (#524). A visitor (prospect, auditor) proves an
// email address with a one-time link and gets a session cookie. They are not
// ISMS users: nothing here reads or writes users, roles or API tokens.
//
//   - The credential is an opaque random token stored hashed, delivered only in
//     a cookie and never in a JSON body. It is not an isms-audience JWT, and
//     member authentication reads only the Authorization header, so a visitor
//     token or cookie is rejected by every member route by construction.
//   - The cookie is HttpOnly, SameSite=Strict and scoped by Path to one
//     organization's trust prefix, so two organizations on one host never share
//     it. Every cookie-bearing POST must be application/json, which together
//     with SameSite=Strict closes CSRF without a token.
//   - A link is redeemed by POST, never by GET, so a mail scanner that fetches
//     the URL cannot consume it. inspect lets the page show "Sign in as <email>"
//     first without consuming anything.
//   - A link request gets the same answer body whether or not the address is
//     known, blocked or throttled, and the mail is sent off the request path.
//     The timing is not identical: a known blocked visitor is answered before
//     the capped transaction, while any other address also pays for that
//     transaction (two advisory locks, three counts and an insert). It is a
//     few database round trips, kept small by the indexes on the links table,
//     and is not treated as a signal. Only a blocked email domain is refused
//     openly: that discloses policy, not an account.

const (
	trustVisitorCookie = "isms_trust_visitor"
	trustLinkTTL       = 15 * time.Minute
	trustSessionTTL    = 7 * 24 * time.Hour
	// Per-address cap on link mails: three in one link lifetime.
	trustLinkPerEmail = 3
	// Per-address cap across all organizations, per hour: one inbox cannot be
	// flooded through many trust centers.
	trustLinkPerAddress = 10
	// Per-IP cap on link requests (this route only).
	trustLinkIPBudget = 10
	trustLinkIPWindow = 15 * time.Minute
	// Per-organization hourly cap, a backstop against abuse that is far above
	// real use; overridable with ISMS_TRUST_SIGNIN_ORG_HOURLY. Reaching it shows
	// to managers as sign_in_paused_until in the settings.
	trustSignInOrgHourlyDefault = 500
	trustVisitorBodyLimit       = 8 << 10
	trustMailTimeout            = 30 * time.Second
)

// trustLinkLimiter is the in-memory per-IP limit on link requests. It is off
// when ISMS_RATE_LIMIT=0, like the other limiters. The per-IP key is
// c.RealIP(), which the server's IPExtractor takes from client-sent headers,
// so it is spoofable when the server is exposed directly; the per-organization
// send cap in the handler is what bounds mail volume then.
type trustLinkLimiter struct {
	store middleware.RateLimiterStore // nil = disabled
}

// newTrustLinkLimiter builds the limiter. Build it once per Server.
func newTrustLinkLimiter() *trustLinkLimiter {
	if rateLimitDisabled() {
		return &trustLinkLimiter{}
	}
	return &trustLinkLimiter{store: middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
		Rate:      rate.Every(trustLinkIPWindow / trustLinkIPBudget),
		Burst:     trustLinkIPBudget,
		ExpiresIn: 2 * trustLinkIPWindow,
	})}
}

// allow reports whether the IP may send another link request. A nil or
// disabled limiter allows everything; a store error never refuses a request.
func (l *trustLinkLimiter) allow(ip string) bool {
	if l == nil || l.store == nil {
		return true
	}
	ok, err := l.store.Allow(ip)
	return err != nil || ok
}

// trustSignInOrgHourly is the per-organization cap on link mails per hour.
func trustSignInOrgHourly() int {
	if v := os.Getenv("ISMS_TRUST_SIGNIN_ORG_HOURLY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return trustSignInOrgHourlyDefault
}

// hashToken is the sha256 hex of a raw token, the same form newHashedToken
// stores, so a cookie or link token can be looked up.
func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// trustVisitorReply sends a coded error built at the call site (so the
// code-catalogue checks see it). These are real answers to the caller (the page
// renders them), unlike the single 404.
func trustVisitorReply(c echo.Context, e *echo.HTTPError) error {
	return c.JSON(e.Code, e.Message)
}

type trustVisitorBody struct {
	Email   string `json:"email"`
	Token   string `json:"token"`
	Name    string `json:"name"`
	Company string `json:"company"`
}

// trustVisitorJSON reports whether the request declares a JSON body. It is the
// CSRF defence for the cookie-bearing POSTs (see above).
func trustVisitorJSON(c echo.Context) bool {
	mt, _, err := mime.ParseMediaType(c.Request().Header.Get(echo.HeaderContentType))
	return err == nil && mt == echo.MIMEApplicationJSON
}

// trustVisitorDecode reads the optional JSON body. An empty body is an empty
// struct (sign-out has none); malformed JSON is an error.
func trustVisitorDecode(c echo.Context) (trustVisitorBody, error) {
	var b trustVisitorBody
	raw, err := io.ReadAll(http.MaxBytesReader(c.Response(), c.Request().Body, trustVisitorBodyLimit))
	if err != nil {
		return b, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return b, nil
	}
	return b, json.Unmarshal(raw, &b)
}

// The token travels in the URL fragment, which browsers never send to a server,
// so it stays out of request logs and proxies; the page reads location.hash. The
// key is visitor_token, not token: the web app's SSO handoff takes #token= as a
// member session token on any route, and the two credentials must not share a name.
//
// trustSignInLink builds the link emailed to a visitor, from server-known values
// only: ISMS_BASE_URL and the organization's own domain and slug. The request's
// Host is only compared against them to pick the form the visitor is already
// using; its bytes (port included) never reach the link, so a forged Host
// cannot send the token to another site.
func trustSignInLink(baseURL string, org *db.Organization, subdomainRouting bool, requestHost, token string) string {
	base := strings.TrimRight(baseURL, "/")
	pathForm := base + "/" + org.Slug + "/trust/sign-in#visitor_token=" + token

	scheme, host, port, canSubdomain := resolveSubdomainHost(base, subdomainRouting)
	if scheme == "" {
		return pathForm
	}
	reqHost := strings.ToLower(requestHost)
	if h, _, err := net.SplitHostPort(reqHost); err == nil {
		reqHost = h
	}
	if org.TrustDomain != nil && *org.TrustDomain != "" && reqHost == strings.ToLower(*org.TrustDomain) {
		// On the trust host the page lives at the root, and the base URL's port
		// is not appended (like a custom domain).
		return scheme + "://" + strings.ToLower(*org.TrustDomain) + "/sign-in#visitor_token=" + token
	}
	if org.Domain != nil && *org.Domain != "" {
		// Like orgURLs: a scheme stored with the domain is kept, else the base
		// URL's; the base URL's port is not appended to a custom domain.
		d := strings.ToLower(strings.TrimRight(*org.Domain, "/"))
		domScheme := scheme
		if i := strings.Index(d, "://"); i >= 0 {
			domScheme, d = d[:i], d[i+3:]
		}
		if d != "" && reqHost == d {
			return domScheme + "://" + d + "/trust/sign-in#visitor_token=" + token
		}
	}
	if canSubdomain {
		sub := strings.ToLower(org.Slug) + "." + strings.TrimPrefix(host, "www.")
		if reqHost == sub {
			return scheme + "://" + sub + port + "/trust/sign-in#visitor_token=" + token
		}
	}
	return pathForm
}

// trustVisitorFromRequest resolves the signed-in visitor of org from the
// request's cookie alone: the Authorization header is never read. It returns
// (nil, nil) for "not signed in" (no cookie, unknown, expired, revoked, blocked
// or another organization's session) and an error only for a real failure, so
// callers can tell the two apart.
func (s *Server) trustVisitorFromRequest(c echo.Context, orgID int) (*db.TrustVisitor, error) {
	ck, err := c.Cookie(trustVisitorCookie)
	if err != nil || ck.Value == "" {
		return nil, nil
	}
	v, err := s.db.GetTrustVisitorSession(c.Request().Context(), orgID, hashToken(ck.Value))
	if errors.Is(err, db.ErrTrustVisitorNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

func trustVisitorCookiePath(org *db.Organization) string {
	return trustPublicPrefix + "/" + org.Slug
}

func (s *Server) setTrustVisitorCookie(c echo.Context, org *db.Organization, value string, maxAge int) {
	c.SetCookie(&http.Cookie{
		Name:     trustVisitorCookie,
		Value:    value,
		Path:     trustVisitorCookiePath(org),
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   strings.HasPrefix(os.Getenv("ISMS_BASE_URL"), "https://"),
	})
}

type trustVisitorInfo struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Company string `json:"company"`
}

func trustVisitorResponse(c echo.Context, v *db.TrustVisitor) error {
	var info *trustVisitorInfo
	if v != nil {
		info = &trustVisitorInfo{Email: v.Email, Name: v.Name, Company: v.Company}
	}
	return c.JSON(http.StatusOK, map[string]*trustVisitorInfo{"visitor": info})
}

// trustPublicVisitor answers GET /api/v1/trust/:slug/visitor.
func (s *Server) trustPublicVisitor(c echo.Context, org *db.Organization) error {
	v, err := s.trustVisitorFromRequest(c, org.ID)
	if err != nil {
		return trustPublicError(c, err)
	}
	return trustVisitorResponse(c, v)
}

// trustVisitorPost serves the POST routes under /visitor. Organization, host
// and enabled checks have already passed. route is the path after the slug.
func (s *Server) trustVisitorPost(c echo.Context, org *db.Organization, settings *db.TrustCenterSettings, route string) error {
	if !trustVisitorJSON(c) {
		return trustVisitorReply(c, apiError(http.StatusBadRequest, CodeInvalidRequest))
	}
	body, err := trustVisitorDecode(c)
	if err != nil {
		return trustVisitorReply(c, apiError(http.StatusBadRequest, CodeInvalidRequest))
	}
	switch route {
	case "visitor/link":
		return s.trustVisitorLink(c, org, settings, body)
	case "visitor/link/inspect":
		return s.trustVisitorInspect(c, org, settings, body)
	case "visitor/session":
		return s.trustVisitorSession(c, org, settings, body)
	}
	return s.trustVisitorSignOut(c, org)
}

// trustVisitorLink answers POST /visitor/link.
func (s *Server) trustVisitorLink(c echo.Context, org *db.Organization, settings *db.TrustCenterSettings, body trustVisitorBody) error {
	if s.mailer == nil {
		return trustVisitorReply(c, apiError(http.StatusServiceUnavailable, CodeTrustSignInUnavailable))
	}
	email, ok := parseVisitorEmail(body.Email)
	if !ok {
		return trustVisitorReply(c, apiError(http.StatusBadRequest, CodeInvalidRequest))
	}
	// The one openly refused case: it states policy, not whether an account exists.
	if settings.DomainBlocked(email) {
		return trustVisitorReply(c, apiError(http.StatusBadRequest, CodeTrustEmailDomainBlocked, Value(email[strings.LastIndex(email, "@")+1:])))
	}
	accepted := func() error { return c.JSON(http.StatusAccepted, map[string]string{"status": "sent"}) }

	ctx := c.Request().Context()
	// From here every refusal is silent: same answer, no mail.
	known, err := s.db.GetTrustVisitorByEmail(ctx, org.ID, email)
	if err != nil && !errors.Is(err, db.ErrTrustVisitorNotFound) {
		return trustPublicError(c, err)
	}
	if known != nil && known.Blocked {
		return accepted()
	}
	// Count and insert in one database transaction, so parallel requests cannot
	// all pass the caps before any of them inserts.
	raw, hash := newHashedToken()
	created, reason, err := s.db.CreateTrustVisitorLinkCapped(ctx, org.ID, email, hash, trustLinkTTL, db.TrustLinkCaps{
		PerEmail: trustLinkPerEmail, EmailWindow: trustLinkTTL,
		PerOrg: trustSignInOrgHourly(), OrgWindow: time.Hour,
		PerAddress: trustLinkPerAddress, AddressWindow: time.Hour,
	})
	if err != nil {
		return trustPublicError(c, err)
	}
	if !created {
		if reason == db.TrustLinkCapOrg {
			log.Printf("trust center sign-in: hourly backstop cap reached for organization %q (id %d); no mail sent", org.Slug, org.ID)
		}
		return accepted()
	}
	link := trustSignInLink(os.Getenv("ISMS_BASE_URL"), org, s.subdomainRouting, c.Request().Host, raw)
	s.sendTrustSignInMail(org.ID, email, link)
	return accepted()
}

// sendTrustSignInMail sends the mail in the background so a known, unknown,
// throttled and blocked address all answer in the same time. Failures are
// logged without the address's token or link.
func (s *Server) sendTrustSignInMail(orgID int, to, link string) {
	mailer := s.mailer
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), trustMailTimeout)
		defer cancel()
		branding := s.orgMail(ctx, orgID).Branding
		done := make(chan error, 1)
		go func() { done <- mailer.SendTrustSignInLinkBranded(to, link, branding) }()
		select {
		case err := <-done:
			if err != nil {
				log.Printf("trust center sign-in: sending mail for organization %d failed: %v", orgID, err)
			}
		case <-ctx.Done():
			log.Printf("trust center sign-in: sending mail for organization %d timed out", orgID)
		}
	}()
}

// parseVisitorEmail accepts a bare address only (no display name, no angle
// brackets), at most 320 characters, and returns it lower-cased.
func parseVisitorEmail(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 320 {
		return "", false
	}
	a, err := netmail.ParseAddress(raw)
	if err != nil || a.Name != "" || a.Address != raw || strings.Count(raw, "@") != 1 {
		return "", false
	}
	// An IP literal ("a@[127.0.0.1]") or a bare host is not an address we mail.
	domain := raw[strings.LastIndex(raw, "@")+1:]
	if strings.ContainsAny(domain, "[]") || !strings.Contains(domain, ".") {
		return "", false
	}
	return strings.ToLower(raw), true
}

// trustVisitorInspect answers POST /visitor/link/inspect. It consumes nothing.
func (s *Server) trustVisitorInspect(c echo.Context, org *db.Organization, settings *db.TrustCenterSettings, body trustVisitorBody) error {
	email, v, err := s.db.PeekTrustVisitorLink(c.Request().Context(), org.ID, hashToken(body.Token))
	if errors.Is(err, db.ErrTrustLinkInvalid) {
		return trustVisitorReply(c, apiError(http.StatusBadRequest, CodeTrustLinkInvalid))
	}
	if err != nil {
		return trustPublicError(c, err)
	}
	if v != nil && v.Blocked {
		return trustVisitorReply(c, apiError(http.StatusForbidden, CodeTrustVisitorBlocked))
	}
	// The organization may have blocked this domain since the link was sent.
	if settings.DomainBlocked(email) {
		return trustVisitorReply(c, apiError(http.StatusBadRequest, CodeTrustEmailDomainBlocked, Value(email[strings.LastIndex(email, "@")+1:])))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"email":         email,
		"needs_profile": v == nil || v.Name == "",
	})
}

// trustVisitorSession answers POST /visitor/session: redeem the link, set the cookie.
func (s *Server) trustVisitorSession(c echo.Context, org *db.Organization, settings *db.TrustCenterSettings, body trustVisitorBody) error {
	// A domain blocked since the link was sent cannot sign in. Checked before the
	// link is consumed, so unblocking the domain makes the same link work again
	// within its 15 minutes. (An unknown link falls through to the redeem, which
	// answers it the usual way.)
	if email, _, err := s.db.PeekTrustVisitorLink(c.Request().Context(), org.ID, hashToken(body.Token)); err == nil && settings.DomainBlocked(email) {
		return trustVisitorReply(c, apiError(http.StatusBadRequest, CodeTrustEmailDomainBlocked, Value(email[strings.LastIndex(email, "@")+1:])))
	}
	raw, hash := newHashedToken()
	v, err := s.db.RedeemTrustVisitorLink(c.Request().Context(), org.ID, hashToken(body.Token),
		body.Name, body.Company, hash, trustSessionTTL)
	switch {
	case errors.Is(err, db.ErrTrustLinkInvalid):
		return trustVisitorReply(c, apiError(http.StatusBadRequest, CodeTrustLinkInvalid))
	case errors.Is(err, db.ErrTrustVisitorBlocked):
		return trustVisitorReply(c, apiError(http.StatusForbidden, CodeTrustVisitorBlocked))
	case errors.Is(err, db.ErrTrustVisitorProfileRequired):
		return trustVisitorReply(c, apiError(http.StatusConflict, CodeTrustVisitorProfileRequired))
	case errors.Is(err, db.ErrTrustCenterInvalid):
		return trustVisitorReply(c, apiError(http.StatusBadRequest, CodeInvalidRequest))
	case err != nil:
		return trustPublicError(c, err)
	}
	s.setTrustVisitorCookie(c, org, raw, int(trustSessionTTL/time.Second))
	return trustVisitorResponse(c, v)
}

// trustVisitorSignOut answers POST /visitor/sign-out.
func (s *Server) trustVisitorSignOut(c echo.Context, org *db.Organization) error {
	if ck, err := c.Cookie(trustVisitorCookie); err == nil && ck.Value != "" {
		if err := s.db.RevokeTrustVisitorSession(c.Request().Context(), org.ID, hashToken(ck.Value)); err != nil {
			return trustPublicError(c, err)
		}
	}
	s.setTrustVisitorCookie(c, org, "", -1)
	return trustVisitorResponse(c, nil)
}
