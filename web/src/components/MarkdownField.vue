<template>
  <div class="relative markdown-field" ref="fieldRoot">
    <!-- Compact formatting toolbar -->
    <div class="flex flex-wrap items-center gap-0.5 px-1.5 py-1 bg-slate-800/30 border border-slate-700 border-b-0 rounded-t">
      <!-- Inline formatting -->
      <button type="button" @click="editor?.chain().focus().toggleBold().run()"
        :class="{ 'md-toolbar-btn-active': editor?.isActive('bold') }"
        class="md-toolbar-btn" title="Bold (Ctrl+B)">
        <span class="font-bold text-[11px]">B</span>
      </button>
      <button type="button" @click="editor?.chain().focus().toggleItalic().run()"
        :class="{ 'md-toolbar-btn-active': editor?.isActive('italic') }"
        class="md-toolbar-btn" title="Italic (Ctrl+I)">
        <span class="italic text-[11px]">I</span>
      </button>
      <button type="button" @click="editor?.chain().focus().toggleStrike().run()"
        :class="{ 'md-toolbar-btn-active': editor?.isActive('strike') }"
        class="md-toolbar-btn" title="Strikethrough">
        <span class="line-through text-[11px]">S</span>
      </button>

      <div class="w-px h-4 bg-slate-700 mx-0.5" />

      <!-- Headings -->
      <button v-for="level in [1,2,3]" :key="level" type="button"
        @click="editor?.chain().focus().toggleHeading({ level }).run()"
        :class="{ 'md-toolbar-btn-active': editor?.isActive('heading', { level }) }"
        class="md-toolbar-btn" :title="'Heading ' + level">
        <span class="text-[10px] font-bold">H{{ level }}</span>
      </button>

      <div class="w-px h-4 bg-slate-700 mx-0.5" />

      <!-- Lists / blocks -->
      <button type="button" @click="editor?.chain().focus().toggleBulletList().run()"
        :class="{ 'md-toolbar-btn-active': editor?.isActive('bulletList') }"
        class="md-toolbar-btn" title="Bullet list">
        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
        </svg>
      </button>
      <button type="button" @click="editor?.chain().focus().toggleOrderedList().run()"
        :class="{ 'md-toolbar-btn-active': editor?.isActive('orderedList') }"
        class="md-toolbar-btn" title="Numbered list">
        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M8 6h13M8 12h13M8 18h13M3.5 6l1-1v4M3 18h3m-3-2l2.286-2.286" />
        </svg>
      </button>
      <button type="button" @click="editor?.chain().focus().toggleBlockquote().run()"
        :class="{ 'md-toolbar-btn-active': editor?.isActive('blockquote') }"
        class="md-toolbar-btn" title="Blockquote">
        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 8.25h9m-9 3H12m-9.75 1.51c0 1.6 1.123 2.994 2.707 3.227 1.087.16 2.185.283 3.293.369V21l4.076-4.076" />
        </svg>
      </button>
      <button type="button" @click="editor?.chain().focus().toggleCodeBlock().run()"
        :class="{ 'md-toolbar-btn-active': editor?.isActive('codeBlock') }"
        class="md-toolbar-btn" title="Code block">
        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M17.25 6.75L22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25" />
        </svg>
      </button>
      <button type="button" @click="editor?.chain().focus().setHorizontalRule().run()"
        class="md-toolbar-btn" title="Divider">
        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M3 12h18" />
        </svg>
      </button>

      <div class="w-px h-4 bg-slate-700 mx-0.5" />

      <!-- Link -->
      <button type="button" @click="onLinkClick"
        :class="{ 'md-toolbar-btn-active': editor?.isActive('link') }"
        class="md-toolbar-btn" title="Link (Ctrl+K)">
        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.193-9.193a4.5 4.5 0 016.364 6.364l-4.5 4.5a4.5 4.5 0 01-7.244-1.242" />
        </svg>
      </button>

      <div class="w-px h-4 bg-slate-700 mx-0.5" />

      <!-- Entity link picker (uses existing slash picker) -->
      <button type="button" @click="toggleEntityMenu" class="md-toolbar-btn flex items-center gap-0.5 !w-auto px-1.5" title="Link entity">
        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
        </svg>
        <svg class="w-2.5 h-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      <!-- Inline link URL prompt -->
      <div v-if="showLinkInput" class="flex items-center gap-1 ml-1">
        <input
          ref="linkInputRef"
          v-model="linkUrl"
          @keydown.enter.prevent="confirmLink"
          @keydown.escape.prevent="cancelLink"
          class="link-url-input w-40 px-1.5 py-0.5 bg-slate-900 border border-slate-600 rounded text-[11px] text-white placeholder-slate-500 focus:outline-none focus:border-blue-500"
          placeholder="https://..." />
        <button type="button" @mousedown.prevent="confirmLink" class="px-1.5 py-0.5 text-[10px] bg-blue-600 hover:bg-blue-500 text-white rounded">OK</button>
        <button type="button" @mousedown.prevent="cancelLink" class="px-1 py-0.5 text-[10px] text-slate-500 hover:text-white">&#x2715;</button>
      </div>

      <!-- Entity type menu (mini popover) -->
      <div v-if="showEntityMenu" class="absolute z-40 left-1 top-7 bg-slate-800 border border-slate-700 rounded shadow-xl py-0.5">
        <button v-for="cmd in entityCommands" :key="cmd.id"
          type="button"
          @mousedown.prevent="onEntitySelect(cmd)"
          class="w-full px-3 py-1 text-left text-[11px] text-slate-300 hover:bg-slate-700/60 hover:text-blue-300 flex items-center gap-2">
          <span class="font-mono text-slate-500 text-[9px]">/{{ cmd.shorthand }}</span>
          <span>{{ cmd.label }}</span>
        </button>
      </div>
    </div>

    <!-- Editor content -->
    <div class="md-editor-wrap" :style="{ '--md-min-height': minHeight }">
      <editor-content :editor="editor" class="md-editor-content" />
    </div>

    <!-- Slash command menu -->
    <div v-if="slashMenu.show"
      class="fixed z-[100] bg-slate-800 border border-slate-700 rounded-lg shadow-2xl py-1 min-w-[260px] max-h-[280px] overflow-y-auto"
      :style="{ top: slashMenu.y + 'px', left: slashMenu.x + 'px' }">
      <div class="px-3 py-1.5 text-[9px] text-slate-500 uppercase tracking-wider">Insert block</div>
      <button v-for="(cmd, idx) in filteredSlashCommands" :key="cmd.id"
        @mousedown.prevent="executeSlashCommand(cmd)"
        @mouseenter="slashSelectedIdx = idx"
        class="w-full px-3 py-2 text-left flex items-center gap-3 transition-colors"
        :class="slashSelectedIdx === idx ? 'bg-blue-600/20 text-blue-300' : 'text-slate-300 hover:bg-slate-700/50'">
        <div class="w-7 h-7 rounded-lg flex items-center justify-center flex-shrink-0"
          :class="slashSelectedIdx === idx ? 'bg-blue-600/20' : 'bg-slate-700/50'">
          <span v-if="cmd.icon === 'h1'" class="text-[10px] font-bold">H1</span>
          <span v-else-if="cmd.icon === 'h2'" class="text-[10px] font-bold">H2</span>
          <span v-else-if="cmd.icon === 'h3'" class="text-[10px] font-bold">H3</span>
          <svg v-else-if="cmd.icon === 'list'" class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
          </svg>
          <svg v-else-if="cmd.icon === 'list-ol'" class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M8 6h13M8 12h13M8 18h13M3.5 6l1-1v4" />
          </svg>
          <svg v-else-if="cmd.icon === 'quote'" class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 8.25h9m-9 3H12m-9.75 1.51c0 1.6 1.123 2.994 2.707 3.227 1.087.16 2.185.283 3.293.369V21l4.076-4.076" />
          </svg>
          <svg v-else-if="cmd.icon === 'code'" class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M17.25 6.75L22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25" />
          </svg>
          <svg v-else-if="cmd.icon === 'hr'" class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M3 12h18" />
          </svg>
          <svg v-else-if="cmd.icon === 'link'" class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.193-9.193a4.5 4.5 0 016.364 6.364l-4.5 4.5a4.5 4.5 0 01-7.244-1.242" />
          </svg>
        </div>
        <div class="min-w-0 flex-1">
          <div class="text-xs"><span class="font-semibold">{{ cmd.label }}</span> <span v-if="cmd.shorthand" class="text-[10px] text-slate-500 font-mono ml-1">/{{ cmd.shorthand }}</span></div>
          <div class="text-[10px] text-slate-500 truncate">{{ cmd.desc }}</div>
        </div>
      </button>
      <div v-if="filteredSlashCommands.length === 0" class="px-3 py-3 text-xs text-slate-600 text-center">No matching commands</div>
    </div>
    <div v-if="slashMenu.show" class="fixed inset-0 z-30" @mousedown="slashMenu.show = false" />

    <!-- Entity picker (after a /risk, /asset, etc. command was selected) -->
    <div v-if="entityPicker.show"
      class="fixed z-[100] bg-slate-800 border border-slate-700 rounded-lg shadow-2xl max-h-[320px] overflow-hidden flex flex-col min-w-[320px]"
      :style="{ top: entityPicker.y + 'px', left: entityPicker.x + 'px' }">
      <div class="px-3 py-1.5 text-[9px] text-slate-500 uppercase tracking-wider flex items-center gap-2">
        <span>Link {{ entityPicker.label }}</span>
        <button @mousedown.prevent="closePicker" class="ml-auto text-slate-600 hover:text-slate-400">
          <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
        </button>
      </div>
      <div class="px-2 pb-1">
        <input
          ref="pickerInput"
          v-model="entityPicker.search"
          @keydown.stop="onPickerKeydown"
          class="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-xs text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500"
          placeholder="Search..."
        />
      </div>
      <div class="overflow-y-auto flex-1">
        <div v-if="entityPicker.loading" class="px-3 py-3 text-xs text-slate-500 text-center">Loading...</div>
        <button v-else v-for="(item, idx) in filteredPickerItems" :key="resolveItem(item).id"
          @mousedown.prevent="selectPickerItem(item)"
          @mouseenter="entityPicker.selectedIdx = idx"
          class="w-full px-3 py-2 text-left flex items-center gap-2 transition-colors"
          :class="entityPicker.selectedIdx === idx ? 'bg-blue-600/20 text-blue-300' : 'text-slate-300 hover:bg-slate-700/50'">
          <span class="text-[10px] font-mono text-slate-500 flex-shrink-0">{{ resolveItem(item).id }}</span>
          <span class="text-xs truncate">{{ resolveItem(item).name }}</span>
        </button>
        <div v-if="!entityPicker.loading && filteredPickerItems.length === 0" class="px-3 py-3 text-xs text-slate-600 text-center">No results</div>
      </div>
    </div>
    <div v-if="entityPicker.show" class="fixed inset-0 z-30" @mousedown="closePicker" />
  </div>
