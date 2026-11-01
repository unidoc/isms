package api

// Linking single sign-on to an existing account from Settings: the signed-in
// session is the proof of ownership. Requests go through AuthMiddleware; the
// IdP side is the fake provider from oidc_link_pg_test.go. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run OIDCLinkFlow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"isms.sh/internal/isms/db"
)

func linkFixture(t *testing.T) *containFixture {
	t.Helper()
	t.Setenv("ISMS_BASE_URL", "http://localhost:9999")
	f := newContainFixture(t)
	f.e.POST("/api/v1/auth/oidc/link", f.s.handleOIDCLinkBegin)
	f.e.GET("/api/v1/auth/oidc/callback", f.s.handleOIDCCallback)
	f.e.GET("/api/v1/auth/identities", f.s.handleListIdentities)
	f.e.DELETE("/api/v1/auth/identities/:id", f.s.handleUnlinkIdentity)
	return f
}

// passwordUser is an ordinary account (password set) in org A, with a session.
func (f *containFixture) passwordUser(t *testing.T) (*db.User, string) {
	t.Helper()
	email := oidcLinkEmail("pwuser")
	u := contractTestUser(t, f.s, f.orgA.id, email, "contributor")
	if err := f.s.db.SetPassword(context.Background(), u.ID, "$2a$10$hash"); err != nil {
		t.Fatal(err)
	}
	tok, err := f.s.createSessionJWT(u, f.orgA.id, "contributor", f.orgA.slug, "A")
	if err != nil {
		t.Fatal(err)
	}
	return u, tok
}

// finishLink plays the IdP side: it asserts claims for the flow in authURL and
// sends the browser's callback request.
func (f *containFixture) finishLink(t *testing.T, authURL string, claims map[string]any) ssoResult {
	t.Helper()
	au, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	f.idp.mu.Lock()
	f.idp.claims = claims
	f.idp.nonce = au.Query().Get("nonce")
	f.idp.mu.Unlock()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=abc&state="+au.Query().Get("state"), nil)
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	return ssoResult{status: rec.Code, loc: rec.Header().Get("Location")}
}

func (f *containFixture) beginLink(t *testing.T, token string) string {
	t.Helper()
	code, body := f.do("POST", "/auth/oidc/link", token, `{"provider":"acme"}`)
	if code != 200 {
		t.Fatalf("begin link = %d: %s", code, body)
	}
	var r struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(body), &r); err != nil || !strings.HasPrefix(r.URL, f.idp.srv.URL) {
		t.Fatalf("url = %q (%v), want the IdP authorization URL", r.URL, err)
	}
	return r.URL
}

func TestOIDCLinkFlow_LinksToSignedInUserThenSignsIn(t *testing.T) {
	f := linkFixture(t)
	u, tok := f.passwordUser(t)
	sub := "sub-" + uuid.NewString()
	orgsBefore := orgIDs(t, f.s, u.ID)

	// The IdP's email differs from the account's: the session is the proof.
	res := f.finishLink(t, f.beginLink(t, tok), idpClaims(sub, oidcLinkEmail("idp-mail")))
	if res.status != http.StatusFound || strings.Contains(res.loc, "token=") ||
		!strings.HasSuffix(res.loc, "/"+f.orgA.slug+"/settings?sso_link=linked") {
		t.Fatalf("callback = %d %q, want 302 to settings?sso_link=linked with no token", res.status, res.loc)
	}
	if got := identityIssuer(t, f.s, u.ID); len(got) != 1 || got[0] != f.idp.srv.URL {
		t.Errorf("issuers = %v, want [%s]", got, f.idp.srv.URL)
	}
	if after := orgIDs(t, f.s, u.ID); len(after) != len(orgsBefore) {
		t.Errorf("memberships changed: %v -> %v", orgsBefore, after)
	}

	// Now an ordinary SSO sign-in through the same provider is that user.
	requireSession(t, runSSO(t, f.s, f.orgA, f.idp, idpClaims(sub, oidcLinkEmail("other"))), u.Email)

	// Linking the same identity again is a no-op success.
	res = f.finishLink(t, f.beginLink(t, tok), idpClaims(sub, u.Email))
	if !strings.HasSuffix(res.loc, "sso_link=linked") {
		t.Errorf("relink = %q, want linked", res.loc)
	}
	if n := identityCount(t, f.s, u.ID); n != 1 {
		t.Errorf("identities = %d, want 1", n)
	}
}

