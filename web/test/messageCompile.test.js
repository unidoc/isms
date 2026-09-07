// Every message in every locale must COMPILE, not merely exist.
//
// This exists because a key that resolves is not a key that renders.
// `localeKeyset.test.js` compares key sets, and a plain `t()` in a test only
// proves the one key it names. vue-i18n compiles each message lazily, on first
// render, so a message with invalid syntax passes every static check, passes
// `npm run build`, and then throws in the browser the first time its screen is
// drawn.
//
// The instance that prompted this: `auth.login.email_placeholder` was
// "you@company.com". `@` opens vue-i18n's linked-message syntax (`@:other.key`),
// so the message failed to compile with "Invalid linked format" and took the
// whole Login render down with it. It has to be written `you{'@'}company.com`.
// `|` is the other reserved character — it separates plural forms, so a bare
// pipe silently turns one message into several.
//
// Compiling every message in every locale is cheap and total, which is the
// right shape for a failure mode whose cost is a blank screen.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createI18n } from 'vue-i18n'

const LOCALES_DIR = fileURLToPath(new URL('../src/locales/', import.meta.url))

function localeDirs() {
  return readdirSync(LOCALES_DIR).filter((e) => statSync(join(LOCALES_DIR, e)).isDirectory())
}

function leaves(obj, prefix = '') {
  const out = []
  for (const [k, v] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) out.push(...leaves(v, path))
    else if (typeof v === 'string') out.push(path)
  }
  return out
}

test('every message in every locale compiles', async () => {
  const failures = []

  for (const locale of localeDirs()) {
    const messages = (await import(new URL(`../src/locales/${locale}/index.js`, import.meta.url)))
      .default
    // A fresh instance per locale so one bad message cannot be masked by a
    // cached compile from another.
    const i18n = createI18n({ legacy: false, locale, messages: { [locale]: messages } })

    for (const key of leaves(messages)) {
      try {
        i18n.global.t(key)
      } catch (err) {
        failures.push(`${locale}: ${key} — ${err.message.split('\n')[0]}`)
      }
    }
  }

  assert.deepEqual(
    failures,
    [],
    `messages that resolve but do not compile — they throw on first render, ` +
      `taking their whole screen down. Escape a literal "@" as {'@'} and a literal "|" as {'|'}:\n  ` +
      failures.join('\n  '),
  )
})
