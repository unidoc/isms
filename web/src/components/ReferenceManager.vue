<template>
  <div class="space-y-2">
    <div class="flex items-center justify-between">
      <span class="text-[10px] font-semibold text-slate-500 uppercase tracking-wider">Links</span>
      <button v-if="editable" @click="showAdd = !showAdd"
        class="text-[10px] text-blue-400 hover:text-blue-300 transition-colors">
        {{ showAdd ? 'Cancel' : '+ Add link' }}
      </button>
    </div>

    <!-- Add new reference — universal search -->
    <div v-if="showAdd" class="relative">
      <input v-model="searchQuery" type="text" ref="searchInput"
        class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500"
        placeholder="Search documents, risks, incidents..."
        @input="doSearch"
        @focus="doSearch"
        @blur="hideDropdown"
        @keydown.down.prevent="selectedIdx = Math.min(selectedIdx + 1, searchResults.length - 1)"
        @keydown.up.prevent="selectedIdx = Math.max(selectedIdx - 1, 0)"
        @keydown.tab.prevent="searchResults.length && pickResult(searchResults[selectedIdx])"
        @keydown.enter.prevent="searchResults.length && pickResult(searchResults[selectedIdx])"
        @keydown.escape="showAdd = false" />
      <div v-if="showDropdown && searchResults.length > 0"
        class="absolute z-50 top-full left-0 right-0 mt-1 bg-slate-900 border border-slate-700 rounded-lg shadow-xl max-h-48 overflow-y-auto">
        <button v-for="(s, i) in searchResults" :key="s.type + ':' + s.id"
          @mousedown.prevent="pickResult(s)"
          class="w-full text-left px-3 py-1.5 text-xs transition-colors flex items-center gap-2"
          :class="i === selectedIdx ? 'bg-blue-600/30 text-white' : 'text-slate-300 hover:bg-slate-700'">
          <span class="px-1 py-0.5 rounded text-[9px] font-semibold flex-shrink-0"
            :class="typeColors[s.type] || 'bg-slate-800 text-slate-400'">{{ typeLabels[s.type] || s.type }}</span>
          <span class="text-slate-500 font-mono text-[10px] flex-shrink-0">{{ s.id }}</span>
          <span class="truncate">{{ s.title }}</span>
        </button>
      </div>
      <div v-if="showDropdown && searching" class="absolute z-50 top-full left-0 right-0 mt-1 bg-slate-900 border border-slate-700 rounded-lg p-2 text-[10px] text-slate-500">Searching...</div>
      <div v-if="showDropdown && !searching && searchResults.length === 0 && searched" class="absolute z-50 top-full left-0 right-0 mt-1 bg-slate-900 border border-slate-700 rounded-lg p-2 text-[10px] text-slate-500">No matches</div>
    </div>

    <!-- Existing references -->
    <div v-if="refs.length === 0 && !showAdd" class="text-xs text-slate-600">No linked items</div>
    <div v-else class="space-y-1">
      <div v-for="r in refs" :key="r.id" class="flex items-center gap-1.5 group">
        <router-link :to="refRoute(r)"
          class="flex-1 inline-flex items-center gap-1 px-2 py-1 rounded border text-[11px] font-medium no-underline transition-colors hover:brightness-125"
          :class="refColors(r)">
          <span class="opacity-60">{{ typeLabels[otherSide(r).type] || otherSide(r).type }}</span>
          <span class="truncate max-w-[180px]">{{ r.title || otherSide(r).id }}</span>
        </router-link>
        <button v-if="editable" @click="removeReference(r.id)"
          class="opacity-0 group-hover:opacity-100 p-0.5 text-slate-600 hover:text-red-400 transition-all">
          <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api.js'
import { useToast } from '../composables/useToast'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'

const { show: showError } = useToast()

const props = defineProps({
  entityType: { type: String, required: true },
  entityId: { type: String, required: true },
  editable: { type: Boolean, default: false },
})

const route = useRoute()
const { orgPath } = useCurrentOrg()
const refs = ref([])
const showAdd = ref(false)
const searchQuery = ref('')
const searchResults = ref([])
const searching = ref(false)
const searched = ref(false)
const showDropdown = ref(false)
const selectedIdx = ref(0)
const searchInput = ref(null)
let searchTimer = null

const typeLabels = {
  document: 'DOC', control: 'CTRL', policy: 'POL', procedure: 'PROC',
  clause: 'CLS', requirement: 'REQ', record: 'REC', guideline: 'GUIDE',
  risk: 'RISK', legal: 'LEGAL', asset: 'ASSET',
  supplier: 'SUP', system: 'SYS', incident: 'INC', change: 'CR',
  corrective_action: 'CA', audit: 'AUDIT', objective: 'OBJ', task: 'TASK', program: 'PROG',
}

