package api

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// Stable error codes for HTTP responses, and the helper that emits them.
//
// The governing rule (plan 81 §1): the server never owns a translation
// catalogue for anything a client can render. An error response is read by a
// browser that has the locale files loaded, so the server emits a stable
// `code` plus a closed set of `params` and the browser renders the sentence
// from `common.error.<code>`. The English text stays in Go — see errorMessages
// below — because `message` has to keep working for readers that have no
// catalogue at all.
//
// # Why `message` stays populated
//
// internal/isms/client/client.go:97 does
//
//	return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
//
// — it dumps the raw response body into CLI error output. Emptying `message`,
// or replacing it with a bare code, breaks every CLI error message and every
// server log line at once. `code` and `params` are therefore purely additive:
// a client that ignores them behaves exactly as it does today.
//
// # Wire shape
//
//	{"message": "risk not found", "code": "not_found", "params": {"entity": "risk"}}
//
// This is flat, not nested under `message`, and that is load-bearing. Echo's
// DefaultHTTPErrorHandler wraps a *string* Message in echo.Map{"message": m}
// but serialises any other non-error, non-json.Marshaler value as the response
// body directly (echo.go, `switch m := he.Message.(type)`). So errorBody must
// NOT gain an Error() or MarshalJSON() method — either one silently collapses
// the response back to `{"message": "..."}` and takes `code` with it.
// TestErrorBodyWireShape pins this against an echo instance rather than
// against the struct, so an Echo upgrade that changes the switch fails here.
//
// # Migration
//
// apiError and echo.NewHTTPError coexist. Nothing is converted in one go;
// call sites move per module (plan 81 task 1.5), and an unconverted site is
// indistinguishable to old clients from a converted one.

// Codes. Declared as constants so the catalogue is greppable and a typo is a
// compile error rather than a code the client has no message for.
//
// These strings are part of the wire contract. Renaming one silently drops a
// client back to the English `message` — no error anywhere — so treat them as
// frozen once shipped, the same way notification wire keys are (see
// notification_keys.go).
const (
	// Parameterised families. Three patterns absorb most of the tail: of the
	// 328 distinct literals measured across this package, ~110 are
	// "<entity> not found", ~20 are "invalid <entity> id" and ~8 are
	// "<field> is required".
	CodeNotFound        = "not_found"
	CodeNotFoundInOrg   = "not_found_in_org"
	CodeInvalidEntityID = "invalid_entity_id"
	CodeRequired        = "required"
	CodeFieldEmpty      = "field_empty"

	// Malformed input.
	CodeInvalidID      = "invalid_id"
	CodeInvalidRequest = "invalid_request"
	CodeInvalidRole    = "invalid_role"
	CodeInvalidStatus  = "invalid_status"

	// Authentication.
	CodeInvalidCredentials     = "invalid_credentials"
	CodeInvalidToken           = "invalid_token"
	CodeInvalidOTPCode         = "invalid_otp_code"
	CodeInvalidConfirmLink     = "invalid_confirm_link"
	CodeTokenEmailMismatch     = "token_email_mismatch"
	CodeNotAuthenticated       = "not_authenticated"
	CodeTooManyAttempts        = "too_many_attempts"
	CodePasswordTooShort       = "password_too_short"
	CodeCurrentPasswordNeeded  = "current_password_required"
	CodeCurrentPasswordWrong   = "current_password_incorrect"
	CodeEmailAndPasswordNeeded = "email_and_password_required"
	CodeEmailInUse             = "email_in_use"
	CodeNotYourPasskey         = "not_your_passkey"
	CodeAPIKeyRequired         = "api_key_required"
	CodeAPIKeyWrongOrg         = "api_key_wrong_org"

	// Organization membership and authorization.
	CodeNotOrgMember           = "not_org_member"
	CodeProviderOtherOrg       = "provider_other_org"
	CodeManagerOrAdminRequired = "manager_or_admin_required"

	// Documents and reviews.
	CodeDocumentModified     = "document_modified"
	CodeDocumentFileNotFound = "document_file_not_found"
	CodeNotesRequired        = "notes_required"
	CodeReviewWrongStatus    = "review_wrong_status"
	CodeReviewAlreadyStatus  = "review_already_status"

	// Organization settings.
	CodeAIDisabled        = "ai_disabled"
	CodeUnsupportedLocale = "unsupported_locale"
)

