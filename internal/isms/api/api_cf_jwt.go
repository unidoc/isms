package api

// Cloudflare Access JWT verification. The CF Access proxy signs a JWT and adds
// it to every proxied request as `Cf-Access-Jwt-Assertion`. Without validating
// the signature, an attacker who reaches the origin directly (bypassing the CF
// tunnel) could set the identity header themselves and be anyone.
//
// Public key material lives at `https://<team>.cloudflareaccess.com/cdn-cgi/access/certs`.
// We cache the JWKS in memory and refresh at most every 5 minutes, or on demand
// when a JWT arrives with a kid we don't recognise.
//
// CF Access signs with RS256 (RSA) OR ES256 (EC-P256) depending on the team and
// on key rotations, and a team's JWKS can carry both. An earlier version parsed
// EC keys only, so an RS256 token was rejected as "unknown key ID" and CF Access
// login failed for those teams. This verifier handles both, and cross-checks the
// JWT `alg` against the resolved key type so there is no alg-confusion opening.

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// cfClaims is the projection of the CF Access JWT payload we consume.
type cfClaims struct {
	Email string          `json:"email"`
	Name  string          `json:"name,omitempty"`
	Exp   int64           `json:"exp"`
	Iat   int64           `json:"iat"`
	Iss   string          `json:"iss"`
	Aud   json.RawMessage `json:"aud"` // string OR []string
}

// cfPublicKey wraps either an RSA or ECDSA key so a kid → key lookup returns one
// value the verifier can dispatch on. Exactly one of the pointers is non-nil.
type cfPublicKey struct {
	RSA *rsa.PublicKey
	EC  *ecdsa.PublicKey
}

type cfKeyCache struct {
	teamDomain       string
	expectedAudience string // CF Access Application Audience (AUD) tag
	certsURL         string // JWKS endpoint; overridable in tests
	keys             map[string]cfPublicKey
	mu               sync.RWMutex
	lastFetch        time.Time
	httpClient       *http.Client
}

