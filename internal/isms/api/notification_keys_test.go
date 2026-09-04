package api

import (
	"encoding/json"
	"fmt"
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

// Anything between braces counts, rather than a pattern that tries to describe
// a valid slot.
//
// vue-i18n's own grammar is wider than it looks: @intlify/message-compiler
// starts a named identifier on [a-zA-Z_] but continues it over [a-zA-Z0-9_$-],
// and readNamedIdentifier skips surrounding spaces — so {actor-name}, {a$b} and
// { actor } are all live slots that render, and it also accepts positional
// slots ({0}, {1}) that no call site here sends. A pattern matching only
// [a-zA-Z0-9_] misses every one of those, and missing them is not a false
// negative in isolation: the slot is invisible to the closed-set check AND to
// the fallback-placeholder check below, so a localised {pelaku-nama} renders
// empty with the whole file green.
//
// Mirroring the compiler's character classes here would just be a third twin to
// keep in sync. Capturing everything instead pushes the verdict onto the
// closed-set check, where {actor-name}, {a$b} and {0} all come out as "not a
// param the backend sends" — which is the right answer for each of them.
var placeholderRe = regexp.MustCompile(`\{([^{}]*)\}`)

func placeholdersIn(frame string) map[string]bool {
	out := map[string]bool{}
	for _, m := range placeholderRe.FindAllStringSubmatch(frame, -1) {
		out[strings.TrimSpace(m[1])] = true
	}
	return out
}

// wireKeyForCatalogKey inverts catalogKey so a frame can be checked against the
// params of the notification that writes it.
func wireKeyForCatalogKey(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, wireKey := range NotificationKeys {
		out[catalogKey(wireKey)] = wireKey
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
	allowed := allowedParamNames()
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

// TestNotificationKeyParamsIsExhaustive pins the per-key param table to the key
// set, so a new notification cannot ship without declaring what it sends. A key
// missing from the table would be exempt from the frame check below — the same
// hole TestNotificationKeysIsExhaustive closes for the slice.
func TestNotificationKeyParamsIsExhaustive(t *testing.T) {
	declared := map[string]bool{}
	for key := range NotificationKeyParams {
		declared[key] = true
	}
	for _, wireKey := range NotificationKeys {
		if !declared[wireKey] {
			t.Errorf("wire key %q has no entry in NotificationKeyParams\n"+
				"List the params its call sites send, or its frames are checked against nothing.", wireKey)
		}
		delete(declared, wireKey)
	}
	for key := range declared {
		t.Errorf("NotificationKeyParams has an entry for %q, which is not in NotificationKeys\n"+
			"The notification was renamed or removed and this entry is stale.", key)
	}
	allowed := allowedParamNames()
	for key, names := range NotificationKeyParams {
		for _, name := range names {
			if !allowed[name] {
				t.Errorf("NotificationKeyParams[%q] declares %q, which is not in the closed param set\n"+
					"See the Notification*Params vars in internal/isms/db/notifications.go.", key, name)
			}
		}
	}
}

// allowedParamNames is the package-wide vocabulary: every name the backend is
// permitted to send, regardless of which notification sends it.
func allowedParamNames() map[string]bool {
	allowed := map[string]bool{db.NotificationEntityParam: true}
	for _, n := range db.NotificationEnumParams {
		allowed[n] = true
	}
	for _, n := range db.NotificationVerbatimParams {
		allowed[n] = true
	}
	return allowed
}

// TestFramesOnlyUseTheirOwnKeysParams is the per-key half of the param guard.
//
// The closed-set check above asks whether a slot names a param the backend
// knows; this asks whether it names one THIS notification sends. The difference
// is the whole failure: {reason} added to the incident_status frame in every
// locale is a name the vocabulary contains and a name that call site never
// sends, so the closed-set check passes and the sentence renders with the
// clause silently missing — in English too, which is the one thing that makes
// this variant catchable at all, and only by someone who happens to look.
//
// Subset, not equality: a frame is free to spend fewer params than the call
// site offers (the shared suggestion_resolved title uses only {action} out of
// six), and a grammar may make a slot redundant in one language and not
// another. Asking for a param nobody sends is the unambiguous error.
func TestFramesOnlyUseTheirOwnKeysParams(t *testing.T) {
	wireKeys := wireKeyForCatalogKey(t)
	for _, locale := range localesWithNotifications(t) {
		for key, frame := range loadNotificationLeaves(t, locale) {
			wireKey, ok := wireKeys[key]
			if !ok {
				continue // stale frame; TestENNotificationFramesAreEmitted owns it
			}
			sent := map[string]bool{}
			for _, name := range NotificationKeyParams[wireKey] {
				sent[name] = true
			}
			for name := range placeholdersIn(frame) {
				if !sent[name] {
					t.Errorf("[%s] frame %q interpolates {%s}, which %q does not send\n"+
						"  frame: %q\n  sends: %v\n"+
						"The slot renders empty. Either the frame is wrong, or the call site "+
						"gained a param and NotificationKeyParams was not updated.",
						locale, key, name, wireKey, frame, NotificationKeyParams[wireKey])
				}
			}
		}
	}
}

// TestKeyedWritesUseDeclaredConstants closes the gap the exhaustiveness tests
// cannot see: they prove the declared constants are all indexed and translated,
// never that a notification write uses a declared constant at all.
//
// A call site can still write TitleKey: "notifications.foo" directly, or pass
// that literal into notifyMentions or notifySuggestionResolved, which take the
// key as a plain string parameter. Every check in this file stays green while
// the notification ships with no frame in any language — which is precisely the
// invisible failure the file exists to prevent, arriving through the one door
// left open.
//
// So: no "notifications." string literal anywhere in the server source except
// notification_keys.go, where the constants live. Scanning the whole of
// internal/ and cmd/ rather than this package alone, because the next write site
// need not land here — the MCP server writes notifications too.
func TestKeyedWritesUseDeclaredConstants(t *testing.T) {
	const declSite = "notification_keys.go"
	scanned := 0
	for _, root := range []string{"../../../internal", "../../../cmd"} {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			base := filepath.Base(path)
			// Tests may name keys freely — they assert against them, and a
			// literal in a test cannot reach a user's inbox.
			if strings.HasSuffix(base, "_test.go") || base == declSite {
				return nil
			}
			file, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if perr != nil {
				return fmt.Errorf("parsing %s: %w", path, perr)
			}
			scanned++
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				value, uerr := strconv.Unquote(lit.Value)
				if uerr != nil || !strings.HasPrefix(value, "notifications.") {
					return true
				}
				t.Errorf("%s contains the literal %q\n"+
					"Notification wire keys must come from a NotifyKey* constant in %s. A literal "+
					"is invisible to every check in this file, so the key ships untranslated.",
					path, value, declSite)
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if scanned == 0 {
		t.Fatal("scanned no Go files — the source layout changed and this test is now vacuous")
	}
}

// Anchored on the exact declarations in useNotificationRender.js. A regex over
// declarations is the fragility this file otherwise avoids, and it is the only
// option here — there is no JS parser in the Go toolchain. The not-found path is
// therefore fatal rather than a skip: a rename that moves these declarations out
// of reach must fail loudly, not quietly stop checking.
var (
	jsEnumParamsRe = regexp.MustCompile(`(?m)^const ENUM_PARAMS = \[([^\]]*)\]`)
	jsEntityRe     = regexp.MustCompile(`(?m)^const ENTITY_PARAM = '([^']*)'`)
	jsStringRe     = regexp.MustCompile(`'([^']*)'`)
)

// TestClientParamClassificationMatchesGo keeps the two halves of the
// translated/verbatim split in step.
//
// db.NotificationEnumParams decides which names the catalogue may translate;
// ENUM_PARAMS in useNotificationRender.js decides which ones the client
// actually resolves through common.enum.*. Nothing connected them. Adding an
// enum param on the Go side alone passes every catalogue check here and then
// splices the raw English value ("in_progress") into a translated sentence —
// the half-translated output the split exists to prevent. Dropping one on the
// client side alone does the same. Both are invisible in English.
func TestClientParamClassificationMatchesGo(t *testing.T) {
	const src = "../../../web/src/composables/useNotificationRender.js"
	raw, err := os.ReadFile(filepath.Clean(src))
	if err != nil {
		t.Fatalf("reading %s: %v\nThis test compares the Go param split against that file; "+
			"it cannot pass without it.", src, err)
	}
	m := jsEnumParamsRe.FindSubmatch(raw)
	if m == nil {
		t.Fatalf("no `const ENUM_PARAMS = [...]` declaration in %s\n"+
			"It was renamed or reshaped; update this test rather than leaving the two lists unchecked.", src)
	}
	var jsEnum []string
	for _, s := range jsStringRe.FindAllSubmatch(m[1], -1) {
		jsEnum = append(jsEnum, string(s[1]))
	}
	goEnum := append([]string(nil), db.NotificationEnumParams...)
	sort.Strings(jsEnum)
	sort.Strings(goEnum)
	if !reflect.DeepEqual(jsEnum, goEnum) {
		t.Errorf("ENUM_PARAMS in %s does not match db.NotificationEnumParams\n    js: %v\n    go: %v\n"+
			"A name in one list only is either an enum spliced in untranslated or a value "+
			"translated with no group behind it.", src, jsEnum, goEnum)
	}
	e := jsEntityRe.FindSubmatch(raw)
	if e == nil {
		t.Fatalf("no `const ENTITY_PARAM = '...'` declaration in %s", src)
	}
	if got := string(e[1]); got != db.NotificationEntityParam {
		t.Errorf("ENTITY_PARAM in %s is %q, db.NotificationEntityParam is %q", src, got, db.NotificationEntityParam)
	}
}
