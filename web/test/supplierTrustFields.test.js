// The Suppliers register carries the four public fields (#522). The view cannot
// be mounted under node --test (it pulls half the app), so this reads the source
// the way errorRender.test.js does: the edit form sends the whole form on save,
// so the fields must be in startEdit() (an untouched form echoes the stored
// values, which the server treats as no change) and in the form with the
// server's length limits.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const src = readFileSync(new URL('../src/views/Suppliers.vue', import.meta.url), 'utf8')

test('startEdit loads the four fields from the supplier, so an untouched save echoes them', () => {
  const start = src.slice(src.indexOf('function startEdit('), src.indexOf('function editSection('))
  for (const k of ['public_name', 'public_purpose', 'processing_location', 'data_categories']) {
    assert.ok(start.includes(`${k}: item.${k} || ''`), `startEdit must load ${k}`)
  }
})

test('the edit form has the four fields with the server limits, and the detail panel shows them with an em dash', () => {
  const limits = { public_name: 200, public_purpose: 1000, processing_location: 200, data_categories: 1000 }
  for (const [k, n] of Object.entries(limits)) {
    const re = new RegExp(`v-model="editForm\\.${k}" maxlength="${n}"`)
    assert.match(src, re, `${k} needs maxlength ${n}`)
    assert.ok(src.includes(`selectedItem.${k} || '—'`), `${k} must show in the detail panel`)
  }
  assert.ok(src.includes("t('suppliers.trust.heading')") && src.includes("t('suppliers.trust.help')"))
})
