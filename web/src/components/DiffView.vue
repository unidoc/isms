<template>
  <div class="diff-view rounded-lg border border-slate-800 overflow-hidden text-xs font-mono">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-2 bg-slate-900 border-b border-slate-800">
      <div class="flex items-center gap-4">
        <span class="text-red-400">- {{ removedCount }} removed</span>
        <span class="text-emerald-400">+ {{ addedCount }} added</span>
      </div>
      <div class="flex items-center gap-1">
        <button @click="mode = 'split'" class="px-2 py-1 rounded text-[10px] font-medium transition-colors"
          :class="mode === 'split' ? 'bg-slate-700 text-white' : 'text-slate-500 hover:text-slate-300'">
          Split
        </button>
        <button @click="mode = 'unified'" class="px-2 py-1 rounded text-[10px] font-medium transition-colors"
          :class="mode === 'unified' ? 'bg-slate-700 text-white' : 'text-slate-500 hover:text-slate-300'">
          Unified
        </button>
      </div>
    </div>

    <!-- Split view (side-by-side) -->
    <div v-if="mode === 'split'" class="flex max-h-[500px] overflow-y-auto">
      <!-- Left (old / removed) -->
      <div class="flex-1 border-r border-slate-800 min-w-0">
        <div class="px-2 py-1 bg-slate-900/50 border-b border-slate-800 text-[10px] text-slate-500 sticky top-0">Before</div>
        <table class="w-full">
          <tr v-for="(row, i) in splitRows" :key="'l-' + i"
            :class="row.type === 'remove' || row.type === 'change' ? 'bg-red-950/30' : row.type === 'add' ? 'bg-slate-900/20' : ''">
            <td class="w-8 text-right pr-2 text-slate-700 select-none align-top py-px" :class="row.type === 'remove' || row.type === 'change' ? 'text-red-800' : ''">
              {{ row.leftNum || '' }}
            </td>
            <td v-if="row.type === 'change'" class="pr-2 py-px whitespace-pre-wrap break-all text-red-300">
              <template v-for="(seg, si) in wordDiff(row.left, row.right).oldSegs" :key="'ls-' + si">
                <span :class="seg.changed ? 'bg-red-800/60 text-red-100 rounded px-px' : ''">{{ seg.text }}</span>
              </template>
            </td>
            <td v-else class="pr-2 py-px whitespace-pre-wrap break-all" :class="row.type === 'remove' ? 'text-red-300' : row.type === 'add' ? 'invisible' : 'text-slate-500'">
              {{ row.type === 'add' ? '' : row.left }}
            </td>
          </tr>
        </table>
      </div>
      <!-- Right (new / added) -->
      <div class="flex-1 min-w-0">
        <div class="px-2 py-1 bg-slate-900/50 border-b border-slate-800 text-[10px] text-slate-500 sticky top-0">After</div>
        <table class="w-full">
          <tr v-for="(row, i) in splitRows" :key="'r-' + i"
            :class="row.type === 'add' || row.type === 'change' ? 'bg-emerald-950/30' : row.type === 'remove' ? 'bg-slate-900/20' : ''">
            <td class="w-8 text-right pr-2 text-slate-700 select-none align-top py-px" :class="row.type === 'add' || row.type === 'change' ? 'text-emerald-800' : ''">
              {{ row.rightNum || '' }}
            </td>
            <td v-if="row.type === 'change'" class="pr-2 py-px whitespace-pre-wrap break-all text-emerald-300">
              <template v-for="(seg, si) in wordDiff(row.left, row.right).newSegs" :key="'rs-' + si">
                <span :class="seg.changed ? 'bg-emerald-800/60 text-emerald-100 rounded px-px' : ''">{{ seg.text }}</span>
              </template>
            </td>
            <td v-else class="pr-2 py-px whitespace-pre-wrap break-all" :class="row.type === 'add' ? 'text-emerald-300' : row.type === 'remove' ? 'invisible' : 'text-slate-500'">
              {{ row.type === 'remove' ? '' : row.right }}
            </td>
          </tr>
        </table>
      </div>
    </div>

    <!-- Unified view -->
    <div v-else class="max-h-[500px] overflow-y-auto">
      <table class="w-full">
        <tr v-for="(line, i) in unifiedRows" :key="'u-' + i"
          :class="{
            'bg-red-950/30': line.type === 'remove',
            'bg-emerald-950/30': line.type === 'add',
          }">
          <td class="w-8 text-right pr-1 text-slate-700 select-none align-top py-px text-[10px]">{{ line.oldNum || '' }}</td>
          <td class="w-8 text-right pr-2 text-slate-700 select-none align-top py-px text-[10px]">{{ line.newNum || '' }}</td>
          <td class="w-4 text-center select-none align-top py-px"
            :class="{ 'text-red-500': line.type === 'remove', 'text-emerald-500': line.type === 'add', 'text-slate-700': line.type === 'context', 'text-blue-500': line.type === 'header' }">
            {{ line.type === 'remove' ? '-' : line.type === 'add' ? '+' : line.type === 'header' ? '@' : ' ' }}
          </td>
          <td class="pr-2 py-px whitespace-pre-wrap break-all"
            :class="{
              'text-red-300': line.type === 'remove',
              'text-emerald-300': line.type === 'add',
              'text-blue-400': line.type === 'header',
              'text-slate-500': line.type === 'context',
            }">{{ line.text }}</td>
        </tr>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'

const props = defineProps({
  diff: { type: String, default: '' }, // unified diff text
})

