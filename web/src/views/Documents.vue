<template>
  <div class="flex h-[calc(100vh-57px)] overflow-hidden">
    <!-- Left panel: Document tree (collapsible, hidden on mobile when doc selected) -->
    <aside v-show="showTreePanel" class="flex-shrink-0 bg-slate-900 border-r border-slate-800 flex flex-col overflow-hidden w-full sm:w-[280px]" :class="{ 'hidden sm:flex': activeDoc }">
      <!-- Header (clickable → back to dashboard) -->
      <button @click="activeDoc = null; activeId = null; router.push(orgPath('/documents'))"
        class="w-full px-3 pt-3 pb-2 flex-shrink-0 text-left hover:bg-slate-800/30 transition-colors">
        <div class="flex items-center gap-2 px-1 w-full">
          <svg class="w-4 h-4 text-slate-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 9.776c.112-.017.227-.026.344-.026h15.812c.117 0 .232.009.344.026m-16.5 0a2.25 2.25 0 00-1.883 2.542l.857 6a2.25 2.25 0 002.227 1.932H19.05a2.25 2.25 0 002.227-1.932l.857-6a2.25 2.25 0 00-1.883-2.542m-16.5 0V6A2.25 2.25 0 016 3.75h3.879a1.5 1.5 0 011.06.44l2.122 2.121a1.5 1.5 0 001.06.44H18A2.25 2.25 0 0120.25 9v.776" />
          </svg>
          <span class="text-xs font-semibold text-slate-400 uppercase tracking-wider flex-1">Documents</span>
          <div v-if="canEditMetadata" class="relative">
            <button @click.stop="showNewMenu = !showNewMenu" class="p-1 rounded hover:bg-slate-700 text-slate-500 hover:text-slate-300 transition-colors" title="New">
              <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
              </svg>
            </button>
            <div v-if="showNewMenu" class="absolute right-0 top-7 w-40 bg-slate-800 border border-slate-700 rounded-lg shadow-xl py-1 z-50">
              <button @click="startRootNewFolder(); showNewMenu = false"
                class="w-full flex items-center gap-2 px-3 py-2 text-xs text-slate-300 hover:bg-slate-700 transition-colors text-left">
                <svg class="w-3.5 h-3.5 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 10.5v6m3-3H9m4.06-7.19l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z" />
                </svg>
                New Folder
              </button>
              <button @click="openNewDocModal(); showNewMenu = false"
                :disabled="folders.length === 0"
                class="w-full flex items-center gap-2 px-3 py-2 text-xs text-slate-300 hover:bg-slate-700 transition-colors text-left disabled:opacity-40 disabled:cursor-not-allowed">
                <svg class="w-3.5 h-3.5 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />
                </svg>
                New Document
              </button>
            </div>
          </div>
        </div>
      </button>

      <!-- Loading skeleton -->
      <div v-if="loadingTree" class="px-4 pt-2 space-y-3 overflow-hidden">
        <div v-for="i in 6" :key="i" class="h-4 bg-slate-800 rounded animate-pulse" />
      </div>

      <!-- Empty state: sidebar -->
      <div v-else-if="folders.length === 0" class="flex-1 overflow-y-auto px-4 py-6">
        <div class="text-center">
          <div class="text-xs text-slate-500 mb-4">No documents yet</div>
          <div class="space-y-2">
            <button @click="showTemplatePicker = true" class="w-full text-left flex items-center gap-2.5 px-3 py-2.5 bg-blue-600/10 hover:bg-blue-600/20 border border-blue-500/20 rounded-lg text-xs text-blue-400 transition-colors">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />
              </svg>
              Import template
            </button>
          </div>
        </div>
      </div>

      <!-- Root-level new folder input (always accessible, above tree or onboarding) -->
      <div v-if="rootNewFolder.active" class="flex items-center gap-1.5 py-1 px-3">
          <svg class="w-3.5 h-3.5 text-blue-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 10.5v6m3-3H9m4.06-7.19l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z" />
          </svg>
          <input v-model="rootNewFolder.name"
            @keydown.enter="confirmRootNewFolder"
            @keydown.escape="rootNewFolder.active = false"
            class="root-folder-input flex-1 bg-slate-800 border border-blue-500/50 rounded px-2 py-1 text-xs text-white focus:outline-none"
            placeholder="new folder name..." />
      </div>

      <!-- Recursive folder tree -->
      <nav v-if="folders.length > 0" class="flex-1 overflow-y-auto pb-4 pt-1">
        <DocTreeNode
          :nodes="fileTree"
          :expanded="expandedNodes"
          :activeId="activeId"
          :editable="canEditMetadata"
          :formatName="formatFolderName"
          :formatFileTitle="formatFileTitle"
          :formatFileName="formatFileName"
          :needsReview="fileNeedsReview"
          @toggle="toggleTreeNode"
          @select="({ folder, docId, file }) => selectItem(folder, docId, file)"
          @folder-menu="openFolderMenu"
        >
          <template #new-folder="{ parentPath, depth }">
            <div v-if="inlineNewFolder.active && inlineNewFolder.parentPath === parentPath"
              class="flex items-center gap-1.5 py-1 pr-3" :style="{ paddingLeft: ((depth + 1) * 18 + 8) + 'px' }">
              <svg class="w-3.5 h-3.5 text-blue-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 10.5v6m3-3H9m4.06-7.19l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z" />
              </svg>
              <input v-model="inlineNewFolder.name"
                @keydown.enter="confirmInlineNewFolder"
                @keydown.escape="cancelInlineNewFolder"
                @blur="cancelInlineNewFolder"
                class="inline-folder-input flex-1 bg-slate-800 border border-blue-500/50 rounded px-2 py-1 text-xs text-white focus:outline-none"
                placeholder="folder name..." />
            </div>
          </template>
        </DocTreeNode>
      </nav>

      <!-- Folder context menu -->
      <Teleport to="body">
        <Transition name="modal">
        <div v-if="folderMenu.show" class="fixed z-[60]" :style="{ left: folderMenu.x + 'px', top: folderMenu.y + 'px' }">
          <div class="bg-slate-800 border border-slate-700 rounded-lg shadow-xl py-1 min-w-[160px]">
            <button @click="newDocInFolder(folderMenu.path)" class="w-full flex items-center gap-2 px-3 py-2 text-sm text-slate-300 hover:bg-slate-700 transition-colors text-left">
              <svg class="w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m3.75 9v6m3-3H9m1.5-12H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />
              </svg>
              New Document
            </button>
            <button @click="startInlineNewFolder(folderMenu.path)" class="w-full flex items-center gap-2 px-3 py-2 text-sm text-slate-300 hover:bg-slate-700 transition-colors text-left">
              <svg class="w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 10.5v6m3-3H9m4.06-7.19l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z" />
              </svg>
              New Folder
            </button>
          </div>
        </div>
        </Transition>
        <Transition name="modal">
        <div v-if="folderMenu.show" @click="folderMenu.show = false" class="fixed inset-0 z-[55]" />
        </Transition>
      </Teleport>
    </aside>

    <!-- Left panel toggle -->
    <button @click="showTreePanel = !showTreePanel"
      class="w-5 flex-shrink-0 bg-slate-900/50 border-r border-slate-800 flex items-center justify-center hover:bg-slate-800 transition-colors group"
      :title="showTreePanel ? 'Hide tree' : 'Show tree'">
      <svg class="w-3 h-3 text-slate-600 group-hover:text-slate-400 transition-transform" :class="{ 'rotate-180': !showTreePanel }" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" />
      </svg>
    </button>

    <!-- Center panel: Document content -->
    <main class="flex-1 overflow-y-auto bg-slate-950">
      <!-- No document selected -->
      <div v-if="!activeDoc" class="p-8">
        <div class="max-w-4xl mx-auto">
          <!-- Empty repo hero -->
          <div v-if="folders.length === 0 && !loadingTree" class="flex flex-col items-center justify-center py-20 text-center">
            <svg class="w-16 h-16 text-slate-700 mb-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="0.75">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25" />
            </svg>
            <h2 class="text-xl font-semibold text-slate-200 mb-2">No documents yet</h2>
            <p class="text-sm text-slate-500 mb-8 max-w-md">Import a template to scaffold your document structure, then use the tree menu to add documents and folders.</p>
            <div class="flex gap-3">
              <button @click="showTemplatePicker = true"
                class="flex items-center gap-2 px-5 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />
                </svg>
                Import template
              </button>
            </div>
          </div>

          <!-- Tabs (only when documents exist) -->
          <template v-if="folders.length > 0">
          <div class="flex items-center gap-1 mb-6 border-b border-slate-800">
            <button @click="docDashTab = 'review'" class="px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px"
              :class="docDashTab === 'review' ? 'border-blue-500 text-blue-400' : 'border-transparent text-slate-500 hover:text-slate-300'">
              Needs Review
              <span v-if="needsReviewChangedCount > 0" class="ml-1.5 px-1.5 py-0.5 rounded-full text-[10px] font-bold bg-amber-500/20 text-amber-300 tabular-nums">{{ needsReviewChangedCount }}</span>
            </button>
            <button @click="docDashTab = 'recent'" class="px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px"
              :class="docDashTab === 'recent' ? 'border-blue-500 text-blue-400' : 'border-transparent text-slate-500 hover:text-slate-300'">
              Recently Changed
              <span v-if="changedDocs.length > 0" class="ml-1.5 px-1.5 py-0.5 rounded-full text-[10px] font-bold bg-slate-700 text-slate-400 tabular-nums">{{ changedDocs.length }}</span>
            </button>
            <button @click="docDashTab = 'search'" class="px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px"
              :class="docDashTab === 'search' ? 'border-blue-500 text-blue-400' : 'border-transparent text-slate-500 hover:text-slate-300'">
              Search
            </button>
            <button v-if="canEditMetadata" @click="docDashTab = 'templates'" class="px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px"
              :class="docDashTab === 'templates' ? 'border-blue-500 text-blue-400' : 'border-transparent text-slate-500 hover:text-slate-300'">
              Templates
            </button>
          </div>

          <!-- TAB: Needs Review -->
          <div v-if="docDashTab === 'review'">
            <div class="flex items-center justify-between mb-4">
              <label class="flex items-center gap-2 text-xs text-slate-500 cursor-pointer select-none">
                <input v-model="includeNeverApproved" type="checkbox" class="rounded border-slate-600 bg-slate-800 text-blue-500 focus:ring-blue-500 focus:ring-offset-0 w-3.5 h-3.5" />
                Include never approved ({{ allNeedsReviewDocs.filter(d => d.never_approved).length }})
              </label>
            </div>
            <div v-if="loadingNeedsReview" class="space-y-2">
              <div v-for="i in 4" :key="i" class="h-16 bg-slate-900 rounded-lg animate-pulse" />
            </div>
            <div v-else-if="needsReviewDocs.length === 0" class="text-center py-12">
              <svg class="w-10 h-10 text-emerald-500/30 mx-auto mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <div class="text-sm text-slate-500">All documents are up to date.</div>
            </div>
            <div v-else class="space-y-2">
              <div v-for="doc in needsReviewDocs" :key="doc.document_id"
                class="bg-slate-900 border border-slate-800 rounded-lg p-4 hover:border-slate-700 transition-colors">
                <div class="flex items-start gap-3">
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2 mb-1">
                      <span class="text-sm font-medium text-blue-400">{{ doc.document_id }}</span>
                      <span class="text-[10px] px-1.5 py-0.5 rounded bg-slate-800 text-slate-500">{{ doc.folder }}</span>
                      <span v-if="doc.never_approved" class="text-[10px] px-1.5 py-0.5 rounded font-medium bg-red-500/20 text-red-400">Never approved</span>
                      <span v-else class="text-[10px] px-1.5 py-0.5 rounded font-medium bg-amber-500/20 text-amber-400">Changed</span>
                    </div>
                    <div v-if="doc.title" class="text-xs text-slate-300 mb-2 truncate">{{ doc.title }}</div>
                    <div class="flex items-center gap-4 text-[10px] text-slate-500">
                      <span>Current: <span class="font-mono text-slate-400">{{ doc.current_commit }}</span> ({{ doc.current_commit_time }})</span>
                      <span v-if="doc.approved_commit !== 'never'">Approved: <span class="font-mono text-slate-400">{{ doc.approved_commit }}</span></span>
                    </div>
                    <div v-if="doc.change_summary && doc.change_summary.length > 0" class="mt-2 space-y-0.5">
                      <div v-for="(msg, i) in doc.change_summary.slice(0, 3)" :key="i" class="text-[10px] text-slate-600 truncate">{{ msg }}</div>
                    </div>
                  </div>
                  <div class="flex flex-col gap-1.5 flex-shrink-0">
                    <button @click="openNeedsReviewDoc(doc)"
                      class="inline-flex items-center gap-1 px-2.5 py-1.5 bg-blue-600 hover:bg-blue-500 text-white text-[11px] font-medium rounded-lg transition-colors">
                      <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M6 12L3.269 3.126A59.768 59.768 0 0121.485 12 59.77 59.77 0 013.27 20.876L5.999 12zm0 0h7.5" />
                      </svg>
                      Review
                    </button>
                    <button v-if="!doc.never_approved" @click="viewNeedsReviewDiff(doc)"
                      class="inline-flex items-center gap-1 px-2.5 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-300 text-[11px] font-medium rounded-lg transition-colors border border-slate-700">
                      Diff
                    </button>
                  </div>
                </div>
                <div v-if="needsReviewDiffDoc === doc.document_id && needsReviewDiffLines.length > 0" class="mt-3 border-t border-slate-800 pt-3">
                  <DiffView :diff="needsReviewDiffLines.join('\n')" />
                </div>
              </div>
            </div>
          </div>

          <!-- TAB: Recently Changed -->
          <div v-else-if="docDashTab === 'recent'">
            <div class="flex justify-end mb-3">
              <button @click="loadChangedDocs(); loadNeedsReview()" class="inline-flex items-center gap-1.5 px-2.5 py-1.5 text-xs text-slate-500 hover:text-slate-300 hover:bg-slate-800 rounded-lg transition-colors border border-slate-800">
                <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182" />
                </svg>
                Refresh
              </button>
            </div>
            <div v-if="changedDocs.length === 0" class="text-center py-12 text-sm text-slate-600">No recent changes.</div>
            <div v-else class="space-y-1">
              <button v-for="doc in changedDocs" :key="doc.path" @click="openChangedDoc(doc)"
                class="w-full flex items-center gap-3 px-4 py-3 bg-slate-900 border border-slate-800 rounded-lg hover:border-blue-500/30 hover:bg-slate-900/80 transition-all text-left group">
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="text-sm font-medium text-blue-400 group-hover:text-blue-300">{{ doc.document_id || doc.path.split('/').pop().replace('.md', '') }}</span>
                    <span class="text-[10px] px-1.5 py-0.5 rounded bg-slate-800 text-slate-500">{{ doc.folder }}</span>
                  </div>
                  <div v-if="doc.title" class="text-xs text-slate-400 mt-0.5 truncate">{{ doc.title }}</div>
                  <div class="text-[10px] text-slate-600 mt-1">
                    {{ doc.commit_time }} by {{ doc.author }} — <span class="font-mono">{{ doc.commit_hash }}</span> {{ doc.commit_message }}
                  </div>
                </div>
                <svg class="w-4 h-4 text-slate-700 group-hover:text-blue-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
                </svg>
              </button>
            </div>
          </div>

          <!-- TAB: Search -->
          <div v-else-if="docDashTab === 'search'">
            <div class="relative mb-4">
              <svg class="w-4 h-4 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
              </svg>
              <input
                v-model="searchQuery"
                @input="onSearchInput"
                type="text"
                placeholder="Search all documents..."
                class="w-full pl-10 pr-8 py-2.5 bg-slate-900 border border-slate-800 rounded-xl text-sm text-slate-200 placeholder-slate-600 focus:outline-none focus:border-blue-500"
              />
              <button v-if="searchQuery" @click="clearSearch" class="absolute right-3 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300">
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div v-if="!searchQuery || searchQuery.length < 2" class="text-center py-12 text-sm text-slate-600">
              Type at least 2 characters to search across all documents.
            </div>
            <div v-else-if="searchLoading" class="text-center py-8 text-xs text-slate-600">Searching...</div>
            <div v-else-if="searchResults.length === 0" class="text-center py-8 text-xs text-slate-600">No results for "{{ searchQuery }}"</div>
            <div v-else class="space-y-1">
              <button v-for="result in searchResults" :key="result.path" @click="openSearchResult(result)"
                class="w-full px-4 py-3 bg-slate-900 border border-slate-800 rounded-lg text-left hover:border-blue-500/30 hover:bg-slate-900/80 transition-all group">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium text-blue-400 group-hover:text-blue-300">{{ result.document_id || result.title }}</span>
                  <span class="text-[10px] px-1.5 py-0.5 rounded bg-slate-800 text-slate-500">{{ result.folder }}</span>
                </div>
                <div v-if="result.title && result.document_id" class="text-xs text-slate-400 mt-0.5 truncate">{{ result.title }}</div>
                <div v-if="result.snippet" class="text-[10px] text-slate-600 mt-1 line-clamp-2">{{ result.snippet }}</div>
              </button>
            </div>
          </div>

          <!-- TAB: Templates (admin/manager only) -->
          <div v-else-if="docDashTab === 'templates' && canEditMetadata">
            <div class="flex items-center justify-between mb-4">
              <h3 class="text-sm font-semibold text-slate-400">Templates</h3>
              <p class="text-[11px] text-slate-600">Add or remove document scaffolding for standards.</p>
            </div>
            <div class="space-y-2">
              <div v-for="fw in templateList" :key="fw.id"
                class="flex items-center justify-between px-4 py-3 bg-slate-900 border border-slate-800 rounded-lg">
                <div class="flex items-center gap-3 min-w-0">
                  <span v-if="fw.installed" class="text-emerald-400 text-sm flex-shrink-0">&#10003;</span>
                  <span v-else class="text-slate-600 text-sm flex-shrink-0">+</span>
                  <div class="min-w-0">
                    <div class="text-sm font-medium" :class="fw.installed ? 'text-slate-200' : 'text-slate-500'">{{ fw.label }}</div>
                    <div class="text-[11px] text-slate-600">{{ fw.description }}</div>
                  </div>
                </div>
                <div class="flex items-center gap-2 flex-shrink-0">
                  <button v-if="fw.installed" @click="navigateToTemplate(fw.id)" class="text-[10px] text-emerald-500/70 font-medium px-2 py-0.5 bg-emerald-500/10 hover:bg-emerald-500/20 rounded transition-colors">Installed</button>
                  <button v-if="!fw.installed" @click="addTemplate(fw.id)"
                    :disabled="templateLoading === fw.id"
                    class="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors bg-blue-600/10 text-blue-400 hover:bg-blue-600/20 border border-blue-500/20 disabled:opacity-50">
                    <svg v-if="templateLoading === fw.id" class="w-3 h-3 animate-spin" fill="none" viewBox="0 0 24 24">
                      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                    Add
                  </button>
                  <button v-if="fw.installed" @click="confirmRemoveTemplate(fw)"
                    :disabled="templateLoading === fw.id"
                    class="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors text-red-400/70 hover:text-red-400 hover:bg-red-500/10 border border-red-500/10 hover:border-red-500/20 disabled:opacity-50">
                    <svg v-if="templateLoading === fw.id" class="w-3 h-3 animate-spin" fill="none" viewBox="0 0 24 24">
                      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                    Remove
                  </button>
                </div>
              </div>
            </div>
            <div v-if="templateMessage" class="mt-3 text-xs px-3 py-2 rounded-lg"
              :class="templateMessageError ? 'bg-red-500/10 text-red-400' : 'bg-emerald-500/10 text-emerald-400'">
              {{ templateMessage }}
            </div>
          </div>

          <!-- Remove template confirm dialog -->
          <div v-if="showRemoveConfirm" class="fixed inset-0 z-50 flex items-center justify-center bg-black/60" @click.self="showRemoveConfirm = false">
            <div class="bg-slate-900 border border-slate-700 rounded-xl p-6 max-w-sm w-full mx-4 shadow-xl">
              <h3 class="text-sm font-semibold text-slate-200 mb-2">Remove Template</h3>
              <p class="text-xs text-slate-400 mb-4">
                Are you sure you want to remove <span class="font-semibold text-slate-200">{{ removeTarget?.label }}</span>?
                This will delete all documents under <code class="text-[11px] bg-slate-800 px-1 py-0.5 rounded">documents/{{ removeTarget?.id }}/</code> from the repository.
              </p>
              <div class="flex justify-end gap-2">
                <button @click="showRemoveConfirm = false"
                  class="px-3 py-1.5 text-xs font-medium rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800 border border-slate-700">
                  Cancel
                </button>
                <button @click="removeTemplate(removeTarget.id)"
                  :disabled="templateLoading === removeTarget?.id"
                  class="px-3 py-1.5 text-xs font-medium rounded-lg bg-red-600 hover:bg-red-500 text-white disabled:opacity-50">
                  Remove
                </button>
              </div>
            </div>
          </div>
          </template>
        </div>
      </div>

      <!-- Document view -->
      <div v-else class="p-8">
        <div class="max-w-3xl mx-auto space-y-6">
          <!-- Breadcrumb + actions bar (single line) -->
          <div class="sticky top-0 z-10 bg-slate-950 pb-3 pt-2 flex items-center gap-3">
            <nav v-if="activeBreadcrumb.length > 0" class="flex items-center gap-1 text-xs text-slate-500 min-w-0 flex-1 overflow-hidden">
              <template v-for="(crumb, idx) in activeBreadcrumb" :key="idx">
                <span v-if="idx > 0" class="text-slate-700 flex-shrink-0">/</span>
                <span class="truncate" :class="idx === activeBreadcrumb.length - 1 ? 'text-slate-300 font-medium' : 'text-slate-500'">{{ crumb }}</span>
              </template>
            </nav>
            <div class="flex items-center gap-1.5 flex-shrink-0">
              <!-- Edit (primary CTA with label) -->
              <button v-if="canEditMetadata && !editMode" @click="startEditGuarded"
                class="inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-md transition-colors text-slate-400 hover:text-slate-200 hover:bg-slate-800">
                <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125" />
                </svg>
                Edit
              </button>
              <template v-if="editMode">
                <button @click="saveEdit" :disabled="savingEdit"
                  class="inline-flex items-center gap-1.5 px-3 py-1 text-xs font-medium rounded-md transition-colors bg-emerald-600 hover:bg-emerald-500 text-white disabled:opacity-50">
                  {{ savingEdit ? 'Saving...' : 'Save' }}
                </button>
                <button @click="cancelEdit"
                  class="inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-md transition-colors text-slate-400 hover:text-slate-200 hover:bg-slate-800">
                  Cancel
                </button>
              </template>
              <!-- Send for review (icon only) -->
              <button v-if="!editMode && canEditMetadata" @click="showReviewModal = true"
                class="p-1.5 rounded-md transition-colors text-blue-400 hover:bg-blue-600/15" title="Send for review">
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 12L3.269 3.126A59.768 59.768 0 0121.485 12 59.77 59.77 0 013.27 20.876L5.999 12zm0 0h7.5" />
                </svg>
              </button>
              <!-- History (icon only) — toggles Versions tab -->
              <button v-if="!editMode" @click="toggleHistory"
                class="p-1.5 rounded-md transition-colors" :class="docTab === 'versions' ? 'text-blue-400 bg-blue-600/15' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800'" title="Version history">
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </button>
              <!-- Comments (icon only with count badge) -->
              <button @click="showRightPanel = !showRightPanel"
                class="relative p-1.5 rounded-md transition-colors" :class="showRightPanel ? 'text-blue-400 bg-blue-600/15' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800'" title="Comments">
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 8.25h9m-9 3H12m-9.75 1.51c0 1.6 1.123 2.994 2.707 3.227 1.087.16 2.185.283 3.293.369V21l4.076-4.076a1.526 1.526 0 011.037-.443 48.282 48.282 0 005.68-.494c1.584-.233 2.707-1.626 2.707-3.228V6.741c0-1.602-1.123-2.995-2.707-3.228A48.394 48.394 0 0012 3c-2.392 0-4.744.175-7.043.513C3.373 3.746 2.25 5.14 2.25 6.741v6.018z" />
                </svg>
                <span v-if="openCommentCount > 0" class="absolute -top-1 -right-1 min-w-[16px] h-4 flex items-center justify-center px-1 text-[9px] font-bold text-white bg-blue-500 rounded-full">{{ openCommentCount }}</span>
              </button>
              <!-- Print (icon only) -->
              <button v-if="!editMode" @click="printDocument"
                class="p-1.5 rounded-md transition-colors text-slate-500 hover:text-slate-300 hover:bg-slate-800 no-print" title="Print document">
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6.72 13.829c-.24.03-.48.062-.72.096m.72-.096a42.415 42.415 0 0110.56 0m-10.56 0L6.34 18m10.94-4.171c.24.03.48.062.72.096m-.72-.096L17.66 18m0 0l.229 2.523a1.125 1.125 0 01-1.12 1.227H7.231c-.662 0-1.18-.568-1.12-1.227L6.34 18m11.318 0h1.091A2.25 2.25 0 0021 15.75V9.456c0-1.081-.768-2.015-1.837-2.175a48.055 48.055 0 00-1.913-.247M6.34 18H5.25A2.25 2.25 0 013 15.75V9.456c0-1.081.768-2.015 1.837-2.175a48.041 48.041 0 011.913-.247m10.5 0a48.536 48.536 0 00-10.5 0m10.5 0V3.375c0-.621-.504-1.125-1.125-1.125h-8.25c-.621 0-1.125.504-1.125 1.125v3.659M18.25 7.034V3.375" />
                </svg>
              </button>
              <!-- Delete (icon only) -->
              <button v-if="!editMode && canEditMetadata" @click="deleteCurrentDocument"
                class="p-1.5 rounded-md transition-colors text-slate-600 hover:text-red-400 hover:bg-red-500/10" title="Delete document">
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
                </svg>
              </button>
            </div>
          </div>

          <!-- Print-only header (hidden on screen, visible when printing) -->
          <div class="print-header hidden">
            <div class="print-title">{{ activeDoc.title || activeDoc.Title }}</div>
            <div class="print-meta">
              <span v-if="activeDoc.version">Version {{ activeDoc.version }}</span>
              <span v-if="activeDoc.version && activeDoc.status"> &mdash; </span>
              <span v-if="activeDoc.status">{{ activeDoc.status }}</span>
              <span v-if="activeDoc.author"> &mdash; Author: {{ resolveUserName(activeDoc.author) || activeDoc.author }}</span>
              <span> &mdash; {{ activeId }}</span>
              <span> &mdash; Printed {{ new Date().toLocaleDateString() }}</span>
            </div>
          </div>

          <!-- Title + status pill (always visible at top of pane) -->
          <div v-if="!editMode" class="no-print flex items-start gap-3">
            <h1 class="text-xl font-bold text-slate-100 flex-1 min-w-0">{{ activeDoc.title || activeDoc.Title }}</h1>
            <StatusBadge v-if="activeDoc.status" :status="activeDoc.status" class="flex-shrink-0 mt-1" />
          </div>

          <!-- Tab nav (hidden in edit mode — editor takes the whole pane) -->
          <div v-if="!editMode" class="border-b border-slate-800 no-print">
            <div class="flex gap-1">
              <button
                v-for="t in docTabs"
                :key="t.key"
                @click="switchDocTab(t.key)"
                class="px-4 py-2 text-sm font-medium border-b-2 transition-colors -mb-px"
                :class="docTab === t.key
                  ? 'border-blue-500 text-blue-400'
                  : 'border-transparent text-slate-500 hover:text-slate-300'"
              >{{ t.label }}</button>
            </div>
          </div>

          <!-- Document header card (Info tab) -->
          <div v-if="docTab === 'info' || editMode" class="bg-slate-900 border border-slate-800 rounded-xl p-6 no-print">
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-4">
              <div>
                <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">Status</div>
                <router-link v-if="activeDoc.status === 'in_review' && activeDoc.active_review_id"
                  :to="orgPath(`/reviews/${activeDoc.active_review_id}`)"
                  class="inline-block hover:opacity-80 transition-opacity" title="Go to active review">
                  <StatusBadge :status="activeDoc.status" />
                </router-link>
                <router-link v-else-if="activeDoc.status === 'in_review'"
                  :to="orgPath('/reviews')"
                  class="inline-block hover:opacity-80 transition-opacity" title="Go to reviews">
                  <StatusBadge :status="activeDoc.status" />
                </router-link>
                <StatusBadge v-else-if="activeDoc.status" :status="activeDoc.status" />
                <span v-else class="text-sm text-slate-400">--</span>
              </div>
              <div>
                <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">Type</div>
                <div v-if="editMode">
                  <select :value="activeDoc.type || ''" @change="setDocType($event.target.value)"
                    class="w-full px-2 py-1 bg-slate-800 border border-slate-700 rounded text-sm text-slate-300 focus:outline-none focus:border-blue-500">
                    <option value="">Document</option>
                    <option value="policy">Policy</option>
                    <option value="procedure">Procedure</option>
                    <option value="control">Control</option>
                    <option value="guideline">Guideline</option>
                    <option value="record">Record</option>
                    <option value="clause">Clause</option>
                    <option value="requirement">Requirement</option>
                  </select>
                </div>
                <div v-else class="text-sm text-slate-300 capitalize">{{ activeDoc.type || 'Document' }}</div>
              </div>
              <div>
                <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">Author</div>
                <div v-if="editMode" class="mt-0.5">
                  <MemberPicker
                    :modelValue="activeDoc.author || ''"
                    :members="allUsers"
                    placeholder="Set author..."
                    @update:modelValue="setDocAuthor"
                  />
                </div>
                <div v-else class="text-sm" :class="activeDoc.author ? 'text-slate-300' : 'text-slate-600 italic'">
                  {{ resolveUserName(activeDoc.author) || 'Not set' }}
                </div>
              </div>
              <div>
                <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">Version</div>
                <div v-if="editMode">
                  <input v-model="editVersion"
                    class="w-20 px-2 py-1 bg-slate-800 border border-slate-700 rounded text-sm text-slate-300 focus:outline-none focus:border-blue-500" />
                </div>
                <div v-else class="text-sm text-slate-300">{{ activeDoc.version || '--' }}</div>
              </div>
              <div>
                <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">Owner</div>
                <div v-if="editMode">
                  <select v-model="editOwner"
                    class="w-full px-2 py-1 bg-slate-800 border border-slate-700 rounded text-sm text-slate-300 focus:outline-none focus:border-blue-500">
                    <option value="">Not set</option>
                    <option v-for="u in allUsers" :key="u.email" :value="u.email">{{ u.name || u.email }}</option>
                  </select>
                </div>
                <div v-else class="text-sm" :class="activeDoc.owner ? 'text-slate-300' : 'text-slate-600 italic'">
                  {{ resolveUserName(activeDoc.owner) || resolveUserName(activeDoc.author) || 'Not set' }}
                </div>
              </div>
            </div>

            <!-- Review cadence -->
            <div v-if="activeDoc.review_cycle || approvedAt" class="grid grid-cols-2 sm:grid-cols-4 gap-4 mt-3 pt-3 border-t border-slate-800">
              <div v-if="activeDoc.review_cycle">
                <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">Review Cycle</div>
                <div class="text-sm text-slate-400">Every {{ activeDoc.review_cycle }} months</div>
              </div>
              <div v-if="approvedAt">
                <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">Last Approved</div>
                <div class="text-sm text-slate-400">{{ approvedAt }}</div>
              </div>
              <div v-if="approvedVersion">
                <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">Approved Version</div>
                <div class="text-sm text-slate-400">{{ approvedVersion }}</div>
              </div>
              <div v-if="docNextReview">
                <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">Next Review</div>
                <div class="text-sm" :class="docReviewOverdue ? 'text-red-400 font-medium' : docReviewDueSoon ? 'text-amber-400' : 'text-slate-400'">
                  {{ docNextReview }}
                  <span v-if="docReviewOverdue" class="text-[10px] ml-1 px-1.5 py-0.5 rounded-full bg-red-500/20 text-red-300 font-semibold">OVERDUE</span>
                  <span v-else-if="docReviewDueSoon" class="text-[10px] ml-1 px-1.5 py-0.5 rounded-full bg-amber-500/20 text-amber-300 font-semibold">DUE SOON</span>
                </div>
              </div>
            </div>

            <!-- Document status banner -->
            <!-- In review banner — always visible, even in edit mode.
                 Escalates when reviewer has requested changes so the author knows what to do next. -->
            <div v-if="activeDoc.active_review_id" class="mt-4 pt-4 border-t border-slate-800">
              <!-- Changes requested: explicit "what to do next" banner -->
              <div v-if="activeDoc.active_review_status === 'changes_requested'"
                class="bg-amber-950/30 border border-amber-800/40 rounded-lg px-4 py-3">
                <div class="flex items-start gap-3">
                  <span class="w-2 h-2 mt-1.5 rounded-full bg-amber-500 animate-pulse flex-shrink-0" />
                  <div class="flex-1 min-w-0">
                    <div class="text-sm text-amber-300 font-medium">
                      Round {{ activeDoc.active_review_round || 1 }} complete — changes requested
                    </div>
                    <div class="text-xs text-amber-400/80 mt-0.5">
                      <span v-if="activeDoc.active_review_pending_suggestions > 0">
                        {{ activeDoc.active_review_pending_suggestions }}
                        pending suggestion{{ activeDoc.active_review_pending_suggestions === 1 ? '' : 's' }} to address.
                      </span>
                      <span v-else>Reviewer feedback is waiting on the conversation.</span>
                      Address the feedback, then resubmit for the next round.
                    </div>
                  </div>
                  <router-link :to="orgPath(`/reviews/${activeDoc.active_review_id}`)"
                    class="text-xs px-3 py-1.5 rounded-md bg-amber-600 hover:bg-amber-500 text-white font-medium transition-colors flex-shrink-0">
                    Open review
                  </router-link>
                </div>
              </div>
              <!-- Open / approved-but-not-merged: simple in-review banner -->
              <div v-else class="flex items-center gap-2 text-xs">
                <span class="w-2 h-2 rounded-full bg-blue-500 animate-pulse flex-shrink-0" />
                <span class="text-blue-400">
                  This document is in review<template v-if="activeDoc.active_review_round"> (Round {{ activeDoc.active_review_round }})</template>.
                </span>
                <router-link :to="orgPath(`/reviews/${activeDoc.active_review_id}`)"
                  class="text-blue-400 hover:text-blue-300 ml-auto">View review →</router-link>
              </div>
            </div>

            <!-- Other status banners — hidden in edit mode -->
            <div v-if="!editMode && !activeDoc.active_review_id && (activeDoc.status === 'retired' || isDocDirty || isDocNeverApproved)" class="mt-4 pt-4 border-t border-slate-800">

              <!-- Retired -->
              <div v-if="activeDoc.status === 'retired'" class="flex items-center gap-2 text-xs">
                <span class="w-2 h-2 rounded-full bg-slate-600 flex-shrink-0" />
                <span class="text-slate-500">This document is retired.</span>
              </div>

              <!-- 3. Never approved, not in review -->
              <div v-else-if="isDocNeverApproved" class="flex items-center gap-2 text-xs">
                <span class="w-2 h-2 rounded-full bg-slate-600 flex-shrink-0" />
                <span class="text-slate-500">This document has never been approved.</span>
                <button v-if="canEditMetadata" @click="showReviewModal = true" class="text-blue-400 hover:text-blue-300 ml-auto">Send for review →</button>
              </div>

              <!-- 4. Approved but has changes since -->
              <div v-else-if="isDocDirty" class="flex items-center gap-2 text-xs flex-wrap">
                <span class="w-2 h-2 rounded-full bg-amber-500 animate-pulse flex-shrink-0" />
                <span class="text-amber-400">Changed since last approval</span>
                <span class="text-slate-600">— last approved{{ approvedVersion ? ` ${approvedVersion}` : '' }}<span v-if="approvedAt"> on {{ approvedAt }}</span></span>
                <div class="ml-auto flex gap-2">
                  <button @click="viewApprovedVersion" class="text-slate-500 hover:text-slate-300">
                    {{ showApprovedDiff ? 'Hide diff' : 'View diff' }}
                  </button>
                  <button v-if="canEditMetadata" @click="showReviewModal = true" class="text-blue-400 hover:text-blue-300">Send for review →</button>
                </div>
              </div>
              <!-- Diff since approval -->
              <div v-if="showApprovedDiff" class="mt-3">
                <div v-if="loadingApprovedDiff" class="h-8 bg-slate-800 rounded animate-pulse" />
                <DiffView v-else-if="approvedDiffLines.length > 0" :diff="approvedDiffLines.join('\n')" />
              </div>
            </div>
          </div>

          <!-- Version history panel (Versions tab) -->
          <div v-if="docTab === 'versions' && !editMode" class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
            <div class="px-4 py-3 border-b border-slate-800">
              <span class="text-sm font-semibold text-slate-300">Version History</span>
            </div>
            <div v-if="loadingVersions" class="p-4 space-y-2">
              <div v-for="i in 3" :key="i" class="h-4 bg-slate-800 rounded animate-pulse" />
            </div>
            <div v-else-if="versions.length === 0" class="p-4 text-xs text-slate-600 text-center">
              No version history available
            </div>
            <div v-else class="divide-y divide-slate-800 max-h-48 overflow-y-auto">
              <button
                v-for="(ver, idx) in versions"
                :key="ver.hash || idx"
                @click="selectDiffVersion(ver, idx)"
                class="w-full flex items-center gap-3 px-4 py-2.5 text-left hover:bg-slate-800/50 transition-colors"
                :class="{ 'bg-slate-800/30': selectedVersionIdx === idx }"
              >
                <div class="w-2 h-2 rounded-full flex-shrink-0" :class="idx === 0 ? 'bg-emerald-500' : 'bg-slate-600'" />
                <div class="flex-1 min-w-0">
                  <div class="text-xs text-slate-300 truncate">
                    <span class="font-semibold">{{ ver.version || 'v?' }}</span>
                    <span v-if="ver.message" class="text-slate-400 ml-1.5">{{ ver.message }}</span>
                  </div>
                  <div class="text-[10px] text-slate-600">
                    {{ ver.created_by }} — {{ formatDate(ver.created_at) }}
                  </div>
                </div>
              </button>
            </div>
            <!-- Diff view -->
            <div v-if="diffText" class="border-t border-slate-800 p-3">
              <DiffView :diff="diffText" />
            </div>
            <div v-else-if="loadingDiff" class="border-t border-slate-800 p-4">
              <div class="h-4 bg-slate-800 rounded animate-pulse" />
            </div>
          </div>

          <!-- Links tab — cross-entity references -->
          <div v-if="docTab === 'links' && !editMode && activeDoc && activeId" class="bg-slate-900 border border-slate-800 rounded-xl p-4">
            <ReferenceManager entityType="document" :entityId="activeId" :editable="canEditMetadata" />
          </div>

          <!-- Discussion tab — document-level comments (paragraph-level inline comments stay on Content) -->
          <div v-if="docTab === 'discussion' && !editMode && activeDoc && activeId" class="bg-slate-900 border border-slate-800 rounded-xl p-4">
            <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">Document discussion</div>
            <CommentsPanel entityType="document" :entityId="activeId" />
          </div>

          <!-- History tab — entity changelog -->
          <div v-if="docTab === 'history' && !editMode && activeDoc && activeId" class="bg-slate-900 border border-slate-800 rounded-xl p-4">
            <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">Document changelog</div>
            <HistoryPanel entityType="document" :entityId="activeId" />
          </div>

          <!-- Inline comment count summary (Content tab) -->
          <div v-if="docTab === 'content' && inlineCommentCount > 0 && !editMode" class="flex items-center gap-2 px-1">
            <div class="w-2 h-2 rounded-full bg-blue-500 animate-pulse" />
            <span class="text-xs text-slate-500">
              {{ inlineCommentCount }} inline comment{{ inlineCommentCount === 1 ? '' : 's' }} on this document
            </span>
          </div>

          <!-- Draft recovery banner -->
          <div v-if="editMode && hasDraft" class="flex items-center gap-3 px-4 py-3 bg-amber-950/30 border border-amber-800/30 rounded-lg text-sm">
            <svg class="w-4 h-4 text-amber-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
            </svg>
            <span class="text-amber-300 flex-1">Unsaved draft recovered</span>
            <button @click="discardDraft" class="text-xs text-slate-400 hover:text-white px-2 py-1 rounded hover:bg-slate-800">Discard draft</button>
          </div>

          <!-- Document editor (edit mode) -->
          <!-- NOTE: Documents intentionally use tiptap's internal dirty tracking (the editor is the
               source of truth for content state). Do NOT replace with useDirtyEdit — tiptap already
               tracks transactions, autosave is wired through useDocumentEditor's startAutosave. -->
          <div v-if="editMode" class="rounded-xl overflow-hidden border border-slate-800">
            <DocumentEditor v-model="editContent" :editable="true" :documentId="activeId" :self-type="'document'" :self-id="activeId" @save="saveEdit" />
          </div>

          <!-- Save feedback -->
          <div v-if="editSaveMsg" class="px-3 py-2 rounded-lg text-sm font-medium"
            :class="editSaveError ? 'bg-red-900/30 text-red-400 border border-red-800' : 'bg-emerald-900/30 text-emerald-400 border border-emerald-800'">
            {{ editSaveMsg }}
          </div>

          <!-- Content tab: loading skeleton, rendered markdown body, or empty state -->
          <template v-if="docTab === 'content'">
          <!-- Content loading -->
          <div v-if="!editMode && loadingContent" class="space-y-3 py-4">
            <div class="h-4 bg-slate-800 rounded w-3/4 animate-pulse" />
            <div class="h-4 bg-slate-800 rounded w-full animate-pulse" />
            <div class="h-4 bg-slate-800 rounded w-5/6 animate-pulse" />
            <div class="h-4 bg-slate-800 rounded w-2/3 animate-pulse" />
          </div>

          <!-- Rendered markdown content — paragraph-level commentable blocks -->
          <div v-else-if="!editMode && contentBlocks.length > 0" class="pr-12 pl-8 relative">
            <div
              v-for="block in contentBlocks"
              :key="block.index"
              class="comment-block relative group transition-colors duration-200 rounded -mx-2 px-2"
              :class="{
                'bg-blue-950/20': hasOpenComments(block.index),
                'hover:bg-slate-900/40': !hasOpenComments(block.index),
                'review-checked': reviewedBlocks.has(block.index)
              }"
            >
              <!-- Review checkbox -->
              <button
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

              <!-- The rendered content block (with [[TYPE:ID|Title]] reference pills) -->
              <div v-html="sanitize(renderRefLinks(block.html))" class="doc-prose" @click="onRefLinkClick" />

              <!-- Comment count badge (visible when paragraph has open comments) -->
              <div
                v-if="hasOpenComments(block.index)"
                @click="toggleBlockComments(block.index)"
                class="absolute -right-10 top-1 w-6 h-6 rounded-full bg-blue-600 text-white text-xs font-bold flex items-center justify-center cursor-pointer hover:bg-blue-500 transition-colors shadow-lg shadow-blue-900/30"
                :title="commentCountForBlock(block.index) + ' comment' + (commentCountForBlock(block.index) === 1 ? '' : 's')"
              >
                {{ commentCountForBlock(block.index) }}
              </div>

              <!-- Add comment button (appears on hover) -->
              <button
                v-if="!hasOpenComments(block.index)"
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
                <div v-if="expandedBlock === block.index" class="mt-1 mb-4 ml-4 border-l-2 border-blue-600/60 pl-4 overflow-hidden">
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
                        @click="resolveComment(comment.id)"
                        class="ml-auto text-[10px] text-slate-500 hover:text-emerald-400 transition-colors"
                      >Resolve</button>
                    </div>
                    <!-- Resolved: show collapsed or expanded body -->
                    <template v-if="comment.status === 'resolved'">
                      <div class="text-[10px] text-emerald-500/70 flex items-center gap-1 mb-1">
                        Resolved by {{ comment.resolved_by || 'unknown' }} {{ formatDate(comment.resolved_at) }}
                      </div>
                      <div
                        v-if="!expandedResolvedInline.has(comment.id)"
                        @click="toggleResolvedInline(comment.id)"
                        class="text-sm text-slate-500 leading-relaxed truncate cursor-pointer hover:text-slate-400"
                      >{{ firstLine(comment.body) }}</div>
                      <div
                        v-else
                        @click="toggleResolvedInline(comment.id)"
                        class="text-sm text-slate-500 leading-relaxed whitespace-pre-wrap cursor-pointer hover:text-slate-400"
                        v-html="sanitize(renderCommentBody(comment.body))"
                      ></div>
                    </template>
                    <!-- Open: show full body with @mention highlighting -->
                    <div v-else class="text-sm text-slate-400 leading-relaxed whitespace-pre-wrap" v-html="sanitize(renderCommentBody(comment.body))"></div>
                  </div>

                  <!-- New comment form -->
                  <div class="backdrop-blur rounded-lg p-3 border relative transition-colors duration-200 bg-slate-900/80 border-slate-800/50">
                    <textarea
                      ref="inlineTextareaRef"
                      v-model="inlineCommentText"
                      placeholder="Add a comment on this paragraph... (type @ to mention)"
                      class="w-full bg-transparent border border-slate-700 rounded-md p-2 text-sm text-slate-300 placeholder-slate-600 resize-none focus:outline-none focus:ring-1 focus:border-blue-500 focus:ring-blue-500/30"
                      rows="2"
                      @input="e => handleCommentInput(e, 'inline')"
                      @keydown="handleCommentKeydown"
                      @keydown.meta.enter="submitInlineComment(expandedBlock)"
                      @keydown.ctrl.enter="submitInlineComment(expandedBlock)"
                    />
                    <!-- @mention dropdown for inline textarea -->
                    <div v-if="showMentionDropdown && mentionContext === 'inline'"
                      class="z-50 bg-slate-800 border border-slate-700 rounded-lg shadow-xl py-1 w-56 -mt-1 mb-1">
                      <button v-for="(u, idx) in mentionUsers" :key="u.email || u.name"
                        @mousedown.prevent="selectMention(u)"
                        class="w-full flex items-center gap-2 px-3 py-1.5 text-left hover:bg-slate-700 transition-colors"
                        :class="{ 'bg-slate-700': idx === mentionSelectedIndex }">
                        <div class="w-5 h-5 rounded-full bg-blue-600 flex items-center justify-center text-[10px] font-bold text-white flex-shrink-0">
                          {{ (u.name || '?').charAt(0) }}
                        </div>
                        <div class="min-w-0">
                          <div class="text-xs text-slate-300 truncate">{{ u.name }}</div>
                          <div class="text-[10px] text-slate-600 truncate">{{ u.email }}</div>
                        </div>
                      </button>
                      <div v-if="mentionUsers.length === 0" class="px-3 py-2 text-xs text-slate-500">No users found</div>
                    </div>
                    <div class="flex items-center justify-end mt-2">
                      <div class="flex gap-2">
                        <button
                          @click="expandedBlock = null; inlineCommentText = ''"
                          class="text-xs text-slate-500 hover:text-slate-300 px-2 py-1 rounded transition-colors cursor-pointer"
                        >Cancel</button>
                        <button
                          @click="submitInlineComment(expandedBlock)"
                          :disabled="!inlineCommentText.trim() || submittingInline"
                          class="text-xs bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 disabled:text-slate-500 text-white px-3 py-1 rounded font-medium transition-colors cursor-pointer"
                        >{{ submittingInline ? 'Saving...' : 'Comment' }}
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              </transition>
            </div>
          </div>

          <!-- No content -->
          <div v-else-if="!editMode && !loadingContent" class="py-8 text-center text-sm text-slate-600">
            No content available for this document.
          </div>
          </template>
        </div>
      </div>
    </main>

    <!-- Right panel: Comments & activity (collapsible) -->
    <aside v-if="showRightPanel" class="w-[340px] flex-shrink-0 bg-slate-900 border-l border-slate-800 flex flex-col overflow-hidden">
      <!-- No doc selected -->
      <div v-if="!activeDoc" class="flex items-center justify-center h-full">
        <div class="text-xs text-slate-600">Select a document to view comments</div>
      </div>

      <template v-else>
        <!-- Panel header — clickable to toggle -->
        <button @click="showCommentPanel = !showCommentPanel"
          class="w-full px-5 py-3 border-b border-slate-800 flex-shrink-0 flex items-center justify-between hover:bg-slate-800/30 transition-colors">
          <div class="flex items-center gap-2">
            <svg class="w-3.5 h-3.5 text-slate-500 transition-transform" :class="{ 'rotate-90': showCommentPanel }" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
            </svg>
            <h3 class="text-sm font-semibold text-slate-300">Comments</h3>
          </div>
          <div class="flex items-center gap-2">
            <span v-if="openCommentCount > 0" class="text-xs font-bold bg-blue-600 text-white px-1.5 py-0.5 rounded-full">{{ openCommentCount }}</span>
            <span v-if="resolvedCommentCount > 0" class="text-xs text-slate-600">{{ resolvedCommentCount }} resolved</span>
          </div>
        </button>

        <!-- General comment form (always visible if panel open) -->
        <div v-show="showCommentPanel" class="px-5 py-4 border-b border-slate-800 space-y-3 flex-shrink-0 relative">
          <textarea
            v-model="newComment"
            placeholder="Add a general comment... (type @ to mention)"
            rows="3"
            class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500 focus:border-blue-500 resize-none"
            @input="e => handleCommentInput(e, 'panel')"
            @keydown="handleCommentKeydown"
          />
          <!-- @mention dropdown for panel textarea -->
          <div v-if="showMentionDropdown && mentionContext === 'panel'"
            class="z-50 bg-slate-800 border border-slate-700 rounded-lg shadow-xl py-1 w-56 -mt-2">
            <button v-for="(u, idx) in mentionUsers" :key="u.email || u.name"
              @mousedown.prevent="selectMention(u)"
              class="w-full flex items-center gap-2 px-3 py-1.5 text-left hover:bg-slate-700 transition-colors"
              :class="{ 'bg-slate-700': idx === mentionSelectedIndex }">
              <div class="w-5 h-5 rounded-full bg-blue-600 flex items-center justify-center text-[10px] font-bold text-white flex-shrink-0">
                {{ (u.name || '?').charAt(0) }}
              </div>
              <div class="min-w-0">
                <div class="text-xs text-slate-300 truncate">{{ u.name }}</div>
                <div class="text-[10px] text-slate-600 truncate">{{ u.email }}</div>
              </div>
            </button>
            <div v-if="mentionUsers.length === 0" class="px-3 py-2 text-xs text-slate-500">No users found</div>
          </div>
          <div class="flex items-center gap-2">
            <input
              v-model="commentAuthor"
              placeholder="Your name"
              class="flex-1 bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500 focus:border-blue-500"
            />
            <button
              @click="submitComment"
              :disabled="!newComment.trim() || submitting"
              class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 disabled:text-slate-500 text-white text-sm font-medium rounded-lg transition-colors"
            >
              {{ submitting ? 'Sending...' : 'Send' }}
            </button>
          </div>
        </div>

        <!-- Comments list -->
        <div v-show="showCommentPanel" class="flex-1 overflow-y-auto">
          <div v-if="loadingComments" class="p-5 space-y-3">
            <div v-for="i in 3" :key="i" class="space-y-2">
              <div class="h-3 bg-slate-800 rounded w-1/3 animate-pulse" />
              <div class="h-3 bg-slate-800 rounded w-full animate-pulse" />
            </div>
          </div>

          <template v-else-if="openComments.length > 0 || resolvedComments.length > 0">
            <!-- Open comments -->
            <div
              v-for="comment in openComments"
              :key="comment.id"
              class="px-5 py-4 border-b border-slate-800 transition-colors"
              :class="{ 'hover:bg-slate-800/30 cursor-pointer': comment.paragraph_index != null }"
              @click="comment.paragraph_index != null && scrollToBlock(comment.paragraph_index)"
            >
              <!-- Comment type indicator -->
              <div v-if="comment.paragraph_index != null" class="flex items-center gap-1.5 mb-2">
                <svg class="w-3 h-3 text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
                </svg>
                <span class="text-[10px] text-blue-400 font-medium">Inline comment</span>
                <span v-if="comment.quote" class="text-[10px] text-slate-600 truncate max-w-[180px]">
                  — "{{ comment.quote }}"
                </span>
              </div>
              <div v-else class="flex items-center gap-1.5 mb-2">
                <svg class="w-3 h-3 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                </svg>
                <span class="text-[10px] text-slate-500 font-medium">Document comment</span>
              </div>
              <div class="flex items-baseline gap-2 mb-1.5">
                <span class="text-[13px] font-semibold text-slate-300">{{ comment.author }}</span>
                <span class="text-[11px] text-slate-500">{{ formatDate(comment.created_at) }}</span>
              </div>
              <p class="text-[13px] text-slate-400 leading-relaxed whitespace-pre-wrap" v-html="sanitize(renderCommentBody(comment.body))"></p>
              <div class="mt-2 flex gap-3">
                <button
                  @click.stop="startPanelReply(comment)"
                  class="text-[11px] text-slate-500 hover:text-blue-400 transition-colors cursor-pointer"
                >
                  Reply
                </button>
                <button
                  @click.stop="resolveComment(comment.id)"
                  class="text-[11px] text-slate-500 hover:text-emerald-400 transition-colors cursor-pointer"
                >
                  Resolve
                </button>
              </div>

              <!-- Replies -->
              <div v-for="reply in panelRepliesFor(comment.id)" :key="reply.id" class="mt-2 ml-4 pl-3 border-l-2 border-slate-700">
                <div class="flex items-center gap-2 mb-0.5">
                  <span class="text-[11px] font-semibold text-slate-400">{{ reply.author }}</span>
                  <span class="text-[10px] text-slate-600">{{ formatDate(reply.created_at) }}</span>
                </div>
                <p class="text-[12px] text-slate-400" v-html="sanitize(renderCommentBody(reply.body))"></p>
              </div>

              <!-- Reply form -->
              <div v-if="panelReplyingTo === comment.id" class="mt-2 ml-4 pl-3 border-l-2 border-blue-600">
                <textarea
                  v-model="panelReplyText"
                  placeholder="Write a reply..."
                  rows="2"
                  class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500 resize-none"
                  @keydown.meta.enter="submitPanelReply"
                  @keydown.ctrl.enter="submitPanelReply"
                />
                <div class="flex gap-2 mt-1">
                  <button @click="submitPanelReply" :disabled="!panelReplyText.trim()"
                    class="px-2 py-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-[10px] font-medium rounded cursor-pointer">Reply</button>
                  <button @click="panelReplyingTo = null"
                    class="px-2 py-1 text-[10px] text-slate-500 hover:text-slate-300 cursor-pointer">Cancel</button>
                </div>
              </div>
            </div>

            <!-- Resolved comments toggle -->
            <div v-if="resolvedComments.length > 0" class="border-t border-slate-800">
              <button
                @click="showResolvedPanel = !showResolvedPanel"
                class="w-full px-5 py-2.5 flex items-center gap-2 text-left hover:bg-slate-800/30 transition-colors"
              >
                <svg
                  class="w-3 h-3 text-slate-600 transition-transform"
                  :class="{ 'rotate-90': showResolvedPanel }"
                  fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                </svg>
                <svg class="w-3 h-3 text-emerald-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                </svg>
                <span class="text-[11px] font-medium text-slate-600">
                  {{ resolvedComments.length }} resolved comment{{ resolvedComments.length === 1 ? '' : 's' }}
                </span>
              </button>
              <template v-if="showResolvedPanel">
                <div
                  v-for="comment in resolvedComments"
                  :key="comment.id"
                  class="px-5 py-3 border-b border-slate-800 opacity-40 hover:opacity-60 transition-opacity"
                >
                  <div v-if="comment.paragraph_index != null" class="flex items-center gap-1.5 mb-1.5">
                    <svg class="w-3 h-3 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
                    </svg>
                    <span class="text-[10px] text-slate-600 font-medium">Inline</span>
                    <span v-if="comment.quote" class="text-[10px] text-slate-700 truncate max-w-[180px]">
                      &mdash; "{{ comment.quote }}"
                    </span>
                  </div>
                  <div class="flex items-baseline gap-2 mb-1">
                    <span class="text-[13px] font-semibold text-slate-300">{{ comment.author }}</span>
                    <span class="text-[11px] text-slate-500">{{ formatDate(comment.created_at) }}</span>
                  </div>
                  <p class="text-[13px] text-slate-400 leading-relaxed whitespace-pre-wrap" v-html="sanitize(renderCommentBody(comment.body))"></p>
                  <div class="mt-1 text-[10px] text-emerald-600/70 flex items-center gap-1">
                    <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                    </svg>
                    Resolved by {{ comment.resolved_by || 'unknown' }} {{ formatDate(comment.resolved_at) }}
                  </div>
                </div>
              </template>
            </div>
          </template>

          <div v-else class="p-5 text-center">
            <div class="text-xs text-slate-600 mt-4">No comments yet</div>
          </div>
        </div>
      </template>
    </aside>
    <!-- Send for review modal -->
    <Teleport to="body">
      <Transition name="modal">
      <div v-if="showReviewModal" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="absolute inset-0 bg-black/60" @click="showReviewModal = false" />
        <div class="relative bg-slate-900 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-md p-6 space-y-4">
          <h2 class="text-lg font-bold text-white">Send for review</h2>
          <p class="text-sm text-slate-400">{{ activeDoc?.title || activeId }}</p>

          <div>
            <label class="block text-xs text-slate-500 mb-2">Reviewers</label>
            <!-- Selected reviewers -->
            <div v-if="selectedReviewers.length > 0" class="flex flex-wrap gap-1.5 mb-2">
              <span
                v-for="r in selectedReviewers"
                :key="r.email"
                class="inline-flex items-center gap-1 px-2 py-1 bg-blue-600/20 text-blue-300 text-xs rounded-lg"
              >
                {{ r.name || r.email }}
                <button @click="removeReviewer(r.email)" class="hover:text-white">
                  <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </span>
            </div>
            <!-- Member picker -->
            <div class="relative">
              <input
                v-model="reviewerSearch"
                type="text"
                placeholder="Search members..."
                class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500"
                @focus="showReviewerDropdown = true"
              />
              <div
                v-if="showReviewerDropdown && filteredMembers.length > 0"
                class="absolute z-10 mt-1 w-full bg-slate-800 border border-slate-700 rounded-lg shadow-xl max-h-40 overflow-y-auto"
              >
                <button
                  v-for="member in filteredMembers"
                  :key="member.email"
                  @click="addReviewer(member)"
                  class="w-full px-3 py-2 text-left text-sm hover:bg-slate-700 transition-colors flex items-center justify-between"
                >
                  <div>
                    <div class="text-slate-200">{{ member.name }}</div>
                    <div class="text-[10px] text-slate-500">{{ member.email }}</div>
                  </div>
                  <span class="text-[10px] px-1.5 py-0.5 rounded-full"
                    :class="member.role === 'admin' ? 'bg-purple-500/20 text-purple-300' : member.role === 'manager' ? 'bg-blue-500/20 text-blue-300' : 'bg-slate-500/20 text-slate-400'">
                    {{ member.role }}
                  </span>
                </button>
              </div>
            </div>
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">Version note <span class="text-red-400">*</span></label>
            <textarea
              v-model="reviewMessage"
              rows="3"
              placeholder="e.g. Updated risk assessment criteria. Added data classification table."
              class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500 resize-none"
            />
            <div class="text-[10px] text-slate-600 mt-1">This becomes part of the document's permanent version history after approval.</div>
          </div>

          <div v-if="reviewError" class="text-xs text-red-400">{{ reviewError }}</div>

          <div class="flex gap-2 pt-2">
            <button
              @click="sendForReview"
              :disabled="reviewSending || selectedReviewers.length === 0 || !reviewMessage.trim()"
              class="flex-1 px-4 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50"
            >
              {{ reviewSending ? 'Sending...' : `Send to ${selectedReviewers.length} reviewer${selectedReviewers.length !== 1 ? 's' : ''}` }}
            </button>
            <button @click="showReviewModal = false" class="px-4 py-2.5 text-sm text-slate-400 hover:text-white">
              Cancel
            </button>
          </div>
        </div>
      </div>
      </Transition>
    </Teleport>

    <!-- New Document Modal -->
    <Teleport to="body">
      <Transition name="modal">
      <div v-if="showNewDocModal" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="absolute inset-0 bg-black/60" @click="showNewDocModal = false" />
        <form @submit.prevent="createDocument" class="relative bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 max-w-lg w-full mx-4 space-y-4">
          <h2 class="text-lg font-semibold text-slate-100">New Document</h2>

          <div class="space-y-4">
            <!-- Title — primary field, always first -->
            <div>
              <label class="block text-xs text-slate-500 mb-1">Title <span class="text-red-400">*</span></label>
              <input v-model="newDoc.title" @input="autoSlug" type="text" autofocus
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2.5 text-sm text-white focus:outline-none focus:border-blue-500"
                placeholder="e.g. Data Classification Policy" />
            </div>
            <!-- Type -->
            <div>
              <label class="block text-xs text-slate-500 mb-1">Type</label>
              <select v-model="newDoc.type"
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2.5 text-sm text-white focus:outline-none focus:border-blue-500">
                <option value="">Document</option>
                <option value="policy">Policy</option>
                <option value="procedure">Procedure</option>
                <option value="control">Control</option>
                <option value="guideline">Guideline</option>
                <option value="record">Record</option>
              </select>
            </div>
            <!-- Location — compact, sensible default, not the focus -->
            <div class="flex items-center gap-2 text-xs">
              <svg class="w-3.5 h-3.5 text-slate-600 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z" />
              </svg>
              <span v-if="newDoc.folder && !showFolderPicker" class="text-slate-400">{{ newDoc.folder }}</span>
              <span v-if="!newDoc.folder && !showFolderPicker" class="text-slate-500 italic">No folder selected</span>
              <button v-if="!showFolderPicker" @click="showFolderPicker = true" class="text-blue-400 hover:text-blue-300 ml-auto text-[10px]">
                {{ newDoc.folder ? 'change' : 'pick folder' }}
              </button>
              <div v-if="showFolderPicker" class="flex-1">
                <input v-model="newDoc.folder" type="text" list="folder-list"
                  class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-blue-500"
                  placeholder="Type or select folder" @blur="showFolderPicker = false" />
                <datalist id="folder-list">
                  <option v-for="f in allFolderPaths" :key="f" :value="f" />
                </datalist>
              </div>
            </div>
            <!-- Advanced (collapsed by default) -->
            <button @click="showNewDocAdvanced = !showNewDocAdvanced" class="text-[10px] text-slate-600 hover:text-slate-400 transition-colors">
              {{ showNewDocAdvanced ? 'Hide' : 'Show' }} advanced options
            </button>
            <div v-if="showNewDocAdvanced" class="grid grid-cols-2 gap-3 pt-1">
              <div>
                <label class="block text-[10px] text-slate-600 mb-1">Document ID</label>
                <input v-model="newDoc.document_id" @input="slugManuallyEdited = true" type="text"
                  class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white font-mono focus:outline-none focus:border-blue-500"
                  placeholder="auto from title" />
                <div v-if="newDoc.document_id && docIdExists" class="text-[10px] text-red-400 mt-1">This ID already exists</div>
              </div>
              <div>
                <label class="block text-[10px] text-slate-600 mb-1">Filename</label>
                <input v-model="newDoc.filename" type="text"
                  class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white font-mono focus:outline-none focus:border-blue-500"
                  placeholder="auto from ID" />
              </div>
              <div class="col-span-2">
                <label class="block text-[10px] text-slate-600 mb-1">Author</label>
                <MemberPicker :modelValue="newDoc.author" :members="allUsers" placeholder="Select author..." @update:modelValue="v => newDoc.author = v" />
              </div>
            </div>
          </div>

          <div v-if="newDocError" class="text-xs text-red-400">{{ newDocError }}</div>

          <div class="flex gap-2 pt-2">
            <button type="submit" :disabled="newDocSaving || !newDoc.title || !newDoc.document_id || !newDoc.folder || docIdExists"
              class="flex-1 px-4 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50">
              {{ newDocSaving ? 'Creating...' : 'Create Document' }}
            </button>
            <button type="button" @click="showNewDocModal = false" class="px-4 py-2.5 text-sm text-slate-400 hover:text-white">Cancel</button>
          </div>
        </form>
      </div>
      </Transition>
    </Teleport>

    <!-- Template picker modal -->
    <Teleport to="body">
      <Transition name="modal">
      <div v-if="showTemplatePicker" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="absolute inset-0 bg-black/60" @click="showTemplatePicker = false" />
        <div class="relative bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 max-w-lg w-full mx-4">
          <h2 class="text-lg font-semibold text-slate-100 mb-1">Import template</h2>
          <p class="text-xs text-slate-500 mb-4">Choose a standard to scaffold your document structure.</p>

          <input v-model="templateSearch" type="text" autofocus
            class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500 mb-3"
            placeholder="Search templates..." />

          <div v-if="filteredTemplates.length === 0" class="text-center py-6 text-xs text-slate-600">
            No templates found.
          </div>
          <div v-else class="space-y-1.5 max-h-72 overflow-y-auto">
            <button v-for="tmpl in filteredTemplates" :key="tmpl.id"
              @click="addTemplate(tmpl.id); showTemplatePicker = false"
              :disabled="templateLoading === tmpl.id"
              class="w-full text-left flex items-center gap-3 px-3 py-2.5 hover:bg-slate-800 rounded-lg transition-colors group">
              <div class="w-8 h-8 rounded-lg bg-blue-600/15 flex items-center justify-center flex-shrink-0">
                <svg class="w-4 h-4 text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />
                </svg>
              </div>
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium text-slate-200 group-hover:text-white">{{ tmpl.name }}</div>
                <div v-if="tmpl.description" class="text-[10px] text-slate-500 mt-0.5">{{ tmpl.description }}</div>
              </div>
              <div class="flex-shrink-0">
                <svg v-if="templateLoading === tmpl.id" class="w-4 h-4 text-blue-400 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                <span v-else class="text-xs text-blue-400 group-hover:text-blue-300 font-medium">Import</span>
              </div>
            </button>
          </div>

          <div v-if="templateMessage" class="mt-3 text-[11px] px-3 py-2 rounded-lg"
            :class="templateMessageError ? 'bg-red-500/10 text-red-400' : 'bg-emerald-500/10 text-emerald-400'">
            {{ templateMessage }}
          </div>

          <div class="flex justify-end mt-4">
            <button @click="showTemplatePicker = false" class="text-sm text-slate-400 hover:text-white">Cancel</button>
          </div>
        </div>
      </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup>
