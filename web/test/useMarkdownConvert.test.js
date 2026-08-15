import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body></body></html>')
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.Node = dom.window.Node
globalThis.Element = dom.window.Element
globalThis.HTMLElement = dom.window.HTMLElement

const { markdownToHtml, htmlToMarkdown } = await import('../src/composables/useMarkdownConvert.js')

test('round-trips a Mermaid fence without changing its source', () => {
  const source = '```mermaid\nflowchart TD\n  Draft --> Review --> Approved\n```'

  const html = markdownToHtml(source)

  assert.match(html, /class="language-mermaid"/)
  assert.equal(htmlToMarkdown(html).trim(), source)
})

test('preserves Mermaid info tokens and trailing blank source lines', () => {
  const source = '```mermaid custom-option\nflowchart TD\n  Draft --> Review\n\n```'

  const html = markdownToHtml(source)

  assert.match(html, /data-info-string="mermaid custom-option"/)
  assert.equal(htmlToMarkdown(html).trim(), source)
})

test('keeps custom info tokens when editor controls change language and wrapping', () => {
  const container = document.createElement('div')
  container.innerHTML = markdownToHtml('```mermaid custom-option\nflowchart TD\n  A --> B\n```')
  const pre = container.querySelector('pre')
  pre.setAttribute('data-wrapped', 'true')
  pre.querySelector('code').className = 'language-javascript'

  const markdown = htmlToMarkdown(container.innerHTML).trim()

  assert.match(markdown, /^```javascript custom-option wrap$/m)
})
