// The export formats the server can render, from its /config answer. The server
// sends [] when nothing is licensed; anything else (an older server without the
// field, a non-list, unknown names) also means "none of those".
const KNOWN = ['pdf', 'docx']

export function exportFormatsFrom(cfg) {
  const list = cfg?.export_formats
  if (!Array.isArray(list)) return []
  return KNOWN.filter((f) => list.includes(f))
}
