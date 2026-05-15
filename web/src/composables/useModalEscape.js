import { watch, onUnmounted } from 'vue'

/**
 * Close a modal on Escape key, but NOT when focus is in an input/textarea/select.
 * @param {Ref<boolean>} visible - reactive ref controlling modal visibility
 * @param {Function} [onClose] - optional callback instead of setting visible=false
 */
export function useModalEscape(visible, onClose) {
  function handler(e) {
    if (e.key !== 'Escape') return
    const tag = document.activeElement?.tagName
    if (['INPUT', 'TEXTAREA', 'SELECT'].includes(tag)) return
    if (onClose) {
      onClose()
    } else {
      visible.value = false
    }
  }

  const stop = watch(visible, (open) => {
    if (open) {
      document.addEventListener('keydown', handler)
    } else {
      document.removeEventListener('keydown', handler)
    }
  }, { immediate: true })

  onUnmounted(() => {
    document.removeEventListener('keydown', handler)
    stop()
  })
}
