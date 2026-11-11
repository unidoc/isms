<template>
  <section data-trust="documents-section" class="space-y-4">
    <TrustClassificationConfirm :pending="pending" @answer="answer" />

    <div class="flex items-center justify-between gap-3 flex-wrap">
      <h2 class="text-lg font-semibold text-slate-100">{{ $t('trust_manager.documents.heading') }}</h2>
      <button type="button" data-trust="open-picker" @click="openPicker"
        class="px-3 py-1.5 text-sm font-medium rounded-lg bg-blue-600 hover:bg-blue-500 text-white transition-colors">{{ $t('trust_manager.documents.publish') }}</button>
    </div>

    <div v-if="actionError" class="text-sm text-red-400" data-trust="action-error">{{ actionError }}</div>
    <div v-if="loadError" class="text-sm text-red-400" data-trust="load-error">
      {{ $t('trust_manager.load_error', { message: loadError }) }}
      <button type="button" @click="load" class="ml-2 text-blue-400 hover:text-blue-300">{{ $t('common.action.retry') }}</button>
    </div>
    <div v-else-if="loading && !rows.length" class="text-sm text-slate-500">{{ $t('trust_manager.picker.loading') }}</div>
    <div v-else-if="!rows.length" class="text-sm text-slate-500 border border-dashed border-slate-700 rounded-xl p-6" data-trust="empty">{{ $t('trust_manager.documents.empty') }}</div>

    <ul v-if="rows.length" class="divide-y divide-slate-800 border border-slate-800 rounded-xl bg-slate-900/40">
      <li v-for="(row, i) in rows" :key="row.id" :data-trust-row="row.document.document_id"
        class="p-4 flex flex-wrap items-center gap-x-4 gap-y-3 lg:grid lg:grid-cols-[minmax(0,1fr)_14rem_8rem_4.5rem_13rem]">
        <div class="min-w-0 flex-1 basis-56">
          <router-link :to="orgPath('/documents/' + row.document.document_id)" class="text-sm font-medium text-blue-400 hover:text-blue-300 break-words" data-trust="title">{{ row.document.title }}</router-link>
          <div class="text-xs text-slate-500 mt-0.5 flex flex-wrap gap-x-3">
            <span class="font-mono">{{ row.document.document_id }}</span>
            <span>{{ $t('trust_manager.documents.source') }}</span>
            <span v-if="row.document.classification" data-trust="classification">{{ $t('trust_manager.documents.classification', { value: row.document.classification }) }}</span>
          </div>
          <div v-if="badge(row)" class="mt-1.5 inline-block text-xs px-2 py-0.5 rounded-full bg-red-500/15 text-red-300" data-trust="classified-public">{{ $t('trust_manager.documents.classified_public', { classification: badge(row).classification }) }}</div>
        </div>

        <span :class="statusClass(row)" class="text-xs px-2 py-1 rounded-full lg:justify-self-start" data-trust="status">{{ statusText(row) }}</span>

        <select :value="row.access_level" data-trust="level" :disabled="busy || !!pending" :aria-label="$t('trust_manager.documents.access_label', { title: row.document.title })"
          @change="changeLevel(row, $event)"
          class="text-sm bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-slate-200">
          <option v-for="lvl in levels" :key="lvl" :value="lvl">{{ levelLabel(lvl) }}</option>
        </select>

        <div class="flex items-center gap-1 lg:justify-self-start" :class="row.hidden_reason ? 'invisible' : ''">
          <template v-if="!row.hidden_reason">
            <button type="button" data-trust="up" :disabled="visibleNeighbour(rows, i, -1) < 0 || busy || !!pending" :aria-label="$t('trust_manager.documents.move_up')" :title="$t('trust_manager.documents.move_up')" @click="move(i, -1)"
              class="p-1.5 rounded-md text-slate-300 hover:bg-slate-800 disabled:opacity-30 disabled:cursor-not-allowed">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M5 15l7-7 7 7" /></svg>
            </button>
            <button type="button" data-trust="down" :disabled="visibleNeighbour(rows, i, 1) < 0 || busy || !!pending" :aria-label="$t('trust_manager.documents.move_down')" :title="$t('trust_manager.documents.move_down')" @click="move(i, 1)"
              class="p-1.5 rounded-md text-slate-300 hover:bg-slate-800 disabled:opacity-30 disabled:cursor-not-allowed">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" /></svg>
            </button>
          </template>
        </div>

        <div class="flex flex-wrap items-center gap-2 text-xs">
          <button v-if="actions(row).unpublish" type="button" data-trust="unpublish" @click="ask = { kind: 'unpublish', row }"
            class="text-slate-400 hover:text-red-400 transition-colors">{{ $t('trust_manager.documents.unpublish') }}</button>
          <template v-if="actions(row).publishAgain">
            <button type="button" data-trust="publish-again" :disabled="actions(row).publishAgain !== 'enabled' || busy" @click="publishAgain(row)"
              class="px-2.5 py-1 rounded-md bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50 disabled:cursor-not-allowed">{{ $t('trust_manager.documents.publish_again') }}</button>
            <span v-if="actions(row).publishAgain === 'disabled_retired'" class="text-slate-500" data-trust="retired-hint">{{ $t('trust_manager.documents.retired_hint') }}</span>
          </template>
          <button v-if="actions(row).remove" type="button" data-trust="remove" :disabled="busy" @click="ask = { kind: 'remove', row }"
            class="text-slate-400 hover:text-red-400 transition-colors">{{ $t('trust_manager.documents.remove') }}</button>
        </div>
      </li>
    </ul>

    <!-- Unpublish / remove confirmation -->
    <div v-if="ask" class="fixed inset-0 z-50 flex items-center justify-center px-4" data-trust="confirm-dialog">
      <div class="absolute inset-0 bg-black/60" @click="ask = null" />
      <div class="relative bg-slate-900 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-md p-6 space-y-4 text-sm">
        <h2 class="text-lg font-bold text-white">{{ askTitle() }}</h2>
        <p class="text-slate-400">{{ askBody() }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" @click="ask = null" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ $t('common.action.cancel') }}</button>
          <button type="button" data-trust="confirm-yes" :disabled="busy" @click="removeItem(ask.row)"
            class="px-3 py-1.5 text-xs font-medium rounded-lg bg-red-600 hover:bg-red-500 text-white disabled:opacity-50">{{ askAction() }}</button>
        </div>
      </div>
    </div>

    <!-- Publish document picker -->
    <div v-if="picker" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4" data-trust="picker">
      <div class="absolute inset-0 bg-black/60" @click="picker = null" />
      <div class="relative bg-slate-900 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-lg p-6 space-y-4 text-sm max-h-[84vh] overflow-y-auto">
        <h2 class="text-lg font-bold text-white">{{ $t('trust_manager.picker.title') }}</h2>
        <TrustDocumentSearch v-if="!picker.doc" :exclude="itemIds" @pick="pick" />
        <template v-else>
          <div class="flex items-center justify-between gap-2">
            <div>
              <div class="text-slate-200 font-medium">{{ picker.doc.title }}</div>
              <div class="text-xs text-slate-500 font-mono">{{ picker.doc.document_id }}</div>
            </div>
            <button type="button" data-trust="picker-back" @click="picker.doc = null; picker.state = null" class="text-xs text-blue-400 hover:text-blue-300">{{ $t('trust_manager.picker.choose_other') }}</button>
          </div>
          <div v-if="picker.error" class="text-red-400 text-xs" data-trust="picker-error">{{ picker.error }}</div>
          <template v-if="picker.state">
            <div v-if="!picker.state.can_publish" class="text-amber-400 text-xs" data-trust="picker-reason">
              {{ reasonText(picker.state.cannot_publish_reason) }}
            </div>
            <fieldset class="space-y-2">
              <legend class="text-xs text-slate-500 mb-2">{{ $t('trust_manager.picker.level_label') }}</legend>
              <label v-for="opt in levelOptions" :key="opt" class="flex items-center gap-2 text-slate-300 cursor-pointer">
                <input type="radio" name="trust-picker-level" :value="opt" v-model="picker.level" />
                <span>{{ levelLabel(opt) }}</span>
              </label>
            </fieldset>
          </template>
        </template>
        <div class="flex justify-end gap-2">
          <button type="button" @click="picker = null" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ $t('common.action.cancel') }}</button>
          <button v-if="picker.doc" type="button" data-trust="picker-publish" :disabled="busy || !picker.state || !picker.state.can_publish" @click="publishPicked"
            class="px-3 py-1.5 text-xs font-medium rounded-lg bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50 disabled:cursor-not-allowed">{{ $t('trust_manager.picker.publish') }}</button>
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
import { useClassificationConfirm } from '../../composables/useClassificationConfirm.js'
import { useCurrentOrg } from '../../composables/useCurrentOrg.js'
import { compareTrustItems } from '../../utils/trustOrder.js'
import { rowStatus, rowActions, visibleNeighbour, sensitiveBadge } from '../../utils/trustManager.js'
import TrustClassificationConfirm from './TrustClassificationConfirm.js'
import TrustDocumentSearch from './TrustDocumentSearch.vue'

