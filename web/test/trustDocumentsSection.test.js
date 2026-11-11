// The Documents section of the trust center manager screen (#520). Mounts the
// real component with a stubbed fetch and reads what lands on the page.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const Section = await compileSfc(new URL('../src/components/trust/TrustDocumentsSection.vue', import.meta.url).pathname, { stubChildren: true })

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}

const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const served = v => ({ version: v, approved_at: 1760000000, content_hash: 'h' })
const row = (id, docId, title, order, over = {}, doc = {}) => ({
  id, kind: 'document', section: '', access_level: 'gated', sort_order: order, hidden_reason: '', published_at: 1760000000,
  document: {
    document_id: docId, title, classification: '', status: 'published', document_status: 'approved', document_version: '1.0',
    served_version: served('1.0'), ...doc,
  },
  ...over,
})
const A = '11111111-1111-4111-8111-111111111111'
const B = '22222222-2222-4222-8222-222222222222'
const C = '33333333-3333-4333-8333-333333333333'
const D = '44444444-4444-4444-8444-444444444444'

// `handler(method, path, body)` may return a Response; anything it returns
// undefined for falls through to the default list. Every call is recorded.
function stub(list, handler = () => undefined) {
  const calls = []
  const state = { list }
  globalThis.fetch = async (url, opts = {}) => {
    const method = opts.method || 'GET'
    const path = String(url).replace(/^.*\/api\/v1/, '')
    const body = opts.body ? JSON.parse(opts.body) : undefined
    calls.push({ method, path, body })
    const r = handler(method, path, body, state)
    if (r) return r
    if (method === 'GET' && path === '/trust-center/items') return json(200, state.list)
    return json(404, {})
  }
  return calls
}

const ready = h => h.includes('data-trust-row') || h.includes('data-trust="empty"') || h.includes('data-trust="load-error"')
const mount = (interact, isReady = ready) => renderSfcSettled(Section, {}, isReady, interact)
const click = (host, sel) => {
  const el = host.querySelector(sel)
  assert.ok(el, `${sel} not found: ${host.innerHTML}`)
  el.click()
}
const rowEl = (host, docId) => host.querySelector(`[data-trust-row="${docId}"]`)
const titles = host => [...host.querySelectorAll('[data-trust-row] [data-trust="title"]')].map(e => e.textContent.trim())
const flush = () => new Promise(r => setTimeout(r, 30))

test('rows follow the public page order: sort_order, then title', async () => {
  stub([row(A, 'pol-a', 'Zulu', 2), row(B, 'pol-b', 'Mike', 0), row(C, 'pol-c', 'Bravo', 0)])
  let seen
  await mount(async host => { seen = titles(host) })
  assert.deepEqual(seen, ['Bravo', 'Mike', 'Zulu'])
})

test('every status is shown with its text; classification and source appear when set', async () => {
  stub([
    row(A, 'pol-a', 'Pub', 1),
    row(B, 'pol-b', 'Drafty', 2, {}, { status: 'draft_in_progress', document_status: 'draft', document_version: '2.0' }),
    row(C, 'pol-c', 'Never', 3, {}, { status: 'cannot_publish', served_version: null }),
    row(D, 'pol-d', 'Old', 4, { hidden_reason: 'retired' }, { status: 'hidden', document_status: 'retired' }),
    row('55555555-5555-4555-8555-555555555555', 'pol-e', 'Gone', 5, { hidden_reason: 'deleted' }, { status: 'hidden', document_status: '' }),
    row('66666666-6666-4666-8666-666666666666', 'pol-f', 'Broken', 6, {}, { status: 'error', title: 'Broken' }),
    row('77777777-7777-4777-8777-777777777777', 'pol-g', 'Secret', 7, {}, { classification: 'Confidential' }),
  ])
  let text = {}
  await mount(async host => {
    for (const id of ['pol-a', 'pol-b', 'pol-c', 'pol-d', 'pol-e', 'pol-f']) text[id] = rowEl(host, id).querySelector('[data-trust="status"]').textContent.trim()
    text.g = rowEl(host, 'pol-g').textContent
  })
  assert.equal(text['pol-a'], 'Published')
  assert.equal(text['pol-b'], 'Draft v2.0 in progress, visitors see v1.0')
  assert.equal(text['pol-c'], 'Cannot publish: no approved version')
  assert.equal(text['pol-d'], 'Retired, hidden')
  assert.equal(text['pol-e'], 'Deleted, hidden')
  assert.equal(text['pol-f'], 'Could not read this document')
  assert.ok(text.g.includes('Classification: Confidential') && text.g.includes('Document'), text.g)
})

