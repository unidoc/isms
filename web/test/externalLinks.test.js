// The supplier Links tab's external-links section (#316): what it renders for a
// stored link, that an anchor can never carry a script URL, that a read-only
// viewer gets no controls, and that a DATE column is shown as the day stored.
// Mounts the real component with a stubbed fetch.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const COMPONENT = new URL('../src/components/ExternalLinks.vue', import.meta.url).pathname
const ExternalLinks = await compileSfc(COMPONENT)

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

// A DATE column arrives as epoch seconds at UTC midnight.
const day = (y, m, d) => Date.UTC(y, m - 1, d) / 1000

const LINKS = [
  { id: 1, entity_type: 'supplier', entity_id: 7, url: 'https://trust.example.com/soc2.pdf', label: 'SOC 2 report',
    link_type: 'audit_report', valid_until: day(2026, 3, 5), state: 'expired',
    verified_at: day(2026, 1, 2), verified_by: 'alice@example.com' },
  { id: 2, entity_type: 'supplier', entity_id: 7, url: 'javascript:alert(1)', label: 'Sneaky',
    link_type: 'other', state: 'current' },
  { id: 3, entity_type: 'supplier', entity_id: 7, url: 'HTTP://Upper.example.org/x', label: 'Upper scheme',
    link_type: 'certificate', valid_until: day(2026, 12, 31), state: 'expiring' },
]

async function mount(props = {}, links = LINKS) {
  let answered = false
  globalThis.fetch = async (url) => {
    if (String(url).includes('/external-links')) {
      try { return json(200, { data: links }) } finally { answered = true }
    }
    return json(200, [])
  }
  const html = await renderSfcSettled(
    ExternalLinks, { entityType: 'supplier', entityId: 'SUPPLIER-7', editable: false, ...props }, () => answered)
  const host = document.createElement('div')
  host.innerHTML = html
  return { html, host }
}

const rowOf = (host, label) =>
  [...host.querySelectorAll('[data-testid="external-link-row"]')].find(r => r.textContent.includes(label))

test('rows show label, type and state', async () => {
  const { host } = await mount()
  const rows = host.querySelectorAll('[data-testid="external-link-row"]')
  assert.equal(rows.length, 3)
  const soc = rowOf(host, 'SOC 2 report')
  assert.ok(soc.textContent.includes('Audit report'), soc.textContent)
  assert.ok(soc.textContent.includes('Expired'), soc.textContent)
  assert.ok(soc.textContent.includes('trust.example.com'), 'host shown')
  assert.ok(rowOf(host, 'Upper scheme').textContent.includes('Expiring soon'))
  assert.ok(rowOf(host, 'Sneaky').textContent.includes('Current'))
})

test('an anchor opens in a new tab with noopener noreferrer', async () => {
  const { host } = await mount()
  const a = rowOf(host, 'SOC 2 report').querySelector('a')
  assert.equal(a.getAttribute('href'), 'https://trust.example.com/soc2.pdf')
  assert.equal(a.getAttribute('target'), '_blank')
  assert.equal(a.getAttribute('rel'), 'noopener noreferrer')
  // The scheme check is case-insensitive.
  assert.equal(rowOf(host, 'Upper scheme').querySelector('a')?.getAttribute('href'), 'HTTP://Upper.example.org/x')
})

test('a javascript: URL renders no href at all', async () => {
  const { html, host } = await mount()
  const row = rowOf(host, 'Sneaky')
  assert.ok(row.textContent.includes('Sneaky'), 'label still shown')
  assert.equal(row.querySelector('a'), null, 'no anchor for a non-http(s) URL')
  assert.ok(!html.includes('javascript:'), 'the script URL must not reach the markup')
})

test('a read-only viewer sees no Add, Edit, Verify or Remove controls', async () => {
  const { host } = await mount({ editable: false })
  assert.equal(host.querySelectorAll('button').length, 0, [...host.querySelectorAll('button')].map(b => b.textContent).join('|'))
})

test('control: an editor sees the controls', async () => {
  const { host } = await mount({ editable: true })
  const labels = [...host.querySelectorAll('button')].map(b => b.textContent.trim())
  for (const want of ['Add', 'Edit', 'Mark verified today', 'Delete']) {
    assert.ok(labels.includes(want), `${want} missing from ${labels}`)
  }
})

test('DATE columns show the stored calendar day, not a shifted one', async () => {
  const { host } = await mount()
  const soc = rowOf(host, 'SOC 2 report').textContent
  assert.ok(soc.includes('5 Mar 2026'), soc)
  assert.ok(soc.includes('2 Jan 2026'), soc)
  assert.ok(soc.includes('alice@example.com'), soc)
  assert.ok(rowOf(host, 'Upper scheme').textContent.includes('31 Dec 2026'))
})

test('an empty list says so', async () => {
  const { html } = await mount({}, [])
  assert.ok(html.includes('No external links'), html)
})
