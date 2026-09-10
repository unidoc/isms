<template>
  <!-- Hidden entirely when the deployment ships a single locale: a picker with
       one option is noise, and it would suggest a choice that does not exist.
       The wrapper is inside that condition, and so is the label, because only
       this component knows whether it will render. A caller drawing its own
       label had no way to find out, and left it stranded above nothing
       (#281). Anything a call site wants beside the picker belongs in the slot
       for the same reason.

       Attribute fallthrough still works: when hidden the root is this comment
       plus a `v-if` placeholder, which Vue treats as a single comment root and
       exempts from the extraneous-attrs warning, so a caller's classes land on
       the wrapper when there is a wrapper and are dropped when there is not. -->
  <div v-if="options.length > 1">
    <label v-if="label" :for="selectId" class="block text-xs text-slate-500 mb-1">{{ label }}</label>
    <select :id="selectId" :value="selected" @change="onChange"
      :disabled="saving"
      :aria-label="label ? undefined : $t('common.locale.label')"
      :class="compact
        ? 'bg-transparent border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 hover:text-slate-200 focus:outline-none transition-colors disabled:opacity-50'
        : 'w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500 disabled:opacity-50'">
      <!-- Only offered when signed in: with no account to store it against,
           "follow the org default" is not a state the user can be in. -->
      <option v-if="persist" value="">{{ $t('common.locale.org_default') }}</option>
      <option v-for="l in options" :key="l.tag" :value="l.tag">{{ l.name }}</option>
    </select>
    <slot />
  </div>
</template>

<script setup>
import { computed, ref, useId } from 'vue'
import { useLocale } from '../composables/useLocale.js'
import { renderApiError } from '../composables/useApiError.js'

const props = defineProps({
  // false on login / landing, where there is no session to save against.
  persist: { type: Boolean, default: true },
  compact: { type: Boolean, default: false },
  // The visible label. Absent on the compact pickers, which stand alone and
  // fall back to the aria-label. Passed in rather than decided here so a call
  // site can name the setting in its own terms.
  label: { type: String, default: '' },
})
const emit = defineEmits(['error'])

const { options, preference, active, chooseLocale } = useLocale()
const saving = ref(false)
// Per instance, not a constant: the same picker can be mounted more than once
// in a session, and a duplicated id would point every label at the first one.
const selectId = useId()

// Signed in, the picker reflects the stored preference — including the empty
// value that means "org default". Signed out there is no preference, so it
// shows what is actually being rendered.
const selected = computed(() => (props.persist ? (preference.value ?? '') : active.value))

async function onChange(e) {
  const tag = e.target.value
  saving.value = true
  try {
    await chooseLocale(tag, { persist: props.persist })
    // Clear any message from an earlier failure. Without this a stale error sits
    // next to a locale that visibly did change, which reads as the save having
    // failed again.
    emit('error', '')
  } catch (err) {
    // Put the control back where it was; the locale did not change.
    e.target.value = selected.value
    emit('error', renderApiError(err) || String(err))
  } finally {
    saving.value = false
  }
}
</script>
