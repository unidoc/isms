<template>
  <TrustSignIn v-if="resolved && route.meta.signIn" :slug="slug" :base-path="paths.prefix" :root-path="paths.root" @navigate="go" />
  <TrustCenterPage v-else-if="resolved" :slug="slug" :base-path="paths.prefix" :root-path="paths.root" :doc-id="docId" @navigate="go" />
  <div v-else class="trust-page min-h-screen bg-slate-50" />
</template>

<script setup>
// The public trust center route (#519). Works out which organization the page is
// for, then hands over to the page component: on the apex in path mode the slug
// is in the URL, on a tenant host it comes from the host-resolved /config.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '../api.js'
import { isSubdomainMode, trustHostSlug } from '../composables/useCurrentOrg'
import { trustPaths } from '../utils/trustPaths.js'
import TrustCenterPage from '../components/TrustCenterPage.vue'
import TrustSignIn from '../components/TrustSignIn.vue'

const route = useRoute()
const router = useRouter()
const hostSlug = trustHostSlug() // set when served on the organization's trust domain
const subdomain = !hostSlug && isSubdomainMode()

const tenantSlug = ref('')
const resolved = ref(!subdomain)

const slug = computed(() => (hostSlug || (subdomain ? tenantSlug.value : String(route.params.org || ''))))
const paths = computed(() => trustPaths({ trustHost: !!hostSlug, subdomain, slug: slug.value }))
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
