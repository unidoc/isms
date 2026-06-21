<template>
  <div class="document-editor" ref="editorRoot">
    <!-- Toolbar (sticky) -->
    <div class="sticky top-0 z-20 flex flex-wrap items-center gap-0.5 px-3 py-2 bg-slate-900 border-b border-slate-800 rounded-t-xl">
      <!-- History -->
      <button @click="editor?.chain().focus().undo().run()" :disabled="!editor?.can().undo()"
        class="toolbar-btn" title="Undo (Ctrl+Z)">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 15L3 9m0 0l6-6M3 9h12a6 6 0 010 12h-3" />
        </svg>
      </button>
      <button @click="editor?.chain().focus().redo().run()" :disabled="!editor?.can().redo()"
        class="toolbar-btn" title="Redo (Ctrl+Shift+Z)">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M15 15l6-6m0 0l-6-6m6 6H9a6 6 0 000 12h3" />
        </svg>
      </button>

      <div class="w-px h-5 bg-slate-700 mx-1" />

      <!-- Text formatting -->
      <button @click="editor?.chain().focus().toggleBold().run()"
        :class="{ 'toolbar-btn-active': editor?.isActive('bold') }" class="toolbar-btn" title="Bold (Ctrl+B)">
        <span class="font-bold text-sm">B</span>
      </button>
      <button @click="editor?.chain().focus().toggleItalic().run()"
        :class="{ 'toolbar-btn-active': editor?.isActive('italic') }" class="toolbar-btn" title="Italic (Ctrl+I)">
        <span class="italic text-sm">I</span>
      </button>
      <button @click="editor?.chain().focus().toggleStrike().run()"
        :class="{ 'toolbar-btn-active': editor?.isActive('strike') }" class="toolbar-btn" title="Strikethrough (Ctrl+Shift+X)">
        <span class="line-through text-sm">S</span>
      </button>

      <!-- Text color -->
      <div class="relative">
        <button @click="showTextColorPicker = !showTextColorPicker" class="toolbar-btn" title="Text color">
          <span class="text-sm font-bold leading-none">A</span>
          <span class="absolute bottom-1 left-1/2 -translate-x-1/2 w-4 h-1 rounded-sm" :style="{ backgroundColor: currentTextColor || '#94a3b8' }"></span>
        </button>
        <div v-if="showTextColorPicker" class="absolute top-full left-0 mt-1 z-50 bg-slate-800 border border-slate-700 rounded-lg shadow-2xl">
          <div class="px-2 pt-2 text-[9px] text-slate-500 uppercase tracking-wider">Text color</div>
          <ColorPicker :modelValue="currentTextColor || '#94a3b8'" @update:modelValue="applyTextColor" @apply="showTextColorPicker = false" />
        </div>
      </div>
      <div v-if="showTextColorPicker" class="fixed inset-0 z-40" @click="showTextColorPicker = false" />

      <div class="w-px h-5 bg-slate-700 mx-1" />

      <!-- Headings -->
      <button v-for="level in [1,2,3,4]" :key="level"
        @click="editor?.chain().focus().toggleHeading({ level }).run()"
        :class="{ 'toolbar-btn-active': editor?.isActive('heading', { level }) }"
        class="toolbar-btn" :title="'Heading ' + level + ' (Ctrl+Alt+' + level + ')'">
        <span class="text-xs font-bold">H{{ level }}</span>
      </button>

      <div class="w-px h-5 bg-slate-700 mx-1" />

      <!-- Lists -->
      <button @click="editor?.chain().focus().toggleBulletList().run()"
        :class="{ 'toolbar-btn-active': editor?.isActive('bulletList') }" class="toolbar-btn" title="Bullet list (Ctrl+Shift+8)">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
        </svg>
      </button>
      <button @click="editor?.chain().focus().toggleOrderedList().run()"
        :class="{ 'toolbar-btn-active': editor?.isActive('orderedList') }" class="toolbar-btn" title="Ordered list (Ctrl+Shift+7)">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M8 6h13M8 12h13M8 18h13M3.5 6l1-1v4M3 18h3m-3-2l2.286-2.286" />
        </svg>
      </button>

      <!-- Blockquote -->
      <button @click="editor?.chain().focus().toggleBlockquote().run()"
        :class="{ 'toolbar-btn-active': editor?.isActive('blockquote') }" class="toolbar-btn" title="Blockquote (Ctrl+Shift+B)">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 8.25h9m-9 3H12m-9.75 1.51c0 1.6 1.123 2.994 2.707 3.227 1.087.16 2.185.283 3.293.369V21l4.076-4.076a1.526 1.526 0 011.037-.443 48.282 48.282 0 005.68-.494c1.584-.233 2.707-1.626 2.707-3.228V6.741c0-1.602-1.123-2.995-2.707-3.228A48.394 48.394 0 0012 3c-2.392 0-4.744.175-7.043.513C3.373 3.746 2.25 5.14 2.25 6.741v6.018z" />
        </svg>
      </button>

      <!-- Code block + HR -->
      <button @click="editor?.chain().focus().toggleCodeBlock().run()"
        :class="{ 'toolbar-btn-active': editor?.isActive('codeBlock') }" class="toolbar-btn" title="Code block (Ctrl+Alt+C)">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M17.25 6.75L22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3l-4.5 16.5" />
        </svg>
      </button>
      <button @click="editor?.chain().focus().setHorizontalRule().run()" class="toolbar-btn" title="Horizontal rule">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M3 12h18" />
        </svg>
      </button>

      <div class="w-px h-5 bg-slate-700 mx-1" />

      <!-- Link -->
      <button @click="setLink" :class="{ 'toolbar-btn-active': editor?.isActive('link') }" class="toolbar-btn" title="Link (Ctrl+K)">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.193-9.193a4.5 4.5 0 016.364 6.364l-4.5 4.5a4.5 4.5 0 01-7.244-1.242" />
        </svg>
      </button>
      <div v-if="showLinkInput" class="flex items-center gap-1 ml-1">
        <input v-model="linkUrl" @keydown.enter="confirmLink" @keydown.escape="cancelLink"
          class="link-url-input w-48 px-2 py-1 bg-slate-800 border border-slate-600 rounded text-xs text-white placeholder-slate-500 focus:outline-none focus:border-blue-500"
          placeholder="https://..." />
        <button @click="confirmLink" class="px-2 py-1 text-xs bg-blue-600 hover:bg-blue-500 text-white rounded">OK</button>
        <button @click="cancelLink" class="px-1 py-1 text-xs text-slate-500 hover:text-white">&#x2715;</button>
      </div>

      <div class="w-px h-5 bg-slate-700 mx-1" />

      <!-- Insert table -->
      <button @click="insertTable" class="toolbar-btn" title="Insert table">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M3.375 19.5h17.25m-17.25 0a1.125 1.125 0 01-1.125-1.125M3.375 19.5h7.5c.621 0 1.125-.504 1.125-1.125m-9.75 0V5.625m0 12.75v-1.5c0-.621.504-1.125 1.125-1.125m18.375 2.625V5.625m0 12.75c0 .621-.504 1.125-1.125 1.125m1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125m0 3.75h-7.5A1.125 1.125 0 0112 18.375m9.75-12.75c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125m19.5 0v1.5c0 .621-.504 1.125-1.125 1.125M2.25 5.625v1.5c0 .621.504 1.125 1.125 1.125m0 0h17.25m-17.25 0h7.5c.621 0 1.125.504 1.125 1.125M3.375 8.25c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125m17.25-3.75h-7.5c-.621 0-1.125.504-1.125 1.125m8.625-1.125c.621 0 1.125.504 1.125 1.125v1.5c0 .621-.504 1.125-1.125 1.125m-17.25 0h7.5m-7.5 0c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125M12 10.875v-1.5m0 1.5c0 .621-.504 1.125-1.125 1.125M12 10.875c0 .621.504 1.125 1.125 1.125m-2.25 0c.621 0 1.125.504 1.125 1.125M12 12h7.5m-7.5 0c-.621 0-1.125.504-1.125 1.125M3.375 12h7.5m-7.5 0c-.621 0-1.125.504-1.125 1.125m0 1.5v-1.5m0 0c0-.621.504-1.125 1.125-1.125m0 0h7.5" />
        </svg>
      </button>
    </div>

    <!-- Editor content -->
    <div class="relative" @contextmenu="onCellRightClick" @keydown="onSlashKeydown">
      <editor-content :editor="editor" class="editor-content" />

      <!-- Slash command menu -->
      <div v-if="slashMenu.show" ref="slashMenuRef"
        class="fixed z-[100] bg-slate-800 border border-slate-700 rounded-lg shadow-2xl py-1 min-w-[260px] max-h-[280px] overflow-y-auto"
        :style="{ bottom: slashMenu.above ? ('calc(100vh - ' + slashMenu.y + 'px)') : undefined, top: slashMenu.above ? undefined : (slashMenu.y + 'px'), left: slashMenu.x + 'px' }">
        <div class="px-3 py-1.5 text-[9px] text-slate-500 uppercase tracking-wider">Insert block</div>
        <button v-for="(cmd, idx) in filteredSlashCommands" :key="cmd.id"
          @click="executeSlashCommand(cmd)"
          @mouseenter="slashSelectedIdx = idx"
          class="w-full px-3 py-2 text-left flex items-center gap-3 transition-colors"
          :class="slashSelectedIdx === idx ? 'bg-blue-600/20 text-blue-300' : 'text-slate-300 hover:bg-slate-700/50'">
          <div class="w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0"
            :class="slashSelectedIdx === idx ? 'bg-blue-600/20' : 'bg-slate-700/50'">
            <svg v-if="cmd.icon === 'table'" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M3.375 19.5h17.25m-17.25 0a1.125 1.125 0 01-1.125-1.125M3.375 19.5h7.5c.621 0 1.125-.504 1.125-1.125m-9.75 0V5.625m0 12.75v-1.5c0-.621.504-1.125 1.125-1.125m18.375 2.625V5.625m0 12.75c0 .621-.504 1.125-1.125 1.125m1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125m0 3.75h-7.5A1.125 1.125 0 0112 18.375" />
            </svg>
            <span v-else-if="cmd.icon === 'h1'" class="text-xs font-bold">H1</span>
            <span v-else-if="cmd.icon === 'h2'" class="text-xs font-bold">H2</span>
            <span v-else-if="cmd.icon === 'h3'" class="text-xs font-bold">H3</span>
            <svg v-else-if="cmd.icon === 'list'" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
            </svg>
            <svg v-else-if="cmd.icon === 'list-ol'" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M8 6h13M8 12h13M8 18h13M3.5 6l1-1v4" />
            </svg>
            <svg v-else-if="cmd.icon === 'quote'" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 8.25h9m-9 3H12m-9.75 1.51c0 1.6 1.123 2.994 2.707 3.227 1.087.16 2.185.283 3.293.369V21l4.076-4.076" />
            </svg>
            <svg v-else-if="cmd.icon === 'code'" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M17.25 6.75L22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25" />
            </svg>
            <svg v-else-if="cmd.icon === 'hr'" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M3 12h18" />
            </svg>
            <svg v-else-if="cmd.icon === 'link'" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.193-9.193a4.5 4.5 0 016.364 6.364l-4.5 4.5a4.5 4.5 0 01-7.244-1.242" />
            </svg>
          </div>
          <div class="min-w-0">
            <div class="text-sm"><span class="font-semibold">{{ cmd.label }}</span> <span v-if="cmd.shorthand" class="text-[10px] text-slate-500 font-mono ml-1">/{{ cmd.shorthand }}</span></div>
            <div class="text-[10px] text-slate-500 truncate">{{ cmd.desc }}</div>
          </div>
        </button>
        <div v-if="filteredSlashCommands.length === 0" class="px-3 py-3 text-xs text-slate-600 text-center">No matching commands</div>
      </div>
      <!-- Click-away for slash menu -->
      <div v-if="slashMenu.show" class="fixed inset-0 z-30" @click="slashMenu.show = false" />

      <!-- Entity picker (for /risk, /legal, /asset, etc.) -->
      <div v-if="entityPicker.show"
        class="absolute z-40 bg-slate-800 border border-slate-700 rounded-lg shadow-2xl py-1 min-w-[320px] max-h-[320px] overflow-hidden flex flex-col"
        :style="{ bottom: entityPicker.above ? ('calc(100% - ' + entityPicker.y + 'px)') : undefined, top: entityPicker.above ? undefined : (entityPicker.y + 'px'), left: entityPicker.x + 'px' }">
        <div class="px-3 py-1.5 text-[9px] text-slate-500 uppercase tracking-wider flex items-center gap-2">
          <span>Link {{ entityPicker.typeLabel }}</span>
          <button @click="entityPicker.show = false" class="ml-auto text-slate-600 hover:text-slate-400">
            <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <div class="px-2 pb-1">
          <input
            ref="entitySearchInput"
            v-model="entityPicker.search"
            @keydown.stop="onEntityPickerKeydown"
            class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-xs text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500"
            placeholder="Search..."
            autofocus
          />
        </div>
        <div class="overflow-y-auto flex-1">
          <div v-if="entityPicker.loading" class="px-3 py-3 text-xs text-slate-500 text-center">Loading...</div>
          <button v-else v-for="(item, idx) in filteredEntities" :key="item.id"
            @click="selectEntity(item)"
            @mouseenter="entityPicker.selectedIdx = idx"
            class="w-full px-3 py-2 text-left flex items-center gap-2 transition-colors"
            :class="entityPicker.selectedIdx === idx ? 'bg-blue-600/20 text-blue-300' : 'text-slate-300 hover:bg-slate-700/50'">
            <span class="text-[10px] font-mono text-slate-500 flex-shrink-0">{{ item.identifier || item.document_id || '' }}</span>
            <span class="text-xs truncate">{{ item.title || item.name || '' }}</span>
          </button>
          <div v-if="!entityPicker.loading && filteredEntities.length === 0" class="px-3 py-3 text-xs text-slate-600 text-center">No results</div>
        </div>
      </div>
      <div v-if="entityPicker.show" class="fixed inset-0 z-30" @click="entityPicker.show = false" />

      <!-- Cell context menu (right-click on table cell) -->
      <div v-if="cellMenu.show && !cellMenu.colorOpen" class="fixed z-50 bg-slate-800 border border-slate-700 rounded-lg shadow-2xl py-1 min-w-[180px]"
        :style="{ top: cellMenu.y + 'px', left: cellMenu.x + 'px' }">
        <button @click="cellMenu.colorOpen = true" class="ctx-menu-item flex items-center gap-2">
          <div class="w-3.5 h-3.5 rounded-sm border border-slate-600" :style="{ backgroundColor: currentCellColor || 'transparent' }" />
          Cell color...
        </button>
        <div class="border-t border-slate-700 my-1" />
        <button @click="editor?.chain().focus().addRowBefore().run(); cellMenu.show = false" class="ctx-menu-item">Add row above</button>
        <button @click="editor?.chain().focus().addRowAfter().run(); cellMenu.show = false" class="ctx-menu-item">Add row below</button>
        <button @click="editor?.chain().focus().addColumnBefore().run(); cellMenu.show = false" class="ctx-menu-item">Add column left</button>
        <button @click="editor?.chain().focus().addColumnAfter().run(); cellMenu.show = false" class="ctx-menu-item">Add column right</button>
        <div class="border-t border-slate-700 my-1" />
        <button @click="editor?.chain().focus().deleteRow().run(); cellMenu.show = false" class="ctx-menu-item text-red-400">Delete row</button>
        <button @click="editor?.chain().focus().deleteColumn().run(); cellMenu.show = false" class="ctx-menu-item text-red-400">Delete column</button>
        <button @click="editor?.chain().focus().deleteTable().run(); cellMenu.show = false" class="ctx-menu-item text-red-400">Delete table</button>
      </div>

      <!-- Cell color picker (separate panel) -->
      <div v-if="cellMenu.show && cellMenu.colorOpen" class="fixed z-50 bg-slate-800 border border-slate-700 rounded-lg shadow-2xl"
        :style="{ top: cellMenu.y + 'px', left: cellMenu.x + 'px' }">
        <div class="flex items-center gap-2 px-3 pt-2">
          <button @click="cellMenu.colorOpen = false" class="text-slate-400 hover:text-white">
            <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
            </svg>
          </button>
          <span class="text-[10px] text-slate-400 font-medium">Cell color</span>
        </div>
        <ColorPicker :modelValue="currentCellColor || '#3b82f6'" @update:modelValue="setCellColor" @apply="cellMenu.show = false; cellMenu.colorOpen = false" />
      </div>

      <!-- Click-away for cell menu -->
      <div v-if="cellMenu.show" class="fixed inset-0 z-40" @click="cellMenu.show = false; cellMenu.colorOpen = false" @contextmenu.prevent="cellMenu.show = false" />
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, watch, onBeforeUnmount, onMounted, computed, nextTick } from 'vue'
import { useEditor, EditorContent, VueNodeViewRenderer } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight'
import { createLowlight, common } from 'lowlight'
import CodeBlockView from './CodeBlockView.vue'
import { Table, TableRow, TableHeader, TableCell } from '@tiptap/extension-table'
import Highlight from '@tiptap/extension-highlight'
import { TextStyle } from '@tiptap/extension-text-style'
import Color from '@tiptap/extension-color'
import Link from '@tiptap/extension-link'
import Placeholder from '@tiptap/extension-placeholder'
import ColorPicker from './ColorPicker.vue'
import { api } from '../api.js'
import { slashCommands as sharedSlashCommands, fetchPickerItems, resolveEntity } from '../composables/useSlashCommands.js'
import { markdownToHtml, htmlToMarkdown } from '../composables/useMarkdownConvert.js'

