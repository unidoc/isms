import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://acme-logistics.example/login' })
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.localStorage = dom.window.localStorage

// Regression: on a path-based (non-subdomain) deployment, a user who belongs
// to more than one org would get bounced back to the org picker after a
// correct password login, because the request never told the backend which
// org they meant to log into — it silently relied on the backend's
// exactly-one-org auto-select fallback. Login.vue knows the org (the visitor
// typed or navigated it in), it just wasn't being sent.
test('login() sends the org slug when one is known', async () => {
  let capturedBody = null
  globalThis.fetch = async (url, opts) => {
    capturedBody = JSON.parse(opts.body)
    return {
      ok: true,
      json: async () => ({ token: 't', email: 'a@b.com' }),
    }
  }

  const { login } = await import('../src/api.js')
  await login('a@b.com', 'pw', undefined, 'acme-logistics')

  assert.equal(capturedBody.organization, 'acme-logistics')
})

test('login() omits organization when none is known', async () => {
  let capturedBody = null
  globalThis.fetch = async (url, opts) => {
    capturedBody = JSON.parse(opts.body)
    return {
      ok: true,
      json: async () => ({ token: 't', email: 'a@b.com' }),
    }
  }

  const { login } = await import('../src/api.js')
  await login('a@b.com', 'pw', undefined, undefined)

  assert.equal('organization' in capturedBody, false)
})
