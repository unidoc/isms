package api

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// The error-code catalogue has the same failure mode as the notification
// catalogue in notification_keys_test.go, and for the same reason: the client
// falls back to the English `message` whenever a code has no translation, so a
// missing entry is invisible to everyone reading English — which is everyone
// writing and reviewing the change. It is wrong only for the non-English user,
// silently, in production. Hence the parity tests below rather than a review
// checklist.

// ---- locale catalogue loading -------------------------------------------

// requireLocalesDir returns the shared locale root declared in
// notification_keys_test.go, failing if it is missing. A missing catalogue is
// the failure these tests exist to report, so it must never skip.
func requireLocalesDir(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(localesDir); err != nil {
		t.Fatalf("locales directory not found at %s: %v", localesDir, err)
	}
	return localesDir
}

// commonErrors returns the `common.error` group of one locale, flattened to
// code -> message. A nested value would mean somebody grouped codes, which the
// client's flat `common.error.<code>` lookup cannot reach, so that fails here
// rather than at render time.
func commonErrors(t *testing.T, locale string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(requireLocalesDir(t), locale, "common.json"))
	if err != nil {
		t.Fatalf("reading %s/common.json: %v", locale, err)
	}
	var parsed struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parsing %s/common.json: %v", locale, err)
	}
	out := make(map[string]string, len(parsed.Error))
	for code, v := range parsed.Error {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("%s common.error.%s is %T, want a flat string — the client looks up common.error.<code> and cannot reach a nested group", locale, code, v)
		}
		out[code] = s
	}
	return out
}

func commonGroup(t *testing.T, locale, group string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(requireLocalesDir(t), locale, "common.json"))
	if err != nil {
		t.Fatalf("reading %s/common.json: %v", locale, err)
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parsing %s/common.json: %v", locale, err)
	}
	out := map[string]string{}
	if body, ok := parsed[group]; ok {
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("parsing %s common.%s: %v", locale, group, err)
		}
	}
	return out
}

func enumInlineGroup(t *testing.T, locale, group string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(requireLocalesDir(t), locale, "common.json"))
	if err != nil {
		t.Fatalf("reading %s/common.json: %v", locale, err)
	}
	var parsed struct {
		EnumInline map[string]map[string]string `json:"enum_inline"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parsing %s/common.json: %v", locale, err)
	}
	return parsed.EnumInline[group]
}

func localeDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(requireLocalesDir(t))
	if err != nil {
		t.Fatalf("listing locales: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// Sets, not slices. A language that needs the noun twice repeats the slot, and
// that is a correct translation — comparing slices would fail CI on it.
func placeholders(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range regexp.MustCompile(`\{([^{}]*)\}`).FindAllStringSubmatch(s, -1) {
		name := strings.TrimSpace(m[1])
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ---- wire shape ---------------------------------------------------------

// The flat body is a property of Echo's error handler, not of errorBody, so it
// is asserted through a real echo instance. An Echo upgrade that changes the
// type switch in DefaultHTTPErrorHandler fails here rather than in a browser.
func TestErrorBodyWireShape(t *testing.T) {
	e := echo.New()
	e.GET("/boom", func(c echo.Context) error {
		return apiError(http.StatusNotFound, CodeNotFound, Entity("corrective_action"))
	})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	var body struct {
		Message string            `json:"message"`
		Code    string            `json:"code"`
		Params  map[string]string `json:"params"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshalling %s: %v", rec.Body.String(), err)
	}

	// message stays English prose: internal/isms/client/client.go dumps this
	// body straight into CLI output.
	if body.Message != "corrective action not found" {
		t.Errorf("message = %q, want %q", body.Message, "corrective action not found")
	}
	if body.Code != CodeNotFound {
		t.Errorf("code = %q, want %q", body.Code, CodeNotFound)
	}
	// The wire param keeps the identifier, not the humanised prose — it is a
	// catalogue lookup key on the client, not text.
	if body.Params[ParamEntity] != "corrective_action" {
		t.Errorf("params.entity = %q, want %q", body.Params[ParamEntity], "corrective_action")
	}
}

