<template>
  <div v-if="refs.length > 0" class="mt-3">
    <div class="text-[10px] font-semibold text-slate-500 uppercase tracking-wider mb-1.5">Referenced in</div>
    <div class="flex flex-wrap gap-1.5">
      <router-link
        v-for="r in refs"
        :key="r.id"
        :to="refRoute(r)"
        class="inline-flex items-center gap-1 px-2 py-0.5 rounded border text-[11px] font-medium no-underline transition-colors hover:brightness-125"
        :class="refColors(r)"
        :title="refLabel(r) + ': ' + refId(r)"
      >
        <span class="opacity-60">{{ refLabel(r) }}</span>
        <span class="truncate max-w-[180px]">{{ r.title || refId(r) }}</span>
      </router-link>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'

const props = defineProps({
  entityType: { type: String, required: true },
  entityId: { type: String, required: true },
})

const route = useRoute()
const { orgPath } = useCurrentOrg()
const refs = ref([])

async function loadRefs() {
  if (!props.entityType || !props.entityId) {
    refs.value = []
    return
  }
  try {
    const data = await api.getReferences(props.entityType, props.entityId)
    refs.value = data || []
  } catch {
    refs.value = []
  }
}

onMounted(loadRefs)
watch(() => [props.entityType, props.entityId], loadRefs)

const typeColors = {
  risk: 'bg-red-900/40 text-red-300 border-red-800/50',
  legal: 'bg-purple-900/40 text-purple-300 border-purple-800/50',
  document: 'bg-blue-900/40 text-blue-300 border-blue-800/50',
  asset: 'bg-amber-900/40 text-amber-300 border-amber-800/50',
  supplier: 'bg-emerald-900/40 text-emerald-300 border-emerald-800/50',
  system: 'bg-cyan-900/40 text-cyan-300 border-cyan-800/50',
  incident: 'bg-orange-900/40 text-orange-300 border-orange-800/50',
  change: 'bg-sky-900/40 text-sky-300 border-sky-800/50',
  audit: 'bg-fuchsia-900/40 text-fuchsia-300 border-fuchsia-800/50',
  audit_finding: 'bg-rose-900/40 text-rose-300 border-rose-800/50',
  corrective_action: 'bg-pink-900/40 text-pink-300 border-pink-800/50',
  control: 'bg-indigo-900/40 text-indigo-300 border-indigo-800/50',
  objective: 'bg-teal-900/40 text-teal-300 border-teal-800/50',
  program: 'bg-violet-900/40 text-violet-300 border-violet-800/50',
  task: 'bg-lime-900/40 text-lime-300 border-lime-800/50',
}

const typeLabels = {
  risk: 'RISK',
  legal: 'LEGAL',
  document: 'DOC',
  asset: 'ASSET',
  supplier: 'SUPPLIER',
  system: 'SYSTEM',
  incident: 'INCIDENT',
  change: 'CHANGE',
  audit: 'AUDIT',
  audit_finding: 'FINDING',
  corrective_action: 'CA',
  control: 'CTRL',
  objective: 'OBJ',
  program: 'PROG',
  task: 'TASK',
}

const typeRoutes = {
  risk: 'risks',
  legal: 'legal',
  document: 'documents',
  asset: 'assets',
  supplier: 'suppliers',
  system: 'systems',
  incident: 'incidents',
  change: 'changes',
  audit: 'audit',
  audit_finding: 'audit',
  corrective_action: 'corrective-actions',
  control: 'documents',
  objective: 'objectives',
  program: 'programs',
  task: 'tasks',
}

function otherSide(r) {
  if (r.source_type === props.entityType && r.source_id === props.entityId) {
    return { type: r.target_type, id: r.target_id }
  }
  return { type: r.source_type, id: r.source_id }
}

function refLabel(r) {
  const other = otherSide(r)
  return typeLabels[other.type] || other.type.toUpperCase()
}

function refId(r) {
  return otherSide(r).id
}

function refColors(r) {
  const other = otherSide(r)
  return typeColors[other.type] || 'bg-slate-800 text-slate-300 border-slate-700'
}

function refRoute(r) {
  const other = otherSide(r)
  const routeBase = typeRoutes[other.type] || ''
  if (other.type === 'document' || other.type === 'control') {
    return orgPath(`/documents/${other.id}`)
  }
  // Deep link for entity types that support /:id routes
  const deepLinkTypes = ['change', 'incident', 'corrective_action', 'supplier', 'asset', 'task', 'risk', 'legal', 'system', 'program']
  if (deepLinkTypes.includes(other.type) && other.id) {
    const numId = other.id.replace(/^[A-Z]+-/, '')
    return orgPath(`/${routeBase}/${numId}`)
  }
  // Objective deep-links need tab prefix
  if (other.type === 'objective' && other.id) {
    const numId = other.id.replace(/^[A-Z]+-/, '')
    return orgPath(`/objectives/${numId}`)
  }
  // Audit deep-links need tab prefix
  if (other.type === 'audit' && other.id) {
    const numId = other.id.replace(/^[A-Z]+-/, '')
    return orgPath(`/audit/audits/${numId}`)
  }
  if (other.type === 'audit_finding' && other.id) {
    const numId = other.id.replace(/^[A-Z]+-/, '')
    return orgPath(`/audit/findings/${numId}`)
  }
  return orgPath(`/${routeBase}`)
}
</script>
