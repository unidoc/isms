// The Visitors section of the trust center manager screen (#524).
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const Section = await compileSfc(new URL('../src/components/trust/TrustVisitorsSection.vue', import.meta.url).pathname, { stubChildren: true })

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}
const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

const visitor = (id, extra = {}) => ({
  id, email: `v${id}@auditco.test`, name: `Name ${id}`, company: `Co ${id}`, verified_at: 1760000000,
  first_seen: 1760000000, last_seen: 1760003600, blocked: false, blocked_at: null, blocked_by: null, active_sessions: 1, ...extra,
})

function stub({ list = [visitor(1), visitor(2, { active_sessions: 0 })], listStatus = 200, action = null } = {}) {
  const st = { calls: [], list }
  globalThis.fetch = async (url, opts = {}) => {
    const method = opts.method || 'GET'
    const path = String(url).replace(/^.*\/api\/v1/, '')
    st.calls.push({ method, path, body: opts.body ? JSON.parse(opts.body) : undefined })
    if (method === 'GET' && path === '/trust-center/visitors') {
      return listStatus === 200 ? json(200, st.list) : json(listStatus, { message: 'boom' })
    }
    if (action) { const r = action(method, path, opts); if (r) return r }
    let m = path.match(/^\/trust-center\/visitors\/(\d+)\/revoke-sessions$/)
    if (method === 'POST' && m) {
      st.list = st.list.map(v => (v.id === +m[1] ? { ...v, active_sessions: 0 } : v))
      return json(200, { revoked: 1 })
    }
    m = path.match(/^\/trust-center\/visitors\/(\d+)\/blocked$/)
    if (method === 'PUT' && m) {
      const blocked = JSON.parse(opts.body).blocked
      const next = { ...st.list.find(v => v.id === +m[1]), blocked, blocked_by: blocked ? 'mgr@acme.test' : null, blocked_at: blocked ? 1760009999 : null, active_sessions: blocked ? 0 : 1 }
      st.list = st.list.map(v => (v.id === next.id ? next : v))
      return json(200, next)
    }
    return json(404, {})
  }
  return st
}
const ready = h => ['data-trust="visitors-empty"', 'data-trust="visitors-error"', 'data-trust-visitor='].some(m => h.includes(m))
const mount = (interact, isReady = ready) => renderSfcSettled(Section, {}, isReady, interact)
const norm = h => h.replaceAll(/<!--[\s\S]*?-->/g, '').replaceAll(/\s+/g, ' ')
const row = (host, email) => host.querySelector(`[data-trust-visitor="${email}"]`)

test('rows show email, name, company, sessions and an Active badge', async () => {
  stub()
  const html = norm(await mount())
  for (const want of ['v1@auditco.test', 'Name 1', 'Co 1', 'data-trust="v-active"', 'Visitors', 'Active sessions', 'First seen']) {
    assert.ok(html.includes(want), `${want}: ${html}`)
  }
  assert.ok(!html.includes('data-trust="v-blocked"'))
})

test('timestamps go through formatDate (an instant in the viewer zone, not a UTC day)', async () => {
  stub()
  const html = norm(await mount())
  const expected = new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(1760003600 * 1000))
  assert.ok(html.includes(expected), `${expected} not in ${html}`)
})

test('revoke is disabled with no active sessions and enabled otherwise', async () => {
  stub()
  const html = await mount()
  const doc = new window.DOMParser().parseFromString(html, 'text/html')
  const r1 = doc.querySelector('[data-trust-visitor="v1@auditco.test"] [data-trust=revoke]')
  const r2 = doc.querySelector('[data-trust-visitor="v2@auditco.test"] [data-trust=revoke]')
  assert.equal(r1.hasAttribute('disabled'), false)
  assert.equal(r2.hasAttribute('disabled'), true)
  assert.ok(r1.getAttribute('title').includes('sign in again'), 'the title says what revoking does')
})

test('revoke calls the endpoint and refreshes the list', async () => {
  const st = stub()
  const html = norm(await mount(async (host, settle) => {
    row(host, 'v1@auditco.test').querySelector('[data-trust=revoke]').click()
    await settle(() => st.calls.filter(c => c.path === '/trust-center/visitors').length === 2)
    await new Promise(r => setTimeout(r, 30))
  }))
  assert.ok(st.calls.some(c => c.method === 'POST' && c.path === '/trust-center/visitors/1/revoke-sessions'), JSON.stringify(st.calls))
  assert.equal(st.calls.filter(c => c.method === 'GET').length, 2, 'the list is read again')
  const doc = new window.DOMParser().parseFromString(html, 'text/html')
  assert.equal(doc.querySelector('[data-trust-visitor="v1@auditco.test"] [data-trust=v-sessions]').textContent.trim(), '0')
})

