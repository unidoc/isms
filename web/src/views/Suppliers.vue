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
        <span>{{ error }}</span>
        <RefreshButton :loading="refreshing" @refresh="reload" />
      </div>
    </div>

    <!-- Main content -->
    <div v-else class="max-w-6xl mx-auto px-8 py-10 space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">{{ t('suppliers.title') }}</h1>
          <p class="text-sm text-slate-500 mt-1">{{ t('suppliers.subtitle') }}</p>
        </div>
        <div class="flex gap-2">
          <button v-if="canWrite" @click="showCreateForm = !showCreateForm"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors">
            {{ showCreateForm ? t('common.action.cancel') : t('suppliers.action.add') }}
          </button>
          <SuggestNewButton entityType="supplier" :typeLabel="entityLabel('supplier')" />
        </div>
      </div>

      <!-- Create form (compact modal) -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="showCreateForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="showCreateForm = false" />
        <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
          <div class="flex items-center justify-between mb-2">
            <h2 class="text-sm font-semibold text-slate-200">{{ t('suppliers.create.heading') }}</h2>
            <button @click="showCreateForm = false" class="text-slate-500 hover:text-slate-300">
              <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <div class="space-y-3">
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('suppliers.create.name_label') }}</label>
              <input v-model="newItem.name" autofocus class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('suppliers.create.name_placeholder')" />
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('suppliers.create.type_label') }}</label>
              <div class="flex flex-wrap gap-1.5">
                <button v-for="o in typeOptions" :key="o.value"
                  @click="newItem.supplier_type = newItem.supplier_type === o.value ? '' : o.value"
                  class="px-2.5 py-1 text-[11px] font-medium rounded-lg border transition-colors"
                  :class="newItem.supplier_type === o.value ? 'bg-blue-600/20 text-blue-400 border-blue-500/40' : 'bg-slate-800 text-slate-400 border-slate-700 hover:border-slate-600'">
                  {{ o.label }}
                </button>
              </div>
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('suppliers.create.criticality_label') }}</label>
              <div class="flex flex-wrap gap-1.5">
                <button v-for="c in criticalityOptions" :key="c.value"
                  @click="newItem.criticality = newItem.criticality === c.value ? '' : c.value"
                  class="px-2.5 py-1 text-[11px] font-medium rounded-lg border transition-colors"
                  :class="newItem.criticality === c.value ? criticalityChipColor(c.value) : 'bg-slate-800 text-slate-400 border-slate-700 hover:border-slate-600'">
                  {{ c.label }}
                </button>
              </div>
            </div>
          </div>
          <div class="text-[10px] text-slate-600 mt-1">{{ t('suppliers.create.fill_in_later') }}</div>
          <div class="flex justify-end gap-3 pt-3 border-t border-slate-800">
            <button @click="showCreateForm = false" class="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">{{ t('common.action.cancel') }}</button>
            <button @click="createItem" :disabled="!newItem.name" class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
              {{ t('suppliers.create.submit') }}
            </button>
          </div>
        </div>
      </div>
      </Transition>
      </Teleport>

      <!-- Summary strip -->
      <StatStrip :stats="statusStats" v-model="filterStatus" />

      <!-- Search + Filters -->
      <div class="space-y-3">
        <div class="flex flex-wrap items-center gap-3">
          <RefreshButton :loading="refreshing" @refresh="reload" class="shrink-0" />
          <div class="relative flex-1 max-w-xs">
            <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
            </svg>
            <input v-model="searchQuery" type="text" :placeholder="t('common.placeholder.search')"
              class="w-full pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500" />
          </div>
          <select v-model="filterType" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">{{ t('common.filter.all_types') }}</option>
            <option v-for="o in typeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
          <select v-model="filterCriticality" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">{{ t('common.filter.all_criticality') }}</option>
            <option v-for="c in criticalityOptions" :key="c.value" :value="c.value">{{ c.label }}</option>
          </select>
          <select v-model="filterStatus" class="bg-slate-900 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-400 focus:outline-none focus:border-blue-500">
            <option value="">{{ t('common.filter.all_statuses') }}</option>
            <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
          <button v-if="filterType || filterCriticality || filterStatus || searchQuery"
            @click="filterType = ''; filterCriticality = ''; filterStatus = ''; searchQuery = ''"
            class="text-[10px] text-slate-600 hover:text-slate-400 transition-colors">
            {{ t('common.action.clear') }}
          </button>
          <div class="ml-auto text-xs text-slate-500 tabular-nums">
            {{ t('common.count.total', { count: total }) }}
          </div>
        </div>
      </div>

      <!-- Empty state -->
      <div v-if="suppliers.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
        <div class="text-sm text-slate-500">{{ t('suppliers.filter.empty') }}</div>
      </div>

      <!-- Table -->
      <div v-else class="bg-slate-900 border border-slate-800 rounded-xl overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="border-b border-slate-800">
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('suppliers.table.header.supplier') }}</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('suppliers.table.header.type') }}</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('suppliers.table.header.criticality') }}</th>
              <th class="text-center px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('suppliers.table.header.cia') }}</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('suppliers.table.header.status') }}</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('suppliers.table.header.owner') }}</th>
              <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('suppliers.table.header.next_review') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/50">
            <tr v-for="supplier in suppliers" :key="supplier.id"
              @click="selectItem(supplier)"
              class="hover:bg-slate-800/50 transition-colors cursor-pointer"
              :class="isOverdue(supplier.next_review) ? 'bg-amber-950/10' : ''">
              <td class="px-5 py-3.5">
                <div class="text-sm font-medium text-slate-200">{{ supplier.name }}</div>
              </td>
              <td class="px-5 py-3.5 text-sm text-slate-400">{{ typeLabel(supplier.supplier_type) || '—' }}</td>
              <td class="px-5 py-3.5">
                <span class="inline-block px-2 py-0.5 rounded text-[10px] font-medium" :class="criticalityColor(supplier.criticality)">{{ criticalityLabel(supplier.criticality) }}</span>
              </td>
              <td class="px-5 py-3.5 text-center">
                <div class="flex gap-0.5 justify-center">
                  <span v-if="supplier.confidentiality > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(supplier.confidentiality)" :title="t('suppliers.table.cia_title.confidentiality', { level: ciaLabel(supplier.confidentiality) })">C{{ supplier.confidentiality }}</span>
                  <span v-if="supplier.integrity > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(supplier.integrity)" :title="t('suppliers.table.cia_title.integrity', { level: ciaLabel(supplier.integrity) })">I{{ supplier.integrity }}</span>
                  <span v-if="supplier.availability > 0" class="inline-block px-1 py-0.5 rounded text-[9px] font-medium" :class="ciaColor(supplier.availability)" :title="t('suppliers.table.cia_title.availability', { level: ciaLabel(supplier.availability) })">A{{ supplier.availability }}</span>
                  <span v-if="!supplier.confidentiality && !supplier.integrity && !supplier.availability" class="text-slate-600 text-xs">-</span>
                </div>
              </td>
              <td class="px-5 py-3.5">
                <StatusBadge :status="supplier.status" />
              </td>
              <td class="px-5 py-3.5 text-sm text-slate-400">{{ resolveUserName(supplier.owner) }}</td>
              <td class="px-5 py-3.5 text-sm" :class="isOverdue(supplier.next_review) ? 'text-amber-400 font-medium' : 'text-slate-500'">
                {{ formatDay(supplier.next_review) || '-' }}
              </td>
            </tr>
          </tbody>
        </table>
        <Pagination :page="page" :pageSize="pageSize" :total="total" @update:page="page = $event" @update:pageSize="pageSize = $event" />
      </div>

      <!-- Detail modal -->
      <Teleport to="body">
      <Transition name="modal">
      <div v-if="selectedItem" class="fixed inset-0 z-50 flex items-start justify-center pt-[3vh] px-4">
        <div class="absolute inset-0 bg-black/60" @click="closeDetail" />
        <div class="relative w-full max-w-4xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-h-[90vh] flex flex-col">
          <!-- Header -->
          <div class="flex-shrink-0 border-b border-slate-800 px-6 py-3 flex items-center justify-between gap-4">
            <div class="flex items-center gap-6 min-w-0">
              <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ selectedItem.identifier }}</span>
              <h2 class="text-[15px] font-semibold text-slate-200 truncate">{{ selectedItem.name }}</h2>
            </div>
            <div class="flex items-center gap-2 flex-shrink-0">
              <CopyLinkButton />
              <StatusBadge :status="selectedItem.status" />
              <button @click="closeDetail" class="p-1 rounded-lg text-slate-600 hover:text-slate-300 hover:bg-slate-800 transition-colors">
                <svg class="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          </div>

          <!-- Body -->
          <div class="flex flex-1 min-h-0">
            <!-- Left nav -->
            <nav class="flex-shrink-0 w-28 border-r border-slate-800 py-3">
              <div class="space-y-0.5">
                <button v-for="tab in detailTabs" :key="tab.key" @click="switchDetailTab(tab.key)"
                  class="w-full text-left px-3 py-2 text-xs font-medium transition-colors"
                  :class="detailTab === tab.key ? 'text-blue-400 bg-blue-500/10 border-r-2 border-blue-500' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'">
                  {{ tab.label }}
                </button>
              </div>
            </nav>

            <!-- Content -->
            <div class="flex-1 overflow-y-auto min-h-0">

              <!-- ═══ OVERVIEW ═══ -->
              <template v-if="detailTab === 'overview'">
                <div class="px-6 py-5 space-y-5">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('suppliers.detail.overview') }}</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('overview')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                  </div>
                  <template v-if="editingSection === 'overview'">
                    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div class="sm:col-span-2">
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('suppliers.field.name') }}</label>
                        <input v-model="editForm.name" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('suppliers.field.type') }}</label>
                        <select v-model="editForm.supplier_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option value="">{{ t('common.option.none') }}</option>
                          <option v-for="o in typeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('suppliers.field.criticality') }}</label>
                        <select v-model="editForm.criticality" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="c in criticalityOptions" :key="c.value" :value="c.value">{{ c.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('suppliers.field.status') }}</label>
                        <select v-model="editForm.status" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                          <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('suppliers.field.owner') }}</label>
                        <MemberPicker v-model="editForm.owner" :members="orgMembers" :placeholder="t('common.placeholder.select_owner')" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('suppliers.field.contact') }}</label>
                        <input v-model="editForm.contact" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('suppliers.placeholder.contact')" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('suppliers.field.contract_ref') }}</label>
                        <input v-model="editForm.contract_ref" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('suppliers.placeholder.contract_ref')" />
                      </div>
                      <div>
                        <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('suppliers.field.contract_expiry') }}</label>
                        <input v-model="editForm.contract_expiry" type="date" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                      </div>
                      <div class="flex items-center gap-2 sm:col-span-2 pt-1">
                        <input id="data_access" type="checkbox" v-model="editForm.data_access" class="w-4 h-4 bg-slate-800 border-slate-700 rounded focus:ring-blue-500" />
                        <label for="data_access" class="text-xs text-slate-400">{{ t('suppliers.data_access.checkbox') }}</label>
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div class="space-y-4">
                      <div class="grid grid-cols-2 gap-x-8 gap-y-3 pt-1">
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('suppliers.field.type') }}</div>
                          <div class="text-sm text-slate-300">{{ typeLabel(selectedItem.supplier_type) || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('suppliers.field.criticality') }}</div>
                          <span class="inline-block px-2 py-0.5 rounded text-[10px] font-medium" :class="criticalityColor(selectedItem.criticality)">{{ criticalityLabel(selectedItem.criticality) }}</span>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('suppliers.field.status') }}</div>
                          <StatusBadge :status="selectedItem.status" />
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('suppliers.field.owner') }}</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedItem.owner) }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('suppliers.field.contact') }}</div>
                          <div class="text-sm text-slate-300">{{ selectedItem.contact || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('suppliers.field.contract_ref') }}</div>
                          <div class="text-sm text-slate-300">{{ selectedItem.contract_ref || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('suppliers.field.contract_expiry') }}</div>
                          <div class="text-sm text-slate-300">{{ formatDay(selectedItem.contract_expiry) || '—' }}</div>
                        </div>
                        <div>
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('suppliers.field.data_access') }}</div>
                          <div class="text-sm text-slate-300">{{ selectedItem.data_access ? t('suppliers.data_access.yes') : t('suppliers.data_access.no') }}</div>
                        </div>
                        <div v-if="selectedItem.created_at">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('suppliers.field.created') }}</div>
                          <div class="text-sm text-slate-300">{{ formatDate(selectedItem.created_at) }}</div>
                        </div>
                        <div v-if="selectedItem.created_by">
                          <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">{{ t('suppliers.field.created_by') }}</div>
                          <div class="text-sm text-slate-300">{{ resolveUserName(selectedItem.created_by) }}</div>
                        </div>
                      </div>

                      <!-- Linked systems (reverse direction: systems.supplier_id -> this supplier) -->
                      <div v-if="linkedSystems.length" class="border-t border-slate-800 pt-4">
                        <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-2">
                          {{ t('suppliers.linked_systems.heading', linkedSystems.length) }}
                        </div>
                        <div class="space-y-1">
                          <router-link v-for="sys in linkedSystems" :key="sys.id" :to="orgPath(`/systems/${sys.id}`)"
                            class="flex items-center gap-2 px-3 py-2 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 transition-colors">
                            <span class="text-xs font-mono text-slate-500">{{ sys.identifier }}</span>
                            <span class="text-sm text-slate-200 flex-1 truncate">{{ sys.name }}</span>
                            <StatusBadge :status="sys.status" />
                          </router-link>
                        </div>
                      </div>
                    </div>
                  </template>
                </div>
              </template>

              <!-- ═══ ASSESSMENT ═══ -->
              <template v-if="detailTab === 'assessment'">
                <div class="px-6 py-5 space-y-5">
                  <div v-if="selectedItem.next_review && isOverdue(selectedItem.next_review)" class="flex items-center gap-2 px-3 py-2 rounded-lg text-xs bg-red-950/40 border border-red-900/40 text-red-300">
                    <svg class="w-3.5 h-3.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                    {{ t('common.review.overdue_since', { date: formatDay(selectedItem.next_review) }) }}
                  </div>

                  <!-- CIA scores -->
                  <div class="grid grid-cols-3 gap-4">
                    <div class="bg-slate-800/40 border border-slate-700/50 rounded-lg px-5 py-4 text-center">
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider">{{ t('suppliers.assessment.confidentiality') }}</div>
                      <div v-if="selectedItem.confidentiality" class="text-2xl font-bold tabular-nums mt-1.5" :class="ciaColorText(selectedItem.confidentiality)">{{ selectedItem.confidentiality }}</div>
                      <div v-else class="text-2xl text-slate-700 mt-1.5">—</div>
                      <div v-if="selectedItem.confidentiality" class="text-[10px] text-slate-500 mt-0.5">{{ ciaLabel(selectedItem.confidentiality) }}</div>
                    </div>
                    <div class="bg-slate-800/40 border border-slate-700/50 rounded-lg px-5 py-4 text-center">
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider">{{ t('suppliers.assessment.integrity') }}</div>
                      <div v-if="selectedItem.integrity" class="text-2xl font-bold tabular-nums mt-1.5" :class="ciaColorText(selectedItem.integrity)">{{ selectedItem.integrity }}</div>
                      <div v-else class="text-2xl text-slate-700 mt-1.5">—</div>
                      <div v-if="selectedItem.integrity" class="text-[10px] text-slate-500 mt-0.5">{{ ciaLabel(selectedItem.integrity) }}</div>
                    </div>
                    <div class="bg-slate-800/40 border border-slate-700/50 rounded-lg px-5 py-4 text-center">
                      <div class="text-[10px] text-slate-500 uppercase tracking-wider">{{ t('suppliers.assessment.availability') }}</div>
                      <div v-if="selectedItem.availability" class="text-2xl font-bold tabular-nums mt-1.5" :class="ciaColorText(selectedItem.availability)">{{ selectedItem.availability }}</div>
                      <div v-else class="text-2xl text-slate-700 mt-1.5">—</div>
                      <div v-if="selectedItem.availability" class="text-[10px] text-slate-500 mt-0.5">{{ ciaLabel(selectedItem.availability) }}</div>
                    </div>
                  </div>

                  <div class="flex gap-4 text-[10px] text-slate-500 flex-wrap">
                    <span v-if="selectedItem.last_review">{{ t('common.review.last', { date: formatDay(selectedItem.last_review) }) }}</span>
                    <span v-if="selectedItem.next_review && !isOverdue(selectedItem.next_review)">{{ t('common.review.next', { date: formatDay(selectedItem.next_review) }) }}</span>
                  </div>

                  <!-- Readings -->
                  <div class="border-t border-slate-800 pt-4">
                    <ReadingsPanel entityType="supplier" :entityId="selectedItem.id" :identifier="selectedItem.identifier || ('SUPPLIER-' + selectedItem.id)" :canWrite="canWrite"
                      :currentValues="{ confidentiality: selectedItem.confidentiality, integrity: selectedItem.integrity, availability: selectedItem.availability, criticality: selectedItem.criticality }"
                      @saved="refreshSelectedItem" />
                  </div>
                </div>
              </template>

              <!-- ═══ REVIEWS (periodic supplier verification) ═══ -->
              <template v-if="detailTab === 'reviews'">
                <div class="px-6 py-5">
                  <SupplierReviewPanel :supplierId="selectedItem.id" :canWrite="canWrite" @reviewed="refreshSelectedItem" />
                </div>
              </template>

              <!-- ═══ NOTES ═══ -->
              <template v-if="detailTab === 'notes'">
                <div class="px-6 py-5 space-y-5">
                  <div class="flex items-center justify-between">
                    <div class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ t('suppliers.detail.notes') }}</div>
                    <button v-if="canWrite && !editingSection" @click="editSection('notes')" class="text-[11px] text-slate-600 hover:text-blue-400 transition-colors">{{ t('common.action.edit') }}</button>
                  </div>
                  <template v-if="editingSection === 'notes'">
                    <MarkdownField v-model="editForm.notes" :self-type="'supplier'" :self-id="selectedItem?.identifier || ''" :rows="12" :placeholder="t('suppliers.placeholder.notes')" />
                  </template>
                  <template v-else>
                    <div v-if="selectedItem.notes" class="text-sm doc-prose text-slate-300 leading-relaxed" v-mermaid v-html="renderMd(selectedItem.notes)"></div>
                    <div v-else class="text-sm text-slate-600 italic">{{ t('common.state.no_notes') }}</div>
                  </template>
                </div>
              </template>

              <!-- ═══ LINKS ═══ -->
              <template v-if="detailTab === 'links'">
                <div class="px-6 py-5 space-y-4">
                  <ReferenceManager entityType="supplier" :entityId="selectedItem.identifier || ('SUPPLIER-' + selectedItem.id)" :editable="canWrite" />
                </div>
              </template>

              <!-- ═══ SUGGESTIONS ═══ -->
              <template v-if="detailTab === 'suggestions'">
                <div class="px-6 py-5">
                  <SuggestionPanel entityType="supplier" :entityId="selectedItem.identifier" :canReview="canWrite" @applied="loadSuppliers" />
                </div>
              </template>

              <!-- ═══ COMMENTS ═══ -->
              <template v-if="detailTab === 'comments'">
                <div class="px-6 py-5">
                  <CommentsPanel entityType="supplier" :entityId="selectedItem.identifier" />
                </div>
              </template>

              <!-- ═══ HISTORY ═══ -->
              <template v-if="detailTab === 'history'">
                <div class="px-6 py-5 space-y-6">
                  <HistoryPanel entityType="supplier" :entityId="String(selectedItem.id)" />
                  <div v-if="canWrite" class="border border-red-900/40 rounded-lg p-4 space-y-3">
                    <div class="text-[11px] font-semibold text-red-400 uppercase tracking-wider">{{ t('common.heading.danger_zone') }}</div>
                    <div class="text-xs text-slate-400">{{ t('suppliers.danger.warning') }}</div>
                    <button @click="deleteSelectedItem" class="px-3 py-1.5 text-xs font-medium bg-red-900/40 hover:bg-red-800/60 text-red-300 border border-red-800/50 rounded-lg transition-colors">
                      {{ t('suppliers.danger.delete') }}
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
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import StatStrip from '../components/StatStrip.vue'
import RefreshButton from '../components/RefreshButton.vue'
import MemberPicker from '../components/MemberPicker.vue'
import MarkdownField from '../components/MarkdownField.vue'
import ReferenceManager from '../components/ReferenceManager.vue'
import CopyLinkButton from '../components/CopyLinkButton.vue'
import SuggestionPanel from '../components/SuggestionPanel.vue'
import CommentsPanel from '../components/CommentsPanel.vue'
import HistoryPanel from '../components/HistoryPanel.vue'
import SuggestNewButton from '../components/SuggestNewButton.vue'
import Pagination from '../components/Pagination.vue'
import ReadingsPanel from '../components/ReadingsPanel.vue'
import SupplierReviewPanel from '../components/SupplierReviewPanel.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import { renderMarkdown } from '../composables/useRenderMd.js'
import { useModalEscape } from '../composables/useModalEscape.js'
import { useConfirm } from '../composables/useConfirm.js'
import { useToast } from '../composables/useToast.js'
import { useDirtyEdit } from '../composables/useDirtyEdit.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import { formatDate, formatDay } from '../composables/useFormat.js'
import { useEnumLabel } from '../composables/useEnumLabel.js'
import { renderApiError } from '../composables/useApiError.js'

