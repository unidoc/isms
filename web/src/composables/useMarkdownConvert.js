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

import { marked } from 'marked'
import TurndownService from 'turndown'
import { gfm } from 'turndown-plugin-gfm'

export function markdownToHtml(md) {
  if (!md) return ''
  // breaks: true preserves single newlines as <br> so user-typed line breaks
  // survive the WYSIWYG round-trip (tiptap → HTML → markdown → HTML).
  return marked.parse(md, { breaks: true })
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

  return td
}

export function htmlToMarkdown(html) {
  if (!html) return ''
  const td = createTurndown()
  return td.turndown(html)
}
