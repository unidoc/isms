<template>
  <div class="min-h-screen bg-slate-950 flex items-center justify-center px-4">
    <div class="w-full max-w-sm">
      <!-- Logo / Branding -->
      <div class="text-center mb-8">
        <router-link to="/" class="inline-flex items-center justify-center mb-4 hover:opacity-80 transition-opacity">
          <img v-if="logoUrl && !logoError" :src="logoUrl" @error="logoError = true" alt="" class="h-14 w-auto" />
          <div v-else class="w-14 h-14 rounded-2xl flex items-center justify-center" style="background: linear-gradient(to bottom right, var(--brand-color), color-mix(in srgb, var(--brand-color) 70%, black))">
            <svg class="w-7 h-7 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" />
            </svg>
          </div>
        </router-link>
        <h1 class="text-xl font-bold text-white">Confirm email change</h1>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="text-center text-sm text-slate-400">Confirming…</div>

      <!-- Success -->
      <div v-else-if="!error" class="space-y-4 text-center">
        <div class="rounded-lg px-4 py-3 text-sm bg-emerald-950/50 border border-emerald-900/60 text-emerald-300">
          Your email has been changed<span v-if="newEmail"> to <strong>{{ newEmail }}</strong></span>.
          Please sign in again with your new address.
        </div>
        <router-link to="/login"
          class="inline-block w-full text-white font-medium text-sm rounded-lg px-4 py-2.5 transition-colors brand-btn">
          Go to sign in
        </router-link>
      </div>

      <!-- Error -->
      <div v-else class="space-y-4">
        <div class="rounded-lg px-4 py-3 text-sm bg-red-950/50 border border-red-900/60 text-red-300">
          {{ error }}
        </div>
        <div class="text-center">
          <router-link to="/login" class="text-sm text-slate-500 brand-text-hover transition-colors">
            ← Back to sign in
          </router-link>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import api from '../api'

const route = useRoute()
const token = computed(() => String(route.query.token || ''))

const loading = ref(true)
const error = ref('')
const newEmail = ref('')

const logoUrl = ref(null)
const logoError = ref(false)

onMounted(async () => {
  try {
    const cfg = await api.getConfig()
    if (cfg.branding?.branding_color) document.documentElement.style.setProperty('--brand-color', cfg.branding.branding_color)
    logoUrl.value = cfg.branding?.branding_logo || ''
  } catch { /* ignore */ }

  if (!token.value) {
    error.value = 'Missing or invalid confirmation link.'
    loading.value = false
    return
  }
  try {
    const res = await api.postJSON('/api/v1/auth/verify-email-change', { token: token.value })
    newEmail.value = res.email || ''
  } catch (e) {
    error.value = e.message || 'This confirmation link is invalid or has expired.'
  } finally {
    loading.value = false
  }
})
</script>