test('row actions depend on the state of the row', async () => {
  stub([
    row(A, 'pol-a', 'Visible', 1),
    row(B, 'pol-b', 'StillRetired', 2, { hidden_reason: 'retired' }, { status: 'hidden', document_status: 'retired' }),
    row(C, 'pol-c', 'Restored', 3, { hidden_reason: 'retired' }, { status: 'hidden', document_status: 'approved' }),
    row(D, 'pol-d', 'Deleted', 4, { hidden_reason: 'deleted' }, { status: 'hidden', document_status: '' }),
  ])
  const q = {}
  await mount(async host => {
    for (const id of ['pol-a', 'pol-b', 'pol-c', 'pol-d']) {
      const r = rowEl(host, id)
      const pa = r.querySelector('[data-trust="publish-again"]')
      q[id] = { unpublish: !!r.querySelector('[data-trust="unpublish"]'), pa: pa ? (pa.disabled ? 'disabled' : 'enabled') : null, remove: !!r.querySelector('[data-trust="remove"]'), hint: r.textContent.includes('Retired; restore the document first') }
    }
  })
  assert.deepEqual(q['pol-a'], { unpublish: true, pa: null, remove: false, hint: false })
  assert.deepEqual(q['pol-b'], { unpublish: false, pa: 'disabled', remove: false, hint: true })
  assert.deepEqual(q['pol-c'], { unpublish: false, pa: 'enabled', remove: false, hint: false })
  assert.deepEqual(q['pol-d'], { unpublish: false, pa: null, remove: true, hint: false })
})

test('up and down are disabled at the ends; down on the first row sends the full swapped id list and shows the answer', async () => {
  const list = [row(A, 'pol-a', 'First', 1), row(B, 'pol-b', 'Second', 2), row(C, 'pol-c', 'Third', 3)]
  const calls = stub(list, (m, p, body) => {
    if (m === 'PUT' && p === '/trust-center/items/order') return json(200, [row(B, 'pol-b', 'Second', 1), row(A, 'pol-a', 'First', 2), row(C, 'pol-c', 'Third', 3)])
  })
  let ends, after
  await mount(async (host, settle) => {
    ends = [rowEl(host, 'pol-a').querySelector('[data-trust="up"]').disabled, rowEl(host, 'pol-c').querySelector('[data-trust="down"]').disabled, rowEl(host, 'pol-b').querySelector('[data-trust="up"]').disabled]
    click(rowEl(host, 'pol-a'), '[data-trust="down"]')
    await settle(() => calls.some(c => c.method === 'PUT'))
    await flush()
    after = titles(host)
  })
  assert.deepEqual(ends, [true, true, false])
  assert.deepEqual(calls.find(c => c.method === 'PUT').body, { kind: 'document', ids: [B, A, C] })
  assert.deepEqual(after, ['Second', 'First', 'Third'])
})

test('a stale order (409) reloads the list and shows the localized error', async () => {
  const list = [row(A, 'pol-a', 'First', 1), row(B, 'pol-b', 'Second', 2)]
  const calls = stub(list, (m, p, body, st) => {
    if (m === 'PUT') { st.list = [row(B, 'pol-b', 'Second', 1), row(C, 'pol-c', 'Third', 2), row(A, 'pol-a', 'First', 3)]; return json(409, { message: 'raw', code: 'trust_order_stale' }) }
  })
  let text, shown
  await mount(async (host, settle) => {
    click(rowEl(host, 'pol-a'), '[data-trust="down"]')
    await settle(h => h.includes('action-error'))
    await flush()
    text = host.querySelector('[data-trust="action-error"]').textContent
    shown = titles(host)
  })
  assert.ok(text.includes('the trust center changed while you were reordering'), text)
  assert.equal(calls.filter(c => c.method === 'GET' && c.path === '/trust-center/items').length, 2)
  assert.deepEqual(shown, ['Second', 'Third', 'First'])
})

