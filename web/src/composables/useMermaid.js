import DOMPurify from 'dompurify'

/**
 * Client-side Mermaid rendering for sanitized Markdown display roots.
 *
 * Invariants:
 * - source is read only from inert text placeholders;
 * - every SVG is sanitized before insertion;
 * - pending nodes are claimed synchronously to avoid overlapping renders;
 * - a failure is isolated to its own accessible placeholder.
 *
 * See docs/markdown-rendering.md for the complete display and editor pipelines.
 */

let sequence = 0
let rendererPromise

export const mermaidConfig = {
  startOnLoad: false,
  securityLevel: 'strict',
  suppressErrorRendering: true,
  // The base theme is the only Mermaid theme whose full palette can be
  // controlled through themeVariables. The dark theme overwrites most of
  // these values after initialization.
  theme: 'base',
  htmlLabels: false,
  fontFamily: 'ui-sans-serif, system-ui, sans-serif',
  themeVariables: {
    background: '#020617',
    primaryColor: '#1e293b',
    primaryTextColor: '#e2e8f0',
    primaryBorderColor: '#475569',
    lineColor: '#64748b',
    secondaryColor: '#0f172a',
    tertiaryColor: '#172554',
    textColor: '#e2e8f0',
    titleColor: '#e2e8f0',
  },
}

async function loadDefaultRenderer() {
  if (!rendererPromise) {
    rendererPromise = import('mermaid').then(({ default: mermaid }) => {
      mermaid.initialize(mermaidConfig)
      return mermaid
    })
  }
  return rendererPromise
}

function showError(target, source) {
  target.replaceChildren()
  target.removeAttribute('data-mermaid-pending')
  target.dataset.mermaidState = 'error'
  target.setAttribute('role', 'alert')

  const message = document.createElement('span')
  message.className = 'mermaid-error-message'
  message.textContent = 'Diagram could not be rendered'

  const sourceBlock = document.createElement('pre')
  sourceBlock.className = 'mermaid-error-source'
  sourceBlock.textContent = source
  target.append(message, sourceBlock)
}

function removeTemporaryElements(id) {
  for (const temporaryId of [id, `d${id}`, `i${id}`]) {
    document.getElementById(temporaryId)?.remove()
  }
}

export async function renderMermaidDiagrams(root, loadRenderer = loadDefaultRenderer) {
  const targets = Array.from(
    root?.querySelectorAll?.('.mermaid-diagram[data-mermaid-pending="true"]') || [],
  )
  if (targets.length === 0) return

  // Claim placeholders synchronously. Vue can call the directive's updated
  // hook while the lazy Mermaid import is still pending; removing the marker
  // here prevents the same DOM node from being queued a second time.
  const jobs = targets.map(target => {
    const job = {
      target,
      source: target.textContent || '',
      id: `isms-mermaid-${++sequence}`,
    }
    target.removeAttribute('data-mermaid-pending')
    target.dataset.mermaidState = 'rendering'
    return job
  })

  let renderer
  try {
    renderer = await loadRenderer()
  } catch {
    jobs.forEach(({ target, source }) => {
      if (root.contains(target)) showError(target, source)
    })
    return
  }

  for (const { target, source, id } of jobs) {
    if (!root.contains(target)) continue

    try {
      const { svg, bindFunctions } = await renderer.render(id, source)
      if (!root.contains(target)) continue

      target.innerHTML = DOMPurify.sanitize(svg, {
        USE_PROFILES: { svg: true, svgFilters: true },
      })
      target.dataset.mermaidState = 'rendered'
      bindFunctions?.(target)
    } catch {
      removeTemporaryElements(id)
      if (root.contains(target)) showError(target, source)
    }
  }
}
