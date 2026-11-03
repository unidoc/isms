package api

// An organization's OIDC provider must not be able to sign in as an account it
// did not create. These drive the real authorize and callback handlers against
// a fake identity provider (httptest + RSA) that issues whatever claims a test
// asks for. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run OIDCLink

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

var oidcLinkSeq atomic.Int64

func oidcLinkUniq() string {
	return fmt.Sprintf("%d%d", time.Now().UnixNano()%1_000_000_000, oidcLinkSeq.Add(1))
}

const oidcLinkClientID = "isms-test-client"

// fakeIDP is a minimal OIDC provider: discovery, JWKS and a token endpoint that
// returns an RS256 ID token carrying the claims set with set().
type fakeIDP struct {
	srv *httptest.Server
	key *rsa.PrivateKey

	mu     sync.Mutex
	claims map[string]any
	nonce  string
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	idp := &fakeIDP{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                idp.srv.URL,
			"authorization_endpoint":                idp.srv.URL + "/authorize",
			"token_endpoint":                        idp.srv.URL + "/token",
			"jwks_uri":                              idp.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "k1",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		idp.mu.Lock()
		claims := jwt.MapClaims{
			"iss": idp.srv.URL, "aud": oidcLinkClientID, "nonce": idp.nonce,
			"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
		}
		for k, v := range idp.claims {
			claims[k] = v
		}
		idp.mu.Unlock()
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = "k1"
		signed, err := tok.SignedString(key)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": signed,
		})
	})
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	return idp
}

type oidcOrg struct {
	id       int
	slug     string
	provider string
}

// newOIDCOrg creates an org with an auto-add provider (reader role) named
// providerName pointing at idp.
func newOIDCOrg(t *testing.T, s *Server, idp *fakeIDP, providerName string) oidcOrg {
	t.Helper()
	name := "oidclink" + oidcLinkUniq()
	id := newTestOrg(t, s, name)
	p := &db.OIDCProvider{
		OrganizationID: id, ProviderName: providerName, DisplayName: providerName,
		ClientID: oidcLinkClientID, ClientSecret: "secret", DiscoveryURL: idp.srv.URL,
		AutoAddMembers: true, DefaultRole: "reader", Enabled: true,
	}
	if err := s.db.CreateOIDCProvider(context.Background(), p); err != nil {
		t.Fatalf("creating provider: %v", err)
	}
	return oidcOrg{id: id, slug: fmt.Sprintf("%s-%d", name, os.Getpid()), provider: providerName}
}

type ssoResult struct {
	status int
	loc    string
	err    error
}

func (r ssoResult) token() string {
	if i := strings.Index(r.loc, "#token="); i >= 0 {
		tok := r.loc[i+len("#token="):]
		if j := strings.Index(tok, "&"); j >= 0 {
			tok = tok[:j]
		}
		return tok
	}
	return ""
}

// sessionEmail is the email in the minted session token ("" if none).
func (r ssoResult) sessionEmail(t *testing.T) string {
	t.Helper()
	tok := r.token()
	if tok == "" {
		return ""
	}
	var claims SessionClaims
	if _, _, err := jwt.NewParser().ParseUnverified(tok, &claims); err != nil {
		t.Fatalf("parsing session token: %v", err)
	}
	return claims.Email
}

// runSSO signs in through org's provider; the IdP asserts claims.
func runSSO(t *testing.T, s *Server, org oidcOrg, idp *fakeIDP, claims map[string]any) ssoResult {
	t.Helper()
	t.Setenv("ISMS_BASE_URL", "http://localhost:9999")
	s.secret = "oidc-link-test-secret-0123456789abcdef"
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/authorize?org="+url.QueryEscape(org.slug)+"&provider="+org.provider, nil)
	rec := httptest.NewRecorder()
	if err := s.handleOIDCAuthorize(e.NewContext(req, rec)); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	au, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("authorize redirect: %v", err)
	}
	idp.mu.Lock()
	idp.claims = claims
	idp.nonce = au.Query().Get("nonce")
	idp.mu.Unlock()

	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=abc&state="+au.Query().Get("state"), nil)
	rec = httptest.NewRecorder()
	err = s.handleOIDCCallback(e.NewContext(req, rec))
	return ssoResult{status: rec.Code, loc: rec.Header().Get("Location"), err: err}
}

func idpClaims(sub, email string) map[string]any {
	return map[string]any{"sub": sub, "email": email, "email_verified": true, "name": "Some One"}
}

func oidcLinkEmail(tag string) string {
	return fmt.Sprintf("%s-%s@example.com", tag, oidcLinkUniq())
}

