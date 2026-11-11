<template>
  <section data-trust="settings-section" class="space-y-6 max-w-2xl">
    <!-- On / off. Takes effect at once, so turning it on asks first. -->
    <div class="flex items-start justify-between gap-4 border border-slate-800 rounded-xl p-4 bg-slate-900/40">
      <div>
        <div class="text-sm font-medium text-slate-200">{{ $t('trust_manager.settings.enabled_label') }}</div>
        <p class="text-xs text-slate-500 mt-1">{{ $t('trust_manager.settings.enabled_help') }}</p>
      </div>
      <button type="button" role="switch" :aria-checked="String(settings.enabled)" data-trust="toggle-enabled" :disabled="busy"
        :aria-label="$t('trust_manager.settings.enabled_label')" @click="toggle"
        :class="settings.enabled ? 'bg-emerald-600' : 'bg-slate-700'"
        class="relative flex-shrink-0 w-11 h-6 rounded-full transition-colors disabled:opacity-50">
        <span :class="settings.enabled ? 'translate-x-5' : 'translate-x-0.5'" class="absolute top-0.5 left-0 w-5 h-5 bg-white rounded-full transition-transform" />
      </button>
    </div>

    <form class="space-y-5" @submit.prevent="save">
      <div>
        <div class="text-sm font-medium text-slate-200 mb-1">{{ $t('trust_manager.settings.overview_label') }}</div>
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-sm" :class="form.overview_document_id ? 'text-slate-200' : 'text-slate-500'" data-trust="overview-value">{{ overviewText }}</span>
          <button type="button" data-trust="overview-choose" @click="showPicker = true" class="text-xs text-blue-400 hover:text-blue-300">{{ $t('trust_manager.settings.overview_choose') }}</button>
          <button v-if="form.overview_document_id" type="button" data-trust="overview-clear" @click="clearOverview" class="text-xs text-slate-400 hover:text-red-400">{{ $t('trust_manager.settings.overview_clear') }}</button>
        </div>
        <p v-if="overviewNoApproved" class="text-xs text-amber-400 mt-1" data-trust="overview-hint">{{ $t('trust_manager.settings.overview_no_approved') }}</p>
      </div>

      <div>
        <label for="trust-contact" class="block text-sm font-medium text-slate-200 mb-1">{{ $t('trust_manager.settings.contact_label') }}</label>
        <input id="trust-contact" v-model="form.security_contact" type="text" maxlength="320" data-trust="contact"
          class="w-full px-3 py-2 text-sm bg-slate-800 border border-slate-700 rounded-lg text-slate-200 focus:outline-none focus:border-blue-500" />
        <p class="text-xs text-slate-500 mt-1">{{ $t('trust_manager.settings.contact_help') }}</p>
      </div>

      <div>
        <label for="trust-default-level" class="block text-sm font-medium text-slate-200 mb-1">{{ $t('trust_manager.settings.default_level_label') }}</label>
        <select id="trust-default-level" v-model="form.default_access_level" data-trust="default-level"
          class="px-3 py-2 text-sm bg-slate-800 border border-slate-700 rounded-lg text-slate-200">
          <option v-for="lvl in levels" :key="lvl" :value="lvl">{{ levelLabel(lvl) }}</option>
        </select>
        <p class="text-xs text-slate-500 mt-1">{{ $t('trust_manager.settings.default_level_help') }}</p>
      </div>

      <div class="flex items-center gap-3">
        <button type="submit" data-trust="save" :disabled="busy || !dirty"
          class="px-4 py-2 text-sm font-medium rounded-lg bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50 disabled:cursor-not-allowed">{{ $t('common.action.save') }}</button>
        <span v-if="showSaved" class="text-xs text-emerald-400" data-trust="saved">{{ $t('trust_manager.settings.saved') }}</span>
        <span v-if="error" class="text-xs text-red-400" data-trust="settings-error">{{ error }}</span>
      </div>
    </form>

    <!-- Turning on -->
    <div v-if="confirmOn" class="fixed inset-0 z-50 flex items-center justify-center px-4" data-trust="enable-dialog">
      <div class="absolute inset-0 bg-black/60" @click="confirmOn = false" />
      <div class="relative bg-slate-900 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-md p-6 space-y-4 text-sm" role="alertdialog">
        <h2 class="text-lg font-bold text-white">{{ $t('trust_manager.settings.enable_title') }}</h2>
        <p class="text-slate-300">{{ $t('trust_manager.settings.enable_body') }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" data-trust="enable-cancel" @click="confirmOn = false" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ $t('common.action.cancel') }}</button>
          <button type="button" data-trust="enable-confirm" :disabled="busy" @click="turnOn"
            class="px-3 py-1.5 text-xs font-medium rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white disabled:opacity-50">{{ $t('trust_manager.settings.enable_confirm') }}</button>
        </div>
      </div>
    </div>

    <!-- Overview document picker -->
    <div v-if="showPicker" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4" data-trust="overview-picker">
      <div class="absolute inset-0 bg-black/60" @click="showPicker = false" />
      <div class="relative bg-slate-900 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-lg p-6 space-y-4 text-sm">
        <h2 class="text-lg font-bold text-white">{{ $t('trust_manager.settings.overview_label') }}</h2>
        <TrustDocumentSearch hide-retired @pick="pickOverview" />
        <div class="flex justify-end">
          <button type="button" @click="showPicker = false" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ $t('common.action.cancel') }}</button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../../api'
