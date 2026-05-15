<template>
  <div class="flex flex-col h-full">
    <!-- Toggle header -->
    <button @click="expanded = !expanded"
      class="w-full px-5 py-3 border-b border-slate-800 flex items-center justify-between hover:bg-slate-800/30 transition-colors flex-shrink-0">
      <div class="flex items-center gap-2">
        <svg class="w-3.5 h-3.5 text-slate-500 transition-transform" :class="{ 'rotate-90': expanded }" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <h3 class="text-sm font-semibold text-slate-300">Comments</h3>
      </div>
      <div class="flex items-center gap-2">
        <span v-if="openCount > 0" class="text-xs font-bold bg-blue-600 text-white px-1.5 py-0.5 rounded-full">{{ openCount }}</span>
        <span v-if="resolvedCount > 0" class="text-xs text-slate-600">{{ resolvedCount }} resolved</span>
      </div>
    </button>

    <template v-if="expanded">
      <!-- Comment form -->
      <div class="px-5 py-4 border-b border-slate-800 space-y-3 flex-shrink-0">
        <MentionTextarea
          v-model="newComment"
          :members="members"
          :placeholder="placeholder || 'Add a comment... (type @ to mention)'"
          rows="3"
          class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500 focus:border-blue-500 resize-none"
        />
        <div class="flex items-center gap-2">
          <input
            v-model="author"
            placeholder="Your name"
            class="flex-1 bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500 focus:border-blue-500"
          />
          <button
            @click="submitComment"
            :disabled="!newComment.trim() || submitting"
            class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 disabled:text-slate-500 text-white text-sm font-medium rounded-lg transition-colors"
          >
            {{ submitting ? '...' : 'Send' }}
          </button>
        </div>
      </div>

      <!-- Comments list -->
      <div class="flex-1 overflow-y-auto">
        <div v-if="loading" class="p-5 text-xs text-slate-600">Loading...</div>
        <div v-else-if="comments.length === 0" class="p-5 text-xs text-slate-600">No comments yet</div>
        <template v-else>
          <!-- Open comments -->
          <div v-for="c in openComments" :key="c.id" class="px-5 py-4 border-b border-slate-800">
            <div class="flex items-center gap-2 mb-1.5">
              <div class="w-5 h-5 rounded-full bg-blue-600 flex items-center justify-center text-[10px] font-bold text-white flex-shrink-0">
                {{ (c.author || '?').charAt(0).toUpperCase() }}
              </div>
              <span class="text-xs font-semibold text-slate-300">{{ c.author }}</span>
              <span class="text-[10px] text-slate-600">{{ formatDate(c.created_at) }}</span>
              <button @click="startReply(c)"
                class="ml-auto text-[10px] text-slate-600 hover:text-blue-400 transition-colors">
                Reply
              </button>
              <button @click="resolveComment(c.id)"
                class="text-[10px] text-slate-600 hover:text-emerald-400 transition-colors">
                Resolve
              </button>
            </div>
            <div class="text-sm text-slate-400 leading-relaxed" v-html="sanitize(renderBody(c.body))" />

            <!-- Replies -->
            <div v-for="reply in repliesFor(c.id)" :key="reply.id" class="mt-2 ml-4 pl-3 border-l-2 border-slate-700">
              <div class="flex items-center gap-2 mb-0.5">
                <div class="w-4 h-4 rounded-full bg-slate-600 flex items-center justify-center text-[8px] font-bold text-white flex-shrink-0">
                  {{ (reply.author || '?').charAt(0).toUpperCase() }}
                </div>
                <span class="text-[10px] font-semibold text-slate-400">{{ reply.author }}</span>
                <span class="text-[10px] text-slate-600">{{ formatDate(reply.created_at) }}</span>
              </div>
              <div class="text-xs text-slate-400 leading-relaxed" v-html="sanitize(renderBody(reply.body))" />
            </div>

            <!-- Reply form -->
            <div v-if="replyingTo === c.id" class="mt-2 ml-4 pl-3 border-l-2 border-blue-600">
              <MentionTextarea
                v-model="replyText"
                :members="members"
                placeholder="Write a reply... (type @ to mention)"
                rows="2"
                class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500 resize-none"
                @keydown.meta.enter="submitReply"
                @keydown.ctrl.enter="submitReply"
              />
              <div class="flex gap-2 mt-1">
                <button @click="submitReply" :disabled="!replyText.trim()"
                  class="px-2 py-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-[10px] font-medium rounded">
                  Reply
                </button>
                <button @click="replyingTo = null; replyText = ''"
                  class="px-2 py-1 text-[10px] text-slate-500 hover:text-slate-300">
                  Cancel
                </button>
              </div>
            </div>
          </div>

          <!-- Resolved toggle -->
          <button v-if="resolvedCount > 0" @click="showResolved = !showResolved"
            class="w-full px-5 py-2 text-xs text-slate-600 hover:text-slate-400 border-b border-slate-800 flex items-center gap-1">
            <svg class="w-3 h-3 transition-transform" :class="{ 'rotate-90': showResolved }" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
            </svg>
            {{ resolvedCount }} resolved
          </button>
          <template v-if="showResolved">
            <div v-for="c in resolvedComments" :key="c.id" class="px-5 py-3 border-b border-slate-800 opacity-40">
              <div class="flex items-center gap-2 mb-1">
                <span class="text-xs font-semibold text-slate-400">{{ c.author }}</span>
                <span class="text-[10px] text-emerald-500 flex items-center gap-0.5">
                  <svg class="w-2.5 h-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                  Resolved
                </span>
              </div>
              <div class="text-xs text-slate-500" v-html="sanitize(renderBody(c.body))" />
            </div>
          </template>
        </template>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import DOMPurify from 'dompurify'
