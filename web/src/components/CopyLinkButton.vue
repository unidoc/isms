<template>
  <button
    type="button"
    @click="copy"
    :title="copied ? 'Link copied' : 'Copy link to this item'"
    :aria-label="copied ? 'Link copied' : 'Copy link to this item'"
    class="p-1 rounded-lg transition-colors"
    :class="copied ? 'text-emerald-400' : 'text-slate-600 hover:text-slate-300 hover:bg-slate-800'"
  >
    <svg v-if="!copied" class="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
      <path stroke-linecap="round" stroke-linejoin="round" d="M13.828 10.172a4 4 0 010 5.656l-3 3a4 4 0 01-5.656-5.656l1.5-1.5m6.828-6.828l1.5-1.5a4 4 0 015.656 5.656l-3 3a4 4 0 01-5.656 0" />
    </svg>
    <svg v-else class="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
      <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
    </svg>
  </button>
</template>

<script setup>
import { ref } from 'vue'

const copied = ref(false)

function markCopied() {
  copied.value = true
  setTimeout(() => { copied.value = false }, 1500)
}

// Copies the current page URL. After #166 a detail view's URL is the shareable
// identifier deep-link (e.g. …/tasks/TASK-6), so "copy link" is just the href.
// The Clipboard API needs a secure context (https / localhost); self-hosted
// plain-http boxes fall back to execCommand so copy works everywhere — mirrors
// the code-copy handler in App.vue.
function copy() {
  const url = window.location.href
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(url).then(markCopied).catch(() => {})
    return
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = url
    ta.style.position = 'fixed'
    ta.style.top = '-9999px'
    document.body.appendChild(ta)
    ta.focus()
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    if (ok) markCopied()
  } catch (_) { /* clipboard unavailable — no-op */ }
}
</script>
