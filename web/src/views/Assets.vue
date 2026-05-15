<template>
  <div class="min-h-full">
    <div class="overflow-y-auto">
    <!-- Loading -->
    <div v-if="loading" class="max-w-6xl mx-auto px-8 py-10">
      <ListSkeleton :rows="5" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="max-w-5xl mx-auto px-8 py-12">
      <div class="bg-red-950/40 border border-red-900/50 rounded-lg p-6 text-red-300 text-sm">
        {{ error }}
      </div>
    </div>

    <!-- Main content -->
    <div v-else class="max-w-6xl mx-auto px-8 py-10 space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">Asset Register</h1>
          <p class="text-sm text-slate-500 mt-1">Information assets, classification, and ownership</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canWrite" @click="showCreateForm = !showCreateForm"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            {{ showCreateForm ? 'Cancel' : 'Add Asset' }}
          </button>
          <SuggestNewButton entityType="asset" typeLabel="Asset" />
        </div>
      </div>

      <!-- Create form (modal) -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreateForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreateForm = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
          <div class="flex items-center justify-between mb-2">
            <h2 class="text-sm font-semibold text-slate-200">Add Asset</h2>
            <button @click="showCreateForm = false" class="text-slate-500 hover:text-slate-300">
              <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <div class="space-y-3">
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">Name *</label>
              <input v-model="newItem.name" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="e.g. Production Database, Customer Portal" />
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">Type</label>
              <div class="flex flex-wrap gap-1.5">
                <button v-for="t in assetTypes" :key="t.key"
                  @click="newItem.asset_type = newItem.asset_type === t.key ? '' : t.key"
                  class="px-2.5 py-1 text-[11px] font-medium rounded-lg border transition-colors"
                  :class="newItem.asset_type === t.key ? 'bg-blue-600/20 text-blue-400 border-blue-500/40' : 'bg-slate-800 text-slate-400 border-slate-700 hover:border-slate-600'"
                  :title="newItem.asset_type === t.key ? 'Click to deselect' : ''">
                  {{ t.label }}
                </button>
              </div>
            </div>
          </div>
          <div class="text-[10px] text-slate-600 mt-1">You can add CIA classification, owner, and more after creating.</div>
          <div class="flex justify-end gap-3 pt-3 border-t border-slate-800">
            <button @click="showCreateForm = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
            <button @click="createItem" :disabled="!newItem.name" class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
              Add
            </button>
          </div>
        </div>
      </div>
      </Transition>
      </Teleport>

      <!-- Summary cards -->
      <div class="grid grid-cols-2 lg:grid-cols-5 gap-4">
        <button @click="filterStatus = ''" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="!filterStatus ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-slate-100 tabular-nums">{{ stats.total }}</div>
          <div class="text-xs text-slate-500 mt-1">Total</div>
        </button>
        <button @click="filterStatus = 'draft'" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="filterStatus === 'draft' ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-amber-400 tabular-nums">{{ stats.draft || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Draft</div>
        </button>
        <button @click="filterStatus = 'open'" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="filterStatus === 'open' ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-blue-400 tabular-nums">{{ stats.open }}</div>
          <div class="text-xs text-slate-500 mt-1">Open</div>
        </button>
        <button @click="filterStatus = 'archived'" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="filterStatus === 'archived' ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-slate-400 tabular-nums">{{ stats.archived }}</div>
          <div class="text-xs text-slate-500 mt-1">Archived</div>
        </button>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold tabular-nums" :class="stats.critical > 0 ? 'text-red-400' : 'text-slate-100'">{{ stats.critical }}</div>
          <div class="text-xs text-slate-500 mt-1">Severe (CIA = 5)</div>
        </div>
      </div>

      <!-- Search + Filters -->
      <div class="space-y-3">
        <div class="flex items-center gap-3">
          <div class="relative flex-1 max-w-xs">
            <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
            </svg>
            <input v-model="searchQuery" type="text" placeholder="Search..."
              class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
          </div>
          <select v-model="filterType" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">All types</option>
            <option v-for="t in assetTypes" :key="t.key" :value="t.key">{{ t.label }}</option>
          </select>
          <select v-model="filterStatus" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">All statuses</option>
            <option value="draft">Draft</option>
            <option value="open">Open</option>
            <option value="archived">Archived</option>
          </select>
          <button v-if="filterType || filterStatus || searchQuery"
            @click="filterType = ''; filterStatus = ''; searchQuery = ''"
            class="text-[10px] text-slate-600 hover:text-slate-400 transition-colors">
            Clear
          </button>
          <div class="ml-auto text-xs text-slate-500 tabular-nums">
            {{ total }} total
          </div>
        </div>
      </div>

      <!-- Empty state -->
      <div v-if="assets.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
        <div class="text-sm text-slate-500">No assets found</div>
      </div>

      <!-- Table -->
      <div v-else class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        <table class="w-full">
          <thead>
            <tr class="border-b border-slate-800">
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Asset</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Type</th>
              <th class="text-center px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">C/I/A</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Status</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Owner</th>
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
              <td class="px-5 py-3.5 text-sm text-slate-400 capitalize">{{ formatLabel(asset.asset_type) }}</td>
              <td class="px-5 py-3.5 text-center">
                <div class="flex gap-0.5 justify-center">
                  <span v-if="asset.confidentiality > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(asset.confidentiality)" :title="'Confidentiality: ' + ciaLabel(asset.confidentiality)">C{{ asset.confidentiality }}</span>
                  <span v-if="asset.integrity > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(asset.integrity)" :title="'Integrity: ' + ciaLabel(asset.integrity)">I{{ asset.integrity }}</span>
                  <span v-if="asset.availability > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(asset.availability)" :title="'Availability: ' + ciaLabel(asset.availability)">A{{ asset.availability }}</span>
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
                <button v-for="t in detailTabs" :key="t.key" @click="switchDetailTab(t.key)"
                  class="w-full text-left px-3 py-2 text-xs font-medium transition-colors"
                  :class="detailTab === t.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                  {{ t.label }}
                </button>
              </div>
            </nav>

            <!-- Content -->
            <div class="flex-1 overflow-y-auto min-h-0">

              <!-- ═══ OVERVIEW ═══ -->
              <template v-if="detailTab === 'overview'">
                <div class="px-6 py-5 space-y-5">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Overview</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('overview')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                  </div>
                  <template v-if="editingSection === 'overview'">
                    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Name</label>
                        <input v-model="editForm.name" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Description</label>
                        <MarkdownField v-model="editForm.description" :self-type="'asset'" :self-id="selectedItem?.identifier || ''" :rows="3" placeholder="Describe the asset..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Type</label>
                        <select v-model="editForm.asset_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="">None</option>
                          <option v-for="t in assetTypes" :key="t.key" :value="t.key">{{ t.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Status</label>
                        <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="draft">Draft</option>
                          <option value="open">Open</option>
                          <option value="archived">Archived</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Owner</label>
                        <MemberPicker v-model="editForm.owner" :members="orgMembers" placeholder="Select owner..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Primary Location</label>
                        <input v-model="editForm.primary_location" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="e.g. Reykjavík DC, AWS eu-west-1" />
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Name</div>
                        <div class="text-sm text-slate-200 font-medium">{{ selectedItem.name }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Description</div>
                        <div v-if="selectedItem.description" class="text-sm text-slate-300 leading-relaxed doc-prose" v-html="renderMd(selectedItem.description)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>
                      <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Status</div>
                          <StatusBadge :status="selectedItem.status" />
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Type</div>
                          <div class="text-sm text-slate-300 capitalize">{{ formatLabel(selectedItem.asset_type) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Owner</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedItem.owner) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Primary Location</div>
                          <div class="text-sm text-slate-300">{{ selectedItem.primary_location || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Next Review</div>
                          <div class="text-sm" :class="isOverdue(selectedItem.next_review) ? 'text-red-400' : 'text-slate-300'">
                            {{ formatDate(selectedItem.next_review) || '—' }}
                          </div>
                        </div>
                        <div v-if="selectedItem.created_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created</div>
                          <div class="text-sm text-slate-300">{{ formatDate(selectedItem.created_at) }}</div>
                        </div>
                        <div v-if="selectedItem.created_by">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created by</div>
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
                    Review overdue since {{ formatDate(selectedItem.next_review) }}
                  </div>

                  <!-- CIA scores -->
                  <div class="grid grid-cols-3 gap-4">
                    <div class="bg-slate-800/40 border border-slate-700/50 rounded-lg px-5 py-4 text-center">
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider">Confidentiality</div>
                      <div v-if="selectedItem.confidentiality" class="text-2xl font-bold tabular-nums mt-1.5" :class="ciaColorText(selectedItem.confidentiality)">{{ selectedItem.confidentiality }}</div>
                      <div v-else class="text-2xl text-slate-700 mt-1.5">—</div>
                      <div v-if="selectedItem.confidentiality" class="text-[10px] text-slate-500 mt-0.5">{{ ciaLabel(selectedItem.confidentiality) }}</div>
                    </div>
                    <div class="bg-slate-800/40 border border-slate-700/50 rounded-lg px-5 py-4 text-center">
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider">Integrity</div>
                      <div v-if="selectedItem.integrity" class="text-2xl font-bold tabular-nums mt-1.5" :class="ciaColorText(selectedItem.integrity)">{{ selectedItem.integrity }}</div>
                      <div v-else class="text-2xl text-slate-700 mt-1.5">—</div>
                      <div v-if="selectedItem.integrity" class="text-[10px] text-slate-500 mt-0.5">{{ ciaLabel(selectedItem.integrity) }}</div>
                    </div>
                    <div class="bg-slate-800/40 border border-slate-700/50 rounded-lg px-5 py-4 text-center">
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider">Availability</div>
                      <div v-if="selectedItem.availability" class="text-2xl font-bold tabular-nums mt-1.5" :class="ciaColorText(selectedItem.availability)">{{ selectedItem.availability }}</div>
                      <div v-else class="text-2xl text-slate-700 mt-1.5">—</div>
                      <div v-if="selectedItem.availability" class="text-[10px] text-slate-500 mt-0.5">{{ ciaLabel(selectedItem.availability) }}</div>
                    </div>
                  </div>

                  <div class="flex gap-4 text-[10px] text-slate-500 flex-wrap">
                    <span v-if="selectedItem.last_review">Last review: {{ formatDate(selectedItem.last_review) }}</span>
                    <span v-if="selectedItem.next_review && !isOverdue(selectedItem.next_review)">Next review: {{ formatDate(selectedItem.next_review) }}</span>
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
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Notes</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('notes')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                  </div>
                  <template v-if="editingSection === 'notes'">
                    <MarkdownField v-model="editForm.notes" :self-type="'asset'" :self-id="selectedItem?.identifier || ''" :rows="12" placeholder="Additional notes... Type /doc to link a document" />
                  </template>
                  <template v-else>
                    <div v-if="selectedItem.notes" class="text-sm doc-prose text-slate-300 leading-relaxed" v-html="renderMd(selectedItem.notes)"></div>
                    <div v-else class="text-sm text-slate-600 italic">No notes yet.</div>
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
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">Danger zone</div>
                    <div class="text-xs text-slate-400">Deleting this asset is permanent and cannot be undone. History, comments, and linked references will be lost.</div>
                    <button @click="deleteSelectedItem" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                      Delete asset
                    </button>
                  </div>
                </div>
              </template>

            </div>
          </div>

          <!-- Footer action bar (edit mode only) -->
          <div v-if="editingSection" class="flex-shrink-0 border-t border-slate-800 px-6 py-3 flex justify-end gap-3">
            <button @click="cancelSection" class="px-4 py-1.5 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
            <button @click="saveSection" :disabled="saving" class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 text-white text-sm font-medium rounded-lg transition-colors">{{ saving ? 'Saving...' : 'Save' }}</button>
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
import { api } from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import MemberPicker from '../components/MemberPicker.vue'
import MarkdownField from '../components/MarkdownField.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
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

const { confirm: confirmDialog } = useConfirm()
const { show: showError, success: showSaved } = useToast()

const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()

const renderMd = renderMarkdown

const userRole = ref('')
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')

const loading = ref(true)
const error = ref(null)
const assets = ref([])
const orgMembers = ref([])
const saving = ref(false)
const stats = ref({ total: 0, draft: 0, open: 0, archived: 0, critical: 0 })

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

const assetTypes = [
  { key: 'infrastructure', label: 'Infrastructure' },
  { key: 'processing_devices', label: 'Processing Devices' },
  { key: 'software', label: 'Software' },
  { key: 'system', label: 'System' },
  { key: 'network', label: 'Network' },
  { key: 'service', label: 'Service' },
  { key: 'financial_info', label: 'Financial Info' },
  { key: 'personal_data', label: 'Personal Data' },
  { key: 'ipr', label: 'Intellectual Property' },
  { key: 'sales_marketing', label: 'Sales & Marketing' },
  { key: 'processing_facility', label: 'Processing Facility' },
  { key: 'products_services', label: 'Products & Services' },
  { key: 'supply_chain', label: 'Supply Chain' },
  { key: 'other', label: 'Other' },
]

const detailTabs = [
  { key: 'overview', label: 'Overview' },
  { key: 'assessment', label: 'Assessment' },
  { key: 'notes', label: 'Notes' },
  { key: 'links', label: 'Links' },
  { key: 'suggestions', label: 'Suggestions' },
  { key: 'comments', label: 'Comments' },
  { key: 'history', label: 'History' },
]

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

const ciaLabel = (v) => ['Not Assessed','Insignificant','Minor','Moderate','Major','Severe'][v] || 'N/A'

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

function formatLabel(s) {
  return (s || '').replace(/_/g, ' ')
}

function formatDate(dateStr) {
  if (!dateStr && dateStr !== 0) return ''
  const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
  return d.toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric' })
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
    error.value = e.message
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
    showSaved('Saved')
  } catch (e) {
    showError('Failed to save: ' + e.message)
  } finally {
    saving.value = false
  }
}

async function selectItem(item) {
  if (selectedItem.value?.id === item.id) {
    closeDetail()
    return
  }
  router.push(orgPath(`/assets/${item.id}`))
}

async function openItemFromRoute(id) {
  const numId = parseInt(id)
  let item = assets.value.find(a => a.id === numId)
  if (!item) {
    try { item = await api.fetchJSON(`/api/v1/assets/${numId}`) } catch { return }
  }
  if (!item) return
  selectedItem.value = item
  detailTab.value = 'overview'
  startEdit(item)
}

async function switchDetailTab(key) {
  if (editingSection.value && isDirty()) {
    const ok = await confirmDialog({
      message: 'You have unsaved changes. Discard and switch tab?',
      variant: 'danger',
      confirmLabel: 'Discard',
    })
    if (!ok) return
  }
  detailTab.value = key
  editingSection.value = ''
}

async function closeDetail() {
  if (editingSection.value && isDirty()) {
    const ok = await confirmDialog({
      message: 'You have unsaved changes. Discard and close?',
      variant: 'danger',
      confirmLabel: 'Discard',
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
    showError('Failed to create asset: ' + e.message)
  }
}

async function deleteSelectedItem() {
  if (!selectedItem.value) return
  const ok = await confirmDialog({ message: 'Delete this asset? This cannot be undone.', variant: 'danger', confirmLabel: 'Delete' })
  if (!ok) return
  try {
    await api.deleteJSON(`/api/v1/assets/${selectedItem.value.id}`)
    closeDetail()
    await loadAssets()
  } catch (e) {
    showError('Failed to delete: ' + e.message)
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
    error.value = e.message
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
  if (selectedItem.value?.id === parseInt(id)) return
  openItemFromRoute(id)
})
</script>
