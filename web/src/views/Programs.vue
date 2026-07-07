<template>
  <div class="min-h-full">
    <div class="overflow-y-auto">
    <!-- Loading -->
    <div v-if="loading" class="max-w-6xl mx-auto px-8 py-10">
      <ListSkeleton :rows="5" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="max-w-6xl mx-auto px-8 py-12">
      <div class="bg-red-950/40 border border-red-900/50 rounded-lg p-6 text-red-300 text-sm">
        {{ error }}
      </div>
    </div>

    <!-- Main content -->
    <div v-else class="max-w-6xl mx-auto px-8 py-10 space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">Program Register</h1>
          <p class="text-sm text-slate-500 mt-1">Group ISMS objectives into programs of work</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canWrite" @click="showCreateForm = !showCreateForm"
            class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            Add Program
          </button>
          <SuggestNewButton entityType="program" typeLabel="Program" />
        </div>
      </div>

      <!-- Filter/search bar -->
      <div class="flex items-center gap-3 flex-wrap">
        <div class="relative flex-1 max-w-xs">
          <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
          </svg>
          <input v-model="searchQuery" type="text" placeholder="Search..."
            class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
        </div>
        <div class="ml-auto text-xs text-slate-500 tabular-nums">{{ filtered.length }} total</div>
      </div>

      <!-- Create program form (modal) -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreateForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreateForm = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
          <h3 class="text-sm font-semibold text-slate-300">Add Program</h3>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="block text-xs text-slate-500 mb-1">Key (uppercase) *</label>
              <input v-model="newProgram.key" @input="newProgram.key = newProgram.key.toUpperCase().replace(/[^A-Z0-9]/g, '')"
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500 uppercase"
                placeholder="AWARE" />
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1">Title *</label>
              <input v-model="newProgram.title"
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500"
                placeholder="Security Awareness" />
            </div>
          </div>
          <div class="text-[10px] text-slate-600 mt-1">You can add description and notes after creating.</div>
          <div class="flex justify-end gap-2 pt-2">
            <button @click="showCreateForm = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200">Cancel</button>
            <button @click="createProgram" :disabled="!newProgram.key || !newProgram.title"
              class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 disabled:text-slate-500 text-white text-sm font-medium rounded-lg transition-colors">
              Add
            </button>
          </div>
        </div>
      </div>
      </Transition>
      </Teleport>

      <!-- Programs table -->
      <div class="bg-slate-900 border border-slate-800 rounded-xl overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b border-slate-800">
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Key</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Title</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Description</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/50">
            <tr v-for="p in paged" :key="p.id"
              @click="selectProgram(p)"
              class="hover:bg-slate-800/30 transition-colors cursor-pointer"
              :class="selected?.id === p.id ? 'bg-slate-800/50' : ''">
              <td class="px-4 py-3 font-mono text-blue-400 font-medium whitespace-nowrap">{{ p.key }}</td>
              <td class="px-4 py-3 text-slate-200">{{ p.title }}</td>
              <td class="px-4 py-3 text-slate-500 truncate max-w-md">{{ p.description || '—' }}</td>
            </tr>
            <tr v-if="filtered.length === 0">
              <td colspan="3" class="px-4 py-8 text-center text-slate-600 text-sm">No programs yet</td>
            </tr>
          </tbody>
        </table>
        <Pagination :page="page" :pageSize="pageSize" :total="filtered.length" @update:page="page = $event" @update:pageSize="pageSize = $event" />
      </div>
    </div>

    <!-- Detail slide-over -->
    <Teleport to="body">
    <Transition name="modal">
    <div v-if="selected" class="fixed inset-0 z-50 flex items-start justify-center pt-[3vh] px-4">
      <div class="absolute inset-0 bg-black/60" @click="closeDetail" />
      <div class="relative w-full max-w-4xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-h-[90vh] flex flex-col">
        <!-- Header -->
        <div class="flex-shrink-0 border-b border-slate-800 px-6 py-3 flex items-center justify-between gap-4">
          <div class="flex items-center gap-6 min-w-0">
            <span class="text-[10px] font-mono uppercase tracking-wider text-blue-400 flex-shrink-0">{{ selected.key }}</span>
            <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selected.title }}</h2>
          </div>
          <button @click="closeDetail" class="p-1 rounded-lg text-slate-600 hover:text-slate-300 hover:bg-slate-800 transition-colors">
            <svg class="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <!-- Body: sidebar nav + content -->
        <div class="flex flex-1 min-h-0">
          <nav class="flex-shrink-0 w-28 border-r border-slate-800 py-3">
            <div class="space-y-0.5">
              <button v-for="t in detailTabs" :key="t.key" @click="detailTab = t.key"
                class="w-full text-left px-3 py-2 text-xs font-medium transition-colors"
                :class="detailTab === t.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                {{ t.label }}
              </button>
            </div>
          </nav>

          <div class="flex-1 overflow-y-auto min-h-0">
            <!-- OVERVIEW -->
            <template v-if="detailTab === 'overview'">
              <div class="px-6 py-5 space-y-5">
                <div class="flex items-center justify-between">
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Overview</div>
                  <button v-if="canWrite && !editing" @click="startEdit" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                </div>
                <template v-if="editing">
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Key</label>
                      <input v-model="editForm.key" @input="editForm.key = editForm.key.toUpperCase().replace(/[^A-Z0-9]/g, '')"
                        class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500 uppercase" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Title</label>
                      <input v-model="editForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Description</label>
                      <MarkdownField v-model="editForm.description" :rows="3" placeholder="What is this program?" />
                    </div>
                  </div>
                </template>
                <template v-else>
                  <div class="space-y-4">
                    <div>
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Description</div>
                      <div v-if="selected.description" class="text-sm text-slate-300 leading-relaxed doc-prose" v-html="renderMd(selected.description)"></div>
                      <div v-else class="text-sm text-slate-600">—</div>
                    </div>
                    <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Key</div>
                        <div class="text-sm text-slate-300 font-mono">{{ selected.key }}</div>
                      </div>
                      <div v-if="selected.created_at">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created</div>
                        <div class="text-sm text-slate-300">{{ formatDate(selected.created_at) }}</div>
                      </div>
                    </div>
                  </div>
                </template>
              </div>
            </template>

            <!-- NOTES -->
            <template v-if="detailTab === 'notes'">
              <div class="px-6 py-5 space-y-5">
                <div class="flex items-center justify-between">
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Notes</div>
                  <button v-if="canWrite && !editing" @click="startEdit" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                </div>
                <template v-if="editing">
                  <MarkdownField v-model="editForm.notes" :rows="12" placeholder="Add notes..." />
                </template>
                <template v-else>
                  <div v-if="selected.notes" class="text-sm doc-prose text-slate-300 leading-relaxed" v-html="renderMd(selected.notes)"></div>
                  <div v-else class="text-sm text-slate-600 italic">No notes yet.</div>
                </template>
              </div>
            </template>

            <!-- LINKS -->
            <template v-if="detailTab === 'links'">
              <div class="px-6 py-5">
                <ReferenceManager entityType="program" :entityId="selected.key" :editable="canWrite" />
              </div>
            </template>

            <!-- SUGGESTIONS -->
            <template v-if="detailTab === 'suggestions'">
              <div class="px-6 py-5">
                <SuggestionPanel entityType="program" :entityId="selected.key" :canReview="canWrite" @applied="loadAll" />
              </div>
            </template>

            <!-- COMMENTS -->
            <template v-if="detailTab === 'comments'">
              <div class="px-6 py-5">
                <CommentsPanel entityType="program" :entityId="selected.key" />
              </div>
            </template>

            <!-- HISTORY -->
            <template v-if="detailTab === 'history'">
              <div class="px-6 py-5 space-y-6">
                <HistoryPanel entityType="program" :entityId="String(selected.id)" />
                <div v-if="canWrite" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                  <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">Danger zone</div>
                  <div class="text-xs text-slate-400">Deleting this program also deletes all objectives under it. This cannot be undone.</div>
                  <button @click="deleteSelected" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                    Delete program
                  </button>
                </div>
              </div>
            </template>
          </div>
        </div>

        <!-- Footer (edit mode) -->
        <div v-if="editing" class="flex-shrink-0 border-t border-slate-800 px-6 py-3 flex justify-end gap-3">
          <button @click="cancelEdit" class="px-4 py-1.5 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
          <button @click="saveEdit" :disabled="saving || !editForm.key || !editForm.title" class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 text-white text-sm font-medium rounded-lg transition-colors">{{ saving ? 'Saving...' : 'Save' }}</button>
        </div>
      </div>
    </div>
    </Transition>
    </Teleport>

    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { renderMarkdown } from '../composables/useRenderMd.js'