</template>

<script setup>
import { ref, reactive, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { useEditor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import Placeholder from '@tiptap/extension-placeholder'
import { slashCommands as sharedSlashCommands, fetchPickerItems, resolveEntity } from '../composables/useSlashCommands.js'
import { markdownToHtml, htmlToMarkdown } from '../composables/useMarkdownConvert.js'

const props = defineProps({
  modelValue: { type: String, default: '' },
  rows: { type: Number, default: 3 },
  placeholder: { type: String, default: 'Type / for commands...' },
  // selfType + selfId let the entity picker filter out the entity itself —
  // a Risk page's /risk picker shouldn't list the risk you're editing.
  selfType: { type: String, default: '' },
  selfId: { type: String, default: '' },
})

const emit = defineEmits(['update:modelValue', 'blur'])

const fieldRoot = ref(null)
const linkInputRef = ref(null)
const pickerInput = ref(null)

// Min-height scales with `rows` prop. Roughly tracks textarea sizing — each row
// is ~1.5em of body text plus a little padding on the wrapper.
const minHeight = computed(() => `${props.rows * 1.5}em`)

// Map a shared slash-command id to a tiptap action. Reuses the same id-switch
// pattern as DocumentEditor so both editors stay in sync.
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
        return () => openPicker(cmd.picker)
      }
      return () => {}
  }
}

