<template>
  <div>
    <!-- Summary bar -->
    <div class="flex items-center justify-between px-6 py-3 bg-slate-900 border border-slate-800 rounded-t-xl">
      <div class="flex items-center gap-4 text-xs">
        <span v-if="changedCount > 0" class="text-amber-400">{{ changedCount }} changed paragraph{{ changedCount !== 1 ? 's' : '' }}</span>
        <span v-else class="text-emerald-400">No changes</span>
        <span v-if="commentCount > 0" class="text-blue-400">{{ commentCount }} comment{{ commentCount !== 1 ? 's' : '' }}</span>
        <span class="text-slate-600">{{ reviewStatus }}</span>
      </div>
      <button @click="showAdvanced = !showAdvanced"
        class="text-[10px] text-slate-600 hover:text-slate-400 transition-colors">
        {{ showAdvanced ? 'Document view' : 'Advanced diff' }}
      </button>
    </div>

    <!-- Advanced diff (code) -->
    <div v-if="showAdvanced" class="border border-t-0 border-slate-800 rounded-b-xl overflow-hidden">
      <DiffView :diff="rawDiff" />
    </div>

    <!-- Side-by-side rendered view -->
    <div v-else class="flex border border-t-0 border-slate-800 rounded-b-xl overflow-hidden">
      <!-- Left: Previous version -->
      <div class="flex-1 min-w-0 border-r border-slate-800 overflow-y-auto max-h-[80vh]">
        <div class="px-3 py-2 bg-slate-900/80 border-b border-slate-800 sticky top-0 z-10">
          <span class="text-[10px] font-medium text-slate-500 uppercase tracking-wider">Previous</span>
        </div>
        <div class="px-6 py-4">
          <template v-for="(block, i) in leftBlocks" :key="'l-' + i">
            <div :class="{ 'sbs-removed': block.changed }" class="sbs-block">
              <div class="doc-prose" v-html="block.html"></div>
            </div>
          </template>
          <div v-if="!oldBody" class="py-8 text-center text-sm text-slate-600">No previous version</div>
        </div>
      </div>
      <!-- Right: New version -->
      <div class="flex-1 min-w-0 overflow-y-auto max-h-[80vh]">
        <div class="px-3 py-2 bg-slate-900/80 border-b border-slate-800 sticky top-0 z-10">
          <span class="text-[10px] font-medium text-slate-500 uppercase tracking-wider">Current</span>
        </div>
        <div class="px-6 py-4">
          <div class="pr-14 relative">
            <template v-for="(block, i) in rightBlocks" :key="'r-' + i">
              <div :class="{ 'sbs-added': block.changed }" class="sbs-block group/para relative rounded -mx-2 px-2 transition-colors duration-200"
                :style="activeCommentsForParagraph(i).length > 0 ? 'background: rgba(59,130,246,0.06)' : ''">
                <div class="doc-prose" v-html="block.html"></div>

                <!-- Comment count badge (active comments only) -->
                <div v-if="activeCommentsForParagraph(i).length > 0"
                  @click="expandedBlock = expandedBlock === i ? null : i"
                  class="absolute -right-12 top-1 w-6 h-6 rounded-full bg-blue-600 text-white text-xs font-bold flex items-center justify-center cursor-pointer hover:bg-blue-500 transition-colors shadow-lg shadow-blue-900/30"
                  :title="activeCommentsForParagraph(i).length + ' comment' + (activeCommentsForParagraph(i).length === 1 ? '' : 's')">
                  {{ activeCommentsForParagraph(i).length }}
                </div>
                <!-- Outdated comment indicator -->
                <div v-else-if="outdatedCommentsForParagraph(i).length > 0"
                  @click="expandedBlock = expandedBlock === i ? null : i"
                  class="absolute -right-12 top-1 w-6 h-6 rounded-full bg-slate-700 text-slate-500 text-xs font-bold flex items-center justify-center cursor-pointer hover:bg-slate-600 transition-colors"
                  :title="outdatedCommentsForParagraph(i).length + ' outdated comment' + (outdatedCommentsForParagraph(i).length === 1 ? '' : 's')">
                  {{ outdatedCommentsForParagraph(i).length }}
                </div>

                <!-- Add comment button (hover) — shows when no active comments and not readonly -->
                <button v-if="!readonly && activeCommentsForParagraph(i).length === 0 && outdatedCommentsForParagraph(i).length === 0"
                  @click.stop="expandedBlock = i; commentingIndex = i"
                  class="absolute -right-12 top-1 w-6 h-6 rounded-full bg-slate-700 text-slate-400 text-xs flex items-center justify-center cursor-pointer opacity-0 group-hover/para:opacity-100 transition-all duration-200 hover:bg-blue-600 hover:text-white hover:shadow-lg hover:shadow-blue-900/30 hover:scale-110"
                  title="Add comment">
                  +
                </button>
              </div>

              <!-- Expanded comment thread -->
              <transition
                enter-active-class="transition-all duration-300 ease-out"
                enter-from-class="opacity-0 -translate-y-2 max-h-0"
                enter-to-class="opacity-100 translate-y-0 max-h-[2000px]"
                leave-active-class="transition-all duration-200 ease-in"
                leave-from-class="opacity-100 translate-y-0 max-h-[2000px]"
                leave-to-class="opacity-0 -translate-y-2 max-h-0">
                <div v-if="expandedBlock === i" class="mt-1 mb-4 ml-4 border-l-2 border-blue-600/60 pl-4 overflow-hidden">
                  <!-- Outdated comments (collapsed by default, like GitHub) -->
                  <div v-if="outdatedCommentsForParagraph(i).length > 0" class="mb-3">
                    <button @click="showOutdated[i] = !showOutdated[i]"
                      class="flex items-center gap-1.5 text-[10px] text-slate-600 hover:text-slate-400 transition-colors">
                      <svg class="w-3 h-3 transition-transform" :class="showOutdated[i] ? 'rotate-90' : ''" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                      </svg>
                      <span class="px-1.5 py-0.5 rounded bg-slate-800 text-slate-500 font-medium">Outdated</span>
                      {{ outdatedCommentsForParagraph(i).length }} comment{{ outdatedCommentsForParagraph(i).length === 1 ? '' : 's' }} from previous round
                    </button>
                    <div v-if="showOutdated[i]" class="mt-2 opacity-50">
                      <div v-for="c in outdatedCommentsForParagraph(i)" :key="c.id" class="mb-2">
                        <div class="flex items-center gap-2 text-[11px]">
                          <span class="font-medium text-slate-500">{{ c.author?.split('@')[0] }}</span>
                          <span class="text-slate-700 text-[10px]">{{ formatTime(c.created_at) }}</span>
                          <span class="text-[9px] px-1 py-0.5 rounded bg-slate-800 text-slate-600">outdated</span>
                        </div>
                        <div class="text-sm text-slate-600 mt-0.5 line-through decoration-slate-700">{{ c.body }}</div>
                      </div>
                    </div>
                  </div>

                  <!-- Active comments (current round) -->
                  <div v-for="c in activeCommentsForParagraph(i)" :key="c.id" class="mb-2">
                    <div class="flex items-center gap-2 text-[11px]">
                      <span class="font-medium text-slate-300">{{ c.author?.split('@')[0] }}</span>
                      <span class="text-slate-600 text-[10px]">{{ formatTime(c.created_at) }}</span>
                    </div>
                    <div class="text-sm text-slate-400 mt-0.5">{{ c.body }}</div>
                  </div>

                  <!-- Comment input (hidden on readonly/merged reviews) -->
                  <div v-if="!readonly" class="mt-2">
                    <textarea v-model="commentBody" rows="2" placeholder="Add a comment..."
                      class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500 resize-none"
                      @keydown.meta.enter.prevent="submitComment(i, block.text)"
                      @keydown.ctrl.enter.prevent="submitComment(i, block.text)"></textarea>
                    <div class="flex gap-2 mt-1.5">
                      <button @click="submitComment(i, block.text)" :disabled="!commentBody.trim()"
                        class="px-3 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-xs font-medium rounded-lg transition-colors">Comment</button>
                      <button @click="expandedBlock = null; commentBody = ''" class="px-3 py-1.5 text-xs text-slate-500 hover:text-slate-300">Cancel</button>
                    </div>
                  </div>
                </div>
              </transition>
            </template>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import DOMPurify from 'dompurify'
