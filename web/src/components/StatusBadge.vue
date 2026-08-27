<template>
  <span :class="classes">{{ label }}</span>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useEnumLabel } from '../composables/useEnumLabel.js'

const props = defineProps({
  status: String,
  // The enum family the value belongs to. Almost every caller passes a status
  // column, and status values are distinct across registers — the colour map
  // below has always been one flat table for the same reason — so `status` is
  // one shared group rather than one per register. The few callers rendering a
  // different family (classification, criticality, an audit result) name it.
  group: { type: String, default: 'status' },
})

const { t } = useI18n()
const { enumLabel } = useEnumLabel()

const label = computed(() => {
  if (!props.status) return t('common.state.unknown')
  return enumLabel(props.group, props.status)
})

const classes = computed(() => {
  const base = 'inline-block px-2 py-0.5 rounded text-xs font-semibold'
  const map = {
    // Document statuses
    draft: 'bg-slate-700 text-slate-300',
    approved: 'bg-emerald-900/60 text-emerald-300',
    in_review: 'bg-amber-900/60 text-amber-300',
    retired: 'bg-slate-800 text-slate-500',

    // Risk / general statuses
    open: 'bg-blue-900/60 text-blue-300',
    treating: 'bg-amber-900/60 text-amber-300',
    accepted: 'bg-emerald-900/60 text-emerald-300',
    closed: 'bg-slate-700 text-slate-400',

    // Risk levels / priority
    low: 'bg-emerald-900/60 text-emerald-300',
    medium: 'bg-amber-900/60 text-amber-300',
    high: 'bg-orange-900/60 text-orange-300',
    critical: 'bg-red-900/60 text-red-300',

    // Task statuses
    in_progress: 'bg-blue-900/60 text-blue-300',
    done: 'bg-emerald-900/60 text-emerald-300',
    cancelled: 'bg-slate-700 text-slate-400',
    todo: 'bg-red-900/60 text-red-300',

    // Change management
    proposed: 'bg-amber-900/60 text-amber-300',
    implementing: 'bg-purple-900/60 text-purple-300',
    implemented: 'bg-emerald-900/60 text-emerald-300',
    rejected: 'bg-red-900/60 text-red-300',

    // Review
    changes_requested: 'bg-orange-900/60 text-orange-300',

    // Supplier
    pending: 'bg-amber-900/60 text-amber-300',
    under_review: 'bg-blue-900/60 text-blue-300',

    // Implementation
    not_started: 'bg-slate-700 text-slate-400',
    verified: 'bg-emerald-900/60 text-emerald-300',

    // Audit
    planned: 'bg-slate-700 text-slate-300',
    completed: 'bg-emerald-900/60 text-emerald-300',
    active: 'bg-blue-900/60 text-blue-300',
    conforming: 'bg-emerald-900/60 text-emerald-300',
    non_conforming: 'bg-red-900/60 text-red-300',
    non_conformity: 'bg-red-900/60 text-red-300',
    not_assessed: 'bg-slate-700 text-slate-400',
    observation: 'bg-amber-900/60 text-amber-300',
    opportunity: 'bg-blue-900/60 text-blue-300',

    // Classification
    public: 'bg-emerald-900/60 text-emerald-300',
    internal: 'bg-slate-700 text-slate-300',
    confidential: 'bg-amber-900/60 text-amber-300',
    restricted: 'bg-red-900/60 text-red-300',

    // Corrective actions
    assessment: 'bg-purple-900/60 text-purple-300',
    awaiting_approval: 'bg-amber-900/60 text-amber-300',
    implementation: 'bg-blue-900/60 text-blue-300',
    monitoring: 'bg-cyan-900/60 text-cyan-300',
    resolved: 'bg-emerald-900/60 text-emerald-300',

    // Objectives
    at_risk: 'bg-orange-900/60 text-orange-300',
    paused: 'bg-slate-700 text-slate-400',
    complete: 'bg-emerald-900/60 text-emerald-300',

    // Assets
    archived: 'bg-slate-800 text-slate-500',

    // Incidents
    investigating: 'bg-amber-900/60 text-amber-300',
    contained: 'bg-blue-900/60 text-blue-300',
  }
  return `${base} ${map[props.status] || 'bg-slate-700 text-slate-300'}`
})
</script>
