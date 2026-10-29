// The dashboard's overdue list must render the external_links group (#316):
// one row per link (two links on one supplier are two rows), the link label and
// its supplier's name, and expired / expiring in their own wording.
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

const link = (over) => ({
  entity_type: 'external_link', entity_id: 'SUPPLIER-1', parent_title: 'Acme Hosting',
  criticality: 'high', next_review: 1760000000, ...over,
})

const SUMMARY = {
  risks: [], suppliers: [], systems: [], legal: [], tasks: [],
  supplier_contracts: [
    { entity_type: 'supplier_contract', entity_id: 'SUPPLIER-2', title: 'Beta Corp', criticality: 'low', days_late: -4, state: 'expiring' },
  ],
  external_links: [
    link({ link_id: 11, title: 'SOC 2 report', days_late: 5, state: 'expired' }),
    link({ link_id: 12, title: 'ISO 27001 certificate', days_late: -9, state: 'expiring' }),
  ],
  total_count: 3,
}

async function mount(summary) {
  let answered = false
  globalThis.fetch = async (url) => {
    if (!String(url).includes('/overdue')) return json(200, {})
    try { return json(200, summary) } finally { answered = true }
  }
  const html = await renderSfcSettled(OverdueItems, { isAdmin: false }, () => answered)
  const host = document.createElement('div')
  host.innerHTML = html
  return { html, host }
}

test('two links on one supplier render as two rows with label and supplier name', async () => {
  const { html, host } = await mount(SUMMARY)
  assert.ok(html.includes('SOC 2 report'), `first label missing: ${html}`)
  assert.ok(html.includes('ISO 27001 certificate'), `second label missing: ${html}`)
  const rows = [...host.querySelectorAll('.divide-y > div')]
  assert.equal(rows.length, 3, 'two link rows plus the contract row')
  const linkRows = rows.filter(r => r.textContent.includes('Acme Hosting'))
  assert.equal(linkRows.length, 2, 'each link row shows the supplier name')
  assert.ok(linkRows.every(r => r.textContent.includes('SUPPLIER-1')), 'identifier badge shown')
})

test('expired and expiring links read differently', async () => {
  const { host } = await mount(SUMMARY)
  const row = text => [...host.querySelectorAll('.divide-y > div')].find(r => r.textContent.includes(text))
  const expired = row('SOC 2 report')
  const expiring = row('ISO 27001 certificate')
  assert.ok(expired.textContent.includes('5d ago'), expired.textContent)
  assert.ok(expired.innerHTML.includes('text-red-400'))
  assert.ok(expiring.textContent.includes('in 9d'), expiring.textContent)
  assert.ok(expiring.innerHTML.includes('text-amber-400'))
})

test('control: the supplier contract row still renders', async () => {
  const { host } = await mount(SUMMARY)
  const row = [...host.querySelectorAll('.divide-y > div')].find(r => r.textContent.includes('Beta Corp'))
  assert.ok(row, 'contract row missing')
  assert.ok(row.textContent.includes('in 4d'), row.textContent)
  assert.ok(row.textContent.includes('contract'), row.textContent)
})