// errorMessages is the English rendering of every code, and — because the
// placeholders are read back out of it — the declaration of which params each
// code carries. #244 needed a hand-maintained param table because notification
// params are assembled conditionally at each call site; here the template is
// the contract, so the two cannot drift.
//
// Placeholders use vue-i18n's `{name}` syntax deliberately: the parity test
// diffs placeholder sets between these templates and `common.error.*` in
// web/src/locales/en/common.json, which only works if both sides spell them
// the same way.
//
// Only codes appearing at two or more call sites are catalogued here. That
// boundary is deliberate: a code invented for a single sentence nobody has
// converted yet is a guess about wording that task 1.5 will revise. Converting
// a single-occurrence literal means adding its code and its `common.error.*`
// key in the same commit, which the parity test enforces.
var errorMessages = map[string]string{
	CodeNotFound:        "{entity} not found",
	CodeNotFoundInOrg:   "{entity} not found in this organization",
	CodeInvalidEntityID: "invalid {entity} id",
	CodeRequired:        "{field} is required",
	CodeFieldEmpty:      "{field} cannot be empty",

	CodeInvalidID:      "invalid id",
	CodeInvalidRequest: "invalid request",
	CodeInvalidRole:    "invalid role",
	CodeInvalidStatus:  "invalid status: {value}",

	CodeInvalidCredentials:     "invalid credentials",
	CodeInvalidToken:           "invalid token",
	CodeInvalidOTPCode:         "invalid OTP code",
	CodeInvalidConfirmLink:     "invalid or expired confirmation link",
	CodeTokenEmailMismatch:     "token email mismatch",
	CodeNotAuthenticated:       "not authenticated",
	CodeTooManyAttempts:        "too many attempts, try again later",
	CodePasswordTooShort:       "password must be at least {count} characters",
	CodeCurrentPasswordNeeded:  "current password required",
	CodeCurrentPasswordWrong:   "current password is incorrect",
	CodeEmailAndPasswordNeeded: "email and password required",
	CodeEmailInUse:             "that email address is already in use",
	CodeNotYourPasskey:         "not your passkey",
	CodeAPIKeyRequired:         "API key required. Use: isms server api-key create",
	CodeAPIKeyWrongOrg:         "API key is not authorized for this organization",

	CodeNotOrgMember:           "not a member of this organization",
	CodeProviderOtherOrg:       "provider belongs to another organization",
	CodeManagerOrAdminRequired: "requires manager or admin role",

	CodeDocumentModified:     "the document was modified by another user — please refresh and try again",
	CodeDocumentFileNotFound: "document file not found",
	CodeNotesRequired:        "notes are required — describe what was reviewed and confirmed",
	CodeReviewWrongStatus:    "review is {status}",
	CodeReviewAlreadyStatus:  "review is already {status}",

	CodeAIDisabled:        "AI features are disabled for this organization",
	CodeUnsupportedLocale: "unsupported locale",
}

