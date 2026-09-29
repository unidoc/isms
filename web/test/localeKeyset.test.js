// The keyset contract from docs/i18n.md ("Adding a locale" step 5, and "The
// frozen keyset"), enforced as a test so it runs in CI with everything else
// rather than needing its own workflow.
//
//   missing keys  -> warning. fallbackLocale renders them in English, and a
//                    translation lagging a few keys must not block an unrelated PR.
//   extra keys    -> failure. A key with no `en` counterpart is a typo or a
//                    stale key: nothing will ever read it.
//
// Also enforced: every locale directory is loadable through the same
// index.js-exports-areas shape, since that is what the lazy loaders import.
//
// And, since the freeze: `en` itself matches `test/keyset.snapshot.json`. The
// two checks above are *relative* — they ask whether a translation agrees with
// `en` — so before the snapshot existed nothing pinned `en`, and a rename on
// master was invisible in CI until it surfaced as a conflict in a translator's
// branch, on a key they had already translated. See scripts/i18nKeyset.mjs for
// why this one asserts equality rather than ratcheting a count.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { FALLBACK } from '../src/i18n.js'
// leafKeys comes from the snapshot writer rather than being defined twice. It
// used to live here, and the freeze is exactly what made a second copy
// dangerous: the relative checks below and the snapshot check must walk the
// bundle identically, or `en` can satisfy the snapshot while a locale is
// measured against a different key set, and the numbers disagree with no test
// to say why.
import { currentKeyset, leafKeys, readSnapshot } from '../scripts/i18nKeyset.mjs'

const LOCALES_DIR = fileURLToPath(new URL('../src/locales/', import.meta.url))

function localeDirs() {
  return readdirSync(LOCALES_DIR).filter((e) => statSync(join(LOCALES_DIR, e)).isDirectory())
}

async function load(locale) {
  const m = await import(new URL(`../src/locales/${locale}/index.js`, import.meta.url))
  return m.default
}

test('every locale directory exports an area map', async () => {
  for (const locale of localeDirs()) {
    const messages = await load(locale)
    assert.equal(typeof messages, 'object', `${locale}/index.js must default-export an object`)
    assert.ok(Object.keys(messages).length > 0, `${locale} exports no areas`)
  }
})

test('no locale carries a key absent from the fallback', async () => {
  const reference = new Set(leafKeys(await load(FALLBACK)))
  const others = localeDirs().filter((l) => l !== FALLBACK)

  for (const locale of others) {
    const keys = leafKeys(await load(locale))
    const extra = keys.filter((k) => !reference.has(k))
    assert.deepEqual(extra, [], `${locale} has keys with no ${FALLBACK} counterpart: ${extra.join(', ')}`)

    // Missing keys are reported, not failed: fallbackLocale covers them.
    const missing = [...reference].filter((k) => !keys.includes(k))
    if (missing.length) {
      console.warn(
        `[keyset] ${locale} is missing ${missing.length} of ${reference.size} keys ` +
          `(rendered in ${FALLBACK}): ${missing.slice(0, 10).join(', ')}` +
          (missing.length > 10 ? ', …' : ''),
      )
    }
  }
})

// The named slots a message interpolates: `{email}` gives "email". A quoted
// literal such as `{'@'}` is text, not a slot, and the pattern cannot match it.
// A list slot (`{0}`) counts by its index. This agrees with vue-i18n's own
// parser (@intlify/message-compiler) on every message in every locale. The
// syntax is safe to read this way because messageCompile.test.js already
// fails on any message that doesn't compile. Plural forms (`a | b`) are
// pooled: a language with fewer forms still has to use the same slots.
function slots(message) {
  const names = [...message.matchAll(/\{\s*([A-Za-z_$][\w$]*|\d+)\s*\}/g)].map((m) => m[1])
  return [...new Set(names)].sort()
}

function listSlots(names) {
  return names.length ? names.map((n) => `{${n}}`).join(', ') : 'no placeholders'
}

function valueAt(messages, key) {
  return key.split('.').reduce((node, part) => (node == null ? undefined : node[part]), messages)
}

// A translator who renames `{email}` to `{surel}`, or drops it, gets a message
// that still resolves and still compiles, and renders a blank where the address
// should be. The keyset checks above can't see that, because the key is fine.
// The Go side has the same check for common.error.* only
// (TestPlaceholdersMatchInEveryLocale); this covers the whole UI keyset.
//
// Same policy as the keyset: a key missing from a translation is skipped
// (fallbackLocale renders it in en, slots and all), and an extra key is the
// test above's failure, not this one's. Adding a slot to an en message fails
// here until every translation that has the key uses it too.
test('every translation uses the same placeholders as the fallback', async () => {
  const reference = await load(FALLBACK)
  const referenceKeys = leafKeys(reference)
  const mismatches = []

  for (const locale of localeDirs().filter((l) => l !== FALLBACK)) {
    const messages = await load(locale)
    for (const key of referenceKeys) {
      const want = valueAt(reference, key)
      const got = valueAt(messages, key)
      if (typeof want !== 'string' || typeof got !== 'string') continue
      const wantSlots = slots(want)
      const gotSlots = slots(got)
      if (wantSlots.join() !== gotSlots.join()) {
        mismatches.push(`${locale} ${key}: ${listSlots(gotSlots)}, but ${FALLBACK} has ${listSlots(wantSlots)}`)
      }
    }
  }

  assert.deepEqual(
    mismatches,
    [],
    `translations whose placeholders differ from ${FALLBACK}. A slot the caller doesn't ` +
      `pass renders as an empty string, and a dropped one loses what it named. Use exactly ` +
      `the slots the ${FALLBACK} message has; order doesn't matter:\n  ` +
      mismatches.join('\n  '),
  )
})

// The freeze, as a test rather than a promise.
//
// Failing here is not a defect: adding a key is ordinary feature work. The fix
// is `npm run i18n:keyset`, which records the change so it appears as a diff in
// the PR that caused it. `npm run i18n:keyset-diff` prints the change first,
// removals before additions.
//
// It matters most for the half that is silent everywhere else. A key *missing*
// from a translation only warns above, because fallbackLocale covers it — so a
// rename lands on master green, and the translator discovers it as a merge
// conflict on work already done. That is the failure this pins.
test('en matches the frozen keyset snapshot', async () => {
  const current = await currentKeyset()
  const snapshot = readSnapshot()

  const before = new Set(snapshot)
  const after = new Set(current)
  const removed = snapshot.filter((k) => !after.has(k))
  const added = current.filter((k) => !before.has(k))

  // Reported separately, because the two mean different things: an addition is
  // routine, a removal is a key some translation may already have rendered.
  assert.deepEqual(
    removed,
    [],
    `${removed.length} key(s) removed from en since the freeze. A translation may already ` +
      `render these; confirm that before recording it, then run \`npm run i18n:keyset\`: ` +
      `${removed.slice(0, 10).join(', ')}${removed.length > 10 ? ', …' : ''}`,
  )
  assert.deepEqual(
    added,
    [],
    `${added.length} key(s) added to en. Run \`npm run i18n:keyset\` to record them so the ` +
      `change is reviewable: ${added.slice(0, 10).join(', ')}${added.length > 10 ? ', …' : ''}`,
  )
  assert.equal(current.length, snapshot.length)
})
