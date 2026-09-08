<template>
  <div class="min-h-full">
    <div class="overflow-y-auto">
    <!-- Loading -->
    <div v-if="loading" class="max-w-6xl mx-auto px-8 py-10">
      <ListSkeleton :rows="5" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="max-w-5xl mx-auto px-8 py-12">
      <div class="bg-red-950/40 border border-red-900/50 rounded-lg p-6 text-red-300 text-sm flex items-center justify-between gap-4">
        <span>{{ error }}</span>
        <RefreshButton :loading="refreshing" @refresh="reload" />
      </div>
    </div>

    <!-- Main content -->
    <div v-else class="max-w-6xl mx-auto px-8 py-10 space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">{{ t('assets.title') }}</h1>
          <p class="text-sm text-slate-500 mt-1">{{ t('assets.subtitle') }}</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canWrite" @click="showCreateForm = !showCreateForm"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            {{ showCreateForm ? t('common.action.cancel') : t('assets.action.add') }}
          </button>
          <SuggestNewButton entityType="asset" :typeLabel="entityLabel('asset')" />
        </div>
      </div>

      <!-- Create form (modal) -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreateForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreateForm = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
          <div class="flex items-center justify-between mb-2">
            <h2 class="text-sm font-semibold text-slate-200">{{ t('assets.create.heading') }}</h2>
            <button @click="showCreateForm = false" class="text-slate-500 hover:text-slate-300">
              <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <div class="space-y-3">
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('assets.create.name_label') }}</label>
              <input v-model="newItem.name" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('assets.create.name_placeholder')" />
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('assets.create.type_label') }}</label>
              <div class="flex flex-wrap gap-1.5">
                <button v-for="o in typeOptions" :key="o.value"
                  @click="newItem.asset_type = newItem.asset_type === o.value ? '' : o.value"
                  class="px-2.5 py-1 text-[11px] font-medium rounded-lg border transition-colors"
                  :class="newItem.asset_type === o.value ? 'bg-blue-600/20 text-blue-400 border-blue-500/40' : 'bg-slate-800 text-slate-400 border-slate-700 hover:border-slate-600'"
                  :title="newItem.asset_type === o.value ? t('assets.create.deselect_hint') : ''">
                  {{ o.label }}
                </button>
              </div>
            </div>
          </div>
          <div class="text-[10px] text-slate-600 mt-1">{{ t('assets.create.fill_in_later') }}</div>
          <div class="flex justify-end gap-3 pt-3 border-t border-slate-800">
            <button @click="showCreateForm = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">{{ t('common.action.cancel') }}</button>
            <button @click="createItem" :disabled="!newItem.name" class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
              {{ t('assets.create.submit') }}
            </button>
          </div>
        </div>
      </div>
      </Transition>
      </Teleport>

      <!-- Summary strip -->
      <StatStrip :stats="statusStats" v-model="filterStatus" />

      <!-- Search + Filters -->
      <div class="space-y-3">
        <div class="flex flex-wrap items-center gap-3">
          <RefreshButton :loading="refreshing" @refresh="reload" class="shrink-0" />
          <div class="relative flex-1 max-w-xs">
            <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
            </svg>
            <input v-model="searchQuery" type="text" :placeholder="t('common.placeholder.search')"
              class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
          </div>
          <select v-model="filterType" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">{{ t('common.filter.all_types') }}</option>
            <option v-for="o in typeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
          <select v-model="filterStatus" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">{{ t('common.filter.all_statuses') }}</option>
            <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
          <button v-if="filterType || filterStatus || searchQuery"
            @click="filterType = ''; filterStatus = ''; searchQuery = ''"
            class="text-[10px] text-slate-600 hover:text-slate-400 transition-colors">
            {{ t('common.action.clear') }}
          </button>
          <div class="ml-auto text-xs text-slate-500 tabular-nums">
            {{ t('common.count.total', { count: total }) }}
          </div>
        </div>
      </div>

      <!-- Empty state -->
      <div v-if="assets.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
        <div class="text-sm text-slate-500">{{ t('assets.filter.empty') }}</div>
      </div>

      <!-- Table -->
      <div v-else class="bg-slate-900 border border-slate-800 rounded-xl overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="border-b border-slate-800">
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('assets.table.header.asset') }}</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('assets.table.header.type') }}</th>
              <th class="text-center px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('assets.table.header.cia') }}</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('assets.table.header.status') }}</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('assets.table.header.owner') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/50">
            <tr v-for="asset in assets" :key="asset.id"
              @click="selectItem(asset)"
              class="hover:bg-slate-800/50 transition-colors cursor-pointer">
              <td class="px-5 py-3.5">
                <div class="text-sm font-medium text-slate-200">{{ asset.name }}</div>
                <div v-if="asset.description" class="text-xs text-slate-500 mt-0.5 truncate max-w-xs">{{ asset.description }}</div>
              </td>
              <td class="px-5 py-3.5 text-sm text-slate-400">{{ typeLabel(asset.asset_type) || '—' }}</td>
              <td class="px-5 py-3.5 text-center">
                <div class="flex gap-0.5 justify-center">
                  <span v-if="asset.confidentiality > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(asset.confidentiality)" :title="t('assets.table.cia_title.confidentiality', { level: ciaLabel(asset.confidentiality) })">C{{ asset.confidentiality }}</span>
                  <span v-if="asset.integrity > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(asset.integrity)" :title="t('assets.table.cia_title.integrity', { level: ciaLabel(asset.integrity) })">I{{ asset.integrity }}</span>
                  <span v-if="asset.availability > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(asset.availability)" :title="t('assets.table.cia_title.availability', { level: ciaLabel(asset.availability) })">A{{ asset.availability }}</span>
                  <span v-if="!asset.confidentiality && !asset.integrity && !asset.availability" class="text-slate-600 text-xs">-</span>
                </div>
              </td>
              <td class="px-5 py-3.5">
                <StatusBadge :status="asset.status" />
              </td>
              <td class="px-5 py-3.5 text-sm text-slate-400">{{ resolveUserName(asset.owner) }}</td>
            </tr>
          </tbody>
        </table>
        <Pagination :page="page" :pageSize="pageSize" :total="total" @update:page="page = $event" @update:pageSize="pageSize = $event" />
      </div>

      <!-- Detail modal -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="selectedItem" class="fixed inset-0 z-50 flex items-start justify-center pt-[3vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="closeDetail" />
        <div class="relative w-full max-w-4xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-h-[90vh] flex flex-col">
          <!-- Header -->
          <div class="flex-shrink-0 border-b border-slate-800 px-6 py-3 flex items-center justify-between gap-4">
            <div class="flex items-center gap-6 min-w-0">
              <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ selectedItem.identifier }}</span>
              <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selectedItem.name }}</h2>
            </div>
            <div class="flex items-center gap-3 flex-shrink-0">
              <CopyLinkButton />
              <StatusBadge :status="selectedItem.status" />
              <button @click="closeDetail" class="p-1 rounded-lg text-slate-600 hover:text-slate-300 hover:bg-slate-800 transition-colors">
                <svg class="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          </div>

          <!-- Body -->
          <div class="flex flex-1 min-h-0">
            <!-- Left nav -->
            <nav class="flex-shrink-0 w-28 border-r border-slate-800 py-3">
              <div class="space-y-0.5">
                <button v-for="tab in detailTabs" :key="tab.key" @click="switchDetailTab(tab.key)"
                  class="w-full text-left px-3 py-2 text-xs font-medium transition-colors"
                  :class="detailTab === tab.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                  {{ tab.label }}
                </button>
              </div>
            </nav>

            <!-- Content -->
            <div class="flex-1 overflow-y-auto min-h-0">

              <!-- ═══ OVERVIEW ═══ -->
              <template v-if="detailTab === 'overview'">
                <div class="px-6 py-5 space-y-5">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('assets.detail.overview') }}</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('overview')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                  </div>
                  <template v-if="editingSection === 'overview'">
                    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('assets.field.name') }}</label>
                        <input v-model="editForm.name" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('assets.field.description') }}</label>
                        <MarkdownField v-model="editForm.description" :self-type="'asset'" :self-id="selectedItem?.identifier || ''" :rows="3" :placeholder="t('assets.placeholder.description')" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('assets.field.type') }}</label>
                        <select v-model="editForm.asset_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="">{{ t('common.option.none') }}</option>
                          <option v-for="o in typeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('assets.field.status') }}</label>
                        <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('assets.field.owner') }}</label>
                        <MemberPicker v-model="editForm.owner" :members="orgMembers" :placeholder="t('common.placeholder.select_owner')" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('assets.field.primary_location') }}</label>
                        <input v-model="editForm.primary_location" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('assets.placeholder.primary_location')" />
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('assets.field.name') }}</div>
                        <div class="text-sm text-slate-200 font-medium">{{ selectedItem.name }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('assets.field.description') }}</div>
                        <div v-if="selectedItem.description" class="text-sm text-slate-300 leading-relaxed doc-prose" v-mermaid v-html="renderMd(selectedItem.description)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>
                      <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('assets.field.status') }}</div>
                          <StatusBadge :status="selectedItem.status" />
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('assets.field.type') }}</div>
                          <div class="text-sm text-slate-300">{{ typeLabel(selectedItem.asset_type) || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('assets.field.owner') }}</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedItem.owner) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('assets.field.primary_location') }}</div>
                          <div class="text-sm text-slate-300">{{ selectedItem.primary_location || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('assets.field.next_review') }}</div>
                          <div class="text-sm" :class="isOverdue(selectedItem.next_review) ? 'text-red-400' : 'text-slate-300'">
                            {{ formatDay(selectedItem.next_review) || '—' }}
                          </div>
                        </div>
                        <div v-if="selectedItem.created_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('assets.field.created') }}</div>
                          <div class="text-sm text-slate-300">{{ formatDate(selectedItem.created_at) }}</div>
                        </div>
                        <div v-if="selectedItem.created_by">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('assets.field.created_by') }}</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedItem.created_by) }}</div>
                        </div>
                      </div>
                    </div>
                  </template>
                </div>
              </template>

              <!-- ═══ ASSESSMENT ═══ -->
              <template v-if="detailTab === 'assessment'">
                <div class="px-6 py-5 space-y-5">
                  <div v-if="selectedItem.next_review && isOverdue(selectedItem.next_review)" class="flex items-center gap-2 px-3 py-2 rounded-lg text-xs bg-red-950/40 border border-red-900/40 text-red-300">
                    <svg class="w-3.5 h-3.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                    {{ t('common.review.overdue_since', { date: formatDay(selectedItem.next_review) }) }}
                  </div>

                  <!-- CIA scores -->
                  <div class="grid grid-cols-3 gap-4">
                    <div class="bg-slate-800/40 border border-slate-700/50 rounded-lg px-5 py-4 text-center">
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider">{{ t('assets.assessment.confidentiality') }}</div>
                      <div v-if="selectedItem.confidentiality" class="text-2xl font-bold tabular-nums mt-1.5" :class="ciaColorText(selectedItem.confidentiality)">{{ selectedItem.confidentiality }}</div>
                      <div v-else class="text-2xl text-slate-700 mt-1.5">—</div>
                      <div v-if="selectedItem.confidentiality" class="text-[10px] text-slate-500 mt-0.5">{{ ciaLabel(selectedItem.confidentiality) }}</div>
                    </div>
                    <div class="bg-slate-800/40 border border-slate-700/50 rounded-lg px-5 py-4 text-center">
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider">{{ t('assets.assessment.integrity') }}</div>
                      <div v-if="selectedItem.integrity" class="text-2xl font-bold tabular-nums mt-1.5" :class="ciaColorText(selectedItem.integrity)">{{ selectedItem.integrity }}</div>
                      <div v-else class="text-2xl text-slate-700 mt-1.5">—</div>
                      <div v-if="selectedItem.integrity" class="text-[10px] text-slate-500 mt-0.5">{{ ciaLabel(selectedItem.integrity) }}</div>
                    </div>
                    <div class="bg-slate-800/40 border border-slate-700/50 rounded-lg px-5 py-4 text-center">
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider">{{ t('assets.assessment.availability') }}</div>
                      <div v-if="selectedItem.availability" class="text-2xl font-bold tabular-nums mt-1.5" :class="ciaColorText(selectedItem.availability)">{{ selectedItem.availability }}</div>
                      <div v-else class="text-2xl text-slate-700 mt-1.5">—</div>
                      <div v-if="selectedItem.availability" class="text-[10px] text-slate-500 mt-0.5">{{ ciaLabel(selectedItem.availability) }}</div>
                    </div>
                  </div>

                  <div class="flex gap-4 text-[10px] text-slate-500 flex-wrap">
                    <span v-if="selectedItem.last_review">{{ t('common.review.last', { date: formatDay(selectedItem.last_review) }) }}</span>
                    <span v-if="selectedItem.next_review && !isOverdue(selectedItem.next_review)">{{ t('common.review.next', { date: formatDay(selectedItem.next_review) }) }}</span>
                  </div>

                  <!-- Readings -->
                  <div class="border-t border-slate-800 pt-4">
                    <ReadingsPanel entityType="asset" :entityId="selectedItem.id" :identifier="selectedItem.identifier || ('ASSET-' + selectedItem.id)" :canWrite="canWrite"
                      :currentValues="{ confidentiality: selectedItem.confidentiality, integrity: selectedItem.integrity, availability: selectedItem.availability }"
                      @saved="refreshSelectedItem" />
                  </div>
                </div>
              </template>

              <!-- ═══ NOTES ═══ -->
              <template v-if="detailTab === 'notes'">
                <div class="px-6 py-5 space-y-5">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('assets.detail.notes') }}</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('notes')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                  </div>
                  <template v-if="editingSection === 'notes'">
                    <MarkdownField v-model="editForm.notes" :self-type="'asset'" :self-id="selectedItem?.identifier || ''" :rows="12" :placeholder="t('assets.placeholder.notes')" />
                  </template>
                  <template v-else>
                    <div v-if="selectedItem.notes" class="text-sm doc-prose text-slate-300 leading-relaxed" v-mermaid v-html="renderMd(selectedItem.notes)"></div>
                    <div v-else class="text-sm text-slate-600 italic">{{ t('common.state.no_notes') }}</div>
                  </template>
                </div>
              </template>

              <!-- ═══ LINKS ═══ -->
              <template v-if="detailTab === 'links'">
                <div class="px-6 py-5 space-y-4">
                  <ReferenceManager entityType="asset" :entityId="selectedItem.identifier || ('ASSET-' + selectedItem.id)" :editable="canWrite" />
                </div>
              </template>

              <!-- ═══ SUGGESTIONS ═══ -->
              <template v-if="detailTab === 'suggestions'">
                <div class="px-6 py-5">
                  <SuggestionPanel entityType="asset" :entityId="selectedItem.identifier" :canReview="canWrite" @applied="loadAssets" />
                </div>
              </template>

              <!-- ═══ COMMENTS ═══ -->
              <template v-if="detailTab === 'comments'">
                <div class="px-6 py-5">
                  <CommentsPanel entityType="asset" :entityId="selectedItem.identifier" />
                </div>
              </template>

              <!-- ═══ HISTORY ═══ -->
              <template v-if="detailTab === 'history'">
                <div class="px-6 py-5 space-y-6">
                  <HistoryPanel entityType="asset" :entityId="String(selectedItem.id)" />
                  <div v-if="canWrite" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">{{ t('common.heading.danger_zone') }}</div>
                    <div class="text-xs text-slate-400">{{ t('assets.danger.warning') }}</div>
                    <button @click="deleteSelectedItem" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                      {{ t('assets.danger.delete') }}
                    </button>
                  </div>
                </div>
              </template>

            </div>
          </div>

          <!-- Footer action bar (edit mode only) -->
          <div v-if="editingSection" class="flex-shrink-0 border-t border-slate-800 px-6 py-3 flex justify-end gap-3">
            <button @click="cancelSection" class="px-4 py-1.5 text-sm text-slate-400 hover:text-slate-200 transition-colors">{{ t('common.action.cancel') }}</button>
            <button @click="saveSection" :disabled="saving" class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 text-white text-sm font-medium rounded-lg transition-colors">{{ saving ? t('common.state.saving') : t('common.action.save') }}</button>
          </div>
        </div>
      </div>
      </Transition>
      </Teleport>

    </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import StatStrip from '../components/StatStrip.vue'
