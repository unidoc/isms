// Export a document as PDF or DOCX from its page.
//
// The token lives in localStorage, so a plain link cannot download: api.js
// fetches the file with the Authorization header and hands back a blob.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://acme.example/documents' })
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.localStorage = dom.window.localStorage

const { api, apiErrorFrom } = await import('../src/api.js')
const { exportFormatsFrom } = await import('../src/composables/exportFormats.js')

function stubFetch(handler) {
  const calls = []
  globalThis.fetch = async (url, opts) => {
    calls.push({ url, opts })
    return handler(url, opts)
  }
  return calls
}

const fileResponse = (cd) => ({
  ok: true,
  status: 200,
  headers: { get: (h) => ({ 'content-disposition': cd, 'content-type': 'application/pdf' })[h.toLowerCase()] ?? null },
  blob: async () => 'BLOB',
})

test('exportDocument encodes the doc id and query, and sends the token', async () => {
  localStorage.setItem('isms_api_token', 'tok-123')
  const calls = stubFetch(() => fileResponse('attachment; filename=pol-1-v1.0.pdf'))
  const out = await api.exportDocument('a b/c', { format: 'docx', versionId: '42' })
  assert.equal(calls.length, 1)
  assert.equal(calls[0].url, '/api/v1/documents/a%20b%2Fc/export?format=docx&version_id=42')
  assert.equal(calls[0].opts.headers.Authorization, 'Bearer tok-123')
  assert.deepEqual(out, { blob: 'BLOB', filename: 'pol-1-v1.0.pdf' })
})

test('exportDocument sends neither version nor version_id for the current copy', async () => {
  for (const versionId of [undefined, null, '']) {
    const calls = stubFetch(() => fileResponse('attachment; filename=x.pdf'))
    await api.exportDocument('pol-1', { format: 'pdf', versionId })
    assert.equal(calls[0].url, '/api/v1/documents/pol-1/export?format=pdf', `versionId=${JSON.stringify(versionId)}`)
  }
})

test('a chosen version is sent by id and never by label', async () => {
  for (const versionId of ['7', 7]) {
    const calls = stubFetch(() => fileResponse('attachment; filename=x.pdf'))
    await api.exportDocument('pol-1', { format: 'pdf', versionId })
    const u = new URL(calls[0].url, 'https://x.example')
    assert.equal(u.searchParams.get('version_id'), '7')
    assert.equal(u.searchParams.has('version'), false, 'the label parameter must not be sent')
  }
})

test('exportDocument filename: quoted, and the <docId>.<format> fallback', async () => {
  stubFetch(() => fileResponse('attachment; filename="my-doc.pdf"'))
  assert.equal((await api.exportDocument('pol-1', { format: 'pdf' })).filename, 'my-doc.pdf')
  stubFetch(() => fileResponse(undefined))
  assert.equal((await api.exportDocument('pol-1', { format: 'docx' })).filename, 'pol-1.docx')
})

test('exportDocument throws the shared error shape on a refusal', async () => {
  stubFetch(() => ({
    ok: false,
    status: 403,
    statusText: 'Forbidden',
    headers: { get: () => 'application/json' },
    json: async () => ({ message: 'PDF export is not available on this server', code: 'export_not_licensed', params: { value: 'PDF' } }),
  }))
  await assert.rejects(api.exportDocument('pol-1', { format: 'pdf' }), (e) => {
    assert.equal(e.status, 403)
    assert.equal(e.code, 'export_not_licensed')
    assert.deepEqual(e.params, { value: 'PDF' })
    assert.equal(e.message, 'PDF export is not available on this server')
    return true
  })
  assert.equal(typeof apiErrorFrom, 'function')
})

test('exportFormatsFrom keeps a list of strings and nothing else', () => {
  assert.deepEqual(exportFormatsFrom({ export_formats: ['pdf'] }), ['pdf'])
  assert.deepEqual(exportFormatsFrom({ export_formats: ['pdf', 'docx'] }), ['pdf', 'docx'])
  assert.deepEqual(exportFormatsFrom({}), [])
  assert.deepEqual(exportFormatsFrom(null), [])
  assert.deepEqual(exportFormatsFrom(undefined), [])
  for (const bad of ['pdf', 7, {}, true, null]) {
    assert.deepEqual(exportFormatsFrom({ export_formats: bad }), [], String(bad))
  }
  assert.deepEqual(exportFormatsFrom({ export_formats: ['pdf', 3, null, 'xls', 'docx'] }), ['pdf', 'docx'])
})

// ---- Documents.vue, as source (no SFC mount precedent for this view) --------

const DOCUMENTS = readFileSync(new URL('../src/views/Documents.vue', import.meta.url), 'utf8')
const buttonTags = (src) => src.match(/<button\b[^>]*>/gs) ?? []

test('the Export button sits before Print, typed, and only shows with a licensed format', () => {
  const exportAt = DOCUMENTS.indexOf('<!-- Export (labelled)')
  const printAt = DOCUMENTS.indexOf('<!-- Print (icon only)')
  assert.ok(exportAt > 0 && printAt > 0, 'markers not found')
  assert.ok(exportAt < printAt, 'Export must come before Print')
  const between = DOCUMENTS.slice(exportAt, printAt)
  const tags = buttonTags(between)
  assert.equal(tags.length, 1, 'expected one button between the markers')
  assert.match(tags[0], /\stype="button"/)
  assert.match(tags[0], /v-if="[^"]*!editMode[^"]*exportFormats\.length/)
  assert.match(between, /t\('documents\.toolbar\.export'\)/)
})

test('every button in the export modal declares a type', () => {
  const start = DOCUMENTS.indexOf('<!-- Export Modal -->')
  assert.ok(start > 0, 'export modal marker not found')
  const end = DOCUMENTS.indexOf('</Teleport>', start)
  const tags = buttonTags(DOCUMENTS.slice(start, end))
  assert.ok(tags.length >= 2, `expected Cancel and Download, found ${tags.length}`)
  for (const t of tags) assert.match(t, /\stype="button"/, `typeless button: ${t}`)
})

// Two versions can share the label "" (a document with no version field), so the
// dialog's option values are the version ids: "" is only ever "Current version".
test('the export dialog uses version ids as option values', () => {
  const m = DOCUMENTS.match(/<select id="export-version"[\s\S]*?<\/select>/)
  assert.ok(m, 'export version select not found')
  const sel = m[0]
  assert.match(sel, /<option value="">\{\{ t\('documents\.export\.current_version'\) \}\}<\/option>/)
  assert.match(sel, /:value="String\(ver\.id\)"/)
  assert.doesNotMatch(sel, /:value="ver\.version"/)
  assert.match(sel, /t\('documents\.export\.unnumbered_version'\)/)
  assert.match(DOCUMENTS, /versionId: exportVersion\.value/)
})
