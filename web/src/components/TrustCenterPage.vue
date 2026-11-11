<template>
  <!-- The public trust center (#519). A light page whatever the app theme is: it
       is read by visitors with no account, often from a link in a security
       questionnaire. Everything it shows comes from the anonymous trust API, with
       no Authorization header, so a signed-in reader sees what a visitor sees. -->
  <div class="trust-page min-h-screen bg-slate-50 text-slate-900 flex flex-col">
    <div v-if="state === 'loading'" class="flex-1 flex items-center justify-center text-sm text-slate-500" data-trust="loading">
      {{ $t('trust.public.loading') }}
    </div>

    <div v-else-if="state === 'unavailable'" class="flex-1 flex items-center justify-center px-4" data-trust="unavailable">
      <p class="text-slate-600 text-center">{{ $t('trust.public.unavailable') }}</p>
    </div>

    <div v-else-if="state === 'error'" class="flex-1 flex flex-col items-center justify-center gap-3 px-4" data-trust="error">
      <p class="text-slate-600 text-center">{{ $t('trust.public.error_load') }}</p>
      <button type="button" class="px-4 py-2 rounded-lg text-sm font-medium text-white hover:opacity-90 trust-fill" data-trust="retry" @click="load">
        {{ $t('trust.public.retry') }}
      </button>
    </div>

    <template v-else>
      <header class="bg-white border-b border-slate-200" data-trust="page">
        <div class="max-w-4xl mx-auto px-4 py-4 flex items-center gap-3">
          <img v-if="!logoFailed" :src="logoSrc" alt="" class="h-9 w-auto max-w-[8rem] object-contain" @error="logoFailed = true" />
          <div class="min-w-0 flex-1">
            <div class="font-semibold truncate trust-brand-text" data-trust="brand">{{ ov.branding_name }}</div>
            <div class="text-xs text-slate-500">{{ $t('trust.public.heading') }}</div>
          </div>
          <a v-if="hasSession" href="/" class="text-sm font-medium trust-brand-text hover:underline shrink-0" data-trust="back-to-app" @click="go($event, '/')">
            {{ $t('trust.public.back_to_app') }}
          </a>
        </div>
      </header>

      <main class="flex-1 w-full max-w-4xl mx-auto px-4 py-8 space-y-8">
        <!-- Document page -->
        <section v-if="docId" data-trust="document-page">
          <a :href="basePath" class="text-sm font-medium trust-brand-text hover:underline" data-trust="doc-back" @click="go($event, basePath)">
            <svg class="inline w-4 h-4 -mt-0.5" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5M11 6l-6 6 6 6" /></svg>
            {{ $t('trust.public.doc_back') }}
          </a>

          <p v-if="docState === 'loading'" class="mt-6 text-sm text-slate-500">{{ $t('trust.public.loading') }}</p>

          <p v-else-if="docState === 'missing'" class="mt-6 text-slate-600" data-trust="doc-unavailable">
            {{ $t('trust.public.doc_unavailable') }}
          </p>

          <div v-else-if="docState === 'error'" class="mt-6 flex flex-col items-start gap-3">
            <p class="text-slate-600">{{ $t('trust.public.error_load') }}</p>
            <button type="button" class="px-4 py-2 rounded-lg text-sm font-medium text-white hover:opacity-90 trust-fill" @click="loadDocument">
              {{ $t('trust.public.retry') }}
            </button>
          </div>

          <div v-else-if="docState === 'locked'" class="mt-6 bg-white border border-slate-200 rounded-xl p-6" data-trust="locked">
            <h1 class="text-2xl font-semibold">{{ lockedItem.title }}</h1>
            <p class="mt-2 text-sm text-slate-600">{{ $t('trust.public.doc_locked', { access: accessLabel(lockedItem.access_level) }) }}</p>
            <p class="mt-4 text-sm">
              <a v-if="isEmail(ov.security_contact)" :href="'mailto:' + ov.security_contact" class="font-medium trust-brand-text hover:underline">{{ $t('trust.public.action_contact', { contact: ov.security_contact }) }}</a>
              <span v-else-if="ov.security_contact" class="font-medium">{{ $t('trust.public.action_contact', { contact: ov.security_contact }) }}</span>
              <span v-else class="text-slate-600">{{ $t('trust.public.action_on_request') }}</span>
            </p>
          </div>

          <article v-else-if="docState === 'ready'" class="mt-6" data-trust="doc">
            <h1 class="text-2xl font-semibold">{{ doc.title }}</h1>
            <p class="mt-2 text-sm text-slate-600 flex flex-wrap gap-x-4 gap-y-1">
              <span>{{ $t('trust.public.doc_version', { version: doc.version }) }}</span>
              <span>{{ $t('trust.public.doc_approved', { date: fmtDate(doc.approved_at) }) }}</span>
              <span class="min-w-0 truncate" :title="doc.content_hash">{{ $t('trust.public.doc_hash') }}: <code class="text-xs">{{ shortHash(doc.content_hash) }}</code></span>
            </p>
            <div class="trust-prose mt-6" data-trust="doc-body" @click="onContentClick" v-html="docHtml"></div>
          </article>
        </section>

        <!-- Landing page -->
        <template v-else>
          <p v-if="!hasOverview && !documents.length" class="text-slate-600" data-trust="empty">
            {{ $t('trust.public.empty') }}
          </p>

          <section v-if="hasOverview" data-trust="overview">
            <div class="trust-prose" @click="onContentClick" v-html="overviewHtml"></div>
            <p v-if="ov.security_contact" class="mt-4 text-sm text-slate-600">
              {{ $t('trust.public.security_contact') }}:
              <a v-if="isEmail(ov.security_contact)" :href="'mailto:' + ov.security_contact" class="font-medium trust-brand-text hover:underline">{{ ov.security_contact }}</a>
              <span v-else class="font-medium text-slate-900">{{ ov.security_contact }}</span>
            </p>
          </section>

          <section v-if="documents.length" data-trust="documents">
            <h2 class="text-lg font-semibold">{{ $t('trust.public.documents') }}</h2>

            <div class="mt-3 flex flex-wrap gap-2" role="group" :aria-label="$t('trust.public.filter_label')">
              <button v-for="f in filters" :key="f.key" type="button" :aria-pressed="filter === f.key"
                class="px-3 py-1 rounded-full text-sm border transition-colors"
                :class="filter === f.key ? 'text-white border-transparent trust-fill' : 'bg-white text-slate-700 border-slate-300 hover:border-slate-400'"
                :data-trust-filter="f.key" @click="filter = f.key">
                {{ f.label }}
              </button>
            </div>

            <div class="mt-4 bg-white border border-slate-200 rounded-xl overflow-hidden" role="table">
              <div class="hidden md:grid md:grid-cols-[minmax(0,2fr)_minmax(0,1.4fr)_minmax(0,1.2fr)_minmax(0,1.4fr)] gap-4 px-4 py-2 text-xs font-medium uppercase tracking-wide text-slate-500 border-b border-slate-200 bg-slate-50" role="row">
                <div role="columnheader">{{ $t('trust.public.col_document') }}</div>
                <div role="columnheader">{{ $t('trust.public.col_version') }}</div>
                <div role="columnheader">{{ $t('trust.public.col_access') }}</div>
                <div role="columnheader">{{ $t('trust.public.col_action') }}</div>
              </div>
              <p v-if="!visibleDocuments.length" class="px-4 py-6 text-sm text-slate-500">{{ $t('trust.public.empty_filter') }}</p>
              <div v-for="d in visibleDocuments" :key="d.id" role="row" data-trust="row"
                class="grid gap-1 md:gap-4 md:grid-cols-[minmax(0,2fr)_minmax(0,1.4fr)_minmax(0,1.2fr)_minmax(0,1.4fr)] px-4 py-3 border-b border-slate-100 last:border-b-0 md:items-center">
                <div role="cell" class="font-medium break-words">{{ d.title }}</div>
                <div role="cell" class="text-sm text-slate-600 min-w-0">
                  <span v-if="d.version">v{{ d.version }}</span>
                  <code v-if="d.content_hash" class="ml-2 text-xs text-slate-500" :title="d.content_hash">{{ shortHash(d.content_hash) }}</code>
                </div>
                <div role="cell" class="text-sm text-slate-700 flex items-center gap-1.5">
                  <svg v-if="d.access_level === 'public'" class="w-4 h-4 shrink-0" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" /></svg>
                  <svg v-else-if="d.access_level === 'gated'" class="w-4 h-4 shrink-0" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="5" y="11" width="14" height="9" rx="2" /><path d="M8 11V8a4 4 0 0 1 8 0v3" /></svg>
                  <svg v-else class="w-4 h-4 shrink-0" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="8" cy="15" r="4" /><path d="M11 12l9-9M16 7l3 3" /></svg>
                  <span>{{ accessLabel(d.access_level) }}</span>
                </div>
                <div role="cell" class="text-sm min-w-0 break-words">
                  <a v-if="d.access_level === 'public'" :href="basePath + '/documents/' + d.id" class="font-medium trust-brand-text hover:underline" data-trust="view"
                    @click="go($event, basePath + '/documents/' + d.id)">{{ $t('trust.public.action_view') }}</a>
                  <a v-else-if="isEmail(ov.security_contact)" :href="'mailto:' + ov.security_contact" class="font-medium trust-brand-text hover:underline" data-trust="contact">{{ $t('trust.public.action_contact', { contact: ov.security_contact }) }}</a>
                  <span v-else-if="ov.security_contact" class="text-slate-700" data-trust="contact">{{ $t('trust.public.action_contact', { contact: ov.security_contact }) }}</span>
                  <span v-else class="text-slate-500" data-trust="on-request">{{ $t('trust.public.action_on_request') }}</span>
                </div>
              </div>
            </div>
            <p class="mt-3 text-xs text-slate-500">{{ $t('trust.public.hash_note') }}</p>
          </section>
        </template>
      </main>

      <footer class="border-t border-slate-200 bg-white">
        <div class="max-w-4xl mx-auto px-4 py-4 flex items-center justify-between gap-3 text-xs text-slate-500">
          <LocalePicker :persist="false" compact light />
          <a v-if="ov.show_powered_by" href="https://isms.sh" target="_blank" rel="noopener" class="hover:text-slate-700" data-trust="powered-by">{{ $t('trust.public.powered_by') }}</a>
        </div>
      </footer>
    </template>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch, watchEffect } from 'vue'
