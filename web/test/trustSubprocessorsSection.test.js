// The Subprocessors section of the trust center manager screen (#522). Mounts
// the real component with a stubbed fetch and reads what lands on the page.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const Section = await compileSfc(new URL('../src/components/trust/TrustSubprocessorsSection.vue', import.meta.url).pathname, { stubChildren: true })

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}

const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const I1 = '11111111-1111-4111-8111-111111111111'
const I2 = '22222222-2222-4222-8222-222222222222'
const I3 = '33333333-3333-4333-8333-333333333333'

const sup = (id, name, over = {}, item) => ({
  supplier_id: id, identifier: 'SUPPLIER-' + id, name, public_name: '', public_purpose: '', processing_location: '', data_categories: '',
  status: 'active', data_access: true, deleted: false, eligible: true, ineligible_reason: '', item: item || null, ...over,
})
const it = (id, order, hidden = '') => ({ id, hidden_reason: hidden, sort_order: order, published_at: 1 })

function world() {
  return {
    changed_at: 1760000000,
    suppliers: [
      sup(4, 'Zed Candidate'),
      sup(2, 'Beta', { public_name: 'Beta GmbH', public_purpose: 'Hosting', processing_location: 'EU', data_categories: 'Email' }, it(I2, 2)),
      sup(1, 'Alpha', {}, it(I1, 1)), // all three details empty
      sup(3, 'Gamma', { status: 'terminated', eligible: false, ineligible_reason: 'terminated', public_purpose: 'Payments' }, it(I3, 3, 'terminated')),
      sup(5, 'Aaa Back', { public_purpose: 'x', eligible: true }, it('44444444-4444-4444-8444-444444444444', 4, 'no_data_access')),
      sup(6, 'Noaccess', { data_access: false, eligible: false, ineligible_reason: 'no_data_access' }), // not a candidate
    ],
  }
}

function stub(w, handler = () => undefined) {
  const calls = []
  globalThis.fetch = async (url, opts = {}) => {
    const method = opts.method || 'GET'
    const path = String(url).replace(/^.*\/api\/v1/, '')
    const body = opts.body ? JSON.parse(opts.body) : undefined
    calls.push({ method, path, body })
    const r = handler(method, path, body, w)
    if (r) return r
    if (method === 'GET' && path === '/trust-center/subprocessors') return json(200, w)
    return json(404, {})
  }
  return calls
}

const ready = h => h.includes('data-trust-sp-row') || h.includes('sp-empty') || h.includes('sp-load-error')
const mount = (interact, isReady = ready) => renderSfcSettled(Section, {}, isReady, interact)
const q = (host, sel) => { const el = host.querySelector(sel); assert.ok(el, `${sel} not found: ${host.innerHTML}`); return el }
const click = (host, sel) => q(host, sel).click()
const rowEl = (host, id) => host.querySelector(`[data-trust-sp-row="${id}"]`)
const names = host => [...host.querySelectorAll('[data-trust="sp-name"]')].map(e => e.textContent.trim())
const flush = () => new Promise(r => setTimeout(r, 40))
const mutations = calls => calls.filter(c => c.method !== 'GET')

test('rows: published suppliers in sort order with the public name, summary, fixed Public label and register link', async () => {
  stub(world())
  const seen = {}
  await mount(async host => {
    seen.names = names(host)
    seen.changed = host.querySelector('[data-trust="sp-changed"]')?.textContent.trim()
    const beta = rowEl(host, 2)
    seen.betaSummary = [...q(beta, '[data-trust="sp-summary"]').children].map(e => e.textContent.trim()).join(' ')
    seen.betaLevel = q(beta, '[data-trust="sp-level"]').textContent.trim()
    seen.betaSelect = !!beta.querySelector('select')
    seen.betaHidden = !!beta.querySelector('[data-trust="sp-hidden"]')
    seen.betaMissing = !!beta.querySelector('[data-trust="sp-details-missing"]')
    seen.betaLink = beta.querySelector('[data-trust="sp-link"]').textContent.trim()
    seen.alphaMissing = q(rowEl(host, 1), '[data-trust="sp-details-missing"]').textContent.trim()
    seen.noLevelPicker = !host.querySelector('[data-trust-sp-row] select')
  })
  assert.deepEqual(seen.names, ['Alpha', 'Beta GmbH', 'Gamma', 'Aaa Back'])
  assert.ok(seen.changed.startsWith('Last changed '), seen.changed)
  assert.equal(seen.betaSummary, 'Hosting EU Email')
  assert.equal(seen.betaLevel, 'Public')
  assert.equal(seen.betaSelect, false, 'no level picker')
  assert.equal(seen.betaHidden, false)
  assert.equal(seen.betaMissing, false)
  assert.equal(seen.betaLink, 'SUPPLIER-2')
  assert.ok(seen.alphaMissing.startsWith('No public details yet'), seen.alphaMissing)
  assert.ok(seen.noLevelPicker)
})

