// The render-site ratchet.
//
// The server now sends a stable `code` on its errors and `renderApiError`
// translates it, but a component that assigns a caught error's `.message`
// straight to a template ref never calls the renderer. Every such site is a
// place the reader sees the server's English regardless of their locale.
//
// This is deliberately not a per-component test. Thirty-one assertions each
// proving one import exists would pass while the population stayed flat; the
// claim worth pinning is that it is shrinking. So it is a ratchet over counts,
// the same shape and for the same reasons as rawText.test.js:
//
//   count > baseline  -> fail. A new unconverted site.
//   count < baseline  -> fail, asking for `npm run i18n:error-baseline`.
//                        Progress must be committed, or the budget silently
//                        re-opens room for a regression.
//   file not listed   -> must be zero. Converted and new files start clean.
//
// The scanner's exemptions are by path, not pattern — see i18nErrorRender.mjs.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { counts, scanAll } from '../scripts/i18nErrorRender.mjs'

const BASELINE_PATH = fileURLToPath(new URL('./errorRender.baseline.json', import.meta.url))
const baseline = JSON.parse(readFileSync(BASELINE_PATH, 'utf8'))

const FIX = 'run `npm run i18n:error-baseline` to update the committed baseline'

test('no file renders more raw error messages than its baseline', () => {
  const found = counts()
  const all = scanAll()
  const regressions = []

  for (const [file, n] of Object.entries(found)) {
    const budget = baseline[file] ?? 0
    if (n > budget) {
      const worst = all[file].slice(-(n - budget)).map((h) => `${file}:${h.line} ${h.text}`)
      regressions.push(
        `${file}: ${n} raw error renders, baseline ${budget} (+${n - budget}). ` +
          `Route them through renderApiError() from composables/useApiError.js. ` +
          `Candidates:\n    ${worst.join('\n    ')}`,
      )
    }
  }

  assert.deepEqual(regressions, [], `raw error rendering reintroduced:\n  ${regressions.join('\n  ')}`)
})

test('the error-render baseline is tight', () => {
  const found = counts()
  const loosened = []

  for (const [file, budget] of Object.entries(baseline)) {
    const n = found[file] ?? 0
    if (n < budget) loosened.push(`${file}: ${n} raw error renders but baseline allows ${budget}`)
  }

  assert.deepEqual(
    loosened,
    [],
    `the baseline is looser than the tree — adoption must be committed (${FIX}):\n  ${loosened.join('\n  ')}`,
  )
})

// The five auth views this PR converted must stay at zero, named explicitly.
// The ratchet above already enforces it through the "not listed -> must be
// zero" rule, but naming them makes the failure say which view regressed
// rather than only that some budget was exceeded.
test('the converted auth views render no raw error messages', () => {
  const found = counts()
  for (const file of [
    'views/Login.vue',
    'views/Signup.vue',
    'views/ForgotPassword.vue',
    'views/VerifyEmail.vue',
    'views/VerifyEmailChange.vue',
  ]) {
    assert.equal(found[file] ?? 0, 0, `${file} reintroduced a raw error render`)
  }
})
