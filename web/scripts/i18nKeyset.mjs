// Writer and reporter for the frozen `en` keyset.
//
// The other two scripts in here are ratchets over a *count*. This one is not a
// ratchet at all: it records the exact set of leaf keys `en` exports, and the
// test asserts equality. The difference is deliberate.
//
// A translator works against a set, not a total. `localeKeyset.test.js` already
// compares each locale *to* `en` — but nothing pinned `en` itself, so a rename
// on master was invisible in CI and surfaced as a conflict in a translator's
// branch days later, on a key they had already translated. The snapshot makes
// every keyset change a reviewable diff in the PR that causes it.
//
// "Frozen" therefore does not mean "no more keys". Features add strings, and
// that is fine: the test fails, `npm run i18n:keyset` regenerates, and the new
// keys show up as additions in the snapshot diff. What the freeze buys is that
// a *removal* or a *rename* can no longer happen without someone seeing a
// deletion in that diff and asking whether a translation depends on it.
//
// There is no `--force`, and no ratchet direction to protect. Unlike the raw-text
// baseline this file has no "right direction": a keyset legitimately grows and
// legitimately shrinks, so a guard here would only ever be noise. Review of the
// diff is the control.
import { readFileSync, writeFileSync } from 'node:fs'
import { relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const SNAPSHOT = fileURLToPath(new URL('../test/keyset.snapshot.json', import.meta.url))

// Flatten to dotted leaf paths. Exported because localeKeyset.test.js walks the
// same bundles for its relative checks and must not do it differently — the
// snapshot and those checks disagreeing is a failure no test would explain.
//
// An empty object counts as a leaf in its own right. No bundle contains one
// today; `common.enum` shipped empty in #217 and was filled in #229, which is
// where this rule came from. It is kept because a group reserved ahead of its
// members is a reasonable thing to commit, and because the equivalent walk in
// internal/isms/i18n/release_gate_test.go has to match this one key for key —
// a divergence there would silently change a locale's measured coverage.
export function leafKeys(obj, prefix = '') {
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

export async function currentKeyset() {
  const m = await import(new URL('../src/locales/en/index.js', import.meta.url))
  return leafKeys(m.default).sort()
}

export function readSnapshot() {
  return JSON.parse(readFileSync(SNAPSHOT, 'utf8'))
}

function write(keys) {
  writeFileSync(SNAPSHOT, `${JSON.stringify(keys, null, 2)}\n`)
  console.log(`wrote ${relative(process.cwd(), SNAPSHOT)}: ${keys.length} keys`)
}

function report(keys) {
  let previous
  try {
    previous = readSnapshot()
  } catch {
    console.log(`no snapshot yet; \`en\` exports ${keys.length} keys`)
    return
  }
  const before = new Set(previous)
  const after = new Set(keys)
  const added = keys.filter((k) => !before.has(k))
  const removed = previous.filter((k) => !after.has(k))
  if (!added.length && !removed.length) {
    console.log(`keyset matches the snapshot: ${keys.length} keys`)
    return
  }
  // Removals first: they are the half that breaks an in-flight translation.
  for (const k of removed) console.log(`- ${k}`)
  for (const k of added) console.log(`+ ${k}`)
  console.log(
    `${removed.length} removed, ${added.length} added ` +
      `(${previous.length} -> ${keys.length}). --write to record.`,
  )
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const keys = await currentKeyset()
  if (process.argv.includes('--write')) write(keys)
  else report(keys)
}
