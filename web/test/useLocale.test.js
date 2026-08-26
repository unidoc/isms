import test from 'node:test'
import assert from 'node:assert/strict'
import api from '../src/api.js'
import { FALLBACK, STORAGE_KEY, i18n, setLocale, setSupportedLocales } from '../src/i18n.js'
import {
  applyConfigLocales,
  applySessionLocale,
  chooseLocale,
  useLocale,
} from '../src/composables/useLocale.js'

const CONFIG = {
  locales: [
    { tag: 'en', name: 'English' },
    { tag: 'id-ID', name: 'Bahasa Indonesia' },
  ],
  default_locale: 'id-ID',
}

// Node's global `localStorage` exists but throws unless the runtime was started
// with a backing file, so this file installs a working in-memory one. It has to
// be real rather than a write-recording stub: precedence step 2 reads back what
// step 1 wrote, and half these tests assert on that round trip. `node --test`
// gives each file its own process, so this stays local to this file.
const store = new Map()
globalThis.localStorage = {
  getItem: (k) => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: (k) => store.delete(k),
}

// A value left by an earlier test would win precedence step 2 over the org
// default and make these assertions order-dependent. "The user has no stored
// locale" is the state each test means to start from.
function clearStored() {
  store.clear()
}

// Node's `navigator.languages` is ['en-US'], which legitimately outranks the org
// default (precedence step 3 beats step 4). To exercise the org-default path at
// all, the browser signal has to name something this build does not ship.
const realNavigator = globalThis.navigator
function navigatorSays(...languages) {
  Object.defineProperty(globalThis, 'navigator', {
    value: { ...realNavigator, languages },
    configurable: true,
    writable: true,
  })
}
function restoreNavigator() {
  Object.defineProperty(globalThis, 'navigator', {
    value: realNavigator,
    configurable: true,
    writable: true,
  })
}

// The composable is module-level state, shared the way useSession is, so each
// test puts it back.
async function reset() {
  setSupportedLocales([])
  await setLocale(FALLBACK)
  clearStored()
  restoreNavigator()
}

test('applyConfigLocales seeds the picker from the server, not a constant', async (t) => {
  t.after(reset)
  clearStored()
  navigatorSays('ja-JP')
  await applyConfigLocales(CONFIG)
  const { options, orgDefault } = useLocale()
  assert.deepEqual(
    options.value.map((l) => l.name),
    ['English', 'Bahasa Indonesia'],
  )
  assert.equal(orgDefault.value, 'id-ID')
  // Nothing above the org default applies here — no stored value, and the
  // browser asks for a language this build does not ship — so the org default
  // is what renders.
  assert.equal(i18n.global.locale.value, 'id-ID')
})

test('applyConfigLocales tolerates a config call that returned nothing', async (t) => {
  t.after(reset)
  clearStored()
  await applyConfigLocales(null)
  await applyConfigLocales({})
  assert.equal(i18n.global.locale.value, FALLBACK)
})

test('an explicit stored preference outranks the org default', async (t) => {
  t.after(reset)
  await applyConfigLocales(CONFIG)
  await applySessionLocale({ locale: 'en', locale_preference: 'en' })
  const { preference } = useLocale()
  assert.equal(preference.value, 'en')
  assert.equal(i18n.global.locale.value, 'en')
})

test('no preference means follow the org default, and the picker says so', async (t) => {
  t.after(reset)
  clearStored()
  navigatorSays('ja-JP')
  await applyConfigLocales(CONFIG)
  await applySessionLocale({ locale: 'id-ID', locale_preference: null })
  const { preference } = useLocale()
  // null, not 'id-ID' — the difference is what lets the picker show "Organization
  // default" rather than pinning the user to today's default.
  assert.equal(preference.value, null)
  assert.equal(i18n.global.locale.value, 'id-ID')
})

test('choosing a locale persists only the locale field', async (t) => {
  t.after(reset)
  const calls = []
  const original = api.putJSON
  api.putJSON = async (path, body) => {
    calls.push([path, body])
    return {}
  }
  t.after(() => {
    api.putJSON = original
  })

  await applyConfigLocales(CONFIG)
  await chooseLocale('en')
  // `name` must be absent, not echoed back: sending it would race a concurrent
  // rename and silently revert it.
  assert.deepEqual(calls, [['/api/v1/auth/profile', { locale: 'en' }]])
  assert.equal(useLocale().preference.value, 'en')
  assert.equal(i18n.global.locale.value, 'en')
})

test('clearing the choice returns to the org default, not the cleared value', async (t) => {
  t.after(reset)
  const original = api.putJSON
  api.putJSON = async () => ({})
  t.after(() => {
    api.putJSON = original
  })

  await applyConfigLocales(CONFIG)
  await chooseLocale('en')
  await chooseLocale('')
  // localStorage still holds 'en' at this point, so re-running the precedence
  // chain would win the cleared choice straight back. The org default has to be
  // applied directly.
  assert.equal(useLocale().preference.value, null)
  assert.equal(i18n.global.locale.value, 'id-ID')
})

