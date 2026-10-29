// The Overdue stat card must not show a number it could not get (#492). When
// GET /overdue failed it fell back to a browser-side count that ignored systems,
// contracts and evidence links, so it showed "0 / all on track" (or a low count)
// beside the panel's load error. A failed load now shows a dash and "could not
// load". The real Dashboard is mounted with a stubbed fetch and a memory router.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc } from './support/renderSfc.js'

const DASHBOARD = new URL('../src/views/Dashboard.vue', import.meta.url).pathname
const Dashboard = await compileSfc(DASHBOARD, { stubChildren: true })

// vue-router touches a bare `history` even with a memory history.
globalThis.history ??= window.history

// api.js reads a bare `localStorage` for the token; node has none.
const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}

const json = (status, body) => new Response(JSON.stringify(body), {
  status, headers: { 'Content-Type': 'application/json' },
})

const LATE_TASK = { id: 1, title: 'Late task', status: 'open', due_date: '2020-01-01' }

// Every endpoint answers an empty 200 except /me, /overdue and /tasks.
async function mountCard(overdue, tasks = [], interact = null, risks = []) {
  const { createApp, h } = await import('vue')
  const { createRouter, createMemoryHistory } = await import('vue-router')
  const { i18n } = await import('../src/i18n.js')
  let overdueAnswered = false
  globalThis.fetch = async (url) => {
    const u = String(url)
    if (u.endsWith('/me')) return json(200, { organization_id: 1, role: 'admin', email: 'a@b.c' })
    if (u.endsWith('/overdue')) { try { return await overdue() } finally { overdueAnswered = true } }
    if (u.includes('/tasks')) return json(200, tasks)
    if (u.endsWith('/risks')) return json(200, risks)
    return json(200, [])
  }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { render: () => null } }],
  })
  const app = createApp({ render: () => h(Dashboard) })
  app.use(router)
  app.use(i18n)
  const host = document.createElement('div')
  document.body.appendChild(host)
  app.mount(host)
  for (let i = 0; i < 150; i++) {
    await new Promise(r => setTimeout(r, 10))
    if (overdueAnswered && host.innerHTML.includes('Overdue')) break
  }
  await new Promise(r => setTimeout(r, 50))
  if (interact) {
    await interact()
    await new Promise(r => setTimeout(r, 50))
  }
  // The card is the one whose label is "Overdue": take its link element.
  const card = [...host.querySelectorAll('a')].find(a => a.textContent.includes('Overdue') && a.textContent.length < 80)
  const text = card ? card.textContent.replace(/\s+/g, ' ').trim() : null
  app.unmount()
  host.remove()
  return text
}

test('a failed overdue load shows a dash and "could not load"', async () => {
  const card = await mountCard(async () => json(500, { message: 'boom' }))
  assert.ok(card, 'overdue card not found')
  assert.ok(card.includes('—'), `dash missing: ${card}`)
  assert.ok(card.includes('could not load'), `unavailable text missing: ${card}`)
  assert.ok(!card.includes('all on track'), `all-clear shown for a failure: ${card}`)
  assert.ok(!/\b0\b/.test(card), `a zero is shown for a failure: ${card}`)
})

test('a failed load does not fall back to counting the task list', async () => {
  const card = await mountCard(async () => json(500, { message: 'boom' }), [LATE_TASK])
  assert.ok(card, 'overdue card not found')
  assert.ok(card.includes('—'), `dash missing: ${card}`)
  assert.ok(!/\b1\b/.test(card), `browser-side count shown: ${card}`)
})

test('control: total_count 2 shows 2 and "items past due"', async () => {
  const card = await mountCard(async () => json(200, { total_count: 2 }))
  assert.ok(card.includes('2') && card.includes('items past due'), card)
  assert.ok(!card.includes('could not load'), card)
})

test('control: total_count 0 shows 0 and "all on track"', async () => {
  const card = await mountCard(async () => json(200, { total_count: 0 }))
  assert.ok(card.includes('0') && card.includes('all on track'), card)
  assert.ok(!card.includes('could not load') && !card.includes('—'), card)
})

// The panel (OverdueItems) reloads the summary on Retry and after Create Tasks;
// the card follows what it last got. The panel is stubbed here, so its events are
// fired through the listeners the Dashboard gave the stub.
const panelListeners = () => (globalThis.__sfcStubAttrs ?? []).filter(a => a.onLoaded).at(-1)

test('the card recovers when the panel reports a successful load', async () => {
  globalThis.__sfcStubAttrs = []
  const card = await mountCard(async () => json(500, { message: 'boom' }), [], async () => {
    const l = panelListeners()
    assert.ok(l, 'the Dashboard gave the panel no loaded listener')
    l.onLoaded({ total_count: 2 })
  }, [{ id: 1, title: 'r' }])
  assert.ok(card.includes('2') && card.includes('items past due'), card)
  assert.ok(!card.includes('—'), card)
})

test('the card goes unavailable when the panel reports a failed load', async () => {
  globalThis.__sfcStubAttrs = []
  const card = await mountCard(async () => json(200, { total_count: 2 }), [], async () => {
    const l = panelListeners()
    assert.ok(l, 'the Dashboard gave the panel no loaded listener')
    l.onLoadFailed()
  }, [{ id: 1, title: 'r' }])
  assert.ok(card.includes('—') && card.includes('could not load'), card)
})