import DiffView from './DiffView.vue'
import { parseMd } from '../composables/useRenderMd'
import { diffTables, isTableBlock } from '../composables/useTableDiff'

const props = defineProps({
  oldBody: { type: String, default: '' },
  newBody: { type: String, default: '' },
  rawDiff: { type: String, default: '' },
  commentCount: { type: Number, default: 0 },
  reviewStatus: { type: String, default: '' },
  comments: { type: Array, default: () => [] },
  readonly: { type: Boolean, default: false },
})

const emit = defineEmits(['comment'])
const showAdvanced = ref(false)
const commentingIndex = ref(null)
const commentBody = ref('')
const expandedBlock = ref(null)

watch(expandedBlock, () => { commentBody.value = '' })

function formatTime(d) {
  if (!d && d !== 0) return ''
  const dt = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  return dt.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })
}

const showOutdated = ref({})

function commentsForParagraph(index) {
  return props.comments.filter(c => c.paragraph_index === index)
}

function activeCommentsForParagraph(index) {
  return props.comments.filter(c => c.paragraph_index === index && !c.is_outdated)
}

function outdatedCommentsForParagraph(index) {
  return props.comments.filter(c => c.paragraph_index === index && c.is_outdated)
}

function submitComment(index, quote) {
  if (!commentBody.value.trim()) return
  emit('comment', { index, quote, body: commentBody.value })
  commentBody.value = ''
  commentingIndex.value = null
}

