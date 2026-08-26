// The two formatting seams from docs/i18n.md: no component formats a date or
// de-slugs an enum itself, so these are the only implementations to test.
import test from 'node:test'
import assert from 'node:assert/strict'
import { enumLabel } from '../src/composables/useEnumLabel.js'
import { formatDate, formatNumber, formatRelative, useFormat } from '../src/composables/useFormat.js'
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

test('relative time picks the largest fitting unit in both directions', () => {
  const now = Date.parse('2026-08-24T12:00:00Z')
  const at = (ms) => formatRelative(new Date(now + ms), now)
  assert.equal(at(-90 * 60 * 1000), '1 hour ago') // not "90 minutes ago"
  assert.equal(at(-45 * 1000), '45 seconds ago')
  assert.equal(at(3 * 24 * 60 * 60 * 1000), 'in 3 days')
  assert.equal(formatRelative(null), '')
})

test('useFormat exposes the seam under the names the contract documents', () => {
  const { date, number, relative } = useFormat()
  assert.equal(typeof date, 'function')
  assert.equal(typeof number, 'function')
  assert.equal(typeof relative, 'function')
})
