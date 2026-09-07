<template>
  <div class="min-h-screen bg-slate-950 flex items-center justify-center px-4">
    <div class="w-full max-w-sm">
      <!-- Logo / Branding -->
      <div class="text-center mb-8">
        <router-link to="/" class="inline-flex items-center justify-center mb-4 hover:opacity-80 transition-opacity cursor-pointer">
          <img v-if="logoUrl && !logoError" :src="logoUrl" @error="logoError = true" alt="" class="h-14 w-auto" />
          <div v-else class="w-14 h-14 rounded-2xl flex items-center justify-center" style="background: linear-gradient(to bottom right, var(--brand-color), color-mix(in srgb, var(--brand-color) 70%, black))">
            <svg class="w-7 h-7 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" />
            </svg>
          </div>
        </router-link>
        <h1 class="text-xl font-bold text-white">{{ $t('auth.signup.title') }}</h1>
        <p class="text-sm text-slate-500 mt-1">{{ $t('auth.signup.join', { org: orgName || $t('auth.product_name') }) }}</p>
      </div>

      <!-- Success state -->
      <div v-if="sent" class="space-y-4">
        <div class="rounded-lg px-4 py-3 text-sm bg-emerald-950/50 border border-emerald-900/60 text-emerald-300">
          <div class="font-semibold mb-1">{{ $t('auth.check_email') }}</div>
          <i18n-t keypath="auth.signup.sent" tag="div" class="text-emerald-400/80" scope="global">
            <template #email><span class="font-mono">{{ submittedEmail }}</span></template>
          </i18n-t>
        </div>
        <router-link to="/login" class="block text-center text-sm text-slate-400 hover:text-white transition-colors">
          {{ $t('auth.back_to_sign_in') }}
        </router-link>
      </div>

      <!-- Form -->
      <form v-else @submit.prevent="doSignup" class="space-y-3">
        <input v-model="name" type="text" :placeholder="$t('auth.signup.name_placeholder')" required autofocus
          class="w-full bg-slate-900 border border-slate-700 rounded-lg px-3.5 py-2.5 text-sm text-white placeholder-slate-600 focus:outline-none brand-focus transition-colors" />
        <input v-model="email" type="email" :placeholder="$t('auth.signup.email_placeholder')" required
          class="w-full bg-slate-900 border border-slate-700 rounded-lg px-3.5 py-2.5 text-sm text-white placeholder-slate-600 focus:outline-none brand-focus transition-colors" />
        <input v-model="password" type="password" :placeholder="$t('auth.signup.password_placeholder')" required minlength="7"
          class="w-full bg-slate-900 border border-slate-700 rounded-lg px-3.5 py-2.5 text-sm text-white placeholder-slate-600 focus:outline-none brand-focus transition-colors" />
        <div v-if="error" class="text-sm text-red-400">{{ error }}</div>
        <button type="submit" :disabled="loading || !email || !password"
          class="w-full text-white font-medium text-sm rounded-lg px-4 py-2.5 transition-colors disabled:opacity-40 disabled:cursor-not-allowed brand-btn">
          {{ loading ? $t('auth.signup.submitting') : $t('auth.signup.submit') }}
        </button>
        <div class="text-center pt-2">
          <router-link to="/login" class="text-sm text-slate-500 brand-text-hover transition-colors">
            {{ $t('auth.signup.have_account') }} <span class="brand-text">{{ $t('auth.sign_in') }}</span>
          </router-link>
        </div>
      </form>
    </div>
    <div class="fixed bottom-4 left-0 right-0 text-center text-xs text-slate-600">
      <a v-if="privacyUrl" :href="privacyUrl" target="_blank" class="hover:text-slate-400 transition-colors">{{ $t('auth.footer.privacy') }}</a>
      <span v-if="termsUrl && privacyUrl" class="mx-2">&middot;</span> <!-- i18n-ignore: typographic separator -->
      <a v-if="termsUrl" :href="termsUrl" target="_blank" class="hover:text-slate-400 transition-colors">{{ $t('auth.footer.terms') }}</a>
      <span v-if="showPoweredBy && (termsUrl || privacyUrl)" class="mx-2">&middot;</span> <!-- i18n-ignore: typographic separator -->
      <a v-if="showPoweredBy" href="https://isms.sh" target="_blank" rel="noopener" class="hover:text-slate-400 transition-colors">{{ $t('auth.footer.powered_by') }}</a>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import api from '../api'
import { renderApiError } from '../composables/useApiError.js'

const { t } = useI18n()

const router = useRouter()

const name = ref('')
const email = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)
const sent = ref(false)
const submittedEmail = ref('')

// Branding — pulled from /api/v1/config the same way Login.vue does
const orgName = ref('')
const logoUrl = ref(null)
const logoError = ref(false)
const termsUrl = ref('')
const privacyUrl = ref('')
const showPoweredBy = ref(true)

async function doSignup() {
  loading.value = true
  error.value = ''
  try {
    await api.postJSON('/api/v1/auth/signup', {
      email: email.value,
      name: name.value,
      password: password.value,
    })
    submittedEmail.value = email.value
    sent.value = true
  } catch (e) {
    error.value = renderApiError(e) || t('auth.signup.error_failed')
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  try {
    const cfg = await api.getConfig()
    // Self-registration disabled (ISMS_USER_SIGNUP unset) — the signup endpoint
    // 403s anyway; don't dangle a dead form. Send them to the login page.
    if (cfg.signup_enabled !== true) {
      router.replace('/login')
      return
    }
    if (cfg.organization?.name) orgName.value = cfg.organization.name
    if (cfg.organization_name) orgName.value = cfg.organization_name
    if (cfg.branding?.branding_name) orgName.value = cfg.branding.branding_name
    if (cfg.branding?.branding_color) document.documentElement.style.setProperty('--brand-color', cfg.branding.branding_color)
    logoUrl.value = cfg.branding?.branding_logo || ''
    showPoweredBy.value = cfg.show_powered_by !== false
    if (cfg.terms_url) termsUrl.value = cfg.terms_url
    else if (cfg.has_terms) termsUrl.value = '/terms'
    if (cfg.privacy_url) privacyUrl.value = cfg.privacy_url
    else if (cfg.has_privacy) privacyUrl.value = '/privacy'
  } catch {
    // Config unreachable — fail closed, same as signup_enabled === false.
    router.replace('/login')
  }
})
</script>
