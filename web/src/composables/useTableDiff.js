// Structural, cell-level diff for tables in the review diff view (#6).
//
// Tables are stored as raw HTML (see useMarkdownConvert `alwaysHtmlTable`) to
// preserve colors/formatting/complex cells. Diffing that HTML as text — or
// word-diffing the tag soup — is unreadable: a one-line `<table>…</table>` reads
// as a wholesale replace. Worse, the first WYSIWYG edit converts a markdown pipe
// table into HTML, so a single cell tweak looks like the entire table changed.
//
// Instead we render both sides to a table DOM (markdown pipe tables are rendered
// to HTML first, so the md→HTML conversion is NOT shown as a change), match rows
// by a key column (first cell text, index fallback), and highlight only what
// actually changed — word-level inside each changed cell — while keeping every
// unchanged cell's own markup and colors. The output is the NEW table's markup,
// so styling/colgroup/colors are preserved.

import DOMPurify from 'dompurify'
import { tokenize, diffTokens, escapeHtml } from './useWordDiff'
import { parseMd } from './useRenderMd'

const HTML_TABLE_RE = /^\s*<table[\s>]/i
// A markdown pipe table: a row with a pipe followed by a `|---|` separator line.
const MD_TABLE_RE = /\|[^\n]*\n\s*\|?[\s:|-]*-{3,}/

export function isHtmlTable(text) {
  return HTML_TABLE_RE.test(text || '')
}

// True for either an HTML table or a markdown pipe table — used to detect table
// blocks before diffing them.
export function isTableBlock(text) {
  return isHtmlTable(text) || (text.includes('|') && MD_TABLE_RE.test(text))
}

// Normalise either an HTML table or a markdown pipe table to a <table> element.
// Returns null when the text doesn't contain a table.
function toTableEl(text) {
  const html = isHtmlTable(text) ? text : parseMd(text)
  const doc = new DOMParser().parseFromString(html, 'text/html')
  return doc.querySelector('table')
}

function rowCells(tr) {
  return Array.from(tr.children).filter(el => el.tagName === 'TD' || el.tagName === 'TH')
}

function isHeaderRow(tr) {
  const cells = rowCells(tr)
  return cells.length > 0 && cells.every(c => c.tagName === 'TH')
}

function rowKey(tr) {
  const cells = rowCells(tr)
  return cells.length ? cells[0].textContent.trim() : ''
}

function cellWordDiff(oldText, newText) {
  return diffTokens(tokenize(oldText), tokenize(newText))
    .map(op => op.type === 'equal' ? escapeHtml(op.text)
      : op.type === 'remove' ? `<del class="tc-word-del">${escapeHtml(op.text)}</del>`
        : `<ins class="tc-word-ins">${escapeHtml(op.text)}</ins>`)
    .join('')
}

// Diff two tables (each given as HTML or markdown). Returns sanitised HTML for
// the merged, highlighted table, or null when either side isn't a table — so
// the caller can fall back to its normal (prose) diff path.
export function diffTables(oldText, newText) {
  const oldTable = toTableEl(oldText)
  const newTable = toTableEl(newText)
  if (!oldTable || !newTable) return null

  // Index old rows by key (first-cell text); duplicate keys keep insertion order
  // so identical-key rows still pair up in order. Header rows are matched too.
  const oldRows = Array.from(oldTable.querySelectorAll('tr'))
  const oldByKey = new Map()
  for (const tr of oldRows) {
    const k = rowKey(tr)
    if (!oldByKey.has(k)) oldByKey.set(k, [])
    oldByKey.get(k).push(tr)
  }
  const consumed = new Set()

  for (const tr of newTable.querySelectorAll('tr')) {
    const bucket = oldByKey.get(rowKey(tr))
    let oldTr = null
    if (bucket) { for (const cand of bucket) { if (!consumed.has(cand)) { oldTr = cand; consumed.add(cand); break } } }

    if (!oldTr) {
      // A genuinely new row — but don't flag a header row as "added content".
      if (!isHeaderRow(tr)) tr.classList.add('tc-row-add')
      continue
    }

    // Matched row — diff its cells by position.
    const newCells = rowCells(tr)
    const oldCells = rowCells(oldTr)
    for (let ci = 0; ci < newCells.length; ci++) {
      const nc = newCells[ci]
      const oc = oldCells[ci]
      if (!oc) { nc.classList.add('tc-cell-add'); continue }
      const oldCellText = oc.textContent.trim()
      const newCellText = nc.textContent.trim()
      if (oldCellText !== newCellText) {
        nc.innerHTML = cellWordDiff(oldCellText, newCellText)
        nc.classList.add('tc-cell-change')
      }
    }
  }

  // Old rows never matched were removed — append them (skip headers) so the
  // author still sees what was dropped. (v1: removed rows render at the end.)
  let removedHtml = ''
  for (const tr of oldRows) {
    if (consumed.has(tr) || isHeaderRow(tr)) continue
    tr.classList.add('tc-row-del')
    removedHtml += tr.outerHTML
  }
  if (removedHtml) {
    const tbody = newTable.querySelector('tbody') || newTable
    tbody.insertAdjacentHTML('beforeend', removedHtml)
  }

  return DOMPurify.sanitize(newTable.outerHTML, {
    ADD_TAGS: ['del', 'ins', 'colgroup', 'col'],
    ADD_ATTR: ['class', 'colspan', 'rowspan', 'style'],
  })
}
