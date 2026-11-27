// The click-through NDA on the public trust center page (#525): labels, the
// access rule, the NDA panel, the 409 path and the preview.
process.env.TZ = 'UTC'
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: see support/renderSfc.js.
import { compileSfc, renderSfcSettled } from './support/renderSfc.js'

const Page = await compileSfc(new URL('../src/components/TrustCenterPage.vue', import.meta.url).pathname, { stubChildren: true })

const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}
const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

const GD = '11111111-1111-4111-8111-111111111111' // gated document
const PD = '22222222-2222-4222-8222-222222222222' // public document
const GC = '33333333-3333-4333-8333-333333333333' // gated certification file
const VISITOR = { email: 'ada@auditco.test', name: 'Ada', company: 'Audit Co' }
const hash = c => c.repeat(64)
const NF = { message: 'item not found', code: 'not_found' }

// A stateful stand-in for the server: `status` is where the visitor stands with
// the NDA, `hash` the current NDA version.
function stub({ signedIn = true, status = 'required', ndaRequired = true, hashNow = hash('a') } = {}) {
  const st = { calls: [], signedIn, status, hashNow, ndaRequired, text: 'Keep it secret.', onDocument: null }
  const open = () => st.signedIn && ['none', 'accepted', 'skipped'].includes(st.status)
  const nda = () => ({ version: '2.0', approved_at: 1760000000, content_hash: st.hashNow, title: 'Mutual NDA', html: `<p>${st.text}</p>`, company: VISITOR.company })
  globalThis.fetch = async (url, opts = {}) => {
    const u = String(url)
    const method = opts.method || 'GET'
    const body = opts.body ? JSON.parse(opts.body) : undefined
    st.calls.push({ method, url: u, body })
    if (u.endsWith('/api/v1/config')) return json(200, { locales: [{ tag: 'en', name: 'English' }] })
    if (u.endsWith('/api/v1/trust/acme')) return json(200, { title: '', security_contact: 'sec@acme.test', branding_name: 'Acme', branding_color: '', default_locale: 'en', show_powered_by: false, overview: null, visitor_sign_in: true, nda_required: st.ndaRequired })
    if (u.endsWith('/api/v1/trust/acme/items')) {
      const cert = { id: GC, kind: 'certification', section: '', sort_order: 1, access_level: 'gated', title: 'SOC 2 report', link_type: 'audit_report', issuer: 'X', source: 'file', state: 'current' }
      if (open()) Object.assign(cert, { file_name: 'soc2.pdf', file_size: 10 })
      return json(200, [
        { id: GD, kind: 'document', section: '', sort_order: 1, access_level: 'gated', title: 'Gated Policy', version: '1.0', approved_at: 1760000000, content_hash: hash('b') },
        { id: PD, kind: 'document', section: '', sort_order: 2, access_level: 'public', title: 'Public Policy', version: '1.0', approved_at: 1760000000, content_hash: hash('c') },
        cert,
      ])
    }
    if (u.endsWith('/api/v1/trust/acme/visitor/sign-out')) { st.signedIn = false; return json(200, { visitor: null, nda: null }) }
    if (u.endsWith('/api/v1/trust/acme/visitor')) return json(200, st.signedIn ? { visitor: VISITOR, nda: { status: st.status } } : { visitor: null, nda: null })
    if (u.endsWith('/api/v1/trust/acme/nda/accept')) {
      if (!st.signedIn) return json(404, NF)
      if (body.content_hash !== st.hashNow) return json(409, { message: 'changed', code: 'trust_nda_outdated', nda: nda() })
      st.status = 'accepted'
      return json(200, { visitor: VISITOR, nda: { status: 'accepted', version: '2.0', content_hash: st.hashNow } })
    }
    if (u.endsWith('/api/v1/trust/acme/nda')) return st.signedIn && st.status === 'required' ? json(200, nda()) : json(404, NF)
    const m = u.match(/\/api\/v1\/trust\/acme\/documents\/(.+)$/)
    if (m) {
      if (st.onDocument) st.onDocument()
      if (m[1] === PD || (m[1] === GD && open())) return json(200, { id: m[1], title: 'Doc', version: '1.0', approved_at: 1760000000, content_hash: hash('b'), html: '<p>BODY</p>' })
      return json(404, NF)
    }
    return json(404, {})
  }
  return st
}

