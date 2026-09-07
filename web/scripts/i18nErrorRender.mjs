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
// It matches a caught error's `.message` — `e.message`, `err.message`,
// `error.message`, `ex.message` — in .vue and .js sources under src/. Two
// exemptions, both by path rather than by pattern, because a pattern exemption
// would silently cover a real site that happened to look similar:
//
//   src/api.js                     — constructs the Error; there is no code yet.
//   src/composables/useApiError.js — implements the fallback it is reading.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const SRC = fileURLToPath(new URL('../src/', import.meta.url))

const EXEMPT = new Set(['api.js', 'composables/useApiError.js'])

const SITE = /\b(?:e|err|error|ex)\.message\b/

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
    const hits = []
    readFileSync(full, 'utf8')
      .split('\n')
      .forEach((line, i) => {
        if (SITE.test(line)) hits.push({ line: i + 1, text: line.trim() })
      })
    if (hits.length) out[file] = hits
  }
  return out
}

export function counts() {
  return Object.fromEntries(Object.entries(scanAll()).map(([f, h]) => [f, h.length]))
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const c = counts()
  if (process.argv.includes('--write')) {
    const path = fileURLToPath(new URL('../test/errorRender.baseline.json', import.meta.url))
    const { writeFileSync } = await import('node:fs')
    writeFileSync(path, JSON.stringify(Object.fromEntries(Object.entries(c).sort()), null, 2) + '\n')
    console.log(`wrote ${path}`)
  } else {
    const total = Object.values(c).reduce((a, b) => a + b, 0)
    for (const [f, n] of Object.entries(c).sort()) console.log(`${String(n).padStart(4)}  ${f}`)
    console.log(`total ${total} across ${Object.keys(c).length} files`)
  }
}
