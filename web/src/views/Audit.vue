<template>
  <div class="min-h-full">
    <div class="overflow-y-auto">
    <!-- Loading -->
    <div v-if="loading" class="max-w-6xl mx-auto px-8 py-10">
      <ListSkeleton :rows="5" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="max-w-5xl mx-auto px-8 py-12">
      <div class="bg-red-950/40 border border-red-900/50 rounded-lg p-6 text-red-300 text-sm flex items-center justify-between gap-4">
        <span>{{ t('audit.error.load', { message: error }) }}</span>
        <RefreshButton :loading="refreshing" @refresh="reload" />
      </div>
    </div>

    <!-- Main content -->
    <div v-else class="max-w-6xl mx-auto px-8 py-10 space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">{{ t('audit.title') }}</h1>
          <p class="text-sm text-slate-500 mt-1">{{ t('audit.subtitle') }}</p>
        </div>
        <div class="flex gap-2">
          <RefreshButton :loading="refreshing" @refresh="reload" />
          <button v-if="canWrite && activeTab === 'programmes'"
            @click="openCreateProgramme"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            {{ t('audit.action.add_programme') }}
          </button>
          <button v-if="canWrite && activeTab === 'findings'"
            @click="openCreateFinding(null)"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            {{ t('audit.action.add_finding') }}
          </button>
          <button v-if="canWrite && activeTab === 'calendar'"
            @click="openCreateAudit(null)"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            {{ t('audit.action.add_audit') }}
          </button>
          <SuggestNewButton entityType="audit_finding" :typeLabel="entityLabel('audit_finding')" />
        </div>
      </div>

      <!-- Top-level tabs -->
      <div class="flex items-center border-b border-slate-800">
        <div class="flex gap-1 flex-1">
          <button v-for="tab in topTabs" :key="tab.key"
            @click="switchTab(tab.key)"
            class="flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px"
            :class="activeTab === tab.key ? 'border-blue-500 text-blue-400' : 'border-transparent text-slate-500 hover:text-slate-300'">
            {{ tab.label }}
          </button>
        </div>
      </div>

      <!-- ═══════════════════ CALENDAR TAB (default) ═══════════════════ -->
      <template v-if="activeTab === 'calendar'">
        <!-- Year switcher -->
        <div class="flex items-center justify-between">
          <button @click="changeCalendarYear(-1)"
            class="px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-300 text-sm font-medium rounded-lg transition-colors">
            <svg class="w-4 h-4 inline" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" /></svg>
            {{ calendarYear - 1 }}
          </button>
          <h2 class="text-lg font-bold text-slate-100 tabular-nums">{{ calendarYear }}</h2>
          <button @click="changeCalendarYear(1)"
            class="px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-300 text-sm font-medium rounded-lg transition-colors">
            {{ calendarYear + 1 }}
            <svg class="w-4 h-4 inline" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" /></svg>
          </button>
        </div>

        <!-- Type legend -->
        <div class="flex items-center gap-4 flex-wrap">
          <div v-for="type in auditTypeOptions" :key="type.value" class="flex items-center gap-1.5 text-xs text-slate-400">
            <span class="w-2.5 h-2.5 rounded-full" :class="auditTypeDot(type.value)"></span>
            <span>{{ type.label }}</span>
          </div>
        </div>

        <!-- Loading -->
        <div v-if="calendarLoading" class="flex items-center justify-center h-48">
          <div class="text-slate-400 text-sm">{{ t('audit.calendar.loading') }}</div>
        </div>

        <!-- Month sections -->
        <div v-else class="space-y-4">
          <div v-for="month in calendarMonths" :key="month.month"
            class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
            <div class="px-5 py-3 border-b border-slate-800 flex items-center justify-between">
              <h3 class="text-sm font-semibold text-slate-300">{{ monthName(month) }}</h3>
              <span class="text-[11px] text-slate-500">
                {{ t('audit.calendar.audit_count', { count: month.audits.length }, month.audits.length) }}
              </span>
            </div>
            <div v-if="month.audits.length === 0" class="px-5 py-3 text-xs text-slate-600 italic">{{ t('audit.calendar.month_empty') }}</div>
            <div v-else class="divide-y divide-slate-800/50">
              <div v-for="audit in month.audits" :key="audit.id"
                @click="selectAuditFromCalendar(audit)"
                class="px-5 py-3 flex items-center gap-3 hover:bg-slate-800/50 transition-colors cursor-pointer">
                <span class="w-2 h-2 rounded-full flex-shrink-0" :class="auditTypeDot(audit.audit_type)"></span>
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium text-slate-200 truncate">{{ audit.title }}</div>
                  <div class="text-[11px] text-slate-500 truncate">
                    {{ programmeTitle(audit.programme_id) }} {{ dot }}
                    {{ resolveUserName(audit.auditor) }} {{ dot }}
                    <span>{{ auditTypeLabel(audit.audit_type) }}</span>
                  </div>
                </div>
                <span class="text-xs text-slate-500 tabular-nums whitespace-nowrap flex-shrink-0">
                  {{ formatDateRange(audit.planned_date, audit.end_date) }}
                </span>
                <StatusBadge :status="audit.status" />
              </div>
            </div>
          </div>

          <!-- Unscheduled bucket -->
          <div v-if="calendarUnscheduled.length > 0" class="bg-slate-900 border border-amber-900/40 rounded-xl overflow-hidden">
            <div class="px-5 py-3 border-b border-amber-900/40 flex items-center justify-between">
              <h3 class="text-sm font-semibold text-amber-400">{{ t('audit.calendar.unscheduled') }}</h3>
              <span class="text-[11px] text-slate-500">
                {{ t('audit.calendar.unscheduled_count', { count: calendarUnscheduled.length }, calendarUnscheduled.length) }}
              </span>
            </div>
            <div class="divide-y divide-slate-800/50">
              <div v-for="audit in calendarUnscheduled" :key="audit.id"
                @click="selectAuditFromCalendar(audit)"
                class="px-5 py-3 flex items-center gap-3 hover:bg-slate-800/50 transition-colors cursor-pointer">
                <span class="w-2 h-2 rounded-full flex-shrink-0" :class="auditTypeDot(audit.audit_type)"></span>
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium text-slate-200 truncate">{{ audit.title }}</div>
                  <div class="text-[11px] text-slate-500 truncate">
                    {{ programmeTitle(audit.programme_id) }} {{ dot }} {{ resolveUserName(audit.auditor) }}
                  </div>
                </div>
                <StatusBadge :status="audit.status" />
              </div>
            </div>
          </div>

          <div v-if="calendarMonths.every(m => m.audits.length === 0) && calendarUnscheduled.length === 0"
            class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center text-sm text-slate-500">
            {{ t('audit.calendar.empty', { year: calendarYear }) }}
          </div>
        </div>
      </template>

      <!-- ═══════════════════ PROGRAMMES TAB ═══════════════════ -->
      <template v-if="activeTab === 'programmes'">
        <!-- Stats strip -->
        <StatStrip :stats="programmeStatusStats" v-model="programmeStatusFilter" />

        <!-- Filters -->
        <div class="flex items-center gap-3 flex-wrap">
          <div class="relative flex-1 max-w-xs">
            <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
            </svg>
            <input v-model="programmeSearch" type="text" :placeholder="t('audit.programme.search_placeholder')"
              class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
          </div>
          <select v-model.number="programmeYearFilter" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option :value="0">{{ t('audit.programme.all_years') }}</option>
            <option v-for="y in programmeYears" :key="y" :value="y">{{ y }}</option>
          </select>
          <select v-model="programmeStatusFilter" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">{{ t('common.filter.all_statuses') }}</option>
            <option v-for="o in programmeStatusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
          <button v-if="programmeSearch || programmeYearFilter || programmeStatusFilter"
            @click="programmeSearch = ''; programmeYearFilter = 0; programmeStatusFilter = ''"
            class="text-[10px] text-slate-600 hover:text-slate-400 transition-colors">
            {{ t('common.action.clear') }}
          </button>
          <div class="ml-auto text-xs text-slate-500 tabular-nums">{{ t('common.count.of', { shown: filteredProgrammes.length, total: programmes.length }) }}</div>
        </div>

        <!-- List -->
        <div v-if="filteredProgrammes.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
          <div v-if="programmeSearch || programmeYearFilter || programmeStatusFilter" class="text-sm text-slate-500">{{ t('audit.programme.empty_filtered') }}</div>
          <div v-else class="text-sm text-slate-500">{{ t('audit.programme.empty') }}</div>
        </div>
        <div v-else class="bg-slate-900 border border-slate-800 rounded-xl overflow-x-auto">
          <table class="w-full">
            <thead>
              <tr class="border-b border-slate-800">
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.programme.table.id') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.programme.table.title') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.programme.table.year') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.programme.table.audits') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.programme.table.findings') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.programme.table.status') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/50">
              <tr v-for="prog in filteredProgrammes" :key="prog.id"
                @click="selectProgramme(prog)"
                class="hover:bg-slate-800/50 transition-colors cursor-pointer">
                <td class="px-5 py-3.5 text-[10px] font-mono uppercase tracking-wider text-slate-600">{{ progRef(prog.id) }}</td>
                <td class="px-5 py-3.5">
                  <div class="text-sm font-medium text-slate-200">{{ prog.title }}</div>
                  <div v-if="prog.description" class="text-xs text-slate-500 mt-0.5 truncate max-w-md">{{ stripMd(prog.description) }}</div>
                </td>
                <td class="px-5 py-3.5 text-sm text-slate-400 tabular-nums">{{ prog.year }}</td>
                <td class="px-5 py-3.5 text-sm text-slate-400 tabular-nums">{{ prog.audit_count || 0 }}</td>
                <td class="px-5 py-3.5 text-sm tabular-nums" :class="programmeOpenFindings(prog.id) > 0 ? 'text-red-400' : 'text-slate-400'">
                  {{ programmeOpenFindings(prog.id) }}
                </td>
                <td class="px-5 py-3.5"><StatusBadge :status="prog.status" /></td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

      <!-- ═══════════════════ FINDINGS TAB ═══════════════════ -->
      <template v-if="activeTab === 'findings'">
        <!-- Stats strip (+ Overdue toggle, a separate boolean dimension) -->
        <div class="flex flex-wrap items-center gap-2">
          <StatStrip :stats="findingStatusStats" v-model="findingStatusFilter" />
          <button type="button" @click="findingOverdueOnly = !findingOverdueOnly"
            class="inline-flex items-baseline gap-1.5 rounded-full border px-3 py-1 text-xs transition-colors"
            :class="findingOverdueOnly
              ? 'border-amber-500/50 bg-amber-500/10 text-amber-200'
              : 'border-slate-800 bg-slate-900 text-slate-400 hover:border-slate-700 hover:text-slate-300'"
            :title="t('audit.findings.overdue_toggle_title')">
            <span class="font-bold tabular-nums" :class="findingOverdueOnly ? '' : (overdueFindingsTotal > 0 ? 'text-amber-400' : 'text-slate-100')">{{ overdueFindingsTotal }}</span>
            <span>{{ t('audit.findings.overdue') }}</span>
          </button>
        </div>

        <!-- Filters -->
        <div class="flex items-center gap-3 flex-wrap">
          <div class="relative flex-1 max-w-xs">
            <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
            </svg>
            <input v-model="findingSearch" type="text" :placeholder="t('audit.findings.search_placeholder')"
              class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
          </div>
          <select v-model="findingStatusFilter" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">{{ t('common.filter.all_statuses') }}</option>
            <option v-for="o in findingStatusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
          <select v-model="findingTypeFilter" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">{{ t('common.filter.all_types') }}</option>
            <option v-for="o in findingTypeAbbrOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
          <select v-model.number="findingAuditFilter" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option :value="0">{{ t('audit.findings.all_audits') }}</option>
            <option v-for="a in audits" :key="a.id" :value="a.id">{{ a.title }}</option>
          </select>
          <select v-model="findingOwnerFilter" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">{{ t('common.filter.all_owners') }}</option>
            <option v-for="m in orgMembers" :key="m.email" :value="m.email">{{ m.name || m.email }}</option>
          </select>
          <label class="flex items-center gap-1.5 text-xs text-slate-400">
            <input type="checkbox" v-model="findingOverdueOnly" class="rounded bg-slate-800 border-slate-700" />
            {{ t('audit.findings.overdue_only') }}
          </label>
          <div class="ml-auto text-xs text-slate-500 tabular-nums">{{ t('common.count.total', { count: findingTotal }) }}</div>
        </div>

        <!-- Table -->
        <div v-if="findings.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
          <div class="text-sm text-slate-500">{{ findingsHasFilter ? t('audit.findings.empty_filtered') : t('audit.findings.empty') }}</div>
        </div>
        <div v-else class="bg-slate-900 border border-slate-800 rounded-xl overflow-x-auto">
          <table class="w-full">
            <thead>
              <tr class="border-b border-slate-800">
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.findings.table.id') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.findings.table.title') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.findings.table.audit') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.findings.table.type') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.findings.table.owner') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.findings.table.due') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('audit.findings.table.status') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/50">
              <tr v-for="f in findings" :key="f.id"
                @click="selectFinding(f)"
                class="hover:bg-slate-800/50 transition-colors cursor-pointer">
                <td class="px-5 py-3.5 text-[10px] font-mono uppercase tracking-wider text-slate-600">{{ findingRef(f.id) }}</td>
                <td class="px-5 py-3.5">
                  <div class="text-sm font-medium text-slate-200">{{ f.title }}</div>
                  <div v-if="f.description" class="text-xs text-slate-500 mt-0.5 truncate max-w-md">{{ stripMd(f.description) }}</div>
                </td>
                <td class="px-5 py-3.5 text-sm text-slate-400 truncate max-w-xs">{{ f.audit_title || '-' }}</td>
                <td class="px-5 py-3.5">
                  <span class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-semibold rounded uppercase tracking-wider"
                    :class="findingTypeBadge(f.finding_type)">{{ findingTypeLabel(f.finding_type) }}</span>
                </td>
                <td class="px-5 py-3.5 text-sm text-slate-500 truncate max-w-[160px]">{{ resolveUserName(f.owner) }}</td>
                <td class="px-5 py-3.5 text-sm" :class="isOverdue(f.due_date) && f.status === 'open' ? 'text-red-400 font-medium' : 'text-slate-500'">
                  {{ f.due_date ? formatDay(f.due_date) : '-' }}
                </td>
                <td class="px-5 py-3.5"><StatusBadge :status="f.status" /></td>
              </tr>
            </tbody>
          </table>
          <Pagination :page="findingPage" :pageSize="findingPageSize" :total="findingTotal" @update:page="findingPage = $event" @update:pageSize="findingPageSize = $event" />
        </div>
      </template>
    </div>
    </div>

    <!-- ═══════════════════ CREATE PROGRAMME MODAL ═══════════════════ -->
    <Teleport to="body">
    <Transition name="modal">
    <div v-if="showCreateProgramme" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
      <div class="absolute inset-0 bg-black/60" @click="showCreateProgramme = false" />
      <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
        <div class="flex items-center justify-between mb-2">
          <h2 class="text-sm font-semibold text-slate-200">{{ t('audit.programme.create.heading') }}</h2>
          <button @click="showCreateProgramme = false" class="text-slate-500 hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <div class="space-y-3">
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.programme.create.title_label') }}</label>
            <input v-model="newProg.title" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('audit.programme.create.title_placeholder')" />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.programme.create.year_label') }}</label>
            <input v-model.number="newProg.year" type="number" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="2026" />
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">{{ t('audit.programme.create.fill_in_later') }}</div>
        <div class="flex justify-end gap-3 pt-3 border-t border-slate-800">
          <button @click="showCreateProgramme = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">{{ t('common.action.cancel') }}</button>
          <button @click="createProgramme" :disabled="!newProg.title.trim() || !newProg.year || progSaving"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
            {{ progSaving ? t('audit.programme.create.submitting') : t('audit.programme.create.submit') }}
          </button>
        </div>
      </div>
    </div>
    </Transition>
    </Teleport>

    <!-- ═══════════════════ CREATE AUDIT MODAL ═══════════════════ -->
    <Teleport to="body">
    <Transition name="modal">
    <div v-if="showCreateAudit" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
      <div class="absolute inset-0 bg-black/60" @click="showCreateAudit = false" />
      <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
        <div class="flex items-center justify-between mb-2">
          <h2 class="text-sm font-semibold text-slate-200">{{ t('audit.audit.create.heading') }}</h2>
          <button @click="showCreateAudit = false" class="text-slate-500 hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div class="sm:col-span-2">
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.audit.create.title_label') }}</label>
            <input v-model="newAudit.title" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('audit.audit.create.title_placeholder')" />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.audit.create.programme_label') }}</label>
            <select v-model.number="newAudit.programme_id" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option :value="0">{{ t('audit.audit.create.no_programme') }}</option>
              <option v-for="p in programmes" :key="p.id" :value="p.id">{{ p.title }} ({{ p.year }})</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.audit.create.type_label') }}</label>
            <select v-model="newAudit.audit_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option v-for="o in auditTypeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">{{ t('audit.audit.create.fill_in_later') }}</div>
        <div class="flex justify-end gap-3 pt-3 border-t border-slate-800">
          <button @click="showCreateAudit = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">{{ t('common.action.cancel') }}</button>
          <button @click="createAudit" :disabled="!newAudit.title.trim() || auditCreating"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
            {{ auditCreating ? t('audit.audit.create.submitting') : t('audit.audit.create.submit') }}
          </button>
        </div>
      </div>
    </div>
    </Transition>
    </Teleport>

    <!-- ═══════════════════ CREATE FINDING MODAL ═══════════════════ -->
    <Teleport to="body">
    <Transition name="modal">
    <div v-if="showCreateFinding" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
      <div class="absolute inset-0 bg-black/60" @click="showCreateFinding = false" />
      <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
        <div class="flex items-center justify-between mb-2">
          <h2 class="text-sm font-semibold text-slate-200">{{ t('audit.findings.create.heading') }}</h2>
          <button @click="showCreateFinding = false" class="text-slate-500 hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div class="sm:col-span-2">
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.findings.create.title_label') }}</label>
            <input v-model="newFinding.title" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('audit.findings.create.title_placeholder')" />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.findings.create.audit_label') }}</label>
            <select v-model.number="newFinding.audit_id" :disabled="newFindingAuditLocked" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50">
              <option :value="0">{{ t('audit.findings.create.audit_placeholder') }}</option>
              <option v-for="a in audits" :key="a.id" :value="a.id">{{ a.title }}</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.findings.create.type_label') }}</label>
            <select v-model="newFinding.finding_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option v-for="o in findingTypeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">{{ t('audit.findings.create.fill_in_later') }}</div>
        <div class="flex justify-end gap-3 pt-3 border-t border-slate-800">
          <button @click="showCreateFinding = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">{{ t('common.action.cancel') }}</button>
          <button @click="createFinding" :disabled="!newFinding.title.trim() || !newFinding.audit_id || findingCreating"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
            {{ findingCreating ? t('audit.findings.create.submitting') : t('audit.findings.create.submit') }}
          </button>
        </div>
      </div>
    </div>
    </Transition>
    </Teleport>

    <!-- ═══════════════════ PROGRAMME DETAIL MODAL ═══════════════════ -->
    <Teleport to="body">
    <Transition name="modal">
    <div v-if="selectedProgramme" class="fixed inset-0 z-40 flex items-start justify-center pt-[3vh] px-4">
      <div class="absolute inset-0 bg-black/60" @click="closeProgrammeDetail" />
      <div class="relative w-full max-w-4xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-h-[90vh] flex flex-col">
        <!-- Header -->
        <div class="flex-shrink-0 border-b border-slate-800 px-6 py-3 flex items-center justify-between gap-4">
          <div class="flex items-center gap-6 min-w-0">
            <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ progRef(selectedProgramme.id) }}</span>
            <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selectedProgramme.title }}</h2>
            <span class="text-xs text-slate-500 flex-shrink-0 tabular-nums">{{ selectedProgramme.year }}</span>
          </div>
          <div class="flex items-center gap-3 flex-shrink-0">
            <StatusBadge :status="selectedProgramme.status" />
            <button @click="closeProgrammeDetail" class="p-1 rounded-lg text-slate-600 hover:text-slate-300 hover:bg-slate-800 transition-colors">
              <svg class="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
          </div>
        </div>
        <!-- Body -->
        <div class="flex flex-1 min-h-0">
          <nav class="flex-shrink-0 w-28 border-r border-slate-800 py-3">
            <div class="space-y-0.5">
              <button v-for="tab in programmeDetailTabs" :key="tab.key" @click="switchProgrammeTab(tab.key)"
                class="w-full text-left px-3 py-2 text-xs font-medium transition-colors flex items-center justify-between"
                :class="programmeTab === tab.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                <span>{{ tab.label }}</span>
                <span v-if="tab.count !== undefined" class="text-[10px] text-slate-600">{{ tab.count }}</span>
              </button>
            </div>
          </nav>
          <div class="flex-1 overflow-y-auto min-h-0">
            <!-- Overview -->
            <template v-if="programmeTab === 'overview'">
              <div class="px-6 py-5 space-y-5">
                <div class="flex items-center justify-between">
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('audit.detail.overview') }}</div>
                  <button v-if="canWrite && !progEditing" @click="startProgEdit" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                </div>
                <template v-if="progEditing">
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.programme.field.title') }}</label>
                      <input v-model="progEditForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.programme.field.year') }}</label>
                      <input v-model.number="progEditForm.year" type="number" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.programme.field.status') }}</label>
                      <select v-model="progEditForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option v-for="o in programmeStatusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                      </select>
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.programme.field.description') }}</label>
                      <MarkdownField v-model="progEditForm.description" :self-type="'audit_programme'" :self-id="String(selectedProgramme.id)" :rows="3" :placeholder="t('audit.programme.placeholder.description')" />
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.programme.field.notes') }}</label>
                      <MarkdownField v-model="progEditForm.notes" :self-type="'audit_programme'" :self-id="String(selectedProgramme.id)" :rows="3" :placeholder="t('audit.programme.placeholder.notes')" />
                    </div>
                  </div>
                </template>
                <template v-else>
                  <div class="space-y-4">
                    <div>
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('audit.programme.field.description') }}</div>
                      <div v-if="selectedProgramme.description" class="text-sm text-slate-300 doc-prose" v-mermaid v-html="renderMd(selectedProgramme.description)"></div>
                      <div v-else class="text-sm text-slate-600">—</div>
                    </div>
                    <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.programme.field.year') }}</div>
                        <div class="text-sm text-slate-300 tabular-nums">{{ selectedProgramme.year }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.programme.field.status') }}</div>
                        <StatusBadge :status="selectedProgramme.status" />
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.programme.field.audits') }}</div>
                        <div class="text-sm text-slate-300 tabular-nums">{{ selectedProgramme.audit_count || 0 }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.programme.field.created_by') }}</div>
                        <div class="text-sm text-slate-300">{{ resolveUserName(selectedProgramme.created_by) }}</div>
                      </div>
                      <div v-if="selectedProgramme.created_at">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.programme.field.created') }}</div>
                        <div class="text-sm text-slate-300">{{ formatDate(selectedProgramme.created_at) }}</div>
                      </div>
                    </div>
                    <div v-if="selectedProgramme.notes" class="border-t border-slate-800 pt-4">
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('audit.programme.field.notes') }}</div>
                      <div class="text-sm text-slate-300 doc-prose" v-mermaid v-html="renderMd(selectedProgramme.notes)"></div>
                    </div>
                  </div>
                </template>
              </div>
            </template>

            <!-- Audits -->
            <template v-if="programmeTab === 'audits'">
              <div class="px-6 py-5 space-y-3">
                <div class="flex items-center justify-between">
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('audit.programme.audits_heading') }}</div>
                  <button v-if="canWrite" @click="openCreateAudit(selectedProgramme.id)" class="text-xs text-blue-400 hover:text-blue-300">{{ t('audit.programme.add_audit') }}</button>
                </div>
                <div v-if="programmeAudits(selectedProgramme.id).length === 0" class="text-xs text-slate-600 italic">{{ t('audit.programme.audits_empty') }}</div>
                <div v-else class="space-y-2">
                  <div v-for="a in programmeAudits(selectedProgramme.id)" :key="a.id"
                    @click="selectAuditFromProgramme(a)"
                    class="bg-slate-950 border border-slate-800 rounded-lg px-4 py-3 flex items-center gap-3 cursor-pointer hover:border-slate-700 transition-colors">
                    <span class="w-2 h-2 rounded-full flex-shrink-0" :class="auditTypeDot(a.audit_type)"></span>
                    <div class="flex-1 min-w-0">
                      <div class="text-sm font-medium text-slate-200 truncate">{{ a.title }}</div>
                      <div class="text-[11px] text-slate-500 truncate">
                        <span>{{ auditTypeLabel(a.audit_type) }}</span> {{ dot }}
                        {{ resolveUserName(a.auditor) || '—' }} {{ dot }}
                        {{ formatDateRange(a.planned_date, a.end_date) || t('audit.calendar.unscheduled_range') }}
                      </div>
                    </div>
                    <span v-if="a.open_findings > 0" class="text-[10px] px-1.5 py-0.5 rounded bg-red-900/40 text-red-400 font-medium">{{ t('audit.programme.open_findings', { count: a.open_findings }) }}</span>
                    <StatusBadge :status="a.status" />
                  </div>
                </div>
              </div>
            </template>

            <!-- Discussion -->
            <template v-if="programmeTab === 'discussion'">
              <div class="px-6 py-5">
                <CommentsPanel entityType="audit_programme" :entityId="String(selectedProgramme.id)" />
              </div>
            </template>

            <!-- History -->
            <template v-if="programmeTab === 'history'">
              <div class="px-6 py-5 space-y-6">
                <HistoryPanel entityType="audit_programme" :entityId="String(selectedProgramme.id)" />
                <div v-if="canWrite" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                  <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">{{ t('common.heading.danger_zone') }}</div>
                  <div class="text-xs text-slate-400">{{ t('audit.programme.danger.warning') }}</div>
                  <button @click="deleteSelectedProgramme" :disabled="(selectedProgramme.audit_count || 0) > 0"
                    class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 disabled:bg-slate-800 disabled:text-slate-600 disabled:cursor-not-allowed text-red-300 border border-red-800/50 rounded-lg transition-colors">
                    {{ t('audit.programme.danger.delete') }}
                  </button>
                </div>
              </div>
            </template>
          </div>
        </div>
        <!-- Footer (edit) -->
        <div v-if="progEditing" class="flex-shrink-0 border-t border-slate-800 px-6 py-3 flex justify-end gap-3">
          <button @click="cancelProgEdit" class="px-4 py-1.5 text-sm text-slate-400 hover:text-slate-200 transition-colors">{{ t('common.action.cancel') }}</button>
          <button @click="saveProgEdit" :disabled="progSaving" class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 text-white text-sm font-medium rounded-lg transition-colors">
            {{ progSaving ? t('common.state.saving') : t('common.action.save') }}
          </button>
        </div>
      </div>
    </div>
    </Transition>
    </Teleport>

    <!-- ═══════════════════ AUDIT DETAIL MODAL ═══════════════════ -->
    <Teleport to="body">
    <Transition name="modal">
    <div v-if="selectedAudit" class="fixed inset-0 z-40 flex items-start justify-center pt-[3vh] px-4">
      <div class="absolute inset-0 bg-black/60" @click="closeAuditDetail" />
      <div class="relative w-full max-w-5xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-h-[92vh] flex flex-col">
        <!-- Header -->
        <div class="flex-shrink-0 border-b border-slate-800 px-6 py-3 flex items-center justify-between gap-4">
          <div class="flex items-center gap-3 min-w-0">
            <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ auditRef(selectedAudit.id) }}</span>
            <span class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-semibold rounded uppercase tracking-wider whitespace-nowrap"
              :class="auditTypeBadge(selectedAudit.audit_type)">{{ auditTypeLabel(selectedAudit.audit_type) }}</span>
            <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selectedAudit.title }}</h2>
          </div>
          <div class="flex items-center gap-2 flex-shrink-0">
            <CopyLinkButton />
            <StatusBadge :status="selectedAudit.status" />
            <button @click="closeAuditDetail" class="p-1 rounded-lg text-slate-600 hover:text-slate-300 hover:bg-slate-800 transition-colors">
              <svg class="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
          </div>
        </div>
        <!-- Body -->
        <div class="flex flex-1 min-h-0">
          <nav class="flex-shrink-0 w-28 border-r border-slate-800 py-3">
            <div class="space-y-0.5">
              <button v-for="tab in auditDetailTabs" :key="tab.key" @click="switchAuditTab(tab.key)"
                class="w-full text-left px-3 py-2 text-xs font-medium transition-colors flex items-center justify-between"
                :class="auditTab === tab.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                <span>{{ tab.label }}</span>
                <span v-if="tab.count !== undefined" class="text-[10px] text-slate-600">{{ tab.count }}</span>
              </button>
            </div>
          </nav>
          <div class="flex-1 overflow-y-auto min-h-0">
            <div v-if="auditDetailLoading" class="px-6 py-10 text-center text-xs text-slate-500">{{ t('audit.audit.loading_detail') }}</div>
            <template v-else>

              <!-- Overview -->
              <template v-if="auditTab === 'overview'">
                <div class="px-6 py-5 space-y-5">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('audit.detail.overview') }}</div>
                    <button v-if="canWrite && !auditEditing" @click="startAuditEdit" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                  </div>
                  <template v-if="auditEditing">
                    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.audit.field.title') }}</label>
                        <input v-model="auditEditForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.audit.field.scope') }}</label>
                        <MarkdownField v-model="auditEditForm.scope" :self-type="'audit'" :self-id="String(selectedAudit.id)" :rows="3" :placeholder="t('audit.audit.placeholder.scope')" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.audit.field.type') }}</label>
                        <select v-model="auditEditForm.audit_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in auditTypeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.audit.field.status') }}</label>
                        <select v-model="auditEditForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in auditStatusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.audit.field.auditor') }}</label>
                        <MemberPicker v-model="auditEditForm.auditor" :members="orgMembers" :placeholder="t('audit.audit.placeholder.auditor')" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.audit.field.programme') }}</label>
                        <select v-model.number="auditEditForm.programme_id" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option :value="0">{{ t('audit.audit.create.no_programme') }}</option>
                          <option v-for="p in programmes" :key="p.id" :value="p.id">{{ p.title }} ({{ p.year }})</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.audit.field.planned_date') }}</label>
                        <input v-model="auditEditForm.planned_date" type="date" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.audit.field.end_date') }}</label>
                        <input v-model="auditEditForm.end_date" type="date" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.audit.field.notes') }}</label>
                        <MarkdownField v-model="auditEditForm.notes" :self-type="'audit'" :self-id="String(selectedAudit.id)" :rows="3" :placeholder="t('audit.audit.placeholder.notes')" />
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('audit.audit.field.scope') }}</div>
                        <div v-if="selectedAudit.scope" class="text-sm text-slate-300 doc-prose" v-mermaid v-html="renderMd(selectedAudit.scope)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>
                      <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.audit.field.programme') }}</div>
                          <div class="text-sm">
                            <button v-if="selectedAudit.programme_id" @click="goToProgrammeFromAudit(selectedAudit.programme_id)" class="text-blue-400 hover:text-blue-300">{{ programmeTitle(selectedAudit.programme_id) }}</button>
                            <span v-else class="text-slate-600">—</span>
                          </div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.audit.field.auditor') }}</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedAudit.auditor) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.audit.field.planned') }}</div>
                          <div class="text-sm text-slate-300">{{ formatDateRange(selectedAudit.planned_date, selectedAudit.end_date) || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.audit.field.type') }}</div>
                          <div class="text-sm text-slate-300">{{ auditTypeLabel(selectedAudit.audit_type) }}</div>
                        </div>
                        <div v-if="selectedAudit.started_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.audit.field.started') }}</div>
                          <div class="text-sm text-slate-300">{{ formatDateTime(selectedAudit.started_at) }}</div>
                        </div>
                        <div v-if="selectedAudit.completed_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.audit.field.completed') }}</div>
                          <div class="text-sm text-emerald-400">{{ formatDateTime(selectedAudit.completed_at) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.audit.field.items') }}</div>
                          <div class="text-sm text-slate-300 tabular-nums">{{ selectedAudit.item_count || 0 }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.audit.field.findings') }}</div>
                          <div class="text-sm tabular-nums" :class="(selectedAudit.open_findings || 0) > 0 ? 'text-red-400' : 'text-slate-300'">
                            {{ selectedAudit.finding_count || 0 }}<span v-if="(selectedAudit.open_findings || 0) > 0" class="text-slate-500"> {{ t('audit.audit.open_count', { count: selectedAudit.open_findings }) }}</span>
                          </div>
                        </div>
                      </div>
                      <div v-if="selectedAudit.notes" class="border-t border-slate-800 pt-4">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('audit.audit.field.notes') }}</div>
                        <div class="text-sm text-slate-300 doc-prose" v-mermaid v-html="renderMd(selectedAudit.notes)"></div>
                      </div>
                    </div>
                  </template>
                </div>
              </template>

              <!-- Items (work surface) -->
              <template v-if="auditTab === 'items'">
                <div class="px-6 py-5 space-y-3">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('audit.items.heading', { count: auditItems.length }) }}</div>
                    <button v-if="canWrite" @click="showItemPicker = !showItemPicker" class="text-xs text-blue-400 hover:text-blue-300">
                      {{ showItemPicker ? t('common.action.cancel') : t('audit.items.add') }}
                    </button>
                  </div>

                  <!-- Item picker (universal search) -->
                  <div v-if="showItemPicker" class="bg-slate-950 border border-slate-700 rounded-lg p-3 space-y-2">
                    <input v-model="itemSearchQuery" type="text" ref="itemSearchInput"
                      class="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500"
                      :placeholder="t('audit.items.search_placeholder')"
                      @input="doItemSearch"
                      @focus="doItemSearch"
                      @keydown.down.prevent="itemSelectedIdx = Math.min(itemSelectedIdx + 1, itemSearchResults.length - 1)"
                      @keydown.up.prevent="itemSelectedIdx = Math.max(itemSelectedIdx - 1, 0)"
                      @keydown.tab.prevent="itemSearchResults.length && pickAuditItem(itemSearchResults[itemSelectedIdx])"
                      @keydown.enter.prevent="itemSearchResults.length && pickAuditItem(itemSearchResults[itemSelectedIdx])"
                      @keydown.escape="showItemPicker = false" />
                    <div v-if="itemSearching" class="text-[10px] text-slate-500 italic">{{ t('audit.items.searching') }}</div>
                    <div v-else-if="itemSearchResults.length > 0" class="max-h-56 overflow-y-auto space-y-0.5 bg-slate-900 border border-slate-800 rounded">
                      <button v-for="(s, i) in itemSearchResults" :key="s.type + ':' + s.id"
                        @click="pickAuditItem(s)"
                        class="w-full text-left px-3 py-1.5 text-xs transition-colors flex items-center gap-2"
                        :class="i === itemSelectedIdx ? 'bg-blue-600/30 text-white' : 'text-slate-300 hover:bg-slate-800'">
                        <span class="px-1 py-0.5 rounded text-[9px] font-semibold flex-shrink-0 uppercase"
                          :class="entityTypeBadge(s.type)">{{ entityTypeShort(s.type) }}</span>
                        <span class="text-slate-500 font-mono text-[10px] flex-shrink-0">{{ s.id }}</span>
                        <span class="truncate">{{ s.title }}</span>
                      </button>
                    </div>
                    <div v-else-if="itemSearched" class="text-[10px] text-slate-500 italic">{{ t('audit.items.no_matches') }}</div>
                  </div>

                  <!-- Items list -->
                  <div v-if="auditItems.length === 0 && !showItemPicker" class="bg-slate-950 border border-slate-800 rounded-lg p-8 text-center text-xs text-slate-600 italic">
                    {{ t('audit.items.empty') }}
                  </div>
                  <div v-else-if="auditItems.length > 0" class="space-y-2">
                    <div v-for="item in auditItems" :key="item.id"
                      class="bg-slate-950 border rounded-lg p-3"
                      :class="resultRowClass(item.result)">
                      <!-- Top row: title + linked badge + result + actions -->
                      <div class="flex items-start gap-2 flex-wrap">
                        <span class="px-1 py-0.5 rounded text-[9px] font-semibold flex-shrink-0 uppercase mt-0.5"
                          :class="entityTypeBadge(item.item_type)">{{ entityTypeShort(item.item_type) }}</span>
                        <span class="text-[10px] font-mono text-slate-500 flex-shrink-0 mt-1">{{ item.item_id }}</span>
                        <span class="text-sm font-medium text-slate-200 flex-1 min-w-0 truncate mt-0.5">{{ item.title }}</span>
                        <select v-if="canWrite" v-model="item.result" @change="saveItemField(item, 'result', item.result)"
                          class="bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500"
                          :class="resultSelectClass(item.result)">
                          <option v-for="o in resultOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                        <span v-else class="text-xs"><StatusBadge :status="item.result || 'not_assessed'" group="audit_result" /></span>
                        <button v-if="canWrite && (item.result === 'minor_nc' || item.result === 'major_nc')"
                          @click="raiseFindingFromItem(item)"
                          class="text-[10px] text-amber-400 hover:text-amber-300 px-2 py-1 rounded border border-amber-800/50 bg-amber-900/20"
                          :title="t('audit.items.raise_finding_title')">
                          {{ t('audit.items.raise_finding') }}
                        </button>
                        <button v-if="canWrite" @click="deleteItem(item)"
                          class="text-slate-600 hover:text-red-400 p-1 transition-colors"
                          :title="t('audit.items.delete_title')">
                          <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                            <path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6M1 7h22M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                          </svg>
                        </button>
                      </div>

                      <!-- Evidence -->
                      <div class="mt-2 flex items-center gap-2">
                        <span class="text-[10px] text-slate-500 uppercase tracking-wider whitespace-nowrap">{{ t('audit.items.evidence') }}</span>
                        <input v-if="canWrite" v-model="item.evidence" @change="saveItemField(item, 'evidence', item.evidence)"
                          type="text" :placeholder="t('audit.items.evidence_placeholder')"
                          class="flex-1 bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                        <span v-else class="flex-1 text-xs text-slate-500">{{ item.evidence || '—' }}</span>
                      </div>

                      <!-- Notes (collapsible) -->
                      <div class="mt-1.5">
                        <button @click="toggleItemNotes(item.id)" class="text-[10px] text-slate-500 hover:text-slate-300 transition-colors">
                          {{ expandedItemNotes.has(item.id) ? '▼' : '▶' }} {{ item.notes ? t('audit.items.notes') : t('audit.items.notes_empty') }}
                        </button>
                        <div v-if="expandedItemNotes.has(item.id)" class="mt-2">
                          <MarkdownField v-if="canWrite" v-model="item.notes" :self-type="'audit'" :self-id="String(selectedAudit.id)" :rows="3" :placeholder="t('audit.items.notes_placeholder')" />
                          <div v-else class="text-xs text-slate-400 doc-prose" v-mermaid v-html="renderMd(item.notes || '—')"></div>
                          <div v-if="canWrite" class="flex justify-end mt-1.5">
                            <button @click="saveItemField(item, 'notes', item.notes)" class="text-[10px] text-blue-400 hover:text-blue-300">{{ t('audit.items.save_notes') }}</button>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </template>

              <!-- Findings (within audit) -->
              <template v-if="auditTab === 'findings'">
                <div class="px-6 py-5 space-y-3">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('audit.findings.heading_count', { count: auditFindings.length }) }}</div>
                    <button v-if="canWrite" @click="openCreateFinding(selectedAudit.id)" class="text-xs text-blue-400 hover:text-blue-300">{{ t('audit.findings.add') }}</button>
                  </div>
                  <div v-if="auditFindings.length === 0" class="text-xs text-slate-600 italic">{{ t('audit.findings.audit_empty') }}</div>
                  <div v-else class="space-y-2">
                    <div v-for="f in auditFindings" :key="f.id"
                      @click="selectFinding(f)"
                      class="bg-slate-950 border rounded-lg px-4 py-3 cursor-pointer hover:border-slate-700 transition-colors"
                      :class="f.status === 'open' ? 'border-red-900/40' : 'border-slate-800'">
                      <div class="flex items-center gap-2 flex-wrap">
                        <span class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-semibold rounded uppercase tracking-wider whitespace-nowrap"
                          :class="findingTypeBadge(f.finding_type)">{{ findingTypeLabel(f.finding_type) }}</span>
                        <span class="text-sm font-medium text-slate-200 flex-1 truncate min-w-0">{{ f.title }}</span>
                        <span v-if="f.audit_item_id" class="text-[10px] text-slate-500 font-mono">{{ t('audit.findings.item_ref', { id: f.audit_item_id }) }}</span>
                        <span v-if="isOverdue(f.due_date) && f.status === 'open'" class="text-[10px] px-1.5 py-0.5 rounded bg-red-900/60 text-red-300 font-semibold tracking-wider uppercase">{{ t('audit.findings.overdue') }}</span>
                        <span v-if="f.due_date" class="text-xs text-slate-500">{{ formatDay(f.due_date) }}</span>
                        <StatusBadge :status="f.status" />
                      </div>
                    </div>
                  </div>
                </div>
              </template>

              <!-- Report -->
              <template v-if="auditTab === 'report'">
                <div class="px-6 py-5 space-y-4">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('audit.report.heading') }}</div>
                    <button v-if="canWrite && !reportEditing" @click="startReportEdit" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                  </div>
                  <template v-if="reportEditing">
                    <MarkdownField v-model="reportForm.summary" :self-type="'audit'" :self-id="String(selectedAudit.id)" :rows="20" :placeholder="t('audit.report.placeholder')" />
                  </template>
                  <template v-else>
                    <div v-if="selectedAudit.summary" class="text-sm text-slate-300 doc-prose leading-relaxed" v-mermaid v-html="renderMd(selectedAudit.summary)"></div>
                    <div v-else class="text-sm text-slate-600 italic">{{ t('audit.report.empty') }}</div>
                  </template>
                </div>
              </template>

              <!-- Links -->
              <template v-if="auditTab === 'links'">
                <div class="px-6 py-5">
                  <ReferenceManager entityType="audit" :entityId="String(selectedAudit.id)" :editable="canWrite" />
                </div>
              </template>

              <!-- Discussion -->
              <template v-if="auditTab === 'discussion'">
                <div class="px-6 py-5">
                  <CommentsPanel entityType="audit" :entityId="String(selectedAudit.id)" />
                </div>
              </template>

              <!-- History -->
              <template v-if="auditTab === 'history'">
                <div class="px-6 py-5 space-y-6">
                  <HistoryPanel entityType="audit" :entityId="String(selectedAudit.id)" />
                  <div v-if="canWrite" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">{{ t('common.heading.danger_zone') }}</div>
                    <div class="text-xs text-slate-400">{{ t('audit.audit.danger.warning') }}</div>
                  </div>
                </div>
              </template>
            </template>
          </div>
        </div>
        <!-- Footer (edit) -->
        <div v-if="auditEditing || reportEditing" class="flex-shrink-0 border-t border-slate-800 px-6 py-3 flex justify-end gap-3">
          <button @click="auditEditing ? cancelAuditEdit() : cancelReportEdit()" class="px-4 py-1.5 text-sm text-slate-400 hover:text-slate-200 transition-colors">{{ t('common.action.cancel') }}</button>
          <button @click="auditEditing ? saveAuditEdit() : saveReportEdit()" :disabled="auditSaving"
            class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 text-white text-sm font-medium rounded-lg transition-colors">
            {{ auditSaving ? t('common.state.saving') : t('common.action.save') }}
          </button>
        </div>
      </div>
    </div>
    </Transition>
    </Teleport>

    <!-- ═══════════════════ FINDING DETAIL MODAL ═══════════════════ -->
    <Teleport to="body">
    <Transition name="modal">
    <div v-if="selectedFinding" class="fixed inset-0 z-50 flex items-start justify-center pt-[3vh] px-4">
      <div class="absolute inset-0 bg-black/60" @click="closeFindingDetail" />
      <div class="relative w-full max-w-4xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-h-[90vh] flex flex-col">
        <!-- Header -->
        <div class="flex-shrink-0 border-b border-slate-800 px-6 py-3 flex items-center justify-between gap-4">
          <div class="flex items-center gap-3 min-w-0">
            <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ findingRef(selectedFinding.id) }}</span>
            <span class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-semibold rounded uppercase tracking-wider whitespace-nowrap"
              :class="findingTypeBadge(selectedFinding.finding_type)">{{ findingTypeLabel(selectedFinding.finding_type) }}</span>
            <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selectedFinding.title }}</h2>
          </div>
          <div class="flex items-center gap-2 flex-shrink-0">
            <CopyLinkButton />
            <StatusBadge :status="selectedFinding.status" />
            <button @click="closeFindingDetail" class="p-1 rounded-lg text-slate-600 hover:text-slate-300 hover:bg-slate-800 transition-colors">
              <svg class="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
          </div>
        </div>
        <div class="flex flex-1 min-h-0">
          <nav class="flex-shrink-0 w-32 border-r border-slate-800 py-3">
            <div class="space-y-0.5">
              <button v-for="tab in findingDetailTabs" :key="tab.key" @click="switchFindingTab(tab.key)"
                class="w-full text-left px-3 py-2 text-xs font-medium transition-colors"
                :class="findingTab === tab.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                {{ tab.label }}
              </button>
            </div>
          </nav>
          <div class="flex-1 overflow-y-auto min-h-0">
            <!-- Overview -->
            <template v-if="findingTab === 'overview'">
              <div class="px-6 py-5 space-y-5">
                <div class="flex items-center justify-between">
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('audit.detail.overview') }}</div>
                  <button v-if="canWrite && !findingEditing" @click="startFindingEdit" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                </div>
                <template v-if="findingEditing">
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.findings.field.title') }}</label>
                      <input v-model="findingEditForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.findings.field.description') }}</label>
                      <MarkdownField v-model="findingEditForm.description" :self-type="'audit_finding'" :self-id="String(selectedFinding.id)" :rows="6" :placeholder="t('audit.findings.placeholder.description')" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.findings.field.type') }}</label>
                      <div class="flex flex-wrap gap-1.5">
                        <button v-for="type in findingTypeKeys" :key="type"
                          @click="findingEditForm.finding_type = type"
                          class="px-2.5 py-1 text-[11px] font-medium rounded-lg border transition-colors"
                          :class="findingEditForm.finding_type === type ? findingTypeChipActive(type) : 'bg-slate-800 text-slate-400 border-slate-700 hover:border-slate-600'">
                          {{ findingTypeLabel(type) }}
                        </button>
                      </div>
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.findings.field.status') }}</label>
                      <select v-model="findingEditForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option v-for="o in findingStatusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                      </select>
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.findings.field.owner') }}</label>
                      <MemberPicker v-model="findingEditForm.owner" :members="orgMembers" :placeholder="t('audit.findings.placeholder.owner')" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('audit.findings.field.due_date') }}</label>
                      <input v-model="findingEditForm.due_date" type="date" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                  </div>
                </template>
                <template v-else>
                  <div class="space-y-4">
                    <div>
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{{ t('audit.findings.field.description') }}</div>
                      <div v-if="selectedFinding.description" class="text-sm text-slate-300 doc-prose leading-relaxed" v-mermaid v-html="renderMd(selectedFinding.description)"></div>
                      <div v-else class="text-sm text-slate-600">—</div>
                    </div>
                    <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.findings.field.audit') }}</div>
                        <button v-if="selectedFinding.audit_id" @click="goToAuditFromFinding(selectedFinding.audit_id)" class="text-sm text-blue-400 hover:text-blue-300 truncate text-left">{{ selectedFinding.audit_title || auditRef(selectedFinding.audit_id) }}</button>
                        <span v-else class="text-sm text-slate-600">—</span>
                      </div>
                      <div v-if="selectedFinding.audit_item_id">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.findings.field.audit_item') }}</div>
                        <div class="text-sm text-slate-300 font-mono">{{ t('audit.findings.item_ref', { id: selectedFinding.audit_item_id }) }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.findings.field.type') }}</div>
                        <span class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-semibold rounded uppercase tracking-wider"
                          :class="findingTypeBadge(selectedFinding.finding_type)">{{ findingTypeLabel(selectedFinding.finding_type) }}</span>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.findings.field.status') }}</div>
                        <StatusBadge :status="selectedFinding.status" />
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.findings.field.owner') }}</div>
                        <div class="text-sm text-slate-300">{{ resolveUserName(selectedFinding.owner) }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.findings.field.due_date_read') }}</div>
                        <div class="text-sm" :class="isOverdue(selectedFinding.due_date) && selectedFinding.status === 'open' ? 'text-red-400 font-medium' : 'text-slate-300'">
                          {{ selectedFinding.due_date ? formatDay(selectedFinding.due_date) : '—' }}
                        </div>
                      </div>
                      <div v-if="selectedFinding.created_at">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.findings.field.created') }}</div>
                        <div class="text-sm text-slate-300">{{ formatDate(selectedFinding.created_at) }}</div>
                      </div>
                      <div v-if="selectedFinding.closed_at">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('audit.findings.field.closed') }}</div>
                        <div class="text-sm text-emerald-400">
                          <i18n-t keypath="common.detail.at_by" scope="global">
                            <template #datetime>{{ formatDateTime(selectedFinding.closed_at) }}</template>
                            <template #by>
                              <span v-if="selectedFinding.closed_by" class="text-slate-500">{{ t('common.detail.by', { name: resolveUserName(selectedFinding.closed_by) }) }}</span>
                            </template>
                          </i18n-t>
                        </div>
                      </div>
                    </div>
                  </div>
                </template>
              </div>
            </template>

            <!-- Linked CAs -->
            <template v-if="findingTab === 'linked'">
              <div class="px-6 py-5 space-y-4">
                <ReferenceManager entityType="audit_finding" :entityId="String(selectedFinding.id)" :editable="canWrite" />
                <div v-if="canWrite" class="bg-slate-950 border border-slate-800 rounded-lg p-4">
                  <div class="text-[11px] font-semibold text-slate-400 uppercase tracking-wider mb-2">{{ t('audit.findings.quick_action') }}</div>
                  <button @click="createCAFromFinding(selectedFinding)"
                    class="flex items-center justify-between w-full gap-3 px-4 py-2.5 rounded-lg bg-slate-900 hover:bg-slate-800 border border-slate-700 hover:border-slate-600 transition-colors text-left">
                    <div>
                      <div class="text-sm font-medium text-slate-200">{{ t('audit.findings.create_ca') }}</div>
                      <div class="text-xs text-slate-500 mt-0.5">{{ t('audit.findings.create_ca_hint') }}</div>
                    </div>
                    <span class="text-slate-500 text-lg">→</span>
                  </button>
                </div>
              </div>
            </template>

            <!-- Discussion -->
            <template v-if="findingTab === 'discussion'">
              <div class="px-6 py-5">
                <CommentsPanel entityType="audit_finding" :entityId="String(selectedFinding.id)" />
              </div>
            </template>

            <!-- History -->
            <template v-if="findingTab === 'history'">
              <div class="px-6 py-5 space-y-6">
                <HistoryPanel entityType="audit_finding" :entityId="String(selectedFinding.id)" />
                <div v-if="canWrite" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                  <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">{{ t('common.heading.danger_zone') }}</div>
                  <div class="text-xs text-slate-400">{{ t('audit.findings.danger.warning') }}</div>
                  <button @click="deleteSelectedFinding"
                    class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                    {{ t('audit.findings.danger.delete') }}
                  </button>
                </div>
              </div>
            </template>
          </div>
        </div>
        <!-- Footer (edit) -->
        <div v-if="findingEditing" class="flex-shrink-0 border-t border-slate-800 px-6 py-3 flex justify-end gap-3">
          <button @click="cancelFindingEdit" class="px-4 py-1.5 text-sm text-slate-400 hover:text-slate-200 transition-colors">{{ t('common.action.cancel') }}</button>
          <button @click="saveFindingEdit" :disabled="findingSaving"
            class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 text-white text-sm font-medium rounded-lg transition-colors">
            {{ findingSaving ? t('common.state.saving') : t('common.action.save') }}
          </button>
        </div>
      </div>
    </div>
    </Transition>
    </Teleport>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import CopyLinkButton from '../components/CopyLinkButton.vue'
