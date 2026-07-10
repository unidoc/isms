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
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">Legal Register</h1>
          <p class="text-sm text-slate-500 mt-1">Applicable legislation and regulatory risk assessment</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canWrite" @click="showCreateForm = !showCreateForm"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            {{ showCreateForm ? 'Cancel' : 'Add Legal Requirement' }}
          </button>
          <SuggestNewButton entityType="legal_requirement" typeLabel="Legal Requirement" />
        </div>
      </div>

      <!-- Create form (modal) -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreateForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreateForm = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
        <div class="flex items-center justify-between mb-2">
          <h2 class="text-sm font-semibold text-slate-200">Add Legal Requirement</h2>
          <button @click="showCreateForm = false" class="text-slate-500 hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="space-y-3">
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Title *</label>
            <input v-model="newItem.title" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="e.g. GDPR, NIS2 Directive" />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Category</label>
            <div class="flex flex-wrap gap-1.5">
              <button v-for="cat in legalCategories" :key="cat.key"
                @click="newItem.category = newItem.category === cat.key ? '' : cat.key"
                class="px-2.5 py-1 text-[11px] font-medium rounded-lg border transition-colors"
                :class="newItem.category === cat.key ? 'bg-blue-600/20 text-blue-400 border-blue-500/40' : 'bg-slate-800 text-slate-400 border-slate-700 hover:border-slate-600'"
                :title="newItem.category === cat.key ? 'Click to deselect' : ''">
                {{ cat.label }}
              </button>
            </div>
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">You can add jurisdiction, owner, treatment and more after creating.</div>

        <!-- Footer -->
        <div class="flex justify-end gap-3 pt-3 border-t border-slate-800">
          <button @click="showCreateForm = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
          <button @click="createItem" :disabled="!newItem.title" class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
            Add
          </button>
        </div>
      </div>
      </div>
      </Transition>
      </Teleport>

      <!-- Summary strip -->
      <StatStrip :stats="statusStats" v-model="filterStatus" />

      <!-- Risk Map (collapsible) -->
      <details class="group">
        <summary class="flex items-center gap-2 cursor-pointer text-sm font-semibold text-slate-400 uppercase tracking-wider select-none">
          <svg class="w-4 h-4 transition-transform group-open:rotate-90" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
          </svg>
          Risk Map
        </summary>
        <div class="mt-3">
          <HeatMap :items="heatMapItems" title="Compliance Risk Map" />
        </div>
      </details>

      <!-- Search + Filters -->
      <div class="space-y-3">
        <div class="flex flex-wrap items-center gap-3">
          <div class="relative flex-1 max-w-xs">
            <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
            </svg>
            <input v-model="searchQuery" type="text" placeholder="Search..."
              class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
          </div>
          <select v-model="filterLevel" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">All levels</option>
            <option value="critical">Critical</option>
            <option value="high">High</option>
            <option value="medium">Medium</option>
            <option value="low">Low</option>
          </select>
          <select v-model="filterCategory" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">All categories</option>
            <option v-for="cat in legalCategories" :key="cat.key" :value="cat.key">{{ cat.label }}</option>
          </select>
          <select v-model="filterStatus" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">All statuses</option>
            <option value="draft">Draft</option>
            <option value="open">Open</option>
            <option value="closed">Closed</option>
          </select>
          <button v-if="filterLevel || filterCategory || filterStatus || searchQuery"
            @click="filterLevel = ''; filterCategory = ''; filterStatus = ''; searchQuery = ''"
            class="text-[10px] text-slate-600 hover:text-slate-400 transition-colors">
            Clear
          </button>
          <div class="ml-auto text-xs text-slate-500 tabular-nums">
            {{ total }} total
          </div>
        </div>
      </div>

      <!-- Empty state -->
      <div v-if="items.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
        <div class="text-sm text-slate-500">No legal requirements found</div>
      </div>

      <!-- Table -->
      <div v-else class="bg-slate-900 border border-slate-800 rounded-xl overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="border-b border-slate-800">
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Title</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Jurisdiction</th>
              <th class="text-center px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Score</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Treatment</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Status</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Owner</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/50">
            <tr v-for="item in items" :key="item.id"
              @click="selectItem(item)"
              class="hover:bg-slate-800/50 transition-colors cursor-pointer">
              <td class="px-5 py-3.5">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium text-slate-200">{{ item.title }}</span>
                  <span v-if="isOverdue(item.next_review)" class="px-1.5 py-0.5 rounded text-[9px] font-semibold bg-red-900/50 text-red-400">OVERDUE</span>
                </div>
                <div v-if="item.reference" class="text-xs text-slate-500 mt-0.5">{{ item.reference }}</div>
              </td>
              <td class="px-5 py-3.5">
                <span class="inline-block px-2 py-0.5 rounded text-xs font-medium bg-slate-800 text-slate-400">{{ item.jurisdiction }}</span>
              </td>
              <td class="px-5 py-3.5 text-center">
                <span v-if="item.current_score > 0"
                  class="inline-flex items-center justify-center w-8 h-8 rounded-lg text-sm font-bold tabular-nums"
                  :class="scoreColor(item.current_score)">
                  {{ item.current_score }}
                </span>
                <span v-else class="text-slate-600 text-xs">-</span>
              </td>
              <td class="px-5 py-3.5 text-sm text-slate-400 capitalize">{{ (item.treatment || '-').replace(/_/g, ' ') }}</td>
              <td class="px-5 py-3.5">
                <StatusBadge :status="item.status" />
              </td>
              <td class="px-5 py-3.5 text-sm text-slate-400">{{ resolveUserName(item.owner) }}</td>
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
              <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selectedItem.title }}</h2>
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
                        <label class="block text-xs font-medium text-slate-500 mb-1">Title</label>
                        <input v-model="editForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Description</label>
                        <MarkdownField v-model="editForm.description" :self-type="'legal'" :self-id="selectedItem?.identifier || ''" :rows="3" placeholder="Describe the requirement..." />
                      </div>
                      <div class="relative">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Jurisdiction</label>
                        <input v-model="editForm.jurisdiction"
                          @focus="showJurisdictionPicker = true"
                          @input="showJurisdictionPicker = true"
                          @blur="hideJurisdictionPicker"
                          @keydown.down.prevent="jurisdictionIdx = Math.min(jurisdictionIdx + 1, filteredJurisdictions.length - 1)"
                          @keydown.up.prevent="jurisdictionIdx = Math.max(jurisdictionIdx - 1, 0)"
                          @keydown.tab.prevent="filteredJurisdictions.length && (editForm.jurisdiction = filteredJurisdictions[jurisdictionIdx], showJurisdictionPicker = false)"
                          @keydown.enter.prevent="filteredJurisdictions.length && (editForm.jurisdiction = filteredJurisdictions[jurisdictionIdx], showJurisdictionPicker = false)"
                          @keydown.escape="showJurisdictionPicker = false"
                          class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500"
                          placeholder="Type to search..." />
                        <div v-if="showJurisdictionPicker && filteredJurisdictions.length > 0"
                          class="absolute z-50 left-0 right-0 mt-1 bg-slate-900 border border-slate-700 rounded-lg shadow-xl max-h-48 overflow-y-auto">
                          <button v-for="(j, i) in filteredJurisdictions" :key="j"
                            @mousedown.prevent="editForm.jurisdiction = j; showJurisdictionPicker = false"
                            class="w-full text-left px-3 py-1.5 text-xs transition-colors"
                            :class="i === jurisdictionIdx ? 'bg-blue-600/30 text-white' : 'text-slate-300 hover:bg-slate-700'">
                            {{ j }}
                          </button>
                        </div>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Category</label>
                        <select v-model="editForm.category" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="">None</option>
                          <option v-for="cat in legalCategories" :key="cat.key" :value="cat.key">{{ cat.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Status</label>
                        <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="draft">Draft</option>
                          <option value="open">Open</option>
                          <option value="closed">Closed</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Owner</label>
                        <MemberPicker v-model="editForm.owner" :members="orgMembers" placeholder="Select owner..." />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Reference (article/section)</label>
                        <input v-model="editForm.reference" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="e.g. Article 32, Section 4.2" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">URL</label>
                        <input v-model="editForm.url" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="https://..." />
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Title</div>
                        <div class="text-sm text-slate-200 font-medium">{{ selectedItem.title }}</div>
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
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Owner</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedItem.owner) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Jurisdiction</div>
                          <div class="text-sm text-slate-300">{{ selectedItem.jurisdiction || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Category</div>
                          <div class="text-sm text-slate-300 capitalize">{{ formatLabel(selectedItem.category) || '—' }}</div>
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
                      <div class="pt-1">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Reference</div>
                        <div class="text-sm text-slate-300">{{ selectedItem.reference || '—' }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">URL</div>
                        <a v-if="selectedItem.url" :href="selectedItem.url" target="_blank" rel="noopener" class="text-sm text-blue-400 hover:text-blue-300 break-all">{{ selectedItem.url }}</a>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>
                    </div>
                  </template>
                </div>
              </template>

              <!-- ═══ TREATMENT ═══ -->
              <template v-if="detailTab === 'treatment'">
                <div class="px-6 py-5 space-y-5">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Treatment</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('treatment')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                  </div>
                  <template v-if="editingSection === 'treatment'">
                    <div class="space-y-4">
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Treatment</label>
                        <select v-model="editForm.treatment" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="">Not decided</option><option value="mitigate">Mitigate</option><option value="accept">Accept</option><option value="transfer">Transfer</option><option value="avoid">Avoid</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Treatment Plan</label>
                        <MarkdownField v-model="editForm.treatment_plan" :self-type="'legal'" :self-id="selectedItem?.identifier || ''" :rows="4" placeholder="How are we addressing this requirement? (controls, policies, actions...)" />
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Treatment</div>
                        <div class="text-sm text-slate-300 capitalize">{{ selectedItem.treatment || '—' }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Treatment Plan</div>
                        <div v-if="selectedItem.treatment_plan" class="text-sm doc-prose text-slate-300 leading-relaxed" v-html="renderMd(selectedItem.treatment_plan)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
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
                  <div class="flex items-center gap-4">
                    <div class="flex gap-3 flex-1">
                      <div class="bg-slate-800/60 border border-slate-600/50 rounded-lg px-5 py-3 text-center min-w-[90px] ring-1 ring-slate-600/20">
                        <div class="text-[9px] text-slate-400 uppercase tracking-wider">Current</div>
                        <div v-if="selectedItem.current_score" class="text-xl font-bold tabular-nums mt-1" :class="scoreColor(selectedItem.current_score)">{{ selectedItem.current_score }}</div>
                        <div v-else class="text-xl text-slate-700 mt-1">—</div>
                      </div>
                    </div>
                  </div>
                  <div class="flex gap-4 text-[10px] text-slate-500 flex-wrap">
                    <span v-if="selectedItem.last_review">Last review: {{ formatDate(selectedItem.last_review) }}</span>
                    <span v-if="selectedItem.next_review && !isOverdue(selectedItem.next_review)">Next review: {{ formatDate(selectedItem.next_review) }}</span>
                  </div>
                  <div class="border-t border-slate-800 pt-4">
                    <ReadingsPanel entityType="legal_requirement" :entityId="selectedItem.id" :identifier="selectedItem.identifier || ('LEGAL-' + selectedItem.id)" :canWrite="canWrite"
                      :currentValues="{ current_likelihood: selectedItem.current_likelihood, current_impact: selectedItem.current_impact }"
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
                    <MarkdownField v-model="editForm.notes" :self-type="'legal'" :self-id="selectedItem?.identifier || ''" :rows="12" placeholder="Additional notes, observations, interpretations... Type /doc to link a document" />
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
                  <ReferenceManager entityType="legal_requirement" :entityId="selectedItem.identifier || ('LEGAL-' + selectedItem.id)" :editable="canWrite" />
                </div>
              </template>

              <!-- ═══ SUGGESTIONS ═══ -->
              <template v-if="detailTab === 'suggestions'">
                <div class="px-6 py-5">
                  <SuggestionPanel entityType="legal_requirement" :entityId="selectedItem.identifier" :canReview="canWrite" @applied="loadItems" />
                </div>
              </template>

              <!-- ═══ COMMENTS ═══ -->
              <template v-if="detailTab === 'comments'">
                <div class="px-6 py-5">
                  <CommentsPanel entityType="legal_requirement" :entityId="selectedItem.identifier" />
                </div>
              </template>

              <!-- ═══ HISTORY ═══ -->
              <template v-if="detailTab === 'history'">
                <div class="px-6 py-5 space-y-6">
                  <HistoryPanel entityType="legal_requirement" :entityId="String(selectedItem.id)" />
                  <div v-if="canWrite" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">Danger zone</div>
                    <div class="text-xs text-slate-400">Deleting this legal requirement is permanent and cannot be undone. History, comments, and linked references will be lost.</div>
                    <button @click="deleteSelectedItem" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                      Delete requirement
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
import StatStrip from '../components/StatStrip.vue'
import HeatMap from '../components/HeatMap.vue'
import MemberPicker from '../components/MemberPicker.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
import SuggestionPanel from '../components/SuggestionPanel.vue'
import CommentsPanel from '../components/CommentsPanel.vue'
import HistoryPanel from '../components/HistoryPanel.vue'
import SuggestNewButton from '../components/SuggestNewButton.vue'
import ReadingsPanel from '../components/ReadingsPanel.vue'
import MarkdownField from '../components/MarkdownField.vue'
import Pagination from '../components/Pagination.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import jurisdictions from '../data/countries.js'
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
const items = ref([])
const orgMembers = ref([])
const saving = ref(false)

const selectedItem = ref(null)
const detailTab = ref('overview')
const editingSection = ref('')
const editForm = ref({})
const { capture: captureEditSnapshot, isDirty } = useDirtyEdit(editForm)

const showCreateForm = ref(false)
const newItem = ref({
  title: '', category: '',
})

const searchQuery = ref('')
const filterLevel = ref('')
const filterCategory = ref('')
const filterStatus = ref('')
const page = ref(1)
const pageSize = ref(50)
const total = ref(0)

const showJurisdictionPicker = ref(false)
const jurisdictionIdx = ref(0)
const filteredJurisdictions = computed(() => {
  const q = (editForm.value.jurisdiction || '').toLowerCase()
  const list = jurisdictions.filter(j => !q || j.toLowerCase().includes(q))
  return list.slice(0, 15)
})
watch(() => editForm.value.jurisdiction, () => { jurisdictionIdx.value = 0 })
function hideJurisdictionPicker() { setTimeout(() => { showJurisdictionPicker.value = false }, 200) }

const detailTabs = [
  { key: 'overview', label: 'Overview' },
  { key: 'treatment', label: 'Treatment' },
  { key: 'assessment', label: 'Assessment' },
  { key: 'notes', label: 'Notes' },
  { key: 'links', label: 'Links' },
  { key: 'suggestions', label: 'Suggestions' },
  { key: 'comments', label: 'Comments' },
  { key: 'history', label: 'History' },
]

const legalCategories = [
  { key: 'data_protection', label: 'Data Protection' },
  { key: 'employment', label: 'Employment Law' },
  { key: 'contractual', label: 'Contractual' },
  { key: 'regulatory', label: 'Regulatory Compliance' },
  { key: 'intellectual_property', label: 'Intellectual Property' },
  { key: 'financial', label: 'Financial Regulation' },
  { key: 'environmental', label: 'Environmental' },
  { key: 'corporate', label: 'Corporate Governance' },
]

useModalEscape(showCreateForm)
useModalEscape(computed(() => !!selectedItem.value), closeDetail)

const stats = ref({ total: 0, critical: 0, high: 0, medium: 0, low: 0, not_assessed: 0, open: 0, closed: 0, draft: 0 })
const criticalCount = computed(() => stats.value.critical)
const highCount = computed(() => stats.value.high)
const statusStats = computed(() => [
  { key: '', label: 'Total', count: stats.value.total || items.value.length, color: 'text-slate-100' },
  { key: 'open', label: 'Open', count: stats.value.open || 0, color: 'text-blue-400' },
  { key: 'draft', label: 'Draft', count: stats.value.draft || 0, color: 'text-amber-400' },
  { key: 'closed', label: 'Closed', count: stats.value.closed || 0, color: 'text-slate-400' },
  { key: 'critical', label: 'Critical', count: criticalCount.value, color: criticalCount.value > 0 ? 'text-red-400' : 'text-slate-100', static: true },
  { key: 'high', label: 'High', count: highCount.value, color: highCount.value > 0 ? 'text-orange-400' : 'text-slate-100', static: true },
])
const notAssessedCount = computed(() => stats.value.not_assessed)

const heatMapItems = computed(() => items.value.filter(i => i.current_likelihood > 0 && i.current_impact > 0))

async function loadStats() {
  try { stats.value = await api.fetchJSON('/api/v1/legal/stats') || stats.value } catch { /* silent */ }
}

// Refetch on filter/search change with debounce on text input
let searchTimer = null
watch([searchQuery], () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { page.value = 1; loadItems() }, 250)
})
watch([filterLevel, filterCategory, filterStatus], () => {
  page.value = 1
  loadItems()
})
watch([page, pageSize], () => loadItems())

function scoreColor(score) {
  if (score == null) return 'bg-slate-800 text-slate-500'
  if (score >= 16) return 'bg-red-900/60 text-red-300'
  if (score >= 10) return 'bg-orange-900/60 text-orange-300'
  if (score >= 5) return 'bg-amber-900/60 text-amber-300'
  return 'bg-emerald-900/60 text-emerald-300'
}

function formatLabel(s) { return (s || '').replace(/_/g, ' ') }

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

async function loadItems() {
  try {
    const params = new URLSearchParams()
    params.set('page', String(page.value))
    params.set('limit', String(pageSize.value))
    if (searchQuery.value) params.set('q', searchQuery.value)
    if (filterLevel.value) params.set('level', filterLevel.value)
    if (filterCategory.value) params.set('category', filterCategory.value)
    if (filterStatus.value) params.set('status', filterStatus.value)
    const res = await api.fetchRaw(`/api/v1/legal?${params.toString()}`)
    items.value = Array.isArray(res?.data) ? res.data : []
    total.value = res?.total || 0
    loadStats()
  } catch (e) {
    error.value = e.message
  }
}

async function refreshSelectedItem() {
  if (!selectedItem.value) return
  const id = selectedItem.value.id
  try {
    const fresh = await api.fetchJSON(`/api/v1/legal/${id}`)
    if (fresh) selectedItem.value = fresh
  } catch { /* silent */ }
  // Also refresh the visible page (awaited so callers see fresh data)
  await loadItems()
}

function startEdit(item) {
  editForm.value = {
    title: item.title || '',
    description: item.description || '',
    jurisdiction: item.jurisdiction || 'EU',
    category: item.category || '',
    reference: item.reference || '',
    url: item.url || '',
    status: item.status || 'open',
    owner: item.owner || '',
    treatment: item.treatment || '',
    treatment_plan: item.treatment_plan || '',
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
    await api.putJSON(`/api/v1/legal/${selectedItem.value.id}`, { ...editForm.value })
    await loadItems()
    const fresh = items.value.find(i => i.id === selectedItem.value.id)
    if (fresh) { selectedItem.value = fresh; startEdit(fresh) }
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
  router.push(orgPath(`/legal/${item.id}`))
}

async function openItemFromRoute(id) {
  const numId = parseInt(id)
  // Try current page first; otherwise fetch the item directly so deep links work across pages
  let item = items.value.find(i => i.id === numId || i.identifier === id)
  if (!item) {
    try { item = await api.fetchJSON(`/api/v1/legal/${numId}`) } catch { return }
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
  router.push(orgPath('/legal'))
}

async function createItem() {
  try {
    const payload = { ...newItem.value }
    const created = await api.postJSON('/api/v1/legal', payload)
    showCreateForm.value = false
    // Release focus from the autofocused input so subsequent clicks aren't blocked
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur()
    newItem.value = { title: '', category: '' }
    await loadItems()
    if (created?.id && !items.value.find(i => i.id === created.id)) {
      items.value = [created, ...items.value]
    }
    // Drop user into detail modal in edit mode on Overview to keep filling things in.
    if (created && created.id) {
      let fresh = created
      try { fresh = await api.fetchJSON(`/api/v1/legal/${created.id}`) } catch { /* fall back */ }
      selectedItem.value = fresh
      detailTab.value = 'overview'
      startEdit(fresh)
      editingSection.value = 'overview'
      router.push(orgPath(`/legal/${fresh.id}`))
    }
  } catch (e) {
    showError('Failed to create legal requirement: ' + e.message)
  }
}

async function deleteSelectedItem() {
  if (!selectedItem.value) return
  const ok = await confirmDialog({ message: 'Delete this legal requirement? This cannot be undone.', variant: 'danger', confirmLabel: 'Delete' })
  if (!ok) return
  try {
    await api.deleteJSON(`/api/v1/legal/${selectedItem.value.id}`)
    closeDetail()
    await loadItems()
  } catch (e) {
    showError('Failed to delete: ' + e.message)
  }
}

onMounted(async () => {
  try { const me = await api.getMe(); userRole.value = me?.role || '' } catch {}
  try {
    const [, users] = await Promise.all([
      loadItems(),
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
