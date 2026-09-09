// The jurisdiction picker's commit timing, which is the one part of the region
// conversion that is a behaviour change rather than a rendering change.
//
// The picker used to bind its input straight to the form field
// (`v-model="editForm.jurisdiction"`), so every keystroke was already saved.
// That stopped being possible once the stored value and the visible text
// diverged — the column holds `IS`, the box shows "Iceland" — so a commit step
// exists, and this file is about the window that step opened.
//
// This imports `useJurisdictionPicker` itself. The first version of this file
// transplanted the logic out of Legal.vue by hand, following the convention in
// loginOrgSlug.test.js and router.test.js — and a PR review pointed out that a
// transplant cannot catch the regression the file exists for: moving the commit
// back inside the timeout would leave every assertion green. Extracting the
// composable was the fix.
//
// One gap remains and cannot be closed here. The synchronous commit is only safe
// because the option buttons in Legal.vue carry `@mousedown.prevent`, which
// keeps focus on the input so blur never fires from a pick. That is template
// behaviour; nothing in this file can assert it, and the composable's doc
// comment states it as a caller contract instead.
import test from 'node:test'
import assert from 'node:assert/strict'
import { nextTick, ref } from 'vue'
import { useJurisdictionPicker } from '../src/composables/useJurisdictionPicker.js'
import { i18n } from '../src/i18n.js'

i18n.global.mergeLocaleMessage('en', {
  common: { region: { global: 'Global', eu: 'EU', eea: 'EEA', apac: 'APAC' } },
})

function picker(initial = 'EU') {
  const form = ref({ jurisdiction: initial })
  const p = useJurisdictionPicker(form)
  p.seed(initial)
  return { form, ...p }
}

test('seeding shows the label for the stored value', () => {
  assert.equal(picker('IS').query.value, 'Iceland')
  assert.equal(picker('EU').query.value, 'EU')
  // An unrecognised legacy value shows itself, which is what makes it editable
  // rather than mysterious.
  assert.equal(picker('Germany/France').query.value, 'Germany/France')
})

test('picking an option stores the code and shows the label', () => {
  const p = picker()
  p.query.value = 'Iceland'
  p.pickHighlighted()
  assert.equal(p.form.value.jurisdiction, 'IS')
  assert.equal(p.query.value, 'Iceland')
  assert.equal(p.open.value, false)
})

test('blur commits synchronously, so a click on Save cannot outrun it', () => {
  // THE regression this file exists for, and the reason it had to import the
  // real composable. A click runs mousedown -> blur -> mouseup -> click. If the
  // commit sat inside blur()'s 200ms timeout, the save handler would read the
  // form ~195ms before the commit ran and would persist the OLD value, silently
  // discarding what was typed. So the assertion is specifically that the form is
  // correct with no timers run.
  const p = picker('EU')
  p.query.value = 'Iceland'
  p.blur()
  assert.equal(p.form.value.jurisdiction, 'IS', 'commit was deferred — a Save click would lose it')
})

test('free text that matches nothing is stored verbatim, not discarded', () => {
  // The column has no CHECK, plenty of real jurisdictions are not countries,
  // and silently snapping a manager's input to a neighbour is worse than
  // keeping it.
  const p = picker('EU')
  p.query.value = 'Germany/France'
  p.blur()
  assert.equal(p.form.value.jurisdiction, 'Germany/France')
})

test('a typed label or code resolves to the code, case-insensitively', () => {
  for (const [typed, want] of [
    ['Iceland', 'IS'],
    ['iceland', 'IS'],
    ['IS', 'IS'],
    ['is', 'IS'], // someone typing the code in lower case means the code
    ['United Kingdom', 'GB'],
    ['Czechia', 'CZ'],
  ]) {
    const p = picker('EU')
    p.query.value = typed
    p.blur()
    assert.equal(p.form.value.jurisdiction, want, `typed ${typed}`)
  }
})

test('a blur after picking an option is a no-op, not a corruption', () => {
  const p = picker('EU')
  p.pick({ code: 'IS', label: 'Iceland' })
  p.blur()
  assert.equal(p.form.value.jurisdiction, 'IS')
})

test('an empty box clears the field rather than resurrecting the old code', () => {
  const p = picker('IS')
  p.query.value = ''
  p.blur()
  assert.equal(p.form.value.jurisdiction, '')
})

test('the four regions stay reachable without typing', () => {
  // The dropdown renders only the first 15 matches. Sorting the sentinels in
  // with the countries pushed EU — the column's own default — to 60th and out of
  // reach. Order, not membership: every other test here asserts values and none
  // of them caught it, which is why it was found by opening the picker instead.
  const p = picker()
  p.query.value = ''
  assert.deepEqual(
    p.filtered.value.slice(0, 4).map(o => o.code),
    ['Global', 'EU', 'EEA', 'APAC'],
  )
  const labels = p.filtered.value.slice(4).map(o => o.label)
  assert.deepEqual(labels, [...labels].sort((a, b) => a.localeCompare(b, 'en')))
})

test('search matches the visible label and the stored code', () => {
  const p = picker()
  p.query.value = 'iceland'
  assert.ok(p.filtered.value.some(o => o.code === 'IS'))
  // Codes are what the API, the CLI and every existing test use, so someone who
  // knows the row says IS should not have to recall what we render it as.
  p.query.value = 'IS'
  assert.ok(p.filtered.value.some(o => o.code === 'IS'))
})

test('keyboard navigation stays inside the visible list', async () => {
  const p = picker()
  p.query.value = ''
  p.moveUp()
  assert.equal(p.index.value, 0, 'must not go negative')
  for (let i = 0; i < 40; i++) p.moveDown()
  assert.equal(p.index.value, p.filtered.value.length - 1, 'must not run past the last option')
  // Retyping resets the highlight, or Enter would take an option the user can no
  // longer see. `await nextTick()` because the reset is a default-flush watcher,
  // which is how it already shipped — in the browser the reset lands long before
  // the keypress that would read it, since those are separate events. Asserting
  // it synchronously would be asserting something the app never relied on.
  p.query.value = 'ice'
  await nextTick()
  assert.equal(p.index.value, 0)
})
