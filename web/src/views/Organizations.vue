<template>
  <div class="min-h-full flex items-center justify-center px-4 py-16">
    <div class="w-full max-w-md">

      <div class="text-center mb-10">
        <div class="w-14 h-14 rounded-2xl bg-gradient-to-br from-blue-500 to-blue-700 flex items-center justify-center mx-auto mb-4">
          <svg class="w-7 h-7 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
          </svg>
        </div>
        <h1 class="text-2xl font-bold text-white">Your organizations</h1>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="text-center text-slate-500 text-sm py-8">Loading...</div>

      <!-- Org list -->
      <div v-else>
        <div v-if="orgs.length > 0" class="space-y-2 mb-8">
          <button v-for="org in orgs" :key="org.slug" @click="enterOrg(org)"
            class="w-full flex items-center justify-between px-5 py-4 bg-slate-900 border border-slate-800 rounded-xl hover:border-blue-500/50 hover:bg-slate-900/80 transition-all text-left group">
            <div>
              <div class="text-sm font-semibold text-white group-hover:text-blue-100">{{ org.name }}</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ org.slug }}</div>
            </div>
            <div class="flex items-center gap-3">
              <span class="text-[10px] px-2 py-0.5 rounded-full font-medium"
                :class="org.role === 'admin' ? 'bg-purple-500/20 text-purple-300' : org.role === 'manager' ? 'bg-blue-500/20 text-blue-300' : 'bg-slate-500/20 text-slate-400'">
                {{ org.role }}
              </span>
              <svg class="w-4 h-4 text-slate-600 group-hover:text-blue-400 transition-colors" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
              </svg>
            </div>
          </button>
        </div>

        <div v-else class="text-center py-8 mb-6">
          <p class="text-slate-400 text-sm">You're not a member of any organization yet.</p>
        </div>

        <!-- Create new -->
        <div v-if="!showCreate" class="text-center">
          <button @click="showCreate = true"
            class="inline-flex items-center gap-2 px-5 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-xl transition-colors">
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15"/></svg>
            Create organization
          </button>
        </div>

        <!-- Create form -->
        <form v-else @submit.prevent="createOrg" class="bg-slate-900 border border-slate-800 rounded-xl p-5 space-y-3">
          <h3 class="text-sm font-semibold text-white mb-3">New organization</h3>
          <div>
            <label class="block text-xs text-slate-500 mb-1">Name</label>
            <input v-model="newOrg.name" @input="onNameInput" type="text" placeholder="Acme Corp" required autofocus
              class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">URL slug</label>
            <div class="flex items-center">
              <span class="px-3 py-2 bg-slate-950 border border-r-0 border-slate-700 rounded-l-lg text-sm text-slate-500">{{ baseDomain }}/</span>
              <input v-model="newOrg.slug" @input="onSlugInput" type="text" placeholder="acme" required
                class="flex-1 px-3 py-2 bg-slate-800 border border-slate-700 rounded-r-lg text-sm text-white focus:outline-none focus:border-blue-500" />
            </div>
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">Template (optional)</label>
            <select v-model="newOrg.template"
              class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500">
              <option value="">None — I'll set up documents manually</option>
              <option value="iso27001">ISO 27001 — full document set</option>
              <option value="soc2">SOC 2 — Trust Services Criteria</option>
              <option value="nis2">NIS2 — EU directive requirements</option>
            </select>
            <div class="text-[10px] text-slate-600 mt-1">Scaffolds document templates in git. Can add more templates later.</div>
          </div>
          <div class="flex gap-2 pt-1">
            <button type="submit" :disabled="creating || !newOrg.name.trim() || !newOrg.slug.trim()"
              class="flex-1 px-4 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50">
              {{ creating ? 'Creating...' : 'Create' }}
            </button>
            <button type="button" @click="showCreate = false" class="px-4 py-2.5 text-sm text-slate-400 hover:text-white">Cancel</button>
          </div>
          <div v-if="error" class="text-xs text-red-400">{{ error }}</div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api, { setApiToken } from '../api'
import { orgEntryURL } from '../composables/useCurrentOrg'

const router = useRouter()

const loading = ref(true)
const orgs = ref([])
const showCreate = ref(false)
const creating = ref(false)
const error = ref('')
const newOrg = ref({ name: '', slug: '', template: 'iso27001' })
const slugTouched = ref(false)
const baseDomain = window.location.hostname

function onNameInput() {
  if (!slugTouched.value) {
    newOrg.value.slug = newOrg.value.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
  }
}

function onSlugInput() {
  slugTouched.value = true
  newOrg.value.slug = newOrg.value.slug.toLowerCase().replace(/[^a-z0-9-]/g, '')
}

async function enterOrg(org) {
  try {
    const result = await api.postJSON('/api/v1/auth/switch-org', { slug: org.slug })
    if (result.token) {
      setApiToken(result.token)
      // Hop to the org's canonical URL: subdomain on hosts that can serve it
      // (e.g. isms.sh → unidoc.isms.sh), path-based on localhost / dev hosts.
      const url = orgEntryURL(org.slug, '/overview')
      if (url.startsWith('http')) {
        window.location.href = url
      } else {
        router.push(url)
      }
    }
  } catch {
    router.push('/login?org=' + org.slug)
  }
}

async function loadOrgs() {
  try {
    const data = await api.getMyOrgs()
    orgs.value = Array.isArray(data) ? data : []
    if (orgs.value.length === 0) {
      showCreate.value = true
    }
  } catch {
    // 401 is handled by fetchJSON (clears token + fires isms:unauthorized)
    return
  }
}

async function createOrg() {
  creating.value = true
  error.value = ''
  try {
    const result = await api.postJSON('/api/v1/organizations', {
      name: newOrg.value.name.trim(),
      slug: newOrg.value.slug.trim(),
      template: newOrg.value.template || '',
    })
    // Switch into the new org
    if (result.slug) {
      await enterOrg(result)
    } else {
      await loadOrgs()
      showCreate.value = false
    }
  } catch (e) {
    error.value = e.message || 'Failed to create organization'
  } finally {
    creating.value = false
  }
}

onMounted(async () => {
  await loadOrgs()
  loading.value = false
})
</script>