test('changing the level PATCHes the item and shows the returned row', async () => {
  const calls = stub([row(A, 'pol-a', 'Doc', 1)], (m, p, body) => {
    if (m === 'PATCH') return json(200, row(A, 'pol-a', 'Doc', 1, { access_level: body.access_level }))
  })
  let level
  await mount(async (host, settle) => {
    const sel = host.querySelector('[data-trust="level"]')
    sel.value = 'restricted'
    sel.dispatchEvent(new window.Event('change'))
    await settle(() => calls.some(c => c.method === 'PATCH'))
    await flush()
    level = host.querySelector('[data-trust="level"]').value
  })
  assert.deepEqual(calls.find(c => c.method === 'PATCH'), { method: 'PATCH', path: `/trust-center/items/${A}`, body: { access_level: 'restricted' } })
  assert.equal(level, 'restricted')
})

const needsConfirm = { message: 'confirm', code: 'trust_classification_confirm', params: { value: 'confidential' } }

test('D1: going public on a confidential document asks, then resends the same PATCH with the flag', async () => {
  let n = 0
  const calls = stub([row(A, 'pol-a', 'Doc', 1, {}, { classification: 'Confidential' })], (m, p, body) => {
    if (m === 'PATCH') return ++n === 1 ? json(409, needsConfirm) : json(200, row(A, 'pol-a', 'Doc', 1, { access_level: 'public' }, { classification: 'Confidential' }))
  })
  let dialog
  await mount(async (host, settle) => {
    const sel = host.querySelector('[data-trust="level"]')
    sel.value = 'public'
    sel.dispatchEvent(new window.Event('change'))
    await settle(h => h.includes('classification-dialog'))
    dialog = host.querySelector('[data-trust="classification-dialog"]').textContent
    click(host, '[data-trust="classification-confirm"]')
    await settle(() => calls.filter(c => c.method === 'PATCH').length === 2)
    await flush()
  })
  assert.ok(dialog.includes('classified confidential'), dialog)
  assert.deepEqual(calls.filter(c => c.method === 'PATCH').map(c => c.body), [{ access_level: 'public' }, { access_level: 'public', confirm_classification: true }])
})

test('D1: Cancel leaves the stored level in the select and sends nothing more', async () => {
  const calls = stub([row(A, 'pol-a', 'Doc', 1)], (m) => (m === 'PATCH' ? json(409, needsConfirm) : undefined))
  let level
  await mount(async (host, settle) => {
    const sel = host.querySelector('[data-trust="level"]')
    sel.value = 'public'
    sel.dispatchEvent(new window.Event('change'))
    await settle(h => h.includes('classification-dialog'))
    click(host, '[data-trust="classification-cancel"]')
    await flush()
    level = host.querySelector('[data-trust="level"]').value
  })
  assert.equal(calls.filter(c => c.method === 'PATCH').length, 1)
  assert.equal(level, 'gated')
})

test('Unpublish asks first, then DELETEs the document item and reloads', async () => {
  const calls = stub([row(A, 'pol-a', 'Doc', 1)], (m, p, b, st) => {
    if (m === 'DELETE') { st.list = []; return new Response(null, { status: 204 }) }
  })
  let before, empty
  await mount(async (host, settle) => {
    click(host, '[data-trust="unpublish"]')
    await settle(h => h.includes('confirm-dialog'))
    before = calls.filter(c => c.method === 'DELETE').length
    click(host, '[data-trust="confirm-yes"]')
    await settle(h => h.includes('data-trust="empty"'))
    empty = true
  })
  assert.equal(before, 0)
  assert.deepEqual(calls.find(c => c.method === 'DELETE'), { method: 'DELETE', path: '/documents/pol-a/trust-center', body: undefined })
  assert.ok(empty)
})

test('Publish again on a restored hidden row POSTs at its stored level', async () => {
  const calls = stub([row(C, 'pol-c', 'Restored', 3, { hidden_reason: 'retired', access_level: 'restricted' }, { status: 'hidden', document_status: 'approved' })], (m, p, b) => {
    if (m === 'POST') return json(200, {})
  })
  await mount(async (host, settle) => {
    click(host, '[data-trust="publish-again"]')
    await settle(() => calls.some(c => c.method === 'POST'))
  })
  assert.deepEqual(calls.find(c => c.method === 'POST'), { method: 'POST', path: '/documents/pol-c/trust-center', body: { access_level: 'restricted' } })
})

