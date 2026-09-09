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

// The app's English is British — every call site this seam replaces was
// hardcoded to `en-GB`, and bare `en` resolves to US ordering ("Aug 24, 2026"
// against "24 Aug 2026"). The message bundle is tagged `en` and stays that way;
// only the Intl formatting tag is widened, so adopting the seam changes no
// English user's date shape.
const FORMAT_LOCALE = { en: 'en-GB' }

function activeLocale() {
  const tag = i18n.global.locale.value
  return FORMAT_LOCALE[tag] ?? tag
}

// Anything below this is read as epoch seconds, anything above as epoch
// milliseconds. The API writes seconds (`created_at: 1756...`) while
// `Date.now()` arithmetic produces milliseconds, and both reach these helpers.
// 1e11 separates them with no ambiguity in practice: as milliseconds it is
// 1973, as seconds it is the year 5138.
const SECONDS_CEILING = 1e11

// Accepts a Date, an epoch number (seconds or milliseconds, per above), or an
// ISO string (the API's other wire format). Returns null for anything
// unparseable so callers render an em dash rather than "Invalid Date".
function toDate(value) {
  if (value === null || value === undefined || value === '') return null
  let d
  if (value instanceof Date) d = value
  else if (typeof value === 'number') d = new Date(Math.abs(value) < SECONDS_CEILING ? value * 1000 : value)
  else d = new Date(value)
  return Number.isNaN(d.getTime()) ? null : d
}

// `dateStyle` cannot drop the year, so the two year-less shapes the UI uses
// (comment timelines, audit date ranges) are spelled out as component bags.
// Component order is still the locale's, not the bag's.
const DATE_STYLES = {
  short: { dateStyle: 'medium' }, // "24 Aug 2026" — the table default
  long: { dateStyle: 'long' },
  datetime: { dateStyle: 'medium', timeStyle: 'short' },
  dayMonth: { day: 'numeric', month: 'short' }, // "24 Aug"
  dayMonthTime: { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' },
}

export function formatDate(value, style = 'short') {
  const d = toDate(value)
  if (!d) return ''
  const options = DATE_STYLES[style] ?? DATE_STYLES.short
  return formatter(Intl.DateTimeFormat, 'date', activeLocale(), options).format(d)
}

// A calendar date, not an instant. Postgres DATE columns (a due date, a next
// review) come over the wire as epoch seconds at midnight UTC, so rendering
// them in the viewer's zone moves the day backwards for everyone west of UTC —
// a 5 Aug due date reads 4 Aug in Sao Paulo. Pinning the formatter to UTC keeps
// the day the one that was entered, which is the only meaning a DATE has.
// Use this for a DATE column and formatDate() for a timestamp; the wire format
// is identical, so the distinction cannot be recovered from the value.
export function formatDay(value, style = 'short') {
  const d = toDate(value)
  if (!d) return ''
  const options = DATE_STYLES[style] ?? DATE_STYLES.short
  return formatter(Intl.DateTimeFormat, 'date', activeLocale(), { ...options, timeZone: 'UTC' }).format(d)
}

// Short month names for the annual-plan grid, which labels twelve columns with
// no date to format — so `formatDate` cannot serve it. Intl supplies the names,
// which is why no `dashboard.month.jan` keys exist: month names are calendar
// data every locale already carries, not copy anyone should be asked to
// translate.
//
// Reads the active locale on every call rather than caching the array, so a
// `computed()` over it re-evaluates when the locale switches.
export function formatMonthShort(monthIndex) {
  // Deliberately not Number(monthIndex): `Number(null)` and `Number('')` are
  // both 0, which would render an absent value as January rather than as the
  // empty label every other helper here returns for unusable input.
  if (!Number.isInteger(monthIndex) || monthIndex < 0 || monthIndex > 11) return ''
  const i = monthIndex
  // Any year works; 2021 is a non-leap year, so no month is a special case.
  return formatter(Intl.DateTimeFormat, 'date', activeLocale(), {
    month: 'short',
    timeZone: 'UTC',
  }).format(Date.UTC(2021, i, 1))
}

// The same, spelled out — the audit calendar heads each month section with a
// full name. The server sends one too, in English; this reads the active
// locale instead.
export function formatMonthLong(monthIndex) {
  if (!Number.isInteger(monthIndex) || monthIndex < 0 || monthIndex > 11) return ''
  return formatter(Intl.DateTimeFormat, 'date', activeLocale(), {
    month: 'long',
    timeZone: 'UTC',
  }).format(Date.UTC(2021, monthIndex, 1))
}

// A duration in whole hours, for the RPO/RTO columns. Same reasoning as the
// date shapes: the unit is locale-specific ("4h" in English, "4 j" in French),
// so Intl supplies it and no unit letter lands in a translation file. Narrow
// display because these sit in a compact table column.
export function formatHours(value) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return ''
  return formatter(Intl.NumberFormat, 'number', activeLocale(), {
    style: 'unit',
    unit: 'hour',
    unitDisplay: 'narrow',
  }).format(value)
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

// Six components carried the same ladder by hand: relative wording for a recent
// timestamp, an absolute date once it is old enough to be worth pinning down.
// The threshold is a per-surface choice (a comment thread pins after a day, a
// document list after a week), so it stays a parameter; the wording does not.
export function formatRecent(value, { within = 7 * 24 * 60 * 60 * 1000, style = 'short' } = {}) {
  const d = toDate(value)
  if (!d) return ''
  return Math.abs(Date.now() - d.getTime()) < within ? formatRelative(d) : formatDate(d, style)
}

// Jurisdiction / region display, for values stored in
// `legal_requirements.jurisdiction`. Three tiers, and the order is load-bearing:
//
//  1. A region sentinel (`Global`, `EU`, `EEA`, `APAC`) resolves through
//     `common.region.*`. This MUST come first. `EU` is a real ISO 3166
//     exceptional reservation, so `Intl.DisplayNames` happily renders it
//     "European Union" — which is not what the picker means by it, and would
//     also throw away the translated endonym (id-ID says "UE").
//  2. An alpha-2 code goes to `Intl.DisplayNames`. The regex is a guard, not an
//     optimisation: `.of('Iceland')` throws `RangeError`, and rows holding a
//     legacy English name are exactly what tier 3 exists for.
//  3. Anything else renders verbatim. The column is free text — the CLI and
//     agent suggestions can write any string, historical changelog entries hold
//     the pre-migration English names permanently, and a row the migration did
//     not recognise must still be readable rather than rendering blank.
const SENTINEL_KEYS = {
  Global: 'common.region.global',
  EU: 'common.region.eu',
  EEA: 'common.region.eea',
  APAC: 'common.region.apac',
}

const ALPHA2 = /^[A-Z]{2}$/

export function regionLabel(value) {
  if (!value) return ''
  const key = SENTINEL_KEYS[value]
  if (key) return i18n.global.t(key)
  if (!ALPHA2.test(value)) return value
  // `of()` echoes an unrecognised code straight back, so a decommissioned or
  // invented code renders as itself rather than as an empty cell.
  try {
    return formatter(Intl.DisplayNames, 'region', activeLocale(), { type: 'region' }).of(value)
  } catch {
    return value
  }
}

export function useFormat() {
  return {
    date: formatDate,
    day: formatDay,
    monthShort: formatMonthShort,
    monthLong: formatMonthLong,
    hours: formatHours,
    number: formatNumber,
    relative: formatRelative,
    recent: formatRecent,
    region: regionLabel,
  }
}