const sanitize = (html) => DOMPurify.sanitize(html, { ADD_ATTR: ['style'] })
import { api, getCurrentUser } from '../api'
import MentionTextarea from './MentionTextarea.vue'
import { useMembers } from '../composables/useMembers'

const { members } = useMembers()

const props = defineProps({
  documentId: { type: String, required: true },
  placeholder: { type: String, default: 'Add a comment...' }
})

const comments = ref([])
const loading = ref(false)
const newComment = ref('')
const author = ref(getCurrentUser())
const submitting = ref(false)
const expanded = ref(false)
const showResolved = ref(false)
const replyingTo = ref(null)
const replyText = ref('')

const topLevelComments = computed(() => comments.value.filter(c => !c.parent_id))
const openComments = computed(() => topLevelComments.value.filter(c => c.status === 'open'))
const resolvedComments = computed(() => topLevelComments.value.filter(c => c.status !== 'open'))
const openCount = computed(() => openComments.value.length)
const resolvedCount = computed(() => resolvedComments.value.length)

async function loadComments() {
  loading.value = true
  try {
    const data = await api.getDocComments(props.documentId)
    comments.value = Array.isArray(data) ? data : []
    if (openCount.value > 0) expanded.value = true
  } catch {
    comments.value = []
  } finally {
    loading.value = false
  }
}

async function submitComment() {
  if (!newComment.value.trim()) return
  submitting.value = true
  try {
    await api.addComment({
      document_id: props.documentId,
      author: author.value || getCurrentUser(),
      body: newComment.value,
    })
    newComment.value = ''
    await loadComments()
  } finally {
    submitting.value = false
  }
}

async function resolveComment(id) {
  await api.resolveComment(id, getCurrentUser())
  await loadComments()
}

function repliesFor(commentId) {
  return comments.value.filter(c => c.parent_id === commentId)
}

function startReply(comment) {
  replyingTo.value = comment.id
  replyText.value = ''
}

async function submitReply() {
  if (!replyText.value.trim() || !replyingTo.value) return
  try {
    await api.addComment({
      document_id: props.documentId,
      author: author.value || getCurrentUser(),
      body: replyText.value,
      parent_id: replyingTo.value,
    })
    replyText.value = ''
    replyingTo.value = null
    await loadComments()
  } catch (e) {
    console.error('Failed to submit reply:', e)
  }
}

function formatDate(d) {
  if (!d && d !== 0) return ''
  const date = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  const now = new Date()
  const diff = now - date
  if (diff < 60000) return 'just now'
  if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago'
  if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago'
  return date.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })
}

function renderBody(body) {
  if (!body) return ''
  const escaped = body.replace(/&/g, '&amp;').replace(/</g, '&lt;')
  return escaped.replace(/@([\w.+-]+@[\w.-]+)/g, '<span class="text-blue-400 font-medium">@$1</span>')
    .replace(/@(\w+)/g, '<span class="text-blue-400 font-medium">@$1</span>')
}

watch(() => props.documentId, () => {
  if (props.documentId) loadComments()
})

onMounted(() => {
  if (props.documentId) loadComments()
})
</script>
