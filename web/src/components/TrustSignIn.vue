<template>
  <!-- Where the emailed sign-in link lands (#524). The token is read from the URL
       fragment and removed from the address bar at once; the link is only used
       after the visitor confirms "Sign in as <email>?", so a mail scanner or a
       forwarded preview cannot sign anyone in. -->
  <div class="trust-page min-h-screen bg-slate-50 text-slate-900 flex flex-col items-center justify-center px-4">
    <p v-if="state === 'checking'" class="text-sm text-slate-500" data-trust="checking">{{ $t('trust.signin.checking') }}</p>

    <p v-else-if="state === 'unavailable'" class="text-slate-600 text-center" data-trust="unavailable">{{ $t('trust.public.unavailable') }}</p>

    <div v-else class="w-full max-w-md bg-white border border-slate-200 rounded-xl p-6 space-y-4">
      <div class="text-sm font-semibold trust-brand-text" data-trust="brand">{{ brandName }}</div>

      <template v-if="state === 'invalid'">
        <p class="text-slate-700" data-trust="invalid">{{ $t('trust.signin.invalid') }}</p>
        <button type="button" class="px-4 py-2 rounded-lg text-sm font-medium text-white hover:opacity-90 trust-fill" data-trust="request-new" @click="requestNew">
          {{ $t('trust.signin.request_new') }}
        </button>
      </template>

      <template v-else-if="state === 'domain_blocked'">
        <p class="text-slate-700" data-trust="domain-blocked">{{ $t('common.error.trust_email_domain_blocked', { value: blockedDomain }) }}</p>
        <button type="button" class="px-4 py-2 rounded-lg text-sm font-medium text-white hover:opacity-90 trust-fill" data-trust="request-new" @click="requestNew">
          {{ $t('trust.signin.request_new') }}
        </button>
      </template>

      <template v-else-if="state === 'blocked'">
        <p class="text-slate-700" data-trust="blocked">{{ $t('common.error.trust_visitor_blocked') }}</p>
      </template>

      <template v-else-if="state === 'error'">
        <p class="text-slate-700" data-trust="error">{{ $t('trust.signin.error') }}</p>
        <button type="button" class="px-4 py-2 rounded-lg text-sm font-medium text-white hover:opacity-90 trust-fill" data-trust="retry" @click="start">
          {{ $t('trust.signin.retry') }}
        </button>
      </template>

      <form v-else novalidate class="space-y-4" data-trust="confirm" @submit.prevent="submit">
        <h1 class="text-lg font-semibold" data-trust="confirm-title">{{ $t('trust.signin.confirm_title', { email: visitorEmail }) }}</h1>
        <p class="text-sm text-slate-600">{{ $t('trust.signin.confirm_body', { name: brandName }) }}</p>
        <template v-if="needsProfile">
          <p class="text-xs text-slate-500">{{ $t('trust.signin.profile_hint') }}</p>
          <div>
            <label for="trust-visitor-name" class="block text-sm font-medium text-slate-700">{{ $t('trust.signin.name_label') }}</label>
            <input id="trust-visitor-name" v-model="name" type="text" maxlength="200" autocomplete="name" required
              class="mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900" data-trust="name" />
          </div>
          <div>
            <label for="trust-visitor-company" class="block text-sm font-medium text-slate-700">{{ $t('trust.signin.company_label') }}</label>
            <input id="trust-visitor-company" v-model="company" type="text" maxlength="200" autocomplete="organization" required
              class="mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900" data-trust="company" />
          </div>
        </template>
        <p v-if="formError" class="text-sm text-red-700" role="alert" data-trust="form-error">{{ formError }}</p>
        <button type="submit" class="px-4 py-2 rounded-lg text-sm font-medium text-white hover:opacity-90 trust-fill disabled:opacity-60" data-trust="confirm-submit" :disabled="submitting">
          {{ submitting ? $t('trust.signin.submitting') : $t('trust.signin.submit') }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '../api.js'
import { t } from '../i18n.js'
import { applyConfigLocales } from '../composables/useLocale.js'
import { applyBrandColor } from '../utils/brandColor.js'

const props = defineProps({
  slug: { type: String, default: '' },
  // The trust page to go back to (/acme/trust, or /trust on a tenant host).
  basePath: { type: String, required: true },
})
const emit = defineEmits(['navigate'])
const router = useRouter()
const route = useRoute()

const state = ref('checking') // checking | unavailable | invalid | blocked | domain_blocked | error | confirm
// The domain of an address the organization blocked after the link was sent.
const blockedDomain = ref('')
const brandName = ref('')
const visitorEmail = ref('')
const needsProfile = ref(false)
const name = ref('')
const company = ref('')
const formError = ref('')
const submitting = ref(false)

// Kept in memory only, never stored: the fragment is removed from the URL below.
let token = ''
const TOKEN_RE = /^[0-9a-f]{64}$/

// Read the token from the fragment and strip it from the address bar and the
// history entry before anything else happens (and before any request).
// The key is `visitor_token`, never `token`: that name belongs to the SSO
// handoff that stores a member session token, and the two must not share one.
function takeToken(hash = typeof location !== 'undefined' ? location.hash : '') {
  const value = new URLSearchParams(hash.replace(/^#/, '')).get('visitor_token')
  if (value === null) return ''
  stripFragment()
  return value
}

// The fragment is removed THROUGH the router. A bare history.replaceState would
// leave the router's own state.current (the tokenised path) in the history entry,
// and its next navigation would write that back, so Back would return to the
// token URL. Without a router (a bare mount) it falls back to replaceState.
function stripFragment() {
  if (router && route) router.replace({ path: route.path, query: route.query, hash: '' })
  else history.replaceState(history.state, '', location.pathname + location.search)
}

// Opening a second link in the same tab only changes the fragment: the page
// stays mounted, so it takes the new token and starts again.
function restartWith(next) {
  if (!next) return
  token = next
  formError.value = ''
  name.value = ''
  company.value = ''
  start()
}
// With a router the trigger is the router's own route (its fragment changed),
// so the strip below lands after the router has recorded the navigation; a
// window listener could fire first and the strip would be a no-op. Without a
// router (a bare mount) the window event is all there is.
function onHashChange() { restartWith(takeToken()) }
const stopWatch = route ? watch(() => route.hash, (h) => { if (h) restartWith(takeToken(h)) }) : null

const BRAND_VARS = ['--brand-color', '--brand-text-color', '--brand-fill-color']
let saved = null

async function start() {
  if (!props.slug) {
    state.value = 'unavailable'
    return
  }
  state.value = 'checking'
  let o
  try {
    const [ovr, cfg] = await Promise.all([api.trustOverview(props.slug), api.publicConfig().catch(() => null)])
    o = ovr
    applyBrandColor(o.branding_color)
    if (cfg && cfg.locales) await applyConfigLocales({ locales: cfg.locales, default_locale: o.default_locale })
  } catch (err) {
    state.value = err && err.status === 404 ? 'unavailable' : 'error'
    return
  }
  brandName.value = o.branding_name || ''
  if (!TOKEN_RE.test(token)) {
    state.value = 'invalid'
    return
  }
  try {
    const r = await api.trustInspectLink(props.slug, token)
    visitorEmail.value = r.email
    needsProfile.value = !!r.needs_profile
    state.value = 'confirm'
  } catch (err) {
    state.value = classify(err)
  }
}

function classify(err) {
  if (err && err.code === 'trust_link_invalid') return 'invalid'
  if (err && err.code === 'trust_email_domain_blocked') {
    blockedDomain.value = (err.params && err.params.value) || ''
    return 'domain_blocked'
  }
  if (err && err.code === 'trust_visitor_blocked') return 'blocked'
  return 'error'
}

async function submit() {
  if (submitting.value) return
  formError.value = ''
  const n = name.value.trim()
  const c = company.value.trim()
  if (needsProfile.value && (!n || !c)) {
    formError.value = t('common.error.trust_visitor_profile_required')
    return
  }
  submitting.value = true
  try {
    await api.trustSignIn(props.slug, token, needsProfile.value ? n : undefined, needsProfile.value ? c : undefined)
    emit('navigate', props.basePath)
  } catch (err) {
    if (err && err.code === 'trust_visitor_profile_required') {
      needsProfile.value = true
      formError.value = t('common.error.trust_visitor_profile_required')
    } else if (err && (err.code === 'trust_link_invalid' || err.code === 'trust_visitor_blocked' || err.code === 'trust_email_domain_blocked')) {
      state.value = classify(err)
    } else {
      formError.value = t('trust.signin.error')
    }
  } finally {
    submitting.value = false
  }
}

function requestNew() {
  emit('navigate', `${props.basePath}#sign-in`)
}

onMounted(() => {
  const root = document.documentElement.style
  saved = { bodyBg: document.body.style.background, rootBg: root.background, brand: BRAND_VARS.map((n) => [n, root.getPropertyValue(n)]) }
  document.body.style.background = '#f8fafc'
  root.background = '#f8fafc'
  token = takeToken()
  if (!route) window.addEventListener('hashchange', onHashChange)
  start()
})

onBeforeUnmount(() => {
  window.removeEventListener('hashchange', onHashChange)
  if (stopWatch) stopWatch()
  if (!saved) return
  const root = document.documentElement.style
  document.body.style.background = saved.bodyBg
  root.background = saved.rootBg
  for (const [n, v] of saved.brand) {
    if (v) root.setProperty(n, v)
    else root.removeProperty(n)
  }
})
</script>

<style scoped>
.trust-brand-text {
  color: var(--brand-fill-color, #3472d8);
}
.trust-fill {
  background-color: var(--brand-fill-color, #3472d8);
}
</style>
