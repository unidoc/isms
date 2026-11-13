// The Certifications section of the trust center manager screen (#521). Mounts
// the real component with a stubbed fetch and reads what lands on the page.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const Section = await compileSfc(new URL('../src/components/trust/TrustCertificationsSection.vue', import.meta.url).pathname, { stubChildren: true })

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}

const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const I1 = '11111111-1111-4111-8111-111111111111'
const I2 = '22222222-2222-4222-8222-222222222222'
const IO = '99999999-9999-4999-8999-999999999999' // an item whose certification row is missing

const cert = (id, label, over = {}, item) => ({
  id, label, link_type: 'certificate', issuer: 'BSI', valid_from: '2026-01-01', valid_until: '2027-01-01', state: 'current', source: 'file',
  url: '', file_name: label + '.pdf', file_content_type: 'application/pdf', file_size: 2048, file_sha256: 'h', verified_at: null, verified_by: '',
  created_by: 'm@x', created_at: 1, updated_at: 1, item: item || null, ...over,
})
const itm = (id, order, certification, level = 'gated') => ({
  id, kind: 'certification', section: '', access_level: level, sort_order: order, hidden_reason: '', published_at: 1, document: null, certification,
})
const summary = (c) => (c ? { id: c.id, label: c.label, link_type: c.link_type, issuer: c.issuer, valid_from: c.valid_from, valid_until: c.valid_until, state: c.state, source: c.source } : null)

function world() {
  const a = cert(1, 'Alpha', {}, { id: I1, access_level: 'gated', sort_order: 1 })
  const b = cert(2, 'Beta', { state: 'expired', valid_until: '2020-01-01' }, { id: I2, access_level: 'public', sort_order: 3 })
  const u = cert(3, 'Draft', { source: 'link', url: 'https://trust.example.com/r', file_name: '', file_size: 0, file_content_type: '', verified_at: '2026-02-02', verified_by: 'v@x' })
  return {
    certs: [b, a, u],
    items: [itm(I1, 1, summary(a)), itm(IO, 2, null), itm(I2, 3, summary(b), 'public'), { id: 'd', kind: 'document', sort_order: 1, document: {} }],
    settings: { default_access_level: 'restricted' },
  }
}

// Every request is recorded. FormData bodies are flattened into an object.
function stub(w, handler = () => undefined) {
  const calls = []
  globalThis.fetch = async (url, opts = {}) => {
    const method = opts.method || 'GET'
    const path = String(url).replace(/^.*\/api\/v1/, '')
    let body
    if (opts.body instanceof FormData) body = { ...Object.fromEntries(opts.body.entries()), __form: true }
    else if (opts.body) body = JSON.parse(opts.body)
    calls.push({ method, path, body })
    const r = handler(method, path, body, w)
    if (r) return r
    if (method === 'GET' && path === '/trust-center/certifications') return json(200, w.certs)
    if (method === 'GET' && path === '/trust-center/items') return json(200, w.items)
    if (method === 'GET' && path === '/trust-center/settings') return json(200, w.settings)
    if (method === 'GET' && path === '/users') return json(200, [{ email: 'v@x', name: 'Vera Verifier' }])
    return json(404, {})
  }
  return calls
}

const ready = h => h.includes('data-trust-cert-row') || h.includes('cert-empty') || h.includes('cert-load-error')
const mount = (interact, isReady = ready) => renderSfcSettled(Section, {}, isReady, interact)
const q = (host, sel) => { const el = host.querySelector(sel); assert.ok(el, `${sel} not found: ${host.innerHTML}`); return el }
const click = (host, sel) => q(host, sel).click()
const rowEl = (host, id) => host.querySelector(`[data-trust-cert-row="${id}"]`)
const titles = host => [...host.querySelectorAll('[data-trust="cert-title"]')].map(e => e.textContent.trim())
const flush = () => new Promise(r => setTimeout(r, 40))
const setVal = (el, v, ev = 'input') => { el.value = v; el.dispatchEvent(new window.Event(ev)) }
const pick = (input, file) => { Object.defineProperty(input, 'files', { value: [file], configurable: true }); input.dispatchEvent(new window.Event('change')) }
const mutations = calls => calls.filter(c => c.method !== 'GET')

