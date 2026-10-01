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
  // Case-insensitive to match parseExtraStyle's own strip below: without the
  // `i` flag, `BACKGROUND-COLOR:` (valid CSS, browsers don't care about case)
  // would be stripped here as "not a background" and ALSO stripped by
  // parseExtraStyle as "is a background" — the color vanishes from both.
  return style?.match(/background-color:\s*([^;]+)/i)?.[1]?.trim() || null
}

export function parseExtraStyle(style) {
  if (!style) return null
  const rest = style.replace(/background-color:\s*[^;]+;?/i, '').trim()
  return rest || null
}

// True when a cell's non-background style sets an explicit text color.
// `(^|;)` so "background-color" itself can never match — only a `color`
// declaration that is its own property, not a suffix of another one.
//
// Exists because tiptap's Bold extension parses ANY ancestor element's
// `font-weight: 600` (or higher, or the literal word "bold") as a bold MARK
// on the text inside — including a `<td style="font-weight:600">`'s own
// inline style, not just an explicit <strong>/<b> in the source. The
// resulting <strong> then hits `.editor-content .tiptap strong { color:
// #f1f5f9 }` (and `.doc-prose td strong` in view mode), which overrides
// whatever color extraStyle preserved on the cell — the exact bold,
// explicitly-colored cells this module exists to fix stay unreadable
// without a way to tell the stylesheet "this <strong> should inherit, not
// use the generic bold color." See the `.has-text-color` rules in
// DocumentEditor.vue and style.css.
export function hasExplicitTextColor(style) {
  return !!style && /(^|;)\s*color\s*:/i.test(style)
}
