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
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">Systems Register</h1>
          <p class="text-sm text-slate-500 mt-1">IT systems, recovery objectives, access control, and supplier links</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canWrite" @click="showCreateForm = !showCreateForm"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            {{ showCreateForm ? 'Cancel' : 'Add System' }}
          </button>
          <SuggestNewButton entityType="system" typeLabel="System" />
        </div>
      </div>

      <!-- Create form (compact modal) -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreateForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreateForm = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
          <div class="flex items-center justify-between mb-2">
            <h2 class="text-sm font-semibold text-slate-200">Add System</h2>
            <button @click="showCreateForm = false" class="text-slate-500 hover:text-slate-300">
              <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <div class="space-y-3">
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">Name *</label>
              <input v-model="newItem.name" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="e.g. Production ERP" />
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">Classification</label>
              <div class="flex flex-wrap gap-1.5">
                <button v-for="t in classifications" :key="t.key"
                  @click="newItem.classification = t.key"
                  class="px-2.5 py-1 text-[11px] font-medium rounded-lg border transition-colors"
                  :class="newItem.classification === t.key ? 'bg-blue-600/20 text-blue-400 border-blue-500/40' : 'bg-slate-800 text-slate-400 border-slate-700 hover:border-slate-600'">
                  {{ t.label }}
                </button>
              </div>
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">Criticality</label>
              <div class="flex flex-wrap gap-1.5">
                <button v-for="c in criticalityLevels" :key="c.key"
                  @click="newItem.criticality = c.key"
                  class="px-2.5 py-1 text-[11px] font-medium rounded-lg border transition-colors"
                  :class="newItem.criticality === c.key ? criticalityChipColor(c.key) : 'bg-slate-800 text-slate-400 border-slate-700 hover:border-slate-600'">
                  {{ c.label }}
                </button>
              </div>
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">Supplier</label>
              <select v-model="newItem.supplier_id" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                <option :value="null">— None —</option>
                <option v-for="s in suppliers" :key="s.id" :value="s.id">{{ s.name }}</option>
              </select>
            </div>
          </div>
          <div class="text-[10px] text-slate-600 mt-1">You can add CIA classification, RPO/RTO, and owner after creating.</div>
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
        <button @click="filterStatus = 'active'" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="filterStatus === 'active' ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-emerald-400 tabular-nums">{{ stats.active }}</div>
          <div class="text-xs text-slate-500 mt-1">Active</div>
        </button>
        <button @click="filterStatus = 'under_review'" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="filterStatus === 'under_review' ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-amber-400 tabular-nums">{{ stats.under_review }}</div>
          <div class="text-xs text-slate-500 mt-1">Under Review</div>
        </button>
        <button @click="filterStatus = 'decommissioned'" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="filterStatus === 'decommissioned' ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-slate-400 tabular-nums">{{ stats.decommissioned || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Decommissioned</div>
        </button>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold tabular-nums" :class="stats.critical > 0 ? 'text-red-400' : 'text-slate-100'">{{ stats.critical }}</div>
          <div class="text-xs text-slate-500 mt-1">Critical</div>
        </div>
      </div>

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
          <select v-model="filterCriticality" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">All criticality</option>
            <option v-for="c in criticalityLevels" :key="c.key" :value="c.key">{{ c.label }}</option>
          </select>
          <select v-model="filterStatus" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">All statuses</option>
            <option value="active">Active</option>
            <option value="under_review">Under Review</option>
            <option value="decommissioned">Decommissioned</option>
          </select>
          <button v-if="filterCriticality || filterStatus || searchQuery"
            @click="filterCriticality = ''; filterStatus = ''; searchQuery = ''"
            class="text-[10px] text-slate-600 hover:text-slate-400 transition-colors">
            Clear
          </button>
          <div class="ml-auto text-xs text-slate-500 tabular-nums">
            {{ total }} total
          </div>
        </div>
      </div>

      <!-- Empty state -->
      <div v-if="systems.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
        <div class="text-sm text-slate-500">No systems found</div>
      </div>

      <!-- Table -->
      <div v-else class="bg-slate-900 border border-slate-800 rounded-xl overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="border-b border-slate-800">
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">System</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Classification</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Criticality</th>
              <th class="text-center px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">C/I/A</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">RPO</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">RTO</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Status</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Owner</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Next Review</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/50">
            <tr v-for="sys in systems" :key="sys.id"
              @click="selectItem(sys)"
              class="hover:bg-slate-800/50 transition-colors cursor-pointer"
              :class="isOverdue(sys.next_review) ? 'bg-amber-950/10' : ''">
              <td class="px-5 py-3.5">
                <div class="text-sm font-medium text-slate-200">{{ sys.name }}</div>
              </td>
              <td class="px-5 py-3.5"><StatusBadge :status="sys.classification" /></td>
              <td class="px-5 py-3.5">
                <span class="inline-block px-2 py-0.5 rounded text-[10px] font-medium" :class="criticalityColor(sys.criticality)">{{ sys.criticality }}</span>
              </td>
              <td class="px-5 py-3.5 text-center">
                <div class="flex gap-0.5 justify-center">
                  <span v-if="sys.confidentiality > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(sys.confidentiality)" :title="'Confidentiality: ' + ciaLabel(sys.confidentiality)">C{{ sys.confidentiality }}</span>
                  <span v-if="sys.integrity > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(sys.integrity)" :title="'Integrity: ' + ciaLabel(sys.integrity)">I{{ sys.integrity }}</span>
                  <span v-if="sys.availability > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(sys.availability)" :title="'Availability: ' + ciaLabel(sys.availability)">A{{ sys.availability }}</span>
                  <span v-if="!sys.confidentiality && !sys.integrity && !sys.availability" class="text-slate-600 text-xs">-</span>
                </div>
              </td>
              <td class="px-5 py-3.5 text-sm tabular-nums" :class="rpoColor(sys.rpo_hours)">{{ sys.rpo_hours || 0 }}h</td>
              <td class="px-5 py-3.5 text-sm tabular-nums" :class="rtoColor(sys.rto_hours)">{{ sys.rto_hours || 0 }}h</td>
              <td class="px-5 py-3.5">
                <StatusBadge :status="sys.status" />
              </td>
              <td class="px-5 py-3.5 text-sm text-slate-400">{{ resolveUserName(sys.owner) }}</td>
              <td class="px-5 py-3.5 text-sm" :class="isOverdue(sys.next_review) ? 'text-amber-400 font-medium' : 'text-slate-500'">
                {{ formatDate(sys.next_review) || '-' }}
              </td>
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
            <div class="flex items-center gap-2 flex-shrink-0">
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
                        <MarkdownField v-model="editForm.description" :self-type="'system'" :self-id="selectedItem?.identifier || ''" :rows="2" placeholder="Describe the system..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Classification</label>
                        <select v-model="editForm.classification" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="c in classifications" :key="c.key" :value="c.key">{{ c.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Criticality</label>
                        <select v-model="editForm.criticality" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="c in criticalityLevels" :key="c.key" :value="c.key">{{ c.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Status</label>
                        <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="active">Active</option>
                          <option value="under_review">Under Review</option>
                          <option value="decommissioned">Decommissioned</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Owner</label>
                        <MemberPicker v-model="editForm.owner" :members="orgMembers" placeholder="Select owner..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Department</label>
                        <input v-model="editForm.department" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="IT, Engineering, Marketing..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Supplier</label>
                        <select v-model="editForm.supplier_id" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option :value="null">— None —</option>
                          <option v-for="s in suppliers" :key="s.id" :value="s.id">{{ s.name }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">RPO (hours)</label>
                        <input v-model.number="editForm.rpo_hours" type="number" min="0" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                        <div class="text-[10px] text-slate-600 mt-1">Max acceptable data loss (hours)</div>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">RTO (hours)</label>
                        <input v-model.number="editForm.rto_hours" type="number" min="0" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                        <div class="text-[10px] text-slate-600 mt-1">Max acceptable downtime (hours)</div>
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Description</div>
                        <div v-if="selectedItem.description" class="text-sm text-slate-300 leading-relaxed doc-prose" v-html="renderMd(selectedItem.description)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>
                      <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Classification</div>
                          <StatusBadge :status="selectedItem.classification" />
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Criticality</div>
                          <span class="inline-block px-2 py-0.5 rounded text-[10px] font-medium" :class="criticalityColor(selectedItem.criticality)">{{ selectedItem.criticality }}</span>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Status</div>
                          <StatusBadge :status="selectedItem.status" />
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Owner</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedItem.owner) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Department</div>
                          <div class="text-sm text-slate-300">{{ selectedItem.department || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Supplier</div>
                          <div class="text-sm text-slate-300">{{ supplierName(selectedItem.supplier_id) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">RPO / RTO</div>
                          <div class="text-sm text-slate-300">
                            <span class="tabular-nums" :class="rpoColor(selectedItem.rpo_hours)">{{ selectedItem.rpo_hours || 0 }}h</span>
                            <span class="text-slate-600 mx-1">/</span>
                            <span class="tabular-nums" :class="rtoColor(selectedItem.rto_hours)">{{ selectedItem.rto_hours || 0 }}h</span>
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
                    <ReadingsPanel entityType="system" :entityId="selectedItem.id" :identifier="selectedItem.identifier || ('SYSTEM-' + selectedItem.id)" :canWrite="canWrite"
                      :currentValues="{ confidentiality: selectedItem.confidentiality, integrity: selectedItem.integrity, availability: selectedItem.availability, criticality: selectedItem.criticality }"
                      @saved="refreshSelectedItem" />
                  </div>
                </div>
              </template>

              <!-- ═══ REVIEWS (periodic access reviews) ═══ -->
              <template v-if="detailTab === 'reviews'">
                <div class="px-6 py-5 space-y-4">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Access Reviews</div>
                    <button v-if="canWrite" @click="showAddReview = !showAddReview" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">
                      {{ showAddReview ? 'Cancel' : '+ Record review' }}
                    </button>
                  </div>

                  <!-- Add review form -->
                  <div v-if="showAddReview" class="bg-slate-800/50 rounded-lg p-4 space-y-3">
                    <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
                      <div>
                        <label class="block text-[10px] font-medium text-slate-500 mb-1">Users Added</label>
                        <input v-model.number="reviewForm.users_added" type="number" min="0" class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div>
                        <label class="block text-[10px] font-medium text-slate-500 mb-1">Users Removed</label>
                        <input v-model.number="reviewForm.users_removed" type="number" min="0" class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="col-span-2">
                        <label class="block text-[10px] font-medium text-slate-500 mb-1">Notes</label>
                        <input v-model="reviewForm.notes" class="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="Removed former employees..." />
                      </div>
                    </div>
                    <button @click="addAccessReview" :disabled="reviewSaving" class="px-3 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 text-white text-xs font-medium rounded-lg transition-colors">
                      {{ reviewSaving ? 'Saving...' : 'Record Review' }}
                    </button>
                  </div>

                  <!-- Review history -->
                  <div v-if="accessReviews.length > 0" class="space-y-2">
                    <div v-for="ar in accessReviews" :key="ar.id" class="flex items-center justify-between bg-slate-800/30 rounded-lg px-4 py-2">
                      <div>
                        <div class="text-xs text-slate-300">{{ ar.reviewed_by }} <span class="text-slate-600">reviewed</span></div>
                        <div class="text-[10px] text-slate-500 mt-0.5">
                          {{ formatDate(ar.reviewed_at) }}
                          <span v-if="ar.users_added || ar.users_removed" class="ml-2">
                            <span v-if="ar.users_added" class="text-green-400">+{{ ar.users_added }}</span>
                            <span v-if="ar.users_removed" class="text-red-400 ml-1">-{{ ar.users_removed }}</span>
                          </span>
                        </div>
                        <div v-if="ar.notes" class="text-[10px] text-slate-500 mt-0.5">{{ ar.notes }}</div>
                      </div>
                    </div>
                  </div>
                  <div v-else class="text-xs text-slate-600 italic">No access reviews recorded.</div>
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
                    <MarkdownField v-model="editForm.notes" :self-type="'system'" :self-id="selectedItem?.identifier || ''" :rows="12" placeholder="Additional notes... Type /doc to link a document" />
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
                  <ReferenceManager entityType="system" :entityId="selectedItem.identifier || ('SYSTEM-' + selectedItem.id)" :editable="canWrite" />
                </div>
              </template>

              <!-- ═══ SUGGESTIONS ═══ -->
              <template v-if="detailTab === 'suggestions'">
                <div class="px-6 py-5">
                  <SuggestionPanel entityType="system" :entityId="selectedItem.identifier" :canReview="canWrite" @applied="loadSystems" />
                </div>
              </template>

              <!-- ═══ COMMENTS ═══ -->
              <template v-if="detailTab === 'comments'">
                <div class="px-6 py-5">
                  <CommentsPanel entityType="system" :entityId="selectedItem.identifier" />
                </div>
              </template>

              <!-- ═══ HISTORY ═══ -->
              <template v-if="detailTab === 'history'">
                <div class="px-6 py-5 space-y-6">
                  <HistoryPanel entityType="system" :entityId="String(selectedItem.id)" />
                  <div v-if="canWrite" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">Danger zone</div>
                    <div class="text-xs text-slate-400">Deleting this system is permanent and cannot be undone. History, comments, access reviews and linked references will be lost.</div>
                    <button @click="deleteSelectedItem" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                      Delete system
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
const systems = ref([])
const suppliers = ref([])
const orgMembers = ref([])
const saving = ref(false)
const stats = ref({ total: 0, active: 0, under_review: 0, decommissioned: 0, critical: 0, high: 0, medium: 0, low: 0 })

const selectedItem = ref(null)
const detailTab = ref('overview')
const editingSection = ref('')
const editForm = ref({})
const { capture: captureEditSnapshot, isDirty } = useDirtyEdit(editForm)

const showCreateForm = ref(false)
const newItem = ref({ name: '', classification: 'confidential', criticality: 'medium', supplier_id: null })

const searchQuery = ref('')
const filterCriticality = ref('')
const filterStatus = ref('')
const page = ref(1)
const pageSize = ref(50)
const total = ref(0)

// Access reviews (Reviews tab)
const accessReviews = ref([])
const showAddReview = ref(false)
const reviewForm = ref({ users_added: 0, users_removed: 0, notes: '' })
const reviewSaving = ref(false)

const classifications = [
  { key: 'public', label: 'Public' },
  { key: 'internal', label: 'Internal' },
  { key: 'confidential', label: 'Confidential' },
  { key: 'restricted', label: 'Restricted' },
]

const criticalityLevels = [
  { key: 'critical', label: 'Critical' },
  { key: 'high', label: 'High' },
  { key: 'medium', label: 'Medium' },
  { key: 'low', label: 'Low' },
]

const detailTabs = [
  { key: 'overview', label: 'Overview' },
  { key: 'assessment', label: 'Assessment' },
  { key: 'reviews', label: 'Access Reviews' },
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
  searchTimer = setTimeout(() => { page.value = 1; loadSystems() }, 250)
})
watch([filterCriticality, filterStatus], () => {
  page.value = 1
  loadSystems()
})
watch([page, pageSize], () => loadSystems())

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

function criticalityColor(c) {
  switch (c) {
    case 'critical': return 'bg-red-900/60 text-red-300'
    case 'high': return 'bg-orange-900/60 text-orange-300'
    case 'medium': return 'bg-amber-900/60 text-amber-300'
    case 'low': return 'bg-emerald-900/60 text-emerald-300'
    default: return 'bg-slate-800 text-slate-500'
  }
}

function criticalityChipColor(c) {
  switch (c) {
    case 'critical': return 'bg-red-600/20 text-red-400 border-red-500/40'
    case 'high': return 'bg-orange-600/20 text-orange-400 border-orange-500/40'
    case 'medium': return 'bg-amber-600/20 text-amber-400 border-amber-500/40'
    case 'low': return 'bg-emerald-600/20 text-emerald-400 border-emerald-500/40'
    default: return 'bg-blue-600/20 text-blue-400 border-blue-500/40'
  }
}

function rpoColor(hours) {
  if (!hours) return 'text-slate-500'
  if (hours <= 4) return 'text-red-400'
  if (hours <= 12) return 'text-amber-400'
  if (hours <= 24) return 'text-yellow-300'
  return 'text-slate-400'
}

function rtoColor(hours) {
  if (!hours) return 'text-slate-500'
  if (hours <= 4) return 'text-red-400'
  if (hours <= 12) return 'text-amber-400'
  if (hours <= 48) return 'text-blue-400'
  return 'text-slate-400'
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

function supplierName(id) {
  if (!id) return '—'
  const s = suppliers.value.find(sup => sup.id === id)
  return s?.name || '—'
}

async function loadSystems() {
  try {
    const params = new URLSearchParams()
    params.set('page', String(page.value))
    params.set('limit', String(pageSize.value))
    if (searchQuery.value) params.set('q', searchQuery.value)
    if (filterCriticality.value) params.set('criticality', filterCriticality.value)
    if (filterStatus.value) params.set('status', filterStatus.value)
    const res = await api.fetchRaw(`/api/v1/systems?${params.toString()}`)
    systems.value = Array.isArray(res?.data) ? res.data : []
    total.value = res?.total || 0
    loadStats()
  } catch (e) {
    error.value = e.message
  }
}

async function loadStats() {
  try { stats.value = await api.fetchJSON('/api/v1/systems/stats') || stats.value } catch { /* silent */ }
}

async function refreshSelectedItem() {
  if (!selectedItem.value) return
  const id = selectedItem.value.id
  try {
    const fresh = await api.fetchJSON(`/api/v1/systems/${id}`)
    if (fresh) selectedItem.value = fresh
  } catch { /* silent */ }
  loadSystems()
}

async function loadAccessReviews() {
  if (!selectedItem.value) {
    accessReviews.value = []
    return
  }
  try {
    const res = await api.getAccessReviews(selectedItem.value.id)
    accessReviews.value = Array.isArray(res) ? res : (res?.data || [])
  } catch {
    accessReviews.value = []
  }
}

async function addAccessReview() {
  if (!selectedItem.value) return
  reviewSaving.value = true
  try {
    await api.createAccessReview(selectedItem.value.id, reviewForm.value)
    reviewForm.value = { users_added: 0, users_removed: 0, notes: '' }
    showAddReview.value = false
    await loadAccessReviews()
    await refreshSelectedItem()
  } catch (e) {
    showError('Failed to record review: ' + e.message)
  } finally {
    reviewSaving.value = false
  }
}

function startEdit(item) {
  editForm.value = {
    name: item.name || '',
    description: item.description || '',
    classification: item.classification || 'confidential',
    criticality: item.criticality || 'medium',
    status: item.status || 'active',
    owner: item.owner || '',
    department: item.department || '',
    supplier_id: item.supplier_id || null,
    rpo_hours: item.rpo_hours || 0,
    rto_hours: item.rto_hours || 0,
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
    await api.putJSON(`/api/v1/systems/${selectedItem.value.id}`, { ...editForm.value })
    await refreshSelectedItem()
    editingSection.value = ''
    showSaved('Saved')
  } catch (e) {
    showError('Failed to save: ' + e.message)
  } finally {
    saving.value = false
  }
}

async function changeSystemStatus(newStatus) {
  if (!selectedItem.value) return
  try {
    await api.putJSON(`/api/v1/systems/${selectedItem.value.id}`, { status: newStatus })
    selectedItem.value.status = newStatus
    await refreshSelectedItem()
    await loadStats()
    showSaved('Status updated')
  } catch (e) {
    showError('Failed to update status: ' + (e.message || 'unknown error'))
  }
}

async function selectItem(item) {
  if (selectedItem.value?.id === item.id) {
    closeDetail()
    return
  }
  router.push(orgPath(`/systems/${item.id}`))
}

async function openItemFromRoute(id) {
  const numId = parseInt(id)
  let item = systems.value.find(s => s.id === numId)
  if (!item) {
    try { item = await api.fetchJSON(`/api/v1/systems/${numId}`) } catch { return }
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
  router.push(orgPath('/systems'))
}

async function createItem() {
  try {
    const payload = { ...newItem.value }
    const created = await api.postJSON('/api/v1/systems', payload)
    showCreateForm.value = false
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur()
    newItem.value = { name: '', classification: 'confidential', criticality: 'medium', supplier_id: null }
    await loadSystems()
    if (created?.id && !systems.value.find(s => s.id === created.id)) {
      systems.value = [created, ...systems.value]
    }
    // Drop user into detail modal in edit mode on Overview to keep filling things in.
    if (created && created.id) {
      let fresh = created
      try { fresh = await api.fetchJSON(`/api/v1/systems/${created.id}`) } catch { /* fall back */ }
      selectedItem.value = fresh
      detailTab.value = 'overview'
      startEdit(fresh)
      editingSection.value = 'overview'
      router.push(orgPath(`/systems/${fresh.id}`))
    }
  } catch (e) {
    showError('Failed to create system: ' + e.message)
  }
}

async function deleteSelectedItem() {
  if (!selectedItem.value) return
  const ok = await confirmDialog({ message: 'Delete this system? This cannot be undone.', variant: 'danger', confirmLabel: 'Delete' })
  if (!ok) return
  try {
    await api.deleteJSON(`/api/v1/systems/${selectedItem.value.id}`)
    closeDetail()
    await loadSystems()
  } catch (e) {
    showError('Failed to delete: ' + e.message)
  }
}

watch(selectedItem, () => {
  loadAccessReviews()
})

onMounted(async () => {
  try { const me = await api.getMe(); userRole.value = me?.role || '' } catch {}
  try {
    const [, sup, users] = await Promise.all([
      loadSystems(),
      api.getSuppliers().catch(() => []),
      api.getUsers().catch(() => []),
    ])
    suppliers.value = sup || []
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
