import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body></body></html>')
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.Node = dom.window.Node
globalThis.Element = dom.window.Element
globalThis.HTMLElement = dom.window.HTMLElement
globalThis.getComputedStyle = dom.window.getComputedStyle

const { Editor } = await import('@tiptap/core')
const { default: StarterKit } = await import('@tiptap/starter-kit')
const { default: CodeBlock } = await import('@tiptap/extension-code-block')
const { codeBlockMetadataAttributes } = await import('../src/components/codeBlockAttributes.js')
const { markdownToHtml, htmlToMarkdown } = await import('../src/composables/useMarkdownConvert.js')

test('TipTap retains Mermaid info tokens and trailing blank source lines', () => {
  const source = '```mermaid custom-option\nflowchart TD\n  Draft --> Review\n\n```'
  const MetadataCodeBlock = CodeBlock.extend({
    addAttributes() {
      return {
        ...this.parent?.(),
        ...codeBlockMetadataAttributes(),
      }
    },
  })
  const editor = new Editor({
    extensions: [
      StarterKit.configure({ codeBlock: false }),
      MetadataCodeBlock,
    ],
    content: markdownToHtml(source),
  })

  assert.match(editor.getHTML(), /data-info-string="mermaid custom-option"/)
  assert.equal(htmlToMarkdown(editor.getHTML()).trim(), source)
  editor.destroy()
})
