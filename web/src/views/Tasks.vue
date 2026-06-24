<template>
  <div class="min-h-full">
    <div class="max-w-6xl mx-auto px-8 py-10">
      <!-- Header -->
      <div class="flex items-center justify-between mb-8">
        <div>
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">Tasks</h1>
          <p class="text-sm text-slate-500 mt-1">Review tasks, follow-ups, and operational work items.</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canCreate" @click="form = defaultForm(); showCreate = !showCreate"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            Add Task
          </button>
          <SuggestNewButton entityType="task" typeLabel="Task" />
        </div>
      </div>

      <!-- Create form -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreate" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreate = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
        <form @submit.prevent="create" class="space-y-4">
        <h2 class="text-sm font-semibold text-slate-300">Add Task</h2>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div class="sm:col-span-2">
            <label class="block text-xs text-slate-500 mb-1">Title <span class="text-red-400">*</span></label>
            <input v-model="form.title" type="text" required autofocus
              class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
              placeholder="e.g. Review risk register" />
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
            <label class="block text-xs text-slate-500 mb-1">Type</label>
            <select v-model="form.task_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500">
              <!-- Manual task types only. The *_followup types are created
                   automatically by the system, not chosen here (#32). -->
              <option value="general">General</option>
              <option value="review">Review</option>
              <option value="onboarding">Onboarding</option>
              <option value="offboarding">Offboarding</option>
              <option value="training">Training</option>
              <option value="other">Other</option>
            </select>
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">You can add description, assignee, due date and notes after creating.</div>
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
      <!-- Note: 'overdue' tile uses stats.overdue if backend provides it; otherwise 0. -->
      <div class="grid grid-cols-2 lg:grid-cols-5 gap-4 mb-6">
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
        <select v-model="filterTaskType" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
          <option value="">All types</option>
          <option value="general">General</option>
          <option value="review">Review</option>
          <option value="incident_followup">Incident follow-up</option>
          <option value="audit_followup">Audit follow-up</option>
          <option value="ca_followup">Corrective action follow-up</option>
          <option value="change_followup">Change follow-up</option>
          <option value="onboarding">Onboarding</option>
          <option value="offboarding">Offboarding</option>
          <option value="training">Training</option>
          <option value="other">Other</option>
        </select>
        <div class="w-48">
          <MemberPicker v-model="filterAssignee" :members="orgMembers" placeholder="Any assignee" />
        </div>
        <div class="ml-auto text-xs text-slate-500 tabular-nums">{{ total }} total</div>
      </div>

      <!-- Loading -->
      <ListSkeleton v-if="loading" :rows="5" />

      <!-- Empty -->
      <div v-else-if="tasks.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
        <div v-if="filterStatus || filterPriority || filterTaskType || filterAssignee || searchQuery" class="text-slate-500 text-sm">
          No tasks match your filter.
        </div>
        <template v-else>
          <div class="text-slate-500 text-sm">No tasks yet — click Add to create your first one.</div>
          <div class="text-xs text-slate-600 mt-2">Tasks are also generated automatically from overdue review cycles.</div>
        </template>
      </div>

      <!-- Task list -->
      <div v-else class="space-y-2">
        <div v-for="task in tasks" :key="task.id"
          class="bg-slate-900 border rounded-lg px-5 py-4 transition-colors cursor-pointer"
          :class="isOverdue(task) ? 'border-red-900/50' : 'border-slate-800 hover:border-slate-700'"
          @click="selectTask(task)">
          <div class="flex items-center gap-3">
            <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ task.identifier }}</span>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="text-sm font-medium text-slate-200">{{ task.title }}</span>
                <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold"
                  :class="statusClass(task.status)">
                  <span class="w-1.5 h-1.5 rounded-full" :class="statusDot(task.status)"></span>
                  {{ task.status.replace(/_/g, ' ') }}
                </span>
                <span class="px-1.5 py-0.5 rounded text-[10px] font-medium" :class="priorityClass(task.priority)">{{ task.priority }}</span>
                <span v-if="task.task_type && task.task_type !== 'general'" class="px-1.5 py-0.5 rounded text-[10px] text-slate-500 bg-slate-800">{{ task.task_type.replace(/_/g, ' ') }}</span>
              </div>
              <div class="text-xs text-slate-500 mt-1">
                <span v-if="task.assignee">{{ resolveUserName(task.assignee) }}</span>
                <span v-if="task.due_date" class="ml-2" :class="isOverdue(task) ? 'text-red-400' : ''">
                  Due {{ formatDate(task.due_date) }}
                  <span v-if="isOverdue(task)" class="text-red-400 font-semibold ml-1">OVERDUE</span>
                </span>
                <span v-if="task.created_by" class="ml-2 text-slate-600">by {{ resolveUserName(task.created_by) }}</span>
              </div>
            </div>
          </div>
        </div>
        <Pagination :page="page" :pageSize="pageSize" :total="total" @update:page="page = $event" @update:pageSize="pageSize = $event" />
      </div>

      <!-- Detail modal (tabbed) -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="selectedTask" class="fixed inset-0 z-50 flex items-start justify-center pt-[3vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="closeDetail" />
        <div class="relative w-full max-w-4xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-h-[90vh] flex flex-col">
          <!-- Header -->
          <div class="flex-shrink-0 border-b border-slate-800 px-6 py-3 flex items-center justify-between gap-4">
            <div class="flex items-center gap-6 min-w-0">
              <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ selectedTask.identifier }}</span>
              <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selectedTask.title }}</h2>
            </div>
            <div class="flex items-center gap-2 flex-shrink-0">
              <StatusBadge :status="selectedTask.status" />
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
                        <MarkdownField v-model="editForm.description" :self-type="'task'" :self-id="selectedTask?.identifier || ''" :rows="3" placeholder="Describe the task..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Assignee</label>
                        <MemberPicker v-model="editForm.assignee" :members="orgMembers" placeholder="Select assignee..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Priority</label>
                        <select v-model="editForm.priority" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Status</label>
                        <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="open">Open</option>
                          <option value="in_progress">In Progress</option>
                          <option value="done">Done</option>
                          <option value="cancelled">Cancelled</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Due date</label>
                        <input v-model="editForm.due_date_str" type="date"
                          class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Type</label>
                        <select v-model="editForm.task_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <!-- All real task types, so an auto-generated task's
                               type is representable when editing (#32). -->
                          <option value="general">General</option>
                          <option value="review">Review</option>
                          <option value="incident_followup">Incident follow-up</option>
                          <option value="audit_followup">Audit follow-up</option>
                          <option value="ca_followup">Corrective action follow-up</option>
                          <option value="change_followup">Change follow-up</option>
                          <option value="onboarding">Onboarding</option>
                          <option value="offboarding">Offboarding</option>
                          <option value="training">Training</option>
                          <option value="other">Other</option>
                        </select>
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Description</div>
                        <div v-if="selectedTask.description" class="text-sm text-slate-300 leading-relaxed doc-prose" v-html="renderMd(selectedTask.description)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>

                      <!-- 2-column metadata -->
                      <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Assignee</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedTask.assignee) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Priority</div>
                          <span class="px-1.5 py-0.5 rounded text-[10px] font-medium" :class="priorityClass(selectedTask.priority)">{{ selectedTask.priority }}</span>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Due date</div>
                          <div class="text-sm" :class="isOverdue(selectedTask) ? 'text-red-400' : 'text-slate-300'">{{ selectedTask.due_date ? formatDate(selectedTask.due_date) : '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Type</div>
                          <div class="text-sm text-slate-300 capitalize">{{ (selectedTask.task_type || 'general').replace(/_/g, ' ') }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created</div>
                          <div class="text-sm text-slate-300">{{ formatDate(selectedTask.created_at) }}</div>
                        </div>
                        <div v-if="selectedTask.created_by">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created by</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedTask.created_by) }}</div>
                        </div>
                        <div v-if="selectedTask.completed_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Completed</div>
                          <div class="text-sm text-emerald-400">{{ formatDate(selectedTask.completed_at) }}</div>
                        </div>
                      </div>

                      <!-- Status timeline -->
                      <div class="border-t border-slate-800 pt-4">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-3">Progress</div>
                        <div class="flex items-center gap-4 text-xs text-slate-500">
                          <div class="flex items-center gap-1.5">
                            <span class="w-2.5 h-2.5 rounded-full" :class="selectedTask.status !== 'cancelled' ? 'bg-blue-400' : 'bg-slate-600'"></span>
                            <span :class="selectedTask.status !== 'cancelled' ? 'text-slate-300' : ''">Open</span>
                          </div>
                          <div class="h-px flex-1 bg-slate-800"></div>
                          <div class="flex items-center gap-1.5">
                            <span class="w-2.5 h-2.5 rounded-full" :class="selectedTask.status === 'in_progress' || selectedTask.status === 'done' ? 'bg-amber-400' : 'bg-slate-600'"></span>
                            <span :class="selectedTask.status === 'in_progress' || selectedTask.status === 'done' ? 'text-slate-300' : ''">In progress</span>
                          </div>
                          <div class="h-px flex-1 bg-slate-800"></div>
                          <div class="flex items-center gap-1.5">
                            <span class="w-2.5 h-2.5 rounded-full" :class="selectedTask.status === 'done' ? 'bg-emerald-400' : 'bg-slate-600'"></span>
                            <span :class="selectedTask.status === 'done' ? 'text-slate-300' : ''">Done</span>
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
                    <MarkdownField v-model="editForm.notes" :self-type="'task'" :self-id="selectedTask?.identifier || ''" :rows="12" placeholder="Add notes..." />
                  </template>
                  <template v-else>
                    <div v-if="selectedTask.notes" class="text-sm doc-prose text-slate-300 leading-relaxed" v-html="renderMd(selectedTask.notes)"></div>
                    <div v-else class="text-sm text-slate-600 italic">No notes yet.</div>
                  </template>
                </div>
              </template>

              <!-- ═══ LINKS ═══ -->
              <template v-if="detailTab === 'links'">
                <div class="px-6 py-5">
                  <ReferenceManager entityType="task" :entityId="selectedTask.identifier" :editable="canCreate" />
                </div>
              </template>

              <!-- ═══ SUGGESTIONS ═══ -->
              <template v-if="detailTab === 'suggestions'">
                <div class="px-6 py-5">
                  <SuggestionPanel entityType="task" :entityId="selectedTask.identifier" :canReview="canCreate" @applied="loadTasks" />
                </div>
              </template>

              <!-- ═══ COMMENTS ═══ -->
              <template v-if="detailTab === 'comments'">
                <div class="px-6 py-5">
                  <CommentsPanel entityType="task" :entityId="selectedTask.identifier" />
                </div>
              </template>

              <!-- ═══ HISTORY ═══ -->
              <template v-if="detailTab === 'history'">
                <div class="px-6 py-5 space-y-6">
                  <HistoryPanel entityType="task" :entityId="String(selectedTask.id)" />
                  <div v-if="canCreate" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">Danger zone</div>
                    <div class="text-xs text-slate-400">Deleting this task is permanent and cannot be undone.</div>
                    <button @click="confirmDelete(selectedTask)" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                      Delete task
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

      <!-- Generation result -->
      <div v-if="genResult" class="mt-4 px-4 py-3 bg-emerald-950/30 border border-emerald-800/30 rounded-lg text-xs text-emerald-400">
        Generated {{ genResult.created?.length || 0 }} tasks ({{ genResult.skipped || 0 }} already existed).
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
import SuggestNewButton from '../components/SuggestNewButton.vue'
import HistoryPanel from '../components/HistoryPanel.vue'
import Pagination from '../components/Pagination.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import { useConfirm } from '../composables/useConfirm'
import { useModalEscape } from '../composables/useModalEscape.js'
import { useToast } from '../composables/useToast.js'
import { useDirtyEdit } from '../composables/useDirtyEdit.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import { renderMarkdown } from '../composables/useRenderMd.js'

