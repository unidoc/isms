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
        Failed to load risks. {{ error }}
      </div>
    </div>

    <!-- Main content -->
    <div v-else class="max-w-6xl mx-auto px-8 py-10 space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">Risk Register</h1>
          <p class="text-sm text-slate-500 mt-1">Information security risk assessment and treatment</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canWrite"
            @click="showCreateForm = !showCreateForm"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors"
          >
            {{ showCreateForm ? 'Cancel' : 'Add Risk' }}
          </button>
          <SuggestNewButton entityType="risk" typeLabel="Risk" />
        </div>
      </div>

      <!-- Create form (modal) -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreateForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreateForm = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
        <div class="flex items-center justify-between mb-2">
          <h2 class="text-sm font-semibold text-slate-200">Add Risk</h2>
          <button @click="showCreateForm = false" class="text-slate-500 hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="space-y-3">
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Title *</label>
            <input v-model="newRisk.title" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="e.g. Ransomware attack on production systems" />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Category</label>
            <div class="flex flex-wrap gap-1.5">
              <button v-for="cat in riskCategories" :key="cat.key"
                @click="newRisk.category = newRisk.category === cat.key ? '' : cat.key"
                class="px-2.5 py-1 text-[11px] font-medium rounded-lg border transition-colors"
                :class="newRisk.category === cat.key ? 'bg-blue-600/20 text-blue-400 border-blue-500/40' : 'bg-slate-800 text-slate-400 border-slate-700 hover:border-slate-600'"
                :title="newRisk.category === cat.key ? 'Click to deselect' : ''">
                {{ cat.label }}
              </button>
            </div>
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">You can add description, consequences, treatment, owner and more after creating.</div>

        <!-- Footer -->
        <div class="flex justify-end gap-3 pt-3 border-t border-slate-800">
          <button @click="showCreateForm = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
          <button @click="createRisk" :disabled="!newRisk.title" class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
            Add
          </button>
        </div>
      </div>
      </div>
      </Transition>
      </Teleport>


      <!-- Summary cards -->
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-slate-100 tabular-nums">{{ risks.length }}</div>
          <div class="text-xs text-slate-500 mt-1">Total Risks</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold tabular-nums" :class="criticalCount > 0 ? 'text-red-400' : 'text-slate-100'">{{ criticalCount }}</div>
          <div class="text-xs text-slate-500 mt-1">Critical</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold tabular-nums" :class="highCount > 0 ? 'text-orange-400' : 'text-slate-100'">{{ highCount }}</div>
          <div class="text-xs text-slate-500 mt-1">High</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-emerald-400 tabular-nums">{{ treatedCount }}</div>
          <div class="text-xs text-slate-500 mt-1">Closed</div>
        </div>
      </div>

      <!-- Heat Map (collapsible) -->
      <details class="group">
        <summary class="flex items-center gap-2 cursor-pointer text-sm font-semibold text-slate-400 uppercase tracking-wider select-none">
          <svg class="w-4 h-4 transition-transform group-open:rotate-90" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
          </svg>
          Risk Map
        </summary>
        <div class="mt-3">
          <HeatMap :items="allRisksForMap" title="Risk Map" @cell-click="onHeatMapClick" />
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
            <option v-for="cat in riskCategories" :key="cat.key" :value="cat.key">{{ cat.label }}</option>
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
      <div v-if="risks.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
        <div v-if="filterLevel || filterCategory || filterStatus || searchQuery" class="text-sm text-slate-500">
          No risks match your filter.
        </div>
        <div v-else class="text-sm text-slate-500">
          No risks yet — click Add to create your first one.
        </div>
      </div>

      <!-- Table -->
      <div v-else class="bg-slate-900 border border-slate-800 rounded-xl overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="border-b border-slate-800">
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Risk</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Type</th>
              <th class="text-center px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Score</th>
              <th class="text-center px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">C/I/A</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Owner</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Treatment</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Status</th>
              <th class="text-right px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/50">
            <template v-for="risk in risks" :key="risk.id || risk.identifier">
              <tr
                @click="selectRisk(risk)"
                class="hover:bg-slate-800/50 transition-colors cursor-pointer"
              >
                <td class="px-5 py-3.5">
                  <div class="flex items-center gap-2">
                    <span class="text-sm font-medium text-slate-200">{{ risk.title || risk.risk_id }}</span>
                    <span v-if="isOverdue(risk.next_review)" class="px-1.5 py-0.5 rounded text-[9px] font-semibold bg-red-900/50 text-red-400">OVERDUE</span>
                  </div>
                  <div v-if="risk.category" class="text-[10px] text-slate-600 mt-0.5">{{ formatLabel(risk.category) }}</div>
                  <div v-if="risk.description" class="text-xs text-slate-500 mt-0.5 truncate max-w-xs">{{ stripMd(risk.description) }}</div>
                </td>
                <td class="px-5 py-3.5">
                  <span v-if="risk.risk_type" class="inline-block px-2 py-0.5 rounded text-xs font-medium"
                    :class="risk.risk_type === 'opportunity' ? 'bg-emerald-900/40 text-emerald-400' : 'bg-red-900/40 text-red-400'">
                    {{ risk.risk_type }}
                  </span>
                  <span v-else class="text-slate-600 text-xs">-</span>
                </td>
                <td class="px-5 py-3.5 text-center">
                  <div class="flex flex-col items-center gap-0.5">
                    <span
                      class="inline-flex items-center justify-center w-8 h-8 rounded-lg text-sm font-bold tabular-nums"
                      :class="scoreColor(risk.current_score)"
                    >
                      {{ risk.current_score ?? '-' }}
                    </span>
                    <span v-if="risk.inherent_score" class="text-[9px] text-slate-600">was {{ risk.inherent_score }}</span>
                  </div>
                </td>
                <td class="px-5 py-3.5 text-center">
                  <div class="flex gap-0.5 justify-center">
                    <span v-if="risk.confidentiality_impact > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(risk.confidentiality_impact)" :title="'Confidentiality: ' + ciaLabel(risk.confidentiality_impact)">C{{ risk.confidentiality_impact }}</span>
                    <span v-if="risk.integrity_impact > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(risk.integrity_impact)" :title="'Integrity: ' + ciaLabel(risk.integrity_impact)">I{{ risk.integrity_impact }}</span>
                    <span v-if="risk.availability_impact > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(risk.availability_impact)" :title="'Availability: ' + ciaLabel(risk.availability_impact)">A{{ risk.availability_impact }}</span>
                    <span v-if="!risk.confidentiality_impact && !risk.integrity_impact && !risk.availability_impact" class="text-slate-600 text-xs">-</span>
                  </div>
                </td>
                <td class="px-5 py-3.5 text-sm text-slate-400">{{ resolveUserName(risk.owner) }}</td>
                <td class="px-5 py-3.5 text-sm text-slate-400 capitalize">{{ (risk.treatment || '-').replace(/_/g, ' ') }}</td>
                <td class="px-5 py-3.5">
                  <StatusBadge :status="risk.status" />
                </td>
                <td class="px-5 py-3.5 text-right">
                  <div class="flex items-center justify-end gap-2">
                    <span v-if="riskAdvisories[risk.id]?.length" class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[9px] font-medium bg-amber-900/40 text-amber-400 border border-amber-800/40" title="CIA mismatch with linked assets — reassessment needed">
                      <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                      </svg>
                      Review
                    </span>
                  </div>
                </td>
              </tr>

            </template>

          </tbody>
        </table>
        <Pagination :page="page" :pageSize="pageSize" :total="total" @update:page="page = $event" @update:pageSize="pageSize = $event" />
      </div>

      <!-- Detail modal -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="selectedRisk" class="fixed inset-0 z-50 flex items-start justify-center pt-[3vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="closeDetail" />
        <div class="relative w-full max-w-4xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-h-[90vh] flex flex-col">
          <!-- Header -->
          <div class="flex-shrink-0 border-b border-slate-800 px-6 py-3 flex items-center justify-between gap-4">
            <div class="flex items-center gap-6 min-w-0">
              <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ selectedRisk.identifier }}</span>
              <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selectedRisk.title }}</h2>
            </div>
            <div class="flex items-center gap-3 flex-shrink-0">
              <StatusBadge :status="selectedRisk.status" />
              <button @click="closeDetail" class="p-1 rounded-lg text-slate-600 hover:text-slate-300 hover:bg-slate-800 transition-colors">
                <svg class="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          </div>
          <!-- Body: primary nav + (section nav) + content -->
          <div class="flex flex-1 min-h-0">
            <!-- Primary nav -->
            <nav class="flex-shrink-0 w-28 border-r border-slate-800 py-3">
              <div class="space-y-0.5">
                <button v-for="t in detailTabs" :key="t.key" @click="switchDetailTab(t.key)"
                  class="w-full text-left px-3 py-2 text-xs font-medium transition-colors"
                  :class="detailTab === t.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                  {{ t.label }}
                </button>
              </div>
            </nav>
            <!-- Content pane -->
            <div class="flex-1 overflow-y-auto min-h-0">

              <!-- ═══ OVERVIEW ═══ -->
              <template v-if="detailTab === 'overview'">
                <div class="px-6 py-5 space-y-5">
                  <!-- Section header -->
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Overview</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('overview')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                  </div>
                  <template v-if="editingSection === 'overview'">
                    <!-- Edit mode -->
                    <div class="space-y-4">
                      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                        <div class="sm:col-span-2">
                          <label class="block text-xs font-medium text-slate-500 mb-1">Title</label>
                          <input v-model="editForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                        </div>
                        <div class="sm:col-span-2">
                          <label class="block text-xs font-medium text-slate-500 mb-1">Description</label>
                          <MarkdownField v-model="editForm.description" :self-type="'risk'" :self-id="selectedRisk?.identifier || ''" :rows="3" placeholder="Describe the risk..." />
                        </div>
                        <div>
                          <label class="block text-xs font-medium text-slate-500 mb-1">Status</label>
                          <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                            <option value="draft">Draft</option><option value="open">Open</option><option value="closed">Closed</option>
                          </select>
                        </div>
                        <div>
                          <label class="block text-xs font-medium text-slate-500 mb-1">Owner</label>
                          <MemberPicker v-model="editForm.owner" :members="orgMembers" placeholder="Select owner..." />
                        </div>
                        <div>
                          <label class="block text-xs font-medium text-slate-500 mb-1">Category</label>
                          <select v-model="editForm.category" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                            <option value="">None</option>
                            <option v-for="cat in riskCategories" :key="cat.key" :value="cat.key">{{ cat.label }}</option>
                          </select>
                        </div>
                        <div>
                          <label class="block text-xs font-medium text-slate-500 mb-1">Risk Type</label>
                          <select v-model="editForm.risk_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                            <option value="threat">Threat</option><option value="opportunity">Opportunity</option>
                          </select>
                        </div>
                        <div>
                          <label class="block text-xs font-medium text-slate-500 mb-1">Origin</label>
                          <select v-model="editForm.origin" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                            <option value="internal">Internal</option><option value="external">External</option><option value="internal and external">Internal &amp; External</option>
                          </select>
                        </div>
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <!-- View mode -->
                    <div class="space-y-4">
                      <!-- Title -->
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Title</div>
                        <div class="text-sm text-slate-200 font-medium">{{ selectedRisk.title }}</div>
                      </div>

                      <!-- Description -->
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Description</div>
                        <div v-if="selectedRisk.description" class="text-sm text-slate-300 leading-relaxed doc-prose" v-html="renderMd(selectedRisk.description)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>

                      <!-- 2-column metadata -->
                      <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Status</div>
                          <StatusBadge :status="selectedRisk.status" />
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Owner</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedRisk.owner) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Category</div>
                          <div class="text-sm text-slate-300 capitalize">{{ formatLabel(selectedRisk.category) || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Risk Type</div>
                          <div class="text-sm text-slate-300 capitalize">{{ selectedRisk.risk_type || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Origin</div>
                          <div class="text-sm text-slate-300">{{ formatOrigin(selectedRisk.origin) || '—' }}</div>
                        </div>
                        <div v-if="selectedRisk.created_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created</div>
                          <div class="text-sm text-slate-300">{{ formatDate(selectedRisk.created_at) }}</div>
                        </div>
                        <div v-if="selectedRisk.created_by">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created by</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedRisk.created_by) }}</div>
                        </div>
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
                        <MarkdownField v-model="editForm.treatment_plan" :self-type="'risk'" :self-id="selectedRisk?.identifier || ''" :rows="4" placeholder="How are we addressing this risk?" />
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Treatment</div>
                        <div class="text-sm text-slate-300 capitalize">{{ selectedRisk.treatment || '—' }}</div>
                      </div>

                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Treatment Plan</div>
                        <div v-if="selectedRisk.treatment_plan" class="text-sm doc-prose text-slate-300 leading-relaxed" v-html="renderMd(selectedRisk.treatment_plan)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>
                    </div>
                  </template>
                </div>
              </template>

              <!-- ═══ ACTIONS ═══ -->
              <template v-if="detailTab === 'actions'">
                <div class="px-6 py-5 space-y-4">
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Quick actions</div>
                  <div v-if="!canWrite" class="text-xs text-slate-600 italic">Read-only — actions require manager or admin role.</div>
                  <div v-else class="flex flex-col gap-3 max-w-md">
                    <button @click="createLinkedTask"
                      class="flex items-center justify-between gap-3 px-4 py-3 rounded-lg bg-slate-900 hover:bg-slate-800 border border-slate-700 hover:border-slate-600 transition-colors text-left">
                      <div>
                        <div class="text-sm font-medium text-slate-200">Create Implementation Task</div>
                        <div class="text-xs text-slate-500 mt-0.5">Spawn a task to action the treatment plan; auto-linked back to this risk.</div>
                      </div>
                      <span class="text-slate-500 text-lg">→</span>
                    </button>
                  </div>
                </div>
              </template>

              <!-- ═══ EDIT (full form, hidden — kept for backwards compat) ═══ -->
              <template v-if="detailTab === 'edit'">
                <div class="px-6 py-5 space-y-4">
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Title</label>
                      <input v-model="editForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Description</label>
                      <MarkdownField v-model="editForm.description" :self-type="'risk'" :self-id="selectedRisk?.identifier || ''" :rows="3" placeholder="Describe the risk..." />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Status</label>
                      <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option value="draft">Draft</option><option value="open">Open</option><option value="closed">Closed</option>
                      </select>
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Owner</label>
                      <MemberPicker v-model="editForm.owner" :members="orgMembers" placeholder="Select owner..." />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Category</label>
                      <select v-model="editForm.category" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option value="">None</option>
                        <option v-for="cat in riskCategories" :key="cat.key" :value="cat.key">{{ cat.label }}</option>
                      </select>
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Risk Type</label>
                      <select v-model="editForm.risk_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option value="threat">Threat</option><option value="opportunity">Opportunity</option>
                      </select>
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Origin</label>
                      <select v-model="editForm.origin" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option value="internal">Internal</option><option value="external">External</option><option value="internal and external">Internal &amp; External</option>
                      </select>
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Treatment</label>
                      <select v-model="editForm.treatment" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option value="">Not decided</option><option value="mitigate">Mitigate</option><option value="accept">Accept</option><option value="transfer">Transfer</option><option value="avoid">Avoid</option>
                      </select>
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Treatment Plan</label>
                      <MarkdownField v-model="editForm.treatment_plan" :self-type="'risk'" :self-id="selectedRisk?.identifier || ''" :rows="2" placeholder="Treatment plan..." />
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Notes</label>
                      <MarkdownField v-model="editForm.notes" :self-type="'risk'" :self-id="selectedRisk?.identifier || ''" :rows="2" placeholder="Additional notes..." />
                    </div>
                  </div>
                  <div class="flex justify-end gap-3 pt-2 border-t border-slate-800">
                    <button @click="cancelSection(); detailTab = 'overview'" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200">Cancel</button>
                    <button @click="saveSection().then(() => { detailTab = 'overview' })" :disabled="riskSaving" class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 text-white text-sm font-medium rounded-lg">
                      {{ riskSaving ? 'Saving...' : 'Save' }}
                    </button>
                  </div>
                </div>
              </template>

              <!-- ═══ EDIT (shown when editing from overview) ═══ -->
              <template v-if="detailTab === 'edit'">
                <div class="px-6 py-5 space-y-4">
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Title</label>
                      <input v-model="editForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Description</label>
                      <textarea v-model="editForm.description" rows="3" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500 resize-none" placeholder="Risk scenario..."></textarea>
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
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Category</label>
                      <select v-model="editForm.category" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option value="">None</option>
                        <option v-for="cat in riskCategories" :key="cat.key" :value="cat.key">{{ cat.label }}</option>
                      </select>
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Risk Type</label>
                      <select v-model="editForm.risk_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option value="threat">Threat</option>
                        <option value="opportunity">Opportunity</option>
                      </select>
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Origin</label>
                      <select v-model="editForm.origin" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option value="internal">Internal</option>
                        <option value="external">External</option>
                        <option value="internal and external">Internal &amp; External</option>
                      </select>
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Treatment</label>
                      <select v-model="editForm.treatment" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option value="">Not decided</option>
                        <option value="mitigate">Mitigate</option>
                        <option value="accept">Accept</option>
                        <option value="transfer">Transfer</option>
                        <option value="avoid">Avoid</option>
                      </select>
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Treatment Plan</label>
                      <MarkdownField v-model="editForm.treatment_plan" :self-type="'risk'" :self-id="selectedRisk?.identifier || ''" :rows="2" placeholder="Treatment plan..." />
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Notes</label>
                      <MarkdownField v-model="editForm.notes" :self-type="'risk'" :self-id="selectedRisk?.identifier || ''" :rows="2" placeholder="Additional notes..." />
                    </div>
                  </div>
                  <div class="flex justify-end gap-3 pt-2 border-t border-slate-800">
                    <button @click="cancelSection(); detailTab = 'overview'" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
                    <button @click="saveSection().then(() => { detailTab = 'overview' })" :disabled="riskSaving" class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 disabled:text-slate-500 text-white text-sm font-medium rounded-lg transition-colors">
                      {{ riskSaving ? 'Saving...' : 'Save' }}
                    </button>
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
                    <div>
                      <MarkdownField v-model="editForm.notes" :self-type="'risk'" :self-id="selectedRisk?.identifier || ''" :rows="12" placeholder="Add notes, observations, meeting minutes, references..." />
                    </div>
                  </template>
                  <template v-else>
                    <div v-if="selectedRisk.notes" class="text-sm doc-prose text-slate-300 leading-relaxed" v-html="renderMd(selectedRisk.notes)"></div>
                    <div v-else class="text-sm text-slate-600 italic">No notes yet.</div>
                  </template>
                </div>
              </template>

              <!-- ═══ ASSESSMENT ═══ -->
              <template v-if="detailTab === 'assessment'">
                <div class="px-6 py-5 space-y-5">
                  <!-- Overdue warning -->
                  <div v-if="selectedRisk.next_review && isOverdue(selectedRisk.next_review)" class="flex items-center gap-2 px-3 py-2 rounded-lg text-xs bg-red-950/40 border border-red-900/40 text-red-300">
                    <svg class="w-3.5 h-3.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                    Review overdue since {{ formatDate(selectedRisk.next_review) }}
                  </div>

                  <!-- Score summary -->
                  <div class="flex items-center gap-4">
                    <div class="flex gap-3 flex-1">
                      <div class="bg-slate-800/40 border border-slate-700/50 rounded-lg px-5 py-3 text-center min-w-[90px]">
                        <div class="text-[9px] text-slate-500 uppercase tracking-wider">Inherent</div>
                        <div v-if="selectedRisk.inherent_score" class="text-xl font-bold tabular-nums mt-1" :class="scoreColor(selectedRisk.inherent_score)">{{ selectedRisk.inherent_score }}</div>
                        <div v-else class="text-xl text-slate-700 mt-1">—</div>
                      </div>
                      <div class="bg-slate-800/60 border border-slate-600/50 rounded-lg px-5 py-3 text-center min-w-[90px] ring-1 ring-slate-600/20">
                        <div class="text-[9px] text-slate-400 uppercase tracking-wider">Current</div>
                        <div v-if="selectedRisk.current_score" class="text-xl font-bold tabular-nums mt-1" :class="scoreColor(selectedRisk.current_score)">{{ selectedRisk.current_score }}</div>
                        <div v-else class="text-xl text-slate-700 mt-1">—</div>
                      </div>
                    </div>
                    <div v-if="selectedRisk.confidentiality_impact || selectedRisk.integrity_impact || selectedRisk.availability_impact" class="flex items-center gap-1.5">
                      <span class="inline-flex items-center justify-center w-8 h-8 rounded text-[11px] font-bold" :class="scoreColor(selectedRisk.confidentiality_impact)">C{{ selectedRisk.confidentiality_impact || 0 }}</span>
                      <span class="inline-flex items-center justify-center w-8 h-8 rounded text-[11px] font-bold" :class="scoreColor(selectedRisk.integrity_impact)">I{{ selectedRisk.integrity_impact || 0 }}</span>
                      <span class="inline-flex items-center justify-center w-8 h-8 rounded text-[11px] font-bold" :class="scoreColor(selectedRisk.availability_impact)">A{{ selectedRisk.availability_impact || 0 }}</span>
                    </div>
                  </div>
                  <div class="flex gap-4 text-[10px] text-slate-500">
                    <span v-if="selectedRisk.last_review">Last review: {{ formatDate(selectedRisk.last_review) }}</span>
                    <span v-if="selectedRisk.next_review && !isOverdue(selectedRisk.next_review)">Next review: {{ formatDate(selectedRisk.next_review) }}</span>
                  </div>

                  <!-- Readings panel -->
                  <div class="border-t border-slate-800 pt-4">
                    <ReadingsPanel entityType="risk" :entityId="selectedRisk.id" :identifier="selectedRisk.identifier || ('RISK-' + selectedRisk.id)" :canWrite="canWrite"
                      :currentValues="{ current_likelihood: selectedRisk.current_likelihood, current_impact: selectedRisk.current_impact, confidentiality_impact: selectedRisk.confidentiality_impact, integrity_impact: selectedRisk.integrity_impact, availability_impact: selectedRisk.availability_impact }"
                      @saved="refreshSelectedRisk" />
                  </div>
                </div>
              </template>

              <!-- ═══ LINKS ═══ -->
              <template v-if="detailTab === 'links'">
                <div class="px-6 py-5 space-y-4">
                  <ReferenceManager entityType="risk" :entityId="selectedRisk.identifier || ('RISK-' + selectedRisk.id)" :editable="canWrite" />

                  <!-- Linked Assets -->
                  <div v-if="riskLinkedAssets[selectedRisk.id]?.length" class="bg-slate-800/40 border border-slate-700/50 rounded-lg p-3 space-y-1.5">
                    <div class="text-[10px] font-semibold text-slate-400 uppercase tracking-wider">Linked Assets</div>
                    <div v-for="asset in riskLinkedAssets[selectedRisk.id]" :key="asset.id" class="flex items-center gap-3 px-3 py-1.5 bg-slate-900/60 rounded">
                      <span class="text-sm text-slate-200 truncate flex-1">{{ asset.name }}</span>
                      <span v-if="asset.confidentiality > 0" class="px-1 py-0.5 rounded text-[9px] font-semibold" :class="ciaColor(asset.confidentiality)">C{{ asset.confidentiality }}</span>
                      <span v-if="asset.integrity > 0" class="px-1 py-0.5 rounded text-[9px] font-semibold" :class="ciaColor(asset.integrity)">I{{ asset.integrity }}</span>
                      <span v-if="asset.availability > 0" class="px-1 py-0.5 rounded text-[9px] font-semibold" :class="ciaColor(asset.availability)">A{{ asset.availability }}</span>
                    </div>
                  </div>

                  <!-- Advisories -->
                  <div v-if="riskAdvisories[selectedRisk.id]?.length" class="space-y-1.5">
                    <div v-for="(adv, i) in riskAdvisories[selectedRisk.id]" :key="i"
                      class="flex items-start gap-2 px-3 py-2 rounded-lg text-xs"
                      :class="adv.level === 'warning' ? 'bg-amber-950/40 border border-amber-900/40 text-amber-300' : 'bg-blue-950/40 border border-blue-900/40 text-blue-300'">
                      <svg class="w-3.5 h-3.5 flex-shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                      </svg>
                      <span>{{ adv.message }}</span>
                    </div>
                  </div>

                </div>
              </template>

              <!-- ═══ SUGGESTIONS ═══ -->
              <template v-if="detailTab === 'suggestions'">
                <div class="px-6 py-5">
                  <SuggestionPanel entityType="risk" :entityId="selectedRisk.identifier" :canReview="canWrite" @applied="loadRisks" />
                </div>
              </template>

              <!-- ═══ COMMENTS ═══ -->
              <template v-if="detailTab === 'comments'">
                <div class="px-6 py-5">
                  <CommentsPanel entityType="risk" :entityId="selectedRisk.identifier" />
                </div>
              </template>

              <!-- ═══ HISTORY ═══ -->
              <template v-if="detailTab === 'history'">
                <div class="px-6 py-5 space-y-6">
                  <HistoryPanel entityType="risk" :entityId="String(selectedRisk.id)" />

                  <!-- Danger zone -->
                  <div v-if="canWrite" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">Danger zone</div>
                    <div class="text-xs text-slate-400">Deleting this risk is permanent and cannot be undone. History, comments, and linked references will be lost.</div>
                    <button @click="deleteSelectedRisk" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                      Delete risk
                    </button>
                  </div>
                </div>
              </template>

            </div>
          </div>
          <!-- Footer action bar (edit mode only) -->
          <div v-if="editingSection" class="flex-shrink-0 border-t border-slate-800 px-6 py-3 flex justify-end gap-3">
            <button @click="cancelSection" class="px-4 py-1.5 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
            <button @click="saveSection" :disabled="riskSaving" class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 text-white text-sm font-medium rounded-lg transition-colors">{{ riskSaving ? 'Saving...' : 'Save' }}</button>
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
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import MemberPicker from '../components/MemberPicker.vue'
import HeatMap from '../components/HeatMap.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
import SuggestionPanel from '../components/SuggestionPanel.vue'
import CommentsPanel from '../components/CommentsPanel.vue'
import HistoryPanel from '../components/HistoryPanel.vue'
import SuggestNewButton from '../components/SuggestNewButton.vue'
import Pagination from '../components/Pagination.vue'
import ReadingsPanel from '../components/ReadingsPanel.vue'
import MarkdownField from '../components/MarkdownField.vue'
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

// Strip markdown syntax to plain text for list-row previews. Never show raw markdown to the user.
function stripMd(text) {
  if (!text) return ''
  return String(text)
    .replace(/^#{1,6}\s+/gm, '')           // headings
    .replace(/\*\*([^*]+)\*\*/g, '$1')     // bold
    .replace(/\*([^*]+)\*/g, '$1')         // italic
    .replace(/`([^`]+)`/g, '$1')           // code
    .replace(/!\[[^\]]*\]\([^)]+\)/g, '')  // images
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1') // links → text
    .replace(/^>\s+/gm, '')                // blockquote
    .replace(/^[-*+]\s+/gm, '')            // bullets
    .replace(/^\d+\.\s+/gm, '')            // ordered list
    .replace(/\n+/g, ' ')                  // collapse newlines for snippet
    .trim()
}

// Thin alias so templates can keep saying `renderMd(...)`. The canonical
// renderer lives in useRenderMd.js; link rewriting is handled by the
// global click delegate in App.vue, not per-view regex.
const renderMd = renderMarkdown
const userRole = ref('')
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')

const loading = ref(true)
const error = ref(null)
const risks = ref([])
// Risk Map must aggregate across the full register, not just the current page.
// Loaded separately via loadAllRisksForMap() with no pagination.
const allRisksForMap = ref([])
const filterLevel = ref('')
const filterCategory = ref('')
const filterStatus = ref('')
const filterOwner = ref('')
const searchQuery = ref('')
const page = ref(1)
const pageSize = ref(50)
const total = ref(0)
const editForm = ref({})
const { capture: captureEditSnapshot, isDirty } = useDirtyEdit(editForm)
const riskSaving = ref(false)
const showCreateForm = ref(false)
const selectedRisk = ref(null)
const detailTab = ref('details')
const showOverflow = ref(false)
const editingSection = ref('') // which section is being edited: 'core', 'classification', 'response', 'additional'

// orgPath is provided by useCurrentOrg() — see top of script.

const detailTabs = [
  { key: 'overview', label: 'Overview' },
  { key: 'treatment', label: 'Treatment' },
  { key: 'assessment', label: 'Assessment' },
  { key: 'notes', label: 'Notes' },
  { key: 'links', label: 'Links' },
  { key: 'actions', label: 'Actions' },
  { key: 'suggestions', label: 'Suggestions' },
  { key: 'comments', label: 'Comments' },
  { key: 'history', label: 'History' },
]

function editSection(section) {
  startEdit(selectedRisk.value)
  editingSection.value = section
}

function cancelSection() {
  editingSection.value = ''
  startEdit(selectedRisk.value)
}

async function saveSection() {
  riskSaving.value = true
  try {
    const id = selectedRisk.value.risk_id || selectedRisk.value.id || selectedRisk.value.document_id
    const payload = { ...editForm.value }
    await api.putJSON(`/api/v1/risks/${id}`, payload)
    await loadRisks()
    const updated = risks.value.find(r => r.id === selectedRisk.value.id)
    if (updated) {
      selectedRisk.value = updated
      startEdit(updated) // refresh editForm with saved data
    }
    editingSection.value = ''
    showSaved('Saved')
  } catch (e) {
    showError('Failed to save: ' + e.message)
  } finally {
    riskSaving.value = false
  }
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
  // router watch below clears state
  router.push(orgPath('/risks'))
}

useModalEscape(showCreateForm)
useModalEscape(computed(() => !!selectedRisk.value), closeDetail)
const riskAdvisories = reactive({})
const riskLinkedAssets = reactive({})
const allAssets = ref([])
const orgMembers = ref([])
const pendingRefs = ref([])

const riskCategories = [
  { key: 'people_process', label: 'People & Process' },
  { key: 'technology', label: 'Technology' },
  { key: 'third_party', label: 'Third Party' },
  { key: 'legal_regulatory', label: 'Legal & Regulatory' },
  { key: 'physical_environmental', label: 'Physical & Environmental' },
  { key: 'business_continuity', label: 'Business Continuity' },
  { key: 'governance', label: 'Governance' },
  { key: 'quality_operations', label: 'Quality & Operations' },
]

const newRisk = ref({
  title: '',
  category: '',
})

function isOverdue(dateStr) {
  if (!dateStr) return false
  const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
  return d < new Date()
}

const levelOptions = [
  { value: '', label: 'All' },
  { value: 'critical', label: 'Critical' },
  { value: 'high', label: 'High' },
  { value: 'medium', label: 'Medium' },
  { value: 'low', label: 'Low' },
]

const stats = ref({ total: 0, critical: 0, high: 0, medium: 0, low: 0, open: 0, closed: 0, draft: 0 })
const criticalCount = computed(() => stats.value.critical)
const highCount = computed(() => stats.value.high)
const treatedCount = computed(() => stats.value.closed)

// Refetch on filter/search/page change
let searchTimer = null
watch([searchQuery], () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { page.value = 1; loadRisks() }, 250)
})
watch([filterLevel, filterCategory, filterStatus, filterOwner], () => {
  page.value = 1
  loadRisks()
})
watch([page, pageSize], () => loadRisks())

async function loadStats() {
  try { stats.value = await api.fetchJSON('/api/v1/risks/stats') || stats.value } catch { /* silent */ }
}

function resolveUserName(email) {
  if (!email) return '—'
  const u = orgMembers.value.find(m => m.email === email)
  return u?.name || email
}

function formatLabel(s) {
  return (s || '').replace(/_/g, ' ')
}

function formatOrigin(o) {
  if (o === 'internal and external') return 'Internal & External'
  if (o === 'internal') return 'Internal'
  if (o === 'external') return 'External'
  return o ? o.charAt(0).toUpperCase() + o.slice(1) : ''
}

function formatDate(dateStr) {
  if (!dateStr && dateStr !== 0) return '-'
  const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
  return d.toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric' })
}

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

async function loadRisks() {
  try {
    const params = new URLSearchParams()
    params.set('page', String(page.value))
    params.set('limit', String(pageSize.value))
    if (searchQuery.value) params.set('q', searchQuery.value)
    if (filterLevel.value) params.set('level', filterLevel.value)
    if (filterCategory.value) params.set('category', filterCategory.value)
    if (filterStatus.value) params.set('status', filterStatus.value)
    if (filterOwner.value) params.set('owner', filterOwner.value)
    const res = await api.fetchRaw(`/api/v1/risks?${params.toString()}`)
    risks.value = Array.isArray(res?.data) ? res.data : []
    total.value = res?.total || 0
    loadStats()
    loadAllAdvisories()
    loadAllRisksForMap()
  } catch (e) {
    error.value = e.message
  }
}

// Risk Map must aggregate across the full register regardless of the current
// list page/filters. Fetch a lightweight all-risks slice for the heatmap only.
async function loadAllRisksForMap() {
  try {
    const res = await api.fetchRaw(`/api/v1/risks?limit=10000`)
    allRisksForMap.value = Array.isArray(res?.data) ? res.data : []
  } catch { /* heatmap is non-critical */ }
}

async function refreshSelectedRisk() {
  if (!selectedRisk.value) return
  const id = selectedRisk.value.id
  try {
    // Fetch by id directly so deep links / detail still works across pages
    const fresh = await api.fetchJSON(`/api/v1/risks/${id}`)
    if (fresh) selectedRisk.value = fresh
  } catch { /* silent */ }
  loadRisks()
}

function onHeatMapClick({ likelihood, impact }) {
  // Filter to show only risks at this likelihood/impact
  // Reset filter and scroll to table
  filterLevel.value = ''
}

async function selectRisk(risk) {
  if (selectedRisk.value?.id === risk.id) {
    closeDetail()
    return
  }
  router.push(orgPath(`/risks/${risk.id}`))
}

async function openRiskFromRoute(id) {
  const numId = parseInt(id)
  let risk = risks.value.find(r => r.id === numId)
  if (!risk) {
    try { risk = await api.fetchJSON(`/api/v1/risks/${numId}`) } catch { return }
  }
  if (!risk) return
  selectedRisk.value = risk
  detailTab.value = 'overview'
  showOverflow.value = false
  startEdit(risk)
  loadAdvisories(risk.id)
  loadLinkedAssets(risk.id)
}


async function loadAdvisories(riskId) {
  if (riskAdvisories[riskId]) return // already loaded
  try {
    const data = await api.getRiskAdvisories(riskId)
    riskAdvisories[riskId] = data || []
  } catch {
    riskAdvisories[riskId] = []
  }
}

async function loadLinkedAssets(riskId) {
  if (riskLinkedAssets[riskId]) return // already loaded
  try {
    const refs = await api.getReferences('risk', String(riskId))
    const assetIds = new Set()
    for (const r of (refs || [])) {
      if (r.source_type === 'asset') assetIds.add(r.source_id)
      else if (r.target_type === 'asset') assetIds.add(r.target_id)
    }
    if (assetIds.size === 0) {
      riskLinkedAssets[riskId] = []
      return
    }
    // Ensure assets are loaded
    if (allAssets.value.length === 0) {
      try { allAssets.value = await api.getAssets() || [] } catch { allAssets.value = [] }
    }
    riskLinkedAssets[riskId] = allAssets.value.filter(a => assetIds.has(String(a.id)))
  } catch {
    riskLinkedAssets[riskId] = []
  }
}

async function loadAllAdvisories() {
  // Preload advisories for all risks so the table badge shows without expanding
  for (const risk of risks.value) {
    if (risk.id && !riskAdvisories[risk.id]) {
      loadAdvisories(risk.id) // fire-and-forget, no await to avoid blocking
    }
  }
}

function startEdit(risk) {
  editForm.value = {
    title: risk.title || '',
    description: risk.description || '',
    owner: risk.owner || '',
    risk_type: risk.risk_type || 'threat',
    origin: risk.origin || 'internal',
    category: risk.category || '',
    treatment: risk.treatment || '',
    treatment_plan: risk.treatment_plan || '',
    notes: risk.notes || '',
    status: risk.status || 'open',
  }
  captureEditSnapshot()
}

async function saveRiskField(risk, field, value) {
  try {
    const id = risk.risk_id || risk.id || risk.document_id
    await api.putJSON(`/api/v1/risks/${id}`, { [field]: value })
  } catch (e) {
    showError('Failed to save: ' + (e.message || 'unknown error'))
  }
}

async function changeRiskStatus(risk, status) {
  try {
    const id = risk.risk_id || risk.id || risk.document_id
    await api.putJSON(`/api/v1/risks/${id}`, { status })
    risk.status = status
    if (selectedRisk.value?.id === risk.id) {
      selectedRisk.value = { ...selectedRisk.value, status }
    }
  } catch (e) {
    showError('Failed to update status: ' + e.message)
  }
}

const canEdit = computed(() => userRole.value === 'admin' || userRole.value === 'manager')

async function saveRisk() {
  riskSaving.value = true
  try {
    const id = selectedRisk.value.risk_id || selectedRisk.value.id || selectedRisk.value.document_id
    const payload = { ...editForm.value }
    await api.putJSON(`/api/v1/risks/${id}`, payload)
    await loadRisks()
    if (selectedRisk.value) {
      const updated = risks.value.find(r => r.id === selectedRisk.value.id)
      if (updated) selectedRisk.value = updated
    }
    // Re-populate editForm with fresh data
    if (selectedRisk.value) startEdit(selectedRisk.value)
  } catch (e) {
    showError('Failed to save risk: ' + e.message)
  } finally {
    riskSaving.value = false
  }
}

async function createRisk() {
  try {
    const payload = { ...newRisk.value }
    const created = await api.addRisk(payload)
    const entityId = created?.identifier || ''
    if (entityId) {
      for (const ref of pendingRefs.value) {
        try {
          await api.createReference({ source_type: 'risk', source_id: entityId, target_type: ref.type, target_id: ref.id })
        } catch { /* non-fatal, entity was already created */ }
      }
    }
    pendingRefs.value = []
    showCreateForm.value = false
    newRisk.value = { title: '', category: '' }
    await loadRisks()
    // Drop user into detail modal in edit mode on Overview to keep filling things in.
    if (created && created.id) {
      let fresh = created
      try { fresh = await api.fetchJSON(`/api/v1/risks/${created.id}`) } catch { /* fall back */ }
      selectedRisk.value = fresh
      detailTab.value = 'overview'
      showOverflow.value = false
      startEdit(fresh)
      editingSection.value = 'overview'
      router.push(orgPath(`/risks/${fresh.id}`))
    }
  } catch (e) {
    showError('Failed to create risk: ' + e.message)
  }
}

function createLinkedTask() {
  if (!selectedRisk.value) return
  router.push({
    path: orgPath('/tasks'),
    query: {
      from_risk: selectedRisk.value.identifier,
      title: selectedRisk.value.title,
      task_type: 'risk_review',
      priority: selectedRisk.value.current_level === 'critical' ? 'critical' : selectedRisk.value.current_level === 'high' ? 'high' : 'medium',
    },
  })
}

async function deleteSelectedRisk() {
  if (!selectedRisk.value) return
  const ok = await confirmDialog({ message: 'Delete this risk? This cannot be undone.', variant: 'danger', confirmLabel: 'Delete' })
  if (!ok) return
  showOverflow.value = false
  try {
    const id = selectedRisk.value.risk_id || selectedRisk.value.id || selectedRisk.value.document_id
    await api.deleteJSON(`/api/v1/risks/${id}`)
    selectedRisk.value = null
    await loadRisks()
  } catch (e) {
    showError('Failed to delete risk: ' + e.message)
  }
}

function scoreColor(score) {
  if (score == null) return 'bg-slate-800 text-slate-500'
  if (score >= 16) return 'bg-red-900/60 text-red-300'
  if (score >= 10) return 'bg-orange-900/60 text-orange-300'
  if (score >= 5) return 'bg-amber-900/60 text-amber-300'
  return 'bg-emerald-900/60 text-emerald-300'
}

onMounted(async () => {
  try { const me = await api.getMe(); userRole.value = me?.role || '' } catch {}
  try {
    orgMembers.value = await api.getUsers() || []
  } catch { orgMembers.value = [] }
  try {
    await loadRisks()
    loadAllAdvisories()
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
  if (route.params.id) openRiskFromRoute(route.params.id)
})

watch(() => route.params.id, (id) => {
  if (!id) {
    selectedRisk.value = null
    detailTab.value = 'overview'
    showOverflow.value = false
    editingSection.value = ''
    return
  }
  if (selectedRisk.value?.id === parseInt(id)) return
  openRiskFromRoute(id)
})
</script>
