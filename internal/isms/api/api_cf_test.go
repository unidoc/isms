package api

import "testing"

func TestDeriveNameFromEmail(t *testing.T) {
	cases := map[string]string{
		"ari.bjarna@acme.com": "ari.bjarna",
		"jon@example.org":     "jon",
		"weird-no-at":         "weird-no-at",
		"@leading":            "@leading", // no local part — fall back to the raw input
	}
	for in, want := range cases {
		if got := deriveNameFromEmail(in); got != want {
			t.Errorf("deriveNameFromEmail(%q) = %q, want %q", in, got, want)
		}
	}
}
