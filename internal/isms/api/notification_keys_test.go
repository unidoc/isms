package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"isms.sh/internal/isms/db"
)

// The guard for plan 82 step 7: the keys this package emits and the frames
// web/src/locales/en/notifications.json provides must be the same set, in both
// directions, and every placeholder in those frames must be a param name the
// backend is allowed to send.
//
// This lives in the go-unit job rather than under `npm test` because the Go
// side owns the key set. It reads the locale file across the module boundary,
// which is unusual and deliberate: the invariant is a coupling between two
// languages, so the test has to sit on one side and look at the other. The
// alternative — asserting a hand-typed list, as web/test/notificationRender.test.js
// does — is what let the set drift in the first place: add a notification type
// and every existing check stays green.
//
// If the locale files move, fix this path. Do NOT make a missing file skip:
// no catalogue is the failure this test exists to report.
const localesDir = "../../../web/src/locales"

// fallbackLocale is the catalogue the key set is checked against. It is the
// one locale guaranteed complete by contract — useNotificationRender.js probes
// te() against it rather than the active locale, so a lagging locale renders
// English through fallbackLocale instead of dropping to the stored string.
const fallbackLocale = "en"

func notificationsPath(locale string) string {
	return filepath.Join(localesDir, locale, "notifications.json")
}

// localesWithNotifications lists every locale directory shipping a
// notifications catalogue.
//
// Checking only `en` would aim the guard away from the person it exists to
// protect: `en` is the locale that renders correctly no matter what, and the
// stated victim of every failure here is the non-English reader. A translator
// localising a placeholder name — {actor} to {pelaku}, a plausible-looking
// "fix" — is invisible to every other check in the tree: localeKeyset.test.js
// compares key sets and not frame contents, and notificationRender.test.js
// cannot see it because vue-i18n renders an unknown slot as the empty string
// rather than leaving a literal {…} behind, so its `!includes("{")` assertion
// passes on a sentence with the actor silently missing.
func localesWithNotifications(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(localesDir)
	if err != nil {
		t.Fatalf("reading %s: %v", localesDir, err)
	}
	var locales []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(notificationsPath(e.Name())); err == nil {
			locales = append(locales, e.Name())
		}
	}
	if len(locales) == 0 {
		t.Fatalf("no locale under %s ships notifications.json", localesDir)
	}
	return locales
}

// catalogKey maps a wire key onto the catalogue key that holds its frame.
//
// A title wire key (notifications.ca_resolved) is also the parent of its body
// wire key (notifications.ca_resolved.body), and no single JSON node can be a
// leaf and a group at once — so the catalogue nests uniformly and titles get
// ".title" appended. Body keys already name a leaf and pass through.
//
// This is a deliberate twin of catalogKey in
// web/src/composables/useNotificationRender.js:33. The same rule in two
// languages is a coupling worth naming: if that one changes, this one is wrong
// and the test will report keys as missing that render perfectly in the app.
func catalogKey(wireKey string) string {
	if strings.HasSuffix(wireKey, ".body") || strings.HasSuffix(wireKey, ".body_with_note") {
		return wireKey
	}
	return wireKey + ".title"
}

// loadENNotificationLeaves returns every leaf in the catalogue, keyed as a full
// dotted path with the "notifications." prefix restored, so leaves and
// catalogKey output are directly comparable.
func loadNotificationLeaves(t *testing.T, locale string) map[string]string {
	t.Helper()
	path := notificationsPath(locale)
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("reading the %s notification catalogue: %v\n"+
			"This test compares the keys Go emits against that file; it cannot pass without it.", locale, err)
	}
	// The file is one level of groups holding string leaves, but decode
	// generically so a future nesting change fails loudly here rather than
	// silently dropping frames from the comparison.
	var tree map[string]map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	leaves := map[string]string{}
	for group, node := range tree {
		for leaf, v := range node {
			s, ok := v.(string)
			if !ok {
				t.Fatalf("catalogue node notifications.%s.%s in %s is %T, want a string frame; "+
					"the catalogue is expected to be groups of string leaves", group, leaf, locale, v)
			}
			leaves["notifications."+group+"."+leaf] = s
		}
	}
	return leaves
}

// TestNotificationKeysHaveENFrames is the forward direction: nothing Go can
// write is untranslatable.
//
// This is the assertion that matters most, because its failure mode in
// production is invisible. useNotificationRender.js falls back to the stored
// English title when a key has no frame, so a missing translation looks exactly
// like correct behaviour to everyone except a non-English user.
func TestNotificationKeysHaveENFrames(t *testing.T) {
	leaves := loadNotificationLeaves(t, fallbackLocale)
	for _, wireKey := range NotificationKeys {
		if _, ok := leaves[catalogKey(wireKey)]; !ok {
			t.Errorf("wire key %q has no frame at %q in %s\n"+
				"Add it to the en catalogue (and to every other locale, or localeKeyset.test.js fails).",
				wireKey, catalogKey(wireKey), notificationsPath(fallbackLocale))
		}
	}
}

// TestENNotificationFramesAreEmitted is the reverse direction: no stale frames.
//
// A renamed or removed notification leaves a frame behind that every locale
// then keeps translating forever, because localeKeyset.test.js only compares
// locales to each other and would see nothing wrong with all of them carrying
// the same dead key.
func TestENNotificationFramesAreEmitted(t *testing.T) {
	emitted := map[string]bool{}
	for _, wireKey := range NotificationKeys {
		emitted[catalogKey(wireKey)] = true
	}
	for key := range loadNotificationLeaves(t, fallbackLocale) {
		if !emitted[key] {
			t.Errorf("catalogue frame %q is not emitted by any key in NotificationKeys\n"+
				"Either a notification was renamed/removed and this frame is stale, or a new "+
				"call site is missing its constant in notification_keys.go.", key)
		}
	}
}

