<template>
  <div>
    <!-- Stats bar -->
    <div v-if="hasChanges" class="flex items-center justify-between px-8 pt-4 pb-2">
      <div class="flex items-center gap-3 text-xs text-slate-500">
        <span class="text-emerald-400">+{{ stats.added }} added</span>
        <span class="text-red-400">-{{ stats.removed }} removed</span>
        <span v-if="activeCommentCount > 0" class="text-blue-400">{{ activeCommentCount }} comment{{ activeCommentCount !== 1 ? 's' : '' }}</span>
      </div>
      <button @click="showRaw = !showRaw"
        class="text-[10px] text-slate-600 hover:text-slate-400 transition-colors">
        {{ showRaw ? 'Normal' : 'Advanced' }}
      </button>
    </div>
    <!-- Advanced: code diff -->
    <div v-if="showRaw" class="px-4 pb-4">
      <DiffView :diff="rawDiff" />
    </div>
    <!-- Normal: rendered unified diff with paragraph-anchored comments -->
    <div v-else class="py-4">
      <template v-for="(para, pi) in unifiedParagraphs" :key="pi">
        <!-- Paragraph content -->
        <div class="group/para relative">
          <div v-for="(block, bi) in para.blocks" :key="bi" class="flex">
            <!-- Blame gutter -->
            <div class="w-32 flex-shrink-0 pr-3 text-right">
              <div v-if="block.blame && (bi === 0 || para.blocks[bi-1]?.blameKey !== block.blameKey)"
                class="text-[10px] leading-snug pt-1 cursor-default group/blame relative">
                <div class="text-slate-500 truncate">{{ shortName(block.blame.author) }}</div>
                <div class="text-slate-700 font-mono">{{ relTime(block.blame.date) }}</div>
                <div class="absolute right-0 top-full mt-1 z-30 hidden group-hover/blame:block bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 shadow-xl whitespace-nowrap text-left">
                  <div class="text-slate-300 text-[11px] font-medium">{{ block.blame.author }}</div>
                  <div class="text-slate-500 text-[10px] mt-0.5">{{ fullDate(block.blame.date) }}</div>
                  <div class="text-slate-600 text-[10px] font-mono mt-0.5">{{ block.blame.hash }}</div>
                </div>
              </div>
            </div>
            <!-- Content -->
            <div class="flex-1 min-w-0 pr-14"
              :class="{
                'tc-del': block.type === 'remove',
                'tc-ins': block.type === 'add',
                'tc-change': block.type === 'change',
              }">
              <div class="doc-prose" v-html="block.html"></div>
            </div>
          </div>
          <!-- Comment badge / add button -->
          <template v-if="para.paragraphIndex >= 0">
            <div v-if="activeCommentsForParagraph(para.paragraphIndex).length > 0"
              @click="expandedParagraph = expandedParagraph === para.paragraphIndex ? null : para.paragraphIndex"
              class="absolute right-4 top-1 w-6 h-6 rounded-full bg-blue-600 text-white text-xs font-bold flex items-center justify-center cursor-pointer hover:bg-blue-500 transition-colors shadow-lg shadow-blue-900/30 z-10"
              :title="activeCommentsForParagraph(para.paragraphIndex).length + ' comment(s)'">
              {{ activeCommentsForParagraph(para.paragraphIndex).length }}
            </div>
            <div v-else-if="outdatedCommentsForParagraph(para.paragraphIndex).length > 0"
              @click="expandedParagraph = expandedParagraph === para.paragraphIndex ? null : para.paragraphIndex"
              class="absolute right-4 top-1 w-6 h-6 rounded-full bg-slate-700 text-slate-500 text-xs font-bold flex items-center justify-center cursor-pointer hover:bg-slate-600 transition-colors z-10"
              :title="outdatedCommentsForParagraph(para.paragraphIndex).length + ' outdated'">
              {{ outdatedCommentsForParagraph(para.paragraphIndex).length }}
            </div>
            <button v-else-if="!readonly"
              @click.stop="expandedParagraph = para.paragraphIndex"
              class="absolute right-4 top-1 w-6 h-6 rounded-full bg-slate-700 text-slate-400 text-xs flex items-center justify-center cursor-pointer opacity-0 group-hover/para:opacity-100 transition-all duration-200 hover:bg-blue-600 hover:text-white hover:shadow-lg z-10"
              title="Add comment">+</button>
          </template>
        </div>

        <!-- Comment thread -->
        <div v-if="expandedParagraph === para.paragraphIndex && para.paragraphIndex >= 0" class="flex mb-2">
          <div class="w-32 flex-shrink-0"></div>
          <div class="flex-1 min-w-0 pr-8 border-l-2 border-blue-600/60 pl-4 ml-2">
            <!-- Outdated comments -->
            <div v-if="outdatedCommentsForParagraph(para.paragraphIndex).length > 0" class="mb-3">
              <button @click="showOutdated[para.paragraphIndex] = !showOutdated[para.paragraphIndex]"
                class="flex items-center gap-1.5 text-[10px] text-slate-600 hover:text-slate-400 transition-colors">
                <svg class="w-3 h-3 transition-transform" :class="showOutdated[para.paragraphIndex] ? 'rotate-90' : ''" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                </svg>
                <span class="px-1.5 py-0.5 rounded bg-slate-800 text-slate-500 font-medium">Outdated</span>
                {{ outdatedCommentsForParagraph(para.paragraphIndex).length }} comment{{ outdatedCommentsForParagraph(para.paragraphIndex).length === 1 ? '' : 's' }}
              </button>
              <div v-if="showOutdated[para.paragraphIndex]" class="mt-2 opacity-50">
                <div v-for="c in outdatedCommentsForParagraph(para.paragraphIndex)" :key="c.id" class="mb-2">
                  <div class="flex items-center gap-2 text-[11px]">
                    <span class="font-medium text-slate-500">{{ c.author?.split('@')[0] }}</span>
                    <span class="text-slate-700 text-[10px]">{{ formatTime(c.created_at) }}</span>
                    <span class="text-[9px] px-1 py-0.5 rounded bg-slate-800 text-slate-600">outdated</span>
                  </div>
                  <div class="text-sm text-slate-600 mt-0.5 line-through decoration-slate-700">{{ c.body }}</div>
                </div>
              </div>
            </div>
            <!-- Active comments -->
            <div v-for="c in activeCommentsForParagraph(para.paragraphIndex)" :key="c.id" class="mb-2">
              <div class="flex items-center gap-2 text-[11px]">
                <span class="font-medium text-slate-300">{{ c.author?.split('@')[0] }}</span>
                <span class="text-slate-600 text-[10px]">{{ formatTime(c.created_at) }}</span>
              </div>
              <div class="text-sm text-slate-400 mt-0.5">{{ c.body }}</div>
            </div>
            <!-- Comment input -->
            <div v-if="!readonly" class="mt-2">
              <textarea v-model="commentBody" rows="2" placeholder="Add a comment..."
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500 resize-none"
                @keydown.meta.enter.prevent="submitComment(para.paragraphIndex)"
                @keydown.ctrl.enter.prevent="submitComment(para.paragraphIndex)"></textarea>
              <div class="flex gap-2 mt-1.5">
                <button @click="submitComment(para.paragraphIndex)" :disabled="!commentBody.trim()"
                  class="px-3 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-xs font-medium rounded-lg transition-colors">Comment</button>
                <button @click="expandedParagraph = null; commentBody = ''" class="px-3 py-1.5 text-xs text-slate-500 hover:text-slate-300">Cancel</button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, reactive } from 'vue'
