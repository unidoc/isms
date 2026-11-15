// Where the emailed link lands (#524): the token comes from the fragment and is
// stripped at once, the visitor confirms "Sign in as <email>?", and only then is
// the link used.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const SignIn = await compileSfc(new URL('../src/components/TrustSignIn.vue', import.meta.url).pathname, { stubChildren: true })

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}

const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const TOKEN = 'ab'.repeat(32)
const norm = h => h.replaceAll(/<!--[\s\S]*?-->/g, '').replaceAll(/\s+/g, ' ')
const type = (el, value) => { el.value = value; el.dispatchEvent(new window.Event('input', { bubbles: true })) }

// The component reads location.hash and calls history.replaceState; node has
// neither, so the test supplies them and records the replaceState calls.
function setLocation(hash) {
  const replaced = []
  globalThis.location = { hash, pathname: '/acme/trust/sign-in', search: '' }
  globalThis.history = { state: null, replaceState: (...a) => { replaced.push(a); globalThis.location.hash = '' } }
  return replaced
}

function stub({ ovStatus = 200, inspect = () => json(200, { email: 'ada@auditco.test', needs_profile: false }), session = () => json(200, { visitor: { email: 'ada@auditco.test', name: 'A', company: 'C' } }) } = {}) {
  const calls = []
  globalThis.fetch = async (url, opts = {}) => {
    const u = String(url)
    calls.push({ url: u, method: opts.method || 'GET', body: opts.body, headers: opts.headers || {} })
    if (u.endsWith('/api/v1/config')) return json(200, { locales: [{ tag: 'en', name: 'English' }] })
    if (u.endsWith('/api/v1/trust/acme')) return ovStatus === 200 ? json(200, { branding_name: 'Acme Inc', branding_color: '#0e7490', default_locale: 'en' }) : json(ovStatus, { message: 'nf', code: 'not_found' })
    if (u.endsWith('/visitor/link/inspect')) return inspect(opts)
    if (u.endsWith('/visitor/session')) return session(opts)
    return json(404, {})
  }
  return calls
}

const props = (extra = {}) => ({ slug: 'acme', basePath: '/acme/trust', ...extra })
const settled = h => ['confirm', 'invalid', 'blocked', 'error', 'unavailable'].some(s => h.includes(`data-trust="${s}"`))
const mount = (p, interact, isReady = settled) => renderSfcSettled(SignIn, p, isReady, interact)

test('the fragment token is read and removed from the address bar before any request', async () => {
  const replaced = setLocation(`#token=${TOKEN}`)
  let firstFetchSawHash = null
  const calls = stub()
  const inner = globalThis.fetch
  globalThis.fetch = async (...a) => { if (firstFetchSawHash === null) firstFetchSawHash = globalThis.location.hash; return inner(...a) }
  await mount(props())
  assert.equal(replaced.length, 1)
  assert.equal(replaced[0][2], '/acme/trust/sign-in', 'the new URL has no fragment')
  assert.ok(!String(replaced[0][2]).includes('token'))
  assert.equal(firstFetchSawHash, '', 'the hash was already gone at the first request')
  const inspect = calls.find(c => c.url.endsWith('/visitor/link/inspect'))
  assert.deepEqual(JSON.parse(inspect.body), { token: TOKEN })
  assert.equal(inspect.method, 'POST')
  assert.equal(globalThis.location.hash, '')
})

test('inspect leads to the confirm screen with the email, and nothing is redeemed yet', async () => {
  setLocation(`#token=${TOKEN}`)
  const calls = stub()
  const html = norm(await mount(props()))
  assert.ok(html.includes('data-trust="confirm"') && html.includes('Sign in as ada@auditco.test?'), html)
  assert.ok(html.includes('Acme Inc'), html)
  assert.ok(!html.includes('data-trust="name"'), 'no profile inputs for a returning visitor')
  assert.ok(!calls.some(c => c.url.endsWith('/visitor/session')), 'no auto-redeem')
})

test('needs_profile shows required name and company inputs, capped at 200, with labels', async () => {
  setLocation(`#token=${TOKEN}`)
  stub({ inspect: () => json(200, { email: 'new@auditco.test', needs_profile: true }) })
  const html = norm(await mount(props()))
  assert.ok(/<input[^>]*data-trust="name"[^>]*/.test(html) && html.includes('maxlength="200"'), html)
  assert.ok(html.includes('data-trust="company"') && html.includes('>Name<') && html.includes('>Company<'), html)
  assert.ok(html.includes('for="trust-visitor-name"') && html.includes('id="trust-visitor-name"'), html)
})

test('confirming signs in and navigates to the trust page', async () => {
  setLocation(`#token=${TOKEN}`)
  const calls = stub()
  const navs = []
  await mount(props({ onNavigate: p => navs.push(p) }), async (host, settle) => {
    host.querySelector('[data-trust=confirm-submit]').click()
    await settle(() => navs.length > 0)
  })
  assert.deepEqual(navs, ['/acme/trust'])
  const s = calls.find(c => c.url.endsWith('/visitor/session'))
  assert.deepEqual(JSON.parse(s.body), { token: TOKEN })
  assert.equal(s.headers['Content-Type'], 'application/json')
})

