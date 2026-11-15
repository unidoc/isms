// #503: the Dashboard calendar marked an item overdue by comparing month
// indexes only, so browsing next year painted Jan-Oct red, and nothing in the
// current month was ever overdue. An item is overdue once its date has ended on
// the UTC day, in whatever year is browsed; the current-month highlight needs
// the year to match too.
import test from 'node:test'
import assert from 'node:assert/strict'
import { buildCalendarItems, isCurrentMonth } from '../src/utils/calendarItems.js'

const NOW = new Date('2026-11-15T12:00:00Z')
const ZONES = ['America/Los_Angeles', 'Asia/Jakarta']
const startTZ = process.env.TZ
function inZone(tz, fn) {
  process.env.TZ = tz
  try { fn() } finally { if (startTZ === undefined) delete process.env.TZ; else process.env.TZ = startTZ }
}
const epoch = (day) => Math.floor(new Date(day + 'T00:00:00Z').getTime() / 1000)
const overdueOf = (items, id) => items.find(i => i.id === id)?.overdue

for (const tz of ZONES) {
  test(`calendar overdue follows the item's date and year (${tz})`, () => {
    inZone(tz, () => {
      const data = {
        tasks: [
          { id: 1, title: 'earlier this month', due_date: epoch('2026-11-03'), status: 'open' },
          { id: 2, title: 'due today', due_date: epoch('2026-11-15'), status: 'open' },
          { id: 3, title: 'next year', due_date: epoch('2027-03-01'), status: 'open' },
          { id: 4, title: 'last year', due_date: epoch('2025-12-01'), status: 'open' },
          { id: 5, title: 'done and past', due_date: epoch('2026-11-03'), status: 'done' },
        ],
        audits: [{ id: 6, title: 'completed audit', planned_date: epoch('2026-03-01'), status: 'completed' }],
        risks: [
          { id: 7, title: 'closed risk', next_review: epoch('2026-03-01'), status: 'closed' },
          { id: 8, title: 'open risk', next_review: epoch('2026-03-01'), status: 'open' },
        ],
        suppliers: [{ id: 9, name: 'supplier', next_review: epoch('2026-02-01') }],
        legal: [{ id: 10, title: 'legal', next_review: epoch('2026-12-01') }],
      }
      const y26 = buildCalendarItems(data, 2026, 'Untitled', NOW)
      assert.equal(overdueOf(y26, 't-1'), true, 'due 3 Nov, today 15 Nov')
      assert.equal(overdueOf(y26, 't-2'), false, 'due today')
      assert.equal(overdueOf(y26, 't-5'), false, 'done task')
      assert.equal(overdueOf(y26, 'a-6'), false, 'completed audit')
      assert.equal(overdueOf(y26, 'r-7'), false, 'closed risk')
      assert.equal(overdueOf(y26, 'r-8'), true, 'open past risk')
      assert.equal(overdueOf(y26, 's-9'), true)
      assert.equal(overdueOf(y26, 'l-10'), false, 'future legal review')
      assert.equal(overdueOf(y26, 't-3'), undefined, 'other years are not shown')

      const y27 = buildCalendarItems(data, 2027, 'Untitled', NOW)
      assert.equal(overdueOf(y27, 't-3'), false, 'browsing next year: March 2027 is in the future')
      const y25 = buildCalendarItems(data, 2025, 'Untitled', NOW)
      assert.equal(overdueOf(y25, 't-4'), true, 'browsing last year: Dec 2025 is past')
    })
  })

  test(`the current month is highlighted only in the current year (${tz})`, () => {
    inZone(tz, () => {
      assert.equal(isCurrentMonth(10, 2026, NOW), true)
      assert.equal(isCurrentMonth(10, 2027, NOW), false)
      assert.equal(isCurrentMonth(10, 2025, NOW), false)
      assert.equal(isCurrentMonth(9, 2026, NOW), false)
    })
  })
}

test('an untitled audit gets the fallback label', () => {
  const items = buildCalendarItems({ audits: [{ id: 1, planned_date: epoch('2026-11-20'), status: 'planned' }] }, 2026, 'Untitled', NOW)
  assert.equal(items[0].short, 'Untitled')
})
