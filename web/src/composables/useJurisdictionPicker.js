// The jurisdiction combobox: options, filtering, keyboard index, and the commit
// step that moves a resolved region code onto the form.
//
// This lives outside the view because it is the one part of the region-code
// change that is a behaviour change rather than a rendering change, and it has
// to be testable. The input used to be `v-model`'d straight onto
// `editForm.jurisdiction`, which worked only while the stored value and the
// visible text were the same string. They are not any more — the column stores
// `IS`, the box shows "Iceland" — so a commit step exists, and the timing of
// that step is load-bearing enough to need a test that runs the real code
// rather than a copy of it.
//
// TWO CONTRACTS THE CALLER OWNS, because they live in the template and nothing
// here can enforce them:
//
//  1. The option buttons MUST use `@mousedown.prevent`. That is what stops the
//     input losing focus when an option is clicked, which is in turn what makes
//     `blur()` safe to commit synchronously. Drop the modifier and picking an
//     option fires blur first, committing the half-typed query over the pick.
//  2. `blur()` is bound to the input's blur, and `seed()` is called whenever the
//     form is (re)loaded from a row.
import { computed, ref, watch } from 'vue'
import codes from '../data/countries.js'
import { i18n } from '../i18n.js'
import { REGION_SENTINELS, regionLabel } from './useFormat.js'

/**
 * @param {import('vue').Ref<{jurisdiction: string}>} form the edit form; its
 *   `jurisdiction` is the STORED value (a region code, a sentinel, or free
 *   text), never the display label.
 */
