// Regression coverage for #241 review finding F1/F2: the router guard bounces
// an unauthenticated deep link (e.g. /acme/documents) to the BARE /login with
// ?redirect=/acme/documents — not /acme/login — so Login.vue's getOrgSlug()
// has to recover the org from the redirect target, and the org it recovers
// has to actually reach the login request (Login.vue:handleLogin -> api.js's
// login()), not just live in a local variable.
//
// getOrgSlug() is transplanted verbatim (see router.test.js for the same
// convention and its caveat: this is a copy, not an import — Login.vue
// imports .vue components and browser globals that this harness doesn't
// load under `node --test`). login() itself is the REAL implementation from
// src/api.js, imported normally, so the request-body assertion below is not
// testing a copy on both ends.
import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'
import { createRouter, createMemoryHistory } from 'vue-router'

const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://isms.example/login' })
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.localStorage = dom.window.localStorage

const C = { render: () => null }
function buildRouter(subdomainMode) {
  const orgRoutes = subdomainMode
    ? [{ path: '/documents', component: C }, { path: '/documents/:docId', component: C }]
    : [{ path: '/:org/documents', component: C }, { path: '/:org/documents/:docId', component: C }]
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: C },
      { path: '/login', component: C },
      { path: '/organizations', component: C },
      ...orgRoutes,
    ],
  })
}

// Transplant of Login.vue's getOrgSlug(), source 4 (the #241 fix under
// review). `search` stands in for window.location.search, `path` for
// window.location.pathname.
function getOrgSlug({ search, path, subSlug, router }) {
  const params = new URLSearchParams(search)
  if (params.get('org')) return params.get('org')
  if (subSlug) return subSlug
  const pathParts = path.split('/').filter(Boolean)
  if (pathParts.length >= 2 && pathParts[1] === 'login') return pathParts[0]
  const redirect = params.get('redirect')
  if (redirect) {
    const org = router.resolve(redirect).params.org
    if (org) return org
  }
  return ''
}

test('deep link bounced to bare /login recovers the org from ?redirect=', () => {
  const router = buildRouter(false)
  const slug = getOrgSlug({ search: '?redirect=%2Facme%2Fdocuments', path: '/login', subSlug: null, router })
  assert.equal(slug, 'acme')
})

test('?redirect=/organizations does not get misread as an org slug', () => {
  // A naive first-path-segment split would yield "organizations" here.
  const router = buildRouter(false)
  const slug = getOrgSlug({ search: '?redirect=%2Forganizations', path: '/login', subSlug: null, router })
  assert.equal(slug, '')
})

test('subdomain mode: ?redirect=/documents does not get misread as an org slug', () => {
  // Org-scoped routes are mounted at the top level in subdomain mode, so
  // "documents" is a real route segment, not an org — a naive split would
  // still yield "documents".
  const router = buildRouter(true)
  const slug = getOrgSlug({ search: '?redirect=%2Fdocuments', path: '/login', subSlug: null, router })
  assert.equal(slug, '')
})

test('an explicit ?org= still wins over a redirect-derived org', () => {
  const router = buildRouter(false)
  const slug = getOrgSlug({ search: '?org=beta&redirect=%2Facme%2Fdocuments', path: '/login', subSlug: null, router })
  assert.equal(slug, 'beta')
})

test('the redirect-derived org reaches the real login() request body', async () => {
  const router = buildRouter(false)
  const slug = getOrgSlug({ search: '?redirect=%2Facme%2Fdocuments', path: '/login', subSlug: null, router })

  let capturedBody = null
  globalThis.fetch = async (url, opts) => {
    capturedBody = JSON.parse(opts.body)
    return { ok: true, json: async () => ({ token: 't', email: 'a@b.com' }) }
  }

  const { login } = await import('../src/api.js')
  await login('a@b.com', 'pw', undefined, slug || undefined)

  assert.equal(capturedBody.organization, 'acme')
})