import DOMPurify from 'dompurify'
import api, { getApiToken } from '../api.js'
import { i18n, t } from '../i18n.js'
import { applyConfigLocales } from '../composables/useLocale.js'
import { applyBrandColor } from '../utils/brandColor.js'
import { compareTrustItems } from '../utils/trustOrder.js'
import LocalePicker from './LocalePicker.vue'

const props = defineProps({
  slug: { type: String, default: '' },
  // Where this page lives (/acme/trust, or /trust on a tenant host).
  basePath: { type: String, required: true },
  // The public id of the document page to show, when on one.
  docId: { type: String, default: '' },
})
// Navigation is the parent's job: the component renders real hrefs and hands a
// plain click up, so it needs no router to be mounted.
const emit = defineEmits(['navigate'])

const state = ref('loading') // loading | ready | unavailable | error
const ov = ref({})
const items = ref([])
const logoFailed = ref(false)
const filter = ref('all')
const hasSession = ref(false)

const docState = ref('loading') // loading | ready | locked | missing | error
const doc = ref({})
const docHtml = ref('')

const logoSrc = computed(() => `/branding/logo?org=${encodeURIComponent(props.slug)}`)

// Visitors never see anything but a trust API answer; the server's HTML is
// still sanitised here, because this is the last stop before v-html.
function sanitize(html) {
  const hook = (node) => {
    if (node.tagName === 'A' && node.getAttribute('href') && /^https?:/i.test(node.getAttribute('href'))) {
      node.setAttribute('target', '_blank')
      node.setAttribute('rel', 'noopener noreferrer')
    }
  }
  DOMPurify.addHook('afterSanitizeAttributes', hook)
  try {
    return DOMPurify.sanitize(html || '', { ADD_ATTR: ['data-trust-document'] })
  } finally {
    DOMPurify.removeHook('afterSanitizeAttributes', hook)
  }
}

