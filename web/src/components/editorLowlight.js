import { common, createLowlight } from 'lowlight'

function mermaidPlainSource() {
  return { name: 'mermaid', contains: [] }
}

// CodeBlockLowlight falls back to highlightAuto for unknown languages.
// Mermaid has no grammar in lowlight's common set, so register an empty one to
// keep its source plain and editable instead of misclassifying it as Rust.
export function createEditorLowlight() {
  const lowlight = createLowlight(common)
  lowlight.register('mermaid', mermaidPlainSource)
  return lowlight
}
