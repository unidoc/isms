package api

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// AuthConfig configures the authentication middleware.
type AuthConfig struct {
	CloudflareTeamDomain string // e.g. "mycompany.cloudflareaccess.com"
	CloudflareAudience   string // CF Access Application Audience (AUD) tag — set via ISMS_CF_AUDIENCE
	DB                   *db.DB // for API token lookups
	Secret               string // for validating JWT session tokens
}

// AuthMiddleware validates authentication on all API routes.
// Two auth methods: Bearer token or Cloudflare Zero Trust. No exceptions.
func AuthMiddleware(cfg AuthConfig) echo.MiddlewareFunc {
	var keyCache *cfKeyCache
	if cfg.CloudflareTeamDomain != "" {
		keyCache = newCFKeyCache(cfg.CloudflareTeamDomain, cfg.CloudflareAudience)
		if cfg.CloudflareAudience == "" {
			log.Println("WARNING: ISMS_CF_AUDIENCE not set — Cloudflare Access JWT audience validation is disabled. Any CF Access JWT from any application will be accepted. Set ISMS_CF_AUDIENCE to the Application Audience (AUD) tag from your CF Access dashboard.")
		}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path

			// Paths that don't require auth but benefit from org context if a token is present.
			softAuthPaths := path == "/api/v1/config" || strings.HasPrefix(path, "/branding/") ||
				path == "/terms" || path == "/privacy"

			// Hard-skip: no auth processing at all (login, signup, health, etc.)
			if path == "/healthz" ||
				path == "/docs" || path == "/api/openapi.yaml" ||
				path == "/api/v1/auth/login" || path == "/api/v1/auth/signup" || path == "/api/v1/auth/verify-email" ||
				path == "/api/v1/auth/forgot-password" ||
				path == "/api/v1/auth/passkey/login/begin" || path == "/api/v1/auth/passkey/login/complete" ||
				path == "/api/v1/auth/oidc/providers" || path == "/api/v1/auth/oidc/authorize" || path == "/api/v1/auth/oidc/callback" {
				return next(c)
			}

			// Soft auth: try to extract org context from JWT if present, but never block.
			// This lets /config and /branding/ serve the right org in multi-tenant setups.
			if softAuthPaths {
				if authHeader := c.Request().Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
					rawToken := strings.TrimPrefix(authHeader, "Bearer ")
					if cfg.Secret != "" && !strings.HasPrefix(rawToken, "isms_") {
						if claims, err := validateSessionJWT(rawToken, cfg.Secret); err == nil {
							if claims.OrganizationID > 0 {
								c.Set("org_id", claims.OrganizationID)
							}
							c.Set("user_email", claims.Email)
						}
					}
				}
				return next(c)
			}

			// Skip auth for static files (Vue SPA) — but not /api/ or /git/ paths
			if !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/git/") {
				return next(c)
			}

			// Support Basic auth for git HTTP clients: username is ignored,
			// password is treated as the Bearer token.
			if username, password, ok := c.Request().BasicAuth(); ok && username != "" && password != "" {
				_ = username // git clients send "x-token-auth" or similar; we only care about the password
				c.Request().Header.Set("Authorization", "Bearer "+password)
			}

			// 1. Bearer token auth (API keys or JWT sessions)
			if authHeader := c.Request().Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
				rawToken := strings.TrimPrefix(authHeader, "Bearer ")

				// API key (starts with isms_ prefix) — DB lookup
				if strings.HasPrefix(rawToken, "isms_") {
					if cfg.DB != nil {
						hash := sha256.Sum256([]byte(rawToken))
						tokenHash := hex.EncodeToString(hash[:])
						tok, err := cfg.DB.LookupAPIKey(c.Request().Context(), tokenHash)
						if err == nil && tok != nil {
							// Set user identity from token.
							c.Set("user_email", tok.UserEmail)
							c.Set("api_key_id", tok.ID)
							c.Set("api_key_permissions", tok.Permissions)

							// If OrgResolverMiddleware already set org_id (from subdomain/domain/path),
							// verify the user is a member and set their role.
							if resolvedOrgID, ok := c.Get("org_id").(int); ok && resolvedOrgID > 0 {
								// Enforce org scope: if the token is scoped to a specific org,
								// reject requests targeting a different org.
								if tok.OrganizationID != nil && *tok.OrganizationID != resolvedOrgID {
									return echo.NewHTTPError(http.StatusForbidden, "API key is not authorized for this organization")
								}
								role, err := cfg.DB.GetUserRole(c.Request().Context(), resolvedOrgID, tok.UserID)
								if err != nil {
									return echo.NewHTTPError(http.StatusForbidden, "not a member of this organization")
								}
								c.Set("user_role", role)
							} else if orgUUID := c.Request().Header.Get("X-Organization-UUID"); orgUUID != "" {
								// Best-effort org resolve from header — don't block auth if UUID is wrong
								if org, err := cfg.DB.GetOrganizationByUUID(c.Request().Context(), orgUUID); err == nil {
									// Enforce org scope: if the token is scoped to a specific org,
									// reject requests targeting a different org.
									if tok.OrganizationID != nil && *tok.OrganizationID != org.ID {
										return echo.NewHTTPError(http.StatusForbidden, "API key is not authorized for this organization")
									}
									if role, err := cfg.DB.GetUserRole(c.Request().Context(), org.ID, tok.UserID); err == nil {
										c.Set("org_id", org.ID)
										c.Set("user_role", role)
									}
								}
							} else if tok.OrganizationID != nil {
								// No org resolved from subdomain or header, but token is org-scoped.
								// Auto-select the token's org so scoped tokens work without
								// requiring X-Organization-UUID header.
								role, err := cfg.DB.GetUserRole(c.Request().Context(), *tok.OrganizationID, tok.UserID)
								if err == nil {
									c.Set("org_id", *tok.OrganizationID)
									c.Set("user_role", role)
								}
							}

							// AI kill switch: block agent users when ai_enabled = false
							if resolvedOrgID, ok := c.Get("org_id").(int); ok && resolvedOrgID > 0 {
								if cfg.DB.IsUserAgent(c.Request().Context(), tok.UserEmail) {
									aiEnabled, _ := cfg.DB.GetOrgSetting(c.Request().Context(), resolvedOrgID, "ai_enabled")
									if aiEnabled == "false" {
										return echo.NewHTTPError(http.StatusForbidden, "AI features are disabled for this organization")
									}
								}
							}

							return next(c)
						}
					}
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid API key")
				}

				// JWT session token (everything without isms_ prefix)
				if cfg.Secret != "" {
					claims, err := validateSessionJWT(rawToken, cfg.Secret)
					if err == nil {
						// Check JWT blocklist (revoked tokens)
						tokenHash := sha256Hash(rawToken)
						if cfg.DB.IsJWTBlocked(c.Request().Context(), tokenHash) {
							return echo.NewHTTPError(http.StatusUnauthorized, "token revoked")
						}

						// If OrgResolverMiddleware already set org_id (from subdomain/domain),
						// verify the JWT belongs to that org.
						if resolvedOrgID, ok := c.Get("org_id").(int); ok && resolvedOrgID > 0 {
							if claims.OrganizationID != resolvedOrgID {
								return echo.NewHTTPError(http.StatusForbidden, "session does not belong to this organization")
							}
						}
						// Verify user still exists and is active
						jwtUser, jwtErr := cfg.DB.GetUserByEmail(c.Request().Context(), claims.Email)
						if jwtErr != nil || !jwtUser.Active {
							return echo.NewHTTPError(http.StatusUnauthorized, "user not found or inactive")
						}
						c.Set("user_email", claims.Email)

						// Verify org membership from DB (not JWT claims — they can be stale)
						if claims.OrganizationID > 0 {
							dbRole, roleErr := cfg.DB.GetUserRole(c.Request().Context(), claims.OrganizationID, jwtUser.ID)
							if roleErr != nil {
								// Org gone or user removed — allow request but with no org context
								c.Set("org_id", 0)
								c.Set("user_role", "")
							} else {
								c.Set("org_id", claims.OrganizationID)
								c.Set("user_role", dbRole)

								// AI kill switch for JWT-authenticated agent users
								if jwtUser.IsAgent {
									aiEnabled, _ := cfg.DB.GetOrgSetting(c.Request().Context(), claims.OrganizationID, "ai_enabled")
									if aiEnabled == "false" {
										return echo.NewHTTPError(http.StatusForbidden, "AI features are disabled for this organization")
									}
								}
							}
						} else {
							c.Set("org_id", 0)
							c.Set("user_role", "")
						}
						return next(c)
					}
				}

				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}

			// For git paths: challenge with WWW-Authenticate so git client sends credentials
			if strings.HasPrefix(path, "/git/") {
				c.Response().Header().Set("WWW-Authenticate", `Basic realm="isms"`)
				return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
			}

			// 2. Cloudflare Zero Trust JWT
			email := c.Request().Header.Get("Cf-Access-Authenticated-User-Email")
			if email == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "API key required. Use: isms server api-key create")
			}

			// CF headers are only trusted when Cloudflare is configured
			if keyCache == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "API key required. Use: isms server api-key create")
			}

			{
				jwt := c.Request().Header.Get("Cf-Access-Jwt-Assertion")
				if jwt == "" {
					return echo.NewHTTPError(http.StatusUnauthorized, "missing access token")
				}
				claims, err := keyCache.VerifyJWT(jwt)
				if err != nil {
					return echo.NewHTTPError(http.StatusUnauthorized, fmt.Sprintf("invalid token: %v", err))
				}
				if claims.Email != "" && claims.Email != email {
					return echo.NewHTTPError(http.StatusUnauthorized, "token email mismatch")
				}
			}

			// Resolve user and org for CF-authenticated users
			c.Set("user_email", email)
			ctx := c.Request().Context()
			user, err := cfg.DB.GetUserByEmail(ctx, email)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
			}

			// If OrgResolverMiddleware already resolved the org (subdomain/domain/path),
			// use that and verify membership.
			if resolvedOrgID, ok := c.Get("org_id").(int); ok && resolvedOrgID > 0 {
				member, err := cfg.DB.GetOrgMember(ctx, resolvedOrgID, user.ID)
				if err != nil {
					return echo.NewHTTPError(http.StatusForbidden, "not a member of this organization")
				}
				c.Set("org_id", resolvedOrgID)
				c.Set("user_role", member.Role)

				// AI kill switch for CF-authenticated agent users
				if user.IsAgent {
					aiEnabled, _ := cfg.DB.GetOrgSetting(ctx, resolvedOrgID, "ai_enabled")
					if aiEnabled == "false" {
						return echo.NewHTTPError(http.StatusForbidden, "AI features are disabled for this organization")
					}
				}

				return next(c)
			}

			// Resolve org from UUID header only — no slug fallback
			var org *db.Organization
			if orgUUID := c.Request().Header.Get("X-Organization-UUID"); orgUUID != "" {
				var err error
				org, err = cfg.DB.GetOrganizationByUUID(ctx, orgUUID)
				if err != nil {
					return echo.NewHTTPError(http.StatusNotFound, "organization not found")
				}
			}

			var resolvedOrgID int
			if org != nil {
				member, err := cfg.DB.GetOrgMember(ctx, org.ID, user.ID)
				if err != nil {
					return echo.NewHTTPError(http.StatusForbidden, "not a member of this organization")
				}
				c.Set("org_id", org.ID)
				c.Set("user_role", member.Role)
				resolvedOrgID = org.ID
			} else {
				// No org specified — auto-select if user belongs to exactly one org
				orgs, err := cfg.DB.ListUserOrgs(ctx, user.ID)
				if err != nil || len(orgs) == 0 {
					return echo.NewHTTPError(http.StatusBadRequest, "no organization found for user")
				}
				if len(orgs) > 1 {
					return echo.NewHTTPError(http.StatusBadRequest, "multiple organizations — set X-Organization-UUID header")
				}
				member, err := cfg.DB.GetOrgMember(ctx, orgs[0].ID, user.ID)
				if err != nil {
					return echo.NewHTTPError(http.StatusForbidden, "not a member of this organization")
				}
				c.Set("org_id", orgs[0].ID)
				c.Set("user_role", member.Role)
				resolvedOrgID = orgs[0].ID
			}

			// AI kill switch for CF-authenticated agent users (all org resolution paths)
			if resolvedOrgID > 0 && user.IsAgent {
				aiEnabled, _ := cfg.DB.GetOrgSetting(ctx, resolvedOrgID, "ai_enabled")
				if aiEnabled == "false" {
					return echo.NewHTTPError(http.StatusForbidden, "AI features are disabled for this organization")
				}
			}

			return next(c)
		}
	}
}

