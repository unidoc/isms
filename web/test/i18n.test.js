import test from 'node:test'
import assert from 'node:assert/strict'
import {
  FALLBACK,
  i18n,
  loadableLocales,
  negotiate,
  resolveInitialLocale,
  setLocale,
  setSupportedLocales,
  supportedLocales,
} from '../src/i18n.js'

// Every test that touches the supported set restores it, since the module holds
// it as process-wide state.
function withServerLocales(locales, fn) {
  setSupportedLocales(locales)
  try {
    return fn()
  } finally {
    setSupportedLocales([])
  }
}

test('negotiate matches exactly, then region-tolerantly', () => {
  const available = ['en', 'id-ID']
  assert.equal(negotiate('en', available), 'en')
  assert.equal(negotiate('id-ID', available), 'id-ID')
  // Case is not part of a BCP 47 identity: ID-id is the same tag as id-ID.
  assert.equal(negotiate('ID-id', available), 'id-ID')
  // A bare primary subtag resolves to the region variant we ship.
  assert.equal(negotiate('id', available), 'id-ID')
  // ...and so does a different region of the same language: a pt-BR speaker is
  // better served by pt-PT copy than by English.
  assert.equal(negotiate('pt-BR', ['en', 'pt-PT']), 'pt-PT')
})

test('negotiate returns null rather than guessing', () => {
  const available = ['en', 'id-ID']
  assert.equal(negotiate('de', available), null)
  assert.equal(negotiate('', available), null)
  assert.equal(negotiate(null, available), null)
  assert.equal(negotiate(undefined, available), null)
  assert.equal(negotiate(42, available), null)
})

test('loadable locales exclude a server tag this build cannot render', () => {
  // A newer server advertising a locale whose chunk does not exist in this
  // bundle must not become selectable — that renders raw keys.
  withServerLocales([{ tag: 'en' }, { tag: 'fr-FR', name: 'Français' }], () => {
    assert.deepEqual(loadableLocales(), ['en'])
  })
})

test('supported locales survive a malformed /config payload', () => {
  withServerLocales(null, () => assert.deepEqual(supportedLocales(), []))
  withServerLocales([{ name: 'no tag' }, null], () => {
    assert.deepEqual(supportedLocales(), [])
  })
})

test('resolution precedence: user choice beats every weaker signal', () => {
  assert.equal(
    resolveInitialLocale({
      userLocale: 'en',
      orgLocale: 'id-ID',
      stored: 'id-ID',
      navigatorLocales: ['id-ID'],
    }),
    'en',
  )
})

test('resolution falls through unrenderable signals instead of stopping', () => {
  // de-DE, fr and ja-JP are not shipped, so each must be skipped rather than
  // returned; en-GB is the first signal that resolves.
  assert.equal(
    resolveInitialLocale({
      userLocale: 'de-DE',
      stored: 'fr',
      navigatorLocales: ['ja-JP', 'en-GB'],
      orgLocale: 'id-ID',
    }),
    'en',
  )
  // ...and with nothing renderable above it, the org default applies.
  assert.equal(
    resolveInitialLocale({
      userLocale: 'de-DE',
      stored: null,
      navigatorLocales: ['ja-JP'],
      orgLocale: 'id-ID',
    }),
    'id-ID',
  )
})

test('resolution ends at the fallback when nothing matches', () => {
  assert.equal(
    resolveInitialLocale({ stored: null, navigatorLocales: [], orgLocale: null }),
    FALLBACK,
  )
  assert.equal(resolveInitialLocale({}), FALLBACK)
})

test('setLocale degrades an unsupported tag to the fallback', async () => {
  // A stored value for a locale that was dropped from the build must not brick
  // the app: it renders English.
  assert.equal(await setLocale('kl-GL'), FALLBACK)
  assert.equal(i18n.global.locale.value, FALLBACK)
  assert.equal(await setLocale(undefined), FALLBACK)
})

test('the fallback bundle is resident and its keys resolve', () => {
  assert.ok(i18n.global.availableLocales.includes(FALLBACK))
  assert.equal(i18n.global.t('common.action.save'), 'Save')
  assert.equal(i18n.global.te('common.action.save'), true)
  assert.equal(i18n.global.te('common.action.no_such_key'), false)
})

test('each area file is merged under its own filename as the area key', () => {
  // The convention is <area>.<group>.<key> where area IS the filename; a file
  // that is imported but not exported under its own name breaks that silently.
  // Only `common` exists until extraction starts adding areas, so this asserts
  // the shape every area must hold rather than a list that would go stale on
  // every extraction PR.
  const messages = i18n.global.getLocaleMessage(FALLBACK)
  assert.equal(typeof messages.common, 'object')
  for (const [area, groups] of Object.entries(messages)) {
    assert.equal(typeof groups, 'object', `area ${area} must be a group object`)
    // snake_case, mirroring the key convention — `correctiveactions` would slip
    // through a looser check and read wrong next to `common.enum.status`.
    assert.ok(/^[a-z][a-z0-9_]*$/.test(area), `area ${area} must be snake_case`)
  }
})

test('setLocale loads the id-ID chunk and renders it', async () => {
  // The whole point of shipping a second locale in the foundation: prove the
  // resolution chain against something other than the fallback bundle.
  assert.equal(await setLocale('id-ID'), 'id-ID')
  assert.equal(i18n.global.locale.value, 'id-ID')
  assert.equal(i18n.global.t('common.action.save'), 'Simpan')
  assert.equal(i18n.global.t('common.state.loading'), 'Memuat\u2026')
  // A bare 'id' resolves to the region variant we actually ship.
  assert.equal(await setLocale('id'), 'id-ID')
  await setLocale(FALLBACK)
  assert.equal(i18n.global.t('common.action.save'), 'Save')
})

test('a key missing from a translation falls back to English', async () => {
  // A translation lagging the keyset must render English, not a raw key — this
  // is what lets CI warn rather than fail on missing keys.
  await setLocale('id-ID')
  const full = i18n.global.getLocaleMessage('id-ID')
  const { save: _dropped, ...rest } = full.common.action
  i18n.global.setLocaleMessage('id-ID', {
    ...full,
    common: { ...full.common, action: rest },
  })
  assert.equal(i18n.global.t('common.action.save'), 'Save')
  assert.equal(i18n.global.t('common.action.cancel'), 'Batal')
  i18n.global.setLocaleMessage('id-ID', full)
  await setLocale(FALLBACK)
})
