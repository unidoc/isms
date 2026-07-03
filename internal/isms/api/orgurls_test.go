package api

import (
	"testing"

	"isms.sh/internal/isms/db"
)

func ptr(s string) *string { return &s }

// orgURLs must build links into the tenant's own space — never a flat shared
// base — matching the SPA router: org-scoped pages carry the slug/subdomain,
// public pages (verify-email, login) are never mounted under /:org. Subdomain
// URLs are used ONLY when subdomainRouting is enabled — otherwise links stay
// path-based, matching how requests are actually routed.
func TestOrgURLs(t *testing.T) {
	cases := []struct {
		name             string
		base             string
		org              *db.Organization
		subdomainRouting bool
		wantApp          string
		wantPublic       string
	}{
		{
			name:             "subdomain routing on: org becomes the subdomain for both",
			base:             "https://isms.sh",
			org:              &db.Organization{Slug: "sts"},
			subdomainRouting: true,
			wantApp:          "https://sts.isms.sh",
			wantPublic:       "https://sts.isms.sh",
		},
		{
			name:             "subdomain routing OFF: a real domain stays path-based (single-tenant box)",
			base:             "https://isms.stsplatform.com",
			org:              &db.Organization{Slug: "sts"},
			subdomainRouting: false,
			wantApp:          "https://isms.stsplatform.com/sts",
			wantPublic:       "https://isms.stsplatform.com",
		},
		{
			name:             "path-based host stays path-based regardless",
			base:             "http://localhost:9090",
			org:              &db.Organization{Slug: "sts"},
			subdomainRouting: true,
			wantApp:          "http://localhost:9090/sts",
			wantPublic:       "http://localhost:9090",
		},
		{
			name:             "custom domain wins for both, independent of routing mode",
			base:             "https://isms.sh",
			org:              &db.Organization{Slug: "sts", Domain: ptr("audit.sts.is")},
			subdomainRouting: false,
			wantApp:          "https://audit.sts.is",
			wantPublic:       "https://audit.sts.is",
		},
		{
			name:             "custom domain with explicit scheme is preserved",
			base:             "https://isms.sh",
			org:              &db.Organization{Slug: "sts", Domain: ptr("https://audit.sts.is/")},
			subdomainRouting: true,
			wantApp:          "https://audit.sts.is",
			wantPublic:       "https://audit.sts.is",
		},
		{
			name:             "www is stripped from the apex (subdomain routing on)",
			base:             "https://www.isms.sh",
			org:              &db.Organization{Slug: "sts"},
			subdomainRouting: true,
			wantApp:          "https://sts.isms.sh",
			wantPublic:       "https://sts.isms.sh",
		},
		{
			name:             "port is preserved on the subdomain (subdomain routing on)",
			base:             "https://isms.sh:8443",
			org:              &db.Organization{Slug: "sts"},
			subdomainRouting: true,
			wantApp:          "https://sts.isms.sh:8443",
			wantPublic:       "https://sts.isms.sh:8443",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, public := orgURLs(tc.base, tc.org, tc.subdomainRouting)
			if app != tc.wantApp {
				t.Errorf("app = %q, want %q", app, tc.wantApp)
			}
			if public != tc.wantPublic {
				t.Errorf("public = %q, want %q", public, tc.wantPublic)
			}
		})
	}
}

// orgTokenRedirectURL (post-OIDC login redirect) must make the SAME host
// decision as orgURLs — custom domain > subdomain (only when routing on) > path
// — so a user logging in via OIDC lands on the same host their notification/
// email links point to. It carried the identical subdomainRouting gate as
// orgURLs but had no test; this locks the precedence in CI, including the
// custom-domain case that was previously ignored (#108 review).
func TestOrgTokenRedirectURL(t *testing.T) {
	cases := []struct {
		name             string
		base             string
		slug             string
		domain           *string
		subdomainRouting bool
		want             string
	}{
		{
			name:             "subdomain routing on: hop to the tenant subdomain",
			base:             "https://isms.sh",
			slug:             "sts",
			subdomainRouting: true,
			want:             "https://sts.isms.sh/login#token=TOK&role=reader",
		},
		{
			name:             "subdomain routing OFF on a real domain: stay path-based",
			base:             "https://isms.stsplatform.com",
			slug:             "sts",
			subdomainRouting: false,
			want:             "https://isms.stsplatform.com/sts/login#token=TOK&role=reader",
		},
		{
			name:             "localhost stays path-based regardless of routing flag",
			base:             "http://localhost:9090",
			slug:             "sts",
			subdomainRouting: true,
			want:             "http://localhost:9090/sts/login#token=TOK&role=reader",
		},
		{
			name:             "custom domain wins regardless of routing mode",
			base:             "https://isms.sh",
			slug:             "sts",
			domain:           ptr("audit.sts.is"),
			subdomainRouting: true,
			want:             "https://audit.sts.is/login#token=TOK&role=reader",
		},
		{
			name:             "custom domain with explicit scheme is preserved",
			base:             "https://isms.sh",
			slug:             "sts",
			domain:           ptr("https://audit.sts.is/"),
			subdomainRouting: false,
			want:             "https://audit.sts.is/login#token=TOK&role=reader",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := orgTokenRedirectURL(tc.base, tc.slug, tc.domain, "TOK", "reader", tc.subdomainRouting)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