const { ask, confirm: confirmDialog } = useConfirm()
const { success: showSaved, error: showError } = useToast()

const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()

const renderMd = renderMarkdown

const loading = ref(true)
const tasks = ref([])
// Default to "active" (open + in_progress) so completed/cancelled tasks don't
// clutter the working list; remember the user's choice across visits.
const TASKS_FILTER_KEY = 'isms.tasks.filterStatus'
const filterStatus = ref(localStorage.getItem(TASKS_FILTER_KEY) ?? 'active')
watch(filterStatus, (v) => {
  try { localStorage.setItem(TASKS_FILTER_KEY, v ?? '') } catch { /* storage unavailable */ }
})
const filterPriority = ref('')
const filterTaskType = ref('')
const filterAssignee = ref('')
const searchQuery = ref('')
const page = ref(1)
const pageSize = ref(50)
const total = ref(0)
const stats = ref({ total: 0, open: 0, in_progress: 0, done: 0, cancelled: 0, critical: 0, high: 0, medium: 0, low: 0 })
const selectedTask = ref(null)
const orgMembers = ref([])
const showCreate = ref(false)
const creating = ref(false)
const generating = ref(false)
const genResult = ref(null)
const userRole = ref('')

// Tab-based detail state
const detailTab = ref('overview')
const editingSection = ref('')
const editForm = ref({})
const { capture: captureEditSnapshot, isDirty } = useDirtyEdit(editForm)
const saving = ref(false)