const overviewHtml = computed(() => (ov.value.overview ? sanitize(ov.value.overview.html) : ''))
const hasOverview = computed(() => !!ov.value.overview && overviewHtml.value.trim() !== '')

const documents = computed(() =>
  items.value
    .filter((i) => i.kind === 'document')
    .sort(compareTrustItems),
)
const filterLevel = { public: 'public', gated: 'gated', restricted: 'restricted' }
const visibleDocuments = computed(() =>
  filter.value === 'all' ? documents.value : documents.value.filter((d) => d.access_level === filterLevel[filter.value]),
)
const filters = computed(() => {
  const have = (lvl) => documents.value.some((d) => d.access_level === lvl)
  return [
    { key: 'all', label: t('trust.public.filter_all') },
    have('public') && { key: 'public', label: t('trust.public.filter_public') },
    have('gated') && { key: 'gated', label: t('trust.public.filter_gated') },
    have('restricted') && { key: 'restricted', label: t('trust.public.filter_restricted') },
  ].filter(Boolean)
})

function accessLabel(level) {
  return t(`trust.public.access_${level}`)
}
const isEmail = (v) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v || '')
const shortHash = (h) => (h || '').slice(0, 10)
function fmtDate(sec) {
  try {
    return new Intl.DateTimeFormat(i18n.global.locale.value, { dateStyle: 'medium' }).format(new Date(sec * 1000))
  } catch {
    return new Date(sec * 1000).toISOString().slice(0, 10)
  }
}

