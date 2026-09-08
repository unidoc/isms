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

    <!-- ==================== DETAIL VIEW ==================== -->
    <div v-else-if="reviewId" class="px-6 py-6">
      <!-- Back link -->
      <button @click="goToList" class="flex items-center gap-1.5 text-sm text-slate-500 hover:text-slate-300 mb-6 transition-colors">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M10.5 19.5L3 12m0 0l7.5-7.5M3 12h18" />
        </svg>
        {{ t('reviews.back') }}
      </button>

      <div v-if="!review" class="flex items-center justify-center h-64">
        <div class="text-slate-500 text-sm">{{ t('reviews.not_found') }}</div>
      </div>

      <template v-else>
        <!-- Header -->
        <div class="mb-8">
          <div class="flex items-start gap-4">
            <div class="flex-1 min-w-0">
              <h1 class="text-2xl font-bold text-slate-100 tracking-tight flex items-center gap-3 flex-wrap">
                {{ review.title }}
                <span class="text-base font-normal text-slate-500">#{{ review.id }}</span>
              </h1>
              <div class="flex items-center gap-3 mt-2 flex-wrap">
                <span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold"
                  :class="statusClass(review.status)">
                  <span class="w-1.5 h-1.5 rounded-full" :class="statusDotClass(review.status)"></span>
                  {{ statusLabel(review.status) }}
                </span>
                <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-semibold bg-slate-700/50 text-slate-300">
                  {{ t('common.label.round', { round: review.round || 1 }) }}
                </span>
                <span v-if="aiReviewActive" class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-purple-500/15 text-purple-400">
                  {{ t('reviews.ai.active') }}
                </span>
                <span v-if="aiReviewEscalated" class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-red-500/15 text-red-400">
                  {{ t('reviews.ai.escalated') }}
                </span>
                <span class="text-sm text-slate-400">
                  {{ review.version }}
                </span>
                <span class="text-slate-700">|</span>
                <div class="flex items-center gap-1.5">
                  <div class="w-5 h-5 rounded-full flex items-center justify-center text-[9px] font-bold text-white flex-shrink-0"
                    :class="avatarColor(review.requested_by)">
                    {{ initial(review.requested_by) }}
                  </div>
                  <span class="text-sm text-slate-400">{{ review.requested_by }}</span>
                </div>
                <span class="text-xs text-slate-600">{{ t('reviews.opened_ago', { time: timeAgo(review.created_at) }) }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Review message -->
        <div v-if="review.message" class="mb-6 px-4 py-3 bg-slate-900 border border-slate-800 rounded-lg">
          <div class="text-sm text-slate-300 leading-relaxed whitespace-pre-wrap">{{ review.message }}</div>
        </div>

        <!-- Review guide for first-time visitors -->
        <div v-if="review.status === 'open' && !reviewGuideHidden && !userIsAuthor && userCanReview" class="mb-6 bg-blue-950/20 border border-blue-800/20 rounded-xl px-5 py-4">
          <div class="flex items-start justify-between">
            <div class="space-y-2 text-sm text-blue-300/80">
              <div class="flex items-center gap-2"><span class="w-5 h-5 rounded-full bg-blue-600/30 flex items-center justify-center text-xs font-bold text-blue-300">1</span> {{ t('reviews.guide.step_review') }}</div>
              <div class="flex items-center gap-2"><span class="w-5 h-5 rounded-full bg-blue-600/30 flex items-center justify-center text-xs font-bold text-blue-300">2</span> {{ t('reviews.guide.step_comment') }}</div>
              <div class="flex items-center gap-2"><span class="w-5 h-5 rounded-full bg-blue-600/30 flex items-center justify-center text-xs font-bold text-blue-300">3</span> {{ t('reviews.guide.step_decide') }}</div>
            </div>
            <button @click="reviewGuideHidden = true" class="text-blue-600 hover:text-blue-400 p-1">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        <!-- Tabs + Content layout -->
        <div class="flex flex-col lg:flex-row gap-4 lg:gap-6">
          <!-- Main content area -->
          <div class="flex-1 min-w-0">
            <!-- Detail tabs -->
            <div class="flex gap-1 bg-slate-900 border border-slate-800 rounded-lg p-1 mb-6">
              <button v-for="tab in detailTabs" :key="tab.key" @click="detailTab = tab.key"
                class="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-md transition-colors"
                :class="detailTab === tab.key ? 'bg-slate-800 text-white' : 'text-slate-500 hover:text-slate-300'">
                {{ tab.label }}
                <span v-if="tab.count > 0" class="inline-flex items-center justify-center min-w-[20px] h-5 px-1.5 text-[11px] font-semibold rounded-full"
                  :class="detailTab === tab.key ? 'bg-slate-700 text-slate-200' : 'bg-slate-800 text-slate-400'">
                  {{ tab.count }}
                </span>
              </button>
            </div>

            <!-- Conversation Tab -->
            <div v-if="detailTab === 'conversation'" class="space-y-0">
              <!-- Timeline -->
              <div class="relative">
                <!-- Vertical line -->
                <div class="absolute left-4 top-0 bottom-0 w-px bg-slate-800"></div>

                <!-- Timeline entries -->
                <template v-for="(entry, i) in timeline" :key="i">
                <!-- Round divider -->
                <div v-if="entry.round > 1 && (i === 0 || timeline[i-1].round !== entry.round)"
                  class="relative pl-10 pb-4 pt-2">
                  <div class="absolute left-1.5 top-3 w-5 h-5 rounded-full bg-slate-800 border-2 border-slate-600 flex items-center justify-center z-10">
                    <span class="text-[8px] font-bold text-slate-400">{{ entry.round }}</span>
                  </div>
                  <div class="flex items-center gap-3">
                    <div class="h-px flex-1 bg-slate-700"></div>
                    <span class="text-xs font-semibold text-slate-500 whitespace-nowrap">{{ t('common.label.round', { round: entry.round }) }}</span>
                    <div class="h-px flex-1 bg-slate-700"></div>
                  </div>
                </div>
                <div class="relative pl-10 pb-6">
                  <!-- Dot -->
                  <div class="absolute left-2.5 top-1.5 w-3 h-3 rounded-full border-2 z-10"
                    :class="timelineDotClass(entry)"></div>

                  <!-- Comment entry -->
                  <div v-if="entry.type === 'comment'" class="bg-slate-900 border border-slate-800 rounded-lg overflow-hidden">
                    <div class="flex items-center gap-2 px-4 py-2.5 bg-slate-900/80 border-b border-slate-800">
                      <div class="w-5 h-5 rounded-full flex items-center justify-center text-[9px] font-bold text-white flex-shrink-0"
                        :class="avatarColor(entry.actor)">
                        {{ initial(entry.actor) }}
                      </div>
                      <span class="text-sm font-medium text-slate-200">{{ entry.actor }}</span>
                      <span class="text-xs text-slate-600">{{ entry.data?.suggestion_body ? t('reviews.timeline.suggested_edit') : t('reviews.timeline.commented') }} {{ timeAgo(entry.created_at) }}</span>
                      <span v-if="entry.data?.suggestion_status"
                        class="text-[10px] font-semibold px-1.5 py-0.5 rounded-full"
                        :class="{ 'bg-amber-800/40 text-amber-300': entry.data.suggestion_status === 'pending', 'bg-emerald-800/40 text-emerald-300': entry.data.suggestion_status === 'accepted', 'bg-red-800/40 text-red-300': entry.data.suggestion_status === 'rejected' }">
                        {{ suggestionStatusLabel(entry.data.suggestion_status) }}
                      </span>
                      <span v-if="entry.round > 1" class="text-[10px] text-slate-600 px-1.5 py-0.5 rounded bg-slate-800/50 font-medium">{{ t('reviews.timeline.round_short', { round: entry.round }) }}</span>
                    </div>
                    <div v-if="entry.quote" class="mx-4 mt-3 px-3 py-2 border-l-2 border-slate-700 bg-slate-800/30 rounded-r text-xs text-slate-500 italic">
                      "{{ entry.quote }}"
                    </div>
                    <!-- Suggestion diff or regular body -->
                    <div v-if="entry.data?.suggestion_body" class="px-4 py-3">
                      <div class="rounded bg-slate-950 border border-slate-800 p-2 text-xs font-mono">
                        <div v-if="entry.quote" class="text-red-400/60 line-through">{{ entry.quote }}</div>
                        <div class="text-emerald-400/80 mt-1">{{ entry.data.suggestion_body }}</div>
                      </div>
                    </div>
                    <div v-else class="px-4 py-3 text-sm text-slate-300 leading-relaxed whitespace-pre-wrap">{{ entry.body }}</div>
                  </div>

                  <!-- Approval entry -->
                  <div v-else-if="entry.type === 'approval'" class="flex items-center gap-2">
                    <div class="w-5 h-5 rounded-full flex items-center justify-center text-[9px] font-bold text-white flex-shrink-0"
                      :class="avatarColor(entry.actor)">
                      {{ initial(entry.actor) }}
                    </div>
                    <span class="text-sm font-medium text-slate-200">{{ entry.actor }}</span>
                    <span v-if="entry.decision === 'approved'" class="text-sm text-emerald-400 font-medium">{{ t('reviews.timeline.approved') }}</span>
                    <span v-else-if="entry.decision === 'proposed_revision'" class="text-sm text-blue-400 font-medium">{{ t('reviews.timeline.proposed_revision') }}</span>
                    <span v-else-if="entry.decision === 'changes_requested'" class="text-sm text-amber-400 font-medium">{{ t('reviews.timeline.requested_changes') }}</span>
                    <span class="text-xs text-slate-600 ml-1">{{ timeAgo(entry.created_at) }}</span>
                    <span v-if="entry.round > 1" class="text-[10px] text-slate-600 px-1.5 py-0.5 rounded bg-slate-800/50 font-medium">{{ t('reviews.timeline.round_short', { round: entry.round }) }}</span>
                    <div v-if="entry.detail" class="ml-2 text-xs text-slate-500 italic">"{{ entry.detail }}"</div>
                  </div>

                  <!-- Activity entry -->
                  <div v-else-if="entry.type === 'activity'" class="flex items-center gap-2 text-sm">
                    <div class="w-5 h-5 rounded-full flex items-center justify-center text-[9px] font-bold text-white flex-shrink-0 bg-slate-700">
                      {{ initial(entry.actor) }}
                    </div>
                    <span class="text-slate-400">{{ entry.detail || entry.action }}</span>
                    <span class="text-xs text-slate-600">{{ timeAgo(entry.created_at) }}</span>
                  </div>

                  <!-- Assignment entry -->
                  <div v-else-if="entry.type === 'assignment'" class="flex items-center gap-2 text-sm">
                    <svg class="w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M19 7.5v3m0 0v3m0-3h3m-3 0h-3m-2.25-4.125a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zM4 19.235v-.11a6.375 6.375 0 0112.75 0v.109A12.318 12.318 0 0110.374 21c-2.331 0-4.512-.645-6.374-1.766z" />
                    </svg>
                    <i18n-t keypath="reviews.timeline.assigned" tag="span" class="text-slate-400" scope="global">
                      <template #reviewer><span class="text-slate-300 font-medium">{{ entry.reviewer }}</span></template>
                    </i18n-t>
                    <span class="text-xs text-slate-600">{{ timeAgo(entry.created_at) }}</span>
                  </div>

                  <!-- Decision record entry (immutable audit trail) -->
                  <div v-else-if="entry.type === 'decision'" class="bg-slate-900/50 border border-slate-800 rounded-lg px-4 py-3">
                    <div class="flex items-center gap-2 mb-1">
                      <svg class="w-4 h-4 flex-shrink-0" :class="entry.decision === 'merged' ? 'text-purple-400' : entry.decision === 'approved' ? 'text-emerald-400' : entry.decision === 'proposed_revision' ? 'text-blue-400' : 'text-amber-400'" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" />
                      </svg>
                      <div class="w-5 h-5 rounded-full flex items-center justify-center text-[9px] font-bold text-white flex-shrink-0"
                        :class="avatarColor(entry.actor)">
                        {{ initial(entry.actor) }}
                      </div>
                      <span class="text-sm font-medium text-slate-200">{{ entry.actor }}</span>
                      <span class="text-sm font-medium" :class="entry.decision === 'merged' ? 'text-purple-400' : entry.decision === 'approved' ? 'text-emerald-400' : entry.decision === 'proposed_revision' ? 'text-blue-400' : 'text-amber-400'">
                        {{ decisionLabel(entry.decision) }}
                      </span>
                      <span class="text-xs text-slate-600 ml-1">{{ timeAgo(entry.created_at) }}</span>
                    </div>
                    <div v-if="entry.detail" class="text-xs text-slate-400 ml-6 mt-1 italic">"{{ entry.detail }}"</div>
                    <div v-if="entry.content_hash" class="flex items-center gap-1.5 ml-6 mt-1.5">
                      <svg class="w-3 h-3 text-slate-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M7.864 4.243A7.5 7.5 0 0119.5 10.5c0 2.92-.556 5.709-1.568 8.268M5.742 6.364A7.465 7.465 0 004.5 10.5a7.464 7.464 0 01-1.15 3.993m1.989 3.559A11.209 11.209 0 008.25 10.5a3.75 3.75 0 117.5 0c0 .527-.021 1.049-.064 1.565M12 10.5a14.94 14.94 0 01-3.6 9.75m6.633-4.596a18.666 18.666 0 01-2.485 5.33" />
                      </svg>
                      <span class="text-[10px] font-mono text-slate-600" :title="t('reviews.timeline.content_hash')">{{ entry.content_hash.substring(0, 16) }}...</span>
                    </div>
                    <!-- Per-event diff: exactly what this revision changed (#6) -->
                    <div v-if="entry.decision === 'proposed_revision' && entry.data?.commit_ref" class="ml-6 mt-2">
                      <button @click="toggleEventDiff(entry)" class="text-xs font-medium text-blue-400 hover:text-blue-300 transition-colors">
                        {{ openEventDiff === entry.data.commit_ref ? t('reviews.timeline.hide_changes') : t('reviews.timeline.view_changes') }}
                      </button>
                      <div v-if="openEventDiff === entry.data.commit_ref" class="mt-2 bg-slate-900 border border-slate-800 rounded-lg overflow-hidden">
                        <div v-if="eventDiff[entry.data.commit_ref]?.loading" class="px-4 py-3 text-xs text-slate-500">{{ t('reviews.timeline.diff_loading') }}</div>
                        <div v-else-if="eventDiff[entry.data.commit_ref]?.error" class="px-4 py-3 text-xs text-red-400">{{ t('reviews.timeline.diff_error') }}</div>
                        <TrackChanges v-else
                          :old-body="eventDiff[entry.data.commit_ref]?.old || ''"
                          :new-body="eventDiff[entry.data.commit_ref]?.new || ''"
                          :document-id="review?.document_id || ''"
                          :readonly="true" />
                      </div>
                    </div>
                  </div>
                </div>
                </template>
              </div>

              <!-- Updated since sent notification -->
              <div v-if="isUpdatedSinceSent" class="mb-4 px-4 py-3 bg-blue-950/30 border border-blue-800/30 rounded-lg flex items-center gap-3">
                <svg class="w-5 h-5 text-blue-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182" />
                </svg>
                <div class="flex-1">
                  <div class="text-sm text-blue-300 font-medium">{{ t('reviews.updated.heading') }}</div>
                  <div v-if="lastModifiedBy" class="text-xs text-blue-400/70 mt-0.5">
                    {{ lastCommitMsg
                      ? t('reviews.updated.last_modified_by_with_message', { name: lastModifiedBy, message: lastCommitMsg })
                      : t('reviews.updated.last_modified_by', { name: lastModifiedBy }) }}
                  </div>
                  <div class="text-xs text-slate-500 mt-0.5">{{ t('reviews.updated.note') }}</div>
                </div>
              </div>

              <!-- Changes summary (always visible at top of conversation) -->
              <div v-if="oldBody || newBody" class="mb-6">
                <div class="text-xs text-slate-500 font-medium mb-2">{{ t('reviews.diff.summary_heading') }}</div>
                <div class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
                  <TrackChanges :old-body="oldBody" :new-body="newBody" :author="diffMeta?.last_modified_by" :date="diffMeta?.last_modified" :document-id="review?.document_id || ''" :blame-ref="diffMeta?.has_branch ? `review/${review?.id}` : ''" />
                </div>
              </div>
              <div v-else-if="!diffLoading" class="mb-6 px-4 py-3 bg-slate-900 border border-slate-800 rounded-lg text-xs text-slate-500">
                <span v-if="!diffMeta?.commit_hash">{{ t('reviews.diff.never_approved_inline') }}</span>
                <span v-else>{{ t('reviews.diff.none_inline') }}</span>
              </div>

              <!-- Empty state -->
              <div v-if="timeline.length === 0 && !diffData" class="text-center py-12 text-slate-500 text-sm">
                {{ t('reviews.timeline.empty') }}
              </div>

              <!-- Comment input -->
              <div v-if="canCommentOnReview" class="mt-6 bg-slate-900 border border-slate-800 rounded-lg overflow-hidden">
                <div class="px-4 py-2.5 border-b border-slate-800">
                  <span class="text-xs text-slate-500 font-medium">{{ t('reviews.comment.heading') }}</span>
                </div>
                <textarea v-model="newComment" rows="3" :placeholder="t('reviews.comment.placeholder')"
                  class="w-full bg-transparent px-4 py-3 text-sm text-slate-200 placeholder-slate-600 focus:outline-none resize-none"></textarea>
                <div class="flex justify-end px-4 py-2.5 border-t border-slate-800">
                  <button @click="submitComment" :disabled="!newComment.trim() || submitting"
                    class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
                    {{ t('common.action.comment') }}
                  </button>
                </div>
              </div>
            </div>

            <!-- Changes Tab -->
            <div v-if="detailTab === 'changes'">
              <div v-if="diffLoading" class="flex items-center justify-center h-48">
                <div class="text-slate-400 text-sm">{{ t('reviews.diff.loading') }}</div>
              </div>
              <div v-else-if="oldBody || newBody || activeOldBody || activeNewBody" class="space-y-3">
                <!-- Scope + view mode toggles -->
                <div class="flex items-center justify-between">
                  <!-- Diff scope: this round vs all changes -->
                  <div v-if="review.round > 1" class="flex items-center gap-0.5 bg-slate-900 border border-slate-800 rounded-lg p-0.5">
                    <button @click="switchDiffScope('round')" class="px-3 py-1.5 rounded-md text-[11px] font-medium transition-colors"
                      :class="diffScope === 'round' ? 'bg-blue-600 text-white' : 'text-slate-500 hover:text-slate-300'">
                      {{ t('reviews.diff.scope_round') }}
                    </button>
                    <button @click="switchDiffScope('all')" class="px-3 py-1.5 rounded-md text-[11px] font-medium transition-colors"
                      :class="diffScope === 'all' ? 'bg-blue-600 text-white' : 'text-slate-500 hover:text-slate-300'">
                      {{ t('reviews.diff.scope_all') }}
                    </button>
                  </div>
                  <div v-else></div>
                  <!-- View mode toggle -->
                  <div class="flex items-center gap-0.5 bg-slate-900 border border-slate-800 rounded-lg p-0.5">
                    <button @click="changesViewMode = 'split'" class="px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors"
                      :class="changesViewMode === 'split' ? 'bg-slate-700 text-white' : 'text-slate-500 hover:text-slate-300'">
                      {{ t('reviews.diff.mode_split') }}
                    </button>
                    <button @click="changesViewMode = 'unified'" class="px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors"
                      :class="changesViewMode === 'unified' ? 'bg-slate-700 text-white' : 'text-slate-500 hover:text-slate-300'">
                      {{ t('reviews.diff.mode_unified') }}
                    </button>
                  </div>
                </div>
                <!-- Split view (side by side with comments) -->
                <SideBySideReview v-if="changesViewMode === 'split'"
                  :old-body="activeOldBody" :new-body="activeNewBody" :raw-diff="activeDiffData || ''"
                  :comment-count="review.comment_count || 0" :review-status="statusLabel(review.status)"
                  :readonly="!canCommentOnReview"
                  :comments="diffComments"
                  @comment="onDiffComment" />
                <!-- Unified view (inline track changes with comments) -->
                <TrackChanges v-else
                  :old-body="activeOldBody" :new-body="activeNewBody"
                  :author="diffMeta?.last_modified_by" :date="diffMeta?.last_modified"
                  :document-id="review?.document_id || ''" :blame-ref="diffMeta?.has_branch ? `review/${review?.id}` : ''"
                  :comments="diffComments"
                  :readonly="!canCommentOnReview"
                  @comment="onDiffComment" />

              </div>
              <div v-else class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
                <div v-if="!diffMeta?.commit_hash" class="space-y-2">
                  <div class="text-slate-400 text-sm font-medium">{{ t('reviews.diff.never_approved_heading') }}</div>
                  <div class="text-slate-500 text-xs">{{ t('reviews.diff.never_approved_body') }}</div>
                </div>
                <div v-else class="text-slate-500 text-sm">{{ t('reviews.diff.none_in_round') }}</div>
              </div>
            </div>

            <!-- Document Tab -->
            <div v-if="detailTab === 'document'">
              <!-- Edit toolbar -->
              <div v-if="reviewEditMode" class="space-y-2 mb-3">
                <div class="flex items-center gap-2">
                  <button @click="userIsAuthor ? saveAndResubmit() : saveReviewEdit()" :disabled="savingReviewEdit || (!userIsAuthor && !revisionNote.trim())"
                    class="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors text-white disabled:opacity-50"
                    :class="userIsAuthor ? 'bg-blue-600 hover:bg-blue-500' : 'bg-amber-600 hover:bg-amber-500'">
                    {{ savingReviewEdit ? t('common.state.sending') : userIsAuthor ? t('reviews.edit.save_and_resubmit') : t('reviews.edit.send_revision') }}
                  </button>
                  <span v-if="revisionDraftSaved" class="text-[10px] text-slate-600">{{ t('reviews.edit.draft_saved') }}</span>
                  <button @click="cancelReviewEdit"
                    class="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors text-slate-400 hover:text-slate-200 hover:bg-slate-800 border border-slate-700">
                    {{ t('common.action.cancel') }}
                  </button>
                </div>
<textarea v-model="revisionNote" rows="2"
                  :placeholder="t('reviews.edit.note_placeholder')"
                  class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-xs text-slate-200 placeholder-slate-600 focus:outline-none focus:ring-1 focus:ring-amber-500 resize-none" />
              </div>
              <!-- Editor (same rich editor as Documents) -->
              <div v-if="reviewEditMode" class="space-y-2">
                <div v-if="revisionDraftRecovered"
                  class="flex items-center gap-2 px-3 py-2 bg-blue-950/30 border border-blue-800/30 rounded-lg text-xs text-blue-400">
                  <span>{{ t('reviews.edit.draft_recovered') }}</span>
                  <button @click="reviewEditContent = documentContent; discardRevisionDraft(); revisionDraftRecovered = false" class="text-slate-500 hover:text-slate-300 ml-auto">{{ t('common.action.discard_draft') }}</button>
                </div>
                <div class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
                  <DocumentEditor v-model="reviewEditContent" :editable="true" :documentId="review.document_id" @save="saveReviewEdit" />
                </div>
              </div>
              <!-- Read-only viewer -->
              <div v-else class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden px-8 py-6">
                <DocumentViewer
                  :content="documentContent"
                  :document-id="review.document_id"
                  :review-id="review.id"
                  :show-comments="true"
                  :show-review-progress="true"
                  :can-accept-suggestions="canWrite || review?.requested_by === userEmail"
                  :can-comment="canCommentOnReview"
                  :can-resolve-comments="canResolveReviewComments"
                  @suggestion-accepted="onSuggestionAccepted"
                />
              </div>
            </div>
          </div>

          <!-- Sidebar -->
          <div class="w-full lg:w-64 flex-shrink-0 space-y-4 order-first lg:order-last">
            <!-- Review status summary — always shown for active reviews -->
            <div v-if="review.status !== 'merged' && review.status !== 'closed'"
              class="bg-blue-950/30 border border-blue-800/30 rounded-lg p-4">
              <div class="text-sm font-semibold text-blue-300 mb-1">{{ t('common.label.round', { round: review.round || 1 }) }}</div>
              <div class="text-xs text-blue-400/70 mb-3">
                {{ (review.round || 1) > 1 ? t('reviews.summary.resubmitted') : t('reviews.summary.initial') }}
              </div>
              <div class="border-t border-blue-800/20 pt-2 space-y-1.5">
                <div class="text-xs text-slate-400">
                  <span class="text-slate-500">{{ t('common.label.approved_prefix') }}</span>
                  {{ t('reviews.summary.approved_count', { approved: approvedAssignmentCount, total: assignments.length }) }}
                </div>
                <div v-if="pendingReviewerNames" class="text-xs text-slate-400">
                  <span class="text-slate-500">{{ t('reviews.summary.waiting_on') }}</span>
                  {{ pendingReviewerNames }}
                </div>
                <div v-else-if="review.status === 'approved'" class="text-xs text-emerald-400 font-medium">
                  {{ t('reviews.summary.ready_to_merge') }}
                </div>
                <div v-else-if="review.status === 'changes_requested'" class="text-xs text-amber-400 font-medium">
                  {{ userIsAuthor
                    ? t('reviews.summary.changes_requested_you')
                    : t('reviews.summary.changes_requested_author', { author: authorShortName }) }}
                </div>
              </div>
            </div>

            <!-- Suggestion summary -->
            <div v-if="suggestionSummary.total > 0" class="bg-amber-950/20 border border-amber-800/20 rounded-lg p-4">
              <div class="text-xs font-semibold text-amber-300 mb-2">
                {{ t('reviews.suggestion.count', { count: suggestionSummary.total }, suggestionSummary.total) }}
              </div>
              <div class="space-y-1 text-xs">
                <div v-if="suggestionSummary.pending" class="text-amber-400/80">{{ t('reviews.suggestion.pending', { count: suggestionSummary.pending }) }}</div>
                <div v-if="suggestionSummary.accepted" class="text-emerald-400/80">{{ t('reviews.suggestion.accepted', { count: suggestionSummary.accepted }) }}</div>
                <div v-if="suggestionSummary.rejected" class="text-red-400/80">{{ t('reviews.suggestion.rejected', { count: suggestionSummary.rejected }) }}</div>
              </div>
            </div>

            <!-- Reviewers -->
            <div class="bg-slate-900 border border-slate-800 rounded-lg p-4">
              <h3 class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-3">
                {{ t('common.label.reviewers') }}<span class="text-slate-600 normal-case font-normal"> {{ t('common.label.round_paren', { round: review.round || 1 }) }}</span>
              </h3>
              <div v-if="assignments.length === 0" class="text-xs text-slate-600">{{ t('reviews.assignment.empty') }}</div>
              <div v-else class="space-y-2.5">
                <div v-for="a in assignments" :key="a.id" class="flex items-center gap-2.5">
                  <div class="w-6 h-6 rounded-full flex items-center justify-center text-[9px] font-bold text-white flex-shrink-0"
                    :class="avatarColor(a.reviewer)">
                    {{ initial(a.reviewer) }}
                  </div>
                  <div class="flex-1 min-w-0">
                    <div class="text-sm text-slate-300 truncate">{{ a.reviewer }}</div>
                  </div>
                  <span class="flex-shrink-0 inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold"
                    :class="assignmentStatusClass(a.status)">
                    <span v-if="a.status === 'approved'" class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
                    <span v-else-if="a.status === 'proposed_revision'" class="w-1.5 h-1.5 rounded-full bg-blue-400"></span>
                    <span v-else-if="a.status === 'changes_requested'" class="w-1.5 h-1.5 rounded-full bg-amber-400"></span>
                    <span v-else class="w-1.5 h-1.5 rounded-full bg-slate-500"></span>
                    {{ assignmentStatusLabel(a.status) }}
                  </span>
                </div>
              </div>
            </div>

            <!-- Policy compliance -->
            <div v-if="policyStatus && policyStatus.policies?.length > 0" class="bg-slate-900 border border-slate-800 rounded-lg p-4">
              <h3 class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-3">{{ t('reviews.policy.heading') }}</h3>
              <div class="space-y-2.5">
                <div v-for="p in policyStatus.policies" :key="p.policy_id" class="text-xs">
                  <div class="flex items-center gap-1.5 mb-1">
                    <span class="w-2 h-2 rounded-full flex-shrink-0" :class="p.satisfied ? 'bg-emerald-400' : 'bg-amber-400'" />
                    <span class="text-slate-300 font-medium">{{ p.name }}</span>
                  </div>
                  <div class="pl-3.5 space-y-0.5 text-slate-500">
                    <div>{{ t('reviews.policy.approvals', { current: p.current_approvals, min: p.min_approvals }) }}</div>
                    <div v-if="p.missing_roles?.length" class="text-amber-400/70">{{ t('reviews.policy.missing_roles', { roles: p.missing_roles.join(', ') }) }}</div>
                    <div v-if="p.missing_users?.length" class="text-amber-400/70">{{ t('reviews.policy.missing_users', { users: p.missing_users.join(', ') }) }}</div>
                  </div>
                </div>
              </div>
              <div v-if="!policyStatus.all_satisfied" class="mt-3 pt-2 border-t border-slate-800 text-[10px] text-amber-400">
                {{ t('reviews.policy.blocked') }}
              </div>
              <div v-else-if="policyStatus.can_auto_merge" class="mt-3 pt-2 border-t border-slate-800 text-[10px] text-emerald-400">
                {{ t('reviews.policy.will_auto_merge') }}
              </div>
              <div v-for="p in policyStatus.policies" :key="'info-'+p.policy_id">
                <div v-if="p.require_human && !p.satisfied" class="text-[10px] text-amber-400/70 mt-1 pl-3.5">
                  {{ t('reviews.policy.require_human') }}
                </div>
                <div v-if="p.auto_merge" class="text-[10px] text-emerald-400/70 mt-1 pl-3.5">
                  {{ t('reviews.policy.auto_merge') }}
                </div>
              </div>
            </div>

            <!-- Actions -->
            <div v-if="review.status !== 'merged' && review.status !== 'closed'" class="bg-slate-900 border border-slate-800 rounded-lg p-4 space-y-2">
              <h3 class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-3">{{ t('reviews.actions.heading') }}</h3>

              <!-- Approval feedback -->
              <div v-if="approvalFeedback" class="px-3 py-2 bg-emerald-950/30 border border-emerald-800/30 rounded-lg text-xs text-emerald-400 mb-2">
                {{ approvalFeedback }}
              </div>

              <!-- Review is approved → show Merge only -->
              <!-- Approved → Publish (managers/admins) or info message -->
              <template v-if="review.status === 'approved'">
                <button v-if="canWrite" @click="mergeReview"
                  :disabled="submitting || (policyStatus && !policyStatus.all_satisfied)"
                  class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
                  </svg>
                  {{ publishButtonLabel }}
                </button>
                <div v-if="canWrite" class="text-[10px] text-slate-600 text-center mt-1">
                  {{ publishNote }}
                </div>
                <div v-else class="px-3 py-2 bg-emerald-950/30 border border-emerald-800/30 rounded-lg text-xs text-emerald-400">
                  {{ t('reviews.actions.awaiting_manager') }}
                </div>
              </template>

              <!-- Author view (hidden during edit mode) -->
              <template v-else-if="userIsAuthor && !reviewEditMode">
                <!-- Proposed revision — reviewer edited the document -->
                <div v-if="review.status === 'changes_requested' && hasProposedRevision" class="space-y-2">
                  <div class="px-3 py-2 bg-blue-950/30 border border-blue-800/30 rounded-lg text-xs text-blue-400">
                    <i18n-t keypath="reviews.actions.revision_proposed_by" tag="span" scope="global">
                      <template #name><span class="font-medium">{{ proposedRevisionBy }}</span></template>
                    </i18n-t>
                  </div>
                  <button @click="acceptAndPublish" :disabled="submitting"
                    class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
                    <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                    </svg>
                    {{ submitting ? t('reviews.actions.publishing') : t('reviews.actions.accept_and_publish') }}
                  </button>
                  <button @click="acceptProposedRevision" :disabled="submitting"
                    class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-slate-700 hover:bg-slate-600 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
                    {{ t('reviews.actions.request_another_review') }}
                  </button>
                  <button @click="detailTab = 'document'; startReviewEdit()"
                    class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 text-sm font-medium rounded-lg transition-colors border border-slate-700">
                    {{ t('reviews.actions.propose_new_revision') }}
                  </button>
                </div>
                <!-- Regular changes requested -->
                <div v-else-if="review.status === 'changes_requested'" class="space-y-2">
                  <div class="px-3 py-2 bg-amber-950/30 border border-amber-800/30 rounded-lg text-xs text-amber-400">
                    {{ t('reviews.actions.changes_were_requested') }}
                  </div>
                  <button @click="detailTab = 'document'; startReviewEdit()"
                    class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-amber-600 hover:bg-amber-500 text-white text-sm font-medium rounded-lg transition-colors">
                    <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125" />
                    </svg>
                    {{ t('reviews.actions.edit_and_resubmit') }}
                  </button>
                </div>
                <!-- Waiting for feedback -->
                <div v-else class="px-3 py-2 bg-slate-800/50 border border-slate-700 rounded-lg text-xs text-slate-400">
                  <span class="text-slate-300 font-medium">{{ t('reviews.actions.you_sent_this') }}</span> {{ t('reviews.actions.waiting_for_feedback') }}
                </div>
              </template>

              <!-- Reviewer actions -->
              <template v-else-if="userCanReview">
                <!-- Already acted in this round -->
                <div v-if="userAlreadyActed" class="px-3 py-2 bg-slate-800/50 border border-slate-700 rounded-lg text-xs text-slate-400">
                  <span v-if="userAssignmentStatus === 'approved'" class="text-emerald-400 font-medium">{{ t('reviews.actions.you_approved') }}</span>
                  <span v-else-if="userAssignmentStatus === 'proposed_revision'" class="text-blue-400 font-medium">{{ t('reviews.actions.you_proposed_revision') }}</span>
                  <span v-else-if="userAssignmentStatus === 'changes_requested'" class="text-amber-400 font-medium">{{ t('reviews.actions.you_requested_changes') }}</span>
                  <template v-if="review.status === 'changes_requested'"> {{ t('reviews.actions.waiting_for_author', { author: authorShortName }) }}</template>
                  <template v-else>{{ t('reviews.actions.waiting_for_reviewers') }}</template>
                </div>

                <!-- Actions (only if user hasn't acted yet) -->
                <template v-else>
                  <div v-if="isUpdatedSinceSent" class="px-3 py-2 bg-amber-950/30 border border-amber-800/30 rounded-lg text-xs text-amber-400 mb-2">
                    {{ t('reviews.actions.stale_warning') }}
                  </div>

                  <!-- Default: show all 3 buttons. When one is active, hide the others. -->
                  <template v-if="!activeAction">
                    <button @click="activeAction = 'approve'" :disabled="submitting"
                      class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
                      <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                      </svg>
                      {{ t('common.action.approve') }}
                    </button>

                    <button @click="activeAction = 'changes'" :disabled="submitting"
                      class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
                      <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182" />
                      </svg>
                      {{ t('reviews.actions.request_changes') }}
                    </button>

                    <button @click="startProposeRevision" :disabled="submitting"
                      class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
                      <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125" />
                      </svg>
                      {{ t('reviews.actions.propose_revision') }}
                    </button>
                  </template>

                  <!-- Approve confirmation -->
                  <div v-if="activeAction === 'approve'" class="bg-emerald-950/30 border border-emerald-800/30 rounded-lg p-3 space-y-2">
                    <div class="text-xs text-emerald-400 font-medium">{{ t('reviews.actions.approve_heading') }}</div>
                    <textarea v-model="approveComment" rows="2" :placeholder="t('reviews.actions.approve_placeholder')"
                      class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder-slate-600 focus:outline-none focus:ring-1 focus:ring-emerald-500 resize-none"></textarea>
                    <div class="flex gap-2">
                      <button @click="isUpdatedSinceSent ? confirmApproveStale() : submitApproval('approved')" :disabled="submitting"
                        class="flex-1 px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-xs font-medium rounded-lg transition-colors">
                        {{ submitting ? t('reviews.actions.approving') : t('reviews.actions.confirm_approve') }}
                      </button>
                      <button @click="activeAction = null; approveComment = ''" class="px-3 py-1.5 text-xs text-slate-500 hover:text-slate-300">{{ t('common.action.cancel') }}</button>
                    </div>
                  </div>

                  <!-- Request changes confirmation -->
                  <div v-if="activeAction === 'changes'" class="bg-amber-950/30 border border-amber-800/30 rounded-lg p-3 space-y-2">
                    <div class="text-xs text-amber-400 font-medium">{{ t('reviews.actions.changes_heading') }}</div>
                    <textarea v-model="changesComment" rows="2" :placeholder="t('reviews.actions.changes_placeholder')"
                      class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder-slate-600 focus:outline-none focus:ring-1 focus:ring-amber-500 resize-none"></textarea>
                    <div class="flex gap-2">
                      <button @click="submitApproval('changes_requested')" :disabled="!changesComment.trim() || submitting"
                        class="flex-1 px-3 py-1.5 bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-white text-xs font-medium rounded-lg transition-colors">
                        {{ submitting ? t('reviews.actions.submitting') : t('reviews.actions.submit_request') }}
                      </button>
                      <button @click="activeAction = null; changesComment = ''" class="px-3 py-1.5 text-xs text-slate-500 hover:text-slate-300">{{ t('common.action.cancel') }}</button>
                    </div>
                  </div>
                </template>
              </template>

              <!-- Close/Withdraw — author or manager can close -->
              <div v-if="canWrite || userIsAuthor" class="border-t border-slate-800 pt-2 mt-2">
                <button @click="closeReview" :disabled="submitting"
                  class="w-full flex items-center justify-center gap-2 px-4 py-1.5 text-slate-500 hover:text-red-400 text-xs font-medium transition-colors">
                  {{ userIsAuthor ? t('reviews.actions.withdraw') : t('reviews.actions.close_without_merging') }}
                </button>
              </div>

              <!-- No actions for this user -->
              <div v-if="!canWrite && !userIsAuthor && !userCanReview" class="px-3 py-2 bg-slate-800/50 border border-slate-700 rounded-lg text-xs text-slate-500">
                {{ t('reviews.actions.not_a_reviewer') }}
              </div>
            </div>

            <!-- Merged/Closed badge -->
            <div v-else class="bg-slate-900 border border-slate-800 rounded-lg p-4 text-center">
              <div v-if="review.status === 'merged'" class="text-purple-400 font-semibold text-sm">
                <svg class="w-5 h-5 mx-auto mb-1" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
                </svg>
                {{ t('reviews.closed_state.merged') }}
              </div>
              <div v-else class="text-slate-500 font-semibold text-sm">
                {{ t('reviews.closed_state.closed') }}
              </div>
              <div class="text-xs text-slate-600 mt-1">{{ timeAgo(review.updated_at) }}</div>
            </div>

            <!-- Document info -->
            <div class="bg-slate-900 border border-slate-800 rounded-lg p-4">
              <h3 class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-3">{{ t('reviews.document.heading') }}</h3>
              <div class="space-y-2 text-sm">
                <div class="flex justify-between">
                  <span class="text-slate-600">{{ t('reviews.document.id') }}</span>
                  <router-link :to="orgPath(`/documents/${encodeURIComponent(review.document_id)}`)"
                    class="text-blue-400 hover:text-blue-300 font-mono text-xs transition-colors">
                    {{ review.document_id }}
                  </router-link>
                </div>
                <div class="flex justify-between">
                  <span class="text-slate-600">{{ t('common.label.version') }}</span>
                  <span class="text-slate-300">{{ review.version }}</span>
                </div>
                <div class="flex justify-between">
                  <span class="text-slate-600">{{ t('reviews.document.comments') }}</span>
                  <span class="text-slate-300">{{ review.comment_count || 0 }}</span>
                </div>
                <div v-if="review.open_comments > 0" class="flex justify-between">
                  <span class="text-slate-600">{{ t('reviews.document.open_comments') }}</span>
                  <span class="text-amber-400">{{ review.open_comments }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- ==================== LIST VIEW ==================== -->
    <div v-else class="max-w-5xl mx-auto px-8 py-10 space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">{{ t('reviews.title') }}</h1>
          <p class="text-sm text-slate-500 mt-1">{{ t('reviews.subtitle') }}</p>
        </div>
        <RefreshButton :loading="refreshing" @refresh="reload" />
      </div>

      <!-- Stats tiles -->
      <div class="grid grid-cols-2 lg:grid-cols-5 gap-4">
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-blue-400 tabular-nums">{{ reviewStats.open || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">{{ statOpen }}</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-emerald-400 tabular-nums">{{ reviewStats.approved || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">{{ statApproved }}</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-amber-400 tabular-nums">{{ reviewStats.changes_requested || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">{{ statChangesRequested }}</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-slate-400 tabular-nums">{{ reviewStats.closed || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">{{ statClosed }}</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-purple-400 tabular-nums">{{ reviewStats.merged || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">{{ statMerged }}</div>
        </div>
      </div>

      <!-- Filter tabs -->
      <div class="flex gap-1 bg-slate-900 border border-slate-800 rounded-lg p-1">
        <button @click="statusFilter = 'open'"
          class="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-md transition-colors"
          :class="statusFilter === 'open' ? 'bg-slate-800 text-white' : 'text-slate-500 hover:text-slate-300'">
          {{ statOpen }}
          <span v-if="openCount > 0" class="inline-flex items-center justify-center min-w-[20px] h-5 px-1.5 text-[11px] font-semibold rounded-full"
            :class="statusFilter === 'open' ? 'bg-slate-700 text-slate-200' : 'bg-slate-800 text-slate-400'">
            {{ openCount }}
          </span>
        </button>
        <button @click="statusFilter = 'closed'"
          class="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-md transition-colors"
          :class="statusFilter === 'closed' ? 'bg-slate-800 text-white' : 'text-slate-500 hover:text-slate-300'">
          {{ statClosed }}
          <span v-if="closedCount > 0" class="inline-flex items-center justify-center min-w-[20px] h-5 px-1.5 text-[11px] font-semibold rounded-full"
            :class="statusFilter === 'closed' ? 'bg-slate-700 text-slate-200' : 'bg-slate-800 text-slate-400'">
            {{ closedCount }}
          </span>
        </button>
      </div>

      <!-- Reviews list -->
      <div v-if="filteredReviews.length === 0" class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
        <svg class="w-10 h-10 text-slate-700 mx-auto mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
        </svg>
        <div class="text-slate-500 text-sm">{{ emptyListLabel }}</div>
      </div>

      <div v-else class="bg-slate-900 border border-slate-800 rounded-lg divide-y divide-slate-800 overflow-hidden">
        <div v-for="r in filteredReviews" :key="r.id"
          @click="openReview(r.id)"
          class="px-5 py-4 hover:bg-slate-800/50 cursor-pointer transition-colors group">
          <div class="flex items-start gap-3">
            <!-- Status icon -->
            <div class="mt-0.5 flex-shrink-0">
              <svg v-if="r.status === 'merged'" class="w-5 h-5 text-purple-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
              </svg>
              <svg v-else-if="r.status === 'approved'" class="w-5 h-5 text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <svg v-else-if="r.status === 'changes_requested'" class="w-5 h-5 text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182" />
              </svg>
              <svg v-else-if="r.status === 'closed'" class="w-5 h-5 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
              </svg>
              <div v-else class="w-5 h-5 rounded-full border-2 border-blue-400 flex items-center justify-center">
                <div class="w-2 h-2 rounded-full bg-blue-400"></div>
              </div>
            </div>

            <!-- Content -->
            <div class="flex-1 min-w-0">
              <div class="flex items-baseline gap-2">
                <span class="text-sm font-semibold text-slate-200 group-hover:text-white transition-colors truncate">
                  {{ t('reviews.list.item_title', { id: r.id, title: r.title }) }}
                </span>
              </div>
              <div class="flex items-center gap-2 mt-1 text-xs text-slate-500 flex-wrap">
                <span>{{ r.version }}</span>
                <span class="text-slate-700">*</span>
                <span>{{ t('reviews.list.opened_by', { name: r.requested_by }) }}</span>
                <span class="text-slate-700">*</span>
                <span>{{ timeAgo(r.created_at) }}</span>
                <span v-if="r.comment_count > 0" class="flex items-center gap-1 ml-2 text-slate-500">
                  <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 8.25h9m-9 3H12m-9.75 1.51c0 1.6 1.123 2.994 2.707 3.227 1.129.166 2.27.293 3.423.379.35.026.67.21.865.501L12 21l2.755-4.133a1.14 1.14 0 01.865-.501 48.172 48.172 0 003.423-.379c1.584-.233 2.707-1.626 2.707-3.228V6.741c0-1.602-1.123-2.995-2.707-3.228A48.394 48.394 0 0012 3c-2.392 0-4.744.175-7.043.513C3.373 3.746 2.25 5.14 2.25 6.741v6.018z" />
                  </svg>
                  {{ r.comment_count }}
                </span>
              </div>
              <!-- Reviewer statuses (inline on list items) -->
              <div v-if="r._assignments && r._assignments.length > 0" class="flex items-center gap-3 mt-2">
                <div v-for="a in r._assignments" :key="a.id" class="flex items-center gap-1.5">
                  <div class="w-4 h-4 rounded-full flex items-center justify-center text-[8px] font-bold text-white flex-shrink-0"
                    :class="avatarColor(a.reviewer)">
                    {{ initial(a.reviewer) }}
                  </div>
                  <span class="text-[11px]"
                    :class="a.status === 'approved' ? 'text-emerald-400' : a.status === 'changes_requested' ? 'text-amber-400' : 'text-slate-500'">
                    {{ t('reviews.list.reviewer_status', { name: a.reviewer.split('@')[0], status: assignmentStatusLabel(a.status) }) }}
                  </span>
                </div>
              </div>
            </div>

            <!-- Right side status -->
            <div class="flex-shrink-0">
              <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[10px] font-semibold"
                :class="statusClass(r.status)">
                <span class="w-1.5 h-1.5 rounded-full" :class="statusDotClass(r.status)"></span>
                {{ statusLabel(r.status) }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <Pagination v-if="reviews.length > 0 || page > 1"
        :page="page" :pageSize="pageSize" :total="total"
        @update:page="page = $event" @update:pageSize="pageSize = $event" />
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useConfirm } from '../composables/useConfirm'
import { useRoute, useRouter } from 'vue-router'
import api from '../api'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { defineAsyncComponent } from 'vue'
import DiffView from '../components/DiffView.vue'
import DocumentDiff from '../components/DocumentDiff.vue'
import TrackChanges from '../components/TrackChanges.vue'
import SideBySideReview from '../components/SideBySideReview.vue'
import DocumentViewer from '../components/DocumentViewer.vue'
import Pagination from '../components/Pagination.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import RefreshButton from '../components/RefreshButton.vue'
import { useToast } from '../composables/useToast.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import { formatRelative } from '../composables/useFormat.js'
import { enumLabel } from '../composables/useEnumLabel.js'
import { renderApiError } from '../composables/useApiError.js'
const DocumentEditor = defineAsyncComponent(() => import('../components/DocumentEditor.vue'))

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()
const { show: showError, success: showSaved } = useToast()

// State
const userRole = ref('')
const userEmail = ref('')
const canWrite = computed(() => userRole.value === 'admin' || userRole.value === 'manager')
// Check if current user already acted in this round (non-pending assignment)
const userAlreadyActed = computed(() => {
  if (!userEmail.value || !assignments.value.length) return false
  const myAssignment = assignments.value.find(a => a.reviewer === userEmail.value)
  return myAssignment && myAssignment.status !== 'pending'
})
const userAssignmentStatus = computed(() => {
  if (!userEmail.value) return null
  const myAssignment = assignments.value.find(a => a.reviewer === userEmail.value)
  return myAssignment?.status || null
})

const userIsAuthor = computed(() => review.value?.requested_by === userEmail.value)

// Assignment — not role — is what grants comment rights on a review (CLAUDE.md:
// "Review assignment grants approve/comment rights to any role"), so an assigned
// reader participates fully. Mirrors isReviewParticipant in api_collab.go.
const userIsParticipant = computed(() =>
  userIsAuthor.value || assignments.value.some(a => a.reviewer === userEmail.value))

// Mirrors authorizeReviewComment, including its refusal on a merged or closed
// review — a published or abandoned review is a closed record nobody appends to.
// Hide the composer rather than let it 403 on submit.
const canCommentOnReview = computed(() => {
  if (!review.value) return false
  if (review.value.status === 'merged' || review.value.status === 'closed') return false
  return canWrite.value || userIsParticipant.value
})

// The manager/participant half of handleResolveCommentDB; DocumentViewer adds the
// comment's own author per comment. Resolve is not gated on review status server
// side, so it isn't gated here either.
const canResolveReviewComments = computed(() => canWrite.value || userIsParticipant.value)

// Check if a reviewer proposed a revision (edited the doc and sent back)
const hasProposedRevision = computed(() => {
  return assignments.value.some(a => a.status === 'proposed_revision')
})

const proposedRevisionBy = computed(() => {
  const a = assignments.value.find(a => a.status === 'proposed_revision')
  return a?.reviewer?.split('@')[0] || t('reviews.reviewer_fallback')
})

// The author's local-part, or the translated word "author" when the review
// carries no requester. Two sentences splice it and both used to build the
// fallback inline.
const authorShortName = computed(
  () => review.value?.requested_by?.split('@')[0] || t('reviews.author_fallback'))

const approvedAssignmentCount = computed(
  () => assignments.value.filter(a => a.status === 'approved').length)

// In a computed, not the template: a bare 'pending' in a v-if expression is a
// quoted word the raw-text scanner counts, and it is right to — most of them
// are copy.
const pendingReviewerNames = computed(() => assignments.value
  .filter(a => a.status === 'pending')
  .map(a => a.reviewer.split('@')[0])
  .join(', '))

// Three states in one button, so the ternary lives here rather than in the
// template where the raw-text scanner would count each arm.
const publishButtonLabel = computed(() => {
  if (policyStatus.value && !policyStatus.value.all_satisfied) return t('reviews.actions.requirements_not_met')
  return review.value?.version
    ? t('reviews.actions.publish_version', { version: review.value.version })
    : t('reviews.actions.publish')
})

// Two whole sentences, not one with a spliced fragment: a language may not be
// able to drop "this version" into the same slot a version number fills.
const publishNote = computed(() => review.value?.version
  ? t('reviews.actions.publish_note', { version: review.value.version })
  : t('reviews.actions.publish_note_this_version'))

// The five stat tiles and the two filter tabs name review statuses, so they
// read the same catalogue the badge beside them does. Keeping their own copies
// is what let one screen show "Changes Requested" on the tile and "Changes
// requested" on the badge two inches away.
const statOpen = computed(() => statusLabel('open'))
const statApproved = computed(() => statusLabel('approved'))
const statChangesRequested = computed(() => statusLabel('changes_requested'))
const statClosed = computed(() => statusLabel('closed'))
const statMerged = computed(() => statusLabel('merged'))

const emptyListLabel = computed(() => statusFilter.value === 'closed'
  ? t('reviews.list.empty_closed')
  : t('reviews.list.empty_open'))

const userCanReview = computed(() => {
  if (!userEmail.value || !review.value) return false
  // Author cannot review their own submission
  if (userIsAuthor.value) return false
  // Assigned reviewer can always review
  if (assignments.value.some(a => a.reviewer === userEmail.value)) return true
  // Admin/manager can review even if not assigned
  return canWrite.value
})

const loading = ref(true)
const refreshing = ref(false)
async function reload() {
  refreshing.value = true
  error.value = null
  try {
    await Promise.all([loadReviews(), loadReviewStats()])
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    refreshing.value = false
  }
}
const error = ref(null)
const reviews = ref([])
const statusFilter = ref('open')
const submitting = ref(false)

// Detail state
const review = ref(null)
const assignments = ref([])
const timeline = ref([])
const diffData = ref(null)
const diffMeta = ref(null) // {from, to, current_head}
const documentContent = ref('') // raw markdown content
const diffLoading = ref(false)
const detailTab = ref('changes')
const newComment = ref('')

async function saveAndResubmit() {
  // Author saves edits to review branch, then resubmits for new round
  savingReviewEdit.value = true
  try {
    // 1. Save content to review branch
    await api.updateReviewContent(review.value.id, reviewEditContent.value)
    // 2. Resubmit (triggers new round)
    await api.sendForReview(review.value.document_id, {
      reviewers: assignments.value.map(a => a.reviewer),
      message: revisionNote.value.trim() || t('reviews.revision_note_fallback'),
    })
    reviewEditMode.value = false
    reviewEditContent.value = ''
    revisionNote.value = ''
    activeAction.value = null
    await loadReviewDetail(review.value.id)
  } catch (e) {
    console.error('Save & resubmit failed:', e)
    showError(t('reviews.error.save_and_resubmit', { message: renderApiError(e) }))
  } finally {
    savingReviewEdit.value = false
  }
}

async function acceptAndPublish() {
  submitting.value = true
  try {
    await api.acceptAndMerge(review.value.id)
    approvalFeedback.value = t('reviews.toast.revision_published')
    await loadReviewDetail(review.value.id)
  } catch (e) {
    approvalFeedback.value = t('reviews.error.publish', { message: renderApiError(e) })
  } finally {
    submitting.value = false
  }
}

async function acceptProposedRevision() {
  // Accept the reviewer's proposed revision by resubmitting for another round
  submitting.value = true
  try {
    await api.sendForReview(review.value.document_id, {
      reviewers: assignments.value.map(a => a.reviewer),
      message: t('reviews.toast.revision_accepted'),
    })
    await loadReviewDetail(review.value.id)
  } catch (e) {
    approvalFeedback.value = t('reviews.error.accept_revision', { message: renderApiError(e) })
  } finally {
    submitting.value = false
  }
}

const diffComments = computed(() =>
  timeline.value
    .filter(e => e.type === 'comment' && e.data?.paragraph_index != null)
    .map(e => ({ id: e.data?.id || e.id, author: e.actor, body: e.body, paragraph_index: e.data.paragraph_index, is_outdated: e.data?.is_outdated || false, created_at: e.created_at }))
)

async function onDiffComment({ index, quote, body }) {
  if (!body) return
  try {
    await api.addReviewComment(reviewId.value, {
      body,
      paragraph_index: index,
      quote,
    })
    // Reload timeline to get new comment — stay on current tab
    const tl = await api.getReviewTimeline(reviewId.value)
    timeline.value = Array.isArray(tl) ? tl : []
  } catch {}
}
const reviewGuideHidden = ref(false)
const diffViewMode = ref('track-changes')
const changesViewMode = ref('split')
const diffScope = ref('round') // 'round' = changes in this round, 'all' = all changes in review

// Per-event diff (#6): expand a `proposed_revision` timeline entry to see exactly
// what that one revision changed (the proposal commit vs its parent). Cached by
// commit ref so re-opening is instant.
const openEventDiff = ref(null)
const eventDiff = reactive({})
async function toggleEventDiff(entry) {
  const ref_ = entry.data?.commit_ref
  if (!ref_ || !review.value) return
  if (openEventDiff.value === ref_) { openEventDiff.value = null; return }
  openEventDiff.value = ref_
  if (!eventDiff[ref_]) {
    eventDiff[ref_] = { loading: true, old: '', new: '' }
    try {
      const d = await api.getReviewDiff(review.value.id, null, ref_)
      eventDiff[ref_] = { loading: false, old: d.old_body || '', new: d.new_body || '' }
    } catch {
      eventDiff[ref_] = { loading: false, error: true, old: '', new: '' }
    }
  }
}
const approvalFeedback = ref('')
const policyStatus = ref(null)

// AI review detection: uses is_agent from API responses
const authorIsAgent = ref(false)

const aiReviewActive = computed(() => {
  if (!review.value || !assignments.value.length) return false
  if (review.value.status === 'merged' || review.value.status === 'closed') return false
  const hasAgentReviewer = assignments.value.some(a => a.is_agent)
  return authorIsAgent.value && hasAgentReviewer
})

const aiReviewEscalated = computed(() => {
  if (!aiReviewActive.value) return false
  return (review.value?.round || 1) >= 3 && review.value?.status === 'changes_requested'
})
const oldBody = ref('')
const newBody = ref('')
const allOldBody = ref('')
const allNewBody = ref('')
const allDiffData = ref(null)
const allDiffLoaded = ref(false)
const showChangesInput = ref(false)
const activeAction = ref(null) // 'approve' | 'changes' | null
const approveComment = ref('')
const changesComment = ref('')
const reviewEditMode = ref(false)
const reviewEditContent = ref('')
const savingReviewEdit = ref(false)
const revisionNote = ref('')

// Computed
const reviewId = computed(() => route.params.id ? parseInt(route.params.id) : null)
// orgSlug is provided by useCurrentOrg() above (covers both subdomain and path modes).

// Stats — single source of truth from /reviews/stats (whole-org counts).
const reviewStatsRaw = ref({})
const reviewStats = computed(() => ({
  open: reviewStatsRaw.value.open || 0,
  approved: reviewStatsRaw.value.approved || 0,
  changes_requested: reviewStatsRaw.value.changes_requested || 0,
  closed: reviewStatsRaw.value.closed || 0,
  merged: reviewStatsRaw.value.merged || 0,
}))
const openCount = computed(() => reviewStats.value.open + reviewStats.value.changes_requested + reviewStats.value.approved)
const closedCount = computed(() => reviewStats.value.merged + reviewStats.value.closed)

// Server-side pagination state
const page = ref(1)
const pageSize = ref(25)
const total = ref(0)
const isUpdatedSinceSent = computed(() => {
  if (!review.value || !diffMeta.value) return false
  if (review.value.status === 'merged' || review.value.status === 'closed') return false
  return diffMeta.value.updated_since_sent === true
})
const lastModifiedBy = computed(() => diffMeta.value?.last_modified_by || '')
const lastCommitMsg = computed(() => diffMeta.value?.last_commit_msg || '')

// reviews.value is already phase-filtered server-side, so the list iterates it directly.
// Keep filteredReviews as an alias to minimize template churn.
const filteredReviews = computed(() => reviews.value)

// Watchers: phase change resets page; page/pageSize change reloads.
watch([statusFilter], () => { page.value = 1; loadReviews() })
watch([page, pageSize], () => { loadReviews() })

const detailTabs = computed(() => [
  { key: 'changes', label: t('reviews.tab.changes', { round: review.value?.round || 1 }), count: changedParagraphCount.value },
  { key: 'document', label: t('reviews.tab.document'), count: 0 },
  { key: 'conversation', label: t('reviews.tab.conversation'), count: timeline.value.filter(e => e.type === 'comment').length },
])

// Active diff bodies: switch between current-round and all-changes views
const activeOldBody = computed(() => diffScope.value === 'all' ? allOldBody.value : oldBody.value)
const activeNewBody = computed(() => diffScope.value === 'all' ? allNewBody.value : newBody.value)
const activeDiffData = computed(() => diffScope.value === 'all' ? allDiffData.value : diffData.value)

async function switchDiffScope(scope) {
  if (scope === 'all' && !allDiffLoaded.value && review.value && diffMeta.value?.commit_hash) {
    // Fetch full-review diff BEFORE switching scope (so UI doesn't flash empty)
    try {
      const result = await api.getReviewDiff(review.value.id, diffMeta.value.commit_hash)
      allOldBody.value = result.old_body || ''
      allNewBody.value = result.new_body || ''
      allDiffData.value = result.diff || null
      allDiffLoaded.value = true
    } catch {
      // Fall back to current round diff
      allOldBody.value = oldBody.value
      allNewBody.value = newBody.value
      allDiffData.value = diffData.value
    }
  }
  diffScope.value = scope
}

const suggestionSummary = computed(() => {
  const suggestions = timeline.value.filter(e => e.type === 'comment' && e.data?.suggestion_body)
  return {
    total: suggestions.length,
    pending: suggestions.filter(e => e.data.suggestion_status === 'pending').length,
    accepted: suggestions.filter(e => e.data.suggestion_status === 'accepted').length,
    rejected: suggestions.filter(e => e.data.suggestion_status === 'rejected').length,
  }
})

const changedParagraphCount = computed(() => {
  if (!oldBody.value && newBody.value) return newBody.value.split(/\n{2,}/).length
  if (!oldBody.value || !newBody.value || oldBody.value === newBody.value) return 0
  const oldP = oldBody.value.split(/\n{2,}/)
  const newP = newBody.value.split(/\n{2,}/)
  let changed = 0
  const maxLen = Math.max(oldP.length, newP.length)
  for (let i = 0; i < maxLen; i++) {
    if ((oldP[i] || '') !== (newP[i] || '')) changed++
  }
  return changed
})

// Helpers
function statusClass(status) {
  switch (status) {
    case 'open': return 'bg-blue-500/15 text-blue-400'
    case 'approved': return 'bg-emerald-500/15 text-emerald-400'
    case 'changes_requested': return 'bg-amber-500/15 text-amber-400'
    case 'merged': return 'bg-purple-500/15 text-purple-400'
    case 'closed': return 'bg-slate-500/15 text-slate-400'
    default: return 'bg-slate-500/15 text-slate-400'
  }
}

function statusDotClass(status) {
  switch (status) {
    case 'open': return 'bg-blue-400'
    case 'approved': return 'bg-emerald-400'
    case 'changes_requested': return 'bg-amber-400'
    case 'merged': return 'bg-purple-400'
    case 'closed': return 'bg-slate-400'
    default: return 'bg-slate-400'
  }
}

// The five review statuses are all in common.enum.status, which is also what
// StatusBadge renders — so this is a lookup, not a switch. It changes the case
// of one label: "Changes Requested" becomes the catalogue's "Changes requested",
// the same normalisation the register views took.
function statusLabel(status) {
  return enumLabel('status', status)
}

// Abbreviated assignment status for the reviewer chips, which are too narrow for
// the full label — "changes", not "Changes requested". Authored per language
// rather than derived, and kept in reviews.* because the abbreviation is a
// property of this control's width, not of the value.
// Keys are spelled out rather than built from the value: a concatenated key is
// invisible to the keyset walk, and both value sets are closed anyway.
const ASSIGNMENT_STATUS_KEYS = {
  approved: 'reviews.assignment.status.approved',
  pending: 'reviews.assignment.status.pending',
  changes_requested: 'reviews.assignment.status.changes_requested',
  proposed_revision: 'reviews.assignment.status.proposed_revision',
}
function assignmentStatusLabel(status) {
  const key = ASSIGNMENT_STATUS_KEYS[status]
  return key ? t(key) : status
}

function suggestionStatusLabel(status) {
  return enumLabel('status', status)
}

// The decision record's own wording, which differs from the timeline entry above
// it — "approved (decision record)" against "approved this review". Anything
// that is not merged/approved/proposed_revision is a change request, which is
// how the ternary this replaces behaved.
const DECISION_KEYS = {
  merged: 'reviews.timeline.decision.merged',
  approved: 'reviews.timeline.decision.approved',
  proposed_revision: 'reviews.timeline.decision.proposed_revision',
}
function decisionLabel(decision) {
  return t(DECISION_KEYS[decision] || 'reviews.timeline.decision.changes_requested')
}

function assignmentStatusClass(status) {
  switch (status) {
    case 'approved': return 'bg-emerald-500/15 text-emerald-400'
    case 'changes_requested': return 'bg-amber-500/15 text-amber-400'
    case 'proposed_revision': return 'bg-blue-500/15 text-blue-400'
    default: return 'bg-slate-500/15 text-slate-400'
  }
}

function timelineDotClass(entry) {
  if (entry.type === 'comment') return 'border-blue-500 bg-blue-500/20'
  if (entry.type === 'approval') {
    if (entry.decision === 'approved') return 'border-emerald-500 bg-emerald-500/20'
    if (entry.decision === 'proposed_revision') return 'border-blue-500 bg-blue-500/20'
    if (entry.decision === 'changes_requested') return 'border-amber-500 bg-amber-500/20'
  }
  if (entry.type === 'decision') {
    if (entry.decision === 'merged') return 'border-purple-500 bg-purple-500/20'
    if (entry.decision === 'approved') return 'border-emerald-500 bg-emerald-500/20'
    if (entry.decision === 'proposed_revision') return 'border-blue-500 bg-blue-500/20'
    if (entry.decision === 'changes_requested') return 'border-amber-500 bg-amber-500/20'
    return 'border-slate-500 bg-slate-500/20'
  }
  if (entry.type === 'assignment') return 'border-slate-600 bg-slate-800'
  return 'border-slate-600 bg-slate-800'
}

const avatarColors = ['bg-blue-600', 'bg-emerald-600', 'bg-purple-600', 'bg-amber-600', 'bg-rose-600', 'bg-cyan-600', 'bg-indigo-600', 'bg-pink-600']
function avatarColor(email) {
  if (!email) return 'bg-slate-600'
  let hash = 0
  for (let i = 0; i < email.length; i++) hash = ((hash << 5) - hash) + email.charCodeAt(i)
  return avatarColors[Math.abs(hash) % avatarColors.length]
}

function initial(email) {
  if (!email) return '?'
  return email.charAt(0).toUpperCase()
}

const timeAgo = (dateStr) => formatRelative(dateStr)

// Navigation
function goToList() {
  router.push(orgPath('/reviews'))
}

function openReview(id) {
  router.push(orgPath(`/reviews/${id}`))
}


const { ask } = useConfirm()

async function confirmApproveStale() {
  if (await ask(t('reviews.confirm.approve_stale_body'), { confirm: t('reviews.confirm.approve_stale_title'), variant: 'warning' })) {
    submitApproval('approved')
  }
}

function startProposeRevision() {
  activeAction.value = 'revision'
  detailTab.value = 'document'
  startReviewEdit()
}

// Review document editing — writes to review branch, not main
// Drafts saved to localStorage per review ID
const revisionDraftSaved = ref(false)
const revisionDraftRecovered = ref(false)

function revisionDraftKey() {
  return review.value ? `isms_revision_draft_${review.value.id}` : null
}

function startReviewEdit() {
  const key = revisionDraftKey()
  const saved = key ? localStorage.getItem(key) : null
  revisionDraftRecovered.value = false
  // Only recover if draft exists, is non-empty, and differs from current content
  if (saved && saved.trim() && saved !== documentContent.value) {
    reviewEditContent.value = saved
    revisionNote.value = (key ? localStorage.getItem(key + '_note') : null) || ''
    revisionDraftRecovered.value = true
  } else {
    // No valid draft — start fresh, clear any stale draft
    if (key) localStorage.removeItem(key)
    reviewEditContent.value = documentContent.value
  }
  revisionDraftSaved.value = false
  reviewEditMode.value = true
  startRevisionAutosave()
}

async function cancelReviewEdit() {
  const key = revisionDraftKey()
  if (key && reviewEditContent.value !== documentContent.value) {
    if (!await ask(t('reviews.confirm.discard_changes'), { confirm: t('common.action.discard'), variant: 'danger' })) return
  }
  stopRevisionAutosave()
  reviewEditMode.value = false
  reviewEditContent.value = ''
  revisionDraftSaved.value = false
  activeAction.value = null
}

function discardRevisionDraft() {
  const key = revisionDraftKey()
  if (key) { localStorage.removeItem(key); localStorage.removeItem(key + '_note') }
  revisionDraftSaved.value = false
}

let revisionAutosaveTimer = null
function startRevisionAutosave() {
  stopRevisionAutosave()
  revisionAutosaveTimer = setInterval(() => {
    const key = revisionDraftKey()
    if (key && reviewEditContent.value && reviewEditContent.value !== documentContent.value) {
      localStorage.setItem(key, reviewEditContent.value)
      if (revisionNote.value.trim()) localStorage.setItem(key + '_note', revisionNote.value)
      else localStorage.removeItem(key + '_note')
      revisionDraftSaved.value = true
    }
  }, 5000)
}
function stopRevisionAutosave() {
  if (revisionAutosaveTimer) { clearInterval(revisionAutosaveTimer); revisionAutosaveTimer = null }
}

async function saveReviewEdit() {
  if (savingReviewEdit.value || !review.value) return
  savingReviewEdit.value = true
  try {
    // 1. Save revision to review branch
    await api.updateReviewContent(review.value.id, reviewEditContent.value)
    documentContent.value = reviewEditContent.value
    // 2. Submit proposed_revision decision — review transitions to changes_requested, assignment to proposed_revision
    //    This makes Propose revision a terminal action for this round
    await api.approveReview(review.value.id, 'proposed_revision', revisionNote.value.trim())
    // Clear draft and close editor
    discardRevisionDraft()
    stopRevisionAutosave()
    reviewEditMode.value = false
    reviewEditContent.value = ''
    revisionNote.value = ''
    await loadReviewDetail(review.value.id)
    loadDiff()
  } catch (e) {
    showError(t('reviews.error.submit_revision', { message: renderApiError(e) }))
  } finally {
    savingReviewEdit.value = false
  }
}

// Data loading — server-side filter / sort / pagination via /reviews?page=&limit=&phase=
async function loadReviews() {
  try {
    const params = {
      page: String(page.value),
      limit: String(pageSize.value),
      phase: statusFilter.value, // 'open' or 'closed' — maps to 3-or-2 statuses server-side
    }
    const res = await api.getReviewsPaginated(params)
    reviews.value = Array.isArray(res?.data) ? res.data : []
    total.value = res?.total || 0
    // Load assignments in parallel for the current page only
    await Promise.all(reviews.value.map(async (r) => {
      if (!r.id) { r._assignments = []; return }
      try {
        const asn = await api.getReviewAssignments(r.id)
        r._assignments = Array.isArray(asn) ? asn : []
      } catch {
        r._assignments = []
      }
    }))
  } catch (e) {
    showError(t('reviews.error.load_list', { message: renderApiError(e) }))
  }
}

async function loadReviewStats() {
  try {
    reviewStatsRaw.value = await api.getReviewStats() || {}
  } catch {
    /* non-critical */
  }
}

async function loadReviewDetail(id) {
  try {
    const reviewResp = await api.getReview(id)
    review.value = reviewResp
    authorIsAgent.value = reviewResp?.author_is_agent || false

    const asnResp = await api.getReviewAssignments(id)
    const asn = asnResp?.data || asnResp
    assignments.value = Array.isArray(asn) ? asn : []

    const tl = await api.getReviewTimeline(id)
    timeline.value = Array.isArray(tl) ? tl : []

    // Load document content, diff, and policy status
    loadDocumentContent()
    loadDiff()
    loadPolicyStatus()
  } catch (e) {
    error.value = t('reviews.error.load_one', { message: renderApiError(e) })
  }
}

async function loadPolicyStatus() {
  if (!reviewId.value) return
  try {
    policyStatus.value = await api.getReviewPolicyStatus(reviewId.value)
  } catch {
    policyStatus.value = null
  }
}

async function loadDocumentContent() {
  if (!review.value) return
  try {
    // Read from review branch if it has edits, otherwise from main
    const doc = await api.getReviewContent(review.value.id)
    documentContent.value = doc?.body || ''
  } catch {
    documentContent.value = ''
  }
}

async function onSuggestionAccepted() {
  await loadDocumentContent()
  await loadDiff()
  if (reviewId.value) await loadReviewDetail(reviewId.value)
}

async function loadDiff() {
  if (!review.value) return
  diffLoading.value = true
  // Reset scope state for new review
  diffScope.value = 'round'
  allDiffLoaded.value = false
  allOldBody.value = ''
  allNewBody.value = ''
  allDiffData.value = null
  try {
    const result = await api.getReviewDiff(reviewId.value)
    diffData.value = result?.diff || null
    diffMeta.value = result || null
    oldBody.value = result?.old_body || ''
    newBody.value = result?.new_body || ''
  } catch {
    diffData.value = null
    diffMeta.value = null
    oldBody.value = ''
    newBody.value = ''
  } finally {
    diffLoading.value = false
  }
}

// Actions
async function submitComment() {
  if (!newComment.value.trim() || submitting.value) return
  submitting.value = true
  try {
    await api.addReviewComment(reviewId.value, newComment.value.trim())
    newComment.value = ''
    await loadReviewDetail(reviewId.value)
  } catch (e) {
    showError(t('reviews.error.add_comment', { message: renderApiError(e) }))
  } finally {
    submitting.value = false
  }
}

async function submitApproval(decision) {
  if (submitting.value) return
  const comment = decision === 'changes_requested' ? changesComment.value.trim() : approveComment.value.trim()
  if (decision === 'changes_requested' && !comment) return
  submitting.value = true
  try {
    const result = await api.approveReview(reviewId.value, decision, comment)
    activeAction.value = null
    showChangesInput.value = false
    changesComment.value = ''
    approveComment.value = ''
    // Show feedback with round context
    // The round clause is a suffix on all three sentences, so it stays one
    // param rather than three spliced fragments; the plural on "reviewer" is
    // vue-i18n's, not an appended "s".
    const round = result?.round > 1 ? t('reviews.toast.for_round', { round: result.round }) : ''
    const pending = result?.pending_reviewers?.length || 0
    if (pending > 0) {
      const key = decision === 'approved'
        ? 'reviews.toast.decision_recorded'
        : 'reviews.toast.decision_recorded_changes'
      approvalFeedback.value = t(key, { round, count: pending }, pending)
    } else if (decision === 'approved') {
      approvalFeedback.value = t('reviews.toast.all_approved', { round })
    } else {
      approvalFeedback.value = t('reviews.toast.changes_requested', { round })
    }
    setTimeout(() => { approvalFeedback.value = '' }, 8000)
    await loadReviewDetail(reviewId.value)
  } catch (e) {
    showError(t('reviews.error.submit_decision', { message: renderApiError(e) }))
  } finally {
    submitting.value = false
  }
}

async function mergeReview() {
  if (submitting.value) return
  submitting.value = true
  try {
    await api.mergeReview(reviewId.value)
    await loadReviewDetail(reviewId.value)
    loadReviewStats()
  } catch (e) {
    showError(t('reviews.error.merge', { message: renderApiError(e) }))
  } finally {
    submitting.value = false
  }
}

async function closeReview() {
  if (submitting.value) return
  submitting.value = true
  try {
    await api.updateReviewStatus(reviewId.value, 'closed')
    await loadReviewDetail(reviewId.value)
    loadReviewStats()
  } catch (e) {
    showError(t('reviews.error.close', { message: renderApiError(e) }))
  } finally {
    submitting.value = false
  }
}

// Watch for route changes
watch(() => route.params.id, async (newId) => {
  if (newId) {
    loading.value = true
    error.value = null
    await loadReviewDetail(parseInt(newId))
    loading.value = false
  } else {
    review.value = null
    timeline.value = []
    assignments.value = []
    diffData.value = null
    detailTab.value = 'changes'
  }
}, { immediate: false })

// Load diff when Changes tab is selected
watch(detailTab, (tab) => {
  if (tab === 'changes' && !diffData.value && !diffLoading.value) {
    loadDiff()
  }
})

// Initial load
onMounted(async () => {
  try { const me = await api.getMe(); userRole.value = me?.role || ''; userEmail.value = me?.email || '' } catch {}
  try {
    await Promise.all([loadReviews(), loadReviewStats()])
    if (reviewId.value) {
      await loadReviewDetail(reviewId.value)
    }
  } catch (e) {
    error.value = renderApiError(e)
  } finally {
    loading.value = false
  }
})

// Warn before leaving with unsaved revision
function beforeUnloadHandler(e) {
  if (reviewEditMode.value && reviewEditContent.value !== documentContent.value) {
    e.preventDefault()
    e.returnValue = ''
  }
}
window.addEventListener('beforeunload', beforeUnloadHandler)

onBeforeUnmount(() => {
  stopRevisionAutosave()
  window.removeEventListener('beforeunload', beforeUnloadHandler)
})
</script>