// errorBody must stay outside both special cases in Echo's type switch:
// implementing error collapses the body to {"message": ...}, and implementing
// json.Marshaler hands serialisation to a method that would then have to
// reproduce the shape by hand. Both are one-line edits away and neither would
// fail any other test.
func TestErrorBodyIsNotSpecialCasedByEcho(t *testing.T) {
	var b any = errorBody{Message: "x", Code: "y"}
	if _, ok := b.(error); ok {
		t.Error("errorBody implements error, so Echo will serialise it as {\"message\": ...} and drop code/params")
	}
	if _, ok := b.(json.Marshaler); ok {
		t.Error("errorBody implements json.Marshaler, so Echo skips its own encoding path")
	}
}

// HTTPError.Error() formats Message with %v. Without the Stringer, server logs
// and test failures would print the struct.
func TestErrorMessageStaysReadableInLogs(t *testing.T) {
	err := apiError(http.StatusBadRequest, CodeRequired, Field("title"))
	if got, want := err.Error(), "code=400, message=title is required"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// ---- rendering ----------------------------------------------------------

func TestApiErrorRendering(t *testing.T) {
	cases := []struct {
		name   string
		err    *echo.HTTPError
		msg    string
		params map[string]string
	}{
		{
			name:   "no params",
			err:    apiError(http.StatusBadRequest, CodeInvalidID),
			msg:    "invalid id",
			params: nil,
		},
		{
			name:   "entity is humanised in the message, raw on the wire",
			err:    errNotFound("legal_requirement"),
			msg:    "legal requirement not found",
			params: map[string]string{ParamEntity: "legal_requirement"},
		},
		{
			name:   "invalid-id wrapper",
			err:    errInvalidEntityID("review"),
			msg:    "invalid review id",
			params: map[string]string{ParamEntity: "review"},
		},
		{
			// field is humanised the same way entity is, so a snake_case JSON
			// field name reads as prose in the CLI: today's literal at this
			// call site says "path_pattern is required".
			name:   "required wrapper humanises the field name",
			err:    errRequired("path_pattern"),
			msg:    "path pattern is required",
			params: map[string]string{ParamField: "path_pattern"},
		},
		{
			name:   "count is spliced verbatim",
			err:    apiError(http.StatusBadRequest, CodePasswordTooShort, Count(7)),
			msg:    "password must be at least 7 characters",
			params: map[string]string{ParamCount: "7"},
		},
		{
			name:   "status uses the enum member",
			err:    apiError(http.StatusBadRequest, CodeReviewWrongStatus, Status("changes_requested")),
			msg:    "review is changes requested",
			params: map[string]string{ParamStatus: "changes_requested"},
		},
		{
			// Degrade, do not panic: an operator losing a word beats an
			// operator losing the request.
			name:   "unknown code falls back to the code",
			err:    apiError(http.StatusInternalServerError, "no_such_code"),
			msg:    "no_such_code",
			params: nil,
		},
		{
			// A surplus param leaves the message alone but still reaches the
			// client, which is the direction that fails safe.
			name:   "param with no slot is kept on the wire",
			err:    apiError(http.StatusBadRequest, CodeInvalidID, Field("title")),
			msg:    "invalid id",
			params: map[string]string{ParamField: "title"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, ok := tc.err.Message.(errorBody)
			if !ok {
				t.Fatalf("Message is %T, want errorBody", tc.err.Message)
			}
			if body.Message != tc.msg {
				t.Errorf("message = %q, want %q", body.Message, tc.msg)
			}
			if !reflect.DeepEqual(body.Params, tc.params) {
				t.Errorf("params = %v, want %v", body.Params, tc.params)
			}
			// Whatever else happens, a code always reaches the client.
			if body.Code == "" {
				t.Error("code is empty")
			}
		})
	}
}

func TestParamConstructorsCoverTheClosedSet(t *testing.T) {
	got := []string{Entity("x").Key, Field("x").Key, Status("x").Key, Count(1).Key, Value("x").Key}
	sort.Strings(got)
	want := []string{ParamCount, ParamEntity, ParamField, ParamStatus, ParamValue}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("constructors emit %v, want the closed set %v", got, want)
	}
}