import DOMPurify from 'dompurify'
import { parseMd } from '../composables/useRenderMd'
import DiffView from './DiffView.vue'
import api from '../api'

const emit = defineEmits(['comment'])

const props = defineProps({
  oldBody: { type: String, default: '' },
  newBody: { type: String, default: '' },
  author: { type: String, default: '' },
  date: { type: String, default: '' },
  documentId: { type: String, default: '' },
  blameRef: { type: String, default: '' },
  comments: { type: Array, default: () => [] },
  readonly: { type: Boolean, default: false },
})

const showRaw = ref(false)
const blameData = ref([])
const expandedParagraph = ref(null)
const commentBody = ref('')
const showOutdated = reactive({})

watch(() => expandedParagraph.value, () => { commentBody.value = '' })

function activeCommentsForParagraph(idx) {
  return props.comments.filter(c => c.paragraph_index === idx && !c.is_outdated)
}
function outdatedCommentsForParagraph(idx) {
  return props.comments.filter(c => c.paragraph_index === idx && c.is_outdated)
}
const activeCommentCount = computed(() => props.comments.filter(c => !c.is_outdated).length)

function formatTime(ts) {
  if (!ts) return ''
  try {
    const d = new Date(typeof ts === 'number' ? ts * 1000 : ts)
    const diff = Math.floor((Date.now() - d.getTime()) / 60000)
    if (diff < 1) return 'just now'
    if (diff < 60) return `${diff}m ago`
    if (diff < 1440) return `${Math.floor(diff / 60)}h ago`
    return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })
  } catch { return '' }
}

