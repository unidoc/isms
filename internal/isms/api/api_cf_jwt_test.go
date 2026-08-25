package api

// Unit tests for the Cloudflare Access JWT verifier. The load-bearing case is
// RS256: an earlier verifier parsed EC keys only, so RSA-signed CF Access tokens
// were rejected as "unknown key ID" and login failed for teams whose JWKS signs
// with RSA. These exercise the real JWKS fetch + parse via an httptest server
// (certsURL override), so the RSA key-parsing path is covered, not stubbed.

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testTeamDomain = "testteam.cloudflareaccess.com"
	testAudience   = "test-aud-tag"
	rsaKid         = "rsa-kid-1"
	ecKid          = "ec-kid-1"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// jwksServer serves a JWKS carrying both the RSA and the EC public key, the way a
// real CF Access team can — the mix this verifier exists to handle.
func jwksServer(t *testing.T, rsaPub *rsa.PublicKey, ecPub *ecdsa.PublicKey) *httptest.Server {
	t.Helper()
	jwks := cfJWKS{Keys: []cfJWK{
		{
			Kid: rsaKid, Kty: "RSA", Alg: "RS256",
			N: b64(rsaPub.N.Bytes()),
			E: b64(big.NewInt(int64(rsaPub.E)).Bytes()),
		},
		{
			Kid: ecKid, Kty: "EC", Alg: "ES256", Crv: "P-256",
			X: b64(ecPub.X.FillBytes(make([]byte, 32))),
			Y: b64(ecPub.Y.FillBytes(make([]byte, 32))),
		},
	}}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jwks)
	}))
}

func baseClaims() map[string]any {
	return map[string]any{
		"email": "user@example.com",
		"name":  "Test User",
		"iss":   "https://" + testTeamDomain,
		"aud":   testAudience,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
}

func mintToken(t *testing.T, header map[string]string, claims map[string]any, sign func([]byte) []byte) string {
	t.Helper()
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	signingInput := b64(hb) + "." + b64(cb)
	return signingInput + "." + b64(sign([]byte(signingInput)))
}

func signRS256(t *testing.T, priv *rsa.PrivateKey, kid string, claims map[string]any) string {
	return mintToken(t, map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"}, claims, func(in []byte) []byte {
		h := sha256.Sum256(in)
		sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, h[:])
		if err != nil {
			t.Fatalf("rsa sign: %v", err)
		}
		return sig
	})
}

func signES256(t *testing.T, priv *ecdsa.PrivateKey, kid string, claims map[string]any) string {
	return mintToken(t, map[string]string{"alg": "ES256", "kid": kid}, claims, func(in []byte) []byte {
		h := sha256.Sum256(in)
		r, s, err := ecdsa.Sign(rand.Reader, priv, h[:])
		if err != nil {
			t.Fatalf("ecdsa sign: %v", err)
		}
		sig := make([]byte, 64)
		r.FillBytes(sig[:32])
		s.FillBytes(sig[32:])
		return sig
	})
}

// testKeys generates a fresh RSA + EC keypair and a cache wired to a JWKS server
// carrying both. Returns the cache and the private keys for minting tokens.
func testKeys(t *testing.T) (*cfKeyCache, *rsa.PrivateKey, *ecdsa.PrivateKey) {
	t.Helper()
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	srv := jwksServer(t, &rsaPriv.PublicKey, &ecPriv.PublicKey)
	t.Cleanup(srv.Close)
	c := newCFKeyCache(testTeamDomain, testAudience)
	c.certsURL = srv.URL // point the JWKS fetch at the test server instead of CF
	return c, rsaPriv, ecPriv
}

// The regression: an RS256-signed token must verify. EC-only parsing dropped the
// RSA key at JWKS parse, so this token's kid was never found.
func TestVerifyJWT_RS256_Verifies(t *testing.T) {
	c, rsaPriv, _ := testKeys(t)
	claims, err := c.VerifyJWT(signRS256(t, rsaPriv, rsaKid, baseClaims()))
	if err != nil {
		t.Fatalf("RS256 token should verify, got: %v", err)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("email = %q, want user@example.com", claims.Email)
	}
}

