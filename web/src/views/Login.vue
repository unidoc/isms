<template>
  <div class="min-h-screen bg-slate-950 flex items-center justify-center px-4">
    <div class="w-full max-w-sm">
      <!-- Org discovery: when no org is known, ask the user first -->
      <div v-if="!orgSlug" class="text-center">
        <router-link to="/" class="inline-flex items-center justify-center mb-4 hover:opacity-80 transition-opacity">
          <img v-if="logoUrl && !logoError" :src="logoUrl" @error="logoError = true" alt="" class="h-14 w-auto" />
          <div v-else class="w-14 h-14 rounded-2xl flex items-center justify-center" style="background: linear-gradient(to bottom right, var(--brand-color), color-mix(in srgb, var(--brand-color) 70%, black))">
            <svg class="w-7 h-7 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" />
            </svg>
          </div>
        </router-link>
        <h1 class="text-xl font-bold text-white mb-1">Sign in to ISMS</h1>
        <p class="text-sm text-slate-500 mb-6">Enter your organization to continue</p>
        <form @submit.prevent="goToOrg" class="space-y-3">
          <input
            v-model="orgDiscoveryInput"
            type="text"
            placeholder="Organization name, e.g. acme"
            autocomplete="off"
            spellcheck="false"
            class="w-full bg-slate-900 border border-slate-700 rounded-lg px-3.5 py-2.5 text-sm text-white placeholder-slate-600 focus:outline-none brand-focus transition-colors"
          />
          <div v-if="orgDiscoveryError" class="text-sm text-red-400">{{ orgDiscoveryError }}</div>
          <button
            type="submit"
            :disabled="!orgDiscoveryInput.trim()"
            class="w-full text-white font-medium text-sm rounded-lg px-4 py-2.5 transition-colors disabled:opacity-40 disabled:cursor-not-allowed brand-btn"
          >
            Continue
          </button>
        </form>
        <div v-if="signupEnabled" class="text-sm text-slate-500 mt-6">
          Don't have an account?
          <router-link to="/signup" class="brand-text hover:brightness-125 transition-colors">Get started</router-link>
        </div>
      </div>

      <!-- Normal login form: when org is known -->
      <template v-else>
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
        <h1 class="text-xl font-bold text-white">Sign in to {{ orgName || 'ISMS' }}</h1>
        <p v-if="orgSlug" class="text-sm text-slate-500 mt-1">{{ orgSlug }}</p>
      </div>

      <!-- OIDC Providers -->
      <div v-if="oidcProviders.length > 0" class="space-y-3 mb-6">
        <button
          v-for="provider in oidcProviders"
          :key="provider.provider_name"
          @click="oidcLogin(provider)"
          class="w-full flex items-center justify-center gap-3 px-4 py-2.5 rounded-lg text-sm font-medium transition-colors border"
          :class="oidcButtonClass(provider.provider_name)"
        >
          <!-- Microsoft icon -->
          <svg v-if="provider.provider_name === 'microsoft' || provider.provider_name === 'azure'" class="w-5 h-5" viewBox="0 0 21 21" fill="none">
            <rect x="1" y="1" width="9" height="9" fill="#F25022"/>
            <rect x="11" y="1" width="9" height="9" fill="#7FBA00"/>
            <rect x="1" y="11" width="9" height="9" fill="#00A4EF"/>
            <rect x="11" y="11" width="9" height="9" fill="#FFB900"/>
          </svg>
          <!-- Google icon -->
          <svg v-else-if="provider.provider_name === 'google'" class="w-5 h-5" viewBox="0 0 24 24">
            <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 01-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z" fill="#4285F4"/>
            <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
            <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/>
            <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
          </svg>
          <!-- Generic icon -->
          <svg v-else class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
          </svg>
          Sign in with {{ provider.display_name || provider.provider_name }}
        </button>

        <!-- Divider -->
        <div class="relative my-6">
          <div class="absolute inset-0 flex items-center">
            <div class="w-full border-t border-slate-800"></div>
          </div>
          <div class="relative flex justify-center text-xs">
            <span class="px-3 bg-slate-950 text-slate-600">or sign in with email</span>
          </div>
        </div>
      </div>

      <!-- Login Form -->
      <form @submit.prevent="handleLogin" class="space-y-4">
        <div>
          <label for="email" class="block text-sm font-medium text-slate-400 mb-1.5">Email</label>
          <input
            id="email"
            v-model="email"
            type="email"
            required
            autocomplete="email"
            :disabled="loading"
            class="w-full bg-slate-900 border border-slate-700 rounded-lg px-3.5 py-2.5 text-sm text-white placeholder-slate-600 focus:outline-none brand-focus transition-colors disabled:opacity-50"
            placeholder="you@company.com"
          />
        </div>

        <div>
          <label for="password" class="block text-sm font-medium text-slate-400 mb-1.5">Password</label>
          <div class="flex gap-2">
            <input
              id="password"
              v-model="password"
              type="password"
              :required="!passkeyMode"
              autocomplete="current-password"
              :disabled="loading || passkeyMode"
              class="flex-1 bg-slate-900 border border-slate-700 rounded-lg px-3.5 py-2.5 text-sm text-white placeholder-slate-600 focus:outline-none brand-focus transition-colors disabled:opacity-50"
              placeholder="Password"
            />
            <button
              v-if="email && passkeyAvailable"
              type="button"
              @click="passkeyLogin"
              :disabled="loading"
              class="flex items-center gap-1.5 px-3 py-2.5 bg-slate-800 hover:bg-slate-700 border border-slate-700 rounded-lg text-sm text-slate-300 hover:text-white transition-colors disabled:opacity-50 whitespace-nowrap"
            >
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17m3.839 1.132c.645-2.266.99-4.659.99-7.132A8 8 0 008 4.07M3 15.364c.64-1.319 1-2.8 1-4.364 0-1.457.39-2.823 1.07-4" />
              </svg>
              Passkey
            </button>
          </div>
        </div>

        <!-- OTP code (shown after password if account has TOTP enabled) -->
        <div v-if="otpRequired">
          <label for="otp" class="block text-sm font-medium text-slate-400 mb-1.5">Authenticator code</label>
          <input
            id="otp"
            v-model="otpCode"
            type="text"
            inputmode="numeric"
            pattern="[0-9]*"
            autocomplete="one-time-code"
            maxlength="6"
            required
            autofocus
            :disabled="loading"
            class="w-full bg-slate-900 border border-slate-700 rounded-lg px-3.5 py-2.5 text-sm text-white placeholder-slate-600 focus:outline-none brand-focus transition-colors disabled:opacity-50 font-mono tracking-widest text-center"
            placeholder="6-digit code"
          />
        </div>

        <!-- Error message -->
        <div v-if="error" class="rounded-lg bg-red-500/10 border border-red-500/20 px-3.5 py-2.5 text-sm text-red-400">
          {{ error }}
        </div>

        <!-- Passkey status message -->
        <div v-if="passkeyStatus" class="rounded-lg px-3.5 py-2.5 text-sm brand-info-box">
          {{ passkeyStatus }}
        </div>

        <button
          type="submit"
          :disabled="loading || passkeyMode"
          class="w-full text-white font-medium text-sm rounded-lg px-4 py-2.5 transition-colors disabled:opacity-50 disabled:cursor-not-allowed brand-btn"
        >
          <span v-if="loading && !passkeyMode" class="inline-flex items-center gap-2">
            <svg class="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            Signing in...
          </span>
          <span v-else>Sign in</span>
        </button>
      </form>

      <!-- Forgot password + Signup links -->
      <div class="mt-6 text-center space-y-2">
        <div>
          <router-link to="/forgot-password" class="text-sm brand-text hover:brightness-125 transition-colors">
            Forgot password?
          </router-link>
        </div>
        <div v-if="signupEnabled" class="text-sm text-slate-500">
          Don't have an account?
          <router-link to="/signup" class="brand-text hover:brightness-125 transition-colors">Sign up</router-link>
        </div>
      </div>
      </template>
    </div>
    <div class="fixed bottom-4 left-0 right-0 text-center text-xs text-slate-600">
      <a v-if="privacyUrl" :href="privacyUrl" target="_blank" class="hover:text-slate-400 transition-colors">Privacy Policy</a>
      <span v-if="termsUrl && privacyUrl" class="mx-2">&middot;</span>
      <a v-if="termsUrl" :href="termsUrl" target="_blank" class="hover:text-slate-400 transition-colors">Terms of Service</a>
      <span v-if="showPoweredBy && (termsUrl || privacyUrl)" class="mx-2">&middot;</span>
      <a v-if="showPoweredBy" href="https://isms.sh" target="_blank" rel="noopener" class="hover:text-slate-400 transition-colors">Powered by isms.sh</a>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { login, getApiToken, setApiToken } from '../api'
