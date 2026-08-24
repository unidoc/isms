// vue-i18n wiring and the locale resolution chain.
//
// Runtime-only: messages are plain JSON imports, so no @intlify Vite plugin is
// needed. The plugin only buys precompiled messages and <i18n> custom blocks;
// neither is worth a build-time dependency at this stage.
//
// The single source of truth for which locales exist is the Go package
// internal/isms/i18n, surfaced on GET /config as `locales`. This module is
// seeded from that response (see setSupportedLocales) rather than hardcoding a
// list, so the two cannot drift. FALLBACK stays a constant because the fallback
// bundle has to be known at build time — it is the one locale always resident.
import { createI18n } from 'vue-i18n'
import en from './locales/en/index.js'

export const FALLBACK = 'en'

export const STORAGE_KEY = 'isms_locale'

// Lazy loaders for every non-fallback locale. `en` is deliberately absent: it
// is bundled. A locale the server advertises but that has no loader here cannot
// be rendered, which is why applicability is checked against this map and not
// against the server list alone.
const loaders = {
  'id-ID': () => import('./locales/id-ID/index.js'),
}

// Locales the server says it supports, as {tag, name} — name is the endonym, for
// the picker. Empty until /config is read; loadable() falls back to what is
// bundled so boot works before (and without) that call.
let serverLocales = []

export function setSupportedLocales(locales) {
  serverLocales = Array.isArray(locales) ? locales.filter((l) => l && l.tag) : []
}

export function supportedLocales() {
  return serverLocales
}

// The tags this build can actually render: the fallback, plus every server tag
// with a loader. A tag advertised by a newer server than this bundle is skipped
// rather than selected and left rendering raw keys.
export function loadableLocales() {
  const advertised = serverLocales.map((l) => l.tag)
  const tags = advertised.length ? advertised : [FALLBACK, ...Object.keys(loaders)]
  return tags.filter((t) => t === FALLBACK || Object.hasOwn(loaders, t))
}

export const i18n = createI18n({
  legacy: false, // Composition API; the repo has no Options-API i18n usage
  globalInjection: true, // $t in templates without per-component setup
  locale: FALLBACK, // the real value is applied by setLocale() during boot
  fallbackLocale: FALLBACK,
  messages: { en },
  missingWarn: import.meta.env?.DEV ?? false,
  fallbackWarn: import.meta.env?.DEV ?? false,
})

// `t` for plain .js modules (api.js, router guards, utils) where useI18n() is
// unavailable because there is no component instance. Same keyspace, so there is
// exactly one mechanism, not two.
export const t = i18n.global.t
export const te = i18n.global.te

// Region-tolerant match of a requested tag against what this build can render:
// an exact hit first, then the same primary subtag ('id' -> 'id-ID', 'pt-BR' ->
// 'pt'). Returns null when nothing matches, so callers can fall through the
// precedence chain rather than being handed a wrong locale.
export function negotiate(tag, available = loadableLocales()) {
  if (!tag || typeof tag !== 'string') return null
  const want = tag.trim().toLowerCase()
  if (!want) return null
  const exact = available.find((a) => a.toLowerCase() === want)
  if (exact) return exact
  const base = want.split('-')[0]
  return available.find((a) => a.toLowerCase().split('-')[0] === base) ?? null
}

// Precedence, highest first (plan 80 §5):
//   1. the user's explicit choice, from the DB (survives a device change)
//   2. localStorage, for pre-login and anonymous pages
//   3. navigator.languages, in the browser's own order of preference
//   4. the organization default
//   5. FALLBACK
export function resolveInitialLocale({
  userLocale = null,
  orgLocale = null,
  stored = readStored(),
  navigatorLocales = typeof navigator === 'undefined' ? [] : (navigator.languages ?? []),
} = {}) {
  const candidates = [userLocale, stored, ...navigatorLocales, orgLocale]
  for (const c of candidates) {
    const hit = negotiate(c)
    if (hit) return hit
  }
  return FALLBACK
}

function readStored() {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage.getItem(STORAGE_KEY)
  } catch {
    return null // Safari private mode and friends throw rather than returning null
  }
}

// Apply a locale: load its chunk if needed, switch vue-i18n, and keep <html lang>
// in sync so screen readers, spellcheckers and :lang() CSS follow. An unsupported
// or unloadable tag degrades to FALLBACK instead of throwing — a stale stored
// value must not brick the app.
export async function setLocale(tag) {
  let locale = negotiate(tag) ?? FALLBACK
  if (locale !== FALLBACK && !i18n.global.availableLocales.includes(locale)) {
    try {
      const m = await loaders[locale]()
      i18n.global.setLocaleMessage(locale, m.default)
    } catch {
      locale = FALLBACK // chunk failed to load (offline, bad deploy)
    }
  }
  i18n.global.locale.value = locale
  if (typeof document !== 'undefined') document.documentElement.lang = locale
  try {
    if (typeof localStorage !== 'undefined') localStorage.setItem(STORAGE_KEY, locale)
  } catch {
    /* storage unavailable; the DB preference is the durable copy anyway */
  }
  return locale
}