import RefreshButton from '../components/RefreshButton.vue'
import MemberPicker from '../components/MemberPicker.vue'
import MarkdownField from '../components/MarkdownField.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
import CopyLinkButton from '../components/CopyLinkButton.vue'
import SuggestionPanel from '../components/SuggestionPanel.vue'
import CommentsPanel from '../components/CommentsPanel.vue'
import HistoryPanel from '../components/HistoryPanel.vue'
import SuggestNewButton from '../components/SuggestNewButton.vue'
import Pagination from '../components/Pagination.vue'
import ReadingsPanel from '../components/ReadingsPanel.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import { renderMarkdown } from '../composables/useRenderMd.js'
import { useModalEscape } from '../composables/useModalEscape.js'
import { useConfirm } from '../composables/useConfirm.js'
import { useToast } from '../composables/useToast.js'
import { useDirtyEdit } from '../composables/useDirtyEdit.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import { formatDate, formatDay } from '../composables/useFormat.js'
import { useEnumLabel } from '../composables/useEnumLabel.js'
import { renderApiError } from '../composables/useApiError.js'

const { t } = useI18n()
const { enumLabel, entityLabel } = useEnumLabel()
const { confirm: confirmDialog } = useConfirm()
const { show: showError, success: showSaved } = useToast()