const props = (extra = {}) => ({ slug: 'acme', basePath: '/acme/trust', ...extra })
const ready = h => h.includes('data-trust="page"') || h.includes('data-trust="error"')
const mount = (p, interact) => renderSfcSettled(Page, p, ready, interact)
const norm = h => h.replaceAll(/<!--[\s\S]*?-->/g, '').replaceAll(/\s+/g, ' ')
const dom = html => { const d = document.createElement('div'); d.innerHTML = html; return d }
const flush = () => new Promise(r => setTimeout(r, 40))
const tick = (host, sel) => { const el = host.querySelector(sel); el.checked = true; el.dispatchEvent(new window.Event('change', { bubbles: true })) }
const nCalls = (st, re, method) => st.calls.filter(c => re.test(c.url) && (!method || c.method === method)).length

test('gated reads "Sign in + NDA" when an NDA is required (access column and filter chip), "Sign in" otherwise', async () => {
  stub({ signedIn: false, ndaRequired: true })
  const withNda = dom(await mount(props()))
  assert.ok(withNda.querySelector('[data-trust="row"]').textContent.includes('Sign in + NDA'))
  assert.equal(withNda.querySelector('[data-trust-filter="gated"]').textContent.trim(), 'Sign in + NDA')
  assert.ok(withNda.querySelector('[data-trust="cert-sign-in"]').textContent.includes('Sign in + NDA'))
  stub({ signedIn: false, ndaRequired: false })
  const without = norm(await mount(props()))
  assert.ok(!without.includes('NDA'), without)
  assert.ok(without.includes('Sign in to view') && without.includes('>Sign in<'), without)
})

test('mayRead per NDA status: none, accepted and skipped read; required asks to accept; unavailable is inert', async () => {
  for (const status of ['none', 'accepted', 'skipped']) {
    stub({ status })
    const host = dom(await mount(props()))
    assert.ok(host.querySelector(`[data-trust="view"][href="/acme/trust/documents/${GD}"]`), `${status}: no View link`)
    assert.ok(host.querySelector('[data-trust="cert-download"]'), `${status}: no download`)
    assert.equal(host.querySelector('[data-trust="nda-panel"]'), null, `${status}: panel shown`)
  }
  stub({ status: 'required' })
  const req = dom(await mount(props()))
  assert.ok(!req.querySelector(`[data-trust="view"][href="/acme/trust/documents/${GD}"]`), 'required: View offered')
  assert.ok(req.querySelector('[data-trust="accept-nda"]').textContent.includes('Accept NDA to view'))
  assert.ok(req.querySelector('[data-trust="cert-accept-nda"]'))
  assert.ok(req.querySelector(`[data-trust="view"][href="/acme/trust/documents/${PD}"]`), 'public stays readable')

  stub({ status: 'unavailable' })
  const un = dom(await mount(props()))
  assert.equal(un.querySelector('[data-trust="accept-nda"]'), null)
  assert.equal(un.querySelector('[data-trust="nda-panel"]'), null)
  assert.ok(un.querySelector('[data-trust="nda-unavailable"]').textContent.includes('Not available right now'))
  assert.equal(un.querySelector('[data-trust="nda-unavailable"]').tagName, 'SPAN', 'inert')
  assert.ok(un.querySelector('[data-trust="nda-unavailable-note"]'))
  assert.ok(!un.querySelector(`[data-trust="view"][href="/acme/trust/documents/${GD}"]`))
})

