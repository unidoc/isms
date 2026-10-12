// Keeps every row of one grid table scrolled together.
//
// Each table row is its own comment block (so it can carry a "+" button and a
// review checkbox outside the scrolling part), and each block's `.doc-prose`
// is its own horizontal scroller. Wired as `@scroll.capture` on the element
// holding the blocks (scroll events don't bubble), this copies the scrolled
// row's scrollLeft to the other rows of the same table.
//
// No re-entrancy flag: a row only writes when its value actually differs, so
// the scroll events our own writes trigger find nothing left to change.
export function syncTableScroll(event) {
  const scroller = event.target
  if (!scroller || !scroller.classList || !scroller.classList.contains('tbl-scroll')) return
  const block = scroller.closest('[data-table-id]')
  if (!block) return
  const left = scroller.scrollLeft
  const rows = block.parentElement.querySelectorAll('[data-table-id]')
  for (const row of rows) {
    if (row.dataset.tableId !== block.dataset.tableId) continue
    const other = row.querySelector(':scope > .tbl-scroll')
    if (other && other !== scroller && other.scrollLeft !== left) other.scrollLeft = left
  }
}
