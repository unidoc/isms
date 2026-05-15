<template>
  <div class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
    <div class="px-5 py-3.5 border-b border-slate-800 flex items-center justify-between">
      <h3 class="text-sm font-semibold text-slate-200">Overdue Reviews</h3>
      <div class="flex items-center gap-2">
        <span v-if="totalCount > 0" class="text-xs font-semibold bg-red-900/60 text-red-300 px-2.5 py-0.5 rounded-full tabular-nums">
          {{ totalCount }}
        </span>
        <button
          v-if="totalCount > 0 && isAdmin"
          @click="createTasks"
          :disabled="creating"
          class="text-xs px-2.5 py-1 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-medium transition-colors disabled:opacity-50"
        >
          {{ creating ? 'Creating...' : 'Create Tasks' }}
        </button>
      </div>
    </div>

    <!-- Task creation result -->
    <div v-if="taskResult" class="px-5 py-2.5 bg-emerald-950/30 border-b border-slate-800 text-xs text-emerald-300">
      Created {{ taskResult.created?.length || 0 }} tasks{{ taskResult.skipped > 0 ? `, ${taskResult.skipped} skipped (already exist)` : '' }}
    </div>

    <div v-if="allItems.length > 0" class="divide-y divide-slate-800/50 max-h-[400px] overflow-y-auto">
      <div
        v-for="item in allItems"
        :key="item.entity_type + '-' + item.entity_id"
        class="flex items-center gap-3 px-5 py-3 hover:bg-slate-800/30 transition-colors"
      >
        <!-- Type icon -->
        <div class="w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0" :class="typeIconBg(item.entity_type)">
          <svg v-if="item.entity_type === 'risk'" class="w-4 h-4 text-orange-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          <svg v-else-if="item.entity_type === 'supplier'" class="w-4 h-4 text-purple-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z" />
          </svg>
          <svg v-else-if="item.entity_type === 'system'" class="w-4 h-4 text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2" />
          </svg>
          <svg v-else-if="item.entity_type === 'legal'" class="w-4 h-4 text-cyan-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M3 6l3 1m0 0l-3 9a5.002 5.002 0 006.001 0M6 7l3 9M6 7l6-2m6 2l3-1m-3 1l-3 9a5.002 5.002 0 006.001 0M18 7l3 9m-3-9l-6-2m0-2v2m0 16V5m0 16H9m3 0h3" />
          </svg>
          <svg v-else-if="item.entity_type === 'task'" class="w-4 h-4 text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4" />
          </svg>
        </div>

        <!-- Details -->
        <div class="flex-1 min-w-0">
          <div class="text-sm font-medium text-slate-200 truncate">{{ item.title }}</div>
          <div class="flex items-center gap-2 mt-0.5">
            <span class="text-[10px] font-medium px-1.5 py-0.5 rounded-full" :class="typeBadgeClass(item.entity_type)">{{ item.entity_id }}</span>
            <span v-if="item.criticality" class="text-[10px] font-medium px-1.5 py-0.5 rounded-full bg-slate-800 text-slate-400">{{ item.criticality }}</span>
          </div>
        </div>

        <!-- Days overdue -->
        <div class="text-right flex-shrink-0">
          <div class="text-sm font-bold tabular-nums" :class="daysColor(item.days_late)">{{ item.days_late }}d</div>
          <div class="text-[10px] text-slate-600">overdue</div>
        </div>
      </div>
    </div>
    <div v-else class="p-8 text-center">
      <div class="text-sm text-emerald-400 font-medium">All reviews on schedule</div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api'

const props = defineProps({
  isAdmin: { type: Boolean, default: false },
})

const emit = defineEmits(['updated'])

const overdue = ref(null)
const creating = ref(false)
const taskResult = ref(null)

const totalCount = computed(() => overdue.value?.total_count || 0)

const allItems = computed(() => {
  if (!overdue.value) return []
  const items = [
    ...(overdue.value.risks || []),
    ...(overdue.value.suppliers || []),
    ...(overdue.value.systems || []),
    ...(overdue.value.legal || []),
    ...(overdue.value.tasks || []),
  ]
  items.sort((a, b) => b.days_late - a.days_late)
  return items
})

async function loadOverdue() {
  try {
    overdue.value = await api.getOverdue()
  } catch { /* ignore */ }
}

async function createTasks() {
  creating.value = true
  taskResult.value = null
  try {
    taskResult.value = await api.createOverdueTasks()
    // Refresh the overdue list
    await loadOverdue()
    emit('updated')
    // Auto-hide result after 5 seconds
    setTimeout(() => { taskResult.value = null }, 5000)
  } catch { /* ignore */ }
  creating.value = false
}

onMounted(loadOverdue)

function typeIconBg(type) {
  switch (type) {
    case 'risk': return 'bg-orange-950/60'
    case 'supplier': return 'bg-purple-950/60'
    case 'system': return 'bg-blue-950/60'
    case 'legal': return 'bg-cyan-950/60'
    case 'task': return 'bg-emerald-950/60'
    default: return 'bg-slate-800'
  }
}

function typeBadgeClass(type) {
  switch (type) {
    case 'risk': return 'bg-orange-900/40 text-orange-400'
    case 'supplier': return 'bg-purple-900/40 text-purple-400'
    case 'system': return 'bg-blue-900/40 text-blue-400'
    case 'legal': return 'bg-cyan-900/40 text-cyan-400'
    case 'task': return 'bg-emerald-900/40 text-emerald-400'
    default: return 'bg-slate-800 text-slate-400'
  }
}

function daysColor(days) {
  if (days >= 30) return 'text-red-400'
  if (days >= 14) return 'text-orange-400'
  if (days >= 7) return 'text-amber-400'
  return 'text-yellow-400'
}
</script>