// RoleMiddleware restricts write operations to manager/admin roles
// and enforces API key permissions (read, write, read-write).
// Requires AuthMiddleware to have already set user_role on the context.
func (s *Server) RoleMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			method := c.Request().Method

			// Enforce API key permissions if set (Bearer token auth path)
			if perms, ok := c.Get("api_key_permissions").(string); ok && perms != "" {
				isRead := method == "GET" || method == "HEAD" || method == "OPTIONS"
				if perms == "read" && !isRead {
					return echo.NewHTTPError(http.StatusForbidden, "read-only API key")
				}
				if perms == "write" && isRead {
					return echo.NewHTTPError(http.StatusForbidden, "write-only API key")
				}
			}

			// Only check role on write operations
			if method == "GET" || method == "HEAD" || method == "OPTIONS" {
				return next(c)
			}

			// Skip role check for git (handled by git handlers) and auth endpoints
			path := c.Request().URL.Path
			if strings.HasPrefix(path, "/git/") || strings.HasPrefix(path, "/api/v1/auth/") || path == "/api/v1/organizations" {
				return next(c)
			}

			// Role must have been set by AuthMiddleware (token or CF path)
			role, _ := c.Get("user_role").(string)
			if role == "reader" {
				// Allow readers to perform review actions (assignment-based auth in handlers)
				if strings.Contains(path, "/reviews/") && (strings.HasSuffix(path, "/approve") ||
					strings.HasSuffix(path, "/comment") || strings.HasSuffix(path, "/content")) {
					return next(c)
				}
				// Allow readers to add comments (assignment check in handler)
				if strings.HasPrefix(path, "/api/v1/comments") {
					return next(c)
				}
				return echo.NewHTTPError(http.StatusForbidden, "read-only access")
			}

			return next(c)
		}
	}
}

