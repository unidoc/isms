<template>
  <div class="relative" ref="pickerRef">
    <div
      @click="open = true"
      class="w-full flex items-center gap-2 px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm cursor-pointer hover:border-slate-600 transition-colors"
      :class="{ 'border-blue-500': open }"
    >
      <!-- Selected member -->
      <template v-if="selected">
        <span class="text-slate-200 truncate flex-1">{{ selected.name || selected.email }}</span>
        <button @click.stop="clear" class="text-slate-500 hover:text-slate-300 flex-shrink-0">
          <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </template>
      <!-- Placeholder -->
      <span v-else class="text-slate-600 flex-1">{{ placeholder }}</span>
      <svg class="w-3.5 h-3.5 text-slate-600 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M8.25 15L12 18.75 15.75 15m-7.5-6L12 5.25 15.75 9" />
      </svg>
    </div>

    <!-- Dropdown -->
    <div
      v-if="open"
      class="absolute z-50 mt-1 w-full bg-slate-800 border border-slate-700 rounded-lg shadow-xl overflow-hidden"
    >
      <div class="p-2 border-b border-slate-700">
        <input
          ref="searchInput"
          v-model="search"
          type="text"
          placeholder="Search..."
          class="w-full px-2 py-1.5 bg-slate-900 border border-slate-700 rounded text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500"
          @keydown.escape="open = false"
        />
      </div>
      <div class="max-h-48 overflow-y-auto">
        <div v-if="filtered.length === 0" class="px-3 py-3 text-xs text-slate-600 text-center">No members found</div>
        <button
          v-for="member in filtered"
          :key="member.email"
          @click="select(member)"
          class="w-full px-3 py-2 text-left hover:bg-slate-700/50 transition-colors flex items-center gap-2"
        >
          <div class="flex-1 min-w-0">
            <div class="text-sm text-slate-200 truncate">{{ member.name || member.email }}</div>
            <div v-if="member.name" class="text-[10px] text-slate-500 truncate">{{ member.email }}</div>
          </div>
          <span class="text-[10px] px-1.5 py-0.5 rounded-full flex-shrink-0"
            :class="roleBadge(member.role)">
            {{ member.role }}
          </span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'

const props = defineProps({
  modelValue: { type: String, default: '' },
  members: { type: Array, default: () => [] },
  placeholder: { type: String, default: 'Select member...' },
})

const emit = defineEmits(['update:modelValue'])

const open = ref(false)
const search = ref('')
const pickerRef = ref(null)
const searchInput = ref(null)

const selected = computed(() => {
  if (!props.modelValue) return null
  return props.members.find(m => m.email === props.modelValue) || { email: props.modelValue, name: props.modelValue }
})

const filtered = computed(() => {
  const q = search.value.toLowerCase()
  return props.members.filter(m =>
    (m.name || '').toLowerCase().includes(q) ||
    (m.email || '').toLowerCase().includes(q)
  )
})

function select(member) {
  emit('update:modelValue', member.email)
  open.value = false
  search.value = ''
}

function clear() {
  emit('update:modelValue', '')
}

function roleBadge(role) {
  const map = {
    admin: 'bg-purple-500/20 text-purple-300',
    manager: 'bg-blue-500/20 text-blue-300',
    contributor: 'bg-emerald-500/20 text-emerald-300',
    reader: 'bg-slate-500/20 text-slate-400',
  }
  return map[role] || 'bg-slate-500/20 text-slate-400'
}

function onClickOutside(e) {
  if (pickerRef.value && !pickerRef.value.contains(e.target)) {
    open.value = false
  }
}

watch(open, (val) => {
  if (val) {
    search.value = ''
    nextTick(() => searchInput.value?.focus())
  }
})

onMounted(() => document.addEventListener('click', onClickOutside))
onBeforeUnmount(() => document.removeEventListener('click', onClickOutside))
</script>