// ---- catalogue exhaustiveness -------------------------------------------

// A Code* constant that never reaches errorMessages compiles, passes review
// and ships an error whose `message` is the raw code. The constants are read
// back out of the source because Go cannot enumerate them at runtime — the
// same extraction problem #244 hit, solved the same way: cheaply, and only
// because the declaration site is one file with one shape.
func TestEveryCodeConstantIsCatalogued(t *testing.T) {
	src, err := os.ReadFile("errors.go")
	if err != nil {
		t.Fatalf("reading errors.go: %v", err)
	}
	decl := regexp.MustCompile(`(?m)^\s*(Code[A-Za-z0-9]*)\s*=\s*"([^"]+)"`)
	matches := decl.FindAllStringSubmatch(string(src), -1)
	if len(matches) == 0 {
		t.Fatal("found no Code* constants — the extraction regex has gone stale, which would make this test pass vacuously")
	}
	for _, m := range matches {
		if _, ok := errorMessages[m[2]]; !ok {
			t.Errorf("%s = %q has no entry in errorMessages, so apiError would send the code as its own message", m[1], m[2])
		}
	}
	if len(matches) != len(errorMessages) {
		t.Errorf("%d Code* constants but %d errorMessages entries — an entry keyed by a bare literal is unreachable from call sites", len(matches), len(errorMessages))
	}
}

func TestErrorCodesIsDerivedFromTheCatalogue(t *testing.T) {
	if len(ErrorCodes()) != len(errorMessages) {
		t.Fatalf("ErrorCodes() has %d entries, errorMessages has %d", len(ErrorCodes()), len(errorMessages))
	}
	if !sort.StringsAreSorted(ErrorCodes()) {
		t.Error("ErrorCodes() is not sorted")
	}
}

func TestErrorCodeParamsReadsTheTemplate(t *testing.T) {
	if got, want := ErrorCodeParams(CodeNotFound), []string{ParamEntity}; !reflect.DeepEqual(got, want) {
		t.Errorf("params(%s) = %v, want %v", CodeNotFound, got, want)
	}
	if got := ErrorCodeParams(CodeInvalidID); got != nil {
		t.Errorf("params(%s) = %v, want none", CodeInvalidID, got)
	}
	if got := ErrorCodeParams("no_such_code"); got != nil {
		t.Errorf("params of an unknown code = %v, want none", got)
	}
}

// Every param a template names must be one of the four the client knows how to
// resolve. A fifth name is not merely unrecognised — it is spliced in raw, so
// an identifier reaches the reader untranslated in every language.
func TestTemplatesUseOnlyTheClosedParamSet(t *testing.T) {
	closed := map[string]bool{ParamEntity: true, ParamField: true, ParamStatus: true, ParamCount: true, ParamValue: true}
	for _, code := range ErrorCodes() {
		for _, p := range ErrorCodeParams(code) {
			if !closed[p] {
				t.Errorf("errorMessages[%s] names {%s}, which is outside the closed param set — the client has no lookup rule for it", code, p)
			}
		}
	}
}

// ---- parity with the web catalogue --------------------------------------

func TestEveryCodeHasAnEnglishKey(t *testing.T) {
	cat := commonErrors(t, "en")
	for _, code := range ErrorCodes() {
		if _, ok := cat[code]; !ok {
			t.Errorf("common.error.%s is missing from en — the client falls back to the English message, so this ships as a silent English leak", code)
		}
	}
}

// The other direction. A code renamed on the Go side leaves a dead entry that
// every future translator keeps translating, and that nothing will ever read.
func TestEveryEnglishKeyHasACode(t *testing.T) {
	known := map[string]bool{}
	for _, code := range ErrorCodes() {
		known[code] = true
	}
	for code := range commonErrors(t, "en") {
		if !known[code] {
			t.Errorf("common.error.%s has no code in errorMessages — stale key", code)
		}
	}
}

