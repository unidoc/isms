// The trust center control on the document page (#518). Mounts the real
// component with a stubbed fetch and reads what lands on the page.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled, stripComments } from './support/renderSfc.js'

const Control = await compileSfc(new URL('../src/components/TrustCenterPublishControl.vue', import.meta.url).pathname)

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}

const json = (status, body) => new Response(JSON.stringify(body), {
  status, headers: { 'Content-Type': 'application/json' },
})

const base = {
  published: false, hidden_reason: '', served_version: null,
  document_status: 'draft', document_version: '0.1', can_publish: false, cannot_publish_reason: '',
}
const served = (version) => ({ version, approved_at: 1760000000, content_hash: 'h' })

// Answers GET with `state`, records every call, and lets a scenario override
// the POST / DELETE answers.
function stub(state, { post, del } = {}) {
  const calls = []
  globalThis.fetch = async (url, opts = {}) => {
    const method = opts.method || 'GET'
    calls.push({ method, url: String(url), body: opts.body ? JSON.parse(opts.body) : undefined })
    if (!String(url).endsWith('/documents/pol-1/trust-center')) return json(404, {})
    if (method === 'POST') return post ? post() : json(201, state)
    if (method === 'DELETE') return del ? del() : new Response(null, { status: 204 })
    return json(200, state)
  }
  return calls
}

const mount = (role, interact, ready) =>
  renderSfcSettled(Control, { docId: 'pol-1', role }, ready || (h => h.includes('data-trust') || h.includes('Retry')), interact)
const click = (host, sel) => {
  const el = host.querySelector(sel)
  assert.ok(el, `${sel} not found: ${host.innerHTML}`)
  el.click()
}

test('a. publishable: Publish button enabled, no reason text', async () => {
  stub({ ...base, served_version: served('0.1'), document_status: 'approved', can_publish: true })
  const html = await mount('manager')
  assert.ok(html.includes('Publish to Trust Center'), html)
  assert.ok(!/data-trust="publish"[^>]*\sdisabled=""/.test(html), html)
  assert.ok(!html.includes('data-trust="reason"'), html)
})

test('b. no approved version: disabled with the reason', async () => {
  stub({ ...base, cannot_publish_reason: 'no_approved_version' })
  const html = await mount('admin')
  assert.ok(/data-trust="publish"[^>]*\sdisabled=""/.test(html), html)
  assert.ok(html.includes('No approved version yet. Get the document approved before publishing.'), html)
})

test('b. retired: disabled with the reason', async () => {
  stub({ ...base, document_status: 'retired', cannot_publish_reason: 'retired' })
  const html = await mount('manager')
  assert.ok(/data-trust="publish"[^>]*\sdisabled=""/.test(html), html)
  assert.ok(html.includes('Retired documents cannot be published.'), html)
})

test('c. published: status line and Unpublish; no draft note when HEAD is the served version', async () => {
  stub({ ...base, published: true, access_level: 'public', served_version: served('0.1'), document_status: 'approved', document_version: '0.1', can_publish: true })
  const html = await mount('manager')
  assert.ok(html.includes('On Trust Center · Public · visitors see v0.1'), html)
  assert.ok(html.includes('Unpublish'), html)
  assert.ok(!html.includes('in progress'), html)
})

test('c. published with a newer draft: says visitors still see the served version', async () => {
  stub({ ...base, published: true, access_level: 'gated', served_version: served('0.1'), document_status: 'draft', document_version: '0.2' })
  const html = await mount('manager')
  assert.ok(html.includes('Draft v0.2 in progress — visitors still see v0.1.'), html)
  assert.ok(html.includes('Gated'), html)
})

test('d. hidden after retire: says why and offers Publish again', async () => {
  stub({ ...base, hidden_reason: 'retired', served_version: served('0.1'), document_status: 'approved', can_publish: true })
  const html = await mount('manager')
  assert.ok(html.includes('Hidden from the Trust Center: the document was retired.'), html)
  assert.ok(html.includes('Publish again'), html)
})

test('role: reader and contributor see nothing and send no request', async () => {
  for (const role of ['reader', 'contributor', '']) {
    const calls = stub({ ...base, can_publish: true })
    const html = stripComments(await mount(role, null, () => true))
    assert.equal(html, '', `${role}: ${html}`)
    assert.equal(calls.length, 0, `${role} made requests`)
  }
})

test('publish with the default level sends no access_level', async () => {
  const published = { ...base, published: true, access_level: 'gated', served_version: served('0.1'), document_status: 'approved' }
  const calls = stub({ ...base, served_version: served('0.1'), document_status: 'approved', can_publish: true }, { post: () => json(201, published) })
  const html = await mount('manager', async (host, settle) => {
    click(host, '[data-trust="publish"]')
    await settle(h => h.includes('publish-dialog'))
    click(host, '[data-trust="publish-confirm"]')
    await settle(h => h.includes('data-trust="published"'))
  })
  const post = calls.find(c => c.method === 'POST')
  assert.ok(post, 'no POST sent')
  assert.deepEqual(post.body, {})
  assert.ok(html.includes('On Trust Center'), html)
})

