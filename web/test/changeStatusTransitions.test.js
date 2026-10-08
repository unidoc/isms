// alip/F4 (#423 review): CHANGE_STATUS_TRANSITIONS in Changes.vue is a hand
// copy of internal/isms/db/changes.go's changeStatusTransitions, and nothing
// enforced that the two stayed equal. If the Go table changes and the Vue one
// doesn't, the edit form's dropdown either offers a move the server now
// refuses (a 409 toast) or hides one the server allows.
//
// This can't import Changes.vue directly — CHANGE_STATUS_TRANSITIONS lives
// inside a <script setup> block, not an exported module — so it scans the
// file as source text instead, the same approach errorRender.test.js already
// uses for .vue files. The extracted object is compared against a literal
// copy of the Go table (the same one internal/isms/db/change_transition_test.go
// pins as `allowed`), so changing either side alone without updating its own
// test fails that language's test.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const CHANGES_VUE_PATH = fileURLToPath(new URL('../src/views/Changes.vue', import.meta.url))

// Mirrors the `allowed` map in internal/isms/db/change_transition_test.go,
// expanded to full from -> [to...] form. Keep in sync with both
// internal/isms/db/changes.go's changeStatusTransitions and the Vue copy.
const GO_TRANSITIONS = {
  proposed: ['approved', 'rejected', 'closed'],
  approved: ['in_progress', 'implemented', 'rejected', 'proposed'],
  in_progress: ['implemented', 'rejected', 'proposed'],
  implemented: ['closed', 'in_progress'],
  rejected: [],
  closed: [],
}

function extractTransitionsTable() {
  const source = readFileSync(CHANGES_VUE_PATH, 'utf8')
  const match = source.match(/const CHANGE_STATUS_TRANSITIONS = (\{[\s\S]*?\n\})\n/)
  assert.ok(match, 'CHANGE_STATUS_TRANSITIONS not found in Changes.vue — did it get renamed or moved?')
  // The literal is plain JS object syntax (unquoted keys), not JSON, so it is
  // evaluated rather than JSON.parse'd. Safe here: the input is this repo's
  // own source file, read at test time, never external or user-supplied.
  return new Function(`return (${match[1]})`)()
}

test('Changes.vue CHANGE_STATUS_TRANSITIONS matches db.changeStatusTransitions', () => {
  const vueTable = extractTransitionsTable()
  assert.deepEqual(
    Object.keys(vueTable).sort(),
    Object.keys(GO_TRANSITIONS).sort(),
    'the set of statuses differs — did db.ChangeStatuses change without updating Changes.vue?',
  )
  for (const from of Object.keys(GO_TRANSITIONS)) {
    assert.deepEqual(
      [...vueTable[from]].sort(),
      [...GO_TRANSITIONS[from]].sort(),
      `CHANGE_STATUS_TRANSITIONS.${from} = ${JSON.stringify(vueTable[from])}, ` +
        `want ${JSON.stringify(GO_TRANSITIONS[from])} (internal/isms/db/changes.go)`,
    )
  }
})
