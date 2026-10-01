// Tables are stored as raw HTML in markdown specifically to preserve per-cell
// formatting (see useMarkdownConvert.js) — a cell can carry any inline style,
// not just a background color. The tiptap table-cell extensions round-trip
// exactly two things through parse/render: `backgroundColor` (its own
// attribute, since the cell-color picker reads and writes that one property
// in isolation) and `extraStyle` (everything else on the cell — color,
// font-weight, text-align, vertical-align, padding, border, width, ...).
//
// Without extraStyle, every inline style but background-color was silently
// dropped the moment a table loaded into the editor: a cell authored with a
// light background and an explicit dark, readable text color kept its
// background but lost the color, falling back to the editor's generic table
// text color (a light gray-blue meant for the default dark theme) —
// unreadable against a light background. The read-only view was never
// affected, since it renders the stored HTML's style attribute as-is and
// never round-trips it through this parse/render step at all.

export function parseBackgroundColor(style) {
  return style?.match(/background-color:\s*([^;]+)/)?.[1]?.trim() || null
}

export function parseExtraStyle(style) {
  if (!style) return null
  const rest = style.replace(/background-color:\s*[^;]+;?/i, '').trim()
  return rest || null
}
