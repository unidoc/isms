// The keyset contract from docs/i18n.md step 4, enforced as a test so it runs in
// CI with everything else rather than needing its own workflow.
//
//   missing keys  -> warning. fallbackLocale renders them in English, and a
//                    translation lagging a few keys must not block an unrelated PR.
//   extra keys    -> failure. A key with no `en` counterpart is a typo or a
//                    stale key: nothing will ever read it.
//
// Also enforced: every locale directory is loadable through the same
// index.js-exports-areas shape, since that is what the lazy loaders import.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { FALLBACK } from '../src/i18n.js'

const LOCALES_DIR = fileURLToPath(new URL('../src/locales/', import.meta.url))

function localeDirs() {
  return readdirSync(LOCALES_DIR).filter((e) => statSync(join(LOCALES_DIR, e)).isDirectory())
}

// Flatten to dotted leaf paths. An empty object is a leaf in its own right:
// `common.enum` is a reserved-but-unfilled group in `en`, and a locale that
// fills it is adding keys under it, not diverging from the contract.
function leafKeys(obj, prefix = '') {
  const out = []
  for (const [k, v] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v) && Object.keys(v).length > 0) {
      out.push(...leafKeys(v, path))
    } else {
      out.push(path)
    }
  }
  return out
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
