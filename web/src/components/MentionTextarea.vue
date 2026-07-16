<template>
  <div class="mention-textarea-wrapper relative">
    <textarea
      ref="textareaRef"
      :value="modelValue"
      @input="onInput"
      @keydown="onKeydown"
      v-bind="$attrs"
    />
    <!-- Autocomplete dropdown: @ mentions a user, # references an entity -->
    <div v-if="showMentions && items.length > 0"
      class="absolute z-50 bg-slate-900 border border-slate-700 rounded-lg shadow-xl max-h-40 overflow-y-auto"
      :style="dropdownStyle">
      <button v-for="(m, i) in items" :key="mentionMode === 'user' ? (m.email || m.id) : (m.type + ':' + m.id)"
        @mousedown.prevent="selectItem(m)"
        class="w-full text-left px-3 py-1.5 text-xs hover:bg-slate-700 transition-colors flex items-center gap-2"
        :class="i === activeIndex ? 'bg-slate-700 text-white' : 'text-slate-300'">
        <template v-if="mentionMode === 'user'">
          <span class="w-5 h-5 rounded-full bg-blue-600 text-white text-[10px] font-bold flex items-center justify-center flex-shrink-0">
            {{ (m.name || m.email || '?').charAt(0).toUpperCase() }}
          </span>
          <span class="truncate">{{ m.name || m.email?.split('@')[0] }}</span>
          <span class="text-slate-500 text-[10px] ml-auto flex-shrink-0">{{ m.email }}</span>
        </template>
        <template v-else>
          <span class="text-[9px] font-mono uppercase tracking-wider text-blue-400 flex-shrink-0">{{ m.id }}</span>
          <span class="truncate">{{ m.title }}</span>
        </template>
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, nextTick } from 'vue'
import { api } from '../api'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  modelValue: { type: String, default: '' },
  members: { type: Array, default: () => [] },
})

const emit = defineEmits(['update:modelValue'])

const textareaRef = ref(null)
const showMentions = ref(false)
const mentionQuery = ref('')
const mentionStart = ref(-1)
const activeIndex = ref(0)
const mentionMode = ref('user') // 'user' (@) or 'entity' (#)
const entityResults = ref([])

const dropdownStyle = computed(() => {
  return { bottom: '100%', left: '0', right: '0', marginBottom: '4px' }
})

const filteredMembers = computed(() => {
  const q = mentionQuery.value.toLowerCase()
  return props.members.filter(m => {
    const name = (m.name || '').toLowerCase()
    const email = (m.email || '').toLowerCase()
    return name.includes(q) || email.includes(q)
  }).slice(0, 8)
})

// Unified list the dropdown/keyboard nav operate on.
const items = computed(() => mentionMode.value === 'user' ? filteredMembers.value : entityResults.value)

function onInput(e) {
  emit('update:modelValue', e.target.value)
  checkForMention(e.target)
}

function checkForMention(el) {
  const cursor = el.selectionStart
  const text = el.value
  // Walk backwards from the cursor to the nearest @ or # that starts a word.
  let pos = -1
  let mode = ''
  for (let i = cursor - 1; i >= 0; i--) {
    const ch = text[i]
    if (ch === '@' || ch === '#') {
      if (i === 0 || /\s/.test(text[i - 1])) {
        pos = i
        mode = ch === '@' ? 'user' : 'entity'
      }
      break
    }
    if (/\s/.test(ch)) break
  }

  if (pos >= 0) {
    mentionStart.value = pos
    mentionMode.value = mode
    mentionQuery.value = text.substring(pos + 1, cursor)
    showMentions.value = true
    activeIndex.value = 0
    if (mode === 'entity') searchEntities(mentionQuery.value)
  } else {
    showMentions.value = false
  }
}

let searchTimer = null
function searchEntities(q) {
  clearTimeout(searchTimer)
  if (!q) { entityResults.value = []; return }
  searchTimer = setTimeout(async () => {
    try {
      const res = await api.search(q)
      const data = (res && (res.data || res)) || []
      entityResults.value = data.slice(0, 8)
    } catch { entityResults.value = [] }
  }, 200)
}

function onKeydown(e) {
  if (!showMentions.value || items.value.length === 0) return

  if (e.key === 'ArrowDown') {
    e.preventDefault()
    activeIndex.value = (activeIndex.value + 1) % items.value.length
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    activeIndex.value = (activeIndex.value - 1 + items.value.length) % items.value.length
  } else if (e.key === 'Enter' && !e.shiftKey && !e.metaKey && !e.ctrlKey) {
    e.preventDefault()
    selectItem(items.value[activeIndex.value])
  } else if (e.key === 'Escape') {
    showMentions.value = false
  }
}

function selectItem(item) {
  const el = textareaRef.value
  if (!el || !item) return
  const text = el.value
  const before = text.substring(0, mentionStart.value)
  const after = text.substring(el.selectionStart)
  // @ inserts the user's email; # inserts the entity identifier (e.g. TASK-6),
  // which renderMention turns into a deep-link.
  const token = mentionMode.value === 'user' ? `@${item.email} ` : `#${item.id} `
  const newVal = before + token + after
  emit('update:modelValue', newVal)
  showMentions.value = false

  nextTick(() => {
    if (textareaRef.value) {
      const pos = before.length + token.length
      textareaRef.value.selectionStart = pos
      textareaRef.value.selectionEnd = pos
      textareaRef.value.focus()
    }
  })
}

defineExpose({ focus: () => textareaRef.value?.focus(), el: textareaRef })
</script>