const props = defineProps({
  modelValue: { type: String, default: '' },
  editable: { type: Boolean, default: true },
  documentId: { type: String, default: '' },
  selfType: { type: String, default: '' },
  selfId: { type: String, default: '' },
})

const emit = defineEmits(['update:modelValue', 'save'])
const editorRoot = ref(null)

// --- Color presets ---
const presetColors = [
  { label: 'None', value: null },
  { label: 'Red', value: '#ef4444' },
  { label: 'Red light', value: '#fca5a5' },
  { label: 'Amber', value: '#f59e0b' },
  { label: 'Amber light', value: '#fcd34d' },
  { label: 'Green', value: '#22c55e' },
  { label: 'Green light', value: '#86efac' },
  { label: 'Blue', value: '#3b82f6' },
  { label: 'Blue light', value: '#93c5fd' },
  { label: 'Purple', value: '#8b5cf6' },
  { label: 'Gray', value: '#6b7280' },
  { label: 'Gray light', value: '#d1d5db' },
]

const showTextColorPicker = ref(false)

const currentTextColor = computed(() => {
  if (!editor.value) return null
  const attrs = editor.value.getAttributes('textStyle')
  return attrs.color || null
})

function applyTextColor(color) {
  if (!editor.value) return
  if (color) {
    editor.value.chain().focus().setColor(color).run()
  } else {
    editor.value.chain().focus().unsetColor().run()
  }
}

