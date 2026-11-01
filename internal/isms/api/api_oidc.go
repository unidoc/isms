package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
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

	// Discover OIDC endpoints
	if provider.DiscoveryURL == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "OIDC provider has no discovery URL configured — re-save the provider in Admin")
	}
	oidcProvider, err := oidc.NewProvider(ctx, provider.DiscoveryURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
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
		return echo.NewHTTPError(http.StatusInternalServerError, "generating state")
	}
	if _, err := rand.Read(nonceBytes); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "generating nonce")
	}
	state := hex.EncodeToString(stateBytes)
	nonce := hex.EncodeToString(nonceBytes)

	// Store session (10 minute expiry)
	session := &db.OIDCSession{
		State:          state,
		Nonce:          nonce,
		ProviderID:     provider.ID,
		OrganizationID: org.ID,
		RedirectURI:    redirectURI,
		ExpiresAt:      time.Now().Add(10 * time.Minute),
	}
	if err := s.db.CreateOIDCSession(ctx, session); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating OIDC session: "+err.Error())
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

	authURL := oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce))
	return c.Redirect(http.StatusFound, authURL)
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
