import { computed, ref } from 'vue'

import { i18n } from '../i18n.js'

// The dialog's two buttons are the same words on every screen, so they come
// from the shared action catalogue rather than being retyped as defaults here.
//
// `i18n.global.t` rather than `useI18n()`: this is a plain module with no
// component instance, the same reason useFormat/useEnumLabel/useNotificationRender
// reach for the global scope. Same keyspace either way.
//
// Resolved reactively, not snapshotted when the dialog opens.
//
// A first draft snapshotted them, on the reasoning that the dialog is modal so
// no user can reach the locale picker while it is up. That is true and it is
// not the whole picture, as review pointed out: `main.js:16` calls the async
// setLocale() WITHOUT awaiting it before mounting, and applyConfigLocales() and
// applySessionLocale() re-apply the locale again when GET /config and GET /me
// land. So the app is interactive before the locale settles, and a dialog
// opened in that window would keep its old-language buttons while the page
// behind it switched. Narrow and cosmetic, but the reactive version is also
// simply the simpler code — it needs no seed value and no ordering argument.

const visible = ref(false)
const message = ref('')
// What the caller asked for, or '' to follow the catalogue. Storing the
// override rather than the resolved label is what lets the default stay live.
const confirmOverride = ref('')
const cancelOverride = ref('')
const confirmLabel = computed(() => confirmOverride.value || i18n.global.t('common.action.confirm'))
const cancelLabel = computed(() => cancelOverride.value || i18n.global.t('common.action.cancel'))
const variant = ref('danger') // 'danger' or 'warning'
let resolveFn = null

export function useConfirm() {
  function ask(msg, opts = {}) {
    message.value = msg
    confirmOverride.value = opts.confirm || ''
    cancelOverride.value = opts.cancel || ''
    variant.value = opts.variant || 'danger'
    visible.value = true
    return new Promise(resolve => { resolveFn = resolve })
  }

  // Options-object form: confirm({ message, variant, confirmLabel, cancelLabel })
  function confirm(opts = {}) {
    message.value = opts.message || ''
    confirmOverride.value = opts.confirmLabel || ''
    cancelOverride.value = opts.cancelLabel || ''
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
