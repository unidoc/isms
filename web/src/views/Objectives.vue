<template>
  <div class="min-h-full">
    <div class="overflow-y-auto">
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
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">{{ t('objectives.title') }}</h1>
          <p class="text-sm text-slate-500 mt-1">{{ t('objectives.subtitle') }}</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canWrite" @click="showCreateObjective = !showCreateObjective"
            class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            {{ t('objectives.action.add') }}
          </button>
          <SuggestNewButton entityType="objective" :typeLabel="entityLabel('objective')" />
        </div>
      </div>

      <!-- Objectives -->
      <div class="space-y-6">
        <!-- Stats strip -->
        <StatStrip :stats="statusStats" v-model="filterStatus" />

        <!-- Filters + Create -->
        <div class="flex items-center gap-3 flex-wrap">
          <RefreshButton :loading="refreshing" @refresh="reload" class="shrink-0" />
          <div class="relative flex-1 max-w-xs">
            <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
            </svg>
            <input v-model="searchQuery" type="text" :placeholder="t('common.placeholder.search')"
              class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
          </div>
          <select v-model="filterProgramId"
            class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">{{ t('objectives.filter.all_programs') }}</option>
            <option v-for="p in programs" :key="p.id" :value="p.id">{{ p.key }}: {{ p.title }}</option>
          </select>
          <select v-model="filterStatus"
            class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">{{ t('common.filter.all_statuses') }}</option>
            <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
          <input v-model="filterOwner" type="text" :placeholder="t('objectives.filter.owner_placeholder')"
            class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 placeholder-slate-600 focus:outline-none focus:border-blue-500 w-40" />
          <div class="ml-auto text-xs text-slate-500 tabular-nums">{{ t('common.count.total', { count: total }) }}</div>
        </div>

        <!-- Create objective form (modal) -->
        <Teleport to="body">
        <Transition name="modal">
        <div v-if="showCreateObjective" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
          <div class="absolute inset-0 bg-black/60" @click="showCreateObjective = false" />
          <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
          <h3 class="text-sm font-semibold text-slate-300">{{ t('objectives.create.heading') }}</h3>
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="block text-xs text-slate-500 mb-1">{{ t('objectives.create.program_label') }}</label>
              <select v-model="newObjective.program_id"
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                <option value="">{{ t('objectives.create.program_placeholder') }}</option>
                <option v-for="p in programs" :key="p.id" :value="p.id">{{ p.key }}: {{ p.title }}</option>
              </select>
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1">{{ t('objectives.create.owner_label') }}</label>
              <MemberPicker v-model="newObjective.owner" :members="orgMembers" :placeholder="t('objectives.placeholder.owner')" />
            </div>
            <div class="md:col-span-2">
              <label class="block text-xs text-slate-500 mb-1">{{ t('objectives.create.title_label') }}</label>
              <input v-model="newObjective.title"
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500"
                :placeholder="t('objectives.create.title_placeholder')" />
            </div>
          </div>
          <div class="text-[10px] text-slate-600 mt-1">{{ t('objectives.create.fill_in_later') }}</div>
          <div class="flex justify-end gap-2">
            <button @click="showCreateObjective = false"
              class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">{{ t('common.action.cancel') }}</button>
            <button @click="createObjective" :disabled="!newObjective.program_id || !newObjective.title"
              class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 disabled:text-slate-500 text-white text-sm rounded-lg transition-colors">
              {{ t('objectives.create.submit') }}
            </button>
          </div>
          </div>
        </div>
        </Transition>
        </Teleport>

        <!-- Objectives table -->
        <div class="bg-slate-900 border border-slate-800 rounded-xl overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-slate-800">
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('objectives.table.header.id') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('objectives.table.header.title') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('objectives.table.header.target') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('objectives.table.header.status') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('objectives.table.header.owner') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider w-20"></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/50">
              <tr v-for="o in objectives" :key="o.id"
                @click="selectObjective(o)"
                class="hover:bg-slate-800/30 transition-colors cursor-pointer"
                :class="selectedObjective?.id === o.id ? 'bg-slate-800/50' : ''">
                <td class="px-4 py-3 font-mono text-blue-400 font-medium whitespace-nowrap">{{ o.display_id }}</td>
                <td class="px-4 py-3 text-slate-200">{{ o.title }}</td>
                <td class="px-4 py-3 text-slate-400 whitespace-nowrap">
                  <template v-if="o.target_value != null">
                    {{ opSymbol(o.target_operator) }} {{ o.target_value }} {{ o.unit }}
                  </template>
                  <span v-else class="text-slate-600">-</span>
                </td>
                <td class="px-4 py-3">
                  <StatusBadge :status="o.status" />
                </td>
                <td class="px-4 py-3 text-slate-400 text-xs">{{ resolveUserName(o.owner) }}</td>
                <td class="px-4 py-3" @click.stop>
                  <div v-if="canWrite" class="flex items-center gap-2">
                    <select @change="quickStatusChange(o, $event.target.value); $event.target.value = ''"
                      class="bg-slate-800 border border-slate-700 rounded-lg px-2 py-1 text-[10px] text-slate-400 focus:outline-none focus:border-blue-500 cursor-pointer">
                      <option value="" disabled selected>{{ statusLabel(o.status) }}</option>
                      <option v-for="s in statusOptions" :key="s.value" :value="s.value">{{ s.label }}</option>
                    </select>
                  </div>
                </td>
              </tr>
              <tr v-if="objectives.length === 0">
                <td colspan="6" class="px-4 py-8 text-center text-slate-600 text-sm">{{ t('objectives.filter.empty') }}</td>
              </tr>
            </tbody>
          </table>
          <Pagination :page="page" :pageSize="pageSize" :total="total" @update:page="page = $event" @update:pageSize="pageSize = $event" />
        </div>

      </div>
    </div>

    <!-- Detail modal -->
    <Teleport to="body">
    <Transition name="modal">
    <div v-if="selectedObjective" class="fixed inset-0 z-50 flex items-start justify-center pt-[3vh] px-4">
      <div class="absolute inset-0 bg-black/60" @click="closeDetail" />
      <div class="relative w-full max-w-4xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-h-[90vh] flex flex-col">
        <!-- Header -->
        <div class="flex-shrink-0 border-b border-slate-800 px-6 py-3 flex items-center justify-between gap-4">
          <div class="flex items-center gap-6 min-w-0">
            <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ selectedObjective.display_id }}</span>
            <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selectedObjective.title }}</h2>
          </div>
          <div class="flex items-center gap-2 flex-shrink-0">
            <CopyLinkButton />
            <StatusBadge :status="selectedObjective.status" />
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
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('objectives.detail.overview') }}</div>
                  <button v-if="canWrite && !editingSection" @click="editSection('overview')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                </div>
                <template v-if="editingSection === 'overview'">
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('objectives.field.title') }}</label>
                      <input v-model="editForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('objectives.field.description') }}</label>
                      <MarkdownField v-model="editForm.description" :self-type="'objective'" :self-id="selectedObjective?.display_id || String(selectedObjective?.id || '')" :rows="3" :placeholder="t('objectives.placeholder.description')" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('objectives.field.owner') }}</label>
                      <MemberPicker v-model="editForm.owner" :members="orgMembers" :placeholder="t('objectives.placeholder.owner')" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('objectives.field.source') }}</label>
                      <input v-model="editForm.source" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('objectives.placeholder.source')" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('objectives.field.status') }}</label>
                      <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                      </select>
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('objectives.field.measurement_method') }}</label>
                      <input v-model="editForm.measurement_method" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('objectives.placeholder.measurement_method')" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('objectives.field.operator') }}</label>
                      <select v-model="editForm.target_operator" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option v-for="o in operatorOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                      </select>
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('objectives.field.target_value') }}</label>
                      <input v-model.number="editForm.target_value" type="number" step="any" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('objectives.field.unit') }}</label>
                      <input v-model="editForm.unit" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('objectives.placeholder.unit')" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('objectives.field.checkin_cycle_months') }}</label>
                      <input v-model.number="editForm.checkin_cycle" type="number" min="1" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                  </div>
                </template>
                <template v-else>
                  <div class="space-y-4">
                    <div>
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('objectives.field.description') }}</div>
                      <div v-if="selectedObjective.description" class="text-sm text-slate-300 leading-relaxed doc-prose" v-mermaid v-html="renderMd(selectedObjective.description)"></div>
                      <div v-else class="text-sm text-slate-600">—</div>
                    </div>

                    <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('objectives.field.owner') }}</div>
                        <div class="text-sm text-slate-300">{{ resolveUserName(selectedObjective.owner) }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('objectives.field.source') }}</div>
                        <div class="text-sm text-slate-300">{{ selectedObjective.source || '—' }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('objectives.field.target') }}</div>
                        <div class="text-sm text-slate-300 font-mono">
                          <template v-if="selectedObjective.target_value != null">
                            {{ opSymbol(selectedObjective.target_operator) }} {{ selectedObjective.target_value }} {{ selectedObjective.unit }}
                          </template>
                          <span v-else class="text-slate-600">—</span>
                        </div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('objectives.field.measurement') }}</div>
                        <div class="text-sm text-slate-300">{{ selectedObjective.measurement_method || '—' }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('objectives.field.checkin_cycle') }}</div>
                        <div class="text-sm text-slate-300">{{ t('objectives.field.months', { count: selectedObjective.checkin_cycle || 12 }, selectedObjective.checkin_cycle || 12) }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('objectives.field.last_checkin') }}</div>
                        <div class="text-sm" :class="latestCheckinIsPass ? 'text-emerald-400' : (latestCheckinIsFail ? 'text-red-400' : 'text-slate-300')">
                          {{ checkins.length > 0 ? formatDate(checkins[0].occurred_at) : '—' }}
                        </div>
                      </div>
                      <div v-if="selectedObjective.created_at">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('objectives.field.created') }}</div>
                        <div class="text-sm text-slate-300">{{ formatDate(selectedObjective.created_at) }}</div>
                      </div>
                      <div v-if="selectedObjective.created_by">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('objectives.field.created_by') }}</div>
                        <div class="text-sm text-slate-300">{{ resolveUserName(selectedObjective.created_by) }}</div>
                      </div>
                    </div>
                  </div>
                </template>
              </div>
            </template>

            <!-- ═══ CHECKINS ═══ -->
            <template v-if="detailTab === 'checkins'">
              <div class="px-6 py-5 space-y-5">
                <div class="flex items-center justify-between">
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('objectives.checkin.heading') }}</div>
                  <div v-if="checkins.length > 0" class="text-[11px] text-slate-500">
                    {{ t('objectives.checkin.summary', { pass: passCount, fail: failCount, total: checkins.length }) }}
                  </div>
                </div>

                <!-- Add checkin form -->
                <div v-if="canWrite" class="bg-slate-800/40 border border-slate-700/50 rounded-lg p-4 space-y-3">
                  <div class="text-[10px] font-semibold text-slate-400 uppercase tracking-wider">{{ t('objectives.checkin.record_heading') }}</div>
                  <div class="grid grid-cols-1 md:grid-cols-4 gap-3">
                    <div>
                      <label class="block text-xs text-slate-500 mb-1">{{ t('objectives.checkin.value_label') }}</label>
                      <input v-model.number="newCheckin.value_numeric" type="number" step="any"
                        class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500"
                        :placeholder="t('objectives.checkin.value_placeholder')" />
                    </div>
                    <div>
                      <label class="block text-xs text-slate-500 mb-1">{{ t('objectives.checkin.result_label') }}</label>
                      <select v-model="newCheckin.success"
                        class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option :value="null">{{ t('objectives.checkin.result.unspecified') }}</option>
                        <option :value="true">{{ t('objectives.checkin.result.pass') }}</option>
                        <option :value="false">{{ t('objectives.checkin.result.fail') }}</option>
                      </select>
                    </div>
                    <div class="md:col-span-2">
                      <label class="block text-xs text-slate-500 mb-1">{{ t('objectives.checkin.message_label') }}</label>
                      <input v-model="newCheckin.message"
                        class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500"
                        :placeholder="t('objectives.checkin.message_placeholder')" />
                    </div>
                  </div>
                  <div class="flex items-center gap-2">
                    <input v-model="newCheckin.public_note"
                      class="flex-1 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500"
                      :placeholder="t('objectives.checkin.public_note_placeholder')" />
                    <button @click="createCheckin"
                      class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors whitespace-nowrap">
                      {{ t('objectives.checkin.submit') }}
                    </button>
                  </div>
                </div>

                <!-- Checkins timeline -->
                <div v-if="checkins.length === 0" class="text-center text-slate-600 text-sm py-12 border border-dashed border-slate-800 rounded-lg">
                  {{ t('objectives.checkin.empty') }}
                </div>
                <div v-else class="space-y-2">
                  <div v-for="ci in checkins" :key="ci.id"
                    class="bg-slate-800/30 border rounded-lg p-3"
                    :class="ci.success === true ? 'border-emerald-900/40' : ci.success === false ? 'border-red-900/40' : 'border-slate-800'">
                    <div class="flex items-start justify-between gap-3">
                      <div class="flex items-center gap-2 flex-wrap">
                        <span class="text-xs text-slate-500 font-mono">{{ formatDate(ci.occurred_at) }}</span>
                        <span v-if="ci.success === true" class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-bold bg-emerald-500/20 text-emerald-300">{{ t('objectives.checkin.pass_badge') }}</span>
                        <span v-else-if="ci.success === false" class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-bold bg-red-500/20 text-red-300">{{ t('objectives.checkin.fail_badge') }}</span>
                        <span v-if="ci.value_numeric != null" class="text-sm font-mono text-slate-200">
                          {{ ci.value_numeric }}{{ selectedObjective?.unit ? ' ' + selectedObjective.unit : '' }}
                        </span>
                      </div>
                      <div class="flex items-center gap-2 flex-shrink-0">
                        <span class="text-[10px] text-slate-600">{{ resolveUserName(ci.created_by) }}</span>
                        <button v-if="canWrite" @click="deleteCheckin(ci)" class="text-[10px] text-red-400/60 hover:text-red-400">{{ t('common.action.delete') }}</button>
                      </div>
                    </div>
                    <div v-if="ci.message" class="text-sm text-slate-300 mt-2">{{ ci.message }}</div>
                    <div v-if="ci.public_note" class="text-sm text-blue-300/80 mt-1 italic">{{ t('objectives.checkin.public', { note: ci.public_note }) }}</div>

                    <!-- Evidence -->
                    <div class="mt-3 space-y-1">
                      <div v-for="ev in (evidenceByCheckin[ci.id] || [])" :key="ev.id"
                        class="flex items-center justify-between text-xs bg-slate-800/50 rounded px-2 py-1.5">
                        <div class="flex items-center gap-2 min-w-0">
                          <svg class="w-3.5 h-3.5 text-slate-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                            <path stroke-linecap="round" stroke-linejoin="round" d="M15.172 7l-6.586 6.586a2 2 0 102.828 2.828l6.414-6.586a4 4 0 00-5.656-5.656l-6.415 6.585a6 6 0 108.486 8.486L20.5 13" />
                          </svg>
                          <span class="text-slate-300 truncate">{{ ev.title }}</span>
                          <span class="text-slate-600 flex-shrink-0">{{ ev.content_type }}</span>
                          <span v-if="ev.size_bytes" class="text-slate-600 flex-shrink-0">{{ formatBytes(ev.size_bytes) }}</span>
                        </div>
                        <div class="flex items-center gap-2 flex-shrink-0 ml-2">
                          <button @click="downloadEv(ev)" class="text-blue-400 hover:text-blue-300">{{ t('objectives.evidence.download') }}</button>
                          <button v-if="canWrite" @click="deleteEv(ev)" class="text-red-400/60 hover:text-red-400">{{ t('common.action.delete') }}</button>
                        </div>
                      </div>
                      <div v-if="canWrite" class="flex items-center gap-2">
                        <input type="file" :id="'ev-' + ci.id" class="hidden" @change="(e) => uploadEvidence(ci.id, e)" />
                        <label :for="'ev-' + ci.id"
                          class="text-[11px] text-slate-500 hover:text-blue-400 transition-colors flex items-center gap-1 cursor-pointer">
                          <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
                          </svg>
                          {{ t('objectives.evidence.attach') }}
                        </label>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </template>

            <!-- ═══ NOTES ═══ -->
            <template v-if="detailTab === 'notes'">
              <div class="px-6 py-5 space-y-5">
                <div class="flex items-center justify-between">
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('objectives.detail.notes') }}</div>
                  <button v-if="canWrite && !editingSection" @click="editSection('notes')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                </div>
                <template v-if="editingSection === 'notes'">
                  <MarkdownField v-model="editForm.notes" :self-type="'objective'" :self-id="selectedObjective?.display_id || String(selectedObjective?.id || '')" :rows="12" :placeholder="t('objectives.placeholder.notes')" />
                </template>
                <template v-else>
                  <div v-if="selectedObjective.notes" class="text-sm doc-prose text-slate-300 leading-relaxed" v-mermaid v-html="renderMd(selectedObjective.notes)"></div>
                  <div v-else class="text-sm text-slate-600 italic">{{ t('common.state.no_notes') }}</div>
                </template>
              </div>
            </template>

            <!-- ═══ LINKS ═══ -->
            <template v-if="detailTab === 'links'">
              <div class="px-6 py-5">
                <ReferenceManager entityType="objective" :entityId="selectedObjective.display_id" :editable="canWrite" />
              </div>
            </template>

            <!-- ═══ SUGGESTIONS ═══ -->
            <template v-if="detailTab === 'suggestions'">
              <div class="px-6 py-5">
                <SuggestionPanel entityType="objective" :entityId="selectedObjective.display_id" :canReview="canWrite" @applied="loadAll" />
              </div>
            </template>

            <!-- ═══ COMMENTS ═══ -->
            <template v-if="detailTab === 'comments'">
              <div class="px-6 py-5">
                <CommentsPanel entityType="objective" :entityId="selectedObjective.display_id" />
              </div>
            </template>

            <!-- ═══ HISTORY ═══ -->
            <template v-if="detailTab === 'history'">
              <div class="px-6 py-5 space-y-6">
                <HistoryPanel entityType="objective" :entityId="String(selectedObjective.id)" />
                <div v-if="canWrite" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                  <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">{{ t('common.heading.danger_zone') }}</div>
                  <div class="text-xs text-slate-400">{{ t('objectives.danger.warning') }}</div>
                  <button @click="deleteSelectedObjective" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                    {{ t('objectives.danger.delete') }}
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
import { useToast } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm.js'
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { renderMarkdown } from '../composables/useRenderMd.js'
import MemberPicker from '../components/MemberPicker.vue'
import StatStrip from '../components/StatStrip.vue'
import RefreshButton from '../components/RefreshButton.vue'
import MarkdownField from '../components/MarkdownField.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
import CopyLinkButton from '../components/CopyLinkButton.vue'
import SuggestionPanel from '../components/SuggestionPanel.vue'
import SuggestNewButton from '../components/SuggestNewButton.vue'
import HistoryPanel from '../components/HistoryPanel.vue'
import CommentsPanel from '../components/CommentsPanel.vue'
import Pagination from '../components/Pagination.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { useModalEscape } from '../composables/useModalEscape.js'
import { useDirtyEdit } from '../composables/useDirtyEdit.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import { formatDate, formatNumber } from '../composables/useFormat.js'
import { renderApiError } from '../composables/useApiError.js'
import { enumLabel, entityLabel } from '../composables/useEnumLabel.js'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { orgPath } = useCurrentOrg()
const { success: showSaved, error: showError } = useToast()
const { ask: confirmAsk, confirm: confirmDialog } = useConfirm()

