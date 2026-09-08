<template>
  <div class="min-h-full">
    <!-- Loading -->
    <div v-if="loading" class="max-w-6xl mx-auto px-8 py-10">
      <ListSkeleton :rows="5" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="max-w-6xl mx-auto px-8 py-12">
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
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">{{ t('corrective_actions.title') }}</h1>
          <p class="text-sm text-slate-500 mt-1">{{ t('corrective_actions.subtitle') }}</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canWrite"
            @click="showCreateForm = !showCreateForm"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            {{ t('corrective_actions.action.add') }}
          </button>
          <SuggestNewButton entityType="corrective_action" :typeLabel="entityLabel('corrective_action')" />
        </div>
      </div>

      <!-- Stats strip -->
      <StatStrip :stats="statusStats" v-model="filterStatus" />

      <!-- Actions bar -->
      <div class="flex items-center gap-3 flex-wrap">
        <RefreshButton :loading="refreshing" @refresh="reload" class="shrink-0" />
        <div class="relative flex-1 max-w-xs">
          <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
          </svg>
          <input v-model="searchQuery" type="text" :placeholder="t('common.placeholder.search')"
            class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
        </div>
        <select v-model="filterStatus" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
          <option value="">{{ t('common.filter.all_statuses') }}</option>
          <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
        <select v-model="filterSeverity" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
          <option value="">{{ t('common.filter.all_severities') }}</option>
          <option v-for="o in severityAbbrOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
        <select v-model="filterSource" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
          <option value="">{{ t('common.filter.all_sources') }}</option>
          <option v-for="o in sourceOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
        <div class="w-48">
          <MemberPicker v-model="filterAssignee" :members="orgMembers" :placeholder="t('corrective_actions.filter.assignee_placeholder')" />
        </div>
        <div class="ml-auto text-xs text-slate-500 tabular-nums">{{ t('common.count.total', { count: total }) }}</div>
      </div>

      <!-- Create form -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreateForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreateForm = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
        <div class="flex items-center justify-between mb-2">
          <h2 class="text-sm font-semibold text-slate-200">{{ t('corrective_actions.create.heading') }}</h2>
          <button @click="showCreateForm = false" class="text-slate-500 hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div class="sm:col-span-2">
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('corrective_actions.create.title_label') }}</label>
            <input v-model="newCA.title" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('corrective_actions.create.title_placeholder')" />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('corrective_actions.field.source') }}</label>
            <select v-model="newCA.source" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option v-for="o in sourceFormOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('corrective_actions.field.severity') }}</label>
            <select v-model="newCA.severity" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option v-for="o in severityOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">{{ t('corrective_actions.create.fill_in_later') }}</div>
        <div class="flex justify-end gap-3 pt-2">
          <button @click="showCreateForm = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">{{ t('common.action.cancel') }}</button>
          <button @click="createCA" :disabled="!newCA.title" class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
            {{ t('corrective_actions.create.submit') }}
          </button>
        </div>
        </div>
      </div>
      </Transition>
      </Teleport>

      <!-- Action list -->
      <div v-if="actions.length === 0" class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
        <div v-if="filterStatus || filterSeverity || filterSource || filterAssignee || searchQuery" class="text-slate-500 text-sm">
          {{ t('corrective_actions.filter.empty') }}
        </div>
        <div v-else class="text-slate-500 text-sm">
          {{ t('corrective_actions.filter.empty_all') }}
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
              {{ formatDay(ca.due_date) }}
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
              <CopyLinkButton />
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
                <button v-for="tab in detailTabs" :key="tab.key" @click="switchDetailTab(tab.key)"
                  class="w-full text-left px-3 py-2 text-xs font-medium transition-colors"
                  :class="detailTab === tab.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                  {{ tab.label }}
                </button>
              </div>
            </nav>

            <!-- Content pane -->
            <div class="flex-1 overflow-y-auto min-h-0">

              <!-- ═══ OVERVIEW ═══ -->
              <template v-if="detailTab === 'overview'">
                <div class="px-6 py-5 space-y-5">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('corrective_actions.detail.overview') }}</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('overview')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                  </div>
                  <template v-if="editingSection === 'overview'">
                    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('corrective_actions.field.title') }}</label>
                        <input v-model="editForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('corrective_actions.field.description') }}</label>
                        <MarkdownField v-model="editForm.description" :self-type="'corrective_action'" :self-id="selectedCA?.identifier || ''" :rows="3" :placeholder="t('corrective_actions.placeholder.description')" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('corrective_actions.field.source') }}</label>
                        <select v-model="editForm.source" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in sourceFormOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('corrective_actions.field.severity') }}</label>
                        <select v-model="editForm.severity" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in severityOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('corrective_actions.field.status') }}</label>
                        <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('corrective_actions.field.assignee') }}</label>
                        <MemberPicker v-model="editForm.assignee" :members="orgMembers" :placeholder="t('corrective_actions.placeholder.assignee')" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('corrective_actions.field.due_date') }}</label>
                        <input v-model="editForm.due_date" type="date" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('corrective_actions.field.root_cause') }}</label>
                        <MarkdownField v-model="editForm.root_cause" :self-type="'corrective_action'" :self-id="selectedCA?.identifier || ''" :rows="3" :placeholder="t('corrective_actions.placeholder.root_cause')" />
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('corrective_actions.field.description') }}</div>
                        <div v-if="selectedCA.description" class="text-sm text-slate-300 leading-relaxed doc-prose" v-mermaid v-html="renderMd(selectedCA.description)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>
                      <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('corrective_actions.field.source') }}</div>
                          <div class="text-sm text-slate-300">{{ sourceLabel(selectedCA.source) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('corrective_actions.field.severity') }}</div>
                          <span class="inline-flex items-center px-2 py-0.5 text-[10px] font-medium rounded" :class="severityClass(selectedCA.severity)">{{ severityLabel(selectedCA.severity) }}</span>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('corrective_actions.field.assignee') }}</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedCA.assignee) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('corrective_actions.field.due_date') }}</div>
                          <div class="text-sm" :class="selectedCA.due_date && isOverdue(selectedCA.due_date) && selectedCA.status !== 'resolved' ? 'text-red-400' : 'text-slate-300'">{{ selectedCA.due_date ? formatDay(selectedCA.due_date) : '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('corrective_actions.field.created_by') }}</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedCA.created_by) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('corrective_actions.field.created') }}</div>
                          <div class="text-sm text-slate-300">{{ formatDate(selectedCA.created_at) }}</div>
                        </div>
                        <div v-if="selectedCA.resolved_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('corrective_actions.field.resolved') }}</div>
                          <div class="text-sm text-emerald-400">
                            <i18n-t keypath="corrective_actions.detail.resolved_by" scope="global">
                              <template #datetime>{{ formatDateTime(selectedCA.resolved_at) }}</template>
                              <template #by>
                                <span v-if="selectedCA.resolved_by" class="text-slate-500">{{ t('corrective_actions.detail.by', { name: resolveUserName(selectedCA.resolved_by) }) }}</span>
                              </template>
                            </i18n-t>
                          </div>
                        </div>
                      </div>

                      <div v-if="selectedCA.root_cause" class="border-t border-slate-800 pt-4">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('corrective_actions.field.root_cause') }}</div>
                        <div class="text-sm text-slate-300 doc-prose" v-mermaid v-html="renderMd(selectedCA.root_cause)"></div>
                      </div>

                    </div>
                  </template>
                </div>
              </template>

              <!-- ═══ ACTIONS ═══ -->
              <template v-if="detailTab === 'actions'">
                <div class="px-6 py-5 space-y-4">
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('common.heading.quick_actions') }}</div>
                  <div v-if="!canWrite" class="text-xs text-slate-600 italic">{{ t('common.read_only.actions') }}</div>
                  <div v-else class="flex flex-col gap-3 max-w-md">
                    <button @click="createLinkedTask"
                      class="flex items-center justify-between gap-3 px-4 py-3 rounded-lg bg-slate-900 hover:bg-slate-800 border border-slate-700 hover:border-slate-600 transition-colors text-left">
                      <div>
                        <div class="text-sm font-medium text-slate-200">{{ t('corrective_actions.actions.create_task') }}</div>
                        <div class="text-xs text-slate-500 mt-0.5">{{ t('corrective_actions.actions.create_task_hint') }}</div>
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
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('corrective_actions.detail.notes') }}</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('notes')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                  </div>
                  <template v-if="editingSection === 'notes'">
                    <MarkdownField v-model="editForm.notes" :self-type="'corrective_action'" :self-id="selectedCA?.identifier || ''" :rows="12" :placeholder="t('corrective_actions.placeholder.notes')" />
                  </template>
                  <template v-else>
                    <div v-if="selectedCA.notes" class="text-sm doc-prose text-slate-300 leading-relaxed" v-mermaid v-html="renderMd(selectedCA.notes)"></div>
                    <div v-else class="text-sm text-slate-600 italic">{{ t('common.state.no_notes') }}</div>
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
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">{{ t('common.heading.danger_zone') }}</div>
                    <div class="text-xs text-slate-400">{{ t('corrective_actions.danger.warning') }}</div>
                    <button @click="deleteCA(selectedCA.id)" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                      {{ t('corrective_actions.danger.delete') }}
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
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import api from '../api'
import MemberPicker from '../components/MemberPicker.vue'
import MarkdownField from '../components/MarkdownField.vue'
import StatusBadge from '../components/StatusBadge.vue'
import StatStrip from '../components/StatStrip.vue'
import RefreshButton from '../components/RefreshButton.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
import CopyLinkButton from '../components/CopyLinkButton.vue'
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
import { formatDate, formatDay } from '../composables/useFormat.js'
import { renderApiError } from '../composables/useApiError.js'
import { enumLabel, enumLabelAbbr, entityLabel } from '../composables/useEnumLabel.js'