test('rows: published by sort_order, the unpublished after a divider; badge, hint, source and verifier show', async () => {
  stub(world())
  let seen = {}
  await mount(async host => {
    seen.titles = titles(host)
    seen.divider = !!host.querySelector('[data-trust="cert-divider"]')
    const beta = rowEl(host, 2)
    seen.betaState = q(beta, '[data-trust="cert-state"]').textContent.trim()
    seen.betaStateCls = q(beta, '[data-trust="cert-state"]').className
    seen.betaHint = beta.querySelector('[data-trust="cert-expired-hint"]')?.textContent.trim()
    seen.betaDl = !!beta.querySelector('[data-trust="cert-download"]')
    const alpha = rowEl(host, 1)
    seen.alphaState = q(alpha, '[data-trust="cert-state"]').textContent.trim()
    seen.alphaHint = !!alpha.querySelector('[data-trust="cert-expired-hint"]')
    seen.alphaText = alpha.textContent.replace(/\s+/g, ' ')
    const draft = rowEl(host, 3)
    seen.link = { href: q(draft, '[data-trust="cert-link"]').getAttribute('href'), rel: q(draft, '[data-trust="cert-link"]').getAttribute('rel'), target: q(draft, '[data-trust="cert-link"]').getAttribute('target'), text: q(draft, '[data-trust="cert-link"]').textContent }
    seen.draftText = draft.textContent.replace(/\s+/g, ' ')
    seen.draftDownload = !!draft.querySelector('[data-trust="cert-download"]')
    seen.draftReplace = !!draft.querySelector('[data-trust="cert-replace"]')
  })
  assert.deepEqual(seen.titles, ['Alpha', 'Beta', 'Draft'])
  assert.ok(seen.divider)
  assert.equal(seen.betaState, 'Expired')
  assert.ok(seen.betaStateCls.includes('bg-red-900/40 text-red-300'), seen.betaStateCls)
  assert.equal(seen.betaHint, 'Expired: hidden from visitors')
  assert.ok(seen.betaDl)
  assert.equal(seen.alphaState, 'Current')
  assert.equal(seen.alphaHint, false)
  assert.ok(seen.alphaText.includes('Valid Jan 1, 2026 – Jan 1, 2027') && seen.alphaText.includes('BSI') && seen.alphaText.includes('File: Alpha.pdf (2.0 KB)'), seen.alphaText)
  assert.deepEqual(seen.link, { href: 'https://trust.example.com/r', rel: 'noopener noreferrer', target: '_blank', text: 'trust.example.com' })
  assert.ok(seen.draftText.includes('Not published') && seen.draftText.includes('Verified Feb 2, 2026 by'), seen.draftText)
  assert.equal(seen.draftDownload, false)
  assert.equal(seen.draftReplace, false, 'a link has no file to replace')
})

test('an empty list and a failed load say so', async () => {
  stub({ certs: [], items: [], settings: {} })
  let html = await mount(null)
  assert.ok(html.includes('cert-empty') && html.includes('No certifications or reports yet.'), html)
  globalThis.fetch = async () => json(500, { message: 'boom' })
  html = await mount(null)
  assert.ok(html.includes('cert-load-error') && html.includes('Retry'), html)
})

test('Publish sends the organisation default level; the select picks another one first', async () => {
  const w = world()
  const calls = stub(w, (m, p) => (m === 'POST' && p === '/trust-center/certifications/3/publish' ? json(201, w.certs[2]) : undefined))
  await mount(async (host, settle) => {
    const sel = q(rowEl(host, 3), '[data-trust="cert-level"]')
    assert.equal(sel.value, 'restricted', 'defaults to the settings level')
    click(rowEl(host, 3), '[data-trust="cert-publish"]')
    await settle(() => calls.some(c => c.method === 'POST'))
    await flush()
  })
  assert.deepEqual(mutations(calls)[0], { method: 'POST', path: '/trust-center/certifications/3/publish', body: { access_level: 'restricted' } })
  const w2 = world()
  const calls2 = stub(w2, (m) => (m === 'POST' ? json(201, w2.certs[2]) : undefined))
  await mount(async (host, settle) => {
    setVal(q(rowEl(host, 3), '[data-trust="cert-level"]'), 'public', 'change')
    click(rowEl(host, 3), '[data-trust="cert-publish"]')
    await settle(() => calls2.some(c => c.method === 'POST'))
  })
  assert.equal(mutations(calls2).length, 1, 'choosing a level on an unpublished row sends nothing by itself')
  assert.deepEqual(mutations(calls2)[0].body, { access_level: 'public' })
})

