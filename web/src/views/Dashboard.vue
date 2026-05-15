<template>
  <div class="min-h-full">
    <!-- Loading state -->
    <div v-if="loading" class="flex items-center justify-center h-96">
      <div class="text-slate-400 text-sm">Loading dashboard...</div>
    </div>

    <!-- Has orgs but none selected — org picker -->
    <div v-else-if="hasOrgsButNoneSelected" class="max-w-lg mx-auto px-8 py-24 text-center">
      <div class="w-16 h-16 rounded-2xl bg-gradient-to-br from-blue-500 to-blue-700 flex items-center justify-center mx-auto mb-6">
        <svg class="w-8 h-8 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
        </svg>
      </div>
      <h2 class="text-xl font-bold text-white mb-2">Select an organization</h2>
      <p class="text-slate-400 mb-8">Choose which organization to work in.</p>

      <div class="space-y-2">
        <button v-for="org in myOrgs" :key="org.slug" @click="selectOrg(org)"
          class="w-full flex items-center justify-between px-4 py-3 bg-slate-900 border border-slate-800 rounded-xl hover:border-slate-600 transition-colors text-left">
          <div>
            <div class="text-sm font-medium text-white">{{ org.name }}</div>
            <div class="text-xs text-slate-500">{{ org.slug }}</div>
          </div>
          <span class="text-[10px] px-1.5 py-0.5 rounded-full" :class="org.role === 'admin' ? 'bg-purple-500/20 text-purple-300' : 'bg-slate-500/20 text-slate-400'">{{ org.role }}</span>
        </button>
      </div>

      <div class="mt-6 pt-6 border-t border-slate-800">
        <button @click="showCreateForm = true" v-if="!showCreateForm" class="text-sm text-slate-400 hover:text-white transition-colors">
          + Create a new organization
        </button>
        <form v-if="showCreateForm" @submit.prevent="createOrg" class="text-left space-y-3">
          <div>
            <label class="block text-xs text-slate-500 mb-1">Organization name</label>
            <input v-model="newOrg.name" @input="onNameInput" type="text" placeholder="Acme Corp" required
              class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">URL slug</label>
            <div class="flex items-center gap-0">
              <span class="px-3 py-2 bg-slate-900 border border-r-0 border-slate-700 rounded-l-lg text-sm text-slate-500">{{ baseDomain }}/</span>
              <input v-model="newOrg.slug" @input="onSlugInput" type="text" placeholder="acme" required
                class="flex-1 px-3 py-2 bg-slate-800 border border-slate-700 rounded-r-lg text-sm text-white focus:outline-none focus:border-blue-500" />
            </div>
          </div>
          <div class="flex gap-2">
            <button type="submit" :disabled="creatingOrg || !newOrg.name.trim() || !newOrg.slug.trim()"
              class="flex-1 px-4 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50">
              {{ creatingOrg ? 'Creating...' : 'Create organization' }}
            </button>
            <button type="button" @click="showCreateForm = false" class="px-4 py-2.5 text-sm text-slate-400 hover:text-white transition-colors">Cancel</button>
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
      <h2 class="text-xl font-bold text-white mb-2">You're not in any organization</h2>
      <p class="text-slate-400 mb-6">Create your organization to get started, or wait for an invite.</p>

      <form @submit.prevent="createOrg" class="text-left space-y-3 mt-8">
        <div>
          <label class="block text-xs text-slate-500 mb-1">Organization name</label>
          <input v-model="newOrg.name" @input="onNameInput" type="text" placeholder="Acme Corp" required
            class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
        </div>
        <div>
          <label class="block text-xs text-slate-500 mb-1">URL slug</label>
          <div class="flex items-center gap-0">
            <span class="px-3 py-2 bg-slate-900 border border-r-0 border-slate-700 rounded-l-lg text-sm text-slate-500">{{ baseDomain }}/</span>
            <input v-model="newOrg.slug" @input="onSlugInput" type="text" placeholder="acme" required
              class="flex-1 px-3 py-2 bg-slate-800 border border-slate-700 rounded-r-lg text-sm text-white focus:outline-none focus:border-blue-500" />
          </div>
        </div>
        <button type="submit" :disabled="creatingOrg || !newOrg.name.trim() || !newOrg.slug.trim()"
          class="w-full px-4 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50">
          {{ creatingOrg ? 'Creating...' : 'Create organization' }}
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
        <h1 class="text-2xl font-bold text-slate-100 tracking-tight">Overview</h1>
        <p class="text-sm text-slate-500 mt-1">ISO 27001 ISMS compliance and implementation overview</p>
      </div>

      <!-- ============================================ -->
      <!-- 1. HERO ROW                                  -->
      <!-- ============================================ -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <!-- Left: Overall compliance -->
        <router-link :to="orgPath('/documents')" class="bg-slate-900 border border-slate-800 rounded-xl p-6 block hover:border-slate-700 transition-colors">
          <div class="text-xs font-medium text-slate-500 uppercase tracking-wider mb-4">
            Overall Compliance
          </div>
          <div class="flex items-end gap-3 mb-4">
            <span class="text-5xl font-bold tabular-nums" :class="complianceColor">
              {{ compliancePercent }}%
            </span>
            <span class="text-sm text-slate-500 mb-1.5">
              {{ approvedDocs }} of {{ totalDocs }} documents approved
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
              {{ draftDocs }} draft
            </span>
            <span class="flex items-center gap-1.5">
              <span class="w-1.5 h-1.5 rounded-full bg-amber-500"></span>
              {{ inReviewDocs }} in review
            </span>
            <span class="flex items-center gap-1.5">
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
              {{ approvedDocs }} approved
            </span>
          </div>
        </router-link>

        <!-- Right: Module overview -->
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-6">
          <div class="text-xs font-medium text-slate-500 uppercase tracking-wider mb-4">
            Module Overview
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
          <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider">Business Impact Analysis</h2>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          <table class="w-full">
            <thead>
              <tr class="border-b border-slate-800">
                <th class="px-4 py-2.5 text-left text-xs font-medium text-slate-500 uppercase">System</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium text-slate-500 uppercase">Criticality</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium text-slate-500 uppercase">RPO</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium text-slate-500 uppercase">RTO</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800">
              <tr v-for="sys in systemsList" :key="sys.id" class="hover:bg-slate-800/30 cursor-pointer" @click="router.push(orgPath('/systems'))">
                <td class="px-4 py-2.5 text-sm text-slate-300">{{ sys.name }}</td>
                <td class="px-4 py-2.5"><StatusBadge :status="sys.criticality" /></td>
                <td class="px-4 py-2.5 text-sm text-slate-400">{{ sys.rpo_hours }}h</td>
                <td class="px-4 py-2.5 text-sm text-slate-400">{{ sys.rto_hours }}h</td>
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
          Needs Your Attention
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
              <div class="text-sm font-medium text-slate-200">Open reviews</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ openReviewCount }} review{{ openReviewCount !== 1 ? 's' : '' }} awaiting response</div>
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
              <div class="text-sm font-medium text-slate-200">High &amp; critical risks</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ highRiskCount }} risk{{ highRiskCount !== 1 ? 's' : '' }} requiring treatment or escalation</div>
            </div>
            <StatusBadge status="critical" />
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
              <div class="text-sm font-medium text-slate-200">Overdue documents</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ overdueCount }} polic{{ overdueCount !== 1 ? 'ies' : 'y' }} past review date</div>
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
              <div class="text-sm font-medium text-slate-200">Open incidents</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ openIncidentCount }} incident{{ openIncidentCount !== 1 ? 's' : '' }} need investigation</div>
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
              <div class="text-sm font-medium text-slate-200">Changes awaiting approval</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ pendingChangeCount }} change{{ pendingChangeCount !== 1 ? 's' : '' }} proposed</div>
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
              <div class="text-sm font-medium text-slate-200">Open corrective actions</div>
              <div class="text-xs text-slate-500 mt-0.5">{{ openCACount }} corrective action{{ openCACount !== 1 ? 's' : '' }} in progress</div>
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
          Quick Stats
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
              <div class="text-xs text-slate-500 font-medium uppercase tracking-wider">Documents</div>
            </div>
            <div class="text-3xl font-bold text-slate-100 tabular-nums">{{ totalDocs }}</div>
            <div class="text-xs text-slate-600 mt-1">{{ needsReviewCount }} need review</div>
          </router-link>

          <!-- Risks -->
          <router-link :to="orgPath('/risks')" class="bg-slate-900 border border-slate-800 rounded-xl p-5 hover:border-slate-700 transition-colors group">
            <div class="flex items-center gap-3 mb-3">
              <div class="w-8 h-8 rounded-lg bg-orange-950/60 flex items-center justify-center">
                <svg class="w-4 h-4 text-orange-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <div class="text-xs text-slate-500 font-medium uppercase tracking-wider">Risks</div>
            </div>
            <div class="text-3xl font-bold tabular-nums" :class="highRiskCount > 0 ? 'text-orange-400' : 'text-slate-100'">
              {{ risks.length }}
            </div>
            <div class="text-xs mt-1" :class="highRiskCount > 0 ? 'text-orange-500/70' : 'text-slate-600'">
              {{ highRiskCount > 0 ? highRiskCount + ' high/critical' : 'All within tolerance' }}
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
              <div class="text-xs text-slate-500 font-medium uppercase tracking-wider">Incidents</div>
            </div>
            <div class="text-3xl font-bold tabular-nums" :class="openIncidentCount > 0 ? 'text-red-400' : 'text-slate-100'">
              {{ openIncidentCount }}
            </div>
            <div class="text-xs text-slate-600 mt-1">open incidents</div>
          </router-link>

          <!-- Corrective Actions -->
          <router-link :to="orgPath('/corrective-actions')" class="bg-slate-900 border border-slate-800 rounded-xl p-5 hover:border-slate-700 transition-colors group">
            <div class="flex items-center gap-3 mb-3">
              <div class="w-8 h-8 rounded-lg bg-amber-950/60 flex items-center justify-center">
                <svg class="w-4 h-4 text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4" />
                </svg>
              </div>
              <div class="text-xs text-slate-500 font-medium uppercase tracking-wider">CAs</div>
            </div>
            <div class="text-3xl font-bold tabular-nums" :class="openCACount > 0 ? 'text-amber-400' : 'text-slate-100'">
              {{ openCACount }}
            </div>
            <div class="text-xs text-slate-600 mt-1">open actions</div>
          </router-link>

          <!-- Changes -->
          <router-link :to="orgPath('/changes')" class="bg-slate-900 border border-slate-800 rounded-xl p-5 hover:border-slate-700 transition-colors group">
            <div class="flex items-center gap-3 mb-3">
              <div class="w-8 h-8 rounded-lg bg-sky-950/60 flex items-center justify-center">
                <svg class="w-4 h-4 text-sky-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
                </svg>
              </div>
              <div class="text-xs text-slate-500 font-medium uppercase tracking-wider">Changes</div>
            </div>
            <div class="text-3xl font-bold tabular-nums" :class="pendingChangeCount > 0 ? 'text-sky-400' : 'text-slate-100'">
              {{ pendingChangeCount }}
            </div>
            <div class="text-xs text-slate-600 mt-1">{{ pendingChangeCount > 0 ? 'awaiting approval' : 'none pending' }}</div>
          </router-link>

          <!-- Overdue -->
          <router-link :to="orgPath('/inbox')" class="bg-slate-900 border border-slate-800 rounded-xl p-5 hover:border-slate-700 transition-colors group">
            <div class="flex items-center gap-3 mb-3">
              <div class="w-8 h-8 rounded-lg flex items-center justify-center" :class="totalOverdueCount > 0 ? 'bg-red-950/60' : 'bg-emerald-950/60'">
                <svg class="w-4 h-4" :class="totalOverdueCount > 0 ? 'text-red-400' : 'text-emerald-400'" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <div class="text-xs text-slate-500 font-medium uppercase tracking-wider">Overdue</div>
            </div>
            <div class="text-3xl font-bold tabular-nums" :class="totalOverdueCount > 0 ? 'text-red-400' : 'text-emerald-400'">
              {{ totalOverdueCount }}
            </div>
            <div class="text-xs text-slate-600 mt-1">{{ totalOverdueCount > 0 ? 'items past due' : 'all on track' }}</div>
          </router-link>
        </div>
      </div>

      <!-- ============================================ -->
      <!-- 3b. RISK HEAT MAP                            -->
      <!-- ============================================ -->
      <div v-if="risks.length > 0" class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <HeatMap :items="risks" title="Risk Map" />
        <OverdueItems :is-admin="isAdmin" @updated="refreshTasks" />
      </div>

      <!-- ============================================ -->
      <!-- 3c. ANNUAL PLAN                              -->
      <!-- ============================================ -->
      <div>
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider">
            Annual Plan
          </h2>
          <div class="flex items-center gap-2">
            <button @click="calendarYear--" class="text-xs text-slate-500 hover:text-slate-300 px-1.5 py-0.5 rounded hover:bg-slate-800">&larr;</button>
            <span class="text-sm font-bold text-slate-300 tabular-nums">{{ calendarYear }}</span>
            <button @click="calendarYear++" class="text-xs text-slate-500 hover:text-slate-300 px-1.5 py-0.5 rounded hover:bg-slate-800">&rarr;</button>
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
          Recent Activity
        </h2>
        <div v-if="activity.length" class="bg-slate-900 border border-slate-800 rounded-xl divide-y divide-slate-800 overflow-hidden">
          <div v-for="a in activity" :key="a.id" class="flex items-start gap-3 px-5 py-3.5">
            <div class="w-1.5 h-1.5 rounded-full bg-blue-500 flex-shrink-0 mt-1.5" />
            <div class="flex-1 min-w-0">
              <div class="text-sm text-slate-300 leading-snug">{{ a.detail || a.action }}</div>
              <div class="text-xs text-slate-600 mt-0.5">
                {{ a.actor }} &middot; {{ formatDate(a.created_at) }}
              </div>
            </div>
          </div>
        </div>
        <div v-else class="bg-slate-900 border border-slate-800 rounded-xl p-10 text-center">
          <div class="text-sm text-slate-500">Activity will appear as the team works</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import HeatMap from '../components/HeatMap.vue'
