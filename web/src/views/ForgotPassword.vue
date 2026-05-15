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
        <h1 class="text-xl font-bold text-white">Reset your password</h1>
        <p class="text-sm text-slate-500 mt-1">We'll send a link to your email.</p>
      </div>

      <!-- Success state -->
      <div v-if="sent" class="space-y-4">
        <div class="rounded-lg px-4 py-3 text-sm bg-emerald-950/50 border border-emerald-900/60 text-emerald-300">
          <div class="font-semibold mb-1">Check your email</div>
          <div class="text-emerald-400/80">If an account exists for <span class="font-mono">{{ submittedEmail }}</span>, we've sent a reset link. Click it to set a new password. The link is valid for 1 hour.</div>
        </div>
        <router-link to="/login" class="block text-center text-sm text-slate-400 hover:text-white transition-colors">
          ← Back to sign in
        </router-link>
      </div>

      <!-- Form -->
      <form v-else @submit.prevent="doRequest" class="space-y-3">
        <input v-model="email" type="email" placeholder="Email" required autofocus
          class="w-full bg-slate-900 border border-slate-700 rounded-lg px-3.5 py-2.5 text-sm text-white placeholder-slate-600 focus:outline-none brand-focus transition-colors" />
        <div v-if="error" class="text-sm text-red-400">{{ error }}</div>
        <button type="submit" :disabled="loading || !email"
          class="w-full text-white font-medium text-sm rounded-lg px-4 py-2.5 transition-colors disabled:opacity-40 disabled:cursor-not-allowed brand-btn">
          {{ loading ? 'Sending...' : 'Send reset link' }}
        </button>
        <div class="text-center pt-2">
          <router-link to="/login" class="text-sm text-slate-500 brand-text-hover transition-colors">
            Remember your password? <span class="brand-text">Sign in</span>
          </router-link>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import api from '../api'

const email = ref('')
const error = ref('')
const loading = ref(false)
const sent = ref(false)
const submittedEmail = ref('')

const logoUrl = ref(null)
const logoError = ref(false)

async function doRequest() {
  loading.value = true
  error.value = ''
  try {
    await api.postJSON('/api/v1/auth/forgot-password', { email: email.value })
    submittedEmail.value = email.value
    sent.value = true
  } catch (e) {
    error.value = e.message || 'Request failed'
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  try {
    const cfg = await api.getConfig()
    if (cfg.branding?.branding_color) document.documentElement.style.setProperty('--brand-color', cfg.branding.branding_color)
    logoUrl.value = cfg.branding?.branding_logo || ''
  } catch { /* ignore */ }
})
</script>