test('the pre-login picker changes the locale without calling the API', async (t) => {
  t.after(reset)
  const original = api.putJSON
  api.putJSON = async () => {
    throw new Error('must not be called before login')
  }
  t.after(() => {
    api.putJSON = original
  })

  await applyConfigLocales(CONFIG)
  await chooseLocale('en', { persist: false })
  assert.equal(i18n.global.locale.value, 'en')
})

test("the browser's language outranks the org default", async (t) => {
  t.after(reset)
  clearStored()
  navigatorSays('id', 'en-US')
  // A user whose browser asks for Indonesian gets Indonesian even though the org
  // defaults to English — a deliberate browser setting is a better signal than
  // an org-wide default the user never saw. A bare 'id' still resolves to the
  // 'id-ID' bundle.
  await applyConfigLocales({ ...CONFIG, default_locale: 'en' })
  assert.equal(i18n.global.locale.value, 'id-ID')
  // ...but an explicit account preference still beats it.
  await applySessionLocale({ locale: 'en', locale_preference: 'en' })
  assert.equal(i18n.global.locale.value, 'en')
})

// --- Persistence. `setLocale` defaults to `persist: false`, so every write to
// localStorage is a deliberate decision by this module about what counts as a
// preference. These tests pin those decisions, because getting them wrong fails
// silently: the app still renders, just in the wrong language on the next load.

test('an explicit pick is remembered across a reload', async (t) => {
  t.after(reset)
  const original = api.putJSON
  api.putJSON = async () => ({})
  t.after(() => {
    api.putJSON = original
  })

  await applyConfigLocales(CONFIG)
  await chooseLocale('en')
  // Without this the pre-login picker would forget the choice on reload, since
  // there is no account preference to restore it from.
  assert.equal(localStorage.getItem(STORAGE_KEY), 'en')
})

test('a pre-login pick is remembered even though nothing is sent to the server', async (t) => {
  t.after(reset)
  const original = api.putJSON
  api.putJSON = async () => {
    throw new Error('must not be called before login')
  }
  t.after(() => {
    api.putJSON = original
  })

  await applyConfigLocales(CONFIG)
  await chooseLocale('id-ID', { persist: false })
  // `persist: false` is about the API call, not about the device: localStorage
  // is the only place a signed-out choice can live.
  assert.equal(localStorage.getItem(STORAGE_KEY), 'id-ID')
})

test('clearing the choice also forgets the stored one', async (t) => {
  t.after(reset)
  const original = api.putJSON
  api.putJSON = async () => ({})
  t.after(() => {
    api.putJSON = original
  })

  await applyConfigLocales(CONFIG)
  await chooseLocale('en')
  await chooseLocale('')
  // Leaving 'en' behind would let it outrank the org default on the next boot
  // and silently undo the clear. Nor is the org default written in its place:
  // following the default is the absence of a choice, and storing it would
  // recreate the pin the user just removed.
  assert.equal(localStorage.getItem(STORAGE_KEY), null)
})

test('a negotiated locale never hardens into a stored preference', async (t) => {
  t.after(reset)
  clearStored()
  navigatorSays('ja-JP')
  await applyConfigLocales(CONFIG)
  // The org default rendered, but the user never chose it — storing it would
  // pin them to today's default and outrank tomorrow's.
  assert.equal(i18n.global.locale.value, 'id-ID')
  assert.equal(localStorage.getItem(STORAGE_KEY), null)
})

test('the account preference corrects a conflicting stored value', async (t) => {
  t.after(reset)
  localStorage.setItem(STORAGE_KEY, 'en')
  await applyConfigLocales(CONFIG)
  await applySessionLocale({ locale: 'id-ID', locale_preference: 'id-ID' })
  assert.equal(i18n.global.locale.value, 'id-ID')
  // The account is the durable copy and storage is its cache. Leaving 'en'
  // behind would make the pre-login pages of this device disagree with the
  // account for as long as the stale value survived.
  assert.equal(localStorage.getItem(STORAGE_KEY), 'id-ID')
})

test('a locale this build cannot render is never offered', async (t) => {
  t.after(reset)
  clearStored()
  await applyConfigLocales({
    ...CONFIG,
    // A server newer than this bundle: it supports fr-FR, but there is no
    // loader here, so selecting it would render raw message keys.
    locales: [...CONFIG.locales, { tag: 'fr-FR', name: 'Français' }],
  })
  assert.deepEqual(
    useLocale().options.value.map((l) => l.tag),
    ['en', 'id-ID'],
  )
})

test('a failed save changes nothing — not the preference, the locale, or storage', async (t) => {
  t.after(reset)
  const original = api.putJSON
  api.putJSON = async () => {
    throw new Error('locale not supported')
  }
  t.after(() => {
    api.putJSON = original
  })

  await applyConfigLocales(CONFIG)
  await applySessionLocale({ locale: 'en', locale_preference: 'en' })
  await assert.rejects(() => chooseLocale('id-ID'), /locale not supported/)
  // The API call comes first precisely so a rejection leaves every downstream
  // effect unrun: the picker reverts to `preference`, which must still describe
  // what the server actually holds.
  assert.equal(useLocale().preference.value, 'en')
  assert.equal(i18n.global.locale.value, 'en')
  assert.equal(localStorage.getItem(STORAGE_KEY), 'en')
})