// The Documents section of the trust center manager screen. Rows come from
// GET /trust-center/items and are ordered like the public page.
const { t } = useI18n()
const { orgPath } = useCurrentOrg()
const { pending, answer, run } = useClassificationConfirm()

const levels = ['public', 'gated', 'restricted']
const levelOptions = ['', 'public', 'gated', 'restricted']

const items = ref([])
const loading = ref(false)
const loadError = ref('')
const actionError = ref('')
const busy = ref(false)
const ask = ref(null)
const picker = ref(null)

const rows = computed(() =>
  items.value
    .filter((r) => r.kind === 'document' && r.document)
    .sort((a, b) => compareTrustItems(a, b, (r) => r.document.title)),
)
const itemIds = computed(() => rows.value.map((r) => r.document.document_id))

const actions = rowActions
const badge = sensitiveBadge
const levelLabel = (lvl) => t('trust_manager.level.' + (lvl || 'default'))
const askKey = () => 'trust_manager.documents.' + ask.value.kind
const askTitle = () => t(askKey() + '_title', { title: ask.value.row.document.title })
const askBody = () => t(askKey() + '_body')
const askAction = () => t(askKey())
const reasonText = (reason) => t('trust_manager.picker.cannot_' + reason)
const statusText = (row) => {
  const s = rowStatus(row)
  return t('trust_manager.status.' + s.key, s.params)
}
const statusClass = (row) => ({
  published: 'bg-emerald-500/15 text-emerald-300',
  draft_in_progress: 'bg-amber-500/15 text-amber-300',
  cannot_publish: 'bg-amber-500/15 text-amber-300',
  hidden_retired: 'bg-slate-500/20 text-slate-300',
  hidden_deleted: 'bg-slate-500/20 text-slate-300',
  error: 'bg-red-500/15 text-red-300',
}[rowStatus(row).key])

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    items.value = (await api.getTrustItems()) || []
  } catch (e) {
    loadError.value = renderApiError(e) || t('common.state.unknown')
  } finally {
    loading.value = false
  }
}

