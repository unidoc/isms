package db

import (
	"slices"
	"testing"
)

func TestNDASkipMatches(t *testing.T) {
	cases := []struct {
		list  []string
		email string
		want  bool
	}{
		{[]string{"example.com"}, "a@example.com", true},
		{[]string{"example.com"}, "A@Example.COM", true},
		{[]string{"example.com"}, "a@sub.example.com", false},
		{[]string{"example.com"}, "a@notexample.com", false},
		{[]string{"example.com"}, "a@example.com.evil.test", false},
		{[]string{"*.example.com"}, "a@sub.example.com", true},
		{[]string{"*.example.com"}, "a@x.sub.example.com", true},
		{[]string{"*.example.com"}, "a@example.com", false},
		{[]string{"*.example.com"}, "a@notexample.com", false},
		{[]string{"*.example.com"}, "a@evilexample.com", false},
		{[]string{"*.example.com"}, "a@.example.com", false},
		{[]string{"example.com", "*.example.com"}, "a@example.com", true},
		{[]string{"example.com", "*.example.com"}, "a@x.example.com", true},
		{nil, "a@example.com", false},
		{[]string{"example.com"}, "no-at-sign", false},
		{[]string{"example.com"}, "", false},
	}
	for _, c := range cases {
		if got := NDASkipMatches(c.list, c.email); got != c.want {
			t.Errorf("NDASkipMatches(%v, %q) = %v, want %v", c.list, c.email, got, c.want)
		}
	}
}

func TestNormalizeNDASkipDomains(t *testing.T) {
	got, err := NormalizeNDASkipDomains([]string{" Example.COM ", "@b.example.org", "*.Example.com", "example.com", "*.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"*.example.com", "b.example.org", "example.com"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// A wildcard under a public suffix is fine; the suffix itself is refused.
	ok, err := NormalizeNDASkipDomains([]string{"*.example.co.uk", "*.acme.com.au", "co.uk"})
	if err != nil || !slices.Equal(ok, []string{"*.acme.com.au", "*.example.co.uk", "co.uk"}) {
		t.Errorf("wildcards under a public suffix = %v, %v", ok, err)
	}
	if got, err := NormalizeNDASkipDomains(nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("nil input = %#v, %v; want an empty non-nil list", got, err)
	}
	for _, bad := range []string{"*.co.uk", "*.com.au", "*.github.io", "*.gov.uk", "*.CO.UK", "*.blogspot.com", "*.", "", "  ", "localhost", "*.com", "*.", "*", "a.*.com", "ex*.example.com", "a b.com", "a@b.com", "a/b.com", ".example.com", "example.com.", "a..com", "**.example.com"} {
		if _, err := NormalizeNDASkipDomains([]string{bad}); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	long := make([]string, maxNDASkipDomains+1)
	for i := range long {
		long[i] = "d.example.com"
	}
	if _, err := NormalizeNDASkipDomains(long); err == nil {
		t.Errorf("more than %d entries were accepted", maxNDASkipDomains)
	}
}
