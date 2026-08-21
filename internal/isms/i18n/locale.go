// Package i18n holds the locale primitives shared by every server-side render
// path (email, Slack/Matrix) and by the API surface that tells the web app which
// locales exist.
//
// This package is deliberately the ONLY place the supported-locale set is
// declared. The web app reads it from GET /api/v1/config rather than keeping its
// own list, so the two cannot drift — a locale added here becomes selectable in
// the UI without a second edit.
//
// Scope note: this package does not translate anything. The server owns a
// translation catalog only for artifacts it pushes outward with no client
// present (email, chat notifications). Everything a client can render — HTTP
// error messages, in-app notifications — stays keyed and is rendered by the
// client from the web locale files.
package i18n

import (
	"sort"
	"strings"
)

// Default is the fallback locale. It must always be a member of supported: it is
// the last resort of every resolution path, and the locale the web bundle ships
// resident rather than lazy-loading.
const Default = "en"

// supported maps a canonical locale tag to its endonym — the language's name in
// itself, which is what a locale picker should display. A user who has landed in
// the wrong locale cannot read "Portuguese (Brazil)"; they can read
// "Português (Brasil)".
//
// Keys are canonical BCP 47 tags and are matched case-insensitively on input.
// Adding a locale here is the entire server-side cost of supporting it.
var supported = map[string]string{
	"en":    "English",
	"pt-BR": "Português (Brasil)",
}

// Locale is one selectable locale, as exposed to clients.
type Locale struct {
	Tag  string `json:"tag"`  // canonical BCP 47 tag, e.g. "pt-BR"
	Name string `json:"name"` // endonym, for display in a picker
}

// Supported returns every selectable locale, Default first and the remainder
// sorted by tag. The ordering is stable so the API response and any UI built
// from it do not shuffle between requests.
func Supported() []Locale {
	out := make([]Locale, 0, len(supported))
	for tag, name := range supported {
		if tag == Default {
			continue
		}
		out = append(out, Locale{Tag: tag, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tag < out[j].Tag })
	return append([]Locale{{Tag: Default, Name: supported[Default]}}, out...)
}

// IsSupported reports whether tag names a supported locale, case-insensitively.
func IsSupported(tag string) bool {
	_, ok := Canonical(tag)
	return ok
}

// Canonical resolves an arbitrary locale tag to its canonical supported form,
// reporting whether a match was found.
//
// Matching is deliberately region-tolerant in both directions, because the tags
// clients send rarely match a catalog exactly:
//
//	"PT-br"  -> "pt-BR"  (case difference only)
//	"pt"     -> "pt-BR"  (bare language, one supported region)
//	"pt-PT"  -> "pt-BR"  (different region, same language)
//	"en-US"  -> "en"     (region we do not distinguish)
//
// The pt-PT case is a judgement call worth stating: European Portuguese is not
// Brazilian Portuguese, but serving a Portuguese speaker pt-BR is far better
// than serving them English. If pt-PT is ever added, the exact match wins and
// this fallback stops applying to it.
func Canonical(tag string) (string, bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", false
	}
	// Normalize separator: BCP 47 uses "-", but Unix locale strings and some
	// Accept-Language headers in the wild use "_" (pt_BR).
	tag = strings.ReplaceAll(tag, "_", "-")

	// Reject malformed input BEFORE any fallback. Without this the primary-subtag
	// fallback below is far too eager: it only ever looks at the text before the
	// first "-", so "pt-BR-2", "pt-Latn-BR-x-junk" and even "pt-!!!" all reduce to
	// "pt" and resolve to pt-BR. That silently defeats the 400 that the write
	// paths (PUT /auth/profile, PUT /admin/settings) depend on for anything that
	// is not a well-formed tag.
	if !wellFormed(tag) {
		return "", false
	}

	// Exact match, case-insensitive.
	for s := range supported {
		if strings.EqualFold(s, tag) {
			return s, true
		}
	}

	// Language-only match: fall back to a supported locale sharing the primary
	// subtag. Iterate the sorted set rather than the map so that a language with
	// several supported regions resolves deterministically instead of picking a
	// different one per request.
	//
	// The fallback applies ONLY to simple tags — "pt" or "pt-PT", at most a
	// language and one region. That restriction is the substance of the fix, not
	// the syntax check above: "pt-Latn-BR-x-junk" is a perfectly well-formed BCP 47
	// tag, so shape validation alone still let it reduce to "pt" and resolve to
	// pt-BR. A caller naming a script, variant or extension is asking for something
	// specific; answering with pt-BR would be a guess, and for the write paths it
	// would mean storing a value the caller never asked for. Exact match or nothing.
	parts := strings.Split(tag, "-")
	if len(parts) > 2 {
		return "", false
	}
	lang := strings.ToLower(parts[0])
	if lang == "" {
		return "", false
	}
	for _, l := range Supported() {
		cand, _, _ := strings.Cut(strings.ToLower(l.Tag), "-")
		if cand == lang {
			return l.Tag, true
		}
	}
	return "", false
}

