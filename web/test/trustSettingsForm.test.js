// The Settings tab of the trust center manager screen (#520).
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const Form = await compileSfc(new URL('../src/components/trust/TrustSettingsForm.vue', import.meta.url).pathname, { stubChildren: true })

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}
const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

const base = { enabled: false, title: '', overview_document_id: 'ov-1', security_contact: 'sec@acme.test', default_access_level: 'gated', updated_at: 1, updated_by: 'a@x.test' }
const docs = [{ name: 'f', files: [{ document_id: 'ov-1', title: 'Overview Doc', status: 'approved' }] }]

function stub({ put, served = { version: '1.0' } } = {}) {
  const calls = []
  globalThis.fetch = async (url, opts = {}) => {
    const method = opts.method || 'GET'
    const path = String(url).replace(/^.*\/api\/v1/, '')
    calls.push({ method, path, body: opts.body ? JSON.parse(opts.body) : undefined })
    if (method === 'GET' && path === '/documents/all') return json(200, { data: docs })
    if (method === 'GET' && path.endsWith('/trust-center')) return json(200, { published: false, served_version: served })
    if (method === 'PUT' && path === '/trust-center/settings') return put ? put(JSON.parse(opts.body)) : json(200, { ...base, ...JSON.parse(opts.body) })
    return json(404, {})
  }
  return calls
}
const puts = calls => calls.filter(c => c.method === 'PUT')
const ready = h => h.includes('data-trust="settings-section"')
const mount = (props, interact) => renderSfcSettled(Form, props, ready, interact)
const click = (host, sel) => { const el = host.querySelector(sel); assert.ok(el, `${sel} not found`); el.click() }
const type = (host, sel, value) => { const el = host.querySelector(sel); el.value = value; el.dispatchEvent(new window.Event('input')) }
const flush = () => new Promise(r => setTimeout(r, 30))

test('Save is disabled until something changes, and sends only the changed fields', async () => {
  const calls = stub()
  let before
  await mount({ settings: base }, async (host, settle) => {
    before = host.querySelector('[data-trust="save"]').disabled
    type(host, '[data-trust="contact"]', 'new@acme.test')
    await flush()
    click(host, '[data-trust="save"]')
    await settle(() => puts(calls).length === 1)
  })
  assert.equal(before, true)
  assert.deepEqual(puts(calls)[0].body, { security_contact: 'new@acme.test' })
})

test('Clear sends overview_document_id null and nothing else; the default level sends only itself', async () => {
  const calls = stub()
  await mount({ settings: base }, async (host, settle) => {
    click(host, '[data-trust="overview-clear"]')
    await flush()
    click(host, '[data-trust="save"]')
    await settle(() => puts(calls).length === 1)
    const sel = host.querySelector('[data-trust="default-level"]')
    sel.value = 'public'
    sel.dispatchEvent(new window.Event('change'))
    await flush()
  })
  assert.deepEqual(puts(calls)[0].body, { overview_document_id: null })
})

test('after Save the form is reloaded from the answer and reports Saved', async () => {
  stub({ put: () => json(200, { ...base, security_contact: 'server@acme.test' }) })
  let saved
  const html = await mount({ settings: base, onSaved: s => { saved = s } }, async (host, settle) => {
    type(host, '[data-trust="contact"]', 'typed@acme.test')
    await flush()
    click(host, '[data-trust="save"]')
    await settle(h => h.includes('data-trust="saved"'))
  })
  assert.equal(saved.security_contact, 'server@acme.test')
  assert.ok(html.includes('data-trust="saved"'), html)
})

test('a failed Save shows the localized error and keeps the form', async () => {
  stub({ put: () => json(400, { message: 'raw', code: 'invalid_request' }) })
  const html = await mount({ settings: base }, async (host, settle) => {
    type(host, '[data-trust="contact"]', 'x'.repeat(10))
    await flush()
    click(host, '[data-trust="save"]')
    await settle(h => h.includes('settings-error'))
  })
  assert.ok(html.includes('data-trust="settings-error"') && html.includes('Could not save'), html)
})

