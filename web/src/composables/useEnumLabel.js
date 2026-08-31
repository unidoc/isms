// The single seam for turning a stored enum value into display text.
//
// Enum values are database identifiers — `changes_requested`, `in_review`,
// `very_high`. De-slugging them inline (replace('_',' ') + capitalize) reads
// fine in English and is untranslatable everywhere else, so every label comes
// from the message catalogue instead: `common.enum.<group>.<value>`.
//
// A value with no key falls back to the de-slugged form rather than rendering a
// raw key, so a locale file that lags behind a new enum member degrades to
// today's English-ish output instead of showing `common.enum.status.foo`.
import { FALLBACK, i18n } from '../i18n.js'

// De-slug of last resort: `changes_requested` -> `Changes requested`.
function humanize(value) {
  const s = String(value).replace(/[_-]+/g, ' ').trim()
  return s ? s.charAt(0).toUpperCase() + s.slice(1) : ''
}

// group: the enum family, e.g. 'status', 'severity', 'likelihood'.
// value:  the stored member, e.g. 'in_review'.
export function enumLabel(group, value) {
  if (value === null || value === undefined || value === '') return ''
  const key = `common.enum.${group}.${value}`
  // te() checks one locale only and does not consult fallbackLocale, so probe
  // the fallback catalogue: it is complete by contract, and a locale that lags
  // behind then renders the English label through fallbackLocale rather than
  // dropping to humanize(). The difference is visible — `major_nc` is "Major
  // non-conformity", not "Major nc".
  return i18n.global.te(key, FALLBACK) ? i18n.global.t(key) : humanize(value)
}

// Mid-sentence form of the same label.
//
// `common.enum.*` holds the standalone label — what a badge or an <option>
// shows, capitalised because it starts its own line. Interpolated into a frame
// it is wrong: "New Critical incident", "Incident Resolved: …". The inline form
// is a separate authored string, not a transformation of the standalone one:
// which words a language capitalises mid-sentence is a property of that
// language (German capitalises every noun), so deriving one from the other is
// the same locale-blind munging that `common.enum.*` exists to replace.
//
// Falls back to the standalone label, so a group with no inline forms authored
// still renders — today's output, not a raw key.
export function enumLabelInline(group, value) {
  if (value === null || value === undefined || value === '') return ''
  const key = `common.enum_inline.${group}.${value}`
  return i18n.global.te(key, FALLBACK) ? i18n.global.t(key) : enumLabel(group, value)
}

// Entity names run through common.entity.* rather than common.enum.*, and need
// the same two forms for the same reason.
export function entityLabel(value, { inline = false } = {}) {
  if (value === null || value === undefined || value === '') return ''
  if (inline) {
    const key = `common.entity_inline.${value}`
    if (i18n.global.te(key, FALLBACK)) return i18n.global.t(key)
  }
  const key = `common.entity.${value}`
  return i18n.global.te(key, FALLBACK) ? i18n.global.t(key) : value
}

// Component-facing form. Deliberately thin: the implementation lives in
// enumLabel() so plain .js modules can use the same seam without a component
// instance, exactly as `t` is exported from @/i18n for the same reason.
export function useEnumLabel() {
  return { enumLabel, enumLabelInline, entityLabel }
}
