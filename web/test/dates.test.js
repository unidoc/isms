// #503: calendar dates are UTC days for everyone, and an item is overdue only
// after its due day ends on the UTC day. The unit tests run under two browser
// zones (Node honours a runtime change of process.env.TZ); the view test mounts
// the real Tasks page and checks the badge.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc } from './support/renderSfc.js'
import { toDay, dayToEpoch, utcToday, isPastDue } from '../src/utils/dates.js'

const ZONES = ['America/Los_Angeles', 'Asia/Jakarta']
const startTZ = process.env.TZ
function inZone(tz, fn) {
  process.env.TZ = tz
  try { fn() } finally { if (startTZ === undefined) delete process.env.TZ; else process.env.TZ = startTZ }
}

for (const tz of ZONES) {
  test(`toDay and dayToEpoch use the UTC day (${tz})`, () => {
    inZone(tz, () => {
      assert.equal(toDay(1767225600), '2026-01-01')
      assert.equal(toDay(1767225599), '2025-12-31')
      assert.equal(dayToEpoch('2026-01-01'), 1767225600)
      assert.equal(toDay(dayToEpoch('2026-01-01')), '2026-01-01')
      assert.equal(toDay('2026-01-01'), '2026-01-01')
      assert.equal(toDay('2026-01-01T00:00:00Z'), '2026-01-01')
      assert.equal(toDay('2026-01-01T00:00:00+07:00'), '2025-12-31')
      assert.equal(toDay(''), '')
      assert.equal(toDay(null), '')
      assert.equal(toDay(undefined), '')
      assert.equal(toDay('not a date'), '')
      assert.equal(dayToEpoch(''), null)
      assert.equal(dayToEpoch(null), null)
    })
  })

  test(`isPastDue flips when the due day ends, on the UTC day (${tz})`, () => {
    inZone(tz, () => {
      const lateToday = new Date('2026-11-15T23:30:00Z')
      assert.equal(isPastDue('2026-11-15', lateToday), false, 'due today is not past due')
      assert.equal(isPastDue(dayToEpoch('2026-11-15'), lateToday), false)
      assert.equal(isPastDue('2026-11-14', lateToday), true)
      assert.equal(isPastDue(dayToEpoch('2026-11-14'), lateToday), true)
      const justAfter = new Date('2026-11-16T00:10:00Z')
      assert.equal(isPastDue('2026-11-15', justAfter), true, 'due yesterday (UTC) is past due')
      assert.equal(isPastDue('2026-11-16', justAfter), false)
      assert.equal(utcToday(lateToday), '2026-11-15')
      assert.equal(isPastDue('', lateToday), false)
      assert.equal(isPastDue(null, lateToday), false)
    })
  })
}

// ---- the real Tasks page ----

const TASKS = new URL('../src/views/Tasks.vue', import.meta.url).pathname
const Tasks = await compileSfc(TASKS, { stubChildren: true })
globalThis.history ??= window.history
const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}
const json = (body) => new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })

async function overdueBadges(tasks) {
  const { createApp, h } = await import('vue')
  const { createRouter, createMemoryHistory } = await import('vue-router')
  const { i18n } = await import('../src/i18n.js')
  globalThis.fetch = async (url) => {
    const u = String(url)
    if (u.endsWith('/me')) return json({ organization_id: 1, role: 'admin', email: 'a@b.c' })
    if (u.includes('/tasks/stats')) return json({})
    if (u.includes('/tasks')) return json({ data: tasks, total: tasks.length })
    return json([])
  }
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { render: () => null } }] })
  const app = createApp({ render: () => h(Tasks) })
  app.use(router)
  app.use(i18n)
  const host = document.createElement('div')
  document.body.appendChild(host)
  app.mount(host)
  for (let i = 0; i < 150; i++) {
    await new Promise(r => setTimeout(r, 10))
    if (host.textContent.includes(tasks[0].title)) break
  }
  await new Promise(r => setTimeout(r, 50))
  const badges = [...host.querySelectorAll('span.text-red-400.font-semibold')].map(e => e.textContent.trim())
  const titles = host.textContent
  app.unmount()
  host.remove()
  return { badges, titles }
}

const day = (offset) => new Date(Date.now() + offset * 86400000).toISOString().slice(0, 10)

for (const tz of ZONES) {
  test(`Tasks page: due today (UTC) is not overdue, due yesterday is (${tz})`, async () => {
    process.env.TZ = tz
    try {
      const mk = (id, title, d) => ({ id, identifier: `TASK-${id}`, title, status: 'open', priority: 'medium', task_type: 'manual', assignee: 'a@b.c', due_date: dayToEpoch(d) })
      const today = await overdueBadges([mk(1, 'Due today task', day(0))])
      assert.ok(today.titles.includes('Due today task'), 'task not rendered')
      assert.equal(today.badges.length, 0, `due today shows ${today.badges}`)
      const yest = await overdueBadges([mk(2, 'Due yesterday task', day(-1))])
      assert.ok(yest.titles.includes('Due yesterday task'), 'task not rendered')
      assert.equal(yest.badges.length, 1, `due yesterday shows ${yest.badges}`)
    } finally {
      if (startTZ === undefined) delete process.env.TZ; else process.env.TZ = startTZ
    }
  })
}