function submitComment(paragraphIndex) {
  if (!commentBody.value.trim()) return
  const paragraphs = splitParagraphs(props.newBody || '')
  const quote = paragraphs[paragraphIndex] || ''
  emit('comment', { index: paragraphIndex, quote, body: commentBody.value.trim() })
  commentBody.value = ''
  expandedParagraph.value = null
}

function splitParagraphs(text) {
  if (!text) return []
  return text.split(/\n{2,}/).filter(p => p.trim())
}

// Build unified paragraphs: each paragraph from newBody mapped to its diff blocks.
// Strategy: split newBody into paragraphs, then for each finalBlock, figure out which
// paragraph it belongs to based on cumulative line counting in the new body.
const unifiedParagraphs = computed(() => {
  const newParas = splitParagraphs(props.newBody || '')
  const blocks = finalBlocks.value
  if (blocks.length === 0 && newParas.length === 0) return []

  // Compute line ranges for each paragraph in new body
  const paraRanges = [] // { start, end } line indices in new body
  let lineStart = 0
  const newLines = (props.newBody || '').split('\n')
  for (let pi = 0; pi < newParas.length; pi++) {
    const paraLines = newParas[pi].split('\n')
    // Find where this paragraph starts in the full newLines array
    // Skip blank lines between paragraphs
    while (lineStart < newLines.length && newLines[lineStart].trim() === '') lineStart++
    const start = lineStart
    lineStart += paraLines.length
    paraRanges.push({ start, end: lineStart })
    // Skip blank lines after paragraph
    while (lineStart < newLines.length && newLines[lineStart].trim() === '') lineStart++
  }

  // Now assign each block to a paragraph based on which new-body lines it covers
  const result = newParas.map((_, pi) => ({ paragraphIndex: pi, blocks: [] }))
  let removedBlocks = [] // removed blocks before any paragraph
  let newLineIdx = 0

  for (const block of blocks) {
    if (block.type === 'remove') {
      // Removed blocks: attach to the current paragraph context
      const paraIdx = findParagraph(newLineIdx, paraRanges)
      if (paraIdx >= 0 && result[paraIdx]) {
        result[paraIdx].blocks.push(block)
      } else {
        removedBlocks.push(block)
      }
      continue
    }

    const blockLines = block.text.split('\n').length
    const paraIdx = findParagraph(newLineIdx, paraRanges)

    if (paraIdx >= 0 && result[paraIdx]) {
      result[paraIdx].blocks.push(block)
    }

    newLineIdx += blockLines
  }

  // Prepend any orphaned removed blocks to the first paragraph
  if (removedBlocks.length > 0 && result.length > 0) {
    result[0].blocks = [...removedBlocks, ...result[0].blocks]
  }

  // Filter out empty paragraphs
  return result.filter(p => p.blocks.length > 0)
})

