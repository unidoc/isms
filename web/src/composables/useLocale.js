// Locale selection: the precedence chain applied to real session data, and the
// one place that persists a user's choice.
//
// `i18n.js` owns the mechanism (negotiation, chunk loading, <html lang>). This
// composable owns the policy: which signal wins, and when the server is told.
import { computed, ref } from 'vue'
import api from '../api.js'
import {
  FALLBACK,
  i18n,
  resolveInitialLocale,
  setLocale,
  setSupportedLocales,
  supportedLocales,
} from '../i18n.js'

// Module-level so the picker on the login page and the one in Settings share a
// single source of truth, the way useSession does.
const options = ref([])
const orgDefault = ref(FALLBACK)
// The user's explicit choice. `null` means "following the org default", which is
// what lets the picker show that as a distinct option rather than pretending the
// user picked whatever the org happens to default to today.
const preference = ref(null)

const active = computed(() => i18n.global.locale.value)

// Seed the supported set from GET /config and apply the best pre-login guess.
// Safe to call from every page that already fetches /config; later calls just
// re-apply the same resolution.
export async function applyConfigLocales(cfg) {
  if (!cfg) return
  setSupportedLocales(cfg.locales)
  options.value = supportedLocales()
  if (cfg.default_locale) orgDefault.value = cfg.default_locale
  // Pre-login there is no user preference, so this is localStorage, then the
  // browser's languages, then the org default.
  await setLocale(resolveInitialLocale({ orgLocale: orgDefault.value }))
}

// Apply the locale for an authenticated session, from GET /me.
//
// `locale_preference` is the raw choice and `locale` is what the server already
// resolved (preference, else org default). An explicit choice outranks every
// client-side signal — it was made deliberately and travels with the account.
// Without one, the client-side signals apply, because a browser set to
// Indonesian is a better guess than the org default.
export async function applySessionLocale(me) {
  if (!me) return
  preference.value = me.locale_preference ?? null
  if (preference.value) {
    await setLocale(preference.value)
    return
  }
  await setLocale(resolveInitialLocale({ orgLocale: me.locale || orgDefault.value }))
}

// Change the active locale. `tag` is '' to clear an explicit choice and follow
// the org default again.
//
// `persist` is false on the login and landing pages, where there is no session
// to save against — the choice still survives in localStorage, and is upgraded
// to a stored preference the next time the user saves one while signed in.
export async function chooseLocale(tag, { persist = true } = {}) {
  if (persist) {
    // Send only `locale`. `name` is optional server-side precisely so a
    // locale-only update cannot race a concurrent rename.
    await api.putJSON('/api/v1/auth/profile', { locale: tag })
  }
  preference.value = tag || null
  // Clearing applies the org default directly rather than re-running the
  // precedence chain: localStorage still holds the choice being cleared, and
  // would immediately win it back.
  await setLocale(tag || orgDefault.value)
}

export function useLocale() {
  return { options, orgDefault, preference, active, applyConfigLocales, applySessionLocale, chooseLocale }
}
