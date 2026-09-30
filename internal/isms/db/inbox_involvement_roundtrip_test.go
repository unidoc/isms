package db

import (
	"context"
	"fmt"
	"sort"
	"testing"
)

// Requires a migrated Postgres — see notifications_roundtrip_test.go's header
// for how to stand one up. Skipped when ISMS_TEST_DATABASE_URL is unset.
//
// Regression test for #205: the Inbox listed the whole organization's comments,
// reviews, tasks, changes and suggestions. The scoped queries decide "mine" in
// SQL, so the web page, the CLI and the API agree and nothing is cut off by a
// page limit before a client-side filter runs.

type inboxFixture struct {
	d   *DB
	org int

	admin, manager, manager2, contributor, reader string

	t1, t2, t3, t6 int64 // tasks
	r1, r2         int   // reviews
	k1             int   // comment
	s1, s2, s3     int64 // suggestions
	ch1            int   // change request
}

// newInboxFixture seeds one org with five users and one row of each kind:
//
//	admin A        created tasks T1 (→C, open), T2 (→C, done today), T3 (→C, done
//	               20 days ago) and T6 (→M, private, open); requested change CH1
//	manager M      requested reviews R1 (reviewer C, pending) and R2 (no reviewer)
//	contributor C  wrote comment K1 on R1, suggested S1 (open), S2 (open, later
//	               claimed by M) and S3 (applied); CH1 is assigned to C (proposed)
//	reader Z       is involved in nothing
func newInboxFixture(t *testing.T, name string) *inboxFixture {
	t.Helper()
	d := testDB(t)
	ctx := context.Background()
	f := &inboxFixture{d: d}

	// Registered first so it runs last: the org (and with it every row that
	// references these users) is deleted by newTestOrgUser's own cleanup first.
	suffix := "@inbox-involvement.test"
	mk := func(role string) string {
		return fmt.Sprintf("%s-%s%s", name, role, suffix)
	}
	f.manager, f.manager2, f.contributor, f.reader = mk("manager"), mk("manager2"), mk("contributor"), mk("reader")
	t.Cleanup(func() {
		for _, e := range []string{f.manager, f.manager2, f.contributor, f.reader} {
			_, _ = d.pool.Exec(ctx, `DELETE FROM users WHERE email = $1`, e)
		}
	})
	for _, e := range []string{f.manager, f.manager2, f.contributor, f.reader} {
		if _, err := d.pool.Exec(ctx, `INSERT INTO users (email, name) VALUES ($1, $1)`, e); err != nil {
			t.Fatalf("creating user %s: %v", e, err)
		}
	}
	f.org, f.admin = newTestOrgUser(t, d, name)

	mustTask := func(title, assignee string, private bool) int64 {
		task := &Task{Title: title, TaskType: "general", Assignee: assignee, CreatedBy: f.admin,
			Status: "open", Priority: "medium", Private: private}
		if err := d.CreateTask(ctx, f.org, task); err != nil {
			t.Fatalf("CreateTask %s: %v", title, err)
		}
		return task.ID
	}
	f.t1 = mustTask("T1 open, delegated", f.contributor, false)
	f.t2 = mustTask("T2 done today", f.contributor, false)
	f.t3 = mustTask("T3 done 20 days ago", f.contributor, false)
	f.t6 = mustTask("T6 private", f.manager, true)
	if err := d.UpdateTaskStatus(ctx, f.org, f.t2, "done"); err != nil {
		t.Fatalf("done T2: %v", err)
	}
	if err := d.UpdateTaskStatus(ctx, f.org, f.t3, "done"); err != nil {
		t.Fatalf("done T3: %v", err)
	}
	if _, err := d.pool.Exec(ctx, `UPDATE tasks SET completed_at = now() - interval '20 days' WHERE id = $1`, f.t3); err != nil {
		t.Fatalf("backdating T3: %v", err)
	}

	mustReview := func(doc string) int {
		r := &Review{DocumentID: doc, DocumentType: "policy", Title: doc, Version: "1",
			RequestedBy: f.manager, Status: "open"}
		if err := d.CreateReview(ctx, f.org, r); err != nil {
			t.Fatalf("CreateReview %s: %v", doc, err)
		}
		return r.ID
	}
	f.r1, f.r2 = mustReview("inv-r1"), mustReview("inv-r2")
	if err := d.AddReviewAssignment(ctx, f.org, &ReviewAssignment{ReviewID: f.r1, Reviewer: f.contributor, Status: "pending"}); err != nil {
		t.Fatalf("assigning R1: %v", err)
	}

	k1 := &Comment{ReviewID: &f.r1, DocumentID: "inv-r1", Author: f.contributor, Body: "K1"}
	if err := d.AddComment(ctx, f.org, k1); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	f.k1 = k1.ID

	mustSuggestion := func(title string) int64 {
		s := &Suggestion{EntityType: "risk", SuggestionType: "create", Title: title, SuggestedBy: f.contributor}
		if err := d.CreateSuggestion(ctx, f.org, s); err != nil {
			t.Fatalf("CreateSuggestion %s: %v", title, err)
		}
		return s.ID
	}
	f.s1, f.s2, f.s3 = mustSuggestion("S1"), mustSuggestion("S2"), mustSuggestion("S3")
	if err := d.ApplySuggestion(ctx, f.org, f.s3, f.manager, ""); err != nil {
		t.Fatalf("applying S3: %v", err)
	}

	ch := &ChangeRequest{Title: "CH1", Description: "d", Priority: "medium", Category: "process",
		RiskLevel: "low", Status: "proposed", RequestedBy: f.admin, AssignedTo: f.contributor}
	if err := d.CreateChangeRequest(ctx, f.org, ch); err != nil {
		t.Fatalf("CreateChangeRequest: %v", err)
	}
	f.ch1 = ch.ID
	return f
}

