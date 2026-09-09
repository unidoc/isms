// The jurisdiction picker's commit timing, which is the one part of the region
// conversion that is a behaviour change rather than a rendering change.
//
// The picker used to bind its input straight to the form field
// (`v-model="editForm.jurisdiction"`), so every keystroke was already saved.
// That stopped being possible once the stored value and the visible text
// diverged — the column holds `IS`, the box shows "Iceland" — so the input binds
// a separate query ref and a commit step moves the resolved code onto the form.
// Introducing that step introduced a window in which the two disagree, and this
// file is about that window.
//
// Transplanted verbatim from Legal.vue, following the convention in
// loginOrgSlug.test.js and router.test.js: Legal.vue imports a dozen child
// components and browser globals that do not load under `node --test`. This is
// a COPY, and a divergence between it and the view would not be caught here —
// keep them in step by hand.
import test from 'node:test'
import assert from 'node:assert/strict'
import { computed, ref } from 'vue'
import { regionLabel } from '../src/composables/useFormat.js'
import countries from '../src/data/countries.js'
import { i18n } from '../src/i18n.js'

i18n.global.mergeLocaleMessage('en', {
  common: { region: { global: 'Global', eu: 'EU', eea: 'EEA', apac: 'APAC' } },
})

// --- Transplant of Legal.vue's picker, source: the region-codes change ---
function picker(initial = 'EU') {
  const editForm = ref({ jurisdiction: initial })
  const jurisdictionQuery = ref(regionLabel(initial))
  const showJurisdictionPicker = ref(false)

  const jurisdictionOptions = computed(() =>
    countries.map((code) => ({ code, label: regionLabel(code) })).sort((a, b) => a.label.localeCompare(b.label)),
  )

  const filteredJurisdictions = computed(() => {
    const q = jurisdictionQuery.value.trim().toLowerCase()
    const list = jurisdictionOptions.value.filter(
      (o) => !q || o.label.toLowerCase().includes(q) || o.code.toLowerCase().includes(q),
    )
    return list.slice(0, 15)
  })

  function pickJurisdiction(option) {
    editForm.value.jurisdiction = option.code
    jurisdictionQuery.value = option.label
    showJurisdictionPicker.value = false
  }

  function commitJurisdictionText() {
    const typed = jurisdictionQuery.value.trim()
    const exact = jurisdictionOptions.value.find(
      (o) => o.label.toLowerCase() === typed.toLowerCase() || o.code.toUpperCase() === typed.toUpperCase(),
    )
    editForm.value.jurisdiction = exact ? exact.code : typed
  }

  function hideJurisdictionPicker() {
    commitJurisdictionText()
    setTimeout(() => {
      showJurisdictionPicker.value = false
    }, 200)
  }

  return {
    editForm,
    jurisdictionQuery,
    showJurisdictionPicker,
    filteredJurisdictions,
    pickJurisdiction,
    hideJurisdictionPicker,
  }
}

test('picking an option stores the code and shows the label', () => {
  const p = picker()
  const iceland = p.filteredJurisdictions.value.find((o) => o.code === 'IS') ?? { code: 'IS', label: 'Iceland' }
  p.pickJurisdiction(iceland)
  assert.equal(p.editForm.value.jurisdiction, 'IS')
  assert.equal(p.jurisdictionQuery.value, 'Iceland')
})

test('blur commits synchronously, so a click on Save cannot outrun it', () => {
  // THE regression this file exists for. A click runs mousedown -> blur ->
  // mouseup -> click. If the commit sat inside hideJurisdictionPicker's 200ms
  // timeout, the save handler would read the form ~195ms before the commit ran
  // and would persist the OLD value, silently discarding what was typed. So the
  // assertion is specifically that the form is correct with no timers run.
  const p = picker('EU')
  p.jurisdictionQuery.value = 'Iceland'
  p.hideJurisdictionPicker()
  assert.equal(p.editForm.value.jurisdiction, 'IS', 'commit was deferred — a Save click would lose it')
})

test('free text that matches nothing is stored verbatim, not discarded', () => {
  // The column has no CHECK, plenty of real jurisdictions are not countries,
  // and silently snapping a manager's input to a neighbour is worse than
  // keeping it.
  const p = picker('EU')
  p.jurisdictionQuery.value = 'Germany/France'
  p.hideJurisdictionPicker()
  assert.equal(p.editForm.value.jurisdiction, 'Germany/France')
})

test('a typed label or code resolves to the code, case-insensitively', () => {
  for (const [typed, want] of [
    ['Iceland', 'IS'],
    ['iceland', 'IS'],
    ['IS', 'IS'],
    ['is', 'IS'], // someone typing the code in lower case means the code
    ['United Kingdom', 'GB'],
  ]) {
    const p = picker('EU')
    p.jurisdictionQuery.value = typed
    p.hideJurisdictionPicker()
    assert.equal(p.editForm.value.jurisdiction, want, `typed ${typed}`)
  }
})

test('a blur after picking an option is a no-op, not a corruption', () => {
  // The option buttons use `@mousedown.prevent`, so picking never fires blur —
  // but clicking away afterwards does, and it must re-resolve the label
  // pickJurisdiction() wrote back to the same code.
  const p = picker('EU')
  p.pickJurisdiction({ code: 'IS', label: 'Iceland' })
  p.hideJurisdictionPicker()
  assert.equal(p.editForm.value.jurisdiction, 'IS')
})

test('an empty box clears the field rather than resurrecting the old code', () => {
  const p = picker('IS')
  p.jurisdictionQuery.value = ''
  p.hideJurisdictionPicker()
  assert.equal(p.editForm.value.jurisdiction, '')
})

test('search matches the visible label and the stored code', () => {
  const p = picker()
  p.jurisdictionQuery.value = 'iceland'
  assert.ok(p.filteredJurisdictions.value.some((o) => o.code === 'IS'))
  // Codes are what the API, the CLI and every existing test use, so someone who
  // knows the row says IS should not have to recall what we render it as.
  p.jurisdictionQuery.value = 'IS'
  assert.ok(p.filteredJurisdictions.value.some((o) => o.code === 'IS'))
})
