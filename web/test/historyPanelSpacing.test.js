// The single-entry History row must keep a gap between its bold label and the
// field / value spans that follow it.
//
// Those spans used to rely on a leading space inside the span
// (`<span> {{ x }}</span>`). Vue's template compiler (whitespace: 'condense')
// drops a whitespace-only text node that sits right before an interpolation, so
// rows rendered "Confirmed0.1" and "Changedstatusin_review". A margin utility
// does not depend on the compiler's whitespace rules, so assert on the compiled
// output: each trailing span either carries ml-1 or compiles to text that still
// starts with a space.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'

const require = createRequire(import.meta.url)
const { parse, compileTemplate } = require('@vue/compiler-sfc')

const FILE = fileURLToPath(new URL('../src/components/HistoryPanel.vue', import.meta.url))

test('single-entry History row spans are separated from the label', () => {
  const { descriptor } = parse(readFileSync(FILE, 'utf8'), { filename: FILE })
  const { code, errors } = compileTemplate({ source: descriptor.template.content, filename: FILE, id: 'history' })
  assert.deepEqual(errors, [])

  // The three trailing spans of the entries.length === 1 branch are the keyed
  // ones (they carry a v-if), hoisted as props objects: find each by its class,
  // then the span that uses it.
  const keyed = [...code.matchAll(/const (_hoisted_\d+) = \{\s*key: \d+,\s*class: "([^"]*)"\s*\}/g)]
  const wanted = ['text-slate-500', 'text-emerald-400/60', 'text-slate-600']
  for (const colour of wanted) {
    const prop = keyed.find((k) => k[2].split(/\s+/).filter((c) => c !== 'ml-1').join(' ') === colour)
    assert.ok(prop, `no keyed span with class ${colour}`)
    const [, name, cls] = prop
    const use = code.match(new RegExp(`"span", ${name}, ([^\\n]*)`))
    assert.ok(use, `no compiled span using ${name}`)
    const separated = cls.split(/\s+/).includes('ml-1') || use[1].startsWith('" ')
    assert.ok(separated, `span "${cls}" has no gap after the label; compiled text starts: ${use[1].slice(0, 40)}`)
  }
})
