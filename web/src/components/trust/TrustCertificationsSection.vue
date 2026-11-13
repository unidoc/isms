<template>
  <section data-trust="certifications-section" class="space-y-4">
    <div class="flex items-center justify-between gap-3 flex-wrap">
      <h2 class="text-lg font-semibold text-slate-100">{{ $t('trust_manager.certifications.heading') }}</h2>
      <button type="button" data-trust="cert-add-open" @click="openAdd"
        class="px-3 py-1.5 text-sm font-medium rounded-lg bg-blue-600 hover:bg-blue-500 text-white transition-colors">{{ $t('trust_manager.certifications.add') }}</button>
    </div>

    <div v-if="actionError" class="text-sm text-red-400" data-trust="cert-action-error">{{ actionError }}</div>
    <div v-if="loadError" class="text-sm text-red-400" data-trust="cert-load-error">
      {{ $t('trust_manager.load_error', { message: loadError }) }}
      <button type="button" @click="load" class="ml-2 text-blue-400 hover:text-blue-300">{{ $t('common.action.retry') }}</button>
    </div>
    <div v-else-if="loading && !certs.length" class="text-sm text-slate-500">{{ $t('trust_manager.picker.loading') }}</div>
    <div v-else-if="!certs.length" class="text-sm text-slate-500 border border-dashed border-slate-700 rounded-xl p-6" data-trust="cert-empty">{{ $t('trust_manager.certifications.empty') }}</div>

    <!-- Add form -->
    <form v-if="add" data-trust="cert-add-form" @submit.prevent="submitAdd" class="border border-slate-700 rounded-xl bg-slate-900/60 p-4 space-y-3 text-sm">
      <fieldset :disabled="uploading" class="space-y-3">
        <div class="flex gap-4" role="radiogroup">
          <label class="flex items-center gap-2 text-slate-300 cursor-pointer"><input type="radio" value="file" v-model="add.source" data-trust="cert-source-file" /> {{ $t('trust_manager.certifications.source_file_option') }}</label>
          <label class="flex items-center gap-2 text-slate-300 cursor-pointer"><input type="radio" value="link" v-model="add.source" data-trust="cert-source-link" /> {{ $t('trust_manager.certifications.source_link_option') }}</label>
        </div>
        <div v-if="add.source === 'file'">
          <label class="block text-xs text-slate-500 mb-1" for="cert-add-file">{{ $t('trust_manager.certifications.f_file') }}</label>
          <input id="cert-add-file" ref="addFile" type="file" :accept="FILE_ACCEPT" data-trust="cert-f-file" @change="onAddFile" class="text-xs text-slate-300" />
          <p v-if="addErrors.file" class="text-xs text-red-400 mt-1" data-trust="cert-error-file">{{ errText(addErrors.file) }}</p>
        </div>
        <div v-else>
          <label class="block text-xs text-slate-500 mb-1" for="cert-add-url">{{ $t('trust_manager.certifications.f_url') }}</label>
          <input id="cert-add-url" v-model="add.url" type="text" data-trust="cert-f-url" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100" />
          <p v-if="addErrors.url" class="text-xs text-red-400 mt-1" data-trust="cert-error-url">{{ errText(addErrors.url) }}</p>
        </div>
        <div class="grid sm:grid-cols-2 gap-3">
          <div>
            <label class="block text-xs text-slate-500 mb-1" for="cert-add-label">{{ $t('trust_manager.certifications.f_label') }}</label>
            <input id="cert-add-label" v-model="add.label" type="text" data-trust="cert-f-label" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100" />
            <p v-if="addErrors.label" class="text-xs text-red-400 mt-1" data-trust="cert-error-label">{{ errText(addErrors.label) }}</p>
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1" for="cert-add-type">{{ $t('trust_manager.certifications.f_type') }}</label>
            <select id="cert-add-type" v-model="add.link_type" data-trust="cert-f-type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100">
              <option value="" disabled></option>
              <option v-for="ty in CERT_TYPES" :key="ty" :value="ty">{{ typeLabel(ty) }}</option>
            </select>
            <p v-if="addErrors.link_type" class="text-xs text-red-400 mt-1" data-trust="cert-error-type">{{ errText(addErrors.link_type) }}</p>
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1" for="cert-add-issuer">{{ $t('trust_manager.certifications.f_issuer') }}</label>
            <input id="cert-add-issuer" v-model="add.issuer" type="text" data-trust="cert-f-issuer" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1" for="cert-add-verified">{{ $t('trust_manager.certifications.f_verified') }}</label>
            <input id="cert-add-verified" v-model="add.verified_at" type="date" data-trust="cert-f-verified" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1" for="cert-add-validfrom">{{ $t('trust_manager.certifications.f_valid_from') }}</label>
            <input id="cert-add-validfrom" v-model="add.valid_from" type="date" data-trust="cert-f-validfrom" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1" for="cert-add-until">{{ $t('trust_manager.certifications.f_valid_until') }}</label>
            <input id="cert-add-until" v-model="add.valid_until" type="date" data-trust="cert-f-validuntil" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100" />
            <p v-if="addErrors.valid_until" class="text-xs text-red-400 mt-1" data-trust="cert-error-dates">{{ errText(addErrors.valid_until) }}</p>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-4">
          <label class="flex items-center gap-2 text-slate-300 cursor-pointer"><input type="checkbox" v-model="add.publish" data-trust="cert-f-publish" /> {{ $t('trust_manager.certifications.publish_now') }}</label>
          <select v-if="add.publish" v-model="add.access_level" data-trust="cert-f-level" :aria-label="$t('trust_manager.picker.level_label')" class="text-sm bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-slate-200">
            <option v-for="lvl in CERT_LEVELS" :key="lvl" :value="lvl">{{ levelLabel(lvl) }}</option>
          </select>
        </div>
      </fieldset>
      <div v-if="addError" class="text-xs text-red-400" data-trust="cert-add-error">{{ addError }}</div>
      <div class="flex justify-end items-center gap-3">
        <span v-if="uploading" class="text-xs text-slate-400" data-trust="cert-uploading">{{ $t('trust_manager.certifications.uploading') }}</span>
        <button type="button" :disabled="uploading" @click="add = null" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ $t('common.action.cancel') }}</button>
        <button type="submit" data-trust="cert-add-submit" :disabled="uploading" class="px-3 py-1.5 text-xs font-medium rounded-lg bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50">{{ $t('trust_manager.certifications.save_add') }}</button>
      </div>
    </form>

    <ul v-if="certs.length" class="divide-y divide-slate-800 border border-slate-800 rounded-xl bg-slate-900/40" data-trust="cert-list">
      <template v-for="(entry, idx) in entries" :key="entry.c.id">
        <li v-if="entry.firstUnpublished && idx > 0" class="px-4 py-2 text-xs font-semibold uppercase tracking-wide text-slate-500 bg-slate-900/60" data-trust="cert-divider">{{ $t('trust_manager.certifications.not_published') }}</li>
        <li :data-trust-cert-row="entry.c.id" class="p-4 space-y-2">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium text-slate-100 break-words" data-trust="cert-title">{{ entry.c.label }}</span>
            <span class="text-[10px] font-semibold px-1.5 py-0.5 rounded bg-slate-800 text-slate-400" data-trust="cert-type">{{ typeLabel(entry.c.link_type) }}</span>
            <span v-if="entry.c.state" class="text-[10px] font-semibold px-1.5 py-0.5 rounded" :class="stateClass(entry.c.state)" data-trust="cert-state">{{ stateLabel(entry.c.state) }}</span>
            <span class="text-xs px-2 py-0.5 rounded-full" :class="entry.published ? 'bg-emerald-500/15 text-emerald-300' : 'bg-slate-500/20 text-slate-300'" data-trust="cert-status">{{ entry.published ? $t('trust_manager.certifications.status_published', { level: levelLabel(entry.c.item.access_level) }) : $t('trust_manager.certifications.status_unpublished') }}</span>
          </div>
          <div class="text-xs text-slate-500 flex flex-wrap gap-x-3 gap-y-0.5">
            <span v-if="entry.c.issuer" data-trust="cert-issuer">{{ entry.c.issuer }}</span>
            <span data-trust="cert-validity">{{ validity(entry.c) }}</span>
            <span v-if="entry.c.source === 'file'" data-trust="cert-source">{{ $t('trust_manager.certifications.source_file', { name: entry.c.file_name, size: formatBytes(entry.c.file_size) }) }}</span>
            <a v-else-if="safeHref(entry.c.url)" :href="safeHref(entry.c.url)" target="_blank" rel="noopener noreferrer" class="text-blue-400 hover:text-blue-300" data-trust="cert-link">{{ hostOf(entry.c.url) }}</a>
            <span v-else data-trust="cert-source">{{ hostOf(entry.c.url) }}</span>
            <span v-if="entry.c.verified_at" data-trust="cert-verified">{{ entry.c.verified_by
              ? $t('trust_manager.certifications.verified_by', { date: day(entry.c.verified_at), name: nameFor(entry.c.verified_by) })
              : $t('trust_manager.certifications.verified_on', { date: day(entry.c.verified_at) }) }}</span>
          </div>
          <div v-if="entry.published && entry.c.state === 'expired'" class="text-xs text-red-300" data-trust="cert-expired-hint">{{ $t('trust_manager.certifications.expired_hint') }}</div>

          <div class="flex flex-wrap items-center gap-x-3 gap-y-2 text-xs">
            <select :value="levelOf(entry.c)" data-trust="cert-level" :disabled="busy" :aria-label="$t('trust_manager.documents.access_label', { title: entry.c.label })"
              @change="changeLevel(entry.c, $event)"
              class="text-sm bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-slate-200">
              <option v-for="lvl in CERT_LEVELS" :key="lvl" :value="lvl">{{ levelLabel(lvl) }}</option>
            </select>
            <template v-if="entry.published">
              <span class="flex items-center gap-1">
                <button type="button" data-trust="cert-up" :disabled="busy || !canMove(entry.c, -1)" :aria-label="$t('trust_manager.documents.move_up')" :title="$t('trust_manager.documents.move_up')" @click="move(entry.c, -1)"
                  class="p-1.5 rounded-md text-slate-300 hover:bg-slate-800 disabled:opacity-30 disabled:cursor-not-allowed">
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M5 15l7-7 7 7" /></svg>
                </button>
                <button type="button" data-trust="cert-down" :disabled="busy || !canMove(entry.c, 1)" :aria-label="$t('trust_manager.documents.move_down')" :title="$t('trust_manager.documents.move_down')" @click="move(entry.c, 1)"
                  class="p-1.5 rounded-md text-slate-300 hover:bg-slate-800 disabled:opacity-30 disabled:cursor-not-allowed">
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" /></svg>
                </button>
              </span>
              <button type="button" data-trust="cert-unpublish" :disabled="busy" @click="ask = { kind: 'unpublish', c: entry.c }" class="text-slate-400 hover:text-red-400 transition-colors">{{ $t('trust_manager.certifications.unpublish') }}</button>
            </template>
            <button v-else type="button" data-trust="cert-publish" :disabled="busy" @click="publish(entry.c)"
              class="px-2.5 py-1 rounded-md bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50">{{ $t('trust_manager.certifications.publish') }}</button>
            <button v-if="entry.c.source === 'file'" type="button" data-trust="cert-download" :disabled="busy" @click="download(entry.c)" class="text-blue-400 hover:text-blue-300 transition-colors">{{ $t('trust_manager.certifications.download') }}</button>
            <button type="button" data-trust="cert-edit" :disabled="busy" @click="openEdit(entry.c)" class="text-blue-400 hover:text-blue-300 transition-colors">{{ $t('common.action.edit') }}</button>
            <button v-if="entry.c.source === 'file'" type="button" data-trust="cert-replace" :disabled="busy" @click="pickReplace(entry.c)" class="text-blue-400 hover:text-blue-300 transition-colors">{{ $t('trust_manager.certifications.replace_file') }}</button>
            <button type="button" data-trust="cert-verify" :disabled="busy" @click="verify(entry.c)" class="text-emerald-400 hover:text-emerald-300 transition-colors">{{ $t('trust_manager.certifications.mark_verified') }}</button>
            <button type="button" data-trust="cert-delete" :disabled="busy" @click="ask = { kind: 'delete', c: entry.c }" class="text-slate-400 hover:text-red-400 transition-colors">{{ $t('common.action.delete') }}</button>
          </div>

          <!-- Edit form -->
          <form v-if="edit && edit.id === entry.c.id" data-trust="cert-edit-form" @submit.prevent="saveEdit(entry.c)" class="mt-3 border border-slate-700 rounded-xl bg-slate-900/60 p-4 space-y-3 text-sm">
            <div class="grid sm:grid-cols-2 gap-3">
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ $t('trust_manager.certifications.f_label') }}</label>
                <input v-model="edit.form.label" type="text" data-trust="cert-e-label" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100" />
                <p v-if="edit.errors.label" class="text-xs text-red-400 mt-1" data-trust="cert-e-error-label">{{ errText(edit.errors.label) }}</p>
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ $t('trust_manager.certifications.f_type') }}</label>
                <select v-model="edit.form.link_type" data-trust="cert-e-type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100">
                  <option v-for="ty in CERT_TYPES" :key="ty" :value="ty">{{ typeLabel(ty) }}</option>
                </select>
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ $t('trust_manager.certifications.f_issuer') }}</label>
                <input v-model="edit.form.issuer" type="text" data-trust="cert-e-issuer" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100" />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ $t('trust_manager.certifications.f_verified') }}</label>
                <input v-model="edit.form.verified_at" type="date" data-trust="cert-e-verified" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100" />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ $t('trust_manager.certifications.f_valid_from') }}</label>
                <input v-model="edit.form.valid_from" type="date" data-trust="cert-e-validfrom" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100" />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ $t('trust_manager.certifications.f_valid_until') }}</label>
                <input v-model="edit.form.valid_until" type="date" data-trust="cert-e-validuntil" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100" />
                <p v-if="edit.errors.valid_until" class="text-xs text-red-400 mt-1" data-trust="cert-e-error-dates">{{ errText(edit.errors.valid_until) }}</p>
              </div>
              <div v-if="entry.c.source === 'link'" class="sm:col-span-2">
                <label class="block text-xs text-slate-500 mb-1">{{ $t('trust_manager.certifications.f_url') }}</label>
                <input v-model="edit.form.url" type="text" data-trust="cert-e-url" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm text-slate-100" />
                <p v-if="edit.errors.url" class="text-xs text-red-400 mt-1" data-trust="cert-e-error-url">{{ errText(edit.errors.url) }}</p>
              </div>
            </div>
            <div v-if="edit.error" class="text-xs text-red-400" data-trust="cert-edit-error">{{ edit.error }}</div>
            <div class="flex justify-end gap-2">
              <button type="button" @click="edit = null" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ $t('common.action.cancel') }}</button>
              <button type="submit" data-trust="cert-edit-save" :disabled="busy" class="px-3 py-1.5 text-xs font-medium rounded-lg bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50">{{ $t('common.action.save') }}</button>
            </div>
          </form>
        </li>
      </template>
    </ul>

    <input ref="replaceInput" type="file" :accept="FILE_ACCEPT" class="hidden" data-trust="cert-replace-input" @change="onReplaceFile" />

    <!-- Delete / unpublish confirmation, as in the documents section -->
    <div v-if="ask" class="fixed inset-0 z-50 flex items-center justify-center px-4" data-trust="confirm-dialog">
      <div class="absolute inset-0 bg-black/60" @click="ask = null" />
      <div class="relative bg-slate-900 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-md p-6 space-y-4 text-sm">
        <h2 class="text-lg font-bold text-white">{{ askTitle() }}</h2>
        <p class="text-slate-400">{{ askBody() }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" @click="ask = null" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ $t('common.action.cancel') }}</button>
          <button type="button" data-trust="confirm-yes" :disabled="busy" @click="confirmAsk"
            class="px-3 py-1.5 text-xs font-medium rounded-lg bg-red-600 hover:bg-red-500 text-white disabled:opacity-50">{{ askAction() }}</button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../../api'
import { renderApiError } from '../../composables/useApiError.js'
import { enumLabel } from '../../composables/useEnumLabel.js'
import { useMembers } from '../../composables/useMembers.js'
import { stateClass } from '../../utils/expiryBadge.js'
import { saveBlob } from '../../utils/saveBlob.js'
import {
  CERT_TYPES, CERT_LEVELS, FILE_ACCEPT, blankForm, changedFields, fileProblem, formFromCert, formatBytes,
  formatDayUTC, hostOf, linkBody, moveIds, orderCertifications, safeHref, uploadFields, validateForm,
} from '../../utils/trustCertifications.js'

// The Certifications and reports section of the trust center manager screen
// (#521): every certification, published or not, with its validity, source and
// publication. Published ones are ordered like the public page.
const { t, locale } = useI18n()
const { nameFor, loadMembers } = useMembers()

const certs = ref([])
const items = ref([]) // every GET /trust-center/items row, for ordering
const defaultLevel = ref('gated')
const loading = ref(false)
const loadError = ref('')
const actionError = ref('')
const busy = ref(false)
const ask = ref(null)
const edit = ref(null) // { id, form, errors, error }
const add = ref(null)
const addErrors = ref({})
const addError = ref('')
const uploading = ref(false)
const addFile = ref(null)
const replaceInput = ref(null)
let addFileObj = null
let replaceFor = null

const itemById = computed(() => new Map(items.value.map((i) => [i.id, i])))
const ordered = computed(() => orderCertifications(certs.value, (c) => itemById.value.get(c.item?.id)?.sort_order ?? c.item?.sort_order ?? 0))
// Rows in display order: published ones, then the unpublished ones.
const entries = computed(() => [
  ...ordered.value.published.map((c) => ({ c, published: true, firstUnpublished: false })),
  ...ordered.value.unpublished.map((c, i) => ({ c, published: false, firstUnpublished: i === 0 })),
])
const pendingLevel = ref({}) // the level chosen for a row that is not published yet
const levelOf = (c) => (c.item ? c.item.access_level : pendingLevel.value[c.id] || defaultLevel.value)
const knownItemIds = computed(() => new Set(certs.value.filter((c) => c.item).map((c) => c.item.id)))

const askKey = () => 'trust_manager.certifications.' + ask.value.kind
const askTitle = () => t(askKey() + '_title', { title: ask.value.c.label })
const askBody = () => t(askKey() + '_body')
const askAction = () => t(askKey())
const typeLabel = (v) => enumLabel('external_link_type', v)
const stateLabel = (v) => enumLabel('external_link_state', v)
const levelLabel = (lvl) => t('trust_manager.level.' + lvl)
const errText = (key) => t('trust_manager.certifications.err_' + key)
const day = (d) => formatDayUTC(d, locale.value)

function validity(c) {
  if (c.valid_from && c.valid_until) return t('trust_manager.certifications.valid_range', { from: day(c.valid_from), until: day(c.valid_until) })
  if (c.valid_until) return t('trust_manager.certifications.valid_until', { date: day(c.valid_until) })
  if (c.valid_from) return t('trust_manager.certifications.valid_from', { date: day(c.valid_from) })
  return t('trust_manager.certifications.no_dates')
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [list, its, settings] = await Promise.all([
      api.getTrustCertifications(),
      api.getTrustItems(),
      api.getTrustSettings().catch(() => null),
    ])
    certs.value = list || []
    items.value = its || []
    if (settings?.default_access_level) defaultLevel.value = settings.default_access_level
  } catch (e) {
    loadError.value = renderApiError(e) || t('common.state.unknown')
  } finally {
    loading.value = false
  }
}

