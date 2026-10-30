// Brand color text contrast (#408): link and button text written as
// text-blue-400 must stay readable (>= 4.5:1) on the dark theme whatever brand
// color an admin picks. The text color is derived by utils/brandColor.js and
// every writer of --brand-color goes through it.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  parseHex, contrastRatio, readableTextColor, readableFillColor, MIN_TEXT_CONTRAST, TEXT_BACKGROUND,
} from '../src/utils/brandColor.js'

const SLATE_800 = '#1e293b'

test('background constant is slate-800 and threshold is 4.5', () => {
  assert.equal(TEXT_BACKGROUND, SLATE_800)
  assert.equal(MIN_TEXT_CONTRAST, 4.5)
})

test('readableTextColor lifts dark and mid brand colors to >= 4.5:1', () => {
  for (const hex of ['#1a365d', '#000000', '#3b82f6', '#2563eb', '#dc2626']) {
    const out = readableTextColor(hex)
    assert.match(out, /^#[0-9a-f]{6}$/, hex)
    assert.ok(contrastRatio(out, SLATE_800) >= 4.5, `${hex} -> ${out}`)
    // a lighter mix of the input, not a fixed color
    const [r, g, b] = parseHex(hex)
    const [R, G, B] = parseHex(out)
    assert.ok(R >= r && G >= g && B >= b, `${hex} -> ${out} is not lighter`)
  }
  assert.notEqual(readableTextColor('#1a365d'), readableTextColor('#2563eb'))
})

test('readableTextColor keeps already readable colors, normalised', () => {
  assert.equal(readableTextColor('#60a5fa'), '#60a5fa')
  assert.equal(readableTextColor('#fde047'), '#fde047')
  assert.equal(readableTextColor('#ffffff'), '#ffffff')
  assert.equal(readableTextColor('#FDE047'), '#fde047')
})

test('parseHex accepts 3/4/6/8 digit forms and drops alpha', () => {
  assert.deepEqual(parseHex('#abc'), [0xaa, 0xbb, 0xcc])
  assert.deepEqual(parseHex('#ABCDEF'), [0xab, 0xcd, 0xef])
  assert.deepEqual(parseHex('#1a365dff'), [0x1a, 0x36, 0x5d])
  assert.deepEqual(parseHex('#abcd'), [0xaa, 0xbb, 0xcc])
})

test('parseHex rejects malformed input', () => {
  for (const bad of ['#12345', '#xyz', '', undefined, null, '1a365d']) {
    assert.equal(parseHex(bad), null, String(bad))
    assert.equal(readableTextColor(bad), null, String(bad))
  }
})

test('contrastRatio matches WCAG', () => {
  assert.equal(contrastRatio('#ffffff', '#000000'), 21)
  assert.equal(contrastRatio('#336699', '#336699'), 1)
  assert.equal(contrastRatio('#60a5fa', SLATE_800).toFixed(2), '5.75')
})

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walk(p, out)
    else if (/\.(js|vue|css)$/.test(name)) out.push(p)
  }
  return out
}