const picked = { document_id: 'pol-n', title: 'New Doc' }
const trustState = (over = {}) => ({ published: false, hidden_reason: '', served_version: served('1.0'), document_status: 'approved', document_version: '1.0', can_publish: true, cannot_publish_reason: '', ...over })

test('picker: shows why a document cannot be published and keeps Publish disabled', async () => {
  stub([row(A, 'pol-a', 'Doc', 1)], (m, p) => {
    if (m === 'GET' && p === '/documents/pol-n/trust-center') return json(200, trustState({ can_publish: false, cannot_publish_reason: 'no_approved_version', served_version: null }))
  })
  let reason, disabled
  await mount(async (host, settle) => {
    click(host, '[data-trust="open-picker"]')
    await settle(h => h.includes('data-trust="picker"'))
    globalThis.__sfcStubAttrs.at(-1).onPick(picked)
    await settle(h => h.includes('picker-reason'))
    reason = host.querySelector('[data-trust="picker-reason"]').textContent.trim()
    disabled = host.querySelector('[data-trust="picker-publish"]').disabled
  })
  assert.equal(reason, 'No approved version yet. Get the document approved before publishing.')
  assert.equal(disabled, true)
})

test('picker: publishing with the default level sends no level, then reloads the list', async () => {
  let published = false
  const calls = stub([row(A, 'pol-a', 'Doc', 1)], (m, p, b, st) => {
    if (m === 'GET' && p === '/documents/pol-n/trust-center') return json(200, trustState())
    if (m === 'POST') { published = true; st.list = [...st.list, row(B, 'pol-n', 'New Doc', 2)]; return json(201, {}) }
  })
  let after
  await mount(async (host, settle) => {
    click(host, '[data-trust="open-picker"]')
    await settle(h => h.includes('data-trust="picker"'))
    globalThis.__sfcStubAttrs.at(-1).onPick(picked)
    await settle(h => h.includes('picker-publish') && !host.querySelector('[data-trust="picker-publish"]').disabled)
    click(host, '[data-trust="picker-publish"]')
    await settle(() => published)
    await flush()
    after = titles(host)
  })
  assert.deepEqual(calls.find(c => c.method === 'POST'), { method: 'POST', path: '/documents/pol-n/trust-center', body: {} })
  assert.deepEqual(after, ['Doc', 'New Doc'])
})

test('picker: existing items are excluded from the choices', async () => {
  stub([row(A, 'pol-a', 'Doc', 1), row(B, 'pol-b', 'Other', 2)])
  let attrs
  await mount(async (host, settle) => {
    click(host, '[data-trust="open-picker"]')
    await settle(h => h.includes('data-trust="picker"'))
    attrs = globalThis.__sfcStubAttrs.at(-1)
  })
  assert.deepEqual([...attrs.exclude].sort(), ['pol-a', 'pol-b'])
})

test('picker: a public publish of a confidential document asks, then resends with the flag', async () => {
  let n = 0
  const calls = stub([], (m, p) => {
    if (m === 'GET' && p === '/documents/pol-n/trust-center') return json(200, trustState())
    if (m === 'POST') return ++n === 1 ? json(409, needsConfirm) : json(201, {})
  })
  await mount(async (host, settle) => {
    click(host, '[data-trust="open-picker"]')
    await settle(h => h.includes('data-trust="picker"'))
    globalThis.__sfcStubAttrs.at(-1).onPick(picked)
    await settle(() => host.querySelector('[data-trust="picker-publish"]') && !host.querySelector('[data-trust="picker-publish"]').disabled)
    host.querySelector('input[value="public"]').click()
    click(host, '[data-trust="picker-publish"]')
    await settle(h => h.includes('classification-dialog'))
    click(host, '[data-trust="classification-confirm"]')
    await settle(() => calls.filter(c => c.method === 'POST').length === 2)
  })
  assert.deepEqual(calls.filter(c => c.method === 'POST').map(c => c.body), [{ access_level: 'public' }, { access_level: 'public', confirm_classification: true }])
})

test('a failing list shows an error with Retry', async () => {
  globalThis.fetch = async () => json(500, { message: 'boom' })
  const html = await mount(null)
  assert.ok(html.includes('Retry'), html)
})
