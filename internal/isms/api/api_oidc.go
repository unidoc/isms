package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
	"isms.sh/internal/isms/db"
)

// handleOIDCProviders returns the list of enabled OIDC providers for an organization.
// GET /auth/oidc/providers?org=slug
func (s *Server) handleOIDCProviders(c echo.Context) error {
	slug := c.QueryParam("org")
	if slug == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "org query parameter required")
	}

	ctx := c.Request().Context()
	org, err := s.db.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}

	providers, err := s.db.ListEnabledOIDCProviders(ctx, org.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing providers: "+err.Error())
	}
	if providers == nil {
		providers = []db.OIDCProvider{}
	}

	// ClientSecret is json:"-" so it is already excluded from the response.
	return c.JSON(http.StatusOK, map[string]interface{}{"data": providers})
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
		return echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}

	provider, err := s.db.GetOIDCProvider(ctx, org.ID, providerName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "OIDC provider not found")
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

	// Clean up expired sessions in the background
	go s.db.DeleteExpiredOIDCSessions(ctx)

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

	// User resolution:
	// 1. Try by linked identity (provider + sub)
	// 2. Try by email (and link)
	// 3. Create new user
	var user *db.User

	user, err = s.db.GetUserByIdentity(ctx, provider.ProviderName, claims.Sub)
	if err != nil {
		// Not found by identity — try email
		user, err = s.db.GetUserByEmail(ctx, claims.Email)
		if err != nil {
			// Not found by email either — create new user
			name := claims.Name
			if name == "" {
				name = claims.Email
			}
			user = &db.User{
				Email:  claims.Email,
				Name:   name,
				Active: true,
			}
			if err := s.db.UpsertUser(ctx, user); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "creating user: "+err.Error())
			}
		}
		// Link identity to user
		if err := s.db.LinkIdentity(ctx, user.ID, provider.ProviderName, claims.Sub, claims.Email); err != nil {
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
	token, err := s.createSessionJWT(user, orgID, role, org.Slug, org.Name)
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
	redirectURL := orgTokenRedirectURL(baseURL, org.Slug, token, role, s.subdomainRouting)
	return c.Redirect(http.StatusFound, redirectURL)
}

// orgTokenRedirectURL builds the post-OIDC-login redirect for a given org.
// Prefers a subdomain hop (https://<slug>.<apex>/#token=...) when the base URL
// has a hostname that can host subdomains; falls back to path-based routing
// (<base>/<slug>/#token=...) for single-label / localhost hosts.
func orgTokenRedirectURL(baseURL, slug, token, role string, subdomainRouting bool) string {
	// Parse the base URL — extract scheme and host so we can splice in the slug.
	// Cheap parse: assume baseURL like "https://isms.sh" or "http://localhost:9090".
	const sep = "://"
	i := strings.Index(baseURL, sep)
	if i < 0 {
		// Malformed base — fall back to apex with fragment, SPA will route from there.
		return fmt.Sprintf("%s/#token=%s&role=%s", baseURL, token, role)
	}
	scheme := baseURL[:i]
	hostAndPort := baseURL[i+len(sep):]
	// Split host:port
	host := hostAndPort
	port := ""
	if c := strings.LastIndex(hostAndPort, ":"); c > 0 {
		host = hostAndPort[:c]
		port = hostAndPort[c:] // includes ':'
	}
	// Subdomain hop only when the deployment routes by subdomain; otherwise stay
	// path-based so the redirect matches how requests are actually routed.
	canSubdomain := subdomainRouting &&
		strings.Contains(host, ".") &&
		!strings.HasPrefix(host, "localhost") &&
		!isIPLiteral(host)
	if canSubdomain {
		// www.isms.sh → isms.sh
		apex := strings.TrimPrefix(host, "www.")
		// Land on /login — Login.vue's onMounted handler parses the token
		// fragment, sets the session, and routes onward to /overview.
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
