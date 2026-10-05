// #194: a #RISK-1 reference in a comment rendered as a link in one comment
// surface out of four. The linking now lives in one function every surface
// calls, so it is tested once here, plus a check that each surface uses it.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { linkEntityReferences } from '../src/composables/entityReferenceLinks.js'

const opts = { programKeys: new Set(['ISMS']), orgPath: (route) => `/acme${route}` }
const link = (s) => linkEntityReferences(s, opts)

test('a fixed-prefix identifier links to its register record', () => {
  assert.equal(
    link('see #RISK-1'),
    'see <a href="/acme/risks/RISK-1" class="text-blue-400 font-medium hover:underline">#RISK-1</a>',
  )
  assert.match(link('#CA-12'), /href="\/acme\/corrective-actions\/CA-12"/)
  assert.match(link('#CR-3'), /href="\/acme\/changes\/CR-3"/)
})

test('program keys link to the objective or the program', () => {
  assert.match(link('#ISMS-4'), /href="\/acme\/objectives\/ISMS-4"/)
  assert.match(link('#ISMS'), /href="\/acme\/programs\/ISMS"/)
})

test('unknown identifiers are highlighted, plain words left as typed', () => {
  assert.equal(link('#FOO-1'), '<span class="text-blue-400 font-medium">#FOO-1</span>')
  assert.equal(link('#FOO'), '#FOO')
  assert.equal(link('issue #42 and #lowercase-1'), 'issue #42 and #lowercase-1')
  // No word boundary after the number, so only #RISK matches, and RISK is not a
  // program key. internal/isms/api/comment_mentions_test.go has the same case.
  assert.equal(link('#RISK-1x'), '#RISK-1x')
})

test('escaped input is passed through, not unescaped', () => {
  assert.equal(link('&lt;b&gt; #RISK-1').startsWith('&lt;b&gt; <a '), true)
})

test('every comment surface renders #references through the shared linker', () => {
  const here = fileURLToPath(import.meta.url)
  const webRoot = new URL('../', `file://${here}`)
  const read = (p) => readFileSync(new URL(p, webRoot), 'utf8')

  for (const p of ['src/components/CommentsPanel.vue', 'src/components/CommentSidebar.vue', 'src/components/DocumentViewer.vue']) {
    const src = read(p)
    assert.ok(src.includes('renderMention(c.body)') || src.includes('renderMention(comment.body)'), `${p} renders comments with renderMention`)
  }
  assert.ok(read('src/views/Documents.vue').includes('linkEntityRefs(escaped)'), 'Documents.vue links #references with linkEntityRefs')
  assert.ok(read('src/composables/useMention.js').includes('linkEntityReferences('), 'useMention delegates to linkEntityReferences')
})
