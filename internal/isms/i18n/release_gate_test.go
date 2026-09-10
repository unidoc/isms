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
// fully extracted tree may still carry a residue, and 100 was an estimate of it.
//
// For scale: the tree started at 2488 and sits at 0, every surface — auth,
// shell, component, admin, register, workflow and document — now extracted. So
// the residue this threshold estimates turned out to be nothing the scanner can
// still see: 100 is now headroom for a future unextracted view rather than an
// estimate of an unmeasured remainder. Revisiting the number is therefore a
// judgement about how much new raw text a release may carry, and says so. The
// coupling it enforces is not.
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

// keysetSnapshot is the frozen `en` keyset — the set a translation is measured
// against. Read from here rather than re-walking the locale files a third time:
// web/scripts/i18nKeyset.mjs writes it, web/test/localeKeyset.test.js pins `en`
// to it, and this gate reads it. One artifact, three consumers.
const keysetSnapshot = "../../../web/test/keyset.snapshot.json"

// localesDir is the message-bundle root. One directory per tag, one file per
// area, and the area key is the filename — see web/src/locales/en/index.js.
const localesDir = "../../../web/src/locales"

// translationGateThreshold is the share of the frozen keyset an enabled locale's
// bundle must *cover* — that is, contain a key for — in percent.
//
// Coverage, not translatedness: what this number bounds is how much of the app
// a bundle has entries for, and the paragraph below on `cp -r en` is why the
// distinction has to be stated rather than left as a synonym. The procedure in
// docs/i18n.md is what makes coverage stand in for translation, by having a key
// arrive only when someone has translated it.
//
// This gate exists because the extraction gate above was necessary and not
// sufficient, and the gap was invisible for exactly as long as extraction was
// the binding constraint. Extraction finished, so the raw-text *baseline* fell
// to zero — `extractionGateThreshold` itself is unchanged at 100, and the two
// are easy to conflate: the threshold is the budget, the baseline is the
// measurement. With the measurement at zero the gate is satisfied, and `id-ID`
// — 244 of 2448 keys, just under 10% at the time — became one flag away from
// being offered. That is the precise failure the other gate's comment
// describes: "the picker offers a language, and the app answers in mostly
// English". It cannot see it, because it measures the *app*, not the *bundle*.
// This one measures the bundle, and it is what held id-ID back until the bundle
// was finished to 2448 of 2448 and the locale enabled.
//
// The limit that follows from measuring presence is worth spelling out, because
// the obvious way to start a locale walks straight into it: `cp -r en <tag>`
// yields a bundle that is missing nothing, so this gate reads 100% while every
// value is English — the exact release it exists to block, waved through.
// Verified, not assumed.
//
// Detecting that here would need a value comparison, and values legitimately
// agree with `en` sometimes — in id-ID, loanwords like "Audit", "Program" and
// "Passkey" do. So the control is the procedure instead —
// docs/i18n.md step 3 says to add area files as they are finished and why — and
// a reviewer seeing 25 new files of English in one commit. If a copied bundle
// ever reaches a release, the fix is a check that fails on a *large* share of
// identical values, not on any.
//
// 90 rather than 100 because `localeKeyset.test.js` deliberately lets a
// translation lag: a missing key renders in English through fallbackLocale, and
// a bundle a few keys behind must not block an unrelated PR. So a small,
// shrinking remainder is the designed steady state and 100 would fight it. 90 is
// a judgement about how much English a reader may meet before the offer becomes
// a lie, and like the threshold above it is meant to be argued with in a commit
// that says why — not adjusted to make a red build green.
const translationGateThreshold = 90

// leafKeys flattens a message bundle to dotted paths. It must match the walk in
// web/scripts/i18nKeyset.mjs, which is the single JS implementation — the web
// test imports it rather than keeping a copy, so there are two walkers in the
// repo and not three.
//
// Including the treatment of an empty object as a leaf in its own right. No
// bundle contains one today (`common.enum` shipped empty in #217 and was filled
// in #229, which is where the rule came from), so this branch is currently
// defensive rather than exercised. It stays because the two walkers diverging is
// the failure mode: the snapshot would count a reserved group and this gate
// would not, quietly changing a locale's measured coverage in the direction of
// passing.
func leafKeys(v any, prefix string, out *[]string) {
	obj, ok := v.(map[string]any)
	if !ok || len(obj) == 0 {
		*out = append(*out, prefix)
		return
	}
	for k, child := range obj {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		leafKeys(child, path, out)
	}
}

// bundleKeys reads every area file of one locale and returns its leaf keys.
// The area key is the filename without its extension, which is the contract
// index.js implements on the JS side.
func bundleKeys(t *testing.T, tag string) []string {
	t.Helper()
	dir := filepath.Join(filepath.Clean(localesDir), tag)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading bundle for enabled locale %q at %s: %v", tag, dir, err)
	}
	var keys []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading %s/%s: %v", tag, name, err)
		}
		var area any
		if err := json.Unmarshal(raw, &area); err != nil {
			t.Fatalf("parsing %s/%s: %v", tag, name, err)
		}
		leafKeys(area, name[:len(name)-len(".json")], &keys)
	}
	return keys
}

// TestEnabledLocalesAreTranslated is the second half of the release gate.
//
// TestSecondLocaleRequiresExtraction asks whether the *app* is extracted.
// This asks whether the *bundle* covers it. Both were once satisfied by the
// same fact — nothing was extracted, so nothing could be translated — and they
// came apart the moment extraction finished. A reader meets the worse of the
// two, so both are gated.
//
// Only enabled locales are checked, and Default is skipped: `en` is the
// reference, so measuring it against itself would assert 100% by construction.
func TestEnabledLocalesAreTranslated(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean(keysetSnapshot))
	if err != nil {
		// A missing snapshot is a failure, not a skip — the same reasoning as the
		// baseline above. It is the evidence half of the gate.
		t.Fatalf("keyset snapshot not found at %s: %v", keysetSnapshot, err)
	}
	var frozen []string
	if err := json.Unmarshal(raw, &frozen); err != nil {
		t.Fatalf("parsing %s: %v", keysetSnapshot, err)
	}
	if len(frozen) == 0 {
		t.Fatalf("%s is empty: every locale would pass vacuously", keysetSnapshot)
	}

	for tag, e := range supported {
		if tag == Default || !e.enabled {
			continue
		}
		have := make(map[string]bool, len(frozen))
		for _, k := range bundleKeys(t, tag) {
			have[k] = true
		}
		missing := 0
		var examples []string
		for _, k := range frozen {
			if !have[k] {
				missing++
				if len(examples) < 5 {
					examples = append(examples, k)
				}
			}
		}
		covered := len(frozen) - missing
		percent := covered * 100 / len(frozen)
		if percent < translationGateThreshold {
			t.Errorf(
				"locale %q (%s) is enabled but covers %d of %d keys (%d%%), under the %d%% "+
					"threshold.\n\n"+
					"Coverage counts keys the bundle has, so this is the generous reading; a key "+
					"that is present but still English counts here and the procedure in "+
					"docs/i18n.md is what keeps that from happening at scale.\n"+
					"Missing keys render in English, so enabling it now offers a language the app "+
					"largely cannot speak — which is worse for the contributor who translated the "+
					"bundle than for anyone else.\n"+
					"Finish the bundle (issue #212), or disable the locale until it is finished.\n"+
					"First missing keys: %v\n"+
					"If the threshold is what is wrong, change translationGateThreshold and say why "+
					"in the commit message.",
				tag, e.name, covered, len(frozen), percent, translationGateThreshold, examples,
			)
		}
	}
}