test('block says it ends sessions, calls the endpoint, and shows the badge with who blocked', async () => {
  const st = stub()
  let title = ''
  const html = norm(await mount(async (host, settle) => {
    const b = row(host, 'v1@auditco.test').querySelector('[data-trust=block]')
    title = b.getAttribute('title')
    b.click()
    await settle(h => h.includes('data-trust="v-blocked"'))
  }))
  assert.ok(/ends all their sessions/.test(title), title)
  const c = st.calls.find(x => x.method === 'PUT')
  assert.equal(c.path, '/trust-center/visitors/1/blocked')
  assert.deepEqual(c.body, { blocked: true })
  assert.ok(html.includes('Blocked') && html.includes('Blocked by mgr@acme.test'), html)
  assert.ok(html.includes('data-trust="unblock"'), html)
})

test('unblock calls the endpoint with blocked false', async () => {
  const st = stub({ list: [visitor(1, { blocked: true, blocked_by: 'mgr@acme.test', active_sessions: 0 })] })
  const html = norm(await mount(async (host, settle) => {
    row(host, 'v1@auditco.test').querySelector('[data-trust=unblock]').click()
    await settle(h => h.includes('data-trust="v-active"'))
  }))
  assert.deepEqual(st.calls.find(c => c.method === 'PUT').body, { blocked: false })
  assert.ok(html.includes('data-trust="v-active"') && !html.includes('data-trust="v-blocked"'), html)
})

test('empty state', async () => {
  stub({ list: [] })
  const html = norm(await mount())
  assert.ok(html.includes('data-trust="visitors-empty"') && html.includes('No visitors have signed in yet.'), html)
})

test('a failed load shows an error with Retry, and Retry reads the list again', async () => {
  let n = 0
  const st = stub()
  const inner = globalThis.fetch
  globalThis.fetch = async (url, opts) => (String(url).endsWith('/trust-center/visitors') && !opts?.method && ++n === 1 ? json(500, { message: 'boom' }) : inner(url, opts))
  const html = norm(await mount(async (host, settle) => {
    host.querySelector('[data-trust=visitors-retry]').click()
    await settle(h => h.includes('data-trust-visitor='))
  }, h => h.includes('data-trust="visitors-error"')))
  assert.ok(html.includes('data-trust-visitor='), html)
  assert.equal(n, 2, 'the list was requested again after Retry')
  assert.equal(st.calls.filter(c => c.method === 'GET').length, 1, 'the retry reached the server')
})

test('a failed action shows an error and keeps the list', async () => {
  stub({ action: (m, p) => (m === 'PUT' ? json(500, { message: 'boom' }) : null) })
  const html = norm(await mount(async (host, settle) => {
    row(host, 'v1@auditco.test').querySelector('[data-trust=block]').click()
    await settle(h => h.includes('data-trust="visitor-action-error"'))
  }))
  assert.ok(html.includes('Could not update the visitor') && html.includes('data-trust-visitor="v1@auditco.test"'), html)
})

test('name and company are untrusted: shown as text, never as markup', async () => {
  window.__x = 0
  stub({ list: [visitor(1, { name: '<img src=x onerror="window.__x=1">', company: '<b>Evil</b><script>window.__x=2</script>' })] })
  const html = await mount()
  const doc = new window.DOMParser().parseFromString(html, 'text/html')
  const r = doc.querySelector('[data-trust-visitor]')
  assert.equal(r.querySelector('img'), null)
  assert.equal(r.querySelector('script'), null)
  assert.equal(r.querySelector('b'), null)
  assert.equal(r.querySelector('[data-trust=v-name]').textContent.trim(), '<img src=x onerror="window.__x=1">')
  assert.equal(r.querySelector('[data-trust=v-company]').textContent.trim(), '<b>Evil</b><script>window.__x=2</script>')
  assert.equal(window.__x, 0)
})

test('every button is type=button', async () => {
  stub()
  const html = await mount()
  const doc = new window.DOMParser().parseFromString(html, 'text/html')
  for (const b of doc.querySelectorAll('button')) assert.equal(b.getAttribute('type'), 'button', b.outerHTML)
})