function findParagraph(lineIdx, ranges) {
  for (let i = 0; i < ranges.length; i++) {
    if (lineIdx < ranges[i].end) return i
  }
  return ranges.length - 1 // default to last paragraph
}

watch([() => props.documentId, () => props.blameRef], async ([id]) => {
  if (!id) { blameData.value = []; return }
  try {
    const refParam = props.blameRef ? `?ref=${encodeURIComponent(props.blameRef)}` : ''
    const res = await api.fetchJSON(`/api/v1/documents/${encodeURIComponent(id)}/blame${refParam}`)
    blameData.value = res?.lines || []
  } catch {
    blameData.value = []
  }
}, { immediate: true })

function relTime(dateStr) {
  if (!dateStr) return ''
  try {
    const d = new Date(dateStr)
    const diffH = Math.floor((new Date() - d) / 3600000)
    if (diffH < 1) return 'just now'
    if (diffH < 24) return `${diffH}h ago`
    const diffD = Math.floor(diffH / 24)
    if (diffD < 30) return `${diffD}d ago`
    return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })
  } catch { return '' }
}

function fullDate(dateStr) {
  if (!dateStr) return ''
  try {
    return new Date(dateStr).toLocaleString('en-GB', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })
  } catch { return '' }
}

function shortName(email) {
  if (!email) return ''
  const at = email.indexOf('@')
  return at > 0 ? email.substring(0, at) : email
}

const segments = computed(() => {
  const oldText = (props.oldBody || '').trim()
  const newText = (props.newBody || '').trim()
  if (!oldText && !newText) return []
  if (!oldText) return [{ type: 'add', text: newText }]
  if (oldText === newText) return [{ type: 'equal', text: newText }]

  const raw = diffLines(oldText.split('\n'), newText.split('\n'))
  const paired = []
  let i = 0
  while (i < raw.length) {
    if (raw[i].type === 'remove' && i + 1 < raw.length && raw[i + 1].type === 'add') {
      paired.push({ type: 'change', oldText: raw[i].text, newText: raw[i + 1].text, html: wordDiffHtml(raw[i].text, raw[i + 1].text) })
      i += 2
    } else {
      paired.push(raw[i])
      i++
    }
  }
  return paired
})

const stats = computed(() => {
  let added = 0, removed = 0
  for (const s of segments.value) {
    if (s.type === 'add') added += s.text.split('\n').length
    else if (s.type === 'remove') removed += s.text.split('\n').length
    else if (s.type === 'change') { removed += s.oldText.split('\n').length; added += s.newText.split('\n').length }
  }
  return { added, removed }
})

const hasChanges = computed(() => stats.value.added > 0 || stats.value.removed > 0)

const finalBlocks = computed(() => {
  const blame = blameData.value
  const blocks = []
  let newLineIdx = 0

  for (const seg of segments.value) {
    if (seg.type === 'remove') {
      blocks.push({ type: 'remove', text: seg.text, html: renderMd(seg.text), blame: null, blameKey: 'removed' })
      continue
    }
    if (seg.type === 'change') {
      const lineBlame = newLineIdx < blame.length ? blame[newLineIdx] : null
      newLineIdx += seg.newText.split('\n').length
      blocks.push({ type: 'change', text: seg.newText, html: seg.html, blame: lineBlame, blameKey: lineBlame ? lineBlame.hash : 'change' })
      continue
    }
    // equal or add — split by blame boundaries
    let currentAuthorHash = null, currentLines = [], currentBlame = null
    for (const line of seg.text.split('\n')) {
      const lineBlame = newLineIdx < blame.length ? blame[newLineIdx] : null
      newLineIdx++
      const authorHash = lineBlame ? lineBlame.hash : 'unknown'
      if (authorHash !== currentAuthorHash && currentLines.length > 0) {
        blocks.push({ type: seg.type, text: currentLines.join('\n'), html: renderMd(currentLines.join('\n')), blame: currentBlame, blameKey: currentAuthorHash })
        currentLines = []
      }
      if (currentLines.length === 0) { currentBlame = lineBlame; currentAuthorHash = authorHash }
      currentLines.push(line)
    }
    if (currentLines.length > 0) {
      blocks.push({ type: seg.type, text: currentLines.join('\n'), html: renderMd(currentLines.join('\n')), blame: currentBlame, blameKey: currentAuthorHash })
    }
  }
  return blocks
})