func identityCount(t *testing.T, s *Server, userID int) int {
	t.Helper()
	var n int
	if err := s.db.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM user_identities WHERE user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("counting identities: %v", err)
	}
	return n
}

func orgIDs(t *testing.T, s *Server, userID int) []int {
	t.Helper()
	rows, err := s.db.Pool().Query(context.Background(),
		`SELECT organization_id FROM organization_members WHERE user_id = $1`, userID)
	if err != nil {
		t.Fatalf("listing orgs: %v", err)
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

// findUser looks an account up case-insensitively.
func findUser(s *Server, email string) (id int, err error) {
	err = s.db.Pool().QueryRow(context.Background(),
		`SELECT id FROM users WHERE lower(email) = lower($1)`, email).Scan(&id)
	return id, err
}

func mustUser(t *testing.T, s *Server, email string) int {
	t.Helper()
	id, err := findUser(s, email)
	if err != nil {
		t.Fatalf("user %s not found: %v", email, err)
	}
	return id
}

// requireRefused asserts a redirect to the login page with the account_exists
// error and no session.
func requireRefused(t *testing.T, r ssoResult) {
	t.Helper()
	if r.err != nil {
		t.Fatalf("callback returned error %v, want a refusal redirect", r.err)
	}
	if r.status != http.StatusFound {
		t.Fatalf("status = %d, want 302", r.status)
	}
	if r.token() != "" || strings.Contains(r.loc, "token=") {
		t.Fatalf("a session was issued: %s", strings.SplitN(r.loc, "#token=", 2)[0]+"#token=<redacted>")
	}
	if !strings.Contains(r.loc, "/login?sso_error=account_exists") {
		t.Fatalf("redirect = %q, want .../login?sso_error=account_exists", r.loc)
	}
}

func requireSession(t *testing.T, r ssoResult, wantEmail string) {
	t.Helper()
	if r.err != nil {
		t.Fatalf("callback error: %v", r.err)
	}
	if got := r.sessionEmail(t); !strings.EqualFold(got, wantEmail) {
		t.Fatalf("session email = %q, want %q (redirect %s)", got, wantEmail, r.loc)
	}
}

func identityIssuer(t *testing.T, s *Server, userID int) []string {
	t.Helper()
	rows, err := s.db.Pool().Query(context.Background(),
		`SELECT COALESCE(issuer, '<null>') FROM user_identities WHERE user_id = $1`, userID)
	if err != nil {
		t.Fatalf("issuer query: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		_ = rows.Scan(&v)
		out = append(out, v)
	}
	return out
}

// a. The takeover: org B's provider asserts the email of an account that has a
// password and belongs only to org A.
func TestOIDCLink_ForeignProviderCannotTakeOverAccount(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgA := newTestOrg(t, s, "oidclinkA"+oidcLinkUniq())
	victimEmail := oidcLinkEmail("victim")
	victim := contractTestUser(t, s, orgA, victimEmail, "admin")
	if err := s.db.SetPassword(ctx, victim.ID, "$2a$10$hash"); err != nil {
		t.Fatal(err)
	}
	idp := newFakeIDP(t)
	orgB := newOIDCOrg(t, s, idp, "attacker")

	r := runSSO(t, s, orgB, idp, idpClaims("attacker-sub-"+oidcLinkUniq(), victimEmail))
	requireRefused(t, r)
	if n := identityCount(t, s, victim.ID); n != 0 {
		t.Errorf("victim has %d linked identities, want 0", n)
	}
	if ids := orgIDs(t, s, victim.ID); len(ids) != 1 || ids[0] != orgA {
		t.Errorf("victim memberships = %v, want only org A (%d)", ids, orgA)
	}
}

// b. A provider in another org with the same NAME must not match a link made
// through a different issuer.
func TestOIDCLink_SameProviderNameDifferentIssuer(t *testing.T) {
	s := testServer(t)
	idp1, idp2 := newFakeIDP(t), newFakeIDP(t)
	orgA := newOIDCOrg(t, s, idp1, "microsoft")
	orgB := newOIDCOrg(t, s, idp2, "microsoft")
	sub := "shared-sub-" + oidcLinkUniq()
	uEmail := oidcLinkEmail("u")

	requireSession(t, runSSO(t, s, orgA, idp1, idpClaims(sub, uEmail)), uEmail)
	uID := mustUser(t, s, uEmail)

	otherEmail := oidcLinkEmail("other")
	r := runSSO(t, s, orgB, idp2, idpClaims(sub, otherEmail))
	if got := r.sessionEmail(t); strings.EqualFold(got, uEmail) {
		t.Fatalf("org B's provider signed in as %s", uEmail)
	}
	if r.err != nil {
		t.Fatalf("callback error: %v", r.err)
	}
	requireSession(t, r, otherEmail)
	if n := identityCount(t, s, uID); n != 1 {
		t.Errorf("U has %d identities, want 1", n)
	}
}

// c. A new email through SSO creates the account and links it with the issuer.
func TestOIDCLink_NewEmailCreatesAccountAndLinksIssuer(t *testing.T) {
	s := testServer(t)
	idp := newFakeIDP(t)
	org := newOIDCOrg(t, s, idp, "acme")
	email := oidcLinkEmail("fresh")

	requireSession(t, runSSO(t, s, org, idp, idpClaims("sub-"+oidcLinkUniq(), email)), email)
	uID := mustUser(t, s, email)
	if got := identityIssuer(t, s, uID); len(got) != 1 || got[0] != idp.srv.URL {
		t.Errorf("identity issuers = %v, want [%s]", got, idp.srv.URL)
	}
	if ids := orgIDs(t, s, uID); len(ids) != 1 || ids[0] != org.id {
		t.Errorf("memberships = %v, want [%d]", ids, org.id)
	}
}

// d. A returning user is found by (issuer, sub) even when the email changed.
func TestOIDCLink_ReturningUserBySubject(t *testing.T) {
	s := testServer(t)
	idp := newFakeIDP(t)
	org := newOIDCOrg(t, s, idp, "acme")
	sub := "sub-" + oidcLinkUniq()
	email := oidcLinkEmail("ret")

	requireSession(t, runSSO(t, s, org, idp, idpClaims(sub, email)), email)
	changed := oidcLinkEmail("renamed")
	requireSession(t, runSSO(t, s, org, idp, idpClaims(sub, changed)), email)
	if _, err := findUser(s, changed); err == nil {
		t.Errorf("an account was created for the changed email %s", changed)
	}
}

// e. D11: an admin-created account with no credentials that belongs only to the
// provider's org is linked by email; one that also belongs elsewhere is refused.
func TestOIDCLink_BareAccountOnlyInProvidersOrg(t *testing.T) {
	s := testServer(t)
	idp := newFakeIDP(t)
	org := newOIDCOrg(t, s, idp, "acme")
	email := oidcLinkEmail("bare")
	u := contractTestUser(t, s, org.id, email, "contributor")

	requireSession(t, runSSO(t, s, org, idp, idpClaims("sub-"+oidcLinkUniq(), email)), email)
	if got := identityIssuer(t, s, u.ID); len(got) != 1 || got[0] != idp.srv.URL {
		t.Errorf("identity issuers = %v, want [%s]", got, idp.srv.URL)
	}
}

func TestOIDCLink_BareAccountAlsoInAnotherOrgRefused(t *testing.T) {
	s := testServer(t)
	idp := newFakeIDP(t)
	org := newOIDCOrg(t, s, idp, "acme")
	other := newTestOrg(t, s, "oidclinkO"+oidcLinkUniq())
	email := oidcLinkEmail("bare2")
	u := contractTestUser(t, s, org.id, email, "contributor")
	if err := s.db.AddOrgMember(context.Background(), other, u.ID, "reader"); err != nil {
		t.Fatal(err)
	}

	requireRefused(t, runSSO(t, s, org, idp, idpClaims("sub-"+oidcLinkUniq(), email)))
	if n := identityCount(t, s, u.ID); n != 0 {
		t.Errorf("identities = %d, want 0", n)
	}
}

// An account with a passkey or an identity linked elsewhere is not bare.
func TestOIDCLink_AccountWithPasskeyOrIdentityRefused(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	idp := newFakeIDP(t)
	org := newOIDCOrg(t, s, idp, "acme")

	pkEmail := oidcLinkEmail("pk")
	pk := contractTestUser(t, s, org.id, pkEmail, "contributor")
	if _, err := s.db.Pool().Exec(ctx,
		`INSERT INTO webauthn_credentials (user_id, credential_id, public_key) VALUES ($1, $2, $3)`,
		pk.ID, []byte("cred-"+oidcLinkUniq()), []byte("pub")); err != nil {
		t.Fatalf("inserting passkey: %v", err)
	}
	requireRefused(t, runSSO(t, s, org, idp, idpClaims("sub-"+oidcLinkUniq(), pkEmail)))

	idp2 := newFakeIDP(t)
	org2 := newOIDCOrg(t, s, idp2, "acme")
	linkedEmail := oidcLinkEmail("linked")
	requireSession(t, runSSO(t, s, org2, idp2, idpClaims("sub-"+oidcLinkUniq(), linkedEmail)), linkedEmail)
	// Same account, a second org's provider at a different issuer.
	idp3 := newFakeIDP(t)
	org3 := newOIDCOrg(t, s, idp3, "acme")
	requireRefused(t, runSSO(t, s, org3, idp3, idpClaims("sub-"+oidcLinkUniq(), linkedEmail)))
}

// f. Rows linked before issuers were recorded still work for a member of the
// provider's org, and get their issuer filled in.
func TestOIDCLink_LegacyRowMemberSignsInAndIsBackfilled(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	idp := newFakeIDP(t)
	org := newOIDCOrg(t, s, idp, "microsoft")
	email := oidcLinkEmail("legacy")
	u := contractTestUser(t, s, org.id, email, "contributor")
	sub := "legacy-sub-" + oidcLinkUniq()
	if _, err := s.db.Pool().Exec(ctx,
		`INSERT INTO user_identities (user_id, provider, subject, legacy_org_id) VALUES ($1, 'microsoft', $2, $3)`, u.ID, sub, org.id); err != nil {
		t.Fatal(err)
	}

	requireSession(t, runSSO(t, s, org, idp, idpClaims(sub, oidcLinkEmail("elsewhere"))), email)
	if got := identityIssuer(t, s, u.ID); len(got) != 1 || got[0] != idp.srv.URL {
		t.Errorf("identity issuers = %v, want [%s] (backfilled)", got, idp.srv.URL)
	}
}

func TestOIDCLink_LegacyRowNonMemberNotMatched(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	idp := newFakeIDP(t)
	orgB := newOIDCOrg(t, s, idp, "microsoft")
	orgA := newTestOrg(t, s, "oidclinkL"+oidcLinkUniq())
	email := oidcLinkEmail("legacy2")
	u := contractTestUser(t, s, orgA, email, "admin")
	if err := s.db.SetPassword(ctx, u.ID, "$2a$10$hash"); err != nil {
		t.Fatal(err)
	}
	sub := "legacy-sub-" + oidcLinkUniq()
	if _, err := s.db.Pool().Exec(ctx,
		`INSERT INTO user_identities (user_id, provider, subject, legacy_org_id) VALUES ($1, 'microsoft', $2, $3)`, u.ID, sub, orgA); err != nil {
		t.Fatal(err)
	}

	requireRefused(t, runSSO(t, s, orgB, idp, idpClaims(sub, email)))
	if got := identityIssuer(t, s, u.ID); len(got) != 1 || got[0] != "<null>" {
		t.Errorf("identity issuers = %v, want the untouched legacy row", got)
	}
}

// g. A different spelling of the email is the same account.
func TestOIDCLink_EmailCaseDoesNotBypass(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	idp := newFakeIDP(t)
	orgB := newOIDCOrg(t, s, idp, "attacker")
	orgA := newTestOrg(t, s, "oidclinkC"+oidcLinkUniq())
	email := strings.ToLower(oidcLinkEmail("alice"))
	victim := contractTestUser(t, s, orgA, email, "admin")
	if err := s.db.SetPassword(ctx, victim.ID, "$2a$10$hash"); err != nil {
		t.Fatal(err)
	}

	requireRefused(t, runSSO(t, s, orgB, idp, idpClaims("sub-"+oidcLinkUniq(), strings.ToUpper(email))))
	if n := identityCount(t, s, victim.ID); n != 0 {
		t.Errorf("victim identities = %d, want 0", n)
	}

	// Bare account, upper-case claim: linked to the existing account.
	idp2 := newFakeIDP(t)
	org := newOIDCOrg(t, s, idp2, "acme")
	bareEmail := strings.ToLower(oidcLinkEmail("bob"))
	bare := contractTestUser(t, s, org.id, bareEmail, "contributor")
	requireSession(t, runSSO(t, s, org, idp2, idpClaims("sub-"+oidcLinkUniq(), strings.ToUpper(bareEmail))), bareEmail)
	if n := identityCount(t, s, bare.ID); n != 1 {
		t.Errorf("bare account identities = %d, want 1", n)
	}
}

// A legacy row is bound to the organization it was made in. Another
// organization that reuses the provider name, even one the user was added to,
// does not match it and does not rewrite it.
func TestOIDCLink_LegacyRowBoundToItsOrganization(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	idpA, idpB := newFakeIDP(t), newFakeIDP(t)
	orgA := newOIDCOrg(t, s, idpA, "google")
	orgB := newOIDCOrg(t, s, idpB, "google")
	email := oidcLinkEmail("victim")
	u := contractTestUser(t, s, orgA.id, email, "contributor")
	if err := s.db.AddOrgMember(ctx, orgB.id, u.ID, "reader"); err != nil {
		t.Fatal(err)
	}
	sub := "legacy-sub-" + oidcLinkUniq()
	if _, err := s.db.Pool().Exec(ctx,
		`INSERT INTO user_identities (user_id, provider, subject, legacy_org_id) VALUES ($1, 'google', $2, $3)`, u.ID, sub, orgA.id); err != nil {
		t.Fatal(err)
	}

	// Org B's provider, with the victim's legacy subject: refused, row untouched.
	requireRefused(t, runSSO(t, s, orgB, idpB, idpClaims(sub, email)))
	// Nor does it sign in as the victim when the email differs.
	other := runSSO(t, s, orgB, idpB, idpClaims(sub, oidcLinkEmail("attacker")))
	if got := other.sessionEmail(t); strings.EqualFold(got, email) {
		t.Fatalf("org B signed in as the victim via the legacy subject")
	}
	if got := identityIssuer(t, s, u.ID); len(got) != 1 || got[0] != "<null>" {
		t.Errorf("identity issuers = %v, want the untouched legacy row", got)
	}
	// The organization it belongs to still honours it and backfills.
	requireSession(t, runSSO(t, s, orgA, idpA, idpClaims(sub, email)), email)
	if got := identityIssuer(t, s, u.ID); len(got) != 1 || got[0] != idpA.srv.URL {
		t.Errorf("identity issuers = %v, want [%s]", got, idpA.srv.URL)
	}
}

// A legacy row the migration could not bind (NULL legacy_org_id: ambiguous or
// no matching organization) never matches.
func TestOIDCLink_UnboundLegacyRowNeverMatches(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	idp := newFakeIDP(t)
	org := newOIDCOrg(t, s, idp, "google")
	email := oidcLinkEmail("ambig")
	u := contractTestUser(t, s, org.id, email, "contributor")
	sub := "legacy-sub-" + oidcLinkUniq()
	if _, err := s.db.Pool().Exec(ctx,
		`INSERT INTO user_identities (user_id, provider, subject) VALUES ($1, 'google', $2)`, u.ID, sub); err != nil {
		t.Fatal(err)
	}
	requireRefused(t, runSSO(t, s, org, idp, idpClaims(sub, email)))
	if got := identityIssuer(t, s, u.ID); len(got) != 1 || got[0] != "<null>" {
		t.Errorf("identity issuers = %v, want the untouched legacy row", got)
	}
}

// A lookup that fails for any reason other than "no such row" must not be taken
// for "no account": falling through would create (UpsertUser overwrites an
// existing row) and link. Here the existing account's row cannot be read (its
// stored OTP secret does not decrypt under the database key).
func TestOIDCLink_LookupFailureIsAnErrorNotNoAccount(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	idp := newFakeIDP(t)
	org := newOIDCOrg(t, s, idp, "acme")
	email := oidcLinkEmail("unreadable")
	u := contractTestUser(t, s, org.id, email, "contributor")
	garbage := base64.StdEncoding.EncodeToString(make([]byte, 48))
	if _, err := s.db.Pool().Exec(ctx, `UPDATE users SET otp_secret = $2 WHERE id = $1`, u.ID, garbage); err != nil {
		t.Fatal(err)
	}
	s.db.SetEncryptionKey("lookup-failure-test-key-0123456789abcdef")
	t.Cleanup(func() { s.db.SetEncryptionKey("") })

	// The provider row was stored before the key was set; its secret still reads.
	r := runSSO(t, s, org, idp, idpClaims("sub-"+oidcLinkUniq(), email))
	var he *echo.HTTPError
	if !errors.As(r.err, &he) || he.Code != http.StatusInternalServerError {
		t.Fatalf("callback = status %d err %v loc %q, want a 500 error", r.status, r.err, r.loc)
	}
	if r.token() != "" {
		t.Fatal("a session was issued")
	}
	var name string
	if err := s.db.Pool().QueryRow(ctx, `SELECT name FROM users WHERE id = $1`, u.ID).Scan(&name); err != nil || name != email {
		t.Errorf("account row changed: name %q err %v", name, err)
	}
	if n := identityCount(t, s, u.ID); n != 0 {
		t.Errorf("identities = %d, want 0", n)
	}
}
