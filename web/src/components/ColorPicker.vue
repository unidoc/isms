<template>
  <div class="color-picker-panel" @mousedown.stop @click.stop>
    <!-- Saturation/Brightness square -->
    <div
      ref="satPanel"
      class="sat-panel"
      :style="{ background: `hsl(${hue}, 100%, 50%)` }"
      @mousedown="startSatDrag"
    >
      <div class="sat-overlay-white" />
      <div class="sat-overlay-black" />
      <div class="sat-cursor" :style="{ left: (sat * 100) + '%', top: ((1 - val) * 100) + '%' }" />
    </div>

    <!-- Hue bar -->
    <div ref="hueBar" class="hue-bar" @mousedown="startHueDrag">
      <div class="hue-cursor" :style="{ left: (hue / 360 * 100) + '%' }" />
    </div>

    <!-- Current color preview + hex input -->
    <div class="flex items-center gap-2 mt-2">
      <div class="w-8 h-8 rounded border border-slate-600 flex-shrink-0" :style="{ backgroundColor: hexValue }" />
      <input
        v-model="hexInput"
        @input="onHexInput"
        @keydown.enter="$emit('apply')"
        class="flex-1 px-2 py-1.5 bg-slate-900 border border-slate-700 rounded text-xs text-slate-200 font-mono placeholder-slate-600 focus:outline-none focus:border-blue-500"
        placeholder="#000000"
      />
    </div>

    <!-- Preset quick-picks -->
    <div class="flex gap-1 flex-wrap mt-2">
      <button
        v-for="color in presets"
        :key="color"
        @click="setFromHex(color)"
        class="w-5 h-5 rounded-full border-2 transition-all hover:scale-110"
        :class="hexValue.toLowerCase() === color.toLowerCase() ? 'border-white ring-1 ring-white/30' : 'border-slate-600'"
        :style="{ backgroundColor: color }"
        :title="color"
      />
      <button
        @click="setFromHex(''); $emit('update:modelValue', '')"
        class="w-5 h-5 rounded-full border-2 border-slate-600 flex items-center justify-center hover:scale-110 transition-all"
        title="Remove color"
      >
        <svg class="w-3 h-3 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
        </svg>
      </button>
    </div>

    <!-- OK button -->
    <button @click="$emit('apply')" class="w-full mt-2 px-3 py-1.5 bg-blue-600 hover:bg-blue-500 text-white text-xs font-medium rounded transition-colors">
      OK
    </button>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'

const props = defineProps({
  modelValue: { type: String, default: '#3b82f6' },
})
const emit = defineEmits(['update:modelValue', 'apply'])

const presets = [
  '#ef4444', '#fca5a5', '#f59e0b', '#fcd34d',
  '#22c55e', '#86efac', '#3b82f6', '#93c5fd',
  '#8b5cf6', '#6b7280', '#d1d5db', '#ffffff',
]

const hue = ref(210)
const sat = ref(0.7)
const val = ref(0.8)
const hexInput = ref('')
const satPanel = ref(null)
const hueBar = ref(null)

const hexValue = computed(() => hsvToHex(hue.value, sat.value, val.value))

watch(hexValue, (v) => {
  hexInput.value = v
  emit('update:modelValue', v)
})

onMounted(() => {
  if (props.modelValue) setFromHex(props.modelValue)
  hexInput.value = hexValue.value
})

watch(() => props.modelValue, (v) => {
  if (v && v !== hexValue.value) setFromHex(v)
})

function setFromHex(hex) {
  if (!hex) { hue.value = 0; sat.value = 0; val.value = 0.5; return }
  const rgb = hexToRgb(hex)
  if (!rgb) return
  const hsv = rgbToHsv(rgb.r, rgb.g, rgb.b)
  hue.value = hsv.h
  sat.value = hsv.s
  val.value = hsv.v
}

function onHexInput() {
  const v = hexInput.value.trim()
  if (/^#[0-9a-fA-F]{6}$/.test(v)) {
    setFromHex(v)
  }
}