// --- Cell right-click context menu ---
const cellMenu = reactive({ show: false, colorOpen: false, x: 0, y: 0 })

const CELL_MENU_WIDTH = 200
const CELL_MENU_HEIGHT = 340 // approximate max height

function onCellRightClick(e) {
  const cell = e.target.closest('td, th')
  if (!cell || !editor.value?.isActive('table')) return
  e.preventDefault()

  const vw = window.innerWidth
  const vh = window.innerHeight

  let x = e.clientX
  let y = e.clientY

  // Clamp horizontally
  if (x + CELL_MENU_WIDTH > vw - 8) {
    x = vw - CELL_MENU_WIDTH - 8
  }
  if (x < 8) x = 8

  // If click is in the lower half, show menu above click point
  if (y > vh / 2) {
    y = y - CELL_MENU_HEIGHT
    if (y < 8) y = 8
  } else {
    // Clamp so menu doesn't go off bottom
    if (y + CELL_MENU_HEIGHT > vh - 8) {
      y = vh - CELL_MENU_HEIGHT - 8
    }
  }

  cellMenu.x = x
  cellMenu.y = y
  cellMenu.show = true
}

// --- Custom TableCell with backgroundColor ---
const CustomTableCell = TableCell.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      backgroundColor: {
        default: null,
        parseHTML: element => element.getAttribute('style')?.match(/background-color:\s*([^;]+)/)?.[1]?.trim() || null,
        renderHTML: attributes => {
          if (!attributes.backgroundColor) return {}
          return { style: `background-color: ${attributes.backgroundColor}` }
        },
      },
    }
  },
})

