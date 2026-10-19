import test, { mock } from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'
import { ref } from 'vue'

const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://acme.example/acme/documents' })
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.localStorage = dom.window.localStorage

// saveEdit() leaves a 3s "Saved" message timer behind; fake setTimeout so it can't hold the runner open.
mock.timers.enable({ apis: ['setTimeout'] })

// The composable registers onBeforeUnmount outside a component; vue warns once per call. Not a failure here.
const warn = console.warn
console.warn = (...args) => { if (!String(args[0]).includes('onBeforeUnmount')) warn(...args) }

const { useDocumentEditor } = await import('../src/composables/useDocumentEditor.js')

// Builds the composable around plain refs and a fake api that records the
// arguments of updateDocumentContent(docId, content, version, author, owner).
function setup(doc) {
  const calls = []
  const activeDoc = ref({ ...doc })
  const editor = useDocumentEditor({
    activeId: ref('doc-1'),
    activeDoc,
    activeType: ref('policies'),
    rawContent: ref('body'),
    api: {
      updateDocumentContent: async (...args) => { calls.push(args) },
    },
    loadContent: async () => {},
    loadChangedDocs: async () => {},
    loadNeedsReview: async () => {},
  })
  return { editor, activeDoc, calls }
}

const doc = { version: '1.0', author: 'a@acme.io', owner: 'o@acme.io' }

test('a changed author is sent even when the caller also wrote it to activeDoc', async () => {
  const { editor, activeDoc, calls } = setup(doc)
  editor.startEdit()
  // What Documents.vue setDocAuthor did in edit mode: buffer AND live document.
  editor.editAuthor.value = 'b@acme.io'
  activeDoc.value.author = 'b@acme.io'
  await editor.saveEdit()
  assert.equal(calls.length, 1)
  assert.equal(calls[0][3], 'b@acme.io')
})

test('a changed author is sent when only the edit buffer changed', async () => {
  const { editor, calls } = setup(doc)
  editor.startEdit()
  editor.editAuthor.value = 'b@acme.io'
  await editor.saveEdit()
  assert.equal(calls[0][3], 'b@acme.io')
})

test('an unchanged author is not sent', async () => {
  const { editor, calls } = setup(doc)
  editor.startEdit()
  await editor.saveEdit()
  assert.equal(calls.length, 1)
  assert.equal(calls[0][3], undefined)
})

test('owner is sent only when changed', async () => {
  const changed = setup(doc)
  changed.editor.startEdit()
  changed.editor.editOwner.value = 'p@acme.io'
  await changed.editor.saveEdit()
  assert.equal(changed.calls[0][4], 'p@acme.io')

  const same = setup(doc)
  same.editor.startEdit()
  await same.editor.saveEdit()
  assert.equal(same.calls[0][4], undefined)
})

test('a version-only edit sends just the version', async () => {
  const { editor, calls } = setup(doc)
  editor.startEdit()
  editor.editVersion.value = '1.1'
  await editor.saveEdit()
  const [docId, content, version, author, owner] = calls[0]
  assert.equal(docId, 'doc-1')
  assert.equal(content, 'body')
  assert.equal(version, '1.1')
  assert.equal(author, undefined)
  assert.equal(owner, undefined)
})

// Edit must not open before the document's content has loaded: startEdit copies
// rawContent once, so an early click left the editor empty and Save wiped the body.
function setupReady(contentReady) {
  const rawContent = ref('')
  const editor = useDocumentEditor({
    activeId: ref('doc-ready'),
    activeDoc: ref({ ...doc }),
    activeType: ref('policies'),
    rawContent,
    contentReady,
    api: { updateDocumentContent: async () => {} },
    loadContent: async () => {},
    loadChangedDocs: async () => {},
    loadNeedsReview: async () => {},
  })
  return { editor, rawContent }
}

test('startEdit does nothing while the content is not ready', () => {
  const { editor } = setupReady(ref(false))
  editor.editContent.value = 'untouched'
  editor.startEdit()
  try {
    assert.equal(editor.editMode.value, false)
    assert.equal(editor.editContent.value, 'untouched')
  } finally {
    editor.cancelEdit() // stops the autosave interval a wrongly-entered edit leaves running
  }
})

test('startEdit copies the content once it is ready', () => {
  const contentReady = ref(false)
  const { editor, rawContent } = setupReady(contentReady)
  try {
    editor.startEdit()
    assert.equal(editor.editMode.value, false)
    rawContent.value = 'loaded body'
    contentReady.value = true
    editor.startEdit()
    assert.equal(editor.editMode.value, true)
    assert.equal(editor.editContent.value, 'loaded body')
  } finally {
    editor.cancelEdit()
  }
})

test('startEdit behaves as before when no contentReady is passed', () => {
  const { editor, rawContent } = setupReady(undefined)
  rawContent.value = 'body'
  editor.startEdit()
  try {
    assert.equal(editor.editMode.value, true)
    assert.equal(editor.editContent.value, 'body')
  } finally {
    editor.cancelEdit()
  }
})
