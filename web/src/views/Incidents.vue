<template>
  <div class="min-h-full">
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
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">{{ t('incidents.title') }}</h1>
          <p class="text-sm text-slate-500 mt-1">{{ t('incidents.subtitle') }}</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canReport"
            @click="showCreateForm = !showCreateForm"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            {{ t('incidents.action.add') }}
          </button>
          <SuggestNewButton entityType="incident" :typeLabel="entityLabel('incident')" />
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
          <option v-for="o in severityOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
        <div class="ml-auto text-xs text-slate-500 tabular-nums">{{ t('common.count.total', { count: total }) }}</div>
      </div>

      <!-- Create form -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreateForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreateForm = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
        <div class="flex items-center justify-between mb-2">
          <h2 class="text-sm font-semibold text-slate-200">{{ t('incidents.create.heading') }}</h2>
          <button @click="showCreateForm = false" class="text-slate-500 hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div class="sm:col-span-2">
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.create.title_label') }}</label>
            <input v-model="newIncident.title" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('incidents.create.title_placeholder')" />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.create.severity_label') }}</label>
            <select v-model="newIncident.severity" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option v-for="o in severityOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.create.type_label') }}</label>
            <select v-model="newIncident.incident_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option v-for="o in typeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">{{ t('incidents.create.fill_in_later') }}</div>
        <div class="flex justify-end gap-3 pt-2">
          <button @click="showCreateForm = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">{{ t('common.action.cancel') }}</button>
          <button @click="createIncident" :disabled="!newIncident.title" class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
            {{ t('incidents.create.submit') }}
          </button>
        </div>
        </div>
      </div>
      </Transition>
      </Teleport>

      <!-- Incident list -->
      <div v-if="incidents.length === 0" class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
        <div class="text-slate-500 text-sm">{{ t('incidents.filter.empty') }}</div>
      </div>

      <div v-else class="space-y-2">
        <div v-for="inc in incidents" :key="inc.id"
          @click="selectIncident(inc)"
          class="bg-slate-900 border border-slate-800 rounded-lg p-4 hover:border-slate-700 transition-colors cursor-pointer">
          <div class="flex items-center gap-3">
            <span class="inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-full uppercase tracking-wider"
              :class="severityClass(inc.severity)">
              {{ severityLabel(inc.severity) }}
            </span>
            <span class="inline-flex items-center px-2 py-0.5 text-[11px] font-medium rounded-full"
              :class="typeClass(inc.incident_type)">
              {{ typeLabel(inc.incident_type) }}
            </span>
            <StatusBadge :status="inc.status" />
            <span class="text-sm font-medium text-slate-200 flex-1 truncate">{{ inc.title }}</span>
            <span class="text-xs text-slate-600 font-mono">{{ inc.identifier }}</span>
            <span class="text-xs text-slate-600">{{ formatDate(inc.created_at) }}</span>
          </div>
          <div class="mt-1.5 flex items-center gap-3 text-xs text-slate-500">
            <span v-if="classificationLabel(inc)">{{ classificationLabel(inc) }}</span>
            <span>{{ t('incidents.list.source', { value: originLabel(inc.source) }) }}</span>
            <span>{{ t('incidents.list.reporter', { name: resolveUserName(inc.reporter) }) }}</span>
            <span v-if="inc.assignee">{{ t('incidents.list.assignee', { name: resolveUserName(inc.assignee) }) }}</span>
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
              <CopyLinkButton />
              <span class="inline-flex items-center px-2 py-0.5 text-[10px] font-semibold rounded-full uppercase tracking-wider"
                :class="severityClass(selectedIncident.severity)">
                {{ severityLabel(selectedIncident.severity) }}
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
                <button v-for="tab in detailTabs" :key="tab.key" @click="switchDetailTab(tab.key)"
                  class="w-full text-left px-3 py-2 text-xs font-medium transition-colors"
                  :class="detailTab === tab.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                  {{ tab.label }}
                </button>
              </div>
            </nav>

            <div class="flex-1 overflow-y-auto min-h-0">

              <!-- ═══ OVERVIEW ═══ -->
              <template v-if="detailTab === 'overview'">
                <div class="px-6 py-5 space-y-5">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('incidents.detail.overview') }}</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('overview')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                  </div>
                  <template v-if="editingSection === 'overview'">
                    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.field.title') }}</label>
                        <input v-model="editForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.field.description') }}</label>
                        <MarkdownField v-model="editForm.description" :self-type="'incident'" :self-id="selectedIncident ? String(selectedIncident.id) : ''" :rows="3" :placeholder="t('incidents.placeholder.description')" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.field.severity') }}</label>
                        <select v-model="editForm.severity" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in severityOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.field.type') }}</label>
                        <select v-model="editForm.incident_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in typeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.field.origin') }}</label>
                        <select v-model="editForm.source" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in originOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.field.classification') }}</label>
                        <div class="flex flex-wrap gap-4 mt-1">
                          <label class="flex items-center gap-2 cursor-pointer">
                            <input type="checkbox" v-model="editForm.affects_c" class="rounded bg-slate-800 border-slate-600 text-blue-500 focus:ring-blue-500 focus:ring-offset-0" />
                            <span class="text-xs text-slate-300">{{ t('incidents.classification.confidentiality') }}</span>
                          </label>
                          <label class="flex items-center gap-2 cursor-pointer">
                            <input type="checkbox" v-model="editForm.affects_i" class="rounded bg-slate-800 border-slate-600 text-blue-500 focus:ring-blue-500 focus:ring-offset-0" />
                            <span class="text-xs text-slate-300">{{ t('incidents.classification.integrity') }}</span>
                          </label>
                          <label class="flex items-center gap-2 cursor-pointer">
                            <input type="checkbox" v-model="editForm.affects_a" class="rounded bg-slate-800 border-slate-600 text-blue-500 focus:ring-blue-500 focus:ring-offset-0" />
                            <span class="text-xs text-slate-300">{{ t('incidents.classification.availability') }}</span>
                          </label>
                        </div>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.field.status') }}</label>
                        <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.field.assignee') }}</label>
                        <MemberPicker v-model="editForm.assignee" :members="orgMembers" :placeholder="t('incidents.placeholder.assignee')" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.field.root_cause') }}</label>
                        <MarkdownField v-model="editForm.root_cause" :self-type="'incident'" :self-id="selectedIncident ? String(selectedIncident.id) : ''" :rows="3" :placeholder="t('incidents.placeholder.root_cause')" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.field.lessons_learned') }}</label>
                        <MarkdownField v-model="editForm.lessons_learned" :self-type="'incident'" :self-id="selectedIncident ? String(selectedIncident.id) : ''" :rows="3" :placeholder="t('incidents.placeholder.lessons_learned')" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="flex items-center gap-2 cursor-pointer">
                          <input type="checkbox" v-model="editForm.data_breach" class="rounded bg-slate-800 border-slate-600 text-blue-500 focus:ring-blue-500 focus:ring-offset-0" />
                          <span class="text-xs font-medium text-slate-400">{{ t('incidents.data_breach_checkbox') }}</span>
                        </label>
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('incidents.field.description') }}</div>
                        <div v-if="selectedIncident.description" class="text-sm text-slate-300 leading-relaxed doc-prose" v-mermaid v-html="renderMd(selectedIncident.description)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>

                      <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('incidents.field.severity') }}</div>
                          <span class="inline-flex items-center px-2 py-0.5 text-[10px] font-semibold rounded-full uppercase tracking-wider"
                            :class="severityClass(selectedIncident.severity)">{{ severityLabel(selectedIncident.severity) }}</span>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('incidents.field.type') }}</div>
                          <div class="text-sm text-slate-300">{{ typeLabel(selectedIncident.incident_type) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('incidents.field.origin') }}</div>
                          <div class="text-sm text-slate-300">{{ originLabel(selectedIncident.source) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('incidents.field.classification') }}</div>
                          <div class="flex items-center gap-1.5">
                            <span v-if="selectedIncident.affects_c" class="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-blue-900/40 text-blue-300 border border-blue-800">{{ t('common.cia_abbr.c') }}</span>
                            <span v-if="selectedIncident.affects_i" class="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-purple-900/40 text-purple-300 border border-purple-800">{{ t('common.cia_abbr.i') }}</span>
                            <span v-if="selectedIncident.affects_a" class="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-emerald-900/40 text-emerald-300 border border-emerald-800">{{ t('common.cia_abbr.a') }}</span>
                            <span v-if="!selectedIncident.affects_c && !selectedIncident.affects_i && !selectedIncident.affects_a" class="text-sm text-slate-600">—</span>
                          </div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('incidents.field.reporter') }}</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedIncident.reporter) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('incidents.field.assignee') }}</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedIncident.assignee) }}</div>
                        </div>
                        <div v-if="selectedIncident.created_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('incidents.field.created') }}</div>
                          <div class="text-sm text-slate-300">{{ formatDate(selectedIncident.created_at) }}</div>
                        </div>
                        <div v-if="selectedIncident.created_by">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('incidents.field.created_by') }}</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedIncident.created_by) }}</div>
                        </div>
                      </div>

                      <!-- Investigation/resolution fields -->
                      <div class="border-t border-slate-800 pt-4 space-y-3">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('incidents.field.root_cause') }}</div>
                          <div v-if="selectedIncident.root_cause" class="text-sm text-slate-300 doc-prose" v-mermaid v-html="renderMd(selectedIncident.root_cause)"></div>
                          <div v-else class="text-sm text-slate-600">—</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('incidents.field.lessons_learned') }}</div>
                          <div v-if="selectedIncident.lessons_learned" class="text-sm text-slate-300 doc-prose" v-mermaid v-html="renderMd(selectedIncident.lessons_learned)"></div>
                          <div v-else class="text-sm text-slate-600">—</div>
                        </div>
                      </div>

                      <!-- Timeline -->
                      <div class="border-t border-slate-800 pt-4">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-3">{{ t('incidents.timeline.heading') }}</div>
                        <div class="grid grid-cols-2 gap-3 text-xs">
                          <div>
                            <span class="text-slate-500">{{ t('incidents.timeline.detected') }}</span>
                            <span class="text-slate-300 ml-1">{{ formatDateTime(selectedIncident.detected_at) }}</span>
                          </div>
                          <div v-if="selectedIncident.contained_at">
                            <span class="text-slate-500">{{ t('incidents.timeline.contained') }}</span>
                            <span class="text-slate-300 ml-1">{{ formatDateTime(selectedIncident.contained_at) }}</span>
                          </div>
                          <div v-if="selectedIncident.resolved_at">
                            <span class="text-slate-500">{{ t('incidents.timeline.resolved') }}</span>
                            <span class="text-slate-300 ml-1">{{ formatDateTime(selectedIncident.resolved_at) }}</span>
                          </div>
                          <div v-if="selectedIncident.closed_at">
                            <span class="text-slate-500">{{ t('incidents.timeline.closed') }}</span>
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
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('common.heading.quick_actions') }}</div>
                  <div v-if="!canWrite" class="text-xs text-slate-600 italic">{{ t('common.read_only.actions') }}</div>
                  <div v-else class="flex flex-col gap-3 max-w-md">
                    <button @click="createLinkedCA"
                      class="flex items-center justify-between gap-3 px-4 py-3 rounded-lg bg-slate-900 hover:bg-slate-800 border border-slate-700 hover:border-slate-600 transition-colors text-left">
                      <div>
                        <div class="text-sm font-medium text-slate-200">{{ t('incidents.actions.create_ca') }}</div>
                        <div class="text-xs text-slate-500 mt-0.5">{{ t('incidents.actions.create_ca_desc') }}</div>
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
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('incidents.breach.heading') }}</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('data_breach')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                  </div>
                  <template v-if="editingSection === 'data_breach'">
                    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.breach.gdpr_role') }}</label>
                        <select v-model="editForm.gdpr_role" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in gdprRoleOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.breach.authority_notification') }}</label>
                        <select v-model="editForm.authority_notified" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in notificationOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('incidents.breach.subjects_notification') }}</label>
                        <select v-model="editForm.subjects_notified" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in notificationOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="bg-red-950/30 border border-red-900/40 rounded-lg p-4 space-y-3">
                      <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 text-xs">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('incidents.breach.gdpr_role') }}</div>
                          <div class="text-slate-300">{{ gdprRoleLabel(selectedIncident.gdpr_role) || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('incidents.breach.authority') }}</div>
                          <span class="px-1.5 py-0.5 rounded text-[10px] font-medium"
                            :class="selectedIncident.authority_notified === 'notified' ? 'bg-emerald-900/40 text-emerald-400' : selectedIncident.authority_notified === 'pending' ? 'bg-amber-900/40 text-amber-400' : 'bg-slate-800 text-slate-500'">
                            {{ notificationLabel(selectedIncident.authority_notified) }}
                          </span>
                          <div v-if="selectedIncident.authority_notified_at" class="text-slate-600 mt-1">{{ formatDateTime(selectedIncident.authority_notified_at) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('incidents.breach.subjects') }}</div>
                          <span class="px-1.5 py-0.5 rounded text-[10px] font-medium"
                            :class="selectedIncident.subjects_notified === 'notified' ? 'bg-emerald-900/40 text-emerald-400' : selectedIncident.subjects_notified === 'pending' ? 'bg-amber-900/40 text-amber-400' : 'bg-slate-800 text-slate-500'">
                            {{ notificationLabel(selectedIncident.subjects_notified) }}
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
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('incidents.detail.notes') }}</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('notes')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                  </div>
                  <template v-if="editingSection === 'notes'">
                    <MarkdownField v-model="editForm.notes" :self-type="'incident'" :self-id="selectedIncident ? String(selectedIncident.id) : ''" :rows="12" :placeholder="t('incidents.placeholder.notes')" />
                  </template>
                  <template v-else>
                    <div v-if="selectedIncident.notes" class="text-sm doc-prose text-slate-300 leading-relaxed" v-mermaid v-html="renderMd(selectedIncident.notes)"></div>
                    <div v-else class="text-sm text-slate-600 italic">{{ t('common.state.no_notes') }}</div>
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
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">{{ t('common.heading.danger_zone') }}</div>
                    <div class="text-xs text-slate-400">{{ t('incidents.danger.warning') }}</div>
                    <button @click="deleteSelectedIncident" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                      {{ t('incidents.danger.delete') }}
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
import StatusBadge from '../components/StatusBadge.vue'
import StatStrip from '../components/StatStrip.vue'
import RefreshButton from '../components/RefreshButton.vue'
import MemberPicker from '../components/MemberPicker.vue'
import MarkdownField from '../components/MarkdownField.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
import CopyLinkButton from '../components/CopyLinkButton.vue'
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
import { formatDate } from '../composables/useFormat.js'
import { useEnumLabel } from '../composables/useEnumLabel.js'
import { renderApiError } from '../composables/useApiError.js'