const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()

const renderMd = renderMarkdown

const userRole = ref('')
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')

const loading = ref(true)
const refreshing = ref(false)
async function reload() {
  refreshing.value = true
  error.value = null
  try {
    await loadAssets()
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    refreshing.value = false
  }
}
const error = ref(null)
const assets = ref([])
const orgMembers = ref([])
const saving = ref(false)
const stats = ref({ total: 0, draft: 0, open: 0, archived: 0, critical: 0 })
const statusStats = computed(() => [
  { key: '', label: t('common.stat.total'), count: stats.value.total, color: 'text-slate-100' },
  { key: 'draft', label: statusLabel('draft'), count: stats.value.draft || 0, color: 'text-amber-400' },
  { key: 'open', label: statusLabel('open'), count: stats.value.open, color: 'text-blue-400' },
  { key: 'archived', label: statusLabel('archived'), count: stats.value.archived, color: 'text-slate-400' },
  // Not a status and not an enum member: this chip counts assets whose highest
  // CIA rating is 5, so its label is this view's own copy.
  { key: 'critical', label: t('assets.stat.severe'), count: stats.value.critical, color: stats.value.critical > 0 ? 'text-red-400' : 'text-slate-100', static: true },
])

const selectedItem = ref(null)
const detailTab = ref('overview')
const editingSection = ref('')
const editForm = ref({})
const { capture: captureEditSnapshot, isDirty } = useDirtyEdit(editForm)