// The English sentence is written twice — in errorMessages for the CLI and in
// common.error.* for the browser — and comparing only placeholder sets lets
// the two drift in wording while every check stays green. They are the same
// language, so they must be the same string.
func TestEnglishCatalogueMatchesTheGoTemplates(t *testing.T) {
	cat := commonErrors(t, "en")
	for _, code := range ErrorCodes() {
		if got, want := cat[code], errorMessages[code]; got != want {
			t.Errorf("common.error.%s = %q but errorMessages says %q — one reader gets each", code, got, want)
		}
	}
}

// An override claims Go and the en locale agree on an identifier's English
// prose. Checking it means the CLI and the browser cannot disagree the way
// they would have for "oidc provider" / "OIDC provider".
func TestOverridesMatchTheEnglishCatalogue(t *testing.T) {
	entities := commonGroup(t, "en", "entity_inline")
	fields := commonGroup(t, "en", "field")
	for value, override := range paramMessageOverrides {
		var catalogued string
		switch {
		case entities[value] != "":
			catalogued = entities[value]
		case fields[value] != "":
			catalogued = fields[value]
		default:
			continue // not yet a param value anywhere; the override is harmless
		}
		if catalogued != override {
			t.Errorf("paramMessageOverrides[%q] = %q but the en catalogue says %q", value, override, catalogued)
		}
	}
}

// Every status a status-carrying code can send must have an inline form.
// TestParamValuesHaveCatalogueKeys cannot see these: the call sites pass
// review.Status, not a literal.
func TestErrorStatusValuesHaveInlineForms(t *testing.T) {
	for _, locale := range localeDirs(t) {
		inline := enumInlineGroup(t, locale, "status")
		if len(inline) == 0 {
			t.Fatalf("%s common.enum_inline.status is empty", locale)
		}
		for code, values := range errorStatusValues {
			for _, v := range values {
				if _, ok := inline[v]; !ok {
					t.Errorf("%s can send status %q, which has no common.enum_inline.status.%s key in %s — enumLabelInline then falls back to the capitalised standalone label mid-sentence", code, v, v, locale)
				}
			}
		}
	}
}

// A new code carrying {status} without an errorStatusValues entry would make
// the check above pass by having nothing to check.
func TestErrorStatusValuesCoverEveryStatusCode(t *testing.T) {
	for _, code := range ErrorCodes() {
		carries := false
		for _, p := range ErrorCodeParams(code) {
			if p == ParamStatus {
				carries = true
			}
		}
		if _, declared := errorStatusValues[code]; carries != declared {
			t.Errorf("%s carries {status}=%v but has an errorStatusValues entry=%v", code, carries, declared)
		}
	}
}

// Checked for every locale, not just en. en is the one language that renders
// correctly whatever the slots say, because it is also the fallback; the
// locale this whole seam exists to protect is the one nobody on the team
// reads.
func TestPlaceholdersMatchInEveryLocale(t *testing.T) {
	for _, locale := range localeDirs(t) {
		cat := commonErrors(t, locale)
		for _, code := range ErrorCodes() {
			msg, ok := cat[code]
			if !ok {
				// Missing is a warning, not a failure: fallbackLocale
				// covers it, and a translation lagging a few keys must not
				// block an unrelated PR. Same policy as localeKeyset.test.js.
				continue
			}
			got := placeholders(msg)
			want := ErrorCodeParams(code)
			if len(got) == 0 {
				got = nil
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s common.error.%s has slots %v, Go sends %v — an extra slot renders blank, a missing one drops what it named",
					locale, code, got, want)
			}
		}
	}
}

// ---- param values resolve to catalogue keys -----------------------------

