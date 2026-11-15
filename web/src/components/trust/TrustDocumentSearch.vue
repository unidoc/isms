<template>
  <div class="space-y-2" data-trust="doc-search">
    <input v-model="query" type="search" :placeholder="$t('trust_manager.picker.search_placeholder')" :aria-label="$t('trust_manager.picker.search')"
      class="w-full px-3 py-2 text-sm bg-slate-800 border border-slate-700 rounded-lg text-slate-200 placeholder-slate-500 focus:outline-none focus:border-blue-500" />
    <div v-if="loading" class="text-xs text-slate-500">{{ $t('trust_manager.picker.loading') }}</div>
    <div v-else-if="error" class="text-xs text-red-400">
      {{ renderApiError(error) }}
      <button type="button" @click="load" class="ml-2 text-blue-400 hover:text-blue-300">{{ $t('common.action.retry') }}</button>
    </div>
    <ul v-else class="max-h-64 overflow-y-auto divide-y divide-slate-800 border border-slate-800 rounded-lg">
      <li v-for="d in shown" :key="d.document_id">
        <button type="button" :data-trust-doc="d.document_id" @click="$emit('pick', d)"
          class="w-full text-left px-3 py-2 hover:bg-slate-800 transition-colors">
          <span class="block text-sm text-slate-200">{{ d.title }}</span>
          <span class="block text-xs text-slate-500 font-mono">{{ d.document_id }}</span>
        </button>
      </li>
      <li v-if="!shown.length" class="px-3 py-3 text-xs text-slate-500">{{ $t('trust_manager.picker.none') }}</li>
    </ul>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useOrgDocuments } from '../../composables/useOrgDocuments.js'
import { renderApiError } from '../../composables/useApiError.js'

// Search over the organization's documents. `exclude` hides document ids (those
// that already are items); `hideRetired` hides retired documents.
const props = defineProps({
  exclude: { type: Array, default: () => [] },
  hideRetired: { type: Boolean, default: false },
})
defineEmits(['pick'])

const { documents, loading, error, load } = useOrgDocuments()
const query = ref('')
const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  const skip = new Set(props.exclude.map((x) => String(x).toLowerCase()))
  return documents.value
    .filter((d) => !skip.has(d.document_id.toLowerCase()))
    .filter((d) => !(props.hideRetired && d.status === 'retired'))
    .filter((d) => !q || d.title.toLowerCase().includes(q) || d.document_id.toLowerCase().includes(q))
    .sort((a, b) => a.title.localeCompare(b.title))
})
onMounted(load)
</script>