const { t } = useI18n()
const { enumLabel, entityLabel } = useEnumLabel()
const { confirm: confirmDialog } = useConfirm()
const { show: showError, success: showSaved } = useToast()

const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()

const renderMd = renderMarkdown

const userRole = ref('')
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')

const loading = ref(true)
const refreshing = ref(false)
async function reload() {
  refreshing.value = true
  error.value = null
  try {
    await loadSuppliers()
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    refreshing.value = false
  }
}
const error = ref(null)
const suppliers = ref([])
const orgMembers = ref([])
const saving = ref(false)
const stats = ref({ total: 0, active: 0, under_review: 0, suspended: 0, terminated: 0, critical: 0, high: 0, medium: 0, low: 0 })
const statusStats = computed(() => [
  { key: '', label: t('common.stat.total'), count: stats.value.total, color: 'text-slate-100' },
  { key: 'active', label: statusLabel('active'), count: stats.value.active, color: 'text-emerald-400' },
  { key: 'under_review', label: statusLabel('under_review'), count: stats.value.under_review, color: 'text-amber-400' },
  { key: 'suspended', label: statusLabel('suspended'), count: stats.value.suspended || 0, color: 'text-orange-400' },
  // `critical` here counts criticality, not status — a different family, so a
  // different group.
  { key: 'critical', label: criticalityLabel('critical'), count: stats.value.critical, color: stats.value.critical > 0 ? 'text-red-400' : 'text-slate-100', static: true },
])

