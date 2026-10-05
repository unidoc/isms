// The Documents tree must render the parent's `new-folder` slot for every
// expanded folder, at any depth.
//
// This is issue #309. DocTreeNode renders nested folders with a nested
// DocTreeNode that was never handed the slot, so the inline "New folder" input
// showed under a top-level folder and nowhere below it.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: the helper installs the DOM that
// vue/runtime-dom captures at evaluation time. See its header.
import { compileSfc, renderSfc } from './support/renderSfc.js'

const DocTreeNode = await compileSfc(new URL('../src/components/DocTreeNode.vue', import.meta.url).pathname)

const leaf = { name: 'dropbox', title: 'Dropbox', type: 'folder', path: 'tpe/certs/dropbox', depth: 2, children: [], fileCount: 0 }
const mid = { name: 'certs', title: 'Certifications', type: 'folder', path: 'tpe/certs', depth: 1, children: [leaf], fileCount: 0 }
const top = { name: 'tpe', title: 'Third Party Evidence', type: 'folder', path: 'tpe', depth: 0, children: [mid], fileCount: 0 }

const MARKERS = ['[slot:tpe:0]', '[slot:tpe/certs:1]', '[slot:tpe/certs/dropbox:2]']

function render(expanded) {
  return renderSfc(
    DocTreeNode,
    {
      nodes: [top],
      expanded,
      editable: true,
      formatName: n => n.title || n.name,
      formatFileTitle: f => f.title || '',
      formatFileName: f => f.path || '',
      needsReview: () => false,
    },
    { 'new-folder': ({ parentPath, depth }) => `[slot:${parentPath}:${depth}]` },
  )
}

const count = (html, marker) => html.split(marker).length - 1

test('renders the new-folder slot for an expanded folder at every depth', async () => {
  const html = await render(new Set(['tpe', 'tpe/certs', 'tpe/certs/dropbox']))
  for (const m of MARKERS) assert.ok(html.includes(m), `missing ${m}`)
})

test('calls the slot once per expanded folder', async () => {
  const html = await render(new Set(['tpe', 'tpe/certs', 'tpe/certs/dropbox']))
  for (const m of MARKERS) assert.equal(count(html, m), 1, `${m} count`)
})

test('does not render the slot for a collapsed folder or anything under it', async () => {
  const html = await render(new Set(['tpe']))
  assert.ok(html.includes('[slot:tpe:0]'))
  assert.ok(!html.includes('[slot:tpe/certs:1]'))
  assert.ok(!html.includes('[slot:tpe/certs/dropbox:2]'))
})
