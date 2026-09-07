<template>
  <div class="min-h-full">
    <!-- Loading state -->
    <div v-if="loading" class="flex items-center justify-center h-96">
      <div class="text-slate-400 text-sm">{{ t('dashboard.loading') }}</div>
    </div>

    <!-- Has orgs but none selected — org picker -->
    <div v-else-if="hasOrgsButNoneSelected" class="max-w-lg mx-auto px-8 py-24 text-center">
      <div class="w-16 h-16 rounded-2xl bg-gradient-to-br from-blue-500 to-blue-700 flex items-center justify-center mx-auto mb-6">
        <svg class="w-8 h-8 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
        </svg>
      </div>
      <h2 class="text-xl font-bold text-white mb-2">{{ t('dashboard.org_picker.title') }}</h2>
      <p class="text-slate-400 mb-8">{{ t('dashboard.org_picker.subtitle') }}</p>

      <div class="space-y-2">
        <button v-for="org in myOrgs" :key="org.slug" @click="selectOrg(org)"
          class="w-full flex items-center justify-between px-4 py-3 bg-slate-900 border border-slate-800 rounded-xl hover:border-slate-600 transition-colors text-left">
          <div>
            <div class="text-sm font-medium text-white">{{ org.name }}</div>
            <div class="text-xs text-slate-500">{{ org.slug }}</div>
          </div>
          <span class="text-[10px] px-1.5 py-0.5 rounded-full" :class="org.role === 'admin' ? 'bg-purple-500/20 text-purple-300' : 'bg-slate-500/20 text-slate-400'">{{ roleLabel(org.role) }}</span>
        </button>
      </div>

      <div class="mt-6 pt-6 border-t border-slate-800">
        <button @click="showCreateForm = true" v-if="!showCreateForm" class="text-sm text-slate-400 hover:text-white transition-colors">
          {{ t('dashboard.org_picker.create_toggle') }}
        </button>
        <form v-if="showCreateForm" @submit.prevent="createOrg" class="text-left space-y-3">
          <div>
            <label class="block text-xs text-slate-500 mb-1">{{ t('organizations.form.name_label') }}</label>
            <input v-model="newOrg.name" @input="onNameInput" type="text" :placeholder="t('organizations.form.name_placeholder')" required
              class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">{{ t('organizations.form.slug_label') }}</label>
            <div class="flex items-center gap-0">
              <span class="px-3 py-2 bg-slate-900 border border-r-0 border-slate-700 rounded-l-lg text-sm text-slate-500">{{ baseDomain }}/</span>
              <input v-model="newOrg.slug" @input="onSlugInput" type="text" :placeholder="t('organizations.form.slug_placeholder')" required
                class="flex-1 px-3 py-2 bg-slate-800 border border-slate-700 rounded-r-lg text-sm text-white focus:outline-none focus:border-blue-500" />
            </div>
          </div>
          <div class="flex gap-2">
            <button type="submit" :disabled="creatingOrg || !newOrg.name.trim() || !newOrg.slug.trim()"
              class="flex-1 px-4 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50">
              {{ creatingOrg ? t('organizations.form.creating') : t('organizations.create') }}
            </button>
            <button type="button" @click="showCreateForm = false" class="px-4 py-2.5 text-sm text-slate-400 hover:text-white transition-colors">{{ t('common.action.cancel') }}</button>
          </div>
          <div v-if="orgError" class="text-xs text-red-400">{{ orgError }}</div>
        </form>
      </div>
    </div>

    <!-- No organization at all -->
    <div v-else-if="noOrg" class="max-w-lg mx-auto px-8 py-24 text-center">
      <div class="w-16 h-16 rounded-2xl bg-gradient-to-br from-blue-500 to-blue-700 flex items-center justify-center mx-auto mb-6">
        <svg class="w-8 h-8 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
        </svg>
      </div>
      <h2 class="text-xl font-bold text-white mb-2">{{ t('dashboard.no_org.title') }}</h2>
      <p class="text-slate-400 mb-6">{{ t('dashboard.no_org.subtitle') }}</p>

      <form @submit.prevent="createOrg" class="text-left space-y-3 mt-8">
        <div>
          <label class="block text-xs text-slate-500 mb-1">{{ t('organizations.form.name_label') }}</label>
          <input v-model="newOrg.name" @input="onNameInput" type="text" :placeholder="t('organizations.form.name_placeholder')" required
            class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
        </div>
        <div>
          <label class="block text-xs text-slate-500 mb-1">{{ t('organizations.form.slug_label') }}</label>
          <div class="flex items-center gap-0">
            <span class="px-3 py-2 bg-slate-900 border border-r-0 border-slate-700 rounded-l-lg text-sm text-slate-500">{{ baseDomain }}/</span>
            <input v-model="newOrg.slug" @input="onSlugInput" type="text" :placeholder="t('organizations.form.slug_placeholder')" required
              class="flex-1 px-3 py-2 bg-slate-800 border border-slate-700 rounded-r-lg text-sm text-white focus:outline-none focus:border-blue-500" />
          </div>
        </div>
        <button type="submit" :disabled="creatingOrg || !newOrg.name.trim() || !newOrg.slug.trim()"
          class="w-full px-4 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50">
          {{ creatingOrg ? t('organizations.form.creating') : t('organizations.create') }}
        </button>
        <div v-if="orgError" class="text-xs text-red-400">{{ orgError }}</div>
      </form>
    </div>

    <!-- Error state -->
    <div v-else-if="error" class="max-w-6xl mx-auto px-8 py-12">
      <div class="bg-red-950/40 border border-red-900/50 rounded-lg p-6 text-red-300 text-sm">
        {{ error }}
      </div>
    </div>

    <!-- Main content -->
    <div v-else class="max-w-6xl mx-auto px-8 py-10 space-y-10">
      <!-- Page header -->
      <div>
        <h1 class="text-2xl font-bold text-slate-100 tracking-tight">{{ t('dashboard.title') }}</h1>
        <p class="text-sm text-slate-500 mt-1">{{ t('dashboard.subtitle') }}</p>
      </div>

      <!-- ============================================ -->
      <!-- 1. HERO ROW                                  -->
      <!-- ============================================ -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <!-- Left: Overall compliance -->
        <router-link :to="orgPath('/documents')" class="bg-slate-900 border border-slate-800 rounded-xl p-6 block hover:border-slate-700 transition-colors">
          <div class="text-xs font-medium text-slate-500 uppercase tracking-wider mb-4">
            {{ t('dashboard.compliance.title') }}
          </div>
          <div class="flex items-end gap-3 mb-4">
            <span class="text-5xl font-bold tabular-nums" :class="complianceColor">
              {{ compliancePercent }}%
            </span>
            <span class="text-sm text-slate-500 mb-1.5">
              {{ t('dashboard.compliance.approved_of_total', { approved: approvedDocs, total: totalDocs }) }}
            </span>
          </div>
          <div class="w-full bg-slate-800 rounded-full h-2.5 overflow-hidden">
            <div
              class="h-full rounded-full transition-all duration-700 ease-out"
              :class="complianceBarColor"
              :style="{ width: compliancePercent + '%' }"
            />
          </div>
          <div class="flex justify-between mt-3 text-xs text-slate-500">
            <span class="flex items-center gap-1.5">
              <span class="w-1.5 h-1.5 rounded-full bg-slate-600"></span>
              {{ t('dashboard.compliance.draft', { count: draftDocs }) }}
            </span>
            <span class="flex items-center gap-1.5">
              <span class="w-1.5 h-1.5 rounded-full bg-amber-500"></span>
              {{ t('dashboard.compliance.in_review', { count: inReviewDocs }) }}
            </span>
            <span class="flex items-center gap-1.5">
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
              {{ t('dashboard.compliance.approved', { count: approvedDocs }) }}
            </span>
          </div>
        </router-link>

        <!-- Right: Module overview -->
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-6">
          <div class="text-xs font-medium text-slate-500 uppercase tracking-wider mb-4">
            {{ t('dashboard.modules.title') }}
          </div>
          <div class="space-y-2.5">
            <router-link v-for="m in moduleOverview" :key="m.route" :to="orgPath(m.route)"
              class="flex items-center justify-between py-1.5 px-2 -mx-2 rounded-lg hover:bg-slate-800/50 transition-colors group">
              <span class="text-sm text-slate-300 group-hover:text-white">{{ m.label }}</span>
              <div class="flex items-center gap-2">
                <span class="text-sm font-bold tabular-nums text-slate-200">{{ m.total }}</span>
                <span v-if="m.alert" class="text-[10px] px-1.5 py-0.5 rounded-full font-semibold" :class="m.alertClass">{{ m.alert }}</span>
              </div>
            </router-link>
          </div>
        </div>
      </div>

      <!-- ============================================ -->
      <!-- BUSINESS IMPACT ANALYSIS                     -->
      <!-- ============================================ -->
      <div v-if="systemsList.length > 0">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider">{{ t('dashboard.bia.title') }}</h2>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          <table class="w-full">
            <thead>
              <tr class="border-b border-slate-800">
                <th class="px-4 py-2.5 text-left text-xs font-medium text-slate-500 uppercase">{{ t('dashboard.bia.header.system') }}</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium text-slate-500 uppercase">{{ t('dashboard.bia.header.criticality') }}</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium text-slate-500 uppercase">{{ t('dashboard.bia.header.rpo') }}</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium text-slate-500 uppercase">{{ t('dashboard.bia.header.rto') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800">
              <tr v-for="sys in systemsList" :key="sys.id" class="hover:bg-slate-800/30 cursor-pointer" @click="router.push(orgPath('/systems'))">
                <td class="px-4 py-2.5 text-sm text-slate-300">{{ sys.name }}</td>
                <td class="px-4 py-2.5"><StatusBadge :status="sys.criticality" group="criticality" /></td>
                <td class="px-4 py-2.5 text-sm text-slate-400">{{ t('dashboard.bia.hours', { count: sys.rpo_hours }) }}</td>
                <td class="px-4 py-2.5 text-sm text-slate-400">{{ t('dashboard.bia.hours', { count: sys.rto_hours }) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- ============================================ -->
      <!-- 2. ACTION ITEMS                              -->
      <!-- ============================================ -->
      <div v-if="hasActionItems">
        <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">
          {{ t('dashboard.attention.title') }}
        </h2>
        <div class="bg-slate-900 border border-slate-800 rounded-xl divide-y divide-slate-800 overflow-hidden">
          <!-- Open reviews -->
          <router-link
            v-if="openReviewCount > 0"
            :to="orgPath('/inbox/reviews')"
            class="flex items-center gap-4 px-6 py-4 hover:bg-slate-800/50 transition-colors group"
          >
            <div class="w-9 h-9 rounded-lg bg-amber-950/60 flex items-center justify-center flex-shrink-0">
              <svg class="w-4 h-4 text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
              </svg>
            </div>
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium text-slate-200">{{ t('dashboard.attention.reviews.title') }}</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ t('dashboard.attention.reviews.detail', openReviewCount) }}</div>
            </div>
            <StatusBadge status="open" />
            <span class="text-xs font-semibold bg-amber-900/60 text-amber-300 px-2.5 py-0.5 rounded-full tabular-nums">
              {{ openReviewCount }}
            </span>
            <svg class="w-4 h-4 text-slate-600 group-hover:text-slate-400 transition-colors flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
            </svg>
          </router-link>

          <!-- High / critical risks -->
          <router-link
            v-if="highRiskCount > 0"
            :to="orgPath('/risks')"
            class="flex items-center gap-4 px-6 py-4 hover:bg-slate-800/50 transition-colors group"
          >
            <div class="w-9 h-9 rounded-lg bg-red-950/60 flex items-center justify-center flex-shrink-0">
              <svg class="w-4 h-4 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
            </div>
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium text-slate-200">{{ t('dashboard.attention.risks.title') }}</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ t('dashboard.attention.risks.detail', highRiskCount) }}</div>
            </div>
            <StatusBadge status="critical" group="severity" />
            <span class="text-xs font-semibold bg-red-900/60 text-red-300 px-2.5 py-0.5 rounded-full tabular-nums">
              {{ highRiskCount }}
            </span>
            <svg class="w-4 h-4 text-slate-600 group-hover:text-slate-400 transition-colors flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
            </svg>
          </router-link>

          <!-- Overdue documents -->
          <router-link
            v-if="overdueCount > 0"
            :to="orgPath('/documents')"
            class="flex items-center gap-4 px-6 py-4 hover:bg-slate-800/50 transition-colors group"
          >
            <div class="w-9 h-9 rounded-lg bg-blue-950/60 flex items-center justify-center flex-shrink-0">
              <svg class="w-4 h-4 text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
              </svg>
            </div>
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium text-slate-200">{{ t('dashboard.attention.overdue_documents.title') }}</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ t('dashboard.attention.overdue_documents.detail', overdueCount) }}</div>
            </div>
            <StatusBadge status="in_review" />
            <span class="text-xs font-semibold bg-blue-900/60 text-blue-300 px-2.5 py-0.5 rounded-full tabular-nums">
              {{ overdueCount }}
            </span>
            <svg class="w-4 h-4 text-slate-600 group-hover:text-slate-400 transition-colors flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
            </svg>
          </router-link>

          <!-- Open incidents -->
          <router-link
            v-if="openIncidentCount > 0"
            :to="orgPath('/incidents')"
            class="flex items-center gap-4 px-6 py-4 hover:bg-slate-800/50 transition-colors group"
          >
            <div class="w-9 h-9 rounded-lg bg-orange-950/60 flex items-center justify-center flex-shrink-0">
              <svg class="w-4 h-4 text-orange-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M11.25 11.25l.041-.02a.75.75 0 011.063.852l-.708 2.836a.75.75 0 001.063.853l.041-.021M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3.75h.008v.008H12V8.25z" />
              </svg>
            </div>
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium text-slate-200">{{ t('dashboard.attention.incidents.title') }}</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ t('dashboard.attention.incidents.detail', openIncidentCount) }}</div>
            </div>
            <span class="text-xs font-semibold bg-orange-900/60 text-orange-300 px-2.5 py-0.5 rounded-full tabular-nums">
              {{ openIncidentCount }}
            </span>
            <svg class="w-4 h-4 text-slate-600 group-hover:text-slate-400 transition-colors flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
            </svg>
          </router-link>

          <!-- Pending changes -->
          <router-link
            v-if="pendingChangeCount > 0"
            :to="orgPath('/changes')"
            class="flex items-center gap-4 px-6 py-4 hover:bg-slate-800/50 transition-colors group"
          >
            <div class="w-9 h-9 rounded-lg bg-sky-950/60 flex items-center justify-center flex-shrink-0">
              <svg class="w-4 h-4 text-sky-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
              </svg>
            </div>
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium text-slate-200">{{ t('dashboard.attention.changes.title') }}</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ t('dashboard.attention.changes.detail', pendingChangeCount) }}</div>
            </div>
            <span class="text-xs font-semibold bg-sky-900/60 text-sky-300 px-2.5 py-0.5 rounded-full tabular-nums">
              {{ pendingChangeCount }}
            </span>
            <svg class="w-4 h-4 text-slate-600 group-hover:text-slate-400 transition-colors flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
            </svg>
          </router-link>

          <!-- Open corrective actions -->
          <router-link
            v-if="openCACount > 0"
            :to="orgPath('/corrective-actions')"
            class="flex items-center gap-4 px-6 py-4 hover:bg-slate-800/50 transition-colors group"
          >
            <div class="w-9 h-9 rounded-lg bg-pink-950/60 flex items-center justify-center flex-shrink-0">
              <svg class="w-4 h-4 text-pink-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M11.42 15.17l-5.1-5.1m0 0L3.44 12.95m2.88-2.88L3.44 7.19m8.58 10.86l5.1-5.1m0 0l2.88 2.88m-2.88-2.88l2.88-2.88M3.75 7.5h16.5" />
              </svg>
            </div>
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium text-slate-200">{{ t('dashboard.attention.corrective_actions.title') }}</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ t('dashboard.attention.corrective_actions.detail', openCACount) }}</div>
            </div>
            <span class="text-xs font-semibold bg-pink-900/60 text-pink-300 px-2.5 py-0.5 rounded-full tabular-nums">
              {{ openCACount }}
            </span>
            <svg class="w-4 h-4 text-slate-600 group-hover:text-slate-400 transition-colors flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
            </svg>
          </router-link>
        </div>
      </div>

      <!-- ============================================ -->
      <!-- 3. SUMMARY CARDS                              -->
      <!-- ============================================ -->
      <div>
        <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">
          {{ t('dashboard.stats.title') }}
        </h2>
        <div class="grid grid-cols-2 lg:grid-cols-5 gap-4">
          <!-- Documents -->
          <router-link :to="orgPath('/documents')" class="bg-slate-900 border border-slate-800 rounded-xl p-5 hover:border-slate-700 transition-colors group">
            <div class="flex items-center gap-3 mb-3">
              <div class="w-8 h-8 rounded-lg bg-blue-950/60 flex items-center justify-center">
                <svg class="w-4 h-4 text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                </svg>
              </div>
              <div class="text-xs text-slate-500 font-medium uppercase tracking-wider">{{ t('dashboard.stats.documents') }}</div>
            </div>
            <div class="text-3xl font-bold text-slate-100 tabular-nums">{{ totalDocs }}</div>
            <div class="text-xs text-slate-600 mt-1">{{ t('dashboard.stats.documents_detail', { count: needsReviewCount }) }}</div>
          </router-link>

          <!-- Risks -->
          <router-link :to="orgPath('/risks')" class="bg-slate-900 border border-slate-800 rounded-xl p-5 hover:border-slate-700 transition-colors group">
            <div class="flex items-center gap-3 mb-3">
              <div class="w-8 h-8 rounded-lg bg-orange-950/60 flex items-center justify-center">
                <svg class="w-4 h-4 text-orange-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <div class="text-xs text-slate-500 font-medium uppercase tracking-wider">{{ t('dashboard.stats.risks') }}</div>
            </div>
            <div class="text-3xl font-bold tabular-nums" :class="highRiskCount > 0 ? 'text-orange-400' : 'text-slate-100'">
              {{ risks.length }}
            </div>
            <div class="text-xs mt-1" :class="highRiskCount > 0 ? 'text-orange-500/70' : 'text-slate-600'">
              {{ highRiskCount > 0 ? t('dashboard.stats.risks_detail', { count: highRiskCount }) : t('dashboard.stats.risks_none') }}
            </div>
          </router-link>

          <!-- Incidents -->
          <router-link :to="orgPath('/incidents')" class="bg-slate-900 border border-slate-800 rounded-xl p-5 hover:border-slate-700 transition-colors group">
            <div class="flex items-center gap-3 mb-3">
              <div class="w-8 h-8 rounded-lg bg-red-950/60 flex items-center justify-center">
                <svg class="w-4 h-4 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <div class="text-xs text-slate-500 font-medium uppercase tracking-wider">{{ t('dashboard.stats.incidents') }}</div>
            </div>
            <div class="text-3xl font-bold tabular-nums" :class="openIncidentCount > 0 ? 'text-red-400' : 'text-slate-100'">
              {{ openIncidentCount }}
            </div>
            <div class="text-xs text-slate-600 mt-1">{{ t('dashboard.stats.incidents_detail') }}</div>
          </router-link>

          <!-- Corrective Actions -->
          <router-link :to="orgPath('/corrective-actions')" class="bg-slate-900 border border-slate-800 rounded-xl p-5 hover:border-slate-700 transition-colors group">
            <div class="flex items-center gap-3 mb-3">
              <div class="w-8 h-8 rounded-lg bg-amber-950/60 flex items-center justify-center">
                <svg class="w-4 h-4 text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4" />
                </svg>
              </div>
              <div class="text-xs text-slate-500 font-medium uppercase tracking-wider">{{ t('dashboard.stats.corrective_actions') }}</div>
            </div>
            <div class="text-3xl font-bold tabular-nums" :class="openCACount > 0 ? 'text-amber-400' : 'text-slate-100'">
              {{ openCACount }}
            </div>
            <div class="text-xs text-slate-600 mt-1">{{ t('dashboard.stats.corrective_actions_detail') }}</div>
          </router-link>

          <!-- Changes -->
          <router-link :to="orgPath('/changes')" class="bg-slate-900 border border-slate-800 rounded-xl p-5 hover:border-slate-700 transition-colors group">
            <div class="flex items-center gap-3 mb-3">
              <div class="w-8 h-8 rounded-lg bg-sky-950/60 flex items-center justify-center">
                <svg class="w-4 h-4 text-sky-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
                </svg>
              </div>
              <div class="text-xs text-slate-500 font-medium uppercase tracking-wider">{{ t('dashboard.stats.changes') }}</div>
            </div>
            <div class="text-3xl font-bold tabular-nums" :class="pendingChangeCount > 0 ? 'text-sky-400' : 'text-slate-100'">
              {{ pendingChangeCount }}
            </div>
            <div class="text-xs text-slate-600 mt-1">{{ pendingChangeCount > 0 ? t('dashboard.stats.changes_detail') : t('dashboard.stats.changes_none') }}</div>
          </router-link>

          <!-- Overdue -->
          <router-link :to="orgPath('/inbox')" class="bg-slate-900 border border-slate-800 rounded-xl p-5 hover:border-slate-700 transition-colors group">
            <div class="flex items-center gap-3 mb-3">
              <div class="w-8 h-8 rounded-lg flex items-center justify-center" :class="totalOverdueCount > 0 ? 'bg-red-950/60' : 'bg-emerald-950/60'">
                <svg class="w-4 h-4" :class="totalOverdueCount > 0 ? 'text-red-400' : 'text-emerald-400'" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <div class="text-xs text-slate-500 font-medium uppercase tracking-wider">{{ t('dashboard.stats.overdue') }}</div>
            </div>
            <div class="text-3xl font-bold tabular-nums" :class="totalOverdueCount > 0 ? 'text-red-400' : 'text-emerald-400'">
              {{ totalOverdueCount }}
            </div>
            <div class="text-xs text-slate-600 mt-1">{{ totalOverdueCount > 0 ? t('dashboard.stats.overdue_detail') : t('dashboard.stats.overdue_none') }}</div>
          </router-link>
        </div>
      </div>

      <!-- ============================================ -->
      <!-- 3b. RISK HEAT MAP                            -->
      <!-- ============================================ -->
      <div v-if="risks.length > 0" class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <HeatMap :items="risks" :title="t('dashboard.heatmap_title')" />
        <OverdueItems :is-admin="isAdmin" @updated="refreshTasks" />
      </div>

      <!-- ============================================ -->
      <!-- 3c. ANNUAL PLAN                              -->
      <!-- ============================================ -->
      <div>
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider">
            {{ t('dashboard.calendar.title') }}
          </h2>
          <div class="flex items-center gap-2">
            <button @click="calendarYear--" :aria-label="t('dashboard.calendar.previous_year')" class="text-xs text-slate-500 hover:text-slate-300 px-1.5 py-0.5 rounded hover:bg-slate-800">&larr;</button> <!-- i18n-ignore -->
            <span class="text-sm font-bold text-slate-300 tabular-nums">{{ calendarYear }}</span>
            <button @click="calendarYear++" :aria-label="t('dashboard.calendar.next_year')" class="text-xs text-slate-500 hover:text-slate-300 px-1.5 py-0.5 rounded hover:bg-slate-800">&rarr;</button> <!-- i18n-ignore -->
          </div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          <!-- Month headers -->
          <div class="grid grid-cols-12 border-b border-slate-800">
            <div v-for="(m, i) in monthNames" :key="i"
              class="px-2 py-2.5 text-center text-[10px] font-semibold uppercase tracking-wider"
              :class="i === currentMonth ? 'bg-blue-950/40 text-blue-400' : 'text-slate-600'">
              {{ m }}
            </div>
          </div>
          <!-- Rows per category -->
          <div v-for="cat in calendarCategories" :key="cat.key" class="grid grid-cols-12 border-b border-slate-800/50 last:border-0">
            <div v-for="(m, i) in 12" :key="i"
              class="px-1.5 py-2 text-center border-r border-slate-800/30 last:border-0 min-h-[2.5rem]"
              :class="i === currentMonth ? 'bg-blue-950/20' : ''">
              <div v-for="item in calendarItems(cat.key, i)" :key="item.id"
                class="text-[9px] px-1 py-0.5 rounded mb-0.5 truncate cursor-default"
                :class="item.overdue ? 'bg-red-900/40 text-red-300' : cat.color"
                :title="item.label">
                {{ item.short }}
              </div>
            </div>
          </div>
          <!-- Legend -->
          <div class="flex items-center gap-4 px-4 py-2.5 border-t border-slate-800 bg-slate-950/50">
            <div v-for="cat in calendarCategories" :key="cat.key" class="flex items-center gap-1.5 text-[10px]">
              <span class="w-2 h-2 rounded-full" :class="cat.dot"></span>
              <span class="text-slate-500">{{ cat.label }}</span>
              <span class="text-slate-600 font-medium">{{ calendarCategoryCount(cat.key) }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- ============================================ -->
      <!-- 4. RECENT ACTIVITY                           -->
      <!-- ============================================ -->
      <div>
        <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">
          {{ t('dashboard.activity.title') }}
        </h2>
        <div v-if="activity.length" class="bg-slate-900 border border-slate-800 rounded-xl divide-y divide-slate-800 overflow-hidden">
          <div v-for="a in activity" :key="a.id" class="flex items-start gap-3 px-5 py-3.5">
            <div class="w-1.5 h-1.5 rounded-full bg-blue-500 flex-shrink-0 mt-1.5" />
            <div class="flex-1 min-w-0">
              <div class="text-sm text-slate-300 leading-snug">{{ a.detail || a.action }}</div>
              <div class="text-xs text-slate-600 mt-0.5">
                <!-- i18n-ignore -->
                {{ a.actor }} &middot; {{ formatDate(a.created_at) }}
              </div>
            </div>
          </div>
        </div>
        <div v-else class="bg-slate-900 border border-slate-800 rounded-xl p-10 text-center">
          <div class="text-sm text-slate-500">{{ t('dashboard.activity.empty') }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import HeatMap from '../components/HeatMap.vue'
import OverdueItems from '../components/OverdueItems.vue'
import { useCurrentOrg, orgEntryURL, isSubdomainMode } from '../composables/useCurrentOrg.js'
import { formatDate, formatMonthShort } from '../composables/useFormat.js'
import { enumLabel } from '../composables/useEnumLabel.js'
import { renderApiError } from '../composables/useApiError.js'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { orgSlug: currentOrgSlug, orgPath } = useCurrentOrg()

const loading = ref(true)
const error = ref(null)
const me = ref(null)

const documents = ref([])
const risks = ref([])
const reviews = ref([])
const tasks = ref([])
const suppliers = ref([])
const progress = ref(null)
const overdueSummary = ref(null)
const activity = ref([])
const systemsList = ref([])
const incidents = ref([])
const correctiveActions = ref([])
const changes = ref([])
const legal = ref([])
const assets = ref([])

// --- Compliance computeds ---

// The org picker shows the caller's role in each org. It is a stored enum, so
// it renders through the shared catalogue rather than the raw column value —
// the raw form is invisible to the raw-text scanner and untranslatable.
const roleLabel = (role) => enumLabel('role', role)

const asArray = (v) => Array.isArray(v) ? v : []
const allDocs = computed(() => asArray(documents.value))
const totalDocs = computed(() => allDocs.value.length)
const approvedDocs = computed(() => allDocs.value.filter(d => d.status === 'approved').length)
const draftDocs = computed(() => allDocs.value.filter(d => d.status === 'draft').length)
const inReviewDocs = computed(() => allDocs.value.filter(d => d.status === 'in_review').length)

const compliancePercent = computed(() => {
  if (totalDocs.value === 0) return 0
  return Math.round((approvedDocs.value / totalDocs.value) * 100)
})

const complianceColor = computed(() => {
  const p = compliancePercent.value
  if (p >= 80) return 'text-emerald-400'
  if (p >= 50) return 'text-amber-400'
  return 'text-red-400'
})

const complianceBarColor = computed(() => {
  const p = compliancePercent.value
  if (p >= 80) return 'bg-emerald-500'
  if (p >= 50) return 'bg-amber-500'
  return 'bg-red-500'
})

// --- Action item computeds ---

const openReviewCount = computed(() =>
  asArray(reviews.value).filter(r => r.status === 'open').length
)

const highRiskCount = computed(() =>
  asArray(risks.value).filter(r => r.current_level === 'high' || r.current_level === 'critical').length
)

const overdueCount = computed(() => {
  const now = new Date()
  return asArray(documents.value).filter(p => {
    if (!p.next_review && p.next_review !== 0) return false
    const d = typeof p.next_review === 'number' ? new Date(p.next_review * 1000) : new Date(p.next_review)
    return d < now
  }).length
})

const openIncidentCount = computed(() =>
  asArray(incidents.value).filter(i => i.status !== 'closed' && i.status !== 'resolved').length
)

const openCACount = computed(() =>
  asArray(correctiveActions.value).filter(ca => ca.status !== 'closed' && ca.status !== 'resolved').length
)

const pendingChangeCount = computed(() =>
  asArray(changes.value).filter(c => c.status === 'proposed').length
)

const needsReviewCount = computed(() =>
  allDocs.value.filter(d => d.status === 'in_review').length
)

const totalOverdueCount = computed(() => {
  if (overdueSummary.value) return overdueSummary.value.total_count || 0
  // Fallback to client-side calculation
  const now = new Date()
  const nowDate = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  let count = 0
  const checkDate = (dateStr) => {
    if (!dateStr && dateStr !== 0) return false
    const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
    return !isNaN(d.getTime()) && d < nowDate
  }
  count += asArray(risks.value).filter(r => checkDate(r.review_date)).length
  count += asArray(suppliers.value).filter(s => checkDate(s.next_review)).length
  count += asArray(legal.value).filter(l => checkDate(l.next_review)).length
  count += asArray(tasks.value).filter((task) => task.status !== 'done' && task.status !== 'cancelled' && checkDate(task.due_date)).length
  return count
})

const isAdmin = computed(() => {
  const role = me.value?.role || ''
  return role === 'admin' || role === 'manager'
})

const hasActionItems = computed(() =>
  openReviewCount.value > 0 || highRiskCount.value > 0 || overdueCount.value > 0 ||
  openIncidentCount.value > 0 || pendingChangeCount.value > 0 || openCACount.value > 0
)

// ---- Annual Calendar ----
// Month names come from Intl via the format seam, not from a hardcoded array
// and not from translation keys — every locale already carries them. A computed
// so switching locale re-labels the twelve columns.
const monthNames = computed(() => Array.from({ length: 12 }, (_, i) => formatMonthShort(i)))
const currentMonth = new Date().getMonth()

// Keys and colours are static; the label is resolved in a computed so it is
// reactive. A module-level array holding literal labels would freeze them in
// whichever locale happened to be active when this module first evaluated.
// Each entry names its message key in full rather than deriving it from `key`:
// a concatenated key is unreachable to keyset extraction and unused-key
// detection, which is why the locale README forbids building one at runtime.
const CALENDAR_CATEGORIES = [
  { key: 'audit', label: 'dashboard.calendar.category.audit', color: 'bg-purple-900/40 text-purple-300', dot: 'bg-purple-400' },
  { key: 'risk', label: 'dashboard.calendar.category.risk', color: 'bg-red-900/40 text-red-300', dot: 'bg-red-400' },
  { key: 'supplier', label: 'dashboard.calendar.category.supplier', color: 'bg-emerald-900/40 text-emerald-300', dot: 'bg-emerald-400' },
  { key: 'legal', label: 'dashboard.calendar.category.legal', color: 'bg-purple-900/40 text-purple-300', dot: 'bg-purple-400' },
  { key: 'task', label: 'dashboard.calendar.category.task', color: 'bg-blue-900/40 text-blue-300', dot: 'bg-blue-400' },
]

// Resolved in a computed, not at module scope: a literal label array would
// freeze in whichever locale was active when this module first evaluated.
const calendarCategories = computed(() =>
  CALENDAR_CATEGORIES.map((c) => ({ ...c, label: t(c.label) })),
)

const calendarYear = ref(new Date().getFullYear())

function parseMonth(dateVal) {
  if (!dateVal && dateVal !== 0) return -1
  const d = typeof dateVal === 'number' ? new Date(dateVal * 1000) : new Date(dateVal)
  if (isNaN(d.getTime())) return -1
  if (d.getFullYear() !== calendarYear.value) return -1
  return d.getMonth()
}

const calendarData = computed(() => {
  const items = []
  const now = new Date()
  // Audits
  for (const a of asArray(audits.value)) {
    const m = parseMonth(a.planned_date)
    if (m >= 0) items.push({ key: 'audit', month: m, id: 'a-' + a.id, label: a.title, short: a.title?.substring(0, 12) || t('dashboard.calendar.untitled_audit'), overdue: m < currentMonth && a.status !== 'completed' })
  }
  // Risk reviews
  for (const r of asArray(risks.value)) {
    const m = parseMonth(r.review_date || r.next_review)
    if (m >= 0) items.push({ key: 'risk', month: m, id: 'r-' + r.id, label: r.title || r.identifier, short: (r.identifier || r.title || '').substring(0, 12), overdue: m < currentMonth })
  }
  // Supplier reviews
  for (const s of asArray(suppliers.value)) {
    const m = parseMonth(s.next_review)
    if (m >= 0) items.push({ key: 'supplier', month: m, id: 's-' + s.id, label: s.name, short: (s.name || '').substring(0, 12), overdue: m < currentMonth })
  }
  // Legal reviews
  for (const l of asArray(legal.value)) {
    const m = parseMonth(l.next_review)
    if (m >= 0) items.push({ key: 'legal', month: m, id: 'l-' + l.id, label: l.title, short: (l.title || '').substring(0, 12), overdue: m < currentMonth })
  }
  // Tasks
  for (const t of asArray(tasks.value)) {
    const m = parseMonth(t.due_date)
    if (m >= 0) items.push({ key: 'task', month: m, id: 't-' + t.id, label: t.title, short: (t.title || '').substring(0, 12), overdue: m < currentMonth && t.status !== 'done' })
  }
  return items
})

function calendarItems(catKey, monthIdx) {
  return calendarData.value.filter(i => i.key === catKey && i.month === monthIdx)
}

function calendarCategoryCount(catKey) {
  return calendarData.value.filter(i => i.key === catKey).length
}

// Also need audits ref
const audits = ref([])

const moduleOverview = computed(() => {
  const mods = []
  const docCount = allDocs.value.length
  const approved = allDocs.value.filter(d => d.status === 'approved').length
  const draftCount = docCount - approved
  mods.push({ label: t('dashboard.modules.documents'), route: '/documents', total: docCount, alert: docCount > 0 && approved < docCount ? t('dashboard.modules.alert.draft', { count: draftCount }) : '', alertClass: 'bg-amber-500/20 text-amber-300' })
  mods.push({ label: t('dashboard.modules.risks'), route: '/risks', total: asArray(risks.value).length, alert: highRiskCount.value > 0 ? t('dashboard.modules.alert.high_critical', { count: highRiskCount.value }) : '', alertClass: 'bg-red-500/20 text-red-300' })
  mods.push({ label: t('dashboard.modules.incidents'), route: '/incidents', total: asArray(incidents.value).length, alert: openIncidentCount.value > 0 ? t('dashboard.modules.alert.open', { count: openIncidentCount.value }) : '', alertClass: 'bg-orange-500/20 text-orange-300' })
  mods.push({ label: t('dashboard.modules.changes'), route: '/changes', total: asArray(changes.value).length, alert: pendingChangeCount.value > 0 ? t('dashboard.modules.alert.pending', { count: pendingChangeCount.value }) : '', alertClass: 'bg-sky-500/20 text-sky-300' })
  mods.push({ label: t('dashboard.modules.corrective_actions'), route: '/corrective-actions', total: asArray(correctiveActions.value).length, alert: openCACount.value > 0 ? t('dashboard.modules.alert.open', { count: openCACount.value }) : '', alertClass: 'bg-pink-500/20 text-pink-300' })
  mods.push({ label: t('dashboard.modules.suppliers'), route: '/suppliers', total: asArray(suppliers.value).length, alert: '', alertClass: '' })
  mods.push({ label: t('dashboard.modules.assets'), route: '/assets', total: asArray(assets.value).length, alert: '', alertClass: '' })
  mods.push({ label: t('dashboard.modules.legal'), route: '/legal', total: asArray(legal.value).length, alert: '', alertClass: '' })
  mods.push({ label: t('dashboard.modules.systems'), route: '/systems', total: asArray(systemsList.value).length, alert: '', alertClass: '' })
  return mods
})

async function refreshTasks() {
  tasks.value = await api.getTasks('', 'open').catch(() => [])
}

// --- Utilities ---

// --- Data fetch ---

const baseDomain = computed(() => window.location.hostname)
const noOrg = ref(false)
const hasOrgsButNoneSelected = ref(false)
const myOrgs = ref([])
const showCreateForm = ref(false)
const newOrg = ref({ name: '', slug: '' })
const slugTouched = ref(false)
const creatingOrg = ref(false)
const orgError = ref('')

function onNameInput() {
  // Auto-generate slug from name unless user has manually edited it
  if (!slugTouched.value) {
    newOrg.value.slug = newOrg.value.name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-|-$/g, '')
  }
}

function onSlugInput() {
  slugTouched.value = true
  // Force lowercase and valid chars
  newOrg.value.slug = newOrg.value.slug
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, '')
}

async function createOrg() {
  if (!newOrg.value.name.trim() || !newOrg.value.slug.trim()) return
  creatingOrg.value = true
  orgError.value = ''
  try {
    const result = await api.postJSON('/api/v1/organizations', {
      name: newOrg.value.name.trim(),
      slug: newOrg.value.slug.trim(),
    })
    // Success — redirect to new org's overview
    const slug = result.slug || newOrg.value.slug.trim()
    router.push('/' + slug + '/overview')
  } catch (e) {
    orgError.value = renderApiError(e) || t('dashboard.error.create_org')
  } finally {
    creatingOrg.value = false
  }
}

async function selectOrg(org) {
  try {
    const result = await api.postJSON('/api/v1/auth/switch-org', { slug: org.slug })
    if (result.token) {
      localStorage.setItem('isms_api_token', result.token)
      // Subdomain hop on hosts that support it; path-based fallback otherwise.
      // Use a hard navigation so the new host context takes effect cleanly.
      if (isSubdomainMode()) {
        window.location.href = orgEntryURL(org.slug, '/overview')
      } else {
        router.push('/' + org.slug + '/overview')
      }
    }
  } catch {
    router.push('/login?org=' + org.slug)
  }
}

onMounted(async () => {
  try {
    // Check if user has an org first
    const meCheck = await api.getMe().catch(() => null)
    if (!meCheck || !meCheck.organization_id) {
      // Check if user has orgs they could switch to
      try {
        const orgs = await api.getMyOrgs()
        const orgList = Array.isArray(orgs) ? orgs : []
        if (orgList.length > 0) {
          myOrgs.value = orgList
          hasOrgsButNoneSelected.value = true
          loading.value = false
          return
        }
      } catch { /* ignore */ }
      noOrg.value = true
      loading.value = false
      return
    }

    me.value = meCheck
    const [allDocFolders, r, rev, tsk, sup, prog, act, sys, inc, ca, chg, aud, leg, ast, od] = await Promise.all([
      api.getAllDocuments().catch(() => []),
      api.getRisks().catch(() => []),
      api.getReviews('').catch(() => []),
      api.getTasks('', 'open').catch(() => []),
      api.getSuppliers().catch(() => []),
      api.getProgress().catch(() => null),
      api.getActivity(10).catch(() => []),
      api.getSystems().catch(() => []),
      api.getIncidents().catch(() => []),
      api.getCorrectiveActions().catch(() => []),
      api.getChanges().catch(() => []),
      api.getAudits().catch(() => []),
      api.getLegal().catch(() => []),
      api.getAssets().catch(() => []),
      api.getOverdue().catch(() => null),
    ])
    // Flatten folder structure into a flat list of documents
    const flatDocs = []
    const collectDocs = (folders) => {
      for (const f of (Array.isArray(folders) ? folders : [])) {
        if (Array.isArray(f.files)) flatDocs.push(...f.files)
        if (Array.isArray(f.subfolders)) collectDocs(f.subfolders)
      }
    }
    collectDocs(allDocFolders)
    documents.value = flatDocs
    risks.value = r || []
    reviews.value = rev || []
    tasks.value = tsk || []
    suppliers.value = sup || []
    progress.value = prog
    activity.value = act || []
    systemsList.value = sys || []
    incidents.value = Array.isArray(inc) ? inc : []
    correctiveActions.value = Array.isArray(ca) ? ca : []
    changes.value = Array.isArray(chg) ? chg : []
    audits.value = Array.isArray(aud) ? aud : []
    legal.value = Array.isArray(leg) ? leg : []
    assets.value = Array.isArray(ast) ? ast : []
    overdueSummary.value = od
  } catch (e) {
    error.value = renderApiError(e) || t('dashboard.error.load')
  } finally {
    loading.value = false
  }
})
</script>