// A 404 means the row is gone (another manager removed it): show the error and
// reload so no dead row stays on screen.
async function fail(e) {
  actionError.value = renderApiError(e) || t('common.state.unknown')
  if (e?.status === 404) await load()
}

async function changeLevel(row, ev) {
  const level = ev.target.value
  actionError.value = ''
  busy.value = true
  try {
    const out = await run((confirm) => api.setTrustItemLevel(row.id, level, confirm))
    if (out.cancelled) {
      ev.target.value = row.access_level // nothing changed: show the stored level again
      return
    }
    items.value = items.value.map((r) => (r.id === row.id ? out.result : r))
  } catch (e) {
    ev.target.value = row.access_level
    await fail(e)
  } finally {
    busy.value = false
  }
}

async function move(i, delta) {
  const j = visibleNeighbour(rows.value, i, delta)
  if (j < 0) return
  const ids = rows.value.map((r) => r.id)
  ;[ids[i], ids[j]] = [ids[j], ids[i]] // hidden rows in between keep their slots
  actionError.value = ''
  busy.value = true
  try {
    items.value = (await api.reorderTrustItems('document', ids)) || []
  } catch (e) {
    await fail(e)
    if (e?.code === 'trust_order_stale') await load()
  } finally {
    busy.value = false
  }
}

async function removeItem(row) {
  busy.value = true
  actionError.value = ''
  try {
    await api.unpublishDocumentTrust(row.document.document_id)
    ask.value = null
    await load()
  } catch (e) {
    ask.value = null
    await fail(e)
  } finally {
    busy.value = false
  }
}

async function publishAgain(row) {
  busy.value = true
  actionError.value = ''
  try {
    const out = await run((confirm) => api.publishDocumentTrust(row.document.document_id, row.access_level, confirm))
    if (!out.cancelled) await load()
  } catch (e) {
    await fail(e)
  } finally {
    busy.value = false
  }
}

function openPicker() {
  actionError.value = ''
  picker.value = { doc: null, state: null, level: '', error: '' }
}

async function pick(doc) {
  picker.value = { doc, state: null, level: '', error: '' }
  const p = picker.value // the reactive object, so a later close or other pick can be told apart
  try {
    const state = await api.getDocumentTrust(doc.document_id)
    if (picker.value === p) p.state = state
  } catch (e) {
    if (picker.value === p) p.error = renderApiError(e) || t('common.state.unknown')
  }
}

async function publishPicked() {
  const p = picker.value
  busy.value = true
  p.error = ''
  try {
    const out = await run((confirm) => api.publishDocumentTrust(p.doc.document_id, p.level, confirm))
    if (out.cancelled) return
    picker.value = null
    await load()
  } catch (e) {
    p.error = renderApiError(e) || t('common.state.unknown')
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>
