<template>
  <div class="space-y-4">
    <!-- List -->
    <div v-if="loading" class="h-10 bg-slate-800 rounded animate-pulse" />
    <div v-else-if="comments.length === 0" class="text-sm text-slate-600 italic py-2">{{ $t('components.comments_panel.empty') }}</div>
    <div v-else class="space-y-3">
      <div v-for="c in comments" :key="c.id" class="bg-slate-800/40 border border-slate-700/40 rounded-lg px-4 py-3">
        <div class="text-sm text-slate-300" v-html="renderMention(c.body)"></div>
        <div class="flex items-center justify-between mt-1.5">
          <div class="text-[10px] text-slate-600"><span :title="c.author">{{ nameFor(c.author) }}</span> · {{ formatDate(c.created_at) }}</div>
          <button v-if="canDelete" @click="deleteComment(c)"
            class="text-[10px] text-slate-500 hover:text-red-400 transition-colors">{{ $t('components.comments_panel.delete') }}</button>
        </div>
      </div>
    </div>

    <!-- Add -->
    <div v-if="canComment" class="border-t border-slate-800 pt-4 space-y-2">
      <MentionTextarea v-model="newComment" :members="members"
        class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500 resize-none"
        :placeholder="$t('components.comments_panel.placeholder')"
        rows="3"
        @keydown.meta.enter="addComment"
        @keydown.ctrl.enter="addComment" />
      <div class="flex justify-end">
        <button @click="addComment" :disabled="!newComment.trim()"
          class="text-xs px-3 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white rounded-lg font-medium transition-colors">{{ $t('components.comments_panel.post') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { api } from '../api'
import MentionTextarea from './MentionTextarea.vue'
import { useMembers } from '../composables/useMembers'
import { renderMention } from '../composables/useMention'
import { useSession } from '../composables/useSession'
import { useToast } from '../composables/useToast'
import { formatDate } from '../composables/useFormat.js'
import { useI18n } from 'vue-i18n'
import { renderApiError } from '../composables/useApiError.js'
import { useConfirm } from '../composables/useConfirm'

const { t } = useI18n()


const { show: showError } = useToast()
const { ask } = useConfirm()

const { members, nameFor } = useMembers()

// Commenting on a register entity is a contributor-and-above write; readers are
// read-only (#23). Same rule and same reason as SuggestNewButton.vue — hide the
// composer rather than let it 403 on submit.
const { currentUserData } = useSession()
const canComment = computed(() => (currentUserData.value?.role || '') !== 'reader')
// Deleting a register comment is admin/manager only, matching
// DELETE /entity-comments/:id (#403).
const canDelete = computed(() => ['admin', 'manager'].includes(currentUserData.value?.role || ''))

const props = defineProps({
  entityType: { type: String, required: true },
  entityId: { type: String, required: true },
})

const comments = ref([])
const loading = ref(false)
const newComment = ref('')

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
    showError(t('components.comments_panel.error_add', { message: renderApiError(e) || t('common.state.unknown') }))
  }
}

async function deleteComment(c) {
  if (!await ask(t('components.comments_panel.delete_confirm'))) return
  try {
    await api.deleteEntityComment(c.id)
  } catch (e) {
    showError(t('components.comments_panel.error_delete', { message: renderApiError(e) || t('common.state.unknown') }))
  }
  await loadComments()
}

onMounted(loadComments)
watch(() => props.entityId, loadComments)
</script>
