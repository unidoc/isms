<template>
  <section data-trust="subprocessors-section" class="space-y-4">
    <div class="flex items-center justify-between gap-3 flex-wrap">
      <div>
        <h2 class="text-lg font-semibold text-slate-100">{{ $t('trust_manager.subprocessors.heading') }}</h2>
        <p v-if="changedAt" class="text-xs text-slate-500 mt-0.5" data-trust="sp-changed">{{ $t('trust_manager.subprocessors.last_changed', { date: changedLabel }) }}</p>
      </div>
    </div>

    <div v-if="actionError" class="text-sm text-red-400" data-trust="sp-action-error">{{ actionError }}</div>
    <div v-if="loadError" class="text-sm text-red-400" data-trust="sp-load-error">
      {{ $t('trust_manager.load_error', { message: loadError }) }}
      <button type="button" @click="load" class="ml-2 text-blue-400 hover:text-blue-300">{{ $t('common.action.retry') }}</button>
    </div>
    <div v-else-if="loading && !suppliers.length" class="text-sm text-slate-500">{{ $t('trust_manager.picker.loading') }}</div>

    <!-- Add -->
    <div v-if="!loadError && !(loading && !suppliers.length)" class="flex flex-wrap items-center gap-3" data-trust="sp-add">
      <template v-if="candidates.length">
        <select v-model="pick" data-trust="sp-picker" :aria-label="$t('trust_manager.subprocessors.picker_label')" :disabled="busy"
          class="text-sm bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-slate-100 min-w-[14rem]">
          <option value="" disabled>{{ $t('trust_manager.subprocessors.picker_placeholder') }}</option>
          <option v-for="c in candidates" :key="c.supplier_id" :value="String(c.supplier_id)">{{ c.name }} ({{ c.identifier }})</option>
        </select>
        <button type="button" data-trust="sp-publish" :disabled="busy || !pick" @click="publish(Number(pick))"
          class="px-3 py-1.5 text-sm font-medium rounded-lg bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50 transition-colors">{{ $t('trust_manager.subprocessors.publish') }}</button>
      </template>
      <p v-else class="text-sm text-slate-500" data-trust="sp-no-candidates">{{ $t('trust_manager.subprocessors.no_candidates') }}</p>
    </div>

    <div v-if="!loadError && !loading && !published.length" class="text-sm text-slate-500 border border-dashed border-slate-700 rounded-xl p-6" data-trust="sp-empty">{{ $t('trust_manager.subprocessors.empty') }}</div>

    <ul v-if="published.length" class="divide-y divide-slate-800 border border-slate-800 rounded-xl bg-slate-900/40" data-trust="sp-list">
      <li v-for="sp in published" :key="sp.supplier_id" :data-trust-sp-row="sp.supplier_id" class="p-4 space-y-2">
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-sm font-medium text-slate-100 break-words" data-trust="sp-name">{{ sp.public_name || sp.name }}</span>
          <router-link v-if="!sp.deleted" :to="orgPath('/suppliers/' + sp.identifier)" class="text-xs font-mono text-blue-400 hover:text-blue-300" data-trust="sp-link">{{ sp.identifier }}</router-link>
          <span v-else class="text-xs font-mono text-slate-500" data-trust="sp-link">{{ sp.identifier }}</span>
          <span class="text-xs px-2 py-0.5 rounded-full bg-slate-800 text-slate-300" data-trust="sp-level">{{ $t('trust_manager.level.public') }}</span>
          <span v-if="sp.item.hidden_reason" class="text-xs px-2 py-0.5 rounded-full bg-amber-500/15 text-amber-300" data-trust="sp-hidden">{{ $t('trust_manager.subprocessors.hidden', { reason: reasonLabel(sp.item.hidden_reason) }) }}</span>
        </div>
        <div class="text-xs text-slate-500 flex flex-wrap gap-x-4 gap-y-0.5" data-trust="sp-summary">
          <span v-if="sp.public_purpose">{{ sp.public_purpose }}</span>
          <span v-if="sp.processing_location">{{ sp.processing_location }}</span>
          <span v-if="sp.data_categories">{{ sp.data_categories }}</span>
        </div>
        <div v-if="detailsMissing(sp)" class="text-xs text-amber-300" data-trust="sp-details-missing">{{ $t('trust_manager.subprocessors.details_missing') }}</div>
        <div v-if="sp.item.hidden_reason && !sp.eligible" class="text-xs text-slate-400" data-trust="sp-still-ineligible">{{ $t('trust_manager.subprocessors.still_ineligible', { reason: reasonLabel(sp.ineligible_reason) }) }}</div>

        <div class="flex flex-wrap items-center gap-x-3 gap-y-2 text-xs">
          <span class="flex items-center gap-1">
            <button type="button" data-trust="sp-up" :disabled="busy || !canMove(sp, -1)" :aria-label="$t('trust_manager.documents.move_up')" :title="$t('trust_manager.documents.move_up')" @click="move(sp, -1)"
              class="p-1.5 rounded-md text-slate-300 hover:bg-slate-800 disabled:opacity-30 disabled:cursor-not-allowed">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M5 15l7-7 7 7" /></svg>
            </button>
            <button type="button" data-trust="sp-down" :disabled="busy || !canMove(sp, 1)" :aria-label="$t('trust_manager.documents.move_down')" :title="$t('trust_manager.documents.move_down')" @click="move(sp, 1)"
              class="p-1.5 rounded-md text-slate-300 hover:bg-slate-800 disabled:opacity-30 disabled:cursor-not-allowed">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" /></svg>
            </button>
          </span>
          <button v-if="sp.item.hidden_reason && sp.eligible" type="button" data-trust="sp-republish" :disabled="busy" @click="publish(sp.supplier_id)"
            class="px-2.5 py-1 rounded-md bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50">{{ $t('trust_manager.subprocessors.publish_again') }}</button>
          <button type="button" data-trust="sp-unpublish" :disabled="busy" @click="ask = sp" class="text-slate-400 hover:text-red-400 transition-colors">{{ $t('trust_manager.subprocessors.unpublish') }}</button>
        </div>
      </li>
    </ul>

    <div v-if="ask" class="fixed inset-0 z-50 flex items-center justify-center px-4" data-trust="confirm-dialog">
      <div class="absolute inset-0 bg-black/60" @click="ask = null" />
      <div class="relative bg-slate-900 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-md p-6 space-y-4 text-sm">
        <h2 class="text-lg font-bold text-white">{{ $t('trust_manager.subprocessors.unpublish_title', { title: ask.public_name || ask.name }) }}</h2>
        <p class="text-slate-400">{{ $t('trust_manager.subprocessors.unpublish_body') }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" @click="ask = null" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ $t('common.action.cancel') }}</button>
          <button type="button" data-trust="confirm-yes" :disabled="busy" @click="confirmUnpublish"
            class="px-3 py-1.5 text-xs font-medium rounded-lg bg-red-600 hover:bg-red-500 text-white disabled:opacity-50">{{ $t('trust_manager.subprocessors.unpublish') }}</button>
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
import { useCurrentOrg } from '../../composables/useCurrentOrg.js'
import { formatDate } from '../../composables/useFormat.js'

// The Subprocessors section of the trust center manager screen (#522): the
// suppliers published as subprocessors, always at level public, in display
// order. Hidden ones (terminated, no data access, deleted) stay on the list
// until a manager publishes them again or removes them.
const { t } = useI18n()
const { orgPath } = useCurrentOrg()

const suppliers = ref([])
const changedAt = ref(null)
const loading = ref(false)
const loadError = ref('')
const actionError = ref('')
const busy = ref(false)
const ask = ref(null)
const pick = ref('')

const published = computed(() => suppliers.value.filter((s) => s.item).sort((a, b) => a.item.sort_order - b.item.sort_order))
const candidates = computed(() => suppliers.value.filter((s) => s.eligible && !s.item))
const changedLabel = computed(() => formatDate(changedAt.value, 'datetime'))
const detailsMissing = (s) => !s.public_purpose && !s.processing_location && !s.data_categories

const reasonLabel = (r) => t('trust_manager.subprocessors.reason.' + r)

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const res = await api.getTrustSubprocessors()
    suppliers.value = res?.suppliers || []
    changedAt.value = res?.changed_at || null
    if (!candidates.value.some((c) => String(c.supplier_id) === pick.value)) pick.value = ''
  } catch (e) {
    loadError.value = renderApiError(e) || t('common.state.unknown')
  } finally {
    loading.value = false
  }
}