test('a visitor who has not accepted is shown the NDA panel; Accept stays disabled until the box is ticked', async () => {
  const st = stub({ status: 'required' })
  let before, after
  await mount(props(), async (host, settle) => {
    await settle(h => h.includes('data-trust="nda-body"'))
    const p = host.querySelector('[data-trust="nda-panel"]')
    assert.ok(p, 'panel')
    assert.equal(host.querySelector('[data-trust="nda-title"]').textContent.trim(), 'Mutual NDA')
    assert.equal(host.querySelector('[data-trust="nda-version"]').textContent.trim(), 'Version 2.0')
    assert.ok(host.querySelector('[data-trust="nda-body"]').innerHTML.includes('Keep it secret.'))
    assert.ok(p.textContent.includes('I accept this NDA on behalf of Audit Co'))
    before = host.querySelector('[data-trust="nda-accept"]').disabled
    tick(host, '[data-trust="nda-check"]')
    await flush()
    after = host.querySelector('[data-trust="nda-accept"]').disabled
  })
  assert.equal(before, true)
  assert.equal(after, false)
  assert.equal(nCalls(st, /\/nda$/, 'GET'), 1)
})

test('accepting sends the hash, closes the panel and reloads the items so the file appears', async () => {
  const st = stub({ status: 'required' })
  let itemsBefore, host2
  await mount(props(), async (host, settle) => {
    await settle(h => h.includes('data-trust="nda-body"'))
    itemsBefore = nCalls(st, /\/items$/)
    tick(host, '[data-trust="nda-check"]')
    await flush()
    host.querySelector('[data-trust="nda-accept"]').click()
    await settle(h => !h.includes('data-trust="nda-panel"'))
    await flush()
    host2 = dom(host.innerHTML)
  })
  const post = st.calls.find(c => c.method === 'POST' && c.url.endsWith('/nda/accept'))
  assert.deepEqual(post.body, { content_hash: hash('a') })
  assert.equal(host2.querySelector('[data-trust="nda-panel"]'), null)
  assert.ok(nCalls(st, /\/items$/) > itemsBefore, 'items reloaded')
  assert.ok(host2.querySelector('[data-trust="cert-download"]'), 'the gated file link appeared')
  assert.ok(host2.querySelector(`[data-trust="view"][href="/acme/trust/documents/${GD}"]`))
})

test('a 409 replaces the shown NDA, unticks the box and asks again; the new version then goes through', async () => {
  const st = stub({ status: 'required' })
  let changed, ticked, newText, disabledAfter, doneHtml
  await mount(props(), async (host, settle) => {
    await settle(h => h.includes('data-trust="nda-body"'))
    // a newer version is approved while the text is open
    st.hashNow = hash('d')
    st.text = 'The brand new text.'
    tick(host, '[data-trust="nda-check"]')
    await flush()
    host.querySelector('[data-trust="nda-accept"]').click()
    await settle(h => h.includes('data-trust="nda-changed"'))
    changed = host.querySelector('[data-trust="nda-changed"]').textContent.trim()
    ticked = host.querySelector('[data-trust="nda-check"]').checked
    newText = host.querySelector('[data-trust="nda-body"]').textContent
    disabledAfter = host.querySelector('[data-trust="nda-accept"]').disabled
    tick(host, '[data-trust="nda-check"]')
    await flush()
    host.querySelector('[data-trust="nda-accept"]').click()
    await settle(h => !h.includes('data-trust="nda-panel"'))
    doneHtml = host.innerHTML
  })
  assert.equal(changed, 'The NDA has changed. Read the new version and accept it again.')
  assert.equal(ticked, false)
  assert.ok(newText.includes('The brand new text.'))
  assert.equal(disabledAfter, true)
  const posts = st.calls.filter(c => c.method === 'POST' && c.url.endsWith('/nda/accept'))
  assert.deepEqual(posts.map(p => p.body.content_hash), [hash('a'), hash('d')])
  assert.ok(!doneHtml.includes('data-trust="nda-panel"'))
})

