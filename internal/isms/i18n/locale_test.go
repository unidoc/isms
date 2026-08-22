package i18n

import "testing"

func TestCanonical(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"exact", "id-ID", "id-ID", true},
		{"exact default", "en", "en", true},
		{"case insensitive", "ID-id", "id-ID", true},
		{"underscore separator", "id_ID", "id-ID", true},
		{"bare language", "id", "id-ID", true},
		{"unsupported region falls back to same language", "id-SG", "id-ID", true},
		{"region we do not distinguish", "en-US", "en", true},
		{"unknown language", "ja", "", false},
		// Malformed input must be rejected, not reduced to its primary subtag.
		// Before validation every one of these resolved to "id-ID", because only
		// the text before the first "-" was ever examined — which silently
		// defeated the 400 on both locale write paths.
		//
		// Note the fix has two halves: a BCP 47 shape check rejects the first
		// three, but "id-Latn-ID-x-junk" is a well-formed tag and is rejected only
		// because the fallback is restricted to simple tags (language + at most
		// one region). Keep both kinds of case here — dropping the well-formed
		// ones would let a regression in the fallback restriction pass unnoticed.
		{"trailing numeric subtag", "id_ID_2", "", false},
		{"trailing numeric subtag hyphenated", "id-ID-2", "", false},
		{"punctuation subtag", "id-!!!", "", false},
		{"well-formed but over-specified", "id-Latn-ID-x-junk", "", false},
		{"too many subtags", "id-ID-a-b-c-d-e", "", false},
		{"subtag over 8 chars", "id-INDONESIA1", "", false},
		{"empty trailing subtag", "id-", "", false},
		{"empty leading subtag", "-ID", "", false},
		{"digits in primary subtag", "i1-ID", "", false},
		{"reserved 4-alpha primary", "qaaa-ID", "", false},
		{"single-char primary", "i", "", false},
		{"separator only", "-", "", false},
		{"unknown with region", "ja-JP", "", false},
		{"empty", "", "", false},
		{"whitespace only", "   ", "", false},
		{"padded", "  id-ID  ", "id-ID", true},
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
		{"user choice wins", "id-ID", "en", "id-ID"},
		{"org default when user has none", "", "id-ID", "id-ID"},
		{"fallback when neither is set", "", "", Default},
		{"unsupported user locale falls through to org", "ja", "id-ID", "id-ID"},
		{"unsupported at every tier falls back", "ja", "de", Default},
		{"malformed user locale falls through to org", "id-ID-2", "id-ID", "id-ID"},
		{"user choice normalized", "ID_id", "", "id-ID"},
		{"org default normalized", "", "id", "id-ID"},
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
		{"single supported", "id-ID", "id-ID", true},
		{"quality ordering overrides header order", "en;q=0.5, id-ID;q=0.9", "id-ID", true},
		{"implicit q=1 beats explicit lower q", "id-ID, en;q=0.9", "id-ID", true},
		{"first supported entry wins when q is equal", "id-ID, en", "id-ID", true},
		{"skips unsupported and takes next", "ja, de, id-ID", "id-ID", true},
		{"q=0 means not acceptable", "id-ID;q=0, en", "en", true},
		{"wildcard alone tells us nothing", "*", "", false},
		{"wildcard is skipped, real entry used", "*, id-ID", "id-ID", true},
		{"bare language matches a region", "id", "id-ID", true},
		{"empty header", "", "", false},
		{"nothing supported", "ja-JP, ko", "", false},
		{"real-world Chrome header", "id-ID,id;q=0.9,en-US;q=0.8,en;q=0.7", "id-ID", true},
		{"whitespace tolerant", " en ; q=0.2 ,  id-ID ; q=0.8 ", "id-ID", true},
		{"malformed q loses to well-formed", "id-ID;q=abc, en;q=0.1", "en", true},
		// Parameter names are case-insensitive (RFC 9110). A literal "q=" search
		// misses these, leaving q at 1.0 — so an explicitly refused locale would
		// beat an accepted one.
		{"uppercase Q=0 is still a refusal", "id-ID;Q=0, en", "en", true},
		{"uppercase Q respected in ordering", "id-ID;Q=0.1, en;Q=0.9", "en", true},
		{"mixed case q parameter", "id-ID;Q=0.9, en;q=0.1", "id-ID", true},
		{"padded uppercase Q", "id-ID; Q =0", "", false},
		{"q above range is malformed", "id-ID;q=9, en;q=0.1", "en", true},
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

// Every supported tag must survive its own canonicalization. This is the guard
// against adding a locale in a form the matcher cannot return — e.g. a key with
// an underscore, or one with enough subtags that the simple-tag restriction
// rejects it. Without this, a new entry could be unreachable via every write
// path while still appearing in the picker.
func TestEverySupportedTagIsCanonical(t *testing.T) {
	for _, l := range Supported() {
		got, ok := Canonical(l.Tag)
		if !ok || got != l.Tag {
			t.Errorf("Canonical(%q) = (%q, %v), want (%q, true) — supported tag is not self-canonical", l.Tag, got, ok, l.Tag)
		}
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