const userRole = ref('')
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')

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
const programs = ref([])
const objectives = ref([])
const checkins = ref([])
const selectedObjective = ref(null)
const showCreateObjective = ref(false)
useModalEscape(showCreateObjective)
useModalEscape(computed(() => !!selectedObjective.value), () => closeDetail())
const pendingRefs = ref([])
const evidenceByCheckin = ref({})

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
  { key: 'checkins', label: 'objectives.checkin.heading' },
  { key: 'notes', label: 'common.tab.notes' },
  { key: 'links', label: 'common.tab.links' },
  { key: 'suggestions', label: 'common.tab.suggestions' },
  { key: 'comments', label: 'common.tab.comments' },
  { key: 'history', label: 'common.tab.history' },
]
const detailTabs = computed(() => TAB_KEYS.map((tab) => ({ ...tab, label: t(tab.label) })))

const passCount = computed(() => checkins.value.filter(c => c.success === true).length)
const failCount = computed(() => checkins.value.filter(c => c.success === false).length)
const latestCheckinIsPass = computed(() => checkins.value.length > 0 && checkins.value[0].success === true)
const latestCheckinIsFail = computed(() => checkins.value.length > 0 && checkins.value[0].success === false)

const filterProgramId = ref('')
const filterStatus = ref('')
const filterOwner = ref('')
const searchQuery = ref('')
const page = ref(1)
const pageSize = ref(50)
const total = ref(0)
const stats = ref({ total: 0, draft: 0, active: 0, at_risk: 0, paused: 0, complete: 0, archived: 0 })
const STATUS_STATS = [
  { key: '', field: 'total', color: 'text-slate-100' },
  { key: 'active', field: 'active', color: 'text-emerald-400' },
  { key: 'at_risk', field: 'at_risk', color: 'text-red-400' },
  { key: 'paused', field: 'paused', color: 'text-amber-400' },
  { key: 'complete', field: 'complete', color: 'text-blue-400' },
]
const statusStats = computed(() => STATUS_STATS.map(({ key, field, color }) => ({
  key,
  color,
  label: key ? statusLabel(key) : t('common.stat.total'),
  count: stats.value[field] || 0,
})))
const orgMembers = ref([])

