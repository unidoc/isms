import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body></body></html>')
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.Node = dom.window.Node
globalThis.Element = dom.window.Element
globalThis.HTMLElement = dom.window.HTMLElement
globalThis.SVGElement = dom.window.SVGElement

const { mermaidConfig, renderMermaidDiagrams } = await import('../src/composables/useMermaid.js')

test('uses the safe explicit Mermaid configuration', () => {
  assert.equal(mermaidConfig.startOnLoad, false)
  assert.equal(mermaidConfig.securityLevel, 'strict')
  assert.equal(mermaidConfig.suppressErrorRendering, true)
  assert.equal(mermaidConfig.htmlLabels, false)
})

test('uses the configurable base theme with explicit standalone text colors', () => {
  assert.equal(mermaidConfig.theme, 'base')
  assert.equal(mermaidConfig.themeVariables.textColor, '#e2e8f0')
  assert.equal(mermaidConfig.themeVariables.titleColor, '#e2e8f0')
})

test('cleans Mermaid temporary DOM after a real syntax error', async () => {
  const root = document.createElement('div')
  root.innerHTML = '<div class="mermaid-diagram" data-mermaid-pending="true">flowchart ???</div>'
  document.body.append(root)
  const { default: mermaid } = await import('mermaid')
  mermaid.initialize(mermaidConfig)

  await renderMermaidDiagrams(root, async () => mermaid)

  assert.equal(root.querySelector('.mermaid-diagram').dataset.mermaidState, 'error')
  assert.equal(document.querySelectorAll('[id^="disms-mermaid-"]').length, 0)
  root.remove()
})

test('renders every pending diagram with a unique id and sanitized SVG', async () => {
  const root = document.createElement('div')
  root.innerHTML = [
    '<div class="mermaid-diagram" data-mermaid-pending="true">graph TD\nA--&gt;B</div>',
    '<div class="mermaid-diagram" data-mermaid-pending="true">graph LR\nC--&gt;D</div>',
  ].join('')
  const calls = []

  await renderMermaidDiagrams(root, async () => ({
    render: async (id, source) => {
      calls.push({ id, source })
      return { svg: `<svg><text>${source}</text><script>alert(1)</script></svg>` }
    },
  }))

  assert.equal(calls.length, 2)
  assert.notEqual(calls[0].id, calls[1].id)
  assert.deepEqual(calls.map(call => call.source), ['graph TD\nA-->B', 'graph LR\nC-->D'])
  assert.equal(root.querySelectorAll('svg').length, 2)
  assert.equal(root.querySelectorAll('script').length, 0)
  assert.equal(root.querySelectorAll('[data-mermaid-state="rendered"]').length, 2)
  assert.match(root.textContent, /graph TD/)
})

test('isolates a diagram render failure in an accessible error state', async () => {
  const root = document.createElement('div')
  root.innerHTML = '<div class="mermaid-diagram" data-mermaid-pending="true">flowchart ???\n&lt;script&gt;alert(1)&lt;/script&gt;</div>'

  await renderMermaidDiagrams(root, async () => ({
    render: async () => { throw new Error('invalid syntax') },
  }))

  const diagram = root.querySelector('.mermaid-diagram')
  assert.equal(diagram.dataset.mermaidState, 'error')
  assert.equal(diagram.getAttribute('role'), 'alert')
  assert.equal(diagram.querySelector('.mermaid-error-message').textContent, 'Diagram could not be rendered')
  assert.equal(diagram.querySelector('.mermaid-error-source').textContent, 'flowchart ???\n<script>alert(1)</script>')
  assert.equal(diagram.querySelector('.mermaid-error-source script'), null)
})

test('renders fresh placeholders after the containing HTML changes', async () => {
  const root = document.createElement('div')
  const sources = []
  const loadRenderer = async () => ({
    render: async (_id, source) => {
      sources.push(source)
      return { svg: `<svg><text>${source}</text></svg>` }
    },
  })

  root.innerHTML = '<div class="mermaid-diagram" data-mermaid-pending="true">graph TD\nA--&gt;B</div>'
  await renderMermaidDiagrams(root, loadRenderer)
  root.innerHTML = '<div class="mermaid-diagram" data-mermaid-pending="true">graph LR\nB--&gt;C</div>'
  await renderMermaidDiagrams(root, loadRenderer)

  assert.deepEqual(sources, ['graph TD\nA-->B', 'graph LR\nB-->C'])
  assert.equal(root.querySelector('[data-mermaid-state="rendered"] text').textContent, 'graph LR\nB-->C')
})

test('claims pending diagrams before an asynchronous renderer load completes', async () => {
  const root = document.createElement('div')
  root.innerHTML = '<div class="mermaid-diagram" data-mermaid-pending="true">graph TD\nA--&gt;B</div>'
  const calls = []
  let releaseRenderer
  const rendererReady = new Promise(resolve => { releaseRenderer = resolve })
  const loadRenderer = () => rendererReady

  const firstRender = renderMermaidDiagrams(root, loadRenderer)
  const overlappingRender = renderMermaidDiagrams(root, loadRenderer)
  releaseRenderer({
    render: async (id, source) => {
      calls.push({ id, source })
      return { svg: `<svg><text>${source}</text></svg>` }
    },
  })

  await Promise.all([firstRender, overlappingRender])

  assert.equal(calls.length, 1)
})

test('skips stale placeholders removed while the renderer is loading', async () => {
  const root = document.createElement('div')
  root.innerHTML = '<div class="mermaid-diagram" data-mermaid-pending="true">graph TD\nA--&gt;B</div>'
  const calls = []
  let releaseRenderer
  const rendererReady = new Promise(resolve => { releaseRenderer = resolve })
  const rendering = renderMermaidDiagrams(root, () => rendererReady)

  root.replaceChildren()
  releaseRenderer({
    render: async (id, source) => {
      calls.push({ id, source })
      return { svg: '<svg></svg>' }
    },
  })
  await rendering

  assert.equal(calls.length, 0)
})
