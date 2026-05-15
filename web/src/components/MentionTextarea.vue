<template>
  <div class="mention-textarea-wrapper relative">
    <textarea
      ref="textareaRef"
      :value="modelValue"
      @input="onInput"
      @keydown="onKeydown"
      v-bind="$attrs"
    />
    <!-- Mentions dropdown -->
    <div v-if="showMentions && filteredMembers.length > 0"
      class="absolute z-50 bg-slate-900 border border-slate-700 rounded-lg shadow-xl max-h-40 overflow-y-auto"
      :style="dropdownStyle">
      <button v-for="(m, i) in filteredMembers" :key="m.email || m.id"
        @mousedown.prevent="selectMention(m)"
        class="w-full text-left px-3 py-1.5 text-xs hover:bg-slate-700 transition-colors flex items-center gap-2"
        :class="i === activeIndex ? 'bg-slate-700 text-white' : 'text-slate-300'">
        <span class="w-5 h-5 rounded-full bg-blue-600 text-white text-[10px] font-bold flex items-center justify-center flex-shrink-0">
          {{ (m.name || m.email || '?').charAt(0).toUpperCase() }}
        </span>
        <span class="truncate">{{ m.name || m.email?.split('@')[0] }}</span>
        <span class="text-slate-500 text-[10px] ml-auto flex-shrink-0">{{ m.email }}</span>
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, nextTick } from 'vue'

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

function onInput(e) {
  const val = e.target.value
  emit('update:modelValue', val)
  checkForMention(e.target)
}

function checkForMention(el) {
  const cursor = el.selectionStart
  const text = el.value
  // Walk backwards from cursor to find @
  let atPos = -1
  for (let i = cursor - 1; i >= 0; i--) {
    const ch = text[i]
    if (ch === '@') {
      // Only trigger if @ is at start or preceded by whitespace
      if (i === 0 || /\s/.test(text[i - 1])) {
        atPos = i
      }
      break
    }
    if (/\s/.test(ch)) break
  }

  if (atPos >= 0) {
    mentionStart.value = atPos
    mentionQuery.value = text.substring(atPos + 1, cursor)
    showMentions.value = true
    activeIndex.value = 0
  } else {
    showMentions.value = false
  }
}

function onKeydown(e) {
  if (!showMentions.value || filteredMembers.value.length === 0) return

  if (e.key === 'ArrowDown') {
    e.preventDefault()
    activeIndex.value = (activeIndex.value + 1) % filteredMembers.value.length
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    activeIndex.value = (activeIndex.value - 1 + filteredMembers.value.length) % filteredMembers.value.length
  } else if (e.key === 'Enter' && !e.shiftKey && !e.metaKey && !e.ctrlKey) {
    e.preventDefault()
    selectMention(filteredMembers.value[activeIndex.value])
  } else if (e.key === 'Escape') {
    showMentions.value = false
  }
}

function selectMention(member) {
  const el = textareaRef.value
  if (!el) return
  const text = el.value
  const before = text.substring(0, mentionStart.value)
  const after = text.substring(el.selectionStart)
  const mention = `@${member.email} `
  const newVal = before + mention + after
  emit('update:modelValue', newVal)
  showMentions.value = false

  nextTick(() => {
    if (textareaRef.value) {
      const pos = before.length + mention.length
      textareaRef.value.selectionStart = pos
      textareaRef.value.selectionEnd = pos
      textareaRef.value.focus()
    }
  })
}

defineExpose({ focus: () => textareaRef.value?.focus(), el: textareaRef })
</script>
