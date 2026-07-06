<template>
  <div class="min-h-full">
    <div class="max-w-6xl mx-auto px-8 py-10">
      <!-- Header -->
      <div class="flex items-center justify-between mb-8">
        <div>
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">Change Management</h1>
          <p class="text-sm text-slate-500 mt-1">Track and approve changes to your management system.</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canCreate" @click="showCreate = !showCreate"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            Add Change Request
          </button>
          <SuggestNewButton entityType="change_request" typeLabel="Change Request" />
        </div>
      </div>

      <!-- Create form -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreate" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreate = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
        <form @submit.prevent="create" class="space-y-4">
        <h2 class="text-sm font-semibold text-slate-300">Add Change Request</h2>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="block text-xs text-slate-500 mb-1">Type</label>
            <select v-model="form.type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500">
              <option value="change">Change</option>
              <option value="access_request">Access request</option>
            </select>
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">Title <span class="text-red-400">*</span></label>
            <input v-model="form.title" type="text" required autofocus
              class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
              placeholder="e.g. Migrate authentication to OIDC" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">Priority</label>
            <select v-model="form.priority" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500">
              <option value="low">Low</option>
              <option value="medium">Medium</option>
              <option value="high">High</option>
              <option value="critical">Critical</option>
            </select>
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">Category</label>
            <select v-model="form.category" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500">
              <option value="process">Process</option>
              <option value="technology">Technology</option>
              <option value="people">People</option>
              <option value="documentation">Documentation</option>
              <option value="infrastructure">Infrastructure</option>
              <option value="other">Other</option>
            </select>
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">You can add description, justification, planned date, risk level, assignee, rollback plan and notes after creating.</div>
        <div class="flex gap-2 pt-2">
          <button type="submit" :disabled="creating || !form.title"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50">
            {{ creating ? 'Creating...' : 'Add' }}
          </button>
          <button type="button" @click="showCreate = false" class="px-4 py-2 text-sm text-slate-400 hover:text-white">Cancel</button>
        </div>
        </form>
        </div>
      </div>
      </Transition>
      </Teleport>

      <!-- Summary stats -->
      <div class="grid grid-cols-2 lg:grid-cols-6 gap-4 mb-6">
        <button v-for="s in statusStats" :key="s.key"
          @click="filterStatus = filterStatus === s.key ? '' : s.key"
          class="bg-slate-900 border border-slate-800 rounded-xl p-4 text-left hover:border-slate-700 transition-colors"
          :class="filterStatus === s.key ? 'ring-1 ring-blue-500/40' : ''">
          <div class="text-2xl font-bold tabular-nums" :class="s.color">{{ s.count }}</div>
          <div class="text-xs text-slate-500 mt-1">{{ s.label }}</div>
        </button>
      </div>

      <!-- Search + filters -->
      <div class="flex items-center gap-3 mb-4 flex-wrap">
        <div class="relative flex-1 max-w-xs">
          <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
          </svg>
          <input v-model="searchQuery" type="text" placeholder="Search..."
            class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
        </div>
        <select v-model="filterPriority" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
          <option value="">All priorities</option>
          <option value="critical">Critical</option>
          <option value="high">High</option>
          <option value="medium">Medium</option>
          <option value="low">Low</option>
        </select>
        <select v-model="filterCategory" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
          <option value="">All categories</option>
          <option value="process">Process</option>
          <option value="technology">Technology</option>
          <option value="people">People</option>
          <option value="documentation">Documentation</option>
          <option value="infrastructure">Infrastructure</option>
          <option value="other">Other</option>
        </select>
        <div class="ml-auto text-xs text-slate-500 tabular-nums">{{ total }} total</div>
      </div>

      <!-- List -->
      <ListSkeleton v-if="loading" :rows="5" />
      <div v-else-if="changes.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
        <div class="text-slate-500 text-sm">No change requests found</div>
      </div>
      <div v-else class="space-y-2">
        <div v-for="cr in changes" :key="cr.id"
          class="bg-slate-900 border border-slate-800 rounded-lg px-5 py-4 hover:border-slate-700 transition-colors cursor-pointer"
          @click="selectChange(cr)">
          <div class="flex items-center gap-3">
            <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ cr.identifier }}</span>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="text-sm font-medium text-slate-200">{{ cr.title }}</span>
                <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold"
                  :class="statusClass(cr.status)">
                  <span class="w-1.5 h-1.5 rounded-full" :class="statusDot(cr.status)"></span>
                  {{ cr.status }}
                </span>
                <span class="px-1.5 py-0.5 rounded text-[10px] font-medium" :class="priorityClass(cr.priority)">{{ cr.priority }}</span>
                <span class="px-1.5 py-0.5 rounded text-[10px] text-slate-500 bg-slate-800">{{ cr.category }}</span>
              </div>
              <div class="text-xs text-slate-500 mt-1">
                Requested by {{ resolveUserName(cr.requested_by) }}<span v-if="cr.assigned_to"> &middot; Assigned to {{ resolveUserName(cr.assigned_to) }}</span>
                <span v-if="cr.risk_level && cr.risk_level !== 'low'"> &middot; Risk: <span :class="cr.risk_level === 'critical' ? 'text-red-400' : cr.risk_level === 'high' ? 'text-amber-400' : 'text-slate-400'">{{ cr.risk_level }}</span></span>
                &middot; {{ formatDate(cr.created_at) }}
              </div>
            </div>
          </div>
        </div>
        <Pagination :page="page" :pageSize="pageSize" :total="total" @update:page="page = $event" @update:pageSize="pageSize = $event" />
      </div>

      <!-- Detail modal (tabbed) -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="selectedChange" class="fixed inset-0 z-50 flex items-start justify-center pt-[3vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="closeDetail" />
        <div class="relative w-full max-w-4xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-h-[90vh] flex flex-col">
          <!-- Header -->
          <div class="flex-shrink-0 border-b border-slate-800 px-6 py-3 flex items-center justify-between gap-4">
            <div class="flex items-center gap-6 min-w-0">
              <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ selectedChange.identifier }}</span>
              <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selectedChange.title }}</h2>
            </div>
            <div class="flex items-center gap-2 flex-shrink-0">
              <StatusBadge :status="selectedChange.status" />
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
                    <button v-if="canCreate && !editingSection" @click="editSection('overview')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                  </div>
                  <template v-if="editingSection === 'overview'">
                    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Title</label>
                        <input v-model="editForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Description</label>
                        <MarkdownField v-model="editForm.description" :self-type="'change_request'" :self-id="selectedChange?.identifier || ''" :rows="3" placeholder="Describe the change..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Priority</label>
                        <select v-model="editForm.priority" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Category</label>
                        <select v-model="editForm.category" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="process">Process</option>
                          <option value="technology">Technology</option>
                          <option value="people">People</option>
                          <option value="documentation">Documentation</option>
                          <option value="infrastructure">Infrastructure</option>
                          <option value="other">Other</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Risk Level</label>
                        <select v-model="editForm.risk_level" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option>
                        </select>
                      </div>
                      <div v-if="canWrite">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Status</label>
                        <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="proposed">Proposed</option>
                          <option value="approved">Approved</option>
                          <option value="rejected">Rejected</option>
                          <option value="in_progress">In Progress</option>
                          <option value="implemented">Implemented</option>
                          <option value="closed">Closed</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Assigned to</label>
                        <MemberPicker v-model="editForm.assigned_to" :members="orgMembers" placeholder="Select..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Planned for</label>
                        <input v-model="editForm.planned_at_str" type="date"
                          class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Justification</label>
                        <MarkdownField v-model="editForm.justification" :self-type="'change_request'" :self-id="selectedChange?.identifier || ''" :rows="3" placeholder="Why is this change needed?" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Rollback Plan</label>
                        <MarkdownField v-model="editForm.rollback_plan" :self-type="'change_request'" :self-id="selectedChange?.identifier || ''" :rows="3" placeholder="How to revert?" />
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Description</div>
                        <div v-if="selectedChange.description" class="text-sm text-slate-300 leading-relaxed doc-prose" v-html="renderMd(selectedChange.description)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>

                      <div class="grid grid-cols-2 gap-x-8 gap-y-3">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Priority</div>
                          <span class="px-1.5 py-0.5 rounded text-[10px] font-medium" :class="priorityClass(selectedChange.priority)">{{ selectedChange.priority }}</span>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Category</div>
                          <div class="text-sm text-slate-300 capitalize">{{ selectedChange.category || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Risk Level</div>
                          <div class="text-sm font-medium" :class="selectedChange.risk_level === 'critical' ? 'text-red-400' : selectedChange.risk_level === 'high' ? 'text-amber-400' : 'text-slate-300'">{{ selectedChange.risk_level || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Requested by</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedChange.requested_by) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Assignee</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedChange.assigned_to) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created</div>
                          <div class="text-sm text-slate-300">{{ formatDate(selectedChange.created_at) }}</div>
                        </div>
                        <div v-if="selectedChange.planned_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Planned for</div>
                          <div class="text-sm text-slate-300">{{ formatDate(selectedChange.planned_at) }}</div>
                        </div>
                      </div>

                      <div class="border-t border-slate-800 pt-4 space-y-3">
                        <div v-if="selectedChange.justification">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Justification</div>
                          <div class="text-sm text-slate-300 doc-prose" v-html="renderMd(selectedChange.justification)"></div>
                        </div>
                        <div v-if="selectedChange.rollback_plan">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Rollback Plan</div>
                          <div class="text-sm text-slate-300 doc-prose" v-html="renderMd(selectedChange.rollback_plan)"></div>
                        </div>
                      </div>

                      <!-- Implementation timeline -->
                      <div class="border-t border-slate-800 pt-4">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-3">Progress</div>
                        <div class="flex items-center gap-3 text-xs text-slate-500">
                          <div class="flex items-center gap-1.5">
                            <span class="w-2.5 h-2.5 rounded-full" :class="selectedChange.status !== 'proposed' ? 'bg-blue-400' : 'bg-slate-600'"></span>
                            <span :class="selectedChange.status !== 'proposed' ? 'text-slate-300' : ''">Proposed</span>
                          </div>
                          <div class="h-px flex-1 bg-slate-800"></div>
                          <div class="flex items-center gap-1.5">
                            <span class="w-2.5 h-2.5 rounded-full" :class="['approved','in_progress','implemented','closed'].includes(selectedChange.status) ? 'bg-emerald-400' : 'bg-slate-600'"></span>
                            <span :class="['approved','in_progress','implemented','closed'].includes(selectedChange.status) ? 'text-slate-300' : ''">Approved</span>
                          </div>
                          <div class="h-px flex-1 bg-slate-800"></div>
                          <div class="flex items-center gap-1.5">
                            <span class="w-2.5 h-2.5 rounded-full" :class="['in_progress','implemented','closed'].includes(selectedChange.status) ? 'bg-amber-400' : 'bg-slate-600'"></span>
                            <span :class="['in_progress','implemented','closed'].includes(selectedChange.status) ? 'text-slate-300' : ''">In Progress</span>
                          </div>
                          <div class="h-px flex-1 bg-slate-800"></div>
                          <div class="flex items-center gap-1.5">
                            <span class="w-2.5 h-2.5 rounded-full" :class="['implemented','closed'].includes(selectedChange.status) ? 'bg-purple-400' : 'bg-slate-600'"></span>
                            <span :class="['implemented','closed'].includes(selectedChange.status) ? 'text-slate-300' : ''">Implemented</span>
                          </div>
                          <div class="h-px flex-1 bg-slate-800"></div>
                          <div class="flex items-center gap-1.5">
                            <span class="w-2.5 h-2.5 rounded-full" :class="selectedChange.status === 'closed' ? 'bg-slate-400' : 'bg-slate-600'"></span>
                            <span :class="selectedChange.status === 'closed' ? 'text-slate-300' : ''">Closed</span>
                          </div>
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
                    <button v-if="canCreate && !editingSection" @click="editSection('notes')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                  </div>
                  <template v-if="editingSection === 'notes'">
                    <MarkdownField v-model="editForm.notes" :self-type="'change_request'" :self-id="selectedChange?.identifier || ''" :rows="12" placeholder="Add notes..." />
                  </template>
                  <template v-else>
                    <div v-if="selectedChange.notes" class="text-sm doc-prose text-slate-300 leading-relaxed" v-html="renderMd(selectedChange.notes)"></div>
                    <div v-else class="text-sm text-slate-600 italic">No notes yet.</div>
                  </template>
                </div>
              </template>

              <!-- ═══ LINKS ═══ -->
              <template v-if="detailTab === 'links'">
                <div class="px-6 py-5">
                  <ReferenceManager entityType="change_request" :entityId="selectedChange.identifier" :editable="canCreate" />
                </div>
              </template>

              <!-- ═══ SUGGESTIONS ═══ -->
              <template v-if="detailTab === 'suggestions'">
                <div class="px-6 py-5">
                  <SuggestionPanel entityType="change_request" :entityId="selectedChange.identifier" :canReview="canCreate" @applied="loadChanges" />
                </div>
              </template>

              <!-- ═══ COMMENTS ═══ -->
              <template v-if="detailTab === 'comments'">
                <div class="px-6 py-5">
                  <CommentsPanel entityType="change_request" :entityId="selectedChange.identifier" />
                </div>
              </template>

              <!-- ═══ HISTORY ═══ -->
              <template v-if="detailTab === 'history'">
                <div class="px-6 py-5 space-y-6">
                  <HistoryPanel entityType="change_request" :entityId="String(selectedChange.id)" />
                  <div v-if="canCreate" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">Danger zone</div>
                    <div class="text-xs text-slate-400">Deleting this change request is permanent and cannot be undone.</div>
                    <button @click="confirmDeleteChange(selectedChange)" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                      Delete change request
                    </button>
                  </div>
                </div>
              </template>

            </div>
          </div>

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
import { api } from '../api'
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
import { useConfirm } from '../composables/useConfirm.js'
import { useToast } from '../composables/useToast.js'
import { useDirtyEdit } from '../composables/useDirtyEdit.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'

const { confirm: confirmDialog } = useConfirm()

const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()
const { success: showSaved, show: showError } = useToast()

const renderMd = renderMarkdown

const loading = ref(true)
const changes = ref([])
const filterStatus = ref('')
const filterPriority = ref('')
const filterCategory = ref('')
const searchQuery = ref('')
const page = ref(1)
const pageSize = ref(50)
const total = ref(0)
const stats = ref({ total: 0, proposed: 0, approved: 0, rejected: 0, in_progress: 0, implemented: 0, closed: 0 })
const selectedChange = ref(null)
const orgMembers = ref([])
const showCreate = ref(false)
const creating = ref(false)
const userRole = ref('')

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
  { key: 'suggestions', label: 'Suggestions' },
  { key: 'comments', label: 'Comments' },
  { key: 'history', label: 'History' },
]

useModalEscape(showCreate)
useModalEscape(computed(() => !!selectedChange.value), () => closeDetail())

const canCreate = computed(() => ['admin', 'manager', 'contributor'].includes(userRole.value))
// Status transitions are manager/admin-only (the API rejects others with 403),
// so only managers/admins see the status control (#24).
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')
const pendingRefs = ref([])

const form = ref({ type: 'change', title: '', priority: 'medium', category: 'process' })

const statusStats = computed(() => {
  return [
    { key: 'proposed', label: 'Proposed', count: stats.value.proposed || 0, color: 'text-blue-400' },
    { key: 'approved', label: 'Approved', count: stats.value.approved || 0, color: 'text-emerald-400' },
    { key: 'in_progress', label: 'In Progress', count: stats.value.in_progress || 0, color: 'text-amber-400' },
    { key: 'implemented', label: 'Implemented', count: stats.value.implemented || 0, color: 'text-purple-400' },
    { key: 'rejected', label: 'Rejected', count: stats.value.rejected || 0, color: 'text-red-400' },
    { key: 'closed', label: 'Closed', count: stats.value.closed || 0, color: 'text-slate-500' },
  ]
})

function statusClass(s) {
  switch (s) {
    case 'proposed': return 'bg-blue-500/15 text-blue-400'
    case 'approved': return 'bg-emerald-500/15 text-emerald-400'
    case 'in_progress': return 'bg-amber-500/15 text-amber-400'
    case 'rejected': return 'bg-red-500/15 text-red-400'
    case 'implemented': return 'bg-purple-500/15 text-purple-400'
    case 'closed': return 'bg-slate-500/15 text-slate-400'
    default: return 'bg-slate-500/15 text-slate-400'
  }
}

function statusDot(s) {
  switch (s) {
    case 'proposed': return 'bg-blue-400'
    case 'approved': return 'bg-emerald-400'
    case 'in_progress': return 'bg-amber-400'
    case 'rejected': return 'bg-red-400'
    case 'implemented': return 'bg-purple-400'
    default: return 'bg-slate-400'
  }
}

function priorityClass(p) {
  switch (p) {
    case 'critical': return 'bg-red-500/15 text-red-400'
    case 'high': return 'bg-amber-500/15 text-amber-400'
    case 'medium': return 'bg-blue-500/15 text-blue-400'
    case 'low': return 'bg-slate-500/15 text-slate-400'
    default: return 'bg-slate-500/15 text-slate-400'
  }
}

function formatDate(d) {
  if (!d && d !== 0) return ''
  const dt = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  return dt.toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric' })
}

async function selectChange(cr) {
  if (selectedChange.value && selectedChange.value.id === cr.id) {
    closeDetail()
    return
  }
  router.push(orgPath(`/changes/${cr.id}`))
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
  router.push(orgPath('/changes'))
}

async function confirmDeleteChange(cr) {
  const ok = await confirmDialog({ message: 'Delete this change request? This cannot be undone.', variant: 'danger', confirmLabel: 'Delete' })
  if (!ok) return
  try {
    await api.deleteJSON(`/api/v1/changes/${cr.id}`)
    closeDetail()
    await loadChanges()
  } catch (e) {
    console.error('Failed to delete change request:', e)
  }
}

async function loadChanges() {
  try {
    const params = new URLSearchParams()
    params.set('page', String(page.value))
    params.set('limit', String(pageSize.value))
    if (searchQuery.value) params.set('q', searchQuery.value)
    if (filterStatus.value) params.set('status', filterStatus.value)
    if (filterPriority.value) params.set('priority', filterPriority.value)
    if (filterCategory.value) params.set('category', filterCategory.value)
    const res = await api.fetchRaw(`/api/v1/changes?${params.toString()}`)
    changes.value = Array.isArray(res?.data) ? res.data : []
    total.value = res?.total || 0
    loadStats()
  } catch (e) {
    changes.value = []
    showError(e.message || 'Failed to load changes')
  }
}

async function loadStats() {
  try { stats.value = await api.fetchJSON('/api/v1/changes/stats') || stats.value } catch {}
}

function resolveUserName(email) {
  if (!email) return '—'
  const u = orgMembers.value.find(m => m.email === email)
  return u?.name || email
}

let searchTimer = null
watch([searchQuery], () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { page.value = 1; loadChanges() }, 250)
})
watch([filterStatus, filterPriority, filterCategory], () => {
  page.value = 1
  loadChanges()
})
watch([page, pageSize], () => loadChanges())