// Real links are handled in-app only for a plain left click; a modified or
// non-primary click (new tab, new window, download) is left to the browser.
function go(e, path) {
  if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
  e.preventDefault()
  emit('navigate', path)
}

// A link inside server HTML to another published document has no href (the
// server cannot know the hosting mode): route it from here.
function onContentClick(e) {
  const a = e.target && e.target.closest ? e.target.closest('a[data-trust-document]') : null
  if (!a) return
  e.preventDefault()
  emit('navigate', `${props.basePath}/documents/${a.getAttribute('data-trust-document')}`)
}

const lockedItem = ref({})

async function loadDocument() {
  if (!props.docId) return
  const id = props.docId.toLowerCase()
  const it = documents.value.find((d) => d.id === id)
  if (!it) {
    docState.value = 'missing'
    return
  }
  if (it.access_level !== 'public') {
    lockedItem.value = it
    docState.value = 'locked'
    return
  }
  docState.value = 'loading'
  try {
    const d = await api.trustDocument(props.slug, id)
    doc.value = d
    docHtml.value = sanitize(d.html)
    docState.value = 'ready'
  } catch (err) {
    docState.value = err && err.status === 404 ? 'missing' : 'error'
  }
}

async function load() {
  if (!props.slug) {
    state.value = 'unavailable'
    return
  }
  state.value = 'loading'
  hasSession.value = !!getApiToken()
  try {
    const [o, list, cfg] = await Promise.all([
      api.trustOverview(props.slug),
      api.trustItems(props.slug),
      api.publicConfig().catch(() => null),
    ])
    ov.value = o
    items.value = list
    // Brand first, so nothing flashes unbranded; then the language.
    applyBrandColor(o.branding_color)
    if (cfg && cfg.locales) await applyConfigLocales({ locales: cfg.locales, default_locale: o.default_locale })
    state.value = 'ready'
    await loadDocument()
  } catch (err) {
    state.value = err && err.status === 404 ? 'unavailable' : 'error'
  }
}

watch(() => props.docId, () => { if (state.value === 'ready') loadDocument() })
watch(() => props.slug, load)

// ---- global side effects, undone on unmount -------------------------------
// The page borrows the document title, the favicon, the page background and the
// brand colour variables from the app. Going back to the app must leave all of
// them as they were.
const BRAND_VARS = ['--brand-color', '--brand-text-color', '--brand-fill-color']
let saved = null
let createdFavicon = null

