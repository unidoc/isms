<template>
  <div class="space-y-2">
    <label class="block text-[10px] font-semibold text-slate-500 uppercase tracking-wider">Links</label>

    <!-- Collected references (chips) -->
    <div v-if="collected.length > 0" class="flex flex-wrap gap-1.5">
      <span v-for="(r, i) in collected" :key="i"
        class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[11px] font-medium"
        :class="typeColors[r.type] || 'bg-slate-800 text-slate-400'">
        <span class="opacity-60 text-[9px]">{{ typeLabels[r.type] || r.type }}</span>
        {{ r.title || r.id }}
        <button @click="remove(i)" class="hover:text-red-300 ml-0.5">&times;</button>
      </span>
    </div>

    <!-- Universal search -->
    <div class="relative">
      <input v-model="searchQuery" type="text"
        class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500"
        placeholder="Search documents, risks, incidents..."
        @input="doSearch"
        @focus="doSearch"
        @blur="hideDropdown"
        @keydown.down.prevent="selectedIdx = Math.min(selectedIdx + 1, searchResults.length - 1)"
        @keydown.up.prevent="selectedIdx = Math.max(selectedIdx - 1, 0)"
        @keydown.tab.prevent="searchResults.length && selectItem(searchResults[selectedIdx])"
        @keydown.enter.prevent="searchResults.length && selectItem(searchResults[selectedIdx])"
        @keydown.escape="showDropdown = false" />
      <div v-if="showDropdown && searchResults.length > 0"
        class="absolute z-50 top-full left-0 right-0 mt-1 bg-slate-900 border border-slate-700 rounded-lg shadow-xl max-h-48 overflow-y-auto">
        <button v-for="(s, i) in searchResults" :key="s.type + ':' + s.id"
          @mousedown.prevent="selectItem(s)"
          class="w-full text-left px-3 py-1.5 text-xs transition-colors flex items-center gap-2"
          :class="i === selectedIdx ? 'bg-blue-600/30 text-white' : 'text-slate-300 hover:bg-slate-700'">
          <span class="px-1 py-0.5 rounded text-[9px] font-semibold flex-shrink-0"
            :class="typeColors[s.type] || 'bg-slate-800 text-slate-400'">{{ typeLabels[s.type] || s.type }}</span>
          <span class="text-slate-500 font-mono text-[10px] flex-shrink-0">{{ s.id }}</span>
          <span class="truncate">{{ s.title }}</span>
        </button>
      </div>
      <div v-if="showDropdown && searching" class="absolute z-50 top-full left-0 right-0 mt-1 bg-slate-900 border border-slate-700 rounded-lg p-2 text-[10px] text-slate-500">Searching...</div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { api } from '../api.js'

const props = defineProps({
  excludeType: { type: String, default: '' },
  modelValue: { type: Array, default: () => [] },
})

const emit = defineEmits(['update:modelValue'])

const collected = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const searchQuery = ref('')
const searchResults = ref([])
const searching = ref(false)
const showDropdown = ref(false)
const selectedIdx = ref(0)
let searchTimer = null

const typeLabels = {
  document: 'DOC', control: 'CTRL', policy: 'POL', procedure: 'PROC',
  clause: 'CLS', requirement: 'REQ', record: 'REC', guideline: 'GUIDE',
  risk: 'RISK', legal: 'LEGAL', asset: 'ASSET',
  supplier: 'SUP', system: 'SYS', incident: 'INC', change: 'CR',
  corrective_action: 'CA', objective: 'OBJ', task: 'TASK', program: 'PROG',
}

const typeColors = {
  document: 'bg-blue-900/40 text-blue-300',
  control: 'bg-indigo-900/40 text-indigo-300',
  policy: 'bg-blue-900/40 text-blue-300',
  procedure: 'bg-blue-900/40 text-blue-300',
  clause: 'bg-blue-900/40 text-blue-300',
  requirement: 'bg-purple-900/40 text-purple-300',
  record: 'bg-blue-900/40 text-blue-300',
  guideline: 'bg-blue-900/40 text-blue-300',
  risk: 'bg-red-900/40 text-red-300',
  legal: 'bg-purple-900/40 text-purple-300',
  asset: 'bg-amber-900/40 text-amber-300',
  supplier: 'bg-emerald-900/40 text-emerald-300',
  system: 'bg-cyan-900/40 text-cyan-300',
  incident: 'bg-orange-900/40 text-orange-300',
  change: 'bg-sky-900/40 text-sky-300',
  corrective_action: 'bg-pink-900/40 text-pink-300',
  objective: 'bg-teal-900/40 text-teal-300',
  task: 'bg-lime-900/40 text-lime-300',
  program: 'bg-violet-900/40 text-violet-300',
}

async function doSearch() {
  showDropdown.value = true
  clearTimeout(searchTimer)
  searchTimer = setTimeout(async () => {
    searching.value = true
    try {
      const data = await api.search(searchQuery.value)
      const already = new Set(collected.value.map(r => r.type + ':' + r.id))
      searchResults.value = (data || [])
        .filter(s => s.type !== props.excludeType)
        .filter(s => !already.has(s.type + ':' + s.id))
      selectedIdx.value = 0
    } catch { searchResults.value = [] }
    searching.value = false
  }, 150)
}

function hideDropdown() {
  setTimeout(() => { showDropdown.value = false }, 200)
}

function selectItem(item) {
  collected.value = [...collected.value, { type: item.type, id: item.id, title: item.title }]
  searchQuery.value = ''
  showDropdown.value = false
}

function remove(index) {
  collected.value = collected.value.filter((_, i) => i !== index)
}
</script>
