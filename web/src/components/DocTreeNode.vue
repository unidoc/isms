<template>
  <template v-for="node in nodes" :key="node.path || node.name">
    <!-- Folder -->
    <div v-if="node.type === 'folder'" class="relative group/folder">
      <button
        @click="$emit('toggle', node.path)"
        @contextmenu.prevent="editable && $emit('folder-menu', $event, node.path)"
        class="w-full flex items-center gap-1.5 py-1.5 pr-8 text-left text-[13px] transition-colors cursor-pointer hover:bg-slate-800/50"
        :style="{ paddingLeft: (node.depth * 18 + 8) + 'px' }"
      >
        <svg
          class="w-3 h-3 text-slate-600 transition-transform duration-150 flex-shrink-0"
          :class="{ 'rotate-90': expanded.has(node.path) }"
          fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
        >
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <svg v-if="expanded.has(node.path)" class="w-3.5 h-3.5 text-blue-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 9.776c.112-.017.227-.026.344-.026h15.812c.117 0 .232.009.344.026m-16.5 0a2.25 2.25 0 00-1.883 2.542l.857 6a2.25 2.25 0 002.227 1.932H19.05a2.25 2.25 0 002.227-1.932l.857-6a2.25 2.25 0 00-1.883-2.542m-16.5 0V6A2.25 2.25 0 016 3.75h3.879a1.5 1.5 0 011.06.44l2.122 2.121a1.5 1.5 0 001.06.44H18A2.25 2.25 0 0120.25 9v.776" />
        </svg>
        <svg v-else class="w-3.5 h-3.5 text-slate-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z" />
        </svg>
        <span class="truncate font-medium" :class="node.depth === 0 ? 'text-slate-300' : 'text-slate-400'">{{ formatName(node) }}</span>
        <span class="ml-auto text-[10px] text-slate-600 tabular-nums flex-shrink-0">{{ node.fileCount }}</span>
      </button>
      <!-- Add folder button on hover -->
      <div v-if="editable && expanded.has(node.path)" class="absolute right-2 top-1/2 -translate-y-1/2 opacity-0 group-hover/folder:opacity-100 transition-opacity">
        <button @click.stop="$emit('folder-menu', $event, node.path)" class="p-0.5 rounded hover:bg-slate-700 text-slate-600 hover:text-slate-300">
          <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
          </svg>
        </button>
      </div>
      <!-- Recurse into children -->
      <template v-if="expanded.has(node.path)">
        <DocTreeNode
          :nodes="node.children"
          :expanded="expanded"
          :activeId="activeId"
          :editable="editable"
          :formatName="formatName"
          :formatFileTitle="formatFileTitle"
          :formatFileName="formatFileName"
          :needsReview="needsReview"
          @toggle="$emit('toggle', $event)"
          @select="$emit('select', $event)"
          @folder-menu="(e, p) => $emit('folder-menu', e, p)"
        />
        <!-- Inline new folder slot -->
        <slot name="new-folder" :parentPath="node.path" :depth="node.depth" />
      </template>
    </div>
    <!-- File -->
    <button
      v-else
      @click="$emit('select', { folder: node.folder, docId: node.file.document_id, file: node.file })"
      class="w-full flex items-center gap-1.5 py-1.5 pr-3 text-left text-[13px] transition-colors cursor-pointer group/file"
      :class="activeId === node.file.document_id
        ? 'bg-blue-950/60 text-blue-300'
        : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/40'"
      :style="{ paddingLeft: (node.depth * 18 + 8) + 'px' }"
    >
      <svg class="w-3.5 h-3.5 flex-shrink-0" :class="activeId === node.file.document_id ? 'text-blue-400' : 'text-slate-600'" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
        <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m2.25 0H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />
      </svg>
      <span class="truncate flex-1 leading-tight">
        <span class="block text-[13px]">{{ formatFileTitle(node.file) }}</span>
        <span v-if="node.file.title" class="block text-[10px] text-slate-600 font-mono">{{ formatFileName(node.file) }}</span>
      </span>
      <span v-if="needsReview(node.file)" class="w-1.5 h-1.5 rounded-full bg-amber-500 flex-shrink-0" title="Review required" />
    </button>
  </template>
</template>

<script setup>
defineProps({
  nodes: { type: Array, required: true },
  expanded: { type: Set, required: true },
  activeId: { type: String, default: '' },
  editable: { type: Boolean, default: false },
  formatName: { type: Function, required: true },
  formatFileTitle: { type: Function, required: true },
  formatFileName: { type: Function, required: true },
  needsReview: { type: Function, required: true },
})

defineEmits(['toggle', 'select', 'folder-menu'])
</script>
