// The weekly-digest toggle on the Settings page (#232): it shows the value the
// server holds, and flipping it PUTs the new value. The real DigestToggle.vue
// is mounted with a stubbed fetch (Settings.vue itself imports another .vue
// component, which the test helper cannot load).
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const TOGGLE = new URL('../src/components/DigestToggle.vue', import.meta.url).pathname
const Settings = await compileSfc(TOGGLE)

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}

const json = (status, body) => new Response(JSON.stringify(body), {
  status, headers: { 'Content-Type': 'application/json' },
})

// me: what /me answers. digest: { get: enabled, put: handler }.
function stubFetch(me, digest) {
  const calls = []
  globalThis.fetch = async (url, opts = {}) => {
    const u = String(url)
    const method = opts.method || 'GET'
    calls.push({ url: u, method, body: opts.body })
    if (u.endsWith('/me/digest')) return method === 'PUT' ? digest.put(opts) : json(200, { enabled: digest.get })
    if (u.endsWith('/me')) return json(200, me)
    return json(200, [])
  }
  return calls
}

const ME = { email: 'a@x.io', name: 'A', role: 'reader', organization_id: 7, organization_name: 'Acme Corp' }
const loaded = html => html.includes('type="checkbox"') && !/type="checkbox"[^>]*disabled/.test(html)

// Reads the live checkbox: Vue sets `checked` as a DOM property, which the
// serialized HTML does not show.
async function boxState(get) {
  stubFetch(ME, { get })
  let checked
  const html = await renderSfcSettled(Settings, { orgName: 'Acme Corp' }, loaded, async (host) => {
    checked = host.querySelector('input[type="checkbox"]').checked
  })
  return { checked, html }
}

test('the toggle shows the stored value and names the organization', async () => {
  const off = await boxState(false)
  assert.ok(off.html.includes('Weekly digest'), off.html)
  assert.ok(off.html.includes('in Acme Corp'), `org name missing: ${off.html}`)
  assert.equal(off.checked, false, 'box checked although the server says off')
  assert.equal((await boxState(true)).checked, true, 'box unchecked although the server says on')
})

async function flip(put) {
  const calls = stubFetch(ME, { get: true, put })
  let after
  const html = await renderSfcSettled(Settings, { orgName: 'Acme Corp' }, loaded, async (host, settle) => {
    const box = host.querySelector('input[type="checkbox"]')
    box.checked = false
    box.dispatchEvent(new window.Event('change', { bubbles: true }))
    await settle(h => h.includes('text-emerald-400') || h.includes('text-red-400'))
    after = host.querySelector('input[type="checkbox"]').checked
  })
  return { calls, html, after }
}

test('flipping it PUTs the new value and shows the saved message', async () => {
  const { calls, html, after } = await flip(async () => json(200, { enabled: false }))
  const put = calls.find(c => c.method === 'PUT' && c.url.endsWith('/me/digest'))
  assert.ok(put, `no PUT sent: ${JSON.stringify(calls)}`)
  assert.deepEqual(JSON.parse(put.body), { enabled: false })
  assert.ok(html.includes('Saved'), html)
  assert.equal(after, false)
})

test('a failed save shows an error and puts the box back', async () => {
  const { html, after } = await flip(async () => json(500, { message: 'boom' }))
  assert.ok(html.includes('text-red-400'), `no error shown: ${html}`)
  assert.ok(!html.includes('>Saved<'), `saved shown for a failed PUT: ${html}`)
  assert.equal(after, true, 'box left unchecked although the server still holds on')
})