test('only utils/brandColor.js writes --brand-color', () => {
  const src = fileURLToPath(new URL('../src', import.meta.url))
  const hits = walk(src)
    .filter((f) => /setProperty\(\s*['"`]--brand-color['"`]/.test(readFileSync(f, 'utf8')))
    .map((f) => f.slice(src.length + 1))
  assert.deepEqual(hits, ['utils/brandColor.js'])
})

test('App.vue link/button text uses --brand-text-color', () => {
  const css = readFileSync(fileURLToPath(new URL('../src/App.vue', import.meta.url)), 'utf8')
  const m = css.match(/button\.text-blue-400,\s*a\.text-blue-400\s*\{([^}]*)\}/)
  assert.ok(m, 'rule not found')
  assert.match(m[1], /color:\s*var\(--brand-text-color\)/)
  assert.doesNotMatch(m[1], /var\(--brand-color\)/)
  assert.match(css, /:root\s*\{[^}]*--brand-text-color:/)
})

test('no text color is drawn from the raw --brand-color', () => {
  // Text uses --brand-text-color; fills, borders and shadows keep --brand-color.
  // The pattern skips background-color/border-color and --brand-color-* names.
  const src = fileURLToPath(new URL('../src', import.meta.url))
  const re = /(^|[^-\w])color:\s*[^;"}]*var\(--brand-color\b/
  const hits = []
  for (const f of walk(src)) {
    readFileSync(f, 'utf8').split('\n').forEach((line, i) => {
      if (re.test(line)) hits.push(`${f.slice(src.length + 1)}:${i + 1}`)
    })
  }
  assert.deepEqual(hits, [], `text color from var(--brand-color): ${hits.join(', ')}`)
})

test('readableFillColor darkens fills until a white label reaches 4.5:1', () => {
  for (const hex of ['#3b82f6', '#fde047', '#ffffff', '#60a5fa', '#16a34a']) {
    const out = readableFillColor(hex)
    assert.match(out, /^#[0-9a-f]{6}$/, hex)
    assert.ok(contrastRatio('#ffffff', out) >= 4.5, `${hex} -> ${out}`)
    const [r, g, b] = parseHex(hex)
    const [R, G, B] = parseHex(out)
    assert.ok(R <= r && G <= g && B <= b, `${hex} -> ${out} is not darker`)
  }
  assert.equal(readableFillColor('#3b82f6'), '#3472d8')
})

test('readableFillColor keeps fills that already carry white, normalised', () => {
  assert.equal(readableFillColor('#1a365d'), '#1a365d')
  assert.equal(readableFillColor('#000000'), '#000000')
  assert.equal(readableFillColor('#2563eb'), '#2563eb')
  assert.equal(readableFillColor('#DC2626'), '#dc2626')
})

test('readableFillColor rejects malformed input', () => {
  for (const bad of ['#12345', '#xyz', '', undefined, null]) {
    assert.equal(readableFillColor(bad), null, String(bad))
  }
})

test('hard-coded fill defaults equal readableFillColor of the default brand', () => {
  const want = readableFillColor('#3b82f6')
  const app = readFileSync(fileURLToPath(new URL('../src/App.vue', import.meta.url)), 'utf8')
  const css = readFileSync(fileURLToPath(new URL('../src/style.css', import.meta.url)), 'utf8')
  assert.equal(/:root\s*\{[^}]*--brand-fill-color:\s*(#[0-9a-f]{6})/i.exec(app)?.[1], want)
  const btn = /\.brand-btn\s*\{[^}]*var\(--brand-fill-color,\s*(#[0-9a-f]{6})\)/i.exec(css)
  assert.equal(btn?.[1], want)
})

test('brand fills carrying white labels use --brand-fill-color', () => {
  const app = readFileSync(fileURLToPath(new URL('../src/App.vue', import.meta.url)), 'utf8')
  const css = readFileSync(fileURLToPath(new URL('../src/style.css', import.meta.url)), 'utf8')
  const rules = [...app.matchAll(/(button|a)\.bg-blue-600(?::hover)?\s*(?:,\s*a\.bg-blue-600(?::hover)?\s*)?\{([^}]*)\}/g)]
  assert.equal(rules.length, 2, 'expected the bg-blue-600 rule and its :hover rule')
  for (const [, , body] of rules) {
    assert.match(body, /background-color:\s*var\(--brand-fill-color\)/)
    assert.doesNotMatch(body, /background-color:\s*var\(--brand-color\)/)
  }
  const btn = /\.brand-btn\s*\{([^}]*)\}/.exec(css)[1]
  assert.match(btn, /background-color:\s*var\(--brand-fill-color/)
  assert.doesNotMatch(btn, /background-color:\s*var\(--brand-color/)
})