// wellFormed reports whether tag has the shape of a BCP 47 language tag. It is a
// syntax check, not a registry check — whether the language actually exists is
// decided by matching against the supported set.
//
// Deliberately stricter than RFC 5646's full grammar, which also admits grandfathered
// and private-use ("x-...") forms. Those cannot name a supported locale here, so
// accepting them would only widen what reaches the fallback. The shape enforced is:
//
//	primary subtag  2-3 alpha (ISO 639-1/-2/-3) or 5-8 alpha (registered)
//	later subtags   1-8 alphanumeric
//	at most 6 subtags total
//
// Note the 4-alpha primary subtag is excluded: it is reserved in RFC 5646 and no
// real language uses it.
func wellFormed(tag string) bool {
	parts := strings.Split(tag, "-")
	if len(parts) == 0 || len(parts) > 6 {
		return false
	}
	primary := parts[0]
	if n := len(primary); n < 2 || n > 8 || n == 4 {
		return false
	}
	if !allAlpha(primary) {
		return false
	}
	for _, p := range parts[1:] {
		if len(p) < 1 || len(p) > 8 || !allAlphaNum(p) {
			return false
		}
	}
	return true
}

func allAlpha(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

func allAlphaNum(s string) bool {
	for _, r := range s {
		alpha := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		digit := r >= '0' && r <= '9'
		if !alpha && !digit {
			return false
		}
	}
	return true
}

// Resolve picks the locale to render in, given the two stored preferences.
// This is the single resolution chain — every server-side render path must call
// this rather than reading a preference directly, so that precedence stays
// consistent across email, chat and API responses.
//
// Precedence, highest first:
//
//  1. the user's explicit choice (users.locale)
//  2. the organization default (the default_locale org setting)
//  3. Default
//
// Both arguments may be empty or hold an unsupported value — a locale that was
// valid when stored can stop being supported, so every tier is re-validated
// rather than trusted.
func Resolve(userLocale, orgLocale string) string {
	for _, candidate := range []string{userLocale, orgLocale} {
		if tag, ok := Canonical(candidate); ok {
			return tag
		}
	}
	return Default
}

// FromAcceptLanguage picks the best supported locale from an Accept-Language
// header, for requests with no authenticated user to read a preference from —
// login, signup and email-verification pages, which are rendered before anyone
// has an account.
//
// Quality values are honoured, and entries are sorted by descending q so that
// "pt;q=0.9, en;q=0.5" prefers Portuguese regardless of header order. Returns
// ("", false) when nothing matches, letting the caller fall through to an org
// default rather than forcing Default here.
func FromAcceptLanguage(header string) (string, bool) {
	type candidate struct {
		tag string
		q   float64
		pos int // header order, to keep equal-q entries stable
	}
	var cands []candidate

	for i, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tag, params, _ := strings.Cut(part, ";")
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		// "*" means "anything", which tells us nothing about preference.
		if tag == "*" {
			continue
		}
		q := 1.0
		if params != "" {
			// Parameter names are case-insensitive per RFC 9110, so a literal "q="
			// search misses "Q=0.5" and leaves q at its 1.0 default — which would
			// make an explicitly refused locale (Q=0) outrank an accepted one.
			if qv, ok := cutQ(params); ok {
				q = parseQ(qv)
			}
		}
		// q=0 explicitly means "not acceptable".
		if q <= 0 {
			continue
		}
		cands = append(cands, candidate{tag: tag, q: q, pos: i})
	}

	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].q != cands[j].q {
			return cands[i].q > cands[j].q
		}
		return cands[i].pos < cands[j].pos
	})

	for _, c := range cands {
		if tag, ok := Canonical(c.tag); ok {
			return tag, true
		}
	}
	return "", false
}

// cutQ finds the quality parameter in an Accept-Language entry's parameter list,
// matching the "q" name case-insensitively, and returns its raw value.
func cutQ(params string) (string, bool) {
	for _, p := range strings.Split(params, ";") {
		name, value, found := strings.Cut(p, "=")
		if !found {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "q") {
			return value, true
		}
	}
	return "", false
}

// parseQ parses an Accept-Language quality value without pulling in strconv
// error handling at every call site. A malformed q is treated as 0 (unusable)
// rather than 1, so a broken header cannot outrank a well-formed one.
func parseQ(s string) float64 {
	s = strings.TrimSpace(s)
	// Quality values are "0", "1", or a decimal with at most 3 places. Hand-roll
	// the parse: the grammar is tiny and this avoids a strconv import solely for
	// a value we clamp anyway.
	var whole, frac float64
	var scale float64 = 1
	seenDot := false
	for _, r := range s {
		switch {
		case r == '.' && !seenDot:
			seenDot = true
		case r >= '0' && r <= '9':
			if seenDot {
				scale /= 10
				frac += float64(r-'0') * scale
			} else {
				whole = whole*10 + float64(r-'0')
			}
		default:
			return 0 // any other character makes the value malformed
		}
	}
	q := whole + frac
	if q < 0 || q > 1 {
		return 0
	}
	return q
}
