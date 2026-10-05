// #404: the Documents page highlights @-mentions in full, for both the
// "@email" form the comment boxes insert and the "@Jane Doe" display-name form
// its own dropdown inserts.
import test from 'node:test'
import assert from 'node:assert/strict'
import { highlightMentions } from '../src/utils/mentions.js'
import { linkEntityReferences } from '../src/composables/entityReferenceLinks.js'

const span = (s) => `<span class="text-blue-400 font-medium">${s}</span>`
const count = (s) => (s.match(/<span/g) || []).length

test('an @email mention is highlighted whole in one span', () => {
  const out = highlightMentions('@admin@example.com')
  assert.equal(out, span('@admin@example.com'))
  assert.equal(count(out), 1)
})

test('a two-word display name is highlighted, the rest stays plain', () => {
  assert.equal(highlightMentions('ping @Jane Doe now'), `ping ${span('@Jane Doe')} now`)
})

test('a single-word mention is one span', () => {
  assert.equal(highlightMentions('@jane'), span('@jane'))
})

test('an email and a name in one comment give two separate spans', () => {
  const out = highlightMentions('hi @admin@example.com and @Jane Doe')
  assert.equal(out, `hi ${span('@admin@example.com')} and ${span('@Jane Doe')}`)
  assert.equal(count(out), 2)
  assert.ok(!/<span[^>]*>[^<]*<span/.test(out), 'no span is nested in another')
})

test('already-escaped input keeps its entities', () => {
  assert.equal(highlightMentions('&lt;b&gt; @jane'), `&lt;b&gt; ${span('@jane')}`)
})

test('text without an @ is returned unchanged', () => {
  assert.equal(highlightMentions('nothing to see here'), 'nothing to see here')
})

test('a reference link without an @ passes through unchanged', () => {
  const opts = { programKeys: new Set(['ISMS']), orgPath: (route) => `/acme${route}` }
  const linked = linkEntityReferences('see #RISK-1', opts)
  assert.ok(linked.includes('<a href="/acme/risks/RISK-1"'))
  assert.equal(highlightMentions(linked), linked)
})
