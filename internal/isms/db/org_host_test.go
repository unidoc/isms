package db

import (
	"strings"
	"testing"
)

func TestNormalizeTrustDomain(t *testing.T) {
	long63 := strings.Repeat("a", 63)
	cases := []struct {
		name, raw, apex, want string
		wantErr               bool
	}{
		{"plain", "trust.example.com", "", "trust.example.com", false},
		{"upper and spaces", "  Trust.Example.COM ", "", "trust.example.com", false},
		{"one trailing dot", "trust.example.com.", "", "trust.example.com", false},
		{"punycode", "xn--bcher-kva.example.com", "", "xn--bcher-kva.example.com", false},
		{"digits and hyphens", "t-1.ex-ample.co.uk", "", "t-1.ex-ample.co.uk", false},
		{"63 char label", long63 + ".com", "", long63 + ".com", false},
		{"outside apex", "trust.company.com", "isms.sh", "trust.company.com", false},
		{"similar to apex", "notisms.sh", "isms.sh", "notisms.sh", false},
		{"empty", "", "", "", true},
		{"blank", "   ", "", "", true},
		{"only a dot", ".", "", "", true},
		{"scheme", "https://trust.example.com", "", "", true},
		{"port", "trust.example.com:8443", "", "", true},
		{"path", "trust.example.com/x", "", "", true},
		{"trailing slash", "trust.example.com/", "", "", true},
		{"query", "trust.example.com?a=1", "", "", true},
		{"fragment", "trust.example.com#a", "", "", true},
		{"wildcard", "*.example.com", "", "", true},
		{"inner whitespace", "trust .example.com", "", "", true},
		{"non-ascii", "trüst.example.com", "", "", true},
		{"ipv4", "10.0.0.1", "", "", true},
		{"ipv6", "::1", "", "", true},
		{"ipv6 brackets", "[::1]", "", "", true},
		{"localhost", "localhost", "", "", true},
		{"under localhost", "trust.localhost", "", "", true},
		{"single label", "trust", "", "", true},
		{"two dots", "trust.example.com..", "", "", true},
		{"empty label", "trust..example.com", "", "", true},
		{"leading dot", ".example.com", "", "", true},
		{"leading hyphen", "-a.example.com", "", "", true},
		{"trailing hyphen label", "a-.example.com", "", "", true},
		{"underscore", "a_b.example.com", "", "", true},
		{"64 char label", long63 + "a.example.com", "", "", true},
		{"over 253", strings.Repeat(long63[:50]+".", 5) + "com", "", "", true},
		{"equals apex", "isms.sh", "isms.sh", "", true},
		{"equals apex, case", "ISMS.sh", "isms.sh", "", true},
		{"www apex", "www.isms.sh", "isms.sh", "", true},
		{"under apex", "acme.isms.sh", "isms.sh", "", true},
		{"deep under apex", "a.b.isms.sh", "ISMS.sh", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NormalizeTrustDomain(c.raw, c.apex)
			if (err != nil) != c.wantErr {
				t.Fatalf("NormalizeTrustDomain(%q, %q) err = %v, wantErr %v", c.raw, c.apex, err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("NormalizeTrustDomain(%q, %q) = %q, want %q", c.raw, c.apex, got, c.want)
			}
		})
	}
}

func TestApexHost(t *testing.T) {
	cases := []struct{ domain, base, want string }{
		{"ISMS.sh", "https://other.example.com", "isms.sh"},
		{" isms.sh ", "", "isms.sh"},
		{"", "https://App.Example.com:8443/x", "app.example.com"},
		{"", "http://localhost:8080", "localhost"},
		{"", "", ""},
		{"", "   ", ""},
	}
	for _, c := range cases {
		if got := ApexHost(c.domain, c.base); got != c.want {
			t.Errorf("ApexHost(%q, %q) = %q, want %q", c.domain, c.base, got, c.want)
		}
	}
}
