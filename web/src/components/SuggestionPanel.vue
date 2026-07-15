<template>
  <div class="space-y-4">
    <!-- List -->
    <div v-if="loading" class="h-10 bg-slate-800 rounded animate-pulse" />
    <div v-else-if="suggestions.length === 0 && !showCreate" class="text-sm text-slate-600 italic py-2">No suggestions yet.</div>
    <div v-else class="space-y-2">
      <!-- Make the nature of suggestions unmistakable: they're proposals
           pending a decision, not changes that have happened yet (#88).
           openCount covers both 'open' and 'in_review', so the wording must
           fit both — not just the 'Under review' state. -->
      <p v-if="openCount > 0" class="text-[11px] text-amber-400/90 bg-amber-500/5 border border-amber-500/15 rounded-md px-2.5 py-1.5">
        Proposals pending — these changes take effect only once a manager applies them.
      </p>
      <div v-for="sg in suggestions" :key="sg.id"
        class="bg-slate-800/40 border border-slate-700/40 rounded-lg px-4 py-3 space-y-2">
        <div class="flex items-center gap-2 flex-wrap">
          <span class="text-sm font-medium text-slate-300">{{ sg.title }}</span>
          <span class="px-1.5 py-0.5 rounded text-[9px] font-medium bg-blue-500/15 text-blue-400">{{ typeLabel(sg.suggestion_type) }}</span>
          <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-full text-[9px] font-semibold" :class="statusClass(sg.status)">{{ statusLabel(sg.status) }}</span>
          <span v-if="sg.suggested_by_type === 'agent'" class="px-1 py-0.5 rounded text-[9px] bg-purple-500/15 text-purple-400">AI</span>
        </div>
        <div v-if="sg.rationale" class="text-sm text-slate-500">{{ sg.rationale }}</div>
        <div class="text-[10px] text-slate-600">{{ sg.suggested_by }} · {{ formatDate(sg.created_at) }}</div>

        <!-- Actions -->
        <div v-if="canReview && sg.status === 'open'" class="flex items-center gap-1.5 pt-1">
          <button @click="apply(sg.id)" class="text-[10px] px-2.5 py-1 rounded bg-emerald-700 hover:bg-emerald-600 text-white font-medium transition-colors">Apply</button>
          <button v-if="rejecting !== sg.id" @click="rejecting = sg.id; rejectReason = ''"
            class="text-[10px] px-2.5 py-1 rounded bg-slate-700 hover:bg-red-800 text-slate-300 hover:text-red-200 font-medium transition-colors">Reject</button>
          <span v-if="applyError" class="text-[10px] text-red-400 ml-1">{{ applyError }}</span>
        </div>
        <div v-if="rejecting === sg.id" class="flex items-center gap-2">
          <input v-model="rejectReason" type="text" placeholder="Reason..."
            class="flex-1 bg-slate-800 border border-slate-700 rounded-lg px-2.5 py-1.5 text-sm text-white focus:outline-none focus:border-red-500"
            @keyup.enter="reject(sg.id)" />
          <button @click="reject(sg.id)" class="text-xs px-2.5 py-1 bg-red-600 hover:bg-red-500 text-white rounded-lg font-medium">Reject</button>
          <button @click="rejecting = null" class="text-xs text-slate-500 hover:text-slate-300">Cancel</button>
        </div>
        <div v-if="sg.reject_reason" class="text-[10px] text-red-400/70">Rejected: {{ sg.reject_reason }}</div>
        <div v-if="sg.applied_entity_id" class="text-[10px] text-emerald-400/70">Applied {{ sg.applied_entity_id }}</div>
      </div>
    </div>

    <!-- Add -->
    <div class="border-t border-slate-800 pt-4">
      <form v-if="showCreate" @submit.prevent="submitSuggestion" class="space-y-3">
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Type</label>
            <select v-model="newSuggestion.suggestion_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500">
              <option v-for="opt in entityTypeOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Title</label>
            <input v-model="newSuggestion.title" type="text" required
              class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
              placeholder="Short description..." />
          </div>
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-500 mb-1">Rationale</label>
          <textarea v-model="newSuggestion.rationale" rows="2"
            class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500 resize-none"
            placeholder="Why is this change needed?" />
        </div>
        <div v-if="createError" class="text-xs text-red-400">{{ createError }}</div>
        <div class="flex gap-2">
          <button type="submit" :disabled="!newSuggestion.title.trim() || creating"
            class="text-xs px-3 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white rounded-lg font-medium">
            {{ creating ? 'Creating...' : 'Submit' }}
          </button>
          <button type="button" @click="showCreate = false" class="text-xs text-slate-500 hover:text-slate-300">Cancel</button>
        </div>
      </form>
      <button v-else @click="showCreate = true"
        class="text-xs px-3 py-1.5 bg-blue-600/20 text-blue-400 hover:bg-blue-600/30 rounded-lg font-medium transition-colors">
        Suggest
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { api } from '../api'
import { useConfirm } from '../composables/useConfirm'
import { useToast } from '../composables/useToast'

const { show: showError, success: showSuccess } = useToast()

const { ask } = useConfirm()

const props = defineProps({
  entityType: { type: String, required: true },
  entityId: { type: String, required: true },
  canReview: { type: Boolean, default: false },
})

const emit = defineEmits(['applied'])

const suggestions = ref([])
const loading = ref(false)
const rejecting = ref(null)
const rejectReason = ref('')

const applyError = ref('')

// Create form
const showCreate = ref(false)
const creating = ref(false)
const createError = ref('')
const newSuggestion = ref({
  suggestion_type: 'update',
  title: '',
  rationale: '',
})

const openCount = computed(() => suggestions.value.filter(s => s.status === 'open' || s.status === 'in_review').length)

// Module-specific suggestion types for existing entities
const entityModuleTypes = {
  risk:                [{ value: 'update', label: 'Update fields' }, { value: 'reassess', label: 'Reassess rating' }, { value: 'link', label: 'Link to other entities' }],
  incident:            [{ value: 'update', label: 'Update fields' }, { value: 'link', label: 'Link to risks/systems' }],
  supplier:            [{ value: 'update', label: 'Update fields' }, { value: 'reassess', label: 'Reassess criticality' }],
  legal_requirement:   [{ value: 'update', label: 'Update fields' }],
  change_request:      [{ value: 'update', label: 'Update fields' }],
  corrective_action:   [{ value: 'update', label: 'Update fields' }],
  task:                [{ value: 'update', label: 'Update fields' }],
  objective:           [{ value: 'update', label: 'Update fields' }, { value: 'review', label: 'Request review' }],
  system:              [{ value: 'update', label: 'Update fields' }, { value: 'review', label: 'Request access review' }],
  asset:               [{ value: 'update', label: 'Update fields' }, { value: 'review', label: 'Request review' }],
}
const entityTypeOptions = computed(() => entityModuleTypes[props.entityType] || [{ value: 'update', label: 'Update fields' }])

async function load() {
  if (!props.entityType || !props.entityId) return
  loading.value = true
  try {
    const data = await api.getSuggestions({ entity_type: props.entityType, entity_id: props.entityId })
    suggestions.value = Array.isArray(data?.data) ? data.data : (Array.isArray(data) ? data : [])
  } catch { suggestions.value = [] }
  loading.value = false
}

async function submitSuggestion() {
  if (!newSuggestion.value.title.trim()) return
  creating.value = true
  createError.value = ''
  try {
    await api.createSuggestion({
      entity_type: props.entityType,
      entity_id: props.entityId,
      suggestion_type: newSuggestion.value.suggestion_type,
      title: newSuggestion.value.title,
      rationale: newSuggestion.value.rationale,
      payload: {},
    })
    newSuggestion.value = { suggestion_type: 'update', title: '', rationale: '' }
    showCreate.value = false
    showSuccess('Suggestion submitted — a manager will review it before it takes effect.') // #167
    await load()
  } catch (e) {
    createError.value = e.message || 'Failed to create suggestion'
  } finally {
    creating.value = false
  }
}

async function apply(id) {
  try {
    const result = await api.applySuggestion(id)
    if (result?.stale) {
      if (await ask('Entity has changed since this suggestion was created. Apply anyway?', { confirm: 'Apply anyway', variant: 'warning' })) {
        await api.applySuggestion(id, { force: true })
      } else {
        return
      }
    }
    await load()
    emit('applied')
  } catch (e) {
    applyError.value = e.message || 'Apply failed'
  }
}

async function reject(id) {
  const reason = rejectReason.value.trim() || 'Rejected'
  try {
    await api.rejectEntitySuggestion(id, reason)
    rejecting.value = null
    rejectReason.value = ''
    await load()
  } catch (e) {
    applyError.value = e.message || 'Reject failed'
  }
}

async function claim(id) {
  try {
    await api.claimSuggestion(id)
    await load()
  } catch (e) {
    showError('Failed to claim suggestion: ' + (e.message || 'unknown error'))
  }
}

function typeLabel(t) {
  return { create: 'New', update: 'Update', reassess: 'Reassess', link: 'Link', review: 'Review' }[t] || t
}

function statusClass(s) {
  switch (s) {
    case 'applied': return 'bg-emerald-500/15 text-emerald-400'
    case 'rejected': return 'bg-red-500/15 text-red-400'
    case 'in_review': return 'bg-amber-500/15 text-amber-400'
    case 'withdrawn': return 'bg-slate-500/15 text-slate-400'
    default: return 'bg-blue-500/15 text-blue-400'
  }
}

// Labels that say what the state means — "open" reads as "awaiting review", not
// as a finished change (#88).
function statusLabel(s) {
  return {
    open: 'Awaiting review',
    in_review: 'Under review',
    applied: 'Applied',
    rejected: 'Rejected',
    withdrawn: 'Withdrawn',
  }[s] || s.replace(/_/g, ' ')
}

function formatDate(d) {
  if (!d && d !== 0) return ''
  const dt = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  return dt.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })
}

onMounted(load)
watch(() => props.entityId, load)
</script>
