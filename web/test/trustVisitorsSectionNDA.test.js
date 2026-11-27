// The NDA column of the trust center Visitors tab (#525).
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const Section = await compileSfc(new URL('../src/components/trust/TrustVisitorsSection.vue', import.meta.url).pathname, { stubChildren: true })

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}
const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

const visitor = (id, nda) => ({
  id, email: `v${id}@auditco.test`, name: `Name ${id}`, company: `Co ${id}`, verified_at: 1760000000,
  first_seen: 1760000000, last_seen: 1760003600, blocked: false, blocked_at: null, blocked_by: null, active_sessions: 1, nda,
})
const nda = (status, extra = {}) => ({ status, version: '', content_hash: '', accepted_at: null, accepted_version: '', ...extra })

test('the NDA column reads per status', async () => {
  const rows = [
    visitor(1, nda('accepted', { version: '2', content_hash: 'h', accepted_at: 1760000000, accepted_version: '2' })),
    visitor(2, nda('required', { version: '2', content_hash: 'h', accepted_at: 1750000000, accepted_version: '1' })),
    visitor(3, nda('required', { version: '2', content_hash: 'h' })),
    visitor(4, nda('skipped', { version: '2', content_hash: 'h' })),
    visitor(5, nda('none')),
    visitor(6, nda('unavailable')),
  ]
  globalThis.fetch = async (url) => (String(url).endsWith('/trust-center/visitors') ? json(200, rows) : json(404, {}))
  const html = await renderSfcSettled(Section, {}, h => h.includes('data-trust-visitor='))
  const host = document.createElement('div')
  host.innerHTML = html
  const cell = n => host.querySelector(`[data-trust-visitor="v${n}@auditco.test"] [data-trust="v-nda"]`)
  assert.match(html, /NDA/)
  assert.equal(cell(1).textContent.trim(), 'Accepted v2')
  assert.ok(cell(1).title.length > 0, 'the acceptance date is the tooltip')
  assert.equal(cell(2).textContent.trim(), 'Needs to accept again (accepted v1)')
  assert.equal(cell(3).textContent.trim(), 'Not accepted yet')
  assert.equal(cell(3).title, '')
  assert.equal(cell(4).textContent.trim(), 'Skipped (allowed domain)')
  assert.equal(cell(5).textContent.trim(), '-')
  assert.equal(cell(6).textContent.trim(), 'NDA unavailable')
  assert.ok(host.querySelector('table').className.includes('min-w-'), 'the min-width scroll pattern stays')
})

test('a row from a server without the field reads "-"', async () => {
  globalThis.fetch = async (url) => (String(url).endsWith('/trust-center/visitors') ? json(200, [visitor(1, undefined)]) : json(404, {}))
  const html = await renderSfcSettled(Section, {}, h => h.includes('data-trust-visitor='))
  const host = document.createElement('div')
  host.innerHTML = html
  assert.equal(host.querySelector('[data-trust="v-nda"]').textContent.trim(), '-')
})
