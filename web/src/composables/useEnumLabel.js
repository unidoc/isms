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
import { i18n } from '../i18n.js'

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
  return i18n.global.te(key) ? i18n.global.t(key) : humanize(value)
}

// Component-facing form. Deliberately thin: the implementation lives in
// enumLabel() so plain .js modules can use the same seam without a component
// instance, exactly as `t` is exported from @/i18n for the same reason.
export function useEnumLabel() {
  return { enumLabel }
}