// The closed param set. An open-ended bag makes the client's translate-params
// step unenforceable: the renderer has to know, for every param name, whether
// the value is an identifier that needs a catalogue lookup or a value to
// splice in raw. Five names, each with exactly one rule:
//
//	entity -> common.entity_inline.<value>
//	field  -> common.field.<value>
//	status -> common.enum_inline.status.<value>
//	count  -> spliced raw; it is a number, not an identifier
//	value  -> spliced raw; it is the caller's own input echoed back
//
// `status` is not an arbitrary name — it is the name of an existing enum group
// in common.json, which is what lets the renderer derive the lookup path from
// the param name with no second mapping. That is the same convention #244
// settled on for notification params.
//
// `value` exists because the lookup-or-raw question has two answers and the
// first four names only covered one of them. "invalid status: draught" echoes
// what the caller sent so they can see their typo; that string is by
// construction NOT an enum member, so routing it through `status` would mean
// every lookup misses and an unbounded caller-supplied string is spliced into
// an otherwise translated sentence — the leak this catalogue exists to
// prevent, dressed as a translation. Two rules, not one, is the honest model;
// a sixth name is still a design decision rather than a call-site
// convenience.
const (
	ParamEntity = "entity"
	ParamField  = "field"
	ParamStatus = "status"
	ParamCount  = "count"
	ParamValue  = "value"
)

// errorStatusValues declares which `status` members each code carrying that
// param can actually send.
//
// This is the half TestParamValuesHaveCatalogueKeys cannot reach. That test
// reads `Status("…")` literals out of the source, and no real call site has
// one — they all pass a variable (`review.Status`). So for the one param whose
// values are least predictable, the source scan is vacuous. Declaring the
// members here restores the check, the same way #244 declared notification
// params per key rather than trusting a package-wide vocabulary.
//
// Hand-maintained, and pinned by TestErrorStatusValuesCoverEveryStatusCode so
// a new status-carrying code cannot ship without an entry.
var errorStatusValues = map[string][]string{
	// review.Status, whose state machine is documented in CLAUDE.md:
	// open -> approved | changes_requested -> open, approved -> merged, and
	// any active status -> closed. `draft` is reachable through the document
	// status the same field carries at :912.
	CodeReviewWrongStatus:   {"draft", "open", "in_review", "approved", "changes_requested", "merged", "closed"},
	CodeReviewAlreadyStatus: {"approved", "changes_requested", "merged", "closed"},
}

// ErrorParam is a single param, constructible only through the four functions
// below. This is what makes the closed set a compile-time property rather than
// a comment: apiError takes ...ErrorParam, not ...any, so a call site cannot
// invent a name.
type ErrorParam struct {
	Key   string
	Value string
}

// Entity names what the error is about: "risk", "corrective_action". The value
// is the snake_case identifier, not display text — the client looks it up in
// common.entity_inline.*, and every value used here must have a key there
// (TestErrorEntityValuesHaveKeys).
func Entity(v string) ErrorParam { return ErrorParam{ParamEntity, v} }

// Field names an input field, using the JSON field name the API accepts
// ("title", "path_pattern", "new_email") so the message points at something
// the caller actually sent.
func Field(v string) ErrorParam { return ErrorParam{ParamField, v} }

// Status is a member of the `status` enum group.
func Status(v string) ErrorParam { return ErrorParam{ParamStatus, v} }

// Count is a bare number — a length, a limit, a threshold. Never translated.
func Count(n int) ErrorParam { return ErrorParam{ParamCount, strconv.Itoa(n)} }

// Value echoes the caller's own input back at them: the rejected status, the
// unparseable ref. Never translated and never looked up, because it is not
// drawn from any set the catalogue could enumerate. Use it only for input the
// caller supplied; anything drawn from a known set belongs in Entity, Field or
// Status, where the client can translate it.
func Value(v string) ErrorParam { return ErrorParam{ParamValue, v} }

// errorBody is the response payload. It must not implement error or
// json.Marshaler; see the wire-shape note at the top of this file.
type errorBody struct {
	Message string            `json:"message"`
	Code    string            `json:"code"`
	Params  map[string]string `json:"params,omitempty"`
}

// String keeps *echo.HTTPError.Error() readable: it formats Message with %v,
// which reaches this method rather than dumping the struct. Server logs and
// test failures then read "code=404, message=risk not found" exactly as they
// did before.
func (b errorBody) String() string { return b.Message }

