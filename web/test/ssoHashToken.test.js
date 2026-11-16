// The SSO hash handoff must never take a trust center visitor's link token as a
// member session token (#524). The router's global guard runs on every route
// before any component mounts, so the decision lives in ssoHashToken().
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRouter, createMemoryHistory } from 'vue-router'
import { ssoHashToken } from '../src/utils/ssoHashToken.js'
import { buildTrustRoutes } from '../src/trustRoutes.js'

const route = (hash, meta = {}) => ({ hash, meta, path: '/x' })

test('the SSO cases keep working: /login#token= and /<org>/overview#token=', () => {
  assert.equal(ssoHashToken(route('#token=abc')), 'abc')
  assert.equal(ssoHashToken(route('#token=abc&role=reader')), 'abc')
  assert.equal(ssoHashToken({ hash: '#token=abc', meta: { public: true }, path: '/login' }), 'abc')
  assert.equal(ssoHashToken({ hash: '#token=abc', meta: {}, path: '/acme/overview' }), 'abc')
})

test('a trust route never hands off, whatever the fragment says', () => {
  assert.equal(ssoHashToken(route('#token=abc', { trust: true })), null)
  assert.equal(ssoHashToken(route('#visitor_token=abc', { trust: true })), null)
  assert.equal(ssoHashToken(route('#token=abc&visitor_token=def', { trust: true })), null)
})

test('the key is exactly token: visitor_token is not a substring match', () => {
  assert.equal(ssoHashToken(route('#visitor_token=abc')), null)
  assert.equal(ssoHashToken(route('#mytoken=abc')), null)
  assert.equal(ssoHashToken(route('#visitor_token=abc&token=real')), 'real')
})

test('no fragment, an empty token or no route is no handoff', () => {
  assert.equal(ssoHashToken(route('')), null)
  assert.equal(ssoHashToken(route('#')), null)
  assert.equal(ssoHashToken(route('#token=')), null)
  assert.equal(ssoHashToken(route('#other=1')), null)
  assert.equal(ssoHashToken(null), null)
  assert.equal(ssoHashToken({ meta: {} }), null)
})

test('all three trust routes carry meta.trust', () => {
  for (const mode of [false, true]) {
    const routes = buildTrustRoutes(mode)
    assert.equal(routes.length, 3)
    for (const r of routes) assert.equal(r.meta.trust, true, r.path)
  }
})

// The real route table and the real decision function, run through a router.
async function visit(path, subdomainMode = false) {
  const stored = []
  const routes = buildTrustRoutes(subdomainMode).map(r => ({ ...r, component: { render: () => null } }))
  routes.push({ path: '/login', component: { render: () => null }, meta: { public: true } })
  routes.push({ path: '/:org/overview', component: { render: () => null }, meta: {} })
  const router = createRouter({ history: createMemoryHistory(), routes })
  router.beforeEach((to) => {
    const token = ssoHashToken(to)
    if (token) {
      stored.push(token)
      return { path: to.path.replace(/\/login$/, '/overview') }
    }
    return true
  })
  await router.push(path)
  return { stored, current: router.currentRoute.value }
}

test('opening the emailed sign-in link stores no session token and leaves the fragment for the page', async () => {
  const tok = 'ab'.repeat(32)
  for (const [path, sub] of [[`/acme/trust/sign-in#visitor_token=${tok}`, false], [`/trust/sign-in#visitor_token=${tok}`, true]]) {
    const r = await visit(path, sub)
    assert.deepEqual(r.stored, [], path)
    assert.equal(r.current.hash, `#visitor_token=${tok}`, 'the hash reaches the component intact')
    assert.equal(r.current.meta.signIn, true)
  }
  // even an old-style #token= on a trust route is not taken
  const old = await visit(`/acme/trust/sign-in#token=${tok}`)
  assert.deepEqual(old.stored, [])
})

test('the SSO landing still hands off off the trust routes', async () => {
  const r = await visit('/login#token=sso123')
  assert.deepEqual(r.stored, ['sso123'])
})

// router.js cannot load under node, so this checks it uses the function under
// test rather than carrying its own copy of the decision.
test('router.js decides the handoff through ssoHashToken and no longer matches token= by substring', () => {
  const src = readFileSync(new URL('../src/router.js', import.meta.url), 'utf8')
  assert.ok(/import \{ ssoHashToken \} from '\.\/utils\/ssoHashToken'/.test(src))
  assert.ok(/ssoHashToken\(to\)/.test(src))
  assert.ok(!/includes\('token='\)/.test(src))
})
