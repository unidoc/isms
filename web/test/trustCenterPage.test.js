// The public trust center page (#519). Mounts the real component with a stubbed
// fetch and reads what lands on the page.
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

const json = (status, body) => new Response(JSON.stringify(body), {
  status, headers: { 'Content-Type': 'application/json' },
})
const NF = { message: 'item not found', code: 'not_found' }

const PUB = '11111111-1111-4111-8111-111111111111'
const GATED = '22222222-2222-4222-8222-222222222222'
const RESTR = '33333333-3333-4333-8333-333333333333'
const hash = c => c.repeat(64)

const overview = (extra = {}) => ({
  title: '', security_contact: 'security@acme.example', branding_name: 'Acme Inc', branding_color: '#0e7490',
  default_locale: 'en', show_powered_by: true, overview: null, ...extra,
})
const item = (id, level, title, extra = {}) => ({
  id, kind: 'document', section: '', sort_order: 0, access_level: level, title,
  version: '1.0', approved_at: 1760000000, content_hash: hash('a'), ...extra,
})
const doc = (id, html) => ({ id, title: 'Policy One', version: '1.0', approved_at: 1760000000, content_hash: hash('a'), html })

// Stub server. `routes` overrides per path; every call is recorded with headers.
function stub({ ov = overview(), items = [item(PUB, 'public', 'Policy One'), item(GATED, 'gated', 'Policy Two'), item(RESTR, 'restricted', 'Policy Three')], docs = {}, ovStatus = 200 } = {}) {
  const calls = []
  globalThis.fetch = async (url, opts = {}) => {
    const u = String(url)
    calls.push({ url: u, headers: opts.headers || {} })
    if (u.endsWith('/api/v1/config')) return json(200, { locales: [{ tag: 'en', name: 'English' }], organization_slug: 'acme' })
    if (u.endsWith('/api/v1/trust/acme')) return ovStatus === 200 ? json(200, ov) : json(ovStatus, NF)
    if (u.endsWith('/api/v1/trust/acme/items')) return json(200, items)
    const m = u.match(/\/api\/v1\/trust\/acme\/documents\/(.+)$/)
    if (m) return docs[m[1]] ? json(200, docs[m[1]]) : json(404, NF)
    return json(404, {})
  }
  return calls
}

const props = (extra = {}) => ({ slug: 'acme', basePath: '/acme/trust', ...extra })
const ready = h => h.includes('data-trust="page"') || h.includes('data-trust="unavailable"') || h.includes('data-trust="error"')
const mount = (p, interact, isReady = ready) => renderSfcSettled(Page, p, isReady, interact)
const norm = h => h.replaceAll(/<!--[\s\S]*?-->/g, '').replaceAll(/\s+/g, ' ')

test('a public row links to its document page; a gated row shows the contact and is never fetched', async () => {
  const calls = stub()
  const html = norm(await mount(props()))
  assert.ok(html.includes(`href="/acme/trust/documents/${PUB}"`), html)
  assert.ok(html.includes('href="mailto:security@acme.example"'), html)
  assert.ok(html.includes('Contact security@acme.example'), html)
  assert.ok(html.includes('Sign in + NDA') && html.includes('On request'), html)
  assert.ok(!calls.some(c => c.url.includes(`/documents/${GATED}`) || c.url.includes(`/documents/${RESTR}`)), JSON.stringify(calls.map(c => c.url)))
  assert.ok(!html.includes(`documents/${GATED}`), 'locked row must not link to its document')
  assert.ok(html.includes('1.0') && html.includes(hash('a').slice(0, 10)), html)
})

test('a locked row with no security contact says it is available on request, with no link', async () => {
  stub({ ov: overview({ security_contact: '' }), items: [item(GATED, 'gated', 'Policy Two')] })
  const html = norm(await mount(props()))
  assert.ok(html.includes('Available on request'), html)
  assert.ok(!html.includes('mailto:'), html)
  assert.ok(!html.includes(`documents/${GATED}`), html)
})

test('overview 404 shows the one not-available state', async () => {
  stub({ ovStatus: 404 })
  const html = norm(await mount(props()))
  assert.ok(html.includes('data-trust="unavailable"') && html.includes('This trust center is not available.'), html)
  assert.ok(!html.includes('Powered by'), html)
})

test('a server error shows a retry, not the not-available state', async () => {
  globalThis.fetch = async () => json(500, { message: 'internal server error' })
  const html = norm(await mount(props()))
  assert.ok(html.includes('data-trust="error"') && html.includes('Retry'), html)
  assert.ok(!html.includes('not available'), html)
})

test('no overview and no items: only the empty message, no empty sections', async () => {
  stub({ items: [] })
  const html = norm(await mount(props()))
  assert.ok(html.includes('Nothing has been published yet.'), html)
  assert.ok(!html.includes('data-trust="documents"') && !html.includes('data-trust="overview"'), html)
})

test('non-document kinds are ignored', async () => {
  stub({ items: [{ id: PUB, kind: 'faq', section: '', sort_order: 0, access_level: 'public' }] })
  const html = norm(await mount(props()))
  assert.ok(html.includes('Nothing has been published yet.') && !html.includes('data-trust="documents"'), html)
})