import { ref, reactive, computed, watch, onMounted, nextTick, onBeforeUnmount, defineAsyncComponent } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { marked } from 'marked'
marked.setOptions({ breaks: true })
import DOMPurify from 'dompurify'
const sanitize = (html) => DOMPurify.sanitize(html, { ADD_ATTR: ['style', 'data-ref-type', 'data-ref-id'] })

// Render [[TYPE:ID|Title]] reference links as styled pills
const refTypeColors = {
  RISK: 'bg-red-900/40 text-red-300 border-red-800/50',
  LEGAL: 'bg-purple-900/40 text-purple-300 border-purple-800/50',
  DOC: 'bg-blue-900/40 text-blue-300 border-blue-800/50',
  ASSET: 'bg-amber-900/40 text-amber-300 border-amber-800/50',
  SUPPLIER: 'bg-emerald-900/40 text-emerald-300 border-emerald-800/50',
  SYSTEM: 'bg-cyan-900/40 text-cyan-300 border-cyan-800/50',
  INCIDENT: 'bg-orange-900/40 text-orange-300 border-orange-800/50',
  CA: 'bg-pink-900/40 text-pink-300 border-pink-800/50',
}
const refTypeRoutes = {
  RISK: 'risks',
  LEGAL: 'legal',
  DOC: 'documents',
  ASSET: 'assets',
  SUPPLIER: 'suppliers',
  SYSTEM: 'systems',
  INCIDENT: 'incidents',
  CA: 'corrective-actions',
}
function renderRefLinks(html) {
  if (!html) return html
  return html.replace(/\[\[(\w+):([^|]+)\|([^\]]+)\]\]/g, (match, type, id, title) => {
    const colors = refTypeColors[type] || 'bg-slate-800 text-slate-300 border-slate-700'
    const routeBase = refTypeRoutes[type] || ''
    let href = '#'
    if (type === 'DOC') {
      href = orgPath(`/documents/${id}`)
    } else if (routeBase) {
      href = orgPath(`/${routeBase}`)
    }
    return `<a href="${href}" data-ref-type="${type}" data-ref-id="${id}" class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded border text-[11px] font-medium no-underline cursor-pointer ${colors}" title="${type}: ${id}">`
      + `<span class="opacity-60">${type}</span>`
      + `<span>${title}</span>`
      + `</a>`
  })
}
import { api, getCurrentUser } from '../api'
import { useToast } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'
import { useModalEscape } from '../composables/useModalEscape'
import { useDocumentTree } from '../composables/useDocumentTree'
import { useDocumentEditor } from '../composables/useDocumentEditor'
import { useDocumentComments } from '../composables/useDocumentComments'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import StatusBadge from '../components/StatusBadge.vue'
import MemberPicker from '../components/MemberPicker.vue'
import DiffView from '../components/DiffView.vue'
import DocTreeNode from '../components/DocTreeNode.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
import CommentsPanel from '../components/CommentsPanel.vue'
import HistoryPanel from '../components/HistoryPanel.vue'
const DocumentEditor = defineAsyncComponent(() => import('../components/DocumentEditor.vue'))