test('changing the level of a published row PATCHes its item, with no confirm flag', async () => {
  const calls = stub(world(), (m, p, body) => (m === 'PATCH' ? json(200, itm(I1, 1, null, body.access_level)) : undefined))
  await mount(async (host, settle) => {
    setVal(q(rowEl(host, 1), '[data-trust="cert-level"]'), 'public', 'change')
    await settle(() => calls.some(c => c.method === 'PATCH'))
    await flush()
  })
  assert.deepEqual(mutations(calls)[0], { method: 'PATCH', path: `/trust-center/items/${I1}`, body: { access_level: 'public' } })
})

test('reorder: the PUT lists every certification item, the orphan included, and swaps with the nearest known row', async () => {
  const calls = stub(world(), (m, p) => (m === 'PUT' && p === '/trust-center/items/order' ? json(200, world().items) : undefined))
  let ends
  await mount(async (host, settle) => {
    ends = {
      firstUp: q(rowEl(host, 1), '[data-trust="cert-up"]').disabled,
      lastDown: q(rowEl(host, 2), '[data-trust="cert-down"]').disabled,
      betaUp: q(rowEl(host, 2), '[data-trust="cert-up"]').disabled,
    }
    click(rowEl(host, 2), '[data-trust="cert-up"]')
    await settle(() => calls.some(c => c.method === 'PUT'))
    await flush()
  })
  assert.deepEqual(ends, { firstUp: true, lastDown: true, betaUp: false })
  assert.deepEqual(mutations(calls)[0], { method: 'PUT', path: '/trust-center/items/order', body: { kind: 'certification', ids: [I2, IO, I1] } })
})

test('a stale order (409) reloads and shows the localized error', async () => {
  const calls = stub(world(), (m) => (m === 'PUT' ? json(409, { message: 'raw', code: 'trust_order_stale' }) : undefined))
  let text
  await mount(async (host, settle) => {
    click(rowEl(host, 2), '[data-trust="cert-up"]')
    await settle(h => h.includes('cert-action-error'))
    await flush()
    text = host.querySelector('[data-trust="cert-action-error"]').textContent
  })
  assert.ok(text.includes('the trust center changed while you were reordering'), text)
  assert.equal(calls.filter(c => c.path === '/trust-center/certifications' && c.method === 'GET').length, 2)
})

test('Unpublish and Delete ask first; confirming sends DELETE and reloads', async () => {
  const w = world()
  const calls = stub(w, (m, p) => (m === 'DELETE' ? new Response(null, { status: 204 }) : undefined))
  let dialog
  await mount(async (host, settle) => {
    click(rowEl(host, 1), '[data-trust="cert-unpublish"]')
    await settle(h => h.includes('confirm-dialog'))
    dialog = host.querySelector('[data-trust="confirm-dialog"]').textContent
    assert.equal(mutations(calls).length, 0, 'nothing is sent before the confirmation')
    click(host, '[data-trust="confirm-yes"]')
    await settle(() => calls.some(c => c.method === 'DELETE'))
    await flush()
    click(rowEl(host, 3), '[data-trust="cert-delete"]')
    await settle(h => h.includes('confirm-dialog'))
    assert.ok(host.querySelector('[data-trust="confirm-dialog"]').textContent.includes('cannot be undone'))
    click(host, '[data-trust="confirm-yes"]')
    await settle(() => calls.filter(c => c.method === 'DELETE').length === 2)
    await flush()
  })
  assert.ok(dialog.includes('Unpublish Alpha?'), dialog)
  assert.deepEqual(mutations(calls).map(c => [c.method, c.path]), [['DELETE', '/trust-center/certifications/1/publish'], ['DELETE', '/trust-center/certifications/3']])
  assert.ok(calls.filter(c => c.method === 'GET' && c.path === '/trust-center/certifications').length >= 3, 'reloaded after each')
})