func (f *inboxFixture) reviews(t *testing.T, email string) map[int]Review {
	t.Helper()
	items, _, err := f.d.PaginatedReviews(context.Background(), f.org, ReviewListParams{Involving: email, Limit: 200})
	if err != nil {
		t.Fatalf("PaginatedReviews(%s): %v", email, err)
	}
	out := map[int]Review{}
	for _, r := range items {
		out[r.ID] = r
	}
	return out
}

func (f *inboxFixture) tasks(t *testing.T, viewer TaskViewer, involving string) map[int64]Task {
	t.Helper()
	items, _, err := f.d.PaginatedTasks(context.Background(), f.org, viewer, TaskListParams{Involving: involving, Limit: 200})
	if err != nil {
		t.Fatalf("PaginatedTasks(%s): %v", involving, err)
	}
	out := map[int64]Task{}
	for _, task := range items {
		out[task.ID] = task
	}
	return out
}

func (f *inboxFixture) changes(t *testing.T, email string, canApprove bool) map[int]ChangeRequest {
	t.Helper()
	items, _, err := f.d.PaginatedChangeRequests(context.Background(), f.org,
		ChangeRequestListParams{Involving: email, InvolvingCanApprove: canApprove, Limit: 200})
	if err != nil {
		t.Fatalf("PaginatedChangeRequests(%s): %v", email, err)
	}
	out := map[int]ChangeRequest{}
	for _, c := range items {
		out[c.ID] = c
	}
	return out
}

func (f *inboxFixture) suggestions(t *testing.T, email string, canReview bool, status string) map[int64]Suggestion {
	t.Helper()
	items, err := f.d.ListSuggestions(context.Background(), f.org,
		SuggestionFilters{Status: status, Involving: email, InvolvingCanReview: canReview})
	if err != nil {
		t.Fatalf("ListSuggestions(%s): %v", email, err)
	}
	out := map[int64]Suggestion{}
	for _, s := range items {
		out[s.ID] = s
	}
	return out
}

func (f *inboxFixture) comments(t *testing.T, email string) map[int]Comment {
	t.Helper()
	items, err := f.d.OpenCommentsInvolving(context.Background(), f.org, email)
	if err != nil {
		t.Fatalf("OpenCommentsInvolving(%s): %v", email, err)
	}
	out := map[int]Comment{}
	for _, c := range items {
		out[c.ID] = c
	}
	return out
}

