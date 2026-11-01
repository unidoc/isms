package api

import "testing"

func TestOrgLoginErrorURL(t *testing.T) {
	custom := "audit.example.com"
	cases := []struct {
		name, base, slug string
		domain           *string
		sub              bool
		want             string
	}{
		{"path based", "http://localhost:9999", "acme", nil, false, "http://localhost:9999/acme/login?sso_error=account_exists"},
		{"subdomain", "https://isms.sh", "acme", nil, true, "https://acme.isms.sh/login?sso_error=account_exists"},
		{"custom domain", "https://isms.sh", "acme", &custom, true, "https://audit.example.com/login?sso_error=account_exists"},
	}
	for _, c := range cases {
		if got := orgLoginErrorURL(c.base, c.slug, c.domain, "account_exists", c.sub); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