import { useToast } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import { useModalEscape } from '../composables/useModalEscape.js'
import MarkdownField from '../components/MarkdownField.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
import SuggestionPanel from '../components/SuggestionPanel.vue'
import SuggestNewButton from '../components/SuggestNewButton.vue'
import CommentsPanel from '../components/CommentsPanel.vue'
import HistoryPanel from '../components/HistoryPanel.vue'
import Pagination from '../components/Pagination.vue'
import ListSkeleton from '../components/ListSkeleton.vue'

const route = useRoute()
const router = useRouter()
const { orgPath } = useCurrentOrg()
const { success: showSaved, error: showError } = useToast()
const { ask: confirmAsk } = useConfirm()

const userRole = ref('')
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')

const loading = ref(true)
const error = ref(null)
const programs = ref([])
const selected = ref(null)
const showCreateForm = ref(false)
const searchQuery = ref('')
const page = ref(1)
const pageSize = ref(50)

const detailTab = ref('overview')
const editing = ref(false)
const saving = ref(false)
const editForm = reactive({ key: '', title: '', description: '', notes: '' })
const newProgram = reactive({ key: '', title: '' })

useModalEscape(showCreateForm)
useModalEscape(computed(() => !!selected.value), () => closeDetail())

const detailTabs = [
  { key: 'overview', label: 'Overview' },
  { key: 'notes', label: 'Notes' },
  { key: 'links', label: 'Links' },
  { key: 'suggestions', label: 'Suggestions' },
  { key: 'comments', label: 'Comments' },
  { key: 'history', label: 'History' },
]