// The ineligible 409 gets its own sentence with a localized reason; the raw
// token (terminated, no_data_access) never reaches the screen.
function problem(e) {
  if (e?.code === 'trust_subprocessor_ineligible') {
    return t('trust_manager.subprocessors.err_ineligible', { reason: reasonLabel(e.params?.value) })
  }
  return renderApiError(e) || t('common.state.unknown')
}

async function act(fn) {
  actionError.value = ''
  busy.value = true
  try {
    await fn()
  } catch (e) {
    actionError.value = problem(e)
    // The list moved on under us (supplier removed, state changed): show the error and refresh.
    if (e?.status === 404 || e?.status === 409) await load()
  } finally {
    busy.value = false
  }
}

const publish = (supplierId) => act(async () => {
  await api.publishTrustSubprocessor(supplierId)
  await load()
})

const confirmUnpublish = () => act(async () => {
  const sp = ask.value
  ask.value = null
  await api.unpublishTrustSubprocessor(sp.supplier_id)
  await load()
})

// Every subprocessor item id in display order, hidden ones included: the server
// refuses a reorder that leaves one out.
function moveIds(supplierId, delta) {
  const ids = published.value.map((s) => s.item.id)
  const i = published.value.findIndex((s) => s.supplier_id === supplierId)
  const j = i + delta
  if (i < 0 || j < 0 || j >= ids.length) return null
  ;[ids[i], ids[j]] = [ids[j], ids[i]]
  return ids
}
const canMove = (sp, delta) => !!moveIds(sp.supplier_id, delta)

const move = (sp, delta) => act(async () => {
  const ids = moveIds(sp.supplier_id, delta)
  if (!ids) return
  // A stale list (409 trust_order_stale) is reloaded by act().
  await api.reorderTrustItems('subprocessor', ids)
  await load()
})

onMounted(load)
</script>
