// The "Certifications and reports" section of the public trust center page (#521).
process.env.TZ = 'America/Los_Angeles' // a naive 'YYYY-MM-DD' parse shows the day before here
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

const F = '11111111-1111-4111-8111-111111111111'
const L = '22222222-2222-4222-8222-222222222222'
const G = '33333333-3333-4333-8333-333333333333'
const R = '44444444-4444-4444-8444-444444444444'
const D = '55555555-5555-4555-8555-555555555555'

const overview = (extra = {}) => ({ title: '', security_contact: 'security@acme.example', branding_name: 'Acme', branding_color: '', default_locale: 'en', show_powered_by: false, overview: null, ...extra })
const cert = (id, order, level, title, extra = {}) => ({
  id, kind: 'certification', section: '', sort_order: order, access_level: level, title, link_type: 'certificate', issuer: 'BSI',
  valid_until: '2027-01-01', state: 'current', source: 'file', ...extra,
})
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
const rows = (host) => [...host.querySelectorAll('[data-trust="cert-row"]')]

const ITEMS = [
  cert(L, 2, 'public', 'Pen test summary', { source: 'link', url: 'https://example.com/pentest', link_type: 'penetration_test', issuer: '', valid_until: undefined, state: undefined }),
  cert(F, 1, 'public', 'ISO 27001', { file_name: 'iso.pdf', file_size: 100, state: 'expiring', valid_until: '2026-12-01' }),
  cert(G, 3, 'gated', 'SOC 2 report', { source: 'file' }),
  cert(R, 4, 'restricted', 'Attestation', { source: 'link', link_type: 'attestation' }),
  doc(D, 'Policy'),
]

test('the section sits above the documents, rows follow sort_order, and the document filter chips stay documents-only', async () => {
  stub(ITEMS)
  const html = await mount()
  const host = dom(html)
  const order = [...host.querySelectorAll('[data-trust="certifications"], [data-trust="documents"]')].map(e => e.dataset.trust)
  assert.deepEqual(order, ['certifications', 'documents'])
  assert.deepEqual(rows(host).map(r => r.querySelector('[data-trust="cert-title"]').textContent.trim()), ['ISO 27001', 'Pen test summary', 'SOC 2 report', 'Attestation'])
  assert.equal(host.querySelectorAll('[data-trust="documents"] [data-trust="row"]').length, 1, 'documents list holds only the document')
  assert.ok(host.textContent.includes('Certifications and reports'))
  // no filter chip counts a certification: only the document's level (public) is offered
  assert.deepEqual([...host.querySelectorAll('[data-trust-filter]')].map(b => b.dataset.trustFilter), ['all', 'public'])
})

test('a public file is a plain download link to the anonymous file route', async () => {
  stub(ITEMS)
  const host = dom(await mount())
  const dl = rows(host)[0].querySelector('[data-trust="cert-download"]')
  assert.equal(dl.tagName, 'A')
  assert.equal(dl.getAttribute('href'), `/api/v1/trust/acme/certifications/${F}/file`)
  assert.ok(dl.hasAttribute('download'))
  assert.ok(!rows(host)[0].innerHTML.includes('iso.pdf'), 'the stored name is not part of the link')
})

test('a public link opens in a new tab with rel noopener noreferrer, labelled with its host', async () => {
  stub(ITEMS)
  const host = dom(await mount())
  const v = rows(host)[1].querySelector('[data-trust="cert-view"]')
  assert.equal(v.getAttribute('href'), 'https://example.com/pentest')
  assert.equal(v.getAttribute('target'), '_blank')
  assert.equal(v.getAttribute('rel'), 'noopener noreferrer')
  assert.equal(v.textContent.trim(), 'View at example.com')
})

test('a non-http(s) url is never linked', async () => {
  stub([cert(L, 1, 'public', 'Bad', { source: 'link', url: 'javascript:alert(1)' })])
  const host = dom(await mount())
  assert.equal(host.querySelector('[data-trust="cert-view"]'), null)
  assert.ok(!host.innerHTML.includes('javascript:'))
  assert.ok(host.querySelector('[data-trust="contact"]'), 'falls back to the contact')
})

