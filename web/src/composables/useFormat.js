// The single seam for dates, numbers and relative time.
//
// `toLocaleDateString()` scattered through components silently formats in the
// browser's locale, not the one the user chose, and hardcoded option bags drift
// apart. Everything routes through here so the active vue-i18n locale is the
// only input, and Intl does the locale-specific work — no date-formatting
// strings ever land in a translation file.
import { i18n } from '../i18n.js'

// Intl constructors are comparatively expensive; the same few shapes are used
// on every table row, so memoize per (locale, options).
const cache = new Map()
function formatter(Ctor, kind, locale, options) {
  const key = `${kind}|${locale}|${JSON.stringify(options)}`
  let f = cache.get(key)
  if (!f) {
    f = new Ctor(locale, options)
    cache.set(key, f)
  }
  return f
}

function activeLocale() {
  return i18n.global.locale.value
}

// Accepts a Date, an epoch number, or an ISO string (the API's wire format).
// Returns null for anything unparseable so callers render an em dash rather
// than "Invalid Date".
function toDate(value) {
  if (value === null || value === undefined || value === '') return null
  const d = value instanceof Date ? value : new Date(value)
  return Number.isNaN(d.getTime()) ? null : d
}

const DATE_STYLES = {
  short: { dateStyle: 'medium' }, // "24 Aug 2026" — the table default
  long: { dateStyle: 'long' },
  datetime: { dateStyle: 'medium', timeStyle: 'short' },
}

export function formatDate(value, style = 'short') {
  const d = toDate(value)
  if (!d) return ''
  const options = DATE_STYLES[style] ?? DATE_STYLES.short
  return formatter(Intl.DateTimeFormat, 'date', activeLocale(), options).format(d)
}

export function formatNumber(value, options = {}) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return ''
  return formatter(Intl.NumberFormat, 'number', activeLocale(), options).format(value)
}

// Thresholds walked largest-first, so 90 minutes reads "1 hour ago" rather than
// "90 minutes ago". Intl.RelativeTimeFormat supplies the wording, so no
// "x ago" template needs translating.
const UNITS = [
  ['year', 365 * 24 * 60 * 60 * 1000],
  ['month', 30 * 24 * 60 * 60 * 1000],
  ['week', 7 * 24 * 60 * 60 * 1000],
  ['day', 24 * 60 * 60 * 1000],
  ['hour', 60 * 60 * 1000],
  ['minute', 60 * 1000],
]

export function formatRelative(value, now = Date.now()) {
  const d = toDate(value)
  if (!d) return ''
  const diff = d.getTime() - now
  const rtf = formatter(Intl.RelativeTimeFormat, 'relative', activeLocale(), {
    numeric: 'auto', // "yesterday" beats "1 day ago" where the locale has a word
  })
  for (const [unit, ms] of UNITS) {
    if (Math.abs(diff) >= ms) return rtf.format(Math.round(diff / ms), unit)
  }
  return rtf.format(Math.round(diff / 1000), 'second')
}

export function useFormat() {
  return { date: formatDate, number: formatNumber, relative: formatRelative }
}
