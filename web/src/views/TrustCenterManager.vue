<template>
  <div class="min-h-full">
    <div class="max-w-5xl mx-auto px-4 sm:px-8 py-8 sm:py-10">
      <div v-if="role && !isManager" class="text-sm text-slate-400 border border-slate-800 rounded-xl p-6" data-trust="not-allowed">
        {{ $t('trust_manager.not_allowed') }}
      </div>

      <template v-else-if="isManager">
        <div class="mb-6 space-y-3">
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">{{ $t('trust_manager.title') }}</h1>
          <div class="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm">
            <span class="text-slate-500">{{ $t('trust_manager.address.label') }}</span>
            <code class="text-slate-200 bg-slate-800 rounded px-2 py-1 break-all" data-trust="address">{{ address }}</code>
            <button type="button" data-trust="copy" @click="copy"
              class="px-2.5 py-1 text-xs rounded-md border border-slate-700 text-slate-300 hover:bg-slate-800">{{ copied ? $t('trust_manager.address.copied') : $t('common.action.copy') }}</button>
            <a :href="previewHref" target="_blank" rel="noopener" data-trust="preview"
              class="px-2.5 py-1 text-xs rounded-md bg-blue-600 hover:bg-blue-500 text-white">{{ $t('trust_manager.address.preview') }}</a>
          </div>
        </div>

        <div v-if="loadError" class="text-sm text-red-400 mb-4" data-trust="load-error">
          {{ $t('trust_manager.load_error', { message: loadError }) }}
          <button type="button" @click="loadSettings" class="ml-2 text-blue-400 hover:text-blue-300">{{ $t('common.action.retry') }}</button>
        </div>

        <div role="tablist" :aria-label="$t('trust_manager.tabs.label')" class="flex gap-1 border-b border-slate-800 mb-6">
          <button v-for="tb in tabs" :key="tb" type="button" role="tab" :data-trust-tab="tb" :aria-selected="String(tab === tb)" @click="setTab(tb)"
            :class="tab === tb ? 'text-white border-blue-500' : 'text-slate-400 border-transparent hover:text-slate-200'"
            class="px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors">{{ tabLabel(tb) }}</button>
        </div>

        <!-- Both panels stay mounted: switching tabs must not drop unsaved edits. -->
        <div v-show="tab === 'content'" role="tabpanel">
          <div class="space-y-10">
            <TrustDocumentsSection />
            <TrustCertificationsSection />
          </div>
        </div>
        <div v-show="tab === 'settings'" role="tabpanel">
          <TrustSettingsForm v-if="settings" :settings="settings" @saved="onSaved" />
        </div>
      </template>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { renderApiError } from '../composables/useApiError.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import { publicAddress, previewAddress } from '../utils/trustManager.js'
import TrustDocumentsSection from '../components/trust/TrustDocumentsSection.vue'
import TrustCertificationsSection from '../components/trust/TrustCertificationsSection.vue'
import TrustSettingsForm from '../components/trust/TrustSettingsForm.vue'

// The trust center manager screen: a thin shell (header, tabs) around section
// components. Only admins and managers may use it; for anyone else no trust
// center request is made.
const route = useRoute()
const router = useRouter()
const { orgSlug, subdomainMode } = useCurrentOrg()

const { t } = useI18n()
const tabs = ['content', 'settings']
const tabLabel = (tb) => t('trust_manager.tabs.' + tb)
const tab = ref(route?.query?.tab === 'settings' ? 'settings' : 'content')
watch(() => route?.query?.tab, (v) => { tab.value = v === 'settings' ? 'settings' : 'content' })
function setTab(tb) {
  tab.value = tb
  router?.replace({ query: { ...route?.query, tab: tb === 'content' ? undefined : tb } })
}

const role = ref('')
const isManager = computed(() => role.value === 'admin' || role.value === 'manager')
const settings = ref(null)
const loadError = ref('')
const copied = ref(false)

const previewHref = computed(() => previewAddress(globalThis.location?.origin || '', orgSlug.value, subdomainMode))
const address = computed(() => publicAddress(globalThis.location?.origin || '', orgSlug.value, subdomainMode))

async function copy() {
  try {
    await navigator.clipboard.writeText(address.value)
    copied.value = true
    setTimeout(() => { copied.value = false }, 1500)
  } catch { /* clipboard may be unavailable */ }
}

async function loadSettings() {
  loadError.value = ''
  try {
    settings.value = await api.getTrustSettings()
  } catch (e) {
    loadError.value = renderApiError(e)
  }
}
const onSaved = (s) => { settings.value = s }

onMounted(async () => {
  try {
    role.value = (await api.getMe())?.role || ''
  } catch { role.value = '' }
  if (isManager.value) await loadSettings()
})
</script>
