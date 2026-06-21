import { Marked } from 'marked'
import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'

/**
 * Canonical markdown renderer for entity descriptions, treatment plans,
 * notes, documents, reviews and other free-text rendered via `v-html`.
 *
 * All read-only ("display") rendering across the app goes through the single
 * `display` Marked instance below, so syntax highlighting, copy buttons and
 * options stay consistent everywhere. The instance is deliberately separate
 * from the editor conversion (useMarkdownConvert) — the copy-button code
 * renderer must never leak into the tiptap round-trip.
 *
 * No URL rewriting happens here. A global click delegate in App.vue
 * intercepts internal absolute-path links anywhere in the app and routes
 * them through Vue Router with the current org prefix, so markdown can keep
 * absolute paths like `/documents/foo`. The same delegate wires the
 * copy-to-clipboard buttons on code blocks (see below).
 */

function escapeHtml(s) {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

const display = new Marked({ breaks: true })

// Custom code-block renderer: syntax-highlight via highlight.js and wrap in a
// container with a copy button. Copying reads the <code> textContent, so the
// clipboard gets clean source — never the highlight span markup.
display.use({
  renderer: {
    code(token) {
      // Trim trailing blank lines/whitespace so they don't render as empty
      // numbered lines at the end of the block (the editor keeps them as typed;
      // the read view shouldn't show dangling whitespace).
      const text = (token.text || '').replace(/\s+$/, '')
      const info = (token.lang || '').trim().split(/\s+/).filter(Boolean)
      const lang = info[0] || ''
      const wrapped = info.includes('wrap')
      const known = lang && hljs.getLanguage(lang)
      const body = known
        ? hljs.highlight(text, { language: lang, ignoreIllegals: true }).value
        : escapeHtml(text)
      const langClass = known ? ` language-${lang}` : ''
      const blockClass = wrapped ? 'code-block wrapped' : 'code-block'
      const langBadge = known ? `<span class="code-lang">${escapeHtml(lang)}</span>` : ''
      // Line-number gutter: a separate, unselectable column so copying the
      // <code> still yields clean source (the numbers are never in the payload).
      // Lines don't wrap (pre + overflow-x), so a fixed-line-height gutter of
      // 1..N aligns perfectly with the code lines.
      const lineCount = text.replace(/\n$/, '').split('\n').length
      let nums = ''
      for (let i = 1; i <= lineCount; i++) nums += (i > 1 ? '\n' : '') + i
      const gutter = `<span class="code-gutter" aria-hidden="true">${nums}</span>`
      return (
        `<div class="${blockClass}">` +
        langBadge +
        '<button class="copy-code-btn" type="button" aria-label="Copy code">Copy</button>' +
        `<pre>${gutter}<code class="hljs${langClass}">${body}</code></pre>` +
        '</div>'
      )
    },
  },
})

/**
 * Parse markdown to display HTML WITHOUT sanitizing. For callers that apply
 * their own DOMPurify config (e.g. track-changes del/ins, diff blocks, or the
 * document grid splitter). Most callers should use `renderMarkdown` instead.
 */
export function parseMd(text) {
  if (!text) return ''
  return display.parse(text)
}

export function renderMarkdown(text) {
  if (!text) return ''
  return DOMPurify.sanitize(parseMd(text))
}

export default renderMarkdown
