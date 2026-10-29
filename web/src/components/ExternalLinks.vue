<template>
  <div class="space-y-2" data-testid="external-links">
    <div class="flex items-center justify-between">
      <span class="text-[10px] font-semibold text-slate-500 uppercase tracking-wider">{{ $t('components.external_links.title') }}</span>
      <button v-if="editable" type="button" @click="toggleAdd"
        class="text-[10px] text-blue-400 hover:text-blue-300 transition-colors">
        {{ isAdding ? $t('common.action.cancel') : $t('common.action.add') }}
      </button>
    </div>

    <!-- One form for both add and edit (edit pre-fills it) -->
    <form v-if="editable && formMode !== null" @submit.prevent="save" class="bg-slate-800/50 border border-slate-700 rounded-lg p-3 space-y-2">
      <label class="block">
        <span class="block text-[10px] text-slate-500 mb-0.5">{{ $t('components.external_links.url') }}</span>
        <input v-model="form.url" type="text" :placeholder="$t('components.external_links.url_placeholder')"
          class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500" />
        <span v-if="errors.url" class="block text-[10px] text-red-400 mt-0.5">{{ errors.url }}</span>
      </label>
      <label class="block">
        <span class="block text-[10px] text-slate-500 mb-0.5">{{ $t('components.external_links.label') }}</span>
        <input v-model="form.label" type="text" :placeholder="$t('components.external_links.label_placeholder')"
          class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500" />
        <span v-if="errors.label" class="block text-[10px] text-red-400 mt-0.5">{{ errors.label }}</span>
      </label>
      <label class="block">
        <span class="block text-[10px] text-slate-500 mb-0.5">{{ $t('components.external_links.type') }}</span>
        <select v-model="form.link_type"
          class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500">
          <option value="" disabled></option>
          <option v-for="ty in types" :key="ty" :value="ty">{{ typeLabel(ty) }}</option>
        </select>
        <span v-if="errors.link_type" class="block text-[10px] text-red-400 mt-0.5">{{ errors.link_type }}</span>
      </label>
      <div class="grid grid-cols-2 gap-2">
        <label class="block">
          <span class="block text-[10px] text-slate-500 mb-0.5">{{ $t('components.external_links.valid_until') }}</span>
          <input v-model="form.valid_until" type="date"
            class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500" />
        </label>
        <label class="block">
          <span class="block text-[10px] text-slate-500 mb-0.5">{{ $t('components.external_links.verified_on') }}</span>
          <input v-model="form.verified_at" type="date"
            class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500" />
        </label>
      </div>
      <div class="flex justify-end gap-2">
        <button type="button" @click="closeForm" class="text-[11px] px-2.5 py-1 rounded bg-slate-700 hover:bg-slate-600 text-slate-200">{{ $t('common.action.cancel') }}</button>
        <button type="submit" :disabled="saving" class="text-[11px] px-2.5 py-1 rounded bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50">{{ $t('common.action.save') }}</button>
      </div>
    </form>

    <div v-if="links.length === 0 && formMode === null" class="text-xs text-slate-600">{{ $t('components.external_links.empty') }}</div>
    <div v-else class="space-y-1.5">
      <div v-for="l in links" :key="l.id" data-testid="external-link-row"
        class="border border-slate-800 rounded-lg px-3 py-2 space-y-1">
        <div class="flex items-center gap-2 flex-wrap">
          <a v-if="safeHref(l.url)" :href="safeHref(l.url)" target="_blank" rel="noopener noreferrer"
            class="text-xs font-medium text-blue-400 hover:text-blue-300 truncate max-w-[260px]">{{ l.label }}</a>
          <span v-else class="text-xs font-medium text-slate-300 truncate max-w-[260px]">{{ l.label }}</span>
          <span class="text-[10px] text-slate-500 truncate">{{ hostOf(l.url) }}</span>
          <span class="text-[9px] font-semibold px-1.5 py-0.5 rounded bg-slate-800 text-slate-400">{{ typeLabel(l.link_type) }}</span>
          <span v-if="l.state" class="text-[9px] font-semibold px-1.5 py-0.5 rounded" :class="stateClass(l.state)">{{ stateLabel(l.state) }}</span>
        </div>
        <div v-if="l.valid_until || l.verified_at" class="text-[10px] text-slate-500 flex flex-wrap gap-x-3">
          <span v-if="l.valid_until">{{ $t('components.external_links.valid_until_row', { date: formatDay(l.valid_until) }) }}</span>
          <span v-if="l.verified_at">{{ l.verified_by
            ? $t('components.external_links.verified_by', { date: formatDay(l.verified_at), name: nameFor(l.verified_by) })
            : $t('components.external_links.verified_on_row', { date: formatDay(l.verified_at) }) }}</span>
        </div>
        <div v-if="editable" class="flex gap-3 text-[10px]">
          <button type="button" @click="openEdit(l)" class="text-blue-400 hover:text-blue-300">{{ $t('common.action.edit') }}</button>
          <button type="button" @click="verify(l)" class="text-emerald-400 hover:text-emerald-300">{{ $t('components.external_links.mark_verified') }}</button>
          <button type="button" @click="remove(l)" class="text-slate-500 hover:text-red-400">{{ $t('common.action.delete') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api.js'
import { useToast } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm.js'
import { useMembers } from '../composables/useMembers.js'
import { enumLabel } from '../composables/useEnumLabel.js'
import { formatDay } from '../composables/useFormat.js'
import { renderApiError } from '../composables/useApiError.js'

const props = defineProps({
  entityType: { type: String, required: true },
  entityId: { type: String, required: true },
  editable: { type: Boolean, default: false },
})

const { t } = useI18n()
const { show: showError } = useToast()
const { confirm: confirmDialog } = useConfirm()
const { nameFor } = useMembers()

const types = ['certificate', 'audit_report', 'penetration_test', 'attestation', 'contract', 'other']

const links = ref([])
// 'add', a link id (editing that row), or null (no form open).
const formMode = ref(null)
const isAdding = computed(() => formMode.value === 'add')
const form = ref(blankForm())
const errors = ref({})
const saving = ref(false)

const typeLabel = v => enumLabel('external_link_type', v)
const stateLabel = v => enumLabel('external_link_state', v)

function blankForm() {
  return { url: '', label: '', link_type: '', valid_until: '', verified_at: '' }
}

// DATE columns arrive as epoch seconds at UTC midnight; <input type=date> wants
// YYYY-MM-DD, read in UTC so the day is the one stored.
function toInputDay(epoch) {
  return epoch ? new Date(epoch * 1000).toISOString().slice(0, 10) : ''
}

// Defence in depth: the server refuses other schemes, but a stored value is
// still never put in an href unless it is http(s).
function safeHref(url) {
  return typeof url === 'string' && /^https?:\/\//i.test(url.trim()) ? url.trim() : null
}

function hostOf(url) {
  try { return new URL(url).host } catch { return '' }
}

function stateClass(state) {
  if (state === 'expired') return 'bg-red-900/40 text-red-300'
  if (state === 'expiring') return 'bg-amber-900/40 text-amber-300'
  return 'bg-emerald-900/40 text-emerald-300'
}

function toggleAdd() {
  if (formMode.value === 'add') return closeForm()
  form.value = blankForm()
  errors.value = {}
  formMode.value = 'add'
}

function openEdit(l) {
  form.value = {
    url: l.url, label: l.label, link_type: l.link_type,
    valid_until: toInputDay(l.valid_until), verified_at: toInputDay(l.verified_at),
  }
  errors.value = {}
  formMode.value = l.id
}

function closeForm() {
  formMode.value = null
  form.value = blankForm()
  errors.value = {}
}

function validate() {
  const e = {}
  const url = form.value.url.trim()
  if (!url) e.url = t('components.external_links.url_required')
  else {
    let ok = false
    try {
      const u = new URL(url)
      ok = (u.protocol === 'http:' || u.protocol === 'https:') && !!u.host
    } catch { /* not a URL */ }
    if (!ok) e.url = t('components.external_links.url_invalid')
  }
  const label = form.value.label.trim()
  if (!label) e.label = t('components.external_links.label_required')
  else if ([...label].length > 200) e.label = t('components.external_links.label_too_long')
  if (!types.includes(form.value.link_type)) e.link_type = t('components.external_links.type_required')
  errors.value = e
  return Object.keys(e).length === 0
}

async function load() {
  try {
    const data = await api.getExternalLinks(props.entityType, props.entityId)
    links.value = data || []
  } catch (e) {
    links.value = []
    showError(t('components.external_links.error_load', { message: renderApiError(e) || t('common.state.unknown') }))
  }
}

async function save() {
  if (!validate()) return
  // PUT is a full replace: every field is sent, an emptied date as null.
  const body = {
    url: form.value.url.trim(),
    label: form.value.label.trim(),
    link_type: form.value.link_type,
    valid_until: form.value.valid_until || null,
    verified_at: form.value.verified_at || null,
  }
  saving.value = true
  try {
    if (formMode.value === 'add') {
      await api.createExternalLink({ entity_type: props.entityType, entity_id: props.entityId, ...body })
    } else {
      await api.updateExternalLink(formMode.value, body)
    }
    closeForm()
    await load()
  } catch (e) {
    showError(t('components.external_links.error_save', { message: renderApiError(e) || t('common.state.unknown') }))
  }
  saving.value = false
}

async function verify(l) {
  try {
    await api.verifyExternalLink(l.id)
    await load()
  } catch (e) {
    showError(t('components.external_links.error_verify', { message: renderApiError(e) || t('common.state.unknown') }))
  }
}

async function remove(l) {
  const ok = await confirmDialog({
    message: t('components.external_links.confirm_remove'),
    variant: 'danger',
    confirmLabel: t('common.action.delete'),
  })
  if (!ok) return
  try {
    await api.deleteExternalLink(l.id)
    await load()
  } catch (e) {
    showError(t('components.external_links.error_remove', { message: renderApiError(e) || t('common.state.unknown') }))
  }
}

onMounted(load)
watch(() => [props.entityType, props.entityId], () => { closeForm(); load() })
</script>
