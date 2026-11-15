// The sign-in token must not survive in the browser history (#524). The page
// strips the fragment; if that is done behind vue-router's back, the router's own
// history state (state.current) keeps the tokenised path, and its next navigation
// writes that path back, so Back lands on the token URL. This runs the real
// component under a real vue-router with a real (jsdom) history.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, jsdomInstance } from './support/renderSfc.js'

const SignIn = await compileSfc(new URL('../src/components/TrustSignIn.vue', import.meta.url).pathname, { stubChildren: true })

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}
const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const TOKEN = 'ab'.repeat(32)
const OTHER = 'cd'.repeat(32)
const wait = (ms = 40) => new Promise(r => setTimeout(r, ms))

function stubServer() {
  globalThis.fetch = async (url, opts = {}) => {
    const u = String(url)
    if (u.endsWith('/api/v1/config')) return json(200, { locales: [{ tag: 'en', name: 'English' }] })
    if (u.endsWith('/api/v1/trust/acme')) return json(200, { branding_name: 'Acme', branding_color: '', default_locale: 'en' })
    if (u.endsWith('/visitor/link/inspect')) return json(200, { email: 'ada@auditco.test', needs_profile: false })
    if (u.endsWith('/visitor/session')) return json(200, { visitor: { email: 'ada@auditco.test', name: 'A', company: 'C' } })
    return json(404, {})
  }
}

// Mounts the page at `url` with a real router and web history.
async function boot(url) {
  jsdomInstance.reconfigure({ url })
  globalThis.location = window.location
  globalThis.history = window.history
  const { createApp, h } = await import('vue')
  const { createRouter, createWebHistory } = await import('vue-router')
  const { i18n } = await import('../src/i18n.js')
  const Stub = { render: () => h('div', { 'data-page': 'trust' }) }
  let router
  const Sign = { render: () => h(SignIn, { slug: 'acme', basePath: '/acme/trust', onNavigate: p => router.push(p) }) }
  router = createRouter({
    history: createWebHistory(),
    routes: [
      { path: '/acme/trust', component: Stub, meta: { public: true, trust: true } },
      { path: '/acme/trust/sign-in', component: Sign, meta: { public: true, trust: true, signIn: true } },
    ],
  })
  const app = createApp({ render: () => h('div', [h(Stub)]) })
  app.use(router)
  app.use(i18n)
  app.component('RouterView', { render: () => h('div') })
  const host = document.createElement('div')
  document.body.appendChild(host)
  // mount the sign-in component inside the app (so useRouter works) via a tiny root
  const root = createApp({ render: () => h(Sign) })
  root.use(router)
  root.use(i18n)
  await router.replace(window.location.pathname + window.location.search + window.location.hash)
  await router.isReady()
  root.mount(host)
  return { router, root, host }
}

const dump = () => JSON.stringify({ href: window.location.href, state: window.history.state })

test('the fragment is stripped through the router: no history entry or state keeps the token, before or after navigating and going Back', async () => {
  stubServer()
  const pushed = []
  const origPush = window.history.pushState.bind(window.history)
  const origReplace = window.history.replaceState.bind(window.history)
  let tracking = false
  window.history.pushState = (...a) => { if (tracking) pushed.push(JSON.stringify(a)); return origPush(...a) }
  window.history.replaceState = (...a) => { if (tracking) pushed.push(JSON.stringify(a)); return origReplace(...a) }

  const { router, root, host } = await boot(`http://localhost/acme/trust/sign-in#visitor_token=${TOKEN}`)
  await wait(120) // inspect, confirm screen
  tracking = true
  assert.ok(host.innerHTML.includes('data-trust="confirm"'), host.innerHTML)
  // stripped, in the URL and in the router's own state
  assert.ok(!dump().includes('visitor_token') && !dump().includes(TOKEN), dump())
  assert.equal(window.location.hash, '')

  // redeem -> the page navigates to the trust page
  host.querySelector('[data-trust=confirm-submit]').click()
  await wait(120)
  assert.equal(router.currentRoute.value.path, '/acme/trust')
  assert.ok(!dump().includes(TOKEN), dump())

  // Back returns to the sign-in entry WITHOUT the token
  window.history.back()
  await wait(120)
  assert.equal(router.currentRoute.value.path, '/acme/trust/sign-in')
  assert.ok(!dump().includes(TOKEN) && !dump().includes('visitor_token'), dump())

  // nothing written to history after the strip carries the token either
  for (const w of pushed) assert.ok(!w.includes(TOKEN), w)
  window.history.pushState = origPush
  window.history.replaceState = origReplace
  root.unmount()
  host.remove()
})

test('a second link opened in the same tab is stripped through the router too', async () => {
  stubServer()
  const { root, host } = await boot(`http://localhost/acme/trust/sign-in#visitor_token=${TOKEN}`)
  await wait(120)
  window.location.hash = `#visitor_token=${OTHER}`
  await wait(160)
  assert.ok(!dump().includes(OTHER) && !dump().includes('visitor_token'), dump())
  assert.ok(host.innerHTML.includes('data-trust="confirm"'), host.innerHTML)
  root.unmount()
  host.remove()
})