import api from '../api'
import { orgFromSubdomain, isSubdomainMode, orgEntryURL, canHostSubdomain } from '../composables/useCurrentOrg'

const router = useRouter()
const route = useRoute()

const email = ref('')
const otpCode = ref('')
const otpRequired = ref(false)
const password = ref('')
const error = ref('')
const termsUrl = ref('')
const privacyUrl = ref('')
const loading = ref(false)
const orgName = ref('')
const orgSlug = ref('')
const logoUrl = ref(null)
const logoError = ref(false)
const showPoweredBy = ref(true)
const signupEnabled = ref(false)

// Signup
const oidcProviders = ref([])
const passkeyMode = ref(false)
const passkeyStatus = ref('')
const orgDiscoveryInput = ref('')
const orgDiscoveryError = ref('')
const passkeyAvailable = computed(() => {
  return typeof window !== 'undefined' && window.PublicKeyCredential !== undefined
})

function bufferToBase64url(buffer) {
  const bytes = new Uint8Array(buffer)
  let str = ''
  for (const b of bytes) str += String.fromCharCode(b)
  return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
}

function base64urlToBuffer(base64url) {
  const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/')
  const str = atob(base64)
  const bytes = new Uint8Array(str.length)
  for (let i = 0; i < str.length; i++) bytes[i] = str.charCodeAt(i)
  return bytes.buffer
}

