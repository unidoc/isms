<template>
  <!-- Reload the current page's list(s) without a full F5. Icon spins + button
       disables while loading. Wire @refresh to the view's loader and :loading to
       its loading ref. -->
  <button
    type="button"
    :disabled="loading"
    @click="$emit('refresh')"
    class="inline-flex items-center justify-center rounded-lg border border-slate-800 bg-slate-900 p-1.5 text-slate-400 transition-colors hover:border-slate-700 hover:text-slate-200 disabled:opacity-60 disabled:cursor-not-allowed"
    :title="loading ? 'Refreshing…' : 'Refresh'"
    aria-label="Refresh">
    <svg class="h-4 w-4" :class="{ 'animate-spin': loading }" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
      <path stroke-linecap="round" stroke-linejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
    </svg>
  </button>
</template>

<script setup>
defineProps({ loading: { type: Boolean, default: false } })
defineEmits(['refresh'])
</script>