const CustomTableHeader = TableHeader.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      backgroundColor: {
        default: null,
        parseHTML: element => element.getAttribute('style')?.match(/background-color:\s*([^;]+)/)?.[1]?.trim() || null,
        renderHTML: attributes => {
          if (!attributes.backgroundColor) return {}
          return { style: `background-color: ${attributes.backgroundColor}` }
        },
      },
    }
  },
})

// --- Editor setup ---
const lowlight = createLowlight(common)
const editor = useEditor({
  content: markdownToHtml(props.modelValue),
  editable: props.editable,
  extensions: [
    StarterKit.configure({
      heading: { levels: [1, 2, 3, 4] },
      codeBlock: false, // replaced by CodeBlockLowlight (highlighting + language picker)
    }),
    CodeBlockLowlight.extend({
      addAttributes() {
        return {
          ...this.parent?.(),
          // Per-block word-wrap, round-tripped via a `wrap` token in the fence
          // info string (see useMarkdownConvert).
          wrapped: {
            default: false,
            parseHTML: el => el.getAttribute('data-wrapped') === 'true',
            renderHTML: attrs => (attrs.wrapped ? { 'data-wrapped': 'true' } : {}),
          },
        }
      },
      addNodeView() { return VueNodeViewRenderer(CodeBlockView) },
    }).configure({ lowlight, defaultLanguage: 'plaintext' }),
    Table.configure({ resizable: true }),
    TableRow,
    CustomTableCell,
    CustomTableHeader,
    Highlight.configure({ multicolor: true }),
    TextStyle,
    Color,
    Link.configure({ openOnClick: false }),
    Placeholder.configure({ placeholder: 'Start writing...' }),
  ],
  editorProps: {
    handleKeyDown: (view, event) => {
      // Ctrl+S → save
      if ((event.ctrlKey || event.metaKey) && event.key === 's') {
        event.preventDefault()
        emit('save')
        return true
      }
      // Slash menu navigation
      if (slashMenu.show) {
        if (['ArrowDown', 'ArrowUp', 'Enter', 'Escape'].includes(event.key)) {
          onSlashKeydown(event)
          return true
        }
      }
      return false
    },
  },
  onUpdate: ({ editor: ed }) => {
    const html = ed.getHTML()
    const md = htmlToMarkdown(html)
    emit('update:modelValue', md)
    checkSlashMenu(ed)
  },
})