const newObjective = reactive({
  program_id: '',
  title: '',
  owner: '',
})
const newCheckin = reactive({
  value_numeric: null,
  success: null,
  message: '',
  public_note: '',
})

// Lookups and option lists live here rather than in the template: a group name
// is a stored identifier, and the raw-text scanner reads a bare quoted word in
// a mustache as unextracted copy.
const STATUSES = ['draft', 'active', 'at_risk', 'paused', 'complete']
const OPERATORS = ['gte', 'lte', 'eq', 'gt', 'lt']

const statusLabel = (v) => enumLabel('status', v)
const opSymbol = (v) => enumLabel('target_operator', v)

const options = (values, label) => computed(() => values.map((value) => ({ value, label: label(value) })))
const statusOptions = options(STATUSES, statusLabel)
const operatorOptions = options(OPERATORS, opSymbol)

// The unit is a word, not a suffix: it comes from the catalogue, and the
// number goes through Intl so a locale that groups or decimalises differently
// is not handed a JavaScript-formatted string.
function formatBytes(bytes) {
  if (!bytes) return ''
  if (bytes < 1024) return t('common.bytes.b', { value: formatNumber(bytes) })
  if (bytes < 1024 * 1024) {
    return t('common.bytes.kb', { value: formatNumber(bytes / 1024, { maximumFractionDigits: 1, minimumFractionDigits: 1 }) })
  }
  return t('common.bytes.mb', { value: formatNumber(bytes / (1024 * 1024), { maximumFractionDigits: 1, minimumFractionDigits: 1 }) })
}