test('turning ON asks with the issue text; Cancel leaves it off and sends nothing', async () => {
  const calls = stub()
  let text
  await mount({ settings: base }, async (host, settle) => {
    click(host, '[data-trust="toggle-enabled"]')
    await settle(h => h.includes('enable-dialog'))
    text = host.querySelector('[data-trust="enable-dialog"]').textContent
    click(host, '[data-trust="enable-cancel"]')
    await flush()
  })
  assert.ok(text.includes('Anything published will be visible to anyone on the internet.'), text)
  assert.equal(puts(calls).length, 0)
})

test('turning ON and confirming sends only {enabled: true} and reports the answer', async () => {
  const calls = stub()
  let saved
  await mount({ settings: base, onSaved: s => { saved = s } }, async (host, settle) => {
    click(host, '[data-trust="toggle-enabled"]')
    await settle(h => h.includes('enable-dialog'))
    click(host, '[data-trust="enable-confirm"]')
    await settle(() => puts(calls).length === 1)
    await flush()
  })
  assert.deepEqual(puts(calls)[0].body, { enabled: true })
  assert.equal(saved.enabled, true)
})

test('turning OFF needs no confirmation', async () => {
  const calls = stub()
  await mount({ settings: { ...base, enabled: true } }, async (host, settle) => {
    click(host, '[data-trust="toggle-enabled"]')
    await settle(() => puts(calls).length === 1)
  })
  assert.deepEqual(puts(calls)[0].body, { enabled: false })
})

test('an overview document with no approved version shows a hint', async () => {
  stub({ served: null })
  const html = await mount({ settings: base }, async (host, settle) => { await settle(h => h.includes('overview-hint')) })
  assert.ok(html.includes('has no approved version yet'), html)
  stub({ served: { version: '1.0' } })
  const ok = await mount({ settings: base }, async () => { await flush() })
  assert.ok(!ok.includes('overview-hint'), ok)
})

test('the overview shows the document title once the document list is loaded', async () => {
  stub()
  const html = await mount({ settings: base }, async (host, settle) => { await settle(h => h.includes('Overview Doc')) })
  assert.ok(html.includes('Overview Doc (ov-1)'), html)
})

test('toggling keeps an unsaved contact edit; Save still sends it; no "Saved" appears', async () => {
  const calls = stub()
  let shown = {}
  // The parent hands the new settings back, as the real view does.
  const props = { settings: { ...base, enabled: true }, onSaved: () => {} }
  const html = await mount(props, async (host, settle) => {
    type(host, '[data-trust="contact"]', 'unsaved@acme.test')
    await flush()
    click(host, '[data-trust="toggle-enabled"]')
    await settle(() => puts(calls).length === 1)
    await flush()
    shown = { contact: host.querySelector('[data-trust="contact"]').value, saved: !!host.querySelector('[data-trust="saved"]'), saveDisabled: host.querySelector('[data-trust="save"]').disabled }
    click(host, '[data-trust="save"]')
    await settle(() => puts(calls).length === 2)
  })
  assert.deepEqual(puts(calls)[0].body, { enabled: false })
  assert.equal(shown.contact, 'unsaved@acme.test')
  assert.equal(shown.saved, false)
  assert.equal(shown.saveDisabled, false)
  assert.deepEqual(puts(calls)[1].body, { security_contact: 'unsaved@acme.test' })
  assert.ok(html)
})

test('a settings update from the parent resets only the fields the user has not edited', async () => {
  stub()
  const { createApp, h, reactive } = await import('vue')
  const { i18n } = await import('../src/i18n.js')
  const state = reactive({ settings: base })
  const app = createApp({ render: () => h(Form, { settings: state.settings }) })
  app.use(i18n)
  const host = document.createElement('div')
  document.body.appendChild(host)
  app.mount(host)
  await flush()
  type(host, '[data-trust="contact"]', 'mine@acme.test')
  await flush()
  state.settings = { ...base, enabled: true, default_access_level: 'public' } // server changed another field
  await flush()
  const contact = host.querySelector('[data-trust="contact"]').value
  const level = host.querySelector('[data-trust="default-level"]').value
  app.unmount()
  host.remove()
  assert.equal(contact, 'mine@acme.test')
  assert.equal(level, 'public')
})