const mode = ref('split')

// Parse unified diff into structured lines
const parsedLines = computed(() => {
  if (!props.diff) return []
  const lines = props.diff.split('\n')
  const result = []
  for (const line of lines) {
    if (line.startsWith('@@')) {
      result.push({ type: 'header', text: line })
    } else if (line.startsWith('+') && !line.startsWith('+++')) {
      result.push({ type: 'add', text: line.substring(1) })
    } else if (line.startsWith('-') && !line.startsWith('---')) {
      result.push({ type: 'remove', text: line.substring(1) })
    } else if (line.startsWith('---') || line.startsWith('+++')) {
      // skip file headers
    } else {
      result.push({ type: 'context', text: line.startsWith(' ') ? line.substring(1) : line })
    }
  }
  return result
})

const addedCount = computed(() => parsedLines.value.filter(l => l.type === 'add').length)
const removedCount = computed(() => parsedLines.value.filter(l => l.type === 'remove').length)

// Word-level diff for changed lines (paired remove+add)
function wordDiff(oldText, newText) {
  const oldWords = oldText.split(/(\s+)/)
  const newWords = newText.split(/(\s+)/)

  // Simple LCS-based word diff
  const m = oldWords.length, n = newWords.length
  // Build LCS table
  const dp = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0))
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      if (oldWords[i - 1] === newWords[j - 1]) {
        dp[i][j] = dp[i - 1][j - 1] + 1
      } else {
        dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1])
      }
    }
  }

  // Backtrack to find which words are common
  const oldChanged = new Array(m).fill(true)
  const newChanged = new Array(n).fill(true)
  let i = m, j = n
  while (i > 0 && j > 0) {
    if (oldWords[i - 1] === newWords[j - 1]) {
      oldChanged[i - 1] = false
      newChanged[j - 1] = false
      i--; j--
    } else if (dp[i - 1][j] > dp[i][j - 1]) {
      i--
    } else {
      j--
    }
  }

  // Build segments with consecutive same-status words merged
  function buildSegs(words, changed) {
    const segs = []
    for (let k = 0; k < words.length; k++) {
      if (segs.length > 0 && segs[segs.length - 1].changed === changed[k]) {
        segs[segs.length - 1].text += words[k]
      } else {
        segs.push({ text: words[k], changed: changed[k] })
      }
    }
    return segs
  }

  return { oldSegs: buildSegs(oldWords, oldChanged), newSegs: buildSegs(newWords, newChanged) }
}

// Unified view with line numbers
const unifiedRows = computed(() => {
  const rows = []
  let oldNum = 0
  let newNum = 0
  for (const line of parsedLines.value) {
    if (line.type === 'header') {
      // Parse @@ -a,b +c,d @@
      const match = line.text.match(/@@ -(\d+)(?:,\d+)? \+(\d+)/)
      if (match) { oldNum = parseInt(match[1]) - 1; newNum = parseInt(match[2]) - 1 }
      rows.push({ ...line, oldNum: '', newNum: '' })
    } else if (line.type === 'remove') {
      oldNum++
      rows.push({ ...line, oldNum, newNum: '' })
    } else if (line.type === 'add') {
      newNum++
      rows.push({ ...line, oldNum: '', newNum })
    } else {
      oldNum++
      newNum++
      rows.push({ ...line, oldNum, newNum })
    }
  }
  return rows
})

// Split view — pair up removes and adds
const splitRows = computed(() => {
  const rows = []
  let leftNum = 0
  let rightNum = 0
  const lines = parsedLines.value
  let i = 0

  while (i < lines.length) {
    const line = lines[i]

    if (line.type === 'header') {
      const match = line.text.match(/@@ -(\d+)(?:,\d+)? \+(\d+)/)
      if (match) { leftNum = parseInt(match[1]) - 1; rightNum = parseInt(match[2]) - 1 }
      rows.push({ type: 'header', left: line.text, right: line.text, leftNum: '', rightNum: '' })
      i++
      continue
    }

    if (line.type === 'context') {
      leftNum++
      rightNum++
      rows.push({ type: 'context', left: line.text, right: line.text, leftNum, rightNum })
      i++
      continue
    }

    // Collect consecutive removes and adds to pair them
    const removes = []
    const adds = []
    while (i < lines.length && lines[i].type === 'remove') {
      removes.push(lines[i].text)
      i++
    }
    while (i < lines.length && lines[i].type === 'add') {
      adds.push(lines[i].text)
      i++
    }

    const maxLen = Math.max(removes.length, adds.length)
    for (let j = 0; j < maxLen; j++) {
      const hasRemove = j < removes.length
      const hasAdd = j < adds.length
      if (hasRemove) leftNum++
      if (hasAdd) rightNum++

      if (hasRemove && hasAdd) {
        // Changed line — show on both sides
        rows.push({ type: 'change', left: removes[j], right: adds[j], leftNum, rightNum })
      } else if (hasRemove) {
        rows.push({ type: 'remove', left: removes[j], right: '', leftNum, rightNum: '' })
      } else {
        rows.push({ type: 'add', left: '', right: adds[j], leftNum: '', rightNum })
      }
    }
  }
  return rows
})
</script>

<style scoped>
.diff-view table {
  border-collapse: collapse;
}
.diff-view td {
  font-size: 0.75rem;
  line-height: 1.5;
  vertical-align: top;
}
tr:has(td.invisible) {
  opacity: 0.3;
}
</style>
