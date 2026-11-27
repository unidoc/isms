// The NDA block of the trust center Settings tab (#525): status lines, choosing
// and clearing, Set up NDA, and the skip-domain list.
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

const base = {
  enabled: true, title: '', overview_document_id: null, security_contact: 'sec@acme.test', default_access_level: 'gated',
  updated_at: 1, updated_by: 'a@x.test', privacy_notice_url: '', blocked_email_domains: [], blocked_email_domains_default: false,
  default_blocked_email_domains: [], nda_document_id: null, nda_skip_domains: [], nda_status: 'none',
}
const tree = [{ name: 'policies', files: [], subfolders: [{ name: 'legal', files: [{ document_id: 'nda-1', title: 'Our NDA', status: 'approved' }] }] }, { name: 'forms', files: [] }]

function stub({ put, create, served = { version: '1.0' } } = {}) {
  const calls = []
  globalThis.fetch = async (url, opts = {}) => {
    const method = opts.method || 'GET'
    const path = String(url).replace(/^.*\/api\/v1/, '')
    calls.push({ method, path, body: opts.body ? JSON.parse(opts.body) : undefined })
    if (method === 'GET' && path === '/documents/all') return json(200, { data: tree })
    if (method === 'GET' && path === '/documents/nda-1/trust-center') return json(200, { published: false, served_version: served })
    if (method === 'GET' && path === '/documents/nda-draft/trust-center') return json(200, { published: false, served_version: null })
    if (method === 'POST' && path === '/documents') return create ? create(JSON.parse(opts.body)) : json(201, { document_id: JSON.parse(opts.body).document_id })
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
const line = (settings) => mount({ settings }).then(h => { const d = document.createElement('div'); d.innerHTML = h; return d.querySelector('[data-trust="nda-status"], [data-trust="nda-warning"]') })

test('the status line says what the saved NDA does, with a warning when nothing can be served', async () => {
  stubNone()
  let el = await line(base)
  assert.equal(el.textContent.trim(), 'No NDA set: gated items only need sign-in.')
  assert.equal(el.dataset.trust, 'nda-status')

  el = await line({ ...base, nda_document_id: 'nda-1', nda_status: 'approved', nda_version: '2.1', nda_approved_at: 1760000000 })
  assert.equal(el.dataset.trust, 'nda-status')
  assert.match(el.textContent, /^Visitors accept version 2\.1, approved .*2025/)

  const reasons = {
    not_approved: 'The NDA has no approved version: gated items are locked for every visitor.',
    retired: 'The NDA document is retired: gated items are locked for every visitor.',
    deleted: 'The NDA document was deleted: gated items are locked for every visitor.',
    error: 'The NDA document cannot be read: gated items are locked for every visitor.',
  }
  for (const [reason, text] of Object.entries(reasons)) {
    el = await line({ ...base, nda_document_id: 'nda-1', nda_status: 'unavailable', nda_unavailable_reason: reason })
    assert.equal(el.dataset.trust, 'nda-warning', reason)
    assert.equal(el.textContent.trim(), text, reason)
    assert.ok(el.className.includes('amber'))
  }
})
function stubNone() { return stub() }

test('Clear sends nda_document_id null and nothing else', async () => {
  const calls = stub()
  await mount({ settings: { ...base, nda_document_id: 'nda-1', nda_status: 'approved', nda_version: '1' } }, async (host, settle) => {
    click(host, '[data-trust="nda-clear"]')
    await flush()
    click(host, '[data-trust="save"]')
    await settle(() => puts(calls).length === 1)
  })
  assert.deepEqual(puts(calls)[0].body, { nda_document_id: null })
})

test('the picker is open to documents that are not retired, and choosing one saves nda_document_id', async () => {
  const calls = stub()
  await mount({ settings: base }, async (host, settle) => {
    click(host, '[data-trust="nda-choose"]')
    await flush()
    const search = [...(globalThis.__sfcStubAttrs || [])].reverse().find(a => a && 'hide-retired' in a)
    assert.ok(search, 'the document search is used with hide-retired')
    search.onPick({ document_id: 'nda-1', title: 'Our NDA' })
    await flush()
    assert.ok(host.querySelector('[data-trust="nda-value"]').textContent.includes('Our NDA (nda-1)'))
    click(host, '[data-trust="save"]')
    await settle(() => puts(calls).length === 1)
  })
  assert.deepEqual(puts(calls)[0].body, { nda_document_id: 'nda-1' })
})

test('Set up NDA is offered only while none is set, and lists the existing folders', async () => {
  stub()
  let withNda, folders, defaults
  const html = await mount({ settings: base }, async (host) => {
    click(host, '[data-trust="nda-setup-open"]')
    await flush()
    folders = [...host.querySelectorAll('[data-trust="nda-setup-folder"] option')].map(o => o.textContent.trim())
    defaults = [host.querySelector('[data-trust="nda-setup-title"]').value, host.querySelector('[data-trust="nda-setup-id"]').value]
  })
  assert.ok(html.includes('data-trust="nda-setup"'))
  assert.deepEqual(folders, ['policies', 'policies/legal', 'forms'])
  assert.deepEqual(defaults, ['Non-Disclosure Agreement', 'trust-center-nda'])
  withNda = await mount({ settings: { ...base, nda_document_id: 'nda-1', nda_status: 'approved' } })
  assert.ok(!withNda.includes('nda-setup-open'))
})

test('Set up NDA only creates the draft: POST /documents, no PUT, a message with a link to open it', async () => {
  const calls = stub()
  let html
  await mount({ settings: base }, async (host, settle) => {
    click(host, '[data-trust="nda-setup-open"]')
    await flush()
    const folder = host.querySelector('[data-trust="nda-setup-folder"]')
    folder.value = 'policies/legal'
    folder.dispatchEvent(new window.Event('change'))
    type(host, '[data-trust="nda-setup-title"]', 'Mutual NDA')
    type(host, '[data-trust="nda-setup-id"]', 'Mutual-NDA')
    await flush()
    click(host, '[data-trust="nda-setup-create"]')
    await settle(h => h.includes('data-trust="nda-setup-done"'))
    html = host.innerHTML
    // the document is NOT the NDA: the form still shows none and offers Set up again
    assert.equal(host.querySelector('[data-trust="nda-value"]').textContent.trim(), 'None')
    assert.equal(host.querySelector('[data-trust="save"]').disabled, true, 'nothing to save')
  })
  const posts = calls.filter(c => c.method === 'POST')
  assert.equal(posts.length, 1)
  assert.deepEqual(posts[0].body, { folder: 'policies/legal', filename: 'mutual-nda.md', document_id: 'mutual-nda', title: 'Mutual NDA', content: '# Mutual NDA\n\n[Your NDA text]\n' })
  assert.equal(puts(calls).length, 0, 'the settings are not touched')
  const d = document.createElement('div'); d.innerHTML = html
  assert.equal(d.querySelector('[data-trust="nda-setup-done"]').textContent.replace(/\s+/g, ' ').trim(), 'Created Mutual NDA. Write the NDA, send it for review, and choose it here once it is approved. Open the document')
  assert.ok(d.querySelector('[data-trust="nda-open-doc"]') && !d.querySelector('[data-trust="nda-setup"]'))
})

test('choosing a document with no approved version warns before saving; one with a version, or an unknown status, as stated', async () => {
  for (const [id, expectWarn] of [['nda-draft', true], ['nda-1', false], ['nda-unknown', true]]) {
    const calls = stub()
    let warn, afterSave
    await mount({ settings: base }, async (host, settle) => {
      click(host, '[data-trust="nda-choose"]')
      await flush()
      const search = [...(globalThis.__sfcStubAttrs || [])].reverse().find(a => a && 'hide-retired' in a)
      search.onPick({ document_id: id, title: 'Picked' })
      await settle(() => calls.some(c => c.path === `/documents/${id}/trust-center`))
      await flush()
      const el = host.querySelector('[data-trust="nda-pick-warning"]')
      warn = el ? el.textContent.trim() : null
      assert.equal(puts(calls).length, 0, 'warned before anything is saved')
    })
    if (expectWarn) assert.equal(warn, 'This document has no approved version yet: once saved, gated items are locked for every visitor until it is approved.', id)
    else assert.equal(warn, null, id)
  }
})

test('the warning goes once the choice is saved, and the title does not fall back to the bare id', async () => {
  const calls = stub({ put: (b) => json(200, { ...base, ...b, nda_status: 'unavailable', nda_unavailable_reason: 'not_approved' }) })
  const seen = []
  let after
  await mount({ settings: base, onSaved: s => seen.push(s) }, async (host, settle) => {
    click(host, '[data-trust="nda-choose"]')
    await flush()
    const search = [...(globalThis.__sfcStubAttrs || [])].reverse().find(a => a && 'hide-retired' in a)
    search.onPick({ document_id: 'nda-draft', title: 'Draft NDA' })
    await flush()
    click(host, '[data-trust="save"]')
    await settle(() => puts(calls).length === 1)
    await flush()
    after = [host.querySelector('[data-trust="nda-value"]').textContent.trim(), !!host.querySelector('[data-trust="nda-pick-warning"]')]
  })
  assert.deepEqual(puts(calls)[0].body, { nda_document_id: 'nda-draft' })
  assert.deepEqual(after, ['Draft NDA (nda-draft)', false])
})

test('Set up NDA shows the server message inline on a 409 and does not set the NDA', async () => {
  const calls = stub({ create: () => json(409, { message: 'document id already exists', code: 'conflict' }) })
  let msg
  await mount({ settings: base }, async (host, settle) => {
    click(host, '[data-trust="nda-setup-open"]')
    await flush()
    click(host, '[data-trust="nda-setup-create"]')
    await settle(h => h.includes('data-trust="nda-setup-error"'))
    msg = host.querySelector('[data-trust="nda-setup-error"]').textContent.trim()
    assert.ok(host.querySelector('[data-trust="nda-setup"]'), 'dialog stays open')
  })
  assert.ok(msg.length > 0)
  assert.equal(puts(calls).length, 0)
})

test('skip domains: the textarea round-trips, only a changed list is sent, and a 400 shows the error', async () => {
  const calls = stub({ put: (b) => (b.nda_skip_domains && b.nda_skip_domains.includes('bad') ? json(400, { message: 'invalid', code: 'invalid_request' }) : json(200, { ...base, nda_skip_domains: ['*.acme.test', 'acme.test'] })) })
  let initial, unchanged, errShown, okSaved
  await mount({ settings: { ...base, nda_skip_domains: ['*.acme.test', 'acme.test'] } }, async (host, settle) => {
    initial = host.querySelector('[data-trust="nda-skip"]').value
    // the same entries in another order and spelling are not a change
    type(host, '[data-trust="nda-skip"]', 'ACME.test\n*.acme.test')
    await flush()
    unchanged = host.querySelector('[data-trust="save"]').disabled
    type(host, '[data-trust="nda-skip"]', 'acme.test\nbad')
    await flush()
    click(host, '[data-trust="save"]')
    await settle(h => h.includes('data-trust="nda-skip-error"'))
    errShown = !!host.querySelector('[data-trust="nda-skip-error"]')
    type(host, '[data-trust="nda-skip"]', '*.acme.test\nacme.test\nnew.test')
    await flush()
    click(host, '[data-trust="save"]')
    await settle(() => puts(calls).length === 2)
    okSaved = host.querySelector('[data-trust="nda-skip"]').value
  })
  assert.equal(initial, '*.acme.test\nacme.test')
  assert.equal(unchanged, true)
  assert.equal(errShown, true)
  assert.deepEqual(puts(calls).map(c => c.body), [{ nda_skip_domains: ['acme.test', 'bad'] }, { nda_skip_domains: ['*.acme.test', 'acme.test', 'new.test'] }])
  assert.equal(okSaved, '*.acme.test\nacme.test')
})