const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()
const { error: toastError } = useToast()

const {
  folders,
  fileTree,
  expandedNodes,
  activeFolder,
  showTreePanel,
  loadingTree,
  toggleTreeNode,
  expandPathToFile,
  formatFolderName,
  formatFileTitle,
  formatFileName,
  findFileInFolder,
  allFolderPaths,
  loadTree,
} = useDocumentTree(api)

// --- @mention autocomplete ---
const allUsers = ref([])
const mentionQuery = ref('')
const showMentionDropdown = ref(false)
const mentionAnchor = ref(null)
const mentionSelectedIndex = ref(0)
const mentionContext = ref('') // 'inline' or 'panel'

const mentionUsers = computed(() => {
  if (!mentionQuery.value) return allUsers.value.slice(0, 5)
  const q = mentionQuery.value.toLowerCase()
  return allUsers.value.filter(u =>
    (u.name || '').toLowerCase().includes(q) || (u.email || '').toLowerCase().includes(q)
  ).slice(0, 5)
})

function handleCommentInput(e, context) {
  const textarea = e.target
  const text = textarea.value
  const pos = textarea.selectionStart
  const before = text.substring(0, pos)
  const atIndex = before.lastIndexOf('@')
  if (atIndex >= 0 && (atIndex === 0 || before[atIndex - 1] === ' ' || before[atIndex - 1] === '\n')) {
    const query = before.substring(atIndex + 1)
    if (!/\s/.test(query) || query.split(/\s/).length <= 2) {
      mentionQuery.value = query
      showMentionDropdown.value = true
      mentionAnchor.value = textarea
      mentionContext.value = context || 'inline'
      mentionSelectedIndex.value = 0
      return
    }
  }
  showMentionDropdown.value = false
}

