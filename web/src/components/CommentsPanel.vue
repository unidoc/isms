<template>
  <div class="space-y-4">
    <!-- List -->
    <div v-if="loading" class="h-10 bg-slate-800 rounded animate-pulse" />
    <div v-else-if="comments.length === 0" class="text-sm text-slate-600 italic py-2">No comments yet.</div>
    <div v-else class="space-y-3">
      <div v-for="c in comments" :key="c.id" class="bg-slate-800/40 border border-slate-700/40 rounded-lg px-4 py-3">
        <div class="text-sm text-slate-300" v-html="renderMention(c.body)"></div>
        <div class="text-[10px] text-slate-600 mt-1.5">{{ c.author }} · {{ formatDate(c.created_at) }}</div>
      </div>
    </div>

    <!-- Add -->
    <div class="border-t border-slate-800 pt-4 space-y-2">
      <MentionTextarea v-model="newComment" :members="members"
        class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500 resize-none"
        placeholder="Add a comment... (type @ to mention)"
        rows="3"
        @keydown.meta.enter="addComment"
        @keydown.ctrl.enter="addComment" />
      <div class="flex justify-end">
        <button @click="addComment" :disabled="!newComment.trim()"
          class="text-xs px-3 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white rounded-lg font-medium transition-colors">Post</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { api } from '../api'
import MentionTextarea from './MentionTextarea.vue'
import { useMembers } from '../composables/useMembers'
import { renderMention } from '../composables/useMention'
import { useToast } from '../composables/useToast'

const { show: showError } = useToast()

const { members } = useMembers()

const props = defineProps({
  entityType: { type: String, required: true },
  entityId: { type: String, required: true },
})

const comments = ref([])
const loading = ref(false)
const newComment = ref('')

function formatDate(d) {
  if (!d && d !== 0) return ''
  const dt = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  return dt.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

async function loadComments() {
  loading.value = true
  try {
    const data = await api.getEntityComments(props.entityType, props.entityId)
    comments.value = Array.isArray(data) ? data : (Array.isArray(data?.data) ? data.data : [])
  } catch { comments.value = [] }
  loading.value = false
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
  } catch (e) {
    showError('Failed to add comment: ' + (e.message || 'unknown error'))
  }
}

onMounted(loadComments)
watch(() => props.entityId, loadComments)
</script>
