<template>
  <div v-if="role && !isManager" class="min-h-screen bg-slate-50 flex items-center justify-center px-4" data-trust="not-allowed">
    <p class="text-slate-600 text-center">{{ $t('trust_manager.not_allowed') }}</p>
  </div>

  <div v-else-if="isManager" class="trust-preview">
    <div class="bg-amber-100 border-b border-amber-300 text-amber-900 text-sm px-4 py-2 flex flex-wrap items-center gap-x-4 gap-y-1" data-trust="preview-banner" role="status">
      <span class="font-semibold">{{ $t('trust_manager.preview.banner') }}</span>
      <span v-if="settings && !settings.enabled" data-trust="preview-off">{{ $t('trust_manager.preview.off') }}</span>
      <a :href="managerPath" class="ml-auto underline" data-trust="preview-back" @click.prevent="go(managerPath)">{{ $t('trust_manager.preview.back') }}</a>
    </div>
    <TrustCenterPage v-if="settings" :slug="orgSlug" :base-path="basePath" :doc-id="docId" :source="source" @navigate="go" />
  </div>
</template>

<script setup>
// The trust center preview (#520): the public page component against the
// authenticated preview endpoints, which answer exactly what a visitor would get
// and ignore the on/off setting. Bare (no app chrome) but not public: the router
// has already required a session, and this checks the role before any preview
// request.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import TrustCenterPage from '../components/TrustCenterPage.vue'

const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()

const role = ref('')
const isManager = computed(() => role.value === 'admin' || role.value === 'manager')
const settings = ref(null)

const basePath = computed(() => orgPath('/trust-center/preview'))
const managerPath = computed(() => orgPath('/trust-center'))
const docId = computed(() => String(route?.params?.id || ''))

const source = {
  overview: () => api.trustPreviewOverview(),
  items: () => api.trustPreviewItems(),
  document: (id) => api.trustPreviewDocument(id),
  certificationFile: (id) => api.trustPreviewCertificationFile(id),
}

function go(path) {
  router?.push(path)
}

onMounted(async () => {
  try {
    role.value = (await api.getMe())?.role || ''
  } catch { role.value = '' }
  if (!isManager.value) return
  try {
    settings.value = await api.getTrustSettings()
  } catch {
    settings.value = { enabled: true } // the banner's off note is optional; show the page
  }
})
</script>
