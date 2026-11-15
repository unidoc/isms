// The visitor sign-in calls (#524): anonymous like the other trust reads, JSON
// on every POST, and errors that carry the status and the server's code.
import test from 'node:test'
import assert from 'node:assert/strict'

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}
const { api } = await import('../src/api.js')

function record(status, body) {
  const calls = []
  globalThis.fetch = async (url, opts = {}) => {
    calls.push({ url: String(url), opts })
    return new Response(typeof body === 'string' ? body : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
  }
  return calls
}
const headerKeys = c => Object.keys(c.opts.headers || {}).map(k => k.toLowerCase())

test('every visitor call is anonymous even with a session token stored', async () => {
  store.set('isms_api_token', 'tok-123')
  let calls = record(200, { visitor: null })
  await api.trustVisitor('acme')
  for (const call of [
    () => api.trustRequestLink('acme', 'a@b.test'),
    () => api.trustInspectLink('acme', 'x'),
    () => api.trustSignIn('acme', 'x', 'N', 'C'),
    () => api.trustSignOut('acme'),
  ]) {
    calls = record(200, { ok: 1 })
    await call()
    assert.ok(!headerKeys(calls[0]).includes('authorization'), JSON.stringify(calls[0]))
  }
  calls = record(200, { visitor: null })
  await api.trustVisitor('acme')
  assert.ok(!headerKeys(calls[0]).includes('authorization'))
  store.delete('isms_api_token')
})

test('the POST calls use the trust paths, JSON bodies and the JSON content type', async () => {
  const cases = [
    [() => api.trustRequestLink('acme/x', 'a@b.test'), '/api/v1/trust/acme%2Fx/visitor/link', { email: 'a@b.test' }],
    [() => api.trustInspectLink('acme', 'tok'), '/api/v1/trust/acme/visitor/link/inspect', { token: 'tok' }],
    [() => api.trustSignIn('acme', 'tok', 'Ada', 'Co'), '/api/v1/trust/acme/visitor/session', { token: 'tok', name: 'Ada', company: 'Co' }],
    [() => api.trustSignIn('acme', 'tok'), '/api/v1/trust/acme/visitor/session', { token: 'tok' }],
    [() => api.trustSignOut('acme'), '/api/v1/trust/acme/visitor/sign-out', {}],
  ]
  for (const [call, path, body] of cases) {
    const calls = record(200, { visitor: null })
    await call()
    const c = calls[0]
    assert.ok(c.url.endsWith(path), c.url)
    assert.equal(c.opts.method, 'POST')
    assert.equal(c.opts.headers['Content-Type'], 'application/json')
    assert.deepEqual(JSON.parse(c.opts.body), body)
    assert.notEqual(c.opts.credentials, 'omit', 'the visitor cookie must travel')
  }
})

test('GET visitor is a plain GET on the visitor path', async () => {
  const calls = record(200, { visitor: { email: 'a@b.test', name: 'A', company: 'C' } })
  const r = await api.trustVisitor('acme')
  assert.ok(calls[0].url.endsWith('/api/v1/trust/acme/visitor'))
  assert.equal(r.visitor.email, 'a@b.test')
})

test('an error answer throws with status, code and params', async () => {
  record(400, { message: 'blocked', code: 'trust_email_domain_blocked', params: { value: 'gmail.com' } })
  await assert.rejects(api.trustRequestLink('acme', 'a@gmail.com'), e => e.status === 400 && e.code === 'trust_email_domain_blocked' && e.params.value === 'gmail.com')
  record(429, { message: 'too many requests' })
  await assert.rejects(api.trustRequestLink('acme', 'a@b.test'), e => e.status === 429 && e.code === undefined)
  record(503, 'not json at all')
  await assert.rejects(api.trustRequestLink('acme', 'a@b.test'), e => e.status === 503)
  record(409, { message: 'x', code: 'trust_visitor_profile_required' })
  await assert.rejects(api.trustSignIn('acme', 't'), e => e.status === 409 && e.code === 'trust_visitor_profile_required')
})

test('the visitor calls never dispatch the unauthorized event', async () => {
  let fired = false
  globalThis.window = globalThis.window || { dispatchEvent: () => {} }
  const orig = globalThis.window.dispatchEvent
  globalThis.window.dispatchEvent = () => { fired = true; return true }
  record(401, { message: 'nope' })
  await assert.rejects(api.trustVisitor('acme'))
  await assert.rejects(api.trustSignOut('acme'))
  globalThis.window.dispatchEvent = orig
  assert.equal(fired, false)
})