const { confirm: confirmDialog } = useConfirm()

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { orgPath } = useCurrentOrg()
const { success: showSaved, show: showError } = useToast()

const renderMd = renderMarkdown

const userRole = ref('')
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')

const orgMembers = ref([])

const loading = ref(true)
const refreshing = ref(false)
async function reload() {
  refreshing.value = true
  error.value = null
  try {
    await fetchAll()
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    refreshing.value = false
  }
}
const error = ref(null)
const actions = ref([])
const stats = ref({})
const STATUS_STATS = [
  { key: 'todo', color: 'text-red-400' },
  { key: 'assessment', color: 'text-amber-400' },
  { key: 'awaiting_approval', color: 'text-purple-400' },
  { key: 'implementation', color: 'text-blue-400' },
  { key: 'monitoring', color: 'text-cyan-400' },
  { key: 'resolved', color: 'text-emerald-400' },
]
const statusStats = computed(() => STATUS_STATS.map((s) => ({
  ...s, label: statusLabel(s.key), count: stats.value[s.key] || 0,
})))
const selectedCA = ref(null)
const showCreateForm = ref(false)

// Tab-based detail state
const detailTab = ref('overview')
const editingSection = ref('')
const editForm = ref({})
const { capture: captureEditSnapshot, isDirty } = useDirtyEdit(editForm)
const saving = ref(false)

