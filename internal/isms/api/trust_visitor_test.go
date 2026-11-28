package api

// Tests for the trust center visitor sign-in pieces that need no database:
// the link URL builder and the per-IP limiter on link requests.

import (
	"testing"

	"isms.sh/internal/isms/db"
)

func TestTrustSignInLinkIsBuiltFromServerKnownValues(t *testing.T) {
	custom := "trust.acme.test"
	withDomain := &db.Organization{Slug: "acme", Domain: &custom}
	noDomain := &db.Organization{Slug: "acme"}
	httpDomain := "http://trust.acme.test"
	trustDom := "trust.a.test"
	withTrust := &db.Organization{Slug: "acme", TrustDomain: &trustDom}
	const tok = "abc123"

	cases := []struct {
		name    string
		base    string
		org     *db.Organization
		subdom  bool
		reqHost string
		want    string
	}{
		{"path mode", "https://isms.example", noDomain, false, "isms.example", "https://isms.example/acme/trust/sign-in#visitor_token=abc123"},
		{"path mode keeps base port", "http://localhost:9192", noDomain, false, "localhost:9192", "http://localhost:9192/acme/trust/sign-in#visitor_token=abc123"},
		{"subdomain mode on the org's subdomain", "https://isms.example", noDomain, true, "acme.isms.example", "https://acme.isms.example/trust/sign-in#visitor_token=abc123"},
		{"subdomain host case and port ignored", "https://isms.example:8443", noDomain, true, "ACME.isms.example:1", "https://acme.isms.example:8443/trust/sign-in#visitor_token=abc123"},
		{"subdomain mode but request on the apex", "https://isms.example", noDomain, true, "isms.example", "https://isms.example/acme/trust/sign-in#visitor_token=abc123"},
		{"another tenant's subdomain", "https://isms.example", noDomain, true, "other.isms.example", "https://isms.example/acme/trust/sign-in#visitor_token=abc123"},
		{"subdomain host with routing off", "https://isms.example", noDomain, false, "acme.isms.example", "https://isms.example/acme/trust/sign-in#visitor_token=abc123"},
		{"custom domain", "https://isms.example", withDomain, true, "trust.acme.test", "https://trust.acme.test/trust/sign-in#visitor_token=abc123"},
		{"custom domain does not take the base port", "https://isms.example.com:8443", withDomain, true, "trust.acme.test", "https://trust.acme.test/trust/sign-in#visitor_token=abc123"},
		{"custom domain stored with a scheme keeps it", "https://isms.example", &db.Organization{Slug: "acme", Domain: &httpDomain}, true, "trust.acme.test", "http://trust.acme.test/trust/sign-in#visitor_token=abc123"},
		{"custom domain, request on the base host", "https://isms.example", withDomain, false, "isms.example", "https://isms.example/acme/trust/sign-in#visitor_token=abc123"},
		{"trust domain", "https://isms.example", withTrust, true, "trust.a.test", "https://trust.a.test/sign-in#visitor_token=abc123"},
		{"trust domain, dotted host with port", "https://isms.example", withTrust, true, "TRUST.a.test.:443", "https://trust.a.test/sign-in#visitor_token=abc123"},
		{"trust domain, dotted host", "https://isms.example", withTrust, false, "trust.a.test.", "https://trust.a.test/sign-in#visitor_token=abc123"},
		{"forged host for a trust domain org", "https://isms.example", withTrust, true, "evil.test", "https://isms.example/acme/trust/sign-in#visitor_token=abc123"},
		{"forged host", "https://isms.example", noDomain, true, "evil.example", "https://isms.example/acme/trust/sign-in#visitor_token=abc123"},
		{"forged host with port", "https://isms.example", noDomain, false, "evil.example:8080", "https://isms.example/acme/trust/sign-in#visitor_token=abc123"},
		{"forged subdomain of another apex", "https://isms.example", noDomain, true, "acme.evil.example", "https://isms.example/acme/trust/sign-in#visitor_token=abc123"},
		{"forged host with userinfo", "https://isms.example", noDomain, true, "acme.isms.example@evil.example", "https://isms.example/acme/trust/sign-in#visitor_token=abc123"},
		{"trailing slash in base", "https://isms.example/", noDomain, false, "evil.example", "https://isms.example/acme/trust/sign-in#visitor_token=abc123"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := trustSignInLink(c.base, c.org, c.subdom, c.reqHost, tok)
			if got != c.want {
				t.Errorf("link = %q, want %q", got, c.want)
			}
		})
	}
}

func TestTrustLinkLimiterBlocksTheEleventhRequest(t *testing.T) {
	t.Setenv("ISMS_RATE_LIMIT", "")
	l := newTrustLinkLimiter()
	for i := 1; i <= 10; i++ {
		if !l.allow("198.51.100.1") {
			t.Fatalf("request %d was refused, want allowed", i)
		}
	}
	if l.allow("198.51.100.1") {
		t.Error("request 11 was allowed, want refused")
	}
	if !l.allow("198.51.100.2") {
		t.Error("a different IP shares the budget")
	}
}

func TestTrustLinkLimiterOffWhenRateLimitDisabled(t *testing.T) {
	t.Setenv("ISMS_RATE_LIMIT", "0")
	l := newTrustLinkLimiter()
	for i := 0; i < 50; i++ {
		if !l.allow("198.51.100.1") {
			t.Fatalf("request %d refused with ISMS_RATE_LIMIT=0", i+1)
		}
	}
}

func TestTrustSignInOrgHourlyCapDefault(t *testing.T) {
	t.Setenv("ISMS_TRUST_SIGNIN_ORG_HOURLY", "")
	if got := trustSignInOrgHourly(); got != 500 {
		t.Errorf("default = %d, want 500", got)
	}
	t.Setenv("ISMS_TRUST_SIGNIN_ORG_HOURLY", "7")
	if got := trustSignInOrgHourly(); got != 7 {
		t.Errorf("override = %d, want 7", got)
	}
	t.Setenv("ISMS_TRUST_SIGNIN_ORG_HOURLY", "junk")
	if got := trustSignInOrgHourly(); got != 500 {
		t.Errorf("invalid = %d, want the default", got)
	}
}