const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()
const { t } = useI18n()
const { enumLabel, entityLabel } = useEnumLabel()
const { success: showSaved, error: showError } = useToast()
const { confirm: confirmDialog } = useConfirm()

const renderMd = renderMarkdown

const userRole = ref('')
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')
const canReport = computed(() => userRole.value === 'admin' || userRole.value === 'manager')

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
const incidents = ref([])
const stats = ref({})
const statusStats = computed(() => [
  { key: '', label: t('common.stat.total'), count: stats.value.total || total.value || 0, color: 'text-slate-100' },
  { key: 'open', label: statusLabel('open'), count: stats.value.open || 0, color: 'text-red-400' },
  { key: 'investigating', label: statusLabel('investigating'), count: stats.value.investigating || 0, color: 'text-amber-400' },
  { key: 'contained', label: statusLabel('contained'), count: stats.value.contained || 0, color: 'text-blue-400' },
  { key: 'resolved', label: statusLabel('resolved'), count: stats.value.resolved || 0, color: 'text-emerald-400' },
  { key: 'closed', label: statusLabel('closed'), count: stats.value.closed || 0, color: 'text-slate-400' },
])
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

// The members each <select> offers. The set is this view's own choice; only the
// label comes from the shared catalogue.
const STATUSES = ['draft', 'open', 'investigating', 'contained', 'resolved', 'closed']
const SEVERITIES = ['critical', 'high', 'medium', 'low']
const INCIDENT_TYPES = ['incident', 'event', 'weakness']
const ORIGINS = ['internal', 'external', 'internal and external']
const GDPR_ROLES = ['controller', 'processor']
const NOTIFICATION_STATES = ['not_required', 'pending', 'notified']

