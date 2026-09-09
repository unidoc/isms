<template>
  <div class="min-h-full">
    <!-- Loading -->
    <div v-if="loading" class="max-w-4xl mx-auto px-8 py-10">
      <ListSkeleton :rows="5" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="max-w-4xl mx-auto px-8 py-12">
      <div class="bg-red-950/40 border border-red-900/50 rounded-lg p-6 text-red-300 text-sm flex items-center justify-between gap-4">
        <span>{{ error }}</span>
        <RefreshButton :loading="refreshing" @refresh="reload" />
      </div>
    </div>

    <!-- Main content -->
    <div v-else class="max-w-4xl mx-auto px-8 py-10 space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">{{ t('inbox.title') }}</h1>
          <p class="text-sm text-slate-500 mt-1">{{ t('inbox.subtitle', { count: totalActionItems }, totalActionItems) }}</p>
        </div>
        <div class="flex items-center gap-2">
          <RefreshButton :loading="refreshing" @refresh="reload" />
          <button @click="markAllNotificationsRead"
            class="px-3 py-1.5 text-xs text-slate-400 hover:text-white hover:bg-slate-800 rounded-lg border border-slate-800 transition-colors">
            {{ t('inbox.mark_all_read') }}
          </button>
        </div>
      </div>

      <!-- Tab bar -->
      <div class="flex gap-1 bg-slate-900 border border-slate-800 rounded-lg p-1">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          @click="activeTab = tab.key"
          :data-testid="activeTab === tab.key ? 'active-tab' : undefined"
          class="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-md transition-colors"
          :class="activeTab === tab.key
            ? 'bg-slate-800 text-white'
            : 'text-slate-500 hover:text-slate-300'"
        >
          {{ tab.label }}
          <span
            v-if="tab.count > 0"
            class="inline-flex items-center justify-center min-w-[20px] h-5 px-1.5 text-[11px] font-semibold rounded-full"
            :class="activeTab === tab.key
              ? 'bg-slate-700 text-slate-200'
              : 'bg-slate-800 text-slate-400'"
          >
            {{ tab.count }}
          </span>
        </button>
      </div>

      <!-- ===================== COMMENTS TAB ===================== -->
      <template v-if="activeTab === 'comments'">
        <div v-if="openComments.length === 0" class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
          <div class="text-slate-500 text-sm">{{ t('inbox.comments.empty') }}</div>
        </div>
        <div v-else class="space-y-2">
          <div v-for="c in openComments" :key="c.id"
            class="bg-slate-900 border border-slate-800 rounded-lg p-4 hover:border-slate-700 transition-colors">
            <div class="flex items-center gap-2 mb-2">
              <div class="w-6 h-6 rounded-full bg-blue-600 flex items-center justify-center text-[10px] font-bold text-white flex-shrink-0">
                {{ (c.author || '?').charAt(0).toUpperCase() }}
              </div>
              <span class="text-sm font-medium text-slate-200">{{ c.author }}</span>
              <span class="text-xs text-slate-600">{{ formatDate(c.created_at) }}</span>
              <span v-if="c.suggestion_body" class="text-[10px] font-semibold px-1.5 py-0.5 rounded-full bg-amber-800/40 text-amber-300">{{ t('inbox.comments.suggestion_badge') }}</span>
              <router-link :to="orgPath(`/documents/${encodeURIComponent(c.document_id)}`)"
                class="ml-auto text-xs text-blue-400 hover:text-blue-300 bg-slate-800 px-2 py-0.5 rounded font-mono" @click.stop>{{ c.document_id }}</router-link>
            </div>
            <div class="flex items-center gap-1.5 mb-1.5">
              <span v-if="c.paragraph_index != null" class="text-[10px] text-blue-400 font-medium flex items-center gap-1">
                <svg class="w-2.5 h-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
                </svg>
                {{ t('inbox.comments.scope_inline') }}
              </span>
              <span v-else class="text-[10px] text-slate-500 font-medium flex items-center gap-1">
                <svg class="w-2.5 h-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                </svg>
                {{ t('inbox.comments.scope_document') }}
              </span>
            </div>
            <div class="text-sm text-slate-300 leading-relaxed">{{ c.body }}</div>
            <div v-if="c.quote" class="mt-2 text-xs text-slate-600 border-l-2 border-slate-700 pl-2 italic truncate">"{{ c.quote }}"</div>
            <div class="mt-3 flex gap-2">
              <button @click="goToComment(c)" class="text-xs text-blue-400 hover:text-blue-300 cursor-pointer">{{ t('inbox.comments.view_in_document') }}</button>
              <button v-if="canResolveComment(c)" @click="resolveFromInbox(c.id)" class="text-xs text-slate-500 hover:text-emerald-400 ml-auto">{{ t('common.action.resolve') }}</button>
            </div>
          </div>
        </div>
      </template>

      <!-- ===================== REVIEWS TAB ===================== -->
      <template v-if="activeTab === 'reviews'">
        <!-- Empty state -->
        <div v-if="reviews.length === 0" class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
          <svg class="w-10 h-10 text-slate-700 mx-auto mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
          </svg>
          <div class="text-sm text-slate-500">{{ t('inbox.reviews.empty') }}</div>
        </div>

        <!-- Review cards -->
        <div v-else class="space-y-3">
          <div
            v-for="review in reviews"
            :key="review.id"
            class="bg-slate-900 border border-slate-800 rounded-lg overflow-hidden"
          >
            <!-- Review header -->
            <div
              role="button" tabindex="0"
              @click="toggleReviewExpand(review.id)"
              @keydown.enter="toggleReviewExpand(review.id)"
              @keydown.space.prevent="toggleReviewExpand(review.id)"
              :aria-expanded="expandedReviewId === review.id"
              class="flex items-center gap-4 px-5 py-4 hover:bg-slate-800/30 transition-colors cursor-pointer focus:outline-none focus:ring-1 focus:ring-blue-500/50 rounded"
            >
              <StatusBadge :status="review.status" />
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium text-slate-200">{{ review.title || review.document_id || t('inbox.reviews.untitled') }}</span>
                  <span v-if="review.round > 1" class="text-[10px] px-1.5 py-0.5 rounded-full bg-slate-700/50 text-slate-400 font-medium">{{ t('common.label.round', { round: review.round }) }}</span>
                </div>
                <div class="text-xs text-slate-500 mt-0.5">
                  {{ t('inbox.reviews.requested_by', { name: review.requested_by || t('inbox.reviews.unknown_requester') }) }}
                  <span class="mx-1.5 text-slate-700">|</span>
                  {{ formatDate(review.created_at) }}
                  <span v-if="review.version" class="mx-1.5 text-slate-700">|</span>
                  <span v-if="review.version" class="text-slate-400">v{{ review.version }}</span>
                </div>
              </div>
              <div v-if="reviewCommentCounts[review.id]" class="flex items-center gap-1 text-xs text-slate-500 flex-shrink-0">
                <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
                </svg>
                {{ reviewCommentCounts[review.id] }}
              </div>
              <svg
                class="w-4 h-4 text-slate-600 flex-shrink-0 transition-transform duration-200"
                :class="{ 'rotate-180': expandedReviewId === review.id }"
                fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
              </svg>
            </div>

            <!-- Expanded review detail -->
            <div
              v-if="expandedReviewId === review.id"
              class="border-t border-slate-800 px-5 py-5 space-y-5"
            >
              <!-- Metadata -->
              <div class="grid grid-cols-2 sm:grid-cols-4 gap-4">
                <div>
                  <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">{{ t('inbox.reviews.meta.status') }}</div>
                  <StatusBadge :status="review.status" />
                </div>
                <div>
                  <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">{{ t('inbox.reviews.meta.document') }}</div>
                  <div class="text-sm text-slate-300 font-mono">{{ review.document_id || '-' }}</div>
                </div>
                <div>
                  <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">{{ t('inbox.reviews.meta.version') }}</div>
                  <div class="text-sm text-slate-300">{{ review.version || '-' }}</div>
                </div>
                <div>
                  <div class="text-[10px] font-medium text-slate-500 uppercase tracking-wider mb-1">{{ t('inbox.reviews.meta.requested_by') }}</div>
                  <div class="text-sm text-slate-300">{{ review.requested_by || '-' }}</div>
                </div>
              </div>

              <!-- Reviewers -->
              <div v-if="review.reviewers && review.reviewers.length">
                <div class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-2">{{ t('inbox.reviews.assigned_reviewers') }}</div>
                <div class="flex flex-wrap gap-2">
                  <span
                    v-for="reviewer in review.reviewers"
                    :key="reviewer"
                    class="inline-flex items-center gap-1.5 px-2.5 py-1 bg-slate-800 rounded-md text-sm text-slate-300"
                  >
                    <div class="w-5 h-5 rounded-full bg-blue-600/30 text-blue-400 flex items-center justify-center text-[10px] font-bold">
                      {{ reviewer.charAt(0).toUpperCase() }}
                    </div>
                    {{ reviewer }}
                  </span>
                </div>
              </div>

              <!-- Recent comments -->
              <div v-if="expandedReviewComments.length">
                <div class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-2">{{ t('inbox.reviews.recent_comments') }}</div>
                <div class="space-y-2">
                  <div
                    v-for="comment in expandedReviewComments.slice(0, 3)"
                    :key="comment.id"
                    class="bg-slate-800/50 rounded-lg px-4 py-3"
                  >
                    <div class="flex items-center gap-2 mb-1">
                      <span class="text-xs font-semibold text-slate-300">{{ comment.author }}</span>
                      <span class="text-[10px] text-slate-600">{{ formatDate(comment.created_at) }}</span>
                    </div>
                    <div class="text-sm text-slate-400">{{ comment.body }}</div>
                  </div>
                </div>
              </div>

              <!-- Link to Review Request -->
              <div class="pt-2 border-t border-slate-800">
                <router-link
                  :to="orgPath(`/reviews/${review.id}`)"
                  class="w-full px-4 py-3 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors flex items-center justify-center gap-2"
                >
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M13.5 6H5.25A2.25 2.25 0 003 8.25v10.5A2.25 2.25 0 005.25 21h10.5A2.25 2.25 0 0018 18.75V10.5m-10.5 6L21 3m0 0h-5.25M21 3v5.25" />
                  </svg>
                  {{ t('inbox.reviews.open_review', { id: review.id }) }}
                </router-link>
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- ===================== TASKS TAB ===================== -->
      <template v-if="activeTab === 'tasks'">
        <!-- Create task button -->
        <div v-if="canManageInbox" class="flex justify-end">
          <button
            @click="showTaskForm = !showTaskForm"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors"
          >
            {{ t('inbox.tasks.add') }}
          </button>
        </div>

        <!-- Create task form (modal) -->
        <Teleport to="body">
        <Transition name="modal">
        <div v-if="showTaskForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
          <div class="absolute inset-0 bg-black/60" @click="showTaskForm = false" />
          <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
          <div class="flex items-center justify-between mb-2">
            <h2 class="text-sm font-semibold text-slate-200">{{ t('inbox.tasks.add') }}</h2>
            <button @click="showTaskForm = false" class="text-slate-500 hover:text-slate-300">
              <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div class="sm:col-span-2">
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('inbox.tasks.form.title') }}</label>
              <input v-model="newTask.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('inbox.tasks.form.title_placeholder')" />
            </div>
            <div class="sm:col-span-2">
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('inbox.tasks.form.description') }}</label>
              <textarea v-model="newTask.description" rows="2" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500 resize-none" :placeholder="t('inbox.tasks.form.description_placeholder')" />
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('inbox.tasks.form.type') }}</label>
              <select v-model="newTask.task_type" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                <!-- Manual task types only. The *_followup types are created
                     automatically by the system, not chosen here (#32). -->
                <option v-for="o in taskTypeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
              </select>
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('inbox.tasks.form.priority') }}</label>
              <select v-model="newTask.priority" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500">
                <option v-for="o in priorityOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
              </select>
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('inbox.tasks.form.assignee') }}</label>
              <MemberPicker v-model="newTask.assignee" :members="allUsers" :placeholder="t('inbox.tasks.form.assignee_placeholder')" />
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('inbox.tasks.form.due_date') }}</label>
              <input v-model="newTask.due_date" type="date" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500" />
            </div>
          </div>
          <div class="flex items-center gap-3 pt-2">
            <button
              @click="createTask"
              :disabled="!newTask.title.trim() || taskCreating"
              class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 disabled:text-slate-500 text-white text-sm font-medium rounded-lg transition-colors"
            >
              {{ taskCreating ? t('common.state.creating') : t('common.action.add') }}
            </button>
            <button @click="showTaskForm = false" class="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-slate-300 text-sm font-medium rounded-lg transition-colors">
              {{ t('common.action.cancel') }}
            </button>
          </div>
          </div>
        </div>
        </Transition>
        </Teleport>

        <!-- Empty state -->
        <div v-if="tasks.length === 0 && !showTaskForm" class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
          <svg class="w-10 h-10 text-slate-700 mx-auto mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4" />
          </svg>
          <div class="text-sm text-slate-500">{{ t('inbox.tasks.empty') }}</div>
        </div>

        <!-- Tasks list (overdue first, then by priority) -->
        <div v-else-if="tasks.length > 0" class="space-y-2">
          <div
            v-for="task in sortedTasks"
            :key="task.id"
            class="bg-slate-900 border rounded-lg px-5 py-4 flex items-center gap-4 transition-colors"
            :class="isOverdue(task) ? 'border-red-900/50 bg-red-900/20' : 'border-slate-800'"
          >
            <!-- Title + description -->
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium text-slate-200">{{ task.title }}</div>
              <div v-if="task.description" class="text-xs text-slate-500 mt-0.5 truncate">{{ task.description }}</div>
            </div>

            <!-- Type badge -->
            <span class="inline-block px-2 py-0.5 rounded text-xs font-medium bg-slate-800 text-slate-400 flex-shrink-0">
              {{ taskTypeLabel(task.task_type) }}
            </span>

            <!-- Priority badge -->
            <span
              class="inline-block px-2 py-0.5 rounded text-xs font-semibold flex-shrink-0"
              :class="priorityClasses[task.priority] || 'bg-slate-700 text-slate-300'"
            >
              {{ priorityLabel(task.priority) }}
            </span>

            <!-- Assignee -->
            <span class="text-xs text-slate-500 flex-shrink-0 w-28 truncate text-right">{{ resolveUserName(task.assignee) }}</span>

            <!-- Due date -->
            <span
              class="text-xs flex-shrink-0 w-24 text-right"
              :class="isOverdue(task) ? 'text-red-400 font-medium' : 'text-slate-500'"
            >
              {{ formatDay(task.due_date) }}
              <span v-if="isOverdue(task)" class="block text-[10px] text-red-500">{{ t('common.state.overdue') }}</span>
            </span>

            <!-- Status button (click to advance) -->
            <button
              v-if="task.status !== 'done' && canAdvanceTask(task)"
              @click="advanceTaskStatus(task)"
              class="flex-shrink-0"
              :title="taskAdvanceTitle(task)"
            >
              <StatusBadge :status="task.status" class="cursor-pointer hover:opacity-80 transition-opacity" />
            </button>
            <StatusBadge v-else :status="task.status" class="flex-shrink-0" />
          </div>
        </div>
      </template>

      <!-- ===================== INCIDENTS TAB ===================== -->
      <template v-if="activeTab === 'incidents'">
        <div v-if="incidents.length === 0" class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
          <div class="text-sm text-slate-500">{{ t('inbox.incidents.empty') }}</div>
        </div>
        <div v-else class="space-y-2">
          <router-link
            v-for="inc in incidents"
            :key="inc.id"
            :to="orgPath(`/incidents/${inc.id}`)"
            class="bg-slate-900 border border-slate-800 hover:border-slate-700 rounded-lg px-5 py-4 flex items-center gap-4 transition-colors"
          >
            <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ inc.identifier }}</span>
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium text-slate-200 truncate">{{ inc.title }}</div>
              <div v-if="inc.description" class="text-xs text-slate-500 mt-0.5 truncate">{{ inc.description }}</div>
            </div>
            <span class="inline-block px-2 py-0.5 rounded text-xs font-semibold flex-shrink-0" :class="priorityClasses[inc.severity] || 'bg-slate-700 text-slate-300'">{{ severityLabel(inc.severity) }}</span>
            <span class="text-xs text-slate-500 flex-shrink-0 capitalize">{{ inc.category }}</span>
            <span class="text-xs text-slate-500 flex-shrink-0 w-28 truncate text-right">{{ formatDate(inc.detected_at || inc.created_at) }}</span>
            <StatusBadge :status="inc.status" class="flex-shrink-0" />
          </router-link>
        </div>
      </template>

      <!-- ===================== CORRECTIVE ACTIONS TAB ===================== -->
      <template v-if="activeTab === 'corrective_actions'">
        <div v-if="correctiveActions.length === 0" class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
          <div class="text-sm text-slate-500">{{ t('inbox.corrective_actions.empty') }}</div>
        </div>
        <div v-else class="space-y-2">
          <router-link
            v-for="ca in correctiveActions"
            :key="ca.id"
            :to="orgPath(`/corrective-actions/${ca.id}`)"
            class="bg-slate-900 border border-slate-800 hover:border-slate-700 rounded-lg px-5 py-4 flex items-center gap-4 transition-colors"
            :class="ca.due_date && isOverdue(ca) ? 'border-red-900/50 bg-red-900/20' : ''"
          >
            <span class="text-[10px] font-mono uppercase tracking-wider text-slate-600 flex-shrink-0">{{ ca.identifier }}</span>
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium text-slate-200 truncate">{{ ca.title }}</div>
              <div v-if="ca.description" class="text-xs text-slate-500 mt-0.5 truncate">{{ ca.description }}</div>
            </div>
            <span class="inline-block px-2 py-0.5 rounded text-xs font-medium bg-slate-800 text-slate-400 flex-shrink-0">{{ caSeverityLabel(ca.severity) }}</span>
            <span class="text-xs text-slate-500 flex-shrink-0 w-32 truncate text-right">{{ sourceLabel(ca.source) }}</span>
            <span v-if="ca.due_date" class="text-xs flex-shrink-0 w-24 text-right" :class="isOverdue(ca) ? 'text-red-400 font-medium' : 'text-slate-500'">
              {{ formatDay(ca.due_date) }}
              <span v-if="isOverdue(ca)" class="block text-[10px] text-red-500">{{ t('common.state.overdue') }}</span>
            </span>
            <StatusBadge :status="ca.status" class="flex-shrink-0" />
          </router-link>
        </div>
      </template>

      <!-- ===================== CHANGES TAB ===================== -->
      <template v-if="activeTab === 'changes'">
        <!-- Create change button -->
        <div v-if="canManageInbox" class="flex justify-end">
          <button
            @click="showChangeForm = !showChangeForm"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors"
          >
            {{ t('inbox.changes.add') }}
          </button>
        </div>

        <!-- Create change form (modal) -->
        <Teleport to="body">
        <Transition name="modal">
        <div v-if="showChangeForm" class="fixed inset-0 z-50 flex items-start justify-center pt-[8vh] px-4">
          <div class="absolute inset-0 bg-black/60" @click="showChangeForm = false" />
          <div class="relative w-full max-w-2xl bg-slate-900 border border-slate-700 rounded-xl shadow-2xl p-6 space-y-4 max-h-[84vh] overflow-y-auto">
          <div class="flex items-center justify-between mb-2">
            <h2 class="text-sm font-semibold text-slate-200">{{ t('inbox.changes.add') }}</h2>
            <button @click="showChangeForm = false" class="text-slate-500 hover:text-slate-300">
              <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div class="sm:col-span-2">
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('inbox.changes.form.title') }}</label>
              <input v-model="newChange.title" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500" :placeholder="t('inbox.changes.form.title_placeholder')" />
            </div>
            <div class="sm:col-span-2">
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('inbox.changes.form.description') }}</label>
              <textarea v-model="newChange.description" rows="3" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500 resize-none" :placeholder="t('inbox.changes.form.description_placeholder')" />
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('inbox.changes.form.justification') }}</label>
              <textarea v-model="newChange.justification" rows="2" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500 resize-none" :placeholder="t('inbox.changes.form.justification_placeholder')" />
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('inbox.changes.form.impact') }}</label>
              <textarea v-model="newChange.impact" rows="2" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500 resize-none" :placeholder="t('inbox.changes.form.impact_placeholder')" />
            </div>
            <div class="sm:col-span-2">
              <label class="block text-xs font-medium text-slate-500 mb-1">{{ t('inbox.changes.form.assign_to') }}</label>
              <MemberPicker v-model="newChange.assigned_to" :members="allUsers" :placeholder="t('inbox.changes.form.assignee_placeholder')" />
            </div>
          </div>
          <div class="flex items-center gap-3 pt-2">
            <button
              @click="submitChange"
              :disabled="!newChange.title.trim() || changeSubmitting"
              class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 disabled:text-slate-500 text-white text-sm font-medium rounded-lg transition-colors"
            >
              {{ changeSubmitting ? t('common.state.creating') : t('common.action.add') }}
            </button>
            <button @click="showChangeForm = false" class="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-slate-300 text-sm font-medium rounded-lg transition-colors">
              {{ t('common.action.cancel') }}
            </button>
          </div>
          </div>
        </div>
        </Transition>
        </Teleport>

        <!-- Empty state -->
        <div v-if="changes.length === 0 && !showChangeForm" class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
          <svg class="w-10 h-10 text-slate-700 mx-auto mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
          </svg>
          <div class="text-sm text-slate-500">{{ t('inbox.changes.empty') }}</div>
        </div>

        <!-- Change request cards -->
        <div v-else class="space-y-3">
          <div
            v-for="change in changes"
            :key="change.id"
            class="bg-slate-900 border border-slate-800 rounded-lg overflow-hidden"
          >
            <!-- Change header -->
            <div
              @click="toggleChangeExpand(change.id)"
              class="flex items-center gap-4 px-5 py-4 hover:bg-slate-800/30 transition-colors cursor-pointer"
            >
              <StatusBadge :status="change.status" />
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium text-slate-200">{{ change.title }}</div>
                <div class="text-xs text-slate-500 mt-0.5">
                  {{ t('inbox.changes.requested_by', { name: change.requested_by || t('inbox.changes.unknown_requester') }) }}
                  <span class="mx-1.5 text-slate-700">|</span>
                  {{ formatDate(change.created_at) }}
                </div>
              </div>
              <div v-if="change.document_ids && change.document_ids.length" class="flex gap-1.5 flex-shrink-0">
                <span
                  v-for="docId in change.document_ids.slice(0, 2)"
                  :key="docId"
                  class="text-[10px] font-mono bg-slate-800 text-slate-400 px-1.5 py-0.5 rounded"
                >
                  {{ docId }}
                </span>
                <span v-if="change.document_ids.length > 2" class="text-[10px] text-slate-600">
                  +{{ change.document_ids.length - 2 }}
                </span>
              </div>
              <svg
                class="w-4 h-4 text-slate-600 flex-shrink-0 transition-transform duration-200"
                :class="{ 'rotate-180': expandedChangeId === change.id }"
                fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
              </svg>
            </div>

            <!-- Expanded detail -->
            <div
              v-if="expandedChangeId === change.id"
              class="border-t border-slate-800 px-5 py-5 space-y-5"
            >
              <!-- Description -->
              <div v-if="change.description">
                <div class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-2">{{ t('inbox.changes.detail.description') }}</div>
                <p class="text-sm text-slate-400 leading-relaxed whitespace-pre-line">{{ change.description }}</p>
              </div>

              <!-- Justification & Impact -->
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-5">
                <div v-if="change.justification">
                  <div class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-2">{{ t('inbox.changes.detail.justification') }}</div>
                  <p class="text-sm text-slate-400 leading-relaxed whitespace-pre-line">{{ change.justification }}</p>
                </div>
                <div v-if="change.impact">
                  <div class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-2">{{ t('inbox.changes.detail.impact') }}</div>
                  <p class="text-sm text-slate-400 leading-relaxed whitespace-pre-line">{{ change.impact }}</p>
                </div>
              </div>

              <!-- Affected documents -->
              <div v-if="change.document_ids && change.document_ids.length">
                <div class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-2">{{ t('inbox.changes.detail.affected_documents') }}</div>
                <div class="flex flex-wrap gap-2">
                  <span
                    v-for="docId in change.document_ids"
                    :key="docId"
                    class="inline-block px-2.5 py-1 bg-slate-800 rounded-md text-xs font-mono text-slate-300"
                  >
                    {{ docId }}
                  </span>
                </div>
              </div>

              <!-- Actions -->
              <div class="flex items-center gap-3 pt-3 border-t border-slate-800">
                <button
                  v-if="canManageInbox && change.status === 'proposed'"
                  @click="updateChangeStatus(change, 'approved')"
                  :disabled="changeActioning"
                  class="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors"
                >
                  {{ t('inbox.changes.action.approve') }}
                </button>
                <button
                  v-if="canManageInbox && change.status === 'proposed'"
                  @click="updateChangeStatus(change, 'rejected')"
                  :disabled="changeActioning"
                  class="px-4 py-2 bg-red-600 hover:bg-red-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors"
                >
                  {{ t('inbox.changes.action.reject') }}
                </button>
                <button
                  v-if="canManageInbox && change.status === 'approved'"
                  @click="updateChangeStatus(change, 'implemented')"
                  :disabled="changeActioning"
                  class="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors"
                >
                  {{ t('inbox.changes.action.mark_implemented') }}
                </button>
                <button
                  @click="expandedChangeId = null"
                  class="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-slate-300 text-sm font-medium rounded-lg transition-colors"
                >
                  {{ t('inbox.changes.action.close') }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- ═══════ SUGGESTIONS TAB ═══════ -->
      <template v-if="activeTab === 'suggestions'">
        <!-- Filters -->
        <div class="flex items-center gap-2">
          <div class="flex items-center gap-0.5 bg-slate-900 border border-slate-800 rounded-lg p-0.5">
            <button v-for="f in suggestionFilters" :key="f.value" @click="suggestionFilter = f.value; loadSuggestions()"
              class="px-3 py-1.5 text-xs font-medium rounded-md transition-colors"
              :class="suggestionFilter === f.value ? 'bg-slate-700 text-white' : 'text-slate-500 hover:text-slate-300'">
              {{ f.label }}
            </button>
          </div>
        </div>

        <!-- Error -->
        <div v-if="suggestionError" class="bg-red-950/30 border border-red-800/30 rounded-lg px-4 py-3 text-xs text-red-400">
          {{ suggestionError }}
        </div>

        <!-- Empty state -->
        <div v-if="suggestions.length === 0" class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
          <div class="text-sm text-slate-500">{{ t('inbox.suggestions.empty') }}</div>
        </div>

        <!-- Suggestion cards -->
        <div v-else class="space-y-3">
          <div v-for="sg in suggestions" :key="sg.id" :ref="sg.id === highlightSuggestionId ? 'highlightedSuggestion' : undefined"
            class="bg-slate-900 border rounded-lg px-5 py-4 space-y-3 transition-colors"
            :class="sg.id === highlightSuggestionId ? 'border-blue-500/50 ring-1 ring-blue-500/20' : 'border-slate-800'">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0 text-lg" :class="entityIconBg(sg.entity_type)">
                {{ entityIcon(sg.entity_type) }}
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="text-sm font-semibold text-slate-200">{{ suggestionTypeLabel(sg) }} {{ suggestionEntityLabel(sg) }}</span>
                  <span v-if="sg.entity_id" class="text-[10px] text-slate-500 font-mono">{{ sg.entity_id }}</span>
                  <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold"
                    :class="sg.status === 'applied' ? 'bg-emerald-500/15 text-emerald-400'
                      : sg.status === 'rejected' ? 'bg-red-500/15 text-red-400'
                      : sg.status === 'withdrawn' ? 'bg-slate-500/15 text-slate-400'
                      : 'bg-blue-500/15 text-blue-400'">
                    {{ suggestionStatusLabel(sg.status) }}
                  </span>
                  <span v-if="sg.suggested_by_type === 'agent'" class="px-1.5 py-0.5 rounded text-[10px] bg-purple-500/15 text-purple-400">{{ t('inbox.suggestions.agent_badge') }}</span>
                </div>
                <div class="text-sm text-slate-300 mt-0.5">{{ sg.title }}</div>
                <div class="text-xs text-slate-500 mt-1">
                  {{ t('inbox.suggestions.suggested_by', { name: sg.suggested_by?.split('@')[0], date: formatDate(sg.created_at) }) }}
                </div>
              </div>
              <!-- Actions -->
              <div v-if="sg.status === 'open'" class="flex items-center gap-1 flex-shrink-0">
                <template v-if="canReviewSuggestions">
                  <button @click="applySuggestion(sg.id)"
                    class="text-[10px] px-2 py-1 rounded bg-emerald-700 hover:bg-emerald-600 text-white font-medium transition-colors">{{ t('inbox.suggestions.apply') }}</button>
                  <button @click="rejectingId = sg.id; rejectReason = ''"
                    class="text-[10px] px-2 py-1 rounded bg-slate-700 hover:bg-red-800 text-slate-300 hover:text-red-200 font-medium transition-colors">{{ t('inbox.suggestions.reject') }}</button>
                </template>
                <button v-if="sg.suggested_by === currentUserEmail" @click="withdrawSuggestion(sg.id)"
                  class="text-[10px] px-2 py-1 rounded text-slate-600 hover:text-slate-300 transition-colors">{{ t('inbox.suggestions.withdraw') }}</button>
              </div>
            </div>

            <!-- Rationale -->
            <div v-if="sg.rationale" class="text-xs text-slate-400">{{ sg.rationale }}</div>

            <!-- Payload preview -->
            <div v-if="payloadFields(sg).length > 0" class="mt-1 text-xs bg-slate-800/50 rounded px-3 py-2 space-y-0.5">
              <div v-for="f in payloadFields(sg)" :key="f.key" class="flex gap-2">
                <span class="text-slate-500 w-24 flex-shrink-0">{{ f.label }}</span>
                <span class="text-slate-300">{{ f.value }}</span>
              </div>
            </div>

            <!-- Reject form -->
            <div v-if="rejectingId === sg.id" class="flex items-center gap-2">
              <input v-model="rejectReason" type="text" :placeholder="t('inbox.suggestions.reject_placeholder')"
                class="flex-1 bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-xs text-white focus:outline-none focus:border-red-500"
                @keyup.enter="rejectSuggestion(sg.id)" />
              <button @click="rejectSuggestion(sg.id)"
                class="text-xs px-2 py-1.5 bg-red-600 hover:bg-red-500 text-white rounded font-medium">{{ t('inbox.suggestions.reject') }}</button>
              <button @click="rejectingId = null" class="text-xs text-slate-500 hover:text-slate-300">{{ t('common.action.cancel') }}</button>
            </div>

            <!-- Reject reason (for rejected suggestions) -->
            <div v-if="sg.reject_reason" class="text-xs text-red-400/80">{{ t('inbox.suggestions.rejected_reason', { reason: sg.reject_reason }) }}</div>

            <!-- Applied result -->
            <div v-if="sg.applied_entity_id" class="text-xs text-emerald-400/80">{{ t('inbox.suggestions.applied_result', { entity: entityLabel(sg.entity_type, { inline: true }), id: sg.applied_entity_id }) }}</div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { api, getCurrentUser } from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import MemberPicker from '../components/MemberPicker.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import RefreshButton from '../components/RefreshButton.vue'
import { useConfirm } from '../composables/useConfirm'
import { useModalEscape } from '../composables/useModalEscape.js'
import { useToast } from '../composables/useToast.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import { formatDate as formatDateValue, formatDay as formatDayValue, regionLabel } from '../composables/useFormat.js'
import { enumLabel, enumLabelAbbr, entityLabel } from '../composables/useEnumLabel.js'
import { renderApiError } from '../composables/useApiError.js'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()

const { ask } = useConfirm()
const { success: showSaved, error: showError } = useToast()

// ---------- State ----------
const loading = ref(true)
const refreshing = ref(false)
async function reload() {
  refreshing.value = true
  error.value = null
  try {
    await Promise.all([
      loadOpenComments(),
      loadReviews(),
      loadTasks(),
      loadChanges(),
      loadIncidents(),
      loadCAs(),
      loadSuggestions(),
    ])
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    refreshing.value = false
  }
}
const error = ref(null)

const reviews = ref([])
const tasks = ref([])
const changes = ref([])

const validTabs = ['comments', 'reviews', 'tasks', 'changes', 'suggestions', 'incidents', 'corrective_actions']
const initialTab = validTabs.includes(route.params.tab) ? route.params.tab : 'comments'
const activeTab = ref(initialTab)
const highlightSuggestionId = ref(route.query.id ? parseInt(route.query.id) : null)

// Reviews
const expandedReviewId = ref(null)
const expandedReviewComments = ref([])
const reviewCommentCounts = reactive({})
const reviewActioning = ref(false)

// Forward
const currentUserRole = ref('')
const currentUserEmail = ref('')
const allUsers = ref([])
const forwardingReviewId = ref(null)
const forwardTo = ref([])
const forwardMessage = ref('')
const forwardSuccess = ref(null)

// Tasks
const showTaskForm = ref(false)
useModalEscape(showTaskForm)
const taskCreating = ref(false)
const newTask = ref({
  title: '',
  description: '',
  task_type: 'general',
  assignee: '',
  priority: 'medium',
  due_date: '',
})

// Changes
const expandedChangeId = ref(null)
const showChangeForm = ref(false)
useModalEscape(showChangeForm)
const changeSubmitting = ref(false)
const changeActioning = ref(false)
const newChange = ref({
  title: '',
  description: '',
  justification: '',
  impact: '',
  assigned_to: '',
})

// Suggestions
const suggestions = ref([])
const suggestionFilter = ref('open')
const rejectingId = ref(null)
const rejectReason = ref('')

// Comments (all open)
const openComments = ref([])

// Incidents (assigned to me, active)
const incidents = ref([])

// Corrective Actions (assigned to me, active)
const correctiveActions = ref([])

// ---------- Priority styling ----------
const priorityClasses = {
  critical: 'bg-red-900/60 text-red-300',
  high: 'bg-orange-900/60 text-orange-300',
  medium: 'bg-amber-900/60 text-amber-300',
  low: 'bg-slate-700 text-slate-400',
}

// ---------- Computed ----------
const priorityOrder = { critical: 0, high: 1, medium: 2, low: 3 }

const sortedTasks = computed(() => {
  return [...tasks.value].sort((a, b) => {
    // Overdue first
    const aOver = isOverdue(a) ? 0 : 1
    const bOver = isOverdue(b) ? 0 : 1
    if (aOver !== bOver) return aOver - bOver
    // Then by priority
    return (priorityOrder[a.priority] ?? 9) - (priorityOrder[b.priority] ?? 9)
  })
})

// Base role predicate. The computeds below stay separate because each mirrors a
// different server-side rule; they happen to share this base today, but they are
// free to diverge if an endpoint's requireRole() changes.
const isManagerOrAdmin = computed(() => ['admin', 'manager'].includes(currentUserRole.value))

const canReviewSuggestions = computed(() => isManagerOrAdmin.value)

// Mirrors handleCreateTask, handleCreateChange and handleUpdateChangeStatus in
// api_collab.go, all of which requireRole("admin", "manager"). Hide the controls
// rather than let them 403 with the typed input still sitting in the form.
const canManageInbox = computed(() => isManagerOrAdmin.value)

// handleUpdateTaskStatus is wider than canManageInbox but per-task: a contributor
// may advance only a task assigned to them, managers and admins any task. The
// inbox lists other people's tasks too, so this has to be evaluated per row.
const canAdvanceTask = (task) => {
  if (isManagerOrAdmin.value) return true
  return currentUserRole.value === 'contributor' && task?.assignee === currentUserEmail.value
}

// Resolving a comment needs manager rights or authorship of the comment itself
// (the server also accepts a participant in the comment's review, which the
// inbox list has no data for — those users can still resolve from the review
// page). Hide rather than let the button 403.
function canResolveComment(c) {
  if (isManagerOrAdmin.value) return true
  return !!c?.author && c.author === currentUserEmail.value
}
const openSuggestions = computed(() => suggestions.value.filter(s => s.status === 'open' || s.status === 'in_review'))

const totalActionItems = computed(() => {
  return reviews.value.length + tasks.value.length + changes.value.length + openComments.value.length + openSuggestions.value.length + incidents.value.length + correctiveActions.value.length
})

// The whole bar stays in inbox.* rather than common.tab.*, including the two
// labels that happen to match existing common keys. It mixes entity plurals
// with "CAs", an abbreviation sized to this bar — and how far a language can
// abbreviate "corrective actions" is a property of the bar, not of the entity.
const tabs = computed(() => [
  { key: 'comments', label: t('inbox.tab.comments'), count: openComments.value.length },
  { key: 'reviews', label: t('inbox.tab.reviews'), count: reviews.value.length },
  { key: 'tasks', label: t('inbox.tab.tasks'), count: tasks.value.length },
  { key: 'incidents', label: t('inbox.tab.incidents'), count: incidents.value.length },
  { key: 'changes', label: t('inbox.tab.changes'), count: changes.value.length },
  { key: 'corrective_actions', label: t('inbox.tab.corrective_actions'), count: correctiveActions.value.length },
  { key: 'suggestions', label: t('inbox.tab.suggestions'), count: openSuggestions.value.length },
])

// ---------- Catalogue-backed labels ----------
// Every one of these renders a DB value, so it resolves through common.enum.*
// rather than through a label map of this view's own. The <option> factories
// keep the lookup out of the template: a bare group name in a mustache is a
// quoted word the raw-text scanner counts, and rightly.
const taskTypeLabel = (v) => enumLabel('task_type', v)
const priorityLabel = (v) => enumLabel('priority', v)
// Incidents and corrective actions both call their column "severity" and they
// are NOT the same value set: an incident is critical/high/medium/low, a
// corrective action is major_nc/minor_nc/observation/opportunity — the audit
// finding vocabulary, which the catalogue keys as `finding_type`. Mapping both
// to `severity` renders a CA as the raw "Major Nc". Two helpers, deliberately.
const severityLabel = (v) => enumLabel('severity', v)
// The abbreviated form: this chip is the same width as the incident severity
// badge beside it, and the full "Major non-conformity" does not fit.
const caSeverityLabel = (v) => enumLabelAbbr('finding_type', v)
const sourceLabel = (v) => enumLabel('source', v)
const suggestionStatusLabel = (v) => enumLabel('status', v)

// In a helper, not the :title expression: the bare 'open' the ternary compared
// against was counted as raw text, and most bare quoted words in a template are.
const taskAdvanceTitle = (task) => task.status === 'open'
  ? t('inbox.tasks.start')
  : t('inbox.tasks.mark_done')

// Manual task types only. The *_followup types are created automatically by the
// system, not chosen here (#32) — so this list is shorter than the catalogue's.
const TASK_TYPE_VALUES = ['general', 'review', 'onboarding', 'offboarding', 'training', 'other']
const taskTypeOptions = computed(
  () => TASK_TYPE_VALUES.map((value) => ({ value, label: taskTypeLabel(value) })))

const PRIORITY_VALUES = ['low', 'medium', 'high', 'critical']
const priorityOptions = computed(
  () => PRIORITY_VALUES.map((value) => ({ value, label: priorityLabel(value) })))

const suggestionFilters = computed(() => [
  { value: 'open', label: t('inbox.suggestions.filter.open') },
  { value: 'applied', label: t('inbox.suggestions.filter.applied') },
  { value: 'rejected', label: t('inbox.suggestions.filter.rejected') },
])

const forwardableUsers = computed(() => {
  return allUsers.value.filter(u => u.email !== currentUserEmail.value)
})

function resolveUserName(email) {
  if (!email) return '-'
  const u = allUsers.value.find(m => m.email === email)
  return u?.name || email
}

// ---------- Helpers ----------
// The table renders a dash for a missing date, where the shared seam returns
// an empty string; only that choice stays local.
function formatDate(dateStr) {
  return formatDateValue(dateStr) || '-'
}

function formatDay(dateStr) {
  return formatDayValue(dateStr) || '-'
}

function isOverdue(task) {
  if ((!task.due_date && task.due_date !== 0) || task.status === 'done') return false
  const d = typeof task.due_date === 'number' ? new Date(task.due_date * 1000) : new Date(task.due_date)
  return d < new Date()
}

// ---------- Comments ----------
function goToComment(c) {
  const link = commentLink(c)
  const hashIdx = link.indexOf('#')
  const path = hashIdx >= 0 ? link.substring(0, hashIdx) : link
  const hash = hashIdx >= 0 ? link.substring(hashIdx) : ''
  router.push({ path, hash })
}

function commentLink(c) {
  let base = '/documents'
  if (c.document_id === 'suppliers') return orgPath('/suppliers')
  else if (c.document_id === 'risks') return orgPath('/risks')
  else if (c.document_id.startsWith('system')) return orgPath('/systems')
  else base = `/documents/${c.document_id}`
  // Add paragraph anchor — hash + index for disambiguation
  if (c.paragraph_hash && c.paragraph_index != null) base += `#ph${c.paragraph_hash}-${c.paragraph_index}`
  else if (c.paragraph_hash) base += `#ph${c.paragraph_hash}`
  else if (c.paragraph_index != null) base += `#p${c.paragraph_index}`
  return orgPath(base)
}

async function resolveFromInbox(id) {
  try {
    await api.resolveComment(id, getCurrentUser())
    await loadOpenComments()
  } catch (e) {
    showError(t('inbox.error.resolve_comment', { message: renderApiError(e) }))
  }
}

// ---------- Reviews ----------
async function toggleReviewExpand(id) {
  if (expandedReviewId.value === id) {
    expandedReviewId.value = null
    expandedReviewComments.value = []
    return
  }
  expandedReviewId.value = id
  expandedReviewComments.value = []
  const review = reviews.value.find(r => r.id === id)
  if (review?.document_id) {
    try {
      // Scope comments to this review, not the entire document
      const res = await api.fetchJSON(`/api/v1/documents/${encodeURIComponent(review.document_id)}/comments?review_id=${id}`)
      const ec = res?.data || res
      expandedReviewComments.value = Array.isArray(ec) ? ec : []
    } catch { /* ignore */ }
  }
}

async function markAllNotificationsRead() {
  try {
    await api.markAllRead()
    // Dispatch event so App.vue sidebar count updates
    window.dispatchEvent(new Event('isms:notifications-changed'))
    // Reload inbox data
    await loadInbox()
  } catch (e) {
    showError(t('inbox.error.mark_all_read', { message: renderApiError(e) }))
  }
}

function openDocumentForReview(review) {
  const docId = review.document_id || ''
  if (orgSlug.value && docId) {
    router.push({ path: orgPath('/documents'), query: { open: docId, review: review.id } })
  }
}


// Using the proper /reviews/:id/approve endpoint to go through the state machine.
async function approveReview(review) {
  reviewActioning.value = true
  try {
    await api.approveReview(review.id, 'approved', '')
    expandedReviewId.value = null
    await loadReviews()
  } catch (e) {
    error.value = renderApiError(e)
    showError(t('inbox.error.approve_review', { message: renderApiError(e) }))
  } finally {
    reviewActioning.value = false
  }
}

async function requestChangesOnReview(review) {
  reviewActioning.value = true
  try {
    await api.approveReview(review.id, 'changes_requested', '')
    expandedReviewId.value = null
    await loadReviews()
  } catch (e) {
    error.value = renderApiError(e)
    showError(t('inbox.error.request_changes', { message: renderApiError(e) }))
  } finally {
    reviewActioning.value = false
  }
}

function startForward(reviewId) {
  forwardingReviewId.value = reviewId
  forwardTo.value = []
  forwardMessage.value = ''
}

async function submitForward(reviewId) {
  try {
    await api.forwardReview(reviewId, forwardTo.value, forwardMessage.value)
    forwardingReviewId.value = null
    forwardTo.value = []
    forwardMessage.value = ''
    forwardSuccess.value = reviewId
    setTimeout(() => { forwardSuccess.value = null }, 3000)
    await loadData()
  } catch (e) {
    error.value = renderApiError(e)
    showError(t('inbox.error.forward_review', { message: renderApiError(e) }))
  }
}

async function loadData() {
  await Promise.all([loadReviews(), loadTasks(), loadChanges(), loadOpenComments()])
}

async function loadOpenComments() {
  try {
    const all = await api.getAllOpenComments()
    openComments.value = (all || []).filter(c => !c.parent_id)
  } catch {
    openComments.value = []
  }
}

async function loadReviews() {
  try {
    const r = await api.getReviews()
    const all = Array.isArray(r) ? r : []
    // Filter to only show reviews where current user is assigned or is the requester
    const me = currentUserEmail.value
    reviews.value = all.filter(rev =>
      rev.requested_by === me ||
      (rev._assignments || []).some(a => a.reviewer === me || a.reviewer_email === me)
    )
    // If user email not loaded yet, don't show any (wait for identity)
    if (!me) reviews.value = []
    // Fetch comment counts for each review with a document_id
    for (const review of reviews.value) {
      if (review.document_id) {
        try {
          const comments = await api.getDocComments(review.document_id)
          reviewCommentCounts[review.id] = (comments || []).length
        } catch {
          reviewCommentCounts[review.id] = 0
        }
      }
    }
  } catch (e) {
    error.value = renderApiError(e)
  }
}

// ---------- Tasks ----------
async function advanceTaskStatus(task) {
  const next = task.status === 'open' ? 'in_progress' : 'done'
  try {
    await api.updateTaskStatus(task.id, next)
    task.status = next
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function createTask() {
  if (!newTask.value.title.trim() || taskCreating.value) return
  taskCreating.value = true
  try {
    await api.createTask({
      ...newTask.value,
      created_by: getCurrentUser(),
      status: 'open',
    })
    newTask.value = { title: '', description: '', task_type: 'general', assignee: '', priority: 'medium', due_date: '' }
    showTaskForm.value = false
    await loadTasks()
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    taskCreating.value = false
  }
}

async function loadTasks() {
  try {
    const res = await api.getTasks()
    tasks.value = Array.isArray(res) ? res : []
  } catch (e) {
    error.value = renderApiError(e)
  }
}

// ---------- Changes ----------
function toggleChangeExpand(id) {
  expandedChangeId.value = expandedChangeId.value === id ? null : id
}

async function updateChangeStatus(change, status) {
  changeActioning.value = true
  try {
    await api.updateChangeStatus(change.id, status, getCurrentUser())
    expandedChangeId.value = null
    await loadChanges()
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    changeActioning.value = false
  }
}

async function submitChange() {
  if (!newChange.value.title.trim() || changeSubmitting.value) return
  changeSubmitting.value = true
  try {
    await api.createChange({
      title: newChange.value.title,
      description: newChange.value.description,
      justification: newChange.value.justification,
      impact: newChange.value.impact,
      requested_by: getCurrentUser(),
      assigned_to: newChange.value.assigned_to,
    })
    newChange.value = { title: '', description: '', justification: '', impact: '', assigned_to: '' }
    showChangeForm.value = false
    await loadChanges()
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    changeSubmitting.value = false
  }
}

async function loadChanges() {
  try {
    const ch = await api.getChanges()
    changes.value = Array.isArray(ch) ? ch : []
  } catch (e) {
    error.value = renderApiError(e)
  }
}

async function loadIncidents() {
  try {
    const list = await api.getIncidents('', '', currentUserEmail.value)
    const arr = Array.isArray(list) ? list : (Array.isArray(list?.data) ? list.data : [])
    incidents.value = arr.filter(i => i.status !== 'closed' && i.status !== 'resolved')
  } catch (e) {
    incidents.value = []
  }
}

async function loadCAs() {
  try {
    const list = await api.getCorrectiveActions('', '', currentUserEmail.value)
    const arr = Array.isArray(list) ? list : (Array.isArray(list?.data) ? list.data : [])
    correctiveActions.value = arr.filter(c => c.status !== 'resolved')
  } catch (e) {
    correctiveActions.value = []
  }
}

async function loadSuggestions() {
  try {
    const data = await api.getSuggestions({ status: suggestionFilter.value })
    suggestions.value = Array.isArray(data?.data) ? data.data : (Array.isArray(data) ? data : [])
  } catch { suggestions.value = [] }
}

const suggestionError = ref('')

async function claimSuggestion(id) {
  suggestionError.value = ''
  try { await api.claimSuggestion(id); await loadSuggestions() }
  catch (e) { suggestionError.value = t('inbox.error.claim_suggestion', { message: renderApiError(e) }) }
}

async function applySuggestion(id, force) {
  suggestionError.value = ''
  try {
    const result = await api.applySuggestion(id, { force: !!force })
    if (result?.stale) {
      if (await ask(t('inbox.confirm.apply_stale_body'), { confirm: t('inbox.confirm.apply_stale_title'), variant: 'warning' })) {
        await applySuggestion(id, true)
      }
      return
    }
    await loadSuggestions()
  } catch (e) { suggestionError.value = t('inbox.error.apply_suggestion', { message: renderApiError(e) }) }
}

async function rejectSuggestion(id) {
  // Deliberately not translated. This is persisted as the suggestion's
  // reject_reason and read back by everyone in the org, including the CLI and
  // the MCP tools — storing the rejecting user's language would leave a row
  // nobody else can read. Same reasoning as the notification keying work: text
  // that outlives the request is stored in one language, not the sender's.
  const reason = rejectReason.value.trim() || 'Rejected'
  suggestionError.value = ''
  try {
    await api.rejectEntitySuggestion(id, reason)
    rejectingId.value = null
    rejectReason.value = ''
    await loadSuggestions()
  } catch (e) { suggestionError.value = t('inbox.error.reject_suggestion', { message: renderApiError(e) }) }
}

async function withdrawSuggestion(id) {
  suggestionError.value = ''
  try { await api.withdrawSuggestion(id); await loadSuggestions() }
  catch (e) { suggestionError.value = t('inbox.error.withdraw_suggestion', { message: renderApiError(e) }) }
}

// Was a local map whose only divergence from the catalogue was "New" for
// `create`; the catalogue says "Create", and one word for one value beats two.
function suggestionTypeLabel(sg) {
  return enumLabel('suggestion_type', sg.suggestion_type)
}

function parsedPayload(raw) {
  if (!raw) return null
  try {
    const obj = typeof raw === 'string' ? JSON.parse(raw) : raw
    if (obj && typeof obj === 'object' && Object.keys(obj).length > 0) return obj
  } catch {}
  return null
}

// Twelve entity names the catalogue already carries. Four normalise to its
// sentence case — "Legal Requirement" becomes "Legal requirement", and so on
// for change request, corrective action and audit finding.
function suggestionEntityLabel(sg) {
  return entityLabel(sg.entity_type)
}

function entityIcon(type) {
  const icons = { risk: '\u26A0\uFE0F', incident: '\uD83D\uDEA8', supplier: '\uD83C\uDFE2', legal_requirement: '\u2696\uFE0F', change_request: '\uD83D\uDD04', corrective_action: '\uD83D\uDD27', objective: '\uD83C\uDFAF', task: '\u2611\uFE0F', system: '\uD83D\uDDA5\uFE0F', asset: '\uD83D\uDCE6', audit_finding: '\uD83D\uDD0D', document: '\uD83D\uDCC4' }
  return icons[type] || '\uD83D\uDCA1'
}

function entityIconBg(type) {
  const bgs = { risk: 'bg-red-500/10', incident: 'bg-orange-500/10', supplier: 'bg-emerald-500/10', legal_requirement: 'bg-purple-500/10', change_request: 'bg-sky-500/10', corrective_action: 'bg-pink-500/10', objective: 'bg-teal-500/10', task: 'bg-lime-500/10', system: 'bg-cyan-500/10', asset: 'bg-amber-500/10', audit_finding: 'bg-rose-500/10' }
  return bgs[type] || 'bg-slate-500/10'
}

// A module-scope label map is invisible to both scanners, which is exactly why
// it survived this long. Keys are spelled out rather than built from the field
// name so the keyset walk can see them.
const PAYLOAD_FIELD_KEYS = {
  title: 'inbox.suggestions.payload_field.title',
  name: 'inbox.suggestions.payload_field.name',
  description: 'inbox.suggestions.payload_field.description',
  category: 'inbox.suggestions.payload_field.category',
  risk_type: 'inbox.suggestions.payload_field.risk_type',
  origin: 'inbox.suggestions.payload_field.origin',
  severity: 'inbox.suggestions.payload_field.severity',
  priority: 'inbox.suggestions.payload_field.priority',
  status: 'inbox.suggestions.payload_field.status',
  type: 'inbox.suggestions.payload_field.type',
  criticality: 'inbox.suggestions.payload_field.criticality',
  jurisdiction: 'inbox.suggestions.payload_field.jurisdiction',
  classification: 'inbox.suggestions.payload_field.classification',
  finding_type: 'inbox.suggestions.payload_field.finding_type',
  risk_level: 'inbox.suggestions.payload_field.risk_level',
  source: 'inbox.suggestions.payload_field.source',
  incident_type: 'inbox.suggestions.payload_field.incident_type',
  current_likelihood: 'inbox.suggestions.payload_field.current_likelihood',
  current_impact: 'inbox.suggestions.payload_field.current_impact',
}

function payloadFields(sg) {
  const raw = sg.payload
  if (!raw) return []
  try {
    const obj = typeof raw === 'string' ? JSON.parse(raw) : raw
    if (!obj || typeof obj !== 'object') return []
    return Object.entries(obj)
      .filter(([, v]) => v !== null && v !== '' && v !== undefined)
      .map(([k, v]) => ({
        key: k,
        label: PAYLOAD_FIELD_KEYS[k] ? t(PAYLOAD_FIELD_KEYS[k]) : k.replace(/_/g, ' '),
        // `jurisdiction` holds a region code or sentinel, so it renders through
        // the same seam the legal register uses. Historical suggestions hold the
        // pre-migration English name and fall through regionLabel() unchanged.
        value: k === 'jurisdiction' ? regionLabel(v) : v,
      }))
  } catch { return [] }
}

// ---------- Init ----------
onMounted(async () => {
  // Load user identity FIRST so review filtering works
  try {
    const me = await api.getMe()
    currentUserRole.value = me?.role || ''
    currentUserEmail.value = me?.email || ''
  } catch {}
  try {
    const u = await api.getUsers()
    allUsers.value = Array.isArray(u) ? u : []
  } catch {}

  // Now load data with user identity available
  try {
    await Promise.all([
      loadOpenComments(),
      loadReviews(),
      loadTasks(),
      loadChanges(),
      loadIncidents(),
      loadCAs(),
      loadSuggestions(),
    ])
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    loading.value = false
  }
})
</script>
