// A History row whose entry has only an old value (a removed external link, or
// any field cleared to empty) used to render no value at all: just "Changed
// external link". It must show what was cleared, struck through. The two
// neighbouring shapes (new only, old and new) are controls and stay as they were.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const PANEL = new URL('../src/components/HistoryPanel.vue', import.meta.url).pathname
const HistoryPanel = await compileSfc(PANEL)

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

// Each entry has its own actor, so the panel never groups them into one row.
const ENTRIES = [
  { action: 'update', field: 'external_link', old_value: 'SOC 2 (audit_report) https://x.example/a; valid until -; verified -',
    new_value: null, changed_by: 'removed@b.c', created_at: 1760000300 },
  { action: 'update', field: 'external_link', old_value: null, new_value: 'ISO cert (certificate) https://x.example/b',
    changed_by: 'added@b.c', created_at: 1760000200 },
  { action: 'update', field: 'status', old_value: 'draft', new_value: 'active',
    changed_by: 'edited@b.c', created_at: 1760000100 },
]

async function mount() {
  let answered = false
  globalThis.fetch = async (url) => {
    if (!String(url).includes('/changelog/')) return json(200, [])
    try { return json(200, ENTRIES) } finally { answered = true }
  }
  const html = await renderSfcSettled(HistoryPanel, { entityType: 'supplier', entityId: '1' }, () => answered)
  const host = document.createElement('div')
  host.innerHTML = html
  return host
}

const rowFor = (host, actor) =>
  [...host.querySelectorAll('tr')].find(r => r.querySelector(`[title="${actor}"]`))

test('an entry with only an old value shows it struck through', async () => {
  const row = rowFor(await mount(), 'removed@b.c')
  const struck = row.querySelector('.line-through')
  assert.ok(struck, `no line-through element in: ${row.innerHTML}`)
  assert.ok(struck.textContent.includes('SOC 2 (audit_report)'), struck.textContent)
})

test('control: an entry with only a new value renders it, not struck through', async () => {
  const row = rowFor(await mount(), 'added@b.c')
  assert.ok(row.textContent.includes('ISO cert (certificate)'), row.textContent)
  assert.equal(row.querySelector('.line-through'), null)
})

test('control: an entry with old and new renders old → new, not struck through', async () => {
  const row = rowFor(await mount(), 'edited@b.c')
  assert.ok(row.textContent.includes('draft → active'), row.textContent)
  assert.equal(row.querySelector('.line-through'), null)
})
