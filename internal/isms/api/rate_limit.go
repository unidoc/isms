package api

import (
	"net/http"
	"os"
	"strconv"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// ISMS_RATE_LIMIT (documented in the operator guide and README) tunes auth rate
// limiting at runtime:
//   - unset / invalid → defaults apply (per-IP 20, per-email maxLoginAttempts)
//   - 0               → rate limiting OFF (dev / test escape hatch)
//   - N > 0           → per-IP cap becomes N
//
// rateLimitDisabled reports the "0 = off" case; it gates both the per-IP
// middleware and the per-email brute-force checks. Off by default — production is
// always limited unless explicitly turned off.
func rateLimitDisabled() bool {
	return os.Getenv("ISMS_RATE_LIMIT") == "0"
}

// authRateLimitPerIP returns the effective per-IP cap: the ISMS_RATE_LIMIT
// override when it is a positive integer, otherwise the built-in default.
func authRateLimitPerIP() int {
	if v := os.Getenv("ISMS_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return maxAuthAttemptsPerIP
}

// maxAuthAttemptsPerIP is the default per-IP cap on failed auth attempts in a 15-minute
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
			if rateLimitDisabled() {
				return next(c)
			}
			ip := c.RealIP()
			if ip == "" {
				return next(c)
			}
			count, err := database.CountRecentLoginAttemptsByIP(c.Request().Context(), ip)
			if err == nil && count >= authRateLimitPerIP() {
				return echo.NewHTTPError(http.StatusTooManyRequests, "too many attempts from this address, try again later")
			}
			return next(c)
		}
	}
}
