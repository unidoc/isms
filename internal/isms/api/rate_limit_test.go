package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestRateLimitDisabledEnv guards a bug that shipped silently: ISMS_RATE_LIMIT
// was documented (README, operator guide) and set in the dev/test compose, but
// no code read it — so the limiter ran regardless and a full test run tripped it.
// A documented switch that does nothing is worse than no switch.
func TestRateLimitDisabledEnv(t *testing.T) {
	for _, tc := range []struct {
		val  string
		want bool
	}{
		{"0", true},   // the documented "off" value
		{"", false},   // unset → limiter on (production default)
		{"1", false},  // any non-zero → on
		{"20", false}, // a numeric override is still "on"
	} {
		t.Setenv("ISMS_RATE_LIMIT", tc.val)
		if got := rateLimitDisabled(); got != tc.want {
			t.Errorf("ISMS_RATE_LIMIT=%q: rateLimitDisabled()=%v, want %v", tc.val, got, tc.want)
		}
	}
}

// TestAuthRateLimitPerIP checks the numeric override: a positive ISMS_RATE_LIMIT
// becomes the per-IP cap; 0, empty, or garbage fall back to the default.
func TestAuthRateLimitPerIP(t *testing.T) {
	for _, tc := range []struct {
		val  string
		want int
	}{
		{"", maxAuthAttemptsPerIP},         // unset → default
		{"0", maxAuthAttemptsPerIP},        // 0 = "disabled"; cap value itself falls back
		{"5", 5},                           // positive override is used
		{"100", 100},                       // ditto
		{"nonsense", maxAuthAttemptsPerIP}, // garbage → default
	} {
		t.Setenv("ISMS_RATE_LIMIT", tc.val)
		if got := authRateLimitPerIP(); got != tc.want {
			t.Errorf("ISMS_RATE_LIMIT=%q: authRateLimitPerIP()=%d, want %d", tc.val, got, tc.want)
		}
	}
}

// TestAuthRateLimitMiddlewareBypass proves the middleware short-circuits BEFORE
// touching the database when disabled. It is constructed with a nil *db.DB: if
// the bypass ever regresses, the handler reaches CountRecentLoginAttemptsByIP on
// a nil DB and panics — so this test is the canary for the switch actually
// being honored end to end.
func TestAuthRateLimitMiddlewareBypass(t *testing.T) {
	t.Setenv("ISMS_RATE_LIMIT", "0")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "203.0.113.7:12345" // non-empty RealIP, so a live limiter would hit the DB
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	next := func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	}

	// nil DB on purpose: only the bypass keeps this from dereferencing it.
	mw := AuthRateLimitMiddleware(nil)
	if err := mw(next)(c); err != nil {
		t.Fatalf("middleware returned error with limiter disabled: %v", err)
	}
	if !called {
		t.Fatal("next handler was not called — limiter did not bypass with ISMS_RATE_LIMIT=0")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
