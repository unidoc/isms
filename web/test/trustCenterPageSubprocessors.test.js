// The Subprocessors section of the public trust center page (#522).
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const Page = await compileSfc(new URL('../src/components/TrustCenterPage.vue', import.meta.url).pathname, { stubChildren: true })

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}
const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

const overview = (extra = {}) => ({ title: '', security_contact: 'security@acme.example', branding_name: 'Acme', branding_color: '', default_locale: 'en', show_powered_by: false, overview: null, ...extra })
const sp = (id, order, name, extra = {}) => ({ id, kind: 'subprocessor', section: '', sort_order: order, access_level: 'public', name, purpose: '', location: '', data_categories: '', ...extra })
const doc = (id, title) => ({ id, kind: 'document', section: '', sort_order: 0, access_level: 'public', title, version: '1.0', approved_at: 1760000000, content_hash: 'a'.repeat(64) })

function stub(items, ov = overview()) {
  globalThis.fetch = async (url) => {
    const u = String(url)
    if (u.endsWith('/api/v1/config')) return json(200, { locales: [{ tag: 'en', name: 'English' }] })
    if (u.endsWith('/api/v1/trust/acme')) return json(200, ov)
    if (u.endsWith('/api/v1/trust/acme/items')) return json(200, items)
    return json(404, {})
  }
}
const ready = h => h.includes('data-trust="page"') || h.includes('data-trust="error"')
const mount = (props, interact) => renderSfcSettled(Page, { slug: 'acme', basePath: '/acme/trust', ...props }, ready, interact)
const dom = (html) => { const d = document.createElement('div'); d.innerHTML = html; return d }
const text = (el) => el.textContent.replace(/\s+/g, ' ').trim()

test('the section follows the documents, rows in sort order, four columns, an em dash for an empty cell', async () => {
  stub([
    sp('b', 2, 'Beta Ltd', { purpose: 'Email delivery', location: 'EU (Dublin)', data_categories: 'Email address' }),
    sp('a', 1, 'Alpha GmbH'),
    doc('d', 'Policy'),
  ], overview({ subprocessors_changed_at: 1760000000 }))
  const host = dom(await mount())
  const order = [...host.querySelectorAll('[data-trust="documents"], [data-trust="subprocessors"]')].map(e => e.dataset.trust)
  assert.deepEqual(order, ['documents', 'subprocessors'])
  const sec = host.querySelector('[data-trust="subprocessors"]')
  const heads = [...sec.querySelectorAll('[role="columnheader"]')].map(text)
  assert.deepEqual(heads, ['Name', 'Purpose', 'Location', 'Data categories'])
  const rows = [...sec.querySelectorAll('[data-trust="subprocessor-row"]')]
  assert.deepEqual(rows.map(r => text(r.querySelector('[data-trust="sp-name"]'))), ['Alpha GmbH', 'Beta Ltd'])
  assert.equal(text(rows[0].querySelector('[data-trust="sp-purpose"]')), '—')
  assert.equal(text(rows[0].querySelector('[data-trust="sp-location"]')), '—')
  assert.equal(text(rows[0].querySelector('[data-trust="sp-data"]')), '—')
  assert.equal(text(rows[1].querySelector('[data-trust="sp-purpose"]')), 'Email delivery')
  assert.equal(text(rows[1].querySelector('[data-trust="sp-location"]')), 'EU (Dublin)')
  assert.equal(text(rows[1].querySelector('[data-trust="sp-data"]')), 'Email address')
  assert.equal(sec.querySelector('button, a, svg'), null, 'no access-level, lock or sign-in UI')
})

test('"Last updated" shows the date from the overview; absent without it', async () => {
  stub([sp('a', 1, 'Alpha')], overview({ subprocessors_changed_at: 1760000000 }))
  let host = dom(await mount())
  const upd = host.querySelector('[data-trust="subprocessors-updated"]')
  assert.ok(upd && /^Last updated \w{3} \d{1,2}, 2025$/.test(text(upd)), upd && text(upd))
  stub([sp('a', 1, 'Alpha')], overview())
  host = dom(await mount())
  assert.equal(host.querySelector('[data-trust="subprocessors-updated"]'), null)
})

test('no subprocessor items: no section; the page is not "empty" when only subprocessors exist', async () => {
  stub([doc('d', 'Policy')])
  let host = dom(await mount())
  assert.equal(host.querySelector('[data-trust="subprocessors"]'), null)
  stub([sp('a', 1, 'Alpha')])
  host = dom(await mount())
  assert.ok(host.querySelector('[data-trust="subprocessors"]'))
  assert.equal(host.querySelector('[data-trust="empty"]'), null)
})
