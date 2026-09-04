// The raw-text ratchet — plan 80 §6 option B, and task 1.9's outstanding half.
//
// Phase 3 extraction is ~21 PRs by one maintainer over weeks. The keyset test
// already stops a translation from diverging from `en`; nothing stopped a new
// PR from reintroducing a hardcoded literal into a view that had already been
// converted, which is the regression this exists to catch.
//
// It is a ratchet, not a gate: each file carries a committed budget in
// rawText.baseline.json, and the budget may only go down. That is what lets the
// check run green today, with 56 of 59 components unconverted, while still
// failing the moment a converted file grows a literal back.
//
//   count > baseline  -> fail. New raw text.
//   count < baseline  -> fail, asking for `npm run i18n:baseline`. Improvement
//                        must be committed, or the ratchet never tightens and
//                        the budget silently re-opens room for a regression.
//   file not listed   -> must be zero. Converted and new files start clean.
//
// The scanner over-reports (see scripts/i18nRawText.mjs); the baseline numbers
// are budgets, not defect counts, and today's false positives are priced in.
// For a false positive that has to reach zero, mark the line `i18n-ignore`.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { counts, scanAll } from '../scripts/i18nRawText.mjs'

const BASELINE_PATH = fileURLToPath(new URL('./rawText.baseline.json', import.meta.url))
const baseline = JSON.parse(readFileSync(BASELINE_PATH, 'utf8'))

const FIX = 'run `npm run i18n:baseline` to update the committed baseline'

test('no .vue file carries more raw text than its baseline', () => {
  const found = counts()
  const all = scanAll()
  const regressions = []

  for (const [file, n] of Object.entries(found)) {
    const budget = baseline[file] ?? 0
    if (n > budget) {
      // Name the actual lines: a bare count tells the author nothing about
      // which string to extract.
      const worst = all[file].slice(-(n - budget)).map((f) => `${file}:${f.line} "${f.text}"`)
      regressions.push(
        `${file}: ${n} raw strings, baseline ${budget} (+${n - budget}). ` +
          `Extract them through t(), or if these are false positives mark the lines i18n-ignore. ` +
          `Candidates:\n    ${worst.join('\n    ')}`,
      )
    }
  }

  assert.deepEqual(regressions, [], `raw text reintroduced:\n  ${regressions.join('\n  ')}`)
})

test('the baseline is tight — no file is listed above what it carries', () => {
  const found = counts()
  const loosened = []

  for (const [file, budget] of Object.entries(baseline)) {
    const n = found[file] ?? 0
    if (n < budget) {
      loosened.push(`${file}: ${n} raw strings but baseline allows ${budget}`)
    }
  }

  assert.deepEqual(
    loosened,
    [],
    `the baseline is looser than the tree — extraction progress must be committed, or it leaves ` +
      `room for a later regression to slip in unnoticed (${FIX}):\n  ${loosened.join('\n  ')}`,
  )
})

test('every baselined file still exists', () => {
  const all = scanAll()
  const missing = Object.keys(baseline).filter((f) => !(f in all))
  assert.deepEqual(missing, [], `baseline names files that are gone — ${FIX}: ${missing.join(', ')}`)
})

// The scanner is the check's foundation, so its own behaviour is asserted here
// rather than trusted. Each case is a trap found while building it.
test('the scanner recognises what it should and ignores what it should not', async () => {
  const { scanSource } = await import('../scripts/i18nRawText.mjs')

  const kinds = (src) => scanSource(src).map((f) => `${f.kind}:${f.text}`)

  assert.deepEqual(kinds('<template><p>Hello world</p></template>'), ['text:Hello world'])

  // Already extracted: an interpolation is dynamic, whatever is inside it.
  assert.deepEqual(kinds("<template><p>{{ $t('a.b') }}</p></template>"), [])

  // A '>' inside an attribute value must not end the tag and leak the rest of
  // the attributes out as text — this is what reported six findings in a fully
  // converted LocalePicker.vue.
  assert.deepEqual(kinds('<template><select v-if="items.length > 1"\n  :title="x">\n</select></template>'), [])

  // Unbound text attributes count; bound ones are expressions.
  assert.deepEqual(kinds('<template><input placeholder="Acme Corp"></template>'), ['placeholder:Acme Corp'])
  assert.deepEqual(kinds('<template><input :placeholder="t(\'a.b\')"></template>'), [])

  // Not prose: punctuation, numbers, single letters, and code/doc-id samples.
  assert.deepEqual(kinds('<template><span>—</span><span>42</span><span>%</span></template>'), [])
  assert.deepEqual(kinds('<template><code>iso27001-4-1</code><pre>go build ./...</pre></template>'), [])

  // Comments are not user-visible, and the marker suppresses a false positive
  // both on its own line and on the line below.
  assert.deepEqual(kinds('<template><!-- Explain something --><p>Real text</p></template>'), ['text:Real text'])
  assert.deepEqual(kinds('<template><p>Skipped</p><!-- i18n-ignore --></template>'), [])
  assert.deepEqual(kinds('<template><!-- i18n-ignore -->\n<p>Skipped</p></template>'), [])

  // Only the template block. Script-block strings are not user-visible markup,
  // and the script is where t() calls legitimately live.
  assert.deepEqual(kinds('<template><p>Shown</p></template>\n<script setup>const s = "Hidden here"</script>'), [
    'text:Shown',
  ])

  // The marker is numbered against the template block, not the file, so it must
  // still suppress when something precedes `<template`. Every component in the
  // tree opens on line 1 today, which makes an offset bug invisible.
  assert.deepEqual(kinds('<!-- a leading file comment -->\n\n<template>\n<!-- i18n-ignore -->\n<p>Skipped</p></template>'), [])
  assert.deepEqual(kinds('<!-- a leading file comment -->\n\n<template>\n<p>Kept</p></template>'), ['text:Kept'])

  // Line numbers must survive the blanking, or the failure message points at
  // the wrong string.
  assert.deepEqual(scanSource('<template>\n\n\n  <p>Down here</p>\n</template>')[0].line, 4)
})