import StatStrip from '../components/StatStrip.vue'
import RefreshButton from '../components/RefreshButton.vue'
import MemberPicker from '../components/MemberPicker.vue'
import MarkdownField from '../components/MarkdownField.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
import CommentsPanel from '../components/CommentsPanel.vue'
import HistoryPanel from '../components/HistoryPanel.vue'
import SuggestNewButton from '../components/SuggestNewButton.vue'
import Pagination from '../components/Pagination.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import { renderMarkdown } from '../composables/useRenderMd.js'
import { useModalEscape } from '../composables/useModalEscape.js'
import { useConfirm } from '../composables/useConfirm.js'
import { useToast } from '../composables/useToast.js'
import { useDirtyEdit } from '../composables/useDirtyEdit.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import { formatDate, formatDay, formatMonthLong } from '../composables/useFormat.js'
import { renderApiError } from '../composables/useApiError.js'
import { enumLabel, enumLabelAbbr, entityLabel } from '../composables/useEnumLabel.js'

const { confirm: confirmDialog } = useConfirm()
const { show: showError, success: showSaved } = useToast()
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { orgPath } = useCurrentOrg()

// ═════════════════════ Constants ═════════════════════
const auditTypeKeys = ['internal', 'external', 'surveillance', 'certification', 'recertification']
const findingTypeKeys = ['major_nc', 'minor_nc', 'observation', 'opportunity']

