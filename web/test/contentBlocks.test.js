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
    assert.match(b.html, /grid-template-columns: minmax\(5ch, 1\.00fr\) minmax\(5ch, 1\.00fr\);/)
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

// Regression (review finding F1 on #391, part 1): a bare `fr` track is
// really `minmax(auto, fr)`, and that automatic "auto" minimum is the grid
// item's own min-content size — a header label's own minimum, specifically,
// since .tbl-hdr-cell has no min-width:0 (see the CSS). Since every row is
// its own independent grid, only the header's grid would widen a narrow
// column past its template share while body rows shrink to the template's
// literal share — the header and body columns would drift apart, even
// though they share one template string. The explicit minmax() floor must
// be wide enough for the header's own label, not just "1" like the old flat
// weight floor, or min-width:0 on .tbl-hdr-cell (needed so that explicit
// floor is the one that governs) would let a header label truncate instead.
test('a single-word header still reserves enough width for its own label', () => {
  const md = '| Transfer | Note |\n| --- | --- |\n| SCC | ok |\n'
  const blocks = buildContentBlocks(md)
  const [header] = blocks
  const widths = header.html.match(/grid-template-columns: minmax\((\d+)ch, [\d.]+fr\) minmax\((\d+)ch, [\d.]+fr\);/)
  assert.ok(widths, `expected two minmax(...) floors in: ${header.html}`)
  const [, firstFloor] = widths.map(Number)
  // "Transfer" is 8 characters — the floor must clear that by a real margin,
  // not just equal it, to leave room for the uppercase label's own padding
  // and letter-spacing.
  assert.ok(firstFloor > 8, `expected the "Transfer" column's floor (${firstFloor}ch) to clear its own 8-character label`)
})

// Regression (review finding F1 on #391, part 2 — the fix's own first
// attempt regressed this): sizing the floor off a header's whole label,
// rather than its longest word, measured live (review #2) at nearly the
// entire width of a typical content column — undoing most of the #391 fix
// for exactly the tables it targets. .tbl-hdr-cell has no white-space:nowrap,
// so headers wrap by word like any other text; a multi-word header's floor
// only needs to fit its longest word, which is free to sit alone on its own
// line, not the full label on one line.
test("a multi-word header's floor follows its longest word, not the whole label", () => {
  const md = '| Categories of data subjects | Note |\n| --- | --- |\n| Employees | ok |\n'
  const blocks = buildContentBlocks(md)
  const [header] = blocks
  const widths = header.html.match(/grid-template-columns: minmax\((\d+)ch, [\d.]+fr\) minmax\((\d+)ch, [\d.]+fr\);/)
  assert.ok(widths, `expected two minmax(...) floors in: ${header.html}`)
  const [, firstFloor] = widths.map(Number)
  // Longest word is "Categories" (10 chars) — the floor must clear that, but
  // must stay well short of the full 28-character label.
  assert.ok(firstFloor > 10, `expected the floor (${firstFloor}ch) to clear "Categories" (10 chars)`)
  assert.ok(firstFloor < 20, `expected the floor (${firstFloor}ch) to stay well short of the full 28-character label`)
})