func keys[K int | int64, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func TestInboxScopedQueriesRoundTrip(t *testing.T) {
	f := newInboxFixture(t, "inbox-involvement")
	ctx := context.Background()

	// 1. The headline case: a user with no involvement gets nothing back from any
	// scoped query, however much the rest of the org has going on.
	t.Run("uninvolved reader sees nothing", func(t *testing.T) {
		if got := f.comments(t, f.reader); len(got) != 0 {
			t.Errorf("comments: %v, want none", keys(got))
		}
		if got := f.reviews(t, f.reader); len(got) != 0 {
			t.Errorf("reviews: %v, want none", keys(got))
		}
		if got := f.tasks(t, TaskViewer{Email: f.reader}, f.reader); len(got) != 0 {
			t.Errorf("tasks: %v, want none", keys(got))
		}
		if got := f.changes(t, f.reader, false); len(got) != 0 {
			t.Errorf("changes: %v, want none", keys(got))
		}
		if got := f.suggestions(t, f.reader, false, "active"); len(got) != 0 {
			t.Errorf("suggestions: %v, want none", keys(got))
		}
	})

	// 2. The contributor: a pending assignment, an assigned task, a change
	// assigned to them and their own suggestion.
	t.Run("contributor", func(t *testing.T) {
		c := f.contributor
		reviews := f.reviews(t, c)
		if len(reviews) != 1 {
			t.Fatalf("reviews = %v, want only R1", keys(reviews))
		}
		r1, ok := reviews[f.r1]
		if !ok {
			t.Fatalf("R1 missing from %v", keys(reviews))
		}
		if r1.InboxGroup != "to_review" || !r1.NeedsAction {
			t.Errorf("R1 group=%q needs_action=%v, want to_review/true", r1.InboxGroup, r1.NeedsAction)
		}
		if len(r1.Reviewers) != 1 || r1.Reviewers[0] != c {
			t.Errorf("R1 reviewers = %v, want [%s]", r1.Reviewers, c)
		}

		// T2 is done (the contributor already finished it), T3 is done, T6 is not theirs.
		tasks := f.tasks(t, TaskViewer{Email: c}, c)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %v, want only T1", keys(tasks))
		}
		if t1, ok := tasks[f.t1]; !ok || t1.InboxGroup != "assigned" || !t1.NeedsAction {
			t.Errorf("T1 = %+v (present %v), want assigned/needs_action", t1, ok)
		}

		changes := f.changes(t, c, false)
		if ch, ok := changes[f.ch1]; len(changes) != 1 || !ok || ch.NeedsAction {
			t.Errorf("changes = %v (CH1 %+v), want only CH1 with needs_action=false: proposed changes wait on a manager", keys(changes), ch)
		}

		sugg := f.suggestions(t, c, false, "active")
		if len(sugg) != 2 {
			t.Fatalf("active suggestions = %v, want S1 and S2 only (S3 is applied)", keys(sugg))
		}
		if sugg[f.s1].NeedsAction {
			t.Error("S1 needs_action = true for its author, want false: it waits on a manager")
		}

		if k, ok := f.comments(t, c)[f.k1]; !ok || k.NeedsAction {
			t.Errorf("K1 = %+v (present %v), want present with needs_action=false (newest message is theirs)", k, ok)
		}

		applied := f.suggestions(t, c, false, "applied")
		if len(applied) != 1 || applied[f.s3].NeedsAction {
			t.Errorf("applied suggestions = %v, want S3 and needs_action=false", keys(applied))
		}
	})

	// 3. The admin delegated three tasks to the contributor. Delegated rows are
	// listed but never counted: the creator is told by a notification when the
	// assignee finishes one, so a done task is not an action item (#205).
	t.Run("task creator", func(t *testing.T) {
		tasks := f.tasks(t, TaskViewer{Email: f.admin, CanSeeAll: true}, f.admin)
		if _, ok := tasks[f.t3]; ok {
			t.Error("T3 (done 20 days ago) is returned, want it aged out of the 14 day window")
		}
		for _, tc := range []struct {
			name   string
			id     int64
			action bool
		}{
			{"T1 still open", f.t1, false},
			{"T2 done today", f.t2, false},
			{"T6 private, open", f.t6, false},
		} {
			got, ok := tasks[tc.id]
			if !ok {
				t.Errorf("%s: missing", tc.name)
				continue
			}
			if got.InboxGroup != "delegated" || got.NeedsAction != tc.action {
				t.Errorf("%s: group=%q needs_action=%v, want delegated/%v", tc.name, got.InboxGroup, got.NeedsAction, tc.action)
			}
		}
	})

	// 4. The manager requested both reviews, and sees what others wait on them for.
	t.Run("manager", func(t *testing.T) {
		m := f.manager
		reviews := f.reviews(t, m)
		r1, ok := reviews[f.r1]
		if !ok || r1.InboxGroup != "sent" || r1.NeedsAction {
			t.Errorf("R1 = %+v (present %v), want sent and not counted: open, waiting on reviewers", r1, ok)
		}

		if k, ok := f.comments(t, m)[f.k1]; !ok || !k.NeedsAction {
			t.Errorf("K1 = %+v (present %v), want present with needs_action=true: a comment on a review M requested", k, ok)
		}

		sugg := f.suggestions(t, m, true, "active")
		if s1, ok := sugg[f.s1]; !ok || !s1.NeedsAction {
			t.Errorf("S1 = %+v (present %v), want needs_action=true for a manager", s1, ok)
		}

		// M is neither requester nor assignee of CH1, so only the manager branch returns it.
		ch, ok := f.changes(t, m, true)[f.ch1]
		if !ok || !ch.NeedsAction {
			t.Errorf("CH1 = %+v (present %v), want returned with needs_action=true: proposed, M can approve", ch, ok)
		}
		if _, ok := f.changes(t, m, false)[f.ch1]; ok {
			t.Error("CH1 returned to M without the approver flag, want it gone: M is neither requester nor assignee")
		}

		// A terminal tab shows the manager's own suggestions only, and M made none.
		if got := f.suggestions(t, m, true, "applied"); len(got) != 0 {
			t.Errorf("applied suggestions for M = %v, want none: S3 is the contributor's", keys(got))
		}
	})

	// 4b. Once R2 is approved, merging it is the requester's job.
	t.Run("approved review waits on its requester", func(t *testing.T) {
		if err := f.d.AddReviewAssignment(ctx, f.org, &ReviewAssignment{ReviewID: f.r2, Reviewer: f.manager2, Status: "approved"}); err != nil {
			t.Fatalf("assigning R2: %v", err)
		}
		if _, err := f.d.pool.Exec(ctx, `UPDATE reviews SET status = 'approved' WHERE id = $1`, f.r2); err != nil {
			t.Fatalf("approving R2: %v", err)
		}
		r2, ok := f.reviews(t, f.manager)[f.r2]
		if !ok || r2.InboxGroup != "sent" || !r2.NeedsAction {
			t.Errorf("R2 = %+v (present %v), want sent with needs_action=true", r2, ok)
		}
		if _, ok := f.reviews(t, f.manager2)[f.r2]; ok {
			t.Error("R2 is in the reviewer's inbox after they approved it, want it gone")
		}
	})

	// 5. A suggestion claimed by one manager is not another manager's work.
	t.Run("claimed suggestion", func(t *testing.T) {
		if _, err := f.d.ClaimSuggestion(ctx, f.org, f.s2, f.manager); err != nil {
			t.Fatalf("claiming S2: %v", err)
		}
		if s2, ok := f.suggestions(t, f.manager, true, "active")[f.s2]; !ok || !s2.NeedsAction || s2.Status != "in_review" {
			t.Errorf("S2 for the claimer = %+v (present %v), want in_review with needs_action=true", s2, ok)
		}
		if _, ok := f.suggestions(t, f.manager2, true, "active")[f.s2]; ok {
			t.Error("S2 is in a second manager's inbox, want it left out: another manager claimed it")
		}
		if _, ok := f.suggestions(t, f.manager2, true, "active")[f.s1]; !ok {
			t.Error("open S1 is missing from the second manager's inbox")
		}
		// The author still sees it, now in review, without being asked to act.
		if s2, ok := f.suggestions(t, f.contributor, false, "active")[f.s2]; !ok || s2.NeedsAction {
			t.Errorf("S2 for its author = %+v (present %v), want present, needs_action=false", s2, ok)
		}
	})

	// 6. The privacy rule is ANDed with the inbox scope, never replaced by it.
	// With the viewer and the involving email equal, the scope already implies
	// visibility, so the check that matters is a viewer who is not allowed to see
	// a task that the scope alone would return: the admin's delegated private task.
	t.Run("private tasks stay hidden", func(t *testing.T) {
		if _, ok := f.tasks(t, TaskViewer{Email: f.contributor}, f.contributor)[f.t6]; ok {
			t.Error("T6 (private, assigned to M) is in the contributor's inbox")
		}
		if _, ok := f.tasks(t, TaskViewer{Email: f.contributor}, f.admin)[f.t6]; ok {
			t.Error("T6 is returned to a viewer who may not see it: the involving scope replaced the privacy rule")
		}
		if _, ok := f.tasks(t, TaskViewer{Email: f.admin, CanSeeAll: true}, f.admin)[f.t6]; !ok {
			t.Error("T6 is missing from its creator's inbox")
		}
	})
}