// Tab labels are keys, resolved in computeds below: an array built at module
// load freezes its labels in whatever locale was active when the file was
// imported.
const TOP_TAB_KEYS = [
  { key: 'calendar', label: 'audit.tab.calendar' },
  { key: 'programmes', label: 'audit.tab.programmes' },
  { key: 'findings', label: 'audit.tab.findings' },
]

const PROGRAMME_TAB_KEYS = [
  { key: 'overview', label: 'common.tab.overview' },
  { key: 'audits', label: 'audit.tab.audits' },
  { key: 'discussion', label: 'audit.tab.discussion' },
  { key: 'history', label: 'common.tab.history' },
]

const AUDIT_TAB_KEYS = [
  { key: 'overview', label: 'common.tab.overview' },
  { key: 'items', label: 'audit.tab.items' },
  { key: 'findings', label: 'audit.tab.findings' },
  { key: 'report', label: 'audit.tab.report' },
  { key: 'links', label: 'common.tab.links' },
  { key: 'discussion', label: 'audit.tab.discussion' },
  { key: 'history', label: 'common.tab.history' },
]

const FINDING_TAB_KEYS = [
  { key: 'overview', label: 'common.tab.overview' },
  { key: 'linked', label: 'audit.tab.linked' },
  { key: 'discussion', label: 'audit.tab.discussion' },
  { key: 'history', label: 'common.tab.history' },
]

