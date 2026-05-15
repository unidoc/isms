import { marked } from 'marked'
import DOMPurify from 'dompurify'

/**
 * Canonical markdown renderer for entity descriptions, treatment plans,
 * notes, and other free-text fields rendered via `v-html`.
 *
 * No URL rewriting happens here. A global click delegate in App.vue
 * intercepts internal absolute-path links anywhere in the app and routes
 * them through Vue Router with the current org prefix, so markdown can
 * keep absolute paths like `/documents/foo` or `/risks/RISK-5` without
 * each view re-implementing a regex rewrite.
 */
export function renderMarkdown(text) {
  if (!text) return ''
  return DOMPurify.sanitize(marked.parse(text, { breaks: true }))
}

export default renderMarkdown
