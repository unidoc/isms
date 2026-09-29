package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	gowebauthn "github.com/go-webauthn/webauthn/webauthn"

	"isms.sh/internal/isms/db"
)

// TestBuildOTPAuthURI needs no database: it only exercises the pure encoding
// helper, so it always runs, unlike the *_pg_test.go suite.
func TestBuildOTPAuthURI(t *testing.T) {
	issuer := "Acme Corp & Co: Security"
	account := "a+b@example.com"
	secret := "SECRETXYZ"

	uri := buildOTPAuthURI(issuer, account, secret)

	u, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("uri does not round-trip through url.Parse: %v (uri=%q)", err, uri)
	}

	if got := u.Query().Get("issuer"); got != issuer {
		t.Errorf("issuer query param = %q, want %q", got, issuer)
	}
	if got := u.Query().Get("secret"); got != secret {
		t.Errorf("secret query param = %q, want %q", got, secret)
	}

	// The label is "issuer:account" — split on the raw (still-escaped) path so
	// a colon inside the issuer (encoded as %3A) can't be confused with the
	// literal separator colon. Decoding first, as u.Path does, would make both
	// indistinguishable.
	escLabel := strings.TrimPrefix(u.EscapedPath(), "/")
	i := strings.LastIndex(escLabel, ":")
	if i < 0 {
		t.Fatalf("label has no separator colon: %q", escLabel)
	}
	gotIssuer, err := url.QueryUnescape(escLabel[:i])
	if err != nil {
		t.Fatalf("unescaping label issuer part: %v", err)
	}
	gotAccount, err := url.QueryUnescape(escLabel[i+1:])
	if err != nil {
		t.Fatalf("unescaping label account part: %v", err)
	}
	if gotIssuer != issuer {
		t.Errorf("label issuer = %q, want %q", gotIssuer, issuer)
	}
	if gotAccount != account {
		t.Errorf("label account = %q, want %q", gotAccount, account)
	}
}

// TestOrgBrandNamePrecedence covers branding_name set / blank / unset, and the
// "no organization" fallback, against a live Postgres. Requires a migrated
// database; skipped otherwise (see id_resolution_pg_test.go's testServer).
func TestOrgBrandNamePrecedence(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "brand-precedence")

	// newTestOrg gives the org a Name equal to its slug — a non-empty org.Name
	// to fall back to, distinct from the "ISMS" default.
	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}

	if got := s.orgBrandName(ctx, orgID); got != org.Name {
		t.Errorf("with branding_name unset: got %q, want org.Name %q", got, org.Name)
	}

	if err := s.db.SetOrgSetting(ctx, orgID, "branding_name", "   "); err != nil {
		t.Fatalf("setting blank branding_name: %v", err)
	}
	if got := s.orgBrandName(ctx, orgID); got != org.Name {
		t.Errorf("with branding_name blank/whitespace: got %q, want org.Name %q", got, org.Name)
	}

	if err := s.db.SetOrgSetting(ctx, orgID, "branding_name", "Acme Security & Co"); err != nil {
		t.Fatalf("setting branding_name: %v", err)
	}
	if got := s.orgBrandName(ctx, orgID); got != "Acme Security & Co" {
		t.Errorf("with branding_name set: got %q, want %q", got, "Acme Security & Co")
	}

	if got := s.orgBrandName(ctx, 0); got != "ISMS" {
		t.Errorf("org 0: got %q, want %q", got, "ISMS")
	}
}

// TestHandleOTPSetupUsesOrgBrand confirms the returned otpauth URI carries the
// org's branding_name, percent-encoded, as issuer. Requires a live Postgres.
func TestHandleOTPSetupUsesOrgBrand(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "otp-brand")

	if err := s.db.SetOrgSetting(ctx, orgID, "branding_name", "Acme Security & Co"); err != nil {
		t.Fatalf("setting branding_name: %v", err)
	}

	u := &db.User{Email: "otp-brand@example.com", Name: "OTP Brand", Active: true}
	if err := s.db.UpsertUser(ctx, u); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	t.Cleanup(func() { _ = s.db.DeleteUser(context.Background(), u.ID) })

	c, rec := ctxForPath(orgID, http.MethodPost, "/api/v1/auth/otp/setup", "", "reader")
	c.Set("user_email", u.Email)
	if err := s.handleOTPSetup(c); err != nil {
		t.Fatalf("handleOTPSetup: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}

	var resp otpSetupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	parsed, err := url.Parse(resp.URI)
	if err != nil {
		t.Fatalf("uri does not parse: %v (uri=%q)", err, resp.URI)
	}
	if got := parsed.Query().Get("issuer"); got != "Acme Security & Co" {
		t.Errorf("issuer = %q, want %q", got, "Acme Security & Co")
	}
}

// TestHandlePasskeyRegisterBeginUsesOrgBrand confirms the WebAuthn creation
// options carry the org's brand as the relying party display name, while the
// relying party ID (which existing passkeys are bound to) stays unchanged.
// testServer doesn't configure s.webauthn, so this test builds its own,
// mirroring the "ISMS" global fallback in server.go. Requires a live Postgres.
func TestHandlePasskeyRegisterBeginUsesOrgBrand(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "passkey-brand")

	if err := s.db.SetOrgSetting(ctx, orgID, "branding_name", "Acme Security & Co"); err != nil {
		t.Fatalf("setting branding_name: %v", err)
	}

	u := &db.User{Email: "passkey-brand@example.com", Name: "Passkey Brand", Active: true}
	if err := s.db.UpsertUser(ctx, u); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	t.Cleanup(func() { _ = s.db.DeleteUser(context.Background(), u.ID) })

	wan, err := gowebauthn.New(&gowebauthn.Config{
		RPDisplayName: "ISMS",
		RPID:          "localhost",
		RPOrigins:     []string{"http://localhost"},
	})
	if err != nil {
		t.Fatalf("gowebauthn.New: %v", err)
	}
	s.webauthn = wan

	c, rec := ctxForPath(orgID, http.MethodPost, "/api/v1/auth/passkey/register/begin", "", "reader")
	c.Set("user_email", u.Email)
	if err := s.handlePasskeyRegisterBegin(c); err != nil {
		t.Fatalf("handlePasskeyRegisterBegin: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}

	var creation protocol.CredentialCreation
	if err := json.Unmarshal(rec.Body.Bytes(), &creation); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got := creation.Response.RelyingParty.Name; got != "Acme Security & Co" {
		t.Errorf("rp.name = %q, want %q", got, "Acme Security & Co")
	}
	if got := creation.Response.RelyingParty.ID; got != "localhost" {
		t.Errorf("rp.id = %q, want unchanged %q", got, "localhost")
	}
}
