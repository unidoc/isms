// The two formatting seams from docs/i18n.md: no component formats a date or
// de-slugs an enum itself, so these are the only implementations to test.
import test from 'node:test'
import assert from 'node:assert/strict'
import { enumLabel } from '../src/composables/useEnumLabel.js'
import { formatDate, formatDay, formatNumber, formatRecent, formatRelative, useFormat } from '../src/composables/useFormat.js'
import { i18n } from '../src/i18n.js'

test('enum labels come from the catalogue, and de-slug only as a fallback', () => {
  i18n.global.mergeLocaleMessage('en', {
    common: { enum: { status: { in_review: 'In review' } } },
  })
  assert.equal(enumLabel('status', 'in_review'), 'In review')
  // A member the catalogue has not caught up with must not render a raw key.
  // Use a value no register defines: every real status is catalogued now, so a
  // real one would exercise the lookup rather than the fallback.
  assert.equal(enumLabel('status', 'awaiting_signoff'), 'Awaiting signoff')
  assert.equal(enumLabel('status', null), '')
  assert.equal(enumLabel('status', ''), '')
})

test('dates and numbers format through Intl, and reject unusable input', () => {
  const iso = '2026-08-24T10:30:00Z' // the API's wire format
  assert.match(formatDate(iso), /2026/)
  assert.match(formatDate(iso, 'datetime'), /2026/)
  // An unknown style degrades to the table default rather than throwing.
  assert.equal(formatDate(iso, 'nonsense'), formatDate(iso, 'short'))
  assert.equal(formatDate('not a date'), '')
  assert.equal(formatDate(null), '')

  assert.equal(formatNumber(1234.5, { maximumFractionDigits: 1 }), '1,234.5')
  assert.equal(formatNumber(Number.NaN), '')
  assert.equal(formatNumber('12'), '')
})

test('epoch numbers are read as seconds or milliseconds by magnitude', () => {
  const iso = '2026-08-24T10:30:00Z'
  const ms = Date.parse(iso)
  // The API writes seconds; Date.now() arithmetic produces milliseconds. Both
  // reach these helpers and must land on the same instant.
  assert.equal(formatDate(ms / 1000, 'datetime'), formatDate(ms, 'datetime'))
  assert.equal(formatDate(ms / 1000), formatDate(iso))
  // 0 is the epoch, not "no value" — the callers this replaced all special-cased
  // it back in with `!d && d !== 0`. Asserted through formatDay so the
  // expectation does not depend on the machine's timezone.
  assert.equal(formatDay(0), '1 Jan 1970')
})

test('English formats British, so adopting the seam keeps the shape the UI had', () => {
  // The message bundle is tagged `en`; bare `en` would order dates US-style.
  assert.equal(formatDate('2026-08-24T10:30:00Z'), '24 Aug 2026')
  assert.equal(formatDate('2026-08-24T10:30:00Z', 'dayMonth'), '24 Aug')
  assert.match(formatDate('2026-08-24T10:30:00Z', 'dayMonthTime'), /^24 Aug, \d{2}:\d{2}$/)
})

test('relative time picks the largest fitting unit in both directions', () => {
  const now = Date.parse('2026-08-24T12:00:00Z')
  const at = (ms) => formatRelative(new Date(now + ms), now)
  assert.equal(at(-90 * 60 * 1000), '1 hour ago') // not "90 minutes ago"
  assert.equal(at(-45 * 1000), '45 seconds ago')
  assert.equal(at(3 * 24 * 60 * 60 * 1000), 'in 3 days')
  assert.equal(formatRelative(null), '')
})

test('recent timestamps read as relative, older ones pin to a date', () => {
  const DAY = 24 * 60 * 60 * 1000
  assert.equal(formatRecent(Date.now() - 2 * 60 * 60 * 1000), '2 hours ago')
  // Past the surface's threshold the wording gives way to an absolute date, so
  // a year-old item does not read "12 months ago".
  assert.equal(formatRecent('2020-08-24T10:30:00Z'), '24 Aug 2020')
  assert.equal(formatRecent(Date.now() - 2 * DAY, { within: DAY }), formatDate(Date.now() - 2 * DAY))
  assert.equal(formatRecent(null), '')
})

test('a DATE column keeps its calendar day in every timezone', () => {
  // Postgres DATE arrives as epoch seconds at midnight UTC. Formatted in a
  // zone west of UTC that instant is still the previous evening, so the plain
  // date formatter moves a 5 Aug due date to 4 Aug for the whole of the
  // Americas — which is where this project's pt-BR users are.
  const midnightUTC = Date.UTC(2026, 7, 5) / 1000
  const westOfUTC = new Intl.DateTimeFormat('en-GB', {
    dateStyle: 'medium',
    timeZone: 'America/Sao_Paulo',
  }).format(new Date(midnightUTC * 1000))
  assert.equal(westOfUTC, '4 Aug 2026') // the shift this guards against
  assert.equal(formatDay(midnightUTC), '5 Aug 2026')
  assert.equal(formatDay(midnightUTC, 'dayMonth'), '5 Aug')
  assert.equal(formatDay(null), '')
})

test('the active locale drives every shape, not just the English fallback', async () => {
  // The point of the whole seam: switching locale must change the output. An
  // English-only assertion would still pass if the locale stopped being read.
  const previous = i18n.global.locale.value
  // Intl needs the tag, not the message bundle, so no loader is involved here.
  i18n.global.locale.value = 'id-ID'
  try {
    assert.equal(formatDate('2026-08-05T14:30:00Z'), '5 Agu 2026')
    // Indonesian separates hours from minutes with a dot.
    assert.match(formatDate('2026-08-05T14:30:00Z', 'datetime'), /5 Agu 2026, \d{2}\.\d{2}/)
    assert.equal(formatDay(Date.UTC(2026, 7, 5) / 1000), '5 Agu 2026')
    const now = Date.parse('2026-08-24T12:00:00Z')
    assert.equal(formatRelative(new Date(now - 5 * 60 * 1000), now), '5 menit yang lalu')
    assert.equal(formatRelative(new Date(now - 24 * 60 * 60 * 1000), now), 'kemarin')
    assert.equal(formatNumber(1234.5, { maximumFractionDigits: 1 }), '1.234,5')
  } finally {
    i18n.global.locale.value = previous
  }
  // …and the switch is not sticky.
  assert.equal(formatDate('2026-08-05T14:30:00Z'), '5 Aug 2026')
})

test('useFormat exposes the seam under the names the contract documents', () => {
  const { date, day, number, relative, recent } = useFormat()
  assert.equal(typeof date, 'function')
  assert.equal(typeof number, 'function')
  assert.equal(typeof relative, 'function')
  assert.equal(typeof day, 'function')
  assert.equal(typeof recent, 'function')
})