// A server error as text. A 413 is the body limit: say what the limit is.
function problem(e) {
  if (e?.status === 413) return t('trust_manager.certifications.err_file_too_large')
  return renderApiError(e) || t('common.state.unknown')
}

// A 404 means the row is gone (another manager removed it): show the error and
// reload so no dead row stays on screen.
async function fail(e) {
  actionError.value = problem(e)
  if (e?.status === 404) await load()
}

async function act(fn) {
  actionError.value = ''
  busy.value = true
  try {
    await fn()
  } catch (e) {
    await fail(e)
  } finally {
    busy.value = false
  }
}

function changeLevel(c, ev) {
  const level = ev.target.value
  if (!c.item) { pendingLevel.value = { ...pendingLevel.value, [c.id]: level }; return }
  return act(async () => {
    try {
      await api.setTrustItemLevel(c.item.id, level)
    } catch (e) {
      ev.target.value = c.item.access_level
      throw e
    }
    await load()
  })
}
const publish = (c) => act(async () => {
  await api.publishTrustCertification(c.id, pendingLevel.value[c.id] || defaultLevel.value)
  await load()
})

const move = (c, delta) => act(async () => {
  const ids = moveIds(items.value, knownItemIds.value, c.item.id, delta)
  if (!ids) return
  try {
    items.value = (await api.reorderTrustItems('certification', ids)) || []
  } catch (e) {
    if (e?.code === 'trust_order_stale') await load()
    throw e
  }
})

