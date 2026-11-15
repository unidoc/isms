// Visitor sign-in on the public trust page (#524): the header, the gated rows,
// the document page for a signed-in visitor, and the preview staying anonymous.
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
const NF = { message: 'item not found', code: 'not_found' }
const PUB = '11111111-1111-4111-8111-111111111111'
const GATED = '22222222-2222-4222-8222-222222222222'
const RESTR = '33333333-3333-4333-8333-333333333333'
const hash = c => c.repeat(64)

const overview = (extra = {}) => ({
  title: '', security_contact: 'security@acme.example', branding_name: 'Acme Inc', branding_color: '#0e7490',
  default_locale: 'en', show_powered_by: true, overview: null, visitor_sign_in: true, privacy_notice_url: '', ...extra,
})
const item = (id, level, title) => ({
  id, kind: 'document', section: '', sort_order: 0, access_level: level, title,
  version: '1.0', approved_at: 1760000000, content_hash: hash('a'),
})
const ITEMS = [item(PUB, 'public', 'Policy One'), item(GATED, 'gated', 'Policy Two'), item(RESTR, 'restricted', 'Policy Three')]
const VISITOR = { email: 'ada@auditco.test', name: 'Ada', company: 'Audit Co' }

// A stub server with a visitor session switch. Every call is recorded.
function stub({ ov = overview(), signedIn = false, link = () => json(202, { status: 'sent' }) } = {}) {
  const st = { calls: [], signedIn }
  globalThis.fetch = async (url, opts = {}) => {
    const u = String(url)
    st.calls.push({ url: u, method: opts.method || 'GET', body: opts.body, headers: opts.headers || {} })
    if (u.endsWith('/api/v1/config')) return json(200, { locales: [{ tag: 'en', name: 'English' }] })
    if (u.endsWith('/api/v1/trust/acme')) return json(200, ov)
    if (u.endsWith('/api/v1/trust/acme/items')) return json(200, ITEMS)
    if (u.endsWith('/api/v1/trust/acme/visitor')) return json(200, { visitor: st.signedIn ? VISITOR : null })
    if (u.endsWith('/api/v1/trust/acme/visitor/link')) return link(opts)
    if (u.endsWith('/api/v1/trust/acme/visitor/sign-out')) { st.signedIn = false; return json(200, { visitor: null }) }
    const m = u.match(/\/api\/v1\/trust\/acme\/documents\/(.+)$/)
    if (m) {
      if (m[1] === PUB || (m[1] === GATED && st.signedIn)) return json(200, { id: m[1], title: 'Doc', version: '1.0', approved_at: 1760000000, content_hash: hash('a'), html: '<p>BODY ' + m[1].slice(0, 2) + '</p>' })
      return json(404, NF)
    }
    return json(404, {})
  }
  return st
}

const props = (extra = {}) => ({ slug: 'acme', basePath: '/acme/trust', ...extra })
const ready = h => h.includes('data-trust="page"') || h.includes('data-trust="unavailable"') || h.includes('data-trust="error"')
const mount = (p, interact, isReady = ready) => renderSfcSettled(Page, p, isReady, interact)
const norm = h => h.replaceAll(/<!--[\s\S]*?-->/g, '').replaceAll(/\s+/g, ' ')
const type = (el, value) => { el.value = value; el.dispatchEvent(new window.Event('input', { bubbles: true })) }

test('anonymous, sign-in on: header link, gated shows "Sign in to view", restricted shows the contact', async () => {
  const st = stub()
  const html = norm(await mount(props()))
  assert.ok(html.includes('data-trust="sign-in-link"') && html.includes('Already have access? Get a sign-in link'), html)
  assert.ok(html.includes('data-trust="sign-in-to-view"') && html.includes('Sign in to view'), html)
  assert.ok(html.includes('Contact security@acme.example'), html)
  assert.ok(!html.includes('data-trust="visitor"') && !html.includes('data-trust="sign-in-form"'), html)
  assert.ok(!html.includes(`documents/${GATED}`), 'a locked gated row must not link to its document')
  assert.ok(st.calls.some(c => c.url.endsWith('/visitor')), 'asks who is signed in')
  assert.ok(!st.calls.some(c => c.url.includes(`/documents/${GATED}`)))
})

