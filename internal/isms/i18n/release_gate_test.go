package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// rawTextBaseline is the web track's extraction ratchet, read from the Go side
// because the decision it gates — which locales this build offers — lives here.
const rawTextBaseline = "../../../web/test/rawText.baseline.json"

// extractionGateThreshold is the total raw-text budget below which the UI counts
// as extracted for the purpose of offering a second language.
//
// It is not zero, and it should not be. The scanner over-reports — brand names,
// punctuation, and attribute values it cannot distinguish from copy — so a
// fully extracted tree still carries a residue, and 100 is an estimate of it.
//
// For scale: that is about 25x below where the tree sits today (2415) and about
// 25x below where it started (2488). Those two numbers are nearly identical,
// which is the honest summary of how far extraction has got. The estimate wants
// revisiting once a mostly-extracted tree shows what the residue actually is —
// the threshold is a judgement, and says so. The coupling it enforces is not.
const extractionGateThreshold = 100

// TestSecondLocaleRequiresExtraction is a release gate, not a unit test.
//
// A locale entry is one word away from `enabled: true`, and the reason it must
// stay false is nowhere near the flag: it lives in the *web* tree, in how much
// of the UI has been extracted through t(). Someone flipping it in six months
// sees a one-word change and a comment they have no way to verify.
//
// The failure mode is specific and bad. With only `en` enabled, a half-extracted
// UI is invisible: an extracted string and a hardcoded one render the same
// English to the same reader, so extraction can land incrementally on master at
// no user-visible cost. Enable a second locale before extraction finishes and
// that changes in one step — the picker offers a language, and the app answers
// in mostly English. That is worse than offering nothing, and it is worse for
// the contributor who translated the bundle than for anyone else.
//
// So the two are coupled here mechanically: a second enabled locale requires
// the extraction ratchet to have come down. Both halves of the condition are
// facts in the tree rather than a promise in a comment.
//
// To enable a locale: finish extraction, let the ratchet fall, then flip the
// flag. If the threshold itself is what needs revisiting, change it here in a
// commit that says why — which is the conversation this test exists to force.
func TestSecondLocaleRequiresExtraction(t *testing.T) {
	var enabled []string
	for tag, e := range supported {
		if e.enabled {
			enabled = append(enabled, tag)
		}
	}
	if len(enabled) <= 1 {
		return // Nothing to gate: this build offers one language.
	}

	path := filepath.Clean(rawTextBaseline)
	raw, err := os.ReadFile(path)
	if err != nil {
		// A missing baseline is a failure, not a skip. It is the evidence half
		// of the gate; without it the check would pass vacuously in exactly the
		// situation it exists to catch.
		t.Fatalf("raw-text baseline not found at %s: %v", path, err)
	}
	var budgets map[string]int
	if err := json.Unmarshal(raw, &budgets); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	total := 0
	for _, n := range budgets {
		total += n
	}

	if total > extractionGateThreshold {
		t.Errorf(
			"%d locales are enabled (%v) but the UI is not extracted: the raw-text baseline "+
				"totals %d across %d files, over the %d threshold.\n\n"+
				"Offering a language the app cannot actually speak is worse than offering none. "+
				"Finish extracting the remaining views (issue #212) so the ratchet falls, then "+
				"enable the locale.\n"+
				"If the threshold is what is wrong, change extractionGateThreshold and say why in "+
				"the commit message.",
			len(enabled), enabled, total, len(budgets), extractionGateThreshold,
		)
	}
}