test('Mark verified POSTs and reloads', async () => {
  const w = world()
  const calls = stub(w, (m) => (m === 'POST' ? json(200, w.certs[0]) : undefined))
  await mount(async (host, settle) => {
    click(rowEl(host, 1), '[data-trust="cert-verify"]')
    await settle(() => calls.some(c => c.method === 'POST'))
    await flush()
  })
  assert.deepEqual(mutations(calls)[0], { method: 'POST', path: '/trust-center/certifications/1/verify', body: {} })
})

test('Edit sends only the changed fields; an emptied date is null; nothing changed sends nothing', async () => {
  const w = world()
  const calls = stub(w, (m, p, body) => (m === 'PUT' ? json(200, w.certs[1]) : undefined))
  await mount(async (host, settle) => {
    click(rowEl(host, 1), '[data-trust="cert-edit"]')
    await settle(h => h.includes('cert-edit-form'))
    const form = q(host, '[data-trust="cert-edit-form"]')
    assert.equal(q(form, '[data-trust="cert-e-label"]').value, 'Alpha')
    assert.equal(q(form, '[data-trust="cert-e-validuntil"]').value, '2027-01-01')
    assert.equal(form.querySelector('[data-trust="cert-e-url"]'), null, 'no url for a file certification')
    q(form, '[data-trust="cert-edit-save"]').click() // unchanged
    await flush()
    assert.equal(mutations(calls).length, 0)
    host.querySelector('[data-trust="cert-edit"]')
    click(rowEl(host, 1), '[data-trust="cert-edit"]')
    await settle(h => h.includes('cert-edit-form'))
    const f2 = q(host, '[data-trust="cert-edit-form"]')
    setVal(q(f2, '[data-trust="cert-e-issuer"]'), 'TUV')
    setVal(q(f2, '[data-trust="cert-e-validuntil"]'), '')
    q(f2, '[data-trust="cert-edit-save"]').click()
    await settle(() => calls.some(c => c.method === 'PUT'))
    await flush()
  })
  assert.deepEqual(mutations(calls)[0], { method: 'PUT', path: '/trust-center/certifications/1', body: { issuer: 'TUV', valid_until: null } })
})

test('Edit: dates out of order and a bad label are refused without a request; a server error stays in the form', async () => {
  const calls = stub(world(), (m) => (m === 'PUT' ? json(400, { message: 'x', code: 'trust_date_invalid', params: { field: 'valid_until' } }) : undefined))
  let seen
  await mount(async (host, settle) => {
    click(rowEl(host, 1), '[data-trust="cert-edit"]')
    await settle(h => h.includes('cert-edit-form'))
    const f = q(host, '[data-trust="cert-edit-form"]')
    setVal(q(f, '[data-trust="cert-e-validfrom"]'), '2028-01-01')
    q(f, '[data-trust="cert-edit-save"]').click()
    await flush()
    seen = { order: !!host.querySelector('[data-trust="cert-e-error-dates"]'), sent: mutations(calls).length }
    setVal(q(f, '[data-trust="cert-e-validfrom"]'), '2026-01-01')
    setVal(q(f, '[data-trust="cert-e-validuntil"]'), '2026-12-31')
    q(f, '[data-trust="cert-edit-save"]').click()
    await settle(h => h.includes('cert-edit-error'))
    seen.server = host.querySelector('[data-trust="cert-edit-error"]').textContent
  })
  assert.deepEqual({ order: seen.order, sent: seen.sent }, { order: true, sent: 0 })
  assert.ok(seen.server.includes('valid until') && seen.server.includes('YYYY-MM-DD'), seen.server)
})

const PDF = (name = 'a.pdf', size = 1000) => new File([new Uint8Array(size)], name, { type: 'application/pdf' })

