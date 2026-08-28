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
      const thead = child.querySelector('thead')
      const tbody = child.querySelector('tbody')
      const ths = thead ? Array.from(thead.querySelectorAll('th')) : []
      const rows = tbody ? Array.from(tbody.querySelectorAll('tr')) : []
      const colCount = ths.length || (rows[0] ? rows[0].children.length : 1)
      const gridCols = 'grid-template-columns: ' + Array(colCount).fill('1fr').join(' ') + ';'

      // Header as one block
      if (ths.length > 0) {
        const headerCells = ths.map(th => {
          const styleAttr = th.getAttribute('style') || ''
          return `<div class="tbl-hdr-cell" style="${styleAttr}">${th.innerHTML}</div>`
        }).join('')
        addBlock(`<div class="tbl-grid" style="${gridCols}">${headerCells}</div>`, 'thead', thead.textContent || '')
      }

      // Each body row as separate block — gets its own "+" button!
      for (const tr of rows) {
        const tds = Array.from(tr.querySelectorAll('td'))
        const cells = tds.map(td => {
          const styleAttr = td.getAttribute('style') || ''
          return `<div class="tbl-cell" style="${styleAttr}">${td.innerHTML}</div>`
        }).join('')
        addBlock(`<div class="tbl-grid tbl-row" style="${gridCols}">${cells}</div>`, 'tr', tr.textContent || '')
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
