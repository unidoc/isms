<template>
  <div v-if="canSuggest" class="relative">
    <button @click="showForm = !showForm"
      class="px-3 py-2 bg-amber-600/20 hover:bg-amber-600/30 text-amber-400 text-sm font-medium rounded-lg transition-colors">
      {{ $t('components.suggest_new.button') }}
    </button>

    <!-- Dropdown form -->
    <div v-if="showForm" class="absolute right-0 top-full mt-2 w-96 bg-slate-900 border border-slate-700 rounded-xl shadow-xl z-50 p-4 space-y-3">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-semibold text-slate-200">{{ formTitle }}</h3>
        <button @click="showForm = false" class="text-slate-500 hover:text-slate-300 text-xs">{{ $t('components.suggest_new.close') }}</button>
      </div>

      <!-- Detail context: show type picker (edit, reassess, review, link) -->
      <div v-if="isDetail && typeOptions.length > 1">
        <label class="block text-[10px] text-slate-600 mb-1">{{ $t('components.suggest_new.action') }}</label>
        <select v-model="form.suggestion_type" class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500">
          <option v-for="opt in typeOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
        </select>
      </div>

      <div>
        <label class="block text-[10px] text-slate-600 mb-1">{{ $t('components.suggest_new.title') }}</label>
        <input v-model="form.title" type="text"
          class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500"
          :placeholder="titlePlaceholder" />
      </div>
      <div v-if="form.suggestion_type === 'create'">
        <label class="block text-[10px] text-slate-600 mb-1">{{ $t('components.suggest_new.description') }}</label>
        <textarea v-model="form.description" rows="2"
          class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500 resize-none"
          :placeholder="$t('components.suggest_new.description_placeholder')" />
      </div>
      <div>
        <label class="block text-[10px] text-slate-600 mb-1">{{ $t('components.suggest_new.rationale') }}</label>
        <textarea v-model="form.rationale" rows="2"
          class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500 resize-none"
          :placeholder="$t('components.suggest_new.rationale_placeholder')" />
      </div>
      <div v-if="error" class="text-[10px] text-red-400">{{ error }}</div>
      <div class="flex gap-2">
        <button @click="submit" :disabled="!form.title.trim() || submitting"
          class="text-xs px-3 py-1.5 bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-white rounded-lg font-medium">
          {{ submitting ? $t('components.suggest_new.submitting') : $t('components.suggest_new.submit') }}
        </button>
        <button @click="showForm = false" class="text-xs text-slate-500 hover:text-slate-300">{{ $t('common.action.cancel') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { api } from '../api'
import { useSession } from '../composables/useSession'
import { useToast } from '../composables/useToast.js'
import { useI18n } from 'vue-i18n'
import { renderApiError } from '../composables/useApiError.js'
import { entityLabel } from '../composables/useEnumLabel.js'

const { t } = useI18n()


const { success: toastSuccess } = useToast()

// Suggestions are the contributor entry point; readers are read-only (#23), so
// hide the control entirely for them rather than letting it 403 on submit.
const { currentUserData } = useSession()
const canSuggest = computed(() => (currentUserData.value?.role || '') !== 'reader')

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

// The entity name is interpolated whole. It used to be lower-cased for the
// "new" variant, which is locale-blind — German capitalises every noun — so
// the casing now belongs to whatever the caller passes.
// Mid-sentence ("Suggest new risk") wants the inline entity name, which
// `common.entity_inline.*` authors per locale — the old code lower-cased
// `typeLabel`, which is locale-blind. Falls back to the prop for an entity
// type with no catalogue entry.
const inlineEntity = computed(
  () => entityLabel(props.entityType, { inline: true }) || props.typeLabel,
)

const formTitle = computed(() =>
  isDetail.value
    ? t('components.suggest_new.form_title_change', { entity: props.typeLabel })
    : t('components.suggest_new.form_title_new', { entity: inlineEntity.value }),
)

// Detail-only types (list always uses 'create', no picker needed)
const detailTypes = {
  risk:                [{ value: 'update', labelKey: 'edit_risk' }, { value: 'reassess', labelKey: 'reassess_risk' }],
  incident:            [{ value: 'update', labelKey: 'edit_incident' }, { value: 'link', labelKey: 'link_entities' }],
  supplier:            [{ value: 'update', labelKey: 'edit_supplier' }, { value: 'reassess', labelKey: 'reassess_supplier' }],
  legal_requirement:   [{ value: 'update', labelKey: 'edit_requirement' }],
  change_request:      [{ value: 'update', labelKey: 'edit_change_request' }],
  corrective_action:   [{ value: 'update', labelKey: 'edit_corrective_action' }],
  task:                [{ value: 'update', labelKey: 'edit_task' }],
  objective:           [{ value: 'update', labelKey: 'edit_objective' }, { value: 'review', labelKey: 'request_review' }],
  system:              [{ value: 'update', labelKey: 'edit_system' }, { value: 'review', labelKey: 'request_access_review' }],
  asset:               [{ value: 'update', labelKey: 'edit_asset' }, { value: 'review', labelKey: 'request_asset_review' }],
}

// The per-entity list holds keys; labels resolve here so the dropdown rebuilds
// when the locale changes.
const typeOptions = computed(() => {
  if (!isDetail.value) return []
  const defs = detailTypes[props.entityType]
  if (!defs) {
    return [{ value: 'update', label: t('components.suggest_new.type.edit_entity', { entity: props.typeLabel }) }]
  }
  return defs.map((o) => ({ value: o.value, label: t(`components.suggest_new.type.${o.labelKey}`) }))
})

const titlePlaceholder = computed(() => {
  if (!isDetail.value) return t('components.suggest_new.placeholder_new', { entity: inlineEntity.value })
  const kind = form.value.suggestion_type
  if (kind === 'reassess') return t('components.suggest_new.placeholder_reassess')
  if (kind === 'link') return t('components.suggest_new.placeholder_link')
  if (kind === 'review') return t('components.suggest_new.placeholder_review')
  return t('components.suggest_new.placeholder_update')
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
    // #167: a suggestion goes to managers for review rather than taking effect,
    // and it isn't shown on this list view — without this the submit felt like it
    // vanished. Confirm it landed and is pending review.
    toastSuccess(t('components.suggestions.created_toast'))
    emit('created')
  } catch (e) {
    error.value = renderApiError(e) || t('components.suggest_new.error_create')
  } finally {
    submitting.value = false
  }
}
</script>