test('Cancel closes the panel and the row offers Accept NDA to view again', async () => {
  stub({ status: 'required' })
  const html = await mount(props(), async (host, settle) => {
    await settle(h => h.includes('data-trust="nda-body"'))
    host.querySelector('[data-trust="nda-cancel"]').click()
    await settle(h => !h.includes('data-trust="nda-panel"'))
    host.querySelector('[data-trust="accept-nda"]').click()
    await settle(h => h.includes('data-trust="nda-body"'))
  })
  assert.ok(html.includes('data-trust="nda-panel"'))
})

test('a 404 on a gated document while signed in, when a new NDA version needs accepting, shows the panel and not the signed-out view', async () => {
  const st = stub({ status: 'accepted' })
  // the NDA is re-approved between the page load and the document request
  st.onDocument = () => { st.status = 'required'; st.hashNow = hash('e') }
  const html = norm(await mount(props({ docId: GD }), async (host, settle) => {
    await settle(h => h.includes('data-trust="nda-panel"'))
  }))
  assert.ok(html.includes('data-trust="nda-panel"'), html)
  assert.ok(html.includes('data-trust="locked"') && html.includes('data-trust="visitor"'), 'still signed in')
  assert.ok(!html.includes('data-trust="doc-sign-in"') && !html.includes('doc-unavailable'), html)
})

test('the gated download pre-check shows the NDA panel when a new version was approved while the page was open', async () => {
  const st = stub({ status: 'accepted' })
  let html
  await mount(props(), async (host, settle) => {
    const dl = host.querySelector('[data-trust="cert-download"]')
    assert.ok(dl, 'download offered while accepted')
    st.status = 'required'
    st.hashNow = hash('f')
    dl.dispatchEvent(new window.MouseEvent('click', { bubbles: true, cancelable: true, button: 0 }))
    await settle(h => h.includes('data-trust="nda-panel"'))
    html = norm(host.innerHTML)
  })
  assert.ok(html.includes('data-trust="nda-panel"'), html)
  assert.ok(html.includes('data-trust="visitor"'), 'not the signed-out view')
  assert.ok(!html.includes('data-trust="cert-download"'))
  assert.ok(nCalls(st, /\/visitor$/) >= 2, 'the visitor was asked again')
})

test('a 404 when opening the panel (session ended) drops to the signed-out view', async () => {
  const st = stub({ status: 'required' })
  let html
  await mount(props(), async (host, settle) => {
    await settle(h => h.includes('data-trust="nda-body"'))
    host.querySelector('[data-trust="nda-cancel"]').click()
    await flush()
    st.signedIn = false
    host.querySelector('[data-trust="accept-nda"]').click()
    await settle(h => h.includes('data-trust="sign-in-link"'))
    html = host.innerHTML
  })
  assert.ok(html.includes('data-trust="sign-in-link"') && !html.includes('data-trust="nda-panel"'))
})

test('signing out closes the panel and clears the NDA state', async () => {
  stub({ status: 'required' })
  const html = await mount(props(), async (host, settle) => {
    await settle(h => h.includes('data-trust="nda-body"'))
    host.querySelector('[data-trust="sign-out"]').click()
    await settle(h => h.includes('data-trust="sign-in-link"'))
  })
  assert.ok(!html.includes('data-trust="nda-panel"') && !html.includes('accept-nda'))
  assert.ok(html.includes('Sign in + NDA'))
})

test('the manager preview stays anonymous: labels follow nda_required, no panel, no visitor or NDA request', async () => {
  const st = stub({ status: 'required' })
  const source = {
    overview: () => fetch('/api/v1/trust/acme').then(r => r.json()),
    items: () => fetch('/api/v1/trust/acme/items').then(r => r.json()),
    document: id => fetch(`/api/v1/trust/acme/documents/${id}`).then(r => r.json()),
  }
  st.signedIn = false
  const html = norm(await mount(props({ source })))
  assert.ok(html.includes('Sign in + NDA'))
  assert.ok(!html.includes('data-trust="nda-panel"') && !html.includes('accept-nda'))
  assert.equal(nCalls(st, /\/(visitor|nda)/), 0, 'no visitor or NDA request: ' + JSON.stringify(st.calls.map(c => c.url)))
})
