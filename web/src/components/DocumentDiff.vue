<template>
  <div class="document-diff prose prose-invert prose-sm max-w-none px-8 py-6" v-html="renderedDiff"></div>
</template>

<script setup>
import { computed } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'

const props = defineProps({
  diff: { type: String, default: '' },
})

const renderedDiff = computed(() => {
  if (!props.diff) return '<p class="text-slate-600 text-center">No changes</p>'

  const lines = props.diff.split('\n')

  // Collect removed and added blocks, interleaved with context
  const segments = [] // { type: 'context'|'removed'|'added', text: string }
  let removeBuf = []
  let addBuf = []

  function flushChanges() {
    if (removeBuf.length > 0) {
      segments.push({ type: 'removed', text: removeBuf.join('\n') })
      removeBuf = []
    }
    if (addBuf.length > 0) {
      segments.push({ type: 'added', text: addBuf.join('\n') })
      addBuf = []
    }
  }

  for (const line of lines) {
    if (line.startsWith('@@') || line.startsWith('---') || line.startsWith('+++')) continue
    if (line.startsWith('-')) {
      removeBuf.push(line.substring(1))
    } else if (line.startsWith('+')) {
      addBuf.push(line.substring(1))
    } else {
      flushChanges()
      segments.push({ type: 'context', text: line.startsWith(' ') ? line.substring(1) : line })
    }
  }
  flushChanges()

  // Merge consecutive same-type segments
  const merged = []
  for (const seg of segments) {
    if (merged.length > 0 && merged[merged.length - 1].type === seg.type) {
      merged[merged.length - 1].text += '\n' + seg.text
    } else {
      merged.push({ ...seg })
    }
  }

  // Build one markdown string with HTML markers for changes.
  // We wrap changed blocks in div tags so they survive markdown rendering.
  const mdParts = []
  for (const seg of merged) {
    if (seg.type === 'context') {
      mdParts.push(seg.text)
    } else if (seg.type === 'removed') {
      mdParts.push(`\n<div class="diff-block-removed">\n\n${seg.text}\n\n</div>\n`)
    } else if (seg.type === 'added') {
      mdParts.push(`\n<div class="diff-block-added">\n\n${seg.text}\n\n</div>\n`)
    }
  }

  const md = mdParts.join('\n')
  const html = marked.parse(md, { breaks: true })
  return DOMPurify.sanitize(html, { ADD_TAGS: ['div', 'span'], ADD_ATTR: ['class'] })
})
</script>

<style scoped>
.document-diff :deep(.diff-block-removed) {
  background-color: rgba(239, 68, 68, 0.1);
  border-left: 3px solid rgba(239, 68, 68, 0.4);
  padding: 0.5rem 1rem;
  margin: 0.5rem 0;
  border-radius: 0 0.375rem 0.375rem 0;
  text-decoration: line-through;
  text-decoration-color: rgba(239, 68, 68, 0.4);
  color: #fca5a5;
}

.document-diff :deep(.diff-block-added) {
  background-color: rgba(34, 197, 94, 0.1);
  border-left: 3px solid rgba(34, 197, 94, 0.4);
  padding: 0.5rem 1rem;
  margin: 0.5rem 0;
  border-radius: 0 0.375rem 0.375rem 0;
  color: #86efac;
}

.document-diff :deep(.diff-block-removed p),
.document-diff :deep(.diff-block-added p) {
  margin-bottom: 0.25em;
}

.document-diff :deep(.diff-block-removed h1),
.document-diff :deep(.diff-block-removed h2),
.document-diff :deep(.diff-block-removed h3),
.document-diff :deep(.diff-block-added h1),
.document-diff :deep(.diff-block-added h2),
.document-diff :deep(.diff-block-added h3) {
  margin-top: 0.25em;
  color: inherit;
}

.document-diff :deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin: 0.75em 0;
}

.document-diff :deep(th),
.document-diff :deep(td) {
  border: 1px solid #334155;
  padding: 0.375rem 0.75rem;
  text-align: left;
}

.document-diff :deep(th) {
  background-color: rgba(30, 41, 59, 0.5);
  font-weight: 600;
}
</style>