// --- Slash menu (Nuclino-style) ---
const slashMenu = reactive({ show: false, x: 0, y: 0, query: '', startPos: 0 })
const slashMenuRef = ref(null)
const slashSelectedIdx = ref(0)

// Map a shared slash-command id to a tiptap action. Markdown-style commands in
// the shared list (which produce textarea text) are mapped to their semantic
// tiptap equivalent so the rich editor inserts a real block, not literal markdown.
function tiptapAction(cmd) {
  switch (cmd.id) {
    case 'h1':      return (ed) => ed.chain().focus().toggleHeading({ level: 1 }).run()
    case 'h2':      return (ed) => ed.chain().focus().toggleHeading({ level: 2 }).run()
    case 'h3':      return (ed) => ed.chain().focus().toggleHeading({ level: 3 }).run()
    case 'bullet':  return (ed) => ed.chain().focus().toggleBulletList().run()
    case 'ordered': return (ed) => ed.chain().focus().toggleOrderedList().run()
    case 'quote':   return (ed) => ed.chain().focus().toggleBlockquote().run()
    case 'code':    return (ed) => ed.chain().focus().toggleCodeBlock().run()
    case 'hr':      return (ed) => ed.chain().focus().setHorizontalRule().run()
    default:
      if (cmd.picker) {
        return () => openEntityPicker(cmd.picker.type, cmd.picker.label, () => fetchPickerItems(cmd.picker))
      }
      return () => {}
  }
}

