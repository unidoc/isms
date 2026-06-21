<template>
  <node-view-wrapper class="editor-code-block" :class="{ wrapped: node.attrs.wrapped }">
    <div class="code-block-controls" contenteditable="false">
      <select
        class="code-lang-select"
        :value="displayLang"
        @change="onChange"
      >
        <option value="plaintext">Plain text</option>
        <option v-for="l in languages" :key="l.id" :value="l.id">{{ l.label }}</option>
      </select>
      <button
        class="code-wrap-btn"
        type="button"
        :class="{ active: node.attrs.wrapped }"
        :title="node.attrs.wrapped ? 'Disable word wrap' : 'Enable word wrap'"
        @click="toggleWrap"
      >Wrap</button>
    </div>
    <pre><span class="code-gutter" contenteditable="false" aria-hidden="true">{{ lineNumbers }}</span><node-view-content as="code" /></pre>
  </node-view-wrapper>
</template>

<script setup>
import { computed } from 'vue'
import { NodeViewWrapper, NodeViewContent, nodeViewProps } from '@tiptap/vue-3'

const props = defineProps(nodeViewProps)

// Common highlight.js aliases → the canonical id used in the dropdown, so a
// fence like ```js or ```py shows the right option selected. We only normalize
// the *displayed* value; the stored language attribute is left untouched unless
// the user actively picks a different one (so ```js stays ```js on save).
const ALIASES = {
  js: 'javascript', ts: 'typescript', py: 'python', rb: 'ruby', yml: 'yaml',
  sh: 'bash', shell: 'bash', 'c++': 'cpp', cs: 'csharp', kt: 'kotlin',
  rs: 'rust', md: 'markdown', html: 'xml', htm: 'xml', plain: 'plaintext', text: 'plaintext',
}
const displayLang = computed(() => {
  const l = (props.node.attrs.language || 'plaintext').toLowerCase()
  return ALIASES[l] || l
})

// Live line-number gutter, matching the read view. Recomputes as the node's
// text changes (TipTap updates the `node` prop on every edit). Numbers the
// actual editable lines — including any trailing blanks you've typed — because
// the gutter must line up 1:1 with what you're editing. Hidden when wrapped.
const lineNumbers = computed(() => {
  const n = (props.node.textContent || '').split('\n').length
  let s = ''
  for (let i = 1; i <= n; i++) s += (i > 1 ? '\n' : '') + i
  return s
})

// Curated list — every id is a language registered by lowlight's `common` set,
// so it actually highlights. Sorted by label for the dropdown.
const languages = [
  { id: 'bash', label: 'Bash' },
  { id: 'c', label: 'C' },
  { id: 'cpp', label: 'C++' },
  { id: 'csharp', label: 'C#' },
  { id: 'css', label: 'CSS' },
  { id: 'diff', label: 'Diff' },
  { id: 'go', label: 'Go' },
  { id: 'graphql', label: 'GraphQL' },
  { id: 'xml', label: 'HTML / XML' },
  { id: 'ini', label: 'INI / TOML' },
  { id: 'java', label: 'Java' },
  { id: 'javascript', label: 'JavaScript' },
  { id: 'json', label: 'JSON' },
  { id: 'kotlin', label: 'Kotlin' },
  { id: 'lua', label: 'Lua' },
  { id: 'makefile', label: 'Makefile' },
  { id: 'markdown', label: 'Markdown' },
  { id: 'perl', label: 'Perl' },
  { id: 'php', label: 'PHP' },
  { id: 'python', label: 'Python' },
  { id: 'r', label: 'R' },
  { id: 'ruby', label: 'Ruby' },
  { id: 'rust', label: 'Rust' },
  { id: 'scss', label: 'SCSS' },
  { id: 'shell', label: 'Shell' },
  { id: 'sql', label: 'SQL' },
  { id: 'swift', label: 'Swift' },
  { id: 'typescript', label: 'TypeScript' },
  { id: 'yaml', label: 'YAML' },
]

function onChange(e) {
  props.updateAttributes({ language: e.target.value })
}

function toggleWrap() {
  props.updateAttributes({ wrapped: !props.node.attrs.wrapped })
}
</script>
