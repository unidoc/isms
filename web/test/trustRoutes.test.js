// The public trust center routes (#519). Imports the real route table: unlike
// router.js it has no top-level side effects, so it loads under node.
import test from 'node:test'
import assert from 'node:assert/strict'
import { createRouter, createMemoryHistory } from 'vue-router'
import { buildTrustRoutes } from '../src/trustRoutes.js'

const Stub = { render: () => null }

async function resolveIn(subdomainMode, path) {
  const routes = buildTrustRoutes(subdomainMode).map(r => ({ ...r, component: Stub }))
  // A route that must not be shadowed by the trust table.
  routes.push({ path: '/:org/login', component: Stub, meta: { public: true } })
  routes.push({ path: '/:org/documents/:docId', component: Stub, meta: { orgScoped: true } })
  const router = createRouter({ history: createMemoryHistory(), routes })
  return router.resolve(path)
}

test('path mode: /:org/trust and /:org/trust/documents/:id are public routes', async () => {
  const a = await resolveIn(false, '/acme/trust')
  assert.equal(a.matched.length, 1)
  assert.equal(a.meta.public, true)
  assert.equal(a.params.org, 'acme')
  const b = await resolveIn(false, '/acme/trust/documents/x')
  assert.equal(b.meta.public, true)
  assert.deepEqual({ org: b.params.org, id: b.params.id }, { org: 'acme', id: 'x' })
})

test('tenant host: /trust and /trust/documents/:id are public routes', async () => {
  const a = await resolveIn(true, '/trust')
  assert.equal(a.meta.public, true)
  const b = await resolveIn(true, '/trust/documents/x')
  assert.equal(b.meta.public, true)
  assert.equal(b.params.id, 'x')
})

test('the trust routes do not shadow login or org-scoped document routes', async () => {
  const login = await resolveIn(false, '/acme/login')
  assert.equal(login.meta.public, true)
  assert.ok(!login.path.includes('trust'))
  const doc = await resolveIn(false, '/acme/documents/pol-1')
  assert.equal(doc.meta.orgScoped, true)
})

test('path mode does not register the bare /trust, tenant mode does not register /:org/trust', async () => {
  assert.equal((await resolveIn(false, '/trust')).matched.length, 0)
  assert.equal((await resolveIn(true, '/acme/trust')).matched.length, 0)
})
