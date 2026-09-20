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

// The block's markup is not only rendered — useDocumentComments.blockHash
// hashes it to anchor inline comments, and commentsForBlock hard-rejects on a
// hash mismatch. So adding `start` to the wrapper must not change what gets
// hashed, or every inline comment already stored against an ordered-list item
// silently detaches. `raw` is what keeps the hashed string stable.
test('ordered-list blocks hash the same as before start was threaded', () => {
  // verbatim from web/src/composables/useDocumentComments.js:35-44
  const blockHash = (block) => {
    const text = block.raw || block.html || ''
    let h = 5381
    for (let i = 0; i < text.length; i++) h = ((h << 5) + h + text.charCodeAt(i)) & 0xffffffff
    return h.toString(36)
  }
  const md = Array.from({ length: 14 }, (_, i) => `${i + 1}. Item ${i + 1}`).join('\n')
  for (const b of buildContentBlocks(md)) {
    // the pre-`start` wrapper shape, which is what the hash must still see
    assert.match(b.raw, /^<ol><li>/)
    assert.doesNotMatch(b.raw, /start=/)
    assert.match(b.html, /^<ol start="\d+">/)
    assert.equal(blockHash(b), blockHash({ html: b.raw }))
  }
})

test('unordered-list blocks also carry a stable raw, unchanged from html', () => {
  // ul's rendered html never gains `start`, so raw is redundant there — but
  // still set (addBlock applies it uniformly), and it must equal html.
  const md = '- First\n- Second\n'
  for (const b of buildContentBlocks(md)) {
    assert.equal(b.raw, b.html)
  }
})

test('a list starting at 0 keeps 0 as its base, not 1', () => {
  // parseInt('0', 10) is 0, which is falsy — a naive `|| 1` fallback would
  // treat a deliberate "0." start the same as no start attribute at all.
  const md = '0. Zeroth\n1. First\n2. Second\n'
  const blocks = buildContentBlocks(md)
  assert.deepEqual(
    blocks.map((b) => b.html.match(/start="(\d+)"/)[1]),
    ['0', '1', '2'],
  )
})

// Regression: a table inserted with the editor's "Insert table" command is
// stored as raw Tiptap HTML inline in the markdown, and Tiptap puts the header
// <tr> of <th> cells straight inside <tbody> without ever emitting a <thead>.
// Looking the header up via querySelector('thead') therefore found nothing:
// the header block was skipped and the header row fell through the body-row
// loop as an empty grid row, so readers and printed PDFs lost the column
// labels entirely. The input below is that real editor output shape.
test('an editor-authored table with no <thead> keeps its header row', () => {
  const md = '<table><colgroup><col style="width: 200px"><col style="width: 200px"><col style="width: 200px"></colgroup><tbody><tr><th colwidth="200"><p><span style="color: rgb(0,0,0)">Internal Issue</span></p></th><th colwidth="200"><p><span style="color: rgb(0,0,0)">Overview</span></p></th><th colwidth="200"><p><span style="color: rgb(0,0,0)">Risk Ref. on Risk Register</span></p></th></tr><tr><td colwidth="200"><p>Issue 1</p></td><td colwidth="200"><p>Overview text</p></td><td colwidth="200"><p>R-1</p></td></tr></tbody></table>'
  const blocks = buildContentBlocks(md)
  assert.deepEqual(blocks.map((b) => b.tag), ['thead', 'tr'])
  assert.equal(blocks[0].html.split('tbl-hdr-cell').length - 1, 3)
  assert.match(blocks[0].html, /Internal Issue/)
  assert.match(blocks[0].html, /Overview/)
  assert.match(blocks[0].html, /Risk Ref\. on Risk Register/)
  assert.equal(blocks[1].html.split('tbl-cell').length - 1, 3)
  assert.match(blocks[1].html, /Issue 1/)
  assert.doesNotMatch(blocks[1].html, /Internal Issue/)
})

// A table block carries no `raw`, so its `html` string IS the anchor that
// useDocumentComments.blockHash hashes, and commentsForBlock hard-rejects on a
// hash mismatch. These three strings are therefore inline-comment anchors: if
// the emitted html for a markdown pipe table changes by even one byte, every
// inline comment already stored against that table silently detaches.
test('markdown pipe-table block html is byte-stable', () => {
  const md = '| A | B |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |\n'
  const blocks = buildContentBlocks(md)
  assert.deepEqual(blocks.map((b) => b.tag), ['thead', 'tr', 'tr'])
  assert.equal(
    blocks[0].html,
    '<div class="tbl-grid" style="grid-template-columns: 1fr 1fr;"><div class="tbl-hdr-cell" style="">A</div><div class="tbl-hdr-cell" style="">B</div></div>',
  )
  assert.equal(
    blocks[1].html,
    '<div class="tbl-grid tbl-row" style="grid-template-columns: 1fr 1fr;"><div class="tbl-cell" style="">1</div><div class="tbl-cell" style="">2</div></div>',
  )
  assert.equal(
    blocks[2].html,
    '<div class="tbl-grid tbl-row" style="grid-template-columns: 1fr 1fr;"><div class="tbl-cell" style="">3</div><div class="tbl-cell" style="">4</div></div>',
  )
})