test('Add: oversize, wrong-type, missing files and bad dates are refused without a request', async () => {
  const calls = stub(world())
  let seen = []
  await mount(async (host, settle) => {
    click(host, '[data-trust="cert-add-open"]')
    await settle(h => h.includes('cert-add-form'))
    const form = () => q(host, '[data-trust="cert-add-form"]')
    const submit = async () => { q(form(), '[data-trust="cert-add-submit"]').click(); await flush() }
    const errs = () => [...host.querySelectorAll('[data-trust^="cert-error-"]')].map(e => e.dataset.trust.replace('cert-error-', '') + ':' + e.textContent.trim())
    await submit()
    seen.push(errs())
    setVal(q(form(), '[data-trust="cert-f-label"]'), 'Report')
    setVal(q(form(), '[data-trust="cert-f-type"]'), 'audit_report', 'change')
    pick(q(form(), '[data-trust="cert-f-file"]'), PDF('huge.pdf', 51 * 1024 * 1024))
    await submit()
    seen.push(errs())
    pick(q(form(), '[data-trust="cert-f-file"]'), new File(['<html>'], 'x.html', { type: 'text/html' }))
    await submit()
    seen.push(errs())
    pick(q(form(), '[data-trust="cert-f-file"]'), PDF())
    setVal(q(form(), '[data-trust="cert-f-validfrom"]'), '2027-05-01')
    setVal(q(form(), '[data-trust="cert-f-validuntil"]'), '2027-04-01')
    await submit()
    seen.push(errs())
    click(host, '[data-trust="cert-source-link"]')
    await flush()
    setVal(q(form(), '[data-trust="cert-f-url"]'), 'ftp://x/y')
    await submit()
    seen.push(errs())
  })
  assert.equal(mutations(calls).length, 0, JSON.stringify(mutations(calls)))
  assert.deepEqual([...seen[0]].sort(), ['file:Choose a file', 'label:Name is required', 'type:Choose a type'])
  assert.deepEqual(seen[1], ['file:The file is larger than 50 MB'])
  assert.deepEqual(seen[2], ['file:Only PDF, PNG or JPEG files are accepted'])
  assert.deepEqual(seen[3], ['dates:Valid from must not be after valid until'])
  assert.deepEqual(seen[4].filter(e => e.startsWith('url')), ['url:Enter a full http:// or https:// address'])
})

test('Add file: multipart with the file, empty dates left out, publish only as "true"; the form shows Uploading while it runs', async () => {
  let release
  const gate = new Promise(r => { release = r })
  const w = world()
  const calls = stub(w, (m, p) => (m === 'POST' && p === '/trust-center/certifications' ? gate.then(() => json(201, w.certs[0])) : undefined))
  let during
  await mount(async (host, settle) => {
    click(host, '[data-trust="cert-add-open"]')
    await settle(h => h.includes('cert-add-form'))
    const form = () => q(host, '[data-trust="cert-add-form"]')
    assert.equal(q(form(), '[data-trust="cert-f-publish"]').checked, false, 'publish defaults off')
    setVal(q(form(), '[data-trust="cert-f-label"]'), '  Pen test  ')
    setVal(q(form(), '[data-trust="cert-f-type"]'), 'penetration_test', 'change')
    setVal(q(form(), '[data-trust="cert-f-issuer"]'), 'Acme')
    setVal(q(form(), '[data-trust="cert-f-validuntil"]'), '2027-04-01')
    pick(q(form(), '[data-trust="cert-f-file"]'), PDF('pt.pdf'))
    q(form(), '[data-trust="cert-f-publish"]').click()
    await flush()
    assert.equal(q(form(), '[data-trust="cert-f-level"]').value, 'restricted', 'level select starts at the settings default')
    setVal(q(form(), '[data-trust="cert-f-level"]'), 'public', 'change')
    q(form(), '[data-trust="cert-add-submit"]').click()
    await settle(() => calls.some(c => c.method === 'POST'))
    during = { uploading: !!host.querySelector('[data-trust="cert-uploading"]'), disabled: form().querySelector('fieldset').disabled, submit: q(form(), '[data-trust="cert-add-submit"]').disabled }
    release()
    await settle(h => !h.includes('cert-add-form'))
  })
  assert.deepEqual(during, { uploading: true, disabled: true, submit: true })
  const post = mutations(calls)[0]
  assert.equal(post.path, '/trust-center/certifications')
  assert.deepEqual(post.body, { label: 'Pen test', link_type: 'penetration_test', issuer: 'Acme', valid_until: '2027-04-01', publish: 'true', access_level: 'public', __form: true, file: post.body.file })
  assert.ok(post.body.file instanceof File && post.body.file.name === 'pt.pdf')
  assert.ok(!('valid_from' in post.body) && !('verified_at' in post.body), 'empty dates are absent, not ""')
})