test('a first sign-in sends the trimmed name and company, and refuses empty ones', async () => {
  setLocation(`#token=${TOKEN}`)
  const calls = stub({ inspect: () => json(200, { email: 'new@auditco.test', needs_profile: true }) })
  const navs = []
  let html = ''
  html = norm(await mount(props({ onNavigate: p => navs.push(p) }), async (host, settle) => {
    host.querySelector('[data-trust=confirm-submit]').click()
    await settle(h => h.includes('data-trust="form-error"'))
    assert.equal(calls.filter(c => c.url.endsWith('/visitor/session')).length, 0, 'empty profile is not sent')
    type(host.querySelector('[data-trust=name]'), '  Ada Auditor ')
    type(host.querySelector('[data-trust=company]'), ' Audit Co ')
    host.querySelector('[data-trust=confirm-submit]').click()
    await settle(() => navs.length > 0)
  }))
  assert.ok(html.length > 0)
  assert.deepEqual(JSON.parse(calls.find(c => c.url.endsWith('/visitor/session')).body), { token: TOKEN, name: 'Ada Auditor', company: 'Audit Co' })
  assert.deepEqual(navs, ['/acme/trust'])
})

test('a 409 on confirm shows the profile fields', async () => {
  setLocation(`#token=${TOKEN}`)
  stub({ session: () => json(409, { message: 'x', code: 'trust_visitor_profile_required' }) })
  const html = norm(await mount(props(), async (host, settle) => {
    host.querySelector('[data-trust=confirm-submit]').click()
    await settle(h => h.includes('data-trust="name"'))
  }))
  assert.ok(html.includes('data-trust="name"') && html.includes('enter your name and company to continue'), html)
})

test('an invalid or expired link says so, with a way back to the sign-in form', async () => {
  setLocation(`#token=${TOKEN}`)
  stub({ inspect: () => json(400, { message: 'x', code: 'trust_link_invalid' }) })
  const navs = []
  const html = norm(await mount(props({ onNavigate: p => navs.push(p) }), async (host) => {
    host.querySelector('[data-trust=request-new]').click()
  }))
  assert.ok(html.includes('data-trust="invalid"') && html.includes('This sign-in link is invalid or has expired.'), html)
  assert.deepEqual(navs, ['/acme/trust#sign-in'])
})

test('redeeming an already-used link shows the invalid state', async () => {
  setLocation(`#token=${TOKEN}`)
  stub({ session: () => json(400, { message: 'x', code: 'trust_link_invalid' }) })
  const html = norm(await mount(props(), async (host, settle) => {
    host.querySelector('[data-trust=confirm-submit]').click()
    await settle(h => h.includes('data-trust="invalid"'))
  }))
  assert.ok(html.includes('data-trust="invalid"'), html)
})

test('a blocked visitor sees the blocked message', async () => {
  setLocation(`#token=${TOKEN}`)
  stub({ inspect: () => json(403, { message: 'x', code: 'trust_visitor_blocked' }) })
  const html = norm(await mount(props()))
  assert.ok(html.includes('data-trust="blocked"') && html.includes('this email address cannot sign in to this trust center'), html)
})

test('no token, or a malformed one, is the invalid state and calls nothing', async () => {
  for (const hash of ['', '#', '#token=', '#token=zzz', '#other=1', `#token=${TOKEN.slice(1)}`]) {
    setLocation(hash)
    const calls = stub()
    const html = norm(await mount(props()))
    assert.ok(html.includes('data-trust="invalid"'), `${hash}: ${html}`)
    assert.ok(!calls.some(c => c.url.includes('/visitor/')), `${hash}: ${JSON.stringify(calls.map(c => c.url))}`)
  }
})

test('an unknown organization or a trust center that is off is the unavailable state', async () => {
  setLocation(`#token=${TOKEN}`)
  const calls = stub({ ovStatus: 404 })
  const html = norm(await mount(props()))
  assert.ok(html.includes('data-trust="unavailable"') && html.includes('This trust center is not available.'), html)
  assert.ok(!calls.some(c => c.url.includes('/visitor/')))
})

test('any other failure is a generic error with a retry that tries again', async () => {
  setLocation(`#token=${TOKEN}`)
  let n = 0
  const calls = stub({ inspect: () => (++n === 1 ? json(500, { message: 'x' }) : json(200, { email: 'ada@auditco.test', needs_profile: false })) })
  const html = norm(await mount(props(), async (host, settle) => {
    host.querySelector('[data-trust=retry]').click()
    await settle(h => h.includes('data-trust="confirm"'))
  }, h => h.includes('data-trust="error"')))
  assert.ok(html.includes('data-trust="confirm"'), html)
  assert.equal(calls.filter(c => c.url.endsWith('/visitor/link/inspect')).length, 2)
})

test('nothing is written to localStorage', async () => {
  setLocation(`#token=${TOKEN}`)
  stub()
  const before = JSON.stringify([...store])
  await mount(props())
  assert.equal(JSON.stringify([...store]), before)
})