// orgPath is provided by useCurrentOrg() above.

const detailTabs = [
  { key: 'overview', label: 'Overview' },
  { key: 'notes', label: 'Notes' },
  { key: 'links', label: 'Links' },
  { key: 'suggestions', label: 'Suggestions' },
  { key: 'comments', label: 'Comments' },
  { key: 'history', label: 'History' },
]

const canCreate = computed(() => ['admin', 'manager'].includes(userRole.value))
const canAdvance = computed(() => ['admin', 'manager', 'contributor'].includes(userRole.value))
const pendingRefs = ref([])

useModalEscape(showCreate)
useModalEscape(computed(() => !!selectedTask.value), () => closeDetail())

const currentUserEmail = ref('')
function defaultDueDate() {
  const d = new Date()
  d.setDate(d.getDate() + 7)
  return d.toISOString().slice(0, 10)
}
function defaultForm() {
  return { title: '', priority: 'medium', task_type: 'general' }
}
const form = ref(defaultForm())

const statusStats = computed(() => {
  return [
    { key: 'active', label: 'Active', count: (stats.value.open || 0) + (stats.value.in_progress || 0), color: 'text-indigo-400' },
    { key: '', label: 'Total', count: stats.value.total || 0, color: 'text-slate-100' },
    { key: 'open', label: 'Open', count: stats.value.open || 0, color: 'text-blue-400' },
    { key: 'in_progress', label: 'In Progress', count: stats.value.in_progress || 0, color: 'text-amber-400' },
    { key: 'overdue', label: 'Overdue', count: stats.value.overdue || 0, color: 'text-red-400' },
    { key: 'done', label: 'Done', count: stats.value.done || 0, color: 'text-emerald-400' },
  ]
})

