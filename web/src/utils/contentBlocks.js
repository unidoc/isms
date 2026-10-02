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
// an explicit `minmax(Nch, fr)` floor per column — sized to that column's
// own header label — replaces it with a floor every grid agrees on, so the
// header is never asked to go narrower than its own label needs, and
// narrow columns don't get truncated by the sqrt-compressed share alone.
//
// Exported so DocumentViewer.vue's copy of this same table-rendering logic
// shares the exact formula rather than drifting from it.
export function columnWidths(colCount, ths, ...rowCellGroups) {
  const maxLen = Array(colCount).fill(0)
  const headerLen = Array(colCount).fill(0)
  ths.forEach((cell, i) => {
    if (i >= colCount) return
    const len = (cell.textContent || '').trim().length
    headerLen[i] = len
    maxLen[i] = Math.max(maxLen[i], len)
  })
  for (const cells of rowCellGroups) {
    cells.forEach((cell, i) => {
      if (i < colCount) maxLen[i] = Math.max(maxLen[i], (cell.textContent || '').trim().length)
    })
  }
  return maxLen.map((n, i) => {
    const weight = Math.max(Math.sqrt(n), 1).toFixed(2)
    // The header's own padding plus its uppercase, letter-spaced, bold
    // transform all widen it well past a plain `ch` (the '0' glyph's own
    // width) per character — this is a deliberately generous approximation,
    // not a measurement, so it errs toward a column being a little wider
    // than the label strictly needs rather than risking a cut-off word.
    const floorCh = Math.max(Math.ceil(headerLen[i] * 1.3) + 4, 3)
    return `minmax(${floorCh}ch, ${weight}fr)`
  }).join(' ')
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
      // Header rows are found by their cells, not by a <thead> wrapper: tables authored
      // in the editor are stored as raw Tiptap HTML, which puts the <th> row directly in
      // <tbody> and emits no <thead> at all. A row mixing <th> and <td> is a body row with
      // a row-header cell, not a table header, so it stays in `rows`.
      const allRows = Array.from(child.querySelectorAll('tr'))
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
      const gridCols = 'grid-template-columns: ' +
        columnWidths(colCount, ths, ...rows.map(tr => Array.from(tr.querySelectorAll('td')))) + ';'

      // Header as one block
      if (ths.length > 0) {
        const headerCells = ths.map(th => {
          const styleAttr = th.getAttribute('style') || ''
          return `<div class="tbl-hdr-cell" style="${styleAttr}">${th.innerHTML}</div>`
        }).join('')
        const rawHeader = `<div class="tbl-grid" style="${equalGridCols}">${headerCells}</div>`
        addBlock(`<div class="tbl-grid" style="${gridCols}">${headerCells}</div>`, 'thead', headerRows.map(tr => tr.textContent).join(''), rawHeader)
      }

      // Each body row as separate block — gets its own "+" button!
      for (const tr of rows) {
        const tds = Array.from(tr.querySelectorAll('td'))
        const cells = tds.map(td => {
          const styleAttr = td.getAttribute('style') || ''
          return `<div class="tbl-cell" style="${styleAttr}">${td.innerHTML}</div>`
        }).join('')
        const rawRow = `<div class="tbl-grid tbl-row" style="${equalGridCols}">${cells}</div>`
        addBlock(`<div class="tbl-grid tbl-row" style="${gridCols}">${cells}</div>`, 'tr', tr.textContent || '', rawRow)
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
