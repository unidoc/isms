import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body></body></html>')
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.Node = dom.window.Node
globalThis.Element = dom.window.Element
globalThis.HTMLElement = dom.window.HTMLElement

const { wrapTables } = await import('../src/utils/wideTables.js')

const long = 'Hosting of the production platform and customer data, with backups'
const table = `<table><tbody><tr><th>Company</th><th>Purpose</th></tr><tr><td>Acme</td><td>${long}</td></tr></tbody></table>`

function parse(html) {
  const d = document.createElement('div')
  d.innerHTML = html
  return d
}

test('wrapTables wraps every table exactly once', () => {
  const d = parse(wrapTables(`<h2>T</h2>${table}<p>x</p>${table}`))
  const tables = d.querySelectorAll('table')
  assert.equal(tables.length, 2)
  for (const t of tables) {
    assert.ok(t.parentElement.classList.contains('tbl-wide-scroll'))
    assert.equal(t.parentElement.children.length, 1)
  }
  assert.equal(d.querySelectorAll('.tbl-wide-scroll .tbl-wide-scroll').length, 0)
  // already wrapped input is not wrapped again
  const again = parse(wrapTables(d.innerHTML))
  assert.equal(again.querySelectorAll('.tbl-wide-scroll').length, 2)
})

test('html without a table is returned untouched', () => {
  const html = '<h2>Title</h2><p>Some <strong>text</strong></p>'
  assert.equal(wrapTables(html), html)
  assert.equal(wrapTables(''), '')
  assert.equal(wrapTables(undefined), undefined)
})

test('only long-text columns get the floor', () => {
  const d = parse(wrapTables(table))
  const cells = (row) => Array.from(d.querySelectorAll('tr')[row].children)
  for (const row of [0, 1]) {
    assert.match(cells(row)[1].getAttribute('style'), /min-width: calc\(18ch \+ 1rem\);/)
    assert.equal(cells(row)[0].getAttribute('style'), null)
  }
})

test('the floor keeps a cell\'s own inline style', () => {
  const d = parse(wrapTables(`<table><tr><th style="color: red">A</th></tr><tr><td style="color: red">${long}</td></tr></table>`))
  assert.match(d.querySelector('td').getAttribute('style'), /^color: red; min-width: /)
})

test('diff highlight classes and attributes survive', () => {
  const html = `<table><tbody><tr><th>Name</th><th>Notes</th></tr>` +
    `<tr class="tc-row-add"><td class="tc-cell-change"><span class="tc-word-ins">new</span></td><td>${long}</td></tr>` +
    `<tr class="tc-row-del"><td><span class="tc-word-del">old</span></td><td>x</td></tr></tbody></table>`
  const d = parse(wrapTables(html))
  assert.equal(d.querySelectorAll('tr.tc-row-add').length, 1)
  assert.equal(d.querySelectorAll('tr.tc-row-del').length, 1)
  assert.equal(d.querySelectorAll('td.tc-cell-change').length, 1)
  assert.equal(d.querySelectorAll('.tc-word-ins').length, 1)
  assert.equal(d.querySelectorAll('.tc-word-del').length, 1)
  assert.equal(d.querySelector('td.tc-cell-change .tc-word-ins').textContent, 'new')
})