const selectedItem = ref(null)
const detailTab = ref('overview')
const editingSection = ref('')
const editForm = ref({})
const { capture: captureEditSnapshot, isDirty } = useDirtyEdit(editForm)

// Reverse list: systems linked to this supplier (systems.supplier_id)
const linkedSystems = ref([])
async function loadLinkedSystems() {
  if (!selectedItem.value) { linkedSystems.value = []; return }
  try {
    const res = await api.listSystemsLinked({ supplier_id: String(selectedItem.value.id), limit: '100' })
    linkedSystems.value = Array.isArray(res?.data) ? res.data : []
  } catch { linkedSystems.value = [] }
}
watch(() => selectedItem.value?.id, loadLinkedSystems, { immediate: true })

const showCreateForm = ref(false)
const newItem = ref({ name: '', supplier_type: '', criticality: 'medium' })

const searchQuery = ref('')
const filterType = ref('')
const filterCriticality = ref('')
const filterStatus = ref('')
const page = ref(1)
const pageSize = ref(50)
const total = ref(0)

// The members each picker offers, matching the supplier_type CHECK widened by
// migrations/20260713000000_v0.7.1.sql. Only the label comes from the shared
// catalogue.
const SUPPLIER_TYPES = [
  'cloud', 'saas', 'consulting', 'hosting', 'infrastructure', 'software',
  'contractor', 'other',
]
const CRITICALITIES = ['critical', 'high', 'medium', 'low']
const STATUSES = ['active', 'under_review', 'suspended', 'terminated']