function canMove(c, delta) {
  return !!c.item && !!moveIds(items.value, knownItemIds.value, c.item.id, delta)
}

function confirmAsk() {
  const { kind, c } = ask.value
  return act(async () => {
    ask.value = null
    if (kind === 'delete') await api.deleteTrustCertification(c.id)
    else await api.unpublishTrustCertification(c.id)
    await load()
  })
}

const verify = (c) => act(async () => {
  await api.verifyTrustCertification(c.id)
  await load()
})

const download = (c) => act(async () => {
  const { blob, filename } = await api.downloadTrustCertificationFile(c.id)
  saveBlob(blob, filename || c.file_name)
})

function pickReplace(c) {
  replaceFor = c
  if (replaceInput.value) {
    replaceInput.value.value = ''
    replaceInput.value.click()
  }
}

async function onReplaceFile(ev) {
  const file = ev.target.files?.[0]
  const c = replaceFor
  if (!file || !c) return
  const p = fileProblem(file)
  if (p) { actionError.value = t('trust_manager.certifications.err_' + p); return }
  await act(async () => {
    await api.replaceTrustCertificationFile(c.id, file)
    await load()
  })
}

// ---- edit ----------------------------------------------------------------
function openEdit(c) {
  edit.value = { id: c.id, form: formFromCert(c), errors: {}, error: '' }
}

