package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
	"isms.sh/internal/isms/db"
)

// publicOIDCProvider is what the unauthenticated provider list exposes: just
// enough for the login page to render a button and start the flow. Never add
// credentials or provider configuration here.
type publicOIDCProvider struct {
	ID           int    `json:"id"`
	ProviderName string `json:"provider_name"`
	DisplayName  string `json:"display_name"`
}

// handleOIDCProviders returns the list of enabled OIDC providers for an organization.
// Public route: only the fields in publicOIDCProvider are returned.
// GET /auth/oidc/providers?org=slug
func (s *Server) handleOIDCProviders(c echo.Context) error {
	slug := c.QueryParam("org")
	if slug == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "org query parameter required")
	}

	ctx := c.Request().Context()
	org, err := s.db.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		return errNotFound("organization")
	}

	providers, err := s.db.ListEnabledOIDCProviders(ctx, org.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing providers: "+err.Error())
	}
	out := make([]publicOIDCProvider, 0, len(providers))
	for _, p := range providers {
		out = append(out, publicOIDCProvider{ID: p.ID, ProviderName: p.ProviderName, DisplayName: p.DisplayName})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"data": out})
}

// handleOIDCAuthorize initiates the OIDC authorization code flow.
// GET /auth/oidc/authorize?provider=microsoft&org=slug
func (s *Server) handleOIDCAuthorize(c echo.Context) error {
	slug := c.QueryParam("org")
	providerName := c.QueryParam("provider")
	if slug == "" || providerName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "org and provider query parameters required")
	}

	ctx := c.Request().Context()
	org, err := s.db.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		return errNotFound("organization")
	}

	provider, err := s.db.GetOIDCProvider(ctx, org.ID, providerName)
	if err != nil {
		return errNotFound("oidc_provider")
	}
	if !provider.Enabled {
		return echo.NewHTTPError(http.StatusBadRequest, "OIDC provider is disabled")
	}

	authURL, err := s.beginOIDCFlow(ctx, org.ID, provider, 0)
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, authURL)
}

// beginOIDCFlow discovers the provider's endpoints, stores the single-use
// state/nonce session and returns the IdP authorization URL. linkUserID > 0
// marks a flow that links the identity to that signed-in user instead of
// signing anyone in.
func (s *Server) beginOIDCFlow(ctx context.Context, orgID int, provider *db.OIDCProvider, linkUserID int) (string, error) {
	// Discover OIDC endpoints
	if provider.DiscoveryURL == "" {
		return "", echo.NewHTTPError(http.StatusInternalServerError, "OIDC provider has no discovery URL configured — re-save the provider in Admin")
	}
	oidcProvider, err := oidc.NewProvider(ctx, provider.DiscoveryURL)
	if err != nil {
		return "", echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("OIDC discovery failed for %q: %v", provider.DiscoveryURL, err))
	}

	// Single canonical callback URL on the apex — one redirect_uri registered
	// in the IdP regardless of which tenant subdomain initiated the flow. The
	// callback handler reads org_id from the OIDC session and hops the user
	// to their subdomain on success.
	redirectURI := strings.TrimRight(os.Getenv("ISMS_BASE_URL"), "/") + "/api/v1/auth/oidc/callback"

	scopes := []string{oidc.ScopeOpenID, "email", "profile"}
	if provider.Scopes != "" {
		scopes = strings.Split(provider.Scopes, " ")
	}

	oauth2Config := oauth2.Config{
		ClientID:     provider.ClientID,
		ClientSecret: provider.ClientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       scopes,
	}

	// Generate crypto-random state and nonce
	stateBytes := make([]byte, 32)
	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", echo.NewHTTPError(http.StatusInternalServerError, "generating state")
	}
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", echo.NewHTTPError(http.StatusInternalServerError, "generating nonce")
	}
	state := hex.EncodeToString(stateBytes)
	nonce := hex.EncodeToString(nonceBytes)

	// Store session (10 minute expiry)
	session := &db.OIDCSession{
		State:          state,
		Nonce:          nonce,
		ProviderID:     provider.ID,
		OrganizationID: orgID,
		LinkUserID:     linkUserID,
		RedirectURI:    redirectURI,
		ExpiresAt:      time.Now().Add(10 * time.Minute),
	}
	if err := s.db.CreateOIDCSession(ctx, session); err != nil {
		return "", echo.NewHTTPError(http.StatusInternalServerError, "creating OIDC session: "+err.Error())
	}

	// Clean up expired sessions in the background. A detached context so the
	// DELETE survives request completion (the #114 bug), but bounded by a
	// timeout so a slow/overloaded DB can't leave these fire-and-forget
	// goroutines blocked in Exec indefinitely — one is spawned per authorize
	// call, so an unbounded wait would let them pile up under DB stress.
	go func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.db.DeleteExpiredOIDCSessions(cleanupCtx)
	}()

	return oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce)), nil
}

