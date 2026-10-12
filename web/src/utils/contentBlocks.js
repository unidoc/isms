// Splits rendered document HTML into paragraph-level "blocks" for the
// document viewer: one block per heading/paragraph/pre/hr, one per list item,
// one per table row, one per blockquote paragraph. Each block is what gets an
// inline-comment "+" button and a stable #ph<hash> anchor.
//
// Extracted from Documents.vue's `contentBlocks` computed so this can be unit
// tested directly — the logic runs against a DOM (document.createElement),
// which node --test only has via jsdom in the test file, not via importing
// the .vue component itself.
import { parseMd } from '../composables/useRenderMd.js'

// Equal-width columns forced every column to the same share of the table, so
// one long cell (e.g. a paragraph-length description next to short labels)
// got squeezed into the same width as a two-word column — wrapping into an
// unreadably tall row, unlike the editor's real <table>, which sizes columns
// from their content. Weight each column by its longest cell instead,
// compressed through sqrt so one huge cell can't swallow the whole table,
// with a floor so a column of short cells still gets a sane minimum share.
//
// Every row (header included) is its own independent CSS grid — not nested
// under one shared <table> — so columns only line up across rows because
// every grid resolves this same template string to the same pixel widths.
// A bare `fr` track is really `minmax(auto, fr)`, and that automatic `auto`
// minimum is the grid ITEM's own min-content size unless the item opts out
// with `min-width: 0` (which .tbl-cell/.tbl-hdr-cell now both do, in both
// renderers' CSS) — without that, a header's uppercase, letter-spaced,
// unbreakable label can force its own grid to resolve a track wider than
// its fr share while a short body cell's row shrinks all the way down to
// the template's literal share. Once that automatic minimum is suppressed,
// an explicit `minmax(Nch, fr)` floor per column replaces it with a floor
// every grid agrees on.
//
// That floor has to be small: it comes straight out of the long column's
// budget (grid track sizing freezes every track below its floor, then gives
// whatever's left to the unfrozen `fr` tracks — so a few ch of padding on
// each short column is a few ch directly off the one column the whole fix
// is for). The first attempt at this floor sized itself off each column's
// *whole* header label (e.g. "Categories of data subjects", all 28
// characters), which — measured live (review #2) — ate nearly the entire
// row width in a typical ~640-680px content column, undoing most of the fix
// it was meant to protect. Headers wrap by word like any other block of
// text (no white-space:nowrap on .tbl-hdr-cell), so the floor only needs to
// fit the longest WORD, not the whole label — a multi-word header is free
// to wrap onto a second line instead of forcing its column wide enough for
// every word on one.
//
// Per-character sizing is calibrated against a live measurement, not an
// estimate: a single-word header's real min-content width (letter-spacing,
// uppercase, bold, padding, all included) came back at 87-100px for 7-8
// character labels (COMPANY/SERVICE/LOCATION/TRANSFER) against this
// renderer's own grid-template-columns `ch` unit, which resolves at ~9.44px
// (alip's own figure, from four floors summing to "about 680px"). Net of the
// ~32px/3.5ch of the cell's own horizontal padding, that lands within a
// character of 1 header character ≈ 1 of this grid's `ch` units — close
// enough, across two different font-size contexts (the header's own 12px
// vs. the grid container's ambient 15px, which is what `ch` in
// grid-template-columns actually resolves against, since that property
// lives on the container, not the header cell), that no separate
// uppercase/bold/letter-spacing multiplier is needed on top of it.
//
// A column holding a paragraph-length cell (LONG_CELL_CHARS or more) never
// shrinks below LONG_COLUMN_FLOOR_CH, however narrow the table's box is: past
// that the table scrolls sideways instead (see the scroll wrapper in the
// renderers), because a column squeezed to a few characters is unreadable.
// Every other column keeps the header-word floor above, so a table that fits
// today lays out exactly as before.
export const LONG_CELL_CHARS = 40
export const LONG_COLUMN_FLOOR_CH = 18

