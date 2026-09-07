<template>
  <div class="bg-slate-900 border border-slate-800 rounded-xl p-4 max-w-2xl">
    <div class="flex items-center justify-between mb-3">
      <h2 class="text-sm font-semibold text-slate-200">{{ title || $t('components.heat_map.title') }}</h2>
      <div class="text-xs text-slate-500">{{ $t('components.heat_map.total_items', items.length) }}</div>
    </div>

    <!-- Grid -->
    <div class="flex gap-3">
      <!-- Y-axis label -->
      <div class="flex flex-col items-center justify-center">
        <span class="text-[10px] font-medium text-slate-500 tracking-wider uppercase writing-mode-vertical" style="writing-mode: vertical-rl; transform: rotate(180deg);">{{ $t('components.heat_map.likelihood') }}</span>
      </div>

      <div class="flex-1">
        <!-- Grid rows (Y=5 at top, Y=1 at bottom) -->
        <div class="space-y-1">
          <div v-for="y in [5, 4, 3, 2, 1]" :key="y" class="flex items-center gap-1">
            <!-- Y-axis tick label -->
            <div class="w-16 text-right pr-2 flex-shrink-0">
              <span class="text-[10px] text-slate-500 leading-tight">{{ likelihoodLabels[y] }}</span>
            </div>
            <!-- Cells for this row -->
            <div class="flex gap-1 flex-1">
              <div
                v-for="x in [1, 2, 3, 4, 5]"
                :key="x"
                class="relative flex-1 h-10 rounded flex items-center justify-center cursor-pointer transition-all duration-200 hover:ring-2 hover:ring-white/20 min-w-[32px]"
                :style="cellStyle(x, y)"
                @click="$emit('cell-click', { likelihood: y, impact: x, items: cellItems(x, y) })"
                @mouseenter="hoveredCell = `${x}-${y}`"
                @mouseleave="hoveredCell = null"
              >
                <span v-if="cellCount(x, y) > 0" class="text-sm font-bold tabular-nums text-white drop-shadow">
                  {{ cellCount(x, y) }}
                </span>

                <!-- Tooltip -->
                <div
                  v-if="hoveredCell === `${x}-${y}` && cellCount(x, y) > 0"
                  class="absolute z-50 bottom-full left-1/2 -translate-x-1/2 mb-2 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 shadow-xl pointer-events-none min-w-[180px] max-w-[260px]"
                >
                  <div class="text-[10px] font-medium text-slate-400 mb-1.5">
                    L{{ y }} x I{{ x }} = {{ y * x }} ({{ riskLabel(y * x) }})
                  </div>
                  <div class="space-y-0.5 max-h-32 overflow-y-auto">
                    <div
                      v-for="item in cellItems(x, y).slice(0, 8)"
                      :key="item.id || item.risk_id || item.title"
                      class="text-xs text-slate-300 truncate"
                    >
                      {{ item.title || item.risk_id || item.name || $t('components.heat_map.unnamed') }}
                    </div>
                    <div v-if="cellItems(x, y).length > 8" class="text-[10px] text-slate-500">
                      {{ $t('components.heat_map.more', { count: cellItems(x, y).length - 8 }) }}
                    </div>
                  </div>
                  <!-- Arrow -->
                  <div class="absolute top-full left-1/2 -translate-x-1/2 -mt-px">
                    <div class="w-2 h-2 bg-slate-800 border-r border-b border-slate-700 rotate-45 -translate-y-1"></div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- X-axis labels -->
        <div class="flex gap-1 mt-1.5">
          <div class="w-16 flex-shrink-0"></div>
          <div class="flex gap-1 flex-1">
            <div v-for="x in [1, 2, 3, 4, 5]" :key="x" class="flex-1 text-center min-w-[32px]">
              <span class="text-[10px] text-slate-500 leading-tight">{{ impactLabels[x] }}</span>
            </div>
          </div>
        </div>
        <!-- X-axis title -->
        <div class="text-center mt-1.5">
          <span class="text-[10px] font-medium text-slate-500 tracking-wider uppercase">{{ $t('components.heat_map.impact') }}</span>
        </div>
      </div>
    </div>

    <!-- Distribution summary -->
    <div class="flex items-center gap-4 mt-3 pt-3 border-t border-slate-800">
      <div class="flex items-center gap-1.5">
        <span class="w-3 h-3 rounded" style="background-color: rgba(34, 197, 94, 0.6)"></span>
        <span class="text-[11px] text-slate-400">{{ $t('components.heat_map.distribution_item', { label: $t('common.enum.severity.low'), count: distribution.low }) }}</span>
      </div>
      <div class="flex items-center gap-1.5">
        <span class="w-3 h-3 rounded" style="background-color: rgba(245, 158, 11, 0.6)"></span>
        <span class="text-[11px] text-slate-400">{{ $t('components.heat_map.distribution_item', { label: $t('common.enum.severity.medium'), count: distribution.medium }) }}</span>
      </div>
      <div class="flex items-center gap-1.5">
        <span class="w-3 h-3 rounded" style="background-color: rgba(249, 115, 22, 0.6)"></span>
        <span class="text-[11px] text-slate-400">{{ $t('components.heat_map.distribution_item', { label: $t('common.enum.severity.high'), count: distribution.high }) }}</span>
      </div>
      <div class="flex items-center gap-1.5">
        <span class="w-3 h-3 rounded" style="background-color: rgba(239, 68, 68, 0.6)"></span>
        <span class="text-[11px] text-slate-400">{{ $t('components.heat_map.distribution_item', { label: $t('common.enum.severity.critical'), count: distribution.critical }) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps({
  items: { type: Array, default: () => [] },
  title: { type: String, default: '' },
})

defineEmits(['cell-click'])

const { t } = useI18n()

const hoveredCell = ref(null)

// Axis labels are this component's own copy, not props. They are computed
// rather than plain objects for the usual reason: a module-level map is built
// before the locale resolves and would stay English after a locale change.
const likelihoodLabels = computed(() => ({
  1: t('components.heat_map.likelihood_value.rare'),
  2: t('components.heat_map.likelihood_value.unlikely'),
  3: t('components.heat_map.likelihood_value.possible'),
  4: t('components.heat_map.likelihood_value.likely'),
  5: t('components.heat_map.likelihood_value.almost_certain'),
}))

const impactLabels = computed(() => ({
  1: t('components.heat_map.impact_value.negligible'),
  2: t('components.heat_map.impact_value.minor'),
  3: t('components.heat_map.impact_value.moderate'),
  4: t('components.heat_map.impact_value.major'),
  5: t('components.heat_map.impact_value.severe'),
}))

// Only include items with valid likelihood and impact (1-5)
const validItems = computed(() =>
  props.items.filter(i => i.current_likelihood >= 1 && i.current_likelihood <= 5 && i.current_impact >= 1 && i.current_impact <= 5)
)

function cellItems(x, y) {
  return validItems.value.filter(i => i.current_impact === x && i.current_likelihood === y)
}

function cellCount(x, y) {
  return cellItems(x, y).length
}

// Maximum count in any cell, for opacity scaling
const maxCount = computed(() => {
  let max = 0
  for (let x = 1; x <= 5; x++) {
    for (let y = 1; y <= 5; y++) {
      const c = cellCount(x, y)
      if (c > max) max = c
    }
  }
  return max
})

function riskColor(score) {
  if (score >= 16) return '#ef4444'
  if (score >= 10) return '#f97316'
  if (score >= 5) return '#f59e0b'
  return '#22c55e'
}

function riskLabel(score) {
  if (score >= 16) return 'Critical'
  if (score >= 10) return 'High'
  if (score >= 5) return 'Medium'
  return 'Low'
}

function cellStyle(x, y) {
  const score = x * y
  const color = riskColor(score)
  const count = cellCount(x, y)

  // Base opacity: 0.15 for empty, scale up to 0.85 based on count
  let opacity = 0.15
  if (count > 0 && maxCount.value > 0) {
    opacity = 0.25 + (count / maxCount.value) * 0.6
  }

  return {
    backgroundColor: color,
    opacity: opacity,
  }
}

const distribution = computed(() => {
  const d = { low: 0, medium: 0, high: 0, critical: 0 }
  for (const item of validItems.value) {
    const score = item.current_likelihood * item.current_impact
    if (score >= 16) d.critical++
    else if (score >= 10) d.high++
    else if (score >= 5) d.medium++
    else d.low++
  }
  return d
})
</script>
