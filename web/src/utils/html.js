// Shared HTML-escaping helper. Used by the markdown renderers (useRenderMd,
// useMarkdownConvert) so they can't diverge — if attribute-safety escaping
// (e.g. " → &quot;) is ever needed, it's added here once.
export function escapeHtml(s) {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}
