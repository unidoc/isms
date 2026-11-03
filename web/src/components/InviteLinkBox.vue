<template>
  <div class="mt-3 space-y-2">
    <div class="flex gap-2">
      <input ref="field" :value="link.invite_url" type="text" readonly @focus="$event.target.select()"
        :aria-label="t('admin.invite.link_label')"
        class="flex-1 min-w-0 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-xs text-white focus:outline-none focus:border-blue-500" />
      <button type="button" @click="copy"
        class="px-3 py-2 bg-slate-700 hover:bg-slate-600 text-white text-xs font-medium rounded-lg transition-colors">
        {{ copied ? t('admin.invite.copied') : t('admin.invite.copy') }}
      </button>
    </div>
    <div v-if="copyFailed" class="text-xs text-amber-400">{{ t('admin.invite.copy_failed') }}</div>
    <p class="text-xs text-slate-400">
      {{ t('admin.invite.link_note', { hours, date: expires, email: link.email }) }}
    </p>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatDate } from '../composables/useFormat.js'

// link: { invite_url, expires_at, email }. The URL is a credential: it lives in
// this component's props only, never in storage, the address bar or the console.
const props = defineProps({
  link: { type: Object, required: true },
  hours: { type: Number, required: true },
})
const { t } = useI18n({ useScope: 'global' })
const expires = computed(() => formatDate(props.link.expires_at, 'datetime'))

const field = ref(null)
const copied = ref(false)
const copyFailed = ref(false)

async function copy() {
  copyFailed.value = false
  try {
    await navigator.clipboard.writeText(field.value.value)
    copied.value = true
    setTimeout(() => { copied.value = false }, 1500)
  } catch {
    // No clipboard access (plain http, permissions): select it for Ctrl+C.
    copied.value = false
    copyFailed.value = true
    field.value?.focus()
    field.value?.select()
  }
}
</script>