// --- Cloudflare JWT verification ---

type cfClaims struct {
	Email string          `json:"email"`
	Exp   int64           `json:"exp"`
	Iat   int64           `json:"iat"`
	Iss   string          `json:"iss"`
	Aud   json.RawMessage `json:"aud"` // CF Access sends aud as either a string or array of strings
}

type cfKeyCache struct {
	teamDomain       string
	expectedAudience string // CF Access Application Audience (AUD) tag
	keys             map[string]*ecdsa.PublicKey
	mu               sync.RWMutex
	lastFetch        time.Time
}

func newCFKeyCache(teamDomain, audience string) *cfKeyCache {
	return &cfKeyCache{
		teamDomain:       teamDomain,
		expectedAudience: audience,
		keys:             make(map[string]*ecdsa.PublicKey),
	}
}

type cfJWKS struct {
	Keys []cfJWK `json:"keys"`
}

type cfJWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (c *cfKeyCache) fetchKeys() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Since(c.lastFetch) < 5*time.Minute {
		return nil
	}

	url := fmt.Sprintf("https://%s/cdn-cgi/access/certs", c.teamDomain)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var jwks cfJWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return err
	}

	keys := make(map[string]*ecdsa.PublicKey)
	for _, k := range jwks.Keys {
		if k.Kty != "EC" {
			continue
		}
		xBytes, _ := base64.RawURLEncoding.DecodeString(k.X)
		yBytes, _ := base64.RawURLEncoding.DecodeString(k.Y)
		pub := &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(xBytes),
			Y:     new(big.Int).SetBytes(yBytes),
		}
		keys[k.Kid] = pub
	}

	c.keys = keys
	c.lastFetch = time.Now()
	return nil
}

