# Mermaid diagram support

## Goal

Render fenced `mermaid` code blocks as diagrams throughout read-only document and review surfaces while preserving the original fenced Markdown as editable source in the TipTap editor.

## Scope

- Recognize fenced blocks whose first info-string token is `mermaid`.
- Render those blocks as responsive diagrams in every view that uses the canonical display Markdown renderer.
- Keep Mermaid source as an ordinary editable code block in the WYSIWYG editor.
- Preserve the source losslessly when Markdown is loaded, edited, and saved.
- Show a stable inline error state for invalid Mermaid syntax.
- Keep non-Mermaid code rendering unchanged.

Live diagram editing, diagram-specific toolbar controls, exporting diagrams, and server-side rendering are out of scope.

## Architecture

The canonical display renderer in `web/src/composables/useRenderMd.js` identifies Mermaid fences and emits a neutral placeholder containing the source as text. It must not generate or trust SVG markup itself.

A reusable client-side renderer owns Mermaid execution. A globally registered Vue directive discovers placeholders after mounted and updated hooks, and asks the renderer to process each diagram. Each render receives a unique identifier so multiple diagrams can coexist on one page. Placeholders are claimed synchronously before the lazy Mermaid import resolves, preventing overlapping lifecycle hooks from scheduling the same node twice.

All read-only surfaces that render arbitrary Markdown through the canonical renderer use this directive. This includes documents, reviews, change/diff presentations, and register free-text fields. Comment and mention HTML remains on its separate constrained rendering path. Existing entity-reference link rewriting and paragraph-level comment behavior must continue to work.

The existing Markdown-to-HTML and HTML-to-Markdown editor conversion remains responsible for source round-tripping. Mermaid is shown there as a code block with language `mermaid`; the editor does not execute it.

## Data flow

1. A document contains a fenced `mermaid` block.
2. `marked` converts it to a Mermaid placeholder while normal fences continue through the syntax-highlighted code renderer.
3. DOMPurify sanitizes the generated display HTML.
4. After Vue updates the DOM, the Mermaid renderer reads the placeholder's text content and invokes Mermaid.
5. Mermaid returns SVG, which is inserted only into the matching placeholder.
6. When content changes, stale render state is discarded and the current placeholders are rendered again.

## Security and failure handling

Mermaid is configured with `securityLevel: "strict"`, a theme compatible with the application's dark palette, and no automatic page-wide startup scan. Source is transported as text rather than executable HTML. The existing DOMPurify boundary remains in place before diagram execution.

Rendering failures are caught per diagram. A failure replaces only that diagram with a compact `Diagram could not be rendered` message. The rest of the document remains usable, and the original source remains available in the editor.

## Presentation

Rendered diagrams use the available content width, retain their aspect ratio, and permit horizontal scrolling when a diagram cannot fit without becoming illegible. The container follows the existing slate border, background, radius, and spacing conventions. Error states use the existing red/rose semantic colors.

## Verification

- A focused renderer test proves that Mermaid fences become placeholders and ordinary fences retain code-block behavior.
- A round-trip test proves that Mermaid fenced Markdown remains unchanged through editor conversion.
- A rendering test covers successful rendering, multiple unique diagrams, rerendering, and invalid syntax.
- The web production build succeeds.
- Relevant existing tests remain green.
- Browser verification shows a rendered flowchart in the document view and the same source in the editor. An invalid diagram is also checked for the controlled error state.

## Delivery constraints

No changes are committed. The user receives screenshots before any later decision about committing or publishing.
