// The overdue panel's "Create Tasks" result message must say when some tasks
// could not be created (#489), instead of reading like a normal "nothing to do".
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const COMPONENT = new URL('../src/components/OverdueItems.vue', import.meta.url).pathname
const OverdueItems = await compileSfc(COMPONENT)

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

async function mount(result) {
  let posted = false
  globalThis.fetch = async (url, opts = {}) => {
    if (String(url).includes('/overdue/tasks') && opts.method === 'POST') {
      posted = true
      return json(200, result)
    }
    return json(200, SUMMARY)
  }
  return renderSfcSettled(OverdueItems, { isAdmin: true }, () => true, async (host, settle) => {
    const button = [...host.querySelectorAll('button')].find(b => b.textContent.includes('Create Tasks'))
    assert.ok(button, `no Create Tasks button: ${host.innerHTML}`)
    button.click()
    await settle(() => posted && host.textContent.includes('Created'))
  })
}

test('a result with failures says how many could not be created', async () => {
  const html = await mount({ created: [], skipped: 0, total: 1, failed: 2, unassigned: 0 })
  assert.ok(html.includes('2 failed'), html)
  assert.ok(html.includes('text-red-300'), 'the failure note is styled as an error')
})

test('control: a clean result shows no failure note', async () => {
  const html = await mount({ created: [{ id: 1 }], skipped: 1, total: 2, failed: 0, unassigned: 0 })
  assert.ok(html.includes('Created 1 task'), html)
  assert.ok(html.includes('1 skipped'), html)
  assert.ok(!html.includes('failed'), html)
})