func (c *cfKeyCache) VerifyJWT(token string) (*cfClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	// Decode header
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid header: %w", err)
	}
	var header struct {
		Kid string `json:"kid"`
		Alg string `json:"alg"`
	}
	json.Unmarshal(headerBytes, &header)

	// Fetch/refresh keys if needed
	if err := c.fetchKeys(); err != nil {
		return nil, fmt.Errorf("fetching keys: %w", err)
	}

	c.mu.RLock()
	key, ok := c.keys[header.Kid]
	c.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown key ID: %s", header.Kid)
	}

	// Verify signature
	signingInput := parts[0] + "." + parts[1]
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}

	hash := crypto.SHA256.New()
	hash.Write([]byte(signingInput))
	hashed := hash.Sum(nil)

	// ES256 signature is r || s, each 32 bytes
	if len(sigBytes) != 64 {
		return nil, fmt.Errorf("invalid signature length")
	}
	r := new(big.Int).SetBytes(sigBytes[:32])
	sVal := new(big.Int).SetBytes(sigBytes[32:])

	if !ecdsa.Verify(key, hashed, r, sVal) {
		return nil, fmt.Errorf("signature verification failed")
	}

	// Decode claims
	claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid claims: %w", err)
	}
	var claims cfClaims
	json.Unmarshal(claimBytes, &claims)

	// Check expiry
	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}

	// Validate issuer — must match the configured team domain.
	// CF Access sets iss to "https://<team-domain>".
	expectedIss := "https://" + c.teamDomain
	if claims.Iss != expectedIss {
		return nil, fmt.Errorf("issuer mismatch: got %q, want %q", claims.Iss, expectedIss)
	}

	// Validate audience — must match the CF Access Application Audience (AUD) tag.
	// Without this, any CF Access JWT from ANY application on the same team domain is accepted.
	if c.expectedAudience != "" {
		if !cfAudContains(claims.Aud, c.expectedAudience) {
			return nil, fmt.Errorf("audience mismatch: token aud does not contain %q", c.expectedAudience)
		}
	}

	return &claims, nil
}

