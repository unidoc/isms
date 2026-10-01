import test from 'node:test'
import assert from 'node:assert/strict'
import { parseBackgroundColor, parseExtraStyle } from '../src/components/tableCellStyle.js'

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
