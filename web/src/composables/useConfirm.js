import { ref } from 'vue'

const visible = ref(false)
const message = ref('')
const confirmLabel = ref('Confirm')
const cancelLabel = ref('Cancel')
const variant = ref('danger') // 'danger' or 'warning'
let resolveFn = null

export function useConfirm() {
  function ask(msg, opts = {}) {
    message.value = msg
    confirmLabel.value = opts.confirm || 'Confirm'
    cancelLabel.value = opts.cancel || 'Cancel'
    variant.value = opts.variant || 'danger'
    visible.value = true
    return new Promise(resolve => { resolveFn = resolve })
  }

  // Options-object form: confirm({ message, variant, confirmLabel, cancelLabel })
  function confirm(opts = {}) {
    message.value = opts.message || ''
    confirmLabel.value = opts.confirmLabel || 'Confirm'
    cancelLabel.value = opts.cancelLabel || 'Cancel'
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
