<template>
  <section data-trust="settings-section" class="space-y-6 max-w-2xl">
    <!-- On / off. Takes effect at once, so turning it on asks first. -->
    <div class="flex items-start justify-between gap-4 border border-slate-800 rounded-xl p-4 bg-slate-900/40">
      <div>
        <div class="text-sm font-medium text-slate-200">{{ $t('trust_manager.settings.enabled_label') }}</div>
        <p class="text-xs text-slate-500 mt-1">{{ $t('trust_manager.settings.enabled_help') }}</p>
      </div>
      <div class="flex flex-col items-end gap-1">
      <button type="button" role="switch" :aria-checked="String(settings.enabled)" data-trust="toggle-enabled" :disabled="toggleBusy"
        :aria-label="$t('trust_manager.settings.enabled_label')" @click="toggle"
        :class="settings.enabled ? 'bg-emerald-600' : 'bg-slate-700'"
        class="relative flex-shrink-0 w-11 h-6 rounded-full transition-colors disabled:opacity-50">
        <span :class="settings.enabled ? 'translate-x-5' : 'translate-x-0.5'" class="absolute top-0.5 left-0 w-5 h-5 bg-white rounded-full transition-transform" />
      </button>
      <span v-if="toggleError" class="text-xs text-red-400" data-trust="toggle-error">{{ toggleError }}</span>
      </div>
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

      <!-- The click-through NDA (#525): gated items need sign-in and an acceptance
           of this document's approved version, unless the visitor's domain is on
           the skip list. -->
      <div data-trust="nda-block">
        <div class="text-sm font-medium text-slate-200 mb-1">{{ $t('trust_manager.settings.nda_label') }}</div>
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-sm" :class="form.nda_document_id ? 'text-slate-200' : 'text-slate-500'" data-trust="nda-value">{{ ndaText }}</span>
          <button type="button" data-trust="nda-choose" @click="showNdaPicker = true" class="text-xs text-blue-400 hover:text-blue-300">{{ $t('trust_manager.settings.nda_choose') }}</button>
          <button v-if="form.nda_document_id" type="button" data-trust="nda-clear" @click="clearNda" class="text-xs text-slate-400 hover:text-red-400">{{ $t('trust_manager.settings.nda_clear') }}</button>
          <button v-else type="button" data-trust="nda-setup-open" @click="openSetup" class="text-xs text-blue-400 hover:text-blue-300">{{ $t('trust_manager.settings.nda_setup') }}</button>
        </div>
        <p v-if="ndaLine" class="text-xs mt-1" :class="ndaLine.warn ? 'text-amber-400' : 'text-slate-500'" :data-trust="ndaLine.warn ? 'nda-warning' : 'nda-status'">{{ ndaLine.text }}</p>
        <p v-if="setupDone" class="text-xs text-slate-300 mt-1" data-trust="nda-setup-done">
          {{ $t('trust_manager.settings.nda_setup_done') }}
          <router-link :to="orgPath('/documents/' + setupDone)" class="text-blue-400 hover:text-blue-300" data-trust="nda-open-doc">{{ $t('trust_manager.settings.nda_open_doc') }}</router-link>
        </p>

        <label for="trust-nda-skip" class="block text-sm font-medium text-slate-200 mt-4 mb-1">{{ $t('trust_manager.settings.nda_skip_label') }}</label>
        <textarea id="trust-nda-skip" v-model="form.ndaSkipText" rows="4" data-trust="nda-skip" spellcheck="false"
          class="w-full px-3 py-2 text-sm font-mono bg-slate-800 border border-slate-700 rounded-lg text-slate-200 focus:outline-none focus:border-blue-500" />
        <p class="text-xs text-slate-500 mt-1">{{ $t('trust_manager.settings.nda_skip_help') }}</p>
        <p v-if="skipError" class="text-xs text-red-400 mt-1" data-trust="nda-skip-error">{{ $t('trust_manager.settings.nda_skip_error') }}</p>
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

      <div>
        <label for="trust-privacy-url" class="block text-sm font-medium text-slate-200 mb-1">{{ $t('trust_manager.settings.privacy_label') }}</label>
        <input id="trust-privacy-url" v-model="form.privacy_notice_url" type="text" maxlength="2000" data-trust="privacy-url" inputmode="url"
          class="w-full px-3 py-2 text-sm bg-slate-800 border border-slate-700 rounded-lg text-slate-200 focus:outline-none focus:border-blue-500" />
        <p class="text-xs text-slate-500 mt-1">{{ $t('trust_manager.settings.privacy_help') }}</p>
        <p v-if="privacyError" class="text-xs text-red-400 mt-1" data-trust="privacy-error">{{ $t('trust_manager.settings.privacy_error') }}</p>
      </div>

      <div>
        <label for="trust-domains" class="block text-sm font-medium text-slate-200 mb-1">{{ $t('trust_manager.settings.domains_label') }}</label>
        <textarea id="trust-domains" v-model="form.domainsText" rows="6" data-trust="domains" spellcheck="false" @input="form.domainsMode = 'custom'"
          class="w-full px-3 py-2 text-sm font-mono bg-slate-800 border border-slate-700 rounded-lg text-slate-200 focus:outline-none focus:border-blue-500" />
        <p class="text-xs text-slate-500 mt-1">{{ $t('trust_manager.settings.domains_help') }}</p>
        <p v-if="form.domainsMode === 'default'" class="text-xs text-slate-400 mt-1" data-trust="domains-default-note">{{ $t('trust_manager.settings.domains_default_note') }}</p>
        <div class="flex flex-wrap items-center gap-3 mt-2 text-xs">
          <button type="button" data-trust="domains-restore" @click="restoreDomains" class="text-blue-400 hover:text-blue-300">{{ $t('trust_manager.settings.domains_restore') }}</button>
          <button type="button" data-trust="domains-allow-all" @click="allowAllDomains" class="text-blue-400 hover:text-blue-300">{{ $t('trust_manager.settings.domains_allow_all') }}</button>
        </div>
        <p v-if="domainsError" class="text-xs text-red-400 mt-1" data-trust="domains-error">{{ $t('trust_manager.settings.domains_error') }}</p>
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
          <button type="button" data-trust="enable-confirm" :disabled="toggleBusy" @click="turnOn"
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
    <!-- NDA document picker -->
    <div v-if="showNdaPicker" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4" data-trust="nda-picker">
      <div class="absolute inset-0 bg-black/60" @click="showNdaPicker = false" />
      <div class="relative bg-slate-900 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-lg p-6 space-y-4 text-sm">
        <h2 class="text-lg font-bold text-white">{{ $t('trust_manager.settings.nda_label') }}</h2>
        <TrustDocumentSearch hide-retired @pick="pickNda" />
        <div class="flex justify-end">
          <button type="button" @click="showNdaPicker = false" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ $t('common.action.cancel') }}</button>
        </div>
      </div>
    </div>

    <!-- Set up an NDA: a new document the manager then writes and sends for review. -->
    <div v-if="showSetup" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4" data-trust="nda-setup">
      <div class="absolute inset-0 bg-black/60" @click="showSetup = false" />
      <form class="relative bg-slate-900 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-md p-6 space-y-4 text-sm" @submit.prevent="createNda">
        <h2 class="text-lg font-bold text-white">{{ $t('trust_manager.settings.nda_setup') }}</h2>
        <p class="text-xs text-slate-400">{{ $t('trust_manager.settings.nda_setup_help') }}</p>
        <div>
          <label for="trust-nda-folder" class="block text-sm font-medium text-slate-200 mb-1">{{ $t('trust_manager.settings.nda_setup_folder') }}</label>
          <select id="trust-nda-folder" v-model="setup.folder" data-trust="nda-setup-folder" class="w-full px-3 py-2 text-sm bg-slate-800 border border-slate-700 rounded-lg text-slate-200">
            <option v-for="f in folderPaths" :key="f" :value="f">{{ f }}</option>
          </select>
          <p v-if="!folderPaths.length" class="text-xs text-amber-400 mt-1" data-trust="nda-setup-no-folder">{{ $t('trust_manager.settings.nda_setup_no_folder') }}</p>
        </div>
        <div>
          <label for="trust-nda-title" class="block text-sm font-medium text-slate-200 mb-1">{{ $t('trust_manager.settings.nda_setup_title') }}</label>
          <input id="trust-nda-title" v-model="setup.title" type="text" maxlength="200" data-trust="nda-setup-title"
            class="w-full px-3 py-2 text-sm bg-slate-800 border border-slate-700 rounded-lg text-slate-200" />
        </div>
        <div>
          <label for="trust-nda-id" class="block text-sm font-medium text-slate-200 mb-1">{{ $t('trust_manager.settings.nda_setup_id') }}</label>
          <input id="trust-nda-id" v-model="setup.id" type="text" maxlength="100" data-trust="nda-setup-id" spellcheck="false"
            class="w-full px-3 py-2 text-sm font-mono bg-slate-800 border border-slate-700 rounded-lg text-slate-200" />
        </div>
        <p v-if="setupError" class="text-xs text-red-400" role="alert" data-trust="nda-setup-error">{{ setupError }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" data-trust="nda-setup-cancel" @click="showSetup = false" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ $t('common.action.cancel') }}</button>
          <button type="submit" data-trust="nda-setup-create" :disabled="setupBusy || !setup.folder || !setup.title.trim() || !setup.id.trim()"
            class="px-3 py-1.5 text-xs font-medium rounded-lg bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50">{{ $t('trust_manager.settings.nda_setup_create') }}</button>
        </div>
      </form>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../../api'
import { renderApiError } from '../../composables/useApiError.js'
import { useCurrentOrg } from '../../composables/useCurrentOrg.js'
import { formatDate } from '../../composables/useFormat.js'
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
const toggleBusy = ref(false)
const toggleError = ref('')
const error = ref('')
const saved = ref(false)
const savedSnapshot = ref('')
const confirmOn = ref(false)
const showPicker = ref(false)
const pickedTitle = ref('')
const overviewNoApproved = ref(false)
const { documents, folderPaths, load: loadDocs } = useOrgDocuments()
const { orgPath } = useCurrentOrg()
const showNdaPicker = ref(false)
const pickedNdaTitle = ref('')
const skipError = ref(false)
const showSetup = ref(false)
const setupBusy = ref(false)
const setupError = ref('')
const setupDone = ref('')
const setup = reactive({ folder: '', title: '', id: '' })

// What the form shows for a settings object from the server.
function fromSettings(st) {
  return {
    security_contact: st.security_contact,
    default_access_level: st.default_access_level,
    overview_document_id: st.overview_document_id || null,
    privacy_notice_url: st.privacy_notice_url || '',
    nda_document_id: st.nda_document_id || null,
    ndaSkipText: (st.nda_skip_domains || []).join('\n'),
    // 'default' = the built-in list (null on the wire), 'custom' = the list below.
    domainsMode: st.blocked_email_domains_default === false ? 'custom' : 'default',
    domainsText: (st.blocked_email_domains || []).join('\n'),
  }
}
const domainsError = ref(false)
const privacyError = ref(false)

function restoreDomains() {
  form.domainsMode = 'default'
  form.domainsText = (props.settings.default_blocked_email_domains || []).join('\n')
}
function allowAllDomains() {
  form.domainsMode = 'custom'
  form.domainsText = ''
}
Object.assign(form, fromSettings(props.settings))
// When the server's settings change (the switch was flipped, or Save returned),
// only the fields the user has not edited follow; an unsaved edit stays.
watch(() => props.settings, (next, prev) => {
  let overviewChanged = false
  const was = fromSettings(prev || {})
  const now = fromSettings(next)
  for (const k of Object.keys(now)) {
    if ((form[k] ?? null) === (was[k] ?? null) && (now[k] ?? null) !== (was[k] ?? null)) {
      form[k] = now[k]
      if (k === 'overview_document_id') overviewChanged = true
    }
  }
  if (overviewChanged) { pickedTitle.value = ''; checkOverview() }
  if ((next.nda_document_id || null) !== (prev?.nda_document_id || null)) pickedNdaTitle.value = ''
})

const ndaText = computed(() => {
  const id = form.nda_document_id
  if (!id) return t('trust_manager.settings.nda_none')
  const title = pickedNdaTitle.value || documents.value.find((d) => d.document_id.toLowerCase() === id.toLowerCase())?.title
  return title ? `${title} (${id})` : id
})

// The status line describes what the server has saved; with an unsaved change
// of the NDA document there is nothing to say yet.
const ndaLine = computed(() => {
  const st = props.settings
  if ((form.nda_document_id || null) !== (st.nda_document_id || null)) return null
  if (!st.nda_document_id || st.nda_status === 'none' || !st.nda_status) return { warn: false, text: t('trust_manager.settings.nda_status_none') }
  if (st.nda_status === 'approved') {
    return { warn: false, text: t('trust_manager.settings.nda_status_approved', { version: st.nda_version || '', date: formatDate(st.nda_approved_at, 'short') }) }
  }
  const reason = ['not_approved', 'retired', 'deleted'].includes(st.nda_unavailable_reason) ? st.nda_unavailable_reason : 'error'
  return { warn: true, text: t('trust_manager.settings.nda_unavailable_' + reason) }
})

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

function pickNda(doc) {
  form.nda_document_id = doc.document_id
  pickedNdaTitle.value = doc.title
  showNdaPicker.value = false
}
function clearNda() {
  form.nda_document_id = null
  pickedNdaTitle.value = ''
  setupDone.value = ''
}

function openSetup() {
  setup.folder = folderPaths.value[0] || ''
  setup.title = t('trust_manager.settings.nda_setup_default_title')
  setup.id = 'trust-center-nda'
  setupError.value = ''
  showSetup.value = true
}
// Creates the document (a plain POST /documents with a placeholder body), then
// sets it as the NDA. It stays a draft: the manager writes the text and sends it
// for review, and gated items stay locked until a version is approved.
async function createNda() {
  if (setupBusy.value) return
  setupBusy.value = true
  setupError.value = ''
  const id = setup.id.trim().toLowerCase()
  try {
    await api.createDocument({
      folder: setup.folder, filename: id + '.md', document_id: id, title: setup.title.trim(),
      content: `# ${setup.title.trim()}\n\n[Your NDA text]\n`,
    })
  } catch (e) {
    setupError.value = renderApiError(e) || t('common.state.unknown')
    setupBusy.value = false
    return
  }
  try {
    const next = await api.updateTrustSettings({ nda_document_id: id })
    emit('saved', next)
    form.nda_document_id = next.nda_document_id || id
    pickedNdaTitle.value = setup.title.trim()
    setupDone.value = id
    showSetup.value = false
    loadDocs()
  } catch (e) {
    setupError.value = t('trust_manager.settings.save_error', { message: renderApiError(e) || t('common.state.unknown') })
  } finally {
    setupBusy.value = false
  }
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

// Save: sends only the changed fields and shows what the server stored.
async function put(body) {
  busy.value = true
  error.value = ''
  domainsError.value = false
  privacyError.value = false
  skipError.value = false
  saved.value = false
  try {
    const next = await api.updateTrustSettings(body)
    emit('saved', next)
    Object.assign(form, fromSettings(next))
    savedSnapshot.value = JSON.stringify(form)
    saved.value = true
    return true
  } catch (e) {
    // A 400 is the server refusing a value. When exactly one of the two
    // validated fields was in the request the error belongs under it; with both
    // (or neither) it stays the general line.
    const touched = ['blocked_email_domains', 'privacy_notice_url', 'nda_skip_domains'].filter((k) => k in body)
    if (e && e.status === 400 && touched.length === 1) {
      if (touched[0] === 'blocked_email_domains') domainsError.value = true
      else if (touched[0] === 'nda_skip_domains') skipError.value = true
      else privacyError.value = true
    } else {
      error.value = t('trust_manager.settings.save_error', { message: renderApiError(e) || t('common.state.unknown') })
    }
    return false
  } finally {
    busy.value = false
  }
}

// The switch saves only `enabled`. It never touches the form fields (an unsaved
// edit stays) and has its own status, apart from the Save form's "Saved".
async function putEnabled(enabled) {
  toggleBusy.value = true
  toggleError.value = ''
  try {
    emit('saved', await api.updateTrustSettings({ enabled }))
    return true
  } catch (e) {
    toggleError.value = t('trust_manager.settings.save_error', { message: renderApiError(e) || t('common.state.unknown') })
    return false
  } finally {
    toggleBusy.value = false
  }
}

function toggle() {
  if (props.settings.enabled) putEnabled(false)
  else confirmOn.value = true
}
async function turnOn() {
  await putEnabled(true)
  confirmOn.value = false
}
async function save() {
  if (!dirty.value) return
  await put(patch.value)
}

onMounted(() => { loadDocs(); checkOverview() })
</script>
