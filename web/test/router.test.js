// Regression test: the Cloudflare Access probe in web/src/router.js's global
// guard has to run BEFORE any route-type decision, including meta.public.
//
// The guard body below is transplanted verbatim from router.js's beforeEach —
// only the CF-session/localStorage dependencies are injected fakes, and the
// route table is trimmed to what these scenarios need. Kept in its own factory
// (not importing the real router singleton) so each test gets a fresh
// cfTried/sessionValidated closure, matching the harness style already used in
// this file's own history for the same class of guard-ordering bug.
//
// Caveat (review finding F4 on #223): this is a COPY of the guard body, not an
// import of it — the real router.js calls createWebHistory() and imports .vue
// components at module scope, which this harness can't load under `node --test`.
// That means a future edit that reorders the real guard leaves this test green;
// the copy stays internally consistent on its own terms. It documents and
// pins the intended ordering, it does not enforce it against the shipped file.
// The fix — extracting the guard into an exported factory both router.js and
// this test consume — is a larger change, deliberately deferred.
//
// The bug this guards against: every entry point a fresh, not-yet-locally-
// authenticated visitor actually lands on — '/', '/login', '/<org>/login' — is
// meta.public. The probe used to live AFTER the meta.public early-return, so it
// never ran for any of them: a Cloudflare-Access-authenticated visitor with no
// local token got told they were logged out, and the only way in was to already
// know to type a non-public URL (e.g. /organizations) by hand — reported live
// against a self-hosted Cloudflare Access deployment.
import test from 'node:test'
import assert from 'node:assert/strict'
import { createRouter, createMemoryHistory } from 'vue-router'

const C = (name) => ({ __marker: name, render: () => null })
const Login = C('Login')

function buildRoutes(subdomainMode) {
  const extra = subdomainMode ? [] : [{ path: '/:org/login', component: Login, meta: { public: true } }]
  return [
    { path: '/', component: C('Landing'), meta: { public: true } },
    { path: '/login', component: Login, meta: { public: true } },
    { path: '/organizations', component: C('Organizations') },
    { path: subdomainMode ? '/overview' : '/:org/overview', component: C('Dashboard'), meta: { orgScoped: true } },
    ...extra,
  ]
}

// `cfResult`: what the fake api.cfSession() resolves to (null = not behind CF
// Access, or CF Access rejected the request — the real cfSession() swallows
// every failure this way, per web/src/api.js). `cfSession`, if given, replaces
// the fake entirely — used to simulate a hang. `probeTimeoutMs` mirrors the
// real guard's Promise.race bound (production uses 3000ms; tests override it
// to keep the hang case fast).
function makeRouter({ subdomainMode = false, cfResult = null, cfSession = null, probeTimeoutMs = 3000 } = {}) {
  const store = {}
  const getApiToken = () => store.isms_api_token || ''
  const setApiToken = (t) => {
    store.isms_api_token = t
  }
  const localStorageStub = { setItem: (k, v) => (store[k] = v) }
  const api = { cfSession: cfSession ?? (async () => cfResult) }
  const orgFromSubdomain = () => (subdomainMode ? 'acme' : null)

  const router = createRouter({ history: createMemoryHistory(), routes: buildRoutes(subdomainMode) })
  let cfTried = false

  // ---- router.js beforeEach, transplanted verbatim (hash-token handling and
  // sessionValidated/getMe are out of scope for this fix and omitted) ----
  router.beforeEach(async (to) => {
    if (!getApiToken() && !cfTried) {
      cfTried = true
      // Bounded the same way the real guard is (review finding F5 on #223): a
      // hung cfSession() must not hold up a public route's first paint forever.
      const cf = await Promise.race([
        api.cfSession(),
        new Promise((resolve) => setTimeout(() => resolve(null), probeTimeoutMs)),
      ])
      if (cf?.token) {
        setApiToken(cf.token)
        if (cf.email) localStorageStub.setItem('isms_user_email', cf.email)
        if (cf.name) localStorageStub.setItem('isms_user_name', cf.name)
      }
    }

    if (to.path === '/organizations' && orgFromSubdomain()) {
      return getApiToken() ? { path: '/overview' } : { path: '/login' }
    }

    if (to.meta.public) {
      if (to.path === '/' && getApiToken()) {
        const slug = orgFromSubdomain()
        if (slug) return { path: '/overview' }
        return { path: '/organizations' }
      }
      return true
    }

    if (!getApiToken()) {
      return { path: '/login', query: to.fullPath !== '/' ? { redirect: to.fullPath } : undefined }
    }
    return true
  })

  return { router, getApiToken }
}

