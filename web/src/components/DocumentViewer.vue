<template>
  <div class="document-viewer relative">
    <!-- Loading -->
    <div v-if="!content" class="space-y-3 py-4">
      <div class="h-4 bg-slate-800 rounded w-3/4 animate-pulse" />
      <div class="h-4 bg-slate-800 rounded w-full animate-pulse" />
      <div class="h-4 bg-slate-800 rounded w-5/6 animate-pulse" />
      <div class="h-4 bg-slate-800 rounded w-2/3 animate-pulse" />
    </div>

    <!-- Rendered content blocks -->
    <div v-else-if="contentBlocks.length > 0" class="pr-12 relative" :class="showReviewProgress ? 'pl-8' : 'pl-0'">
      <div
        v-for="block in contentBlocks"
        :key="block.index"
        class="comment-block relative group transition-colors duration-200 rounded -mx-2 px-2"
        :class="{
          'bg-blue-950/20': hasOpenComments(block.index),
          'hover:bg-slate-900/40': !hasOpenComments(block.index),
          'review-checked': showReviewProgress && reviewedBlocks.has(block.index)
        }"
      >
        <!-- Review checkbox -->
        <button
          v-if="showReviewProgress"
          @click.stop="toggleReviewBlock(block.index)"
          class="review-checkbox absolute -left-7 top-1.5 w-5 h-5 rounded-full flex items-center justify-center transition-all duration-300 ease-out cursor-pointer z-10"
          :class="reviewedBlocks.has(block.index)
            ? 'bg-emerald-500 text-white scale-100 shadow-md shadow-emerald-900/40'
            : 'border border-slate-700 text-transparent opacity-0 group-hover:opacity-60 hover:!opacity-100 hover:border-slate-500'"
          :title="reviewedBlocks.has(block.index) ? 'Mark as unreviewed' : 'Mark as reviewed'"
        >
          <svg class="w-3 h-3 transition-transform duration-300" :class="reviewedBlocks.has(block.index) ? 'scale-100' : 'scale-0'" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3">
            <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        </button>

        <!-- Rendered content block -->
        <div v-html="sanitize(block.html)" class="doc-prose" />

        <!-- Comment count badge (visible when paragraph has open comments) -->
        <div
          v-if="showComments && hasOpenComments(block.index)"
          @click="toggleBlockComments(block.index)"
          class="absolute -right-10 top-1 w-6 h-6 rounded-full bg-blue-600 text-white text-xs font-bold flex items-center justify-center cursor-pointer hover:bg-blue-500 transition-colors shadow-lg shadow-blue-900/30"
          :title="commentCountForBlock(block.index) + ' comment' + (commentCountForBlock(block.index) === 1 ? '' : 's')"
        >
          {{ commentCountForBlock(block.index) }}
        </div>

        <!-- Add comment button (appears on hover) -->
        <button
          v-if="showComments && !hasOpenComments(block.index)"
          @click.stop="startInlineComment(block.index)"
          class="absolute -right-10 top-1 w-6 h-6 rounded-full bg-slate-700 text-slate-400 text-xs flex items-center justify-center cursor-pointer opacity-0 group-hover:opacity-100 transition-all duration-200 hover:bg-blue-600 hover:text-white hover:shadow-lg hover:shadow-blue-900/30 hover:scale-110"
          title="Add comment"
        >
          +
        </button>

        <!-- Inline comment thread (expanded below the block) -->
        <transition
          enter-active-class="transition-all duration-300 ease-out"
          enter-from-class="opacity-0 -translate-y-2 max-h-0"
          enter-to-class="opacity-100 translate-y-0 max-h-[2000px]"
          leave-active-class="transition-all duration-200 ease-in"
          leave-from-class="opacity-100 translate-y-0 max-h-[2000px]"
          leave-to-class="opacity-0 -translate-y-2 max-h-0"
        >
          <div v-if="showComments && expandedBlock === block.index" class="mt-1 mb-4 ml-4 border-l-2 border-blue-600/60 pl-4 overflow-hidden">
            <!-- Existing comments for this block -->
            <div
              v-for="comment in commentsForBlock(block.index)"
              :key="comment.id"
              class="mb-2 bg-slate-900/80 backdrop-blur rounded-lg p-3 border border-slate-800/50 transition-opacity duration-200"
              :class="{ 'opacity-40 border-emerald-900/30': comment.status === 'resolved' }"
            >
              <div class="flex items-center gap-2 mb-1">
                <span class="text-xs font-semibold text-slate-300">{{ comment.author }}</span>
                <span class="text-[10px] text-slate-600">{{ formatDate(comment.created_at) }}</span>
                <div v-if="comment.status === 'resolved'" class="ml-auto flex items-center gap-1">
                  <svg class="w-3 h-3 text-emerald-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                  <span class="text-[10px] text-emerald-500 font-medium">Resolved</span>
                </div>
                <button
                  v-else
                  @click="doResolveComment(comment.id)"
                  class="ml-auto text-[10px] text-slate-500 hover:text-emerald-400 transition-colors"
                >Resolve</button>
              </div>
              <!-- Resolved: collapsed -->
              <template v-if="comment.status === 'resolved'">
                <div class="text-[10px] text-emerald-500/70 flex items-center gap-1 mb-1">
                  Resolved by {{ comment.resolved_by || 'unknown' }} {{ formatDate(comment.resolved_at) }}
                </div>
                <div
                  v-if="!expandedResolved.has(comment.id)"
                  @click="toggleResolved(comment.id)"
                  class="text-sm text-slate-500 leading-relaxed truncate cursor-pointer hover:text-slate-400"
                >{{ firstLine(comment.body) }}</div>
                <div
                  v-else
                  @click="toggleResolved(comment.id)"
                  class="text-sm text-slate-500 leading-relaxed whitespace-pre-wrap cursor-pointer hover:text-slate-400"
                  v-html="sanitize(renderCommentBody(comment.body))"
                ></div>
              </template>
              <!-- Open: suggestion or regular body -->
              <template v-else>
                <!-- Suggestion display -->
                <div v-if="comment.suggestion_body" class="space-y-2">
                  <span class="inline-flex items-center text-[10px] font-bold px-2 py-0.5 rounded-full"
                    :class="{
                      'text-amber-300 bg-amber-800/40': comment.suggestion_status === 'pending',
                      'text-emerald-300 bg-emerald-800/40': comment.suggestion_status === 'accepted',
                      'text-red-300 bg-red-800/40': comment.suggestion_status === 'rejected',
                    }">
                    {{ comment.suggestion_status === 'pending' ? 'Suggested edit' : comment.suggestion_status === 'accepted' ? 'Accepted' : 'Rejected' }}
                  </span>
                  <div class="rounded bg-slate-950 border border-slate-800 p-2 text-xs font-mono">
                    <div v-if="comment.quote" class="text-red-400/60 line-through mb-1 pb-1 border-b border-slate-800">{{ comment.quote }}</div>
                    <div class="text-emerald-400/80">{{ comment.suggestion_body }}</div>
                  </div>
                  <div v-if="comment.suggestion_status === 'pending' && props.canAcceptSuggestions" class="flex gap-2">
                    <button @click="acceptSuggestion(comment.id)" class="text-[10px] px-2.5 py-1 rounded bg-emerald-700 hover:bg-emerald-600 text-white font-medium transition-colors">Accept</button>
                    <button @click="rejectSuggestion(comment.id)" class="text-[10px] px-2.5 py-1 rounded bg-slate-700 hover:bg-red-800 text-slate-300 hover:text-red-200 font-medium transition-colors">Reject</button>
                  </div>
                  <div v-else-if="comment.suggestion_status === 'pending'" class="text-[10px] text-amber-400/70">Awaiting author decision</div>
                  <div v-else class="text-[10px] text-slate-600">
                    {{ comment.suggestion_status }} by {{ comment.suggestion_resolved_by || 'unknown' }}
                  </div>
                </div>
                <!-- Regular comment body -->
                <div v-else class="text-sm text-slate-400 leading-relaxed whitespace-pre-wrap" v-html="sanitize(renderCommentBody(comment.body))"></div>
              </template>

              <!-- Replies -->
              <div v-if="repliesFor(comment.id).length > 0" class="mt-2 ml-3 border-l border-slate-800 pl-3 space-y-2">
                <div v-for="reply in repliesFor(comment.id)" :key="reply.id" class="bg-slate-950/60 rounded p-2 border border-slate-800/30">
                  <div class="flex items-center gap-2 mb-0.5">
                    <span class="text-[11px] font-semibold text-slate-400">{{ reply.author }}</span>
                    <span class="text-[10px] text-slate-600">{{ formatDate(reply.created_at) }}</span>
                  </div>
                  <div class="text-[12px] text-slate-400" v-html="sanitize(renderCommentBody(reply.body))"></div>
                </div>
              </div>

              <!-- Reply button -->
              <div v-if="comment.status !== 'resolved'" class="mt-1.5">
                <button
                  v-if="replyingTo !== comment.id"
                  @click="replyingTo = comment.id; replyText = ''"
                  class="text-[10px] text-slate-600 hover:text-blue-400 transition-colors"
                >Reply</button>
                <div v-else class="mt-1">
                  <MentionTextarea
                    v-model="replyText"
                    :members="members"
                    placeholder="Reply... (type @ to mention)"
                    class="w-full bg-transparent border border-slate-700 rounded-md p-2 text-sm text-slate-300 placeholder-slate-600 resize-none focus:outline-none focus:ring-1 focus:border-blue-500 focus:ring-blue-500/30"
                    rows="2"
                    @keydown.meta.enter="submitReply(comment.id)"
                    @keydown.ctrl.enter="submitReply(comment.id)"
                  />
                  <div class="flex justify-end gap-2 mt-1">
                    <button @click="replyingTo = null" class="text-xs text-slate-500 hover:text-slate-300 px-2 py-1 rounded transition-colors cursor-pointer">Cancel</button>
                    <button @click="submitReply(comment.id)" :disabled="!replyText.trim() || submittingReply" class="text-xs bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 disabled:text-slate-500 text-white px-3 py-1 rounded font-medium transition-colors cursor-pointer">
                      {{ submittingReply ? 'Saving...' : 'Reply' }}
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <!-- New comment / suggestion form -->
            <div class="backdrop-blur rounded-lg p-3 border relative transition-colors duration-200 bg-slate-900/80 border-slate-800/50">
              <!-- Mode toggle -->
              <div class="flex items-center gap-1 mb-2">
                <button @click="inlineMode = 'comment'"
                  class="text-[10px] px-2 py-0.5 rounded-full font-medium transition-colors"
                  :class="inlineMode === 'comment' ? 'bg-blue-600 text-white' : 'text-slate-500 hover:text-slate-300'">
                  Comment
                </button>
                <button v-if="props.reviewId" @click="inlineMode = 'suggestion'"
                  class="text-[10px] px-2 py-0.5 rounded-full font-medium transition-colors"
                  :class="inlineMode === 'suggestion' ? 'bg-amber-600 text-white' : 'text-slate-500 hover:text-slate-300'">
                  Suggest edit
                </button>
              </div>
              <MentionTextarea
                ref="inlineTextareaRef"
                v-model="inlineCommentText"
                :members="members"
                :placeholder="inlineMode === 'suggestion' ? 'Suggest replacement text for this paragraph...' : 'Add a comment... (type @ to mention)'"
                class="w-full bg-transparent border rounded-md p-2 text-sm text-slate-300 placeholder-slate-600 resize-none focus:outline-none focus:ring-1"
                :class="inlineMode === 'suggestion' ? 'border-amber-700/50 focus:border-amber-500 focus:ring-amber-500/30' : 'border-slate-700 focus:border-blue-500 focus:ring-blue-500/30'"
                rows="3"
                @keydown.meta.enter="submitInlineComment(expandedBlock)"
                @keydown.ctrl.enter="submitInlineComment(expandedBlock)"
              />
              <div class="flex items-center justify-end mt-2">
                <div class="flex gap-2">
                  <button
                    @click="expandedBlock = null; inlineCommentText = ''; inlineMode = 'comment'"
                    class="text-xs text-slate-500 hover:text-slate-300 px-2 py-1 rounded transition-colors cursor-pointer"
                  >Cancel</button>
                  <button
                    @click="submitInlineComment(expandedBlock)"
                    :disabled="!inlineCommentText.trim() || submittingInline"
                    class="text-xs text-white px-3 py-1 rounded font-medium transition-colors cursor-pointer disabled:bg-slate-700 disabled:text-slate-500"
                    :class="inlineMode === 'suggestion' ? 'bg-amber-600 hover:bg-amber-500' : 'bg-blue-600 hover:bg-blue-500'"
                  >{{ submittingInline ? 'Saving...' : inlineMode === 'suggestion' ? 'Suggest' : 'Comment' }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </transition>
      </div>
    </div>

    <!-- No content -->
    <div v-else class="py-8 text-center text-sm text-slate-600">
      No content available.
    </div>
  </div>
</template>

<script setup>
import { ref, computed, reactive, watch, nextTick, onMounted } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { api, getCurrentUser } from '../api'
import MentionTextarea from './MentionTextarea.vue'
import { useMembers } from '../composables/useMembers'

const { members } = useMembers()

marked.setOptions({ breaks: true })

const sanitize = (html) => DOMPurify.sanitize(html, { ADD_ATTR: ['style'] })

const props = defineProps({
  content: { type: String, default: '' },
  documentId: { type: String, default: '' },
  reviewId: { type: Number, default: null },
  editable: { type: Boolean, default: false },
  showComments: { type: Boolean, default: true },
  showReviewProgress: { type: Boolean, default: false },
  canAcceptSuggestions: { type: Boolean, default: false },
})

const emit = defineEmits(['comment', 'resolve', 'suggestion-accepted'])

// --- Paragraph-level content blocks ---
const contentBlocks = computed(() => {
  if (!props.content) return []
  const html = marked.parse(props.content)
  const div = document.createElement('div')
  div.innerHTML = html
  const blocks = []

  function addBlock(html, tag, text) {
    blocks.push({ index: blocks.length, html, tag, text })
  }

  for (const child of div.children) {
    const tag = child.tagName.toLowerCase()

    // Split lists -- each bullet is commentable
    if ((tag === 'ul' || tag === 'ol') && child.children.length > 0) {
      for (const li of child.children) {
        if (li.tagName.toLowerCase() === 'li') {
          addBlock('<' + tag + '>' + li.outerHTML + '</' + tag + '>', 'li', li.textContent || '')
        }
      }
    }
    // Tables -- convert to grid-based rows so each gets a "+" button
    else if (tag === 'table') {
      const thead = child.querySelector('thead')
      const tbody = child.querySelector('tbody')
      const ths = thead ? Array.from(thead.querySelectorAll('th')) : []
      const rows = tbody ? Array.from(tbody.querySelectorAll('tr')) : []
      const colCount = ths.length || (rows[0] ? rows[0].children.length : 1)
      const gridCols = 'grid-template-columns: ' + Array(colCount).fill('1fr').join(' ') + ';'

      if (ths.length > 0) {
        const headerCells = ths.map(th => {
          const styleAttr = th.getAttribute('style') || ''
          return `<div class="tbl-hdr-cell" style="${styleAttr}">${th.innerHTML}</div>`
        }).join('')
        addBlock(`<div class="tbl-grid" style="${gridCols}">${headerCells}</div>`, 'thead', thead.textContent || '')
      }

      for (const tr of rows) {
        const tds = Array.from(tr.querySelectorAll('td'))
        const cells = tds.map(td => {
          const styleAttr = td.getAttribute('style') || ''
          return `<div class="tbl-cell" style="${styleAttr}">${td.innerHTML}</div>`
        }).join('')
        addBlock(`<div class="tbl-grid tbl-row" style="${gridCols}">${cells}</div>`, 'tr', tr.textContent || '')
      }
    }
    // Split blockquotes -- each paragraph inside is commentable
    else if (tag === 'blockquote' && child.children.length > 1) {
      for (const bqChild of child.children) {
        addBlock('<blockquote>' + bqChild.outerHTML + '</blockquote>', 'blockquote-p', bqChild.textContent || '')
      }
    }
    // Everything else -- headings, paragraphs, pre, hr -- one block each
    else {
      addBlock(child.outerHTML, tag, child.textContent || '')
    }
  }
  return blocks
})

// --- Comments ---
const comments = ref([])
const loadingComments = ref(false)
const expandedBlock = ref(null)
const inlineCommentText = ref('')
const inlineMode = ref('comment') // 'comment' | 'suggestion'

// Autosave inline draft to localStorage
watch(inlineCommentText, (val) => {
  if (expandedBlock.value == null) return
  const key = inlineDraftKey(expandedBlock.value)
  if (!key) return
  if (val && val.trim()) {
    localStorage.setItem(key, val)
    localStorage.setItem(key + '_mode', inlineMode.value)
  } else {
    localStorage.removeItem(key)
    localStorage.removeItem(key + '_mode')
  }
})
const submittingInline = ref(false)
const inlineTextareaRef = ref(null)
const expandedResolved = reactive(new Set())
const replyingTo = ref(null)
const replyText = ref('')
const submittingReply = ref(false)

async function loadComments() {
  if (!props.documentId) return
  loadingComments.value = true
  try {
    // If in review context, fetch only review-scoped comments
    const reviewParam = props.reviewId ? `?review_id=${props.reviewId}` : ''
    const data = await api.fetchJSON(`/api/v1/documents/${encodeURIComponent(props.documentId)}/comments${reviewParam}`)
    comments.value = Array.isArray(data?.data || data) ? (data?.data || data) : []
  } catch {
    comments.value = []
  } finally {
    loadingComments.value = false
  }
}

// Simple hash for paragraph content — used for stable comment anchoring
function hashBlock(index) {
  const block = contentBlocks.value[index]
  if (!block) return ''
  const text = block.raw || block.html || ''
  // djb2 hash
  let hash = 5381
  for (let i = 0; i < text.length; i++) {
    hash = ((hash << 5) + hash + text.charCodeAt(i)) & 0xffffffff
  }
  return hash.toString(36)
}

// Build a map from paragraph_hash → current block index for relocated comments
const hashToIndex = computed(() => {
  const map = {}
  for (let i = 0; i < contentBlocks.value.length; i++) {
    const h = hashBlock(i)
    if (h) map[h] = i
  }
  return map
})

function commentsForBlock(index) {
  const h = hashBlock(index)
  return comments.value.filter(c => {
    // Match by hash + index for disambiguation (handles duplicate paragraphs)
    if (c.paragraph_hash && h) {
      if (c.paragraph_hash !== h) return false
      // If hash matches but index also exists, use index as tiebreaker
      if (c.paragraph_index != null) return c.paragraph_index === index
      return true
    }
    return c.paragraph_index === index
  })
}

function hasOpenComments(index) {
  return commentsForBlock(index).some(c => c.status !== 'resolved')
}

function commentCountForBlock(index) {
  return commentsForBlock(index).filter(c => c.status !== 'resolved').length
}

function toggleBlockComments(index) {
  if (expandedBlock.value === index) {
    expandedBlock.value = null
    inlineCommentText.value = ''
  } else {
    expandedBlock.value = index
    recoverInlineDraft(index)
    focusInlineTextarea()
  }
}

function inlineDraftKey(index) {
  return props.reviewId ? `isms_inline_draft_${props.reviewId}_${index}` : null
}

function recoverInlineDraft(index) {
  const key = inlineDraftKey(index)
  const saved = key ? localStorage.getItem(key) : null
  if (saved && saved.trim()) {
    inlineCommentText.value = saved
    inlineMode.value = localStorage.getItem(key + '_mode') || 'comment'
  } else {
    inlineCommentText.value = ''
    inlineMode.value = 'comment'
    // Clear stale empty draft
    if (key) { localStorage.removeItem(key); localStorage.removeItem(key + '_mode') }
  }
}

function startInlineComment(index) {
  expandedBlock.value = index
  recoverInlineDraft(index)
  focusInlineTextarea()
}

function focusInlineTextarea() {
  nextTick(() => {
    if (inlineTextareaRef.value) {
      const el = Array.isArray(inlineTextareaRef.value) ? inlineTextareaRef.value[0] : inlineTextareaRef.value
      if (el?.focus) el.focus()
    }
  })
}

function extractQuote(blockIndex) {
  const block = contentBlocks.value.find(b => b.index === blockIndex)
  if (!block) return ''
  const text = (block.text || '').trim()
  return text.length > 200 ? text.substring(0, 200) + '...' : text
}

async function submitInlineComment(blockIndex) {
  if (!inlineCommentText.value.trim() || submittingInline.value) return
  submittingInline.value = true
  try {
    const text = inlineCommentText.value.trim()
    const isSuggestion = inlineMode.value === 'suggestion'
    const comment = {
      document_id: props.documentId,
      author: getCurrentUser(),
      body: isSuggestion ? 'Suggested replacement for this paragraph' : text,
      paragraph_index: blockIndex,
      paragraph_hash: hashBlock(blockIndex),
      quote: extractQuote(blockIndex),
    }
    if (isSuggestion) comment.suggestion_body = text
    if (props.reviewId) {
      // Use review-specific endpoint which properly sets suggestion_status = 'pending'
      await api.addReviewComment(props.reviewId, comment)
    } else {
      await api.addComment(comment)
    }
    // Clear draft on successful submit
    const draftKey = inlineDraftKey(blockIndex)
    if (draftKey) { localStorage.removeItem(draftKey); localStorage.removeItem(draftKey + '_mode') }
    inlineCommentText.value = ''
    inlineMode.value = 'comment'
    emit('comment', { paragraph_index: blockIndex, body: comment.body })
    await loadComments()
  } catch (e) {
    console.error('Failed to submit inline comment:', e)
  } finally {
    submittingInline.value = false
  }
}

async function acceptSuggestion(commentId) {
  try {
    await api.acceptSuggestion(commentId)
    await loadComments()
    emit('suggestion-accepted')
  } catch (e) {
    console.error('Failed to accept suggestion:', e)
  }
}

async function rejectSuggestion(commentId) {
  try {
    await api.rejectSuggestion(commentId)
    await loadComments()
  } catch (e) {
    console.error('Failed to reject suggestion:', e)
  }
}

async function doResolveComment(id) {
  try {
    await api.resolveComment(id, { resolved_by: getCurrentUser() })
    emit('resolve', id)
    await loadComments()
  } catch (e) {
    console.error('Failed to resolve comment:', e)
  }
}

function repliesFor(commentId) {
  return comments.value.filter(c => c.parent_id === commentId)
}

async function submitReply(parentId) {
  if (!replyText.value.trim() || submittingReply.value) return
  submittingReply.value = true
  try {
    const reply = {
      document_id: props.documentId,
      author: getCurrentUser(),
      body: replyText.value.trim(),
      parent_id: parentId,
    }
    if (props.reviewId) reply.review_id = props.reviewId
    await api.addComment(reply)
    replyText.value = ''
    replyingTo.value = null
    await loadComments()
  } catch (e) {
    console.error('Failed to submit reply:', e)
  } finally {
    submittingReply.value = false
  }
}

function toggleResolved(commentId) {
  if (expandedResolved.has(commentId)) {
    expandedResolved.delete(commentId)
  } else {
    expandedResolved.add(commentId)
  }
}

// --- Comment body rendering ---
function renderCommentBody(body) {
  if (!body) return ''
  const escaped = body.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  return escaped.replace(/@([\w.+-]+@[\w.-]+)/g, '<span class="text-blue-400 font-medium">@$1</span>')
    .replace(/@(\w+)/g, '<span class="text-blue-400 font-medium">@$1</span>')
}

function firstLine(text) {
  if (!text) return ''
  const line = text.split('\n')[0]
  return line.length > 80 ? line.substring(0, 80) + '...' : line
}

// --- Review progress (localStorage) ---
const reviewedBlocks = ref(new Set())

const reviewProgress = computed(() => {
  if (contentBlocks.value.length === 0) return 0
  return Math.round((reviewedBlocks.value.size / contentBlocks.value.length) * 100)
})

function reviewStoragePrefix() {
  if (props.reviewId) return `isms_reviewed_r${props.reviewId}`
  return `isms_reviewed_${props.documentId}`
}

function loadReviewProgress() {
  reviewedBlocks.value = new Set()
  if (!props.documentId) return
  const prefix = reviewStoragePrefix()
  const total = contentBlocks.value.length
  for (let i = 0; i < total; i++) {
    if (localStorage.getItem(`${prefix}_${i}`) === '1') {
      reviewedBlocks.value.add(i)
    }
  }
  reviewedBlocks.value = new Set(reviewedBlocks.value)
}

function toggleReviewBlock(index) {
  const newSet = new Set(reviewedBlocks.value)
  const key = `${reviewStoragePrefix()}_${index}`
  if (newSet.has(index)) {
    newSet.delete(index)
    localStorage.removeItem(key)
  } else {
    newSet.add(index)
    localStorage.setItem(key, '1')
  }
  reviewedBlocks.value = newSet
}

// --- Format helpers ---
function formatDate(dateStr) {
  if (!dateStr && dateStr !== 0) return ''
  try {
    const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
    const now = new Date()
    const diff = now - d
    if (diff < 60000) return 'just now'
    if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`
    if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`
    if (diff < 604800000) return `${Math.floor(diff / 86400000)}d ago`
    return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
  } catch {
    return dateStr
  }
}

// --- Lifecycle ---
watch(() => props.documentId, (newId) => {
  if (newId) {
    loadComments()
    expandedBlock.value = null
    inlineCommentText.value = ''
  }
}, { immediate: true })

watch(() => [props.documentId, contentBlocks.value.length], () => {
  if (props.showReviewProgress && props.documentId && contentBlocks.value.length > 0) {
    loadReviewProgress()
  }
})

// Expose review progress for parent components
defineExpose({ reviewProgress, reviewedBlocks, contentBlocks })
</script>

<style scoped>
/* doc-prose base styles are in style.css (global) */

/* Review checkbox */
.review-checkbox {
  transition: all 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.review-checked .review-checkbox {
  opacity: 1 !important;
}

.review-checked {
  border-left: 2px solid rgba(16, 185, 129, 0.3);
}

/* Comment block transitions */
.comment-block {
  transition: background-color 0.2s ease, border-color 0.3s ease;
}

/* Grid-based table rows */
.doc-prose :deep(.tbl-grid) {
  display: grid;
  gap: 0;
  border-left: 1px solid rgb(51 65 85);
  border-right: 1px solid rgb(51 65 85);
}
.doc-prose :deep(.tbl-hdr-cell) {
  background: rgb(30 41 59);
  padding: 0.65rem 1rem;
  font-weight: 600;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: rgb(148 163 184);
  border-bottom: 2px solid rgb(51 65 85);
  border-right: 1px solid rgb(51 65 85);
}
.doc-prose :deep(.tbl-hdr-cell:last-child) {
  border-right: none;
}
.doc-prose :deep(.tbl-cell) {
  padding: 0.6rem 1rem;
  border-bottom: 1px solid rgba(51 65 85 / 0.4);
  color: rgb(203 213 225);
  line-height: 1.6;
  font-size: 0.875rem;
  border-right: 1px solid rgba(51 65 85 / 0.3);
}
.doc-prose :deep(.tbl-cell:last-child) {
  border-right: none;
}
.doc-prose :deep(.tbl-cell strong) {
  color: rgb(241 245 249);
  font-weight: 600;
}
/* First header block gets top border + rounded top */
.comment-block:has(.tbl-grid:not(.tbl-row)) {
  margin-bottom: 0 !important;
}
.comment-block:has(.tbl-grid:not(.tbl-row)) .doc-prose :deep(.tbl-grid) {
  border-top: 1px solid rgb(51 65 85);
  border-radius: 8px 8px 0 0;
  overflow: hidden;
}
/* Row blocks have no gap */
.comment-block:has(.tbl-row) {
  margin-top: -1px !important;
  margin-bottom: 0 !important;
}
/* Visual separator between comment blocks */
.comment-block:has(.tbl-row) + .comment-block:not(:has(.tbl-row)):not(:has(.tbl-grid)) {
  margin-top: 1rem !important;
}
</style>