// cfAudContains checks whether the JWT "aud" claim (string or []string) contains the expected value.
func cfAudContains(raw json.RawMessage, expected string) bool {
	if len(raw) == 0 {
		return false
	}
	// Try as a single string first
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single == expected
	}
	// Try as an array of strings
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, v := range arr {
			if v == expected {
				return true
			}
		}
	}
	return false
}

// requireRole checks that the current user has one of the specified roles.
// Returns nil if the role matches, or a 403 error if not.
func requireRole(c echo.Context, roles ...string) error {
	role, _ := c.Get("user_role").(string)
	for _, r := range roles {
		if role == r {
			return nil
		}
	}
	return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions")
}

// RLSMiddleware wraps each org-scoped request in a transaction with SET LOCAL for RLS.
// This ensures pooled connections never leak org context between requests.
// The transaction is committed on success, rolled back on error.
func (s *Server) RLSMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			orgID := getOrgID(c)
			if orgID <= 0 {
				return next(c)
			}

			var handlerErr error
			txErr := s.db.WithOrgTx(c.Request().Context(), orgID, func(ctx context.Context, tx pgx.Tx) error {
				// Store tx in context so handlers CAN use it for RLS-scoped queries.
				// Handlers that use d.pool directly still work (app-layer WHERE clause),
				// but won't benefit from RLS. Migrate handlers to use tx over time.
				c.Set("org_tx", tx)
				handlerErr = next(c)
				if handlerErr != nil {
					return handlerErr
				}
				return nil
			})
			if txErr != nil && handlerErr == nil {
				return txErr
			}
			return handlerErr
		}
	}
}

// validateOrgMember checks that an email belongs to a member of the current org.
// Returns nil for empty emails (optional fields). Use for owner_id, assignee_id, etc.
func (s *Server) validateOrgMember(c echo.Context, email string) error {
	if email == "" || email == "system" {
		return nil
	}
	orgID := getOrgID(c)
	if _, err := s.db.ValidateOrgUser(c.Request().Context(), orgID, email); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return nil
}

// validateEnum checks that value is in the allowed slice, returning a 400 if not.
// Empty values are accepted as "unset" — caller decides if that's allowed.
func validateEnum(field, value string, allowed []string) error {
	if value == "" {
		return nil
	}
	for _, v := range allowed {
		if v == value {
			return nil
		}
	}
	return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid %s: %q (allowed: %v)", field, value, allowed))
}

// pgxHTTPError maps a pgx error into an Echo HTTPError.  CHECK violations and
// FK violations are turned into 400s with a friendly message; everything else
// becomes a 500 with the underlying message.  Returns nil if err is nil.
//
// Use this at the top of any write handler that does a single DB call:
//
//	if err := s.db.UpdateThing(...); err != nil {
//	    return pgxHTTPError(err)
//	}
//
// For tx-wrapped writes, pass the txErr through the same helper.
func pgxHTTPError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23514": // CHECK violation
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid value: %s", pgErr.Message))
		case "23503": // foreign key violation
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("referenced entity not found: %s", pgErr.Message))
		case "23505": // unique violation
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("duplicate: %s", pgErr.Message))
		case "23502": // not-null violation
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("required field missing: %s", pgErr.Message))
		}
	}
	return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
}

// sha256Hash returns the hex-encoded SHA-256 hash of a string.
func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
