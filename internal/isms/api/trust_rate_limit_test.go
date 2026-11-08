package api

// The trust center limiter is exercised without a database, through the real
// routes. IPs are told apart by the X-Real-Ip header, which the server's
// IPExtractor reads in production.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// trustLimitServer returns routes() on a bare Server. The trust handler's
// 404 for a non-GET method comes before any database access, so probing with
// POST needs no database.
func trustLimitServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{echo: echo.New(), searchIndex: NewSearchIndex()}
	s.echo.Use(middleware.Recover()) // member handlers have no database here
	s.echo.IPExtractor = func(r *http.Request) string { return r.Header.Get("X-Real-Ip") }
	s.routes()
	return s
}

func trustLimitHit(s *Server, ip, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-Real-Ip", ip)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	return rec
}

// POST is answered by the trust handler with its 404 before any database
// access, so it is a safe probe.
const trustProbe = "/api/v1/trust/acme/items"

func TestTrustRateLimitBlocksAfterBudget(t *testing.T) {
	t.Setenv("ISMS_RATE_LIMIT", "")
	t.Setenv("ISMS_TRUST_RATE_LIMIT", "5")
	s := trustLimitServer(t)

	for i := 0; i < 5; i++ {
		if rec := trustLimitHit(s, "198.51.100.1", http.MethodPost, trustProbe); rec.Code != http.StatusNotFound {
			t.Fatalf("request %d: status %d, want 404 (within budget)", i+1, rec.Code)
		}
	}
	rec := trustLimitHit(s, "198.51.100.1", http.MethodPost, trustProbe)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("over budget: status %d, want 429; body %s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"message":"too many requests"}` {
		t.Errorf("429 body = %s", got)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 lacks Retry-After")
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("429 Cache-Control = %q", cc)
	}
	// The bare prefix shares the same store as the wildcard route.
	if rec := trustLimitHit(s, "198.51.100.1", http.MethodPost, "/api/v1/trust"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("bare prefix status %d, want 429 (shared store)", rec.Code)
	}

	// Another IP has its own budget.
	if rec := trustLimitHit(s, "198.51.100.2", http.MethodPost, trustProbe); rec.Code != http.StatusNotFound {
		t.Errorf("other IP status %d, want 404", rec.Code)
	}
}

func TestTrustRateLimitOffWithAuthSwitch(t *testing.T) {
	t.Setenv("ISMS_RATE_LIMIT", "0")
	t.Setenv("ISMS_TRUST_RATE_LIMIT", "2")
	s := trustLimitServer(t)
	for i := 0; i < 50; i++ {
		if rec := trustLimitHit(s, "198.51.100.1", http.MethodPost, trustProbe); rec.Code != http.StatusNotFound {
			t.Fatalf("request %d: status %d, want 404 with ISMS_RATE_LIMIT=0", i+1, rec.Code)
		}
	}
}

func TestTrustRateLimitDoesNotTouchMemberRoutes(t *testing.T) {
	t.Setenv("ISMS_RATE_LIMIT", "")
	t.Setenv("ISMS_TRUST_RATE_LIMIT", "1")
	s := trustLimitServer(t)
	trustLimitHit(s, "198.51.100.1", http.MethodPost, trustProbe)
	trustLimitHit(s, "198.51.100.1", http.MethodPost, trustProbe)
	if rec := trustLimitHit(s, "198.51.100.1", http.MethodPost, trustProbe); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("precondition: trust prefix status %d, want 429", rec.Code)
	}
	// The handler fails on the missing database (a recovered 500); only a 429
	// would mean this limiter caught it.
	for i := 0; i < 5; i++ {
		if rec := trustLimitHit(s, "198.51.100.1", http.MethodGet, "/api/v1/config"); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("member route request %d was rate limited", i+1)
		}
	}
}

func TestTrustRateLimitEnvParsing(t *testing.T) {
	for val, want := range map[string]int{"": 120, "abc": 120, "0": 120, "-3": 120, "7": 7} {
		t.Setenv("ISMS_TRUST_RATE_LIMIT", val)
		if got := trustRateLimitPerMinute(); got != want {
			t.Errorf("ISMS_TRUST_RATE_LIMIT=%q: %d, want %d", val, got, want)
		}
	}
}
