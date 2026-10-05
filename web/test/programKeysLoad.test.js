// renderMention loads the org's program keys from inside a component's render.
// When that request fails, the failure must not re-render the component into
// another request: the review document view would otherwise keep calling
// GET /programs for as long as the endpoint is down.
import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body><div id="app"></div></body></html>', { url: 'https://acme.example/' })
// Vue's DOM runtime reads these at import time, so they go in before it loads.
for (const key of ['window', 'document', 'localStorage', 'Node', 'Element', 'HTMLElement', 'SVGElement', 'Text', 'Comment', 'DocumentFragment', 'CustomEvent']) {
  globalThis[key] = key === 'window' ? dom.window : dom.window[key]
}

let programCalls = 0
let programsUp = false
globalThis.fetch = async (url) => {
  if (String(url).endsWith('/programs')) programCalls++
  await new Promise(r => setTimeout(r, 5)) // a real request takes a turn of the event loop
  if (programsUp) return { ok: true, status: 200, json: async () => ({ data: [{ key: 'ISMS' }] }) }
  return { ok: false, status: 500, json: async () => ({ message: 'down' }), text: async () => 'down' }
}

const settle = async () => { for (let i = 0; i < 20; i++) await new Promise(r => setTimeout(r, 10)) }

const { createApp, h } = await import('vue')
const { renderMention } = await import('../src/composables/useMention.js')

test('a failed program-key load does not re-render into another request', async () => {
  const app = createApp({ render: () => h('div', { innerHTML: renderMention('see #RISK-1') }) })
  app.mount('#app')
  await settle()
  app.unmount()
  assert.equal(programCalls, 1)
})

test('a later render retries once, and the loaded keys re-render as links', async () => {
  programsUp = true
  programCalls = 0
  const app = createApp({ render: () => h('div', { innerHTML: renderMention('see #ISMS') }) })
  app.mount('#app')
  await settle()
  const html = document.getElementById('app').innerHTML
  app.unmount()
  assert.equal(programCalls, 1)
  assert.match(html, /<a href="\/programs\/ISMS"/)
})
