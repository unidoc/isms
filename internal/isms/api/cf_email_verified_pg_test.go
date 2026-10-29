package api

// Cloudflare Access sign-in must leave users.email_verified = true, like
// password, OIDC and passkey sign-in (#493). These drive both real callers of
// resolveCFUser (handleCFSession and AuthMiddleware) with validly signed CF
// tokens. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run 'CFEmailVerified|CFSessionMarks'

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// cfUnverifiedMember creates an UNVERIFIED user (UpsertUser leaves
// email_verified false) that is the sole member of a fresh org.
func cfUnverifiedMember(t *testing.T, s *Server, tag string) (*db.User, int) {
	t.Helper()
	orgID := newTestOrg(t, s, "cfev-"+tag)
	email := fmt.Sprintf("cfev-%s-%d@example.com", tag, time.Now().UnixNano())
	u := contractTestUser(t, s, orgID, email, "contributor")
	if got := cfUserVerified(t, s, email); got {
		t.Fatalf("precondition: %s must start unverified", email)
	}
	return u, orgID
}

func cfUserVerified(t *testing.T, s *Server, email string) bool {
	t.Helper()
	u, err := s.db.GetUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("GetUserByEmail(%s): %v", email, err)
	}
	return u.EmailVerified
}

// cfServer returns a Server wired with a CF key cache and a signer for tokens
// carrying the given email. signClaims signs arbitrary claims with the same key.
func cfServer(t *testing.T) (s *Server, sign func(email string) string, signClaims func(map[string]any) string) {
	t.Helper()
	s = testServer(t)
	s.secret = "cf-email-verified-test-secret-0123456789"
	cache, rsaPriv, _ := testKeys(t)
	s.cfKeyCache = cache
	signClaims = func(claims map[string]any) string { return signRS256(t, rsaPriv, rsaKid, claims) }
	sign = func(email string) string {
		claims := baseClaims()
		claims["email"] = email
		return signClaims(claims)
	}
	return s, sign, signClaims
}

func cfSessionRequest(s *Server, jwt, hdrEmail string) (int, error) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cf-session", nil)
	if jwt != "" {
		req.Header.Set("Cf-Access-Jwt-Assertion", jwt)
	}
	if hdrEmail != "" {
		req.Header.Set("Cf-Access-Authenticated-User-Email", hdrEmail)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	err := s.handleCFSession(c)
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, err
		}
		return 0, err
	}
	return rec.Code, nil
}