// Message keys, not labels: a module-scope array of translated strings freezes
// the tab bar in whichever locale was active when this module first evaluated.
// `reviews` is this view's own copy — "Reviews", where Systems says "Access
// Reviews" — so it is not a common.tab.* entry.
const TAB_KEYS = [
  { key: 'overview', label: 'common.tab.overview' },
  { key: 'assessment', label: 'common.tab.assessment' },
  { key: 'reviews', label: 'suppliers.tab.reviews' },
  { key: 'notes', label: 'common.tab.notes' },
  { key: 'links', label: 'common.tab.links' },
  { key: 'suggestions', label: 'common.tab.suggestions' },
  { key: 'comments', label: 'common.tab.comments' },
  { key: 'history', label: 'common.tab.history' },
]
const detailTabs = computed(() => TAB_KEYS.map((tab) => ({ ...tab, label: t(tab.label) })))

// Lookups and option lists live here rather than in the template: a group name
// is a stored identifier, and the raw-text scanner reads a bare quoted word in
// a mustache as unextracted copy.
const statusLabel = (v) => enumLabel('status', v)
const typeLabel = (v) => enumLabel('supplier_type', v)
const criticalityLabel = (v) => enumLabel('criticality', v)

const options = (values, label) => computed(() => values.map((value) => ({ value, label: label(value) })))
const statusOptions = options(STATUSES, statusLabel)
const typeOptions = options(SUPPLIER_TYPES, typeLabel)
const criticalityOptions = options(CRITICALITIES, criticalityLabel)

