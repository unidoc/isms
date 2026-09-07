// Scanner for the render-site half of the API error seam.
//
// `renderApiError` translates a caught API error through `common.error.*`. A
// component that assigns `e.message` straight to a template ref bypasses it and
// shows the server's English no matter what locale the reader chose — which is
// invisible today, because `en` is the only enabled locale, and stays invisible
// until the moment it matters.
//
// A per-component test would be one assertion each proving an import exists.
// The claim worth pinning is that the population is shrinking, so this is a
// ratchet over counts, the same shape as i18nRawText.mjs.
//
// It matches a caught error's `.message`, reached either directly or through
// optional chaining, in .vue and .js sources under src/. Both halves of that
// matter and both were wrong in the first version:
//
//   - `err?.message` was invisible, so `LocalePicker.vue` — whose text
//     `Settings.vue` displays — sat outside the baseline entirely. A ratchet
//     that a `?.` walks past is not a ratchet.
//   - the binding names were a fixed list (e/err/error/ex), which happens to
//     cover this tree (208 `catch (e)`, 1 `catch (err)`) but would miss
//     `catch (problem)` the day someone writes it. The names are now derived
//     from the `catch (…)` bindings in each file, unioned with the fixed list
//     so a destructured or re-assigned error still registers.
//
// Two exemptions, both by path rather than by pattern, because a pattern
// exemption would silently cover a real site that happened to look similar:
//
//   src/api.js                     — constructs the Error; there is no code yet.
//   src/composables/useApiError.js — implements the fallback it is reading.
import { readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const SRC = fileURLToPath(new URL('../src/', import.meta.url))

const EXEMPT = new Set(['api.js', 'composables/useApiError.js'])

// Binding names always treated as an error, even with no `catch` in sight —
// arrow-form handlers (`.catch(e => …)`) bind without the statement form.
const FIXED_NAMES = ['e', 'err', 'error', 'ex']

const CATCH_BINDING = /\bcatch\s*\(\s*([A-Za-z_$][\w$]*)\s*\)/g

// A name is a site when it is followed by `.message` or `?.message`.
function siteRegex(names) {
  const alts = [...new Set(names)].map((n) => n.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
  return new RegExp(`\\b(?:${alts.join('|')})\\s*\\??\\.message\\b`)
}

function walk(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) walk(full, out)
    else if (/\.(vue|js)$/.test(entry)) out.push(full)
  }
  return out
}

// scanAll returns {file: [{line, text}]} for every unconverted render site.
export function scanAll() {
  const out = {}
  for (const full of walk(SRC)) {
    const file = relative(SRC, full).split('\\').join('/')
    if (EXEMPT.has(file)) continue
    const source = readFileSync(full, 'utf8')
    const names = [...FIXED_NAMES, ...[...source.matchAll(CATCH_BINDING)].map((m) => m[1])]
    const site = siteRegex(names)
    const hits = []
    source.split('\n').forEach((line, i) => {
      if (site.test(line)) hits.push({ line: i + 1, text: line.trim() })
    })
    if (hits.length) out[file] = hits
  }
  return out
}

export function counts() {
  return Object.fromEntries(Object.entries(scanAll()).map(([f, h]) => [f, h.length]))
}

const BASELINE = fileURLToPath(new URL('../test/errorRender.baseline.json', import.meta.url))

// The ratchet's downward direction cannot be enforced by the test, which sees
// only one checkout: add a raw render, re-run --write, and everything is green
// again. So the guard lives here, exactly as it does in i18nRawText.mjs — a
// write that would *raise* any file's budget is refused and names the files.
// `--force` still allows it, for a scanner change that legitimately finds more
// (as widening this one to optional chaining did), but it has to be typed, and
// the raised numbers then stand out in the baseline diff for review.
function writeBaseline(force) {
  const c = counts()
  let previous = {}
  try {
    previous = JSON.parse(readFileSync(BASELINE, 'utf8'))
  } catch {
    // No baseline yet — the first write has nothing to ratchet against.
  }
  const raised = Object.keys(c)
    .filter((f) => c[f] > (previous[f] ?? 0))
    .map((f) => `  ${f}: ${previous[f] ?? 0} -> ${c[f]}`)
  if (raised.length && !force) {
    console.error(
      `refusing to raise the baseline for ${raised.length} file(s) — budgets may only go down:\n` +
        `${raised.join('\n')}\n\n` +
        `Route the new sites through renderApiError() from composables/useApiError.js. ` +
        `If the scanner itself changed and legitimately sees more, re-run with --force ` +
        `and call the raised numbers out in review.`,
    )
    process.exit(1)
  }
  const total = Object.values(c).reduce((a, b) => a + b, 0)
  writeFileSync(BASELINE, `${JSON.stringify(Object.fromEntries(Object.entries(c).sort()), null, 2)}\n`)
  const note = raised.length ? ` (${raised.length} raised under --force)` : ''
  console.log(`wrote ${relative(process.cwd(), BASELINE)}: ${Object.keys(c).length} files, ${total} sites${note}`)
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  if (process.argv.includes('--write')) {
    writeBaseline(process.argv.includes('--force'))
  } else {
    const c = counts()
    const total = Object.values(c).reduce((a, b) => a + b, 0)
    for (const [f, n] of Object.entries(c).sort()) console.log(`${String(n).padStart(4)}  ${f}`)
    console.log(`total ${total} across ${Object.keys(c).length} files`)
  }
}
