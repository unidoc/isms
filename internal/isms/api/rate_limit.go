package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// maxAuthAttemptsPerIP is the per-IP cap on failed auth attempts in a 15-minute
// window. Complements maxLoginAttempts (per-email) — together they catch both
// "one attacker hammering one account" and "one attacker spraying many accounts
// from one source". Higher than maxLoginAttempts because a shared IP (e.g. an
// office NAT) may legitimately produce more failures than a single user.
const maxAuthAttemptsPerIP = 20

// AuthRateLimitMiddleware returns a middleware that rejects further auth
// attempts from an IP once it has crossed the per-IP threshold. Mount this on
// the unauthenticated auth endpoints (login, signup, forgot-password,
// verify-email, passkey login, OIDC initiate) — NOT on refresh, logout, or any
// authenticated path, which are legitimate session traffic.
//
// IP is read via c.RealIP(), which honors the echo IPExtractor configured at
// server startup (CF-Connecting-IP → X-Real-IP → X-Forwarded-For).
func AuthRateLimitMiddleware(database *db.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()
			if ip == "" {
				return next(c)
			}
			count, err := database.CountRecentLoginAttemptsByIP(c.Request().Context(), ip)
			if err == nil && count >= maxAuthAttemptsPerIP {
				return echo.NewHTTPError(http.StatusTooManyRequests, "too many attempts from this address, try again later")
			}
			return next(c)
		}
	}
}