// Without Involving, nothing about the computed inbox columns leaks into the
// other callers of these queries.
func TestUnscopedListsLeaveInboxColumnsEmpty(t *testing.T) {
	f := newInboxFixture(t, "inbox-unscoped")
	ctx := context.Background()

	reviews, total, err := f.d.PaginatedReviews(ctx, f.org, ReviewListParams{Limit: 200})
	if err != nil || total != 2 {
		t.Fatalf("PaginatedReviews: total=%d err=%v, want 2 reviews", total, err)
	}
	for _, r := range reviews {
		if r.InboxGroup != "" || r.NeedsAction {
			t.Errorf("review %d: group=%q needs_action=%v, want empty without Involving", r.ID, r.InboxGroup, r.NeedsAction)
		}
	}

	tasks, total, err := f.d.PaginatedTasks(ctx, f.org, TaskViewer{CanSeeAll: true}, TaskListParams{Limit: 200})
	if err != nil || total != 4 {
		t.Fatalf("PaginatedTasks: total=%d err=%v, want 4 tasks", total, err)
	}
	for _, task := range tasks {
		if task.InboxGroup != "" || task.NeedsAction {
			t.Errorf("task %d: group=%q needs_action=%v, want empty without Involving", task.ID, task.InboxGroup, task.NeedsAction)
		}
	}

	changes, total, err := f.d.PaginatedChangeRequests(ctx, f.org, ChangeRequestListParams{Limit: 200})
	if err != nil || total != 1 || changes[0].NeedsAction {
		t.Fatalf("PaginatedChangeRequests: total=%d err=%v changes=%+v, want CH1 unflagged", total, err, changes)
	}

	sugg, err := f.d.ListSuggestions(ctx, f.org, SuggestionFilters{})
	if err != nil || len(sugg) != 3 {
		t.Fatalf("ListSuggestions: %d rows err=%v, want 3", len(sugg), err)
	}
	for _, s := range sugg {
		if s.NeedsAction {
			t.Errorf("suggestion %d: needs_action=true without Involving", s.ID)
		}
	}

	// Status filters still apply when the inbox scope is off, and "active" means open + in_review.
	active, err := f.d.ListSuggestions(ctx, f.org, SuggestionFilters{Status: "active"})
	if err != nil || len(active) != 2 {
		t.Errorf("active suggestions = %d err=%v, want 2 (S3 is applied)", len(active), err)
	}
}