// Tab labels are keys, resolved in a computed: an array built at module load
// freezes its labels in whatever locale was active when the file was imported.
const TAB_KEYS = [
  { key: 'overview', label: 'common.tab.overview' },
  { key: 'notes', label: 'common.tab.notes' },
  { key: 'links', label: 'common.tab.links' },
  { key: 'actions', label: 'common.tab.actions' },
  { key: 'suggestions', label: 'common.tab.suggestions' },
  { key: 'comments', label: 'common.tab.comments' },
  { key: 'history', label: 'common.tab.history' },
]
const detailTabs = computed(() => TAB_KEYS.map((tab) => ({ ...tab, label: t(tab.label) })))

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
  if (selectedCA.value && (selectedCA.value.identifier === id || String(selectedCA.value.id) === String(id))) return
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
    await fetchAll()
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    loading.value = false
  }
}

// Fetch body without the loading toggle, so reload() refreshes in place
// (no full-list skeleton flicker) — see #165 review.
async function fetchAll() {
  await Promise.all([loadActions(), loadStats()])
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
    error.value = renderApiError(e)
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
  let ca = actions.value.find(a => a.identifier === id || String(a.id) === String(id))
  if (!ca) {
    try { ca = await api.fetchJSON(`/api/v1/corrective-actions/${encodeURIComponent(id)}`) } catch { return }
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
      seedLines.push(t('corrective_actions.seed.from_incident', { label, link: `/incidents/${id}` }))
    }
    if (route.query.from_audit_finding) {
      const id = 'FIND-' + String(route.query.from_audit_finding)
      sourceLinks.push({ type: 'audit_finding', id })
      const label = sourceTitle ? `${id}: ${sourceTitle}` : id
      seedLines.push(t('corrective_actions.seed.from_audit_finding', { label }))
    }
    if (route.query.from_risk) {
      const id = String(route.query.from_risk)
      sourceLinks.push({ type: 'risk', id })
      const label = sourceTitle ? `${id}: ${sourceTitle}` : id
      seedLines.push(t('corrective_actions.seed.from_risk', { label, link: `/risks/${id}` }))
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
    error.value = renderApiError(e)
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
  const ok = await confirmDialog({
    message: t('corrective_actions.danger.confirm'),
    variant: 'danger',
    confirmLabel: t('common.action.delete'),
  })
  if (!ok) return
  try {
    await api.deleteCorrectiveAction(id)
    closeDetail()
    await loadActions()
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function selectCA(ca) {
  if (selectedCA.value && selectedCA.value.id === ca.id) {
    closeDetail()
    return
  }
  router.push(orgPath(`/corrective-actions/${ca.identifier}`))
}

async function switchDetailTab(key) {
  if (editingSection.value && isDirty()) {
    const ok = await confirmDialog({
      message: t('common.dirty.switch_tab'),
      variant: 'danger',
      confirmLabel: t('common.dirty.discard'),
    })
    if (!ok) return
  }
  detailTab.value = key
  editingSection.value = ''
}

async function closeDetail() {
  if (editingSection.value && isDirty()) {
    const ok = await confirmDialog({
      message: t('common.dirty.close'),
      variant: 'danger',
      confirmLabel: t('common.dirty.discard'),
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
    showSaved(t('common.state.saved'))
  } catch (e) {
    showError(t('corrective_actions.error.save', { message: renderApiError(e) }))
  } finally {
    saving.value = false
  }
}

function isOverdue(dateStr) {
  if (!dateStr && dateStr !== 0) return false
  const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
  return d < new Date()
}

// Lookups and option lists live here rather than in the template: a group name
// is a stored identifier, and the raw-text scanner reads a bare quoted word in
// a mustache as unextracted copy.
//
// The finding taxonomy appears twice over, and both forms are authored: the
// badge and the filter want "Major NC", the create and edit forms want "Major
// non-conformity". `source` likewise — the filter reads "Internal audit", the
// forms spell out "Internal Audit Finding", so the long set stays a local
// `corrective_actions.source_option.*` rather than displacing the catalogue.
const STATUSES = ['todo', 'assessment', 'awaiting_approval', 'implementation', 'monitoring', 'resolved']
const SEVERITIES = ['major_nc', 'minor_nc', 'observation', 'opportunity']
const SOURCES = ['internal_audit', 'external_audit', 'risk_assessment', 'security_incident', 'objective', 'feedback', 'other']

const statusLabel = (v) => enumLabel('status', v)
const severityLabel = (v) => enumLabelAbbr('finding_type', v)
const sourceLabel = (v) => enumLabel('source', v)

const options = (values, label) => computed(() => values.map((value) => ({ value, label: label(value) })))
const statusOptions = options(STATUSES, statusLabel)
const severityAbbrOptions = options(SEVERITIES, severityLabel)
const severityOptions = options(SEVERITIES, (v) => enumLabel('finding_type', v))
const sourceOptions = options(SOURCES, sourceLabel)
const sourceFormOptions = options(SOURCES, (v) => t(`corrective_actions.source_option.${v}`))

function severityClass(sev) {
  switch (sev) {
    case 'major_nc': return 'bg-red-900/60 text-red-300 border border-red-800'
    case 'minor_nc': return 'bg-amber-900/60 text-amber-300 border border-amber-800'
    case 'observation': return 'bg-blue-900/60 text-blue-300 border border-blue-800'
    case 'opportunity': return 'bg-emerald-900/60 text-emerald-300 border border-emerald-800'
    default: return 'bg-slate-800 text-slate-400 border border-slate-700'
  }
}

const formatDateTime = (d) => formatDate(d, 'datetime')
</script>