// handleOIDCCallback handles the OIDC authorization code callback.
// GET /auth/oidc/callback?code=xxx&state=yyy
func (s *Server) handleOIDCCallback(c echo.Context) error {
	code := c.QueryParam("code")
	state := c.QueryParam("state")
	if code == "" || state == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code and state parameters required")
	}

	ctx := c.Request().Context()

	// Look up and consume session (single-use delete)
	session, err := s.db.LookupOIDCSession(ctx, state)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired OIDC session")
	}
	if time.Now().After(session.ExpiresAt) {
		return echo.NewHTTPError(http.StatusBadRequest, "OIDC session expired")
	}

	// Get provider config
	provider, err := s.db.GetOIDCProviderByID(ctx, session.ProviderID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "OIDC provider not found")
	}

	// Discover OIDC endpoints again
	oidcProvider, err := oidc.NewProvider(ctx, provider.DiscoveryURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "OIDC discovery failed: "+err.Error())
	}

	scopes := []string{oidc.ScopeOpenID, "email", "profile"}
	if provider.Scopes != "" {
		scopes = strings.Split(provider.Scopes, " ")
	}

	oauth2Config := oauth2.Config{
		ClientID:     provider.ClientID,
		ClientSecret: provider.ClientSecret,
		RedirectURL:  session.RedirectURI,
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       scopes,
	}

	// Exchange authorization code for tokens
	oauth2Token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "token exchange failed: "+err.Error())
	}

	// Extract and verify ID token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "no id_token in response")
	}

	verifier := oidcProvider.Verifier(&oidc.Config{ClientID: provider.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "ID token verification failed: "+err.Error())
	}

	// Verify nonce matches
	if idToken.Nonce != session.Nonce {
		return echo.NewHTTPError(http.StatusUnauthorized, "nonce mismatch")
	}

	// Extract claims. Microsoft Entra ID often omits `email` and instead returns
	// `preferred_username` or `upn` (the user's UPN, which is email-shaped for
	// most work accounts). Accept any of them as the email identity.
	var claims struct {
		Email             string `json:"email"`
		EmailVerified     *bool  `json:"email_verified"`
		Name              string `json:"name"`
		Sub               string `json:"sub"`
		PreferredUsername string `json:"preferred_username"`
		UPN               string `json:"upn"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "parsing claims: "+err.Error())
	}
	if claims.Email == "" {
		// Microsoft fallback chain
		if strings.Contains(claims.PreferredUsername, "@") {
			claims.Email = claims.PreferredUsername
		} else if strings.Contains(claims.UPN, "@") {
			claims.Email = claims.UPN
		}
	}
	// A flow started from Settings links the identity to the signed-in user who
	// started it; the live session was the proof, so nothing else is resolved.
	if session.LinkUserID > 0 {
		return s.finishOIDCLink(c, session, provider, idToken.Issuer, claims.Sub, claims.Email)
	}
	if claims.Email == "" {
		return echo.NewHTTPError(http.StatusBadRequest,
			"no email identifier in ID token — IdP must include `email`, `preferred_username`, or `upn` claim")
	}
	// Reject unverified emails when auto-add is enabled (prevents onboarding bypass)
	if claims.EmailVerified != nil && !*claims.EmailVerified && provider.AutoAddMembers {
		return echo.NewHTTPError(http.StatusForbidden, "email address is not verified by the identity provider")
	}

	// User resolution. The only things that prove who someone is are the ID
	// token's issuer (bound to the keys that signed it, so another organization's
	// provider cannot claim it) and its subject. The provider name and the email
	// claim are chosen by whoever runs the provider, so neither identifies an
	// existing account on its own.
	//  1. A link by (issuer, subject): that user.
	//  2. A link made before issuers were recorded (provider name + subject),
	//     honoured only for a member of this provider's organization; the issuer
	//     is filled in so the row is keyed properly from now on.
	//  3. No link: no account for the email creates one; an account that has no
	//     way to sign in yet and belongs to no other organization (an admin
	//     pre-added it) is linked; anything else is refused.
	var user *db.User
	issuer := idToken.Issuer

	user, err = s.db.GetUserByIdentity(ctx, issuer, claims.Sub)
	if err != nil {
		var legacyID int
		user, legacyID, err = s.db.GetUserByLegacyIdentity(ctx, session.OrganizationID, provider.ProviderName, claims.Sub)
		if err == nil {
			if err := s.db.SetIdentityIssuer(ctx, legacyID, issuer); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "updating identity: "+err.Error())
			}
		}
	}
	if err != nil {
		user, err = s.db.GetUserByEmailFold(ctx, claims.Email)
		if err != nil {
			// No account for this email — create one
			name := claims.Name
			if name == "" {
				name = claims.Email
			}
			user = &db.User{
				Email:  claims.Email,
				Name:   name,
				Active: true,
			}
			if _, err := s.db.EnsureUser(ctx, user); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "creating user: "+err.Error())
			}
		} else {
			ok, err := s.canLinkByEmail(ctx, user.ID, session.OrganizationID)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "checking account: "+err.Error())
			}
			if !ok {
				// Nothing is linked, no session is minted, no membership added.
				org, err := s.db.GetOrganization(ctx, session.OrganizationID)
				if err != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, "organization not found")
				}
				baseURL := strings.TrimRight(os.Getenv("ISMS_BASE_URL"), "/")
				return c.Redirect(http.StatusFound, orgLoginErrorURL(baseURL, org.Slug, org.Domain, "account_exists", s.subdomainRouting))
			}
		}
		// Link identity to user
		if err := s.db.LinkIdentity(ctx, user.ID, issuer, provider.ProviderName, claims.Sub, claims.Email); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "linking identity: "+err.Error())
		}
	}

	// Org membership resolution
	orgID := session.OrganizationID
	_, err = s.db.GetOrgMember(ctx, orgID, user.ID)
	if err != nil {
		// Not a member
		if provider.AutoAddMembers {
			role := provider.DefaultRole
			if role == "" {
				role = "reader"
			}
			if err := s.db.AddOrgMember(ctx, orgID, user.ID, role); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "adding org member: "+err.Error())
			}
		} else {
			return echo.NewHTTPError(http.StatusForbidden, "not a member of this organization and auto-add is disabled")
		}
	}

	// Mark email as verified (OIDC provider verified it)
	if !user.EmailVerified {
		s.db.SetEmailVerified(ctx, user.ID)
	}

	// Get role
	role, err := s.db.GetUserRole(ctx, orgID, user.ID)
	if err != nil {
		role = "reader"
	}

	// Get org details for JWT claims
	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "organization not found")
	}

	// Create signed JWT session token (stateless — no DB row)
	token, err := s.createContainedSessionJWT(user, orgID, role, org.Slug, org.Name, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating session token: "+err.Error())
	}

	// Log activity
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user.Email,
		Action: "oidc_login",
		Detail: fmt.Sprintf("Logged in via %s (%s)", provider.DisplayName, provider.ProviderName),
	})

	// Redirect to the org's canonical entry URL with the token in the URL
	// fragment. Fragments are never sent to the server, not logged in access
	// logs, and not sent in Referer headers.
	//
	// When the apex domain supports subdomains (e.g. ISMS_BASE_URL=https://isms.sh),
	// hop to https://<slug>.isms.sh/. Otherwise stay on the apex with #token=...
	// and let the SPA route via /:org-prefixed paths.
	baseURL := strings.TrimRight(os.Getenv("ISMS_BASE_URL"), "/")
	redirectURL := orgTokenRedirectURL(baseURL, org.Slug, org.Domain, token, role, s.subdomainRouting)
	return c.Redirect(http.StatusFound, redirectURL)
}

// finishOIDCLink handles the callback of a link flow. The identity provider's
// answer reaches the server through a browser that may not belong to the user
// who started the flow (a link can be forwarded), so nothing is linked here:
// the verified identity is parked behind a one-time code and the browser goes
// to Settings, where the app completes the link with the signed-in session
// (handleOIDCLinkComplete). No session is minted and no membership changes.
func (s *Server) finishOIDCLink(c echo.Context, session *db.OIDCSession, provider *db.OIDCProvider, issuer, sub, email string) error {
	ctx := c.Request().Context()
	org, err := s.db.GetOrganization(ctx, session.OrganizationID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "organization not found")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "generating code")
	}
	code := hex.EncodeToString(raw)
	pending := &db.OIDCLinkPending{
		UserID: session.LinkUserID, OrganizationID: session.OrganizationID,
		Provider: provider.ProviderName, Issuer: issuer, Subject: sub, Email: email,
	}
	if err := s.db.CreateOIDCLinkPending(ctx, sha256Hash(code), pending, time.Now().Add(10*time.Minute)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "storing link: "+err.Error())
	}
	baseURL := strings.TrimRight(os.Getenv("ISMS_BASE_URL"), "/")
	return c.Redirect(http.StatusFound, orgSettingsLinkURL(baseURL, org.Slug, org.Domain, "sso_link_code", code, s.subdomainRouting))
}

// handleOIDCLinkComplete finishes a link flow from inside the app: the code
// from the callback redirect must belong to the signed-in caller, who started
// the flow. A code presented by anyone else is burned and refused.
// POST /api/v1/auth/oidc/link/complete
func (s *Server) handleOIDCLinkComplete(c echo.Context) error {
	if err := refuseAPIKey(c); err != nil {
		return err
	}
	if err := refuseSSOSession(c); err != nil {
		return err
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := c.Bind(&req); err != nil || req.Code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code is required")
	}
	ctx := c.Request().Context()
	user, err := s.db.GetUserByEmail(ctx, getUserEmail(c))
	if err != nil {
		return apiError(http.StatusUnauthorized, CodeNotFound, Entity("user"))
	}
	p, err := s.db.ConsumeOIDCLinkPending(ctx, sha256Hash(req.Code))
	if errors.Is(err, pgx.ErrNoRows) {
		return echo.NewHTTPError(http.StatusBadRequest, "this link has expired or was already used; start again from Settings")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "reading link: "+err.Error())
	}
	if p.UserID != user.ID || p.OrganizationID != getOrgID(c) {
		return echo.NewHTTPError(http.StatusForbidden, "this link was started by another account")
	}
	if _, err := s.db.GetOrgMember(ctx, p.OrganizationID, user.ID); err != nil {
		return echo.NewHTTPError(http.StatusForbidden, "not a member of this organization")
	}
	result, err := s.applyOIDCLink(ctx, user.ID, p)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"result": result})
}

// applyOIDCLink attaches the identity (issuer, subject) to the user unless it
// already belongs to someone else. It returns "linked" (also when the same
// user already had it) or "in_use". Only "no such row" means not found.
func (s *Server) applyOIDCLink(ctx context.Context, userID int, p *db.OIDCLinkPending) (string, error) {
	existing, err := s.db.GetUserByIdentity(ctx, p.Issuer, p.Subject)
	if err == nil {
		if existing.ID != userID {
			return "in_use", nil
		}
		return "linked", nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("looking up identity: %w", err)
	}
	legacy, legacyID, err := s.db.GetUserByLegacyIdentity(ctx, p.OrganizationID, p.Provider, p.Subject)
	if err == nil {
		// A pre-issuer link: the same user adopts it, anyone else owns it.
		if legacy.ID != userID {
			return "in_use", nil
		}
		if err := s.db.SetIdentityIssuer(ctx, legacyID, p.Issuer); err != nil {
			return "", fmt.Errorf("updating identity: %w", err)
		}
		return "linked", nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("looking up legacy identity: %w", err)
	}
	if err := s.db.LinkIdentity(ctx, userID, p.Issuer, p.Provider, p.Subject, p.Email); err != nil {
		return "", fmt.Errorf("linking identity: %w", err)
	}
	return "linked", nil
}

// handleOIDCLinkBegin starts linking the current organization's provider to
// the signed-in user's account. It is a POST returning the IdP URL (rather
// than a redirect) because a bearer token cannot ride a browser navigation.
// POST /api/v1/auth/oidc/link
func (s *Server) handleOIDCLinkBegin(c echo.Context) error {
	if err := refuseAPIKey(c); err != nil {
		return err
	}
	if err := refuseSSOSession(c); err != nil {
		return err
	}
	var req struct {
		Provider string `json:"provider"`
	}
	if err := c.Bind(&req); err != nil || req.Provider == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "provider is required")
	}
	ctx := c.Request().Context()
	orgID := getOrgID(c)
	if orgID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no current organization")
	}
	user, err := s.db.GetUserByEmail(ctx, getUserEmail(c))
	if err != nil {
		return apiError(http.StatusUnauthorized, CodeNotFound, Entity("user"))
	}
	provider, err := s.db.GetOIDCProvider(ctx, orgID, req.Provider)
	if err != nil || !provider.Enabled {
		return errNotFound("oidc_provider")
	}
	authURL, err := s.beginOIDCFlow(ctx, orgID, provider, user.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"url": authURL})
}

type identityItem struct {
	ID        int      `json:"id"`
	Provider  string   `json:"provider"`
	Issuer    string   `json:"issuer,omitempty"`
	Email     string   `json:"email,omitempty"`
	CreatedAt db.Epoch `json:"created_at"`
}

// handleListIdentities returns the caller's linked single sign-on identities.
// GET /api/v1/auth/identities
func (s *Server) handleListIdentities(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := s.db.GetUserByEmail(ctx, getUserEmail(c))
	if err != nil {
		return apiError(http.StatusUnauthorized, CodeNotFound, Entity("user"))
	}
	ids, err := s.db.ListUserIdentities(ctx, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing identities: "+err.Error())
	}
	items := make([]identityItem, 0, len(ids))
	for _, i := range ids {
		items = append(items, identityItem{ID: i.ID, Provider: i.Provider, Issuer: i.Issuer, Email: i.Email, CreatedAt: i.CreatedAt})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": items})
}

// handleUnlinkIdentity removes one of the caller's linked identities, unless
// it is the account's last way to sign in.
// DELETE /api/v1/auth/identities/:id
func (s *Server) handleUnlinkIdentity(c echo.Context) error {
	if err := refuseAPIKey(c); err != nil {
		return err
	}
	if err := refuseSSOSession(c); err != nil {
		return err
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid identity ID")
	}
	ctx := c.Request().Context()
	user, err := s.db.GetUserByEmail(ctx, getUserEmail(c))
	if err != nil {
		return apiError(http.StatusUnauthorized, CodeNotFound, Entity("user"))
	}
	ids, err := s.db.ListUserIdentities(ctx, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing identities: "+err.Error())
	}
	found := false
	for _, i := range ids {
		if i.ID == id {
			found = true
		}
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, "identity not found")
	}
	passkeys, err := s.db.CountWebAuthnCredentials(ctx, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "counting passkeys: "+err.Error())
	}
	if !user.HasPassword() && passkeys == 0 && len(ids) <= 1 {
		return echo.NewHTTPError(http.StatusConflict, "this is the only way to sign in to the account; set a password or add a passkey first")
	}
	if err := s.db.UnlinkIdentity(ctx, user.ID, id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "unlinking identity: "+err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "unlinked"})
}

// orgSettingsLinkURL is where the browser lands after a link flow's identity
// provider leg: the org's Settings page (same host and routing rules as
// orgTokenRedirectURL) with one query parameter.
func orgSettingsLinkURL(baseURL, slug string, domain *string, key, value string, subdomainRouting bool) string {
	u, _, _ := strings.Cut(orgTokenRedirectURL(baseURL, slug, domain, "", "", subdomainRouting), "#")
	u = strings.TrimRight(strings.TrimSuffix(u, "/login"), "/")
	return u + "/settings?" + key + "=" + url.QueryEscape(value)
}

// canLinkByEmail reports whether a sign-in through the provider of orgID may
// attach to the existing account userID on the strength of the email claim
// alone: only when the account is bare (no password, passkey or linked
// identity) and every organization it belongs to is orgID.
func (s *Server) canLinkByEmail(ctx context.Context, userID, orgID int) (bool, error) {
	bare, err := s.db.IsBareAccount(ctx, userID)
	if err != nil || !bare {
		return false, err
	}
	orgIDs, err := s.db.ListUserOrgIDs(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, id := range orgIDs {
		if id != orgID {
			return false, nil
		}
	}
	return true, nil
}

// orgLoginErrorURL is where a refused SSO sign-in lands: the same login page
// orgTokenRedirectURL targets (custom domain, subdomain or path), with an error
// code in the query string instead of a token.
func orgLoginErrorURL(baseURL, slug string, domain *string, code string, subdomainRouting bool) string {
	u, _, _ := strings.Cut(orgTokenRedirectURL(baseURL, slug, domain, "", "", subdomainRouting), "#")
	return u + "?sso_error=" + url.QueryEscape(code)
}

// orgTokenRedirectURL builds the post-OIDC-login redirect for a given org.
// Precedence mirrors orgURLs (server.go): a configured custom domain wins, then
// a subdomain hop (https://<slug>.<apex>/login#token=...) when the deployment
// routes by subdomain, otherwise path-based routing (<base>/<slug>/login#...).
// Sharing resolveSubdomainHost keeps this decision identical to orgURLs, so the
// login redirect always lands on the same host the org's other links use.
func orgTokenRedirectURL(baseURL, slug string, domain *string, token, role string, subdomainRouting bool) string {
	// Custom domain wins — an org with its own domain (e.g. audit.example.com) must
	// land its OIDC login there, not on the apex subdomain/path.
	if domain != nil && *domain != "" {
		d := *domain
		if !strings.Contains(d, "://") {
			scheme := "https"
			if strings.HasPrefix(baseURL, "http://") {
				scheme = "http"
			}
			d = scheme + "://" + d
		}
		d = strings.TrimRight(d, "/")
		return fmt.Sprintf("%s/login#token=%s&role=%s", d, token, role)
	}
	scheme, host, port, canSubdomain := resolveSubdomainHost(baseURL, subdomainRouting)
	if scheme == "" {
		// Malformed base — fall back to apex with fragment, SPA will route from there.
		return fmt.Sprintf("%s/#token=%s&role=%s", baseURL, token, role)
	}
	if canSubdomain {
		// www.isms.sh → isms.sh. Land on /login — Login.vue's onMounted handler
		// parses the token fragment, sets the session, and routes to /overview.
		apex := strings.TrimPrefix(host, "www.")
		return fmt.Sprintf("%s://%s.%s%s/login#token=%s&role=%s", scheme, slug, apex, port, token, role)
	}
	// Path-based fallback (e.g. localhost dev): https://localhost:9090/<slug>/login#token=...
	return fmt.Sprintf("%s/%s/login#token=%s&role=%s", baseURL, slug, token, role)
}

func isIPLiteral(host string) bool {
	if host == "" {
		return false
	}
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}
