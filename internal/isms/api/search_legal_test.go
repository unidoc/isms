package api

import (
	"strings"
	"testing"

	"isms.sh/internal/isms/db"
)

// The regression this guards: `jurisdiction` used to hold English display text,
// so folding the raw column into the index made "Iceland" find the row. The
// column holds `IS` now. Without the names alongside it, every converted row
// silently stopped being findable by the only name its reader has ever seen — a
// search that returns nothing looks like an empty register, not like a bug.
func TestLegalSearchTextIncludesTheRegionName(t *testing.T) {
	got := legalSearchText(&db.LegalRequirement{
		Identifier:   "LR-1",
		Title:        "Data protection",
		Description:  "Applies to processing",
		Jurisdiction: "IS",
	})
	for _, want := range []string{"lr-1", "data protection", "applies to processing", "is", "iceland"} {
		if !strings.Contains(got, want) {
			t.Errorf("search text %q is missing %q", got, want)
		}
	}
	if got != strings.ToLower(got) {
		t.Errorf("search text must be lowercased for the index: %q", got)
	}
}

// Every name the OLD picker offered has to stay searchable, or the conversion
// silently breaks saved habits: someone who has always found the row by typing
// "Czech Republic" gets nothing, because CLDR renamed it "Czechia". 16 of the
// 197 names drifted this way ("and" became "&", "East Timor" became
// "Timor-Leste"), and they are the entire reason the data file carries a frozen
// `legacy` name next to the current label.
//
// 16 is the count against the browser's table, which is the one that matters
// because that is the label a reader sees; `regionCodes.test.js` asserts it.
// Counting against x/text gives 15 — a different CLDR snapshot — and that
// mismatch is why the label is generated and embedded rather than looked up
// here.
func TestLegacyRegionNamesStaySearchable(t *testing.T) {
	cases := map[string][]string{
		"CZ": {"czech republic", "czechia"},
		"TL": {"east timor", "timor-leste"},
		"CI": {"ivory coast", "côte d’ivoire"},
		"AG": {"antigua and barbuda", "antigua & barbuda"},
		"SZ": {"eswatini"},
		"MK": {"north macedonia"},
	}
	for code, wants := range cases {
		got := legalSearchText(&db.LegalRequirement{Jurisdiction: code})
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("%s: search text %q is missing %q", code, got, want)
			}
		}
	}
	// SZ and MK are the reason this package does not ask x/text for the label:
	// its CLDR snapshot says "Swaziland" and "Macedonia" where the browser says
	// "Eswatini" and "North Macedonia". Indexing x/text's answer would make the
	// row findable under a name no reader can see.
	//
	// Only SZ can be asserted. "macedonia" is a substring of the name the UI
	// does display, so its absence is not something a substring index can
	// express — checking it would assert nothing. "Swaziland" shares no prefix
	// with "Eswatini" and is a real discriminator.
	got := legalSearchText(&db.LegalRequirement{Jurisdiction: "SZ"})
	if strings.Contains(got, "swaziland") {
		t.Errorf("SZ: index contains \"swaziland\", which the UI never displays")
	}
}

func TestRegionSearchNamesGatesOnTheOfferedSet(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  []string
	}{
		{"offered code", "IS", []string{"Iceland"}},
		{"offered code with drift", "CZ", []string{"Czechia", "Czech Republic"}},
		// Sentinels are skipped deliberately. CLDR renders EU as "European
		// Union", a name the UI never shows — the picker and common.region.eu
		// both call it "EU" — so indexing it would add a term no reader can see
		// on screen.
		{"EU sentinel is not a country", "EU", nil},
		{"Global sentinel", "Global", nil},
		{"EEA sentinel", "EEA", nil},
		{"APAC sentinel", "APAC", nil},
		// The column is free text and has no CHECK: the CLI and agent
		// suggestions write anything, and the migration matched exactly, so
		// unconverted legacy names survive. Each of these must leave the raw
		// value as the only search term rather than resolving.
		{"legacy English name", "Iceland", nil},
		{"free text", "Germany/France", nil},
		{"lowercase is not a code we write", "is", nil},
		{"empty", "", nil},
		// Codes CLDR answers for that this picker never offered. Gating on the
		// offered set rather than on "two uppercase letters" is what excludes
		// them — and the pseudo-regions are why it matters: none of these is a
		// place, and "Pseudo-Accents" in a jurisdiction field is nonsense.
		{"unknown region", "ZZ", nil},
		{"pseudo-accents", "XA", nil},
		{"eurozone", "EZ", nil},
		{"united nations", "UN", nil},
		{"outlying oceania", "QO", nil},
		// A real ISO code the picker does not offer. Rendered and indexed as
		// typed, which is the free-text contract: we have no basis to claim the
		// user meant the region when we never offered it.
		{"real code outside the offered set", "HK", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := regionSearchNames(tc.value)
			if len(got) != len(tc.want) {
				t.Fatalf("regionSearchNames(%q) = %v, want %v", tc.value, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("regionSearchNames(%q)[%d] = %q, want %q", tc.value, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// The embed is parsed once at package init and panics on a malformed or empty
// file, so a broken data file fails loudly at startup rather than quietly
// de-indexing every jurisdiction. This asserts the parse actually produced the
// whole set.
func TestRegionDataLoaded(t *testing.T) {
	if len(regionSearchTerms) != 197 {
		t.Fatalf("loaded %d region entries, want 197", len(regionSearchTerms))
	}
	for _, sentinel := range []string{"Global", "EU", "EEA", "APAC"} {
		if _, ok := regionSearchTerms[sentinel]; ok {
			t.Errorf("sentinel %q must not be in the region table", sentinel)
		}
	}
}