const typeColors = {
  document: 'bg-blue-900/40 text-blue-300 border-blue-800/50',
  control: 'bg-indigo-900/40 text-indigo-300 border-indigo-800/50',
  policy: 'bg-blue-900/40 text-blue-300 border-blue-800/50',
  procedure: 'bg-blue-900/40 text-blue-300 border-blue-800/50',
  clause: 'bg-blue-900/40 text-blue-300 border-blue-800/50',
  requirement: 'bg-purple-900/40 text-purple-300 border-purple-800/50',
  record: 'bg-blue-900/40 text-blue-300 border-blue-800/50',
  guideline: 'bg-blue-900/40 text-blue-300 border-blue-800/50',
  risk: 'bg-red-900/40 text-red-300 border-red-800/50',
  legal: 'bg-purple-900/40 text-purple-300 border-purple-800/50',
  asset: 'bg-amber-900/40 text-amber-300 border-amber-800/50',
  supplier: 'bg-emerald-900/40 text-emerald-300 border-emerald-800/50',
  system: 'bg-cyan-900/40 text-cyan-300 border-cyan-800/50',
  incident: 'bg-orange-900/40 text-orange-300 border-orange-800/50',
  change: 'bg-sky-900/40 text-sky-300 border-sky-800/50',
  corrective_action: 'bg-pink-900/40 text-pink-300 border-pink-800/50',
  objective: 'bg-teal-900/40 text-teal-300 border-teal-800/50',
  task: 'bg-lime-900/40 text-lime-300 border-lime-800/50',
  program: 'bg-violet-900/40 text-violet-300 border-violet-800/50',
}

const typeRoutes = {
  risk: 'risks', legal: 'legal', document: 'documents', asset: 'assets',
  supplier: 'suppliers', system: 'systems', incident: 'incidents', change: 'changes',
  audit: 'audit', corrective_action: 'corrective-actions',
  objective: 'objectives', program: 'programs', task: 'tasks',
}

function otherSide(r) {
  // Prefer the server-resolved document subtype (control/policy/clause/etc) over
  // the generic 'document' wire-type so the badge reflects the document's role.
  if (r.source_type === props.entityType && String(r.source_id) === String(props.entityId)) {
    return { type: r.subtype || r.target_type, id: r.target_id }
  }
  return { type: r.subtype || r.source_type, id: r.source_id }
}

function refColors(r) { return typeColors[otherSide(r).type] || 'bg-slate-800 text-slate-400 border-slate-700' }

function refRoute(r) {
  const other = otherSide(r)
  const docTypes = ['document', 'control', 'policy', 'procedure', 'clause', 'requirement', 'record', 'guideline']
  if (docTypes.includes(other.type)) return orgPath(`/documents/${other.id}`)
  const base = typeRoutes[other.type]
  if (!base) return '#'
  const numId = other.id.replace(/^[A-Z]+-/, '')
  return orgPath(`/${base}/${numId}`)
}

async function doSearch() {
  showDropdown.value = true
  clearTimeout(searchTimer)
  searchTimer = setTimeout(async () => {
    searching.value = true
    searched.value = false
    try {
      const data = await api.search(searchQuery.value)
      const existing = new Set(refs.value.map(r => {
        const o = otherSide(r)
        return o.type + ':' + o.id
      }))
      searchResults.value = (data || [])
        .filter(s => !existing.has(s.type + ':' + s.id))
        .filter(s => !(s.type === props.entityType && s.id.toLowerCase() === props.entityId.toLowerCase()))
      selectedIdx.value = 0
    } catch (e) {
      searchResults.value = []
      showError('Search failed: ' + (e.message || 'unknown error'))
    }
    searching.value = false
    searched.value = true
  }, 150)
}

function hideDropdown() {
  setTimeout(() => { showDropdown.value = false }, 200)
}

// Document subtypes (clause/control/policy/etc) are surface labels from frontmatter;
// at the reference layer they all collapse to 'document' per the core "everything is a document" rule.
const DOC_SUBTYPES = new Set(['document', 'control', 'policy', 'procedure', 'clause', 'requirement', 'record', 'guideline'])

async function pickResult(item) {
  try {
    const targetType = DOC_SUBTYPES.has(item.type) ? 'document' : item.type
    await api.createReference({
      source_type: props.entityType,
      source_id: props.entityId,
      target_type: targetType,
      target_id: item.id,
    })
    searchQuery.value = ''
    showDropdown.value = false
    showAdd.value = false
    await loadRefs()
  } catch (e) {
    showError('Failed to add reference: ' + (e.message || 'unknown error'))
  }
}

async function removeReference(id) {
  try {
    await api.deleteReference(id)
    await loadRefs()
  } catch (e) {
    showError('Failed to remove reference: ' + (e.message || 'unknown error'))
  }
}

async function loadRefs() {
  try {
    const data = await api.getReferences(props.entityType, props.entityId)
    refs.value = data || []
  } catch (e) {
    refs.value = []
    showError('Failed to load references: ' + (e.message || 'unknown error'))
  }
}

watch(showAdd, (open) => {
  if (open) {
    nextTick(() => {
      searchInput.value?.focus()
      doSearch()
    })
  } else {
    searchQuery.value = ''
    searchResults.value = []
  }
})

onMounted(loadRefs)
watch(() => [props.entityType, props.entityId], loadRefs)
</script>
