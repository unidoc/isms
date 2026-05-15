<template>
  <div v-if="totalPages > 1" class="flex items-center justify-between gap-3 px-2 py-2">
    <div class="text-xs text-slate-500 tabular-nums">
      Showing {{ rangeStart }}–{{ rangeEnd }} of {{ total }}
    </div>
    <div class="flex items-center gap-1">
      <button
        @click="$emit('update:page', 1)"
        :disabled="page === 1"
        class="px-2 py-1 text-xs text-slate-400 hover:text-slate-200 disabled:text-slate-700 disabled:cursor-not-allowed transition-colors"
      >«</button>
      <button
        @click="$emit('update:page', page - 1)"
        :disabled="page === 1"
        class="px-2 py-1 text-xs text-slate-400 hover:text-slate-200 disabled:text-slate-700 disabled:cursor-not-allowed transition-colors"
      >‹</button>
      <button
        v-for="p in visiblePages"
        :key="p.key"
        @click="typeof p.value === 'number' && $emit('update:page', p.value)"
        :disabled="p.value === '…'"
        class="min-w-[28px] px-2 py-1 text-xs rounded font-medium tabular-nums transition-colors"
        :class="p.value === page
          ? 'bg-slate-700 text-white'
          : p.value === '…'
            ? 'text-slate-700 cursor-default'
            : 'text-slate-400 hover:bg-slate-800 hover:text-slate-200'"
      >{{ p.value }}</button>
      <button
        @click="$emit('update:page', page + 1)"
        :disabled="page === totalPages"
        class="px-2 py-1 text-xs text-slate-400 hover:text-slate-200 disabled:text-slate-700 disabled:cursor-not-allowed transition-colors"
      >›</button>
      <button
        @click="$emit('update:page', totalPages)"
        :disabled="page === totalPages"
        class="px-2 py-1 text-xs text-slate-400 hover:text-slate-200 disabled:text-slate-700 disabled:cursor-not-allowed transition-colors"
      >»</button>
    </div>
    <div class="flex items-center gap-2 text-xs text-slate-500">
      <span>Per page</span>
      <select
        :value="pageSize"
        @change="$emit('update:pageSize', Number($event.target.value))"
        class="bg-slate-900 border border-slate-800 rounded px-1.5 py-0.5 text-xs text-slate-400 focus:outline-none focus:border-blue-500"
      >
        <option v-for="n in pageSizeOptions" :key="n" :value="n">{{ n }}</option>
      </select>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  page: { type: Number, required: true },
  pageSize: { type: Number, required: true },
  total: { type: Number, required: true },
  pageSizeOptions: { type: Array, default: () => [25, 50, 100, 200] },
})

defineEmits(['update:page', 'update:pageSize'])

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
const rangeStart = computed(() => props.total === 0 ? 0 : (props.page - 1) * props.pageSize + 1)
const rangeEnd = computed(() => Math.min(props.page * props.pageSize, props.total))

// Build truncated page list: 1 … (current-1) (current) (current+1) … last
const visiblePages = computed(() => {
  const tp = totalPages.value
  const cur = props.page
  const out = []
  const add = (v, k) => out.push({ value: v, key: k })
  if (tp <= 7) {
    for (let i = 1; i <= tp; i++) add(i, i)
    return out
  }
  add(1, 1)
  if (cur > 3) add('…', 'lo')
  const start = Math.max(2, cur - 1)
  const end = Math.min(tp - 1, cur + 1)
  for (let i = start; i <= end; i++) add(i, i)
  if (cur < tp - 2) add('…', 'hi')
  add(tp, tp)
  return out
})
</script>