const renderMd = renderMarkdown

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
  const progs = await api.getPrograms()
  programs.value = Array.isArray(progs) ? progs : []
  await loadObjectives()
}

async function loadObjectives() {
  try {
    const params = new URLSearchParams()
    params.set('page', String(page.value))
    params.set('limit', String(pageSize.value))
    if (searchQuery.value) params.set('q', searchQuery.value)
    if (filterStatus.value) params.set('status', filterStatus.value)
    if (filterProgramId.value) params.set('program_id', String(filterProgramId.value))
    if (filterOwner.value) params.set('owner', filterOwner.value)
    const res = await api.fetchRaw(`/api/v1/objectives?${params.toString()}`)
    objectives.value = Array.isArray(res?.data) ? res.data : []
    total.value = res?.total || 0
    loadStats()
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function loadStats() {
  try { stats.value = await api.fetchJSON('/api/v1/objectives/stats') || stats.value } catch {}
}

let searchTimer = null
let ownerTimer = null
watch([searchQuery], () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { page.value = 1; loadObjectives() }, 250)
})
watch([filterOwner], () => {
  clearTimeout(ownerTimer)
  ownerTimer = setTimeout(() => { page.value = 1; loadObjectives() }, 250)
})
watch([filterStatus, filterProgramId], () => {
  page.value = 1
  loadObjectives()
})
watch([page, pageSize], () => loadObjectives())

