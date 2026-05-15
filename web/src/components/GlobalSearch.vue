<template>
  <!-- Modal overlay -->
  <Teleport to="body">
    <div v-if="visible" class="fixed inset-0 z-[100] flex items-start justify-center pt-[15vh]" @click.self="close">
      <div class="absolute inset-0 bg-black/60" @click="close" />
      <div class="relative w-full max-w-lg mx-4 bg-slate-900 border border-slate-700 rounded-xl shadow-2xl overflow-hidden" @click.stop>
        <!-- Search input -->
        <div class="flex items-center gap-3 px-4 border-b border-slate-800">
          <svg class="w-4 h-4 text-slate-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="m21 21-5.197-5.197m0 0A7.5 7.5 0 1 0 5.196 5.196a7.5 7.5 0 0 0 10.607 10.607Z" />
          </svg>
          <input ref="inputEl" v-model="query" type="text"
            class="flex-1 bg-transparent text-sm text-white py-3 outline-none placeholder-slate-500"
            placeholder="Search documents, risks, suppliers, incidents..."
            @input="onInput"
            @keydown.down.prevent="moveSelection(1)"
            @keydown.up.prevent="moveSelection(-1)"
            @keydown.enter.prevent="selectCurrent"
            @keydown.escape="close" />
          <kbd class="text-[10px] text-slate-600 bg-slate-800 border border-slate-700 rounded px-1.5 py-0.5 font-mono">ESC</kbd>
        </div>

        <!-- Results -->
        <div class="max-h-[50vh] overflow-y-auto" ref="resultsEl">
          <!-- Loading -->
          <div v-if="loading && results.length === 0" class="px-4 py-8 text-center text-xs text-slate-600">
            Searching...
          </div>

          <!-- No results -->
          <div v-else-if="searched && results.length === 0 && query.length > 0" class="px-4 py-8 text-center text-xs text-slate-600">
            No results for "{{ query }}"
          </div>

          <!-- Results list -->
          <div v-else-if="results.length > 0" class="py-1">
            <button v-for="(r, i) in results" :key="r.type + ':' + r.id"
              @click="navigate(r)" @mouseenter="selectedIdx = i"
              class="w-full text-left px-4 py-2 flex items-center gap-2.5 transition-colors"
              :class="i === selectedIdx ? 'bg-blue-600/20 text-white' : 'text-slate-300 hover:bg-slate-800/50'"
              :ref="el => { if (i === selectedIdx) activeEl = el }">
              <span class="px-1.5 py-0.5 rounded text-[9px] font-semibold flex-shrink-0 uppercase"
                :class="typeColors[r.type] || 'bg-slate-800 text-slate-400'">
                {{ typeLabels[r.type] || r.type }}
              </span>
              <span class="text-slate-500 font-mono text-[10px] flex-shrink-0">{{ r.id }}</span>
              <span class="text-sm truncate">{{ r.title }}</span>
            </button>
          </div>

          <!-- Empty state (no query) -->
          <div v-else class="px-4 py-6 text-center text-xs text-slate-600">
            Type to search across all entities
          </div>
        </div>

        <!-- Footer hint -->
        <div v-if="results.length > 0" class="px-4 py-2 border-t border-slate-800 flex items-center gap-4 text-[10px] text-slate-600">
          <span class="flex items-center gap-1">
            <kbd class="px-1 py-0.5 bg-slate-800 border border-slate-700 rounded font-mono">↑↓</kbd> navigate
          </span>
          <span class="flex items-center gap-1">
            <kbd class="px-1 py-0.5 bg-slate-800 border border-slate-700 rounded font-mono">↵</kbd> open
          </span>
          <span class="flex items-center gap-1">
            <kbd class="px-1 py-0.5 bg-slate-800 border border-slate-700 rounded font-mono">esc</kbd> close
          </span>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup>
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'

const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()

const visible = ref(false)
const query = ref('')
const results = ref([])
const loading = ref(false)
const searched = ref(false)
const selectedIdx = ref(0)
const inputEl = ref(null)
const resultsEl = ref(null)
const activeEl = ref(null)
let searchTimer = null

const isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0

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

const typeRoutes = {
  document: 'documents',
  risk: 'risks',
  legal: 'legal',
  asset: 'assets',
  supplier: 'suppliers',
  system: 'systems',
  incident: 'incidents',
  change: 'changes',
  corrective_action: 'corrective-actions',
  objective: 'objectives',
  task: 'tasks',
  program: 'objectives',
}

function open() {
  visible.value = true
  query.value = ''
  results.value = []
  searched.value = false
  selectedIdx.value = 0
  nextTick(() => inputEl.value?.focus())
}

function close() {
  visible.value = false
  query.value = ''
  results.value = []
}

function onInput() {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(doSearch, 120)
}

async function doSearch() {
  loading.value = true
  searched.value = false
  try {
    const data = await api.search(query.value)
    results.value = (data || []).slice(0, 50)
    selectedIdx.value = 0
  } catch {
    results.value = []
  }
  loading.value = false
  searched.value = true
}

function moveSelection(delta) {
  if (results.value.length === 0) return
  selectedIdx.value = Math.max(0, Math.min(selectedIdx.value + delta, results.value.length - 1))
  nextTick(() => {
    activeEl.value?.scrollIntoView?.({ block: 'nearest' })
  })
}

function selectCurrent() {
  if (results.value.length > 0 && selectedIdx.value < results.value.length) {
    navigate(results.value[selectedIdx.value])
  }
}

function navigate(item) {
  // Need either a subdomain-derived slug or a path-param slug to navigate.
  if (!orgSlug.value) return

  let path
  const docTypes = ['document', 'control', 'policy', 'procedure', 'clause', 'requirement', 'record', 'guideline']
  if (docTypes.includes(item.type)) {
    path = orgPath(`/documents/${item.id}`)
  } else {
    const base = typeRoutes[item.type]
    if (!base) return
    const numId = item.id.replace(/^[A-Z]+-/, '')
    path = orgPath(`/${base}/${numId}`)
  }

  close()
  router.push(path)
}

function onKeydown(e) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
    e.preventDefault()
    if (visible.value) {
      close()
    } else {
      open()
    }
  }
}

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
})

defineExpose({ open })
</script>
