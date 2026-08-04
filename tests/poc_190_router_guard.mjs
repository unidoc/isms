// PoC harness for PR #190 (web/src/router.js SSO hash-token guard).
//
// The routes table and the beforeEach body below are TRANSPLANTED VERBATIM from
// web/src/router.js @ e57f8ca — only the Vue components are replaced by inert
// markers and the api.js/localStorage deps are injected, so the control flow
// under test is the real one. Run with:
//   node poc190.mjs
import {
  createRouter,
  createMemoryHistory,
} from '../web/node_modules/vue-router/vue-router.node.mjs'

// ---------------------------------------------------------------- fake api.js
let store = {}
const getApiToken = () => store['isms_api_token'] || ''
const setApiToken = t => { store['isms_api_token'] = t }
const clearApiToken = () => {
  delete store['isms_api_token']
  delete store['isms_user_email']
  delete store['isms_user_name']
}
// getMe(): 'ok' → any token validates. '401' → only the freshly-minted SSO token
// (NEWJWT) validates; the pre-existing local token is expired, so the server 401s
// and api.js fetchRaw's 401 branch clears it (api.js:46-52).
let getMeMode = 'ok'
const api = {
  async getMe() {
    if (getMeMode === '401' && getApiToken() !== 'NEWJWT') {
      clearApiToken()
      const e = new Error('Authentication required'); e.status = 401; throw e
    }
    return { email: 'user@example.com', organization_slug: 'acme' }
  },
  async cfSession() { return null }, // not behind CF Access
}

// -------------------------------------------------- routes (verbatim, ll.35-100)
const C = name => ({ __marker: name, render: () => null })
const Login = C('Login')
const Dashboard = C('Dashboard')
const orgScopedRoutes = [
  { path: '/overview', component: Dashboard },
  { path: '/documents', component: C('Documents') },
  { path: '/documents/:docId', component: C('Documents') },
  { path: '/inbox', component: C('Inbox') },
  { path: '/inbox/:tab?', component: C('Inbox') },
  { path: '/risks', component: C('Risks') },
  { path: '/risks/:id', component: C('Risks') },
  { path: '/settings', component: C('Settings') },
  { path: '/admin', component: C('Admin') },
  { path: '/admin/:tab', component: C('Admin') },
]
function buildOrgRoutes(subdomainMode) {
  return orgScopedRoutes.map(r => ({
    path: subdomainMode ? r.path : '/:org' + r.path,
    component: r.component,
    meta: { orgScoped: true },
  }))
}
function buildRoutes(subdomainMode, opts = {}) {
  const extra = []
  // Finding #2's proposed additions, under test in section E.
  if (opts.orgLoginRoute && !subdomainMode) {
    extra.push({ path: '/:org/login', component: Login, meta: { public: true } })
  }
  if (opts.catchAll) {
    extra.push({ path: '/:pathMatch(.*)*', redirect: '/login' })
  }
  return [
    ...extra,
    { path: '/', component: C('Landing'), meta: { public: true } },
    { path: '/login', component: Login, meta: { public: true } },
    { path: '/signup', component: C('Signup'), meta: { public: true } },
    { path: '/forgot-password', component: C('ForgotPassword'), meta: { public: true } },
    { path: '/verify-email', component: C('VerifyEmail'), meta: { public: true } },
    { path: '/verify-email-change', component: C('VerifyEmailChange'), meta: { public: true } },
    { path: '/organizations', component: C('Organizations') },
    ...buildOrgRoutes(subdomainMode),
  ]
}