// Digits are included deliberately. vue-i18n also accepts positional slots
// ({0}, {1}), which no call site here sends — so a frame acquiring one renders
// it empty, and a name-only pattern would not even see it to complain.
var placeholderRe = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

func placeholdersIn(frame string) map[string]bool {
	out := map[string]bool{}
	for _, m := range placeholderRe.FindAllStringSubmatch(frame, -1) {
		out[m[1]] = true
	}
	return out
}

// TestNotificationPlaceholdersAreInClosedSet checks the catalogue half of the
// param guard: every slot a frame asks to be filled is a name the backend is
// allowed to send.
//
// The write-side half is NotificationContent.Validate plus the log on its
// degrade path (both exercised in internal/isms/db). The two halves catch
// opposite mistakes — a frame asking for a param nobody sends renders an
// unfilled slot; a call site sending a param no frame reads is dead weight or
// a rename done on one side. Neither is visible in English.
func TestNotificationPlaceholdersAreInClosedSet(t *testing.T) {
	allowed := map[string]bool{db.NotificationEntityParam: true}
	for _, n := range db.NotificationEnumParams {
		allowed[n] = true
	}
	for _, n := range db.NotificationVerbatimParams {
		allowed[n] = true
	}
	for _, locale := range localesWithNotifications(t) {
		for key, frame := range loadNotificationLeaves(t, locale) {
			for name := range placeholdersIn(frame) {
				if !allowed[name] {
					t.Errorf("[%s] frame %q interpolates {%s}, which is not in the closed param set\n"+
						"Either the call site sends a name the schema does not document, or this is "+
						"a typo that will render as an empty slot. See "+
						"the Notification*Params vars in internal/isms/db/notifications.go.", locale, key, name)
				}
			}
		}
	}
}

// TestTranslatedFramesReuseTheFallbackPlaceholders catches the localised
// placeholder: a translator renaming {actor} to {pelaku} because every other
// word in the sentence was translated.
//
// Nothing else in the tree catches it. localeKeyset.test.js compares key sets,
// not frame contents. notificationRender.test.js asserts the rendered string
// contains no "{", which holds — vue-i18n drops an unknown slot to the empty
// string rather than leaving the literal behind, so the actor simply vanishes
// from the sentence and the test passes.
//
// Subset, not equality: a translation may legitimately drop a slot its grammar
// makes redundant, and forcing every frame to spend every param would be a
// claim about other languages this test has no standing to make. Introducing a
// name the fallback does not have is the unambiguous error, and it is the one
// that silently loses information.
func TestTranslatedFramesReuseTheFallbackPlaceholders(t *testing.T) {
	fallback := loadNotificationLeaves(t, fallbackLocale)
	for _, locale := range localesWithNotifications(t) {
		if locale == fallbackLocale {
			continue
		}
		for key, frame := range loadNotificationLeaves(t, locale) {
			base, ok := fallback[key]
			if !ok {
				continue // localeKeyset.test.js owns key-set drift
			}
			baseNames := placeholdersIn(base)
			for name := range placeholdersIn(frame) {
				if !baseNames[name] {
					t.Errorf("[%s] frame %q interpolates {%s}, which the %s frame does not have\n"+
						"  %s: %q\n  %s: %q\n"+
						"Placeholder names are param names sent by the backend and are never "+
						"translated; this slot will render empty.",
						locale, key, name, fallbackLocale, fallbackLocale, base, locale, frame)
				}
			}
		}
	}
}

// TestNotificationKeysIsExhaustive holds NotificationKeys to the constants
// declared beside it. The slice is what every check above walks, so a constant
// added to the const block but forgotten here would be exempt from all of them
// — a guard with a hole in exactly the place a new notification lands.
//
// Enumerating the const block means reading the source: Go has no reflection
// over package-level constants. It is parsed with go/ast rather than matched
// with a regex, because a regex over declarations is exactly the fragility
// this file exists to remove. An earlier version anchored on the closing quote
// at end of line and accepted no digits in the key, so a constant carrying
// this repo's usual trailing issue reference —
//
//	NotifyKeyRiskReviewDue = "notifications.risk_review_due" // #241
//
// — or one naming a standard (notifications.iso27001_review_due) was invisible
// to it. Both stayed invisible while gofmt reported the file clean, and a
// constant missing from the slice AND from the scan matches on both sides: the
// test passes, and the key ships with no frame at all.
func TestNotificationKeysIsExhaustive(t *testing.T) {
	const src = "notification_keys.go"
	file, err := parser.ParseFile(token.NewFileSet(), src, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", src, err)
	}
	var declared []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "NotifyKey") || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Errorf("%s is not a string literal; every NotifyKey* constant must be one, "+
						"or it cannot be compared against the catalogue", name.Name)
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquoting %s: %v", name.Name, err)
				}
				declared = append(declared, value)
			}
		}
	}
	if len(declared) == 0 {
		t.Fatal("found no NotifyKey* constants — the declaration shape changed and this test is now vacuous")
	}
	inSlice := append([]string(nil), NotificationKeys...)
	sort.Strings(declared)
	sort.Strings(inSlice)
	if !reflect.DeepEqual(declared, inSlice) {
		t.Errorf("NotificationKeys does not match the declared constants\n slice: %v\nconsts: %v\n"+
			"Add the new constant to NotificationKeys, or it is exempt from every check in this file.",
			inSlice, declared)
	}
}
