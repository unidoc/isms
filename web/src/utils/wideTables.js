// Real <table>s in the diff views (Reviews -> Changes) sit in narrow panes.
// Left alone, auto table layout squeezes the one long-text column down to its
// longest word and then spills the table past its pane, so the whole pane
// scrolls sideways. This wraps every table in its own horizontal scroller and
// gives long-text columns the same readable floor the grid renderers use, so a
// table that does not fit scrolls by itself instead.
import { LONG_CELL_CHARS, LONG_COLUMN_FLOOR_CH } from './contentBlocks.js'

// A border-box min-width includes the cell's 2rem of padding, and `ch` is per
// cell font (14px here, 15px in the grids), so +1rem lands the column near the
// grids' ~170px without making a table that fits today start to scroll.
const FLOOR = `calc(${LONG_COLUMN_FLOOR_CH}ch + 1rem)`

// Wraps each table once (a table already in a scroller is left alone) and
// floors its long-text columns. Returns the input untouched when it has no table.
export function wrapTables(html) {
  if (!html || !html.includes('<table')) return html
  const div = document.createElement('div')
  div.innerHTML = html
  for (const table of Array.from(div.querySelectorAll('table'))) {
    if (table.parentElement && table.parentElement.classList.contains('tbl-wide-scroll')) continue
    const rows = Array.from(table.querySelectorAll('tr'))
    const longest = []
    for (const tr of rows) {
      Array.from(tr.children).forEach((cell, i) => {
        longest[i] = Math.max(longest[i] || 0, (cell.textContent || '').trim().length)
      })
    }
    for (const tr of rows) {
      Array.from(tr.children).forEach((cell, i) => {
        if (longest[i] < LONG_CELL_CHARS) return
        const style = (cell.getAttribute('style') || '').trim()
        const sep = style && !style.endsWith(';') ? '; ' : style ? ' ' : ''
        cell.setAttribute('style', `${style}${sep}min-width: ${FLOOR};`)
      })
    }
    const wrap = document.createElement('div')
    wrap.className = 'tbl-wide-scroll'
    table.parentNode.insertBefore(wrap, table)
    wrap.appendChild(table)
  }
  return div.innerHTML
}