test('sign-in off: no sign-in UI at all, gated keeps the contact fallback, no visitor call', async () => {
  const st = stub({ ov: overview({ visitor_sign_in: false }) })
  const html = norm(await mount(props()))
  assert.ok(!html.includes('data-trust="sign-in-link"') && !html.includes('sign-in-to-view') && !html.includes('Sign in to view'), html)
  assert.ok(html.includes('Contact security@acme.example'), html)
  assert.ok(!st.calls.some(c => c.url.endsWith('/visitor')), 'no visitor call when sign-in is off')
})

test('gated and restricted labels: "Sign in" and "On request" (no NDA)', async () => {
  stub()
  const html = norm(await mount(props()))
  assert.ok(html.includes('>Sign in<') && html.includes('On request') && !html.includes('NDA'), html)
})

test('signed in: "Signed in as", gated row is View, no sign-in link', async () => {
  stub({ signedIn: true })
  const html = norm(await mount(props()))
  assert.ok(html.includes('Signed in as ada@auditco.test') && html.includes('data-trust="sign-out"'), html)
  assert.ok(html.includes(`href="/acme/trust/documents/${GATED}"`), html)
  assert.ok(!html.includes('sign-in-link') && !html.includes('Sign in to view'), html)
  assert.ok(!html.includes(`documents/${RESTR}`), 'restricted stays locked even when signed in')
})

test('signed in: the gated document page fetches and renders; restricted stays locked', async () => {
  const st = stub({ signedIn: true })
  const html = norm(await mount(props({ docId: GATED }), null, h => h.includes('BODY')))
  assert.ok(html.includes('BODY 22'), html)
  assert.ok(st.calls.some(c => c.url.includes(`/documents/${GATED}`)))
  const st2 = stub({ signedIn: true })
  const locked = norm(await mount(props({ docId: RESTR }), null, h => h.includes('data-trust="locked"')))
  assert.ok(locked.includes('data-trust="locked"'), locked)
  assert.ok(!st2.calls.some(c => c.url.includes(`/documents/${RESTR}`)))
})

test('anonymous on a gated document page: locked, with a sign-in button, never fetched', async () => {
  const st = stub()
  const html = norm(await mount(props({ docId: GATED }), null, h => h.includes('data-trust="locked"')))
  assert.ok(html.includes('data-trust="doc-sign-in"') && html.includes('Sign in to read this document.'), html)
  assert.ok(!st.calls.some(c => c.url.includes(`/documents/${GATED}`)))
  // with sign-in off the locked state keeps the contact
  stub({ ov: overview({ visitor_sign_in: false }) })
  const off = norm(await mount(props({ docId: GATED }), null, h => h.includes('data-trust="locked"')))
  assert.ok(!off.includes('doc-sign-in') && off.includes('mailto:security@acme.example'), off)
})

test('a gated document that 404s while a visitor is shown re-asks, and a ended session shows the locked state with the sign-in prompt', async () => {
  const st = stub({ signedIn: true })
  const html = norm(await mount(props({ docId: GATED }), async (host, settle) => {
    await settle(h => h.includes('BODY'))
  }, h => h.includes('BODY')))
  assert.ok(html.includes('BODY 22'), 'control: readable while signed in')

  // The session ended on the server after the page loaded its visitor: the doc
  // fetch 404s, the visitor is re-read (now null), and the page shows "locked".
  const calls = stub({ signedIn: true })
  const real = globalThis.fetch
  let visitorReads = 0
  globalThis.fetch = async (url, opts) => {
    if (String(url).endsWith('/visitor') && ++visitorReads === 2) calls.signedIn = false
    if (String(url).includes(`/documents/${GATED}`)) calls.signedIn = false // revoked just before the read
    return real(url, opts)
  }
  const out = norm(await mount(props({ docId: GATED }), null, h => h.includes('data-trust="locked"') || h.includes('data-trust="doc-unavailable"')))
  assert.ok(out.includes('data-trust="locked"') && out.includes('data-trust="doc-sign-in"'), out)
  assert.ok(!out.includes('data-trust="doc-unavailable"'), 'not "missing"')
  assert.ok(visitorReads >= 2, 'the visitor was asked again')
  void st
})