// The seam most likely to leak English silently (plan 81 task 1.6): the client
// translates an `entity` param through common.entity_inline.* and a `field`
// through common.field.*, so a value with no key there is spliced in as the
// raw snake_case identifier — inside an otherwise translated sentence.
//
// The values are literals at the call site, so they can be read off the
// source. This passes vacuously until task 1.5 converts the first call site,
// which is the point: it starts working the moment there is anything to check.
//
// Parsed rather than grepped: a regex over the source also matches the
// `Status("…")` written inside a doc comment, and a check that fires on its
// own documentation gets deleted rather than fixed.
//
// It sees only literal arguments. Status in particular is passed a variable at
// every real call site (`review.Status`), so the scan is structurally blind to
// exactly the param whose values are hardest to predict — that half is covered
// by errorStatusValues and TestErrorStatusValuesHaveInlineForms, and the limit
// is stated in docs/i18n.md rather than left for a reader to discover.
// paramWrappers maps the convenience helpers onto the param kind they build.
//
// Without this the scan is blind to the exact spelling the documentation tells
// people to use: inside errNotFound the constructor call is `Entity(entity)`,
// a variable, so `errNotFound("widget")` — literal, wrong, and the main path —
// sailed past. A check that only sees the form nobody writes is not a check.
var paramWrappers = map[string]string{
	"errNotFound":        ParamEntity,
	"errInvalidEntityID": ParamEntity,
	"errRequired":        ParamField,
}

// paramLiteral reports the param kind and literal value a call expression
// supplies, for both the constructors and the wrappers above.
func paramLiteral(call *ast.CallExpr) (kind, value string, ok bool) {
	ident, isIdent := call.Fun.(*ast.Ident)
	if !isIdent || len(call.Args) != 1 {
		return "", "", false
	}
	switch ident.Name {
	case "Entity":
		kind = ParamEntity
	case "Field":
		kind = ParamField
	case "Status":
		kind = ParamStatus
	default:
		if k, isWrapper := paramWrappers[ident.Name]; isWrapper {
			kind = k
		} else {
			return "", "", false
		}
	}
	lit, isLit := call.Args[0].(*ast.BasicLit)
	if !isLit || lit.Kind != token.STRING {
		// A variable. Status is passed one at every real call site; those
		// values are covered by errorStatusValues instead.
		return "", "", false
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", "", false
	}
	return kind, v, true
}

func parsePackage(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing package: %v", err)
	}
	var files []*ast.File
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		t.Fatal("parsed no files — the scan would pass vacuously")
	}
	return fset, files
}

func TestParamValuesHaveCatalogueKeys(t *testing.T) {
	fset, files := parsePackage(t)

	// Checked in EVERY locale, not just en.
	//
	// The message-level policy is that a missing translation only warns:
	// fallbackLocale renders the whole sentence in English, which is
	// coherent, and a translation lagging a few keys must not block an
	// unrelated PR. A missing *param* value is a different failure. The
	// sentence around it still translates, so the reader gets one English
	// word wedged into their own language — worse than either language on its
	// own, and not something the fallback can rescue. The vocabulary is also
	// tiny and shared across the whole UI, so requiring it costs a translator
	// nothing.
	for _, locale := range localeDirs(t) {
		catalogues := map[string]map[string]string{
			ParamEntity: commonGroup(t, locale, "entity_inline"),
			ParamField:  commonGroup(t, locale, "field"),
			ParamStatus: enumInlineGroup(t, locale, "status"),
		}
		paths := map[string]string{
			ParamEntity: "common.entity_inline",
			ParamField:  "common.field",
			ParamStatus: "common.enum_inline.status",
		}
		for _, file := range files {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				kind, value, ok := paramLiteral(call)
				if !ok {
					return true
				}
				if _, found := catalogues[kind][value]; !found {
					pos := fset.Position(call.Pos())
					t.Errorf("%s:%d: %s value %q has no %s.%s key in %s — it renders as an English word inside a %s sentence",
						filepath.Base(pos.Filename), pos.Line, kind, value, paths[kind], value, locale, locale)
				}
				return true
			})
		}
	}
}