import OverdueItems from '../components/OverdueItems.vue'
import { useCurrentOrg, orgEntryURL, isSubdomainMode } from '../composables/useCurrentOrg.js'

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
  count += asArray(tasks.value).filter(t => t.status !== 'done' && t.status !== 'cancelled' && checkDate(t.due_date)).length
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
const monthNames = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const currentMonth = new Date().getMonth()

const calendarCategories = [
  { key: 'audit', label: 'Audits', color: 'bg-purple-900/40 text-purple-300', dot: 'bg-purple-400' },
  { key: 'risk', label: 'Risk Reviews', color: 'bg-red-900/40 text-red-300', dot: 'bg-red-400' },
  { key: 'supplier', label: 'Supplier Reviews', color: 'bg-emerald-900/40 text-emerald-300', dot: 'bg-emerald-400' },
  { key: 'legal', label: 'Legal Reviews', color: 'bg-purple-900/40 text-purple-300', dot: 'bg-purple-400' },
  { key: 'task', label: 'Tasks', color: 'bg-blue-900/40 text-blue-300', dot: 'bg-blue-400' },
]

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
    if (m >= 0) items.push({ key: 'audit', month: m, id: 'a-' + a.id, label: a.title, short: a.title?.substring(0, 12) || 'Audit', overdue: m < currentMonth && a.status !== 'completed' })
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
  const approvedDocs = allDocs.value.filter(d => d.status === 'approved').length
  mods.push({ label: 'Documents', route: '/documents', total: docCount, alert: docCount > 0 && approvedDocs < docCount ? `${docCount - approvedDocs} draft` : '', alertClass: 'bg-amber-500/20 text-amber-300' })
  mods.push({ label: 'Risks', route: '/risks', total: asArray(risks.value).length, alert: highRiskCount.value > 0 ? `${highRiskCount.value} high/critical` : '', alertClass: 'bg-red-500/20 text-red-300' })
  mods.push({ label: 'Incidents', route: '/incidents', total: asArray(incidents.value).length, alert: openIncidentCount.value > 0 ? `${openIncidentCount.value} open` : '', alertClass: 'bg-orange-500/20 text-orange-300' })
  mods.push({ label: 'Changes', route: '/changes', total: asArray(changes.value).length, alert: pendingChangeCount.value > 0 ? `${pendingChangeCount.value} pending` : '', alertClass: 'bg-sky-500/20 text-sky-300' })
  mods.push({ label: 'Corrective Actions', route: '/corrective-actions', total: asArray(correctiveActions.value).length, alert: openCACount.value > 0 ? `${openCACount.value} open` : '', alertClass: 'bg-pink-500/20 text-pink-300' })
  mods.push({ label: 'Suppliers', route: '/suppliers', total: asArray(suppliers.value).length, alert: '', alertClass: '' })
  mods.push({ label: 'Assets', route: '/assets', total: asArray(assets.value).length, alert: '', alertClass: '' })
  mods.push({ label: 'Legal', route: '/legal', total: asArray(legal.value).length, alert: '', alertClass: '' })
  mods.push({ label: 'Systems', route: '/systems', total: asArray(systemsList.value).length, alert: '', alertClass: '' })
  return mods
})

