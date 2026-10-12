import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body></body></html>')
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.Node = dom.window.Node
globalThis.Element = dom.window.Element
globalThis.HTMLElement = dom.window.HTMLElement
globalThis.DOMParser = dom.window.DOMParser

const { findPipeTables, regroupPipeTables } = await import('../src/composables/useTableDiff.js')
const { diffLines } = await import('../src/composables/useWordDiff.js')

const ops = (a, b) => diffLines(a.split('\n'), b.split('\n'))
const regroup = (a, b) => regroupPipeTables(ops(a, b))
const T = (rows) => ['| Company | Service | Location |', '|---|---|---|', ...rows].join('\n')

test('findPipeTables finds header + separator + contiguous rows', () => {
  const lines = ['intro', '', '| A | B |', '|---|---|', '| 1 | 2 |', '| 3 | 4 |', '', 'text | with pipe', 'more']
  assert.deepEqual(findPipeTables(lines), [{ start: 2, end: 6 }])
  assert.deepEqual(findPipeTables(['a | b', 'not a separator']), [])
})

test('a changed cell is one change unit with the whole table', () => {
  const a = T(['| Acme | Hosting | Frankfurt |', '| Postbox | Email | Dublin |'])
  const b = T(['| Acme | Hosting | Frankfurt |', '| Postbox | Email | Cork |'])
  const out = regroup(a, b)
  assert.equal(out.length, 1)
  assert.equal(out[0].type, 'change')
  assert.equal(out[0].isTable, true)
  assert.equal(out[0].cellCount, 1)
  assert.equal((out[0].html.match(/<tr/g) || []).length, 3)
  assert.match(out[0].html, /tc-cell-change/)
})

test('an added row and a removed row are marked inside one table', () => {
  const base = ['| Acme | Hosting | Frankfurt |', '| Postbox | Email | Dublin |']
  const added = regroup(T(base), T([...base, '| Newco | Chat | Oslo |']))
  assert.equal(added.length, 1)
  assert.match(added[0].html, /tc-row-add/)
  assert.equal(added[0].cellCount, 3)
  const removed = regroup(T(base), T([base[0]]))
  assert.equal(removed.length, 1)
  assert.match(removed[0].html, /tc-row-del/)
  assert.equal((removed[0].html.match(/<tr/g) || []).length, 3)
})

test('a renamed header is still one table', () => {
  const rows = ['| Acme | Hosting | Frankfurt |']
  const out = regroup(T(rows), T(rows).replace('Company', 'Vendor'))
  assert.equal(out.length, 1)
  assert.equal(out[0].isTable, true)
  assert.match(out[0].html, /Vendor/)
})

test('an unchanged table between changed prose lines is one whole equal unit', () => {
  const table = T(['| Acme | Hosting | Frankfurt |'])
  const out = regroup(`Intro one\n\n${table}\n\nOutro one`, `Intro two\n\n${table}\n\nOutro two`)
  const equal = out.filter(o => o.whole)
  assert.equal(equal.length, 1)
  assert.equal(equal[0].type, 'equal')
  assert.equal(equal[0].text, table)
  // prose keeps its own remove/add runs, none of which contains a table row
  for (const o of out.filter(o => !o.whole)) assert.doesNotMatch(o.text, /\|/)
})

test('two tables separated by one blank line stay two units', () => {
  const t1 = T(['| Acme | Hosting | Frankfurt |'])
  const t2 = T(['| Beta | Email | Dublin |'])
  const out = regroup(`${t1}\n\n${t2}`, `${t1.replace('Frankfurt', 'Berlin')}\n\n${t2}`)
  assert.equal(out.filter(o => o.isTable).length, 1)
  assert.equal(out.filter(o => o.whole && o.type === 'equal').length, 1)
})

test('a table at the very start and one at the very end of the document', () => {
  const t1 = T(['| Acme | Hosting | Frankfurt |'])
  const t2 = T(['| Beta | Email | Dublin |'])
  const out = regroup(`${t1}\n\nmiddle\n\n${t2}`, `${t1.replace('Hosting', 'Cloud')}\n\nmiddle\n\n${t2.replace('Dublin', 'Cork')}`)
  assert.equal(out.length, 3)
  assert.equal(out[0].isTable, true)
  assert.equal(out[1].text, '\nmiddle\n')
  assert.equal(out[2].isTable, true)
})

test('a wholly added or removed table is one whole unit', () => {
  const t = T(['| Acme | Hosting | Frankfurt |'])
  const added = regroup('Intro', `Intro\n\n${t}`)
  assert.deepEqual(added.filter(o => o.whole).map(o => [o.type, o.text]), [['add', t]])
  const removed = regroup(`Intro\n\n${t}`, 'Intro')
  assert.deepEqual(removed.filter(o => o.whole).map(o => [o.type, o.text]), [['remove', t]])
})

test('prose is never handed to the table diff, and prose-only input is untouched', () => {
  const o = ops('one\ntwo', 'one\nthree')
  assert.deepEqual(regroupPipeTables(o), o)
})

test('an old pipe table that became an editor HTML table is one change unit', () => {
  const md = T(['| Acme | Hosting | Frankfurt |', '| Postbox | Email | Dublin |'])
  const html = '<table><tbody><tr><th><p>Company</p></th><th><p>Service</p></th><th><p>Location</p></th></tr>' +
    '<tr><td><p>Acme</p></td><td><p>Hosting</p></td><td><p>Frankfurt</p></td></tr>' +
    '<tr><td><p>Postbox</p></td><td><p>Email</p></td><td><p>Cork</p></td></tr></tbody></table>'
  const out = regroup(md, html)
  assert.equal(out.length, 1)
  assert.equal(out[0].type, 'change')
  assert.equal(out[0].cellCount, 1)
})