test('sign out calls the endpoint and the page returns to anonymous', async () => {
  const st = stub({ signedIn: true })
  const html = norm(await mount(props(), async (host, settle) => {
    host.querySelector('[data-trust=sign-out]').click()
    await settle(h => h.includes('data-trust="sign-in-link"'))
  }))
  assert.ok(st.calls.some(c => c.url.endsWith('/visitor/sign-out') && c.method === 'POST'), JSON.stringify(st.calls.map(c => c.url)))
  assert.ok(html.includes('data-trust="sign-in-link"') && !html.includes('Signed in as') && html.includes('Sign in to view'), html)
})

test('sign out on a gated document page locks it again', async () => {
  stub({ signedIn: true })
  const html = norm(await mount(props({ docId: GATED }), async (host, settle) => {
    await settle(h => h.includes('BODY'))
    host.querySelector('[data-trust=sign-out]').click()
    await settle(h => h.includes('data-trust="locked"'))
  }, h => h.includes('BODY')))
  assert.ok(html.includes('data-trust="locked"') && !html.includes('BODY 22'), html)
})

async function sendLink(email, link, ov = overview()) {
  const st = stub({ link, ov })
  const html = norm(await mount(props(), async (host, settle) => {
    host.querySelector('[data-trust=sign-in-link]').click()
    await settle(h => h.includes('data-trust="sign-in-form"'))
    type(host.querySelector('[data-trust=sign-in-email]'), email)
    host.querySelector('[data-trust=sign-in-submit]').click()
    await settle(h => h.includes('data-trust="sign-in-sent"') || h.includes('data-trust="sign-in-error"'))
  }))
  return { st, html }
}

test('sign-in form: 202 shows the uniform text and posts the trimmed email as JSON', async () => {
  const { st, html } = await sendLink('  ada@auditco.test ')
  assert.ok(html.includes("If this address can sign in, we've sent a link. It expires in 15 minutes."), html)
  const c = st.calls.find(x => x.url.endsWith('/visitor/link'))
  assert.equal(c.method, 'POST')
  assert.deepEqual(JSON.parse(c.body), { email: 'ada@auditco.test' })
  assert.equal(c.headers['Content-Type'], 'application/json')
})

test('sign-in form: the sent text is the same for every address', async () => {
  const a = (await sendLink('known@auditco.test')).html.match(/data-trust="sign-in-sent"[^>]*>([^<]*)</)[1]
  const b = (await sendLink('nobody@elsewhere.test')).html.match(/data-trust="sign-in-sent"[^>]*>([^<]*)</)[1]
  assert.equal(a, b)
})

test('sign-in form: blocked domain, rate limit, unavailable and bad address messages', async () => {
  let r = await sendLink('a@gmail.com', () => json(400, { message: 'x', code: 'trust_email_domain_blocked', params: { value: 'gmail.com' } }))
  assert.ok(r.html.includes('gmail.com') && r.html.includes('work email') && r.html.includes('data-trust="sign-in-error"'), r.html)
  r = await sendLink('a@b.test', () => json(429, { message: 'too many requests' }))
  assert.ok(r.html.includes('Too many requests. Please try again later.'), r.html)
  r = await sendLink('a@b.test', () => json(503, { message: 'x', code: 'trust_sign_in_unavailable' }))
  assert.ok(r.html.includes('Sign-in is not available.'), r.html)
  r = await sendLink('nope', () => json(400, { message: 'x', code: 'invalid_request' }))
  assert.ok(r.html.includes('Enter a valid email address.'), r.html)
  r = await sendLink('a@b.test', () => json(500, { message: 'x' }))
  assert.ok(r.html.includes('could not be sent'), r.html)
})

test('the privacy notice link is rendered only for an http(s) URL, opening safely', async () => {
  const open = async (url) => {
    stub({ ov: overview({ privacy_notice_url: url }) })
    return norm(await mount(props(), async (host, settle) => {
      host.querySelector('[data-trust=sign-in-link]').click()
      await settle(h => h.includes('data-trust="sign-in-form"'))
    }))
  }
  const ok = await open('https://acme.example/privacy')
  assert.ok(/<a [^>]*href="https:\/\/acme.example\/privacy"[^>]*target="_blank"[^>]*rel="noopener noreferrer"|<a [^>]*rel="noopener noreferrer"[^>]*href="https:\/\/acme.example\/privacy"/.test(ok) || (ok.includes('data-trust="privacy"') && ok.includes('rel="noopener noreferrer"') && ok.includes('target="_blank"')), ok)
  for (const bad of ['javascript:alert(1)', 'ftp://x.test', '/relative', '']) {
    const h = await open(bad)
    assert.ok(!h.includes('data-trust="privacy"'), `${bad}: ${h}`)
  }
})