async function saveEdit(c) {
  const e = edit.value
  const errors = validateForm(e.form, { mode: 'edit', cert: c })
  e.errors = errors
  if (Object.keys(errors).length) return
  const patch = changedFields(c, e.form)
  if (!Object.keys(patch).length) { edit.value = null; return }
  e.error = ''
  busy.value = true
  try {
    await api.updateTrustCertification(c.id, patch)
    edit.value = null
    await load()
  } catch (err) {
    e.error = problem(err)
  } finally {
    busy.value = false
  }
}

// ---- add -----------------------------------------------------------------
function openAdd() {
  actionError.value = ''
  addErrors.value = {}
  addError.value = ''
  addFileObj = null
  add.value = blankForm({ access_level: defaultLevel.value })
}

function onAddFile(ev) {
  addFileObj = ev.target.files?.[0] || null
}

async function submitAdd() {
  const f = add.value
  const errors = validateForm(f, { mode: 'add', file: addFileObj })
  addErrors.value = errors
  addError.value = ''
  if (Object.keys(errors).length) return
  uploading.value = true
  try {
    if (f.source === 'file') await api.createTrustCertificationFile(addFileObj, uploadFields(f))
    else await api.createTrustCertificationLink(linkBody(f))
    add.value = null
    await load()
  } catch (e) {
    addError.value = problem(e)
  } finally {
    uploading.value = false
  }
}

onMounted(() => {
  load()
  loadMembers()
})
</script>
