package api

import (
	"encoding/json"
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

func localesDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "web", "src", "locales")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("locales directory not found at %s: %v", dir, err)
	}
	return dir
}

// commonErrors returns the `common.error` group of one locale, flattened to
// code -> message. A nested value would mean somebody grouped codes, which the
// client's flat `common.error.<code>` lookup cannot reach, so that fails here
// rather than at render time.
func commonErrors(t *testing.T, locale string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(localesDir(t), locale, "common.json"))
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
	raw, err := os.ReadFile(filepath.Join(localesDir(t), locale, "common.json"))
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
	raw, err := os.ReadFile(filepath.Join(localesDir(t), locale, "common.json"))
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
	entries, err := os.ReadDir(localesDir(t))
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
	inline := enumInlineGroup(t, "en", "status")
	if len(inline) == 0 {
		t.Fatal("en common.enum_inline.status is empty")
	}
	for code, values := range errorStatusValues {
		for _, v := range values {
			if _, ok := inline[v]; !ok {
				t.Errorf("%s can send status %q, which has no common.enum_inline.status.%s key in en — enumLabelInline falls back to the capitalised standalone label mid-sentence", code, v, v)
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
func TestParamValuesHaveCatalogueKeys(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing package: %v", err)
	}

	entities := commonGroup(t, "en", "entity_inline")
	fields := commonGroup(t, "en", "field")
	statuses := enumInlineGroup(t, "en", "status")

	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					return true
				}
				ctor, ok := call.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					// A variable, which is the common case for Status. Those
					// values are covered by errorStatusValues instead.
					return true
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				pos := fset.Position(lit.Pos())
				switch ctor.Name {
				case "Entity":
					if _, ok := entities[value]; !ok {
						t.Errorf("%s:%d: Entity(%q) has no common.entity_inline.%s key in en — it would render as the raw identifier mid-sentence", filepath.Base(name), pos.Line, value, value)
					}
				case "Field":
					if _, ok := fields[value]; !ok {
						t.Errorf("%s:%d: Field(%q) has no common.field.%s key in en", filepath.Base(name), pos.Line, value, value)
					}
				case "Status":
					if _, ok := statuses[value]; !ok {
						t.Errorf("%s:%d: Status(%q) has no common.enum_inline.status.%s key in en", filepath.Base(name), pos.Line, value, value)
					}
				}
				return true
			})
		}
	}
}

// Errors interpolate mid-sentence ("risk not found"), so the entity param
// resolves through common.entity_inline.*, not common.entity.*. The existing
// enumCatalog.test.js already holds the two groups in step for en and mirrors
// entity_inline into id-ID; this asserts the group errors depend on is not
// empty, which is what would make the check above pass vacuously.
func TestEntityInlineCatalogueIsPopulated(t *testing.T) {
	inline := commonGroup(t, "en", "entity_inline")
	if len(inline) == 0 {
		t.Fatal("en common.entity_inline is empty")
	}
	for _, name := range []string{"risk", "review", "document", "user", "organization"} {
		if _, ok := inline[name]; !ok {
			t.Errorf("common.entity_inline.%s is missing in en — %q not found is unreachable", name, name)
		}
	}
}