// A body cell is 14px text, not the header's 12px uppercase label, and `ch` in
// the template resolves at the grid's 15px (9.45px), so a body word needs its
// own width. In a browser it is measured for real with canvas measureText, in
// the page's font at the cell's 14px; a floor grown from an over-estimate would
// move tables that already fit, so measured widths matter. Without canvas
// (the node tests) it falls back to per-character widths measured live in
// Chromium: capitals ~10px, M/W/m/w ~12px, the rest ~7.8px. Words split on
// whitespace only, so "R-1" stays one word.
const WORD_PX = { wide: 12, upper: 10, other: 7.8 }
const FALLBACK_CH_PX = 9.45 // "0" at 15px in the Mac system font
const BODY_CELL_PAD_PX = 33 // 2 x 1rem padding + 1px of slack
// Canvas context, font family and the grid's `ch` are read once. `ch` in
// grid-template-columns resolves against the container's ambient 15px font and
// differs per platform (Mac, DejaVu/Liberation, Segoe), so it is read from a
// live `width: 1ch` probe at 15px in the page's font. A canvas measureText("0")
// is not used for it: in Chromium it came out 2.4% narrower than the DOM's 1ch
// (9.21px against 9.44px), while 14px words measure identically in both.
let measureCtx
let fontFamily
let gridChPx = FALLBACK_CH_PX
function initMeasure() {
  measureCtx = null
  if (typeof OffscreenCanvas !== 'undefined' && typeof getComputedStyle === 'function') {
    measureCtx = new OffscreenCanvas(1, 1).getContext('2d')
    fontFamily = getComputedStyle(document.body).fontFamily
    const probe = document.createElement('div')
    probe.style.cssText = 'position:absolute;visibility:hidden;width:1ch;height:0;font-size:15px'
    document.body.appendChild(probe)
    gridChPx = probe.getBoundingClientRect().width || FALLBACK_CH_PX
    probe.remove()
  }
}
function measure(word, weight) {
  if (measureCtx === undefined) initMeasure()
  if (measureCtx) {
    measureCtx.font = `${weight} 14px ${fontFamily}`
    return measureCtx.measureText(word).width
  }
  let px = 0
  for (const c of word) {
    px += /[MWmw]/.test(c) ? WORD_PX.wide : /[A-Z]/.test(c) ? WORD_PX.upper : WORD_PX.other
  }
  return px
}
// Header labels are 12px, bold, uppercase and letter-spaced (0.04em), so the
// `longest word + 3.5` ch formula above only holds for the font it was
// calibrated on: in a font whose capitals are wide relative to its "0" a label
// breaks mid-word. Measured here the same way as body words; 0 without canvas,
// and the result only ever raises the floor above that formula.
function headerWordPx(text) {
  if (measureCtx === undefined) initMeasure()
  if (!measureCtx || !('letterSpacing' in measureCtx)) return 0
  measureCtx.font = `600 12px ${fontFamily}`
  measureCtx.letterSpacing = '0.48px'
  let widest = 0
  for (const word of text.split(/\s+/).filter(Boolean)) {
    widest = Math.max(widest, measureCtx.measureText(word.slice(0, LONG_COLUMN_FLOOR_CH).toUpperCase()).width)
  }
  measureCtx.letterSpacing = '0px'
  return widest
}

// Widest ordinary word of a body cell; text inside <strong>/<b> is measured
// bold, since .tbl-cell strong is 600. Only the first LONG_COLUMN_FLOOR_CH
// characters of a word count, as a longer token is capped and may wrap.
function bodyWordPx(cell) {
  let widest = 0
  const add = (text, weight) => {
    for (const word of text.split(/\s+/).filter(Boolean)) {
      widest = Math.max(widest, measure(word.slice(0, LONG_COLUMN_FLOOR_CH), weight))
    }
  }
  add(cell.textContent || '', 400)
  for (const bold of cell.querySelectorAll('strong, b')) add(bold.textContent || '', 600)
  return widest
}

