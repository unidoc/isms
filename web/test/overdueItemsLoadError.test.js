// The dashboard Overdue panel must say when it could not load (#492). It used to
// swallow the failure, leave the list empty and render the green "All reviews on
// schedule", which is what a clean organisation shows. Create Tasks failures
// were swallowed the same way. These tests mount the real OverdueItems with a
// stubbed fetch and read what lands on the page.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const COMPONENT = new URL('../src/components/OverdueItems.vue', import.meta.url).pathname
const OverdueItems = await compileSfc(COMPONENT)

const LOAD_ERROR = 'Failed to load overdue items'
const CREATE_ERROR = 'Could not create tasks'
const ALL_CLEAR = 'All reviews on schedule'

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

const SUMMARY = {
  risks: [{ entity_type: 'risk', entity_id: 'RISK-1', title: 'Old risk', days_late: 20, criticality: 'high' }],
  suppliers: [], systems: [], legal: [], tasks: [], supplier_contracts: [], external_links: [],
  total_count: 1,
}
const EMPTY = { ...SUMMARY, risks: [], total_count: 0 }

const retryButton = host =>
  [...host.querySelectorAll('button')].find(b => b.textContent.trim() === 'Retry')

// `overdue` answers GET /overdue, `create` answers POST /overdue/tasks.
async function mount(overdue, create, interact, events = []) {
  let answered = false
  globalThis.fetch = async (url, opts = {}) => {
    if (String(url).includes('/overdue/tasks') && opts.method === 'POST') return create()
    try { return await overdue() } finally { answered = true }
  }
  return renderSfcSettled(OverdueItems, {
    isAdmin: true,
    onLoaded: s => events.push(['loaded', s]),
    'onLoad-failed': () => events.push(['load-failed']),
  }, () => answered, interact)
}

test('a 500 shows the load error, not "all on schedule"', async () => {
  const html = await mount(async () => json(500, { message: 'boom' }))
  assert.ok(html.includes(LOAD_ERROR), `error text missing: ${html}`)
  assert.ok(!html.includes(ALL_CLEAR), `all-clear rendered for a failure: ${html}`)
})

test('a network failure shows the load error, not "all on schedule"', async () => {
  const html = await mount(async () => { throw new TypeError('Failed to fetch') })
  assert.ok(html.includes(LOAD_ERROR), `error text missing: ${html}`)
  assert.ok(!html.includes(ALL_CLEAR), `all-clear rendered for a failure: ${html}`)
})

test('clicking Retry reloads: the error clears and the item renders', async () => {
  let calls = 0
  const html = await mount(async () => {
    calls++
    return calls === 1 ? json(500, { message: 'boom' }) : json(200, SUMMARY)
  }, null, async (host, settle) => {
    const button = retryButton(host)
    assert.ok(button, `no Retry button to click: ${host.innerHTML}`)
    button.click()
    await settle(h => h.includes('Old risk'))
  })
  assert.equal(calls, 2, 'Retry did not re-request the overdue list')
  assert.ok(html.includes('Old risk'), `item missing after retry: ${html}`)
  assert.ok(!html.includes(LOAD_ERROR), `error still shown after retry: ${html}`)
})

test('a failed Create Tasks shows the error', async () => {
  let posted = false
  const html = await mount(async () => json(200, SUMMARY), async () => {
    posted = true
    return json(500, { message: 'boom' })
  }, async (host, settle) => {
    const button = [...host.querySelectorAll('button')].find(b => b.textContent.includes('Create Tasks'))
    assert.ok(button, `no Create Tasks button: ${host.innerHTML}`)
    button.click()
    await settle(h => posted && h.includes(CREATE_ERROR))
  })
  assert.ok(html.includes(CREATE_ERROR), `create error missing: ${html}`)
})

test('control: an empty 200 shows "all on schedule" and no error', async () => {
  const html = await mount(async () => json(200, EMPTY))
  assert.ok(html.includes(ALL_CLEAR), `all-clear missing: ${html}`)
  assert.ok(!html.includes(LOAD_ERROR), `error rendered for success: ${html}`)
  assert.ok(!html.includes('Retry'), `Retry rendered for success: ${html}`)
})

test('every load reports its outcome: a failure, then a Retry success with the summary', async () => {
  const events = []
  let calls = 0
  await mount(async () => {
    calls++
    return calls === 1 ? json(500, { message: 'boom' }) : json(200, SUMMARY)
  }, null, async (host, settle) => {
    retryButton(host).click()
    await settle(h => h.includes('Old risk'))
  }, events)
  assert.deepEqual(events.map(e => e[0]), ['load-failed', 'loaded'])
  assert.equal(events[1][1].total_count, 1)
})
