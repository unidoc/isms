<template>
  <div class="space-y-2">
    <div
      v-for="i in rows"
      :key="i"
      :class="rowClass"
    />
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  rows: {
    type: Number,
    default: 5,
  },
  height: {
    type: String,
    default: 'h-16',
  },
})

const rowClass = computed(
  () => `${props.height} bg-slate-900 border border-slate-800 rounded-xl animate-pulse`,
)
</script>
