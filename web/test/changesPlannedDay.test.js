// #503: a change's planned date is edited as a UTC day (toDay/dayToEpoch) so it
// must be shown as that same day, not converted to the viewer's zone, where New
// York read 2026-01-01 as "31 Dec 2025". The real Changes page is mounted with
// the browser zone set west of UTC.
import test from 'node:test'
import assert from 'node:assert/strict'
process.env.TZ = 'America/New_York'
import { compileSfc } from './support/renderSfc.js'

const Changes = await compileSfc(new URL('../src/views/Changes.vue', import.meta.url).pathname, { stubChildren: true })
globalThis.history ??= window.history
globalThis.requestAnimationFrame ??= (fn) => setTimeout(fn, 0)
globalThis.cancelAnimationFrame ??= (id) => clearTimeout(id)
const store = new Map()
globalThis.localStorage = {
  getItem: k => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: k => store.delete(k),
}
const json = (body) => new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })

test('planned_at 1767225600 shows as 1 Jan 2026 under America/New_York', async () => {
  const { createApp, h } = await import('vue')
  const { createRouter, createMemoryHistory } = await import('vue-router')
  const { i18n } = await import('../src/i18n.js')
  const cr = {
    id: 1, identifier: 'CR-1', title: 'Planned change', description: 'd', status: 'proposed', priority: 'medium',
    category: 'other', risk_level: 'low', requested_by: 'a@b.c', created_at: 1767268800, updated_at: 1767268800,
    planned_at: 1767225600,
  }
  globalThis.fetch = async (url) => {
    const u = String(url)
    if (u.endsWith('/me')) return json({ organization_id: 1, role: 'admin', email: 'a@b.c' })
    if (u.includes('/changes/stats')) return json({})
    if (/\/changes\/(CR-1|1)(\?|$)/.test(u)) return json(cr)
    if (/\/changes(\?|$)/.test(u)) return json({ data: [cr], total: 1 })
    return json([])
  }
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { render: () => null } }, { path: '/changes/:id?', component: { render: () => null } }] })
  router.push('/changes/CR-1')
  await router.isReady()
  const app = createApp({ render: () => h(Changes) })
  app.use(router)
  app.use(i18n)
  app.directive('mermaid', {})
  const host = document.createElement('div')
  document.body.appendChild(host)
  app.mount(host)
  for (let i = 0; i < 150; i++) {
    await new Promise(r => setTimeout(r, 10))
    if (host.textContent.includes('Planned change')) break
  }
  await new Promise(r => setTimeout(r, 300))
  const text = document.body.textContent.replace(/\s+/g, ' ')
  await new Promise(r => setTimeout(r, 100))
  app.unmount()
  host.remove()
  assert.ok(text.includes('Planned for'), `detail panel not open: ${text.slice(0, 300)}`)
  assert.ok(!text.includes('Dec 31, 2025') && !text.includes('31 Dec 2025'), `planned date shifted to the viewer's zone: ${text.slice(0, 400)}`)
  assert.ok(/Jan 1, 2026|1 Jan 2026/.test(text), `planned date not shown as 1 Jan 2026: ${text.slice(0, 400)}`)
})
