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
	f.e.POST("/api/v1/auth/oidc/link/complete", f.s.handleOIDCLinkComplete)
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

// linkCode is the one-time code in the callback's redirect to Settings.
func linkCode(t *testing.T, res ssoResult) string {
	t.Helper()
	if res.status != http.StatusFound || strings.Contains(res.loc, "token=") {
		t.Fatalf("callback = %d %q, want a 302 with no token", res.status, res.loc)
	}
	u, err := url.Parse(res.loc)
	if err != nil || !strings.HasSuffix(u.Path, "/"+"settings") || u.Query().Get("sso_link_code") == "" {
		t.Fatalf("callback redirect = %q, want .../settings?sso_link_code=...", res.loc)
	}
	return u.Query().Get("sso_link_code")
}

// complete posts the code with the given session; it returns the status and
// the "result" field.
func (f *containFixture) complete(token, code string) (int, string) {
	status, body := f.do("POST", "/auth/oidc/link/complete", token, `{"code":"`+code+`"}`)
	var r struct {
		Result string `json:"result"`
	}
	_ = json.Unmarshal([]byte(body), &r)
	return status, r.Result + " " + body
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
	code := linkCode(t, res)
	if n := identityCount(t, f.s, u.ID); n != 0 {
		t.Fatalf("identity linked by the callback alone: %d", n)
	}
	if status, out := f.complete(tok, code); status != 200 || !strings.HasPrefix(out, "linked ") {
		t.Fatalf("complete = %d %s, want 200 linked", status, out)
	}
	if got := identityIssuer(t, f.s, u.ID); len(got) != 1 || got[0] != f.idp.srv.URL {
		t.Errorf("issuers = %v, want [%s]", got, f.idp.srv.URL)
	}
	if after := orgIDs(t, f.s, u.ID); len(after) != len(orgsBefore) {
		t.Errorf("memberships changed: %v -> %v", orgsBefore, after)
	}
	// The code is single-use.
	if status, out := f.complete(tok, code); status != 400 {
		t.Errorf("reused code = %d %s, want 400", status, out)
	}

	// Now an ordinary SSO sign-in through the same provider is that user.
	requireSession(t, runSSO(t, f.s, f.orgA, f.idp, idpClaims(sub, oidcLinkEmail("other"))), u.Email)

	// Linking the same identity again is a no-op success.
	res = f.finishLink(t, f.beginLink(t, tok), idpClaims(sub, u.Email))
	if status, out := f.complete(tok, linkCode(t, res)); status != 200 || !strings.HasPrefix(out, "linked ") {
		t.Errorf("relink = %d %s, want linked", status, out)
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
	if status, out := f.complete(tok, linkCode(t, res)); status != 200 || !strings.HasPrefix(out, "in_use ") {
		t.Fatalf("complete = %d %s, want 200 in_use", status, out)
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

// The forwarded-link attack: the code must be completed by the account that
// started the flow, not by whoever's browser the identity provider answered.
func TestOIDCLinkFlow_OnlyTheStartingAccountCompletes(t *testing.T) {
	f := linkFixture(t)
	a, tokA := f.passwordUser(t)
	b, tokB := f.passwordUser(t)
	sub := "sub-" + uuid.NewString()

	code := linkCode(t, f.finishLink(t, f.beginLink(t, tokA), idpClaims(sub, b.Email)))
	if status, out := f.complete(tokB, code); status != 403 {
		t.Fatalf("another account completing the link = %d %s, want 403", status, out)
	}
	if identityCount(t, f.s, a.ID)+identityCount(t, f.s, b.ID) != 0 {
		t.Fatal("an identity was linked")
	}
	// The wrong attempt burned the code, so the owner has to start again.
	if status, out := f.complete(tokA, code); status != 400 {
		t.Errorf("owner after the burn = %d %s, want 400", status, out)
	}

	// Expired codes are refused.
	code = linkCode(t, f.finishLink(t, f.beginLink(t, tokA), idpClaims(sub, a.Email)))
	if _, err := f.s.db.Pool().Exec(context.Background(),
		`UPDATE oidc_link_pending SET expires_at = now() - interval '1 minute' WHERE user_id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if status, out := f.complete(tokA, code); status != 400 {
		t.Errorf("expired code = %d %s, want 400", status, out)
	}

	// A contained session and a key cannot complete, even with a valid code.
	code = linkCode(t, f.finishLink(t, f.beginLink(t, tokA), idpClaims(sub, a.Email)))
	key, hash := generateAPIKey()
	orgA := f.orgA.id
	if _, err := f.s.db.CreateAPIKey(context.Background(), "t", hash, a.Email, "read-write", &orgA, nil); err != nil {
		t.Fatal(err)
	}
	if status, out := f.complete(f.sso, code); status != 403 {
		t.Errorf("contained session = %d %s, want 403", status, out)
	}
	if status, out := f.complete(key, code); status != 403 {
		t.Errorf("API key = %d %s, want 403", status, out)
	}
	// Neither refusal consumed it: the owner still completes.
	if status, out := f.complete(tokA, code); status != 200 || !strings.HasPrefix(out, "linked ") {
		t.Errorf("owner = %d %s, want linked", status, out)
	}
}
