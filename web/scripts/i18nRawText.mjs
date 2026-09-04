// Raw-text scanner — plan 80 §6 option B, the grep-based check.
//
// Finds user-visible English literals still hardcoded in `.vue` templates, so
// that Phase 3 extraction cannot silently regress behind itself. Option A
// (`eslint-plugin-vue-i18n`'s `@intlify/no-raw-text`) is the durable answer and
// is precise where this is crude, but the repo has no ESLint at all; introducing
// it mid-extraction means fighting lint noise and translation churn at once.
// Revisit once extraction is complete.
//
// Crude means two things, both deliberate:
//
//   - It is regex over markup, not an AST. Tag matching is quote-aware, so a
//     '>' inside an attribute value is handled, but an unbalanced quote or a
//     '<' inside one still confuses it. Rare enough to absorb.
//   - It over-reports. A count is a budget, not a defect list: the check is a
//     ratchet against a committed baseline (see rawText.test.js), so today's
//     false positives are already priced in and only *new* text fails. That is
//     what lets this run green on a tree where 56 of 59 components are
//     unconverted, which a "fail on any literal" check could not.
//
// For a false positive that has to reach zero, mark the line with an
// `i18n-ignore` comment — `<!-- i18n-ignore -->` anywhere on the same line, or
// on the line immediately before.
import { readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

export const SRC_DIR = fileURLToPath(new URL('../src/', import.meta.url))

// Attributes whose literal value is read out to a user. `alt` and `title` are
// accessibility text; `label` covers both the HTML attribute and the prop that
// several components in this repo take. Bound forms (`:placeholder="expr"`) are
// skipped — the value is an expression, and one that resolves through `t()` is
// exactly what extraction produces.
const TEXT_ATTRS = ['placeholder', 'title', 'aria-label', 'aria-description', 'alt', 'label']

// A run of two or more letters. Anything shorter is punctuation, a symbol, a
// number, or a unit — none of them translatable on their own.
const HAS_WORD = /[A-Za-z]{2,}/

const IGNORE = /i18n-ignore/

function vueFiles(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) vueFiles(full, out)
    else if (entry.endsWith('.vue')) out.push(full)
  }
  return out
}

// The SFC's template block: first `<template` to the last `</template>`, which
// keeps nested `<template v-if>` blocks and excludes `<script>` and `<style>`
// (they follow the template in every file here, but are also stripped below in
// case one precedes it).
function templateBlock(src) {
  const open = src.indexOf('<template')
  const close = src.lastIndexOf('</template>')
  if (open === -1 || close === -1 || close < open) return ''
  return src.slice(open, close)
}

// Lines carrying an `i18n-ignore` marker, plus the line after each — so the
// marker can sit above the offending line as well as on it.
function ignoredLines(src) {
  const out = new Set()
  src.split('\n').forEach((line, i) => {
    if (IGNORE.test(line)) {
      out.add(i + 1)
      out.add(i + 2)
    }
  })
  return out
}

// Blank a matched region while preserving newlines, so every finding keeps its
// real line number instead of collapsing toward the top of the file.
function blank(src, re) {
  return src.replace(re, (m) => m.replace(/[^\n]/g, ' '))
}

/**
 * Raw-text findings for one `.vue` file, as `{line, kind, text}`.
 * `kind` is 'text' for a template text node, or the attribute name.
 */
