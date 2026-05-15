<template>
  <div class="space-y-3">
    <!-- Header -->
    <div class="flex items-center gap-2">
      <button @click="expanded = !expanded" class="flex items-center gap-2 text-[10px] text-slate-600 uppercase tracking-wider hover:text-slate-400 transition-colors">
        <svg class="w-3 h-3 transition-transform" :class="expanded ? 'rotate-90' : ''" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        Readings
        <span v-if="readings.length > 0" class="px-1.5 py-0.5 rounded-full bg-blue-500/15 text-blue-400 text-[10px] font-semibold normal-case">{{ readings.length }}</span>
      </button>
      <button v-if="canWrite" @click="openForm"
        class="ml-auto text-[10px] px-2.5 py-1 rounded bg-blue-600 hover:bg-blue-500 text-white font-medium transition-colors">
        {{ showForm ? 'Cancel' : 'Submit Reading' }}
      </button>
    </div>

    <!-- Submit Reading form -->
    <form v-if="showForm" @submit.prevent="submitReading" class="bg-slate-800/50 border border-blue-500/20 rounded-lg px-4 py-3 space-y-3">
      <div class="text-[10px] text-slate-500 uppercase tracking-wider">Reading for {{ identifier }}</div>

      <!-- Risk fields -->
      <template v-if="entityType === 'risk'">
        <div class="text-[10px] text-slate-500 mb-1">Impact (CIA)</div>
        <div class="grid grid-cols-3 gap-3">
          <div>
            <label class="block text-[10px] text-slate-600 mb-1">Confidentiality *</label>
            <select v-model.number="form.confidentiality" class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-[11px] text-white focus:outline-none focus:border-blue-500">
              <option :value="0">Not assessed</option>
              <option v-for="n in 5" :key="n" :value="n">{{ ciaLabel(n) }}</option>
            </select>
          </div>
          <div>
            <label class="block text-[10px] text-slate-600 mb-1">Integrity *</label>
            <select v-model.number="form.integrity" class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-[11px] text-white focus:outline-none focus:border-blue-500">
              <option :value="0">Not assessed</option>
              <option v-for="n in 5" :key="n" :value="n">{{ ciaLabel(n) }}</option>
            </select>
          </div>
          <div>
            <label class="block text-[10px] text-slate-600 mb-1">Availability *</label>
            <select v-model.number="form.availability" class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-[11px] text-white focus:outline-none focus:border-blue-500">
              <option :value="0">Not assessed</option>
              <option v-for="n in 5" :key="n" :value="n">{{ ciaLabel(n) }}</option>
            </select>
          </div>
        </div>
        <div class="grid grid-cols-2 gap-3 mt-2">
          <div>
            <label class="block text-[10px] text-slate-600 mb-1">Likelihood *</label>
            <select v-model.number="form.current_likelihood" required class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-[11px] text-white focus:outline-none focus:border-blue-500">
              <option :value="0" disabled>Select</option>
              <option v-for="n in 5" :key="n" :value="n">{{ ratingLabel(n) }}</option>
            </select>
          </div>
          <div>
            <label class="block text-[10px] text-slate-600 mb-1">Risk Score</label>
            <div class="h-[30px] flex items-center">
              <span v-if="computedImpact > 0 && form.current_likelihood > 0"
                class="inline-flex items-center gap-1.5 px-2 py-1 rounded text-[11px] font-bold"
                :class="scoreLevelColor(form.current_likelihood * computedImpact)">
                {{ form.current_likelihood * computedImpact }}
                ({{ scoreLevelLabel(form.current_likelihood * computedImpact) }})
              </span>
              <span v-else class="text-[11px] text-slate-600">Set CIA + Likelihood</span>
            </div>
          </div>
        </div>
      </template>

      <!-- Asset / Supplier / System fields (CIA classification) -->
      <template v-if="entityType === 'asset' || entityType === 'supplier' || entityType === 'system'">
        <div class="text-[10px] text-slate-500 mb-1">CIA Classification</div>
        <div class="grid grid-cols-3 gap-3">
          <div>
            <label class="block text-[10px] text-slate-600 mb-1">Confidentiality *</label>
            <select v-model.number="form.confidentiality" class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-[11px] text-white focus:outline-none focus:border-blue-500">
              <option :value="0">Not assessed</option>
              <option v-for="n in 5" :key="n" :value="n">{{ ciaLabel(n) }}</option>
            </select>
          </div>
          <div>
            <label class="block text-[10px] text-slate-600 mb-1">Integrity *</label>
            <select v-model.number="form.integrity" class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-[11px] text-white focus:outline-none focus:border-blue-500">
              <option :value="0">Not assessed</option>
              <option v-for="n in 5" :key="n" :value="n">{{ ciaLabel(n) }}</option>
            </select>
          </div>
          <div>
            <label class="block text-[10px] text-slate-600 mb-1">Availability *</label>
            <select v-model.number="form.availability" class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-[11px] text-white focus:outline-none focus:border-blue-500">
              <option :value="0">Not assessed</option>
              <option v-for="n in 5" :key="n" :value="n">{{ ciaLabel(n) }}</option>
            </select>
          </div>
        </div>
      </template>

      <!-- Legal fields -->
      <template v-if="entityType === 'legal_requirement'">
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-[10px] text-slate-600 mb-1">Likelihood *</label>
            <select v-model.number="form.current_likelihood" required class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-[11px] text-white focus:outline-none focus:border-blue-500">
              <option :value="0" disabled>Select</option>
              <option v-for="n in 5" :key="n" :value="n">{{ ratingLabel(n) }}</option>
            </select>
          </div>
          <div>
            <label class="block text-[10px] text-slate-600 mb-1">Impact *</label>
            <select v-model.number="form.current_impact" required class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-[11px] text-white focus:outline-none focus:border-blue-500">
              <option :value="0" disabled>Select</option>
              <option v-for="n in 5" :key="n" :value="n">{{ ratingLabel(n) }}</option>
            </select>
          </div>
        </div>
      </template>

      <!-- Next review date -->
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-[10px] text-slate-600 mb-1">Next Review</label>
          <input v-model="form.next_review" type="date"
            @change="nextReviewManuallyEdited = true"
            class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-[11px] text-white focus:outline-none focus:border-blue-500" />
        </div>
        <div>
          <label class="block text-[10px] text-slate-600 mb-1">Notes</label>
          <textarea v-model="form.notes" rows="1"
            class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-[11px] text-white focus:outline-none focus:border-blue-500 resize-none"
            placeholder="Assessment notes..." />
        </div>
      </div>

      <div v-if="submitError" class="text-[10px] text-red-400">{{ submitError }}</div>

      <div class="flex gap-2">
        <button type="submit" :disabled="!isFormValid || submitting"
          class="text-[10px] px-3 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white rounded-lg font-medium transition-colors">
          {{ submitting ? 'Saving...' : 'Save Reading' }}
        </button>
        <button type="button" @click="showForm = false" class="text-[10px] text-slate-500 hover:text-slate-300">Cancel</button>
      </div>
    </form>

    <!-- Reading history -->
    <div v-if="expanded" class="space-y-1">
      <div v-if="loadingReadings" class="h-8 bg-slate-800 rounded animate-pulse" />
      <div v-else-if="readings.length === 0" class="text-[11px] text-slate-600 italic py-2">No readings yet.</div>
      <div v-else class="overflow-x-auto">
        <table class="w-full text-[11px]">
          <thead>
            <tr class="text-slate-600 text-left">
              <th class="pb-1.5 pr-3 font-medium">Date</th>
              <th class="pb-1.5 pr-3 font-medium">Assessor</th>
              <th v-if="entityType === 'risk' || entityType === 'asset' || entityType === 'supplier' || entityType === 'system'" class="pb-1.5 pr-2 font-medium">C</th>
              <th v-if="entityType === 'risk' || entityType === 'asset' || entityType === 'supplier' || entityType === 'system'" class="pb-1.5 pr-2 font-medium">I</th>
              <th v-if="entityType === 'risk' || entityType === 'asset' || entityType === 'supplier' || entityType === 'system'" class="pb-1.5 pr-2 font-medium">A</th>
              <th v-if="entityType === 'risk' || entityType === 'legal_requirement'" class="pb-1.5 pr-2 font-medium">L</th>
              <th v-if="entityType === 'risk' || entityType === 'legal_requirement'" class="pb-1.5 pr-2 font-medium">Imp</th>
              <th v-if="entityType === 'risk' || entityType === 'legal_requirement'" class="pb-1.5 pr-2 font-medium">Score</th>
              <th class="pb-1.5 font-medium">Notes</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in readings" :key="r.id" class="border-t border-slate-800/50">
              <td class="py-1.5 pr-3 text-slate-400 whitespace-nowrap">{{ relativeTime(r.created_at) }}</td>
              <td class="py-1.5 pr-3 text-slate-400 whitespace-nowrap">{{ shortEmail(r.assessed_by) }}</td>
              <td v-if="entityType === 'risk' || entityType === 'asset' || entityType === 'supplier' || entityType === 'system'" class="py-1.5 pr-2">
                <span v-if="r.confidentiality" class="inline-flex items-center justify-center w-5 h-5 rounded text-[10px] font-bold" :class="scoreColor(r.confidentiality)">{{ r.confidentiality }}</span>
                <span v-else class="text-slate-700">-</span>
              </td>
              <td v-if="entityType === 'risk' || entityType === 'asset' || entityType === 'supplier' || entityType === 'system'" class="py-1.5 pr-2">
                <span v-if="r.integrity" class="inline-flex items-center justify-center w-5 h-5 rounded text-[10px] font-bold" :class="scoreColor(r.integrity)">{{ r.integrity }}</span>
                <span v-else class="text-slate-700">-</span>
              </td>
              <td v-if="entityType === 'risk' || entityType === 'asset' || entityType === 'supplier' || entityType === 'system'" class="py-1.5 pr-2">
                <span v-if="r.availability" class="inline-flex items-center justify-center w-5 h-5 rounded text-[10px] font-bold" :class="scoreColor(r.availability)">{{ r.availability }}</span>
                <span v-else class="text-slate-700">-</span>
              </td>
              <td v-if="entityType === 'risk' || entityType === 'legal_requirement'" class="py-1.5 pr-2">
                <span v-if="r.current_likelihood" class="inline-flex items-center justify-center w-5 h-5 rounded text-[10px] font-bold" :class="scoreColor(r.current_likelihood)">{{ r.current_likelihood }}</span>
                <span v-else class="text-slate-700">-</span>
              </td>
              <td v-if="entityType === 'risk' || entityType === 'legal_requirement'" class="py-1.5 pr-2">
                <span v-if="r.current_impact" class="inline-flex items-center justify-center w-5 h-5 rounded text-[10px] font-bold" :class="scoreColor(r.current_impact)">{{ r.current_impact }}</span>
                <span v-else class="text-slate-700">-</span>
              </td>
              <td v-if="entityType === 'risk' || entityType === 'legal_requirement'" class="py-1.5 pr-2">
                <span v-if="r.current_likelihood && r.current_impact" class="inline-flex items-center justify-center px-1.5 h-5 rounded text-[10px] font-bold" :class="scoreLevelColor(r.current_likelihood * r.current_impact)">{{ r.current_likelihood * r.current_impact }}</span>
                <span v-else class="text-slate-700">-</span>
              </td>
              <td class="py-1.5 text-slate-500 max-w-[200px] truncate" :title="r.notes">{{ r.notes || '-' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { api } from '../api'

// Auto-recompute next_review when severity changes, unless user has manually edited it

const props = defineProps({
  entityType: { type: String, required: true },
  entityId: { type: Number, required: true },
  identifier: { type: String, default: '' },
  canWrite: { type: Boolean, default: false },
  // Current entity values — pre-fill the form
  currentValues: { type: Object, default: () => ({}) },
})

const emit = defineEmits(['saved'])

const expanded = ref(true)
const showForm = ref(false)
const submitting = ref(false)
const submitError = ref('')
const loadingReadings = ref(false)
const readings = ref([])

const form = ref({})
const nextReviewManuallyEdited = ref(false)

// Review cycle defaults in months, keyed by risk level — mirrors backend reviewCycleDefaults
const REVIEW_CYCLES = { critical: 1, high: 3, medium: 6, low: 12 }

function scoreToLevel(score) {
  if (score >= 16) return 'critical'
  if (score >= 10) return 'high'
  if (score >= 5) return 'medium'
  if (score > 0) return 'low'
  return ''
}

function computeNextReview() {
  let level = ''
  if (props.entityType === 'risk') {
    const impact = Math.max(form.value.confidentiality || 0, form.value.integrity || 0, form.value.availability || 0)
    level = scoreToLevel((form.value.current_likelihood || 0) * impact)
  } else if (props.entityType === 'legal_requirement') {
    level = scoreToLevel((form.value.current_likelihood || 0) * (form.value.current_impact || 0))
  } else if (props.entityType === 'asset') {
    // Asset has no likelihood — severity is the highest CIA value (1-5)
    const max = Math.max(form.value.confidentiality || 0, form.value.integrity || 0, form.value.availability || 0)
    if (max === 5) level = 'critical'
    else if (max === 4) level = 'high'
    else if (max === 3) level = 'medium'
    else if (max >= 1) level = 'low'
  } else if (props.entityType === 'supplier' || props.entityType === 'system') {
    // Supplier/System cycle is driven by criticality, not CIA
    level = (props.currentValues && props.currentValues.criticality) || ''
  }
  const months = (level && REVIEW_CYCLES[level]) || 12
  const d = new Date()
  d.setMonth(d.getMonth() + months)
  return d.toISOString().split('T')[0]
}

function openForm() {
  if (showForm.value) { showForm.value = false; return }
  // Pre-fill with current entity values
  const cv = props.currentValues || {}
  form.value = {
    confidentiality: cv.confidentiality || cv.confidentiality_impact || 0,
    integrity: cv.integrity || cv.integrity_impact || 0,
    availability: cv.availability || cv.availability_impact || 0,
    current_likelihood: cv.current_likelihood || 0,
    current_impact: cv.current_impact || 0,
    notes: '',
    next_review: '',
  }
  form.value.next_review = computeNextReview()
  nextReviewManuallyEdited.value = false
  submitError.value = ''
  showForm.value = true
}

// Labels
const ratingLabels = { 1: 'Very Low (1)', 2: 'Low (2)', 3: 'Medium (3)', 4: 'High (4)', 5: 'Very High (5)' }
const ciaLabels = { 1: 'Insignificant (1)', 2: 'Minor (2)', 3: 'Moderate (3)', 4: 'Major (4)', 5: 'Severe (5)' }
function ratingLabel(n) { return ratingLabels[n] || n }
function ciaLabel(n) { return ciaLabels[n] || n }

function scoreLevelLabel(score) {
  if (score >= 16) return 'Critical'
  if (score >= 10) return 'High'
  if (score >= 5) return 'Medium'
  return 'Low'
}

function scoreLevelColor(score) {
  if (score >= 16) return 'text-red-400'
  if (score >= 10) return 'text-orange-400'
  if (score >= 5) return 'text-amber-400'
  return 'text-emerald-400'
}

// Impact = max(C, I, A) — CIA IS the impact assessment
const computedImpact = computed(() => Math.max(form.value.confidentiality || 0, form.value.integrity || 0, form.value.availability || 0))

const isFormValid = computed(() => {
  if (props.entityType === 'risk') return form.value.current_likelihood > 0 && computedImpact.value > 0
  if (props.entityType === 'legal_requirement') return form.value.current_likelihood > 0 && form.value.current_impact > 0
  if (props.entityType === 'asset') return computedImpact.value > 0
  if (props.entityType === 'supplier') return computedImpact.value > 0
  if (props.entityType === 'system') return computedImpact.value > 0
  return false
})

async function loadReadings() {
  loadingReadings.value = true
  try {
    let data
    if (props.entityType === 'risk') data = await api.getRiskReadings(props.entityId)
    else if (props.entityType === 'legal_requirement') data = await api.getLegalReadings(props.entityId)
    else if (props.entityType === 'asset') data = await api.getAssetReadings(props.entityId)
    else if (props.entityType === 'supplier') data = await api.getSupplierReadings(props.entityId)
    else if (props.entityType === 'system') data = await api.getSystemReadings(props.entityId)
    else return
    readings.value = Array.isArray(data) ? data : (data?.data || [])
  } catch { readings.value = [] }
  finally { loadingReadings.value = false }
}

async function submitReading() {
  submitting.value = true
  submitError.value = ''
  try {
    const payload = { ...form.value }
    // For risk: impact = max(C, I, A)
    if (props.entityType === 'risk') {
      payload.current_impact = computedImpact.value
    }
    // Asset/Supplier/System have no likelihood/impact — strip them so DB stores NULL not 0
    if (props.entityType === 'asset' || props.entityType === 'supplier' || props.entityType === 'system') {
      delete payload.current_likelihood
      delete payload.current_impact
    }
    // Clean zero values
    if (payload.confidentiality === 0) delete payload.confidentiality
    if (payload.integrity === 0) delete payload.integrity
    if (payload.availability === 0) delete payload.availability
    if (payload.current_likelihood === 0) delete payload.current_likelihood
    if (payload.current_impact === 0) delete payload.current_impact

    if (props.entityType === 'risk') await api.createRiskReading(props.entityId, payload)
    else if (props.entityType === 'legal_requirement') await api.createLegalReading(props.entityId, payload)
    else if (props.entityType === 'asset') await api.createAssetReading(props.entityId, payload)
    else if (props.entityType === 'supplier') await api.createSupplierReading(props.entityId, payload)
    else if (props.entityType === 'system') await api.createSystemReading(props.entityId, payload)

    showForm.value = false
    await loadReadings()
    emit('saved')
  } catch (e) {
    submitError.value = e.message || 'Failed to save reading'
  } finally {
    submitting.value = false
  }
}

function scoreColor(v) {
  if (v >= 4) return 'bg-red-900/60 text-red-300'
  if (v >= 3) return 'bg-amber-900/60 text-amber-300'
  if (v >= 2) return 'bg-yellow-900/40 text-yellow-300'
  return 'bg-emerald-900/40 text-emerald-300'
}

function relativeTime(ts) {
  if (!ts) return ''
  const d = new Date(typeof ts === 'number' ? ts * 1000 : ts)
  const diff = Math.floor((Date.now() - d.getTime()) / 60000)
  if (diff < 60) return `${diff}m ago`
  if (diff < 1440) return `${Math.floor(diff / 60)}h ago`
  return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

function shortEmail(e) { return e ? e.split('@')[0] : '-' }

onMounted(loadReadings)
watch(() => props.entityId, loadReadings)

// When severity inputs change, auto-update next_review (unless user edited it manually)
watch(
  () => [form.value.confidentiality, form.value.integrity, form.value.availability, form.value.current_likelihood, form.value.current_impact],
  () => {
    if (showForm.value && !nextReviewManuallyEdited.value) {
      form.value.next_review = computeNextReview()
    }
  }
)

// Safety net: whenever the form opens, ensure next_review is a future date
watch(showForm, (open) => {
  if (open && (!form.value.next_review || form.value.next_review < new Date().toISOString().split('T')[0])) {
    form.value.next_review = computeNextReview()
  }
})
</script>