const showCreateForm = ref(false)
const newItem = ref({
  name: '', asset_type: '',
})

const searchQuery = ref('')
const filterType = ref('')
const filterStatus = ref('')
const page = ref(1)
const pageSize = ref(50)
const total = ref(0)

// The members each picker offers, matching the asset_type CHECK in
// migrations/20260327000000_initial_schema.sql. Only the label comes from the
// shared catalogue.
const ASSET_TYPES = [
  'infrastructure', 'processing_devices', 'software', 'system', 'network',
  'service', 'financial_info', 'personal_data', 'ipr', 'sales_marketing',
  'processing_facility', 'products_services', 'supply_chain', 'other',
]
const STATUSES = ['draft', 'open', 'archived']

// Message keys, not labels: a module-scope array of translated strings freezes
// the tab bar in whichever locale was active when this module first evaluated.
const TAB_KEYS = [
  { key: 'overview', label: 'common.tab.overview' },
  { key: 'assessment', label: 'common.tab.assessment' },
  { key: 'notes', label: 'common.tab.notes' },
  { key: 'links', label: 'common.tab.links' },
  { key: 'suggestions', label: 'common.tab.suggestions' },
  { key: 'comments', label: 'common.tab.comments' },
  { key: 'history', label: 'common.tab.history' },
]
const detailTabs = computed(() => TAB_KEYS.map((tab) => ({ ...tab, label: t(tab.label) })))