export function useJurisdictionPicker(form) {
  // What the user has typed, kept separate from the stored value.
  const query = ref('')
  const open = ref(false)
  const index = ref(0)

  // The stored value the box is currently *showing*, and the label written for
  // it. Together they answer "is the text in this box still the selection we
  // put there, or has the user typed over it?" — which cannot be decided from
  // the text alone, because a label is locale-dependent and the box is not
  // rewritten when the locale changes underneath it.
  //
  // Without this the picker corrupted data with no typing at all. `main.js`
  // calls `setLocale()` WITHOUT awaiting it before mounting, and
  // `applyConfigLocales()` / `applySessionLocale()` re-apply the locale again
  // when `GET /config` and `GET /me` land, so the app is interactive before the
  // locale settles. Open a row holding `IS` while the UI is still English, let
  // `/me` switch it to id-ID, then save without touching anything: the box still
  // said "Iceland", the options now say "Islandia", nothing matched, and the
  // stored region code was overwritten with the free-text string "Iceland".
  //
  // `useConfirm.js` hit the same race and answered it the same way — resolve
  // against the live locale rather than snapshotting. There it was cosmetic;
  // here it changed what is in the database.
  const selectedCode = ref(null)
  const selectedLabel = ref(null)

  // Sentinels pinned first, countries sorted by localized label under them.
  //
  // The pin is not cosmetic. `Global`/`EU`/`EEA`/`APAC` led the old
  // hand-ordered list, `EU` is the column's own default and the most-picked
  // value, and the dropdown shows only the first 15 matches — so sorting the
  // whole list together pushes EU to 60th and makes the three most common
  // values unreachable without typing. Sorting the countries is still right: a
  // hand-ordered list is alphabetical in English only, and this list renders in
  // the reader's language.
  //
  // A computed rather than a module constant: a module-scope array would freeze
  // whichever locale happened to be active when this module first evaluated.
  // The locale comes from the global scope rather than `useI18n()` because a
  // composable has no component instance to read — see docs/i18n.md.
  const options = computed(() => {
    const withLabel = code => ({ code, label: regionLabel(code) })
    const sentinels = codes.filter(code => REGION_SENTINELS.includes(code)).map(withLabel)
    const countries = codes
      .filter(code => !REGION_SENTINELS.includes(code))
      .map(withLabel)
      .sort((a, b) => a.label.localeCompare(b.label, i18n.global.locale.value))
    return [...sentinels, ...countries]
  })

  // Matches on the label a user can see AND on the code, because codes are what
  // the API, the CLI and every existing test use — someone who knows the row
  // says `IS` should not have to remember what we render it as.
  const filtered = computed(() => {
    const q = query.value.trim().toLowerCase()
    return options.value
      .filter(o => !q || o.label.toLowerCase().includes(q) || o.code.toLowerCase().includes(q))
      .slice(0, 15)
  })

  // The single path that puts a selection in the box, so the text and the two
  // tracking refs can never disagree.
  function show(value) {
    selectedCode.value = value
    selectedLabel.value = regionLabel(value)
    query.value = selectedLabel.value
  }

  /** Load the box from a stored value. An unrecognised one shows itself. */
  function seed(value) {
    show(value)
  }

  function pick(option) {
    form.value.jurisdiction = option.code
    show(option.code)
    open.value = false
  }

  /** True while the box still holds the label we wrote, not typed text. */
  function isUntouchedSelection() {
    return selectedCode.value !== null && query.value === selectedLabel.value
  }

  // Free text stays free text. If what was typed matches no option it is stored
  // VERBATIM rather than discarded, snapped to a neighbour, or reformatted — the
  // column has no constraint, plenty of real jurisdictions are not countries, and
  // silently rewriting a manager's input is worse than storing it.
  //
  // The trim is for MATCHING ONLY, and that distinction is the whole point. An
  // earlier version stored the trimmed string, which quietly broke the contract
  // above in a way that needed no typing to trigger: a row already holding
  // " Germany/France " (the CLI and agent suggestions can write it) got rewritten
  // to "Germany/France" the moment someone opened the form to edit the title and
  // saved. A field nobody touched must not change.
  //
  // One deliberate exception: a box holding nothing but whitespace stores the
  // empty string, not the spaces. Whitespace-only is a cleared field rather than
  // a value, and `jurisdiction` is NOT NULL — parking "  " in it would be its own
  // small defect.
  function commit() {
    // Nothing was typed: keep the value we are showing, whatever the locale has
    // done to its label in the meantime. This is the branch that stops an
    // untouched form from rewriting its own field.
    if (isUntouchedSelection()) {
      form.value.jurisdiction = selectedCode.value
      return
    }
    const raw = query.value
    const probe = raw.trim()
    if (!probe) {
      form.value.jurisdiction = ''
      show('')
      return
    }
    const exact = options.value.find(
      o => o.label.toLowerCase() === probe.toLowerCase() || o.code.toUpperCase() === probe.toUpperCase(),
    )
    form.value.jurisdiction = exact ? exact.code : raw
    // Whatever was resolved becomes the new selection, so a later locale change
    // refreshes its label instead of stranding it. For free text this is a
    // no-op: an unrecognised value renders as itself in every locale.
    show(form.value.jurisdiction)
  }

  function blur() {
    // Commit SYNCHRONOUSLY, and only defer closing the dropdown.
    //
    // Not a style preference. Clicking Save runs mousedown -> blur -> mouseup ->
    // click, so a commit inside the timeout below would land ~195ms AFTER the
    // save handler has already read the form: text typed and never picked from
    // the list would be silently dropped, with no dirty-edit warning either.
    //
    // Safe only because of contract 1 above — the option buttons prevent the
    // mousedown default, so blur never fires from picking. A blur after a pick
    // is a no-op, since the label `pick()` wrote re-resolves to the same code.
    commit()
    // Still deferred: a mousedown inside the dropdown but not on a button (its
    // padding, the scrollbar) would otherwise close it before the click.
    setTimeout(() => {
      open.value = false
    }, 200)
  }

  function moveDown() {
    index.value = Math.min(index.value + 1, filtered.value.length - 1)
  }

  function moveUp() {
    index.value = Math.max(index.value - 1, 0)
  }

  /** Enter/Tab: take the highlighted option if there is one. */
  function pickHighlighted() {
    if (filtered.value.length) pick(filtered.value[index.value])
  }

  watch(query, () => {
    index.value = 0
  })

  // Re-render the selection's label when the locale settles — but only while the
  // box still holds that label. Genuinely typed text is left alone, because
  // rewriting what someone is in the middle of typing would be its own bug.
  watch(
    () => i18n.global.locale.value,
    () => {
      if (isUntouchedSelection()) show(selectedCode.value)
    },
  )

  return { query, open, index, options, filtered, seed, pick, commit, blur, moveDown, moveUp, pickHighlighted }
}
