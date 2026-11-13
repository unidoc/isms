// Pure helpers of the trust center certifications UI (#521).
process.env.TZ = 'America/Los_Angeles' // west of UTC: where a local-time date parse shifts a day
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  CERT_TYPES, MAX_FILE_BYTES, blankForm, changedFields, dispositionFilename, fileProblem, formatBytes,
  formatDayUTC, formFromCert, hostOf, linkBody, moveIds, orderCertifications, safeHref, uploadFields, validateForm,
} from '../src/utils/trustCertifications.js'
import { stateClass } from '../src/utils/expiryBadge.js'

test('the type list has no contract, and the badge colours are the external links ones', () => {
  assert.deepEqual(CERT_TYPES, ['certificate', 'audit_report', 'penetration_test', 'attestation', 'other'])
  assert.equal(stateClass('expired'), 'bg-red-900/40 text-red-300')
  assert.equal(stateClass('expiring'), 'bg-amber-900/40 text-amber-300')
  assert.equal(stateClass('current'), 'bg-emerald-900/40 text-emerald-300')
})

test('a YYYY-MM-DD day is formatted as that day even in a time zone west of UTC', () => {
  // The trap this pins: parsing the string as a local-time date shows the day before.
  assert.equal(new Date('2026-11-13').getDate(), 12, 'precondition: TZ=America/Los_Angeles shifts a naive parse')
  assert.equal(formatDayUTC('2026-11-13', 'en'), 'Nov 13, 2026')
  assert.equal(formatDayUTC('2027-01-01', 'en'), 'Jan 1, 2027')
  assert.equal(formatDayUTC('2026-03-08', 'de-DE'), '08.03.2026')
  assert.equal(formatDayUTC('', 'en'), '')
  assert.equal(formatDayUTC('not a date', 'en'), 'not a date')
})

test('file sizes read humanly', () => {
  assert.equal(formatBytes(0), '0 B')
  assert.equal(formatBytes(512), '512 B')
  assert.equal(formatBytes(1536), '1.5 KB')
  assert.equal(formatBytes(300 * 1024), '300 KB')
  assert.equal(formatBytes(5 * 1024 * 1024), '5.0 MB')
  assert.equal(formatBytes(40 * 1024 * 1024), '40 MB')
  assert.equal(formatBytes(-1), '')
})

test('only http(s) addresses become links; the host is shown', () => {
  assert.equal(safeHref('https://example.com/a'), 'https://example.com/a')
  assert.equal(safeHref(' http://example.com '), 'http://example.com')
  for (const bad of ['javascript:alert(1)', 'ftp://x/y', '//example.com', '', null, undefined]) assert.equal(safeHref(bad), null, String(bad))
  assert.equal(hostOf('https://trust.example.com:8443/p'), 'trust.example.com:8443')
  assert.equal(hostOf('nope'), '')
})

test('the download name is read from the Content-Disposition header', () => {
  assert.equal(dispositionFilename('attachment; filename=report.pdf'), 'report.pdf')
  assert.equal(dispositionFilename('attachment; filename="my report.pdf"'), 'my report.pdf')
  assert.equal(dispositionFilename("attachment; filename*=utf-8''na%C3%AFve%20%C3%A9.pdf"), 'naïve é.pdf')
  assert.equal(dispositionFilename(''), '')
})

const cert = (id, label, itemId, order, over = {}) => ({ id, label, source: 'file', item: itemId ? { id: itemId, sort_order: order, access_level: 'gated' } : null, ...over })

test('published rows follow sort_order then label; unpublished ones come after, by label', () => {
  const certs = [cert(1, 'Zed', null), cert(2, 'Beta', 'i2', 2), cert(3, 'Alpha', 'i3', 2), cert(4, 'Amy', null), cert(5, 'Top', 'i5', 1)]
  const { published, unpublished } = orderCertifications(certs)
  assert.deepEqual(published.map((c) => c.label), ['Top', 'Alpha', 'Beta'])
  assert.deepEqual(unpublished.map((c) => c.label), ['Amy', 'Zed'])
  // sort_order can come from the items list instead (refreshed by every reorder)
  const swapped = orderCertifications(certs, (c) => ({ i2: 1, i3: 2, i5: 3 })[c.item?.id])
  assert.deepEqual(swapped.published.map((c) => c.label), ['Beta', 'Alpha', 'Top'])
})

test('moving sends every certification item id; an item without a certification keeps its slot', () => {
  const it = (id, order, label) => ({ id, kind: 'certification', sort_order: order, certification: label ? { label } : null })
  const items = [it('a', 1, 'A'), it('orphan', 2, null), it('b', 3, 'B'), it('c', 4, 'C'), { id: 'doc', kind: 'document', sort_order: 1, certification: null }]
  const known = new Set(['a', 'b', 'c'])
  // b up: nearest known row above is a (orphan skipped), orphan stays where it was
  assert.deepEqual(moveIds(items, known, 'b', -1), ['b', 'orphan', 'a', 'c'])
  assert.deepEqual(moveIds(items, known, 'a', 1), ['b', 'orphan', 'a', 'c'])
  assert.deepEqual(moveIds(items, known, 'c', -1), ['a', 'orphan', 'c', 'b'])
  assert.equal(moveIds(items, known, 'a', -1), null, 'first row cannot move up')
  assert.equal(moveIds(items, known, 'c', 1), null, 'last row cannot move down')
  assert.equal(moveIds(items, known, 'nope', 1), null)
  assert.ok(!moveIds(items, known, 'b', -1).includes('doc'), 'only certification items')
})

