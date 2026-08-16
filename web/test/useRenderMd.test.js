import test from 'node:test'
import assert from 'node:assert/strict'
import { parseMd } from '../src/composables/useRenderMd.js'

test('Mermaid fences become inert diagram placeholders', () => {
  const html = parseMd('```mermaid\nflowchart LR\n  A --> B\n```')

  assert.match(html, /class="mermaid-diagram"/)
  assert.match(html, /data-mermaid-pending="true"/)
  assert.match(html, /flowchart LR\n  A --&gt; B/)
  assert.doesNotMatch(html, /copy-code-btn/)
})

test('Mermaid info strings are case-insensitive and may contain options', () => {
  const html = parseMd('```Mermaid wrap\ngraph TD\n  A --> B\n```')

  assert.match(html, /class="mermaid-diagram"/)
})

test('ordinary code fences retain the existing code presentation', () => {
  const html = parseMd('```js\nconst answer = 42\n```')

  assert.match(html, /class="code-block"/)
  assert.match(html, /copy-code-btn/)
  assert.match(html, /language-js/)
})