// Lookups and option lists live here rather than in the template: a group name
// is a stored identifier, and the raw-text scanner reads a bare quoted word in
// a mustache as unextracted copy.
const statusLabel = (v) => enumLabel('status', v)
const typeLabel = (v) => enumLabel('asset_type', v)

const options = (values, label) => computed(() => values.map((value) => ({ value, label: label(value) })))
const statusOptions = options(STATUSES, statusLabel)
const typeOptions = options(ASSET_TYPES, typeLabel)

useModalEscape(showCreateForm)
useModalEscape(computed(() => !!selectedItem.value), closeDetail)

let searchTimer = null
watch([searchQuery], () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { page.value = 1; loadAssets() }, 250)
})
watch([filterType, filterStatus], () => {
  page.value = 1
  loadAssets()
})
watch([page, pageSize], () => loadAssets())

// Keys written out rather than built from the index: a runtime-composed key
// defeats keyset extraction, which is why the convention forbids it.
const CIA_KEYS = [
  'common.cia.not_assessed',
  'common.cia.insignificant',
  'common.cia.minor',
  'common.cia.moderate',
  'common.cia.major',
  'common.cia.severe',
]
const ciaLabel = (v) => (CIA_KEYS[v] ? t(CIA_KEYS[v]) : t('common.cia.na'))