func newCFKeyCache(teamDomain, audience string) *cfKeyCache {
	return &cfKeyCache{
		teamDomain:       teamDomain,
		expectedAudience: audience,
		certsURL:         fmt.Sprintf("https://%s/cdn-cgi/access/certs", teamDomain),
		keys:             make(map[string]cfPublicKey),
		// Bounded timeout so a hung CF endpoint never wedges a request goroutine
		// indefinitely.
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type cfJWKS struct {
	Keys []cfJWK `json:"keys"`
}

// cfJWK covers both RSA (kty=RSA, n/e) and EC (kty=EC, crv/x/y). The unmarshaller
// populates only the fields the key type actually uses.
type cfJWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg,omitempty"`
	// RSA
	N string `json:"n,omitempty"`
	E string `json:"e,omitempty"`
	// EC
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
}

// fetchKeys refreshes the JWKS. Rate-limited to at most once every 5 minutes for
// normal operation, but callers can pass force to bypass the throttle when they
// see a kid the current cache doesn't know about.
func (c *cfKeyCache) fetchKeys(force bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !force && time.Since(c.lastFetch) < 5*time.Minute && len(c.keys) > 0 {
		return nil
	}

	resp, err := c.httpClient.Get(c.certsURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var jwks cfJWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return err
	}

	keys := make(map[string]cfPublicKey)
	for _, k := range jwks.Keys {
		switch k.Kty {
		case "RSA":
			if k.N == "" || k.E == "" {
				continue
			}
			nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
			if err != nil {
				continue
			}
			eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
			if err != nil {
				continue
			}
			pub := &rsa.PublicKey{
				N: new(big.Int).SetBytes(nBytes),
				E: int(new(big.Int).SetBytes(eBytes).Int64()),
			}
			keys[k.Kid] = cfPublicKey{RSA: pub}
		case "EC":
			if k.X == "" || k.Y == "" {
				continue
			}
			xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
			if err != nil {
				continue
			}
			yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
			if err != nil {
				continue
			}
			pub := &ecdsa.PublicKey{
				Curve: elliptic.P256(),
				X:     new(big.Int).SetBytes(xBytes),
				Y:     new(big.Int).SetBytes(yBytes),
			}
			keys[k.Kid] = cfPublicKey{EC: pub}
		default:
			// Unknown key type; skip silently.
		}
	}

	c.keys = keys
	c.lastFetch = time.Now()
	return nil
}

// VerifyJWT parses and cryptographically validates a CF Access JWT. Returns the
// claims on success. Rejects on any of: malformed JWT, unknown signing key, bad
// signature, expired, wrong issuer, wrong audience.
func (c *cfKeyCache) VerifyJWT(token string) (*cfClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid header: %w", err)
	}
	var header struct {
		Kid string `json:"kid"`
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("invalid header json: %w", err)
	}

	if err := c.fetchKeys(false); err != nil {
		return nil, fmt.Errorf("fetching CF Access keys: %w", err)
	}

	c.mu.RLock()
	key, ok := c.keys[header.Kid]
	c.mu.RUnlock()
	// Unknown kid can mean legitimate key rotation, or the operator pointed us at
	// the wrong team domain. Force one refresh past the 5-min throttle and try
	// again before giving up.
	if !ok {
		if err := c.fetchKeys(true); err != nil {
			return nil, fmt.Errorf("refreshing CF Access keys after kid miss: %w", err)
		}
		c.mu.RLock()
		key, ok = c.keys[header.Kid]
		c.mu.RUnlock()
	}
	if !ok {
		var peek struct {
			Iss string `json:"iss"`
		}
		if b, e := base64.RawURLEncoding.DecodeString(parts[1]); e == nil {
			_ = json.Unmarshal(b, &peek)
		}
		return nil, fmt.Errorf(
			"unknown key id %s (token iss=%q, configured team_domain=%q — do they match?)",
			header.Kid, peek.Iss, c.teamDomain)
	}

	signingInput := parts[0] + "." + parts[1]
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}
	hashed := sha256.Sum256([]byte(signingInput))

	switch header.Alg {
	case "RS256":
		if key.RSA == nil {
			return nil, fmt.Errorf("alg=RS256 but cached key for kid=%s is not RSA", header.Kid)
		}
		if err := rsa.VerifyPKCS1v15(key.RSA, crypto.SHA256, hashed[:], sigBytes); err != nil {
			return nil, fmt.Errorf("RSA signature verification failed: %w", err)
		}
	case "ES256":
		if key.EC == nil {
			return nil, fmt.Errorf("alg=ES256 but cached key for kid=%s is not EC", header.Kid)
		}
		if len(sigBytes) != 64 {
			return nil, fmt.Errorf("invalid ES256 signature length %d", len(sigBytes))
		}
		r := new(big.Int).SetBytes(sigBytes[:32])
		s := new(big.Int).SetBytes(sigBytes[32:])
		if !ecdsa.Verify(key.EC, hashed[:], r, s) {
			return nil, fmt.Errorf("ES256 signature verification failed")
		}
	default:
		return nil, fmt.Errorf("unsupported JWT alg %q (expected RS256 or ES256)", header.Alg)
	}

	claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid claims: %w", err)
	}
	var claims cfClaims
	if err := json.Unmarshal(claimBytes, &claims); err != nil {
		return nil, fmt.Errorf("invalid claims json: %w", err)
	}

	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}

	// Issuer must be the team domain we're validating against, verbatim.
	expectedIss := "https://" + c.teamDomain
	if claims.Iss != expectedIss {
		return nil, fmt.Errorf("issuer mismatch: got %q, want %q", claims.Iss, expectedIss)
	}

	// Without an audience check, ANY CF Access JWT from the same team domain
	// (i.e. any other app policy) would authenticate here.
	if c.expectedAudience != "" {
		if !cfAudContains(claims.Aud, c.expectedAudience) {
			return nil, fmt.Errorf("audience mismatch: token aud does not contain %q", c.expectedAudience)
		}
	}

	return &claims, nil
}

// cfAudContains checks the aud claim, which can be either a string or a JSON
// array of strings.
func cfAudContains(raw json.RawMessage, expected string) bool {
	if len(raw) == 0 {
		return false
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single == expected
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, v := range arr {
			if v == expected {
				return true
			}
		}
	}
	return false
}