function getOrgSlug() {
  // 1. URL query param (?org=acme)
  const params = new URLSearchParams(window.location.search)
  if (params.get('org')) return params.get('org')

  // 2. Subdomain (acme.isms.sh) — highest priority canonical source
  const subSlug = orgFromSubdomain()
  if (subSlug) return subSlug

  // 3. Route path (/acme/login → acme) — only used on apex/localhost
  const pathParts = window.location.pathname.split('/').filter(Boolean)
  if (pathParts.length >= 2 && pathParts[1] === 'login') return pathParts[0]

  // No localStorage fallback — org context must come from URL/subdomain only.
  // Anything else leaks state between orgs (e.g. visiting isms.sh after being
  // in unidoc.isms.sh would silently brand the apex as "unidoc").
  return ''
}

function goToOrg() {
  const slug = orgDiscoveryInput.value.trim().toLowerCase().replace(/[^a-z0-9-]/g, '')
  if (!slug) return
  orgDiscoveryError.value = ''

  // If the host can serve tenant subdomains (apex like isms.sh or already on a
  // subdomain), hop to <slug>.<apex>/login. Otherwise stay path-based.
  if (canHostSubdomain(window.location.hostname)) {
    window.location.href = orgEntryURL(slug, '/login')
    return
  }
  // Path-based — set slug locally and re-render the login form in org context.
  orgSlug.value = slug
}

async function redirectAfterLogin() {
  // Check for a redirect query param first
  const redirectPath = route.query.redirect
  if (redirectPath && redirectPath !== '/overview' && redirectPath.startsWith('/')) {
    router.push(redirectPath)
    return
  }

  // Helper: build the post-login overview URL for a known org slug. In
  // subdomain mode the URL is already org-scoped, so we just push `/overview`.
  function gotoOverview(slug) {
    if (isSubdomainMode()) {
      // Already on the tenant subdomain — path is the suffix.
      router.push('/overview')
    } else {
      router.push('/' + slug + '/overview')
    }
  }

  // If we logged in via an org subdomain/path, the URL already tells us where
  // to go. Skip the org-list detour entirely.
  if (orgSlug.value) {
    gotoOverview(orgSlug.value)
    return
  }
  // Otherwise consult /me for the user's resolved org.
  try {
    const me = await api.getMe()
    if (me?.organization_slug) {
      gotoOverview(me.organization_slug)
      return
    }
  } catch {
    // Fall through to generic overview
  }
  router.push('/organizations')
}

function oidcButtonClass(name) {
  if (name === 'microsoft' || name === 'azure') {
    return 'bg-[#2F2F2F] hover:bg-[#3b3b3b] border-[#3b3b3b] text-white'
  }
  if (name === 'google') {
    return 'bg-white hover:bg-gray-50 border-gray-300 text-gray-700'
  }
  return 'bg-slate-800 hover:bg-slate-700 border-slate-700 text-slate-200'
}

function oidcLogin(provider) {
  const slug = orgSlug.value
  window.location.href = `/api/v1/auth/oidc/authorize?provider=${encodeURIComponent(provider.provider_name)}&org=${encodeURIComponent(slug)}`
}

