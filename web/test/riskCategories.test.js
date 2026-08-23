import test from 'node:test'
import assert from 'node:assert/strict'
import {
  DEFAULT_RISK_CATEGORIES,
  categoryLabel,
  deslugCategory,
  slugifyCategory,
} from '../src/riskCategories.js'

const CONFIGURED = [
  { key: 'cloud_saas', label: 'Cloud / SaaS' },
  { key: 'supply_chain', label: 'Supply Chain' },
]

test('shows the configured label, not the de-slugged key', () => {
  // Reported in review on PR #215: the fetched labels fed the pickers only, so a
  // risk row rendered "cloud saas" instead of the admin's "Cloud / SaaS".
  assert.equal(categoryLabel('cloud_saas', CONFIGURED), 'Cloud / SaaS')
  assert.equal(categoryLabel('supply_chain', CONFIGURED), 'Supply Chain')
})

test('falls back to the de-slugged key for an orphaned category', () => {
  // The org removed this category; risks still hold it and must still display.
  assert.equal(categoryLabel('removed_key', CONFIGURED), 'removed key')
})

test('renders nothing for an empty or missing category', () => {
  assert.equal(categoryLabel('', CONFIGURED), '')
  assert.equal(categoryLabel(null, CONFIGURED), '')
  assert.equal(categoryLabel(undefined, CONFIGURED), '')
})

test('survives a missing or empty category list', () => {
  assert.equal(categoryLabel('cloud_saas', []), 'cloud saas')
  assert.equal(categoryLabel('cloud_saas', undefined), 'cloud saas')
})

test('ignores a definition with a blank label', () => {
  assert.equal(categoryLabel('x', [{ key: 'x', label: '' }]), 'x')
})

test('matches keys exactly — no case folding', () => {
  assert.equal(categoryLabel('Cloud_SaaS', CONFIGURED), 'Cloud SaaS')
})

test('deslugCategory replaces underscores', () => {
  assert.equal(deslugCategory('people_process'), 'people process')
  assert.equal(deslugCategory(''), '')
})

test('slugifyCategory produces a server-valid key', () => {
  const rule = /^[a-z0-9]+(_[a-z0-9]+)*$/
  for (const [label, want] of [
    ['Cloud / SaaS', 'cloud_saas'],
    ['Insider Threat & Fraud', 'insider_threat_fraud'],
    ['  Padded  Label  ', 'padded_label'],
    ['Third-Party', 'third_party'],
    ['ISO 27001', 'iso_27001'],
  ]) {
    assert.equal(slugifyCategory(label), want)
    assert.match(slugifyCategory(label), rule)
  }
})

test('slugifyCategory yields an empty string when nothing usable remains', () => {
  // The caller must reject this rather than send an invalid key.
  assert.equal(slugifyCategory('!!!'), '')
  assert.equal(slugifyCategory('   '), '')
})

test('every built-in default is a valid key with a non-empty label', () => {
  const rule = /^[a-z0-9]+(_[a-z0-9]+)*$/
  assert.equal(DEFAULT_RISK_CATEGORIES.length, 8)
  for (const c of DEFAULT_RISK_CATEGORIES) {
    assert.match(c.key, rule)
    assert.ok(c.label.trim().length > 0)
  }
})
