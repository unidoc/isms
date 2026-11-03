package api

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// SessionClaims are the JWT claims for a web login session.
type SessionClaims struct {
	jwt.RegisteredClaims
	Email          string `json:"email"`
	Name           string `json:"name"`
	UserID         int    `json:"user_id"`
	OrganizationID int    `json:"org_id,omitempty"`
	Role           string `json:"role,omitempty"`
	OrgSlug        string `json:"org_slug,omitempty"`
	OrgName        string `json:"org_name,omitempty"`
	// SSOOrgID is set on sessions minted by an organization's own single
	// sign-on provider. Such a session is valid only in that organization and
	// cannot change the account's sign-in credentials (see refuseSSOSession).
	SSOOrgID int `json:"sso_org,omitempty"`
}

// jwtLifetime returns the configured JWT session lifetime.
// Default 24h, override with ISMS_JWT_LIFETIME (e.g. "720h" for 30 days in dev).
func jwtLifetime() time.Duration {
	if v := os.Getenv("ISMS_JWT_LIFETIME"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 24 * time.Hour
}

// createSessionJWT creates a signed JWT session token for the given user.
func (s *Server) createSessionJWT(user *db.User, orgID int, role, orgSlug, orgName string) (string, error) {
	return s.createContainedSessionJWT(user, orgID, role, orgSlug, orgName, 0)
}

// createContainedSessionJWT is createSessionJWT with the sso_org claim set
// when ssoOrgID > 0.
func (s *Server) createContainedSessionJWT(user *db.User, orgID int, role, orgSlug, orgName string, ssoOrgID int) (string, error) {
	now := time.Now()
	claims := SessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "isms",
			Audience:  jwt.ClaimStrings{"isms"},
			Subject:   user.Email,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(jwtLifetime())),
		},
		Email:          user.Email,
		Name:           user.Name,
		UserID:         user.ID,
		OrganizationID: orgID,
		Role:           role,
		OrgSlug:        orgSlug,
		OrgName:        orgName,
		SSOOrgID:       ssoOrgID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.secret))
}

// validateSessionJWT parses and validates a JWT session token.
func validateSessionJWT(tokenString, secret string) (*SessionClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &SessionClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Pin to HS256 only — accepting any HMAC variant (HS384/HS512) widens
		// the attack surface unnecessarily since we only ever issue HS256.
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithIssuer("isms"), jwt.WithAudience("isms"), jwt.WithLeeway(60*time.Second))
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*SessionClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// ssoContainedMsg is the 403 text for everything a contained session may not do.
const ssoContainedMsg = "signed in with single sign-on: sign in with your password or passkey to change sign-in settings or switch organization"

// getSSOOrg returns the organization a contained (single sign-on) session is
// limited to, or 0 for an ordinary session.
func getSSOOrg(c echo.Context) int {
	if id, ok := c.Get("sso_org").(int); ok {
		return id
	}
	return 0
}

// refuseSSOSession returns a 403 for a contained session and nil otherwise.
// Handlers that change how the account signs in call it first, so the rule
// lives in one place: an organization's identity provider vouches for who is
// signing in to that organization, not for the account everywhere.
func refuseSSOSession(c echo.Context) error {
	if getSSOOrg(c) > 0 {
		return echo.NewHTTPError(http.StatusForbidden, ssoContainedMsg)
	}
	return nil
}

// apiKeyDeniedMsg is the 403 text for what an API key may not do.
const apiKeyDeniedMsg = "API keys cannot change sign-in settings or create sessions or keys; sign in to do this"

// refuseAPIKey returns a 403 when the request is authenticated by an API key.
// A key is a data credential: if it could be refreshed into a session, create
// further keys, or change the password, passkeys or OTP, it would outlive and
// outrank whatever limits its session had (see refuseSSOSession).
func refuseAPIKey(c echo.Context) error {
	if c.Get("api_key_id") != nil {
		return echo.NewHTTPError(http.StatusForbidden, apiKeyDeniedMsg)
	}
	return nil
}