async function passkeyLogin() {
  if (!email.value) {
    error.value = 'Enter your email first'
    return
  }
  error.value = ''
  passkeyMode.value = true
  passkeyStatus.value = 'Starting passkey authentication...'
  loading.value = true

  try {
    // Step 1: Begin passkey login
    const options = await api.passkeyLoginBegin(email.value)

    // Convert base64url fields to ArrayBuffer for WebAuthn API
    if (options.publicKey) {
      if (options.publicKey.challenge) {
        options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge)
      }
      if (options.publicKey.allowCredentials) {
        options.publicKey.allowCredentials = options.publicKey.allowCredentials.map(c => ({
          ...c,
          id: base64urlToBuffer(c.id),
        }))
      }
    }

    passkeyStatus.value = 'Waiting for passkey...'

    // Step 2: Get credential from browser
    const credential = await navigator.credentials.get({
      publicKey: options.publicKey,
    })

    passkeyStatus.value = 'Verifying...'

    // Step 3: Encode response for server
    const credentialData = {
      id: credential.id,
      rawId: bufferToBase64url(credential.rawId),
      type: credential.type,
      response: {
        authenticatorData: bufferToBase64url(credential.response.authenticatorData),
        clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
        signature: bufferToBase64url(credential.response.signature),
      },
    }
    if (credential.response.userHandle) {
      credentialData.response.userHandle = bufferToBase64url(credential.response.userHandle)
    }

    // Step 4: Complete passkey login
    const result = await api.passkeyLoginComplete(email.value, credentialData)

    setApiToken(result.token)
    if (result.email) localStorage.setItem('isms_user_email', result.email)
    if (result.name) localStorage.setItem('isms_user_name', result.name)

    await redirectAfterLogin()
  } catch (e) {
    if (e.name === 'NotAllowedError') {
      error.value = 'Passkey authentication was cancelled'
    } else {
      error.value = e.message || 'Passkey authentication failed'
    }
    passkeyMode.value = false
    passkeyStatus.value = ''
  } finally {
    loading.value = false
  }
}

async function handleLogin() {
  error.value = ''
  loading.value = true
  try {
    const res = await login(email.value, password.value, otpCode.value || undefined)
    if (res.otp_required) {
      // Backend wants a TOTP code — show the input and stop here.
      otpRequired.value = true
      return
    }
    await redirectAfterLogin()
  } catch (e) {
    error.value = e.message || 'Login failed'
    // Wrong OTP — let the user try again without re-entering password.
    if (otpRequired.value) otpCode.value = ''
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  // Check for OIDC callback token in URL fragment (preferred, secure)
  // or query string (fallback)
  let callbackToken = null
  if (window.location.hash) {
    const hashParams = new URLSearchParams(window.location.hash.substring(1))
    callbackToken = hashParams.get('token')
  }
  if (!callbackToken) {
    const qp = new URLSearchParams(window.location.search)
    callbackToken = qp.get('token')
  }
  if (callbackToken) {
    setApiToken(callbackToken)
    // Clean URL fragment and redirect
    window.history.replaceState(null, '', window.location.pathname)
    await redirectAfterLogin()
    return
  }

  // Detect org from URL/subdomain BEFORE the auto-login redirect, so that
  // redirectAfterLogin can route an already-authenticated user straight to
  // /:org/overview instead of detouring through /organizations.
  orgSlug.value = getOrgSlug()

  // If already authenticated, redirect to home
  if (getApiToken()) {
    try {
      await api.getMe()
      await redirectAfterLogin()
      return
    } catch {
      // Token is invalid, stay on login
    }
  }

  // Try to load org name, logo, and legal links for branding
  let configOrgSlug = ''
  try {
    const cfg = await api.getConfig()
    if (cfg.organization?.name) orgName.value = cfg.organization.name
    if (cfg.organization_name) orgName.value = cfg.organization_name
    if (cfg.branding?.branding_name) orgName.value = cfg.branding.branding_name
    if (cfg.branding?.branding_color) document.documentElement.style.setProperty('--brand-color', cfg.branding.branding_color)
    logoUrl.value = cfg.branding?.branding_logo || ''
    showPoweredBy.value = cfg.show_powered_by !== false
    signupEnabled.value = cfg.signup_enabled === true
    if (cfg.terms_url) termsUrl.value = cfg.terms_url
    else if (cfg.has_terms) termsUrl.value = '/terms'
    if (cfg.privacy_url) privacyUrl.value = cfg.privacy_url
    else if (cfg.has_privacy) privacyUrl.value = '/privacy'
    if (cfg.organization_slug) configOrgSlug = cfg.organization_slug
  } catch {
    // Config may not be accessible without auth
  }

  // Detect org slug from URL/subdomain or single-org config (never localStorage)
  orgSlug.value = getOrgSlug()
  // Single-org auto-detect: if config returned an org slug and we don't have one, use it
  if (!orgSlug.value && configOrgSlug) {
    orgSlug.value = configOrgSlug
  }
  if (orgSlug.value) {
    try {
      const providers = await api.getOIDCProviders(orgSlug.value)
      oidcProviders.value = Array.isArray(providers) ? providers : (providers?.data || [])
    } catch {
      // OIDC not configured, that's fine
    }
  }
})
</script>

<!-- brand-* auth controls now live globally in src/style.css so every auth
     screen (not just Login) gets them — see #169. -->