async function create() {
  if (creating.value) return
  creating.value = true
  try {
    const created = await api.createChange(form.value)
    const entityId = created?.identifier || ''
    if (entityId) {
      for (const ref of pendingRefs.value) {
        try {
          await api.createReference({ source_type: 'change_request', source_id: entityId, target_type: ref.type, target_id: ref.id })
        } catch { /* non-fatal */ }
      }
    }
    pendingRefs.value = []
    form.value = { type: 'change', title: '', priority: 'medium', category: 'process' }
    showCreate.value = false
    await loadChanges()
    // Drop user into detail modal in edit mode on Overview to keep filling things in.
    if (created && created.id) {
      let fresh = created
      try { fresh = await api.getChange(created.id) } catch { /* fall back */ }
      selectedChange.value = fresh
      detailTab.value = 'overview'
      startEdit(fresh)
      editingSection.value = 'overview'
      router.push(orgPath(`/changes/${fresh.id}`))
    }
  } catch (e) {
    showError(e.message || 'Failed to create change request')
  } finally {
    creating.value = false
  }
}

async function changeStatus(id, status) {
  try {
    await api.updateChangeStatus(id, status)
    await loadChanges()
    if (selectedChange.value?.id === id) {
      const updated = changes.value.find(c => c.id === id)
      if (updated) selectedChange.value = updated
    }
  } catch (e) {
    showError(e.message || 'Status change failed')
  }
}

