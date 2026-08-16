import test from 'node:test'
import assert from 'node:assert/strict'

test('renders Mermaid placeholders when a bound element mounts and updates', async () => {
  const { createMermaidDirective } = await import('../src/directives/mermaid.js')
  const element = { id: 'markdown-root' }
  const calls = []
  const directive = createMermaidDirective(async root => calls.push(root))

  directive.mounted(element)
  directive.updated(element)

  assert.deepEqual(calls, [element, element])
})