func TestVerifyJWT_ES256_Verifies(t *testing.T) {
	c, _, ecPriv := testKeys(t)
	claims, err := c.VerifyJWT(signES256(t, ecPriv, ecKid, baseClaims()))
	if err != nil {
		t.Fatalf("ES256 token should verify, got: %v", err)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("email = %q, want user@example.com", claims.Email)
	}
}

func TestVerifyJWT_Rejects(t *testing.T) {
	cases := []struct {
		name string
		tok  func(t *testing.T, c *cfKeyCache, rsaPriv *rsa.PrivateKey, ecPriv *ecdsa.PrivateKey) string
		want string // substring expected in the error
	}{
		{
			name: "wrong audience",
			tok: func(t *testing.T, _ *cfKeyCache, r *rsa.PrivateKey, _ *ecdsa.PrivateKey) string {
				cl := baseClaims()
				cl["aud"] = "some-other-app"
				return signRS256(t, r, rsaKid, cl)
			},
			want: "audience",
		},
		{
			name: "wrong issuer",
			tok: func(t *testing.T, _ *cfKeyCache, r *rsa.PrivateKey, _ *ecdsa.PrivateKey) string {
				cl := baseClaims()
				cl["iss"] = "https://evil.cloudflareaccess.com"
				return signRS256(t, r, rsaKid, cl)
			},
			want: "issuer mismatch",
		},
		{
			name: "expired",
			tok: func(t *testing.T, _ *cfKeyCache, r *rsa.PrivateKey, _ *ecdsa.PrivateKey) string {
				cl := baseClaims()
				cl["exp"] = time.Now().Add(-time.Hour).Unix()
				return signRS256(t, r, rsaKid, cl)
			},
			want: "expired",
		},
		{
			name: "bad signature (signed with an unpublished key)",
			tok: func(t *testing.T, _ *cfKeyCache, _ *rsa.PrivateKey, _ *ecdsa.PrivateKey) string {
				other, _ := rsa.GenerateKey(rand.Reader, 2048)
				return signRS256(t, other, rsaKid, baseClaims())
			},
			want: "signature verification failed",
		},
		{
			name: "alg/key-type mismatch (RS256 header on an EC kid)",
			tok: func(t *testing.T, _ *cfKeyCache, r *rsa.PrivateKey, _ *ecdsa.PrivateKey) string {
				// header says RS256 but names the EC kid; must be refused before
				// any signature check (no alg-confusion opening).
				return signRS256(t, r, ecKid, baseClaims())
			},
			want: "is not RSA",
		},
		{
			name: "unsupported alg",
			tok: func(t *testing.T, _ *cfKeyCache, r *rsa.PrivateKey, _ *ecdsa.PrivateKey) string {
				return mintToken(t, map[string]string{"alg": "HS256", "kid": rsaKid}, baseClaims(), func(in []byte) []byte {
					return []byte("irrelevant")
				})
			},
			want: "unsupported JWT alg",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rsaPriv, ecPriv := testKeys(t)
			_, err := c.VerifyJWT(tc.tok(t, c, rsaPriv, ecPriv))
			if err == nil {
				t.Fatalf("expected rejection, got nil error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}

// A string aud and an array aud must both match (CF Access sends either).
func TestCFAudContains(t *testing.T) {
	if !cfAudContains(json.RawMessage(`"tag-1"`), "tag-1") {
		t.Error("string aud should match")
	}
	if !cfAudContains(json.RawMessage(`["tag-0","tag-1"]`), "tag-1") {
		t.Error("array aud should match")
	}
	if cfAudContains(json.RawMessage(`"tag-1"`), "tag-2") {
		t.Error("non-matching string aud should not match")
	}
	if cfAudContains(json.RawMessage(``), "tag-1") {
		t.Error("empty aud should not match")
	}
}
