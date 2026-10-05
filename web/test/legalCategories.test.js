// Cross-language drift guard for the Legal register's category list (#269).
// Legal.vue's CATEGORY_KEYS must equal db.LegalCategories, in order, and every
// key must have an English label. The Go side is checked against the database
// CHECK constraint by internal/isms/db/legal_categories_test.go.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const read = (rel) => readFileSync(fileURLToPath(new URL(rel, import.meta.url)), 'utf8')

const quoted = (body) => [...body.matchAll(/["']([a-z_]+)["']/g)].map((m) => m[1])

const viewMatch = read('../src/views/Legal.vue').match(/const CATEGORY_KEYS = \[([^\]]*)\]/)
const goMatch = read('../../internal/isms/db/legal.go').match(/LegalCategories\s*=\s*\[\]string\{([^}]*)\}/)

test('Legal.vue CATEGORY_KEYS and db.LegalCategories are both found', () => {
  assert.ok(viewMatch, 'CATEGORY_KEYS not found in Legal.vue')
  assert.ok(goMatch, 'LegalCategories not found in internal/isms/db/legal.go')
})

test('Legal.vue offers exactly the categories the server accepts, in order', () => {
  assert.deepEqual(quoted(viewMatch[1]), quoted(goMatch[1]))
})

test('every legal category has an English label', () => {
  const labels = JSON.parse(read('../src/locales/en/common.json')).enum.legal_category
  for (const key of quoted(viewMatch[1])) {
    assert.ok(labels[key], `enum.legal_category.${key} has no label in en/common.json`)
  }
})
