// A <button> with no type attribute is a submit button, so inside a <form> it
// submits the form. The New Document dialog is a form: its advanced toggle was
// typeless and created the document early (issue #433). The folder link and the
// MemberPicker's buttons were typeless too but unmount in their own click
// handler, so they were latent. Every button reachable from that form must say
// what it is. Create is a type="button" click as well: only clicking it creates
// a document, never Enter in a field.
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
  const forms = src.match(/<form\b[^>]*@submit\.prevent[^>]*>.*?<\/form>/gs) ?? []
  assert.equal(forms.length, 1, `expected 1 New Document form, found ${forms.length}`)
  const form = forms[0]
  assert.match(form, /<MemberPicker\b/, 'form no longer holds a MemberPicker')
  const tags = buttonTags(form)
  assert.ok(tags.length >= 4, `expected at least 4 buttons, found ${tags.length}`)
  for (const tag of tags) {
    assert.match(tag, /\stype=/, `typeless button: ${tag}`)
  }
})

// Enter in the picker's search box is implicit form submission; with Create as
// the form's default button it would create the document with the old author.
test('the MemberPicker search input swallows Enter', () => {
  const src = readFileSync(PICKER, 'utf8')
  const inputs = src.match(/<input\b[^>]*>/gs) ?? []
  const search = inputs.filter(t => t.includes('ref="searchInput"'))
  assert.equal(search.length, 1, `expected 1 search input, found ${search.length}`)
  assert.match(search[0], /@keydown\.enter\.prevent/, `Enter not prevented: ${search[0]}`)
})

// Only an explicit click on Create creates a document (#433): the form has no
// submit button, so Enter in a field never submits it, and its own submit event
// is swallowed without a handler.
function newDocForm() {
  const src = readFileSync(DOCUMENTS, 'utf8')
  return src.match(/<form\b[^>]*@submit\.prevent[^>]*>.*?<\/form>/gs)[0]
}

test('the New Document form has no submit button', () => {
  const form = newDocForm()
  assert.ok(!/type="submit"/.test(form), 'a submit button lets Enter in a field create the document')
})

test('the New Document form swallows submit without a handler', () => {
  const tag = newDocForm().match(/^<form\b[^>]*>/s)[0]
  assert.match(tag, /@submit\.prevent(?![.=\w])/, `bare @submit.prevent missing: ${tag}`)
  assert.ok(!/@submit\.prevent="/.test(tag), `form still has a submit handler: ${tag}`)
})

test('Create Document creates on click', () => {
  const create = buttonTags(newDocForm()).filter(t => t.includes('@click="createDocument"'))
  assert.equal(create.length, 1, `expected 1 Create click button, found ${create.length}`)
})
