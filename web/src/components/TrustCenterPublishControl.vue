<template>
  <div v-if="isManager" class="no-print text-xs">
    <div v-if="loadError" class="text-red-400">
      {{ loadError }}
      <button type="button" @click="load" class="ml-2 text-blue-400 hover:text-blue-300 transition-colors">{{ $t('common.action.retry') }}</button>
    </div>
    <div v-else-if="state" class="flex flex-wrap items-center gap-x-3 gap-y-1">
      <template v-if="state.published">
        <span class="text-emerald-400" data-trust="published">{{ $t('components.trust_publish.status_published', { level: levelLabel(state.access_level), version: servedVersion }) }}</span>
        <button type="button" data-trust="unpublish" @click="confirmUnpublish = true"
          class="text-slate-400 hover:text-red-400 transition-colors">{{ $t('components.trust_publish.unpublish') }}</button>
        <span v-if="draftInProgress" class="basis-full text-amber-400" data-trust="draft">{{ $t('components.trust_publish.draft_in_progress', { draft: state.document_version, version: servedVersion }) }}</span>
      </template>
      <template v-else>
        <span v-if="state.hidden_reason" class="text-slate-400" data-trust="hidden">{{ $t('components.trust_publish.hidden_' + state.hidden_reason) }}</span>
        <button type="button" data-trust="publish" :disabled="!state.can_publish" @click="openPublish"
          class="px-2.5 py-1 font-medium rounded-md bg-blue-600 hover:bg-blue-500 text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
          {{ state.hidden_reason ? $t('components.trust_publish.publish_again') : $t('components.trust_publish.publish') }}
        </button>
        <span v-if="state.cannot_publish_reason" class="text-slate-500" data-trust="reason">{{ $t('components.trust_publish.reason_' + state.cannot_publish_reason) }}</span>
      </template>
      <span v-if="actionError" class="basis-full text-red-400" data-trust="error">{{ actionError }}</span>
    </div>

    <div v-if="showPublish" class="fixed inset-0 z-50 flex items-center justify-center" data-trust="publish-dialog">
      <div class="absolute inset-0 bg-black/60" @click="showPublish = false" />
      <div class="relative bg-slate-900 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-md p-6 space-y-4 text-sm">
        <h2 class="text-lg font-bold text-white">{{ $t('components.trust_publish.dialog_title') }}</h2>
        <fieldset class="space-y-2">
          <legend class="text-xs text-slate-500 mb-2">{{ $t('components.trust_publish.level_label') }}</legend>
          <label v-for="opt in levelOptions" :key="opt.value" class="flex items-start gap-2 text-slate-300 cursor-pointer">
            <input type="radio" name="trust-level" :value="opt.value" v-model="chosenLevel" class="mt-1" />
            <span>
              <span class="font-medium">{{ opt.label }}</span>
              <span v-if="opt.help" class="block text-xs text-slate-500">{{ opt.help }}</span>
            </span>
          </label>
        </fieldset>
        <div v-if="dialogError" class="text-red-400 text-xs" data-trust="dialog-error">{{ dialogError }}</div>
        <div class="flex justify-end gap-2">
          <button type="button" @click="showPublish = false" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ $t('common.action.cancel') }}</button>
          <button type="button" data-trust="publish-confirm" :disabled="busy" @click="publish"
            class="px-3 py-1.5 text-xs font-medium rounded-lg bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50">{{ $t('components.trust_publish.publish_confirm') }}</button>
        </div>
      </div>
    </div>

    <div v-if="confirmUnpublish" class="fixed inset-0 z-50 flex items-center justify-center" data-trust="unpublish-dialog">
      <div class="absolute inset-0 bg-black/60" @click="confirmUnpublish = false" />
      <div class="relative bg-slate-900 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-md p-6 space-y-4 text-sm">
        <h2 class="text-lg font-bold text-white">{{ $t('components.trust_publish.unpublish_title') }}</h2>
        <p class="text-slate-400">{{ $t('components.trust_publish.unpublish_body') }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" @click="confirmUnpublish = false" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ $t('common.action.cancel') }}</button>
          <button type="button" data-trust="unpublish-confirm" :disabled="busy" @click="unpublish"
            class="px-3 py-1.5 text-xs font-medium rounded-lg bg-red-600 hover:bg-red-500 text-white disabled:opacity-50">{{ $t('components.trust_publish.unpublish') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { renderApiError } from '../composables/useApiError.js'

const props = defineProps({
  docId: { type: String, required: true },
  role: { type: String, default: '' },
})

const { t } = useI18n()

// Only admins and managers may use the trust center endpoints; everyone else
// gets no control at all, and no request.
const isManager = computed(() => props.role === 'admin' || props.role === 'manager')

const state = ref(null)
const loadError = ref('')
const actionError = ref('')
const dialogError = ref('')
const busy = ref(false)
const showPublish = ref(false)
const confirmUnpublish = ref(false)
const chosenLevel = ref('')

const servedVersion = computed(() => state.value?.served_version?.version || '')
const draftInProgress = computed(() => {
  const s = state.value
  if (!s || !s.served_version) return false
  return s.document_status !== 'approved' || s.document_version !== s.served_version.version
})

const levelLabel = (v) => (['public', 'gated', 'restricted'].includes(v) ? t('components.trust_publish.level_' + v) : v)

const levelOptions = computed(() => [
  { value: '', label: t('components.trust_publish.level_default'), help: '' },
  { value: 'public', label: t('components.trust_publish.level_public'), help: t('components.trust_publish.help_public') },
  { value: 'gated', label: t('components.trust_publish.level_gated'), help: t('components.trust_publish.help_gated') },
  { value: 'restricted', label: t('components.trust_publish.level_restricted'), help: t('components.trust_publish.help_restricted') },
])

async function load() {
  if (!isManager.value || !props.docId) return
  loadError.value = ''
  try {
    state.value = await api.getDocumentTrust(props.docId)
  } catch (e) {
    state.value = null
    loadError.value = t('components.trust_publish.error_load', { message: renderApiError(e) || t('common.state.unknown') })
  }
}

function openPublish() {
  chosenLevel.value = ''
  dialogError.value = ''
  actionError.value = ''
  showPublish.value = true
}

async function publish() {
  busy.value = true
  dialogError.value = ''
  try {
    // No access_level when the organization default is kept: the server picks it.
    state.value = await api.publishDocumentTrust(props.docId, chosenLevel.value)
    showPublish.value = false
  } catch (e) {
    dialogError.value = renderApiError(e) || t('common.state.unknown')
  } finally {
    busy.value = false
  }
}

async function unpublish() {
  busy.value = true
  actionError.value = ''
  try {
    await api.unpublishDocumentTrust(props.docId)
    confirmUnpublish.value = false
    await load()
  } catch (e) {
    confirmUnpublish.value = false
    actionError.value = renderApiError(e) || t('common.state.unknown')
  } finally {
    busy.value = false
  }
}

onMounted(load)
watch(() => [props.docId, props.role], () => {
  state.value = null
  showPublish.value = false
  confirmUnpublish.value = false
  load()
})
</script>