// Same per-column template as columnWidths, plus the sum of the floors. Every
// grid of a table gets that sum as its min-width, so the grid box is as wide
// as its tracks — a table wider than its box then scrolls as one unit instead
// of clipping its header or painting past its own edge.
export function columnLayout(colCount, ths, ...rowCellGroups) {
  const maxLen = Array(colCount).fill(0)
  const headerLongestWord = Array(colCount).fill(0)
  const bodyWidest = Array(colCount).fill(0)
  const headerWidest = Array(colCount).fill(0)
  ths.forEach((cell, i) => {
    if (i >= colCount) return
    const text = (cell.textContent || '').trim()
    maxLen[i] = Math.max(maxLen[i], text.length)
    headerWidest[i] = headerWordPx(text)
    headerLongestWord[i] = text.split(/\s+/).filter(Boolean)
      .reduce((longest, word) => Math.max(longest, word.length), 0)
  })
  for (const cells of rowCellGroups) {
    cells.forEach((cell, i) => {
      if (i >= colCount) return
      const text = (cell.textContent || '').trim()
      maxLen[i] = Math.max(maxLen[i], text.length)
      bodyWidest[i] = Math.max(bodyWidest[i], bodyWordPx(cell))
    })
  }
  let minCh = 0
  const template = maxLen.map((n, i) => {
    const weight = Math.max(Math.sqrt(n), 1).toFixed(2)
    let floorCh = Math.ceil(headerLongestWord[i] + 3.5)
    if (headerWidest[i] > 0) {
      const hdrCh = Math.ceil((headerWidest[i] + BODY_CELL_PAD_PX) / gridChPx) // + 1px right border
      floorCh = Math.max(floorCh, Math.min(hdrCh, LONG_COLUMN_FLOOR_CH))
    }
    // Fit the longest ordinary body word, capped: a longer token (URL, long
    // identifier) wraps mid-word instead of widening the column further.
    if (bodyWidest[i] > 0) {
      const bodyCh = Math.ceil((bodyWidest[i] + BODY_CELL_PAD_PX) / gridChPx)
      floorCh = Math.max(floorCh, Math.min(bodyCh, LONG_COLUMN_FLOOR_CH))
    }
    if (n >= LONG_CELL_CHARS) floorCh = Math.max(floorCh, LONG_COLUMN_FLOOR_CH)
    minCh += floorCh
    return `minmax(${floorCh}ch, ${weight}fr)`
  }).join(' ')
  return { template, minCh }
}

// Exported so DocumentViewer.vue's copy of this same table-rendering logic
// shares the exact formula rather than drifting from it.
export function columnWidths(colCount, ths, ...rowCellGroups) {
  return columnLayout(colCount, ths, ...rowCellGroups).template
}

// Turns one <table> element into its grid-row blocks (header + one per body
// row), shared by Documents.vue and DocumentViewer.vue. `startIndex` is the
// index the first block will get; it names the table (`tableId`) so the
// renderers can keep every row of one table scrolled together. `tableId` and
// `tableLast` are layout-only: `raw`, not them, is what comments hash.
export function tableBlocks(table, startIndex) {
  // Header rows are found by their cells, not by a <thead> wrapper: tables authored
  // in the editor are stored as raw Tiptap HTML, which puts the <th> row directly in
  // <tbody> and emits no <thead> at all. A row mixing <th> and <td> is a body row with
  // a row-header cell, not a table header, so it stays in `rows`.
  const allRows = Array.from(table.querySelectorAll('tr'))
  const headerRows = allRows.filter(tr => tr.querySelector('th') && !tr.querySelector('td'))
  const ths = headerRows.flatMap(tr => Array.from(tr.querySelectorAll('th')))
  const rows = allRows.filter(tr => !headerRows.includes(tr))
  const colCount = ths.length || (rows[0] ? rows[0].children.length : 1)
  // `raw` keeps every row hashed against the pre-existing equal-width
  // markup (see addBlock) — every table block ever saved was hashed
  // against this exact string, so switching to content-aware widths must
  // not change what gets hashed, or every inline comment on every
  // existing table silently detaches.
  const equalGridCols = 'grid-template-columns: ' + Array(colCount).fill('1fr').join(' ') + ';'
  const { template, minCh } = columnLayout(colCount, ths, ...rows.map(tr => Array.from(tr.querySelectorAll('td'))))
  // +2px: the grid's own 1px left/right borders sit inside its border box.
  const gridCols = `grid-template-columns: ${template}; min-width: calc(${minCh}ch + 2px);`
  const tableId = 'tbl-' + startIndex
  const out = []
  const push = (html, tag, text, raw) => out.push({ html, tag, text, raw, tableId })

  // Header as one block
  if (ths.length > 0) {
    const headerCells = ths.map(th => {
      const styleAttr = th.getAttribute('style') || ''
      return `<div class="tbl-hdr-cell" style="${styleAttr}">${th.innerHTML}</div>`
    }).join('')
    const rawHeader = `<div class="tbl-grid" style="${equalGridCols}">${headerCells}</div>`
    push(`<div class="tbl-grid" style="${gridCols}">${headerCells}</div>`, 'thead', headerRows.map(tr => tr.textContent).join(''), rawHeader)
  }

  // Each body row as separate block — gets its own "+" button!
  for (const tr of rows) {
    const tds = Array.from(tr.querySelectorAll('td'))
    const cells = tds.map(td => {
      const styleAttr = td.getAttribute('style') || ''
      const extraClass = td.classList.contains('has-text-color') ? ' has-text-color' : ''
      return `<div class="tbl-cell${extraClass}" style="${styleAttr}">${td.innerHTML}</div>`
    }).join('')
    const rawRow = `<div class="tbl-grid tbl-row" style="${equalGridCols}">${cells}</div>`
    push(`<div class="tbl-grid tbl-row" style="${gridCols}">${cells}</div>`, 'tr', tr.textContent || '', rawRow)
  }
  // The scroller's scrollbar shows on one block only (the last row).
  if (out.length > 0) out[out.length - 1].tableLast = true
  return out
}

