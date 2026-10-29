<template>
  <section class="bg-slate-900 border border-slate-800 rounded-xl p-6">
    <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">{{ t('settings.digest.title') }}</h2>
    <label class="flex items-start gap-3 text-sm text-slate-300">
      <input type="checkbox" :checked="enabled === true" :disabled="enabled === null || saving"
        @change="save($event.target)"
        class="mt-0.5 rounded border-slate-600 bg-slate-800" />
      <span>{{ t('settings.digest.label', { org: orgName }) }}</span>
    </label>
    <p class="text-xs text-slate-500 mt-2">{{ t('settings.digest.note') }}</p>
    <div v-if="msg" class="text-xs mt-2" :class="isError ? 'text-red-400' : 'text-emerald-400'">{{ msg }}</div>
  </section>
</template>

<script setup>
// The caller's weekly-digest preference for the current organization (#232).
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '../api.js'
import { renderApiError } from '../composables/useApiError.js'

defineProps({ orgName: { type: String, default: '' } })

const { t } = useI18n()

// null until the current value has loaded: the checkbox stays disabled until
// then, so a failed load is never shown as "off".
const enabled = ref(null)
const saving = ref(false)
const msg = ref('')
const isError = ref(false)

onMounted(async () => {
  try {
    enabled.value = (await api.getMyDigest()).enabled
  } catch (e) {
    msg.value = renderApiError(e)
    isError.value = true
  }
})

async function save(box) {
  const next = box.checked
  saving.value = true
  msg.value = ''
  try {
    enabled.value = (await api.setMyDigest(next)).enabled
    msg.value = t('settings.digest.saved')
    isError.value = false
  } catch (e) {
    // Put the box back to what the server still holds. The ref did not change,
    // so Vue will not repaint it: the element is set directly.
    box.checked = !next
    msg.value = renderApiError(e)
    isError.value = true
  } finally {
    saving.value = false
  }
}
</script>
