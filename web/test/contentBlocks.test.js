import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body></body></html>')
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.Node = dom.window.Node
globalThis.Element = dom.window.Element
globalThis.HTMLElement = dom.window.HTMLElement

const { buildContentBlocks } = await import('../src/utils/contentBlocks.js')

// Regression: splitting a list into one commentable block per <li> left each
// item alone in a fresh <ol>, which resets an ordered list's visible number
// to 1 for every item — a real production document rendered every item as
// "1." instead of counting up. The fix threads a `start` attribute through
// each split so the browser still shows the item's real position.
test('split ordered-list blocks keep their original numbering', () => {
  const md = Array.from({ length: 14 }, (_, i) => `${i + 1}. Item ${i + 1}`).join('\n')
  const blocks = buildContentBlocks(md)
  assert.equal(blocks.length, 14)
  blocks.forEach((b, i) => {
    assert.equal(b.tag, 'li')
    assert.match(b.html, new RegExp(`^<ol start="${i + 1}">`))
    assert.match(b.html, new RegExp(`Item ${i + 1}`))
  })
})

test('a blank line between list items does not reset numbering', () => {
  // The exact shape that surfaced this: marked keeps a blank-line-separated
  // ordered list as ONE list (just "loose", each item wrapped in <p>), so the
  // split still has to count every item's true position, not restart at the
  // blank line.
  const md = '1. First\n2. Second\n\n3. Third\n4. Fourth\n'
  const blocks = buildContentBlocks(md)
  assert.equal(blocks.length, 4)
  assert.deepEqual(
    blocks.map((b) => b.html.match(/start="(\d+)"/)[1]),
    ['1', '2', '3', '4'],
  )
})

test('an ordered list starting above 1 keeps its real start number', () => {
  // marked emits a `start` attribute on the <ol> itself when the source's
  // first item names a number other than 1. The split must carry that
  // forward as the base, not always assume 1.
  const md = '5. Fifth\n6. Sixth\n7. Seventh\n'
  const blocks = buildContentBlocks(md)
  assert.deepEqual(
    blocks.map((b) => b.html.match(/start="(\d+)"/)[1]),
    ['5', '6', '7'],
  )
})

test('unordered list items are not given a start attribute', () => {
  const md = '- First\n- Second\n- Third\n'
  const blocks = buildContentBlocks(md)
  assert.equal(blocks.length, 3)
  for (const b of blocks) {
    assert.match(b.html, /^<ul>/)
    assert.doesNotMatch(b.html, /start=/)
  }
})

test('non-list content is not split — one block per element', () => {
  const md = '# Heading\n\nA paragraph.\n\nAnother paragraph.\n'
  const blocks = buildContentBlocks(md)
  assert.deepEqual(blocks.map((b) => b.tag), ['h1', 'p', 'p'])
})

test('table rows split into one block per row, header separate', () => {
  const md = '| A | B |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |\n'
  const blocks = buildContentBlocks(md)
  assert.deepEqual(blocks.map((b) => b.tag), ['thead', 'tr', 'tr'])
})

test('empty content produces no blocks', () => {
  assert.deepEqual(buildContentBlocks(''), [])
  assert.deepEqual(buildContentBlocks(null), [])
})