// Slash commands available in the inline editor — same shared list as DocumentEditor,
// minus the table command (no table extension is loaded for the compact variant).
const slashCommandsList = sharedSlashCommands.map(cmd => ({ ...cmd, action: tiptapAction(cmd) }))

const entityCommands = computed(() => sharedSlashCommands.filter(c => c.picker))

// --- Editor instance ---
const editor = useEditor({
  content: markdownToHtml(props.modelValue),
  extensions: [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
    }),
    Link.configure({ openOnClick: false }),
    Placeholder.configure({ placeholder: props.placeholder }),
  ],
  editorProps: {
    handleKeyDown: (view, event) => {
      // Slash menu navigation takes priority when open.
      if (slashMenu.show) {
        if (['ArrowDown', 'ArrowUp', 'Enter', 'Escape', 'Tab'].includes(event.key)) {
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
  onBlur: () => {
    // Defer so picker/link/menu mousedown handlers fire first
    setTimeout(() => {
      if (slashMenu.show) return
      if (entityPicker.show) return
      if (showLinkInput.value) return
      emit('blur')
    }, 200)
  },
})

// --- Slash menu state ---
const slashMenu = reactive({ show: false, x: 0, y: 0, query: '', startPos: 0 })
const slashSelectedIdx = ref(0)

const filteredSlashCommands = computed(() => {
  const q = slashMenu.query.toLowerCase()
  if (!q) return slashCommandsList
  return slashCommandsList.filter(c =>
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

  const slashIdx = textBefore.lastIndexOf('/')
  if (slashIdx >= 0 && (slashIdx === 0 || textBefore[slashIdx - 1] === ' ' || textBefore[slashIdx - 1] === '\n')) {
    const query = textBefore.slice(slashIdx + 1)
    if (query.includes(' ') || query.includes('/') || query.length > 15) {
      slashMenu.show = false
      return
    }

    slashMenu.query = query
    slashMenu.startPos = $from.pos - query.length - 1

    const coords = ed.view.coordsAtPos($from.pos)
    // Place menu just below cursor; flip above when there's no room below.
    const menuHeight = 280
    const vh = window.innerHeight
    if (coords.bottom + menuHeight > vh - 8) {
      slashMenu.y = Math.max(8, coords.top - menuHeight - 8)
    } else {
      slashMenu.y = coords.bottom + 4
    }
    slashMenu.x = coords.left

    slashMenu.show = filteredSlashCommands.value.length > 0
    slashSelectedIdx.value = 0
  } else {
    slashMenu.show = false
  }
}

function executeSlashCommand(cmd) {
  if (!editor.value) return
  const { state } = editor.value
  const { $from } = state.selection
  // Delete the "/query" trigger text first, then run the action.
  editor.value.chain().focus()
    .command(({ tr }) => {
      tr.delete(slashMenu.startPos, $from.pos)
      return true
    })
    .run()
  cmd.action(editor.value)
  slashMenu.show = false
}

function onSlashKeydown(e) {
  if (!slashMenu.show) return
  const cmds = filteredSlashCommands.value
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    slashSelectedIdx.value = (slashSelectedIdx.value + 1) % Math.max(cmds.length, 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    slashSelectedIdx.value = (slashSelectedIdx.value - 1 + Math.max(cmds.length, 1)) % Math.max(cmds.length, 1)
  } else if (e.key === 'Enter' || e.key === 'Tab') {
    e.preventDefault()
    if (cmds[slashSelectedIdx.value]) {
      executeSlashCommand(cmds[slashSelectedIdx.value])
    }
  } else if (e.key === 'Escape') {
    e.preventDefault()
    slashMenu.show = false
  }
}

// Re-clamp the selected index whenever the filtered list shrinks.
watch(filteredSlashCommands, (cmds) => {
  if (slashSelectedIdx.value >= cmds.length) slashSelectedIdx.value = 0
})

// --- Entity picker state ---
const entityPicker = reactive({
  show: false, x: 0, y: 0, search: '', selectedIdx: 0, loading: false,
  items: [], type: '', label: '', linkPath: '', triggerStartPos: -1,
})

const filteredPickerItems = computed(() => {
  const q = entityPicker.search.toLowerCase()
  if (!q) return entityPicker.items.slice(0, 50)
  return entityPicker.items.filter(item => {
    const { id, name } = resolveItem(item)
    return id.toLowerCase().includes(q) || name.toLowerCase().includes(q)
  }).slice(0, 50)
})

function resolveItem(item) {
  return resolveEntity(entityPicker.type, item)
}

async function openPicker(picker) {
  if (!editor.value) return
  // Position picker at current cursor.
  const { state } = editor.value
  const coords = editor.value.view.coordsAtPos(state.selection.$from.pos)
  entityPicker.x = coords.left
  const menuHeight = 320
  const vh = window.innerHeight
  if (coords.bottom + menuHeight > vh - 8) {
    entityPicker.y = Math.max(8, coords.top - menuHeight - 8)
  } else {
    entityPicker.y = coords.bottom + 4
  }
  entityPicker.type = picker.type
  entityPicker.label = picker.label
  entityPicker.linkPath = picker.linkPath
  entityPicker.search = ''
  entityPicker.selectedIdx = 0
  entityPicker.loading = true
  entityPicker.items = []
  entityPicker.show = true
  await nextTick()
  pickerInput.value?.focus()
  let items = await fetchPickerItems(picker)
  // Hide the entity itself — you can't link a Risk to itself.
  if (props.selfType && props.selfId && picker.type === props.selfType) {
    items = items.filter(it => {
      const { id } = resolveEntity(picker.type, it)
      return String(id) !== String(props.selfId)
    })
  }
  entityPicker.items = items
  entityPicker.loading = false
}

function closePicker() {
  entityPicker.show = false
  nextTick(() => editor.value?.commands.focus())
}

function selectPickerItem(item) {
  if (!editor.value) return
  const { id, name } = resolveItem(item)
  // Insert as a real link mark so it renders correctly in the WYSIWYG view
  // and round-trips back to `[name](path)` markdown via turndown.
  const href = `${entityPicker.linkPath}${id}`
  editor.value.chain().focus()
    .insertContent({ type: 'text', text: name, marks: [{ type: 'link', attrs: { href } }] })
    .run()
  entityPicker.show = false
}

function onPickerKeydown(e) {
  if (e.key === 'Escape') {
    e.preventDefault()
    closePicker()
    return
  }
  const items = filteredPickerItems.value
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    entityPicker.selectedIdx = Math.min(entityPicker.selectedIdx + 1, items.length - 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    entityPicker.selectedIdx = Math.max(entityPicker.selectedIdx - 1, 0)
  } else if (e.key === 'Enter' || e.key === 'Tab') {
    if (items.length > 0) {
      e.preventDefault()
      selectPickerItem(items[entityPicker.selectedIdx])
    }
  }
}

// --- Toolbar link prompt ---
const showLinkInput = ref(false)
const linkUrl = ref('')

function onLinkClick() {
  if (!editor.value) return
  if (editor.value.isActive('link')) {
    editor.value.chain().focus().unsetLink().run()
    return
  }
  linkUrl.value = ''
  showLinkInput.value = true
  nextTick(() => linkInputRef.value?.focus())
}

function confirmLink() {
  const url = linkUrl.value.trim()
  if (url && editor.value) {
    editor.value.chain().focus().extendMarkRange('link').setLink({ href: url }).run()
  }
  showLinkInput.value = false
  linkUrl.value = ''
}

function cancelLink() {
  showLinkInput.value = false
  linkUrl.value = ''
  editor.value?.commands.focus()
}

// --- Toolbar entity menu ---
const showEntityMenu = ref(false)

function toggleEntityMenu() {
  showEntityMenu.value = !showEntityMenu.value
}

function onEntitySelect(cmd) {
  showEntityMenu.value = false
  if (cmd.picker) {
    // Make sure the editor has focus so coordsAtPos returns valid viewport coords.
    editor.value?.commands.focus()
    nextTick(() => openPicker(cmd.picker))
  }
}

watch(showEntityMenu, (v) => {
  if (!v) return
  // Close on next outside click.
  const handler = (ev) => {
    if (!ev.target.closest('.markdown-field .md-toolbar-btn')) {
      showEntityMenu.value = false
      document.removeEventListener('mousedown', handler)
    }
  }
  setTimeout(() => document.addEventListener('mousedown', handler), 0)
})

// --- Sync external modelValue changes (only when they actually differ) ---
watch(() => props.modelValue, (newVal) => {
  if (!editor.value) return
  const currentMd = htmlToMarkdown(editor.value.getHTML())
  if ((newVal || '') !== (currentMd || '')) {
    editor.value.commands.setContent(markdownToHtml(newVal || ''), false)
  }
})

// Update placeholder if the prop changes (rare but safe).
watch(() => props.placeholder, (val) => {
  const ext = editor.value?.extensionManager.extensions.find(e => e.name === 'placeholder')
  if (ext) {
    ext.options.placeholder = val
    editor.value?.view.dispatch(editor.value.state.tr)
  }
})

onBeforeUnmount(() => {
  editor.value?.destroy()
})
</script>

<style scoped>
.md-toolbar-btn {
  width: 1.5rem;
  height: 1.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.2rem;
  color: #94a3b8;
  transition: color 0.15s, background-color 0.15s;
  cursor: pointer;
}
.md-toolbar-btn:hover {
  color: #e2e8f0;
  background-color: rgba(30, 41, 59, 0.8);
}
.md-toolbar-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}
.md-toolbar-btn-active {
  color: #60a5fa;
  background-color: rgba(59, 130, 246, 0.15);
}

.md-editor-wrap {
  background-color: #1e293b;
  border: 1px solid #334155;
  border-top: 0;
  border-bottom-left-radius: 0.25rem;
  border-bottom-right-radius: 0.25rem;
}
</style>

<style>
/* Editor content area — must be unscoped so tiptap-generated DOM is styled. */
.markdown-field .md-editor-content .tiptap {
  min-height: var(--md-min-height, 4.5em);
  padding: 0.375rem 0.5rem;
  color: #e2e8f0;
  font-size: 0.75rem;
  line-height: 1.5;
  outline: none;
}
.markdown-field .md-editor-content .tiptap:focus {
  box-shadow: 0 0 0 1px rgba(59, 130, 246, 0.4);
}

/* Placeholder */
.markdown-field .md-editor-content .tiptap p.is-editor-empty:first-child::before {
  color: #64748b;
  float: left;
  height: 0;
  pointer-events: none;
  content: attr(data-placeholder);
}

/* Compact prose styles — tighter than DocumentEditor since this is inline. */
.markdown-field .md-editor-content .tiptap h1 { font-size: 1rem;     font-weight: 700; color: #f1f5f9; margin: 0.5rem 0 0.25rem; }
.markdown-field .md-editor-content .tiptap h2 { font-size: 0.9375rem; font-weight: 700; color: #f1f5f9; margin: 0.4rem 0 0.2rem; }
.markdown-field .md-editor-content .tiptap h3 { font-size: 0.875rem;  font-weight: 600; color: #e2e8f0; margin: 0.35rem 0 0.15rem; }
.markdown-field .md-editor-content .tiptap p  { margin: 0.2rem 0; }
.markdown-field .md-editor-content .tiptap ul { list-style-type: disc;    padding-left: 1.25rem; margin: 0.2rem 0; }
.markdown-field .md-editor-content .tiptap ol { list-style-type: decimal; padding-left: 1.25rem; margin: 0.2rem 0; }
.markdown-field .md-editor-content .tiptap li { margin: 0.1rem 0; }
.markdown-field .md-editor-content .tiptap li p { margin: 0; }
.markdown-field .md-editor-content .tiptap blockquote {
  border-left: 3px solid #475569;
  padding-left: 0.625rem;
  margin: 0.3rem 0;
  color: #94a3b8;
  font-style: italic;
}
.markdown-field .md-editor-content .tiptap pre {
  background-color: #0f172a;
  border-radius: 0.25rem;
  padding: 0.5rem 0.625rem;
  margin: 0.3rem 0;
  overflow-x: auto;
  font-size: 0.6875rem;
}
.markdown-field .md-editor-content .tiptap code {
  background-color: #0f172a;
  padding: 0.05rem 0.25rem;
  border-radius: 0.2rem;
  font-size: 0.6875rem;
  color: #93c5fd;
}
.markdown-field .md-editor-content .tiptap pre code {
  background-color: transparent;
  padding: 0;
  color: #cbd5e1;
}
.markdown-field .md-editor-content .tiptap hr {
  margin: 0.5rem 0;
  border: 0;
  border-top: 1px solid #334155;
}
.markdown-field .md-editor-content .tiptap a { color: #60a5fa; text-decoration: underline; cursor: pointer; }
.markdown-field .md-editor-content .tiptap a:hover { color: #93c5fd; }
.markdown-field .md-editor-content .tiptap strong { font-weight: 700; color: #f1f5f9; }
.markdown-field .md-editor-content .tiptap em { font-style: italic; }
</style>