const rawDiff = computed(() => {
  if (!hasChanges.value) return ''
  const oldLines = (props.oldBody || '').split('\n'), newLines = (props.newBody || '').split('\n')
  const lines = ['--- a/document', '+++ b/document', `@@ -1,${oldLines.length} +1,${newLines.length} @@`]
  for (const s of segments.value) {
    if (s.type === 'change') {
      for (const l of s.oldText.split('\n')) lines.push('-' + l)
      for (const l of s.newText.split('\n')) lines.push('+' + l)
    } else {
      for (const l of s.text.split('\n')) {
        lines.push((s.type === 'equal' ? ' ' : s.type === 'remove' ? '-' : '+') + l)
      }
    }
  }
  return lines.join('\n')
})

function renderMd(text) { return text ? DOMPurify.sanitize(parseMd(text)) : '' }

function wordDiffHtml(oldText, newText) {
  const ops = diffTokens(tokenize(oldText), tokenize(newText))
  const parts = ops.map(op => op.type === 'equal' ? op.text : op.type === 'remove' ? `<del class="tc-word-del">${esc(op.text)}</del>` : `<ins class="tc-word-ins">${esc(op.text)}</ins>`)
  return DOMPurify.sanitize(parseMd(parts.join('')), { ADD_TAGS: ['del', 'ins'], ADD_ATTR: ['class'] })
}

function esc(t) { return t.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;') }
function tokenize(text) { return text.match(/\S+|\s+/g) || [] }

function diffTokens(oldToks, newToks) {
  const m = oldToks.length, n = newToks.length
  const dp = Array.from({ length: m + 1 }, () => new Uint16Array(n + 1))
  for (let i = 1; i <= m; i++) for (let j = 1; j <= n; j++) dp[i][j] = oldToks[i-1] === newToks[j-1] ? dp[i-1][j-1]+1 : Math.max(dp[i-1][j], dp[i][j-1])
  const raw = []; let i = m, j = n
  while (i > 0 || j > 0) { if (i > 0 && j > 0 && oldToks[i-1] === newToks[j-1]) { raw.push({ type: 'equal', text: oldToks[i-1] }); i--; j-- } else if (j > 0 && (i === 0 || dp[i][j-1] >= dp[i-1][j])) { raw.push({ type: 'add', text: newToks[j-1] }); j-- } else { raw.push({ type: 'remove', text: oldToks[i-1] }); i-- } }
  raw.reverse()
  const merged = []; for (const op of raw) { if (merged.length > 0 && merged[merged.length-1].type === op.type) merged[merged.length-1].text += op.text; else merged.push({...op}) }
  return merged
}

function diffLines(oldArr, newArr) {
  const m = oldArr.length, n = newArr.length
  const dp = Array.from({ length: m + 1 }, () => new Uint16Array(n + 1))
  for (let i = 1; i <= m; i++) for (let j = 1; j <= n; j++) dp[i][j] = oldArr[i-1] === newArr[j-1] ? dp[i-1][j-1]+1 : Math.max(dp[i-1][j], dp[i][j-1])
  const raw = []; let i = m, j = n
  while (i > 0 || j > 0) { if (i > 0 && j > 0 && oldArr[i-1] === newArr[j-1]) { raw.push({ type: 'equal', text: oldArr[i-1] }); i--; j-- } else if (j > 0 && (i === 0 || dp[i][j-1] >= dp[i-1][j])) { raw.push({ type: 'add', text: newArr[j-1] }); j-- } else { raw.push({ type: 'remove', text: oldArr[i-1] }); i-- } }
  raw.reverse()
  const merged = []; for (const op of raw) { if (merged.length > 0 && merged[merged.length-1].type === op.type) merged[merged.length-1].text += '\n' + op.text; else merged.push({...op}) }
  return merged
}
</script>
