import test from 'node:test'
import assert from 'node:assert/strict'
import { parseBackgroundColor, parseExtraStyle, hasExplicitTextColor } from '../src/components/tableCellStyle.js'

// Regression: a table cell's inline style used to lose everything but
// background-color the moment it loaded into the editor (tiptap's
// CustomTableCell/CustomTableHeader only ever tracked that one attribute).
// A cell authored with a light background and an explicit dark text color
// kept its background but fell back to the editor's generic light-gray table
// text color — unreadable against a light background, while the read-only
// view (which renders the stored HTML as-is) looked correct. This is the
// exact style string from a real document that showed the bug.
const realCellStyle =
  'border:1px solid #b8bdc3; padding:6px 10px; vertical-align:top; text-align:left; ' +
  'background-color:#ddf0cd; color:#1f1f1f; font-weight:600; text-align:center;'

test('parseBackgroundColor extracts just the background color', () => {
  assert.equal(parseBackgroundColor(realCellStyle), '#ddf0cd')
})

test('parseExtraStyle keeps everything else, including color', () => {
  const rest = parseExtraStyle(realCellStyle)
  assert.match(rest, /color:\s*#1f1f1f/)
  assert.match(rest, /font-weight:\s*600/)
  assert.doesNotMatch(rest, /background-color/)
})

test('a cell with no background color still keeps its other styles', () => {
  const style = 'color:#1f1f1f; font-weight:600;'
  assert.equal(parseBackgroundColor(style), null)
  assert.equal(parseExtraStyle(style), style)
})

test('a cell with only a background color has nothing left over', () => {
  assert.equal(parseExtraStyle('background-color:#ddf0cd;'), null)
})

test('no style attribute at all yields both as null', () => {
  assert.equal(parseBackgroundColor(null), null)
  assert.equal(parseExtraStyle(null), null)
  assert.equal(parseBackgroundColor(undefined), null)
  assert.equal(parseExtraStyle(undefined), null)
})

// Regression (review finding F5 on #388): parseBackgroundColor only matched
// lowercase "background-color", while parseExtraStyle already stripped it
// case-insensitively. An upper-case `BACKGROUND-COLOR:` would be stripped by
// one and picked up by neither — the background vanishes completely.
test('an upper-case BACKGROUND-COLOR is still extracted, and still stripped from extraStyle', () => {
  const style = 'BACKGROUND-COLOR:#ddf0cd; color:#1f1f1f;'
  assert.equal(parseBackgroundColor(style), '#ddf0cd')
  assert.doesNotMatch(parseExtraStyle(style), /background-color/i)
})

// Regression (review finding F3 on #388): hasExplicitTextColor flags the
// cells where a bold mark's generic color would otherwise override the
// cell's own, preserved color (see DocumentEditor.vue's has-text-color CSS).
test('hasExplicitTextColor', () => {
  assert.equal(hasExplicitTextColor('color:#1f1f1f; font-weight:600;'), true)
  assert.equal(hasExplicitTextColor('font-weight:600; color: #1f1f1f;'), true)
  assert.equal(hasExplicitTextColor('BACKGROUND-COLOR:#ddf0cd; COLOR:#1f1f1f;'), true)
  // Must not fire on "background-color" alone — it contains the substring
  // "color:" but is not a color declaration of its own.
  assert.equal(hasExplicitTextColor('background-color:#ddf0cd;'), false)
  assert.equal(hasExplicitTextColor('font-weight:600;'), false)
  assert.equal(hasExplicitTextColor(null), false)
  assert.equal(hasExplicitTextColor(''), false)
})

// tiptap's mergeAttributes (@tiptap/core) concatenates multiple `style`
// contributions property-by-property, parsing each into a Map keyed by
// property name, so a later same-named property overwrites an earlier one —
// this is the same transplant-the-decision pattern used elsewhere in this
// suite: not importing tiptap's internals, just pinning the merge behavior
// this fix depends on against the exact real-world input above.
function mergeStyles(...styles) {
  const entries = s =>
    (s || '')
      .split(';')
      .map(d => d.trim())
      .filter(Boolean)
      .map(d => {
        const i = d.indexOf(':')
        return [d.slice(0, i).trim(), d.slice(i + 1).trim()]
      })
  const map = new Map(styles.flatMap(entries))
  return [...map.entries()].map(([k, v]) => `${k}: ${v}`).join('; ')
}

test('backgroundColor and extraStyle merge back into one correct style attribute', () => {
  const bg = parseBackgroundColor(realCellStyle)
  const rest = parseExtraStyle(realCellStyle)
  const merged = mergeStyles(`background-color: ${bg}`, rest)
  assert.match(merged, /background-color:\s*#ddf0cd/)
  assert.match(merged, /color:\s*#1f1f1f/)
  assert.match(merged, /font-weight:\s*600/)
  // The cell's own later text-align:center (after an earlier text-align:left
  // in the same style string) must still win, same as a browser applying the
  // original inline style would — the Map-based merge keeps "last wins" per
  // property, matching standard CSS cascade behavior for duplicate
  // declarations in one declaration block.
  assert.match(merged, /text-align:\s*center/)
  assert.doesNotMatch(merged, /text-align:\s*left/)
})

// Regression (review finding F7 on #388): the F6 fix reordered
// CustomTableCell/CustomTableHeader's addAttributes() so extraStyle is
// declared before backgroundColor. "background" (shorthand) and
// "background-color" (longhand) are different Map keys, so mergeAttributes
// never collides them — both declarations survive in the merged style
// string. What actually decides the rendered color is the browser's own
// cascade: when a style attribute carries both, the one that appears LATER
// in the string wins. So the fix only works if backgroundColor's own
// declaration is positioned after extraStyle's — a plain "does the merged
// string contain background-color" assertion would pass whether the order
// fix was there or not; this pins down the order itself, the thing F6
// actually changed.
test('extraStyle before backgroundColor: the picker\'s declaration lands after a legacy background shorthand', () => {
  const rest = parseExtraStyle('background: #ddf0cd; color: #1f1f1f;')
  const bg = '#ff0000'
  const merged = mergeStyles(rest, `background-color: ${bg}`)
  assert.equal(merged, 'background: #ddf0cd; color: #1f1f1f; background-color: #ff0000')
  assert.ok(merged.indexOf('background-color') > merged.indexOf('background:'),
    `expected background-color to come after the background shorthand, got: ${merged}`)
})