export function scanSource(src) {
  const skip = ignoredLines(src)
  let tpl = templateBlock(src)
  if (!tpl) return []

  // Order matters. Comments go first (they can contain anything). Mustaches and
  // `v-html` payloads are already-dynamic. `<pre>` and `<code>` hold document
  // ids, commands and samples, which are not translatable prose.
  tpl = blank(tpl, /<!--[\s\S]*?-->/g)
  tpl = blank(tpl, /\{\{[\s\S]*?\}\}/g)
  tpl = blank(tpl, /<pre\b[\s\S]*?<\/pre>/gi)
  tpl = blank(tpl, /<code\b[\s\S]*?<\/code>/gi)
  tpl = blank(tpl, /<script\b[\s\S]*?<\/script>/gi)
  tpl = blank(tpl, /<style\b[\s\S]*?<\/style>/gi)

  const findings = []

  // Line lookup by binary search over precomputed newline offsets. The obvious
  // `src.slice(0, i).split('\n').length` is O(n) per finding and made the whole
  // scan quadratic — 45s across the tree, on files like Documents.vue that run
  // to 3k lines.
  const newlines = []
  for (let i = tpl.indexOf('\n'); i !== -1; i = tpl.indexOf('\n', i + 1)) newlines.push(i)
  const lineOf = (index) => {
    let lo = 0
    let hi = newlines.length
    while (lo < hi) {
      const mid = (lo + hi) >> 1
      if (newlines[mid] < index) lo = mid + 1
      else hi = mid
    }
    return lo + 1
  }

  // Attribute literals, collected before tags are stripped. Only the unbound
  // form: a leading ':' or 'v-bind:' means the value is an expression.
  for (const attr of TEXT_ATTRS) {
    const re = new RegExp(`(^|[\\s{])${attr}\\s*=\\s*"([^"]*)"|(^|[\\s{])${attr}\\s*=\\s*'([^']*)'`, 'gi')
    for (const m of tpl.matchAll(re)) {
      // Reject the bound spellings by looking at what precedes the name.
      const before = tpl.slice(Math.max(0, m.index - 8), m.index + 1)
      if (/[:@]\s*$|v-bind:\s*$/.test(before)) continue
      const value = m[2] ?? m[4] ?? ''
      if (!HAS_WORD.test(value)) continue
      const line = lineOf(m.index)
      if (skip.has(line)) continue
      findings.push({ line, kind: attr.toLowerCase(), text: value.trim() })
    }
  }

  // Text nodes: whatever survives between tags. Tags are replaced by newlines
  // rather than removed so that line numbers keep tracking.
  //
  // TAG is quote-aware rather than the obvious /<[^>]*>/. A '>' inside an
  // attribute value is common in this codebase (`v-if="options.length > 1"`,
  // `v-if="items.length > 0"`) and the naive form ends the tag there, leaking
  // every following attribute out as a text node. That reported six findings in
  // a fully converted component, which would both discredit the check and make
  // a clean file unable to reach zero.
  const TAG = /<\/?[A-Za-z!][^\s>]*(?:"[^"]*"|'[^']*'|[^>"'])*>/g
  const stripped = tpl.replace(TAG, (m) => m.replace(/[^\n]/g, '\n'))
  // A tag is blanked to one newline *per character*, so it acts as a separator
  // without changing any offset: `stripped` and `tpl` are the same length, and a
  // chunk's offset therefore indexes straight back into `tpl` for its real line.
  // (A chunk's *index* in the split is not its line number — the per-character
  // blanking inflates that badly, e.g. a 10-char `<template>` adds 10.)
  let offset = 0
  for (const chunk of stripped.split('\n')) {
    const line = lineOf(offset)
    offset += chunk.length + 1
    const text = chunk.trim()
    if (!text || !HAS_WORD.test(text)) continue
    if (skip.has(line)) continue
    findings.push({ line, kind: 'text', text })
  }

  return findings.sort((a, b) => a.line - b.line)
}

let scanCache = null

/**
 * Findings for every `.vue` file under `web/src`, keyed by src-relative path.
 * Memoized: several tests consume it, and rescanning the tree per call is pure
 * waste in a suite that runs on every push.
 */
export function scanAll() {
  if (scanCache) return scanCache
  const out = {}
  for (const file of vueFiles(SRC_DIR).sort()) {
    const findings = scanSource(readFileSync(file, 'utf8'))
    out[relative(SRC_DIR, file).split(/[\\/]/).join('/')] = findings
  }
  scanCache = out
  return out
}

/** Per-file finding counts, omitting files that are already clean. */
export function counts() {
  const out = {}
  for (const [file, findings] of Object.entries(scanAll())) {
    if (findings.length) out[file] = findings.length
  }
  return out
}

// --- CLI ---------------------------------------------------------------------
//
// `npm run i18n:baseline` rewrites the committed baseline from the current tree.
// Run it after extracting a view; the ratchet in rawText.test.js then holds the
// new, lower number. With `--list <file>` it prints the findings for one file,
// which is the useful form while actually extracting.
const BASELINE = fileURLToPath(new URL('../test/rawText.baseline.json', import.meta.url))

function writeBaseline() {
  const c = counts()
  const total = Object.values(c).reduce((a, b) => a + b, 0)
  writeFileSync(BASELINE, `${JSON.stringify(c, null, 2)}\n`)
  console.log(`wrote ${relative(process.cwd(), BASELINE)}: ${Object.keys(c).length} files, ${total} findings`)
}

if (process.argv[1] && process.argv[1].endsWith('i18nRawText.mjs')) {
  const listIdx = process.argv.indexOf('--list')
  if (listIdx !== -1) {
    const target = process.argv[listIdx + 1]
    const all = scanAll()
    const key = Object.keys(all).find((k) => k === target || k.endsWith(`/${target}`) || k.endsWith(target))
    if (!key) {
      console.error(`no .vue file under web/src matching ${target}`)
      process.exit(1)
    }
    console.log(`${key}: ${all[key].length} findings`)
    for (const f of all[key]) console.log(`  ${key}:${f.line}  [${f.kind}]  ${f.text}`)
  } else if (process.argv.includes('--write')) {
    writeBaseline()
  } else {
    const c = counts()
    const total = Object.values(c).reduce((a, b) => a + b, 0)
    console.log(`${Object.keys(c).length} files, ${total} findings. --write to update the baseline, --list <file> for detail.`)
  }
}