test('the form buttons that are not the submit are type=button', async () => {
  stub()
  const html = await mount(props(), async (host, settle) => {
    host.querySelector('[data-trust=sign-in-link]').click()
    await settle(h => h.includes('data-trust="sign-in-form"'))
  })
  const doc = new window.DOMParser().parseFromString(html, 'text/html')
  for (const b of doc.querySelectorAll('button')) {
    const t = b.getAttribute('type')
    if (b.closest('form')) assert.ok(t === 'button' || b.getAttribute('data-trust') === 'sign-in-submit', b.outerHTML)
    assert.ok(t, `a button with no type: ${b.outerHTML}`)
  }
})

const previewSource = (ov = overview()) => ({
  overview: async () => ov,
  items: async () => ITEMS,
  document: async (id) => ({ id, title: 'D', version: '1', approved_at: 1, content_hash: hash('a'), html: '<p>x</p>' }),
})

test('the preview shows what visitors see when sign-in is on, but inert, and makes no visitor call', async () => {
  const st = stub({ signedIn: true })
  const html = norm(await mount(props({ source: previewSource() })))
  // the same words visitors see
  assert.ok(html.includes('Already have access? Get a sign-in link') && html.includes('Sign in to view'), html)
  // ...as disabled buttons
  const doc = new window.DOMParser().parseFromString(html, 'text/html')
  for (const sel of ['[data-trust=sign-in-link]', '[data-trust=sign-in-to-view]']) {
    const b = doc.querySelector(sel)
    assert.ok(b && b.hasAttribute('disabled') && b.getAttribute('type') === 'button', `${sel}: ${html}`)
  }
  assert.ok(!html.includes('data-trust="visitor"') && !html.includes('Signed in as'), html)
  assert.ok(!html.includes(`documents/${GATED}`), html)
  assert.ok(!st.calls.some(c => c.url.endsWith('/visitor') || c.url.includes('/visitor/')), 'the preview must make no visitor call')
})

test('clicking an inert preview affordance opens nothing and sends nothing', async () => {
  const st = stub()
  const html = norm(await mount(props({ source: previewSource() }), async (host) => {
    host.querySelector('[data-trust=sign-in-link]').click()
    host.querySelector('[data-trust=sign-in-to-view]').click()
    await new Promise(r => setTimeout(r, 40))
  }))
  assert.ok(!html.includes('data-trust="sign-in-form"'), html)
  assert.ok(!st.calls.some(c => c.url.includes('/visitor')))
})

test('the preview with sign-in off shows no sign-in affordance and keeps the contact fallback', async () => {
  const st = stub()
  const html = norm(await mount(props({ source: previewSource(overview({ visitor_sign_in: false })) })))
  assert.ok(!html.includes('sign-in-link') && !html.includes('sign-in-to-view') && !html.includes('Sign in to view'), html)
  assert.ok(html.includes('Contact security@acme.example'), html)
  assert.ok(!st.calls.some(c => c.url.includes('/visitor')))
})

test('the preview of a locked gated document: an inert sign-in hint when sign-in is on, the contact when off', async () => {
  stub()
  const on = norm(await mount(props({ source: previewSource(), docId: GATED }), null, h => h.includes('data-trust="locked"')))
  assert.ok(on.includes('data-trust="locked"') && on.includes('data-trust="doc-sign-in"') && /data-trust="doc-sign-in"[^>]*disabled|disabled[^>]*data-trust="doc-sign-in"/.test(on), on)
  const off = norm(await mount(props({ source: previewSource(overview({ visitor_sign_in: false })), docId: GATED }), null, h => h.includes('data-trust="locked"')))
  assert.ok(!off.includes('doc-sign-in') && off.includes('mailto:security@acme.example'), off)
})