const topTabs = computed(() => TOP_TAB_KEYS.map((tab) => ({ ...tab, label: t(tab.label) })))
const findingDetailTabs = computed(() => FINDING_TAB_KEYS.map((tab) => ({ ...tab, label: t(tab.label) })))

// ═════════════════════ State ═════════════════════
const userRole = ref('')
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')

const loading = ref(true)
const refreshing = ref(false)
async function reload() {
  refreshing.value = true
  error.value = null
  try {
    await Promise.all([loadProgrammes(), loadAudits()])
    await refreshFindingCounts()
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    refreshing.value = false
  }
}
const error = ref(null)
const orgMembers = ref([])

const programmes = ref([])
const audits = ref([])

const activeTab = ref('calendar')

// Programmes filters
const programmeSearch = ref('')
const programmeYearFilter = ref(0)
const programmeStatusFilter = ref('')

// Findings (server-paginated)
const findings = ref([])
const findingTotal = ref(0)
const openFindingsTotal = ref(0)
const closedFindingsTotal = ref(0)
const overdueFindingsTotal = ref(0)
const findingPage = ref(1)
const findingPageSize = ref(50)
const findingSearch = ref('')
const findingStatusFilter = ref('')
const findingTypeFilter = ref('')
const findingAuditFilter = ref(0)
const findingOwnerFilter = ref('')
const findingOverdueOnly = ref(false)