test('hidden rows show the localized reason; Publish again only when eligible again', async () => {
  stub(world())
  const seen = {}
  await mount(async host => {
    const gamma = rowEl(host, 3)
    seen.gammaBadge = q(gamma, '[data-trust="sp-hidden"]').textContent.trim()
    seen.gammaRepublish = !!gamma.querySelector('[data-trust="sp-republish"]')
    seen.gammaWhy = q(gamma, '[data-trust="sp-still-ineligible"]').textContent.trim()
    seen.gammaUnpublish = !!gamma.querySelector('[data-trust="sp-unpublish"]')
    const back = rowEl(host, 5)
    seen.backBadge = q(back, '[data-trust="sp-hidden"]').textContent.trim()
    seen.backRepublish = q(back, '[data-trust="sp-republish"]').textContent.trim()
    seen.backWhy = !!back.querySelector('[data-trust="sp-still-ineligible"]')
    seen.raw = host.textContent.includes('no_data_access')
  })
  assert.equal(seen.gammaBadge, 'Hidden: Supplier terminated')
  assert.equal(seen.gammaRepublish, false)
  assert.ok(seen.gammaWhy.includes('Supplier terminated'), seen.gammaWhy)
  assert.ok(seen.gammaUnpublish)
  assert.equal(seen.backBadge, 'Hidden: No data access')
  assert.equal(seen.backRepublish, 'Publish again')
  assert.equal(seen.backWhy, false)
  assert.equal(seen.raw, false, 'raw tokens never show')
})

test('the picker lists only eligible unpublished suppliers; none left shows the hint', async () => {
  stub(world())
  let opts
  await mount(async host => {
    opts = [...host.querySelectorAll('[data-trust="sp-picker"] option')].map(o => o.textContent.trim())
  })
  assert.deepEqual(opts.slice(1), ['Zed Candidate (SUPPLIER-4)'])
  const w = world()
  w.suppliers = w.suppliers.filter(s => s.supplier_id !== 4)
  stub(w)
  let hint
  await mount(async host => { hint = host.querySelector('[data-trust="sp-no-candidates"]')?.textContent.trim() })
  assert.equal(hint, 'Only suppliers with data access that are not terminated can be listed.')
})

test('Publish and Publish again POST the supplier id; Unpublish asks first, then DELETEs; each reloads', async () => {
  const w = world()
  const calls = stub(w, (m) => (m === 'POST' ? json(201, w.suppliers[0]) : m === 'DELETE' ? new Response(null, { status: 204 }) : undefined))
  await mount(async (host, settle) => {
    const sel = q(host, '[data-trust="sp-picker"]')
    sel.value = '4'
    sel.dispatchEvent(new window.Event('change'))
    await flush()
    click(host, '[data-trust="sp-publish"]')
    await settle(() => calls.some(c => c.method === 'POST'))
    await flush()
    click(rowEl(host, 5), '[data-trust="sp-republish"]')
    await settle(() => calls.filter(c => c.method === 'POST').length === 2)
    await flush()
    click(rowEl(host, 1), '[data-trust="sp-unpublish"]')
    await settle(h => h.includes('confirm-dialog'))
    assert.equal(mutations(calls).filter(c => c.method === 'DELETE').length, 0, 'nothing sent before the confirmation')
    click(host, '[data-trust="confirm-yes"]')
    await settle(() => calls.some(c => c.method === 'DELETE'))
    await flush()
  })
  assert.deepEqual(mutations(calls).map(c => [c.method, c.path, c.body]), [
    ['POST', '/trust-center/subprocessors', { supplier_id: 4 }],
    ['POST', '/trust-center/subprocessors', { supplier_id: 5 }],
    ['DELETE', '/trust-center/subprocessors/1', undefined],
  ])
  assert.ok(calls.filter(c => c.method === 'GET').length >= 4, 'reloaded after each action')
})

test('reorder sends every subprocessor item id, hidden ones included, swapped', async () => {
  const calls = stub(world(), (m) => (m === 'PUT' ? json(200, []) : undefined))
  let ends
  await mount(async (host, settle) => {
    ends = { firstUp: q(rowEl(host, 1), '[data-trust="sp-up"]').disabled, lastDown: q(rowEl(host, 5), '[data-trust="sp-down"]').disabled }
    click(rowEl(host, 2), '[data-trust="sp-up"]')
    await settle(() => calls.some(c => c.method === 'PUT'))
    await flush()
  })
  assert.deepEqual(ends, { firstUp: true, lastDown: true })
  assert.deepEqual(mutations(calls)[0], {
    method: 'PUT', path: '/trust-center/items/order',
    body: { kind: 'subprocessor', ids: [I2, I1, I3, '44444444-4444-4444-8444-444444444444'] },
  })
})

test('a stale order (409) reloads and shows the localized error', async () => {
  const calls = stub(world(), (m) => (m === 'PUT' ? json(409, { message: 'raw', code: 'trust_order_stale' }) : undefined))
  let text
  await mount(async (host, settle) => {
    click(rowEl(host, 2), '[data-trust="sp-up"]')
    await settle(h => h.includes('sp-action-error'))
    await flush()
    text = host.querySelector('[data-trust="sp-action-error"]').textContent
  })
  assert.ok(text.includes('the trust center changed while you were reordering'), text)
  assert.equal(calls.filter(c => c.method === 'GET').length, 2)
})

test('the ineligible 409 shows the localized reason, never the raw token', async () => {
  const w = world()
  stub(w, (m) => (m === 'POST' ? json(409, { message: 'raw english', code: 'trust_subprocessor_ineligible', params: { value: 'no_data_access' } }) : undefined))
  let text
  await mount(async (host, settle) => {
    const sel = q(host, '[data-trust="sp-picker"]')
    sel.value = '4'
    sel.dispatchEvent(new window.Event('change'))
    await flush()
    click(host, '[data-trust="sp-publish"]')
    await settle(h => h.includes('sp-action-error'))
    text = host.querySelector('[data-trust="sp-action-error"]').textContent
  })
  assert.equal(text.trim(), 'This supplier cannot be published as a subprocessor (No data access).')
})

test('a failed load says so with Retry', async () => {
  globalThis.fetch = async () => json(500, { message: 'boom' })
  const html = await mount(null)
  assert.ok(html.includes('sp-load-error') && html.includes('Retry'), html)
})