function handleCommentKeydown(e) {
  if (!showMentionDropdown.value) return
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    mentionSelectedIndex.value = Math.min(mentionSelectedIndex.value + 1, mentionUsers.value.length - 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    mentionSelectedIndex.value = Math.max(mentionSelectedIndex.value - 1, 0)
  } else if (e.key === 'Enter' && !e.metaKey && !e.ctrlKey) {
    if (mentionUsers.value.length > 0) {
      e.preventDefault()
      selectMention(mentionUsers.value[mentionSelectedIndex.value])
    }
  } else if (e.key === 'Tab') {
    if (mentionUsers.value.length > 0) {
      e.preventDefault()
      selectMention(mentionUsers.value[0])
    }
  } else if (e.key === 'Escape') {
    showMentionDropdown.value = false
  }
}

function selectMention(user) {
  const textarea = mentionAnchor.value
  if (!textarea) return
  const text = textarea.value
  const pos = textarea.selectionStart
  const before = text.substring(0, pos)
  const atIndex = before.lastIndexOf('@')
  const after = text.substring(pos)
  const newText = before.substring(0, atIndex) + '@' + user.name + ' ' + after
  const newPos = atIndex + 1 + user.name.length + 1

  // Update the correct reactive model based on context
  if (mentionContext.value === 'panel') {
    newComment.value = newText
  } else {
    inlineCommentText.value = newText
  }

  showMentionDropdown.value = false
  nextTick(() => {
    textarea.selectionStart = newPos
    textarea.selectionEnd = newPos
    textarea.focus()
  })
}