// Slash commands available in the rich editor: table (editor-only) plus the shared list.
const slashCommands = [
  { id: 'table', label: 'Table', shorthand: null, desc: 'Insert a 3x3 table', icon: 'table',
    action: (ed) => ed.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run() },
  ...sharedSlashCommands.map(cmd => ({ ...cmd, action: tiptapAction(cmd) })),
]

const filteredSlashCommands = computed(() => {
  const q = slashMenu.query.toLowerCase()
  if (!q) return slashCommands
  return slashCommands.filter(c =>
    c.label.toLowerCase().includes(q) ||
    c.id.includes(q) ||
    c.desc.toLowerCase().includes(q) ||
    (c.shorthand && c.shorthand.includes(q))
  )
})

function checkSlashMenu(ed) {
  const { state } = ed
  const { $from } = state.selection
  const textBefore = $from.parent.textContent?.slice(0, $from.parentOffset) || ''

  // Find "/" trigger
  const slashIdx = textBefore.lastIndexOf('/')
  if (slashIdx >= 0 && (slashIdx === 0 || textBefore[slashIdx - 1] === ' ' || textBefore[slashIdx - 1] === '\n')) {
    const query = textBefore.slice(slashIdx + 1)
    // Don't show for URLs or paths
    if (query.includes(' ') || query.includes('/') || query.length > 15) {
      slashMenu.show = false
      return
    }

    slashMenu.query = query
    slashMenu.startPos = $from.pos - query.length - 1 // position of "/"

    // Position the menu using viewport coords (fixed positioning).
    // Default to ABOVE the cursor, but flip BELOW if it would be obscured by the
    // sticky toolbar (which sits at the top of the editor's scroll container).
    const coords = ed.view.coordsAtPos($from.pos)
    slashMenu.x = coords.left
    // Estimated menu height for fitting decision (max-h is 280px in CSS)
    const menuHeight = 280
    // Find the toolbar element to compute its bottom edge in viewport coords.
    const toolbarEl = ed.view.dom.closest('.relative')?.parentElement?.querySelector('.sticky')
    const toolbarBottom = toolbarEl ? toolbarEl.getBoundingClientRect().bottom : 0
    if (coords.top - menuHeight < toolbarBottom + 8) {
      // Not enough room above — flip below the cursor line
      slashMenu.y = coords.bottom + 8
      slashMenu.above = false
    } else {
      slashMenu.y = coords.top - 8
      slashMenu.above = true
    }

    slashMenu.show = filteredSlashCommands.value.length > 0
    slashSelectedIdx.value = 0
  } else {
    slashMenu.show = false
  }
}

function executeSlashCommand(cmd) {
  if (!editor.value) return
  // Delete the "/query" text
  const { state } = editor.value
  const { $from } = state.selection
  editor.value.chain().focus()
    .command(({ tr }) => {
      tr.delete(slashMenu.startPos, $from.pos)
      return true
    })
    .run()
  // Execute the command
  cmd.action(editor.value)
  slashMenu.show = false
}

function onSlashKeydown(e) {
  if (!slashMenu.show) return
  const cmds = filteredSlashCommands.value
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    slashSelectedIdx.value = (slashSelectedIdx.value + 1) % cmds.length
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    slashSelectedIdx.value = (slashSelectedIdx.value - 1 + cmds.length) % cmds.length
  } else if (e.key === 'Enter') {
    e.preventDefault()
    if (cmds[slashSelectedIdx.value]) {
      executeSlashCommand(cmds[slashSelectedIdx.value])
    }
  } else if (e.key === 'Escape') {
    e.preventDefault()
    slashMenu.show = false
  }
}

// --- Entity picker (for /risk, /legal, /asset, etc.) ---
const entityPicker = reactive({ show: false, x: 0, y: 0, above: false, search: '', selectedIdx: 0, loading: false, items: [], type: '', typeLabel: '' })
const entitySearchInput = ref(null)

const entityTypeMap = {
  risk: { idField: 'identifier', nameField: 'title', refType: 'RISK' },
  legal: { idField: 'identifier', nameField: 'title', refType: 'LEGAL' },
  asset: { idField: 'identifier', nameField: 'name', refType: 'ASSET' },
  supplier: { idField: 'identifier', nameField: 'name', refType: 'SUPPLIER' },
  system: { idField: 'identifier', nameField: 'name', refType: 'SYSTEM' },
  document: { idField: 'document_id', nameField: 'title', refType: 'DOC' },
  incident: { idField: 'id', nameField: 'title', refType: 'INCIDENT' },
}

