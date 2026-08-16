import test from 'node:test'
import assert from 'node:assert/strict'

let createEditorLowlight
try {
  ;({ createEditorLowlight } = await import('../src/components/editorLowlight.js'))
} catch {
  // The regression test should fail with a useful assertion until the editor
  // owns an explicit Mermaid grammar instead of falling through to auto-detect.
}

test('keeps Mermaid source plain instead of auto-detecting another language', () => {
  assert.equal(typeof createEditorLowlight, 'function')

  const lowlight = createEditorLowlight()
  const result = lowlight.highlight('mermaid', 'flowchart LR\n  Draft --> Review')

  assert.equal(lowlight.registered('mermaid'), true)
  assert.deepEqual(result.children, [{ type: 'text', value: 'flowchart LR\n  Draft --> Review' }])
})
