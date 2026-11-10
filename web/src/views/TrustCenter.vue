<template>
  <TrustCenterPage v-if="resolved" :slug="slug" :base-path="basePath" :doc-id="docId" @navigate="go" />
  <div v-else class="trust-page min-h-screen bg-slate-50" />
</template>

<script setup>
// The public trust center route (#519). Works out which organization the page is
// for, then hands over to the page component: on the apex in path mode the slug
// is in the URL, on a tenant host it comes from the host-resolved /config.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '../api.js'
import { isSubdomainMode } from '../composables/useCurrentOrg'
import TrustCenterPage from '../components/TrustCenterPage.vue'

const route = useRoute()
const router = useRouter()
const subdomain = isSubdomainMode()

const tenantSlug = ref('')
const resolved = ref(!subdomain)

const slug = computed(() => (subdomain ? tenantSlug.value : String(route.params.org || '')))
const basePath = computed(() => (subdomain ? '/trust' : `/${encodeURIComponent(slug.value)}/trust`))
const docId = computed(() => String(route.params.id || ''))

onMounted(async () => {
  if (!subdomain) return
  try {
    const cfg = await api.publicConfig()
    tenantSlug.value = cfg.organization_slug || ''
  } catch {
    tenantSlug.value = ''
  }
  resolved.value = true
})

function go(path) {
  router.push(path)
}
</script>
