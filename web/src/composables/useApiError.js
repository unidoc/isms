// The render seam for API errors.
//
// The server emits a stable `code` and a closed param set alongside an English
// `message` (internal/isms/api/errors.go). The English text stays authoritative
// for readers with no catalogue — the CLI dumps the raw response body, and
// server logs read the same string — so translation happens here, on the one
// side that has the locale files loaded.
//
// Every failure mode falls back to `err.message`, which is the English sentence
// the server already composed: an error thrown before any response (network),
// a route still on echo.NewHTTPError, a code this bundle has no key for. That
// makes adoption safe at any pace — a converted route and an unconverted one
// both render something correct.
import { FALLBACK, i18n } from '../i18n.js'
import { enumLabelInline, entityLabel } from './useEnumLabel.js'

// How each param name resolves. The set is closed on the Go side
// (ParamEntity/ParamField/ParamStatus/ParamCount/ParamValue), and each name has
// exactly one rule — which is the property that makes this table possible
// rather than a per-code mapping.
//
// `count` and `value` are spliced raw and so are absent here: count is a
// number, and value is the caller's own input echoed back ("invalid status:
// draught"), which by construction is not a member of any set the catalogue
// could enumerate.
const PARAM_RESOLVERS = {
  entity: (v) => entityLabel(v, { inline: true }),
  status: (v) => enumLabelInline('status', v),
  field: (v) => fieldLabel(v),
}

// Field names are the API's own JSON field names ("title", "path_pattern"), so
// there is no shared composable for them the way there is for entities and
// enums — the lookup lives here.
//
// te() probes FALLBACK rather than the active locale, the same reasoning as
// enumLabel() and useNotificationRender(): the fallback catalogue is complete
// by contract, so a lagging locale renders the English word through
// fallbackLocale instead of dropping to the raw identifier.
function fieldLabel(value) {
  const key = `common.field.${value}`
  return i18n.global.te(key, FALLBACK) ? i18n.global.t(key) : value
}

// Params are translated BEFORE interpolation. Splicing the English `entity:
// "risk"` into a Portuguese frame puts an English noun inside a translated
// sentence — worse than either language alone, and not something fallbackLocale
// can rescue, because the sentence around it translated fine.
function resolveParams(params) {
  const out = {}
  for (const [name, value] of Object.entries(params ?? {})) {
    const resolve = PARAM_RESOLVERS[name]
    out[name] = resolve ? resolve(value) : value
  }
  return out
}

// renderApiError turns a thrown API error into display text.
//
// Takes the error, not a code, so a call site can hand it whatever it caught
// without inspecting it first — including a network error with no `code` at
// all.
export function renderApiError(err) {
  if (!err) return ''
  const message = err.message ?? ''
  if (!err.code) return message
  const key = `common.error.${err.code}`
  if (!i18n.global.te(key, FALLBACK)) return message
  return i18n.global.t(key, resolveParams(err.params))
}

export function useApiError() {
  return { renderApiError }
}
