// The trust center manager screen shell (#520): role gate, header, tabs, and
// that the nav entry and route exist.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const View = await compileSfc(new URL('../src/views/TrustCenterManager.vue', import.meta.url).pathname, { stubChildren: true })

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}
globalThis.location = { origin: 'https://isms.test', hostname: 'isms.test' }
const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

function stub(role, enabled = true) {
  const calls = []
  globalThis.fetch = async (url) => {
    const path = String(url).replace(/^.*\/api\/v1/, '')
    calls.push(path)
    if (path === '/me') return json(200, { role, email: 'x@y.test' })
    if (path === '/trust-center/settings') return json(200, { enabled, title: '', overview_document_id: null, security_contact: '', default_access_level: 'gated', updated_at: 1, updated_by: 'a' })
    return json(404, {})
  }
  return calls
}
const ready = h => h.includes('data-trust="not-allowed"') || h.includes('data-trust="preview"')
const flush = () => new Promise(r => setTimeout(r, 40))

test('reader and contributor see "not allowed" and no trust center request is made', async () => {
  for (const role of ['reader', 'contributor']) {
    const calls = stub(role)
    const html = await renderSfcSettled(View, {}, ready, async () => { await flush() })
    assert.ok(html.includes('data-trust="not-allowed"'), `${role}: ${html}`)
    assert.ok(html.includes('Only admins and managers can manage the trust center.'), html)
    assert.ok(!html.includes('documents-section'), html)
    assert.deepEqual(calls.filter(p => p.startsWith('/trust-center')), [], `${role} requested the trust center`)
  }
})

test('admin and manager get the header, both tabs, and the settings request', async () => {
  for (const role of ['admin', 'manager']) {
    const calls = stub(role)
    const html = await renderSfcSettled(View, {}, ready)
    assert.ok(html.includes('data-trust-tab="content"') && html.includes('data-trust-tab="settings"'), html)
    assert.ok(html.includes('data-trust="address"') && html.includes('data-trust="copy"'), html)
    assert.ok(calls.includes('/trust-center/settings'), role)
  }
})

test('Preview as visitor is a link when the trust center is on, disabled with a hint when off (D2)', async () => {
  stub('admin', true)
  const on = await renderSfcSettled(View, {}, ready, async () => { await flush() })
  assert.ok(/<a [^>]*data-trust="preview"[^>]*target="_blank"/.test(on) || /<a [^>]*target="_blank"[^>]*data-trust="preview"/.test(on), on)
  stub('admin', false)
  const off = await renderSfcSettled(View, {}, ready, async () => { await flush() })
  assert.ok(/<button[^>]*data-trust="preview"[^>]*disabled/.test(off), off)
  assert.ok(off.includes('Turn the trust center on to preview it'), off)
})

const src = f => readFileSync(new URL(f, import.meta.url), 'utf8')

test('the route exists in the org-scoped table, which serves both path and subdomain mode', () => {
  const router = src('../src/router.js')
  assert.match(router, /\{ path: '\/trust-center', component: TrustCenterManager \}/)
  // orgScopedRoutes is mounted under /:org (path mode) or at the top level (tenant host).
  assert.match(router, /subdomainMode \? r\.path : '\/:org' \+ r\.path/)
})

test('the sidebar entry is shown to admins and managers only, after Tasks', () => {
  const app = src('../src/App.vue')
  const i = app.indexOf('data-nav="trust-center"')
  assert.ok(i > app.indexOf("orgPath('/tasks')"), 'entry must follow Tasks')
  const tag = app.slice(app.lastIndexOf('<router-link', i), i)
  assert.match(tag, /currentUserData\?\.role === 'admin' \|\| currentUserData\?\.role === 'manager'/)
})

test('both tab panels stay mounted so switching tabs cannot drop unsaved edits', async () => {
  stub('admin')
  let before, after
  await renderSfcSettled(View, {}, ready, async (host, settle) => {
    await flush()
    const state = () => [...host.querySelectorAll('[role="tabpanel"]')].map(p => p.style.display === 'none')
    before = state()
    host.querySelector('[data-trust-tab="settings"]').click()
    await flush()
    after = state()
  })
  assert.deepEqual(before, [false, true])
  assert.deepEqual(after, [true, false])
})