// Lookups and option lists live here rather than in the template: a group name
// is a stored identifier, and the raw-text scanner reads a bare quoted word in
// a mustache as unextracted copy.
const statusLabel = (v) => enumLabel('status', v)
const severityLabel = (v) => enumLabel('severity', v)
const typeLabel = (v) => enumLabel('incident_type', v)
// incidents.source shares its value set with risks.origin, so it shares the
// group — `source` is reserved for corrective_actions.source, a different set.
const originLabel = (v) => enumLabel('origin', v)
const gdprRoleLabel = (v) => enumLabel('gdpr_role', v)
// Defaults to the column default rather than taking it from the call site: a
// bare stored value in a mustache is what the raw-text scanner counts, and
// correctly so.
const notificationLabel = (v) => enumLabel('notification_status', v || 'not_required')

const options = (values, label) => computed(() => values.map((value) => ({ value, label: label(value) })))
const statusOptions = options(STATUSES, statusLabel)
const severityOptions = options(SEVERITIES, severityLabel)
const typeOptions = options(INCIDENT_TYPES, typeLabel)
const originOptions = options(ORIGINS, originLabel)
const gdprRoleOptions = options(GDPR_ROLES, gdprRoleLabel)
const notificationOptions = options(NOTIFICATION_STATES, notificationLabel)