async function refreshTasks() {
  tasks.value = await api.getTasks('', 'open').catch(() => [])
}

// --- Implementation progress ---

const progressTypes = computed(() => {
  if (!progress.value || !progress.value.by_type) {
    return [
      { key: 'document', label: 'Documents', total: asArray(documents.value).length, implemented: 0, pct: 0 },
    ]
  }
  const bt = progress.value.by_type
  // Dynamically iterate over whatever folder types exist in by_type
  return Object.keys(bt).map(key => ({
    key,
    label: key.charAt(0).toUpperCase() + key.slice(1) + 's',
    ...typeStats(bt[key]),
  }))
})

function typeStats(t) {
  if (!t) return { total: 0, implemented: 0, pct: 0 }
  const impl = (t.implemented || 0) + (t.verified || 0)
  return {
    total: t.total || 0,
    implemented: impl,
    pct: t.total ? Math.round((impl / t.total) * 100) : 0,
  }
}

// --- Utilities ---

function formatDate(d) {
  if (!d && d !== 0) return ''
  const dt = typeof d === 'number' ? new Date(d * 1000) : new Date(d)
  return dt.toLocaleDateString('en-GB', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  })
}

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
    orgError.value = e.message || 'Failed to create organization'
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
    const [allDocFolders, r, rev, t, sup, prog, act, sys, inc, ca, chg, aud, leg, ast, od] = await Promise.all([
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
    tasks.value = t || []
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
    error.value = e.message
  } finally {
    loading.value = false
  }
})
</script>
