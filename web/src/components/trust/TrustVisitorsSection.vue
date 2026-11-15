<template>
  <section data-trust="visitors-section" class="space-y-4">
    <h2 class="text-lg font-semibold text-slate-100">{{ $t('trust_manager.visitors.heading') }}</h2>

    <div v-if="actionError" class="text-sm text-red-400" data-trust="visitor-action-error">{{ actionError }}</div>

    <div v-if="loadError" class="text-sm text-red-400" data-trust="visitors-error">
      {{ $t('trust_manager.load_error', { message: loadError }) }}
      <button type="button" data-trust="visitors-retry" @click="load" class="ml-2 text-blue-400 hover:text-blue-300">{{ $t('common.action.retry') }}</button>
    </div>
    <div v-else-if="loading && !rows.length" class="text-sm text-slate-500" data-trust="visitors-loading">{{ $t('trust_manager.visitors.loading') }}</div>
    <div v-else-if="!rows.length" class="text-sm text-slate-500 border border-dashed border-slate-700 rounded-xl p-6" data-trust="visitors-empty">{{ $t('trust_manager.visitors.empty') }}</div>

    <!-- The table scrolls inside its box instead of squeezing its columns. -->
    <div v-if="rows.length" class="overflow-x-auto border border-slate-800 rounded-xl bg-slate-900/40">
      <table class="w-full min-w-[56rem] text-sm text-left">
        <thead class="text-xs uppercase tracking-wide text-slate-500 border-b border-slate-800">
          <tr>
            <th class="px-3 py-2 font-medium">{{ $t('trust_manager.visitors.col_email') }}</th>
            <th class="px-3 py-2 font-medium">{{ $t('trust_manager.visitors.col_name') }}</th>
            <th class="px-3 py-2 font-medium">{{ $t('trust_manager.visitors.col_company') }}</th>
            <th class="px-3 py-2 font-medium">{{ $t('trust_manager.visitors.col_first_seen') }}</th>
            <th class="px-3 py-2 font-medium">{{ $t('trust_manager.visitors.col_last_seen') }}</th>
            <th class="px-3 py-2 font-medium">{{ $t('trust_manager.visitors.col_sessions') }}</th>
            <th class="px-3 py-2 font-medium">{{ $t('trust_manager.visitors.col_status') }}</th>
            <th class="px-3 py-2" />
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800">
          <!-- Name and company are typed by the visitor: text only, never HTML. -->
          <tr v-for="v in rows" :key="v.id" :data-trust-visitor="v.email">
            <td class="px-3 py-2 text-slate-200 whitespace-nowrap" data-trust="v-email">{{ v.email }}</td>
            <td class="px-3 py-2 text-slate-300 break-words" data-trust="v-name">{{ v.name }}</td>
            <td class="px-3 py-2 text-slate-300 break-words" data-trust="v-company">{{ v.company }}</td>
            <td class="px-3 py-2 text-slate-400 whitespace-nowrap">{{ when(v.first_seen) }}</td>
            <td class="px-3 py-2 text-slate-400 whitespace-nowrap">{{ when(v.last_seen) }}</td>
            <td class="px-3 py-2 text-slate-300" data-trust="v-sessions">{{ v.active_sessions }}</td>
            <td class="px-3 py-2">
              <span v-if="v.blocked" class="text-xs px-2 py-1 rounded-full bg-red-500/15 text-red-300 whitespace-nowrap" data-trust="v-blocked" :title="blockedTitle(v)">{{ $t('trust_manager.visitors.blocked') }}</span>
              <span v-else class="text-xs px-2 py-1 rounded-full bg-emerald-500/15 text-emerald-300 whitespace-nowrap" data-trust="v-active">{{ $t('trust_manager.visitors.status_active') }}</span>
            </td>
            <td class="px-3 py-2">
              <div class="flex items-center gap-4 text-xs whitespace-nowrap">
                <button type="button" data-trust="revoke" :disabled="v.active_sessions === 0 || busyId === v.id"
                  :title="$t('trust_manager.visitors.revoke_title')" @click="revoke(v)"
                  class="text-slate-300 hover:text-white disabled:opacity-40 disabled:cursor-not-allowed">{{ $t('trust_manager.visitors.revoke') }}</button>
                <button v-if="!v.blocked" type="button" data-trust="block" :disabled="busyId === v.id"
                  :title="$t('trust_manager.visitors.block_title')" @click="setBlocked(v, true)"
                  class="text-slate-300 hover:text-red-400 disabled:opacity-40">{{ $t('trust_manager.visitors.block') }}</button>
                <button v-else type="button" data-trust="unblock" :disabled="busyId === v.id"
                  :title="$t('trust_manager.visitors.unblock_title')" @click="setBlocked(v, false)"
                  class="text-slate-300 hover:text-white disabled:opacity-40">{{ $t('trust_manager.visitors.unblock') }}</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../../api'
import { renderApiError } from '../../composables/useApiError.js'
import { formatDate } from '../../composables/useFormat.js'

// The people who signed in to the public trust center. They are not users: this
// list is its own endpoint and none of these actions touch the user list.
const { t } = useI18n()
const rows = ref([])
const loading = ref(true)
const loadError = ref('')
const actionError = ref('')
const busyId = ref(0)
// An instant (first/last seen): formatDate, never formatDay, which pins UTC.
const when = (sec) => formatDate(sec, 'datetime')
// Who blocked and when: a tooltip, so the badge itself stays one short word.
const blockedTitle = (v) => [v.blocked_by ? t('trust_manager.visitors.blocked_by', { by: v.blocked_by }) : '', v.blocked_at ? when(v.blocked_at) : ''].filter(Boolean).join(' · ')

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    rows.value = await api.listTrustVisitors()
  } catch (e) {
    loadError.value = renderApiError(e)
  } finally {
    loading.value = false
  }
}

async function act(v, fn) {
  busyId.value = v.id
  actionError.value = ''
  try {
    await fn()
  } catch (e) {
    actionError.value = t('trust_manager.visitors.action_error', { message: renderApiError(e) })
  } finally {
    busyId.value = 0
  }
}

// Revoking ends sessions only, so the list is read again for the new counts.
const revoke = (v) => act(v, async () => {
  await api.revokeTrustVisitorSessions(v.id)
  await load()
})

// Blocking also ends every session; the answer is the visitor as it is now.
const setBlocked = (v, blocked) => act(v, async () => {
  const next = await api.setTrustVisitorBlocked(v.id, blocked)
  rows.value = rows.value.map((r) => (r.id === next.id ? next : r))
})

onMounted(load)
</script>
