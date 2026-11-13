// The trust center preview route (#520): authenticated, admin/manager only, bare
// (no app chrome), showing the public page against the preview endpoints.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const View = await compileSfc(new URL('../src/views/TrustCenterPreview.vue', import.meta.url).pathname, { stubChildren: true })

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}
const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

function stub(role, enabled) {
  const calls = []
  globalThis.fetch = async (url) => {
    const path = String(url).replace(/^.*\/api\/v1/, '')
    calls.push(path)
    if (path === '/me') return json(200, { role })
    if (path === '/trust-center/settings') return json(200, { enabled })
    if (path.startsWith('/trust-center/preview')) return json(200, path.endsWith('/items') ? [] : { title: 't' })
    return json(404, {})
  }
  return calls
}
const ready = h => h.includes('data-trust="not-allowed"') || h.includes('data-trust="preview-banner"')
const flush = () => new Promise(r => setTimeout(r, 40))

test('reader and contributor see "not allowed" and no preview request is made', async () => {
  for (const role of ['reader', 'contributor']) {
    const calls = stub(role, true)
    globalThis.__sfcStubAttrs = []
    const html = await renderSfcSettled(View, {}, ready, async () => { await flush() })
    assert.ok(html.includes('data-trust="not-allowed"'), `${role}: ${html}`)
    assert.deepEqual(calls.filter(p => p.startsWith('/trust-center')), [], role)
    assert.equal(globalThis.__sfcStubAttrs.length, 0, `${role}: the page must not be mounted`)
  }
})

test('banner: always says what this is; adds the off text only while the trust center is off', async () => {
  stub('admin', true)
  const on = await renderSfcSettled(View, {}, ready, async () => { await flush() })
  assert.ok(on.includes('Preview: this is what visitors see'), on)
  assert.ok(!on.includes('The trust center is off; visitors currently see nothing.'), on)
  assert.ok(on.includes('Back to the trust center manager'), on)
  stub('manager', false)
  const off = await renderSfcSettled(View, {}, ready, async () => { await flush() })
  assert.ok(off.includes('Preview: this is what visitors see'), off)
  assert.ok(off.includes('The trust center is off; visitors currently see nothing.'), off)
})

test('the page gets the preview loaders and a base path inside the preview', async () => {
  const calls = stub('admin', false)
  globalThis.__sfcStubAttrs = []
  await renderSfcSettled(View, {}, ready, async () => { await flush() })
  const attrs = globalThis.__sfcStubAttrs.at(-1)
  assert.ok(attrs, 'page not mounted')
  assert.match(attrs['base-path'], /\/trust-center\/preview$/)
  await attrs.source.overview()
  await attrs.source.items()
  await attrs.source.document('abc')
  assert.deepEqual(calls.filter(p => p.startsWith('/trust-center/preview')),
    ['/trust-center/preview', '/trust-center/preview/items', '/trust-center/preview/documents/abc'])
  // A certification file is fetched through the preview route with the session (#521).
  globalThis.fetch = async (url, opts = {}) => {
    calls.push(String(url).replace(/^.*\/api\/v1/, ''))
    assert.match(opts.headers?.Authorization || '', /^Bearer /, 'the preview download carries the session')
    return new Response('%PDF', { status: 200, headers: { 'Content-Disposition': 'attachment; filename=r.pdf' } })
  }
  store.set('isms_api_token', 'tok')
  const f = await attrs.source.certificationFile('item-1')
  assert.equal(f.filename, 'r.pdf')
  assert.equal(await f.blob.text(), '%PDF')
  assert.equal(calls.at(-1), '/trust-center/preview/certifications/item-1/file')
  assert.ok(!calls.some(p => p.startsWith('/trust/')), 'no anonymous trust call')
})

const src = f => readFileSync(new URL(f, import.meta.url), 'utf8')

test('the preview routes are org scoped, authenticated and bare', () => {
  const router = src('../src/router.js')
  assert.match(router, /path: '\/trust-center\/preview', component: TrustCenterPreview, bare: true/)
  assert.match(router, /path: '\/trust-center\/preview\/documents\/:id', component: TrustCenterPreview, bare: true/)
  // bare is a meta flag of its own; it never sets `public`, which would skip login.
  assert.match(router, /meta: \{ orgScoped: true, \.\.\.\(r\.bare && \{ bare: true \}\) \}/)
  assert.doesNotMatch(router, /bare[^\n]*public: true|public: true[^\n]*bare/)
})

test('App.vue renders bare routes without chrome but still loads the app data', () => {
  const app = src('../src/App.vue')
  assert.match(app, /v-if="isPublicRoute \|\| isBareRoute \|\| route\.path === '\/organizations'"/)
  assert.match(app, /const isBareRoute = computed\(\(\) => !!route\.meta\.bare\)/)
  // loadAppData is skipped for public routes only, never for bare ones.
  const load = app.slice(app.indexOf('async function loadAppData()'))
  assert.match(load.slice(0, 200), /if \(isPublicRoute\.value \|\| route\.path === '\/organizations'\) return/)
  assert.ok(!/isBareRoute/.test(load.slice(0, 200)))
  assert.doesNotMatch(app.slice(app.indexOf('function isRoutePublic'), app.indexOf('function isRoutePublic') + 200), /bare/)
})