function renderMd(text) {
  if (!text) return ''
  return DOMPurify.sanitize(parseMd(text))
}

// Split into paragraphs (markdown blocks separated by blank lines)
function splitBlocks(text) {
  if (!text) return []
  // Split on double newline (paragraph boundaries)
  const blocks = text.split(/\n{2,}/).filter(b => b.trim())
  return blocks.map(b => ({ text: b.trim(), html: renderMd(b.trim()) }))
}

// Line-level LCS to identify which blocks changed
function diffBlocks(oldBlocks, newBlocks) {
  const m = oldBlocks.length, n = newBlocks.length
  const dp = Array.from({ length: m + 1 }, () => new Uint16Array(n + 1))
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      dp[i][j] = oldBlocks[i - 1].text === newBlocks[j - 1].text
        ? dp[i - 1][j - 1] + 1
        : Math.max(dp[i - 1][j], dp[i][j - 1])
    }
  }
  // Backtrack to mark changed blocks
  const oldChanged = new Array(m).fill(true)
  const newChanged = new Array(n).fill(true)
  let i = m, j = n
  while (i > 0 && j > 0) {
    if (oldBlocks[i - 1].text === newBlocks[j - 1].text) {
      oldChanged[i - 1] = false
      newChanged[j - 1] = false
      i--; j--
    } else if (dp[i - 1][j] > dp[i][j - 1]) {
      i--
    } else {
      j--
    }
  }
  return { oldChanged, newChanged }
}

const oldBlocks = computed(() => splitBlocks(props.oldBody))
const newBlocks = computed(() => splitBlocks(props.newBody))

const diff = computed(() => {
  if (oldBlocks.value.length === 0 && newBlocks.value.length === 0) return { oldChanged: [], newChanged: [] }
  if (oldBlocks.value.length === 0) return { oldChanged: [], newChanged: newBlocks.value.map(() => true) }
  return diffBlocks(oldBlocks.value, newBlocks.value)
})

// A changed table renders as a single per-cell diff in the Current column
// (see useTableDiff) instead of a wholesale red-old / green-new block — so a
// one-cell edit reads as one cell, not a replaced table. The Previous column
// then shows the old table plainly (the merged diff already marks removals).
const changedOldTables = computed(() =>
  oldBlocks.value.filter((b, i) => diff.value.oldChanged[i] && isTableBlock(b.text))
)

const leftBlocks = computed(() =>
  oldBlocks.value.map((b, i) => ({
    ...b,
    // Don't strike through a whole old table — its diff is shown on the right.
    changed: diff.value.oldChanged[i] && !isTableBlock(b.text),
  }))
)

const rightBlocks = computed(() => {
  let tIdx = 0
  return newBlocks.value.map((b, i) => {
    const changed = diff.value.newChanged[i]
    if (changed && isTableBlock(b.text)) {
      const oldText = changedOldTables.value[tIdx]?.text || ''
      tIdx++
      const merged = diffTables(oldText, b.text)
      if (merged) return { ...b, changed: false, html: merged }
    }
    return { ...b, changed }
  })
})

const changedCount = computed(() =>
  diff.value.newChanged.filter(c => c).length
)
</script>

<style>
.sbs-block { transition: background-color 0.2s; }
.sbs-removed {
  background: rgba(239, 68, 68, 0.06);
  border-left: 3px solid rgba(239, 68, 68, 0.3);
  padding-left: 0.75rem;
  margin: 0.25rem 0;
  border-radius: 0 0.25rem 0.25rem 0;
}
.sbs-removed .doc-prose, .sbs-removed .doc-prose * {
  color: #fca5a5 !important;
  text-decoration: line-through;
  text-decoration-color: rgba(239, 68, 68, 0.3);
}
.sbs-added {
  background: rgba(34, 197, 94, 0.06);
  border-left: 3px solid rgba(34, 197, 94, 0.3);
  padding-left: 0.75rem;
  margin: 0.25rem 0;
  border-radius: 0 0.25rem 0.25rem 0;
}
.sbs-added .doc-prose, .sbs-added .doc-prose * {
  color: #86efac !important;
}
</style>