// ------------------------------------------- guard (verbatim, ll.113-184)
// `variant`: 'head'  = PR #190 as merged  (gated on !getApiToken())
//            'fixed' = review finding #1's proposed change (hash always wins)
//            'base'  = master, before PR #190 (no hash block at all)
function makeRouter({ subdomainMode, orgFromSubdomain, variant, routeOpts }) {
  const router = createRouter({ history: createMemoryHistory(), routes: buildRoutes(subdomainMode, routeOpts) })
  let sessionValidated = false
  let cfTried = false

  router.beforeEach(async (to, from) => {
    if (variant === 'head') {
      // ---- PR #190, web/src/router.js:114-127 (verbatim) ----
      if (!getApiToken() && to.hash && to.hash.includes('token=')) {
        const token = new URLSearchParams(to.hash.replace(/^#/, '')).get('token')
        if (token) {
          setApiToken(token)
          return { path: to.path.replace(/\/login$/, '/overview') }
        }
      }
    } else if (variant === 'fixed') {
      // ---- proposed: drop the !getApiToken() gate, reset sessionValidated ----
      if (to.hash && to.hash.includes('token=')) {
        const token = new URLSearchParams(to.hash.replace(/^#/, '')).get('token')
        if (token) {
          setApiToken(token)
          sessionValidated = false
          return { path: to.path.replace(/\/login$/, '/overview') }
        }
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
      if (!cfTried) {
        cfTried = true
        const cf = await api.cfSession()
        if (cf?.token) {
          setApiToken(cf.token)
          if (cf.email) store['isms_user_email'] = cf.email
          if (cf.name) store['isms_user_name'] = cf.name
        }
      }
      if (!getApiToken()) {
        return { path: '/login', query: to.fullPath !== '/' ? { redirect: to.fullPath } : undefined }
      }
    }
    if (!sessionValidated) {
      try {
        const me = await api.getMe()
        if (me?.email) { sessionValidated = true; return true }
      } catch (e) {
        if (e?.status !== 401 && e?.isNetwork) return true
      }
      clearApiToken()
      sessionValidated = false
      return { path: '/login', query: to.fullPath !== '/' ? { redirect: to.fullPath } : undefined }
    }
    return true
  })
  return router
}

// -------------------------------------------------------------- Login.vue stub
// Mirrors Login.vue onMounted (ll.230-248): if the mounted route IS Login and a
// hash token is present, it stores the token and routes to the org overview.
// This is the safety net that exists in subdomain mode and NOT in path mode.
async function mountIfLogin(router) {
  const cur = router.currentRoute.value
  const isLogin = cur.matched.some(m => m.components?.default?.__marker === 'Login')
  if (!isLogin) return { mounted: cur.matched.length ? 'other' : 'NOTHING (blank page)' }
  // Login.vue reads window.location.hash — i.e. the hash of where we LANDED, not
  // of the original SSO URL. A token folded into ?redirect= is invisible to it.
  const hash = cur.hash
  const tok = hash ? new URLSearchParams(hash.replace(/^#/, '')).get('token') : null
  if (tok) {
    setApiToken(tok)
    await router.push(cur.params.org ? `/${cur.params.org}/overview` : '/overview')
    return { mounted: 'Login → handled hash' }
  }
  return { mounted: 'Login (form)' }
}

// ------------------------------------------------------------------- scenarios
let pass = 0, fail = 0
function check(label, got, want) {
  const ok = got === want
  ok ? pass++ : fail++
  console.log(`   ${ok ? 'PASS' : 'FAIL'}  ${label}\n         got:  ${got}\n         want: ${want}`)
}

async function run({ title, subdomainMode, url, preToken, getMe = 'ok', variant, routeOpts, expect }) {
  store = {}
  if (preToken) store['isms_api_token'] = preToken
  getMeMode = getMe
  const router = makeRouter({
    subdomainMode,
    orgFromSubdomain: () => (subdomainMode ? 'acme' : null),
    variant,
    routeOpts,
  })
  // The SSO callback is a full browser navigation: this IS the initial nav.
  try { await router.push(url) } catch (e) { /* navigation failure */ }
  const view = await mountIfLogin(router)   // then the destination component mounts
  const landed = router.currentRoute.value  // read the URL AFTER the view has run
  const outcome = `${landed.fullPath} | view=${view.mounted} | token=${getApiToken() || '(none)'}`
  const ro = routeOpts ? `, routes+=${Object.keys(routeOpts).join('+')}` : ''
  console.log(`\n${title}\n   push: ${url}  [${variant}, ${subdomainMode ? 'subdomain' : 'path'} mode, preToken=${preToken || 'none'}, getMe=${getMe}${ro}]`)
  check(expect.label, outcome, expect.outcome)
  return outcome
}

const JWT = 'NEWJWT'

console.log('='.repeat(78))
console.log('A. Fresh session — the flow PR #190 targets (must work)')
console.log('='.repeat(78))
await run({
  title: 'A1 path mode, /<org>/login#token= — BASE (master, pre-PR)', variant: 'base',
  subdomainMode: false, url: `/acme/login#token=${JWT}&role=reader`,
  expect: { label: '#187 reproduces on master', outcome: '/login?redirect=/acme/login%23token=NEWJWT%26role=reader | view=Login (form) | token=(none)' },
})
await run({
  title: 'A2 path mode, /<org>/login#token= — HEAD (PR #190)', variant: 'head',
  subdomainMode: false, url: `/acme/login#token=${JWT}&role=reader`,
  expect: { label: 'PR fixes the reported flow', outcome: '/acme/overview | view=other | token=NEWJWT' },
})
await run({
  title: 'A3 subdomain mode, /login#token= — HEAD', variant: 'head',
  subdomainMode: true, url: `/login#token=${JWT}&role=reader`,
  expect: { label: 'subdomain mode works', outcome: '/overview | view=other | token=NEWJWT' },
})
await run({
  title: 'A4 apex fallback shape /#token= (api_oidc.go:351) — HEAD', variant: 'head',
  subdomainMode: false, url: `/#token=${JWT}&role=reader`,
  expect: { label: 'lands on org picker, not a dashboard', outcome: '/organizations | view=other | token=NEWJWT' },
})

console.log('\n' + '='.repeat(78))
console.log('B. Finding #1 — a token already in localStorage')
console.log('='.repeat(78))
await run({
  title: 'B1 path mode, live session, /<org>/login#token= — HEAD', variant: 'head',
  subdomainMode: false, url: `/acme/login#token=${JWT}&role=reader`, preToken: 'OLDJWT', getMe: 'ok',
  expect: { label: 'blank page, fresh token dropped', outcome: '/acme/login#token=NEWJWT&role=reader | view=NOTHING (blank page) | token=OLDJWT' },
})
await run({
  title: 'B2 path mode, stale/expired token (getMe 401) — HEAD', variant: 'head',
  subdomainMode: false, url: `/acme/login#token=${JWT}&role=reader`, preToken: 'STALEJWT', getMe: '401',
  expect: { label: '#187 loop returns, token trapped in ?redirect=', outcome: '/login?redirect=/acme/login%23token=NEWJWT%26role=reader | view=Login (form) | token=(none)' },
})
await run({
  title: 'B3 subdomain mode, live session — HEAD (Login.vue is the safety net)', variant: 'head',
  subdomainMode: true, url: `/login#token=${JWT}&role=reader`, preToken: 'OLDJWT', getMe: 'ok',
  expect: { label: 'subdomain mode recovers via Login.vue', outcome: '/overview | view=Login → handled hash | token=NEWJWT' },
})
await run({
  title: 'B4 path mode, live session — FIXED variant', variant: 'fixed',
  subdomainMode: false, url: `/acme/login#token=${JWT}&role=reader`, preToken: 'OLDJWT', getMe: 'ok',
  expect: { label: 'proposed fix resolves it', outcome: '/acme/overview | view=other | token=NEWJWT' },
})
await run({
  title: 'B5 path mode, stale token — FIXED variant', variant: 'fixed',
  subdomainMode: false, url: `/acme/login#token=${JWT}&role=reader`, preToken: 'STALEJWT', getMe: '401',
  expect: { label: 'fix survives an expired local token too', outcome: '/acme/overview | view=other | token=NEWJWT' },
})
await run({
  title: 'B6 fresh session, FIXED variant — no regression vs HEAD', variant: 'fixed',
  subdomainMode: false, url: `/acme/login#token=${JWT}&role=reader`,
  expect: { label: 'fix keeps the happy path', outcome: '/acme/overview | view=other | token=NEWJWT' },
})

console.log('\n' + '='.repeat(78))
console.log('C. Finding #2 — unmatched org-scoped paths in path mode')
console.log('='.repeat(78))
{
  const r = makeRouter({ subdomainMode: false, orgFromSubdomain: () => null, variant: 'head' })
  check('/acme/login matches no route in path mode', String(r.resolve('/acme/login').matched.length), '0')
  check('/login matches Login at top level', String(r.resolve('/login').matched.length), '1')
  check('/acme/typo (any unmatched org path) matches nothing', String(r.resolve('/acme/typo').matched.length), '0')
  const rs = makeRouter({ subdomainMode: true, orgFromSubdomain: () => 'acme', variant: 'head' })
  check('/login matches Login in subdomain mode too', String(rs.resolve('/login').matched.length), '1')
}
await run({
  title: 'C1 path mode, unmatched org path with a valid session — HEAD', variant: 'head',
  subdomainMode: false, url: '/acme/typo', preToken: 'OLDJWT', getMe: 'ok',
  expect: { label: 'no catch-all → blank page', outcome: '/acme/typo | view=NOTHING (blank page) | token=OLDJWT' },
})

console.log('\n' + '='.repeat(78))
console.log('D. Findings #3 / #4 — identity keys, and hash accepted on any path')
console.log('='.repeat(78))
await run({
  title: 'D1 identity keys after a hash login — HEAD', variant: 'head',
  subdomainMode: false, url: `/acme/login#token=${JWT}&role=reader`,
  expect: { label: 'only the token is stored', outcome: '/acme/overview | view=other | token=NEWJWT' },
})
check('finding #3: isms_user_email set by the hash path?', String(store['isms_user_email']), 'undefined')
check('finding #3: isms_user_name set by the hash path?', String(store['isms_user_name']), 'undefined')
check('finding #3: role= from the hash persisted anywhere?',
  String(Object.keys(store).filter(k => k !== 'isms_api_token').length), '0')

await run({
  title: 'D2 #token= on a non-login route — HEAD', variant: 'head',
  subdomainMode: false, url: `/acme/risks#token=${JWT}`,
  expect: { label: 'token accepted on any path (was /login only)', outcome: '/acme/risks | view=other | token=NEWJWT' },
})
{
  const r = makeRouter({ subdomainMode: false, orgFromSubdomain: () => null, variant: 'base' })
  store = {}
  await r.push('/'); store = {}
  try { await r.push(`/acme/risks#token=${JWT}`) } catch {}
  check('finding #4 baseline: master ignores #token= off /login', getApiToken() || '(none)', '(none)')
}

console.log('\n' + '='.repeat(78))
console.log("E. Finding #2's SUGGESTED fix under test — /:org/login route + catch-all")
console.log('='.repeat(78))
const R2 = { orgLoginRoute: true, catchAll: true }
await run({
  title: 'E1 live session + /<org>/login registered — guard UNCHANGED (head)', variant: 'head',
  subdomainMode: false, url: `/acme/login#token=${JWT}&role=reader`, preToken: 'OLDJWT', getMe: 'ok', routeOpts: R2,
  expect: { label: 'route alone is a sufficient safety net for finding #1', outcome: '/acme/overview | view=Login → handled hash | token=NEWJWT' },
})
await run({
  title: 'E2 unmatched org path with catch-all — head', variant: 'head',
  subdomainMode: false, url: '/acme/typo', preToken: 'OLDJWT', getMe: 'ok', routeOpts: R2,
  expect: { label: 'catch-all replaces the blank page', outcome: '/login | view=Login (form) | token=OLDJWT' },
})
await run({
  title: 'E3 existing org route still matches (no shadowing) — head', variant: 'head',
  subdomainMode: false, url: '/acme/risks/5', preToken: 'OLDJWT', getMe: 'ok', routeOpts: R2,
  expect: { label: 'catch-all does not shadow /:org/risks/:id', outcome: '/acme/risks/5 | view=other | token=OLDJWT' },
})
await run({
  title: 'E4 overview still matches — head', variant: 'head',
  subdomainMode: false, url: '/acme/overview', preToken: 'OLDJWT', getMe: 'ok', routeOpts: R2,
  expect: { label: 'no regression on the main landing route', outcome: '/acme/overview | view=other | token=OLDJWT' },
})
await run({
  title: 'E5 fresh session, guard fast path still wins — head', variant: 'head',
  subdomainMode: false, url: `/acme/login#token=${JWT}&role=reader`, routeOpts: R2,
  expect: { label: 'guard handles it before Login.vue mounts', outcome: '/acme/overview | view=other | token=NEWJWT' },
})
await run({
  title: 'E6 route + catch-all combined with the FIXED guard', variant: 'fixed',
  subdomainMode: false, url: `/acme/login#token=${JWT}&role=reader`, preToken: 'OLDJWT', getMe: 'ok', routeOpts: R2,
  expect: { label: 'belt-and-braces: both fixes coexist', outcome: '/acme/overview | view=other | token=NEWJWT' },
})
{
  const r = makeRouter({ subdomainMode: false, orgFromSubdomain: () => null, variant: 'head', routeOpts: R2 })
  check('E: /acme/login now matches Login', String(r.resolve('/acme/login').matched.length), '1')
  check('E: /organizations still matches Organizations',
    String(r.resolve('/organizations').matched[0]?.components?.default?.__marker), 'Organizations')
  check('E: /signup still matches Signup',
    String(r.resolve('/signup').matched[0]?.components?.default?.__marker), 'Signup')
}

console.log('\n' + '='.repeat(78))
console.log(`RESULT: ${pass} pass, ${fail} fail`)
console.log('='.repeat(78))
process.exit(fail ? 1 : 0)