// --- Saturation drag ---
let satDragging = false
function startSatDrag(e) {
  satDragging = true
  updateSat(e)
  document.addEventListener('mousemove', onSatMove)
  document.addEventListener('mouseup', stopSatDrag)
}
function onSatMove(e) { if (satDragging) updateSat(e) }
function stopSatDrag() {
  satDragging = false
  document.removeEventListener('mousemove', onSatMove)
  document.removeEventListener('mouseup', stopSatDrag)
}
function updateSat(e) {
  const rect = satPanel.value.getBoundingClientRect()
  sat.value = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
  val.value = Math.max(0, Math.min(1, 1 - (e.clientY - rect.top) / rect.height))
}

// --- Hue drag ---
let hueDragging = false
function startHueDrag(e) {
  hueDragging = true
  updateHue(e)
  document.addEventListener('mousemove', onHueMove)
  document.addEventListener('mouseup', stopHueDrag)
}
function onHueMove(e) { if (hueDragging) updateHue(e) }
function stopHueDrag() {
  hueDragging = false
  document.removeEventListener('mousemove', onHueMove)
  document.removeEventListener('mouseup', stopHueDrag)
}
function updateHue(e) {
  const rect = hueBar.value.getBoundingClientRect()
  hue.value = Math.max(0, Math.min(360, (e.clientX - rect.left) / rect.width * 360))
}

// --- Color conversion ---
function hsvToHex(h, s, v) {
  const c = v * s
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1))
  const m = v - c
  let r, g, b
  if (h < 60)      { r = c; g = x; b = 0 }
  else if (h < 120) { r = x; g = c; b = 0 }
  else if (h < 180) { r = 0; g = c; b = x }
  else if (h < 240) { r = 0; g = x; b = c }
  else if (h < 300) { r = x; g = 0; b = c }
  else              { r = c; g = 0; b = x }
  const toHex = (n) => Math.round((n + m) * 255).toString(16).padStart(2, '0')
  return '#' + toHex(r) + toHex(g) + toHex(b)
}

function hexToRgb(hex) {
  const m = hex.match(/^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i)
  if (!m) return null
  return { r: parseInt(m[1], 16), g: parseInt(m[2], 16), b: parseInt(m[3], 16) }
}

function rgbToHsv(r, g, b) {
  r /= 255; g /= 255; b /= 255
  const max = Math.max(r, g, b), min = Math.min(r, g, b)
  const d = max - min
  let h = 0
  if (d !== 0) {
    if (max === r) h = ((g - b) / d + 6) % 6
    else if (max === g) h = (b - r) / d + 2
    else h = (r - g) / d + 4
    h *= 60
  }
  const s = max === 0 ? 0 : d / max
  return { h, s, v: max }
}
</script>

<style scoped>
.color-picker-panel {
  width: 220px;
  padding: 0.5rem;
}
.sat-panel {
  position: relative;
  width: 100%;
  height: 140px;
  border-radius: 0.375rem;
  cursor: crosshair;
  overflow: hidden;
}
.sat-overlay-white {
  position: absolute;
  inset: 0;
  background: linear-gradient(to right, #fff, transparent);
}
.sat-overlay-black {
  position: absolute;
  inset: 0;
  background: linear-gradient(to bottom, transparent, #000);
}
.sat-cursor {
  position: absolute;
  width: 12px;
  height: 12px;
  border: 2px solid white;
  border-radius: 50%;
  box-shadow: 0 0 2px rgba(0,0,0,0.5);
  transform: translate(-50%, -50%);
  pointer-events: none;
}
.hue-bar {
  position: relative;
  width: 100%;
  height: 14px;
  margin-top: 0.5rem;
  border-radius: 7px;
  background: linear-gradient(to right, #f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00);
  cursor: pointer;
}
.hue-cursor {
  position: absolute;
  top: -1px;
  width: 10px;
  height: 16px;
  border: 2px solid white;
  border-radius: 3px;
  box-shadow: 0 0 3px rgba(0,0,0,0.4);
  transform: translateX(-50%);
  pointer-events: none;
}
</style>