const findingsHasFilter = computed(() =>
  !!findingSearch.value || !!findingStatusFilter.value || !!findingTypeFilter.value ||
  findingAuditFilter.value > 0 || !!findingOwnerFilter.value || findingOverdueOnly.value)

// Calendar
const calendarYear = ref(new Date().getFullYear())
const calendarMonths = ref([])
const calendarUnscheduled = ref([])
const calendarLoading = ref(false)

// Create form modals
const showCreateProgramme = ref(false)
const showCreateAudit = ref(false)
const showCreateFinding = ref(false)
const newProg = ref({ title: '', year: new Date().getFullYear() })
const newAudit = ref({ title: '', programme_id: 0, audit_type: 'internal' })
const newFinding = ref({ title: '', audit_id: 0, finding_type: 'minor_nc' })
const newFindingAuditLocked = ref(false)
let newFindingExtra = {}
const progSaving = ref(false)
const auditCreating = ref(false)
const findingCreating = ref(false)
useModalEscape(showCreateProgramme)
useModalEscape(showCreateAudit)
useModalEscape(showCreateFinding)

// Programme detail
const selectedProgramme = ref(null)
const programmeTab = ref('overview')
const progEditing = ref(false)
const progEditForm = ref({})
const { capture: captureProgEdit, isDirty: progIsDirty } = useDirtyEdit(progEditForm)
useModalEscape(computed(() => !!selectedProgramme.value), () => closeProgrammeDetail())

