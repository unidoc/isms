// The `api` object in src/api.js must not repeat a key (issue #459). In an
// object literal the last key wins, so a second copy of a function silently
// shadows the first and an edit to the earlier copy is dropped without any
// error. Nothing else guards this (no ESLint), so the source is parsed and its
// keys counted. `vue/compiler-sfc` is used for the parser: see
// support/renderSfc.js for why that path and not a new dependency.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { babelParse } from 'vue/compiler-sfc'

const SRC = new URL('../src/api.js', import.meta.url)

function apiKeys() {
  const ast = babelParse(readFileSync(SRC, 'utf8'), { sourceType: 'module' })
  for (const node of ast.program.body) {
    if (node.type !== 'ExportNamedDeclaration' || node.declaration?.type !== 'VariableDeclaration') continue
    for (const d of node.declaration.declarations) {
      if (d.id.name !== 'api' || d.init?.type !== 'ObjectExpression') continue
      return d.init.properties
        .filter(p => (p.type === 'ObjectProperty' || p.type === 'ObjectMethod') && !p.computed)
        .map(p => (p.key.type === 'Identifier' ? p.key.name : p.key.value))
    }
  }
  return null
}

test('the api object is found and has the expected shape', () => {
  const keys = apiKeys()
  assert.ok(keys, 'export const api = { ... } not found in src/api.js')
  assert.ok(keys.length > 50, `only ${keys.length} keys found; the parser is not seeing the whole object`)
})

test('the api object has no duplicate keys', () => {
  const seen = new Set()
  const repeated = new Set()
  for (const k of apiKeys()) (seen.has(k) ? repeated : seen).add(k)
  assert.deepEqual([...repeated], [], `duplicate keys in api: ${[...repeated].join(', ')}`)
})