var placeholderRE = regexp.MustCompile(`\{([^{}]+)\}`)

// apiError builds an *echo.HTTPError carrying a stable code alongside the
// English message, which is rendered here from the code's template.
//
// An unknown code, or a param the template has no slot for, is a programming
// error that the tests in this package catch at build time. At runtime it
// degrades rather than panicking — an operator losing a word from an error
// message is better than an operator losing the request — so an unknown code
// falls back to the code itself as the message, and a surplus param is
// dropped from the message but kept on the wire.
func apiError(status int, code string, params ...ErrorParam) *echo.HTTPError {
	msg, ok := errorMessages[code]
	if !ok {
		msg = code
	}

	var p map[string]string
	for _, param := range params {
		if p == nil {
			p = make(map[string]string, len(params))
		}
		p[param.Key] = param.Value
		// The message is English prose, so it wants "corrective action",
		// while the wire keeps the identifier the client looks up. Both come
		// from the same call, which is the point: the two cannot disagree.
		msg = strings.ReplaceAll(msg, "{"+param.Key+"}", humanizeParam(param))
	}

	return echo.NewHTTPError(status, errorBody{Message: msg, Code: code, Params: p})
}

// humanizeParam turns a snake_case identifier into the English words the
// message template expects. Raw params pass through untouched.
//
// The de-slug is not enough on its own: `oidc_provider` becomes "oidc
// provider", quietly regressing the "OIDC provider not found" that
// api_oidc.go and api_admin.go ship today. The locale file authors the inline
// form correctly, so without the overrides below the browser would be right
// and the CLI wrong — one code, one language, two texts. Same failure for any
// identifier whose English form is not derivable from its spelling, which in
// practice means acronyms.
func humanizeParam(p ErrorParam) string {
	switch p.Key {
	case ParamCount, ParamValue:
		return p.Value
	}
	if override, ok := paramMessageOverrides[p.Value]; ok {
		return override
	}
	return strings.ReplaceAll(p.Value, "_", " ")
}

// paramMessageOverrides is the English prose for identifiers the de-slug gets
// wrong. Kept small on purpose: an entry here is a claim that Go and the `en`
// locale file agree, which TestOverridesMatchTheEnglishCatalogue checks rather
// than assumes.
var paramMessageOverrides = map[string]string{
	"oidc_provider": "OIDC provider",
	"api_key":       "API key",
	"document_id":   "document ID",
}

// ErrorCodes is every code this build can emit, sorted. Derived from
// errorMessages rather than maintained beside it, so the two cannot fall out
// of step the way a hand-written slice would.
func ErrorCodes() []string {
	codes := make([]string, 0, len(errorMessages))
	for code := range errorMessages {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// ErrorCodeParams reports the param names a code carries, read out of its
// English template. This is the declaration the parity test compares against
// web/src/locales/en/common.json: a translation that invents a slot the
// English sentence does not have renders it blank, and a translation missing a
// slot drops whatever it named.
func ErrorCodeParams(code string) []string {
	msg, ok := errorMessages[code]
	if !ok {
		return nil
	}
	var names []string
	for _, m := range placeholderRE.FindAllStringSubmatch(msg, -1) {
		names = append(names, m[1])
	}
	sort.Strings(names)
	return names
}

// Convenience wrappers for the three families that carry most of the traffic.
// They exist so the common case reads as one call rather than three arguments
// of ceremony, and so the status code cannot be got wrong per site.
func errNotFound(entity string) *echo.HTTPError {
	return apiError(http.StatusNotFound, CodeNotFound, Entity(entity))
}

func errInvalidEntityID(entity string) *echo.HTTPError {
	return apiError(http.StatusBadRequest, CodeInvalidEntityID, Entity(entity))
}

func errRequired(field string) *echo.HTTPError {
	return apiError(http.StatusBadRequest, CodeRequired, Field(field))
}

var _ fmt.Stringer = errorBody{}