function isOverdue(task) {
  if (!task.due_date || task.status === 'done' || task.status === 'cancelled') return false
  const d = typeof task.due_date === 'number' ? new Date(task.due_date * 1000) : new Date(task.due_date)
  return d < new Date()
}

function formatDate(d) {
  if (!d && d !== 0) return ''
  const dt = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  return dt.toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric' })
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

function priorityClass(p) {
  switch (p) {
    case 'critical': return 'bg-red-500/15 text-red-400'
    case 'high': return 'bg-amber-500/15 text-amber-400'
    case 'medium': return 'bg-blue-500/15 text-blue-400'
    default: return 'bg-slate-500/15 text-slate-400'
  }
}

function statusClass(s) {
  switch (s) {
    case 'open': return 'bg-blue-500/15 text-blue-400'
    case 'in_progress': return 'bg-amber-500/15 text-amber-400'
    case 'done': return 'bg-emerald-500/15 text-emerald-400'
    case 'cancelled': return 'bg-slate-500/15 text-slate-400'
    default: return 'bg-slate-500/15 text-slate-400'
  }
}

function statusDot(s) {
  switch (s) {
    case 'open': return 'bg-blue-400'
    case 'in_progress': return 'bg-amber-400'
    case 'done': return 'bg-emerald-400'
    default: return 'bg-slate-400'
  }
}

async function selectTask(task) {
  if (selectedTask.value && selectedTask.value.id === task.id) {
    closeDetail()
    return
  }
  router.push(orgPath(`/tasks/${task.id}`))
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
  router.push(orgPath('/tasks'))
}

async function loadTasks() {
  try {
    const params = new URLSearchParams()
    params.set('page', String(page.value))
    params.set('limit', String(pageSize.value))
    if (searchQuery.value) params.set('q', searchQuery.value)
    if (filterStatus.value) params.set('status', filterStatus.value)
    if (filterPriority.value) params.set('priority', filterPriority.value)
    if (filterTaskType.value) params.set('task_type', filterTaskType.value)
    if (filterAssignee.value) params.set('assignee', filterAssignee.value)
    const res = await api.fetchRaw(`/api/v1/tasks?${params.toString()}`)
    tasks.value = Array.isArray(res?.data) ? res.data : []
    total.value = res?.total || 0
    loadStats()
  } catch { tasks.value = [] }
}

async function loadStats() {
  try { stats.value = await api.fetchJSON('/api/v1/tasks/stats') || stats.value } catch {}
}

function resolveUserName(email) {
  if (!email) return '—'
  const u = orgMembers.value.find(m => m.email === email)
  return u?.name || email
}

let searchTimer = null
let assigneeTimer = null
watch([searchQuery], () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { page.value = 1; loadTasks() }, 250)
})
watch([filterAssignee], () => {
  clearTimeout(assigneeTimer)
  assigneeTimer = setTimeout(() => { page.value = 1; loadTasks() }, 250)
})
watch([filterStatus, filterPriority, filterTaskType], () => {
  page.value = 1
  loadTasks()
})
watch([page, pageSize], () => loadTasks())