// The tab bar was already a computed, so it was reactive — but its labels were
// English literals, so it was reactive to the wrong thing. It holds keys now.
const detailTabs = computed(() => {
  const base = [{ key: 'overview', label: t('common.tab.overview') }]
  if (selectedIncident.value?.data_breach) {
    base.push({ key: 'data_breach', label: t('incidents.tab.data_breach') })
  }
  base.push({ key: 'notes', label: t('common.tab.notes') })
  base.push({ key: 'links', label: t('common.tab.links') })
  base.push({ key: 'actions', label: t('common.tab.actions') })
  base.push({ key: 'suggestions', label: t('common.tab.suggestions') })
  base.push({ key: 'comments', label: t('common.tab.comments') })
  base.push({ key: 'history', label: t('common.tab.history') })
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
  if (selectedIncident.value && (selectedIncident.value.identifier === id || String(selectedIncident.value.id) === String(id))) return
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
  await Promise.all([loadIncidents(), loadStats()])
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
    error.value = renderApiError(e)
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
  let inc = incidents.value.find(i => i.identifier === id || String(i.id) === String(id))
  if (!inc) {
    try { inc = await api.fetchJSON(`/api/v1/incidents/${encodeURIComponent(id)}`) } catch { return }
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
    error.value = renderApiError(e)
    showError(t('incidents.error.create', { message: renderApiError(e) }))
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
  if (!await confirmDialog({ message: t('incidents.danger.confirm', { title: selectedIncident.value.title }), confirmLabel: t('common.action.delete'), variant: 'danger' })) return
  try {
    await api.deleteIncident(selectedIncident.value.id)
    closeDetail()
    await loadIncidents()
  } catch (e) {
    error.value = renderApiError(e)
    showError(t('incidents.error.delete', { message: renderApiError(e) }))
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
    showSaved(t('common.state.saved'))
  } catch (e) {
    showError(t('incidents.error.save', { message: renderApiError(e) }))
  } finally {
    saving.value = false
  }
}

async function selectIncident(inc) {
  if (selectedIncident.value?.id === inc.id) {
    closeDetail()
    return
  }
  router.push(orgPath(`/incidents/${inc.identifier}`))
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
  router.push(orgPath('/incidents'))
}

// The initials are copy: which letters abbreviate confidentiality, integrity
// and availability is a property of the language, not of the data. Invisible to
// both scanners — the raw-text one never reads script, and a lone capital in a
// template fails its has-a-word test.
function classificationLabel(inc) {
  const parts = []
  if (inc.affects_c) parts.push(t('common.cia_abbr.c'))
  if (inc.affects_i) parts.push(t('common.cia_abbr.i'))
  if (inc.affects_a) parts.push(t('common.cia_abbr.a'))
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

const formatDateTime = (d) => formatDate(d, 'datetime')
</script>
