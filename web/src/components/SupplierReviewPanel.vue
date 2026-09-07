<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2">
        <span class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ $t('components.supplier_review.title') }}</span>
        <span v-if="reviews.length > 0" class="px-1.5 py-0.5 rounded-full text-[10px] font-bold bg-slate-800 text-slate-400">{{ reviews.length }}</span>
      </div>
      <button v-if="canWrite" @click="showForm = !showForm"
        class="px-3 py-1.5 text-xs font-medium rounded-lg transition-colors"
        :class="showForm ? 'bg-slate-700 text-slate-300' : 'bg-blue-600 hover:bg-blue-500 text-white'">
        {{ showForm ? $t('common.action.cancel') : $t('components.supplier_review.submit') }}
      </button>
    </div>

    <!-- Review form -->
    <div v-if="showForm" class="bg-slate-800/50 border border-slate-700 rounded-lg p-4 space-y-3">
      <div>
        <label class="block text-[10px] text-slate-500 mb-1">{{ $t('components.supplier_review.outcome') }}</label>
        <select v-model="form.outcome" class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500">
          <option value="satisfactory">{{ $t('components.supplier_review.outcome_value.satisfactory') }}</option>
          <option value="concerns">{{ $t('components.supplier_review.outcome_value.concerns') }}</option>
          <option value="unsatisfactory">{{ $t('components.supplier_review.outcome_value.unsatisfactory') }}</option>
        </select>
      </div>
      <div class="grid grid-cols-3 gap-3">
        <label class="flex items-center gap-2 text-xs text-slate-300">
          <input type="checkbox" v-model="form.certifications_verified" class="rounded bg-slate-700 border-slate-600" />
          {{ $t('components.supplier_review.certifications_verified') }}
        </label>
        <label class="flex items-center gap-2 text-xs text-slate-300">
          <input type="checkbox" v-model="form.data_handling_verified" class="rounded bg-slate-700 border-slate-600" />
          {{ $t('components.supplier_review.data_handling_verified') }}
        </label>
        <label class="flex items-center gap-2 text-xs text-slate-300">
          <input type="checkbox" v-model="form.sla_met" class="rounded bg-slate-700 border-slate-600" />
          {{ $t('components.supplier_review.sla_met') }}
        </label>
      </div>
      <div>
        <label class="block text-[10px] text-slate-500 mb-1">{{ $t('components.supplier_review.notes') }} <span class="text-red-400">*</span></label>
        <textarea v-model="form.notes" rows="3"
          :placeholder="$t('components.supplier_review.notes_placeholder')"
          class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500 resize-none"></textarea>
      </div>
      <div class="flex gap-2">
        <button @click="submit" :disabled="!form.notes.trim() || submitting"
          class="px-3 py-1.5 text-xs font-medium bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white rounded-lg transition-colors">
          {{ submitting ? $t('common.state.saving') : $t('components.supplier_review.save') }}
        </button>
        <button @click="showForm = false" class="px-3 py-1.5 text-xs text-slate-500 hover:text-slate-300">{{ $t('common.action.cancel') }}</button>
      </div>
      <div v-if="error" class="text-[10px] text-red-400">{{ error }}</div>
    </div>

    <!-- Review history -->
    <div v-if="reviews.length === 0 && !showForm" class="text-xs text-slate-600">{{ $t('components.supplier_review.empty') }}</div>
    <div v-else class="space-y-2">
      <div v-for="r in reviews" :key="r.id" class="bg-slate-800/30 border border-slate-800 rounded-lg px-4 py-3">
        <div class="flex items-center gap-2 mb-1">
          <span class="px-1.5 py-0.5 rounded text-[10px] font-semibold"
            :class="r.outcome === 'satisfactory' ? 'bg-emerald-500/15 text-emerald-400'
              : r.outcome === 'concerns' ? 'bg-amber-500/15 text-amber-400'
              : 'bg-red-500/15 text-red-400'">
            {{ outcomeLabel(r.outcome) }}
          </span>
          <span class="text-[10px] text-slate-500">{{ r.reviewed_by?.split('@')[0] }}</span>
          <span class="text-[10px] text-slate-600">{{ formatTime(r.created_at) }}</span>
        </div>
        <div class="flex items-center gap-3 text-[10px] text-slate-500 mb-1">
          <span v-if="r.certifications_verified" class="text-emerald-500">{{ $t('components.supplier_review.certs_verified_short') }}</span>
          <span v-if="r.data_handling_verified" class="text-emerald-500">{{ $t('components.supplier_review.data_handling_verified') }}</span>
          <span v-if="r.sla_met" class="text-emerald-500">{{ $t('components.supplier_review.sla_met') }}</span>
          <span v-if="!r.sla_met" class="text-red-400">{{ $t('components.supplier_review.sla_not_met') }}</span>
        </div>
        <div v-if="r.notes" class="text-xs text-slate-400">{{ r.notes }}</div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { formatRecent } from '../composables/useFormat.js'
import { renderApiError } from '../composables/useApiError.js'

const { t } = useI18n()

// The history rows rendered the stored value verbatim, so every locale saw
// "satisfactory" in English lower case.
function outcomeLabel(outcome) {
  if (outcome === 'concerns') return t('components.supplier_review.outcome_value.concerns')
  if (outcome === 'unsatisfactory') return t('components.supplier_review.outcome_value.unsatisfactory')
  return t('components.supplier_review.outcome_value.satisfactory')
}

const props = defineProps({
  supplierId: { type: Number, required: true },
  canWrite: { type: Boolean, default: false },
})

const emit = defineEmits(['reviewed'])

const reviews = ref([])
const showForm = ref(false)
const submitting = ref(false)
const error = ref('')
const form = ref({
  outcome: 'satisfactory',
  certifications_verified: false,
  data_handling_verified: false,
  sla_met: true,
  notes: '',
})

async function load() {
  try {
    const data = await api.getSupplierReviews(props.supplierId)
    reviews.value = Array.isArray(data) ? data : (data?.data || [])
  } catch { reviews.value = [] }
}

async function submit() {
  if (!form.value.notes.trim()) return
  submitting.value = true
  error.value = ''
  try {
    await api.createSupplierReview(props.supplierId, form.value)
    showForm.value = false
    form.value = { outcome: 'satisfactory', certifications_verified: false, data_handling_verified: false, sla_met: true, notes: '' }
    await load()
    emit('reviewed')
  } catch (e) {
    error.value = renderApiError(e) || t('components.supplier_review.error_save')
  } finally {
    submitting.value = false
  }
}

const formatTime = (ts) => formatRecent(ts, { within: 24 * 60 * 60 * 1000 })

onMounted(load)
watch(() => props.supplierId, load)
</script>