async function create() {
  if (creating.value) return
  creating.value = true
  try {
    const payload = {
      title: form.value.title,
      priority: form.value.priority,
      task_type: form.value.task_type,
    }
    // Source links come from quick-action query params; the same info also seeds
    // a one-line markdown reference into Notes so the source is visible inline
    // (Links tab still owns the formal cross-entity reference).
    const sourceLinks = []
    const seedLines = []
    const sourceTitle = route.query.title ? String(route.query.title) : ''
    if (route.query.from_ca) {
      const id = String(route.query.from_ca)
      sourceLinks.push({ type: 'corrective_action', id })
      const label = sourceTitle ? `${id}: ${sourceTitle}` : id
      seedLines.push(`Created from [${label}](/corrective-actions/${id})`)
    }
    if (route.query.from_risk) {
      const id = String(route.query.from_risk)
      sourceLinks.push({ type: 'risk', id })
      const label = sourceTitle ? `${id}: ${sourceTitle}` : id
      seedLines.push(`Created from [${label}](/risks/${id})`)
    }
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
    if (route.query.from_document) {
      const id = String(route.query.from_document)
      sourceLinks.push({ type: 'document', id })
      const label = sourceTitle ? `${id}: ${sourceTitle}` : id
      seedLines.push(`Created from [${label}](/documents/${id})`)
    }
    if (seedLines.length > 0) {
      payload.notes = seedLines.join('\n')
    }
    const created = await api.createTask(payload)
    const entityId = created?.identifier || ''
    if (entityId) {
      for (const ref of [...sourceLinks, ...pendingRefs.value]) {
        try {
          await api.createReference({ source_type: 'task', source_id: entityId, target_type: ref.type, target_id: ref.id })
        } catch { /* non-fatal */ }
      }
    }
    pendingRefs.value = []
    form.value = defaultForm()
    showCreate.value = false
    await loadTasks()
    // Drop user into detail modal in edit mode on Overview to keep filling things in.
    if (created && created.id) {
      let fresh = created
      try { fresh = await api.getTask(created.id) } catch { /* fall back */ }
      selectedTask.value = fresh
      detailTab.value = 'overview'
      startEdit(fresh)
      editingSection.value = 'overview'
      router.push(orgPath(`/tasks/${fresh.id}`))
    }
  } catch (e) {
    console.error('Failed to create task:', e)
    showError('Failed to create task: ' + (e.message || 'unknown error'))
  } finally {
    creating.value = false
  }
}

