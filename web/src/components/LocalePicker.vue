<template>
  <!-- Hidden entirely when the deployment ships a single locale: a picker with
       one option is noise, and it would suggest a choice that does not exist. -->
  <select v-if="options.length > 1" :value="selected" @change="onChange"
    :disabled="saving"
    :aria-label="$t('common.locale.label')"
    :class="compact
      ? 'bg-transparent border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-500 hover:text-slate-300 focus:outline-none transition-colors disabled:opacity-50'
      : 'w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500 disabled:opacity-50'">
    <!-- Only offered when signed in: with no account to store it against,
         "follow the org default" is not a state the user can be in. -->
    <option v-if="persist" value="">{{ $t('common.locale.org_default') }}</option>
    <option v-for="l in options" :key="l.tag" :value="l.tag">{{ l.name }}</option>
  </select>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useLocale } from '../composables/useLocale'

const props = defineProps({
  // false on login / landing, where there is no session to save against.
  persist: { type: Boolean, default: true },
  compact: { type: Boolean, default: false },
})
const emit = defineEmits(['error'])

const { options, preference, active, chooseLocale } = useLocale()
const saving = ref(false)

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
    emit('error', err?.message || String(err))
  } finally {
    saving.value = false
  }
}
</script>