test('publish with a chosen level sends it', async () => {
  const calls = stub({ ...base, served_version: served('0.1'), document_status: 'approved', can_publish: true })
  await mount('manager', async (host, settle) => {
    click(host, '[data-trust="publish"]')
    await settle(h => h.includes('publish-dialog'))
    const radio = host.querySelector('input[value="restricted"]')
    radio.click()
    click(host, '[data-trust="publish-confirm"]')
    await settle(() => calls.some(c => c.method === 'POST'))
  })
  assert.deepEqual(calls.find(c => c.method === 'POST').body, { access_level: 'restricted' })
})

test('unpublish sends DELETE only after the in-app confirmation', async () => {
  const calls = stub({ ...base, published: true, access_level: 'public', served_version: served('0.1'), document_status: 'approved' })
  let before
  await mount('manager', async (host, settle) => {
    click(host, '[data-trust="unpublish"]')
    await settle(h => h.includes('unpublish-dialog'))
    before = calls.filter(c => c.method === 'DELETE').length
    click(host, '[data-trust="unpublish-confirm"]')
    await settle(() => calls.some(c => c.method === 'DELETE'))
  })
  assert.equal(before, 0, 'DELETE sent before confirming')
  assert.equal(calls.filter(c => c.method === 'DELETE').length, 1)
})

test('a 409 shows the localized error, not raw text', async () => {
  stub({ ...base, served_version: served('0.1'), document_status: 'approved', can_publish: true },
    { post: () => json(409, { message: 'raw english', code: 'trust_already_published' }) })
  const html = await mount('manager', async (host, settle) => {
    click(host, '[data-trust="publish"]')
    await settle(h => h.includes('publish-dialog'))
    click(host, '[data-trust="publish-confirm"]')
    await settle(h => h.includes('dialog-error'))
  })
  assert.ok(html.includes('this document is already published in the trust center'), html)
  assert.ok(!html.includes('raw english'), html)
})

test('a failing state request shows an error with Retry', async () => {
  globalThis.fetch = async () => json(500, { message: 'boom' })
  const html = await mount('manager')
  assert.ok(html.includes('Retry'), html)
})

const needsConfirm = { message: 'confirm', code: 'trust_classification_confirm', params: { value: 'confidential' } }

test('D1: a 409 trust_classification_confirm asks, and Publish anyway resends the same request with the flag', async () => {
  const published = { ...base, published: true, access_level: 'public', served_version: served('0.1'), document_status: 'approved' }
  let n = 0
  const calls = stub({ ...base, served_version: served('0.1'), document_status: 'approved', can_publish: true },
    { post: () => (++n === 1 ? json(409, needsConfirm) : json(201, published)) })
  const html = await mount('manager', async (host, settle) => {
    click(host, '[data-trust="publish"]')
    await settle(h => h.includes('publish-dialog'))
    host.querySelector('input[value="public"]').click()
    click(host, '[data-trust="publish-confirm"]')
    await settle(h => h.includes('classification-dialog'))
    const text = host.querySelector('[data-trust="classification-dialog"]').textContent
    assert.ok(text.includes('This document is classified confidential. Publishing it as Public makes it readable by anyone on the internet.'), text)
    click(host, '[data-trust="classification-confirm"]')
    await settle(h => h.includes('data-trust="published"'))
  })
  const posts = calls.filter(c => c.method === 'POST')
  assert.deepEqual(posts.map(p => p.body), [{ access_level: 'public' }, { access_level: 'public', confirm_classification: true }])
  assert.ok(html.includes('On Trust Center'), html)
})

test('D1: Cancel on the classification dialog sends nothing more and leaves the document unpublished', async () => {
  const calls = stub({ ...base, served_version: served('0.1'), document_status: 'approved', can_publish: true },
    { post: () => json(409, needsConfirm) })
  const html = await mount('manager', async (host, settle) => {
    click(host, '[data-trust="publish"]')
    await settle(h => h.includes('publish-dialog'))
    host.querySelector('input[value="public"]').click()
    click(host, '[data-trust="publish-confirm"]')
    await settle(h => h.includes('classification-dialog'))
    click(host, '[data-trust="classification-cancel"]')
    await settle(h => !h.includes('classification-dialog'))
  })
  assert.equal(calls.filter(c => c.method === 'POST').length, 1)
  assert.ok(!html.includes('data-trust="published"'), html)
})

test('hidden after retire: Remove is offered next to Publish again and DELETEs after the confirmation', async () => {
  const calls = stub({ ...base, hidden_reason: 'retired', served_version: served('0.1'), document_status: 'retired', cannot_publish_reason: 'retired' })
  let before
  const html = await mount('manager', async (host, settle) => {
    click(host, '[data-trust="remove"]')
    await settle(h => h.includes('unpublish-dialog'))
    before = calls.filter(c => c.method === 'DELETE').length
    click(host, '[data-trust="unpublish-confirm"]')
    await settle(() => calls.some(c => c.method === 'DELETE'))
  })
  assert.equal(before, 0)
  assert.equal(calls.filter(c => c.method === 'DELETE').length, 1)
  assert.ok(html.includes('Publish again'), html)
})

test('not hidden and not published: no Remove', async () => {
  stub({ ...base, served_version: served('0.1'), document_status: 'approved', can_publish: true })
  const html = await mount('manager')
  assert.ok(!html.includes('data-trust="remove"'), html)
})
