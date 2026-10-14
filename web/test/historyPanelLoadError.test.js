// A History tab whose request fails must say so (issue #446). It used to swallow
// every error into an empty list and show "No history yet.", which is also what
// a record with no history shows. These tests mount the real HistoryPanel with
// a stubbed fetch and read what lands on the page.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const PANEL = new URL('../src/components/HistoryPanel.vue', import.meta.url).pathname
const HistoryPanel = await compileSfc(PANEL)

const ERROR_TEXT = 'Failed to load history'
const EMPTY_TEXT = 'No history yet.'

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

// The changelog URL gets the scenario's answer; anything else (the member list
// the panel also loads) gets an empty 200.
async function mount(changelog, interact) {
  let answered = false
  globalThis.fetch = async (url) => {
    if (!String(url).includes('/changelog/')) return json(200, [])
    try { return await changelog() } finally { answered = true }
  }
  // The initial render already shows the empty text, before the load starts, so
  // "settled" is the changelog request having been answered (and a tick since).
  return renderSfcSettled(HistoryPanel, { entityType: 'risk', entityId: '1' }, () => answered, interact)
}

test('a 500 shows the error state, not the empty text', async () => {
  const html = await mount(async () => json(500, { message: 'boom' }))
  assert.ok(html.includes(ERROR_TEXT), `error text missing: ${html}`)
  assert.ok(!html.includes(EMPTY_TEXT), `empty text rendered for a failure: ${html}`)
})

test('a network failure shows the error state, not the empty text', async () => {
  const html = await mount(async () => { throw new TypeError('Failed to fetch') })
  assert.ok(html.includes(ERROR_TEXT), `error text missing: ${html}`)
  assert.ok(!html.includes(EMPTY_TEXT), `empty text rendered for a failure: ${html}`)
})

test('a 400 shows the error state, not the empty text', async () => {
  const html = await mount(async () => json(400, { error: 'bad request' }))
  assert.ok(html.includes(ERROR_TEXT), `error text missing: ${html}`)
  assert.ok(!html.includes(EMPTY_TEXT), `empty text rendered for a failure: ${html}`)
})

test('control: a successful empty history shows the empty text and no error', async () => {
  const html = await mount(async () => json(200, []))
  assert.ok(html.includes(EMPTY_TEXT), `empty text missing: ${html}`)
  assert.ok(!html.includes(ERROR_TEXT), `error text rendered for success: ${html}`)
})

test('control: a history with one entry renders a row and neither text', async () => {
  const html = await mount(async () => json(200, [
    { action: 'create', changed_by: 'a@b.c', created_at: 1760000000 },
  ]))
  assert.ok(html.includes('<tr'), `row missing: ${html}`)
  assert.ok(!html.includes(EMPTY_TEXT), `empty text rendered: ${html}`)
  assert.ok(!html.includes(ERROR_TEXT), `error text rendered: ${html}`)
})

test('the error state offers Retry; empty and populated states do not', async () => {
  const failed = await mount(async () => json(500, { message: 'boom' }))
  assert.ok(failed.includes('<button'), `Retry button missing: ${failed}`)
  assert.ok(failed.includes('Retry'), `Retry label missing: ${failed}`)
  const empty = await mount(async () => json(200, []))
  assert.ok(!empty.includes('<button'), `button rendered for empty history: ${empty}`)
  const rows = await mount(async () => json(200, [
    { action: 'create', changed_by: 'a@b.c', created_at: 1760000000 },
  ]))
  assert.ok(!rows.includes('<button'), `button rendered for populated history: ${rows}`)
})

test('clicking Retry reloads: the error clears and the row renders', async () => {
  let calls = 0
  const html = await mount(async () => {
    calls++
    return calls === 1
      ? json(500, { message: 'boom' })
      : json(200, [{ action: 'create', changed_by: 'a@b.c', created_at: 1760000000 }])
  }, async (host, settle) => {
    const button = host.querySelector('button')
    assert.ok(button, `no Retry button to click: ${host.innerHTML}`)
    button.click()
    await settle(h => h.includes('<tr'))
  })
  assert.equal(calls, 2, 'Retry did not re-request the changelog')
  assert.ok(html.includes('<tr'), `row missing after retry: ${html}`)
  assert.ok(!html.includes(ERROR_TEXT), `error still shown after retry: ${html}`)
  assert.ok(!html.includes(EMPTY_TEXT), `empty text shown after retry: ${html}`)
})