function closeMentionOnClickOutside(e) {
  if (showMentionDropdown.value && mentionAnchor.value && !mentionAnchor.value.contains(e.target)) {
    showMentionDropdown.value = false
  }
  if (showReviewerDropdown.value && !e.target.closest('.relative')) {
    showReviewerDropdown.value = false
  }
}

// --- Render @mentions in comment body ---
function renderCommentBody(body) {
  if (!body) return ''
  const escaped = body.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  return escaped.replace(/@(\w+(?:\s\w+)?)/g, '<span class="text-blue-400 font-medium">@$1</span>')
}

// --- Resolved comment expand/collapse ---
const expandedResolvedInline = reactive(new Set())
const showResolvedPanel = ref(false)

function toggleResolvedInline(commentId) {
  if (expandedResolvedInline.has(commentId)) {
    expandedResolvedInline.delete(commentId)
  } else {
    expandedResolvedInline.add(commentId)
  }
}

function firstLine(text) {
  if (!text) return ''
  const line = text.split('\n')[0]
  return line.length > 80 ? line.substring(0, 80) + '...' : line
}

// Search
const searchQuery = ref('')
const searchResults = ref([])
const searchLoading = ref(false)
const isSearching = computed(() => searchQuery.value && searchQuery.value.length >= 2)
let searchTimer = null