useModalEscape(showCreateForm)
useModalEscape(computed(() => !!selectedItem.value), closeDetail)

let searchTimer = null
watch([searchQuery], () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { page.value = 1; loadSuppliers() }, 250)
})
watch([filterType, filterCriticality, filterStatus], () => {
  page.value = 1
  loadSuppliers()
})
watch([page, pageSize], () => loadSuppliers())

// Keys written out rather than built from the index: a runtime-composed key
// defeats keyset extraction, which is why the convention forbids it.
const CIA_KEYS = [
  'common.cia.not_assessed',
  'common.cia.insignificant',
  'common.cia.minor',
  'common.cia.moderate',
  'common.cia.major',
  'common.cia.severe',
]
const ciaLabel = (v) => (CIA_KEYS[v] ? t(CIA_KEYS[v]) : t('common.cia.na'))

function ciaColor(v) {
  switch (v) {
    case 5: return 'bg-red-900/60 text-red-300'
    case 4: return 'bg-orange-900/60 text-orange-300'
    case 3: return 'bg-amber-900/60 text-amber-300'
    case 2: return 'bg-emerald-900/60 text-emerald-300'
    case 1: return 'bg-slate-800 text-slate-400'
    default: return 'bg-slate-800 text-slate-500'
  }
}

