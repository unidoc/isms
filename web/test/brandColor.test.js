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
  parseHex, contrastRatio, readableTextColor, readableFillColor, applyBrandColor, MIN_TEXT_CONTRAST, TEXT_BACKGROUND,
} from '../src/utils/brandColor.js'

const SLATE_800 = '#1d293d' // Tailwind v4 slate-800 as rendered

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

test('readableTextColor output is pinned (slate-800 as rendered)', () => {
  assert.equal(readableTextColor('#1a365d'), '#8190a6')
  assert.equal(readableTextColor('#3b82f6'), '#4d8df7')
  // the reviewer's example landed at 4.496:1 against the v3 value
  assert.ok(contrastRatio(readableTextColor('#ff4477'), SLATE_800) >= 4.5)
})

const mixToward = (hex, target, pct) =>
  '#' + parseHex(hex).map((v) => Math.round(v + (target - v) * pct / 100))
    .map((v) => v.toString(16).padStart(2, '0')).join('')

test('derived colors are minimal: one 1% step less mixing fails', () => {
  const sample = ['#000000', '#1a365d', '#3b82f6', '#2563eb', '#dc2626', '#ff4477',
    '#16a34a', '#fde047', '#60a5fa', '#808080', '#123456', '#abcdef']
  let checkedText = 0
  let checkedFill = 0
  for (const hex of sample) {
    const norm = '#' + parseHex(hex).map((v) => v.toString(16).padStart(2, '0')).join('')
    const text = readableTextColor(hex)
    if (text !== norm) {
      let pct = 1
      while (mixToward(hex, 255, pct) !== text) pct++
      assert.ok(contrastRatio(mixToward(hex, 255, pct - 1), SLATE_800) < 4.5, `text ${hex}`)
      checkedText++
    }
    const fill = readableFillColor(hex)
    if (fill !== norm) {
      let pct = 1
      while (mixToward(hex, 0, pct) !== fill) pct++
      assert.ok(contrastRatio('#ffffff', mixToward(hex, 0, pct - 1)) < 4.5, `fill ${hex}`)
      checkedFill++
    }
  }
  assert.ok(checkedText >= 5 && checkedFill >= 3, `${checkedText}/${checkedFill} checked`)
})

test('applyBrandColor sets the three variables, and nothing for bad input', () => {
  const had = Object.getOwnPropertyDescriptor(globalThis, 'document')
  const props = {}
  globalThis.document = { documentElement: { style: {
    setProperty(k, v) { props[k] = v },
    removeProperty(k) { delete props[k] },
  } } }
  try {
    applyBrandColor('#1a365d')
    assert.deepEqual(props, {
      '--brand-color': '#1a365d',
      '--brand-text-color': '#8190a6',
      '--brand-fill-color': '#1a365d',
    })
    for (const k of Object.keys(props)) delete props[k]
    applyBrandColor('#12345')
    assert.deepEqual(props, {})
  } finally {
    if (had) Object.defineProperty(globalThis, 'document', had)
    else delete globalThis.document
  }
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

test('only utils/brandColor.js writes the --brand-* variables', () => {
  const src = fileURLToPath(new URL('../src', import.meta.url))
  const hits = walk(src)
    .filter((f) => /setProperty\(\s*['"`]--brand-(color|text-color|fill-color)['"`]/.test(readFileSync(f, 'utf8')))
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
  // The pattern skips background-color/border-color and --brand-color-* names,
  // and also flags Tailwind arbitrary text classes using the raw variable.
  const src = fileURLToPath(new URL('../src', import.meta.url))
  const re = new RegExp(
    '(^|[^-\\w])color:\\s*[^;"}]*var\\(--brand-color(?![-\\w])'
    + '|text-\\[var\\(--brand-color\\)\\]|text-\\(--brand-color\\)')
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
