package api

// PoC for PR #223 review finding F2, adapted to measure the SHIPPED fix
// (tryForceRefresh's cooldown) rather than the pre-fix head. Alip's original
// PoC's "head" variant called fetchKeys(true) directly; that path no longer
// exists in production code (VerifyJWT now calls tryForceRefresh), so "head"
// here is inlined instead to reproduce the vulnerable behaviour for comparison,
// and "fixed" calls the real, shipped c.tryForceRefresh() — not a second
// reimplementation of the suggested fix.
//
// Run: go test ./internal/isms/api -run TestPoC223 -v

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	pocJWKSDelay = 200 * time.Millisecond // stand-in for the CF round-trip
	pocFloodSize = 8                      // attacker requests with unknown kids
)

func slowJWKSServer(t *testing.T, rsaPub *rsa.PublicKey, ecPub *ecdsa.PublicKey, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	jwks := cfJWKS{Keys: []cfJWK{
		{Kid: rsaKid, Kty: "RSA", Alg: "RS256",
			N: b64(rsaPub.N.Bytes()),
			E: b64(big.NewInt(int64(rsaPub.E)).Bytes())},
		{Kid: ecKid, Kty: "EC", Alg: "ES256", Crv: "P-256",
			X: b64(ecPub.X.FillBytes(make([]byte, 32))),
			Y: b64(ecPub.Y.FillBytes(make([]byte, 32)))},
	}}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		time.Sleep(pocJWKSDelay)
		_ = json.NewEncoder(w).Encode(jwks)
	}))
}

func pocCache(t *testing.T) (*cfKeyCache, *rsa.PrivateKey, *atomic.Int64) {
	t.Helper()
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hits := &atomic.Int64{}
	srv := slowJWKSServer(t, &rsaPriv.PublicKey, &ecPriv.PublicKey, hits)
	t.Cleanup(srv.Close)
	c := newCFKeyCache(testTeamDomain, testAudience)
	c.certsURL = srv.URL
	return c, rsaPriv, hits
}

// baseKidMiss reproduces master: look the kid up, give up if it's absent.
func baseKidMiss(c *cfKeyCache, kid string) {
	c.mu.RLock()
	_, ok := c.keys[kid]
	c.mu.RUnlock()
	_ = ok
}

// vulnerableKidMiss reproduces the pre-fix behaviour this PoC targets: an
// unconditional fetchKeys(true) on every miss, no cooldown.
func vulnerableKidMiss(c *cfKeyCache, kid string) {
	c.mu.RLock()
	_, ok := c.keys[kid]
	c.mu.RUnlock()
	if !ok {
		_ = c.fetchKeys(true)
		c.mu.RLock()
		_, ok = c.keys[kid]
		c.mu.RUnlock()
	}
	_ = ok
}

// fixedKidMiss calls the actual shipped code — c.tryForceRefresh() — not a
// reimplementation of it.
func fixedKidMiss(c *cfKeyCache, kid string) {
	c.mu.RLock()
	_, ok := c.keys[kid]
	c.mu.RUnlock()
	if !ok {
		if ran, _ := c.tryForceRefresh(); ran {
			c.mu.RLock()
			_, ok = c.keys[kid]
			c.mu.RUnlock()
		}
	}
	_ = ok
}

type pocResult struct {
	forcedFetches int64
	legitLatency  time.Duration
}

func runFlood(t *testing.T, kidMiss func(*cfKeyCache, string)) pocResult {
	t.Helper()
	c, rsaPriv, hits := pocCache(t)

	if _, err := c.VerifyJWT(signRS256(t, rsaPriv, rsaKid, baseClaims())); err != nil {
		t.Fatalf("warm-up verify should succeed: %v", err)
	}
	warm := hits.Load()

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < pocFloodSize; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			kidMiss(c, fmt.Sprintf("attacker-kid-%d", i))
		}(i)
	}

	legit := make(chan time.Duration, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		time.Sleep(20 * time.Millisecond)
		t0 := time.Now()
		if _, err := c.VerifyJWT(signRS256(t, rsaPriv, rsaKid, baseClaims())); err != nil {
			t.Errorf("legitimate RS256 verify failed: %v", err)
		}
		legit <- time.Since(t0)
	}()

	close(start)
	wg.Wait()
	return pocResult{forcedFetches: hits.Load() - warm, legitLatency: <-legit}
}

func TestPoC223_UnknownKidFloodStallsLegitimateVerification(t *testing.T) {
	base := runFlood(t, baseKidMiss)
	vulnerable := runFlood(t, vulnerableKidMiss)
	fixed := runFlood(t, fixedKidMiss)

	for _, r := range []struct {
		name string
		res  pocResult
	}{{"base (master)", base}, {"vulnerable (pre-fix #223)", vulnerable}, {"fixed (shipped)", fixed}} {
		t.Logf("%-25s | forced JWKS fetches from %d unknown-kid requests: %d | legit verify latency: %v",
			r.name, pocFloodSize, r.res.forcedFetches, r.res.legitLatency)
	}

	// The deterministic claim, and the one that matters: the cooldown converts
	// "every unknown-kid request forces an outbound fetch" into "at most one
	// forced fetch per cooldown window," independent of flood size. This is
	// stable across repeated runs.
	got := fmt.Sprintf("base=%d vulnerable=%d fixed=%d", base.forcedFetches, vulnerable.forcedFetches, fixed.forcedFetches)
	want := fmt.Sprintf("base=0 vulnerable=%d fixed=1", pocFloodSize)
	if got != want {
		t.Errorf("forced-fetch counts: got %q, want %q", got, want)
	}
	if base.legitLatency > pocJWKSDelay {
		t.Errorf("base legit latency %v: master should not stall at all (cache is warm)", base.legitLatency)
	}

	// Latency is logged, not asserted on here: with sync.RWMutex, whether the
	// legit reader's RLock() queues behind one pending writer or several
	// depends on exact goroutine-scheduling order, which this harness's
	// concurrent launch does not pin down run to run. Repeated local runs
	// consistently showed vulnerable and fixed landing close together (~1
	// fetch's worth of wait) rather than fixed's 1-fetch case being 8x
	// cheaper than vulnerable's 8-fetch case — i.e. in THIS harness the
	// legit reader tends to queue behind only the currently-held lock, not
	// every pending one. The fix's real, load-bearing guarantee is the
	// fetch-count bound above: it caps outbound Cloudflare traffic and the
	// number of times the exclusive lock is taken at all under flood,
	// regardless of how any one run's queueing happens to land.
}
