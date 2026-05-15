import { ref } from 'vue'

const toasts = ref([])
let nextId = 0

export function useToast() {
  function show(message, type = 'error', duration = 5000) {
    const id = nextId++
    toasts.value.push({ id, message, type })
    if (duration > 0) {
      setTimeout(() => dismiss(id), duration)
    }
  }

  function dismiss(id) {
    toasts.value = toasts.value.filter(t => t.id !== id)
  }

  function error(message) { show(message, 'error') }
  function success(message) { show(message, 'success', 3000) }
  function warning(message) { show(message, 'warning') }

  return { toasts, show, dismiss, error, success, warning }
}
