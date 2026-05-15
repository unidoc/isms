<template>
  <div class="relative">
    <button @click="showForm = !showForm"
      class="px-3 py-2 bg-amber-600/20 hover:bg-amber-600/30 text-amber-400 text-sm font-medium rounded-lg transition-colors">
      Suggest
    </button>

    <!-- Dropdown form -->
    <div v-if="showForm" class="absolute right-0 top-full mt-2 w-96 bg-slate-900 border border-slate-700 rounded-xl shadow-xl z-50 p-4 space-y-3">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-semibold text-slate-200">{{ formTitle }}</h3>
        <button @click="showForm = false" class="text-slate-500 hover:text-slate-300 text-xs">Close</button>
      </div>

      <!-- Detail context: show type picker (edit, reassess, review, link) -->
      <div v-if="isDetail && typeOptions.length > 1">
        <label class="block text-[10px] text-slate-600 mb-1">Action</label>
        <select v-model="form.suggestion_type" class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500">
          <option v-for="opt in typeOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
        </select>
      </div>

      <div>
        <label class="block text-[10px] text-slate-600 mb-1">Title</label>
        <input v-model="form.title" type="text"
          class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500"
          :placeholder="titlePlaceholder" />
      </div>
      <div v-if="form.suggestion_type === 'create'">
        <label class="block text-[10px] text-slate-600 mb-1">Description</label>
        <textarea v-model="form.description" rows="2"
          class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500 resize-none"
          placeholder="Description of the new entity..." />
      </div>
      <div>
        <label class="block text-[10px] text-slate-600 mb-1">Why is this needed?</label>
        <textarea v-model="form.rationale" rows="2"
          class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500 resize-none"
          placeholder="Rationale for this suggestion..." />
      </div>
      <div v-if="error" class="text-[10px] text-red-400">{{ error }}</div>
      <div class="flex gap-2">
        <button @click="submit" :disabled="!form.title.trim() || submitting"
          class="text-xs px-3 py-1.5 bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-white rounded-lg font-medium">
          {{ submitting ? 'Submitting...' : 'Submit' }}
        </button>
        <button @click="showForm = false" class="text-xs text-slate-500 hover:text-slate-300">Cancel</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { api } from '../api'

const props = defineProps({
  entityType: { type: String, required: true },
  typeLabel: { type: String, required: true },
  entityId: { type: String, default: '' },
})

const emit = defineEmits(['created'])

const showForm = ref(false)
const submitting = ref(false)
const error = ref('')
const form = ref({ suggestion_type: 'create', title: '', description: '', rationale: '' })

const isDetail = computed(() => !!props.entityId)

const formTitle = computed(() => {
  if (!isDetail.value) return `Suggest new ${props.typeLabel.toLowerCase()}`
  return `Suggest change to ${props.typeLabel}`
})

// Detail-only types (list always uses 'create', no picker needed)
const detailTypes = {
  risk:                [{ value: 'update', label: 'Edit risk' }, { value: 'reassess', label: 'Reassess risk rating' }],
  incident:            [{ value: 'update', label: 'Edit incident' }, { value: 'link', label: 'Link to other entities' }],
  supplier:            [{ value: 'update', label: 'Edit supplier' }, { value: 'reassess', label: 'Reassess supplier' }],
  legal_requirement:   [{ value: 'update', label: 'Edit requirement' }],
  change_request:      [{ value: 'update', label: 'Edit change request' }],
  corrective_action:   [{ value: 'update', label: 'Edit corrective action' }],
  task:                [{ value: 'update', label: 'Edit task' }],
  objective:           [{ value: 'update', label: 'Edit objective' }, { value: 'review', label: 'Request review' }],
  system:              [{ value: 'update', label: 'Edit system' }, { value: 'review', label: 'Request access review' }],
  asset:               [{ value: 'update', label: 'Edit asset' }, { value: 'review', label: 'Request asset review' }],
  audit_finding:       [{ value: 'update', label: 'Edit finding' }],
}

const typeOptions = computed(() => {
  if (!isDetail.value) return []
  return detailTypes[props.entityType] || [{ value: 'update', label: `Edit ${props.typeLabel}` }]
})

const titlePlaceholder = computed(() => {
  if (!isDetail.value) return `Describe the new ${props.typeLabel.toLowerCase()}...`
  const t = form.value.suggestion_type
  if (t === 'reassess') return 'Reassessment rationale...'
  if (t === 'link') return 'What should be linked and why...'
  if (t === 'review') return 'Why should this be reviewed...'
  return 'What should change...'
})

watch(() => showForm.value, (open) => {
  if (open) {
    form.value = {
      suggestion_type: isDetail.value ? (typeOptions.value[0]?.value || 'update') : 'create',
      title: '',
      description: '',
      rationale: '',
    }
    error.value = ''
  }
})

async function submit() {
  if (!form.value.title.trim()) return
  submitting.value = true
  error.value = ''
  try {
    const entityPayload = {}
    if (form.value.suggestion_type === 'create') {
      // For create: title and description go into payload for the entity
      if (props.entityType === 'supplier') {
        entityPayload.name = form.value.title
      } else {
        entityPayload.title = form.value.title
      }
      if (form.value.description) {
        entityPayload.description = form.value.description
      }
    }
    const payload = {
      entity_type: props.entityType,
      suggestion_type: form.value.suggestion_type,
      title: form.value.title,
      rationale: form.value.rationale,
      payload: entityPayload,
    }
    if (props.entityId && form.value.suggestion_type !== 'create') {
      payload.entity_id = props.entityId
    }
    await api.createSuggestion(payload)
    showForm.value = false
    emit('created')
  } catch (e) {
    error.value = e.message || 'Failed to create suggestion'
  } finally {
    submitting.value = false
  }
}
</script>
