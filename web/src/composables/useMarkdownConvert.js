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
import { escapeHtml } from '../utils/html.js'

// Dedicated parser so the custom code renderer (which carries the complete
// fence info string and per-block `wrap` flag) doesn't leak globally.
const mdParser = new Marked({ breaks: true })
mdParser.use({
  renderer: {
    // Preserve the complete fenced info string so Mermaid options survive an
    // edit/save cycle. Language and wrap are also exposed as normal editor
    // attributes so the code-block controls remain interactive.
    code(token) {
      const infoString = (token.lang || '').trim()
      const info = infoString.split(/\s+/).filter(Boolean)
      const lang = info[0] || ''
      const wrapped = info.includes('wrap')
      const langClass = lang ? ` class="language-${escapeHtml(lang)}"` : ''
      const wrapAttr = wrapped ? ' data-wrapped="true"' : ''
      const infoAttr = infoString ? ` data-info-string="${escapeHtml(infoString)}"` : ''
      // No trailing newline appended — it would become a dangling empty line in
      // the editor (off-by-one vs the read view, which uses token.text as-is).
      return `<pre${wrapAttr}${infoAttr}><code${langClass}>${escapeHtml(token.text)}</code></pre>`
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

  // Fenced code blocks retain their complete original info string. If the user
  // changes language or wrapping in the editor, only those tokens are updated;
  // all other Mermaid options remain intact and in their original order.
  td.addRule('fencedCodeWithWrap', {
    filter: function (node) {
      return node.nodeName === 'PRE' && node.firstChild && node.firstChild.nodeName === 'CODE'
    },
    replacement: function (content, node) {
      const code = node.firstChild
      const m = (code.getAttribute('class') || '').match(/language-(\S+)/)
      const lang = m ? m[1] : ''
      const wrapped = node.getAttribute('data-wrapped') === 'true'
      const originalInfo = (node.getAttribute('data-info-string') || '').trim()
      const originalTokens = originalInfo.split(/\s+/).filter(Boolean)
      const originalLang = originalTokens[0] || ''
      const originallyWrapped = originalTokens.includes('wrap')
      let info = originalInfo

      if (!info || lang !== originalLang || wrapped !== originallyWrapped) {
        const extraTokens = originalTokens.slice(1).filter(token => token !== 'wrap')
        const updatedTokens = [lang, ...extraTokens].filter(Boolean)
        if (wrapped) updatedTokens.push('wrap')
        info = updatedTokens.join(' ')
      }

      // Mermaid source is preserved exactly, including meaningful trailing
      // blank lines. Ordinary code keeps the existing whitespace normalization.
      const rawText = code.textContent || ''
      const text = lang.toLowerCase() === 'mermaid' ? rawText : rawText.replace(/\s+$/, '')
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
