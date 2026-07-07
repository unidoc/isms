<template>
  <div class="min-h-full">
    <!-- Loading -->
    <div v-if="loading" class="max-w-6xl mx-auto px-8 py-10">
      <ListSkeleton :rows="5" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="max-w-6xl mx-auto px-8 py-12">
      <div class="bg-red-950/40 border border-red-900/50 rounded-lg p-6 text-red-300 text-sm">
        {{ error }}
      </div>
    </div>

    <!-- Main content -->
    <div v-else class="max-w-6xl mx-auto px-8 py-10 space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">Corrective Actions</h1>
          <p class="text-sm text-slate-500 mt-1">Track nonconformities, observations, and opportunities for improvement</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canWrite"
            @click="showCreateForm = !showCreateForm"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            Add Corrective Action
          </button>
          <SuggestNewButton entityType="corrective_action" typeLabel="Corrective Action" />
        </div>
      </div>

      <!-- Stats cards -->
      <div class="grid grid-cols-2 lg:grid-cols-6 gap-4">
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-red-400 tabular-nums">{{ stats.todo || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Todo</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-amber-400 tabular-nums">{{ stats.assessment || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Assessment</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-purple-400 tabular-nums">{{ stats.awaiting_approval || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Awaiting Approval</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-blue-400 tabular-nums">{{ stats.implementation || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Implementation</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-cyan-400 tabular-nums">{{ stats.monitoring || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Monitoring</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-emerald-400 tabular-nums">{{ stats.resolved || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Resolved</div>
        </div>
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
          <option value="todo">Todo</option>
          <option value="assessment">Assessment</option>
          <option value="awaiting_approval">Awaiting Approval</option>
          <option value="implementation">Implementation</option>
          <option value="monitoring">Monitoring</option>
          <option value="resolved">Resolved</option>
        </select>
        <select v-model="filterSeverity" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
          <option value="">All severities</option>
          <option value="major_nc">Major NC</option>
          <option value="minor_nc">Minor NC</option>
          <option value="observation">Observation</option>
          <option value="opportunity">OFI</option>
        </select>
        <select v-model="filterSource" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
          <option value="">All sources</option>
          <option value="internal_audit">Internal Audit</option>
          <option value="external_audit">External Audit</option>
          <option value="risk_assessment">Risk Assessment</option>
          <option value="security_incident">Security Incident</option>
          <option value="objective">Objective</option>
          <option value="feedback">Feedback</option>
          <option value="other">Other</option>
        </select>
        <div class="w-48">
          <MemberPicker v-model="filterAssignee" :members="orgMembers" placeholder="Any assignee" />
        </div>
        <div class="ml-auto text-xs text-slate-500 tabular-nums">{{ total }} total</div>
      </div>

      <!-- Create form -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreateForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreateForm = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
        <div class="flex items-center justify-between mb-2">
          <h2 class="text-sm font-semibold text-slate-200">Add Corrective Action</h2>
          <button @click="showCreateForm = false" class="text-slate-500 hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div class="sm:col-span-2">
            <label class="block text-xs font-medium text-slate-500 mb-1">Title *</label>
            <input v-model="newCA.title" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="Brief title of the nonconformity or improvement" />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Source</label>
            <select v-model="newCA.source" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option value="internal_audit">Internal Audit Finding</option>
              <option value="external_audit">External Audit Finding</option>
              <option value="risk_assessment">Risk Assessment</option>
              <option value="security_incident">Security Incident</option>
              <option value="objective">Objective</option>
              <option value="feedback">Feedback / Suggestion</option>
              <option value="other">Other</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Severity</label>
            <select v-model="newCA.severity" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option value="major_nc">Major Non-conformity</option>
              <option value="minor_nc">Minor Non-conformity</option>
              <option value="observation">Observation</option>
              <option value="opportunity">Opportunity for Improvement</option>
            </select>
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">You can add description, assignee, due date and notes after creating.</div>
        <div class="flex justify-end gap-3 pt-2">
          <button @click="showCreateForm = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
          <button @click="createCA" :disabled="!newCA.title" class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
            Add
          </button>
        </div>
        </div>
      </div>
      </Transition>
      </Teleport>

      <!-- Action list -->
      <div v-if="actions.length === 0" class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
        <div v-if="filterStatus || filterSeverity || filterSource || filterAssignee || searchQuery" class="text-slate-500 text-sm">
          No corrective actions match your filter.
        </div>
        <div v-else class="text-slate-500 text-sm">
          No corrective actions yet — click Add to create your first one.
        </div>
      </div>

      <div v-else class="space-y-2">
        <div v-for="ca in actions" :key="ca.id"
          @click="selectCA(ca)"
          class="bg-slate-900 border border-slate-800 rounded-lg p-4 hover:border-slate-700 transition-colors cursor-pointer">
          <div class="flex items-center gap-3">
            <!-- Severity badge -->
            <span class="inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-full uppercase tracking-wider whitespace-nowrap"
              :class="severityClass(ca.severity)">
              {{ severityLabel(ca.severity) }}
            </span>
            <!-- Source badge -->
            <span class="inline-flex items-center px-2 py-0.5 text-[11px] font-medium rounded-full whitespace-nowrap bg-slate-800 text-slate-400">
              {{ sourceLabel(ca.source) }}
            </span>
            <!-- Status badge -->
            <StatusBadge :status="ca.status" />
            <!-- Title -->
            <span class="text-sm font-medium text-slate-200 flex-1 truncate">{{ ca.title }}</span>
            <!-- Assignee -->
            <span v-if="ca.assignee" class="text-xs text-slate-500 truncate max-w-[160px]">{{ resolveUserName(ca.assignee) }}</span>
            <!-- Due date -->
            <span v-if="ca.due_date" class="text-xs" :class="isOverdue(ca.due_date) && ca.status !== 'resolved' ? 'text-red-400' : 'text-slate-600'">
              {{ ca.due_date }}
            </span>
            <!-- ID -->
            <span class="text-xs text-slate-600 font-mono">{{ ca.identifier }}</span>
          </div>
        </div>
        <Pagination :page="page" :pageSize="pageSize" :total="total" @update:page="page = $event" @update:pageSize="pageSize = $event" />
      </div>

      <!-- Detail modal -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="selectedCA" class="fixed inset-0 z-50 flex items-start justify-center pt-[3vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="closeDetail" />
        <div class="relative w-full max-w-4xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-h-[90vh] flex flex-col">
          <!-- Header -->
          <div class="flex-shrink-0 border-b border-slate-800 px-6 py-3 flex items-center justify-between gap-4">
            <div class="flex items-center gap-6 min-w-0">
              <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ selectedCA.identifier }}</span>
              <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selectedCA.title }}</h2>
            </div>
            <div class="flex items-center gap-2 flex-shrink-0">
              <StatusBadge :status="selectedCA.status" />
              <button @click="closeDetail" class="p-1 rounded-lg text-slate-600 hover:text-slate-300 hover:bg-slate-800 transition-colors">
                <svg class="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          </div>

          <!-- Body: sidebar nav + content -->
          <div class="flex flex-1 min-h-0">
            <!-- Sidebar nav -->
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
                        <MarkdownField v-model="editForm.description" :self-type="'corrective_action'" :self-id="selectedCA?.identifier || ''" :rows="3" placeholder="Describe the corrective action..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Source</label>
                        <select v-model="editForm.source" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="(label, key) in sourceLabels" :key="key" :value="key">{{ label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Severity</label>
                        <select v-model="editForm.severity" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="(label, key) in severityFullLabels" :key="key" :value="key">{{ label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Status</label>
                        <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="todo">To do</option>
                          <option value="assessment">Assessment</option>
                          <option value="awaiting_approval">Awaiting approval</option>
                          <option value="implementation">Implementation</option>
                          <option value="monitoring">Monitoring</option>
                          <option value="resolved">Resolved</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Assignee</label>
                        <MemberPicker v-model="editForm.assignee" :members="orgMembers" placeholder="Select assignee..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Due date</label>
                        <input v-model="editForm.due_date" type="date" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Root Cause</label>
                        <MarkdownField v-model="editForm.root_cause" :self-type="'corrective_action'" :self-id="selectedCA?.identifier || ''" :rows="3" placeholder="Root cause analysis..." />
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Description</div>
                        <div v-if="selectedCA.description" class="text-sm text-slate-300 leading-relaxed doc-prose" v-html="renderMd(selectedCA.description)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>
                      <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Source</div>
                          <div class="text-sm text-slate-300">{{ sourceLabel(selectedCA.source) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Severity</div>
                          <span class="inline-flex items-center px-2 py-0.5 text-[10px] font-medium rounded" :class="severityClass(selectedCA.severity)">{{ severityLabel(selectedCA.severity) }}</span>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Assignee</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedCA.assignee) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Due date</div>
                          <div class="text-sm" :class="selectedCA.due_date && isOverdue(selectedCA.due_date) && selectedCA.status !== 'resolved' ? 'text-red-400' : 'text-slate-300'">{{ selectedCA.due_date ? formatDate(selectedCA.due_date) : '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created by</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedCA.created_by) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created</div>
                          <div class="text-sm text-slate-300">{{ formatDate(selectedCA.created_at) }}</div>
                        </div>
                        <div v-if="selectedCA.resolved_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Resolved</div>
                          <div class="text-sm text-emerald-400">{{ formatDateTime(selectedCA.resolved_at) }}<span v-if="selectedCA.resolved_by" class="text-slate-500"> by {{ resolveUserName(selectedCA.resolved_by) }}</span></div>
                        </div>
                      </div>

                      <div v-if="selectedCA.root_cause" class="border-t border-slate-800 pt-4">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Root Cause</div>
                        <div class="text-sm text-slate-300 doc-prose" v-html="renderMd(selectedCA.root_cause)"></div>
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
                        <div class="text-xs text-slate-500 mt-0.5">Spawn a task to do the corrective work; auto-linked back to this CA.</div>
                      </div>
                      <span class="text-slate-500 text-lg">→</span>
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
                    <MarkdownField v-model="editForm.notes" :self-type="'corrective_action'" :self-id="selectedCA?.identifier || ''" :rows="12" placeholder="Add notes..." />
                  </template>
                  <template v-else>
                    <div v-if="selectedCA.notes" class="text-sm doc-prose text-slate-300 leading-relaxed" v-html="renderMd(selectedCA.notes)"></div>
                    <div v-else class="text-sm text-slate-600 italic">No notes yet.</div>
                  </template>
                </div>
              </template>

              <!-- ═══ LINKS ═══ -->
              <template v-if="detailTab === 'links'">
                <div class="px-6 py-5">
                  <ReferenceManager entityType="corrective_action" :entityId="selectedCA.identifier" :editable="canWrite" />
                </div>
              </template>

              <!-- ═══ SUGGESTIONS ═══ -->
              <template v-if="detailTab === 'suggestions'">
                <div class="px-6 py-5">
                  <SuggestionPanel entityType="corrective_action" :entityId="selectedCA.identifier" :canReview="canWrite" @applied="loadAll" />
                </div>
              </template>

              <!-- ═══ COMMENTS ═══ -->
              <template v-if="detailTab === 'comments'">
                <div class="px-6 py-5">
                  <CommentsPanel entityType="corrective_action" :entityId="selectedCA.identifier" />
                </div>
              </template>

              <!-- ═══ HISTORY ═══ -->
              <template v-if="detailTab === 'history'">
                <div class="px-6 py-5 space-y-6">
                  <HistoryPanel entityType="corrective_action" :entityId="String(selectedCA.id)" />
                  <div v-if="canWrite" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">Danger zone</div>
                    <div class="text-xs text-slate-400">Deleting this corrective action is permanent and cannot be undone.</div>
                    <button @click="deleteCA(selectedCA.id)" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                      Delete corrective action
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
import MemberPicker from '../components/MemberPicker.vue'
import MarkdownField from '../components/MarkdownField.vue'
import StatusBadge from '../components/StatusBadge.vue'
import EntityReferences from '../components/EntityReferences.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
import SuggestionPanel from '../components/SuggestionPanel.vue'
import SuggestNewButton from '../components/SuggestNewButton.vue'
import HistoryPanel from '../components/HistoryPanel.vue'
import CommentsPanel from '../components/CommentsPanel.vue'
import Pagination from '../components/Pagination.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import { useModalEscape } from '../composables/useModalEscape.js'
import { useConfirm } from '../composables/useConfirm.js'
import { useToast } from '../composables/useToast.js'
import { useDirtyEdit } from '../composables/useDirtyEdit.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import { renderMarkdown } from '../composables/useRenderMd.js'

const { confirm: confirmDialog } = useConfirm()

const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()
const { success: showSaved, show: showError } = useToast()

const renderMd = renderMarkdown

const userRole = ref('')
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')

const orgMembers = ref([])

const loading = ref(true)
const error = ref(null)
const actions = ref([])
const stats = ref({})
const selectedCA = ref(null)
const showCreateForm = ref(false)

// Tab-based detail state
const detailTab = ref('overview')
const editingSection = ref('')
const editForm = ref({})
const { capture: captureEditSnapshot, isDirty } = useDirtyEdit(editForm)
const saving = ref(false)

const detailTabs = [
  { key: 'overview', label: 'Overview' },
  { key: 'notes', label: 'Notes' },
  { key: 'links', label: 'Links' },
  { key: 'actions', label: 'Actions' },
  { key: 'suggestions', label: 'Suggestions' },
  { key: 'comments', label: 'Comments' },
  { key: 'history', label: 'History' },
]

useModalEscape(showCreateForm)
useModalEscape(computed(() => !!selectedCA.value), () => closeDetail())
const filterStatus = ref('')
const filterSeverity = ref('')
const filterAssignee = ref('')
const filterSource = ref('')
const searchQuery = ref('')
const page = ref(1)
const pageSize = ref(50)
const total = ref(0)
const docIDsInput = ref('')
const controlIDsInput = ref('')
const pendingRefs = ref([])

const newCA = ref({
  title: '',
  description: '',
  source: 'other',
  severity: 'observation',
  assignee: '',
  due_date: '',
  notes: '',
})

onMounted(async () => {
  try { const me = await api.getMe(); userRole.value = me?.role || '' } catch {}
  try { orgMembers.value = await api.getUsers() || [] } catch { orgMembers.value = [] }
  await loadAll()
  if (route.params.id) await openCAFromRoute(route.params.id)
  // Handle "create linked from X" query params — the source link is registered
  // as an entity_reference after creation; the form just pre-fills sane defaults.
  if (route.query.from_incident) {
    newCA.value = {
      title: route.query.title ? String(route.query.title) : '',
      description: '',
      source: 'security_incident',
      severity: route.query.severity ? String(route.query.severity) : 'observation',
      assignee: '',
      due_date: '',
      notes: '',
    }
    showCreateForm.value = true
  } else if (route.query.from_audit_finding) {
    newCA.value = {
      title: route.query.title ? String(route.query.title) : '',
      description: '',
      source: 'internal_audit',
      severity: route.query.severity ? String(route.query.severity) : 'observation',
      assignee: '',
      due_date: '',
      notes: '',
    }
    showCreateForm.value = true
  } else if (route.query.from_risk) {
    newCA.value = {
      title: route.query.title ? String(route.query.title) : '',
      description: '',
      source: 'risk_assessment',
      severity: route.query.severity ? String(route.query.severity) : 'observation',
      assignee: '',
      due_date: '',
      notes: '',
    }
    showCreateForm.value = true
  }
})

watch(() => route.params.id, (id) => {
  if (!id) {
    selectedCA.value = null
    detailTab.value = 'overview'
    editingSection.value = ''
    return
  }
  if (selectedCA.value?.id === parseInt(id)) return
  openCAFromRoute(id)
})

let searchTimer = null
let assigneeTimer = null
watch([searchQuery], () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { page.value = 1; loadActions() }, 250)
})
watch([filterAssignee], () => {
  clearTimeout(assigneeTimer)
  assigneeTimer = setTimeout(() => { page.value = 1; loadActions() }, 250)
})
watch([filterStatus, filterSeverity, filterSource], () => {
  page.value = 1
  loadActions()
})
watch([page, pageSize], () => loadActions())

async function loadAll() {
  loading.value = true
  error.value = null
  try {
    await Promise.all([loadActions(), loadStats()])
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function loadActions() {
  try {
    const params = new URLSearchParams()
    params.set('page', String(page.value))
    params.set('limit', String(pageSize.value))
    if (searchQuery.value) params.set('q', searchQuery.value)
    if (filterStatus.value) params.set('status', filterStatus.value)
    if (filterSeverity.value) params.set('severity', filterSeverity.value)
    if (filterSource.value) params.set('source', filterSource.value)
    if (filterAssignee.value) params.set('assignee', filterAssignee.value)
    const res = await api.fetchRaw(`/api/v1/corrective-actions?${params.toString()}`)
    actions.value = Array.isArray(res?.data) ? res.data : []
    total.value = res?.total || 0
    loadStats()
  } catch (e) {
    error.value = e.message
  }
}

async function loadStats() {
  try {
    stats.value = await api.fetchJSON('/api/v1/corrective-actions/stats') || stats.value
  } catch (e) {
    // stats are non-critical
  }
}

function resolveUserName(email) {
  if (!email) return '—'
  const u = orgMembers.value.find(m => m.email === email)
  return u?.name || email
}

async function openCAFromRoute(id) {
  const numId = parseInt(id)
  let ca = actions.value.find(a => a.id === numId || a.identifier === id)
  if (!ca) {
    try { ca = await api.fetchJSON(`/api/v1/corrective-actions/${numId}`) } catch { return }
  }
  if (!ca) return
  selectedCA.value = ca
  detailTab.value = 'overview'
  editingSection.value = ''
  startEdit(ca)
}

async function createCA() {
  try {
    const payload = { ...newCA.value }
    // Source links come from quick-action query params; the same info also seeds
    // a one-line markdown reference into Notes so the source is visible inline
    // (Links tab still owns the formal cross-entity reference).
    const sourceLinks = []
    const seedLines = []
    const sourceTitle = route.query.title ? String(route.query.title) : ''
    if (route.query.from_incident) {
      const id = 'INC-' + String(route.query.from_incident)
      sourceLinks.push({ type: 'incident', id })
      const label = sourceTitle ? `${id}: ${sourceTitle}` : id
      seedLines.push(`Created from [${label}](/incidents/${id})`)
    }
    if (route.query.from_audit_finding) {
      const id = 'FIND-' + String(route.query.from_audit_finding)
      sourceLinks.push({ type: 'audit_finding', id })
      const label = sourceTitle ? `${id}: ${sourceTitle}` : id
      seedLines.push(`Created from audit finding ${label}`)
    }
    if (route.query.from_risk) {
      const id = String(route.query.from_risk)
      sourceLinks.push({ type: 'risk', id })
      const label = sourceTitle ? `${id}: ${sourceTitle}` : id
      seedLines.push(`Created from [${label}](/risks/${id})`)
    }
    if (seedLines.length > 0) {
      payload.notes = seedLines.join('\n') + (payload.notes ? '\n\n' + payload.notes : '')
    }
    // Clean up optional fields
    if (!payload.due_date) delete payload.due_date
    if (!payload.assignee) delete payload.assignee
    if (!payload.description) delete payload.description
    if (!payload.notes) delete payload.notes
    const created = await api.createCorrectiveAction(payload)
    const entityId = created?.identifier || ''
    if (entityId) {
      for (const ref of [...sourceLinks, ...pendingRefs.value]) {
        try {
          await api.createReference({ source_type: 'corrective_action', source_id: entityId, target_type: ref.type, target_id: ref.id })
        } catch { /* non-fatal, entity was already created */ }
      }
    }
    pendingRefs.value = []
    showCreateForm.value = false
    newCA.value = { title: '', description: '', source: 'other', severity: 'observation', assignee: '', due_date: '', notes: '' }
    docIDsInput.value = ''
    controlIDsInput.value = ''
    await loadActions()
    // Drop user into detail modal in edit mode on Overview to keep filling things in.
    if (created && created.id) {
      let fresh = created
      try { fresh = await api.getCorrectiveAction(created.id) } catch { /* fall back to create response */ }
      selectedCA.value = fresh
      detailTab.value = 'overview'
      startEdit(fresh)
      editingSection.value = 'overview'
      router.push(orgPath(`/corrective-actions/${fresh.id}`))
    }
  } catch (e) {
    error.value = e.message
  }
}

async function changeStatus(ca, status) {
  try {
    await api.updateCorrectiveActionStatus(ca.id, status)
    ca.status = status
    if (selectedCA.value?.id === ca.id) {
      selectedCA.value = { ...ca }
    }
    await loadStats()
  } catch (e) {
    showError(e.message || 'Status change failed')
  }
}

async function saveField(id, field, value) {
  try {
    await api.updateCorrectiveAction(id, { [field]: value })
  } catch (e) {
    // silent save
  }
}

function createLinkedTask() {
  if (!selectedCA.value) return
  router.push({
    path: orgPath('/tasks'),
    query: {
      from_ca: selectedCA.value.identifier,
      title: selectedCA.value.title,
      task_type: 'corrective_action',
      priority: selectedCA.value.severity === 'major_nc' ? 'high' : 'medium',
    },
  })
}

async function deleteCA(id) {
  const ok = await confirmDialog({ message: 'Delete this corrective action? This cannot be undone.', variant: 'danger', confirmLabel: 'Delete' })
  if (!ok) return
  try {
    await api.deleteCorrectiveAction(id)
    closeDetail()
    await loadActions()
  } catch (e) {
    error.value = e.message
  }
}

async function selectCA(ca) {
  if (selectedCA.value && selectedCA.value.id === ca.id) {
    closeDetail()
    return
  }
  router.push(orgPath(`/corrective-actions/${ca.id}`))
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
  router.push(orgPath('/corrective-actions'))
}

function startEdit(ca) {
  editForm.value = {
    title: ca.title || '',
    description: ca.description || '',
    source: ca.source || 'other',
    severity: ca.severity || 'observation',
    status: ca.status || 'todo',
    assignee: ca.assignee || '',
    due_date: ca.due_date ? (typeof ca.due_date === 'number' ? new Date(ca.due_date * 1000).toISOString().slice(0, 10) : String(ca.due_date).slice(0, 10)) : '',
    root_cause: ca.root_cause || '',
    notes: ca.notes || '',
  }
  captureEditSnapshot()
}

function editSection(section) {
  startEdit(selectedCA.value)
  editingSection.value = section
}

function cancelSection() {
  editingSection.value = ''
  startEdit(selectedCA.value)
}

async function saveSection() {
  if (!selectedCA.value) return
  saving.value = true
  try {
    const payload = { ...editForm.value }
    if (!payload.due_date) delete payload.due_date
    await api.updateCorrectiveAction(selectedCA.value.id, payload)
    await loadActions()
    const fresh = actions.value.find(a => a.id === selectedCA.value.id)
    if (fresh) {
      selectedCA.value = fresh
      startEdit(fresh)
    } else {
      try {
        const data = await api.fetchJSON(`/api/v1/corrective-actions/${selectedCA.value.id}`)
        if (data) { selectedCA.value = data; startEdit(data) }
      } catch { /* ignore */ }
    }
    editingSection.value = ''
    showSaved('Saved')
  } catch (e) {
    showError('Failed to save: ' + e.message)
  } finally {
    saving.value = false
  }
}

function isOverdue(dateStr) {
  if (!dateStr && dateStr !== 0) return false
  const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
  return d < new Date()
}

const severityLabels = {
  major_nc: 'Major NC',
  minor_nc: 'Minor NC',
  observation: 'Observation',
  opportunity: 'OFI',
}

const severityFullLabels = {
  major_nc: 'Major Non-conformity',
  minor_nc: 'Minor Non-conformity',
  observation: 'Observation',
  opportunity: 'Opportunity for Improvement',
}

const sourceLabels = {
  internal_audit: 'Internal Audit Finding',
  external_audit: 'External Audit Finding',
  risk_assessment: 'Risk Assessment',
  security_incident: 'Security Incident',
  objective: 'Objective',
  feedback: 'Feedback / Suggestion',
  other: 'Other',
}

const statusLabels = {
  todo: 'Todo',
  assessment: 'Assessment',
  awaiting_approval: 'Awaiting Approval',
  implementation: 'Implementation',
  monitoring: 'Monitoring',
  resolved: 'Resolved',
}

function severityLabel(sev) {
  return severityLabels[sev] || sev
}

function sourceLabel(src) {
  return sourceLabels[src] || src
}

function statusLabel(st) {
  return statusLabels[st] || st
}

function severityClass(sev) {
  switch (sev) {
    case 'major_nc': return 'bg-red-900/60 text-red-300 border border-red-800'
    case 'minor_nc': return 'bg-amber-900/60 text-amber-300 border border-amber-800'
    case 'observation': return 'bg-blue-900/60 text-blue-300 border border-blue-800'
    case 'opportunity': return 'bg-emerald-900/60 text-emerald-300 border border-emerald-800'
    default: return 'bg-slate-800 text-slate-400 border border-slate-700'
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
