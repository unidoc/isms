// The picker's visibility contract: on a single-locale deployment it must put
// nothing on the page — not the select, and not the label or the error row that
// used to be drawn by its callers.
//
// This is issue #281. Settings.vue rendered `<label>Language</label>` outside the
// component and unconditionally, because `v-if="options.length > 1"` lived on
// the component's own root and no caller could see it. The label was left above
// nothing, and the wrapper it sat in kept its slot in a `space-y-4` column. The
// fix moved both inside that one condition; these tests are what stops a caller
// from drawing its own chrome again.
//
// The condition is reachable even though the product now ships two locales:
// `renderableOptions()` intersects the server's advertised set with the loaders
// in this bundle, so a server ahead of the client narrows it to one.
import test from 'node:test'
import assert from 'node:assert/strict'
// FIRST, before any import that reaches vue: the helper installs the DOM that
// vue/runtime-dom captures at evaluation time. See its header.
import { compileSfc, renderSfc, stripComments } from './support/renderSfc.js'
import { applyConfigLocales } from '../src/composables/useLocale.js'
import { setSupportedLocales } from '../src/i18n.js'

const SFC = new URL('../src/components/LocalePicker.vue', import.meta.url).pathname

const EN = { tag: 'en', name: 'English' }
const ID = { tag: 'id-ID', name: 'Bahasa Indonesia' }

// `options` is a module-level ref shared by every picker, seeded from /config —
// so the fixture is a config payload, as in useLocale.test.js.
async function withLocales(locales) {
  await applyConfigLocales({ locales, default_locale: 'en' })
}

test.after(() => setSupportedLocales([]))

const LocalePicker = await compileSfc(SFC)

test('one locale renders nothing at all, label and slot included', async () => {
  await withLocales([EN])
  const html = await renderSfc(
    LocalePicker,
    { persist: false, label: 'Language' },
    { default: () => 'save failed' },
  )
  // Comments stripped: Vue's SSR anchors and the component's own template
  // comment both survive into the output, so emptiness has to be asked without
  // them. The assertion that matters is that no element and no text made it
  // through — the bug was text ("Language") with nothing to label.
  assert.equal(stripComments(html), '')
  assert.ok(!html.includes('Language'), `label leaked into a hidden picker: ${html}`)
  assert.ok(!html.includes('save failed'), `slot leaked into a hidden picker: ${html}`)
})

test('two locales render the label, the select and the slot together', async () => {
  await withLocales([EN, ID])
  const html = await renderSfc(
    LocalePicker,
    { persist: false, label: 'Language' },
    { default: () => '<span>save failed</span>' },
  )
  assert.match(html, /<select/)
  assert.match(html, /Language/)
  assert.match(html, /save failed/)
  assert.match(html, /Bahasa Indonesia/)
})

test('the label points at the select it labels', async () => {
  await withLocales([EN, ID])
  const html = await renderSfc(LocalePicker, { persist: false, label: 'Language' })
  const forAttr = html.match(/<label[^>]*\bfor="([^"]+)"/)
  assert.ok(forAttr, `no labelled label in: ${html}`)
  assert.match(html, new RegExp(`<select[^>]*\\bid="${forAttr[1]}"`))
  // With a visible label the aria-label would be a second, redundant name.
  assert.ok(!/<select[^>]*aria-label/.test(html), `select kept its aria-label: ${html}`)
})

test('without a label the select keeps an accessible name', async () => {
  await withLocales([EN, ID])
  const html = await renderSfc(LocalePicker, { persist: false, compact: true })
  assert.ok(!html.includes('<label'), `unlabelled picker drew a label: ${html}`)
  assert.match(html, /<select[^>]*aria-label="[^"]+"/)
})

// Landing.vue passes `class="shrink-0"` and Login.vue passes its layout classes
// straight to the component, so fallthrough has to survive the wrapper moving
// inside the `v-if`. It does, and this is the check: when hidden the root is a
// comment, which Vue treats as a single root and does not warn about; when
// visible the classes land on the wrapper.
test('a caller class lands on the wrapper when visible and vanishes when not', async () => {
  await withLocales([EN, ID])
  const shown = await renderSfc(LocalePicker, { persist: false, compact: true, class: 'shrink-0' })
  // Not anchored: the component's own template comment precedes the wrapper.
  assert.match(shown, /<div class="shrink-0"><\/?/)

  await withLocales([EN])
  const hidden = await renderSfc(LocalePicker, { persist: false, compact: true, class: 'shrink-0' })
  assert.equal(stripComments(hidden), '')
})
