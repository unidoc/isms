package i18n

import "testing"

func TestCanonical(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"exact", "pt-BR", "pt-BR", true},
		{"exact default", "en", "en", true},
		{"case insensitive", "PT-br", "pt-BR", true},
		{"underscore separator", "pt_BR", "pt-BR", true},
		{"bare language", "pt", "pt-BR", true},
		{"unsupported region falls back to same language", "pt-PT", "pt-BR", true},
		{"region we do not distinguish", "en-US", "en", true},
		{"unknown language", "ja", "", false},
		// Malformed input must be rejected, not reduced to its primary subtag.
		// Before BCP 47 shape validation every one of these resolved to "pt-BR",
		// because only the text before the first "-" was ever examined — which
		// silently defeated the 400 on both locale write paths.
		{"trailing numeric subtag", "pt_BR_2", "", false},
		{"trailing numeric subtag hyphenated", "pt-BR-2", "", false},
		{"punctuation subtag", "pt-!!!", "", false},
		{"private-use junk", "pt-Latn-BR-x-junk", "", false},
		{"too many subtags", "pt-BR-a-b-c-d-e", "", false},
		{"subtag over 8 chars", "pt-BRAZILIAN1", "", false},
		{"empty trailing subtag", "pt-", "", false},
		{"empty leading subtag", "-BR", "", false},
		{"digits in primary subtag", "p1-BR", "", false},
		{"reserved 4-alpha primary", "qaaa-BR", "", false},
		{"single-char primary", "p", "", false},
		{"separator only", "-", "", false},
		{"unknown with region", "ja-JP", "", false},
		{"empty", "", "", false},
		{"whitespace only", "   ", "", false},
		{"padded", "  pt-BR  ", "pt-BR", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Canonical(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Errorf("Canonical(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name string
		user string
		org  string
		want string
	}{
		{"user choice wins", "pt-BR", "en", "pt-BR"},
		{"org default when user has none", "", "pt-BR", "pt-BR"},
		{"fallback when neither is set", "", "", Default},
		{"unsupported user locale falls through to org", "ja", "pt-BR", "pt-BR"},
		{"unsupported at every tier falls back", "ja", "de", Default},
		{"user choice normalized", "PT_br", "", "pt-BR"},
		{"org default normalized", "", "pt", "pt-BR"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.user, tc.org); got != tc.want {
				t.Errorf("Resolve(%q, %q) = %q, want %q", tc.user, tc.org, got, tc.want)
			}
		})
	}
}

func TestFromAcceptLanguage(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"single supported", "pt-BR", "pt-BR", true},
		{"quality ordering overrides header order", "en;q=0.5, pt-BR;q=0.9", "pt-BR", true},
		{"implicit q=1 beats explicit lower q", "pt-BR, en;q=0.9", "pt-BR", true},
		{"first supported entry wins when q is equal", "pt-BR, en", "pt-BR", true},
		{"skips unsupported and takes next", "ja, de, pt-BR", "pt-BR", true},
		{"q=0 means not acceptable", "pt-BR;q=0, en", "en", true},
		{"wildcard alone tells us nothing", "*", "", false},
		{"wildcard is skipped, real entry used", "*, pt-BR", "pt-BR", true},
		{"bare language matches a region", "pt", "pt-BR", true},
		{"empty header", "", "", false},
		{"nothing supported", "ja-JP, ko", "", false},
		{"real-world Chrome header", "pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7", "pt-BR", true},
		{"whitespace tolerant", " en ; q=0.2 ,  pt-BR ; q=0.8 ", "pt-BR", true},
		{"malformed q loses to well-formed", "pt-BR;q=abc, en;q=0.1", "en", true},
		// Parameter names are case-insensitive (RFC 9110). A literal "q=" search
		// misses these, leaving q at 1.0 — so an explicitly refused locale would
		// beat an accepted one.
		{"uppercase Q=0 is still a refusal", "pt-BR;Q=0, en", "en", true},
		{"uppercase Q respected in ordering", "pt-BR;Q=0.1, en;Q=0.9", "en", true},
		{"mixed case q parameter", "pt-BR;Q=0.9, en;q=0.1", "pt-BR", true},
		{"padded uppercase Q", "pt-BR; Q =0", "", false},
		{"q above range is malformed", "pt-BR;q=9, en;q=0.1", "en", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := FromAcceptLanguage(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Errorf("FromAcceptLanguage(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// Default must be selectable, or every resolution path ends at a locale the
// client has no messages for.
func TestDefaultIsSupported(t *testing.T) {
	if !IsSupported(Default) {
		t.Fatalf("Default %q is not in the supported set", Default)
	}
}

// Supported() feeds a UI picker and an API response, so its order must be
// stable and must lead with the fallback.
func TestSupportedOrderingIsStable(t *testing.T) {
	first := Supported()
	if len(first) == 0 {
		t.Fatal("Supported() returned nothing")
	}
	if first[0].Tag != Default {
		t.Errorf("Supported()[0].Tag = %q, want the default %q first", first[0].Tag, Default)
	}
	for i := 0; i < 20; i++ {
		next := Supported()
		if len(next) != len(first) {
			t.Fatalf("Supported() length varies: %d then %d", len(first), len(next))
		}
		for j := range first {
			if first[j] != next[j] {
				t.Fatalf("Supported() order varies at %d: %+v then %+v", j, first[j], next[j])
			}
		}
	}
}

// Every locale needs an endonym, since that is what a picker displays.
func TestEveryLocaleHasAName(t *testing.T) {
	for _, l := range Supported() {
		if l.Name == "" {
			t.Errorf("locale %q has no display name", l.Tag)
		}
	}
}
