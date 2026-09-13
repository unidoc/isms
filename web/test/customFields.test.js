import test from 'node:test'
import assert from 'node:assert/strict'
import {
  slugifyFieldKey,
  emptyCustomFieldValues,
  orphanCustomValues,
  deslugFieldKey,
  customFieldInputType,
  requiredCustomFieldsSatisfied,
} from '../src/customFields.js'

const DEFS = [
  { key: 'vendor', label: 'Vendor', type: 'text' },
  { key: 'cost', label: 'Cost', type: 'number' },
  { key: 'severity', label: 'Severity', type: 'select', required: true, options: ['Low', 'High'] },
]

test('slugifyFieldKey matches the server slug rule', () => {
  assert.equal(slugifyFieldKey('Cloud / SaaS'), 'cloud_saas')
  assert.equal(slugifyFieldKey('  Vendor Name  '), 'vendor_name')
})

test('emptyCustomFieldValues seeds one key per def', () => {
  const out = emptyCustomFieldValues(DEFS)
  assert.equal(Object.keys(out).length, 3)
  assert.equal(out.vendor, '')
  assert.equal(out.cost, null)
  assert.equal(out.severity, '')
})

test('orphanCustomValues finds entries with no matching def', () => {
  const values = { vendor: 'Acme', removed_field: 'still here', empty_orphan: '' }
  const out = orphanCustomValues(DEFS, values)
  assert.deepEqual(out, { removed_field: 'still here' })
})

test('orphanCustomValues never treats a defined key as an orphan', () => {
  const values = { vendor: 'Acme', cost: 10, severity: 'Low' }
  const out = orphanCustomValues(DEFS, values)
  assert.deepEqual(out, {})
})

test('deslugFieldKey renders underscores as spaces', () => {
  assert.equal(deslugFieldKey('removed_field'), 'removed field')
  assert.equal(deslugFieldKey(''), '')
})

test('customFieldInputType maps known types', () => {
  assert.equal(customFieldInputType('number'), 'number')
  assert.equal(customFieldInputType('date'), 'date')
  assert.equal(customFieldInputType('text'), 'text')
  assert.equal(customFieldInputType('select'), 'text') // caller uses <select>, not this
})

test('requiredCustomFieldsSatisfied: missing required field blocks', () => {
  assert.equal(requiredCustomFieldsSatisfied(DEFS, { vendor: 'Acme' }), false)
})

test('requiredCustomFieldsSatisfied: empty string required field blocks', () => {
  assert.equal(requiredCustomFieldsSatisfied(DEFS, { severity: '   ' }), false)
})

test('requiredCustomFieldsSatisfied: required field present passes', () => {
  assert.equal(requiredCustomFieldsSatisfied(DEFS, { severity: 'Low' }), true)
})

test('requiredCustomFieldsSatisfied: no required defs always passes', () => {
  assert.equal(requiredCustomFieldsSatisfied([{ key: 'x', type: 'text' }], {}), true)
})
