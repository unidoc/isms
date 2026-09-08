// The two formatting seams from docs/i18n.md: no component formats a date or
// de-slugs an enum itself, so these are the only implementations to test.
import test from 'node:test'
import assert from 'node:assert/strict'
import { enumLabel } from '../src/composables/useEnumLabel.js'
import { formatDate, formatDay, formatHours, formatMonthShort, formatNumber, formatRecent, formatRelative, useFormat } from '../src/composables/useFormat.js'
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
  //
  // The day floats and the assertions do not name it. formatDate() renders an
  // instant in the machine's zone, which is correct — only formatDay() pins to
  // UTC, and the test below owns that. A fixed instant lands on a different
  // calendar day either side of UTC (10:30Z is the 25th at UTC+14, the 23rd at
  // UTC-12), so an exact-day expectation asserts the runner's timezone rather
  // than the formatter. What matters here is the shape: day before month, short
  // month name, four-digit year — "Aug 24, 2026" fails all three.
  // The day is constrained, not dropped: 10:30Z is the 23rd at UTC-12 and the
  // 25th at UTC+14, and nothing in between reaches any other date. A bare
  // \d{1,2} would also accept "99 Aug 2026", so a regression shifting the
  // instant by weeks would pass.
  assert.match(formatDate('2026-08-24T10:30:00Z'), /^(?:23|24|25) Aug 2026$/)
  assert.match(formatDate('2026-08-24T10:30:00Z', 'dayMonth'), /^(?:23|24|25) Aug$/)
  assert.match(formatDate('2026-08-24T10:30:00Z', 'dayMonthTime'), /^(?:23|24|25) Aug, \d{2}:\d{2}$/)
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
  // Same instant-of-day as above, so the same three possible dates.
  assert.match(formatRecent('2020-08-24T10:30:00Z'), /^(?:23|24|25) Aug 2020$/)
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

test('short month names come from Intl, and reject anything but a month index', () => {
  // The annual-plan grid labels twelve columns with no date to format, so this
  // is the one shape that takes an index rather than a value.
  assert.equal(formatMonthShort(0), 'Jan')
  assert.equal(formatMonthShort(11), 'Dec')
  // Out of range, or not an index at all: an empty label, never "Invalid Date".
  assert.equal(formatMonthShort(-1), '')
  assert.equal(formatMonthShort(12), '')
  assert.equal(formatMonthShort('x'), '')
  assert.equal(formatMonthShort(null), '')
  assert.equal(formatMonthShort(1.5), '')
})

test('durations carry a locale-supplied unit, not a hardcoded letter', () => {
  // "4h" is English; the unit is Intl's to choose, which is why no 'h' lives in
  // a translation file.
  assert.equal(formatHours(4), '4h')
  assert.equal(formatHours(0), '0h')
  assert.equal(formatHours(Number.NaN), '')
  assert.equal(formatHours('4'), '')
  assert.equal(formatHours(null), '')
})

test('the active locale drives every shape, not just the English fallback', async () => {
  // The point of the whole seam: switching locale must change the output. An
  // English-only assertion would still pass if the locale stopped being read.
  const previous = i18n.global.locale.value
  // Intl needs the tag, not the message bundle, so no loader is involved here.
  i18n.global.locale.value = 'id-ID'
  try {
    // "Agu", not "Aug" — the locale is doing the work. 14:30Z is the 5th at
    // UTC-12 and the 6th at UTC+14, so those are the only two possible days.
    assert.match(formatDate('2026-08-05T14:30:00Z'), /^(?:5|6) Agu 2026$/)
    // Indonesian separates hours from minutes with a dot.
    assert.match(formatDate('2026-08-05T14:30:00Z', 'datetime'), /^(?:5|6) Agu 2026, \d{2}\.\d{2}$/)
    assert.equal(formatDay(Date.UTC(2026, 7, 5) / 1000), '5 Agu 2026')
    const now = Date.parse('2026-08-24T12:00:00Z')
    assert.equal(formatRelative(new Date(now - 5 * 60 * 1000), now), '5 menit yang lalu')
    assert.equal(formatRelative(new Date(now - 24 * 60 * 60 * 1000), now), 'kemarin')
    assert.equal(formatNumber(1234.5, { maximumFractionDigits: 1 }), '1.234,5')
    // "Agu"/"Des", not "Aug"/"Dec" — month names are calendar data Intl
    // carries, which is why none of them are translation keys.
    assert.equal(formatMonthShort(7), 'Agu')
    assert.equal(formatMonthShort(11), 'Des')
    // Indonesian writes the hour unit with a space and its own letter.
    assert.match(formatHours(4), /^4\s*j$/)
  } finally {
    i18n.global.locale.value = previous
  }
  // …and the switch is not sticky: "Aug" again, not "Agu".
  assert.match(formatDate('2026-08-05T14:30:00Z'), /^(?:5|6) Aug 2026$/)
})

test('useFormat exposes the seam under the names the contract documents', () => {
  const { date, day, monthShort, hours, number, relative, recent } = useFormat()
  assert.equal(typeof date, 'function')
  assert.equal(typeof monthShort, 'function')
  assert.equal(typeof hours, 'function')
  assert.equal(typeof number, 'function')
  assert.equal(typeof relative, 'function')
  assert.equal(typeof day, 'function')
  assert.equal(typeof recent, 'function')
})