// Audit detail
const selectedAudit = ref(null)
const auditTab = ref('overview')
const auditDetailLoading = ref(false)
const auditItems = ref([])
const auditFindings = ref([])
const auditEditing = ref(false)
const auditEditForm = ref({})
const auditSaving = ref(false)
const reportEditing = ref(false)
const reportForm = ref({ summary: '' })
const { capture: captureAuditEdit, isDirty: auditIsDirty } = useDirtyEdit(auditEditForm)
const { capture: captureReportEdit, isDirty: reportIsDirty } = useDirtyEdit(reportForm)
useModalEscape(computed(() => !!selectedAudit.value), () => closeAuditDetail())

// Item picker (universal search)
const showItemPicker = ref(false)
const itemSearchQuery = ref('')
const itemSearchResults = ref([])
const itemSearching = ref(false)
const itemSearched = ref(false)
const itemSelectedIdx = ref(0)
const itemSearchInput = ref(null)
let itemSearchTimer = null
const expandedItemNotes = reactive(new Set())

// Finding detail
const selectedFinding = ref(null)
const findingTab = ref('overview')
const findingEditing = ref(false)
const findingEditForm = ref({})
const findingSaving = ref(false)
const { capture: captureFindingEdit, isDirty: findingIsDirty } = useDirtyEdit(findingEditForm)
useModalEscape(computed(() => !!selectedFinding.value), () => closeFindingDetail())

// ═════════════════════ Computed ═════════════════════
const programmeYears = computed(() => {
  const ys = new Set()
  programmes.value.forEach(p => p.year && ys.add(p.year))
  return Array.from(ys).sort((a, b) => b - a)
})

const programmeStats = computed(() => {
  const s = { active: 0, closed: 0, draft: 0 }
  programmes.value.forEach(p => { if (s[p.status] !== undefined) s[p.status]++ })
  return s
})

const programmeStatusStats = computed(() => [
  { key: '', label: t('common.stat.total'), count: programmes.value.length, color: 'text-slate-100' },
  { key: 'active', label: statusLabel('active'), count: programmeStats.value.active, color: 'text-emerald-400' },
  { key: 'closed', label: statusLabel('closed'), count: programmeStats.value.closed, color: 'text-slate-400' },
  { key: 'draft', label: statusLabel('draft'), count: programmeStats.value.draft, color: 'text-amber-400' },
])

// Overdue is a boolean toggle (separate dimension), so it rides alongside the
// status strip as its own chip rather than inside StatStrip's single v-model.
const findingStatusStats = computed(() => [
  { key: '', label: t('common.stat.total'), count: findingTotal.value, color: 'text-slate-100' },
  { key: 'open', label: statusLabel('open'), count: openFindingsTotal.value, color: openFindingsTotal.value > 0 ? 'text-red-400' : 'text-slate-100' },
  { key: 'closed', label: statusLabel('closed'), count: closedFindingsTotal.value, color: 'text-emerald-400' },
])

const filteredProgrammes = computed(() => {
  const q = programmeSearch.value.toLowerCase().trim()
  return programmes.value.filter(p => {
    if (q && !(p.title || '').toLowerCase().includes(q) && !(p.description || '').toLowerCase().includes(q)) return false
    if (programmeYearFilter.value && p.year !== programmeYearFilter.value) return false
    if (programmeStatusFilter.value && p.status !== programmeStatusFilter.value) return false
    return true
  })
})

const programmeDetailTabs = computed(() => PROGRAMME_TAB_KEYS.map((tab) => {
  const resolved = { ...tab, label: t(tab.label) }
  if (tab.key === 'audits' && selectedProgramme.value) {
    resolved.count = programmeAudits(selectedProgramme.value.id).length
  }
  return resolved
}))

const auditDetailTabs = computed(() => AUDIT_TAB_KEYS.map((tab) => {
  const resolved = { ...tab, label: t(tab.label) }
  if (tab.key === 'items') resolved.count = auditItems.value.length
  if (tab.key === 'findings') resolved.count = auditFindings.value.length
  return resolved
}))

// ═════════════════════ Helpers ═════════════════════
// orgPath is provided by useCurrentOrg() above.

function resolveUserName(email) {
  if (!email) return '—'
  const u = orgMembers.value.find(m => m.email === email)
  return u?.name || email
}

const renderMd = renderMarkdown

