<template>
  <div class="min-h-full">
    <!-- Loading -->
    <div v-if="loading" class="max-w-6xl mx-auto px-8 py-10">
      <ListSkeleton :rows="5" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="max-w-5xl mx-auto px-8 py-12">
      <div class="bg-red-950/40 border border-red-900/50 rounded-lg p-6 text-red-300 text-sm">
        {{ error }}
      </div>
    </div>

    <!-- ==================== DETAIL VIEW ==================== -->
    <div v-else-if="reviewId" class="px-6 py-6">
      <!-- Back link -->
      <button @click="goToList" class="flex items-center gap-1.5 text-sm text-slate-500 hover:text-slate-300 mb-6 transition-colors">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M10.5 19.5L3 12m0 0l7.5-7.5M3 12h18" />
        </svg>
        Back to reviews
      </button>

      <div v-if="!review" class="flex items-center justify-center h-64">
        <div class="text-slate-500 text-sm">Review not found</div>
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
                  Round {{ review.round || 1 }}
                </span>
                <span v-if="aiReviewActive" class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-purple-500/15 text-purple-400">
                  AI review in progress
                </span>
                <span v-if="aiReviewEscalated" class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-red-500/15 text-red-400">
                  Escalated — AI review stalled
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
                <span class="text-xs text-slate-600">opened {{ timeAgo(review.created_at) }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Review message -->
        <div v-if="review.message" class="mb-6 px-4 py-3 bg-slate-900 border border-slate-800 rounded-lg">
          <div class="text-sm text-slate-300 leading-relaxed whitespace-pre-wrap">{{ review.message }}</div>
        </div>

        <!-- Review guide for first-time visitors -->
        <div v-if="review.status === 'open' && !reviewGuideHidden && !userIsAuthor" class="mb-6 bg-blue-950/20 border border-blue-800/20 rounded-xl px-5 py-4">
          <div class="flex items-start justify-between">
            <div class="space-y-2 text-sm text-blue-300/80">
              <div class="flex items-center gap-2"><span class="w-5 h-5 rounded-full bg-blue-600/30 flex items-center justify-center text-xs font-bold text-blue-300">1</span> Review the document and changes</div>
              <div class="flex items-center gap-2"><span class="w-5 h-5 rounded-full bg-blue-600/30 flex items-center justify-center text-xs font-bold text-blue-300">2</span> Add comments or suggest edits</div>
              <div class="flex items-center gap-2"><span class="w-5 h-5 rounded-full bg-blue-600/30 flex items-center justify-center text-xs font-bold text-blue-300">3</span> Approve or request changes</div>
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
              <button v-for="t in detailTabs" :key="t.key" @click="detailTab = t.key"
                class="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-md transition-colors"
                :class="detailTab === t.key ? 'bg-slate-800 text-white' : 'text-slate-500 hover:text-slate-300'">
                {{ t.label }}
                <span v-if="t.count > 0" class="inline-flex items-center justify-center min-w-[20px] h-5 px-1.5 text-[11px] font-semibold rounded-full"
                  :class="detailTab === t.key ? 'bg-slate-700 text-slate-200' : 'bg-slate-800 text-slate-400'">
                  {{ t.count }}
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
                    <span class="text-xs font-semibold text-slate-500 whitespace-nowrap">Round {{ entry.round }}</span>
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
                      <span class="text-xs text-slate-600">{{ entry.data?.suggestion_body ? 'suggested an edit' : 'commented' }} {{ timeAgo(entry.created_at) }}</span>
                      <span v-if="entry.data?.suggestion_status"
                        class="text-[10px] font-semibold px-1.5 py-0.5 rounded-full"
                        :class="{ 'bg-amber-800/40 text-amber-300': entry.data.suggestion_status === 'pending', 'bg-emerald-800/40 text-emerald-300': entry.data.suggestion_status === 'accepted', 'bg-red-800/40 text-red-300': entry.data.suggestion_status === 'rejected' }">
                        {{ entry.data.suggestion_status }}
                      </span>
                      <span v-if="entry.round > 1" class="text-[10px] text-slate-600 px-1.5 py-0.5 rounded bg-slate-800/50 font-medium">R{{ entry.round }}</span>
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
                    <span v-if="entry.decision === 'approved'" class="text-sm text-emerald-400 font-medium">approved this review</span>
                    <span v-else-if="entry.decision === 'proposed_revision'" class="text-sm text-blue-400 font-medium">proposed a revision</span>
                    <span v-else-if="entry.decision === 'changes_requested'" class="text-sm text-amber-400 font-medium">requested changes</span>
                    <span class="text-xs text-slate-600 ml-1">{{ timeAgo(entry.created_at) }}</span>
                    <span v-if="entry.round > 1" class="text-[10px] text-slate-600 px-1.5 py-0.5 rounded bg-slate-800/50 font-medium">R{{ entry.round }}</span>
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
                    <span class="text-slate-400">
                      <span class="text-slate-300 font-medium">{{ entry.reviewer }}</span> was assigned as reviewer
                    </span>
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
                        {{ entry.decision === 'merged' ? 'merged this review' : entry.decision === 'approved' ? 'approved (decision record)' : entry.decision === 'proposed_revision' ? 'proposed revision (decision record)' : 'requested changes (decision record)' }}
                      </span>
                      <span class="text-xs text-slate-600 ml-1">{{ timeAgo(entry.created_at) }}</span>
                    </div>
                    <div v-if="entry.detail" class="text-xs text-slate-400 ml-6 mt-1 italic">"{{ entry.detail }}"</div>
                    <div v-if="entry.content_hash" class="flex items-center gap-1.5 ml-6 mt-1.5">
                      <svg class="w-3 h-3 text-slate-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M7.864 4.243A7.5 7.5 0 0119.5 10.5c0 2.92-.556 5.709-1.568 8.268M5.742 6.364A7.465 7.465 0 004.5 10.5a7.464 7.464 0 01-1.15 3.993m1.989 3.559A11.209 11.209 0 008.25 10.5a3.75 3.75 0 117.5 0c0 .527-.021 1.049-.064 1.565M12 10.5a14.94 14.94 0 01-3.6 9.75m6.633-4.596a18.666 18.666 0 01-2.485 5.33" />
                      </svg>
                      <span class="text-[10px] font-mono text-slate-600" title="SHA-256 content hash for tamper evidence">{{ entry.content_hash.substring(0, 16) }}...</span>
                    </div>
                    <!-- Per-event diff: exactly what this revision changed (#6) -->
                    <div v-if="entry.decision === 'proposed_revision' && entry.data?.commit_ref" class="ml-6 mt-2">
                      <button @click="toggleEventDiff(entry)" class="text-xs font-medium text-blue-400 hover:text-blue-300 transition-colors">
                        {{ openEventDiff === entry.data.commit_ref ? 'Hide changes' : 'View changes' }}
                      </button>
                      <div v-if="openEventDiff === entry.data.commit_ref" class="mt-2 bg-slate-900 border border-slate-800 rounded-lg overflow-hidden">
                        <div v-if="eventDiff[entry.data.commit_ref]?.loading" class="px-4 py-3 text-xs text-slate-500">Loading changes…</div>
                        <div v-else-if="eventDiff[entry.data.commit_ref]?.error" class="px-4 py-3 text-xs text-red-400">Couldn't load this revision's diff.</div>
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
                  <div class="text-sm text-blue-300 font-medium">Document updated since review was sent</div>
                  <div v-if="lastModifiedBy" class="text-xs text-blue-400/70 mt-0.5">
                    Last modified by {{ lastModifiedBy }}<span v-if="lastCommitMsg"> — {{ lastCommitMsg }}</span>
                  </div>
                  <div class="text-xs text-slate-500 mt-0.5">The diff below shows all changes since the last approved version.</div>
                </div>
              </div>

              <!-- Changes summary (always visible at top of conversation) -->
              <div v-if="oldBody || newBody" class="mb-6">
                <div class="text-xs text-slate-500 font-medium mb-2">Changes in this review</div>
                <div class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
                  <TrackChanges :old-body="oldBody" :new-body="newBody" :author="diffMeta?.last_modified_by" :date="diffMeta?.last_modified" :document-id="review?.document_id || ''" :blame-ref="diffMeta?.has_branch ? `review/${review?.id}` : ''" />
                </div>
              </div>
              <div v-else-if="!diffLoading" class="mb-6 px-4 py-3 bg-slate-900 border border-slate-800 rounded-lg text-xs text-slate-500">
                <span v-if="!diffMeta?.commit_hash">Full review — this document has never been approved. Review the full document in the Document tab.</span>
                <span v-else>No diff available — the document may not have been modified since the review base.</span>
              </div>

              <!-- Empty state -->
              <div v-if="timeline.length === 0 && !diffData" class="text-center py-12 text-slate-500 text-sm">
                No activity yet.
              </div>

              <!-- Comment input -->
              <div v-if="review.status !== 'merged' && review.status !== 'closed'" class="mt-6 bg-slate-900 border border-slate-800 rounded-lg overflow-hidden">
                <div class="px-4 py-2.5 border-b border-slate-800">
                  <span class="text-xs text-slate-500 font-medium">Add a comment</span>
                </div>
                <textarea v-model="newComment" rows="3" placeholder="Leave a comment..."
                  class="w-full bg-transparent px-4 py-3 text-sm text-slate-200 placeholder-slate-600 focus:outline-none resize-none"></textarea>
                <div class="flex justify-end px-4 py-2.5 border-t border-slate-800">
                  <button @click="submitComment" :disabled="!newComment.trim() || submitting"
                    class="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors">
                    Comment
                  </button>
                </div>
              </div>
            </div>

            <!-- Changes Tab -->
            <div v-if="detailTab === 'changes'">
              <div v-if="diffLoading" class="flex items-center justify-center h-48">
                <div class="text-slate-400 text-sm">Loading changes...</div>
              </div>
              <div v-else-if="oldBody || newBody || activeOldBody || activeNewBody" class="space-y-3">
                <!-- Scope + view mode toggles -->
                <div class="flex items-center justify-between">
                  <!-- Diff scope: this round vs all changes -->
                  <div v-if="review.round > 1" class="flex items-center gap-0.5 bg-slate-900 border border-slate-800 rounded-lg p-0.5">
                    <button @click="switchDiffScope('round')" class="px-3 py-1.5 rounded-md text-[11px] font-medium transition-colors"
                      :class="diffScope === 'round' ? 'bg-blue-600 text-white' : 'text-slate-500 hover:text-slate-300'">
                      Changes in this round
                    </button>
                    <button @click="switchDiffScope('all')" class="px-3 py-1.5 rounded-md text-[11px] font-medium transition-colors"
                      :class="diffScope === 'all' ? 'bg-blue-600 text-white' : 'text-slate-500 hover:text-slate-300'">
                      All changes in review
                    </button>
                  </div>
                  <div v-else></div>
                  <!-- View mode toggle -->
                  <div class="flex items-center gap-0.5 bg-slate-900 border border-slate-800 rounded-lg p-0.5">
                    <button @click="changesViewMode = 'split'" class="px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors"
                      :class="changesViewMode === 'split' ? 'bg-slate-700 text-white' : 'text-slate-500 hover:text-slate-300'">
                      Split
                    </button>
                    <button @click="changesViewMode = 'unified'" class="px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors"
                      :class="changesViewMode === 'unified' ? 'bg-slate-700 text-white' : 'text-slate-500 hover:text-slate-300'">
                      Unified
                    </button>
                  </div>
                </div>
                <!-- Split view (side by side with comments) -->
                <SideBySideReview v-if="changesViewMode === 'split'"
                  :old-body="activeOldBody" :new-body="activeNewBody" :raw-diff="activeDiffData || ''"
                  :comment-count="review.comment_count || 0" :review-status="statusLabel(review.status)"
                  :readonly="review.status === 'merged' || review.status === 'closed'"
                  :comments="diffComments"
                  @comment="onDiffComment" />
                <!-- Unified view (inline track changes with comments) -->
                <TrackChanges v-else
                  :old-body="activeOldBody" :new-body="activeNewBody"
                  :author="diffMeta?.last_modified_by" :date="diffMeta?.last_modified"
                  :document-id="review?.document_id || ''" :blame-ref="diffMeta?.has_branch ? `review/${review?.id}` : ''"
                  :comments="diffComments"
                  :readonly="review.status === 'merged' || review.status === 'closed'"
                  @comment="onDiffComment" />

              </div>
              <div v-else class="bg-slate-900 border border-slate-800 rounded-lg p-12 text-center">
                <div v-if="!diffMeta?.commit_hash" class="space-y-2">
                  <div class="text-slate-400 text-sm font-medium">Full review — this document has never been approved</div>
                  <div class="text-slate-500 text-xs">There is no prior approved version to diff against. Review the full document in the Document tab.</div>
                </div>
                <div v-else class="text-slate-500 text-sm">No changes found in this round.</div>
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
                    {{ savingReviewEdit ? 'Sending...' : userIsAuthor ? 'Save &amp; Resubmit' : 'Send proposed revision' }}
                  </button>
                  <span v-if="revisionDraftSaved" class="text-[10px] text-slate-600">Draft saved</span>
                  <button @click="cancelReviewEdit"
                    class="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors text-slate-400 hover:text-slate-200 hover:bg-slate-800 border border-slate-700">
                    Cancel
                  </button>
                </div>
<textarea v-model="revisionNote" rows="2"
                  placeholder="What did you change and why?"
                  class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-xs text-slate-200 placeholder-slate-600 focus:outline-none focus:ring-1 focus:ring-amber-500 resize-none" />
              </div>
              <!-- Editor (same rich editor as Documents) -->
              <div v-if="reviewEditMode" class="space-y-2">
                <div v-if="revisionDraftRecovered"
                  class="flex items-center gap-2 px-3 py-2 bg-blue-950/30 border border-blue-800/30 rounded-lg text-xs text-blue-400">
                  <span>Draft recovered from previous session.</span>
                  <button @click="reviewEditContent = documentContent; discardRevisionDraft(); revisionDraftRecovered = false" class="text-slate-500 hover:text-slate-300 ml-auto">Discard draft</button>
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
              <div class="text-sm font-semibold text-blue-300 mb-1">Round {{ review.round || 1 }}</div>
              <div class="text-xs text-blue-400/70 mb-3">
                {{ (review.round || 1) > 1 ? 'Resubmitted after requested changes' : 'Initial review' }}
              </div>
              <div class="border-t border-blue-800/20 pt-2 space-y-1.5">
                <div class="text-xs text-slate-400">
                  <span class="text-slate-500">Approved:</span>
                  {{ assignments.filter(a => a.status === 'approved').length }} of {{ assignments.length }}
                </div>
                <div v-if="assignments.filter(a => a.status === 'pending').length > 0" class="text-xs text-slate-400">
                  <span class="text-slate-500">Waiting on:</span>
                  {{ assignments.filter(a => a.status === 'pending').map(a => a.reviewer.split('@')[0]).join(', ') }}
                </div>
                <div v-else-if="review.status === 'approved'" class="text-xs text-emerald-400 font-medium">
                  All reviewers have approved. Ready to merge.
                </div>
                <div v-else-if="review.status === 'changes_requested'" class="text-xs text-amber-400 font-medium">
                  Changes requested. {{ userIsAuthor ? 'Waiting for you to respond.' : 'Waiting for ' + (review.requested_by?.split('@')[0] || 'author') + ' to respond.' }}
                </div>
              </div>
            </div>

            <!-- Suggestion summary -->
            <div v-if="suggestionSummary.total > 0" class="bg-amber-950/20 border border-amber-800/20 rounded-lg p-4">
              <div class="text-xs font-semibold text-amber-300 mb-2">
                {{ suggestionSummary.total }} suggestion{{ suggestionSummary.total === 1 ? '' : 's' }}
              </div>
              <div class="space-y-1 text-xs">
                <div v-if="suggestionSummary.pending" class="text-amber-400/80">{{ suggestionSummary.pending }} pending</div>
                <div v-if="suggestionSummary.accepted" class="text-emerald-400/80">{{ suggestionSummary.accepted }} accepted</div>
                <div v-if="suggestionSummary.rejected" class="text-red-400/80">{{ suggestionSummary.rejected }} rejected</div>
              </div>
            </div>

            <!-- Reviewers -->
            <div class="bg-slate-900 border border-slate-800 rounded-lg p-4">
              <h3 class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-3">
                Reviewers<span class="text-slate-600 normal-case font-normal"> (Round {{ review.round || 1 }})</span>
              </h3>
              <div v-if="assignments.length === 0" class="text-xs text-slate-600">No reviewers assigned</div>
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
                    {{ a.status === 'changes_requested' ? 'changes' : a.status === 'proposed_revision' ? 'revision' : a.status }}
                  </span>
                </div>
              </div>
            </div>

            <!-- Policy compliance -->
            <div v-if="policyStatus && policyStatus.policies?.length > 0" class="bg-slate-900 border border-slate-800 rounded-lg p-4">
              <h3 class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-3">Approval Requirements</h3>
              <div class="space-y-2.5">
                <div v-for="p in policyStatus.policies" :key="p.policy_id" class="text-xs">
                  <div class="flex items-center gap-1.5 mb-1">
                    <span class="w-2 h-2 rounded-full flex-shrink-0" :class="p.satisfied ? 'bg-emerald-400' : 'bg-amber-400'" />
                    <span class="text-slate-300 font-medium">{{ p.name }}</span>
                  </div>
                  <div class="pl-3.5 space-y-0.5 text-slate-500">
                    <div>{{ p.current_approvals }}/{{ p.min_approvals }} approvals</div>
                    <div v-if="p.missing_roles?.length" class="text-amber-400/70">Missing: {{ p.missing_roles.join(', ') }} role</div>
                    <div v-if="p.missing_users?.length" class="text-amber-400/70">Awaiting: {{ p.missing_users.join(', ') }}</div>
                  </div>
                </div>
              </div>
              <div v-if="!policyStatus.all_satisfied" class="mt-3 pt-2 border-t border-slate-800 text-[10px] text-amber-400">
                Merge blocked until all requirements are met
              </div>
              <div v-else-if="policyStatus.can_auto_merge" class="mt-3 pt-2 border-t border-slate-800 text-[10px] text-emerald-400">
                Will auto-merge when all approvals are in
              </div>
              <div v-for="p in policyStatus.policies" :key="'info-'+p.policy_id">
                <div v-if="p.require_human && !p.satisfied" class="text-[10px] text-amber-400/70 mt-1 pl-3.5">
                  At least one human approval required
                </div>
                <div v-if="p.auto_merge" class="text-[10px] text-emerald-400/70 mt-1 pl-3.5">
                  Auto-merge enabled for this policy
                </div>
              </div>
            </div>

            <!-- Actions -->
            <div v-if="review.status !== 'merged' && review.status !== 'closed'" class="bg-slate-900 border border-slate-800 rounded-lg p-4 space-y-2">
              <h3 class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-3">Actions</h3>

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
                  {{ policyStatus && !policyStatus.all_satisfied ? 'Requirements not met' : review.version ? `Publish ${review.version}` : 'Publish' }}
                </button>
                <div v-if="canWrite" class="text-[10px] text-slate-600 text-center mt-1">
                  This will make {{ review.version || 'this version' }} the accepted document.
                </div>
                <div v-else class="px-3 py-2 bg-emerald-950/30 border border-emerald-800/30 rounded-lg text-xs text-emerald-400">
                  All reviewers approved. Waiting for a manager to publish.
                </div>
              </template>

              <!-- Author view (hidden during edit mode) -->
              <template v-else-if="userIsAuthor && !reviewEditMode">
                <!-- Proposed revision — reviewer edited the document -->
                <div v-if="review.status === 'changes_requested' && hasProposedRevision" class="space-y-2">
                  <div class="px-3 py-2 bg-blue-950/30 border border-blue-800/30 rounded-lg text-xs text-blue-400">
                    <span class="font-medium">{{ proposedRevisionBy }}</span> proposed a revision to this document.
                  </div>
                  <button @click="acceptAndPublish" :disabled="submitting"
                    class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
                    <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                    </svg>
                    {{ submitting ? 'Publishing...' : 'Accept &amp; Publish' }}
                  </button>
                  <button @click="acceptProposedRevision" :disabled="submitting"
                    class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-slate-700 hover:bg-slate-600 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
                    Request Another Review
                  </button>
                  <button @click="detailTab = 'document'; startReviewEdit()"
                    class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 text-sm font-medium rounded-lg transition-colors border border-slate-700">
                    Propose New Revision
                  </button>
                </div>
                <!-- Regular changes requested -->
                <div v-else-if="review.status === 'changes_requested'" class="space-y-2">
                  <div class="px-3 py-2 bg-amber-950/30 border border-amber-800/30 rounded-lg text-xs text-amber-400">
                    Changes were requested. Edit the document and resubmit.
                  </div>
                  <button @click="detailTab = 'document'; startReviewEdit()"
                    class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-amber-600 hover:bg-amber-500 text-white text-sm font-medium rounded-lg transition-colors">
                    <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125" />
                    </svg>
                    Edit &amp; Resubmit
                  </button>
                </div>
                <!-- Waiting for feedback -->
                <div v-else class="px-3 py-2 bg-slate-800/50 border border-slate-700 rounded-lg text-xs text-slate-400">
                  <span class="text-slate-300 font-medium">You sent this review.</span> Waiting for reviewer feedback.
                </div>
              </template>

              <!-- Reviewer actions -->
              <template v-else-if="userCanReview">
                <!-- Already acted in this round -->
                <div v-if="userAlreadyActed" class="px-3 py-2 bg-slate-800/50 border border-slate-700 rounded-lg text-xs text-slate-400">
                  <span v-if="userAssignmentStatus === 'approved'" class="text-emerald-400 font-medium">You approved this round.</span>
                  <span v-else-if="userAssignmentStatus === 'proposed_revision'" class="text-blue-400 font-medium">You proposed a revision.</span>
                  <span v-else-if="userAssignmentStatus === 'changes_requested'" class="text-amber-400 font-medium">You requested changes.</span>
                  <template v-if="review.status === 'changes_requested'"> Waiting for {{ review.requested_by?.split('@')[0] || 'author' }} to respond.</template>
                  <template v-else>Waiting for other reviewers.</template>
                </div>

                <!-- Actions (only if user hasn't acted yet) -->
                <template v-else>
                  <div v-if="isUpdatedSinceSent" class="px-3 py-2 bg-amber-950/30 border border-amber-800/30 rounded-lg text-xs text-amber-400 mb-2">
                    Document was updated after this review was sent. Review the latest changes before approving.
                  </div>

                  <!-- Default: show all 3 buttons. When one is active, hide the others. -->
                  <template v-if="!activeAction">
                    <button @click="activeAction = 'approve'" :disabled="submitting"
                      class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
                      <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                      </svg>
                      Approve
                    </button>

                    <button @click="activeAction = 'changes'" :disabled="submitting"
                      class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
                      <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182" />
                      </svg>
                      Request Changes
                    </button>

                    <button @click="startProposeRevision" :disabled="submitting"
                      class="w-full flex items-center justify-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
                      <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125" />
                      </svg>
                      Propose Revision
                    </button>
                  </template>

                  <!-- Approve confirmation -->
                  <div v-if="activeAction === 'approve'" class="bg-emerald-950/30 border border-emerald-800/30 rounded-lg p-3 space-y-2">
                    <div class="text-xs text-emerald-400 font-medium">Approve this document</div>
                    <textarea v-model="approveComment" rows="2" placeholder="Optional comment..."
                      class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder-slate-600 focus:outline-none focus:ring-1 focus:ring-emerald-500 resize-none"></textarea>
                    <div class="flex gap-2">
                      <button @click="isUpdatedSinceSent ? confirmApproveStale() : submitApproval('approved')" :disabled="submitting"
                        class="flex-1 px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-xs font-medium rounded-lg transition-colors">
                        {{ submitting ? 'Approving...' : 'Confirm Approve' }}
                      </button>
                      <button @click="activeAction = null; approveComment = ''" class="px-3 py-1.5 text-xs text-slate-500 hover:text-slate-300">Cancel</button>
                    </div>
                  </div>

                  <!-- Request changes confirmation -->
                  <div v-if="activeAction === 'changes'" class="bg-amber-950/30 border border-amber-800/30 rounded-lg p-3 space-y-2">
                    <div class="text-xs text-amber-400 font-medium">Request changes</div>
                    <textarea v-model="changesComment" rows="2" placeholder="Describe what needs to change... (required)"
                      class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 placeholder-slate-600 focus:outline-none focus:ring-1 focus:ring-amber-500 resize-none"></textarea>
                    <div class="flex gap-2">
                      <button @click="submitApproval('changes_requested')" :disabled="!changesComment.trim() || submitting"
                        class="flex-1 px-3 py-1.5 bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-white text-xs font-medium rounded-lg transition-colors">
                        {{ submitting ? 'Submitting...' : 'Submit Request' }}
                      </button>
                      <button @click="activeAction = null; changesComment = ''" class="px-3 py-1.5 text-xs text-slate-500 hover:text-slate-300">Cancel</button>
                    </div>
                  </div>
                </template>
              </template>

              <!-- Close/Withdraw — author or manager can close -->
              <div v-if="canWrite || userIsAuthor" class="border-t border-slate-800 pt-2 mt-2">
                <button @click="closeReview" :disabled="submitting"
                  class="w-full flex items-center justify-center gap-2 px-4 py-1.5 text-slate-500 hover:text-red-400 text-xs font-medium transition-colors">
                  {{ userIsAuthor ? 'Withdraw Review' : 'Close Without Merging' }}
                </button>
              </div>

              <!-- No actions for this user -->
              <div v-if="!canWrite && !userIsAuthor && !userCanReview" class="px-3 py-2 bg-slate-800/50 border border-slate-700 rounded-lg text-xs text-slate-500">
                You are not a reviewer on this request.
              </div>
            </div>

            <!-- Merged/Closed badge -->
            <div v-else class="bg-slate-900 border border-slate-800 rounded-lg p-4 text-center">
              <div v-if="review.status === 'merged'" class="text-purple-400 font-semibold text-sm">
                <svg class="w-5 h-5 mx-auto mb-1" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
                </svg>
                Merged
              </div>
              <div v-else class="text-slate-500 font-semibold text-sm">
                Closed
              </div>
              <div class="text-xs text-slate-600 mt-1">{{ timeAgo(review.updated_at) }}</div>
            </div>

            <!-- Document info -->
            <div class="bg-slate-900 border border-slate-800 rounded-lg p-4">
              <h3 class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-3">Document</h3>
              <div class="space-y-2 text-sm">
                <div class="flex justify-between">
                  <span class="text-slate-600">ID</span>
                  <router-link :to="orgPath(`/documents/${encodeURIComponent(review.document_id)}`)"
                    class="text-blue-400 hover:text-blue-300 font-mono text-xs transition-colors">
                    {{ review.document_id }}
                  </router-link>
                </div>
                <div class="flex justify-between">
                  <span class="text-slate-600">Version</span>
                  <span class="text-slate-300">{{ review.version }}</span>
                </div>
                <div class="flex justify-between">
                  <span class="text-slate-600">Comments</span>
                  <span class="text-slate-300">{{ review.comment_count || 0 }}</span>
                </div>
                <div v-if="review.open_comments > 0" class="flex justify-between">
                  <span class="text-slate-600">Open</span>
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
          <h1 class="text-2xl font-bold text-slate-100 tracking-tight">Reviews</h1>
          <p class="text-sm text-slate-500 mt-1">Document review requests</p>
        </div>
      </div>

      <!-- Stats tiles -->
      <div class="grid grid-cols-2 lg:grid-cols-5 gap-4">
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-blue-400 tabular-nums">{{ reviewStats.open || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Open</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-emerald-400 tabular-nums">{{ reviewStats.approved || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Approved</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-amber-400 tabular-nums">{{ reviewStats.changes_requested || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Changes Requested</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-slate-400 tabular-nums">{{ reviewStats.closed || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Closed</div>
        </div>
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <div class="text-2xl font-bold text-purple-400 tabular-nums">{{ reviewStats.merged || 0 }}</div>
          <div class="text-xs text-slate-500 mt-1">Merged</div>
        </div>
      </div>

      <!-- Filter tabs -->
      <div class="flex gap-1 bg-slate-900 border border-slate-800 rounded-lg p-1">
        <button @click="statusFilter = 'open'"
          class="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-md transition-colors"
          :class="statusFilter === 'open' ? 'bg-slate-800 text-white' : 'text-slate-500 hover:text-slate-300'">
          Open
          <span v-if="openCount > 0" class="inline-flex items-center justify-center min-w-[20px] h-5 px-1.5 text-[11px] font-semibold rounded-full"
            :class="statusFilter === 'open' ? 'bg-slate-700 text-slate-200' : 'bg-slate-800 text-slate-400'">
            {{ openCount }}
          </span>
        </button>
        <button @click="statusFilter = 'closed'"
          class="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-md transition-colors"
          :class="statusFilter === 'closed' ? 'bg-slate-800 text-white' : 'text-slate-500 hover:text-slate-300'">
          Closed
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
        <div class="text-slate-500 text-sm">No {{ statusFilter }} reviews</div>
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
                  Review #{{ r.id }} -- {{ r.title }}
                </span>
              </div>
              <div class="flex items-center gap-2 mt-1 text-xs text-slate-500 flex-wrap">
                <span>{{ r.version }}</span>
                <span class="text-slate-700">*</span>
                <span>Opened by {{ r.requested_by }}</span>
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
                    {{ a.reviewer.split('@')[0] }} ({{ a.status === 'changes_requested' ? 'changes' : a.status }})
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
import { useToast } from '../composables/useToast.js'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
const DocumentEditor = defineAsyncComponent(() => import('../components/DocumentEditor.vue'))

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

// Check if a reviewer proposed a revision (edited the doc and sent back)
const hasProposedRevision = computed(() => {
  return assignments.value.some(a => a.status === 'proposed_revision')
})

const proposedRevisionBy = computed(() => {
  const a = assignments.value.find(a => a.status === 'proposed_revision')
  return a?.reviewer?.split('@')[0] || 'A reviewer'
})

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
      message: revisionNote.value.trim() || 'Author edits',
    })
    reviewEditMode.value = false
    reviewEditContent.value = ''
    revisionNote.value = ''
    activeAction.value = null
    await loadReviewDetail(review.value.id)
  } catch (e) {
    console.error('Save & resubmit failed:', e)
    showError('Save & resubmit failed: ' + (e.message || 'unknown error'))
  } finally {
    savingReviewEdit.value = false
  }
}

async function acceptAndPublish() {
  submitting.value = true
  try {
    await api.acceptAndMerge(review.value.id)
    approvalFeedback.value = 'Revision accepted and published.'
    await loadReviewDetail(review.value.id)
  } catch (e) {
    approvalFeedback.value = 'Failed to publish: ' + (e.message || 'unknown error')
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
      message: 'Accepted proposed revision',
    })
    await loadReviewDetail(review.value.id)
  } catch (e) {
    approvalFeedback.value = 'Failed to accept revision: ' + (e.message || 'unknown error')
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
  { key: 'changes', label: `Changes (Round ${review.value?.round || 1})`, count: changedParagraphCount.value },
  { key: 'document', label: 'Document', count: 0 },
  { key: 'conversation', label: 'Conversation', count: timeline.value.filter(e => e.type === 'comment').length },
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

function statusLabel(status) {
  switch (status) {
    case 'open': return 'Open'
    case 'approved': return 'Approved'
    case 'changes_requested': return 'Changes Requested'
    case 'merged': return 'Merged'
    case 'closed': return 'Closed'
    default: return status
  }
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

function timeAgo(dateStr) {
  if (!dateStr && dateStr !== 0) return ''
  const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
  const now = new Date()
  const seconds = Math.floor((now - d) / 1000)
  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  const months = Math.floor(days / 30)
  return `${months}mo ago`
}

// Navigation
function goToList() {
  router.push(orgPath('/reviews'))
}

function openReview(id) {
  router.push(orgPath(`/reviews/${id}`))
}


const { ask } = useConfirm()

async function confirmApproveStale() {
  if (await ask('The document was modified after this review was sent. Approve based on the current version?', { confirm: 'Approve anyway', variant: 'warning' })) {
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
    if (!await ask('Discard unsaved changes?', { confirm: 'Discard', variant: 'danger' })) return
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
    showError('Failed to submit revision: ' + (e.message || 'unknown error'))
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
    showError('Failed to load reviews: ' + e.message)
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
    error.value = 'Failed to load review: ' + e.message
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
    showError('Failed to add comment: ' + (e.message || 'unknown error'))
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
    const roundLabel = result?.round > 1 ? ` for Round ${result.round}` : ''
    if (result?.pending_reviewers?.length > 0) {
      approvalFeedback.value = `Your ${decision === 'approved' ? 'approval' : 'request for changes'} has been recorded${roundLabel}. Waiting on ${result.pending_reviewers.length} more reviewer${result.pending_reviewers.length > 1 ? 's' : ''}.`
    } else if (decision === 'approved') {
      approvalFeedback.value = `All reviewers have approved${roundLabel}. Ready to merge.`
    } else {
      approvalFeedback.value = `Changes requested${roundLabel}. The author will be notified.`
    }
    setTimeout(() => { approvalFeedback.value = '' }, 8000)
    await loadReviewDetail(reviewId.value)
  } catch (e) {
    showError('Failed to submit decision: ' + (e.message || 'unknown error'))
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
    showError('Failed to merge review: ' + (e.message || 'unknown error'))
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
    showError('Failed to close review: ' + (e.message || 'unknown error'))
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
    error.value = e.message
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