function onSearchInput() {
  clearTimeout(searchTimer)
  if (!searchQuery.value || searchQuery.value.length < 2) {
    searchResults.value = []
    return
  }
  searchLoading.value = true
  searchTimer = setTimeout(async () => {
    try {
      searchResults.value = await api.searchDocuments(searchQuery.value) || []
    } catch {
      searchResults.value = []
    }
    searchLoading.value = false
  }, 250)
}

function clearSearch() {
  searchQuery.value = ''
  searchResults.value = []
}

function openSearchResult(result) {
  // Don't clear search — user may want to browse multiple results
  if (result.folder && result.document_id) {
    const file = findFileInFolder(result.folder, result.document_id)
    if (file) {
      expandPathToFile(file.path)
      selectItem(result.folder, result.document_id, file)
    }
  }
}

// Breadcrumb for active file
const activeBreadcrumb = computed(() => {
  if (!activeDoc.value || !activeDoc.value.path) return []
  const parts = activeDoc.value.path.split('/')
  // Replace last segment (filename) with document_id
  const crumbs = parts.slice(0, -1)
  if (activeId.value) crumbs.push(activeId.value)
  return crumbs
})

// Check if a file needs review (has review_cycle or non-draft status)
function fileNeedsReview(file) {
  if (!file) return false
  if (file.status === 'in_review') return true
  // Check if document is in needs-review list (overdue or changed since approval)
  if (allNeedsReviewDocs.value.some(d => d.document_id === file.document_id)) return true
  return false
}

// Format directory slug for display: "a.5-organizational" → "A.5 Organizational"
// "4-context-of-the-organisation" → "4 Context of the Organisation"
function formatDirName(slug) {
  if (!slug) return ''
  // Split on first hyphen: prefix (number/id) and rest (title words)
  const match = slug.match(/^([a-z0-9.]+)-(.+)$/)
  if (!match) return slug.charAt(0).toUpperCase() + slug.slice(1)
  const prefix = match[1].toUpperCase()
  const title = match[2].split('-').map(w => w.charAt(0).toUpperCase() + w.slice(1)).join(' ')
  return `${prefix} ${title}`
}

// --- Active document ---
const activeId = ref(null)
const activeDoc = ref(null)
const activeType = ref(null)
const rawContent = ref('')
const loadingContent = ref(false)

// --- Center-pane tabs (Content default; surfaces metadata, versions, links, discussion, history) ---
const docTab = ref('content')
const docTabs = computed(() => [
  { key: 'content', label: 'Content' },
  { key: 'info', label: 'Info' },
  { key: 'versions', label: 'Versions' },
  { key: 'links', label: 'Links' },
  { key: 'discussion', label: 'Discussion' },
  { key: 'history', label: 'History' },
])
async function switchDocTab(key) {
  docTab.value = key
  // Lazy-load version history when entering Versions tab
  if (key === 'versions' && versions.value.length === 0 && activeId.value) {
    loadingVersions.value = true
    try {
      versions.value = await api.getDocVersions(activeId.value) || []
    } catch {
      versions.value = []
    } finally {
      loadingVersions.value = false
    }
  }
}

// Document metadata editing (admin/manager only)
const currentUserRole = ref('')
const currentUserEmail = ref('')
const canEditMetadata = computed(() => currentUserRole.value === 'admin' || currentUserRole.value === 'manager')

// Dirty state — document has changes since last approval
const activeDocReviewInfo = computed(() => {
  if (!activeId.value) return null
  return allNeedsReviewDocs.value.find(d => d.document_id === activeId.value) || null
})
const isDocDirty = computed(() => activeDocReviewInfo.value !== null)
const isDocNeverApproved = computed(() => activeDocReviewInfo.value?.never_approved === true)
const approvedCommit = computed(() => activeDocReviewInfo.value?.approved_commit || null)
const approvedAt = computed(() => activeDocReviewInfo.value?.approved_at || null)
const approvedVersion = computed(() => activeDocReviewInfo.value?.approved_version || null)

const docNextReview = computed(() => {
  if (!activeDoc.value || !approvedAt.value) return null
  const cycle = activeDoc.value.review_cycle || 12
  const approved = new Date(approvedAt.value)
  if (isNaN(approved.getTime())) return null
  const next = new Date(approved)
  next.setMonth(next.getMonth() + cycle)
  return next.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
})

const docReviewOverdue = computed(() => {
  if (!docNextReview.value || !approvedAt.value) return false
  const cycle = activeDoc.value?.review_cycle || 12
  const approved = new Date(approvedAt.value)
  const next = new Date(approved)
  next.setMonth(next.getMonth() + cycle)
  return next < new Date()
})

const docReviewDueSoon = computed(() => {
  if (!docNextReview.value || !approvedAt.value || docReviewOverdue.value) return false
  const cycle = activeDoc.value?.review_cycle || 12
  const approved = new Date(approvedAt.value)
  const next = new Date(approved)
  next.setMonth(next.getMonth() + cycle)
  const daysUntil = (next - new Date()) / (1000 * 60 * 60 * 24)
  return daysUntil <= 30
})

// --- Document editing (composable) ---
const {
  editMode,
  editContent,
  savingEdit,
  editVersion,
  editAuthor,
  editOwner,
  editSaveMsg,
  editSaveError,
  draftKey,
  saveDraft,
  loadDraft,
  clearDraft,
  startAutosave,
  stopAutosave,
  hasDraft,
  draftSavedAt,
  startEdit,
  cancelEdit,
  saveEdit,
  discardDraft,
} = useDocumentEditor({ activeId, activeDoc, activeType, rawContent, api, loadContent, loadChangedDocs, loadNeedsReview })

function resolveUserName(email) {
  if (!email) return null
  const user = allUsers.value.find(u => u.email === email)
  return user?.name || email
}

// Block-level raw HTML / SVG the WYSIWYG editor can't represent — it would be
// silently dropped on save. Tables are excluded (they round-trip as HTML).
function hasEditorUnsafeHtml(md) {
  if (!md) return false
  // <div> is intentionally excluded: ProseMirror extracts its text content, so
  // only the wrapper is lost (structural) — not the complete data loss of
  // svg/canvas/iframe etc., which have no node type and vanish entirely.
  return /<\s*(svg|iframe|object|embed|canvas|video|audio|form|style|script|figure|details|section|article|aside|nav)\b/i.test(md)
}

// Guard the editor against silent data loss: if the document has embedded
// HTML/SVG the WYSIWYG editor can't preserve, confirm before entering edit mode
// (one save would drop it). The user must explicitly opt in — never silent.
async function startEditGuarded() {
  if (hasEditorUnsafeHtml(rawContent.value)) {
    const { ask } = useConfirm()
    const ok = await ask(
      "This document contains embedded HTML/SVG the editor can't preserve — editing here will drop it on save. Edit the source via the CLI to keep it. Edit anyway?",
      'Embedded HTML will be lost',
    )
    if (!ok) return
  }
  startEdit()
}

// View approved version — show diff between current and approved commit
const showApprovedDiff = ref(false)
const approvedDiffLines = ref([])
const loadingApprovedDiff = ref(false)

async function viewApprovedVersion() {
  if (!activeId.value || !approvedCommit.value || approvedCommit.value === 'never') return
  showApprovedDiff.value = !showApprovedDiff.value
  if (!showApprovedDiff.value) return

  loadingApprovedDiff.value = true
  try {
    const diff = await api.getDocDiff(activeId.value, approvedCommit.value, 'HEAD')
    const diffText = typeof diff === 'string' ? diff : (diff?.diff || '')
    approvedDiffLines.value = diffText.split('\n')
  } catch {
    approvedDiffLines.value = ['Failed to load diff']
  } finally {
    loadingApprovedDiff.value = false
  }
}

function setDocAuthor(email) {
  if (editMode.value) {
    // Buffer during edit — saved with content on Save
    editAuthor.value = email
    if (activeDoc.value) activeDoc.value.author = email
  } else {
    // Outside edit mode — save immediately
    if (!activeId.value) return
    api.updateDocumentMetadata(activeId.value, { author: email })
      .then(() => { if (activeDoc.value) activeDoc.value.author = email })
      .catch(e => { console.error('Failed to set author:', e); toastError('Failed to set author: ' + (e.message || 'unknown error')) })
  }
}

function setDocType(type) {
  if (!activeId.value) return
  api.updateDocumentMetadata(activeId.value, { type })
    .then(() => { if (activeDoc.value) activeDoc.value.type = type })
    .catch(e => { console.error('Failed to set type:', e); toastError('Failed to set type: ' + (e.message || 'unknown error')) })
}

async function saveVersion() {
  const v = editVersion.value.trim()
  if (!v || !activeId.value || v === activeDoc.value?.version) return
  try {
    await api.updateDocumentMetadata(activeId.value, { version: v })
    if (activeDoc.value) activeDoc.value.version = v
  } catch (e) {
    console.error('Failed to set version:', e)
    toastError('Failed to set version: ' + (e.message || 'unknown error'))
  }
}

// Review modal
// --- Folder context menu ---
const folderMenu = ref({ show: false, x: 0, y: 0, path: '' })

function openFolderMenu(event, folderPath) {
  folderMenu.value = { show: true, x: event.clientX, y: event.clientY, path: folderPath }
}

function newDocInFolder(folderPath) {
  folderMenu.value.show = false
  newDoc.value = { title: '', document_id: '', folder: folderPath, filename: '', author: currentUserEmail.value, template: '' }
  slugManuallyEdited.value = false
  newDocError.value = ''
  showNewDocAdvanced.value = false
  showFolderPicker.value = false
  showNewDocModal.value = true
}

// --- Root new folder ---
const rootNewFolder = ref({ active: false, name: '' })

async function startRootNewFolder() {
  rootNewFolder.value = { active: true, name: '' }
  await nextTick()
  const input = document.querySelector('.root-folder-input')
  if (input) input.focus()
}

async function confirmRootNewFolder() {
  const name = rootNewFolder.value.name.trim()
  if (!name) { rootNewFolder.value.active = false; return }
  const slug = name.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/-+/g, '-')
  rootNewFolder.value.active = false
  try {
    await api.createFolder(slug, name)
    await loadTree()
    expandedNodes.add(slug)
  } catch (e) {
    const { error: showError } = useToast()
    showError('Failed to create folder: ' + e.message)
  }
}

// --- Inline new folder ---
const inlineNewFolder = ref({ active: false, parentPath: '', name: '' })

function startInlineNewFolder(parentPath) {
  folderMenu.value.show = false
  expandedNodes.add(parentPath)
  inlineNewFolder.value = { active: true, parentPath, name: '' }
  nextTick(() => {
    const input = document.querySelector('.inline-folder-input')
    if (input) input.focus()
  })
}

async function confirmInlineNewFolder() {
  const name = inlineNewFolder.value.name.trim()
  if (!name) { cancelInlineNewFolder(); return }
  const slug = name.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/-+/g, '-')
  const folderPath = inlineNewFolder.value.parentPath + '/' + slug
  cancelInlineNewFolder()
  try {
    await api.createFolder(folderPath, name)
    // Reload tree and expand the new folder
    await loadTree()
    expandedNodes.add(folderPath)
  } catch (e) {
    const { error: showError } = useToast()
    showError('Failed to create folder: ' + e.message)
  }
}

function cancelInlineNewFolder() {
  inlineNewFolder.value = { active: false, parentPath: '', name: '' }
}

// --- New Document ---
const showNewDocModal = ref(false)
const newDoc = ref({ title: '', document_id: '', folder: '', filename: '', author: '', type: '', template: '' })
const newDocSaving = ref(false)
const newDocError = ref('')
const showNewDocAdvanced = ref(false)
const showFolderPicker = ref(false)

const docIdExists = computed(() => {
  if (!newDoc.value.document_id) return false
  // Check against all loaded documents
  for (const folder of folders.value) {
    function check(f) {
      for (const file of (f.files || [])) {
        if (file.document_id === newDoc.value.document_id) return true
      }
      for (const sub of (f.subfolders || [])) {
        if (check(sub)) return true
      }
      return false
    }
    if (check(folder)) return true
  }
  return false
})

const slugManuallyEdited = ref(false)

function autoSlug() {
  if (slugManuallyEdited.value) return
  const slug = newDoc.value.title
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
  newDoc.value.document_id = slug
  newDoc.value.filename = slug + '.md'
}

function openNewDocModal() {
  newDoc.value = { title: '', document_id: '', folder: activeFolder.value || (allFolderPaths.value?.[0] || ''), filename: '', author: currentUserEmail.value, type: '', template: '' }
  showFolderPicker.value = false
  slugManuallyEdited.value = false
  newDocError.value = ''
  showNewDocModal.value = true
}

