package api

import (
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

// Per-IP request limit for the trust center public API (#517). It is the only
// in-process request limiter: AuthRateLimitMiddleware counts failed logins, so
// it could never limit a read API, and the rest of the server leaves request
// limiting to the edge. This prefix answers anonymous visitors and looks up an
// organization by slug before anything else, so it is capped here as well,
// which also bounds slug enumeration.
//
// ISMS_TRUST_RATE_LIMIT sets requests per minute per IP (positive integer;
// unset or invalid means the default). ISMS_RATE_LIMIT=0 turns it off, the same
// dev/test switch as the auth limits. Both are read once, when the middleware
// is built.
const (
	trustRateLimitDefault = 120 // requests per minute per IP
	trustRateLimitBurst   = 30  // enough for a page load, capped at the rate
	trustRateLimitExpiry  = 3 * time.Minute
)

// trustRateLimitPerMinute returns the effective requests-per-minute cap.
func trustRateLimitPerMinute() int {
	if v := os.Getenv("ISMS_TRUST_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return trustRateLimitDefault
}

// trustRateLimiter builds the limiter. Build it once and mount the same
// instance on every trust route so they share one per-IP store.
func trustRateLimiter() echo.MiddlewareFunc {
	if rateLimitDisabled() {
		return func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	}
	perMinute := trustRateLimitPerMinute()
	burst := trustRateLimitBurst
	if perMinute < burst {
		burst = perMinute
	}
	retryAfter := strconv.Itoa(int(math.Ceil(60 / float64(perMinute))))

	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
			Rate:      rate.Limit(float64(perMinute) / 60),
			Burst:     burst,
			ExpiresIn: trustRateLimitExpiry,
		}),
		// c.RealIP honors the server's IPExtractor (CF-Connecting-IP, X-Real-IP,
		// X-Forwarded-For). A request with no usable IP is still limited, under
		// its connection address, and never refused for that reason alone.
		IdentifierExtractor: func(c echo.Context) (string, error) {
			if ip := c.RealIP(); ip != "" {
				return ip, nil
			}
			host, _, err := net.SplitHostPort(c.Request().RemoteAddr)
			if err != nil {
				host = c.Request().RemoteAddr
			}
			return host, nil
		},
		DenyHandler: func(c echo.Context, _ string, _ error) error {
			h := c.Response().Header()
			h.Set("Retry-After", retryAfter)
			h.Set("Cache-Control", "no-store")
			return c.JSON(http.StatusTooManyRequests, map[string]string{"message": "too many requests"})
		},
		// Never fail closed on an extractor problem.
		ErrorHandler: func(c echo.Context, _ error) error {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "internal server error"})
		},
	})
}
