package api

import (
	"testing"

	"isms.sh/internal/isms/db"
)

func ptr(s string) *string { return &s }

// orgURLs must build links into the tenant's own space — never a flat shared
// base — matching the SPA router: org-scoped pages carry the slug/subdomain,
// public pages (verify-email, login) are never mounted under /:org.
func TestOrgURLs(t *testing.T) {
	cases := []struct {
		name           string
		base           string
		org            *db.Organization
		wantApp        string
		wantPublic     string
	}{
		{
			name:       "subdomain host: org becomes the subdomain for both",
			base:       "https://isms.sh",
			org:        &db.Organization{Slug: "sts"},
			wantApp:    "https://sts.isms.sh",
			wantPublic: "https://sts.isms.sh",
		},
		{
			name:       "path-based host: app carries the slug, public does not",
			base:       "http://localhost:9090",
			org:        &db.Organization{Slug: "sts"},
			wantApp:    "http://localhost:9090/sts",
			wantPublic: "http://localhost:9090",
		},
		{
			name:       "custom domain wins for both app and public",
			base:       "https://isms.sh",
			org:        &db.Organization{Slug: "sts", Domain: ptr("audit.sts.is")},
			wantApp:    "https://audit.sts.is",
			wantPublic: "https://audit.sts.is",
		},
		{
			name:       "custom domain with explicit scheme is preserved",
			base:       "https://isms.sh",
			org:        &db.Organization{Slug: "sts", Domain: ptr("https://audit.sts.is/")},
			wantApp:    "https://audit.sts.is",
			wantPublic: "https://audit.sts.is",
		},
		{
			name:       "www is stripped from the apex",
			base:       "https://www.isms.sh",
			org:        &db.Organization{Slug: "sts"},
			wantApp:    "https://sts.isms.sh",
			wantPublic: "https://sts.isms.sh",
		},
		{
			name:       "port is preserved on the subdomain",
			base:       "https://isms.sh:8443",
			org:        &db.Organization{Slug: "sts"},
			wantApp:    "https://sts.isms.sh:8443",
			wantPublic: "https://sts.isms.sh:8443",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, public := orgURLs(tc.base, tc.org)
			if app != tc.wantApp {
				t.Errorf("app = %q, want %q", app, tc.wantApp)
			}
			if public != tc.wantPublic {
				t.Errorf("public = %q, want %q", public, tc.wantPublic)
			}
		})
	}
}
