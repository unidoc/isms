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
        <h1 class="text-xl font-bold text-white">{{ $t('auth.verify_email.title') }}</h1>
        <p class="text-sm text-slate-500 mt-1">{{ $t('auth.verify_email.subtitle') }}</p>
      </div>

      <!-- Missing token -->
      <div v-if="!token" class="rounded-lg px-4 py-3 text-sm bg-red-950/50 border border-red-900/60 text-red-300">
        <i18n-t keypath="auth.verify_email.missing_token" scope="global">
          <template #link><router-link to="/forgot-password" class="underline">{{ $t('auth.verify_email.missing_token_link') }}</router-link></template>
        </i18n-t>
      </div>

      <!-- Form -->
      <form v-else @submit.prevent="doVerify" class="space-y-3">
        <input v-model="password" type="password" :placeholder="$t('auth.verify_email.password_placeholder')" required minlength="7" autofocus
          class="w-full bg-slate-900 border border-slate-700 rounded-lg px-3.5 py-2.5 text-sm text-white placeholder-slate-600 focus:outline-none brand-focus transition-colors" />
        <input v-model="confirm" type="password" :placeholder="$t('auth.verify_email.confirm_placeholder')" required minlength="7"
          class="w-full bg-slate-900 border border-slate-700 rounded-lg px-3.5 py-2.5 text-sm text-white placeholder-slate-600 focus:outline-none brand-focus transition-colors" />
        <div v-if="error" class="text-sm text-red-400">{{ error }}</div>
        <button type="submit" :disabled="loading || !password || password !== confirm"
          class="w-full text-white font-medium text-sm rounded-lg px-4 py-2.5 transition-colors disabled:opacity-40 disabled:cursor-not-allowed brand-btn">
          {{ loading ? $t('auth.verify_email.submitting') : $t('auth.verify_email.submit') }}
        </button>
        <div class="text-center pt-2">
          <router-link to="/login" class="text-sm text-slate-500 brand-text-hover transition-colors">
            {{ $t('auth.back_to_sign_in') }}
          </router-link>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import api, { setApiToken } from '../api'
import { isSubdomainMode, orgEntryURL } from '../composables/useCurrentOrg.js'
import { renderApiError } from '../composables/useApiError.js'

const { t } = useI18n()

const route = useRoute()
const router = useRouter()

const token = computed(() => String(route.query.token || ''))
const password = ref('')
const confirm = ref('')
const error = ref('')
const loading = ref(false)

const logoUrl = ref(null)
const logoError = ref(false)

async function doVerify() {
  if (password.value !== confirm.value) {
    error.value = t('auth.verify_email.error_mismatch')
    return
  }
  loading.value = true
  error.value = ''
  try {
    const res = await api.postJSON('/api/v1/auth/verify-email', {
      token: token.value,
      password: password.value,
    })
    // Backend returns a JWT on success — log the user in and go to their org.
    if (res.token) {
      setApiToken(res.token)
      const org = res.organization_slug || res.org_slug || ''
      if (org) {
        // In subdomain mode we're already on the right host; just go to /overview.
        // Otherwise hop to the canonical entry URL (path-based on dev hosts).
        if (isSubdomainMode()) {
          router.push('/overview')
        } else {
          window.location.href = orgEntryURL(org, '/overview')
        }
      } else {
        router.push('/organizations')
      }
    } else {
      router.push('/login')
    }
  } catch (e) {
    error.value = renderApiError(e) || t('auth.verify_email.error_failed')
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