async function createDocument() {
  if (newDocSaving.value) return
  newDocSaving.value = true
  newDocError.value = ''
  try {
    const payload = {
      folder: newDoc.value.folder,
      filename: newDoc.value.filename || newDoc.value.document_id + '.md',
      document_id: newDoc.value.document_id,
      title: newDoc.value.title,
      type: newDoc.value.type || '',
      author: newDoc.value.author,
    }
    const result = await api.createDocument(payload)
    showNewDocModal.value = false
    // Reload tree and navigate to new document
    await loadTree()
    for (const f of folders.value) expandedNodes.add(f.name)
    // Navigate to the new document
    router.push(orgPath(`/documents/${encodeURIComponent(result.document_id)}`))
  } catch (e) {
    newDocError.value = e.message || 'Failed to create document'
  } finally {
    newDocSaving.value = false
  }
}

function printDocument() {
  window.print()
}

async function deleteCurrentDocument() {
  if (!activeId.value) return
  const { ask } = useConfirm()
  if (!await ask(`Delete "${activeDoc.value?.title || activeId.value}"? This cannot be undone.`, 'Delete Document')) return
  try {
    await api.deleteDocument(activeId.value)
    activeDoc.value = null
    activeId.value = null
    // Reload tree
    await loadTree()
    router.push(orgPath('/documents'))
  } catch (e) {
    const { error: showError } = useToast()
    showError('Failed to delete: ' + e.message)
  }
}

const showReviewModal = ref(false)
const reviewMessage = ref('')
const reviewSending = ref(false)
const reviewError = ref('')
const selectedReviewers = ref([])
const reviewerSearch = ref('')
const showReviewerDropdown = ref(false)

const filteredMembers = computed(() => {
  const search = reviewerSearch.value.toLowerCase()
  const selectedEmails = new Set(selectedReviewers.value.map(r => r.email))
  const currentEmail = getCurrentUser()
  return allUsers.value.filter(u =>
    u.email !== currentEmail &&
    !selectedEmails.has(u.email) &&
    (u.name?.toLowerCase().includes(search) || u.email?.toLowerCase().includes(search))
  )
})

function addReviewer(member) {
  if (!selectedReviewers.value.find(r => r.email === member.email)) {
    selectedReviewers.value.push({ email: member.email, name: member.name, role: member.role })
  }
  reviewerSearch.value = ''
  showReviewerDropdown.value = false
}

function removeReviewer(email) {
  selectedReviewers.value = selectedReviewers.value.filter(r => r.email !== email)
}

async function sendForReview() {
  if (selectedReviewers.value.length === 0 || !activeId.value) return
  reviewSending.value = true
  reviewError.value = ''
  try {
    const reviewers = selectedReviewers.value.map(r => r.email)
    await api.postJSON(`/api/v1/documents/${encodeURIComponent(activeId.value)}/reviews`, {
      reviewers,
      message: reviewMessage.value,
    })
    showReviewModal.value = false
    selectedReviewers.value = []
    reviewMessage.value = ''
    // Reload doc to show updated status/version
    if (activeType.value && activeId.value) {
      loadContent(activeType.value, activeId.value)
    }
  } catch (e) {
    reviewError.value = e.message || 'Failed to send for review'
  } finally {
    reviewSending.value = false
  }
}

function openChangedDoc(doc) {
  if (doc.folder && doc.document_id) {
    const file = findFileInFolder(doc.folder, doc.document_id)
    if (file) {
      expandPathToFile(file.path)
      selectItem(doc.folder, doc.document_id, file)
    }
  }
}

// Changed documents
const changedDocs = ref([])
async function loadChangedDocs() {
  try {
    changedDocs.value = await api.getChangedDocuments(20) || []
  } catch {
    changedDocs.value = []
  }
}

// Needs Review
const docDashTab = ref('review')
const allNeedsReviewDocs = ref([])
const includeNeverApproved = ref(false)
const loadingNeedsReview = ref(false)
const needsReviewDiffDoc = ref(null)
const needsReviewDiffLines = ref([])

const needsReviewDocs = computed(() => {
  const docs = allNeedsReviewDocs.value
  if (includeNeverApproved.value) return docs
  return docs.filter(d => !d.never_approved)
})

const needsReviewChangedCount = computed(() => {
  return allNeedsReviewDocs.value.filter(d => !d.never_approved).length
})

async function loadNeedsReview() {
  loadingNeedsReview.value = true
  try {
    allNeedsReviewDocs.value = await api.getNeedsReview() || []
  } catch {
    allNeedsReviewDocs.value = []
  } finally {
    loadingNeedsReview.value = false
  }
}

function openNeedsReviewDoc(doc) {
  showNeedsReview.value = false
  if (doc.folder && doc.document_id) {
    const file = findFileInFolder(doc.folder, doc.document_id)
    if (file) {
      expandPathToFile(file.path)
      selectItem(doc.folder, doc.document_id, file)
      // Open review modal after short delay to let content load
      setTimeout(() => { showReviewModal.value = true }, 300)
    }
  }
}

async function viewNeedsReviewDiff(doc) {
  if (needsReviewDiffDoc.value === doc.document_id) {
    // Toggle off
    needsReviewDiffDoc.value = null
    needsReviewDiffLines.value = []
    return
  }
  needsReviewDiffDoc.value = doc.document_id
  needsReviewDiffLines.value = []
  try {
    const diff = await api.getDocDiff(doc.document_id, doc.approved_commit, 'HEAD')
    if (diff && diff.diff) {
      needsReviewDiffLines.value = diff.diff.split('\n')
    } else if (typeof diff === 'string') {
      needsReviewDiffLines.value = diff.split('\n')
    }
  } catch {
    needsReviewDiffLines.value = ['Failed to load diff']
  }
}

// --- Template management ---
const templateLoading = ref(null)
const templateMessage = ref('')
const templateMessageError = ref(false)
const showTemplatePicker = ref(false)
const showNewMenu = ref(false)
const templateSearch = ref('')
const filteredTemplates = computed(() => {
  if (!templateSearch.value) return availableTemplates.value
  const q = templateSearch.value.toLowerCase()
  return availableTemplates.value.filter(t => t.name.toLowerCase().includes(q) || (t.description || '').toLowerCase().includes(q))
})
const showRemoveConfirm = ref(false)
const removeTarget = ref(null)

const availableTemplates = ref([])

const installedTemplates = computed(() => {
  return folders.value.map(f => f.name)
})

const templateList = computed(() => {
  return availableTemplates.value.map(t => ({
    id: t.id,
    label: t.name,
    description: t.description,
    installed: installedTemplates.value.includes(t.id),
  }))
})

// Load available templates from API
async function loadAvailableTemplates() {
  try {
    const data = await api.fetchJSON('/api/v1/templates/available')
    availableTemplates.value = Array.isArray(data) ? data : (data?.data || [])
  } catch { /* ignore */ }
}

function navigateToTemplate(id) {
  // Switch to tree tab and expand the template folder
  activeTab.value = 'tree'
  expandedNodes.add(id)
  activeFolder.value = id
}

async function addTemplate(id) {
  templateLoading.value = id
  templateMessage.value = ''
  try {
    await api.addTemplate(id)
    const tmpl = availableTemplates.value.find(t => t.id === id)
    templateMessage.value = `${tmpl?.name || id} template added successfully.`
    templateMessageError.value = false
    // Reload the document tree
    await loadTree()
    for (const f of folders.value) {
      expandedNodes.add(f.name)
    }
  } catch (e) {
    templateMessage.value = `Failed to add template: ${e.message}`
    templateMessageError.value = true
  } finally {
    templateLoading.value = null
  }
}

function confirmRemoveTemplate(fw) {
  removeTarget.value = fw
  showRemoveConfirm.value = true
}

async function removeTemplate(id) {
  templateLoading.value = id
  templateMessage.value = ''
  showRemoveConfirm.value = false
  try {
    await api.removeTemplate(id)
    const tmpl = availableTemplates.value.find(t => t.id === id)
    templateMessage.value = `${tmpl?.name || id} template removed.`
    templateMessageError.value = false
    // Reload the document tree
    await loadTree()
  } catch (e) {
    templateMessage.value = `Failed to remove template: ${e.message}`
    templateMessageError.value = true
  } finally {
    templateLoading.value = null
    removeTarget.value = null
  }
}

// --- Paragraph-level content blocks ---
const contentBlocks = computed(() => {
  if (!rawContent.value) return []
  const html = marked.parse(rawContent.value)
  const div = document.createElement('div')
  div.innerHTML = html
  const blocks = []

  function addBlock(html, tag, text) {
    blocks.push({ index: blocks.length, html, tag, text })
  }

  for (const child of div.children) {
    const tag = child.tagName.toLowerCase()

    // Split lists — each bullet is commentable
    if ((tag === 'ul' || tag === 'ol') && child.children.length > 0) {
      for (const li of child.children) {
        if (li.tagName.toLowerCase() === 'li') {
          addBlock('<' + tag + '>' + li.outerHTML + '</' + tag + '>', 'li', li.textContent || '')
        }
      }
    }
    // Tables — convert to grid-based rows so each gets a "+" button
    else if (tag === 'table') {
      const thead = child.querySelector('thead')
      const tbody = child.querySelector('tbody')
      const ths = thead ? Array.from(thead.querySelectorAll('th')) : []
      const rows = tbody ? Array.from(tbody.querySelectorAll('tr')) : []
      const colCount = ths.length || (rows[0] ? rows[0].children.length : 1)
      const gridCols = 'grid-template-columns: ' + Array(colCount).fill('1fr').join(' ') + ';'

      // Header as one block
      if (ths.length > 0) {
        const headerCells = ths.map(th => {
          const styleAttr = th.getAttribute('style') || ''
          return `<div class="tbl-hdr-cell" style="${styleAttr}">${th.innerHTML}</div>`
        }).join('')
        addBlock(`<div class="tbl-grid" style="${gridCols}">${headerCells}</div>`, 'thead', thead.textContent || '')
      }

      // Each body row as separate block — gets its own "+" button!
      for (const tr of rows) {
        const tds = Array.from(tr.querySelectorAll('td'))
        const cells = tds.map(td => {
          const styleAttr = td.getAttribute('style') || ''
          return `<div class="tbl-cell" style="${styleAttr}">${td.innerHTML}</div>`
        }).join('')
        addBlock(`<div class="tbl-grid tbl-row" style="${gridCols}">${cells}</div>`, 'tr', tr.textContent || '')
      }
    }
    // Split blockquotes — each paragraph inside is commentable
    else if (tag === 'blockquote' && child.children.length > 1) {
      for (const bqChild of child.children) {
        addBlock('<blockquote>' + bqChild.outerHTML + '</blockquote>', 'blockquote-p', bqChild.textContent || '')
      }
    }
    // Everything else — headings, paragraphs, pre, hr — one block each
    else {
      addBlock(child.outerHTML, tag, child.textContent || '')
    }
  }
  return blocks
})

async function selectItem(folder, id, listItem) {
  activeId.value = id
  activeType.value = folder
  activeDoc.value = listItem
  expandedBlock.value = null
  inlineCommentText.value = ''
  showHistory.value = false
  versions.value = []
  diffText.value = ''
  selectedVersionIdx.value = null
  showApprovedDiff.value = false
  approvedDiffLines.value = []
  // Reset to Content tab so opening a doc lands on the body
  docTab.value = 'content'
  loadContent(folder, id)
  // Preserve existing hash (e.g. #p5 from Inbox navigation)
  const currentHash = route.hash || window.location.hash || ''
  // Update URL for permalink (use Vue Router, preserve org prefix when needed)
  router.push(`${orgPath(`/documents/${encodeURIComponent(id)}`)}${currentHash}`)
  loadComments(id).then(() => {
    checkHashAnchor()
  })
}