// Comments on a review that is merged or closed have nothing left to act on,
// so they leave every involved user's inbox, whichever branch matched them.
// Document-level comments (no review) are unaffected.
func TestInboxCommentsDropWhenTheReviewEnds(t *testing.T) {
	f := newInboxFixture(t, "inbox-ended-review")
	ctx := context.Background()

	// K1 is C's comment on R1 (requested by M, reviewer C). Add one on R2 and a
	// document-level comment, both by C.
	k2 := &Comment{ReviewID: &f.r2, DocumentID: "inv-r2", Author: f.contributor, Body: "K2"}
	if err := f.d.AddComment(ctx, f.org, k2); err != nil {
		t.Fatalf("AddComment K2: %v", err)
	}
	doc := &Comment{DocumentID: "inv-doc-level", Author: f.contributor, Body: "doc level"}
	if err := f.d.AddComment(ctx, f.org, doc); err != nil {
		t.Fatalf("AddComment doc level: %v", err)
	}

	for who, email := range map[string]string{"requester": f.manager, "commenter": f.contributor} {
		got := f.comments(t, email)
		if _, ok := got[f.k1]; !ok {
			t.Errorf("%s: K1 missing while R1 is open", who)
		}
	}

	setStatus := func(reviewID int, status string) {
		t.Helper()
		if _, err := f.d.pool.Exec(ctx, `UPDATE reviews SET status = $2 WHERE id = $1`, reviewID, status); err != nil {
			t.Fatalf("setting review %d to %s: %v", reviewID, status, err)
		}
	}
	setStatus(f.r1, "closed")
	setStatus(f.r2, "merged")

	for who, email := range map[string]string{"requester": f.manager, "commenter": f.contributor} {
		got := f.comments(t, email)
		if _, ok := got[f.k1]; ok {
			t.Errorf("%s: K1 still listed after R1 was closed", who)
		}
		if _, ok := got[int(k2.ID)]; ok {
			t.Errorf("%s: K2 still listed after R2 was merged", who)
		}
	}

	// Its author still has the document-level comment; nobody else matches it.
	if _, ok := f.comments(t, f.contributor)[doc.ID]; !ok {
		t.Error("author lost their document-level comment")
	}
	if _, ok := f.comments(t, f.manager)[doc.ID]; ok {
		t.Error("requester sees someone else's document-level comment")
	}
}