const renderMd = renderMarkdown

const filtered = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return programs.value
  return programs.value.filter(p =>
    (p.key || '').toLowerCase().includes(q) ||
    (p.title || '').toLowerCase().includes(q) ||
    (p.description || '').toLowerCase().includes(q))
})

const paged = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filtered.value.slice(start, start + pageSize.value)
})

watch([searchQuery, pageSize], () => { page.value = 1 })

function formatDate(dateStr) {
  if (!dateStr && dateStr !== 0) return ''
  const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
  return d.toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric' })
}

async function loadAll() {
  loading.value = true
  error.value = null
  try {
    const progs = await api.getPrograms()
    programs.value = Array.isArray(progs) ? progs : (progs?.data || [])
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

function selectProgram(p) {
  if (selected.value?.id === p.id) { closeDetail(); return }
  router.push(orgPath(`/programs/${p.id}`))
}

function closeDetail() {
  router.push(orgPath('/programs'))
}

async function openFromRoute(id) {
  // id may be a numeric program id (register list links) or a program key like
  // "AWARE" (cross-entity reference chips link by key). Match either, and pass the
  // raw value to the API — the backend resolves id-or-key.
  const numId = parseInt(id)
  let p = programs.value.find(x => x.id === numId || x.key === id)
  if (!p) {
    try { p = await api.getProgram(id) } catch { return }
  }
  if (!p) return
  selected.value = p
  detailTab.value = 'overview'
  editing.value = false
}

function startEdit() {
  Object.assign(editForm, {
    key: selected.value.key || '',
    title: selected.value.title || '',
    description: selected.value.description || '',
    notes: selected.value.notes || '',
  })
  editing.value = true
}

function cancelEdit() {
  editing.value = false
}

async function saveEdit() {
  if (!selected.value) return
  saving.value = true
  try {
    await api.updateProgram(selected.value.id, {
      key: editForm.key,
      title: editForm.title,
      description: editForm.description,
      notes: editForm.notes,
    })
    await loadAll()
    const fresh = programs.value.find(p => p.id === selected.value.id)
    if (fresh) selected.value = fresh
    editing.value = false
    showSaved('Saved')
  } catch (e) {
    showError('Failed to save: ' + e.message)
  } finally {
    saving.value = false
  }
}

async function createProgram() {
  try {
    const created = await api.createProgram({ key: newProgram.key, title: newProgram.title })
    newProgram.key = ''
    newProgram.title = ''
    showCreateForm.value = false
    await loadAll()
    // Drop the user into the new program's detail in edit mode to keep filling it in
    // (matches Risks/Objectives). The route watcher early-returns on the same id, so
    // the edit state set here survives the navigation.
    if (created && created.id) {
      let fresh = created
      try { fresh = await api.getProgram(created.id) } catch { /* fall back */ }
      selected.value = fresh
      detailTab.value = 'overview'
      startEdit()
      router.push(orgPath(`/programs/${created.id}`))
    }
  } catch (e) {
    error.value = e.message
  }
}

async function deleteSelected() {
  if (!selected.value) return
  if (!await confirmAsk(`Delete program "${selected.value.key}"? All objectives under it will also be deleted.`, { confirm: 'Delete', variant: 'danger' })) return
  try {
    await api.deleteProgram(selected.value.id)
    closeDetail()
    await loadAll()
  } catch (e) {
    error.value = e.message
  }
}

onMounted(async () => {
  try { const me = await api.getMe(); userRole.value = me?.role || '' } catch {}
  await loadAll()
  if (route.params.id) await openFromRoute(route.params.id)
})

watch(() => route.params.id, (id) => {
  if (!id) { selected.value = null; editing.value = false; return }
  if (selected.value?.id === parseInt(id)) return
  openFromRoute(id)
})
</script>
