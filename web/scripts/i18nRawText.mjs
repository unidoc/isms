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
// scanned too, but only for the string literals inside the expression: a bound
// value that resolves through `t()` is exactly what extraction produces, while
// `:title="'Delete item'"` is raw text wearing a binding.
const TEXT_ATTRS = ['placeholder', 'title', 'aria-label', 'aria-description', 'alt', 'label']

// A run of two or more letters. Anything shorter is punctuation, a symbol, a
// number, or a unit — none of them translatable on their own.
const HAS_WORD = /[A-Za-z]{2,}/

const IGNORE = /i18n-ignore/

// String literals inside a JS expression — a mustache body or a bound attribute
// value. A literal there is as user-visible as a text node: `{{ busy ? 'Saving'
// : 'Save' }}` and `:title="'Delete item'"` both render English that no `t()`
// will ever reach, and blanking every expression wholesale let a zero-baseline
// component reintroduce copy without failing the ratchet.
const EXPR_STRING = /'([^'\\]*)'|"([^"\\]*)"/g

// A translation key, not prose: dotted lowercase, `common.locale.org_default`.
// This is what keeps `{{ $t('common.locale.org_default') }}` out of the results,
// and it holds where a `\$?t\(` lookbehind does not — `$t(cond ? 'a.b' : 'c.d')`
// puts the call two tokens away from the literal it is keying.
const TRANSLATION_KEY = /^[a-z0-9_]+(\.[a-z0-9_-]+)+$/

// Literals in `expr` that look like user-visible copy, as `{offset, text}` where
// offset is relative to the start of `expr`.
function expressionLiterals(expr) {
  const out = []
  for (const m of expr.matchAll(EXPR_STRING)) {
    const value = m[1] ?? m[2] ?? ''
    if (!HAS_WORD.test(value)) continue
    if (TRANSLATION_KEY.test(value.trim())) continue
    out.push({ offset: m.index, text: value.trim() })
  }
  return out
}

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
//
// Everything outside the block is blanked rather than sliced away, preserving
// its newlines, so a finding's line number is the real *file* line. Slicing made
// them template-relative, which is identical for every component in the tree
// today (all open on line 1) and wrong the moment a script block or a licence
// comment precedes the template — pointing `--list` at the wrong line.
function templateBlock(src) {
  const open = src.indexOf('<template')
  const close = src.lastIndexOf('</template>')
  if (open === -1 || close === -1 || close < open) return ''
  return src.slice(0, open).replace(/[^\n]/g, ' ') + src.slice(open, close)
}

// Lines carrying an `i18n-ignore` marker, plus the line after each — so the
// marker can sit above the offending line as well as on it.
//
// Takes the blanked-prefix template block, so both markers and findings are
// numbered against the file and stay aligned however many lines precede
// `<template`. Getting the two onto different bases is a silent no-op on every
// component in the tree today (all open on line 1) — a suppression that quietly
// fails to suppress, which is the failure class this whole check exists to
// prevent.
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
  let tpl = templateBlock(src)
  if (!tpl) return []
  // Before the comments are blanked: the marker is itself a comment.
  const skip = ignoredLines(tpl)

  // Order matters. Comments go first (they can contain anything). Mustaches and
  // `v-html` payloads are already-dynamic. `<pre>` and `<code>` hold document
  // ids, commands and samples, which are not translatable prose.
  tpl = blank(tpl, /<!--[\s\S]*?-->/g)
  // Mustache bodies are harvested for literals before being blanked; comments
  // are already gone, so a commented-out interpolation does not count.
  const mustaches = [...tpl.matchAll(/\{\{([\s\S]*?)\}\}/g)].map((m) => ({
    index: m.index + 2,
    expr: m[1],
  }))
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

  // Literals inside interpolations. `kind` stays 'text': what renders is a text
  // node, and the author's fix is the same one.
  for (const { index, expr } of mustaches) {
    for (const lit of expressionLiterals(expr)) {
      const line = lineOf(index + lit.offset)
      if (skip.has(line)) continue
      findings.push({ line, kind: 'text', text: lit.text })
    }
  }

  // Bound text attributes. The value is an expression, so only its string
  // literals count — `:title="t('a.b')"` is the extracted form and must stay
  // silent, while `:title="collapsed ? 'Expand sidebar' : 'Collapse sidebar'"`
  // is two pieces of raw copy.
  for (const attr of TEXT_ATTRS) {
    const re = new RegExp(`(?:^|[\\s{])(?::|v-bind:)${attr}\\s*=\\s*(?:"([^"]*)"|'([^']*)')`, 'gi')
    for (const m of tpl.matchAll(re)) {
      const expr = m[1] ?? m[2] ?? ''
      const exprStart = m.index + m[0].length - expr.length - 1
      for (const lit of expressionLiterals(expr)) {
        const line = lineOf(exprStart + lit.offset)
        if (skip.has(line)) continue
        findings.push({ line, kind: attr.toLowerCase(), text: lit.text })
      }
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
// new, lower number. It refuses to raise a budget without `--force`. With `--list <file>` it prints the findings for one file,
// which is the useful form while actually extracting.
const BASELINE = fileURLToPath(new URL('../test/rawText.baseline.json', import.meta.url))

// The ratchet's downward direction cannot be enforced by the test, which sees
// only one checkout: raise a budget, re-run this, and everything is green again.
// So the guard lives here — a write that would *raise* any file's budget is
// refused and names the files. `--force` still allows it (a scanner change that
// legitimately finds more, as this one did), but it has to be typed, and the
// raised numbers then stand out in the `rawText.baseline.json` diff for review.
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
        `Extract the new strings through t(), or mark false positives i18n-ignore. ` +
        `If the scanner itself changed and legitimately sees more, re-run with --force ` +
        `and call the raised numbers out in review.`,
    )
    process.exit(1)
  }
  const total = Object.values(c).reduce((a, b) => a + b, 0)
  writeFileSync(BASELINE, `${JSON.stringify(c, null, 2)}\n`)
  const note = raised.length ? ` (${raised.length} raised under --force)` : ''
  console.log(`wrote ${relative(process.cwd(), BASELINE)}: ${Object.keys(c).length} files, ${total} findings${note}`)
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
    writeBaseline(process.argv.includes('--force'))
  } else {
    const c = counts()
    const total = Object.values(c).reduce((a, b) => a + b, 0)
    console.log(`${Object.keys(c).length} files, ${total} findings. --write to update the baseline, --list <file> for detail.`)
  }
}