function ciaColorText(v) {
  switch (v) {
    case 5: return 'text-red-300'
    case 4: return 'text-orange-300'
    case 3: return 'text-amber-300'
    case 2: return 'text-emerald-300'
    case 1: return 'text-slate-400'
    default: return 'text-slate-500'
  }
}

function criticalityColor(c) {
  switch (c) {
    case 'critical': return 'bg-red-900/60 text-red-300'
    case 'high': return 'bg-orange-900/60 text-orange-300'
    case 'medium': return 'bg-amber-900/60 text-amber-300'
    case 'low': return 'bg-emerald-900/60 text-emerald-300'
    default: return 'bg-slate-800 text-slate-500'
  }
}

function criticalityChipColor(c) {
  switch (c) {
    case 'critical': return 'bg-red-600/20 text-red-400 border-red-500/40'
    case 'high': return 'bg-orange-600/20 text-orange-400 border-orange-500/40'
    case 'medium': return 'bg-amber-600/20 text-amber-400 border-amber-500/40'
    case 'low': return 'bg-emerald-600/20 text-emerald-400 border-emerald-500/40'
    default: return 'bg-blue-600/20 text-blue-400 border-blue-500/40'
  }
}

function isOverdue(dateStr) {
  if (!dateStr) return false
  const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
  return d < new Date()
}

