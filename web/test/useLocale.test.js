import test from 'node:test'
import assert from 'node:assert/strict'
import api from '../src/api.js'
import { FALLBACK, i18n, setLocale, setSupportedLocales } from '../src/i18n.js'
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

// Node exposes a real localStorage, and setLocale writes to it — so a value left
// by an earlier test would win precedence step 2 over the org default and make
// these assertions order-dependent. Clear it explicitly: "the user has no stored
// locale" is the state each test means to start from.
function clearStored() {
  try {
    localStorage.removeItem('isms_locale')
  } catch {
    /* no storage in this environment; nothing to clear */
  }
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
