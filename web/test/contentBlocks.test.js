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

// A table block's `raw` — not `html` — is the anchor that
// useDocumentComments.blockHash hashes, and commentsForBlock hard-rejects on a
// hash mismatch. `raw` must stay byte-stable at the original equal-width
// markup forever: if it changes by even one byte, every inline comment
// already stored against every table ever saved silently detaches. `html` is
// free to evolve (e.g. the content-aware column widths below).
test('markdown pipe-table block raw (the comment-hash anchor) is byte-stable', () => {
  const md = '| A | B |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |\n'
  const blocks = buildContentBlocks(md)
  assert.deepEqual(blocks.map((b) => b.tag), ['thead', 'tr', 'tr'])
  assert.equal(
    blocks[0].raw,
    '<div class="tbl-grid" style="grid-template-columns: 1fr 1fr;"><div class="tbl-hdr-cell" style="">A</div><div class="tbl-hdr-cell" style="">B</div></div>',
  )
  assert.equal(
    blocks[1].raw,
    '<div class="tbl-grid tbl-row" style="grid-template-columns: 1fr 1fr;"><div class="tbl-cell" style="">1</div><div class="tbl-cell" style="">2</div></div>',
  )
  assert.equal(
    blocks[2].raw,
    '<div class="tbl-grid tbl-row" style="grid-template-columns: 1fr 1fr;"><div class="tbl-cell" style="">3</div><div class="tbl-cell" style="">4</div></div>',
  )
  // Every cell is one character here, so the content-aware widths below come
  // out equal too (1.00fr each) — same layout, just no longer a hardcoded "1".
  // The floor is minmax()'d in regardless, sized off each 1-char header label.
  for (const b of blocks) {
    assert.match(b.html, /grid-template-columns: minmax\(6ch, 1\.00fr\) minmax\(6ch, 1\.00fr\);/)
  }
})

// Regression: a table with one short label column and one paragraph-length
// column (e.g. a sub-processor table's "Purpose" column) got forced into the
// same width as the short column, wrapping into an unreadably tall row — the
// editor's real <table> sizes columns from content and looked fine, but the
// read-only renderer's equal-1fr grid didn't. The long column must now get
// noticeably more of the grid than the short one, while `raw` (the comment
// hash anchor) stays the old equal-width string regardless.
test('a column with a long cell gets more width than a column of short cells', () => {
  const long = 'x'.repeat(400)
  const md = `| Third party | Purpose |\n| --- | --- |\n| Acme | ${long} |\n`
  const blocks = buildContentBlocks(md)
  const [, row] = blocks
  assert.match(row.raw, /grid-template-columns: 1fr 1fr;/)
  const widths = row.html.match(/grid-template-columns: minmax\(\d+ch, ([\d.]+)fr\) minmax\(\d+ch, ([\d.]+)fr\);/)
  assert.ok(widths, `expected two minmax(...) widths in: ${row.html}`)
  const [, shortCol, longCol] = widths.map(Number)
  assert.ok(longCol > shortCol * 3, `expected the long column (${longCol}fr) to dominate the short one (${shortCol}fr)`)
})

// Regression (review finding F1 on #391): a bare `fr` track is really
// `minmax(auto, fr)`, and that automatic "auto" minimum is the grid item's
// own min-content size — a header label's own minimum, specifically, since
// .tbl-hdr-cell has no min-width:0 (see the CSS). Since every row is its own
// independent grid, only the header's grid would widen a narrow column past
// its template share while body rows shrink to the template's literal
// share — the header and body columns would drift apart, even though they
// share one template string. The explicit minmax() floor must be wide
// enough for the header's own label (plus some buffer for its own padding
// and uppercase/letter-spacing), not just "1" like the old flat weight
// floor, or min-width:0 on .tbl-hdr-cell (needed so that explicit floor is
// the one that governs) would let a long header label truncate instead.
test('a short column still reserves enough width for its own header label', () => {
  const md = '| Transfer mechanism | Note |\n| --- | --- |\n| SCC | ok |\n'
  const blocks = buildContentBlocks(md)
  const [header] = blocks
  const widths = header.html.match(/grid-template-columns: minmax\((\d+)ch, [\d.]+fr\) minmax\((\d+)ch, [\d.]+fr\);/)
  assert.ok(widths, `expected two minmax(...) floors in: ${header.html}`)
  const [, firstFloor] = widths.map(Number)
  // "Transfer mechanism" is 19 characters — the floor must clear that by a
  // real margin, not just equal it, to leave room for the uppercase label's
  // own padding and letter-spacing.
  assert.ok(firstFloor > 19, `expected the "Transfer mechanism" column's floor (${firstFloor}ch) to clear its own 19-character label`)
})
