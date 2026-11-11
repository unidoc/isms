// Pure helpers behind the trust center manager screen (#520): the order
// comparator shared with the public page, status and action mapping per row,
// the public address, the settings diff and the classification confirm flow.
import test from 'node:test'
import assert from 'node:assert/strict'
import { compareTrustItems } from '../src/utils/trustOrder.js'
import { rowStatus, rowActions, publicAddress, changedSettings } from '../src/utils/trustManager.js'
import { withClassificationConfirm } from '../src/composables/useClassificationConfirm.js'

test('comparator: sort_order first, then title; equal sort orders fall back to the title', () => {
  const items = [
    { sort_order: 2, title: 'Alpha' }, { sort_order: 1, title: 'Zulu' },
    { sort_order: 0, title: 'Mike' }, { sort_order: 0, title: 'Bravo' },
  ]
  assert.deepEqual([...items].sort(compareTrustItems).map(i => i.title), ['Bravo', 'Mike', 'Zulu', 'Alpha'])
})

test('comparator: a title getter lets manager rows use document.title', () => {
  const rows = [
    { sort_order: 0, document: { title: 'b' } }, { sort_order: 0, document: { title: 'a' } },
  ]
  const cmp = (x, y) => compareTrustItems(x, y, r => r.document.title)
  assert.deepEqual(rows.sort(cmp).map(r => r.document.title), ['a', 'b'])
})

const row = (over = {}, doc = {}) => ({
  id: 'i1', kind: 'document', access_level: 'gated', sort_order: 1, hidden_reason: '',
  document: {
    document_id: 'pol-1', title: 'Policy', classification: '', status: 'published',
    document_status: 'approved', document_version: '1.0',
    served_version: { version: '1.0', approved_at: 1, content_hash: 'h' }, ...doc,
  },
  ...over,
})

test('rowStatus: every status the server can send maps to a label key', () => {
  assert.deepEqual(rowStatus(row()), { key: 'published', params: {} })
  assert.deepEqual(
    rowStatus(row({}, { status: 'draft_in_progress', document_version: '2.0' })),
    { key: 'draft_in_progress', params: { draft: '2.0', version: '1.0' } })
  assert.deepEqual(rowStatus(row({}, { status: 'cannot_publish', served_version: null })), { key: 'cannot_publish', params: {} })
  assert.deepEqual(rowStatus(row({ hidden_reason: 'retired' }, { status: 'hidden' })), { key: 'hidden_retired', params: {} })
  assert.deepEqual(rowStatus(row({ hidden_reason: 'deleted' }, { status: 'hidden' })), { key: 'hidden_deleted', params: {} })
  assert.deepEqual(rowStatus(row({}, { status: 'error' })), { key: 'error', params: {} })
})

test('rowActions: visible rows unpublish; hidden ones depend on why they are hidden', () => {
  assert.deepEqual(rowActions(row()), { unpublish: true, publishAgain: null, remove: false })
  // retired and still retired at HEAD: Publish again is shown but disabled with a reason.
  assert.deepEqual(rowActions(row({ hidden_reason: 'retired' }, { status: 'hidden', document_status: 'retired' })),
    { unpublish: false, publishAgain: 'disabled_retired', remove: false })
  // retired earlier, restored since: enabled.
  assert.deepEqual(rowActions(row({ hidden_reason: 'retired' }, { status: 'hidden', document_status: 'approved' })),
    { unpublish: false, publishAgain: 'enabled', remove: false })
  // deleted: only Remove.
  assert.deepEqual(rowActions(row({ hidden_reason: 'deleted' }, { status: 'hidden', document_status: '' })),
    { unpublish: false, publishAgain: null, remove: true })
  // error and cannot_publish rows are still visible rows.
  assert.equal(rowActions(row({}, { status: 'error' })).unpublish, true)
  assert.equal(rowActions(row({}, { status: 'cannot_publish' })).unpublish, true)
})

test('publicAddress: path mode carries the slug, a tenant host does not', () => {
  assert.equal(publicAddress('https://isms.example', 'acme', false), 'https://isms.example/acme/trust')
  assert.equal(publicAddress('https://acme.isms.example', 'acme', true), 'https://acme.isms.example/trust')
})

test('changedSettings sends only what changed; a cleared overview is null', () => {
  const orig = { enabled: false, security_contact: 'a@x.test', default_access_level: 'gated', overview_document_id: 'ov-1' }
  assert.deepEqual(changedSettings(orig, { ...orig }), {})
  assert.deepEqual(changedSettings(orig, { ...orig, enabled: true }), { enabled: true })
  assert.deepEqual(changedSettings(orig, { ...orig, security_contact: 'b@x.test', default_access_level: 'public' }),
    { security_contact: 'b@x.test', default_access_level: 'public' })
  assert.deepEqual(changedSettings(orig, { ...orig, overview_document_id: '' }), { overview_document_id: null })
  assert.deepEqual(changedSettings(orig, { ...orig, overview_document_id: null }), { overview_document_id: null })
  assert.deepEqual(changedSettings({ ...orig, overview_document_id: null }, { ...orig, overview_document_id: '' }), {})
  assert.deepEqual(changedSettings(orig, { ...orig, overview_document_id: 'ov-2' }), { overview_document_id: 'ov-2' })
})

const conflict = (classification) => Object.assign(new Error('confirm'), { status: 409, code: 'trust_classification_confirm', params: { value: classification } })

test('confirm flow: a 409 asks, and on yes resends the same request with the flag', async () => {
  const sent = []
  const asked = []
  const out = await withClassificationConfirm(
    async (confirm) => { sent.push(confirm); if (!confirm) throw conflict('confidential'); return { ok: true } },
    async (cls) => { asked.push(cls); return true })
  assert.deepEqual(sent, [false, true])
  assert.deepEqual(asked, ['confidential'])
  assert.deepEqual(out, { cancelled: false, result: { ok: true } })
})

test('confirm flow: cancel sends nothing more', async () => {
  const sent = []
  const out = await withClassificationConfirm(
    async (confirm) => { sent.push(confirm); throw conflict('restricted') },
    async () => false)
  assert.deepEqual(sent, [false])
  assert.deepEqual(out, { cancelled: true, result: undefined })
})

test('confirm flow: no conflict never asks; other errors pass through', async () => {
  let asked = 0
  const ok = await withClassificationConfirm(async () => 7, async () => { asked++; return true })
  assert.equal(ok.result, 7)
  assert.equal(asked, 0)
  await assert.rejects(withClassificationConfirm(async () => { throw Object.assign(new Error('x'), { status: 409, code: 'trust_already_published' }) }, async () => true), /x/)
  // a second 409 after the flag was sent is not asked again
  await assert.rejects(withClassificationConfirm(async () => { throw conflict('confidential') }, async () => true), /confirm/)
})
