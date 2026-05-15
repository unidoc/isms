package api

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
