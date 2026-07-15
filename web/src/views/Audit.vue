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
        <span>Failed to load audit module. {{ error }}</span>
        <RefreshButton :loading="refreshing" @refresh="reload" />
      </div>
    </div>

    <!-- Main content -->
    <div v-else class="max-w-6xl mx-auto px-8 py-10 space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">Internal Audit</h1>
          <p class="text-sm text-slate-500 mt-1">Audit programmes, schedule, and findings</p>
        </div>
        <div class="flex gap-2">
          <RefreshButton :loading="refreshing" @refresh="reload" />
          <button v-if="canWrite && activeTab === 'programmes'"
            @click="openCreateProgramme"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            Add Programme
          </button>
          <button v-if="canWrite && activeTab === 'findings'"
            @click="openCreateFinding(null)"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            Add Finding
          </button>
          <button v-if="canWrite && activeTab === 'calendar'"
            @click="openCreateAudit(null)"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            Add Audit
          </button>
          <SuggestNewButton entityType="audit_finding" typeLabel="Audit Finding" />
        </div>
      </div>

      <!-- Top-level tabs -->
      <div class="flex items-center border-b border-slate-800">
        <div class="flex gap-1 flex-1">
          <button v-for="t in topTabs" :key="t.key"
            @click="switchTab(t.key)"
            class="flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px"
            :class="activeTab === t.key ? 'border-blue-500 text-blue-400' : 'border-transparent text-slate-500 hover:text-slate-300'">
            {{ t.label }}
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
          <div v-for="t in auditTypeKeys" :key="t" class="flex items-center gap-1.5 text-xs text-slate-400">
            <span class="w-2.5 h-2.5 rounded-full" :class="auditTypeDot(t)"></span>
            <span class="capitalize">{{ t }}</span>
          </div>
        </div>

        <!-- Loading -->
        <div v-if="calendarLoading" class="flex items-center justify-center h-48">
          <div class="text-slate-400 text-sm">Loading calendar...</div>
        </div>

        <!-- Month sections -->
        <div v-else class="space-y-4">
          <div v-for="month in calendarMonths" :key="month.month"
            class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
            <div class="px-5 py-3 border-b border-slate-800 flex items-center justify-between">
              <h3 class="text-sm font-semibold text-slate-300">{{ month.name }}</h3>
              <span class="text-[11px] text-slate-500">
                {{ month.audits.length }} audit{{ month.audits.length === 1 ? '' : 's' }}
              </span>
            </div>
            <div v-if="month.audits.length === 0" class="px-5 py-3 text-xs text-slate-600 italic">No audits planned this month</div>
            <div v-else class="divide-y divide-slate-800/50">
              <div v-for="audit in month.audits" :key="audit.id"
                @click="selectAuditFromCalendar(audit)"
                class="px-5 py-3 flex items-center gap-3 hover:bg-slate-800/50 transition-colors cursor-pointer">
                <span class="w-2 h-2 rounded-full flex-shrink-0" :class="auditTypeDot(audit.audit_type)"></span>
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium text-slate-200 truncate">{{ audit.title }}</div>
                  <div class="text-[11px] text-slate-500 truncate">
                    {{ programmeTitle(audit.programme_id) }} ·
                    {{ resolveUserName(audit.auditor) }} ·
                    <span class="capitalize">{{ audit.audit_type }}</span>
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
              <h3 class="text-sm font-semibold text-amber-400">Unscheduled</h3>
              <span class="text-[11px] text-slate-500">
                {{ calendarUnscheduled.length }} audit{{ calendarUnscheduled.length === 1 ? '' : 's' }} without planned date
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
                    {{ programmeTitle(audit.programme_id) }} · {{ resolveUserName(audit.auditor) }}
                  </div>
                </div>
                <StatusBadge :status="audit.status" />
              </div>
            </div>
          </div>

          <div v-if="calendarMonths.every(m => m.audits.length === 0) && calendarUnscheduled.length === 0"
            class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center text-sm text-slate-500">
            No audits in {{ calendarYear }}. Add an audit from a programme to see it here.
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
            <input v-model="programmeSearch" type="text" placeholder="Search programmes..."
              class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
          </div>
          <select v-model.number="programmeYearFilter" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option :value="0">All years</option>
            <option v-for="y in programmeYears" :key="y" :value="y">{{ y }}</option>
          </select>
          <select v-model="programmeStatusFilter" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">All statuses</option>
            <option value="draft">Draft</option>
            <option value="active">Active</option>
            <option value="closed">Closed</option>
          </select>
          <button v-if="programmeSearch || programmeYearFilter || programmeStatusFilter"
            @click="programmeSearch = ''; programmeYearFilter = 0; programmeStatusFilter = ''"
            class="text-[10px] text-slate-600 hover:text-slate-400 transition-colors">
            Clear
          </button>
          <div class="ml-auto text-xs text-slate-500 tabular-nums">{{ filteredProgrammes.length }} of {{ programmes.length }}</div>
        </div>

        <!-- List -->
        <div v-if="filteredProgrammes.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
          <div v-if="programmeSearch || programmeYearFilter || programmeStatusFilter" class="text-sm text-slate-500">No programmes match your filter.</div>
          <div v-else class="text-sm text-slate-500">No audit programmes yet — click Add Programme to create one.</div>
        </div>
        <div v-else class="bg-slate-900 border border-slate-800 rounded-xl overflow-x-auto">
          <table class="w-full">
            <thead>
              <tr class="border-b border-slate-800">
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">ID</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Title</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Year</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Audits</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Findings</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Status</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/50">
              <tr v-for="prog in filteredProgrammes" :key="prog.id"
                @click="selectProgramme(prog)"
                class="hover:bg-slate-800/50 transition-colors cursor-pointer">
                <td class="px-5 py-3.5 text-[10px] font-mono uppercase tracking-wider text-slate-600">PROG-{{ prog.id }}</td>
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
            title="Toggle: Overdue only">
            <span class="font-bold tabular-nums" :class="findingOverdueOnly ? '' : (overdueFindingsTotal > 0 ? 'text-amber-400' : 'text-slate-100')">{{ overdueFindingsTotal }}</span>
            <span>Overdue</span>
          </button>
        </div>

        <!-- Filters -->
        <div class="flex items-center gap-3 flex-wrap">
          <div class="relative flex-1 max-w-xs">
            <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
            </svg>
            <input v-model="findingSearch" type="text" placeholder="Search findings..."
              class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
          </div>
          <select v-model="findingStatusFilter" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">All statuses</option>
            <option value="open">Open</option>
            <option value="closed">Closed</option>
          </select>
          <select v-model="findingTypeFilter" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">All types</option>
            <option value="major_nc">Major NC</option>
            <option value="minor_nc">Minor NC</option>
            <option value="observation">Observation</option>
            <option value="opportunity">OFI</option>
          </select>
          <select v-model.number="findingAuditFilter" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option :value="0">All audits</option>
            <option v-for="a in audits" :key="a.id" :value="a.id">{{ a.title }}</option>
          </select>
          <select v-model="findingOwnerFilter" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">All owners</option>
            <option v-for="m in orgMembers" :key="m.email" :value="m.email">{{ m.name || m.email }}</option>
          </select>
          <label class="flex items-center gap-1.5 text-xs text-slate-400">
            <input type="checkbox" v-model="findingOverdueOnly" class="rounded bg-slate-800 border-slate-700" />
            Overdue only
          </label>
          <div class="ml-auto text-xs text-slate-500 tabular-nums">{{ findingTotal }} total</div>
        </div>

        <!-- Table -->
        <div v-if="findings.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
          <div class="text-sm text-slate-500">{{ findingsHasFilter ? 'No findings match your filter.' : 'No findings recorded yet.' }}</div>
        </div>
        <div v-else class="bg-slate-900 border border-slate-800 rounded-xl overflow-x-auto">
          <table class="w-full">
            <thead>
              <tr class="border-b border-slate-800">
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">ID</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Title</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Audit</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Type</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Owner</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Due</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Status</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/50">
              <tr v-for="f in findings" :key="f.id"
                @click="selectFinding(f)"
                class="hover:bg-slate-800/50 transition-colors cursor-pointer">
                <td class="px-5 py-3.5 text-[10px] font-mono uppercase tracking-wider text-slate-600">FIND-{{ f.id }}</td>
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
                  {{ f.due_date ? formatDate(f.due_date) : '-' }}
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
          <h2 class="text-sm font-semibold text-slate-200">Add Programme</h2>
          <button @click="showCreateProgramme = false" class="text-slate-500 hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <div class="space-y-3">
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Title *</label>
            <input v-model="newProg.title" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="e.g. 2026 Annual Audit Programme" />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Year *</label>
            <input v-model.number="newProg.year" type="number" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="2026" />
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">You can add description, status and audits after creating.</div>
        <div class="flex justify-end gap-3 pt-3 border-t border-slate-800">
          <button @click="showCreateProgramme = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
          <button @click="createProgramme" :disabled="!newProg.title.trim() || !newProg.year || progSaving"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
            {{ progSaving ? 'Adding...' : 'Add' }}
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
          <h2 class="text-sm font-semibold text-slate-200">Add Audit</h2>
          <button @click="showCreateAudit = false" class="text-slate-500 hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div class="sm:col-span-2">
            <label class="block text-xs font-medium text-slate-500 mb-1">Title *</label>
            <input v-model="newAudit.title" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="Audit title" />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Programme</label>
            <select v-model.number="newAudit.programme_id" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option :value="0">No programme (ad-hoc)</option>
              <option v-for="p in programmes" :key="p.id" :value="p.id">{{ p.title }} ({{ p.year }})</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Audit Type</label>
            <select v-model="newAudit.audit_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option v-for="t in auditTypeKeys" :key="t" :value="t" class="capitalize">{{ t }}</option>
            </select>
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">You can add scope, auditor, dates and items after creating.</div>
        <div class="flex justify-end gap-3 pt-3 border-t border-slate-800">
          <button @click="showCreateAudit = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
          <button @click="createAudit" :disabled="!newAudit.title.trim() || auditCreating"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
            {{ auditCreating ? 'Adding...' : 'Add' }}
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
          <h2 class="text-sm font-semibold text-slate-200">Add Finding</h2>
          <button @click="showCreateFinding = false" class="text-slate-500 hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div class="sm:col-span-2">
            <label class="block text-xs font-medium text-slate-500 mb-1">Title *</label>
            <input v-model="newFinding.title" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="Finding title" />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Audit *</label>
            <select v-model.number="newFinding.audit_id" :disabled="newFindingAuditLocked" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50">
              <option :value="0">Select audit...</option>
              <option v-for="a in audits" :key="a.id" :value="a.id">{{ a.title }}</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-500 mb-1">Type</label>
            <select v-model="newFinding.finding_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
              <option value="major_nc">Major Non-conformity</option>
              <option value="minor_nc">Minor Non-conformity</option>
              <option value="observation">Observation</option>
              <option value="opportunity">Opportunity for Improvement</option>
            </select>
          </div>
        </div>
        <div class="text-[10px] text-slate-600 mt-1">You can add description, owner, due date and corrective action links after creating.</div>
        <div class="flex justify-end gap-3 pt-3 border-t border-slate-800">
          <button @click="showCreateFinding = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
          <button @click="createFinding" :disabled="!newFinding.title.trim() || !newFinding.audit_id || findingCreating"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
            {{ findingCreating ? 'Adding...' : 'Add' }}
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
            <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">PROG-{{ selectedProgramme.id }}</span>
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
              <button v-for="t in programmeDetailTabs" :key="t.key" @click="switchProgrammeTab(t.key)"
                class="w-full text-left px-3 py-2 text-xs font-medium transition-colors flex items-center justify-between"
                :class="programmeTab === t.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                <span>{{ t.label }}</span>
                <span v-if="t.count !== undefined" class="text-[10px] text-slate-600">{{ t.count }}</span>
              </button>
            </div>
          </nav>
          <div class="flex-1 overflow-y-auto min-h-0">
            <!-- Overview -->
            <template v-if="programmeTab === 'overview'">
              <div class="px-6 py-5 space-y-5">
                <div class="flex items-center justify-between">
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Overview</div>
                  <button v-if="canWrite && !progEditing" @click="startProgEdit" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                </div>
                <template v-if="progEditing">
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Title</label>
                      <input v-model="progEditForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Year</label>
                      <input v-model.number="progEditForm.year" type="number" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Status</label>
                      <select v-model="progEditForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option value="draft">Draft</option>
                        <option value="active">Active</option>
                        <option value="closed">Closed</option>
                      </select>
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Description</label>
                      <MarkdownField v-model="progEditForm.description" :self-type="'audit_programme'" :self-id="String(selectedProgramme.id)" :rows="3" placeholder="Programme objectives and scope..." />
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Notes</label>
                      <MarkdownField v-model="progEditForm.notes" :self-type="'audit_programme'" :self-id="String(selectedProgramme.id)" :rows="3" placeholder="Internal notes..." />
                    </div>
                  </div>
                </template>
                <template v-else>
                  <div class="space-y-4">
                    <div>
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Description</div>
                      <div v-if="selectedProgramme.description" class="text-sm text-slate-300 doc-prose" v-html="renderMd(selectedProgramme.description)"></div>
                      <div v-else class="text-sm text-slate-600">—</div>
                    </div>
                    <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Year</div>
                        <div class="text-sm text-slate-300 tabular-nums">{{ selectedProgramme.year }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Status</div>
                        <StatusBadge :status="selectedProgramme.status" />
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Audits</div>
                        <div class="text-sm text-slate-300 tabular-nums">{{ selectedProgramme.audit_count || 0 }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created by</div>
                        <div class="text-sm text-slate-300">{{ resolveUserName(selectedProgramme.created_by) }}</div>
                      </div>
                      <div v-if="selectedProgramme.created_at">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created</div>
                        <div class="text-sm text-slate-300">{{ formatDate(selectedProgramme.created_at) }}</div>
                      </div>
                    </div>
                    <div v-if="selectedProgramme.notes" class="border-t border-slate-800 pt-4">
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Notes</div>
                      <div class="text-sm text-slate-300 doc-prose" v-html="renderMd(selectedProgramme.notes)"></div>
                    </div>
                  </div>
                </template>
              </div>
            </template>

            <!-- Audits -->
            <template v-if="programmeTab === 'audits'">
              <div class="px-6 py-5 space-y-3">
                <div class="flex items-center justify-between">
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Audits in this programme</div>
                  <button v-if="canWrite" @click="openCreateAudit(selectedProgramme.id)" class="text-xs text-blue-400 hover:text-blue-300">+ Add Audit</button>
                </div>
                <div v-if="programmeAudits(selectedProgramme.id).length === 0" class="text-xs text-slate-600 italic">No audits in this programme yet.</div>
                <div v-else class="space-y-2">
                  <div v-for="a in programmeAudits(selectedProgramme.id)" :key="a.id"
                    @click="selectAuditFromProgramme(a)"
                    class="bg-slate-950 border border-slate-800 rounded-lg px-4 py-3 flex items-center gap-3 cursor-pointer hover:border-slate-700 transition-colors">
                    <span class="w-2 h-2 rounded-full flex-shrink-0" :class="auditTypeDot(a.audit_type)"></span>
                    <div class="flex-1 min-w-0">
                      <div class="text-sm font-medium text-slate-200 truncate">{{ a.title }}</div>
                      <div class="text-[11px] text-slate-500 truncate">
                        <span class="capitalize">{{ a.audit_type }}</span> ·
                        {{ resolveUserName(a.auditor) || '—' }} ·
                        {{ formatDateRange(a.planned_date, a.end_date) || 'unscheduled' }}
                      </div>
                    </div>
                    <span v-if="a.open_findings > 0" class="text-[10px] px-1.5 py-0.5 rounded bg-red-900/40 text-red-400 font-medium">{{ a.open_findings }} open</span>
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
                  <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">Danger zone</div>
                  <div class="text-xs text-slate-400">Programmes with audits cannot be deleted. Remove all audits first.</div>
                  <button @click="deleteSelectedProgramme" :disabled="(selectedProgramme.audit_count || 0) > 0"
                    class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 disabled:bg-slate-800 disabled:text-slate-600 disabled:cursor-not-allowed text-red-300 border border-red-800/50 rounded-lg transition-colors">
                    Delete programme
                  </button>
                </div>
              </div>
            </template>
          </div>
        </div>
        <!-- Footer (edit) -->
        <div v-if="progEditing" class="flex-shrink-0 border-t border-slate-800 px-6 py-3 flex justify-end gap-3">
          <button @click="cancelProgEdit" class="px-4 py-1.5 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
          <button @click="saveProgEdit" :disabled="progSaving" class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 text-white text-sm font-medium rounded-lg transition-colors">
            {{ progSaving ? 'Saving...' : 'Save' }}
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
            <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">AUDIT-{{ selectedAudit.id }}</span>
            <span class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-semibold rounded uppercase tracking-wider whitespace-nowrap"
              :class="auditTypeBadge(selectedAudit.audit_type)">{{ selectedAudit.audit_type }}</span>
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
              <button v-for="t in auditDetailTabs" :key="t.key" @click="switchAuditTab(t.key)"
                class="w-full text-left px-3 py-2 text-xs font-medium transition-colors flex items-center justify-between"
                :class="auditTab === t.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                <span>{{ t.label }}</span>
                <span v-if="t.count !== undefined" class="text-[10px] text-slate-600">{{ t.count }}</span>
              </button>
            </div>
          </nav>
          <div class="flex-1 overflow-y-auto min-h-0">
            <div v-if="auditDetailLoading" class="px-6 py-10 text-center text-xs text-slate-500">Loading audit details...</div>
            <template v-else>

              <!-- Overview -->
              <template v-if="auditTab === 'overview'">
                <div class="px-6 py-5 space-y-5">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Overview</div>
                    <button v-if="canWrite && !auditEditing" @click="startAuditEdit" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                  </div>
                  <template v-if="auditEditing">
                    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Title</label>
                        <input v-model="auditEditForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Scope</label>
                        <MarkdownField v-model="auditEditForm.scope" :self-type="'audit'" :self-id="String(selectedAudit.id)" :rows="3" placeholder="Scope description. Type /doc to link controls or clauses..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Audit Type</label>
                        <select v-model="auditEditForm.audit_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="t in auditTypeKeys" :key="t" :value="t" class="capitalize">{{ t }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Status</label>
                        <select v-model="auditEditForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="planned">Planned</option>
                          <option value="in_progress">In Progress</option>
                          <option value="completed">Completed</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Auditor</label>
                        <MemberPicker v-model="auditEditForm.auditor" :members="orgMembers" placeholder="Select auditor..." />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Programme</label>
                        <select v-model.number="auditEditForm.programme_id" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option :value="0">No programme (ad-hoc)</option>
                          <option v-for="p in programmes" :key="p.id" :value="p.id">{{ p.title }} ({{ p.year }})</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">Planned Date</label>
                        <input v-model="auditEditForm.planned_date" type="date" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">End Date</label>
                        <input v-model="auditEditForm.end_date" type="date" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">Notes</label>
                        <MarkdownField v-model="auditEditForm.notes" :self-type="'audit'" :self-id="String(selectedAudit.id)" :rows="3" placeholder="Internal audit notes..." />
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Scope</div>
                        <div v-if="selectedAudit.scope" class="text-sm text-slate-300 doc-prose" v-html="renderMd(selectedAudit.scope)"></div>
                        <div v-else class="text-sm text-slate-600">—</div>
                      </div>
                      <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Programme</div>
                          <div class="text-sm">
                            <button v-if="selectedAudit.programme_id" @click="goToProgrammeFromAudit(selectedAudit.programme_id)" class="text-blue-400 hover:text-blue-300">{{ programmeTitle(selectedAudit.programme_id) }}</button>
                            <span v-else class="text-slate-600">—</span>
                          </div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Auditor</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedAudit.auditor) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Planned</div>
                          <div class="text-sm text-slate-300">{{ formatDateRange(selectedAudit.planned_date, selectedAudit.end_date) || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Audit Type</div>
                          <div class="text-sm text-slate-300 capitalize">{{ selectedAudit.audit_type }}</div>
                        </div>
                        <div v-if="selectedAudit.started_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Started</div>
                          <div class="text-sm text-slate-300">{{ formatDateTime(selectedAudit.started_at) }}</div>
                        </div>
                        <div v-if="selectedAudit.completed_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Completed</div>
                          <div class="text-sm text-emerald-400">{{ formatDateTime(selectedAudit.completed_at) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Items</div>
                          <div class="text-sm text-slate-300 tabular-nums">{{ selectedAudit.item_count || 0 }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Findings</div>
                          <div class="text-sm tabular-nums" :class="(selectedAudit.open_findings || 0) > 0 ? 'text-red-400' : 'text-slate-300'">
                            {{ selectedAudit.finding_count || 0 }}<span v-if="(selectedAudit.open_findings || 0) > 0" class="text-slate-500"> ({{ selectedAudit.open_findings }} open)</span>
                          </div>
                        </div>
                      </div>
                      <div v-if="selectedAudit.notes" class="border-t border-slate-800 pt-4">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Notes</div>
                        <div class="text-sm text-slate-300 doc-prose" v-html="renderMd(selectedAudit.notes)"></div>
                      </div>
                    </div>
                  </template>
                </div>
              </template>

              <!-- Items (work surface) -->
              <template v-if="auditTab === 'items'">
                <div class="px-6 py-5 space-y-3">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Audit Items ({{ auditItems.length }})</div>
                    <button v-if="canWrite" @click="showItemPicker = !showItemPicker" class="text-xs text-blue-400 hover:text-blue-300">
                      {{ showItemPicker ? 'Cancel' : '+ Add Item' }}
                    </button>
                  </div>

                  <!-- Item picker (universal search) -->
                  <div v-if="showItemPicker" class="bg-slate-950 border border-slate-700 rounded-lg p-3 space-y-2">
                    <input v-model="itemSearchQuery" type="text" ref="itemSearchInput"
                      class="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500"
                      placeholder="Search documents, controls, clauses, risks, assets..."
                      @input="doItemSearch"
                      @focus="doItemSearch"
                      @keydown.down.prevent="itemSelectedIdx = Math.min(itemSelectedIdx + 1, itemSearchResults.length - 1)"
                      @keydown.up.prevent="itemSelectedIdx = Math.max(itemSelectedIdx - 1, 0)"
                      @keydown.tab.prevent="itemSearchResults.length && pickAuditItem(itemSearchResults[itemSelectedIdx])"
                      @keydown.enter.prevent="itemSearchResults.length && pickAuditItem(itemSearchResults[itemSelectedIdx])"
                      @keydown.escape="showItemPicker = false" />
                    <div v-if="itemSearching" class="text-[10px] text-slate-500 italic">Searching...</div>
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
                    <div v-else-if="itemSearched" class="text-[10px] text-slate-500 italic">No matches.</div>
                  </div>

                  <!-- Items list -->
                  <div v-if="auditItems.length === 0 && !showItemPicker" class="bg-slate-950 border border-slate-800 rounded-lg p-8 text-center text-xs text-slate-600 italic">
                    No items yet — add controls, clauses, risks or other items to assess.
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
                          <option value="not_assessed">Not Assessed</option>
                          <option value="conforming">Conforming</option>
                          <option value="minor_nc">Minor NC</option>
                          <option value="major_nc">Major NC</option>
                          <option value="observation">Observation</option>
                          <option value="opportunity">Opportunity</option>
                        </select>
                        <span v-else class="text-xs"><StatusBadge :status="item.result || 'not_assessed'" /></span>
                        <button v-if="canWrite && (item.result === 'minor_nc' || item.result === 'major_nc')"
                          @click="raiseFindingFromItem(item)"
                          class="text-[10px] text-amber-400 hover:text-amber-300 px-2 py-1 rounded border border-amber-800/50 bg-amber-900/20"
                          title="Create a finding from this item">
                          Raise finding
                        </button>
                        <button v-if="canWrite" @click="deleteItem(item)"
                          class="text-slate-600 hover:text-red-400 p-1 transition-colors"
                          title="Delete item">
                          <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                            <path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6M1 7h22M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                          </svg>
                        </button>
                      </div>

                      <!-- Evidence -->
                      <div class="mt-2 flex items-center gap-2">
                        <span class="text-[10px] text-slate-500 uppercase tracking-wider whitespace-nowrap">Evidence</span>
                        <input v-if="canWrite" v-model="item.evidence" @change="saveItemField(item, 'evidence', item.evidence)"
                          type="text" placeholder="Evidence reference, link or description..."
                          class="flex-1 bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                        <span v-else class="flex-1 text-xs text-slate-500">{{ item.evidence || '—' }}</span>
                      </div>

                      <!-- Notes (collapsible) -->
                      <div class="mt-1.5">
                        <button @click="toggleItemNotes(item.id)" class="text-[10px] text-slate-500 hover:text-slate-300 transition-colors">
                          {{ expandedItemNotes.has(item.id) ? '▼' : '▶' }} Notes{{ item.notes ? '' : ' (empty)' }}
                        </button>
                        <div v-if="expandedItemNotes.has(item.id)" class="mt-2">
                          <MarkdownField v-if="canWrite" v-model="item.notes" :self-type="'audit'" :self-id="String(selectedAudit.id)" :rows="3" placeholder="Inline notes for this item..." />
                          <div v-else class="text-xs text-slate-400 doc-prose" v-html="renderMd(item.notes || '—')"></div>
                          <div v-if="canWrite" class="flex justify-end mt-1.5">
                            <button @click="saveItemField(item, 'notes', item.notes)" class="text-[10px] text-blue-400 hover:text-blue-300">Save notes</button>
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
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Findings ({{ auditFindings.length }})</div>
                    <button v-if="canWrite" @click="openCreateFinding(selectedAudit.id)" class="text-xs text-blue-400 hover:text-blue-300">+ Add Finding</button>
                  </div>
                  <div v-if="auditFindings.length === 0" class="text-xs text-slate-600 italic">No findings recorded for this audit yet.</div>
                  <div v-else class="space-y-2">
                    <div v-for="f in auditFindings" :key="f.id"
                      @click="selectFinding(f)"
                      class="bg-slate-950 border rounded-lg px-4 py-3 cursor-pointer hover:border-slate-700 transition-colors"
                      :class="f.status === 'open' ? 'border-red-900/40' : 'border-slate-800'">
                      <div class="flex items-center gap-2 flex-wrap">
                        <span class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-semibold rounded uppercase tracking-wider whitespace-nowrap"
                          :class="findingTypeBadge(f.finding_type)">{{ findingTypeLabel(f.finding_type) }}</span>
                        <span class="text-sm font-medium text-slate-200 flex-1 truncate min-w-0">{{ f.title }}</span>
                        <span v-if="f.audit_item_id" class="text-[10px] text-slate-500 font-mono">item #{{ f.audit_item_id }}</span>
                        <span v-if="isOverdue(f.due_date) && f.status === 'open'" class="text-[10px] px-1.5 py-0.5 rounded bg-red-900/60 text-red-300 font-semibold tracking-wider uppercase">Overdue</span>
                        <span v-if="f.due_date" class="text-xs text-slate-500">{{ formatDate(f.due_date) }}</span>
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
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Audit Report</div>
                    <button v-if="canWrite && !reportEditing" @click="startReportEdit" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                  </div>
                  <template v-if="reportEditing">
                    <MarkdownField v-model="reportForm.summary" :self-type="'audit'" :self-id="String(selectedAudit.id)" :rows="20" placeholder="Auditor's final report — observations, conclusions, recommendations. Use slash commands to link controls, clauses, evidence..." />
                  </template>
                  <template v-else>
                    <div v-if="selectedAudit.summary" class="text-sm text-slate-300 doc-prose leading-relaxed" v-html="renderMd(selectedAudit.summary)"></div>
                    <div v-else class="text-sm text-slate-600 italic">No report written yet.</div>
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
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">Danger zone</div>
                    <div class="text-xs text-slate-400">Audits cannot currently be deleted from the UI. Manage destructive actions via CLI.</div>
                  </div>
                </div>
              </template>
            </template>
          </div>
        </div>
        <!-- Footer (edit) -->
        <div v-if="auditEditing || reportEditing" class="flex-shrink-0 border-t border-slate-800 px-6 py-3 flex justify-end gap-3">
          <button @click="auditEditing ? cancelAuditEdit() : cancelReportEdit()" class="px-4 py-1.5 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
          <button @click="auditEditing ? saveAuditEdit() : saveReportEdit()" :disabled="auditSaving"
            class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 text-white text-sm font-medium rounded-lg transition-colors">
            {{ auditSaving ? 'Saving...' : 'Save' }}
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
            <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">FIND-{{ selectedFinding.id }}</span>
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
              <button v-for="t in findingDetailTabs" :key="t.key" @click="switchFindingTab(t.key)"
                class="w-full text-left px-3 py-2 text-xs font-medium transition-colors"
                :class="findingTab === t.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                {{ t.label }}
              </button>
            </div>
          </nav>
          <div class="flex-1 overflow-y-auto min-h-0">
            <!-- Overview -->
            <template v-if="findingTab === 'overview'">
              <div class="px-6 py-5 space-y-5">
                <div class="flex items-center justify-between">
                  <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Overview</div>
                  <button v-if="canWrite && !findingEditing" @click="startFindingEdit" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">Edit</button>
                </div>
                <template v-if="findingEditing">
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Title</label>
                      <input v-model="findingEditForm.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                    <div class="sm:col-span-2">
                      <label class="block text-xs font-medium text-slate-500 mb-1">Description</label>
                      <MarkdownField v-model="findingEditForm.description" :self-type="'audit_finding'" :self-id="String(selectedFinding.id)" :rows="6" placeholder="Detailed finding description, evidence, root cause analysis..." />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Type</label>
                      <div class="flex flex-wrap gap-1.5">
                        <button v-for="t in findingTypeKeys" :key="t"
                          @click="findingEditForm.finding_type = t"
                          class="px-2.5 py-1 text-[11px] font-medium rounded-lg border transition-colors"
                          :class="findingEditForm.finding_type === t ? findingTypeChipActive(t) : 'bg-slate-800 text-slate-400 border-slate-700 hover:border-slate-600'">
                          {{ findingTypeLabel(t) }}
                        </button>
                      </div>
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Status</label>
                      <select v-model="findingEditForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                        <option value="open">Open</option>
                        <option value="closed">Closed</option>
                      </select>
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Owner</label>
                      <MemberPicker v-model="findingEditForm.owner" :members="orgMembers" placeholder="Select owner..." />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-slate-500 mb-1">Due Date</label>
                      <input v-model="findingEditForm.due_date" type="date" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                    </div>
                  </div>
                </template>
                <template v-else>
                  <div class="space-y-4">
                    <div>
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Description</div>
                      <div v-if="selectedFinding.description" class="text-sm text-slate-300 doc-prose leading-relaxed" v-html="renderMd(selectedFinding.description)"></div>
                      <div v-else class="text-sm text-slate-600">—</div>
                    </div>
                    <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Audit</div>
                        <button v-if="selectedFinding.audit_id" @click="goToAuditFromFinding(selectedFinding.audit_id)" class="text-sm text-blue-400 hover:text-blue-300 truncate text-left">{{ selectedFinding.audit_title || ('AUDIT-' + selectedFinding.audit_id) }}</button>
                        <span v-else class="text-sm text-slate-600">—</span>
                      </div>
                      <div v-if="selectedFinding.audit_item_id">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Audit Item</div>
                        <div class="text-sm text-slate-300 font-mono">item #{{ selectedFinding.audit_item_id }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Type</div>
                        <span class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-semibold rounded uppercase tracking-wider"
                          :class="findingTypeBadge(selectedFinding.finding_type)">{{ findingTypeLabel(selectedFinding.finding_type) }}</span>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Status</div>
                        <StatusBadge :status="selectedFinding.status" />
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Owner</div>
                        <div class="text-sm text-slate-300">{{ resolveUserName(selectedFinding.owner) }}</div>
                      </div>
                      <div>
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Due date</div>
                        <div class="text-sm" :class="isOverdue(selectedFinding.due_date) && selectedFinding.status === 'open' ? 'text-red-400 font-medium' : 'text-slate-300'">
                          {{ selectedFinding.due_date ? formatDate(selectedFinding.due_date) : '—' }}
                        </div>
                      </div>
                      <div v-if="selectedFinding.created_at">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Created</div>
                        <div class="text-sm text-slate-300">{{ formatDate(selectedFinding.created_at) }}</div>
                      </div>
                      <div v-if="selectedFinding.closed_at">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Closed</div>
                        <div class="text-sm text-emerald-400">{{ formatDateTime(selectedFinding.closed_at) }}<span v-if="selectedFinding.closed_by" class="text-slate-500"> by {{ resolveUserName(selectedFinding.closed_by) }}</span></div>
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
                  <div class="text-[11px] font-semibold text-slate-400 uppercase tracking-wider mb-2">Quick action</div>
                  <button @click="createCAFromFinding(selectedFinding)"
                    class="flex items-center justify-between w-full gap-3 px-4 py-2.5 rounded-lg bg-slate-900 hover:bg-slate-800 border border-slate-700 hover:border-slate-600 transition-colors text-left">
                    <div>
                      <div class="text-sm font-medium text-slate-200">Create Corrective Action from finding</div>
                      <div class="text-xs text-slate-500 mt-0.5">Spawn a CA pre-filled with this finding's title and severity.</div>
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
                  <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">Danger zone</div>
                  <div class="text-xs text-slate-400">Deleting this finding is permanent. Linked corrective actions stay but lose this back-reference.</div>
                  <button @click="deleteSelectedFinding"
                    class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                    Delete finding
                  </button>
                </div>
              </div>
            </template>
          </div>
        </div>
        <!-- Footer (edit) -->
        <div v-if="findingEditing" class="flex-shrink-0 border-t border-slate-800 px-6 py-3 flex justify-end gap-3">
          <button @click="cancelFindingEdit" class="px-4 py-1.5 text-sm text-slate-400 hover:text-slate-200 transition-colors">Cancel</button>
          <button @click="saveFindingEdit" :disabled="findingSaving"
            class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 text-white text-sm font-medium rounded-lg transition-colors">
            {{ findingSaving ? 'Saving...' : 'Save' }}
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

const { confirm: confirmDialog } = useConfirm()
const { show: showError, success: showSaved } = useToast()
const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()

// ═════════════════════ Constants ═════════════════════
const auditTypeKeys = ['internal', 'external', 'surveillance', 'certification', 'recertification']
const findingTypeKeys = ['major_nc', 'minor_nc', 'observation', 'opportunity']

const topTabs = [
  { key: 'calendar', label: 'Calendar' },
  { key: 'programmes', label: 'Programmes' },
  { key: 'findings', label: 'Findings' },
]

const programmeDetailTabsBase = [
  { key: 'overview', label: 'Overview' },
  { key: 'audits', label: 'Audits' },
  { key: 'discussion', label: 'Discussion' },
  { key: 'history', label: 'History' },
]

const auditDetailTabsBase = [
  { key: 'overview', label: 'Overview' },
  { key: 'items', label: 'Items' },
  { key: 'findings', label: 'Findings' },
  { key: 'report', label: 'Report' },
  { key: 'links', label: 'Links' },
  { key: 'discussion', label: 'Discussion' },
  { key: 'history', label: 'History' },
]

const findingDetailTabs = [
  { key: 'overview', label: 'Overview' },
  { key: 'linked', label: 'Linked CAs' },
  { key: 'discussion', label: 'Discussion' },
  { key: 'history', label: 'History' },
]

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
    error.value = e.message
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
  { key: '', label: 'Total', count: programmes.value.length, color: 'text-slate-100' },
  { key: 'active', label: 'Active', count: programmeStats.value.active, color: 'text-emerald-400' },
  { key: 'closed', label: 'Closed', count: programmeStats.value.closed, color: 'text-slate-400' },
  { key: 'draft', label: 'Draft', count: programmeStats.value.draft, color: 'text-amber-400' },
])

// Overdue is a boolean toggle (separate dimension), so it rides alongside the
// status strip as its own chip rather than inside StatStrip's single v-model.
const findingStatusStats = computed(() => [
  { key: '', label: 'Total', count: findingTotal.value, color: 'text-slate-100' },
  { key: 'open', label: 'Open', count: openFindingsTotal.value, color: openFindingsTotal.value > 0 ? 'text-red-400' : 'text-slate-100' },
  { key: 'closed', label: 'Closed', count: closedFindingsTotal.value, color: 'text-emerald-400' },
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

const programmeDetailTabs = computed(() => programmeDetailTabsBase.map(t => {
  if (t.key === 'audits' && selectedProgramme.value) {
    return { ...t, count: programmeAudits(selectedProgramme.value.id).length }
  }
  return t
}))

const auditDetailTabs = computed(() => auditDetailTabsBase.map(t => {
  if (t.key === 'items') return { ...t, count: auditItems.value.length }
  if (t.key === 'findings') return { ...t, count: auditFindings.value.length }
  return t
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

function formatDate(dateStr) {
  if (!dateStr && dateStr !== 0) return ''
  const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
  return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

function formatDateTime(d) {
  if (!d && d !== 0) return ''
  const dt = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  return dt.toLocaleString('en-GB', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function formatDateRange(planned, end) {
  const sd = planned ? (typeof planned === 'number' ? new Date(planned * 1000) : new Date(planned)) : null
  const ed = end ? (typeof end === 'number' ? new Date(end * 1000) : new Date(end)) : null
  const s = sd ? sd.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' }) : ''
  const e = ed ? ed.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' }) : ''
  if (s && e) return `${s} – ${e}`
  return s || ''
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

function findingTypeLabel(t) {
  return ({ major_nc: 'Major NC', minor_nc: 'Minor NC', observation: 'Observation', opportunity: 'OFI' })[t] || t
}

function findingTypeBadge(t) {
  switch (t) {
    case 'major_nc': return 'bg-red-900/60 text-red-300 border border-red-800'
    case 'minor_nc': return 'bg-amber-900/60 text-amber-300 border border-amber-800'
    case 'observation': return 'bg-blue-900/60 text-blue-300 border border-blue-800'
    case 'opportunity': return 'bg-emerald-900/60 text-emerald-300 border border-emerald-800'
    default: return 'bg-slate-800 text-slate-400 border border-slate-700'
  }
}

function findingTypeChipActive(t) {
  switch (t) {
    case 'major_nc': return 'bg-red-600/20 text-red-400 border-red-500/40'
    case 'minor_nc': return 'bg-amber-600/20 text-amber-400 border-amber-500/40'
    case 'observation': return 'bg-blue-600/20 text-blue-400 border-blue-500/40'
    case 'opportunity': return 'bg-emerald-600/20 text-emerald-400 border-emerald-500/40'
    default: return 'bg-slate-600/20 text-slate-400 border-slate-500/40'
  }
}

function auditTypeBadge(t) {
  switch (t) {
    case 'internal': return 'bg-blue-900/40 text-blue-300'
    case 'external': return 'bg-purple-900/40 text-purple-300'
    case 'surveillance': return 'bg-amber-900/40 text-amber-300'
    case 'certification': return 'bg-green-900/40 text-green-300'
    case 'recertification': return 'bg-emerald-900/40 text-emerald-300'
    default: return 'bg-slate-800 text-slate-400'
  }
}

function auditTypeDot(t) {
  return ({
    internal: 'bg-blue-400',
    external: 'bg-purple-400',
    surveillance: 'bg-amber-400',
    certification: 'bg-green-400',
    recertification: 'bg-emerald-400',
  })[t] || 'bg-slate-400'
}

function entityTypeShort(t) {
  return ({
    document: 'DOC', control: 'CTRL', policy: 'POL', procedure: 'PROC',
    clause: 'CLS', requirement: 'REQ', record: 'REC', guideline: 'GUIDE',
    risk: 'RISK', legal: 'LEGAL', asset: 'ASSET',
    supplier: 'SUP', system: 'SYS', incident: 'INC', change: 'CR',
    corrective_action: 'CA', objective: 'OBJ', task: 'TASK', program: 'PROG',
  })[t] || (t || '?').toUpperCase().slice(0, 5)
}

function entityTypeBadge(t) {
  switch (t) {
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
  } catch (e) { showError('Failed to load programmes: ' + (e.message || e)) }
}

async function loadAudits() {
  try {
    const res = await api.getAudits()
    audits.value = Array.isArray(res) ? res : []
  } catch (e) { showError('Failed to load audits: ' + (e.message || e)) }
}

async function loadCalendar() {
  calendarLoading.value = true
  try {
    const data = await api.getAuditCalendar(calendarYear.value)
    calendarMonths.value = Array.isArray(data?.months) ? data.months : []
    calendarUnscheduled.value = Array.isArray(data?.unscheduled) ? data.unscheduled : []
  } catch (e) {
    showError('Failed to load calendar: ' + (e.message || e))
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
    showError('Failed to load findings: ' + (e.message || e))
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
    showSaved('Programme added')
    if (created?.id) {
      let fresh = created
      try { fresh = await api.getAuditProgramme(created.id) } catch { /* fall back */ }
      selectedProgramme.value = fresh
      programmeTab.value = 'overview'
      startProgEdit()
      router.push(orgPath('/audit/programmes/' + fresh.id))
    }
  } catch (e) {
    showError(e.message || 'Failed to create programme')
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
    showError(e.message || 'Failed to save programme')
  } finally {
    progSaving.value = false
  }
}

async function switchProgrammeTab(key) {
  if (progEditing.value && progIsDirty()) {
    const ok = await confirmDialog({ message: 'You have unsaved changes. Discard and switch tab?', variant: 'danger', confirmLabel: 'Discard' })
    if (!ok) return
  }
  programmeTab.value = key
  progEditing.value = false
}

async function closeProgrammeDetail() {
  if (progEditing.value && progIsDirty()) {
    const ok = await confirmDialog({ message: 'You have unsaved changes. Discard and close?', variant: 'danger', confirmLabel: 'Discard' })
    if (!ok) return
  }
  selectedProgramme.value = null
  progEditing.value = false
  router.push(orgPath('/audit/programmes'))
}

async function deleteSelectedProgramme() {
  if (!selectedProgramme.value) return
  if ((selectedProgramme.value.audit_count || 0) > 0) return
  const ok = await confirmDialog({ message: `Delete programme "${selectedProgramme.value.title}"? This cannot be undone.`, variant: 'danger', confirmLabel: 'Delete' })
  if (!ok) return
  try {
    await api.deleteAuditProgramme(selectedProgramme.value.id)
    selectedProgramme.value = null
    await loadProgrammes()
    router.push(orgPath('/audit/programmes'))
    showSaved('Programme deleted')
  } catch (e) {
    showError(e.message || 'Failed to delete programme')
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
    showSaved('Audit added')
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
    showError(e.message || 'Failed to create audit')
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
    showError('Failed to load audit details: ' + (e.message || e))
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
    showError(e.message || 'Failed to save audit')
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
    showSaved('Report saved')
  } catch (e) {
    showError(e.message || 'Failed to save report')
  } finally {
    auditSaving.value = false
  }
}

async function switchAuditTab(key) {
  if (auditEditing.value && auditIsDirty()) {
    const ok = await confirmDialog({ message: 'You have unsaved changes. Discard and switch tab?', variant: 'danger', confirmLabel: 'Discard' })
    if (!ok) return
  }
  if (reportEditing.value && reportIsDirty()) {
    const ok = await confirmDialog({ message: 'You have unsaved report changes. Discard and switch tab?', variant: 'danger', confirmLabel: 'Discard' })
    if (!ok) return
  }
  auditTab.value = key
  auditEditing.value = false
  reportEditing.value = false
}

async function closeAuditDetail() {
  if (auditEditing.value && auditIsDirty()) {
    const ok = await confirmDialog({ message: 'You have unsaved changes. Discard and close?', variant: 'danger', confirmLabel: 'Discard' })
    if (!ok) return
  }
  if (reportEditing.value && reportIsDirty()) {
    const ok = await confirmDialog({ message: 'You have unsaved report changes. Discard and close?', variant: 'danger', confirmLabel: 'Discard' })
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
    showSaved('Item added')
    nextTick(() => itemSearchInput.value?.focus())
  } catch (e) {
    showError('Failed to add item: ' + (e.message || e))
  }
}

async function saveItemField(item, field, value) {
  try {
    const payload = { [field]: value }
    const updated = await api.updateAuditItem(item.id, payload)
    if (updated && updated.id) Object.assign(item, updated)
  } catch (e) {
    showError('Failed to save: ' + (e.message || e))
  }
}

async function deleteItem(item) {
  const ok = await confirmDialog({ message: `Remove "${item.title}" from this audit?`, variant: 'danger', confirmLabel: 'Remove' })
  if (!ok) return
  try {
    await api.deleteAuditItem(item.id)
    auditItems.value = auditItems.value.filter(x => x.id !== item.id)
    showSaved('Item removed')
  } catch (e) {
    showError('Failed to remove: ' + (e.message || e))
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
    showSaved('Finding added')
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
    showError(e.message || 'Failed to add finding')
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
    showError(e.message || 'Failed to save finding')
  } finally {
    findingSaving.value = false
  }
}

async function switchFindingTab(key) {
  if (findingEditing.value && findingIsDirty()) {
    const ok = await confirmDialog({ message: 'You have unsaved changes. Discard and switch tab?', variant: 'danger', confirmLabel: 'Discard' })
    if (!ok) return
  }
  findingTab.value = key
  findingEditing.value = false
}

async function closeFindingDetail() {
  if (findingEditing.value && findingIsDirty()) {
    const ok = await confirmDialog({ message: 'You have unsaved changes. Discard and close?', variant: 'danger', confirmLabel: 'Discard' })
    if (!ok) return
  }
  selectedFinding.value = null
  findingEditing.value = false
  router.push(orgPath('/audit/' + activeTab.value))
}

async function deleteSelectedFinding() {
  if (!selectedFinding.value) return
  const ok = await confirmDialog({ message: 'Delete this finding? This cannot be undone.', variant: 'danger', confirmLabel: 'Delete' })
  if (!ok) return
  try {
    await api.deleteAuditFinding(selectedFinding.value.id)
    selectedFinding.value = null
    await refreshFindingCounts()
    if (activeTab.value === 'findings') loadFindings()
    router.push(orgPath('/audit/' + activeTab.value))
    showSaved('Finding deleted')
  } catch (e) {
    showError(e.message || 'Failed to delete finding')
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
    error.value = e.message
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