const filteredEntities = computed(() => {
  const q = entityPicker.search.toLowerCase()
  if (!q) return entityPicker.items
  return entityPicker.items.filter(item => {
    const map = entityTypeMap[entityPicker.type] || {}
    const id = String(item[map.idField] || item.identifier || item.document_id || item.id || '')
    const name = String(item[map.nameField] || item.title || item.name || '')
    return id.toLowerCase().includes(q) || name.toLowerCase().includes(q)
  })
})

async function openEntityPicker(type, typeLabel, fetchFn) {
  entityPicker.type = type
  entityPicker.typeLabel = typeLabel
  entityPicker.search = ''
  entityPicker.selectedIdx = 0
  entityPicker.loading = true
  entityPicker.items = []
  // Position at same place as slash menu
  entityPicker.x = slashMenu.x
  entityPicker.y = slashMenu.y
  entityPicker.above = slashMenu.above
  entityPicker.show = true
  try {
    let items = await fetchFn()
    items = items || []
    if (props.selfType && props.selfId && type === props.selfType) {
      items = items.filter(it => {
        const { id } = resolveEntity(type, it)
        return String(id) !== String(props.selfId)
      })
    }
    entityPicker.items = items
  } catch {
    entityPicker.items = []
  }
  entityPicker.loading = false
  // Focus the search input
  await nextTick()
  entitySearchInput.value?.focus()
}

function selectEntity(item) {
  if (!editor.value) return
  const map = entityTypeMap[entityPicker.type] || {}
  const id = String(item[map.idField] || item.identifier || item.document_id || item.id || '')
  const name = String(item[map.nameField] || item.title || item.name || '')
  const refType = map.refType || entityPicker.type.toUpperCase()
  const text = `[[${refType}:${id}|${name}]]`

  // Insert the reference text
  editor.value.chain().focus().insertContent(text).run()

  // Create DB reference if we have a document ID
  if (props.documentId) {
    api.createReference({
      source_type: 'document',
      source_id: props.documentId,
      target_type: entityPicker.type,
      target_id: id,
    }).catch(() => {}) // silent fail
  }

  entityPicker.show = false
}

function onEntityPickerKeydown(e) {
  const items = filteredEntities.value
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    entityPicker.selectedIdx = (entityPicker.selectedIdx + 1) % Math.max(items.length, 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    entityPicker.selectedIdx = (entityPicker.selectedIdx - 1 + Math.max(items.length, 1)) % Math.max(items.length, 1)
  } else if (e.key === 'Enter') {
    e.preventDefault()
    if (items[entityPicker.selectedIdx]) {
      selectEntity(items[entityPicker.selectedIdx])
    }
  } else if (e.key === 'Escape') {
    e.preventDefault()
    entityPicker.show = false
    editor.value?.chain().focus().run()
  }
}

// Track current cell color
const currentCellColor = computed(() => {
  if (!editor.value) return null
  const attrs = editor.value.getAttributes('tableCell')
  return attrs.backgroundColor || null
})

function setCellColor(color) {
  if (!editor.value) return
  editor.value.chain().focus().setCellAttribute('backgroundColor', color || null).run()
}

function insertTable() {
  editor.value?.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()
}

const showLinkInput = ref(false)
const linkUrl = ref('')

function setLink() {
  if (!editor.value) return
  if (editor.value.isActive('link')) {
    editor.value.chain().focus().unsetLink().run()
    return
  }
  linkUrl.value = ''
  showLinkInput.value = true
  nextTick(() => {
    const input = document.querySelector('.link-url-input')
    if (input) input.focus()
  })
}

function confirmLink() {
  if (linkUrl.value.trim() && editor.value) {
    editor.value.chain().focus().extendMarkRange('link').setLink({ href: linkUrl.value.trim() }).run()
  }
  showLinkInput.value = false
  linkUrl.value = ''
}

function cancelLink() {
  showLinkInput.value = false
  linkUrl.value = ''
  if (editor.value) editor.value.commands.focus()
}

// Watch for external modelValue changes
watch(() => props.modelValue, (newVal) => {
  if (!editor.value) return
  const currentMd = htmlToMarkdown(editor.value.getHTML())
  if (newVal !== currentMd) {
    editor.value.commands.setContent(markdownToHtml(newVal), false)
  }
})

watch(() => props.editable, (val) => {
  editor.value?.setEditable(val)
})

onBeforeUnmount(() => {
  editor.value?.destroy()
})
</script>

<style>
/* Toolbar button styles */
.toolbar-btn {
  width: 2rem;
  height: 2rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.25rem;
  color: #94a3b8;
  transition: color 0.15s, background-color 0.15s;
  cursor: pointer;
}
.toolbar-btn:hover {
  color: #e2e8f0;
  background-color: #1e293b;
}
.toolbar-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}
.toolbar-btn-active {
  color: #60a5fa;
  background-color: rgba(59, 130, 246, 0.1);
}

/* Context menu items (right-click) */
.ctx-menu-item {
  display: block;
  width: 100%;
  padding: 0.375rem 0.75rem;
  text-align: left;
  font-size: 0.75rem;
  color: #cbd5e1;
  transition: background-color 0.1s;
  cursor: pointer;
}
.ctx-menu-item:hover {
  background-color: #334155;
}