function ciaColor(v) {
  switch (v) {
    case 5: return 'bg-red-900/60 text-red-300'
    case 4: return 'bg-orange-900/60 text-orange-300'
    case 3: return 'bg-amber-900/60 text-amber-300'
    case 2: return 'bg-emerald-900/60 text-emerald-300'
    case 1: return 'bg-slate-800 text-slate-400'
    default: return 'bg-slate-800 text-slate-500'
  }
}

function ciaColorText(v) {
  switch (v) {
    case 5: return 'text-red-300'
    case 4: return 'text-orange-300'
    case 3: return 'text-amber-300'
    case 2: return 'text-emerald-300'
    case 1: return 'text-slate-400'
    default: return 'text-slate-500'
  }
}

function isOverdue(dateStr) {
  if (!dateStr) return false
  const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
  return d < new Date()
}

function resolveUserName(email) {
  if (!email) return '—'
  const u = orgMembers.value.find(m => m.email === email)
  return u?.name || email
}

async function loadAssets() {
  try {
    const params = new URLSearchParams()
    params.set('page', String(page.value))
    params.set('limit', String(pageSize.value))
    if (searchQuery.value) params.set('q', searchQuery.value)
    if (filterType.value) params.set('asset_type', filterType.value)
    if (filterStatus.value) params.set('status', filterStatus.value)
    const res = await api.fetchRaw(`/api/v1/assets?${params.toString()}`)
    assets.value = Array.isArray(res?.data) ? res.data : []
    total.value = res?.total || 0
    loadStats()
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function loadStats() {
  try { stats.value = await api.fetchJSON('/api/v1/assets/stats') || stats.value } catch { /* silent */ }
}

async function refreshSelectedItem() {
  if (!selectedItem.value) return
  const id = selectedItem.value.id
  try {
    const fresh = await api.fetchJSON(`/api/v1/assets/${id}`)
    if (fresh) selectedItem.value = fresh
  } catch { /* silent */ }
  await loadAssets()
}

function startEdit(item) {
  editForm.value = {
    name: item.name || '',
    description: item.description || '',
    asset_type: item.asset_type || '',
    status: item.status || 'open',
    owner: item.owner || '',
    primary_location: item.primary_location || '',
    confidentiality: item.confidentiality || 0,
    integrity: item.integrity || 0,
    availability: item.availability || 0,
    notes: item.notes || '',
  }
  captureEditSnapshot()
}

function editSection(section) {
  startEdit(selectedItem.value)
  editingSection.value = section
}

function cancelSection() {
  editingSection.value = ''
  startEdit(selectedItem.value)
}

async function saveSection() {
  saving.value = true
  try {
    await api.putJSON(`/api/v1/assets/${selectedItem.value.id}`, { ...editForm.value })
    await refreshSelectedItem()
    editingSection.value = ''
    showSaved(t('common.state.saved'))
  } catch (e) {
    showError(t('assets.error.save', { message: renderApiError(e) }))
  } finally {
    saving.value = false
  }
}

async function selectItem(item) {
  if (selectedItem.value?.id === item.id) {
    closeDetail()
    return
  }
  router.push(orgPath(`/assets/${item.identifier}`))
}

async function openItemFromRoute(id) {
  let item = assets.value.find(a => a.identifier === id || String(a.id) === String(id))
  if (!item) {
    try { item = await api.fetchJSON(`/api/v1/assets/${encodeURIComponent(id)}`) } catch { return }
  }
  if (!item) return
  selectedItem.value = item
  detailTab.value = 'overview'
  startEdit(item)
}

async function switchDetailTab(key) {
  if (editingSection.value && isDirty()) {
    const ok = await confirmDialog({
      message: t('assets.dirty.switch_tab'),
      variant: 'danger',
      confirmLabel: t('assets.dirty.discard'),
    })
    if (!ok) return
  }
  detailTab.value = key
  editingSection.value = ''
}

async function closeDetail() {
  if (editingSection.value && isDirty()) {
    const ok = await confirmDialog({
      message: t('assets.dirty.close'),
      variant: 'danger',
      confirmLabel: t('assets.dirty.discard'),
    })
    if (!ok) return
  }
  router.push(orgPath('/assets'))
}

async function createItem() {
  try {
    const payload = { ...newItem.value }
    const created = await api.postJSON('/api/v1/assets', payload)
    showCreateForm.value = false
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur()
    newItem.value = { name: '', asset_type: '' }
    await loadAssets()
    if (created?.id && !assets.value.find(a => a.id === created.id)) {
      assets.value = [created, ...assets.value]
    }
    // Drop user into detail modal in edit mode on Overview to keep filling things in.
    if (created && created.id) {
      let fresh = created
      try { fresh = await api.fetchJSON(`/api/v1/assets/${created.id}`) } catch { /* fall back */ }
      selectedItem.value = fresh
      detailTab.value = 'overview'
      startEdit(fresh)
      editingSection.value = 'overview'
      router.push(orgPath(`/assets/${fresh.id}`))
    }
  } catch (e) {
    showError(t('assets.error.create', { message: renderApiError(e) }))
  }
}

async function deleteSelectedItem() {
  if (!selectedItem.value) return
  const ok = await confirmDialog({ message: t('assets.danger.confirm'), variant: 'danger', confirmLabel: t('common.action.delete') })
  if (!ok) return
  try {
    await api.deleteJSON(`/api/v1/assets/${selectedItem.value.id}`)
    closeDetail()
    await loadAssets()
  } catch (e) {
    showError(t('assets.error.delete', { message: renderApiError(e) }))
  }
}

onMounted(async () => {
  try { const me = await api.getMe(); userRole.value = me?.role || '' } catch {}
  try {
    const [, users] = await Promise.all([
      loadAssets(),
      api.getUsers().catch(() => []),
    ])
    orgMembers.value = users || []
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    loading.value = false
  }
  if (route.params.id) openItemFromRoute(route.params.id)
})

watch(() => route.params.id, (id) => {
  if (!id) {
    selectedItem.value = null
    detailTab.value = 'overview'
    editingSection.value = ''
    return
  }
  if (selectedItem.value && (selectedItem.value.identifier === id || String(selectedItem.value.id) === String(id))) return
  openItemFromRoute(id)
})
</script>