test('Back to app only when a session token is stored', async () => {
  stub()
  store.delete('isms_api_token')
  assert.ok(!norm(await mount(props())).includes('Back to app'))
  store.set('isms_api_token', 'tok-123')
  assert.ok(norm(await mount(props())).includes('Back to app'))
  store.delete('isms_api_token')
})

test('every request is anonymous even with a token stored', async () => {
  const calls = stub({ docs: { [PUB]: doc(PUB, '<p>hi</p>') } })
  store.set('isms_api_token', 'tok-123')
  await mount(props({ docId: PUB }), null, h => h.includes('hi') || h.includes('data-trust="unavailable"'))
  store.delete('isms_api_token')
  assert.ok(calls.length >= 3, JSON.stringify(calls))
  for (const c of calls) {
    const keys = Object.keys(c.headers).map(k => k.toLowerCase())
    assert.ok(!keys.includes('authorization'), `${c.url} carried ${keys}`)
  }
})

test('show_powered_by false hides the footer credit; true shows it', async () => {
  stub({ ov: overview({ show_powered_by: false }) })
  assert.ok(!norm(await mount(props())).includes('Powered by'))
  stub()
  assert.ok(norm(await mount(props())).includes('Powered by'))
})

test('the overview html and security contact are shown when set', async () => {
  stub({ ov: overview({ overview: { version: '0.1', approved_at: 1, content_hash: 'h', html: '<h2>About us</h2><p>We care.</p>' } }) })
  const html = norm(await mount(props()))
  assert.ok(html.includes('data-trust="overview"') && html.includes('We care.'), html)
})

test('document page: a link to a locked document routes to the locked state without a fetch', async () => {
  const calls = stub({ docs: { [PUB]: doc(PUB, `<p>See <a data-trust-document="${GATED}">the other policy</a>.</p>`) } })
  const navs = []
  const html = norm(await mount(props({ docId: PUB, onNavigate: p => navs.push(p) }),
    async (host, settle) => {
      await settle(h => h.includes('the other policy'))
      host.querySelector('a[data-trust-document]').click()
    }, h => h.includes('the other policy')))
  assert.deepEqual(navs, [`/acme/trust/documents/${GATED}`], html)
  assert.ok(!calls.some(c => c.url.includes(`/documents/${GATED}`)))
})

test('document page for a locked id shows title, access and contact, never fetching', async () => {
  const calls = stub()
  const html = norm(await mount(props({ docId: GATED }), null, h => h.includes('data-trust="locked"') || h.includes('not available')))
  assert.ok(html.includes('data-trust="locked"') && html.includes('Policy Two') && html.includes('mailto:security@acme.example'), html)
  assert.ok(!calls.some(c => c.url.includes(`/documents/${GATED}`)))
})

test('document page for an unknown id, or one the server 404s, is not available', async () => {
  stub()
  let html = norm(await mount(props({ docId: '99999999-9999-4999-8999-999999999999' }), null, h => h.includes('This document is not available.')))
  assert.ok(html.includes('This document is not available.'), html)
  stub({ docs: {} })
  html = norm(await mount(props({ docId: PUB }), null, h => h.includes('This document is not available.')))
  assert.ok(html.includes('This document is not available.'), html)
})

test('document html is sanitized: scripts and handlers go, data-trust-document stays', async () => {
  stub({ docs: { [PUB]: doc(PUB, `<p>ok</p><script>window.__x=1</script><img src="x" onerror="window.__x=2"><a data-trust-document="${GATED}">link</a>`) } })
  const html = norm(await mount(props({ docId: PUB }), null, h => h.includes('>ok<')))
  assert.ok(html.includes('>ok<'), html)
  assert.ok(!html.includes('<script') && !html.includes('onerror'), html)
  assert.ok(html.includes(`data-trust-document="${GATED}"`), html)
})

// A real href must stay usable: only a plain primary click is handled in-app.
async function clickView(init) {
  stub()
  const navs = []
  let defaultPrevented = null
  await mount(props({ onNavigate: p => navs.push(p) }), async (host) => {
    const ev = new window.MouseEvent('click', { bubbles: true, cancelable: true, button: 0, ...init })
    host.querySelector('[data-trust=view]').dispatchEvent(ev)
    defaultPrevented = ev.defaultPrevented
  })
  return { navs, defaultPrevented }
}

test('a plain click on View navigates in-app and cancels the browser navigation', async () => {
  const r = await clickView({})
  assert.deepEqual(r.navs, [`/acme/trust/documents/${PUB}`])
  assert.equal(r.defaultPrevented, true)
})

test('modified or non-primary clicks on View are left to the browser', async () => {
  for (const init of [{ metaKey: true }, { ctrlKey: true }, { shiftKey: true }, { altKey: true }, { button: 1 }]) {
    const r = await clickView(init)
    assert.deepEqual(r.navs, [], JSON.stringify(init))
    assert.equal(r.defaultPrevented, false, JSON.stringify(init))
  }
})