function resolveUserName(email) {
  if (!email) return '—'
  const u = orgMembers.value.find(m => m.email === email)
  return u?.name || email
}

async function loadSuppliers() {
  try {
    const params = new URLSearchParams()
    params.set('page', String(page.value))
    params.set('limit', String(pageSize.value))
    if (searchQuery.value) params.set('q', searchQuery.value)
    if (filterType.value) params.set('supplier_type', filterType.value)
    if (filterCriticality.value) params.set('criticality', filterCriticality.value)
    if (filterStatus.value) params.set('status', filterStatus.value)
    const res = await api.fetchRaw(`/api/v1/suppliers?${params.toString()}`)
    suppliers.value = Array.isArray(res?.data) ? res.data : []
    total.value = res?.total || 0
    loadStats()
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function loadStats() {
  try { stats.value = await api.fetchJSON('/api/v1/suppliers/stats') || stats.value } catch { /* silent */ }
}

async function refreshSelectedItem() {
  if (!selectedItem.value) return
  const id = selectedItem.value.id
  try {
    const fresh = await api.fetchJSON(`/api/v1/suppliers/${id}`)
    if (fresh) selectedItem.value = fresh
  } catch { /* silent */ }
  await loadSuppliers()
}

function startEdit(item) {
  editForm.value = {
    name: item.name || '',
    supplier_type: item.supplier_type || '',
    criticality: item.criticality || 'medium',
    status: item.status || 'active',
    owner: item.owner || '',
    contact: item.contact || '',
    contract_ref: item.contract_ref || '',
    contract_expiry: item.contract_expiry
      ? (typeof item.contract_expiry === 'number' ? new Date(item.contract_expiry * 1000).toISOString().substring(0, 10) : String(item.contract_expiry).substring(0, 10))
      : '',
    data_access: !!item.data_access,
    confidentiality: item.confidentiality || 0,
    integrity: item.integrity || 0,
    availability: item.availability || 0,
    notes: item.notes || '',
  }
  captureEditSnapshot()
}

function editSection(section) {
  startEdit(selectedItem.value)
  editingSection.value = section
}

function cancelSection() {
  editingSection.value = ''
  startEdit(selectedItem.value)
}

async function saveSection() {
  saving.value = true
  try {
    await api.putJSON(`/api/v1/suppliers/${selectedItem.value.id}`, { ...editForm.value })
    await refreshSelectedItem()
    editingSection.value = ''
    showSaved(t('common.state.saved'))
  } catch (e) {
    showError(t('suppliers.error.save', { message: renderApiError(e) }))
  } finally {
    saving.value = false
  }
}

async function selectItem(item) {
  if (selectedItem.value?.id === item.id) {
    closeDetail()
    return
  }
  router.push(orgPath(`/suppliers/${item.identifier}`))
}

async function openItemFromRoute(id) {
  let item = suppliers.value.find(s => s.identifier === id || String(s.id) === String(id))
  if (!item) {
    try { item = await api.fetchJSON(`/api/v1/suppliers/${encodeURIComponent(id)}`) } catch { return }
  }
  if (!item) return
  selectedItem.value = item
  detailTab.value = 'overview'
  startEdit(item)
}

async function switchDetailTab(key) {
  if (editingSection.value && isDirty()) {
    const ok = await confirmDialog({
      message: t('suppliers.dirty.switch_tab'),
      variant: 'danger',
      confirmLabel: t('suppliers.dirty.discard'),
    })
    if (!ok) return
  }
  detailTab.value = key
  editingSection.value = ''
}

async function closeDetail() {
  if (editingSection.value && isDirty()) {
    const ok = await confirmDialog({
      message: t('suppliers.dirty.close'),
      variant: 'danger',
      confirmLabel: t('suppliers.dirty.discard'),
    })
    if (!ok) return
  }
  router.push(orgPath('/suppliers'))
}

async function createItem() {
  try {
    const payload = { ...newItem.value }
    const created = await api.postJSON('/api/v1/suppliers', payload)
    showCreateForm.value = false
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur()
    newItem.value = { name: '', supplier_type: '', criticality: 'medium' }
    await loadSuppliers()
    if (created?.id && !suppliers.value.find(s => s.id === created.id)) {
      suppliers.value = [created, ...suppliers.value]
    }
    // Drop user into detail modal in edit mode on Overview to keep filling things in.
    if (created && created.id) {
      let fresh = created
      try { fresh = await api.fetchJSON(`/api/v1/suppliers/${created.id}`) } catch { /* fall back */ }
      selectedItem.value = fresh
      detailTab.value = 'overview'
      startEdit(fresh)
      editingSection.value = 'overview'
      router.push(orgPath(`/suppliers/${fresh.id}`))
    }
  } catch (e) {
    showError(t('suppliers.error.create', { message: renderApiError(e) }))
  }
}

async function deleteSelectedItem() {
  if (!selectedItem.value) return
  const ok = await confirmDialog({ message: t('suppliers.danger.confirm'), variant: 'danger', confirmLabel: t('common.action.delete') })
  if (!ok) return
  try {
    await api.deleteJSON(`/api/v1/suppliers/${selectedItem.value.id}`)
    closeDetail()
    await loadSuppliers()
  } catch (e) {
    showError(t('suppliers.error.delete', { message: renderApiError(e) }))
  }
}

onMounted(async () => {
  try { const me = await api.getMe(); userRole.value = me?.role || '' } catch {}
  try {
    const [, users] = await Promise.all([
      loadSuppliers(),
      api.getUsers().catch(() => []),
    ])
    orgMembers.value = users || []
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    loading.value = false
  }
  if (route.params.id) openItemFromRoute(route.params.id)
})

watch(() => route.params.id, (id) => {
  if (!id) {
    selectedItem.value = null
    detailTab.value = 'overview'
    editingSection.value = ''
    return
  }
  if (selectedItem.value && (selectedItem.value.identifier === id || String(selectedItem.value.id) === String(id))) return
  openItemFromRoute(id)
})
</script>