async function advanceStatus(task, status) {
  if (!status) return
  try {
    await api.updateTaskStatus(task.id, status)
    task.status = status
    if (status === 'done') task.completed_at = Date.now() / 1000
    if (selectedTask.value?.id === task.id) {
      selectedTask.value = { ...task }
    }
  } catch (e) {
    showError('Failed to update status: ' + (e.message || 'unknown error'))
  }
}

function startEdit(task) {
  editForm.value = {
    title: task.title || '',
    description: task.description || '',
    assignee: task.assignee || '',
    priority: task.priority || 'medium',
    status: task.status || 'open',
    task_type: task.task_type || 'general',
    notes: task.notes || '',
    due_date_str: epochToDateStr(task.due_date),
  }
  captureEditSnapshot()
}

function editSection(section) {
  startEdit(selectedTask.value)
  editingSection.value = section
}

function cancelSection() {
  editingSection.value = ''
  startEdit(selectedTask.value)
}

async function saveSection() {
  if (!selectedTask.value) return
  saving.value = true
  try {
    const payload = {
      title: editForm.value.title,
      description: editForm.value.description,
      assignee: editForm.value.assignee,
      priority: editForm.value.priority,
      status: editForm.value.status,
      task_type: editForm.value.task_type,
      notes: editForm.value.notes,
    }
    if (editForm.value.due_date_str) {
      payload.due_date = dateStrToEpoch(editForm.value.due_date_str)
    } else {
      payload.due_date = null
    }
    await api.updateTask(selectedTask.value.id, payload)
    await loadTasks()
    const fresh = tasks.value.find(t => t.id === selectedTask.value.id)
    if (fresh) {
      selectedTask.value = fresh
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

async function confirmDelete(task) {
  if (!await ask(`Delete task "${task.title}"?`, { confirm: 'Delete', variant: 'danger' })) return
  try {
    await api.deleteTask(task.id)
    closeDetail()
    await loadTasks()
  } catch (e) {
    showError('Failed to delete task: ' + (e.message || 'unknown error'))
  }
}

async function generateOverdueTasks() {
  generating.value = true
  genResult.value = null
  try {
    genResult.value = await api.postJSON('/api/v1/overdue/tasks', {})
    await loadTasks()
  } catch { /* ignore */ }
  generating.value = false
}

async function openTaskFromRoute(id) {
  const numId = parseInt(id)
  let task = tasks.value.find(t => t.id === numId)
  if (!task) {
    try { task = await api.fetchJSON(`/api/v1/tasks/${numId}`) } catch { return }
  }
  if (!task) return
  selectedTask.value = task
  detailTab.value = 'overview'
  editingSection.value = ''
  startEdit(task)
}

// Deep link support
watch(() => route.params.id, (id) => {
  if (!id) {
    selectedTask.value = null
    detailTab.value = 'overview'
    editingSection.value = ''
    return
  }
  if (selectedTask.value?.id === parseInt(id)) return
  openTaskFromRoute(id)
}, { immediate: false })

onMounted(async () => {
  try {
    const me = await api.getMe()
    userRole.value = me?.role || ''
    currentUserEmail.value = me?.email || ''
  } catch {}
  try { orgMembers.value = await api.getUsers() || [] } catch { orgMembers.value = [] }
  await loadTasks()
  if (route.params.id) await openTaskFromRoute(route.params.id)

  // Handle "create linked from X" query params (from Risks/CorrectiveActions etc.)
  if (route.query.from_risk || route.query.from_ca) {
    form.value = {
      ...defaultForm(),
      title: route.query.title ? String(route.query.title) : '',
      task_type: route.query.task_type ? String(route.query.task_type) : 'general',
      priority: route.query.priority ? String(route.query.priority) : 'medium',
    }
    showCreate.value = true
  }

  loading.value = false
})
</script>