// The params a call site passes must be exactly the ones its code's template
// asks for. Nothing in the type system says so — Entity and Field are both
// ErrorParam, so the wrong one, or none at all, compiles and ships a literal
// "{entity} not found" to the reader.
//
// Vacuous until the first call site is converted, like the scan above.
func TestCallSitesMatchTheirCodes(t *testing.T) {
	fset, files := parsePackage(t)

	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok || fn.Name != "apiError" || len(call.Args) < 2 {
				return true
			}
			pos := fset.Position(call.Pos())
			where := fmt.Sprintf("%s:%d", filepath.Base(pos.Filename), pos.Line)

			codeIdent, ok := call.Args[1].(*ast.Ident)
			if !ok {
				t.Errorf("%s: apiError code is not a Code* constant — a bare literal cannot be checked against the catalogue", where)
				return true
			}
			code, known := codeConstants[codeIdent.Name]
			if !known {
				t.Errorf("%s: apiError code %s is not a declared constant", where, codeIdent.Name)
				return true
			}

			var got []string
			for _, arg := range call.Args[2:] {
				argCall, ok := arg.(*ast.CallExpr)
				if !ok {
					return true // built elsewhere; out of this check's reach
				}
				ident, ok := argCall.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				switch ident.Name {
				case "Entity":
					got = append(got, ParamEntity)
				case "Field":
					got = append(got, ParamField)
				case "Status":
					got = append(got, ParamStatus)
				case "Count":
					got = append(got, ParamCount)
				case "Value":
					got = append(got, ParamValue)
				default:
					return true
				}
			}
			sort.Strings(got)
			want := ErrorCodeParams(code)
			if len(got) == 0 {
				got = nil
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s: %s passes params %v but %q needs %v — an unfilled slot reaches the reader verbatim",
					where, codeIdent.Name, got, errorMessages[code], want)
			}
			return true
		})
	}
}

// codeConstants maps constant name -> wire value, read out of the source for
// the same reason TestEveryCodeConstantIsCatalogued does: Go cannot enumerate
// its own constants at runtime.
var codeConstants = func() map[string]string {
	src, err := os.ReadFile("errors.go")
	if err != nil {
		panic(err)
	}
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s*(Code[A-Za-z0-9]*)\s*=\s*"([^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = m[2]
	}
	return out
}()

// The param vocabulary must exist in every locale, whether or not a call site
// uses it yet.
//
// The scan above only sees values some call site actually passes, so a key
// added to `en` and forgotten in `id-ID` is invisible until conversion reaches
// it — and by then it is a translated sentence with an English word in it,
// found by a user rather than by CI. `common.entity_inline` already has this
// mirror in enumCatalog.test.js; `common.field` had none.
func TestParamVocabularyExistsInEveryLocale(t *testing.T) {
	reference := commonGroup(t, "en", "field")
	if len(reference) == 0 {
		t.Fatal("en common.field is empty")
	}
	for _, locale := range localeDirs(t) {
		if locale == "en" {
			continue
		}
		have := commonGroup(t, locale, "field")
		for name := range reference {
			if _, ok := have[name]; !ok {
				t.Errorf("common.field.%s is missing in %s — the sentence around it translates, so the reader gets one English word wedged into their own language", name, locale)
			}
		}
	}
}

// Errors interpolate mid-sentence ("risk not found"), so the entity param
// resolves through common.entity_inline.*, not common.entity.*. The existing
// enumCatalog.test.js already holds the two groups in step for en and mirrors
// entity_inline into id-ID; this asserts the group errors depend on is not
// empty, which is what would make the check above pass vacuously.
func TestEntityInlineCatalogueIsPopulated(t *testing.T) {
	for _, locale := range localeDirs(t) {
		inline := commonGroup(t, locale, "entity_inline")
		if len(inline) == 0 {
			t.Fatalf("%s common.entity_inline is empty", locale)
		}
		for _, name := range []string{"risk", "review", "document", "user", "organization"} {
			if _, ok := inline[name]; !ok {
				t.Errorf("common.entity_inline.%s is missing in %s — %q not found is unreachable there", name, locale, name)
			}
		}
	}
}
