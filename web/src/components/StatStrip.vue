<template>
  <!-- Slim, always-visible register stat strip. Each stat is click-to-filter:
       clicking toggles the active key via v-model. Replaces the tall stat-card
       grids at the top of the registers so the list gets the vertical space,
       while counts + filtering stay glanceable. -->
  <div class="flex flex-wrap gap-2">
    <button
      v-for="s in stats"
      :key="s.key"
      type="button"
      @click="$emit('update:modelValue', modelValue === s.key ? '' : s.key)"
      class="inline-flex items-baseline gap-1.5 rounded-full border px-3 py-1 text-xs transition-colors"
      :class="modelValue === s.key
        ? 'border-blue-500/50 bg-blue-500/10 text-blue-200'
        : 'border-slate-800 bg-slate-900 text-slate-400 hover:border-slate-700 hover:text-slate-300'"
      :title="`Filter: ${s.label}`">
      <span class="font-bold tabular-nums" :class="modelValue === s.key ? '' : (s.color || 'text-slate-100')">{{ s.count }}</span>
      <span>{{ s.label }}</span>
    </button>
  </div>
</template>

<script setup>
// stats: [{ key, label, count, color }] — color is a text-* class for the count.
// key '' means "all / clear filter". modelValue is the active filter key.
defineProps({
  stats: { type: Array, required: true },
  modelValue: { type: String, default: '' },
})
defineEmits(['update:modelValue'])
</script>