const full = cert(7, 'ISO 27001', 'i7', 1, {
  link_type: 'certificate', issuer: 'BSI', valid_from: '2026-01-01', valid_until: '2027-01-01', verified_at: '2026-02-02', url: '',
})
test('the edit diff holds only changed fields, and an emptied date is null', () => {
  const form = formFromCert(full)
  assert.deepEqual(changedFields(full, form), {})
  assert.deepEqual(changedFields(full, { ...form, issuer: 'TUV' }), { issuer: 'TUV' })
  assert.deepEqual(changedFields(full, { ...form, valid_until: '' }), { valid_until: null })
  assert.deepEqual(changedFields(full, { ...form, valid_from: '', verified_at: '', label: ' New ' }), { label: 'New', valid_from: null, verified_at: null })
  assert.ok(!('url' in changedFields(full, { ...form, url: 'https://x.example' })), 'a file certification takes no url')
  const link = { ...full, source: 'link', url: 'https://a.example' }
  assert.deepEqual(changedFields(link, { ...formFromCert(link), url: ' https://b.example ' }), { url: 'https://b.example' })
  for (const body of [changedFields(full, { ...form, valid_until: '' })]) assert.ok(!Object.values(body).includes(''), 'never ""')
})

test('file prechecks', () => {
  const f = (name, size, type = '') => ({ name, size, type })
  assert.equal(fileProblem(null), 'file_required')
  assert.equal(fileProblem(f('a.pdf', MAX_FILE_BYTES + 1)), 'file_too_large')
  assert.equal(fileProblem(f('a.pdf', MAX_FILE_BYTES)), '')
  assert.equal(fileProblem(f('a.html', 10, 'text/html')), 'file_type')
  assert.equal(fileProblem(f('a.svg', 10, 'image/svg+xml')), 'file_type')
  assert.equal(fileProblem(f('scan.JPEG', 10)), '')
  assert.equal(fileProblem(f('noext', 10, 'application/pdf')), '', 'the server reads the bytes; a PDF type is enough')
})

test('add-form validation', () => {
  const ok = { ...blankForm(), label: 'L', link_type: 'other' }
  const file = { name: 'a.pdf', size: 10, type: 'application/pdf' }
  assert.deepEqual(validateForm(ok, { mode: 'add', file }), {})
  assert.deepEqual(validateForm({ ...ok, label: '  ' }, { file }), { label: 'label_required' })
  assert.deepEqual(validateForm({ ...ok, label: 'x'.repeat(201) }, { file }), { label: 'label_too_long' })
  assert.deepEqual(validateForm({ ...ok, label: 'é'.repeat(200) }, { file }), {}, 'characters, not bytes')
  assert.deepEqual(validateForm({ ...ok, link_type: 'contract' }, { file }), { link_type: 'type_required' })
  assert.deepEqual(validateForm({ ...ok, link_type: '' }, { file }), { link_type: 'type_required' })
  assert.deepEqual(validateForm(ok, {}), { file: 'file_required' })
  assert.deepEqual(validateForm(ok, { file: { name: 'a.pdf', size: MAX_FILE_BYTES + 1 } }), { file: 'file_too_large' })
  assert.deepEqual(validateForm(ok, { file: { name: 'a.exe', size: 1 } }), { file: 'file_type' })
  assert.deepEqual(validateForm({ ...ok, valid_from: '2027-01-02', valid_until: '2027-01-01' }, { file }), { valid_until: 'dates_order' })
  assert.deepEqual(validateForm({ ...ok, valid_from: '2027-01-01', valid_until: '2027-01-01' }, { file }), {})
  const link = { ...ok, source: 'link' }
  assert.deepEqual(validateForm(link, {}), { url: 'url_required' })
  assert.deepEqual(validateForm({ ...link, url: 'ftp://x/y' }, {}), { url: 'url_invalid' })
  assert.deepEqual(validateForm({ ...link, url: 'javascript:alert(1)' }, {}), { url: 'url_invalid' })
  assert.deepEqual(validateForm({ ...link, url: 'https://e.com' }, {}), {})
  // edit mode: url only for link certifications, no file needed
  assert.deepEqual(validateForm({ ...ok, url: '' }, { mode: 'edit', cert: full }), {})
  assert.deepEqual(validateForm({ ...ok, url: '' }, { mode: 'edit', cert: { ...full, source: 'link' } }), { url: 'url_required' })
})

test('upload fields leave out empty dates and send publish only as the text "true"', () => {
  const f = { ...blankForm(), label: ' L ', link_type: 'certificate', issuer: ' BSI ' }
  assert.deepEqual(uploadFields(f), { label: 'L', link_type: 'certificate', issuer: 'BSI' })
  assert.deepEqual(uploadFields({ ...f, issuer: '', valid_until: '2027-01-01', publish: true, access_level: 'public' }),
    { label: 'L', link_type: 'certificate', valid_until: '2027-01-01', publish: 'true', access_level: 'public' })
  assert.ok(!('publish' in uploadFields({ ...f, publish: false })))
  assert.deepEqual(linkBody({ ...f, source: 'link', url: ' https://e.com ', verified_at: '2026-05-05' }),
    { label: 'L', link_type: 'certificate', url: 'https://e.com', issuer: 'BSI', verified_at: '2026-05-05' })
  assert.equal(linkBody({ ...f, url: 'https://e.com', publish: true, access_level: 'gated' }).publish, true)
})
