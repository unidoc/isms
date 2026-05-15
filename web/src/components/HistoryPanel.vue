<template>
  <div>
    <div v-if="loading" class="h-10 bg-slate-800 rounded animate-pulse" />
    <div v-else-if="grouped.length === 0" class="text-sm text-slate-600 italic py-2">No history yet.</div>
    <div v-else class="max-h-[50vh] overflow-y-auto">
      <table class="w-full text-xs">
        <tbody>
          <tr v-for="(g, i) in grouped.slice(0, 50)" :key="i"
            class="border-b border-slate-800/30 hover:bg-slate-800/30 transition-colors group cursor-default"
            :title="groupTooltip(g)">
            <td class="py-1.5 pr-3 text-slate-600 whitespace-nowrap w-[110px] align-top">{{ formatDate(g.created_at) }}</td>
            <td class="py-1.5 pr-3 text-slate-500 whitespace-nowrap w-[120px] truncate max-w-[120px] align-top">{{ g.changed_by }}</td>
            <td class="py-1.5 text-slate-400 max-w-0">
              <div class="truncate">
                <template v-if="g.entries.length === 1">
                  <span class="font-medium">{{ shortAction(g.entries[0]) }}</span>
                  <span v-if="g.entries[0].field" class="text-slate-500"> {{ g.entries[0].field.replace(/_/g, ' ') }}</span>
                  <span v-if="g.entries[0].new_value && !g.entries[0].old_value" class="text-emerald-400/60"> {{ trunc(g.entries[0].new_value) }}</span>
                  <span v-if="g.entries[0].old_value && g.entries[0].new_value" class="text-slate-600"> {{ trunc(g.entries[0].old_value) }} → {{ trunc(g.entries[0].new_value) }}</span>
                </template>
                <template v-else>
                  <span class="font-medium">{{ groupLabel(g) }}</span>
                  <span class="text-slate-500"> {{ g.entries.length }} fields</span>
                  <span class="text-slate-600"> ({{ fieldSummary(g) }})</span>
                </template>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-if="grouped.length > 50" class="text-[10px] text-slate-600 pt-2">Showing 50 of {{ grouped.length }}</div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { api } from '../api'

const props = defineProps({
  entityType: { type: String, required: true },
  entityId: { type: String, required: true },
})

const history = ref([])
const loading = ref(false)

function shortAction(e) {
  const m = { create: 'Created', update: 'Changed', delete: 'Deleted', reading: 'Reading', suggestion_applied: 'Applied suggestion' }
  return m[e.action] || e.detail || e.action
}

function groupLabel(g) {
  return shortAction(g.entries[0])
}

function fieldSummary(g, max = 4) {
  const names = g.entries.filter(e => e.field).map(e => e.field.replace(/_/g, ' '))
  if (names.length === 0) return ''
  if (names.length <= max) return names.join(', ')
  return names.slice(0, max).join(', ') + ` +${names.length - max} more`
}

function trunc(v, n = 30) {
  if (!v) return ''
  const s = String(v)
  return s.length > n ? s.slice(0, n) + '…' : s
}

function groupTooltip(g) {
  if (g.entries.length === 1) {
    const e = g.entries[0]
    let t = `${g.changed_by} — ${e.action}`
    if (e.field) t += ` ${e.field}`
    if (e.old_value) t += `\nFrom: ${e.old_value}`
    if (e.new_value) t += `\nTo: ${e.new_value}`
    if (e.reason) t += `\nReason: ${e.reason}`
    return t
  }
  let t = `${g.changed_by} — ${shortAction(g.entries[0])} ${g.entries.length} fields`
  for (const e of g.entries) {
    if (!e.field) continue
    const fieldName = e.field.replace(/_/g, ' ')
    if (e.old_value && e.new_value) {
      t += `\n• ${fieldName}: ${trunc(e.old_value, 20)} → ${trunc(e.new_value, 20)}`
    } else if (e.new_value) {
      t += `\n• ${fieldName}: ${trunc(e.new_value, 30)}`
    } else {
      t += `\n• ${fieldName}`
    }
  }
  return t
}

function formatDate(d) {
  if (!d && d !== 0) return ''
  const dt = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  return dt.toLocaleDateString('en-GB', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })
}

function toEpoch(d) {
  return typeof d === 'number' ? d : new Date(d).getTime() / 1000
}

// Group entries from the same save event: same actor + same action, within 10s.
const grouped = computed(() => {
  const groups = []
  const GROUP_WINDOW = 10 // seconds
  for (const e of history.value) {
    const actor = e.changed_by || e.actor || ''
    const ts = toEpoch(e.created_at)
    const last = groups[groups.length - 1]
    if (last && last.changed_by === actor && last.action === e.action && Math.abs(last.firstTs - ts) <= GROUP_WINDOW) {
      last.entries.push(e)
    } else {
      groups.push({
        created_at: e.created_at,
        firstTs: ts,
        changed_by: actor,
        action: e.action,
        entries: [e],
      })
    }
  }
  return groups
})

async function loadHistory() {
  loading.value = true
  try {
    const data = await api.getEntityChangelog(props.entityType, props.entityId)
    const items = Array.isArray(data) ? data : (Array.isArray(data?.data) ? data.data : [])
    history.value = items.sort((a, b) => toEpoch(b.created_at) - toEpoch(a.created_at))
  } catch { history.value = [] }
  loading.value = false
}

onMounted(loadHistory)
watch(() => props.entityId, loadHistory)
</script>
