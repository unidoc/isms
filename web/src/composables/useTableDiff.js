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
import { wordDiffPlain } from './useWordDiff.js'
import { parseMd } from './useRenderMd.js'

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
        nc.innerHTML = wordDiffPlain(oldCellText, newCellText)
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

// --- Markdown pipe tables in the line-level (Unified) diff ---
//
// The unified view diffs line by line, but a markdown pipe table spans many
// lines, so a touched table fell apart: only the changed row became a
// remove/add pair (one line, not a table, so diffTables() returned null), and
// the other rows came out as separate runs rendered as raw `| a | b |` text. An
// untouched table whose rows come from different commits was cut at every blame
// boundary the same way. Editor-saved tables are one HTML line, so they never hit
// this. These helpers regroup the line-level ops so each pipe table is one unit.

const PIPE_SEP_RE = /^\s*\|?(?:\s*:?-{3,}:?\s*\|?)+\s*$/

// Line ranges [start, end) of the markdown pipe tables in `lines`: a header row
// containing a pipe, a `|---|` separator line, then contiguous pipe rows up to a
// blank or non-table line.
export function findPipeTables(lines) {
  const ranges = []
  let i = 0
  while (i < lines.length) {
    if (lines[i].includes('|') && i + 1 < lines.length && lines[i + 1].includes('|') && PIPE_SEP_RE.test(lines[i + 1])) {
      let j = i + 2
      while (j < lines.length && lines[j].trim() !== '' && lines[j].includes('|')) j++
      ranges.push({ start: i, end: j })
      i = j
    } else {
      i++
    }
  }
  return ranges
}

// Pipe tables plus editor-saved tables (one HTML line), in line order. An old
// pipe table that became an HTML table on the first editor save must still pair
// with it as one unit.
function findTables(lines) {
  const ranges = findPipeTables(lines)
  lines.forEach((l, i) => {
    if (isHtmlTable(l) && !ranges.some(r => i >= r.start && i < r.end)) ranges.push({ start: i, end: i + 1 })
  })
  return ranges.sort((a, b) => a.start - b.start)
}

function countChangedCells(html) {
  const doc = new DOMParser().parseFromString(html, 'text/html')
  return doc.querySelectorAll('td.tc-cell-change, .tc-row-add > td, .tc-row-del > td').length
}

// Takes diffLines() ops (runs of equal/add/remove lines) and returns ops where
// every pipe table touched by the diff is one unit:
//   - changed (lines added/removed inside it): { type: 'change', isTable: true,
//     oldText, newText, html (diffTables of the whole old and new table), cellCount }
//   - unchanged / wholly added / wholly removed: the usual { type, text } with
//     `whole: true`, so it is rendered as one block and not split at blame
//     boundaries.
// Anything that is not a clean, complete table on both sides is left as the
// original line ops, so prose (and its word diff) is never handed to diffTables.
export function regroupPipeTables(ops, diffFn = diffTables) {
  const recs = []
  for (const op of ops) for (const text of op.text.split('\n')) recs.push({ type: op.type, text })
  const oldLines = [], newLines = []
  for (const r of recs) {
    if (r.type !== 'add') { r.oldIdx = oldLines.length; oldLines.push(r.text) }
    if (r.type !== 'remove') { r.newIdx = newLines.length; newLines.push(r.text) }
  }
  const oldRanges = findTables(oldLines)
  const newRanges = findTables(newLines)
  const tidOf = (ranges, n) => {
    const t = new Array(n).fill(-1)
    ranges.forEach((r, k) => { for (let x = r.start; x < r.end; x++) t[x] = k })
    return t
  }
  const oldTid = tidOf(oldRanges, oldLines.length)
  const newTid = tidOf(newRanges, newLines.length)
  const tableful = (r) => (r.type !== 'add' && oldTid[r.oldIdx] >= 0) || (r.type !== 'remove' && newTid[r.newIdx] >= 0)

  const out = []
  const pushRaw = (r) => {
    const last = out[out.length - 1]
    if (last && !last.whole && last.type === r.type && last.type !== 'change') last.text += '\n' + r.text
    else out.push({ type: r.type, text: r.text })
  }

  let i = 0
  while (i < recs.length) {
    if (!tableful(recs[i])) { pushRaw(recs[i]); i++; continue }
    let j = i
    while (j < recs.length && tableful(recs[j])) j++
    const cluster = recs.slice(i, j)
    const unit = clusterUnit(cluster, oldRanges, newRanges, oldTid, newTid, diffFn)
    if (unit) out.push(unit)
    else for (const r of cluster) pushRaw(r)
    i = j
  }
  return out
}

function clusterUnit(cluster, oldRanges, newRanges, oldTid, newTid, diffFn) {
  const oldSide = cluster.filter(r => r.type !== 'add')
  const newSide = cluster.filter(r => r.type !== 'remove')
  // Each side must be exactly one whole table.
  const whole = (side, idxKey, tid, ranges) => {
    if (side.length === 0) return true
    const ids = new Set(side.map(r => tid[r[idxKey]]))
    if (ids.size !== 1 || ids.has(-1)) return false
    const range = ranges[[...ids][0]]
    return side.length === range.end - range.start
  }
  if (!whole(oldSide, 'oldIdx', oldTid, oldRanges) || !whole(newSide, 'newIdx', newTid, newRanges)) return null
  const oldText = oldSide.map(r => r.text).join('\n')
  const newText = newSide.map(r => r.text).join('\n')
  if (cluster.every(r => r.type === 'equal')) return { type: 'equal', text: newText, whole: true }
  if (oldSide.length === 0) return { type: 'add', text: newText, whole: true }
  if (newSide.length === 0) return { type: 'remove', text: oldText, whole: true }
  const html = diffFn(oldText, newText)
  if (!html) return null
  return { type: 'change', oldText, newText, html, isTable: true, cellCount: countChangedCells(html) }
}
