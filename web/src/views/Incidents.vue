<template>
  <div class="min-h-full">
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
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">Incident Management</h1>
          <p class="text-sm text-slate-500 mt-1">Track, respond to, and learn from security incidents</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canReport"
            @click="showCreateForm = !showCreateForm"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            Add Incident
          </button>
          <SuggestNewButton entityType="incident" typeLabel="Incident" />
        </div>
      </div>

      <!-- Stats cards -->
      <div class="grid grid-cols-2 lg:grid-cols-6 gap-4">
        <button @click="filterStatus = ''" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="!filterStatus ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-slate-100 tabular-nums">{{ stats.total || total || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Total</div>
        </button>
        <button @click="filterStatus = 'open'" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="filterStatus === 'open' ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-red-400 tabular-nums">{{ stats.open || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Open</div>
        </button>
        <button @click="filterStatus = 'investigating'" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="filterStatus === 'investigating' ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-amber-400 tabular-nums">{{ stats.investigating || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Investigating</div>
        </button>
        <button @click="filterStatus = 'contained'" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="filterStatus === 'contained' ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-blue-400 tabular-nums">{{ stats.contained || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Contained</div>
        </button>
        <button @click="filterStatus = 'resolved'" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="filterStatus === 'resolved' ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-emerald-400 tabular-nums">{{ stats.resolved || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Resolved</div>
        </button>
        <button @click="filterStatus = 'closed'" class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors" :class="filterStatus === 'closed' ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold text-slate-400 tabular-nums">{{ stats.closed || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Closed</div>
        </button>
      </div>

      <!-- Actions bar -->
      <div class="flex items-center gap-3 flex-wrap">
        <div class="relative flex-1 max-w-xs">
          <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
          </svg>
          <input v-model="searchQuery" type="text" placeholder="Search..."
            class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
        </div>
        <select v-model="filterStatus" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
          <option value="">All statuses</option>
          <option value="draft">Draft</option>
          <option value="open">Open</option>
          <option value="investigating">Investigating</option>
          <option value="contained">Contained</option>
          <option value="resolved">Resolved</option>
          <option value="closed">Closed</option>
        </select>
        <select v-model="filterSeverity" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
          <option value="">All severities</option>
          <option value="critical">Critical</option>
          <option value="high">High</option>
          <option value="medium">Medium</option>
          <option value="low">Low</option>
        </select>
        <div class="ml-auto text-xs text-slate-500 tabular-nums">{{ total }} total</div>
      </div>

      <!-- Create form -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreateForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreateForm = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
        <div class="flex items-center justify-between mb-2">
          <h2 class="text-sm font-semibold text-slate-200">Add Incident</h2>
          <button @click="showCreateForm = false" class="text-slate-500 hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div class="sm:col-span-2">
            <label class="block text-xs font-medium text-slate-500 mb-1">Title *</label>
            <input v-model="newIncident.title" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="Brief incident title" />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Severity</label>
            <select v-model="newIncident.severity" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option value="critical">Critical</option>
              <option value="high">High</option>
              <option value="medium">Medium</option>
              <option value="low">Low</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Type</label>
            <select v-model="newIncident.incident_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option value="incident">Incident</option>
              <option value="event">Event</option>
              <option value="weakness">Weakness</option>
            </select>
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">You can add description, classification, origin, assignee, GDPR details and notes after creating.</div>
        <div class="flex justify-end gap-3 pt-2">
          <button @click="showCreateForm = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
          <button @click="createIncident" :disabled="!newIncident.title" class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
            Add
          </button>
        </div>
        </div>
      </div>
      </Transition>
      </Teleport>

      <!-- Incident list -->
      <div v-if="incidents.length === 0" class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
        <div class="text-slate-500 text-sm">No incidents found</div>
      </div>

      <div v-else class="space-y-2">
        <div v-for="inc in incidents" :key="inc.id"
          @click="selectIncident(inc)"
          class="bg-slate-900 border border-slate-800 rounded-lg p-4 hover:border-slate-700 transition-colors cursor-pointer">
          <div class="flex items-center gap-3">
            <span class="inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-full uppercase tracking-wider"
              :class="severityClass(inc.severity)">
              {{ inc.severity }}
            </span>
            <span class="inline-flex items-center px-2 py-0.5 text-[11px] font-medium rounded-full"
              :class="typeClass(inc.incident_type)">
              {{ inc.incident_type }}
            </span>
            <span class="inline-flex items-center px-2 py-0.5 text-[11px] font-medium rounded-full"
              :class="statusClass(inc.status)">
              {{ inc.status }}
            </span>
            <span class="text-sm font-medium text-slate-200 flex-1 truncate">{{ inc.title }}</span>
            <span class="text-xs text-slate-600 font-mono">{{ inc.identifier }}</span>
            <span class="text-xs text-slate-600">{{ formatDate(inc.created_at) }}</span>
          </div>
          <div class="mt-1.5 flex items-center gap-3 text-xs text-slate-500">
            <span v-if="classificationLabel(inc)">{{ classificationLabel(inc) }}</span>
            <span>Source: {{ inc.source }}</span>
            <span>Reporter: {{ resolveUserName(inc.reporter) }}</span>
            <span v-if="inc.assignee">Assignee: {{ resolveUserName(inc.assignee) }}</span>
          </div>
        </div>
        <Pagination :page="page" :pageSize="pageSize" :total="total" @update:page="page = $event" @update:pageSize="pageSize = $event" />
      </div>

      <!-- Detail modal (tabbed) -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="selectedIncident" class="fixed inset-0 z-50 flex items-start justify-center pt-[3vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="closeDetail" />
        <div class="relative w-full max-w-4xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-h-[90vh] flex flex-col">
          <!-- Header -->
          <div class="flex-shrink-0 border-b border-slate-800 px-6 py-3 flex items-center justify-between gap-4">
            <div class="flex items-center gap-6 min-w-0">
              <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ selectedIncident.identifier }}</span>
              <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selectedIncident.title }}</h2>
            </div>
            <div class="flex items-center gap-3 flex-shrink-0">
              <span class="inline-flex items-center px-2 py-0.5 text-[10px] font-semibold rounded-full uppercase tracking-wider"
                :class="severityClass(selectedIncident.severity)">
                {{ selectedIncident.severity }}
              </span>
              <StatusBadge :status="selectedIncident.status" />
              <button @click="closeDetail" class="p-1 rounded-lg text-slate-600 hover:text-slate-300 hover:bg-slate-800 transition-colors">
                <svg class="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          </div>

          <!-- Body: sidebar nav + content -->
          <div class="flex flex-1 min-h-0">
            <nav class="flex-shrink-0 w-28 border-r border-slate-800 py-3">
              <div class="space-y-0.5">
                <button v-for="t in detailTabs" :key="t.key" @click="switchDetailTab(t.key)"
                  class="w-full text-left px-3 py-2 text-xs font-medium transition-colors"
                  :class="detailTab === t.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                  {{ t.label }}
                </button>
              </div>
            </nav>

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
                        <MarkdownField v-model="editForm.description" :self-type="'incident'" :self-id="selectedIncident ? String(selectedIncident.id) : ''" :rows="3" placeholder="Describe the incident..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Severity</label>
                        <select v-model="editForm.severity" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="critical">Critical</option>
                          <option value="high">High</option>
                          <option value="medium">Medium</option>
                          <option value="low">Low</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Type</label>
                        <select v-model="editForm.incident_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="incident">Incident</option>
                          <option value="event">Event</option>
                          <option value="weakness">Weakness</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Origin</label>
                        <select v-model="editForm.source" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="internal">Internal</option>
                          <option value="external">External</option>
                          <option value="internal and external">Internal &amp; External</option>
                        </select>
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Classification</label>
                        <div class="flex flex-wrap gap-4 mt-1">
                          <label class="flex items-center gap-2 cursor-pointer">
                            <input type="checkbox" v-model="editForm.affects_c" class="rounded bg-slate-800 border-slate-600 text-blue-500 focus:ring-blue-500 focus:ring-offset-0" />
                            <span class="text-xs text-slate-300">Confidentiality</span>
                          </label>
                          <label class="flex items-center gap-2 cursor-pointer">
                            <input type="checkbox" v-model="editForm.affects_i" class="rounded bg-slate-800 border-slate-600 text-blue-500 focus:ring-blue-500 focus:ring-offset-0" />
                            <span class="text-xs text-slate-300">Integrity</span>
                          </label>
                          <label class="flex items-center gap-2 cursor-pointer">
                            <input type="checkbox" v-model="editForm.affects_a" class="rounded bg-slate-800 border-slate-600 text-blue-500 focus:ring-blue-500 focus:ring-offset-0" />
                            <span class="text-xs text-slate-300">Availability</span>
                          </label>
                        </div>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Status</label>
                        <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="draft">Draft</option>
                          <option value="open">Open</option>
                          <option value="investigating">Investigating</option>
                          <option value="contained">Contained</option>
                          <option value="resolved">Resolved</option>
                          <option value="closed">Closed</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Assignee</label>
                        <MemberPicker v-model="editForm.assignee" :members="orgMembers" placeholder="Select assignee..." />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Root Cause</label>
                        <MarkdownField v-model="editForm.root_cause" :self-type="'incident'" :self-id="selectedIncident ? String(selectedIncident.id) : ''" :rows="3" placeholder="Root cause analysis..." />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Lessons Learned</label>
                        <MarkdownField v-model="editForm.lessons_learned" :self-type="'incident'" :self-id="selectedIncident ? String(selectedIncident.id) : ''" :rows="3" placeholder="What can we improve?" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="flex items-center gap-2 cursor-pointer">
                          <input type="checkbox" v-model="editForm.data_breach" class="rounded bg-slate-800 border-slate-600 text-blue-500 focus:ring-blue-500 focus:ring-offset-0" />
                          <span class="text-xs font-medium text-slate-400">This is a personal data breach (GDPR)</span>
                        </label>
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Description</div>
                        <div v-if="selectedIncident.description" class="text-sm text-slate-300 leading-relaxed doc-prose" v-html="renderMd(selectedIncident.description)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>

                      <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Severity</div>
                          <span class="inline-flex items-center px-2 py-0.5 text-[10px] font-semibold rounded-full uppercase tracking-wider"
                            :class="severityClass(selectedIncident.severity)">{{ selectedIncident.severity }}</span>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Type</div>
                          <div class="text-sm text-slate-300 capitalize">{{ selectedIncident.incident_type }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Origin</div>
                          <div class="text-sm text-slate-300 capitalize">{{ selectedIncident.source }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Classification</div>
                          <div class="flex items-center gap-1.5">
                            <span v-if="selectedIncident.affects_c" class="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-blue-900/40 text-blue-300 border border-blue-800">C</span>
                            <span v-if="selectedIncident.affects_i" class="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-purple-900/40 text-purple-300 border border-purple-800">I</span>
                            <span v-if="selectedIncident.affects_a" class="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-emerald-900/40 text-emerald-300 border border-emerald-800">A</span>
                            <span v-if="!selectedIncident.affects_c && !selectedIncident.affects_i && !selectedIncident.affects_a" class="text-sm text-slate-600">—</span>
                          </div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Reporter</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedIncident.reporter) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Assignee</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedIncident.assignee) }}</div>
                        </div>
                        <div v-if="selectedIncident.created_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created</div>
                          <div class="text-sm text-slate-300">{{ formatDate(selectedIncident.created_at) }}</div>
                        </div>
                        <div v-if="selectedIncident.created_by">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created by</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedIncident.created_by) }}</div>
                        </div>
                      </div>

                      <!-- Investigation/resolution fields -->
                      <div class="border-t border-slate-800 pt-4 space-y-3">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Root Cause</div>
                          <div v-if="selectedIncident.root_cause" class="text-sm text-slate-300 doc-prose" v-html="renderMd(selectedIncident.root_cause)"></div>
                          <div v-else class="text-sm text-slate-600">—</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Lessons Learned</div>
                          <div v-if="selectedIncident.lessons_learned" class="text-sm text-slate-300 doc-prose" v-html="renderMd(selectedIncident.lessons_learned)"></div>
                          <div v-else class="text-sm text-slate-600">—</div>
                        </div>
                      </div>

                      <!-- Timeline -->
                      <div class="border-t border-slate-800 pt-4">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-3">Timeline</div>
                        <div class="grid grid-cols-2 gap-3 text-xs">
                          <div>
                            <span class="text-slate-500">Detected:</span>
                            <span class="text-slate-300 ml-1">{{ formatDateTime(selectedIncident.detected_at) }}</span>
                          </div>
                          <div v-if="selectedIncident.contained_at">
                            <span class="text-slate-500">Contained:</span>
                            <span class="text-slate-300 ml-1">{{ formatDateTime(selectedIncident.contained_at) }}</span>
                          </div>
                          <div v-if="selectedIncident.resolved_at">
                            <span class="text-slate-500">Resolved:</span>
                            <span class="text-slate-300 ml-1">{{ formatDateTime(selectedIncident.resolved_at) }}</span>
                          </div>
                          <div v-if="selectedIncident.closed_at">
                            <span class="text-slate-500">Closed:</span>
                            <span class="text-slate-300 ml-1">{{ formatDateTime(selectedIncident.closed_at) }}</span>
                          </div>
                        </div>
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
                    <button @click="createLinkedCA"
                      class="flex items-center justify-between gap-3 px-4 py-3 rounded-lg bg-slate-900 hover:bg-slate-800 border border-slate-700 hover:border-slate-600 transition-colors text-left">
                      <div>
                        <div class="text-sm font-medium text-slate-200">Create Corrective Action</div>
                        <div class="text-xs text-slate-500 mt-0.5">Spawn a corrective action linked back to this incident.</div>
                      </div>
                      <span class="text-slate-500 text-lg">→</span>
                    </button>
                  </div>
                </div>
              </template>

              <!-- ═══ DATA BREACH ═══ -->
              <template v-if="detailTab === 'data_breach' && selectedIncident.data_breach">
                <div class="px-6 py-5 space-y-5">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Personal Data Breach (GDPR)</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('data_breach')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                  </div>
                  <template v-if="editingSection === 'data_breach'">
                    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">GDPR Role</label>
                        <select v-model="editForm.gdpr_role" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="controller">Controller</option>
                          <option value="processor">Processor</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Authority Notification</label>
                        <select v-model="editForm.authority_notified" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="not_required">Not Required</option>
                          <option value="pending">Pending</option>
                          <option value="notified">Notified</option>
                        </select>
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Data Subjects Notification</label>
                        <select v-model="editForm.subjects_notified" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="not_required">Not Required</option>
                          <option value="pending">Pending</option>
                          <option value="notified">Notified</option>
                        </select>
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="bg-red-950/30 border border-red-900/40 rounded-lg p-4 space-y-3">
                      <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 text-xs">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">GDPR Role</div>
                          <div class="text-slate-300 capitalize">{{ selectedIncident.gdpr_role || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Authority</div>
                          <span class="px-1.5 py-0.5 rounded text-[10px] font-medium"
                            :class="selectedIncident.authority_notified === 'notified' ? 'bg-emerald-900/40 text-emerald-400' : selectedIncident.authority_notified === 'pending' ? 'bg-amber-900/40 text-amber-400' : 'bg-slate-800 text-slate-500'">
                            {{ (selectedIncident.authority_notified || 'not_required').replace(/_/g, ' ') }}
                          </span>
                          <div v-if="selectedIncident.authority_notified_at" class="text-slate-600 mt-1">{{ formatDateTime(selectedIncident.authority_notified_at) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Data Subjects</div>
                          <span class="px-1.5 py-0.5 rounded text-[10px] font-medium"
                            :class="selectedIncident.subjects_notified === 'notified' ? 'bg-emerald-900/40 text-emerald-400' : selectedIncident.subjects_notified === 'pending' ? 'bg-amber-900/40 text-amber-400' : 'bg-slate-800 text-slate-500'">
                            {{ (selectedIncident.subjects_notified || 'not_required').replace(/_/g, ' ') }}
                          </span>
                          <div v-if="selectedIncident.subjects_notified_at" class="text-slate-600 mt-1">{{ formatDateTime(selectedIncident.subjects_notified_at) }}</div>
                        </div>
                      </div>
                    </div>
                  </template>
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
                    <MarkdownField v-model="editForm.notes" :self-type="'incident'" :self-id="selectedIncident ? String(selectedIncident.id) : ''" :rows="12" placeholder="Add notes..." />
                  </template>
                  <template v-else>
                    <div v-if="selectedIncident.notes" class="text-sm doc-prose text-slate-300 leading-relaxed" v-html="renderMd(selectedIncident.notes)"></div>
                    <div v-else class="text-sm text-slate-600 italic">No notes yet.</div>
                  </template>
                </div>
              </template>

              <!-- ═══ LINKS ═══ -->
              <template v-if="detailTab === 'links'">
                <div class="px-6 py-5">
                  <ReferenceManager entityType="incident" :entityId="selectedIncident.identifier" :editable="canWrite" />
                </div>
              </template>

              <!-- ═══ SUGGESTIONS ═══ -->
              <template v-if="detailTab === 'suggestions'">
                <div class="px-6 py-5">
                  <SuggestionPanel entityType="incident" :entityId="selectedIncident.identifier" :canReview="canWrite" @applied="loadIncidents" />
                </div>
              </template>

              <!-- ═══ COMMENTS ═══ -->
              <template v-if="detailTab === 'comments'">
                <div class="px-6 py-5">
                  <CommentsPanel entityType="incident" :entityId="selectedIncident.identifier" />
                </div>
              </template>

              <!-- ═══ HISTORY ═══ -->
              <template v-if="detailTab === 'history'">
                <div class="px-6 py-5 space-y-6">
                  <HistoryPanel entityType="incident" :entityId="String(selectedIncident.id)" />
                  <div v-if="canWrite" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">Danger zone</div>
                    <div class="text-xs text-slate-400">Deleting this incident is permanent and cannot be undone.</div>
                    <button @click="deleteSelectedIncident" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                      Delete incident
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
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import MemberPicker from '../components/MemberPicker.vue'
import MarkdownField from '../components/MarkdownField.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
import SuggestionPanel from '../components/SuggestionPanel.vue'
import SuggestNewButton from '../components/SuggestNewButton.vue'
import HistoryPanel from '../components/HistoryPanel.vue'
import CommentsPanel from '../components/CommentsPanel.vue'
import Pagination from '../components/Pagination.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import { renderMarkdown } from '../composables/useRenderMd.js'
import { useModalEscape } from '../composables/useModalEscape.js'
import { useToast } from '../composables/useToast.js'
import { useConfirm } from '../composables/useConfirm.js'
import { useDirtyEdit } from '../composables/useDirtyEdit.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'

const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()
const { success: showSaved, error: showError } = useToast()
const { confirm: confirmDialog } = useConfirm()

const renderMd = renderMarkdown

const userRole = ref('')
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')
const canReport = computed(() => userRole.value === 'admin' || userRole.value === 'manager')

const orgMembers = ref([])
const loading = ref(true)
const error = ref(null)
const incidents = ref([])
const stats = ref({})
const selectedIncident = ref(null)
const showCreateForm = ref(false)
const filterStatus = ref('')
const filterSeverity = ref('')
const searchQuery = ref('')
const page = ref(1)
const pageSize = ref(50)
const total = ref(0)

// Tab-based detail state
const detailTab = ref('overview')
const editingSection = ref('')
const editForm = ref({})
const { capture: captureEditSnapshot, isDirty } = useDirtyEdit(editForm)
const saving = ref(false)

// orgPath is provided by useCurrentOrg() above.

const detailTabs = computed(() => {
  const base = [{ key: 'overview', label: 'Overview' }]
  if (selectedIncident.value?.data_breach) {
    base.push({ key: 'data_breach', label: 'Data Breach' })
  }
  base.push({ key: 'notes', label: 'Notes' })
  base.push({ key: 'links', label: 'Links' })
  base.push({ key: 'actions', label: 'Actions' })
  base.push({ key: 'suggestions', label: 'Suggestions' })
  base.push({ key: 'comments', label: 'Comments' })
  base.push({ key: 'history', label: 'History' })
  return base
})

useModalEscape(showCreateForm)
useModalEscape(computed(() => !!selectedIncident.value), () => closeDetail())

const newIncident = ref({
  title: '',
  severity: 'medium',
  incident_type: 'event',
})

onMounted(async () => {
  try { const me = await api.getMe(); userRole.value = me?.role || '' } catch {}
  try { orgMembers.value = await api.getUsers() || [] } catch { orgMembers.value = [] }
  await loadAll()
  if (route.params.id) await openIncidentFromRoute(route.params.id)
})

watch(() => route.params.id, (id) => {
  if (!id) {
    selectedIncident.value = null
    detailTab.value = 'overview'
    editingSection.value = ''
    return
  }
  if (selectedIncident.value?.id === parseInt(id)) return
  openIncidentFromRoute(id)
})

let searchTimer = null
watch([searchQuery], () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { page.value = 1; loadIncidents() }, 250)
})
watch([filterStatus, filterSeverity], () => {
  page.value = 1
  loadIncidents()
})
watch([page, pageSize], () => loadIncidents())

async function loadAll() {
  loading.value = true
  error.value = null
  try {
    await Promise.all([loadIncidents(), loadStats()])
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function loadIncidents() {
  try {
    const params = new URLSearchParams()
    params.set('page', String(page.value))
    params.set('limit', String(pageSize.value))
    if (searchQuery.value) params.set('q', searchQuery.value)
    if (filterStatus.value) params.set('status', filterStatus.value)
    if (filterSeverity.value) params.set('severity', filterSeverity.value)
    const res = await api.fetchRaw(`/api/v1/incidents?${params.toString()}`)
    incidents.value = Array.isArray(res?.data) ? res.data : []
    total.value = res?.total || 0
    loadStats()
  } catch (e) {
    error.value = e.message
  }
}

async function loadStats() {
  try {
    stats.value = await api.fetchJSON('/api/v1/incidents/stats') || stats.value
  } catch (e) { /* non-critical */ }
}

function resolveUserName(email) {
  if (!email) return '—'
  const u = orgMembers.value.find(m => m.email === email)
  return u?.name || email
}

async function openIncidentFromRoute(id) {
  const numId = parseInt(id)
  let inc = incidents.value.find(i => i.id === numId)
  if (!inc) {
    try { inc = await api.fetchJSON(`/api/v1/incidents/${numId}`) } catch { return }
  }
  if (!inc) return
  selectedIncident.value = inc
  detailTab.value = 'overview'
  editingSection.value = ''
  startEdit(inc)
}

async function createIncident() {
  try {
    const payload = { ...newIncident.value }
    const created = await api.createIncident(payload)
    showCreateForm.value = false
    newIncident.value = { title: '', severity: 'medium', incident_type: 'event' }
    await loadIncidents()
    // Drop user into detail modal in edit mode on Overview to keep filling things in.
    if (created && created.id) {
      let fresh = created
      try { fresh = await api.getIncident(created.id) } catch { /* fall back */ }
      selectedIncident.value = fresh
      detailTab.value = 'overview'
      startEdit(fresh)
      editingSection.value = 'overview'
      router.push(orgPath(`/incidents/${fresh.id}`))
    }
  } catch (e) {
    error.value = e.message
    showError('Failed to create incident: ' + (e.message || 'unknown error'))
  }
}

async function changeStatus(inc, status) {
  try {
    await api.updateIncidentStatus(inc.id, status)
    inc.status = status
    if (selectedIncident.value?.id === inc.id) {
      selectedIncident.value = { ...inc, status }
    }
    await loadStats()
  } catch (e) {
    showError(e.message || 'Status change failed')
  }
}

function createLinkedCA() {
  if (!selectedIncident.value) return
  router.push({
    path: orgPath('/corrective-actions'),
    query: {
      from_incident: selectedIncident.value.id,
      title: selectedIncident.value.title,
      severity: selectedIncident.value.severity === 'critical' ? 'major_nc' : 'minor_nc',
    },
  })
}

async function deleteSelectedIncident() {
  if (!selectedIncident.value) return
  if (!await confirmDialog({ message: `Delete incident "${selectedIncident.value.title}"? This cannot be undone.`, confirmLabel: 'Delete', variant: 'danger' })) return
  try {
    await api.deleteIncident(selectedIncident.value.id)
    closeDetail()
    await loadIncidents()
  } catch (e) {
    error.value = e.message
    showError('Failed to delete incident: ' + (e.message || 'unknown error'))
  }
}

function startEdit(inc) {
  editForm.value = {
    title: inc.title || '',
    description: inc.description || '',
    severity: inc.severity || 'medium',
    status: inc.status || 'draft',
    incident_type: inc.incident_type || 'event',
    source: inc.source || 'internal',
    affects_c: !!inc.affects_c,
    affects_i: !!inc.affects_i,
    affects_a: !!inc.affects_a,
    assignee: inc.assignee || '',
    notes: inc.notes || '',
    root_cause: inc.root_cause || '',
    lessons_learned: inc.lessons_learned || '',
    data_breach: !!inc.data_breach,
    gdpr_role: inc.gdpr_role || 'controller',
    authority_notified: inc.authority_notified || 'not_required',
    subjects_notified: inc.subjects_notified || 'not_required',
  }
  captureEditSnapshot()
}

function editSection(section) {
  startEdit(selectedIncident.value)
  editingSection.value = section
}

function cancelSection() {
  editingSection.value = ''
  startEdit(selectedIncident.value)
}

async function saveSection() {
  if (!selectedIncident.value) return
  saving.value = true
  try {
    await api.updateIncident(selectedIncident.value.id, { ...editForm.value })
    await loadIncidents()
    const fresh = incidents.value.find(i => i.id === selectedIncident.value.id)
    if (fresh) {
      selectedIncident.value = fresh
      startEdit(fresh)
    } else {
      // fallback: refetch
      try {
        const data = await api.fetchJSON(`/api/v1/incidents/${selectedIncident.value.id}`)
        if (data) { selectedIncident.value = data; startEdit(data) }
      } catch {}
    }
    editingSection.value = ''
    showSaved('Saved')
  } catch (e) {
    showError('Failed to save: ' + e.message)
  } finally {
    saving.value = false
  }
}

async function selectIncident(inc) {
  if (selectedIncident.value?.id === inc.id) {
    closeDetail()
    return
  }
  router.push(orgPath(`/incidents/${inc.id}`))
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
  router.push(orgPath('/incidents'))
}

function classificationLabel(inc) {
  const parts = []
  if (inc.affects_c) parts.push('C')
  if (inc.affects_i) parts.push('I')
  if (inc.affects_a) parts.push('A')
  return parts.length ? parts.join('/') : ''
}

function severityClass(sev) {
  switch (sev) {
    case 'critical': return 'bg-red-900/60 text-red-300 border border-red-800'
    case 'high': return 'bg-amber-900/60 text-amber-300 border border-amber-800'
    case 'medium': return 'bg-blue-900/60 text-blue-300 border border-blue-800'
    case 'low': return 'bg-slate-800 text-slate-400 border border-slate-700'
    default: return 'bg-slate-800 text-slate-400 border border-slate-700'
  }
}

function typeClass(type_) {
  switch (type_) {
    case 'incident': return 'bg-red-900/40 text-red-400'
    case 'event': return 'bg-amber-900/40 text-amber-400'
    case 'weakness': return 'bg-purple-900/40 text-purple-400'
    default: return 'bg-slate-800 text-slate-400'
  }
}

function statusClass(status) {
  switch (status) {
    case 'open': return 'bg-red-900/40 text-red-400'
    case 'investigating': return 'bg-amber-900/40 text-amber-400'
    case 'contained': return 'bg-blue-900/40 text-blue-400'
    case 'resolved': return 'bg-emerald-900/40 text-emerald-400'
    case 'closed': return 'bg-slate-800 text-slate-500'
    default: return 'bg-slate-800 text-slate-400'
  }
}

function formatDate(d) {
  if (!d && d !== 0) return ''
  const dt = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  return dt.toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric' })
}

function formatDateTime(d) {
  if (!d && d !== 0) return ''
  const dt = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  return dt.toLocaleString('en-GB', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}
</script>
