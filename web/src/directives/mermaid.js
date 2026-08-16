import { renderMermaidDiagrams } from '../composables/useMermaid.js'

// Vue lifecycle bridge for v-html roots. The renderer itself owns discovery,
// duplicate prevention, asynchronous staleness checks, and error isolation.
export function createMermaidDirective(render = renderMermaidDiagrams) {
  function renderRoot(element) {
    void render(element)
  }

  return {
    mounted: renderRoot,
    updated: renderRoot,
  }
}

export default createMermaidDirective()
