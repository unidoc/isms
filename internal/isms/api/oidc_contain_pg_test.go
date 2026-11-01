package api

// A session minted by an organization's own single sign-on provider is valid
// only in that organization and cannot change how the account signs in. These
// send real requests through AuthMiddleware and the real handlers, with a token
// from the fake-IdP callback flow (oidc_link_pg_test.go). The control is an
// ordinary session minted the way a password login mints it. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run OIDCContain

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

type containFixture struct {
	s          *Server
	e          *echo.Echo
	orgA, orgB oidcOrg
	user       *db.User
	sso        string // session from org A's provider
	plain      string // ordinary session in org A
}

func newContainFixture(t *testing.T) *containFixture {
	t.Helper()
	s := testServer(t)
	s.secret = "oidc-link-test-secret-0123456789abcdef"
	idp := newFakeIDP(t)
	f := &containFixture{s: s, orgA: newOIDCOrg(t, s, idp, "acme")}
	f.orgB = oidcOrg{id: newTestOrg(t, s, "oidccontainB"+oidcLinkUniq())}

	email := oidcLinkEmail("contain")
	r := runSSO(t, s, f.orgA, idp, idpClaims("sub-"+oidcLinkUniq(), email))
	requireSession(t, r, email)
	f.sso = r.token()
	f.user = &db.User{ID: mustUser(t, s, email), Email: email, Name: email}
	// Admin in org B so org creation and switch-org are both possible.
	if err := s.db.AddOrgMember(context.Background(), f.orgB.id, f.user.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	orgB, err := s.db.GetOrganization(context.Background(), f.orgB.id)
	if err != nil {
		t.Fatal(err)
	}
	f.orgB.slug = orgB.Slug

	plain, err := s.createSessionJWT(f.user, f.orgA.id, "reader", f.orgA.slug, "A")
	if err != nil {
		t.Fatal(err)
	}
	f.plain = plain

	e := echo.New()
	e.Use(AuthMiddleware(AuthConfig{DB: s.db, Secret: s.secret}))
	g := e.Group("/api/v1")
	g.POST("/auth/refresh", s.handleRefresh)
	g.POST("/auth/switch-org", s.handleSwitchOrg)
	g.PUT("/auth/password", s.handleChangePassword)
	g.PUT("/auth/email", s.handleRequestEmailChange)
	g.POST("/auth/otp/setup", s.handleOTPSetup)
	g.POST("/auth/otp/verify", s.handleOTPVerify)
	g.DELETE("/auth/otp", s.handleOTPDisable)
	g.POST("/auth/api-keys", s.handleCreateMyAPIKey)
	g.POST("/auth/passkey/register/begin", s.handlePasskeyRegisterBegin)
	g.POST("/auth/passkey/register/complete", s.handlePasskeyRegisterComplete)
	g.PUT("/auth/passkeys/:id", s.handleRenamePasskey)
	g.DELETE("/auth/passkeys/:id", s.handleDeletePasskey)
	g.POST("/organizations", s.handleCreateOrganization)
	f.e = e
	return f
}

func (f *containFixture) do(method, path, token, body string) (int, string) {
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func claimsOf(t *testing.T, token string) SessionClaims {
	t.Helper()
	var c SessionClaims
	if _, _, err := jwt.NewParser().ParseUnverified(token, &c); err != nil {
		t.Fatalf("parsing token: %v", err)
	}
	return c
}

func tokenField(t *testing.T, body string) string {
	t.Helper()
	var r struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(body), &r); err != nil || r.Token == "" {
		t.Fatalf("no token in response %q (%v)", body, err)
	}
	return r.Token
}

func TestOIDCContain_SwitchOrg(t *testing.T) {
	f := newContainFixture(t)
	if c := claimsOf(t, f.sso); c.SSOOrgID != f.orgA.id {
		t.Fatalf("SSO session sso_org = %d, want %d", c.SSOOrgID, f.orgA.id)
	}
	if c := claimsOf(t, f.plain); c.SSOOrgID != 0 {
		t.Fatalf("control session carries sso_org %d", c.SSOOrgID)
	}
	other := `{"slug":"` + f.orgB.slug + `"}`
	if code, body := f.do("POST", "/auth/switch-org", f.sso, other); code != 403 {
		t.Errorf("SSO switch-org to another org = %d, want 403: %s", code, body)
	}
	if code, body := f.do("POST", "/auth/switch-org", f.plain, other); code != 200 {
		t.Errorf("control switch-org = %d, want 200: %s", code, body)
	}
	own := `{"slug":"` + f.orgA.slug + `"}`
	code, body := f.do("POST", "/auth/switch-org", f.sso, own)
	if code != 200 {
		t.Fatalf("SSO switch-org to its own org = %d, want 200: %s", code, body)
	}
	if c := claimsOf(t, tokenField(t, body)); c.SSOOrgID != f.orgA.id {
		t.Errorf("token from switching to own org has sso_org %d, want %d", c.SSOOrgID, f.orgA.id)
	}
}

func TestOIDCContain_RefreshKeepsContainment(t *testing.T) {
	f := newContainFixture(t)
	// The old token is blocklisted on refresh; wait so the new one (same claims)
	// has a later iat and so a different signature.
	time.Sleep(1100 * time.Millisecond)
	code, body := f.do("POST", "/auth/refresh", f.sso, "")
	if code != 200 {
		t.Fatalf("refresh = %d: %s", code, body)
	}
	refreshed := tokenField(t, body)
	if c := claimsOf(t, refreshed); c.SSOOrgID != f.orgA.id {
		t.Fatalf("refreshed token sso_org = %d, want %d", c.SSOOrgID, f.orgA.id)
	}
	if code, body := f.do("POST", "/auth/switch-org", refreshed, `{"slug":"`+f.orgB.slug+`"}`); code != 403 {
		t.Errorf("refreshed token switch-org = %d, want 403: %s", code, body)
	}
	code, body = f.do("POST", "/auth/refresh", f.plain, "")
	if code != 200 {
		t.Fatalf("control refresh = %d: %s", code, body)
	}
	if c := claimsOf(t, tokenField(t, body)); c.SSOOrgID != 0 {
		t.Errorf("refreshing an ordinary session produced sso_org %d", c.SSOOrgID)
	}
}

func TestOIDCContain_APIKeys(t *testing.T) {
	f := newContainFixture(t)
	if code, body := f.do("POST", "/auth/api-keys", f.sso, `{"name":"k","scope":"global"}`); code != 403 {
		t.Errorf("SSO global key = %d, want 403: %s", code, body)
	}
	if code, body := f.do("POST", "/auth/api-keys", f.plain, `{"name":"k","scope":"global"}`); code != 201 {
		t.Errorf("control global key = %d, want 201: %s", code, body)
	}
	for name, tok := range map[string]string{"SSO": f.sso, "control": f.plain} {
		code, body := f.do("POST", "/auth/api-keys", tok, `{"name":"k"}`)
		if code != 201 {
			t.Errorf("%s org key = %d, want 201: %s", name, code, body)
			continue
		}
		var r struct {
			OrganizationID *int `json:"organization_id"`
		}
		_ = json.Unmarshal([]byte(body), &r)
		if r.OrganizationID == nil || *r.OrganizationID != f.orgA.id {
			t.Errorf("%s org key organization_id = %v, want %d", name, r.OrganizationID, f.orgA.id)
		}
	}
}

func TestOIDCContain_CredentialChanges(t *testing.T) {
	f := newContainFixture(t)
	routes := []struct{ method, path, body string }{
		{"POST", "/auth/passkey/register/begin", ""},
		{"POST", "/auth/passkey/register/complete", ""},
		{"PUT", "/auth/passkeys/2000000000", `{"name":"x"}`},
		{"DELETE", "/auth/passkeys/2000000000", ""},
		{"PUT", "/auth/password", `{"new_password":"correct-horse-battery"}`},
		{"PUT", "/auth/email", `{"new_email":"x@example.com","current_password":"pw"}`},
		{"POST", "/auth/otp/setup", ""},
		{"POST", "/auth/otp/verify", `{"code":"123456"}`},
		{"DELETE", "/auth/otp", `{"code":"123456"}`},
	}
	for _, r := range routes {
		if code, body := f.do(r.method, r.path, f.sso, r.body); code != 403 {
			t.Errorf("SSO %s %s = %d, want 403: %s", r.method, r.path, code, body)
		}
		if code, body := f.do(r.method, r.path, f.plain, r.body); code == 403 {
			t.Errorf("control %s %s = 403, want anything else: %s", r.method, r.path, body)
		}
	}
}

// Creating an organization returns no session at all, so a contained session
// cannot obtain one for another org this way.
func TestOIDCContain_OrgCreationReturnsNoToken(t *testing.T) {
	f := newContainFixture(t)
	t.Setenv("ISMS_DATA_DIR", t.TempDir())
	slug := "oidccont" + oidcLinkUniq()
	code, body := f.do("POST", "/organizations", f.sso, `{"name":"N","slug":"`+slug+`"}`)
	if code != 201 {
		t.Fatalf("create org = %d: %s", code, body)
	}
	if org, err := f.s.db.GetOrganizationBySlug(context.Background(), slug); err == nil {
		t.Cleanup(func() { _ = f.s.db.DeleteOrganization(context.Background(), org.ID) })
	}
	var resp map[string]any
	_ = json.Unmarshal([]byte(body), &resp)
	for k := range resp {
		if strings.Contains(strings.ToLower(k), "token") {
			t.Errorf("response carries %q: %s", k, body)
		}
	}
	// And the new org is out of reach for the contained session.
	if code, body := f.do("POST", "/auth/switch-org", f.sso, `{"slug":"`+slug+`"}`); code != 403 {
		t.Errorf("SSO switch-org to the new org = %d, want 403: %s", code, body)
	}
}
