// Plan 80 task 1.7: the confirm dialog's two buttons come from the catalogue.
//
// Small surface, but the failure is invisible in English: the defaults were the
// literals 'Confirm'/'Cancel', which render correctly for the one locale that
// never needed translating and are wrong everywhere else. So the assertions
// that matter here are the id-ID ones.
import test from 'node:test'
import assert from 'node:assert/strict'
import { useConfirm } from '../src/composables/useConfirm.js'
import { i18n } from '../src/i18n.js'
import id from '../src/locales/id-ID/index.js'

i18n.global.setLocaleMessage('id-ID', id)

function withLocale(tag, fn) {
  const previous = i18n.global.locale.value
  i18n.global.locale.value = tag
  try {
    return fn()
  } finally {
    i18n.global.locale.value = previous
  }
}

// The composable is module-level shared state, so every test closes its dialog
// rather than leaving one open for the next.
function open(fn) {
  const c = useConfirm()
  const promise = fn(c)
  const labels = { confirm: c.confirmLabel.value, cancel: c.cancelLabel.value, visible: c.visible.value }
  c.onCancel()
  return { ...labels, promise }
}

test('the default labels come from common.action, not from literals', () => {
  const { confirm, cancel } = open((c) => c.ask('Delete this?'))
  assert.equal(confirm, 'Confirm')
  assert.equal(cancel, 'Cancel')
})

test('the active locale drives the labels', () => {
  withLocale('id-ID', () => {
    const { confirm, cancel } = open((c) => c.ask('Hapus ini?'))
    assert.equal(confirm, 'Konfirmasi')
    assert.equal(cancel, 'Batal')
  })
})

// Two call shapes exist — ask(msg, opts) and confirm({message, ...}) — and the
// defaults were duplicated across both, which is how they drift.
test('both call shapes resolve the same defaults', () => {
  withLocale('id-ID', () => {
    const viaAsk = open((c) => c.ask('x'))
    const viaConfirm = open((c) => c.confirm({ message: 'x' }))
    assert.equal(viaAsk.confirm, viaConfirm.confirm)
    assert.equal(viaAsk.cancel, viaConfirm.cancel)
    assert.equal(viaConfirm.confirm, 'Konfirmasi')
  })
})

test('an explicit label still wins over the catalogue default', () => {
  // The ~12 call sites passing 'Discard'/'Delete' are untouched by this task;
  // they are template text and belong to the extraction phase.
  const viaAsk = open((c) => c.ask('x', { confirm: 'Discard', cancel: 'Keep' }))
  assert.equal(viaAsk.confirm, 'Discard')
  assert.equal(viaAsk.cancel, 'Keep')
  const viaConfirm = open((c) => c.confirm({ message: 'x', confirmLabel: 'Delete' }))
  assert.equal(viaConfirm.confirm, 'Delete')
  assert.equal(viaConfirm.cancel, 'Cancel') // unset half still takes the default
})

test('labels are resolved by the time the dialog is visible', () => {
  // App.vue renders the buttons only under v-if="visible", so the empty seed
  // values must never be reachable. Asserting visibility and labels together is
  // what pins that.
  const { visible, confirm, cancel } = open((c) => c.ask('x'))
  assert.equal(visible, true)
  assert.notEqual(confirm, '')
  assert.notEqual(cancel, '')
})

test('the promise still resolves through the dialog buttons', () => {
  // The labels changed; the control flow must not have.
  const c = useConfirm()
  const confirmed = c.ask('x')
  c.onConfirm()
  const cancelled = c.ask('y')
  c.onCancel()
  return Promise.all([confirmed, cancelled]).then(([a, b]) => {
    assert.equal(a, true)
    assert.equal(b, false)
  })
})
