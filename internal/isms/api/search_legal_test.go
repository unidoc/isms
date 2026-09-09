package api

import (
	"strings"
	"testing"

	"isms.sh/internal/isms/db"
)

// The regression this guards: `jurisdiction` used to hold English display text,
// so folding the raw column into the search index made "Iceland" find the row.
// The column holds `IS` now. Without the English name alongside it, every
// converted row silently stopped being findable by the only name its reader has
// ever seen — a search that returns nothing looks like an empty register, not
// like a bug.
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

func TestEnglishRegionName(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"alpha-2 code", "IS", "Iceland"},
		{"another code", "GB", "United Kingdom"},
		// Sentinels are skipped deliberately. x/text renders EU as "European
		// Union", a name the UI never shows — the picker and common.region.eu
		// both call it "EU" — so indexing it would add a term no reader can see
		// on screen.
		{"EU sentinel is not a country", "EU", ""},
		{"Global sentinel", "Global", ""},
		{"EEA sentinel", "EEA", ""},
		{"APAC sentinel", "APAC", ""},
		// The column is free text and has no CHECK: the CLI and agent
		// suggestions write anything, and the migration matched exactly, so
		// unconverted legacy names survive. Each of these must leave the raw
		// value as the only search term rather than erroring.
		{"legacy English name", "Iceland", ""},
		{"free text", "Germany/France", ""},
		{"lowercase is not a code we write", "is", ""},
		{"empty", "", ""},
		// Well-formed but nameless. x/text answers "Unknown Region", which would
		// index the same phrase against every such row and make them all match
		// each other.
		{"unassigned code", "ZZ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := englishRegionName(tc.value); got != tc.want {
				t.Errorf("englishRegionName(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}
