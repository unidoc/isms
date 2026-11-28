// The trust domain (#528) in the SPA: route table, host classification, link
// roots and the manager's read-only address.
import test from 'node:test'
import assert from 'node:assert/strict'
import { createRouter, createMemoryHistory } from 'vue-router'
import { buildTrustRoutes, buildTrustHostRoutes } from '../src/trustRoutes.js'
import { isSubdomainHost, setTrustHostSlug, trustHostSlug } from '../src/composables/useCurrentOrg.js'
import { trustPaths } from '../src/utils/trustPaths.js'
import { trustDomainAddress } from '../src/utils/trustManager.js'

const Stub = { render: () => null }
const stubbed = (routes) => routes.map((r) => (r.component ? { ...r, component: Stub } : r))

function hostRouter() {
  return createRouter({ history: createMemoryHistory(), routes: stubbed(buildTrustHostRoutes()) })
}

test('trust host table: exactly /, /sign-in and /documents/:id, all public trust routes', () => {
  const r = hostRouter()
  const root = r.resolve('/')
  const signIn = r.resolve('/sign-in')
  const doc = r.resolve('/documents/pol-1')
  for (const m of [root, signIn, doc]) {
    assert.deepEqual({ public: m.meta.public, trust: m.meta.trust, trustHost: m.meta.trustHost }, { public: true, trust: true, trustHost: true })
  }
  assert.equal(signIn.meta.signIn, true)
  assert.equal(root.meta.signIn, undefined)
  assert.equal(doc.params.id, 'pol-1')
  const paths = buildTrustHostRoutes().map((x) => x.path)
  assert.deepEqual(paths, ['/', '/sign-in', '/documents/:id'])
})

test('trust host table has no catch-all: other paths match nothing, so links navigate natively', () => {
  const r = hostRouter()
  for (const p of ['/trust', '/login', '/acme/trust', '/documents', '/documents/a/b', '/branding/logo', '/api/v1/trust/acme/certifications/1/file', '/assets/app.js']) {
    assert.equal(r.resolve(p).matched.length, 0, p)
  }
})

test('buildTrustRoutes is unchanged in both modes', () => {
  assert.deepEqual(buildTrustRoutes(true).map((r) => r.path), ['/trust', '/trust/sign-in', '/trust/documents/:id'])
  assert.deepEqual(buildTrustRoutes(false).map((r) => r.path), ['/:org/trust', '/:org/trust/sign-in', '/:org/trust/documents/:id'])
  for (const r of buildTrustRoutes(true)) assert.equal(r.meta.trustHost, undefined)
})

test('a three-label trust domain is not a tenant subdomain', () => {
  assert.equal(isSubdomainHost('trust.company.com'), true) // the heuristic, before the server says otherwise
  setTrustHostSlug('acme')
  try {
    assert.equal(trustHostSlug(), 'acme')
    assert.equal(isSubdomainHost('trust.company.com'), false)
    assert.equal(isSubdomainHost('acme.isms.sh'), false)
  } finally {
    setTrustHostSlug('')
  }
  assert.equal(trustHostSlug(), '')
  assert.equal(isSubdomainHost('trust.company.com'), true)
})

test('trustPaths: root and prefix per host kind', () => {
  assert.deepEqual(trustPaths({ trustHost: true, subdomain: false, slug: 'acme' }), { root: '/', prefix: '' })
  assert.deepEqual(trustPaths({ trustHost: false, subdomain: true, slug: 'acme' }), { root: '/trust', prefix: '/trust' })
  assert.deepEqual(trustPaths({ trustHost: false, subdomain: false, slug: 'acme' }), { root: '/acme/trust', prefix: '/acme/trust' })
  assert.deepEqual(trustPaths({ trustHost: false, subdomain: false, slug: 'a b' }), { root: '/a%20b/trust', prefix: '/a%20b/trust' })
  const t = trustPaths({ trustHost: true, subdomain: false, slug: 'acme' })
  assert.equal(`${t.prefix}/documents/x`, '/documents/x')
  assert.equal(`${t.root}#sign-in`, '/#sign-in')
})

test('trustDomainAddress', () => {
  assert.equal(trustDomainAddress('https:', 'trust.example.com'), 'https://trust.example.com/')
  assert.equal(trustDomainAddress('http:', ' trust.example.com '), 'http://trust.example.com/')
  assert.equal(trustDomainAddress('', 'trust.example.com'), 'https://trust.example.com/')
  assert.equal(trustDomainAddress('https:', null), '')
  assert.equal(trustDomainAddress('https:', ''), '')
})
