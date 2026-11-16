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

test('Preview as visitor is always a link to the preview route that opens in a new tab, on or off', async () => {
  for (const enabled of [true, false]) {
    stub('admin', enabled)
    const html = await renderSfcSettled(View, {}, ready, async () => { await flush() })
    const m = html.match(/<a [^>]*data-trust="preview"[^>]*>/)
    assert.ok(m, `enabled=${enabled}: ${html}`)
    assert.match(m[0], /target="_blank"/)
    assert.match(m[0], /href="[^"]*\/trust-center\/preview"/)
    assert.ok(!html.includes('Turn the trust center on to preview it'), html)
    assert.ok(!/<button[^>]*data-trust="preview"/.test(html), html)
  }
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
  // content, visitors, settings
  assert.deepEqual(before, [false, true, true])
  assert.deepEqual(after, [true, true, false])
})

// ---- warnings (#524) ----
function stubSettings(extra) {
  globalThis.fetch = async (url) => {
    const path = String(url).replace(/^.*\/api\/v1/, '')
    if (path === '/me') return json(200, { role: 'manager', email: 'x@y.test' })
    if (path === '/trust-center/settings') return json(200, { enabled: true, title: '', overview_document_id: null, security_contact: '', default_access_level: 'gated', updated_at: 1, updated_by: 'a', email_configured: true, sign_in_paused_until: null, overview_withheld: false, ...extra })
    return json(404, {})
  }
}
const warn = (html, key) => html.includes(`data-trust="${key}"`)
const view = () => renderSfcSettled(View, {}, ready, async () => { await flush() })

test('no warning shows when nothing is wrong', async () => {
  stubSettings({})
  const html = await view()
  assert.ok(!html.includes('data-trust="warnings"'), html)
})

test('email not configured warns that visitors cannot sign in', async () => {
  stubSettings({ email_configured: false })
  const html = await view()
  assert.ok(warn(html, 'warn-email') && !warn(html, 'warn-paused') && !warn(html, 'warn-withheld'), html)
  assert.ok(html.includes('Email is not configured: visitors cannot sign in. Gated documents show the security contact instead.'), html)
})

test('a paused sign-in shows the resume time, formatted as an instant (formatDate)', async () => {
  const at = Math.floor(Date.now() / 1000) + 1800
  stubSettings({ sign_in_paused_until: at })
  const html = await view()
  const expected = new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(at * 1000))
  assert.ok(warn(html, 'warn-paused') && html.includes(`They resume at ${expected}.`), `${expected}: ${html}`)
  assert.ok(!warn(html, 'warn-email'))
})

test('a withheld overview warns that visitors see none', async () => {
  stubSettings({ overview_withheld: true })
  const html = await view()
  assert.ok(warn(html, 'warn-withheld') && html.includes('The overview document is published as gated or restricted, so visitors see no overview.'), html)
  assert.ok(!warn(html, 'warn-email') && !warn(html, 'warn-paused'))
})

test('the Visitors tab exists, with the section mounted beside the other two', async () => {
  stubSettings({})
  const html = await view()
  assert.ok(html.includes('data-trust-tab="visitors"'), html)
})

// ---- warnings stay current (#524) ----
test('a change in the Content section re-reads the settings: the withheld warning appears and goes without a reload', async () => {
  let withheld = false
  const calls = []
  globalThis.fetch = async (url) => {
    const path = String(url).replace(/^.*\/api\/v1/, '')
    calls.push(path)
    if (path === '/me') return json(200, { role: 'manager', email: 'x@y.test' })
    if (path === '/trust-center/settings') return json(200, { enabled: true, title: '', overview_document_id: 'ov', security_contact: '', default_access_level: 'gated', updated_at: 1, updated_by: 'a', email_configured: true, sign_in_paused_until: null, overview_withheld: withheld })
    return json(404, {})
  }
  globalThis.__sfcStubAttrs = []
  const seen = []
  await renderSfcSettled(View, {}, ready, async (host, settle) => {
    await flush()
    seen.push(host.innerHTML.includes('data-trust="warn-withheld"'))
    // the Documents section stub got our `changed` listener
    const changed = globalThis.__sfcStubAttrs.map(a => a.onChanged).find(Boolean)
    assert.ok(changed, 'the manager listens for `changed` from the documents section')
    withheld = true // the overview document was set to gated in the Content section
    await changed()
    await flush()
    seen.push(host.innerHTML.includes('data-trust="warn-withheld"'))
    withheld = false // ...and back to public
    await changed()
    await flush()
    seen.push(host.innerHTML.includes('data-trust="warn-withheld"'))
  })
  assert.deepEqual(seen, [false, true, false])
})

test('switching tabs reads the settings again', async () => {
  const calls = []
  globalThis.fetch = async (url) => {
    const path = String(url).replace(/^.*\/api\/v1/, '')
    calls.push(path)
    if (path === '/me') return json(200, { role: 'admin', email: 'x@y.test' })
    if (path === '/trust-center/settings') return json(200, { enabled: true, title: '', overview_document_id: null, security_contact: '', default_access_level: 'gated', updated_at: 1, updated_by: 'a', email_configured: true })
    return json(404, {})
  }
  let before, after
  await renderSfcSettled(View, {}, ready, async (host) => {
    await flush()
    before = calls.filter(p => p === '/trust-center/settings').length
    host.querySelector('[data-trust-tab="settings"]').click()
    await flush()
    after = calls.filter(p => p === '/trust-center/settings').length
  })
  assert.equal(after, before + 1)
})
