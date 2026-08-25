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
//
// Exported so tests can register a controllable loader; production code adds a
// static line here rather than mutating it at runtime.
export const loaders = {
  // 'id-ID': () => import('./locales/id-ID/index.js'),
}

// Locales the server says it supports, as {tag, name} — name is the endonym, for
// the picker. Empty until /config is read; loadable() falls back to what is
// bundled so boot works before (and without) that call.
let serverLocales = []

export function setSupportedLocales(locales) {
  // Validate at the boundary rather than leaning on a downstream filter to
  // reject a malformed entry: a settings parser owes its callers a clean set.
  serverLocales = Array.isArray(locales)
    ? locales.filter((l) => l && typeof l.tag === 'string' && l.tag.trim() !== '')
    : []
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
  // FALLBACK is always renderable — it is bundled — so it belongs in the result
  // whether or not the server advertised it. Without this a server list that
  // omits `en` empties the set, and a consumer building a picker from it (the
  // one documented use) would show no options at all.
  const renderable = tags.filter((t) => t !== FALLBACK && Object.hasOwn(loaders, t))
  return [FALLBACK, ...new Set(renderable)]
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

// The shape a tag must have to be eligible for the primary-subtag fallback: a
// language, and at most one region. Mirrors the restriction in the Go
// Canonical() — the fallback answers "any region of this language will do", and
// a tag naming a script, variant or extension is not asking that. Deliberately
// stricter than BCP 47: 'id-Latn-ID-x-junk' is perfectly well-formed, and that
// is exactly the input that must not silently reduce to 'id'. The 4-alpha
// primary subtag is reserved and excluded, as it is server-side.
const SIMPLE_TAG = /^[a-z]{2,3}(-[a-z0-9]{1,8})?$|^[a-z]{5,8}(-[a-z0-9]{1,8})?$/

// Region-tolerant match of a requested tag against what this build can render:
// an exact hit first, then the same primary subtag ('id' -> 'id-ID', 'pt-BR' ->
// 'pt'). Returns null when nothing matches, so callers can fall through the
// precedence chain rather than being handed a wrong locale.
//
// navigator.languages and a stored tag are uncontrolled input, so the fallback
// is gated on SIMPLE_TAG: an over-specified or malformed tag falls through to
// the next signal instead of being answered with a guess.
export function negotiate(tag, available = loadableLocales()) {
  if (!tag || typeof tag !== 'string') return null
  // BCP 47 uses '-', but Unix locale strings and some headers use '_' (id_ID).
  const want = tag.trim().toLowerCase().replaceAll('_', '-')
  if (!want) return null
  const exact = available.find((a) => a.toLowerCase() === want)
  if (exact) return exact
  if (!SIMPLE_TAG.test(want)) return null
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

// Monotonic ticket for setLocale calls. Applying a locale can await a chunk
// import, and boot does not await setLocale — so a slow first load can resolve
// after a later, higher-precedence call (an explicit pick, or /me arriving) has
// already applied. Whoever took the newest ticket wins; an older continuation
// discards itself rather than clobbering global state out of order.
let applySeq = 0

// Apply a locale: load its chunk if needed, switch vue-i18n, and keep <html lang>
// in sync so screen readers, spellcheckers and :lang() CSS follow. An unsupported
// or unloadable tag degrades to FALLBACK instead of throwing — a stale stored
// value must not brick the app.
//
// `persist` writes the result to localStorage, and defaults to off: only an
// explicit choice by this user on this device is a preference. Boot and the
// /config- or /me-driven applies pass nothing, so an automatically negotiated
// value (browser order, org default, fallback) never hardens into stored state
// that would then outrank the org default on the next visit.
//
// Returns the applied tag, or null when a newer call superseded this one.
export async function setLocale(tag, { persist = false } = {}) {
  const seq = ++applySeq
  let locale = negotiate(tag) ?? FALLBACK
  if (locale !== FALLBACK && !i18n.global.availableLocales.includes(locale)) {
    try {
      const m = await loaders[locale]()
      if (seq !== applySeq) return null // superseded while the chunk was in flight
      i18n.global.setLocaleMessage(locale, m.default)
    } catch {
      if (seq !== applySeq) return null
      locale = FALLBACK // chunk failed to load (offline, bad deploy)
    }
  }
  i18n.global.locale.value = locale
  if (typeof document !== 'undefined') document.documentElement.lang = locale
  if (persist) {
    try {
      if (typeof localStorage !== 'undefined') localStorage.setItem(STORAGE_KEY, locale)
    } catch {
      /* storage unavailable; the DB preference is the durable copy anyway */
    }
  }
  return locale
}
