package api

import (
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"isms.sh/internal/isms/db"
)

// legalSearchText builds the searchable blob for a legal requirement.
//
// It exists because `jurisdiction` stopped being English prose. The column used
// to hold the picker's display string ("Iceland"), and three call sites folded it
// straight into the index, so searching "Iceland" found the row. It now holds an
// ISO 3166-1 alpha-2 code, and "IS" is not a term anyone types — which would
// have made every converted row unfindable by the only name its reader has ever
// seen for it. The English region name is appended alongside the code so both
// work.
//
// English specifically, not the searcher's locale. The index is one denormalised
// row per entity, written once at mutation time by whoever happened to save it,
// and read by every member of the org; there is no locale in scope at write time
// that would be right for all readers. English is the language the column held
// until this release, so appending it keeps every query that worked before this
// change working after it. Indexing the reader's own language means rebuilding
// the index per locale, which is a different piece of work.
//
// Three call sites shared this concatenation by copy-paste (create, update, and
// the full rebuild in api_search.go); they are collapsed here so the next change
// cannot land in two of them.
func legalSearchText(lr *db.LegalRequirement) string {
	parts := []string{lr.Identifier, lr.Title, lr.Description, lr.Jurisdiction}
	if name := englishRegionName(lr.Jurisdiction); name != "" {
		parts = append(parts, name)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// regionSentinels are the jurisdiction values that are deliberately NOT ISO
// 3166. `EU` is the one that matters: it is a real ISO exceptional reservation,
// so x/text renders it "European Union" — a name the UI never shows, because the
// picker and `common.region.eu` call it "EU". Skipping them keeps the index to
// terms a reader can actually see on screen.
var regionSentinels = map[string]bool{"Global": true, "EU": true, "EEA": true, "APAC": true}

// englishRegionName returns the English name for an alpha-2 region code, or ""
// for anything that is not one — a sentinel, a legacy English name the migration
// did not convert, free text from the CLI or an agent suggestion. In each of
// those cases the raw value is already in the blob and is the best term
// available.
func englishRegionName(value string) string {
	if value == "" || regionSentinels[value] {
		return ""
	}
	// Uppercase two letters, checked before parsing. `language.ParseRegion` is
	// case-insensitive and happily reads "is" as Iceland, but the picker only
	// ever writes uppercase, so a lowercase pair is free text somebody typed.
	// This has to match the `/^[A-Z]{2}$/` guard in regionLabel() exactly: if
	// the index resolved a value the UI renders verbatim, a row would be
	// findable under a name it never displays.
	if !isAlpha2(value) {
		return ""
	}
	region, err := language.ParseRegion(value)
	if err != nil {
		return ""
	}
	name := display.English.Regions().Name(region)
	// x/text answers "Unknown Region" for a well-formed code it has no name for
	// (`ZZ`), which would index the same noise phrase against every such row and
	// make them all match each other.
	if name == "" || name == "Unknown Region" {
		return ""
	}
	return name
}

func isAlpha2(value string) bool {
	if len(value) != 2 {
		return false
	}
	for i := 0; i < 2; i++ {
		if value[i] < 'A' || value[i] > 'Z' {
			return false
		}
	}
	return true
}