func TestOIDCLinkFlow_IdentityOfAnotherUserIsInUse(t *testing.T) {
	f := linkFixture(t)
	u, tok := f.passwordUser(t)
	// f.user signed in through SSO, so it owns (idp, its sub). Find that sub.
	var sub string
	if err := f.s.db.Pool().QueryRow(context.Background(),
		`SELECT subject FROM user_identities WHERE user_id = $1`, f.user.ID).Scan(&sub); err != nil {
		t.Fatal(err)
	}
	res := f.finishLink(t, f.beginLink(t, tok), idpClaims(sub, u.Email))
	if res.status != http.StatusFound || strings.Contains(res.loc, "token=") || !strings.HasSuffix(res.loc, "sso_link=in_use") {
		t.Fatalf("callback = %d %q, want sso_link=in_use", res.status, res.loc)
	}
	if n := identityCount(t, f.s, u.ID); n != 0 {
		t.Errorf("caller identities = %d, want 0", n)
	}
	if n := identityCount(t, f.s, f.user.ID); n != 1 {
		t.Errorf("owner identities = %d, want 1", n)
	}
}

func TestOIDCLinkFlow_BeginRefusals(t *testing.T) {
	f := linkFixture(t)
	_, tok := f.passwordUser(t)
	key, hash := generateAPIKey()
	orgA := f.orgA.id
	if _, err := f.s.db.CreateAPIKey(context.Background(), "t", hash, f.user.Email, "read-write", &orgA, nil); err != nil {
		t.Fatal(err)
	}
	body := `{"provider":"acme"}`
	if code, b := f.do("POST", "/auth/oidc/link", f.sso, body); code != 403 {
		t.Errorf("contained session = %d, want 403: %s", code, b)
	}
	if code, b := f.do("POST", "/auth/oidc/link", key, body); code != 403 {
		t.Errorf("API key = %d, want 403: %s", code, b)
	}
	// A provider of another org, a missing one, and a disabled one are all 404.
	idp2 := newFakeIDP(t)
	other := newOIDCOrg(t, f.s, idp2, "elsewhere")
	_ = other
	if code, b := f.do("POST", "/auth/oidc/link", tok, `{"provider":"elsewhere"}`); code != 404 {
		t.Errorf("another org's provider = %d, want 404: %s", code, b)
	}
	if code, b := f.do("POST", "/auth/oidc/link", tok, `{"provider":"nope"}`); code != 404 {
		t.Errorf("unknown provider = %d, want 404: %s", code, b)
	}
	off := &db.OIDCProvider{
		OrganizationID: f.orgA.id, ProviderName: "off", DisplayName: "Off", ClientID: oidcLinkClientID,
		ClientSecret: "s", DiscoveryURL: f.idp.srv.URL, DefaultRole: "reader", Enabled: false,
	}
	if err := f.s.db.CreateOIDCProvider(context.Background(), off); err != nil {
		t.Fatal(err)
	}
	if code, b := f.do("POST", "/auth/oidc/link", tok, `{"provider":"off"}`); code != 404 {
		t.Errorf("disabled provider = %d, want 404: %s", code, b)
	}
	if code, b := f.do("POST", "/auth/oidc/link", tok, `{}`); code != 400 {
		t.Errorf("missing provider = %d, want 400: %s", code, b)
	}
}

func TestOIDCLinkFlow_ListAndUnlink(t *testing.T) {
	f := linkFixture(t)
	key, hash := generateAPIKey()
	orgA := f.orgA.id
	if _, err := f.s.db.CreateAPIKey(context.Background(), "t", hash, f.user.Email, "read-write", &orgA, nil); err != nil {
		t.Fatal(err)
	}
	// f.user has one SSO identity and no password or passkey.
	code, body := f.do("GET", "/auth/identities", f.plain, "")
	var list struct {
		Data []identityItem `json:"data"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &list) != nil || len(list.Data) != 1 ||
		list.Data[0].Provider != "acme" || list.Data[0].Issuer != f.idp.srv.URL {
		t.Fatalf("list = %d %s", code, body)
	}
	id := list.Data[0].ID
	path := "/auth/identities/" + strconv.Itoa(id)

	if code, b := f.do("DELETE", path, f.plain, ""); code != 409 {
		t.Errorf("unlinking the last way to sign in = %d, want 409: %s", code, b)
	}
	if code, b := f.do("DELETE", path, f.sso, ""); code != 403 {
		t.Errorf("contained session unlink = %d, want 403: %s", code, b)
	}
	if code, b := f.do("DELETE", path, key, ""); code != 403 {
		t.Errorf("API key unlink = %d, want 403: %s", code, b)
	}

	// Someone else's identity is a 404, not a 403 or a delete.
	_, tok := f.passwordUser(t)
	if code, b := f.do("DELETE", path, tok, ""); code != 404 {
		t.Errorf("other user's identity = %d, want 404: %s", code, b)
	}
	if n := identityCount(t, f.s, f.user.ID); n != 1 {
		t.Fatalf("identity vanished: %d", n)
	}

	// With a password the identity can go.
	if err := f.s.db.SetPassword(context.Background(), f.user.ID, "$2a$10$hash"); err != nil {
		t.Fatal(err)
	}
	if code, b := f.do("DELETE", path, f.plain, ""); code != 200 {
		t.Errorf("unlink with a password = %d, want 200: %s", code, b)
	}
	if n := identityCount(t, f.s, f.user.ID); n != 0 {
		t.Errorf("identities after unlink = %d, want 0", n)
	}
}