test('gated and restricted rows show the contact fallback in its three variants and no url or file link', async () => {
  stub(ITEMS)
  let host = dom(await mount())
  for (const i of [2, 3]) {
    const r = rows(host)[i]
    assert.equal(r.querySelector('[data-trust="cert-download"]'), null)
    assert.equal(r.querySelector('[data-trust="cert-view"]'), null)
    const c = r.querySelector('[data-trust="contact"]')
    assert.equal(c.getAttribute('href'), 'mailto:security@acme.example')
    assert.equal(c.textContent.trim(), 'Contact security@acme.example')
  }
  stub(ITEMS, overview({ security_contact: 'https://acme.example/security' }))
  host = dom(await mount())
  const text = rows(host)[2].querySelector('[data-trust="contact"]')
  assert.equal(text.tagName, 'SPAN')
  assert.equal(text.textContent.trim(), 'Contact https://acme.example/security')
  stub(ITEMS, overview({ security_contact: '' }))
  host = dom(await mount())
  assert.equal(rows(host)[2].querySelector('[data-trust="on-request"]').textContent.trim(), 'Available on request')
})

test('badges: Current, Expiring soon, and none without a validity date; dates are the stored day', async () => {
  stub(ITEMS)
  const host = dom(await mount())
  const r = rows(host)
  assert.equal(r[0].querySelector('[data-trust="cert-state"]').textContent.trim(), 'Expiring soon')
  assert.ok(r[0].querySelector('[data-trust="cert-state"]').className.includes('amber'))
  assert.equal(r[2].querySelector('[data-trust="cert-state"]').textContent.trim(), 'Current')
  assert.equal(r[1].querySelector('[data-trust="cert-state"]'), null)
  assert.ok(r[0].textContent.includes('Valid until Dec 1, 2026'), r[0].textContent)
  assert.ok(r[2].textContent.includes('Valid until Jan 1, 2027'), r[2].textContent)
  assert.ok(r[0].textContent.includes('Certificate') && r[1].textContent.includes('Penetration test'))
})

test('no certification, no section', async () => {
  stub([doc(D, 'Policy')])
  const host = dom(await mount())
  assert.equal(host.querySelector('[data-trust="certifications"]'), null)
  assert.ok(host.querySelector('[data-trust="documents"]'))
  stub([])
  const empty = dom(await mount())
  assert.ok(empty.querySelector('[data-trust="empty"]'))
  stub([cert(F, 1, 'public', 'Only certs')])
  const only = dom(await mount())
  assert.equal(only.querySelector('[data-trust="empty"]'), null, 'a page with certifications is not empty')
})

test('in the preview the download goes through the session, not the anonymous url', async () => {
  stub(ITEMS)
  const saved = []
  globalThis.URL.createObjectURL = (b) => { saved.push(['url', b.size]); return 'blob:x' }
  globalThis.URL.revokeObjectURL = () => {}
  const asked = []
  const source = {
    overview: async () => overview(),
    items: async () => ITEMS,
    document: async () => ({}),
    certificationFile: async (id) => { asked.push(id); return { blob: new Blob(['%PDF']), filename: 'iso.pdf' } },
  }
  let html
  await mount({ source }, async (host, settle) => {
    const orig = window.HTMLAnchorElement.prototype.click
    window.HTMLAnchorElement.prototype.click = function () { saved.push(['click', this.download]) }
    const btn = host.querySelector('[data-trust="cert-download"]')
    assert.equal(btn.tagName, 'BUTTON')
    assert.ok(!host.innerHTML.includes('/api/v1/trust/acme/certifications'), 'no anonymous file url in the preview')
    btn.click()
    await settle(() => saved.some(s => s[0] === 'click'))
    window.HTMLAnchorElement.prototype.click = orig
    html = host.innerHTML
  })
  assert.deepEqual(asked, [F])
  assert.deepEqual(saved, [['url', 4], ['click', 'iso.pdf']])
  assert.ok(html)
})