async function loadCheckins() {
  if (!selectedObjective.value) return
  try {
    checkins.value = await api.getCheckins(selectedObjective.value.id, 50) || []
    // Load evidence for each checkin
    evidenceByCheckin.value = {}
    for (const ci of checkins.value) {
      try {
        const evs = await api.getEvidence(ci.id)
        if (evs && evs.length) {
          evidenceByCheckin.value[ci.id] = evs
        }
      } catch { /* ignore */ }
    }
  } catch (e) {
    console.error('Failed to load checkins:', e)
  }
}

async function selectObjective(o) {
  if (selectedObjective.value?.id === o.id) {
    closeDetail()
    return
  }
  router.push(orgPath(`/objectives/${o.display_id}`))
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
  router.push(orgPath('/objectives'))
}

async function openObjectiveFromRoute(id) {
  let obj = objectives.value.find(o => o.display_id === id || String(o.id) === String(id))
  if (!obj) {
    try { obj = await api.fetchJSON(`/api/v1/objectives/${encodeURIComponent(id)}`) } catch { return }
  }
  if (!obj) return
  selectedObjective.value = obj
  detailTab.value = 'overview'
  editingSection.value = ''
  startEditObjective(obj)
  loadCheckins()
}

function startEditObjective(o) {
  editForm.value = {
    title: o.title || '',
    description: o.description || '',
    owner: o.owner || '',
    source: o.source || '',
    status: o.status || 'draft',
    measurement_method: o.measurement_method || '',
    target_value: o.target_value,
    target_operator: o.target_operator || 'gte',
    unit: o.unit || '',
    checkin_cycle: o.checkin_cycle || 12,
    notes: o.notes || '',
  }
  captureEditSnapshot()
}

