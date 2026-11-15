// Pure pieces of the Dashboard's annual calendar (#503): which month a DATE
// falls in, whether the item is overdue, and which month is "now". Dates are
// UTC calendar days; an item is overdue once its due day has ended on the UTC
// day, whatever year the calendar is browsing.
import { isPastDue, utcMonthIn, utcToday } from './dates.js'

const arr = (v) => (Array.isArray(v) ? v : [])

// buildCalendarItems returns the calendar entries for `year`. `data` holds the
// audits, risks, suppliers, legal and tasks lists. `untitledAudit` is the label
// for an audit with no title.
export function buildCalendarItems(data, year, untitledAudit, now = new Date()) {
  const items = []
  const add = (key, id, date, label, short, excluded) => {
    const month = utcMonthIn(date, year)
    if (month < 0) return
    items.push({ key, month, id, label, short, overdue: !excluded && isPastDue(date, now) })
  }
  for (const a of arr(data.audits)) {
    add('audit', 'a-' + a.id, a.planned_date, a.title, a.title?.substring(0, 12) || untitledAudit, a.status === 'completed')
  }
  for (const r of arr(data.risks)) {
    add('risk', 'r-' + r.id, r.review_date || r.next_review, r.title || r.identifier,
      (r.identifier || r.title || '').substring(0, 12), r.status === 'closed')
  }
  for (const s of arr(data.suppliers)) {
    add('supplier', 's-' + s.id, s.next_review, s.name, (s.name || '').substring(0, 12), false)
  }
  for (const l of arr(data.legal)) {
    add('legal', 'l-' + l.id, l.next_review, l.title, (l.title || '').substring(0, 12), false)
  }
  for (const t of arr(data.tasks)) {
    add('task', 't-' + t.id, t.due_date, t.title, (t.title || '').substring(0, 12), t.status === 'done')
  }
  return items
}

// isCurrentMonth: month index `i` of the browsed `year` is the current UTC month.
export function isCurrentMonth(i, year, now = new Date()) {
  const today = utcToday(now)
  return Number(today.slice(0, 4)) === year && Number(today.slice(5, 7)) - 1 === i
}