function stripMd(text) {
  if (!text) return ''
  return String(text).replace(/[#*_`>\[\]]/g, '').replace(/\s+/g, ' ').trim().slice(0, 200)
}

const formatDateTime = (d) => formatDate(d, 'datetime')

function formatDateRange(planned, end) {
  const s = formatDay(planned, 'dayMonth')
  const e = formatDay(end, 'dayMonth')
  if (s && e) return `${s} – ${e}`
  return s
}

function isOverdue(dateStr) {
  if (!dateStr && dateStr !== 0) return false
  const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
  return d < new Date()
}

function programmeAudits(progId) {
  return audits.value.filter(a => a.programme_id === progId)
}

function programmeOpenFindings(progId) {
  return programmeAudits(progId).reduce((sum, a) => sum + (a.open_findings || 0), 0)
}

function programmeTitle(progId) {
  if (!progId) return '—'
  const p = programmes.value.find(x => x.id === progId)
  return p ? `${p.title} (${p.year})` : `#${progId}`
}

// Lookups and option lists live here rather than in the template: a group name
// — and a bare enum value — is a stored identifier, and the raw-text scanner
// reads a quoted word in a mustache as unextracted copy.
const PROGRAMME_STATUSES = ['draft', 'active', 'closed']
const AUDIT_STATUSES = ['planned', 'in_progress', 'completed']
const FINDING_STATUSES = ['open', 'closed']
const AUDIT_RESULTS = ['not_assessed', 'conforming', 'minor_nc', 'major_nc', 'observation', 'opportunity']

const statusLabel = (v) => enumLabel('status', v)
const auditTypeLabel = (v) => enumLabel('audit_type', v)
// The badge, the filter and the type chips are all narrow: they take the
// abbreviated form. The create-finding dropdown has room for the full one.
const findingTypeLabel = (v) => enumLabelAbbr('finding_type', v)

const options = (values, label) => computed(() => values.map((value) => ({ value, label: label(value) })))
const auditTypeOptions = options(auditTypeKeys, auditTypeLabel)
const programmeStatusOptions = options(PROGRAMME_STATUSES, statusLabel)
const auditStatusOptions = options(AUDIT_STATUSES, statusLabel)
const findingStatusOptions = options(FINDING_STATUSES, statusLabel)
const findingTypeOptions = options(findingTypeKeys, (v) => enumLabel('finding_type', v))
const findingTypeAbbrOptions = options(findingTypeKeys, findingTypeLabel)
const resultOptions = options(AUDIT_RESULTS, (v) => enumLabelAbbr('audit_result', v))

const dot = computed(() => t('common.separator.dot'))

// Entity identifiers are not copy — building them here keeps the prefix out of
// the template, where the raw-text scanner reads it as an unextracted word.
const progRef = (id) => `PROG-${id}`
const auditRef = (id) => `AUDIT-${id}`
const findingRef = (id) => `FIND-${id}`

// The calendar API sends an English month name alongside the month number.
// Render the number through Intl instead, and keep the server's name only as
// a fallback if it ever sends something unexpected.
function monthName(month) {
  return formatMonthLong(Number(month?.month) - 1) || month?.name || ''
}

function findingTypeBadge(type) {
  switch (type) {
    case 'major_nc': return 'bg-red-900/60 text-red-300 border border-red-800'
    case 'minor_nc': return 'bg-amber-900/60 text-amber-300 border border-amber-800'
    case 'observation': return 'bg-blue-900/60 text-blue-300 border border-blue-800'
    case 'opportunity': return 'bg-emerald-900/60 text-emerald-300 border border-emerald-800'
    default: return 'bg-slate-800 text-slate-400 border border-slate-700'
  }
}

function findingTypeChipActive(type) {
  switch (type) {
    case 'major_nc': return 'bg-red-600/20 text-red-400 border-red-500/40'
    case 'minor_nc': return 'bg-amber-600/20 text-amber-400 border-amber-500/40'
    case 'observation': return 'bg-blue-600/20 text-blue-400 border-blue-500/40'
    case 'opportunity': return 'bg-emerald-600/20 text-emerald-400 border-emerald-500/40'
    default: return 'bg-slate-600/20 text-slate-400 border-slate-500/40'
  }
}

function auditTypeBadge(type) {
  switch (type) {
    case 'internal': return 'bg-blue-900/40 text-blue-300'
    case 'external': return 'bg-purple-900/40 text-purple-300'
    case 'surveillance': return 'bg-amber-900/40 text-amber-300'
    case 'certification': return 'bg-green-900/40 text-green-300'
    case 'recertification': return 'bg-emerald-900/40 text-emerald-300'
    default: return 'bg-slate-800 text-slate-400'
  }
}

function auditTypeDot(type) {
  return ({
    internal: 'bg-blue-400',
    external: 'bg-purple-400',
    surveillance: 'bg-amber-400',
    certification: 'bg-green-400',
    recertification: 'bg-emerald-400',
  })[type] || 'bg-slate-400'
}

// The same map already exists as common.enum.entity_abbr, which StatusBadge
// and the suggestion renderer read; this was a second copy of it.
function entityTypeShort(type) {
  return enumLabel('entity_abbr', type) || (type || '?').toUpperCase().slice(0, 5)
}

function entityTypeBadge(type) {
  switch (type) {
    case 'document': case 'policy': case 'procedure': case 'clause': case 'record': case 'guideline':
      return 'bg-blue-900/40 text-blue-300'
    case 'control': return 'bg-indigo-900/40 text-indigo-300'
    case 'requirement': return 'bg-purple-900/40 text-purple-300'
    case 'risk': return 'bg-red-900/40 text-red-300'
    case 'legal': return 'bg-purple-900/40 text-purple-300'
    case 'asset': return 'bg-amber-900/40 text-amber-300'
    case 'supplier': return 'bg-emerald-900/40 text-emerald-300'
    case 'system': return 'bg-cyan-900/40 text-cyan-300'
    case 'incident': return 'bg-orange-900/40 text-orange-300'
    case 'change': return 'bg-sky-900/40 text-sky-300'
    case 'corrective_action': return 'bg-pink-900/40 text-pink-300'
    case 'objective': return 'bg-teal-900/40 text-teal-300'
    case 'task': return 'bg-lime-900/40 text-lime-300'
    case 'program': return 'bg-violet-900/40 text-violet-300'
    default: return 'bg-slate-800 text-slate-400'
  }
}

function resultRowClass(result) {
  switch (result) {
    case 'major_nc': return 'border-red-900/40'
    case 'minor_nc': return 'border-amber-900/40'
    case 'observation': return 'border-blue-900/30'
    case 'opportunity': return 'border-emerald-900/30'
    case 'conforming': return 'border-emerald-900/40'
    default: return 'border-slate-800'
  }
}

function resultSelectClass(result) {
  switch (result) {
    case 'major_nc': return 'border-red-700 text-red-300'
    case 'minor_nc': return 'border-amber-700 text-amber-300'
    case 'observation': return 'border-blue-700 text-blue-300'
    case 'opportunity': return 'border-emerald-700 text-emerald-300'
    case 'conforming': return 'border-emerald-700 text-emerald-300'
    default: return ''
  }
}

// ═════════════════════ Top-level tab navigation ═════════════════════
function switchTab(key) {
  activeTab.value = key
  router.push(orgPath('/audit/' + key))
  if (key === 'calendar' && calendarMonths.value.length === 0) loadCalendar()
  if (key === 'findings') loadFindings()
}

// ═════════════════════ Loaders ═════════════════════
async function loadProgrammes() {
  try {
    const res = await api.getAuditProgrammes()
    programmes.value = Array.isArray(res) ? res : []
  } catch (e) { showError(t('audit.error.load_programmes', { message: renderApiError(e) })) }
}

async function loadAudits() {
  try {
    const res = await api.getAudits()
    audits.value = Array.isArray(res) ? res : []
  } catch (e) { showError(t('audit.error.load_audits', { message: renderApiError(e) })) }
}

async function loadCalendar() {
  calendarLoading.value = true
  try {
    const data = await api.getAuditCalendar(calendarYear.value)
    calendarMonths.value = Array.isArray(data?.months) ? data.months : []
    calendarUnscheduled.value = Array.isArray(data?.unscheduled) ? data.unscheduled : []
  } catch (e) {
    showError(t('audit.error.load_calendar', { message: renderApiError(e) }))
    calendarMonths.value = []; calendarUnscheduled.value = []
  } finally {
    calendarLoading.value = false
  }
}

async function changeCalendarYear(delta) {
  calendarYear.value += delta
  await loadCalendar()
}

async function loadFindings() {
  try {
    const params = {
      page: String(findingPage.value),
      limit: String(findingPageSize.value),
    }
    if (findingSearch.value) params.q = findingSearch.value
    if (findingStatusFilter.value) params.status = findingStatusFilter.value
    if (findingTypeFilter.value) params.type = findingTypeFilter.value
    if (findingAuditFilter.value > 0) params.audit_id = String(findingAuditFilter.value)
    if (findingOwnerFilter.value) params.owner = findingOwnerFilter.value
    if (findingOverdueOnly.value) params.overdue = 'true'
    const res = await api.getAuditFindingsPaginated(params)
    findings.value = Array.isArray(res?.data) ? res.data : []
    findingTotal.value = res?.total || 0
  } catch (e) {
    showError(t('audit.error.load_findings', { message: renderApiError(e) }))
    findings.value = []
  }
}

async function refreshFindingCounts() {
  try {
    const [openR, closedR, overdueR] = await Promise.all([
      api.getAuditFindingsPaginated({ status: 'open', limit: '1' }),
      api.getAuditFindingsPaginated({ status: 'closed', limit: '1' }),
      api.getAuditFindingsPaginated({ overdue: 'true', limit: '1' }),
    ])
    openFindingsTotal.value = openR?.total || 0
    closedFindingsTotal.value = closedR?.total || 0
    overdueFindingsTotal.value = overdueR?.total || 0
  } catch { /* best-effort */ }
}

// ═════════════════════ Programme create ═════════════════════
function openCreateProgramme() {
  newProg.value = { title: '', year: new Date().getFullYear() }
  showCreateProgramme.value = true
}

async function createProgramme() {
  progSaving.value = true
  try {
    const created = await api.createAuditProgramme({ ...newProg.value })
    showCreateProgramme.value = false
    newProg.value = { title: '', year: new Date().getFullYear() }
    await loadProgrammes()
    showSaved(t('audit.toast.programme_added'))
    if (created?.id) {
      let fresh = created
      try { fresh = await api.getAuditProgramme(created.id) } catch { /* fall back */ }
      selectedProgramme.value = fresh
      programmeTab.value = 'overview'
      startProgEdit()
      router.push(orgPath('/audit/programmes/' + fresh.id))
    }
  } catch (e) {
    showError(t('audit.error.create_programme', { message: renderApiError(e) }))
  } finally {
    progSaving.value = false
  }
}

// ═════════════════════ Programme detail ═════════════════════
function selectProgramme(prog) {
  selectedProgramme.value = prog
  programmeTab.value = 'overview'
  progEditing.value = false
  router.push(orgPath('/audit/programmes/' + prog.id))
}

function startProgEdit() {
  if (!selectedProgramme.value) return
  const p = selectedProgramme.value
  progEditForm.value = {
    title: p.title || '',
    year: p.year || new Date().getFullYear(),
    description: p.description || '',
    notes: p.notes || '',
    status: p.status || 'active',
  }
  captureProgEdit()
  progEditing.value = true
}

function cancelProgEdit() {
  progEditing.value = false
}

async function saveProgEdit() {
  if (!selectedProgramme.value) return
  progSaving.value = true
  try {
    const payload = { ...progEditForm.value }
    const updated = await api.updateAuditProgramme(selectedProgramme.value.id, payload)
    if (updated && updated.id) selectedProgramme.value = updated
    progEditing.value = false
    await loadProgrammes()
    showSaved('Saved')
  } catch (e) {
    showError(t('audit.error.save_programme', { message: renderApiError(e) }))
  } finally {
    progSaving.value = false
  }
}

async function switchProgrammeTab(key) {
  if (progEditing.value && progIsDirty()) {
    const ok = await confirmDialog({ message: t('common.dirty.switch_tab'), variant: 'danger', confirmLabel: t('common.dirty.discard') })
    if (!ok) return
  }
  programmeTab.value = key
  progEditing.value = false
}

async function closeProgrammeDetail() {
  if (progEditing.value && progIsDirty()) {
    const ok = await confirmDialog({ message: t('common.dirty.close'), variant: 'danger', confirmLabel: t('common.dirty.discard') })
    if (!ok) return
  }
  selectedProgramme.value = null
  progEditing.value = false
  router.push(orgPath('/audit/programmes'))
}

async function deleteSelectedProgramme() {
  if (!selectedProgramme.value) return
  if ((selectedProgramme.value.audit_count || 0) > 0) return
  const ok = await confirmDialog({
    message: t('audit.programme.danger.confirm', { title: selectedProgramme.value.title }),
    variant: 'danger',
    confirmLabel: t('common.action.delete'),
  })
  if (!ok) return
  try {
    await api.deleteAuditProgramme(selectedProgramme.value.id)
    selectedProgramme.value = null
    await loadProgrammes()
    router.push(orgPath('/audit/programmes'))
    showSaved(t('audit.toast.programme_deleted'))
  } catch (e) {
    showError(t('audit.error.delete_programme', { message: renderApiError(e) }))
  }
}

// ═════════════════════ Audit create ═════════════════════
function openCreateAudit(programmeId) {
  newAudit.value = {
    title: '',
    programme_id: programmeId || 0,
    audit_type: 'internal',
  }
  showCreateAudit.value = true
}

async function createAudit() {
  auditCreating.value = true
  try {
    const payload = { ...newAudit.value }
    if (!payload.programme_id) delete payload.programme_id
    const created = await api.createAudit(payload)
    showCreateAudit.value = false
    newAudit.value = { title: '', programme_id: 0, audit_type: 'internal' }
    await loadAudits()
    showSaved(t('audit.toast.audit_added'))
    if (created?.id) {
      let fresh = created
      try { fresh = await api.getAudit(created.id) } catch { /* fall back */ }
      // Close any open programme modal so audit modal shows on top of base view
      if (selectedProgramme.value) selectedProgramme.value = null
      selectedAudit.value = fresh
      auditTab.value = 'overview'
      router.push(orgPath('/audit/audits/' + fresh.id))
      await loadAuditDetail(fresh)
      startAuditEdit()
    }
  } catch (e) {
    showError(t('audit.error.create_audit', { message: renderApiError(e) }))
  } finally {
    auditCreating.value = false
  }
}

// ═════════════════════ Audit detail ═════════════════════
async function loadAuditDetail(audit) {
  auditDetailLoading.value = true
  try {
    const [items, fr] = await Promise.all([
      api.getAuditItems(audit.id),
      api.getAuditFindings(audit.id),
    ])
    auditItems.value = Array.isArray(items) ? items : []
    auditFindings.value = Array.isArray(fr) ? fr : []
  } catch (e) {
    showError(t('audit.error.load_audit_detail', { message: renderApiError(e) }))
    auditItems.value = []; auditFindings.value = []
  } finally {
    auditDetailLoading.value = false
  }
}

async function selectAudit(audit) {
  // Close programme modal if open (modals don't stack well visually)
  if (selectedProgramme.value) selectedProgramme.value = null
  selectedAudit.value = audit
  auditTab.value = 'overview'
  auditEditing.value = false
  reportEditing.value = false
  router.push(orgPath('/audit/audits/' + audit.id))
  await loadAuditDetail(audit)
}

function selectAuditFromCalendar(audit) { selectAudit(audit) }
function selectAuditFromProgramme(audit) { selectAudit(audit) }

async function goToAuditFromFinding(auditId) {
  if (!auditId) return
  let audit = audits.value.find(a => a.id === auditId)
  if (!audit) {
    try { audit = await api.getAudit(auditId) } catch { return }
  }
  if (audit) {
    selectedFinding.value = null
    selectAudit(audit)
  }
}

async function goToProgrammeFromAudit(progId) {
  if (!progId) return
  let prog = programmes.value.find(p => p.id === progId)
  if (!prog) {
    try { prog = await api.getAuditProgramme(progId) } catch { return }
  }
  if (prog) {
    selectedAudit.value = null
    selectProgramme(prog)
  }
}

function startAuditEdit() {
  const a = selectedAudit.value
  if (!a) return
  auditEditForm.value = {
    title: a.title || '',
    scope: a.scope || '',
    auditor: a.auditor || '',
    audit_type: a.audit_type || 'internal',
    status: a.status || 'planned',
    notes: a.notes || '',
    programme_id: a.programme_id || 0,
    planned_date: a.planned_date ? (typeof a.planned_date === 'number' ? new Date(a.planned_date * 1000).toISOString().slice(0, 10) : String(a.planned_date).slice(0, 10)) : '',
    end_date: a.end_date ? (typeof a.end_date === 'number' ? new Date(a.end_date * 1000).toISOString().slice(0, 10) : String(a.end_date).slice(0, 10)) : '',
  }
  captureAuditEdit()
  auditEditing.value = true
}

function cancelAuditEdit() { auditEditing.value = false }

async function saveAuditEdit() {
  if (!selectedAudit.value) return
  auditSaving.value = true
  try {
    const payload = { ...auditEditForm.value }
    if (!payload.planned_date) payload.planned_date = null
    if (!payload.end_date) payload.end_date = null
    if (!payload.programme_id) payload.programme_id = null
    const updated = await api.updateAudit(selectedAudit.value.id, payload)
    if (updated && updated.id) selectedAudit.value = updated
    auditEditing.value = false
    await loadAudits()
    showSaved('Saved')
  } catch (e) {
    showError(t('audit.error.save_audit', { message: renderApiError(e) }))
  } finally {
    auditSaving.value = false
  }
}

function startReportEdit() {
  if (!selectedAudit.value) return
  reportForm.value = { summary: selectedAudit.value.summary || '' }
  captureReportEdit()
  reportEditing.value = true
}

function cancelReportEdit() { reportEditing.value = false }

async function saveReportEdit() {
  if (!selectedAudit.value) return
  auditSaving.value = true
  try {
    const updated = await api.updateAudit(selectedAudit.value.id, { summary: reportForm.value.summary })
    if (updated && updated.id) selectedAudit.value = updated
    reportEditing.value = false
    showSaved(t('audit.toast.report_saved'))
  } catch (e) {
    showError(t('audit.error.save_report', { message: renderApiError(e) }))
  } finally {
    auditSaving.value = false
  }
}

async function switchAuditTab(key) {
  if (auditEditing.value && auditIsDirty()) {
    const ok = await confirmDialog({ message: t('common.dirty.switch_tab'), variant: 'danger', confirmLabel: t('common.dirty.discard') })
    if (!ok) return
  }
  if (reportEditing.value && reportIsDirty()) {
    const ok = await confirmDialog({ message: t('audit.dirty.report_switch_tab'), variant: 'danger', confirmLabel: t('common.dirty.discard') })
    if (!ok) return
  }
  auditTab.value = key
  auditEditing.value = false
  reportEditing.value = false
}

async function closeAuditDetail() {
  if (auditEditing.value && auditIsDirty()) {
    const ok = await confirmDialog({ message: t('common.dirty.close'), variant: 'danger', confirmLabel: t('common.dirty.discard') })
    if (!ok) return
  }
  if (reportEditing.value && reportIsDirty()) {
    const ok = await confirmDialog({ message: t('audit.dirty.report_close'), variant: 'danger', confirmLabel: t('common.dirty.discard') })
    if (!ok) return
  }
  selectedAudit.value = null
  auditEditing.value = false; reportEditing.value = false
  auditItems.value = []; auditFindings.value = []
  showItemPicker.value = false
  router.push(orgPath('/audit/' + activeTab.value))
}

// ═════════════════════ Audit items ═════════════════════
function toggleItemNotes(id) {
  if (expandedItemNotes.has(id)) expandedItemNotes.delete(id)
  else expandedItemNotes.add(id)
}

async function doItemSearch() {
  clearTimeout(itemSearchTimer)
  itemSearchTimer = setTimeout(async () => {
    itemSearching.value = true
    itemSearched.value = false
    try {
      const data = await api.search(itemSearchQuery.value)
      const existing = new Set(auditItems.value.map(it => `${it.item_type}:${it.item_id}`))
      itemSearchResults.value = (data || []).filter(s => !existing.has(`${s.type}:${s.id}`)).slice(0, 30)
      itemSelectedIdx.value = 0
    } catch {
      itemSearchResults.value = []
    } finally {
      itemSearching.value = false
      itemSearched.value = true
    }
  }, 150)
}

async function pickAuditItem(s) {
  if (!selectedAudit.value || !s) return
  try {
    const created = await api.createAuditItem(selectedAudit.value.id, {
      item_id: s.id,
      item_type: s.type,
      title: s.title || s.id,
    })
    if (created && created.id) {
      auditItems.value.push(created)
    } else {
      // fallback: refetch list
      const items = await api.getAuditItems(selectedAudit.value.id)
      auditItems.value = Array.isArray(items) ? items : []
    }
    itemSearchQuery.value = ''
    itemSearchResults.value = []
    showSaved(t('audit.toast.item_added'))
    nextTick(() => itemSearchInput.value?.focus())
  } catch (e) {
    showError(t('audit.error.add_item', { message: renderApiError(e) }))
  }
}

async function saveItemField(item, field, value) {
  try {
    const payload = { [field]: value }
    const updated = await api.updateAuditItem(item.id, payload)
    if (updated && updated.id) Object.assign(item, updated)
  } catch (e) {
    showError(t('audit.error.save_item', { message: renderApiError(e) }))
  }
}

async function deleteItem(item) {
  const ok = await confirmDialog({
    message: t('audit.items.confirm_remove', { title: item.title }),
    variant: 'danger',
    confirmLabel: t('audit.items.remove'),
  })
  if (!ok) return
  try {
    await api.deleteAuditItem(item.id)
    auditItems.value = auditItems.value.filter(x => x.id !== item.id)
    showSaved(t('audit.toast.item_removed'))
  } catch (e) {
    showError(t('audit.error.remove_item', { message: renderApiError(e) }))
  }
}

function raiseFindingFromItem(item) {
  if (!selectedAudit.value) return
  const findingType = item.result === 'major_nc' ? 'major_nc' : 'minor_nc'
  newFindingExtra = { audit_item_id: item.id }
  newFinding.value = {
    title: item.title || `Finding from ${item.item_id}`,
    audit_id: selectedAudit.value.id,
    finding_type: findingType,
  }
  newFindingAuditLocked.value = true
  showCreateFinding.value = true
}

watch(showItemPicker, (open) => {
  if (open) nextTick(() => { itemSearchInput.value?.focus(); doItemSearch() })
  else { itemSearchQuery.value = ''; itemSearchResults.value = [] }
})

// ═════════════════════ Finding create ═════════════════════
function openCreateFinding(auditId) {
  newFindingExtra = {}
  newFinding.value = { title: '', audit_id: auditId || 0, finding_type: 'minor_nc' }
  newFindingAuditLocked.value = !!auditId
  showCreateFinding.value = true
}

async function createFinding() {
  findingCreating.value = true
  try {
    const payload = { ...newFinding.value, ...newFindingExtra }
    const created = await api.addAuditFinding(payload)
    showCreateFinding.value = false
    newFinding.value = { title: '', audit_id: 0, finding_type: 'minor_nc' }
    newFindingExtra = {}
    newFindingAuditLocked.value = false
    await refreshFindingCounts()
    if (activeTab.value === 'findings') loadFindings()
    if (selectedAudit.value && selectedAudit.value.id === created?.audit_id) {
      const fr = await api.getAuditFindings(selectedAudit.value.id)
      auditFindings.value = Array.isArray(fr) ? fr : []
    }
    showSaved(t('audit.toast.finding_added'))
    if (created?.id) {
      let fresh = created
      try { fresh = await api.getAuditFinding(created.id) } catch { /* fall back */ }
      // Close audit modal if any (visual stacking) — finding will own the screen.
      if (selectedAudit.value) selectedAudit.value = null
      selectedFinding.value = fresh
      findingTab.value = 'overview'
      startFindingEdit()
      router.push(orgPath('/audit/findings/' + fresh.id))
    }
  } catch (e) {
    showError(t('audit.error.create_finding', { message: renderApiError(e) }))
  } finally {
    findingCreating.value = false
  }
}

// ═════════════════════ Finding detail ═════════════════════
async function selectFinding(f) {
  if (selectedAudit.value) selectedAudit.value = null
  selectedFinding.value = f
  findingTab.value = 'overview'
  findingEditing.value = false
  router.push(orgPath('/audit/findings/' + f.id))
}

function startFindingEdit() {
  const f = selectedFinding.value
  if (!f) return
  findingEditForm.value = {
    title: f.title || '',
    description: f.description || '',
    finding_type: f.finding_type || 'minor_nc',
    status: f.status || 'open',
    owner: f.owner || '',
    due_date: f.due_date ? (typeof f.due_date === 'number' ? new Date(f.due_date * 1000).toISOString().slice(0, 10) : String(f.due_date).slice(0, 10)) : '',
  }
  captureFindingEdit()
  findingEditing.value = true
}

function cancelFindingEdit() { findingEditing.value = false }

async function saveFindingEdit() {
  if (!selectedFinding.value) return
  findingSaving.value = true
  try {
    const payload = { ...findingEditForm.value }
    if (!payload.due_date) payload.due_date = null
    const updated = await api.updateAuditFinding(selectedFinding.value.id, payload)
    if (updated && updated.id) selectedFinding.value = updated
    findingEditing.value = false
    await refreshFindingCounts()
    if (activeTab.value === 'findings') loadFindings()
    showSaved('Saved')
  } catch (e) {
    showError(t('audit.error.save_finding', { message: renderApiError(e) }))
  } finally {
    findingSaving.value = false
  }
}

async function switchFindingTab(key) {
  if (findingEditing.value && findingIsDirty()) {
    const ok = await confirmDialog({ message: t('common.dirty.switch_tab'), variant: 'danger', confirmLabel: t('common.dirty.discard') })
    if (!ok) return
  }
  findingTab.value = key
  findingEditing.value = false
}

async function closeFindingDetail() {
  if (findingEditing.value && findingIsDirty()) {
    const ok = await confirmDialog({ message: t('common.dirty.close'), variant: 'danger', confirmLabel: t('common.dirty.discard') })
    if (!ok) return
  }
  selectedFinding.value = null
  findingEditing.value = false
  router.push(orgPath('/audit/' + activeTab.value))
}

async function deleteSelectedFinding() {
  if (!selectedFinding.value) return
  const ok = await confirmDialog({
    message: t('audit.findings.danger.confirm'),
    variant: 'danger',
    confirmLabel: t('common.action.delete'),
  })
  if (!ok) return
  try {
    await api.deleteAuditFinding(selectedFinding.value.id)
    selectedFinding.value = null
    await refreshFindingCounts()
    if (activeTab.value === 'findings') loadFindings()
    router.push(orgPath('/audit/' + activeTab.value))
    showSaved(t('audit.toast.finding_deleted'))
  } catch (e) {
    showError(t('audit.error.delete_finding', { message: renderApiError(e) }))
  }
}

function createCAFromFinding(f) {
  const severity = f.finding_type === 'opportunity' ? 'opportunity' : f.finding_type
  router.push({
    path: orgPath('/corrective-actions'),
    query: {
      from_audit_finding: String(f.id),
      title: f.title,
      severity,
    },
  })
}

// ═════════════════════ Watchers ═════════════════════
let findingSearchTimer = null
watch([findingSearch], () => {
  clearTimeout(findingSearchTimer)
  findingSearchTimer = setTimeout(() => { findingPage.value = 1; if (activeTab.value === 'findings') loadFindings() }, 250)
})
watch([findingStatusFilter, findingTypeFilter, findingAuditFilter, findingOwnerFilter, findingOverdueOnly], () => {
  findingPage.value = 1
  if (activeTab.value === 'findings') loadFindings()
})
watch([findingPage, findingPageSize], () => {
  if (activeTab.value === 'findings') loadFindings()
})

// Route deep-linking
watch(() => route.params.tab, (tab) => {
  if (!tab) return
  if (['calendar', 'programmes', 'findings'].includes(tab)) {
    activeTab.value = tab
    if (tab === 'calendar' && calendarMonths.value.length === 0) loadCalendar()
    if (tab === 'findings') loadFindings()
  }
})

watch(() => route.params.itemId, async (itemId) => {
  if (!itemId) {
    selectedProgramme.value = null
    selectedAudit.value = null
    selectedFinding.value = null
    return
  }
  const id = parseInt(itemId)
  const tab = route.params.tab
  if (tab === 'programmes') {
    if (selectedProgramme.value?.id === id) return
    let p = programmes.value.find(x => x.id === id)
    if (!p) { try { p = await api.getAuditProgramme(id) } catch { return } }
    if (p) selectProgramme(p)
  } else if (tab === 'audits') {
    if (selectedAudit.value?.id === id) return
    let a = audits.value.find(x => x.id === id)
    if (!a) { try { a = await api.getAudit(id) } catch { return } }
    if (a) selectAudit(a)
  } else if (tab === 'findings') {
    if (selectedFinding.value?.id === id) return
    try {
      const f = await api.getAuditFinding(id)
      if (f) {
        selectedFinding.value = f
        findingTab.value = 'overview'
        findingEditing.value = false
      }
    } catch { /* ignore */ }
  }
})

// ═════════════════════ Init ═════════════════════
onMounted(async () => {
  try { const me = await api.getMe(); userRole.value = me?.role || '' } catch { /* ignore */ }
  const tabParam = route.params.tab
  if (tabParam && ['calendar', 'programmes', 'findings'].includes(tabParam)) {
    activeTab.value = tabParam
  }
  try {
    const [, , users] = await Promise.all([
      loadProgrammes(),
      loadAudits(),
      api.getUsers().catch(() => []),
    ])
    orgMembers.value = users || []
    await refreshFindingCounts()
    if (activeTab.value === 'calendar') await loadCalendar()
    if (activeTab.value === 'findings') await loadFindings()
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    loading.value = false
  }
  // Deep-link: open detail
  if (route.params.tab && route.params.itemId) {
    const id = parseInt(route.params.itemId)
    const tab = route.params.tab
    if (tab === 'programmes') {
      let p = programmes.value.find(x => x.id === id)
      if (!p) { try { p = await api.getAuditProgramme(id) } catch { /* ignore */ } }
      if (p) selectProgramme(p)
    } else if (tab === 'audits') {
      let a = audits.value.find(x => x.id === id)
      if (!a) { try { a = await api.getAudit(id) } catch { /* ignore */ } }
      if (a) selectAudit(a)
    } else if (tab === 'findings') {
      try {
        const f = await api.getAuditFinding(id)
        if (f) { selectedFinding.value = f; findingTab.value = 'overview' }
      } catch { /* ignore */ }
    }
  }
})
</script>