function editSection(section) {
  startEditObjective(selectedObjective.value)
  editingSection.value = section
}

function cancelSection() {
  editingSection.value = ''
  startEditObjective(selectedObjective.value)
}

async function saveSection() {
  if (!selectedObjective.value) return
  saving.value = true
  try {
    const payload = { ...editForm.value }
    if (payload.target_value === '' || payload.target_value === null) delete payload.target_value
    await api.updateObjective(selectedObjective.value.id, payload)
    await loadObjectives()
    const fresh = objectives.value.find(o => o.id === selectedObjective.value.id)
    if (fresh) {
      selectedObjective.value = fresh
      startEditObjective(fresh)
    } else {
      try {
        const data = await api.fetchJSON(`/api/v1/objectives/${selectedObjective.value.id}`)
        if (data) { selectedObjective.value = data; startEditObjective(data) }
      } catch { /* ignore */ }
    }
    editingSection.value = ''
    showSaved(t('common.state.saved'))
  } catch (e) {
    showError(t('objectives.error.save', { message: renderApiError(e) }))
  } finally {
    saving.value = false
  }
}

async function deleteSelectedObjective() {
  if (!selectedObjective.value) return
  const question = t('objectives.danger.confirm', { title: selectedObjective.value.title })
  if (!await confirmAsk(question, { confirm: t('common.action.delete'), variant: 'danger' })) return
  try {
    await api.deleteObjective(selectedObjective.value.id)
    closeDetail()
    await loadObjectives()
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function createObjective() {
  try {
    const payload = { ...newObjective }
    if (payload.program_id) payload.program_id = Number(payload.program_id)
    if (!payload.owner) delete payload.owner
    const created = await api.createObjective(payload)
    const entityId = created?.display_id || (created?.id ? ('OBJ-' + created.id) : '')
    if (entityId) {
      for (const ref of pendingRefs.value) {
        try {
          await api.createReference({ source_type: 'objective', source_id: entityId, target_type: ref.type, target_id: ref.id })
        } catch { /* non-fatal, entity was already created */ }
      }
    }
    pendingRefs.value = []
    showCreateObjective.value = false
    Object.assign(newObjective, { program_id: '', title: '', owner: '' })
    await loadObjectives()
    // Drop user into detail modal in edit mode on Overview to keep filling things in.
    if (created && created.id) {
      let fresh = created
      try { fresh = await api.getObjective(created.id) } catch { /* fall back */ }
      selectedObjective.value = fresh
      detailTab.value = 'overview'
      startEditObjective(fresh)
      editingSection.value = 'overview'
      loadCheckins()
      router.push(orgPath(`/objectives/${fresh.id}`))
    }
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function quickStatusChange(obj, newStatus) {
  try {
    await api.updateObjective(obj.id, { status: newStatus })
    obj.status = newStatus
    loadStats()
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function createCheckin() {
  if (!selectedObjective.value) return
  try {
    const payload = {}
    if (newCheckin.value_numeric !== null && newCheckin.value_numeric !== '') payload.value_numeric = Number(newCheckin.value_numeric)
    if (newCheckin.success !== null) payload.success = newCheckin.success
    if (newCheckin.message) payload.message = newCheckin.message
    if (newCheckin.public_note) payload.public_note = newCheckin.public_note
    await api.createCheckin(selectedObjective.value.id, payload)
    Object.assign(newCheckin, { value_numeric: null, success: null, message: '', public_note: '' })
    await loadCheckins()
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function deleteCheckin(ci) {
  if (!await confirmAsk(t('objectives.checkin.confirm_delete'), { confirm: t('common.action.delete'), variant: 'danger' })) return
  try {
    await api.deleteCheckin(ci.id)
    await loadCheckins()
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function uploadEvidence(checkinId, event) {
  const file = event.target.files?.[0]
  if (!file) return
  try {
    await api.uploadEvidence(checkinId, file, file.name)
    event.target.value = ''
    await loadCheckins()
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function downloadEv(ev) {
  try {
    const result = await api.downloadEvidence(ev.id)
    if (result?.url) {
      // S3 backend: presigned URL.
      window.open(result.url, '_blank')
    } else if (result?.blob) {
      // Local (file) backend: trigger a download of the streamed file.
      const objUrl = URL.createObjectURL(result.blob)
      const a = document.createElement('a')
      a.href = objUrl
      a.download = result.filename || ev.title || 'evidence'
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(objUrl)
    } else {
      error.value = t('objectives.error.download')
    }
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function deleteEv(ev) {
  const question = t('objectives.evidence.confirm_delete', { title: ev.title })
  if (!await confirmAsk(question, { confirm: t('common.action.delete'), variant: 'danger' })) return
  try {
    await api.deleteEvidence(ev.id)
    await loadCheckins()
  } catch (e) {
    error.value = renderApiError(e)
  }
}

function resolveUserName(email) {
  if (!email) return '-'
  const u = orgMembers.value.find(m => m.email === email)
  return u?.name || email
}

onMounted(async () => {
  try { const me = await api.getMe(); userRole.value = me?.role || '' } catch {}
  try { orgMembers.value = await api.getUsers() || [] } catch { orgMembers.value = [] }
  await loadAll()
  // Deep-link: auto-select objective from route param
  if (route.params.objId) await openObjectiveFromRoute(route.params.objId)
})

watch(() => route.params.objId, (objId) => {
  if (!objId) {
    selectedObjective.value = null
    detailTab.value = 'overview'
    editingSection.value = ''
    checkins.value = []
    return
  }
  if (selectedObjective.value && (selectedObjective.value.display_id === objId || String(selectedObjective.value.id) === String(objId))) return
  openObjectiveFromRoute(objId)
})
</script>