test('landing on / with a live CF Access session mints a token and enters the org', async () => {
  const { router, getApiToken } = makeRouter({ cfResult: { token: 'NEWJWT', email: 'a@x.com' } })
  await router.push('/')
  assert.equal(getApiToken(), 'NEWJWT', 'the probe must run on the public landing route, not skip it')
  assert.equal(router.currentRoute.value.path, '/organizations', 'apex/path mode with no subdomain falls to the org picker')
})

test('landing on /login with a live CF Access session mints a token without blocking the navigation', async () => {
  // The guard itself just has to not block; Login.vue's own onMounted (verified
  // separately, api_auth.go-adjacent) redirects an already-authenticated visitor
  // away from the form once it mounts.
  const { router, getApiToken } = makeRouter({ cfResult: { token: 'NEWJWT' } })
  await router.push('/login')
  assert.equal(getApiToken(), 'NEWJWT')
  assert.equal(router.currentRoute.value.path, '/login')
})

test('landing on /<org>/login (path mode) mints a token the same way', async () => {
  const { router, getApiToken } = makeRouter({ cfResult: { token: 'NEWJWT' } })
  await router.push('/acme/login')
  assert.equal(getApiToken(), 'NEWJWT', 'org-scoped login is meta.public too and must not skip the probe')
})

test('not behind Cloudflare Access: the probe runs, finds nothing, and the public route renders normally', async () => {
  const { router, getApiToken } = makeRouter({ cfResult: null })
  await router.push('/')
  assert.equal(getApiToken(), '')
  assert.equal(router.currentRoute.value.path, '/')
})

test('the probe fires at most once per page load, even across several navigations', async () => {
  let calls = 0
  const store = {}
  const getApiToken = () => store.isms_api_token || ''
  const router = createRouter({ history: createMemoryHistory(), routes: buildRoutes(false) })
  let cfTried = false
  router.beforeEach(async (to) => {
    if (!getApiToken() && !cfTried) {
      cfTried = true
      calls++
    }
    if (to.meta.public) return true
    return getApiToken() ? true : { path: '/login' }
  })
  await router.push('/')
  await router.push('/login')
  await router.push('/')
  assert.equal(calls, 1)
})

test('a hung cf-session probe degrades to "not logged in" instead of blocking navigation forever', async () => {
  const { router, getApiToken } = makeRouter({
    cfSession: () => new Promise(() => {}), // never resolves
    probeTimeoutMs: 20,
  })
  await router.push('/')
  assert.equal(getApiToken(), '', 'a hang must not be mistaken for a successful login')
  assert.equal(router.currentRoute.value.path, '/', 'the public route still renders instead of hanging')
})

test('subdomain mode: /organizations with no token also gets the probe before redirecting to /login', async () => {
  // Previously this branch ran BEFORE the (old) probe location too, so a
  // subdomain visitor hitting /organizations directly had the same bug.
  const { router, getApiToken } = makeRouter({ subdomainMode: true, cfResult: { token: 'NEWJWT' } })
  await router.push('/organizations')
  assert.equal(getApiToken(), 'NEWJWT')
  assert.equal(router.currentRoute.value.path, '/overview')
})
