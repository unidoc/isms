// A Readings section whose request fails must say so (issue #459). It used to
// swallow every error into an empty list and show "No readings yet.", which is
// also what a record that was never assessed shows. These tests mount the real
// ReadingsPanel with a stubbed fetch and read what lands on the page.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const PANEL = new URL('../src/components/ReadingsPanel.vue', import.meta.url).pathname
const ReadingsPanel = await compileSfc(PANEL)

const ERROR_TEXT = 'Failed to load readings'
const EMPTY_TEXT = 'No readings yet.'
const ROW = {
  id: 1, created_at: 1760000000, assessed_by: 'a@b.c', confidentiality: 3,
  integrity: 2, availability: 1, current_likelihood: 2, current_impact: 3,
}

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

// The readings URL gets the scenario's answer; anything else (the member list
// the panel also loads) gets an empty 200.
async function mount(readings, interact) {
  let answered = false
  globalThis.fetch = async (url) => {
    if (!String(url).includes('/readings')) return json(200, [])
    try { return await readings() } finally { answered = true }
  }
  // The initial render already shows the empty text, before the load starts, so
  // "settled" is the readings request having been answered (and a tick since).
  return renderSfcSettled(ReadingsPanel, { entityType: 'risk', entityId: 1 }, () => answered, interact)
}

const retryButton = host =>
  [...host.querySelectorAll('button')].find(b => b.textContent.trim() === 'Retry')

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

test('control: successful empty readings show the empty text and no error', async () => {
  const html = await mount(async () => json(200, []))
  assert.ok(html.includes(EMPTY_TEXT), `empty text missing: ${html}`)
  assert.ok(!html.includes(ERROR_TEXT), `error text rendered for success: ${html}`)
  assert.ok(!html.includes('Retry'), `Retry rendered for success: ${html}`)
})

test('control: one reading renders a row and neither text', async () => {
  const html = await mount(async () => json(200, [ROW]))
  assert.ok(html.includes('<tr'), `row missing: ${html}`)
  assert.ok(!html.includes(EMPTY_TEXT), `empty text rendered: ${html}`)
  assert.ok(!html.includes(ERROR_TEXT), `error text rendered: ${html}`)
  assert.ok(!html.includes('Retry'), `Retry rendered for populated readings: ${html}`)
})

test('the error state offers Retry', async () => {
  const html = await mount(async () => json(500, { message: 'boom' }))
  assert.ok(html.includes('Retry'), `Retry label missing: ${html}`)
})

test('clicking Retry reloads: the error clears and the row renders', async () => {
  let calls = 0
  const html = await mount(async () => {
    calls++
    return calls === 1 ? json(500, { message: 'boom' }) : json(200, [ROW])
  }, async (host, settle) => {
    const button = retryButton(host)
    assert.ok(button, `no Retry button to click: ${host.innerHTML}`)
    button.click()
    await settle(h => h.includes('<tr'))
  })
  assert.equal(calls, 2, 'Retry did not re-request the readings')
  assert.ok(html.includes('<tr'), `row missing after retry: ${html}`)
  assert.ok(!html.includes(ERROR_TEXT), `error still shown after retry: ${html}`)
  assert.ok(!html.includes(EMPTY_TEXT), `empty text shown after retry: ${html}`)
})