import { renderApiError } from '../../composables/useApiError.js'
import { useOrgDocuments } from '../../composables/useOrgDocuments.js'
import { changedSettings } from '../../utils/trustManager.js'
import TrustDocumentSearch from './TrustDocumentSearch.vue'

// `settings` is what the server last sent; the form edits a copy. Save sends
// only the fields that differ, so a stale form cannot put back a value the
// server has since changed. The on/off switch is not part of the form: it
// saves at once.
const props = defineProps({ settings: { type: Object, required: true } })
const emit = defineEmits(['saved'])
const { t } = useI18n()

const levels = ['public', 'gated', 'restricted']
const levelLabel = (lvl) => t('trust_manager.level.' + lvl)
const form = reactive({})
const busy = ref(false)
const error = ref('')
const saved = ref(false)
const savedSnapshot = ref('')
const confirmOn = ref(false)
const showPicker = ref(false)
const pickedTitle = ref('')
const overviewNoApproved = ref(false)
const { documents, load: loadDocs } = useOrgDocuments()

function reset() {
  Object.assign(form, {
    security_contact: props.settings.security_contact,
    default_access_level: props.settings.default_access_level,
    overview_document_id: props.settings.overview_document_id || null,
  })
}
reset()
watch(() => props.settings, () => { reset(); pickedTitle.value = ''; checkOverview() })

const patch = computed(() => changedSettings(props.settings, { ...form, enabled: props.settings.enabled }))
const showSaved = computed(() => saved.value && savedSnapshot.value === JSON.stringify(form))
const dirty = computed(() => Object.keys(patch.value).length > 0)

const overviewText = computed(() => {
  const id = form.overview_document_id
  if (!id) return t('trust_manager.settings.overview_none')
  const title = pickedTitle.value || documents.value.find((d) => d.document_id.toLowerCase() === id.toLowerCase())?.title
  return title ? `${title} (${id})` : id
})

// A document with no approved version is accepted, but the page leaves it out.
async function checkOverview() {
  overviewNoApproved.value = false
  const id = form.overview_document_id
  if (!id) return
  try {
    const st = await api.getDocumentTrust(id)
    if (form.overview_document_id === id) overviewNoApproved.value = !st.served_version
  } catch { /* the hint is optional */ }
}

function pickOverview(doc) {
  form.overview_document_id = doc.document_id
  pickedTitle.value = doc.title
  showPicker.value = false
  checkOverview()
}
function clearOverview() {
  form.overview_document_id = null
  pickedTitle.value = ''
  overviewNoApproved.value = false
}

async function put(body) {
  busy.value = true
  error.value = ''
  saved.value = false
  try {
    const next = await api.updateTrustSettings(body)
    emit('saved', next)
    // Show what the server stored, not what was typed.
    Object.assign(form, { security_contact: next.security_contact, default_access_level: next.default_access_level, overview_document_id: next.overview_document_id || null })
    savedSnapshot.value = JSON.stringify(form)
    saved.value = true
    return true
  } catch (e) {
    error.value = t('trust_manager.settings.save_error', { message: renderApiError(e) || t('common.state.unknown') })
    return false
  } finally {
    busy.value = false
  }
}

function toggle() {
  if (props.settings.enabled) put({ enabled: false })
  else confirmOn.value = true
}
async function turnOn() {
  await put({ enabled: true })
  confirmOn.value = false
}
async function save() {
  if (!dirty.value) return
  await put(patch.value)
}

onMounted(() => { loadDocs(); checkOverview() })
</script>
