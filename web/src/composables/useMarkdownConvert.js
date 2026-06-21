// Shared markdown <-> HTML conversion used by DocumentEditor and MarkdownField.
// Both editors are tiptap-based and need the same lossless round-trip behaviour.
//
// - markdownToHtml: feeds tiptap's setContent on load
// - htmlToMarkdown: serializes tiptap output back to markdown for storage
//
// Tables are kept as raw HTML in markdown (turndown rule below) — pipe tables
// would lose cell formatting/colors/structure. MarkdownField doesn't enable
// the table extension, but the rule is harmless when no tables are present.
//
// Note: markdown spec collapses multiple blank lines into one separator. For
// visual spacing, use Shift+Enter (preserved as <br>) or `---` (horizontal rule).

import { Marked } from 'marked'
import TurndownService from 'turndown'
import { gfm } from 'turndown-plugin-gfm'

function escapeHtml(s) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

// Dedicated parser so the custom code renderer (which carries the per-block
// `wrap` flag through to a data-wrapped attribute) doesn't leak globally.
const mdParser = new Marked({ breaks: true })
mdParser.use({
  renderer: {
    // Preserve the fenced info string's `wrap` token as data-wrapped so the
    // editor's code-block node round-trips the word-wrap setting. The language
    // is the first info token (e.g. ```js wrap → lang=js, wrapped=true).
    code(token) {
      const info = (token.lang || '').trim().split(/\s+/).filter(Boolean)
      const lang = info[0] || ''
      const wrapped = info.includes('wrap')
      const langClass = lang ? ` class="language-${lang}"` : ''
      const wrapAttr = wrapped ? ' data-wrapped="true"' : ''
      // No trailing newline appended — it would become a dangling empty line in
      // the editor (off-by-one vs the read view, which uses token.text as-is).
      return `<pre${wrapAttr}><code${langClass}>${escapeHtml(token.text)}</code></pre>`
    },
  },
})

export function markdownToHtml(md) {
  if (!md) return ''
  // breaks: true preserves single newlines as <br> so user-typed line breaks
  // survive the WYSIWYG round-trip (tiptap → HTML → markdown → HTML).
  return mdParser.parse(md)
}

function createTurndown() {
  const td = new TurndownService({
    headingStyle: 'atx',
    codeBlockStyle: 'fenced',
    emDelimiter: '*',
    bulletListMarker: '-',
  })
  td.use(gfm)

  // Always keep tables as HTML in markdown — preserves formatting, colors, and structure.
  // Pipe tables lose cell formatting, colors, and complex content.
  td.addRule('alwaysHtmlTable', {
    filter: 'table',
    replacement: function (content, node) {
      return '\n\n' + node.outerHTML + '\n\n'
    },
  })

  // Fenced code blocks, carrying the per-block word-wrap flag as a `wrap` token
  // in the info string (```js wrap). The language comes from the code's
  // language-* class; wrap from the pre's data-wrapped attribute.
  td.addRule('fencedCodeWithWrap', {
    filter: function (node) {
      return node.nodeName === 'PRE' && node.firstChild && node.firstChild.nodeName === 'CODE'
    },
    replacement: function (content, node) {
      const code = node.firstChild
      const m = (code.getAttribute('class') || '').match(/language-(\S+)/)
      const lang = m ? m[1] : ''
      const wrapped = node.getAttribute('data-wrapped') === 'true'
      const info = lang + (wrapped ? (lang ? ' ' : '') + 'wrap' : '')
      // Trim trailing blank lines/whitespace so saved code blocks don't carry
      // dangling empty lines — keeps edit and read view consistent.
      const text = (code.textContent || '').replace(/\s+$/, '')
      return '\n\n```' + info + '\n' + text + '\n```\n\n'
    },
  })

  return td
}

export function htmlToMarkdown(html) {
  if (!html) return ''
  const td = createTurndown()
  return td.turndown(html)
}