onMounted(() => {
  const root = document.documentElement.style
  const link = document.querySelector('link[rel~="icon"]')
  saved = {
    title: document.title,
    bodyBg: document.body.style.background,
    rootBg: root.background,
    brand: BRAND_VARS.map((n) => [n, root.getPropertyValue(n)]),
    link,
    href: link ? link.getAttribute('href') : null,
  }
  document.body.style.background = '#f8fafc'
  root.background = '#f8fafc'
  load()
})

watchEffect(() => {
  if (state.value !== 'ready' || !saved) return
  const name = ov.value.branding_name || ''
  document.title = ov.value.title || t('trust.public.page_title', { name })
  const href = `/branding/favicon.ico?org=${encodeURIComponent(props.slug)}`
  if (saved.link) saved.link.setAttribute('href', href)
  else if (!createdFavicon) {
    createdFavicon = document.createElement('link')
    createdFavicon.setAttribute('rel', 'icon')
    createdFavicon.setAttribute('href', href)
    document.head.appendChild(createdFavicon)
  }
})

onBeforeUnmount(() => {
  if (!saved) return
  const root = document.documentElement.style
  document.title = saved.title
  document.body.style.background = saved.bodyBg
  root.background = saved.rootBg
  for (const [name, value] of saved.brand) {
    if (value) root.setProperty(name, value)
    else root.removeProperty(name)
  }
  if (saved.link && saved.href !== null) saved.link.setAttribute('href', saved.href)
  if (createdFavicon) createdFavicon.remove()
  createdFavicon = null
})
</script>

<style scoped>
/* Brand colour on a LIGHT page. --brand-text-color is tuned to be read on the
   app's dark surface and would wash out here; --brand-fill-color is darkened
   until white text reaches 4.5:1, which also makes it readable text on white. */
.trust-brand-text {
  color: var(--brand-fill-color, #3472d8);
}
.trust-fill {
  background-color: var(--brand-fill-color, #3472d8);
}
</style>

<style>
/* Server-rendered markdown on the light trust page. The app's .doc-prose is dark
   (code on slate-800), so the page carries its own. */
.trust-prose { line-height: 1.65; color: #0f172a; overflow-wrap: anywhere; }
.trust-prose > :first-child { margin-top: 0; }
.trust-prose h1 { font-size: 1.6rem; font-weight: 700; margin: 1.6em 0 .6em; }
.trust-prose h2 { font-size: 1.3rem; font-weight: 600; margin: 1.5em 0 .5em; }
.trust-prose h3 { font-size: 1.1rem; font-weight: 600; margin: 1.3em 0 .4em; }
.trust-prose h4, .trust-prose h5, .trust-prose h6 { font-weight: 600; margin: 1.2em 0 .3em; }
.trust-prose p { margin: .8em 0; }
.trust-prose ul { list-style: disc; padding-left: 1.5em; margin: .8em 0; }
.trust-prose ol { list-style: decimal; padding-left: 1.5em; margin: .8em 0; }
.trust-prose li { margin: .25em 0; }
.trust-prose li > input[type='checkbox'] { margin-right: .5em; }
.trust-prose ul:has(> li > input[type='checkbox']) { list-style: none; padding-left: .25em; }
.trust-prose a { color: var(--brand-fill-color, #3472d8); text-decoration: underline; cursor: pointer; }
.trust-prose blockquote { border-left: 3px solid #cbd5e1; padding-left: 1em; color: #475569; margin: 1em 0; }
.trust-prose code { background: #f1f5f9; border: 1px solid #e2e8f0; border-radius: .25rem; padding: .05em .35em; font-size: .9em; }
.trust-prose pre { background: #f1f5f9; border: 1px solid #e2e8f0; border-radius: .5rem; padding: .75em 1em; overflow-x: auto; margin: 1em 0; }
.trust-prose pre code { background: none; border: 0; padding: 0; }
.trust-prose hr { border: 0; border-top: 1px solid #e2e8f0; margin: 1.5em 0; }
.trust-prose img { max-width: 100%; height: auto; }
.trust-prose table { display: block; max-width: 100%; overflow-x: auto; border-collapse: collapse; margin: 1em 0; }
.trust-prose th, .trust-prose td { border: 1px solid #cbd5e1; padding: .4em .7em; text-align: left; vertical-align: top; }
.trust-prose th { background: #f1f5f9; font-weight: 600; }
</style>