/* Context menu buttons */
.ctx-btn {
  width: 1.75rem;
  height: 1.75rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.25rem;
  color: #94a3b8;
  transition: all 0.1s;
  cursor: pointer;
}
.ctx-btn:hover {
  color: #e2e8f0;
  background-color: #334155;
}

/* Editor content area */
.editor-content .tiptap {
  min-height: 400px;
  padding: 1.5rem;
  background-color: #020617;
  color: #e2e8f0;
  outline: none;
  border-bottom-left-radius: 0.75rem;
  border-bottom-right-radius: 0.75rem;
}
.editor-content .tiptap:focus {
  box-shadow: 0 0 0 1px rgba(59, 130, 246, 0.3);
}

/* Placeholder */
.editor-content .tiptap p.is-editor-empty:first-child::before {
  color: #475569;
  float: left;
  height: 0;
  pointer-events: none;
  content: attr(data-placeholder);
}

/* Prose styles */
.editor-content .tiptap h1 { font-size: 1.5rem; font-weight: 700; color: #f1f5f9; margin-top: 1.5rem; margin-bottom: 0.75rem; }
.editor-content .tiptap h2 { font-size: 1.25rem; font-weight: 700; color: #f1f5f9; margin-top: 1.25rem; margin-bottom: 0.5rem; }
.editor-content .tiptap h3 { font-size: 1.125rem; font-weight: 600; color: #e2e8f0; margin-top: 1rem; margin-bottom: 0.5rem; }
.editor-content .tiptap h4 { font-size: 1rem; font-weight: 600; color: #e2e8f0; margin-top: 0.75rem; margin-bottom: 0.25rem; }
.editor-content .tiptap p { margin-top: 0.5rem; margin-bottom: 0.5rem; line-height: 1.625; }
.editor-content .tiptap ul { list-style-type: disc; padding-left: 1.5rem; margin-top: 0.5rem; margin-bottom: 0.5rem; }
.editor-content .tiptap ol { list-style-type: decimal; padding-left: 1.5rem; margin-top: 0.5rem; margin-bottom: 0.5rem; }
.editor-content .tiptap li { margin-top: 0.25rem; margin-bottom: 0.25rem; }
.editor-content .tiptap li p { margin: 0; }
.editor-content .tiptap blockquote {
  border-left: 4px solid #475569;
  padding-left: 1rem;
  margin-top: 0.75rem;
  margin-bottom: 0.75rem;
  color: #94a3b8;
  font-style: italic;
}
.editor-content .tiptap pre {
  background-color: #0f172a;
  border-radius: 0.5rem;
  padding: 1rem;
  padding-top: 2.25rem; /* room for the code-block language picker (NodeView) */
  margin-top: 0.75rem;
  margin-bottom: 0.75rem;
  overflow-x: auto;
  font-size: 0.875rem;
}
.editor-content .tiptap code {
  background-color: #1e293b;
  padding: 0.125rem 0.375rem;
  border-radius: 0.25rem;
  font-size: 0.875rem;
  color: #93c5fd;
}
.editor-content .tiptap pre code {
  background-color: transparent;
  padding: 0;
  color: #cbd5e1;
}
.editor-content .tiptap hr {
  margin-top: 1.5rem;
  margin-bottom: 1.5rem;
  border-color: #334155;
}
.editor-content .tiptap a { color: #60a5fa; text-decoration: underline; cursor: pointer; }
.editor-content .tiptap a:hover { color: #93c5fd; }
.editor-content .tiptap strong { font-weight: 700; color: #f1f5f9; }
.editor-content .tiptap em { font-style: italic; }

/* Table styles */
.editor-content .tiptap table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 1rem;
  margin-bottom: 1rem;
}
.editor-content .tiptap table td,
.editor-content .tiptap table th {
  border: 1px solid #334155;
  padding: 0.5rem 0.75rem;
  font-size: 0.875rem;
  position: relative;
  color: #cbd5e1;
}
.editor-content .tiptap table th {
  background-color: #1e293b;
  font-weight: 600;
}
.editor-content .tiptap table .selectedCell {
  background-color: rgba(59, 130, 246, 0.15);
  outline: 2px solid rgba(59, 130, 246, 0.4);
  outline-offset: -2px;
}
.editor-content .tiptap table .column-resize-handle {
  position: absolute;
  right: -2px;
  top: 0;
  bottom: 0;
  width: 4px;
  background-color: #3b82f6;
  pointer-events: none;
}
.editor-content .tiptap .tableWrapper {
  overflow-x: auto;
}

/* Color input reset for dark theme */
input[type="color"] {
  -webkit-appearance: none;
  border: none;
  cursor: pointer;
}
input[type="color"]::-webkit-color-swatch-wrapper {
  padding: 0;
}
input[type="color"]::-webkit-color-swatch {
  border: 1px solid #475569;
  border-radius: 2px;
}
</style>
