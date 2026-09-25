// #198: the payload preview must show one labelled row per field regardless
// of whether the payload nests them under `fields`, and arrays must render as
// readable lists rather than raw JSON. Shared by Inbox.vue and
// SuggestionPanel.vue, so it is tested once here rather than through either
// component.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { payloadFields } from '../src/composables/useSuggestionPayload.js'
import { regionLabel } from '../src/composables/useFormat.js'

const rowsHaveNoObjectValues = (rows) => rows.every((r) => typeof r.value !== 'object')

test('R1: empty, unparseable or non-object input never throws and yields no rows', () => {
  assert.deepEqual(payloadFields(null), [])
  assert.deepEqual(payloadFields(undefined), [])
  assert.deepEqual(payloadFields(''), [])
  assert.deepEqual(payloadFields('not json'), [])
  assert.deepEqual(payloadFields(5), [])
  assert.deepEqual(payloadFields(['a', 'b']), [])
  assert.deepEqual(payloadFields('["a","b"]'), [])
})

test('R2: a top-level primitive renders as before', () => {
  const rows = payloadFields({ severity: 'high' })
  assert.deepEqual(rows, [{ key: 'severity', label: 'Severity', value: 'high' }])
})

test('R3: fields is a plain object — rows with no "fields" prefix in the label', () => {
  const rows = payloadFields({ fields: { notes: 'x', status: 'active' } })
  assert.deepEqual(rows, [
    { key: 'fields.notes', label: 'notes', value: 'x' },
    { key: 'fields.status', label: 'Status', value: 'active' },
  ])
})

test('R4: any other plain-object value expands with a parent-label prefix', () => {
  const rows = payloadFields({ owner: { name: 'Ana' } })
  assert.deepEqual(rows, [{ key: 'owner.name', label: 'owner · Name', value: 'Ana' }])
})

test('R5: nesting deeper than R3/R4 stops recursing and stringifies the inner value', () => {
  const rows1 = payloadFields({ fields: { meta: { a: 1 } } })
  assert.equal(rows1.length, 1)
  assert.equal(rows1[0].key, 'fields.meta')
  assert.equal(rows1[0].value, JSON.stringify({ a: 1 }))

  const rows2 = payloadFields({ owner: { x: { a: 1 } } })
  assert.equal(rows2.length, 1)
  assert.equal(rows2[0].key, 'owner.x')
  assert.equal(rows2[0].value, JSON.stringify({ a: 1 }))
})

test('R6: an array of primitives joins as a comma-separated list', () => {
  const rows = payloadFields({ affected_systems: ['SYS-1', 'SYS-2'] })
  assert.deepEqual(rows, [{ key: 'affected_systems', label: 'affected systems', value: 'SYS-1, SYS-2' }])
})

test('R7: an array of plain objects joins each element\'s values, then joins elements', () => {
  const rows = payloadFields({
    links: [
      { type: 'risk', id: 'RISK-3' },
      { type: 'system', id: 'SYS-1' },
    ],
  })
  assert.deepEqual(rows, [{ key: 'links', label: 'links', value: 'risk RISK-3, system SYS-1' }])
})

test('R7: a link reads "type id" in the key order Postgres JSONB returns (id first)', () => {
  // The payload as the API actually returns it: JSONB re-sorts keys, so
  // `id` precedes `type`. Plain JS literals keep insertion order and would
  // never surface this.
  const stored = '{"links":[{"id":"RISK-1","type":"risk"},{"id":"SYSTEM-1","type":"system"}]}'
  assert.deepEqual(payloadFields(stored), [{ key: 'links', label: 'links', value: 'risk RISK-1, system SYSTEM-1' }])
})

test('R7: an object element that is not a link still joins its values in order', () => {
  assert.deepEqual(payloadFields({ items: [{ a: 'x', b: 'y' }] }), [{ key: 'items', label: 'items', value: 'x y' }])
})

test('R8: empty values are dropped at the leaf, after hoisting', () => {
  assert.deepEqual(payloadFields({ fields: { a: '', b: 'x' } }), [{ key: 'fields.b', label: 'b', value: 'x' }])
  assert.deepEqual(payloadFields({ fields: {} }), [])
  assert.deepEqual(payloadFields({ a: null, b: undefined, c: [], d: {} }), [])
})

test('R9: jurisdiction renders through regionLabel at any level', () => {
  assert.equal(payloadFields({ fields: { jurisdiction: 'DE' } })[0].value, regionLabel('DE'))
  assert.equal(payloadFields({ jurisdiction: 'DE' })[0].value, regionLabel('DE'))
})

test('R10: injected top-level siblings on an update payload are not hidden', () => {
  const rows = payloadFields({ fields: { name: 'New' }, name: 'Title', description: 'Why' })
  assert.deepEqual(rows, [
    { key: 'fields.name', label: 'Name', value: 'New' },
    { key: 'name', label: 'Name', value: 'Title' },
    { key: 'description', label: 'Description', value: 'Why' },
  ])
  const keys = rows.map((r) => r.key)
  assert.equal(new Set(keys).size, keys.length)
})

test('the issue\'s exact reproduction: a JSON-string fields payload renders one plain-text row', () => {
  const raw = '{"fields":{"notes":"Fleet size corrected to 54 MacBooks (was 52)"}}'
  const rows = payloadFields(raw)
  assert.equal(rows.length, 1)
  assert.equal(rows[0].key, 'fields.notes')
  assert.equal(rows[0].value, 'Fleet size corrected to 54 MacBooks (was 52)')
  assert.equal(rows[0].value.includes('{'), false)
})

test('no row value is ever an object or array, across every rule', () => {
  assert.ok(rowsHaveNoObjectValues(payloadFields({ severity: 'high' })))
  assert.ok(rowsHaveNoObjectValues(payloadFields({ fields: { notes: 'x', status: 'active' } })))
  assert.ok(rowsHaveNoObjectValues(payloadFields({ owner: { name: 'Ana' } })))
  assert.ok(rowsHaveNoObjectValues(payloadFields({ fields: { meta: { a: 1 } } })))
  assert.ok(rowsHaveNoObjectValues(payloadFields({ owner: { x: { a: 1 } } })))
  assert.ok(rowsHaveNoObjectValues(payloadFields({ affected_systems: ['SYS-1', 'SYS-2'] })))
  assert.ok(rowsHaveNoObjectValues(payloadFields({ links: [{ type: 'risk', id: 'RISK-3' }] })))
  assert.ok(rowsHaveNoObjectValues(payloadFields({ fields: { name: 'New' }, name: 'Title', description: 'Why' })))
})

test('the panel and the inbox both still call payloadFields(sg.payload)', () => {
  const here = fileURLToPath(import.meta.url)
  const webRoot = new URL('../', `file://${here}`)
  const panel = readFileSync(new URL('src/components/SuggestionPanel.vue', webRoot), 'utf8')
  const inbox = readFileSync(new URL('src/views/Inbox.vue', webRoot), 'utf8')
  assert.ok(panel.includes('payloadFields(sg.payload)'))
  assert.ok(inbox.includes('payloadFields(sg.payload)'))
})
