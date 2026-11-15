// Calendar-date helpers. A DATE field (due date, next review, planned date,
// expiry) is a UTC calendar day for everyone, whatever the browser's zone: the
// API sends it as epoch seconds at UTC midnight, and this server counts overdue
// on the UTC day (#503). Timestamps (created_at, approved_at, ...) are NOT dates
// and keep going through formatDate in the viewer's zone.

const DAY = /^\d{4}-\d{2}-\d{2}$/

function toDate(value) {
  if (value === null || value === undefined || value === '') return null
  const d = typeof value === 'number' ? new Date(value * 1000) : new Date(value)
  return Number.isNaN(d.getTime()) ? null : d
}

// toDay turns epoch seconds, an ISO string or YYYY-MM-DD into YYYY-MM-DD on the
// UTC day. '' for empty or invalid input.
export function toDay(value) {
  if (typeof value === 'string' && DAY.test(value)) return value
  const d = toDate(value)
  return d ? d.toISOString().slice(0, 10) : ''
}

// dayToEpoch turns YYYY-MM-DD into epoch seconds at UTC midnight, the form the
// API stores for a DATE. null for empty or invalid input.
export function dayToEpoch(day) {
  if (!day) return null
  const ms = new Date(String(day).slice(0, 10) + 'T00:00:00Z').getTime()
  return Number.isNaN(ms) ? null : Math.floor(ms / 1000)
}

// utcToday is today's UTC calendar day, YYYY-MM-DD.
export function utcToday(now = new Date()) {
  return now.toISOString().slice(0, 10)
}

// isPastDue is true once the due day has ended on the UTC day: due 15 Nov is
// past due from 16 Nov 00:00 UTC, never on the 15th. False for empty input.
export function isPastDue(value, now = new Date()) {
  const day = toDay(value)
  return day !== '' && day < utcToday(now)
}

// utcMonthIn returns the 0-based month of a DATE value when it falls in `year`,
// read on the UTC day, else -1. For the dashboard's annual calendar.
export function utcMonthIn(value, year) {
  const day = toDay(value)
  if (!day || Number(day.slice(0, 4)) !== year) return -1
  return Number(day.slice(5, 7)) - 1
}