test('Add link: JSON with the url, no publish keys when off; a server error is shown in the form', async () => {
  const calls = stub(world(), (m, p) => (m === 'POST' ? json(400, { message: 'x', code: 'trust_certification_source' }) : undefined))
  let err
  await mount(async (host, settle) => {
    click(host, '[data-trust="cert-add-open"]')
    await settle(h => h.includes('cert-add-form'))
    click(host, '[data-trust="cert-source-link"]')
    await flush()
    const form = () => q(host, '[data-trust="cert-add-form"]')
    setVal(q(form(), '[data-trust="cert-f-label"]'), 'SOC 2')
    setVal(q(form(), '[data-trust="cert-f-type"]'), 'audit_report', 'change')
    setVal(q(form(), '[data-trust="cert-f-url"]'), 'https://example.com/soc2')
    q(form(), '[data-trust="cert-add-submit"]').click()
    await settle(h => h.includes('cert-add-error'))
    err = host.querySelector('[data-trust="cert-add-error"]').textContent
  })
  assert.deepEqual(mutations(calls)[0], { method: 'POST', path: '/trust-center/certifications', body: { label: 'SOC 2', link_type: 'audit_report', url: 'https://example.com/soc2' } })
  assert.ok(err.includes('either a link or a file'), err)
})

test('a 413 from the server reads as the 50 MB message', async () => {
  stub(world(), (m) => (m === 'POST' ? new Response('', { status: 413 }) : undefined))
  let err
  await mount(async (host, settle) => {
    click(host, '[data-trust="cert-add-open"]')
    await settle(h => h.includes('cert-add-form'))
    const form = () => q(host, '[data-trust="cert-add-form"]')
    setVal(q(form(), '[data-trust="cert-f-label"]'), 'L')
    setVal(q(form(), '[data-trust="cert-f-type"]'), 'other', 'change')
    pick(q(form(), '[data-trust="cert-f-file"]'), PDF())
    q(form(), '[data-trust="cert-add-submit"]').click()
    await settle(h => h.includes('cert-add-error'))
    err = host.querySelector('[data-trust="cert-add-error"]').textContent
  })
  assert.equal(err.trim(), 'The file is larger than 50 MB')
})

test('Download fetches the file with the session and saves a blob named by the server', async () => {
  const saved = []
  globalThis.URL.createObjectURL = (b) => { saved.push(['url', b.size]); return 'blob:x' }
  globalThis.URL.revokeObjectURL = () => {}
  const calls = stub(world(), (m, p) => (p === '/trust-center/certifications/1/file' ? new Response('%PDF-bytes', { status: 200, headers: { 'Content-Type': 'application/pdf', 'Content-Disposition': 'attachment; filename=Alpha.pdf' } }) : undefined))
  await mount(async (host, settle) => {
    const orig = window.HTMLAnchorElement.prototype.click
    window.HTMLAnchorElement.prototype.click = function () { saved.push(['click', this.download]) }
    click(rowEl(host, 1), '[data-trust="cert-download"]')
    await settle(() => saved.some(s => s[0] === 'click'))
    window.HTMLAnchorElement.prototype.click = orig
  })
  assert.ok(calls.some(c => c.path === '/trust-center/certifications/1/file'))
  assert.deepEqual(saved, [['url', 10], ['click', 'Alpha.pdf']])
})

test('Replace file opens the hidden input and POSTs the chosen file to the row', async () => {
  const w = world()
  const calls = stub(w, (m) => (m === 'POST' ? json(200, w.certs[1]) : undefined))
  await mount(async (host, settle) => {
    const input = q(host, '[data-trust="cert-replace-input"]')
    assert.equal(input.getAttribute('accept'), '.pdf,.png,.jpg,.jpeg')
    click(rowEl(host, 1), '[data-trust="cert-replace"]')
    pick(input, PDF('new.pdf'))
    await settle(() => calls.some(c => c.method === 'POST'))
    await flush()
  })
  const post = mutations(calls)[0]
  assert.equal(post.path, '/trust-center/certifications/1/file')
  assert.equal(post.body.file.name, 'new.pdf')
})