// cfMiddlewareRequest runs AuthMiddleware around a trivial handler. The org is
// pre-resolved the way OrgResolverMiddleware would. Returns the status and
// whether the next handler ran.
func cfMiddlewareRequest(s *Server, orgID int, jwt, hdrEmail string, pc cfProvisionConfig) (int, bool) {
	e := echo.New()
	ran := false
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if orgID > 0 {
				c.Set("org_id", orgID)
			}
			return next(c)
		}
	})
	e.Use(AuthMiddleware(AuthConfig{DB: s.db, KeyCache: s.cfKeyCache, Secret: s.secret, Provision: pc}))
	e.GET("/api/v1/cfev", func(c echo.Context) error {
		ran = true
		return c.NoContent(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cfev", nil)
	if jwt != "" {
		req.Header.Set("Cf-Access-Jwt-Assertion", jwt)
	}
	if hdrEmail != "" {
		req.Header.Set("Cf-Access-Authenticated-User-Email", hdrEmail)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code, ran
}

func TestCFSessionMarksExistingUserEmailVerified(t *testing.T) {
	s, sign, _ := cfServer(t)
	u, _ := cfUnverifiedMember(t, s, "sess")
	code, err := cfSessionRequest(s, sign(u.Email), u.Email)
	if err != nil || code != http.StatusOK {
		t.Fatalf("cf-session = %d, %v; want 200", code, err)
	}
	if !cfUserVerified(t, s, u.Email) {
		t.Errorf("email_verified = false after Cloudflare Access sign-in, want true")
	}
}

func TestCFEmailVerifiedMiddlewareExistingUser(t *testing.T) {
	s, sign, _ := cfServer(t)
	u, orgID := cfUnverifiedMember(t, s, "mw")
	code, ran := cfMiddlewareRequest(s, orgID, sign(u.Email), u.Email, cfProvisionConfig{})
	if code != http.StatusNoContent || !ran {
		t.Fatalf("middleware = %d, next ran = %v; want 204 and true", code, ran)
	}
	if !cfUserVerified(t, s, u.Email) {
		t.Errorf("email_verified = false after Cloudflare Access request, want true")
	}
}

func TestCFEmailVerifiedJITProvisionedUser(t *testing.T) {
	s, sign, _ := cfServer(t)
	s.cfProvision = cfProvisionConfig{Enabled: true}
	orgID := newTestOrg(t, s, "cfev-jit")

	// Middleware path: JIT creates the user; the request then needs an org the
	// user is not in, so only the user row's flag is asserted here.
	email := fmt.Sprintf("cfev-jit-mw-%d@example.com", time.Now().UnixNano())
	cfMiddlewareRequest(s, orgID, sign(email), email, s.cfProvision)
	if !cfUserVerified(t, s, email) {
		t.Errorf("middleware JIT user: email_verified = false, want true")
	}

	// cf-session path: no org yet gives 403, but the user is created first.
	email2 := fmt.Sprintf("cfev-jit-sess-%d@example.com", time.Now().UnixNano())
	_, _ = cfSessionRequest(s, sign(email2), email2)
	if !cfUserVerified(t, s, email2) {
		t.Errorf("cf-session JIT user: email_verified = false, want true")
	}
}

func TestCFEmailVerifiedAlreadyVerifiedUnchanged(t *testing.T) {
	s, sign, _ := cfServer(t)
	u, orgID := cfUnverifiedMember(t, s, "already")
	if err := s.db.SetEmailVerified(context.Background(), u.ID); err != nil {
		t.Fatal(err)
	}
	code, err := cfSessionRequest(s, sign(u.Email), u.Email)
	if err != nil || code != http.StatusOK {
		t.Fatalf("cf-session = %d, %v; want 200", code, err)
	}
	code, ran := cfMiddlewareRequest(s, orgID, sign(u.Email), u.Email, cfProvisionConfig{})
	if code != http.StatusNoContent || !ran {
		t.Fatalf("middleware = %d, next ran = %v; want 204 and true", code, ran)
	}
	if !cfUserVerified(t, s, u.Email) {
		t.Errorf("already verified user became unverified")
	}
}

// Controls: requests rejected before resolveCFUser must leave the flag alone.
func TestCFEmailVerifiedRejectedRequestsLeaveFlagFalse(t *testing.T) {
	s, sign, signClaims := cfServer(t)
	u, orgID := cfUnverifiedMember(t, s, "ctl")
	other, _ := cfUnverifiedMember(t, s, "ctlother")

	wrongAud := baseClaims()
	wrongAud["email"] = u.Email
	wrongAud["aud"] = "some-other-app"
	// Right kid, but signed by a key the JWKS does not carry.
	_, strangerRSA, _ := testKeys(t)
	badSig := baseClaims()
	badSig["email"] = u.Email

	cases := []struct {
		name, jwt, hdr string
	}{
		{"invalid signature", signRS256(t, strangerRSA, rsaKid, badSig), u.Email},
		{"wrong audience", signClaims(wrongAud), u.Email},
		{"header email differs from token email", sign(other.Email), u.Email},
		{"no token", "", u.Email},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code, _ := cfSessionRequest(s, tc.jwt, tc.hdr); code != http.StatusUnauthorized {
				t.Errorf("cf-session code = %d, want 401", code)
			}
			if code, ran := cfMiddlewareRequest(s, orgID, tc.jwt, tc.hdr, cfProvisionConfig{}); code != http.StatusUnauthorized || ran {
				t.Errorf("middleware = %d, next ran = %v; want 401 and false", code, ran)
			}
			if cfUserVerified(t, s, u.Email) || cfUserVerified(t, s, other.Email) {
				t.Errorf("email_verified became true on a rejected request")
			}
		})
	}
}