function startEdit(cr) {
  editForm.value = {
    title: cr.title || '',
    description: cr.description || '',
    justification: cr.justification || '',
    priority: cr.priority || 'medium',
    category: cr.category || 'process',
    risk_level: cr.risk_level || 'low',
    status: cr.status || 'proposed',
    rollback_plan: cr.rollback_plan || '',
    notes: cr.notes || '',
    assigned_to: cr.assigned_to || '',
    planned_at_str: epochToDateStr(cr.planned_at),
  }
  captureEditSnapshot()
}

function epochToDateStr(d) {
  if (!d) return ''
  const dt = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  return dt.toISOString().split('T')[0]
}

function dateStrToEpoch(str) {
  if (!str) return null
  return Math.floor(new Date(str + 'T00:00:00Z').getTime() / 1000)
}

function editSection(section) {
  startEdit(selectedChange.value)
  editingSection.value = section
}

function cancelSection() {
  editingSection.value = ''
  startEdit(selectedChange.value)
}

async function saveSection() {
  if (!selectedChange.value) return
  saving.value = true
  try {
    const payload = { ...editForm.value }
    payload.planned_at = dateStrToEpoch(payload.planned_at_str)
    delete payload.planned_at_str
    await api.updateChange(selectedChange.value.id, payload)
    await loadChanges()
    const fresh = changes.value.find(c => c.id === selectedChange.value.id)
    if (fresh) {
      selectedChange.value = fresh
      startEdit(fresh)
    }
    editingSection.value = ''
    showSaved('Saved')
  } catch (e) {
    showError('Failed to save: ' + e.message)
  } finally {
    saving.value = false
  }
}

async function openChangeFromRoute(id) {
  const numId = parseInt(id)
  let cr = changes.value.find(c => c.id === numId || c.identifier === id)
  if (!cr) {
    try { cr = await api.fetchJSON(`/api/v1/changes/${numId}`) } catch { return }
  }
  if (!cr) return
  selectedChange.value = cr
  detailTab.value = 'overview'
  editingSection.value = ''
  startEdit(cr)
}

watch(() => route.params.id, (id) => {
  if (!id) {
    selectedChange.value = null
    detailTab.value = 'overview'
    editingSection.value = ''
    return
  }
  if (selectedChange.value?.id === parseInt(id)) return
  openChangeFromRoute(id)
}, { immediate: false })

onMounted(async () => {
  try { const me = await api.getMe(); userRole.value = me?.role || '' } catch {}
  try { orgMembers.value = await api.getUsers() || [] } catch { orgMembers.value = [] }
  await loadChanges()
  if (route.params.id) await openChangeFromRoute(route.params.id)
  loading.value = false
})
</script>
