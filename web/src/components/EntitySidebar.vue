<template>
  <div>
    <!-- Toggle buttons (hidden when forceTab is set) -->
    <div v-if="!forceTab" class="flex items-center gap-1 border-b border-slate-800 pb-2 mb-2">
      <button @click="toggle('suggestions')"
        class="flex items-center gap-1 px-2 py-1 rounded text-[10px] font-medium transition-colors"
        :class="activeTab === 'suggestions' ? 'bg-amber-500/15 text-amber-400' : 'text-slate-600 hover:text-slate-400'">
        Suggestions
        <span v-if="suggestionCount > 0" class="px-1 py-0.5 rounded-full bg-amber-500/15 text-amber-400 text-[9px]">{{ suggestionCount }}</span>
      </button>
      <button @click="toggle('comments')"
        class="flex items-center gap-1 px-2 py-1 rounded text-[10px] font-medium transition-colors"
        :class="activeTab === 'comments' ? 'bg-blue-500/15 text-blue-400' : 'text-slate-600 hover:text-slate-400'">
        Comments
        <span v-if="commentCount > 0" class="px-1 py-0.5 rounded-full bg-blue-500/15 text-blue-400 text-[9px]">{{ commentCount }}</span>
      </button>
      <button @click="toggle('history')"
        class="flex items-center gap-1 px-2 py-1 rounded text-[10px] font-medium transition-colors"
        :class="activeTab === 'history' ? 'bg-slate-500/15 text-slate-400' : 'text-slate-600 hover:text-slate-400'">
        History
      </button>
    </div>

    <!-- Suggestions tab -->
    <div v-if="activeTab === 'suggestions'">
      <SuggestionPanel
        :entityType="entityType"
        :entityId="entityId"
        :canReview="canReview"
        @applied="$emit('updated')"
      />
    </div>

    <!-- Comments tab -->
    <div v-if="activeTab === 'comments'" class="space-y-3">
      <!-- Comment list -->
      <div v-if="comments.length === 0 && !commentLoading" class="text-[11px] text-slate-600 py-2">No comments yet.</div>
      <div v-for="c in comments" :key="c.id" class="space-y-1">
        <div class="text-[11px] text-slate-400" v-html="renderMention(c.body)"></div>
        <div class="text-[10px] text-slate-600">{{ c.author }} &middot; {{ formatDate(c.created_at) }}</div>
      </div>

      <!-- Add comment -->
      <div class="space-y-1.5">
        <MentionTextarea v-model="newComment" :members="members"
          class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-[11px] text-white focus:outline-none focus:border-blue-500 resize-none"
          placeholder="Add a comment... (type @ to mention)"
          rows="2"
          @keydown.meta.enter="addComment"
          @keydown.ctrl.enter="addComment" />
        <div class="flex justify-end">
          <button @click="addComment" :disabled="!newComment.trim()"
            class="text-[10px] px-2 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white rounded font-medium">Post</button>
        </div>
      </div>
    </div>

    <!-- History tab -->
    <div v-if="activeTab === 'history'">
      <HistoryPanel :entityType="entityType" :entityId="entityId" />
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { api } from '../api'
import SuggestionPanel from './SuggestionPanel.vue'
import HistoryPanel from './HistoryPanel.vue'
import MentionTextarea from './MentionTextarea.vue'
import { useMembers } from '../composables/useMembers'

const { members } = useMembers()

function renderMention(body) {
  if (!body) return ''
  const escaped = body.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  return escaped.replace(/@([\w.+-]+@[\w.-]+)/g, '<span class="text-blue-400 font-medium">@$1</span>')
    .replace(/@(\w+)/g, '<span class="text-blue-400 font-medium">@$1</span>')
}

const props = defineProps({
  entityType: { type: String, required: true },
  entityId: { type: String, required: true },
  canReview: { type: Boolean, default: false },
  forceTab: { type: String, default: '' }, // 'suggestions', 'comments', 'history' — skip toggle, show directly
})

const emit = defineEmits(['updated'])

const activeTab = ref(props.forceTab || null)

const comments = ref([])
const commentLoading = ref(false)
const newComment = ref('')
const history = ref([])
const suggestions = ref([])

const suggestionCount = computed(() => suggestions.value.filter(s => s.status === 'open' || s.status === 'in_review').length)
const commentCount = computed(() => comments.value.length)

function toggle(tab) {
  activeTab.value = activeTab.value === tab ? null : tab
  if (activeTab.value === 'comments' && comments.value.length === 0) loadComments()
  if (activeTab.value === 'history' && history.value.length === 0) loadHistory()
  if (activeTab.value === 'suggestions' && suggestions.value.length === 0) loadSuggestions()
}

async function loadComments() {
  commentLoading.value = true
  try {
    const data = await api.getEntityComments(props.entityType, props.entityId)
    comments.value = Array.isArray(data) ? data : (Array.isArray(data?.data) ? data.data : [])
  } catch { comments.value = [] }
  commentLoading.value = false
}

async function addComment() {
  if (!newComment.value.trim()) return
  try {
    await api.createEntityComment({
      entity_type: props.entityType,
      entity_id: props.entityId,
      body: newComment.value,
    })
    newComment.value = ''
    await loadComments()
  } catch {}
}

async function loadHistory() {
  try {
    const data = await api.getEntityChangelog(props.entityType, props.entityId)
    history.value = Array.isArray(data) ? data : (Array.isArray(data?.data) ? data.data : [])
  } catch { history.value = [] }
}

async function loadSuggestions() {
  try {
    const data = await api.getSuggestions({ entity_type: props.entityType, entity_id: props.entityId })
    suggestions.value = Array.isArray(data) ? data : (Array.isArray(data?.data) ? data.data : [])
  } catch { suggestions.value = [] }
}

function formatDate(d) {
  if (!d && d !== 0) return ''
  const dt = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  return dt.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })
}

onMounted(() => {
  loadSuggestions() // preload count for badge
  if (props.forceTab === 'comments') loadComments()
  if (props.forceTab === 'history') loadHistory()
})

watch(() => props.entityId, () => {
  comments.value = []
  history.value = []
  suggestions.value = []
  if (activeTab.value) {
    if (activeTab.value === 'comments') loadComments()
    if (activeTab.value === 'history') loadHistory()
    if (activeTab.value === 'suggestions') loadSuggestions()
  } else {
    loadSuggestions() // just for count badge
  }
})
</script>
