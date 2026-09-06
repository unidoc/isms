import { ref } from 'vue'

import { i18n } from '../i18n.js'

// The dialog's two buttons are the same words on every screen, so they come
// from the shared action catalogue rather than being retyped as defaults here.
//
// `i18n.global.t` rather than `useI18n()`: this is a plain module with no
// component instance, the same reason useFormat/useEnumLabel/useNotificationRender
// reach for the global scope. Same keyspace either way.
//
// Resolved when the dialog opens, not reactively. The dialog is modal and
// transient — there is no path from an open dialog to a locale change — so a
// computed would buy nothing and would mean holding the caller's options in
// module state to recompute from.
const defaultConfirmLabel = () => i18n.global.t('common.action.confirm')
const defaultCancelLabel = () => i18n.global.t('common.action.cancel')

const visible = ref(false)
const message = ref('')
// Seeded empty rather than with English: every ask()/confirm() sets both before
// the dialog becomes visible, and resolving at module scope would read the
// catalogue before setLocale() has run during boot.
const confirmLabel = ref('')
const cancelLabel = ref('')
const variant = ref('danger') // 'danger' or 'warning'
let resolveFn = null

export function useConfirm() {
  function ask(msg, opts = {}) {
    message.value = msg
    confirmLabel.value = opts.confirm || defaultConfirmLabel()
    cancelLabel.value = opts.cancel || defaultCancelLabel()
    variant.value = opts.variant || 'danger'
    visible.value = true
    return new Promise(resolve => { resolveFn = resolve })
  }

  // Options-object form: confirm({ message, variant, confirmLabel, cancelLabel })
  function confirm(opts = {}) {
    message.value = opts.message || ''
    confirmLabel.value = opts.confirmLabel || defaultConfirmLabel()
    cancelLabel.value = opts.cancelLabel || defaultCancelLabel()
    variant.value = opts.variant || 'danger'
    visible.value = true
    return new Promise(resolve => { resolveFn = resolve })
  }

  function onConfirm() {
    visible.value = false
    if (resolveFn) resolveFn(true)
    resolveFn = null
  }

  function onCancel() {
    visible.value = false
    if (resolveFn) resolveFn(false)
    resolveFn = null
  }

  return { visible, message, confirmLabel, cancelLabel, variant, ask, confirm, onConfirm, onCancel }
}