function checkHashAnchor() {
  const hash = route.hash || window.location.hash
  if (!hash) return

  // Hash-based anchor: #ph<hash> or #ph<hash>-<index> (stable across edits)
  const hashMatch = hash.match(/^#ph([^-]+)(?:-(\d+))?$/)
  if (hashMatch) {
    // Paragraph anchors only resolve when the Content tab is rendered.
    if (docTab.value !== 'content') docTab.value = 'content'
    const targetHash = hashMatch[1]
    const hintIndex = hashMatch[2] != null ? parseInt(hashMatch[2], 10) : null
    setTimeout(() => {
      const blocks = contentBlocks.value
      // If we have both hash and index hint, try exact match first
      if (hintIndex != null && hintIndex < blocks.length) {
        const bh = blockHash(hintIndex)
        if (bh === targetHash) {
          scrollToBlockAndExpand(hintIndex)
          return
        }
      }
      // Fallback: find first block with matching hash
      for (let i = 0; i < blocks.length; i++) {
        if (blockHash(i) === targetHash) {
          scrollToBlockAndExpand(i)
          return
        }
      }
    }, 500)
    return
  }

}

function scrollToBlockAndExpand(blockIndex) {
  expandedBlock.value = blockIndex
  nextTick(() => {
    const blocks = document.querySelectorAll('.comment-block')
    const target = blocks[blockIndex] || blocks[Math.floor(blockIndex / 1000)]
    if (target) {
      target.scrollIntoView({ behavior: 'smooth', block: 'center' })
      // Flash highlight
      target.classList.add('ring-2', 'ring-blue-500/50')
      setTimeout(() => target.classList.remove('ring-2', 'ring-blue-500/50'), 3000)
    }
  })
}

async function loadContent(folder, id) {
  loadingContent.value = true
  rawContent.value = ''
  try {
    const detail = await api.getDocument(folder, id)
    if (detail) {
      // Merge detail fields into activeDoc for header display
      activeDoc.value = { ...activeDoc.value, ...detail }
      rawContent.value = detail.content || ''
    }
  } catch (e) {
    rawContent.value = `**Error loading content:** ${e.message}`
  } finally {
    loadingContent.value = false
  }
}

// --- Comments ---
const newComment = ref('')
const commentAuthor = ref('')
const submitting = ref(false)

// --- Review tracking (localStorage) ---
const reviewedBlocks = ref(new Set())

const reviewProgress = computed(() => {
  if (contentBlocks.value.length === 0) return 0
  return Math.round((reviewedBlocks.value.size / contentBlocks.value.length) * 100)
})

function loadReviewProgress() {
  reviewedBlocks.value = new Set()
  if (!activeId.value) return
  const total = contentBlocks.value.length
  for (let i = 0; i < total; i++) {
    const key = `isms_reviewed_${activeId.value}_${i}`
    if (localStorage.getItem(key) === '1') {
      reviewedBlocks.value.add(i)
    }
  }
  // Trigger reactivity
  reviewedBlocks.value = new Set(reviewedBlocks.value)
}

function toggleReviewBlock(index) {
  const newSet = new Set(reviewedBlocks.value)
  const key = `isms_reviewed_${activeId.value}_${index}`
  if (newSet.has(index)) {
    newSet.delete(index)
    localStorage.removeItem(key)
  } else {
    newSet.add(index)
    localStorage.setItem(key, '1')
  }
  reviewedBlocks.value = newSet
}

function clearReviewProgress() {
  if (!activeId.value) return
  const total = contentBlocks.value.length
  for (let i = 0; i < total; i++) {
    localStorage.removeItem(`isms_reviewed_${activeId.value}_${i}`)
  }
  reviewedBlocks.value = new Set()
}

// --- Inline comments (composable) ---
const {
  comments,
  loadingComments,
  inlineCommentCount,
  loadComments: _loadComments,
  commentsForBlock,
  hasOpenComments,
  commentCountForBlock,
  blockHash,
  expandedBlock,
  inlineCommentText,
  submittingInline,
  toggleBlockComments,
  startInlineComment,
  submitInlineComment,
  extractQuote,
} = useDocumentComments({ activeId, contentBlocks, api, commentAuthor })

const showCommentPanel = ref(false)
const panelReplyingTo = ref(null)
const panelReplyText = ref('')
const showRightPanel = ref(false)
const inlineTextareaRef = ref(null)

// Wrapper: load comments and auto-expand panel if there are open comments
async function loadComments(docId) {
  await _loadComments(docId)
  if (comments.value.some(c => c.status === 'open')) {
    showCommentPanel.value = true
  }
}

const openComments = computed(() =>
  comments.value.filter(c => c.status !== 'resolved' && !c.parent_id)
)
const resolvedComments = computed(() =>
  comments.value.filter(c => c.status === 'resolved' && !c.parent_id)
)
const openCommentCount = computed(() => openComments.value.length)
const resolvedCommentCount = computed(() => resolvedComments.value.length)

// --- Reference link click handler (navigate via Vue Router) ---
function onRefLinkClick(e) {
  const link = e.target.closest('a[data-ref-type]')
  if (!link) return
  e.preventDefault()
  const href = link.getAttribute('href')
  if (href && href !== '#') {
    router.push(href)
  }
}

// --- Panel reply helpers ---
function panelRepliesFor(commentId) {
  return comments.value.filter(c => c.parent_id === commentId)
}

function startPanelReply(comment) {
  panelReplyingTo.value = comment.id
  panelReplyText.value = ''
}

async function submitPanelReply() {
  if (!panelReplyText.value.trim() || !panelReplyingTo.value) return
  try {
    await api.addComment({
      document_id: activeId.value,
      author: commentAuthor.value || getCurrentUser(),
      body: panelReplyText.value,
      parent_id: panelReplyingTo.value,
    })
    panelReplyText.value = ''
    panelReplyingTo.value = null
    await loadComments(activeId.value)
  } catch (e) {
    console.error('Failed to submit reply:', e)
    toastError('Failed to submit reply: ' + (e.message || 'unknown error'))
  }
}

function scrollToBlock(blockIndex) {
  // Paragraph blocks only exist on the Content tab — switch first if needed.
  if (docTab.value !== 'content') docTab.value = 'content'
  scrollToBlockAndExpand(blockIndex)
  // Update URL hash
  const base = window.location.pathname
  window.history.replaceState({}, '', `${base}#p${blockIndex}`)
}

// --- Version history & diff ---
const showHistory = ref(false)
const versions = ref([])
const loadingVersions = ref(false)
const selectedVersionIdx = ref(null)
const diffText = ref('')
const loadingDiff = ref(false)

async function toggleHistory() {
  // Toolbar history button toggles into/out of the Versions tab.
  if (docTab.value === 'versions') {
    docTab.value = 'content'
  } else {
    await switchDocTab('versions')
  }
  // Keep showHistory in sync for any legacy bindings (kept for templates).
  showHistory.value = docTab.value === 'versions'
}

async function selectDiffVersion(ver, idx) {
  selectedVersionIdx.value = idx
  diffText.value = ''
  if (!ver.hash) return
  const toHash = idx > 0 ? versions.value[idx - 1]?.hash : null
  loadingDiff.value = true
  try {
    const result = await api.getDocDiff(activeId.value, ver.hash, toHash)
    diffText.value = result?.diff || ''
  } catch {
    diffText.value = ''
  } finally {
    loadingDiff.value = false
  }
}

async function submitComment() {
  if (!newComment.value.trim() || submitting.value) return
  submitting.value = true
  try {
    await api.addComment({
      document_id: activeId.value,
      author: commentAuthor.value.trim() || getCurrentUser(),
      body: newComment.value.trim(),
    })
    newComment.value = ''
    await loadComments(activeId.value)
  } catch (e) {
    console.error('Failed to submit comment:', e)
    toastError('Failed to submit comment: ' + (e.message || 'unknown error'))
  } finally {
    submitting.value = false
  }
}

async function resolveComment(id) {
  try {
    await api.resolveComment(id, { resolved_by: commentAuthor.value.trim() || getCurrentUser() })
    await loadComments(activeId.value)
  } catch (e) {
    console.error('Failed to resolve comment:', e)
    toastError('Failed to resolve comment: ' + (e.message || 'unknown error'))
  }
}

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

// Reload review progress when content blocks change (new doc loaded)
watch(() => [activeId.value, contentBlocks.value.length], () => {
  if (activeId.value && contentBlocks.value.length > 0) {
    loadReviewProgress()
  }
})

// Watch route changes (e.g. navigation from Inbox with hash)
watch(() => route.fullPath, () => {
  const path = route.path
  // Match /documents/<docId>
  const match = path.match(/\/documents\/([^/]+)$/)
  if (!match) {
    // Navigated to /documents with no doc selected — show dashboard
    if (path.endsWith('/documents') || path.endsWith('/documents/')) {
      activeDoc.value = null
      activeId.value = null
    }
    return
  }
  // Wait for data to load
  if (folders.value.length === 0) return

  const id = decodeURIComponent(match[1])

  if (id !== activeId.value) {
    // Search all folders for this document_id
    for (const folder of folders.value) {
      const file = findFileInFolder(folder.name, id)
      if (file) {
        activeFolder.value = folder.name
        expandPathToFile(file.path)
        selectItem(folder.name, id, file)
        break
      }
    }
  } else if (route.hash) {
    checkHashAnchor()
  }
})

// --- Init ---
onMounted(async () => {
  commentAuthor.value = getCurrentUser() || ''
  document.addEventListener('click', closeMentionOnClickOutside)

  // Load users and current user role
  try {
    allUsers.value = await api.getUsers() || []
  } catch {
    allUsers.value = []
  }
  try {
    const me = await api.getMe()
    currentUserRole.value = me?.role || ''
    currentUserEmail.value = me?.email || ''
  } catch { /* ignore */ }

  loadAvailableTemplates()

  try {
    await loadTree()

    // Default to first folder
    if (folders.value.length > 0) {
      activeFolder.value = folders.value[0].name
      // Auto-expand top-level folders
      for (const f of folders.value) {
        expandedNodes.add(f.name)
      }
    }

    // Check URL for permalink — e.g. /:org/documents/:docId
    const urlDocId = route.params.docId || route.query.open
    if (urlDocId) {
      const id = decodeURIComponent(urlDocId)
      for (const folder of folders.value) {
        const file = findFileInFolder(folder.name, id)
        if (file) {
          activeFolder.value = folder.name
          expandPathToFile(file.path)
          selectItem(folder.name, id, file)
          break
        }
      }
    }
  } catch (e) {
    console.error('Failed to load document list:', e)
    toastError('Failed to load documents: ' + (e.message || 'unknown error'))
  } finally {
    loadingTree.value = false
  }

  // Load recently changed documents
  loadChangedDocs()

  // Load needs-review documents
  loadNeedsReview()
})

// Escape handling — match the composable pattern used by every other modal in the app.
// Each modal closes itself when visible and Escape is pressed (skipped if focus is in input).
useModalEscape(showNewMenu)
useModalEscape(showRemoveConfirm)
useModalEscape(showTemplatePicker)
useModalEscape(showNewDocModal)
useModalEscape(showReviewModal)

document.addEventListener('click', (e) => {
  if (showNewMenu.value && !e.target.closest('.relative')) showNewMenu.value = false
})

onBeforeUnmount(() => {
  document.removeEventListener('click', closeMentionOnClickOutside)
})
</script>

<style scoped>
/* Prose-like styles for rendered markdown in dark mode */
.doc-prose {
  color: rgb(148 163 184); /* slate-400 */
  font-size: 0.9375rem;
  line-height: 1.75;
}

.doc-prose :deep(h1) {
  color: rgb(226 232 240); /* slate-200 */
  font-size: 1.5rem;
  font-weight: 700;
  margin-top: 2rem;
  margin-bottom: 1rem;
  line-height: 1.3;
}

.doc-prose :deep(h2) {
  color: rgb(226 232 240);
  font-size: 1.25rem;
  font-weight: 600;
  margin-top: 1.75rem;
  margin-bottom: 0.75rem;
  line-height: 1.35;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid rgb(30 41 59); /* slate-800 */
}

.doc-prose :deep(h3) {
  color: rgb(226 232 240);
  font-size: 1.1rem;
  font-weight: 600;
  margin-top: 1.5rem;
  margin-bottom: 0.5rem;
  line-height: 1.4;
}

.doc-prose :deep(h4),
.doc-prose :deep(h5),
.doc-prose :deep(h6) {
  color: rgb(203 213 225); /* slate-300 */
  font-size: 1rem;
  font-weight: 600;
  margin-top: 1.25rem;
  margin-bottom: 0.5rem;
}

.doc-prose :deep(p) {
  margin-top: 0;
  margin-bottom: 1rem;
}

.doc-prose :deep(a) {
  color: rgb(96 165 250); /* blue-400 */
  text-decoration: none;
}

.doc-prose :deep(a:hover) {
  text-decoration: underline;
}

.doc-prose :deep(strong) {
  color: rgb(203 213 225); /* slate-300 */
  font-weight: 600;
}

.doc-prose :deep(ul),
.doc-prose :deep(ol) {
  margin-top: 0;
  margin-bottom: 1rem;
  padding-left: 1.5rem;
}

.doc-prose :deep(li) {
  margin-bottom: 0.375rem;
}

.doc-prose :deep(ul > li) {
  list-style-type: disc;
}

.doc-prose :deep(ol > li) {
  list-style-type: decimal;
}

.doc-prose :deep(code) {
  color: rgb(147 197 253); /* blue-300 */
  background: rgb(30 41 59); /* slate-800 */
  padding: 0.125rem 0.375rem;
  border-radius: 0.25rem;
  font-size: 0.875em;
}

.doc-prose :deep(pre) {
  background: rgb(15 23 42); /* slate-900 */
  border: 1px solid rgb(30 41 59);
  border-radius: 0.5rem;
  padding: 1rem;
  overflow-x: auto;
  margin-top: 0;
  margin-bottom: 1rem;
}

.doc-prose :deep(pre code) {
  background: transparent;
  padding: 0;
  color: rgb(203 213 225);
}

.doc-prose :deep(blockquote) {
  border-left: 3px solid rgb(51 65 85); /* slate-700 */
  padding-left: 1rem;
  margin-left: 0;
  margin-bottom: 1rem;
  color: rgb(100 116 139); /* slate-500 */
  font-style: italic;
}

.doc-prose :deep(table) {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  margin-bottom: 1rem;
  font-size: 0.875rem;
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid rgb(51 65 85);
}

.doc-prose :deep(th) {
  background: rgb(30 41 59);
  padding: 0.65rem 1rem;
  text-align: left;
  font-weight: 600;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: rgb(148 163 184);
  border-bottom: 2px solid rgb(51 65 85);
}

.doc-prose :deep(th:not(:last-child)) {
  border-right: 1px solid rgb(51 65 85);
}

.doc-prose :deep(td) {
  padding: 0.6rem 1rem;
  border-bottom: 1px solid rgba(51 65 85 / 0.4);
  color: rgb(203 213 225);
  line-height: 1.6;
}

.doc-prose :deep(td:not(:last-child)) {
  border-right: 1px solid rgba(51 65 85 / 0.3);
}

.doc-prose :deep(tr:last-child td) {
  border-bottom: none;
}

.doc-prose :deep(tbody tr:hover td) {
  background: rgba(51 65 85 / 0.15);
}

.doc-prose :deep(td strong) {
  color: rgb(241 245 249);
  font-weight: 600;
}

.doc-prose :deep(hr) {
  border: none;
  border-top: 1px solid rgb(30 41 59);
  margin: 1.5rem 0;
}

.doc-prose :deep(img) {
  max-width: 100%;
  border-radius: 0.5rem;
}

/* Review checkbox — always visible when checked */
.review-checkbox {
  transition: all 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.review-checked .review-checkbox {
  opacity: 1 !important;
}

/* Reviewed block — subtle green left border */
.review-checked {
  border-left: 2px solid rgba(16, 185, 129, 0.3);
}

/* Progress bar gradients */
.review-bar-progress {
  background: linear-gradient(90deg, rgb(16 185 129), rgb(52 211 153));
}
.review-bar-complete {
  background: linear-gradient(90deg, rgb(16 185 129), rgb(110 231 183));
  animation: bar-glow 2s ease-in-out infinite;
}

@keyframes bar-glow {
  0%, 100% { box-shadow: 0 0 4px rgba(16, 185, 129, 0.3); }
  50% { box-shadow: 0 0 12px rgba(16, 185, 129, 0.6); }
}

/* All reviewed badge pulse */
.review-complete-badge {
  animation: badge-pop 0.5s cubic-bezier(0.34, 1.56, 0.64, 1);
}

@keyframes badge-pop {
  0% { transform: scale(0.5); opacity: 0; }
  60% { transform: scale(1.1); }
  100% { transform: scale(1); opacity: 1; }
}

/* Smooth transition for comment block highlights */
.comment-block {
  transition: background-color 0.2s ease, border-color 0.3s ease;
}

/* Grid-based table rows — each row is a commentable block */
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
  min-width: 0; /* allow 1fr grid tracks to shrink below content width (#14) */
}
.doc-prose :deep(.tbl-cell:last-child) {
  border-right: none;
}
/* Inline code chips stay whole (#14) — no mid-token wrap — and clip with an
   ellipsis if longer than the cell, so a long token degrades gracefully instead
   of overflowing into the next grid column. */
.doc-prose :deep(.tbl-cell) code,
.doc-prose :deep(.tbl-hdr-cell) code {
  white-space: nowrap;
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: bottom;
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
