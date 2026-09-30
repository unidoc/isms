import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://acme.example/inbox' })
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.localStorage = dom.window.localStorage

// The Inbox asks each list endpoint for the viewer's own rows with
// `involving=me`. The wrappers predate that: they took a positional status and
// nothing else, and every other caller still passes just that. This pins both
// halves, since a wrapper that dropped the parameter would quietly bring back
// the org-wide list.
async function urlFor(call) {
  let url = null
  globalThis.fetch = async (u) => {
    url = u
    return { ok: true, status: 200, json: async () => ({ data: [] }) }
  }
  const { api } = await import('../src/api.js')
  await call(api)
  return url
}

const INBOX = { involving: 'me', limit: 200 }

test('the inbox wrappers send involving=me alongside the positional arguments', async () => {
  assert.equal(await urlFor((a) => a.getReviews('', INBOX)), '/api/v1/reviews?involving=me&limit=200')
  assert.equal(await urlFor((a) => a.getChanges('', INBOX)), '/api/v1/changes?involving=me&limit=200')
  assert.equal(await urlFor((a) => a.getAllOpenComments({ involving: 'me' })), '/api/v1/comments/open?involving=me')
  assert.equal(await urlFor((a) => a.getTasks('', '', INBOX)), '/api/v1/tasks?involving=me&limit=200')
  assert.equal(await urlFor((a) => a.getReviews('open', INBOX)), '/api/v1/reviews?status=open&involving=me&limit=200')
})

test('callers that pass no params keep their existing URLs', async () => {
  assert.equal(await urlFor((a) => a.getReviews()), '/api/v1/reviews')
  assert.equal(await urlFor((a) => a.getReviews('merged')), '/api/v1/reviews?status=merged')
  assert.equal(await urlFor((a) => a.getChanges()), '/api/v1/changes')
  assert.equal(await urlFor((a) => a.getChanges('proposed')), '/api/v1/changes?status=proposed')
  assert.equal(await urlFor((a) => a.getAllOpenComments()), '/api/v1/comments/open')
  assert.equal(await urlFor((a) => a.getTasks('a@b.io', 'open')), '/api/v1/tasks?assignee=a%40b.io&status=open')
})