// Regression (review finding F1 on #391, part 3): grid track sizing freezes
// every column below its floor, then gives whatever's left to the unfrozen
// `fr` tracks — so floor padding on the short columns comes directly out of
// the long column's share. Pin down that the short columns in the exact
// table from the bug report (#391) don't together eat a typical ~640-680px
// content column (alip's own measured figure, review #2: "about 680px" is
// ~72ch in this renderer's grid-template-columns `ch` unit), leaving the
// long Purpose column meaningful room.
test("the sub-processor table's short-column floors leave the long column real room", () => {
  const md =
    '| Third party | Purpose | Applicable service | US data centre location | EU data centre location |\n' +
    '| --- | --- | --- | --- | --- |\n' +
    '| Acme | Does a thing | Acme Cloud | United States | Europe |\n'
  const blocks = buildContentBlocks(md)
  const [header] = blocks
  const floors = [...header.html.matchAll(/minmax\((\d+)ch,/g)].map((m) => Number(m[1]))
  assert.equal(floors.length, 5, `expected 5 column floors, got: ${header.html}`)
  const [thirdParty, , service, usLocation, euLocation] = floors // index 1 ("Purpose") is the long column, excluded
  const shortFloorSum = thirdParty + service + usLocation + euLocation
  assert.ok(shortFloorSum < 55, `expected the 4 short columns' floors (${shortFloorSum}ch total) to leave real room in a ~72ch budget`)
})

// Table blocks carry two layout-only fields next to `raw`: a `tableId` shared
// by every block of one table (the renderers keep a table's rows scrolled
// together by it) and `tableLast` on the final row (the one that shows the
// scrollbar). Neither is hashed.
const wideMd = 'Intro\n\n| Company | Purpose |\n| --- | --- |\n| Acme | Processing of customer support requests and account administration |\n| Beta | Short |\n\nMid\n\n| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |\n'

test('blocks of one table share a tableId and different tables do not', () => {
  const blocks = buildContentBlocks(wideMd)
  assert.deepEqual(blocks.map((b) => b.tag), ['p', 'thead', 'tr', 'tr', 'p', 'thead', 'tr'])
  const ids = blocks.map((b) => b.tableId)
  assert.equal(ids[0], undefined)
  assert.equal(ids[4], undefined)
  assert.ok(ids[1])
  assert.equal(ids[1], ids[2])
  assert.equal(ids[1], ids[3])
  assert.ok(ids[5])
  assert.equal(ids[5], ids[6])
  assert.notEqual(ids[1], ids[5])
  assert.deepEqual(blocks.map((b) => !!b.tableLast), [false, false, false, true, false, false, true])
})

test('table block raw and order are byte-identical to the pre-scroll output', () => {
  const blocks = buildContentBlocks(wideMd)
  assert.deepEqual(blocks.map((b) => b.index), [0, 1, 2, 3, 4, 5, 6])
  assert.equal(
    blocks[1].raw,
    '<div class="tbl-grid" style="grid-template-columns: 1fr 1fr;"><div class="tbl-hdr-cell" style="">Company</div><div class="tbl-hdr-cell" style="">Purpose</div></div>',
  )
  assert.equal(
    blocks[2].raw,
    '<div class="tbl-grid tbl-row" style="grid-template-columns: 1fr 1fr;"><div class="tbl-cell" style="">Acme</div><div class="tbl-cell" style="">Processing of customer support requests and account administration</div></div>',
  )
  assert.equal(
    blocks[3].raw,
    '<div class="tbl-grid tbl-row" style="grid-template-columns: 1fr 1fr;"><div class="tbl-cell" style="">Beta</div><div class="tbl-cell" style="">Short</div></div>',
  )
  assert.equal(
    blocks[5].raw,
    '<div class="tbl-grid" style="grid-template-columns: 1fr 1fr 1fr;"><div class="tbl-hdr-cell" style="">A</div><div class="tbl-hdr-cell" style="">B</div><div class="tbl-hdr-cell" style="">C</div></div>',
  )
  assert.equal(
    blocks[6].raw,
    '<div class="tbl-grid tbl-row" style="grid-template-columns: 1fr 1fr 1fr;"><div class="tbl-cell" style="">1</div><div class="tbl-cell" style="">2</div><div class="tbl-cell" style="">3</div></div>',
  )
})

// A column whose longest cell is a paragraph (>= 40 characters) keeps a
// readable 18ch floor even when the table has to scroll to honour it. Short
// columns keep the header-longest-word floor and every weight is unchanged.
test('a long-text column gets an 18ch floor; short columns and weights are unchanged', () => {
  const blocks = buildContentBlocks(wideMd)
  const t = blocks[1].html.match(/grid-template-columns: minmax\((\d+)ch, ([\d.]+)fr\) minmax\((\d+)ch, ([\d.]+)fr\);/)
  assert.ok(t, blocks[1].html)
  assert.equal(t[1], '11') // Company: 7 + 3.5 -> 11, short cells only
  assert.equal(t[3], '18') // Purpose: header floor 10, long cell lifts it to 18
  assert.equal(t[4], Math.sqrt('Processing of customer support requests and account administration'.length).toFixed(2))
  assert.equal(t[2], Math.sqrt('Company'.length).toFixed(2))
})

test('a long header word still wins over the 18ch long-text floor', () => {
  const md = `| Extraordinarilylongheaderword | B |\n| --- | --- |\n| ${'x '.repeat(30)} | y |\n`
  const [head] = buildContentBlocks(md)
  assert.match(head.html, /minmax\(33ch, /)
})

test('every grid of a table has the same min-width, the sum of its floors', () => {
  const blocks = buildContentBlocks(wideMd)
  for (const b of blocks.slice(1, 4)) {
    assert.match(b.html, /min-width: calc\(29ch \+ 2px\);/) // 11 + 18
  }
  for (const b of blocks.slice(5)) {
    assert.match(b.html, /min-width: calc\(15ch \+ 2px\);/) // 3 x 5
  }
  // raw stays free of it
  assert.doesNotMatch(blocks[1].raw, /min-width/)
})

test('syncTableScroll copies scrollLeft to the other rows of the same table only', async () => {
  const { syncTableScroll } = await import('../src/utils/tableScroll.js')
  const host = document.createElement('div')
  host.innerHTML = ['a', 'a', 'b', ''].map((id) =>
    `<div class="comment-block" ${id ? `data-table-id="${id}"` : ''}><div class="doc-prose${id ? ' tbl-scroll' : ''}"></div></div>`).join('')
  const scrollers = Array.from(host.querySelectorAll('.doc-prose'))
  scrollers[0].scrollLeft = 120
  syncTableScroll({ target: scrollers[0] })
  assert.equal(scrollers[1].scrollLeft, 120)
  assert.equal(scrollers[2].scrollLeft, 0)
  assert.equal(scrollers[3].scrollLeft, 0)
  syncTableScroll({ target: scrollers[3] }) // not a table row: no-op, no throw
})

// A column's floor also fits its longest ordinary BODY word (body text is
// 14px, not the header label), capped at the long-column floor so a URL wraps
// mid-word instead of widening the column. Words split on whitespace only.
function floors(md) {
  const m = buildContentBlocks(md)[0].html.match(/grid-template-columns: ([^;]+);/)[1]
  return [...m.matchAll(/minmax\((\d+)ch/g)].map((x) => Number(x[1]))
}

test('a body word longer than the header word raises that column\'s floor', () => {
  const [a, b] = floors('| Name | Basis |\n| --- | --- |\n| x | Legitimate |\n')
  assert.equal(a, 8) // header NAME: 4 + 3.5; the body "x" does not raise it
  assert.ok(b > 9, `expected the "Legitimate" column above its header floor (9), got ${b}`)
})

test('the body-word floor is capped for a URL', () => {
  const url = 'https://docs.example.com/policies/information-security/access-control-standard-v3'
  const [, ref] = floors(`| ID | Reference |\n| --- | --- |\n| R-1 | ${url} |\n`)
  assert.equal(ref, 18)
})

test('"R-1" is one 3-character word, not split at the hyphen', () => {
  const [id] = floors('| ID | Ref |\n| --- | --- |\n| R-1 | a |\n')
  // one word of 3 chars (~26px + 34px padding) needs 7ch; split at the hyphen
  // it would only have needed the 6ch header floor
  assert.equal(id, 7)
})
