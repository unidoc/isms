// A <button> with no type attribute is a submit button, so inside a <form> it
// submits the form. The New Document dialog is a form, and its folder link,
// advanced toggle and the MemberPicker's options were typeless, so using them
// created the document early (issue #433). Every button reachable from that
// form must say what it is.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const PICKER = new URL('../src/components/MemberPicker.vue', import.meta.url).pathname
const DOCUMENTS = new URL('../src/views/Documents.vue', import.meta.url).pathname

const buttonTags = (src) => src.match(/<button\b[^>]*>/gs) ?? []

test('every button in MemberPicker.vue declares a type', () => {
  const tags = buttonTags(readFileSync(PICKER, 'utf8'))
  assert.ok(tags.length >= 2, `expected at least 2 buttons, found ${tags.length}`)
  for (const tag of tags) {
    assert.match(tag, /\stype=/, `typeless button: ${tag}`)
  }
})

test('every button in the New Document form declares a type', () => {
  const src = readFileSync(DOCUMENTS, 'utf8')
  const forms = src.match(/<form\b[^>]*@submit\.prevent="createDocument"[^>]*>.*?<\/form>/gs) ?? []
  assert.equal(forms.length, 1, `expected 1 New Document form, found ${forms.length}`)
  const form = forms[0]
  assert.match(form, /<MemberPicker\b/, 'form no longer holds a MemberPicker')
  const tags = buttonTags(form)
  assert.ok(tags.length >= 4, `expected at least 4 buttons, found ${tags.length}`)
  for (const tag of tags) {
    assert.match(tag, /\stype=/, `typeless button: ${tag}`)
  }
})