export function buildContentBlocks(rawContent) {
  if (!rawContent) return []
  const html = parseMd(rawContent)
  const div = document.createElement('div')
  div.innerHTML = html
  const blocks = []

  function addBlock(html, tag, text, raw) {
    const block = { index: blocks.length, html, tag, text }
    // useDocumentComments.blockHash hashes `raw ?? html` to anchor inline
    // comments; commentsForBlock hard-rejects on a hash mismatch, with no
    // index fallback once a comment carries a hash. `raw` lets a block's
    // rendered markup change (e.g. gaining `start=`) without re-hashing and
    // silently detaching every comment already stored against it.
    if (raw !== undefined) block.raw = raw
    blocks.push(block)
  }

  for (const child of div.children) {
    const tag = child.tagName.toLowerCase()

    // Split lists — each bullet is commentable. Each <li> ends up alone in
    // its own fresh <ol>, which resets an ordered list's visible number to 1
    // for every item — so the wrapper carries a `start` tracking this item's
    // real position in the original list, rather than a bare "<ol>" every
    // time. `raw` keeps the hashed markup at the pre-`start` shape (see
    // addBlock) so this doesn't detach comments already anchored to it.
    if ((tag === 'ul' || tag === 'ol') && child.children.length > 0) {
      const parsedStart = parseInt(child.getAttribute('start'), 10)
      const start = tag === 'ol' && Number.isFinite(parsedStart) ? parsedStart : 1
      let position = 0
      for (const li of child.children) {
        if (li.tagName.toLowerCase() === 'li') {
          const openTag = tag === 'ol' ? `<ol start="${start + position}">` : '<ul>'
          const rawWrapper = '<' + tag + '>' + li.outerHTML + '</' + tag + '>'
          addBlock(openTag + li.outerHTML + '</' + tag + '>', 'li', li.textContent || '', rawWrapper)
          position++
        }
      }
    }
    // Tables — convert to grid-based rows so each gets a "+" button
    else if (tag === 'table') {
      for (const { html, tag: rowTag, text, raw, ...layout } of tableBlocks(child, blocks.length)) {
        addBlock(html, rowTag, text, raw)
        Object.assign(blocks[blocks.length - 1], layout)
      }
    }
    // Split blockquotes — each paragraph inside is commentable
    else if (tag === 'blockquote' && child.children.length > 1) {
      for (const bqChild of child.children) {
        addBlock('<blockquote>' + bqChild.outerHTML + '</blockquote>', 'blockquote-p', bqChild.textContent || '')
      }
    }
    // Everything else — headings, paragraphs, pre, hr — one block each
    else {
      addBlock(child.outerHTML, tag, child.textContent || '')
    }
  }
  return blocks
}
