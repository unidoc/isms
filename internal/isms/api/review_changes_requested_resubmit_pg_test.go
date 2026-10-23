package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// Postgres-gated tests for #482: resubmitting a changes_requested review moves
// sent_head to the new main HEAD, which blinds the publish guard. When the
// reviewer's edits sit on review/<id> and main also changed, publishing would
// overwrite the author's edit with the branch, so the resubmit is refused.
// Requires a migrated Postgres — see id_resolution_pg_test.go.

// authorEditReview saves content on the review branch as the document's admin
// (the documented way for an author to change the text during a review).
func (f *approvedFixture) authorEditReview(content string) {
	f.t.Helper()
	id := strconv.Itoa(f.reviewID)
	payload, _ := json.Marshal(map[string]string{"content": content})
	c, rec := reviewCtxForPath(f.orgID, http.MethodPut, "/api/v1/reviews/"+id+"/content", string(payload), "admin", f.admin)
	c.SetParamNames("id")
	c.SetParamValues(id)
	if err := f.s.handleUpdateReviewContent(c); err != nil || rec.Code != http.StatusOK {
		f.t.Fatalf("author review edit: err=%v code=%d body=%s", err, rec.Code, rec.Body.String())
	}
}

func TestReviewChangesRequestedReviewerEditsAndMainEditIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(f *approvedFixture)
	}{
		{"body edit", func(f *approvedFixture) { f.putBody("author text\n") }},
		{"metadata-only edit", func(f *approvedFixture) { f.putMetadata("owner", "someone@example.test") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOpenFixture(t, "cr-refuse-"+tc.name[:4])
			f.reviewerEdit(f.rev1, "reviewer text\n")
			f.decide(f.rev1, "changes_requested")
			tc.edit(f)
			ctx := context.Background()
			before, _ := f.s.db.GetReview(ctx, f.orgID, f.reviewID)
			headBefore := f.head()

			rec, err := f.send(f.rev2)
			status, code := outcome(t, rec, err)
			if status != http.StatusConflict || code != CodeReviewHasReviewerEdits {
				t.Fatalf("expected 409 %q, got %d %q (err=%v body=%s)", CodeReviewHasReviewerEdits, status, code, err, rec.Body.String())
			}
			r, _ := f.s.db.GetReview(ctx, f.orgID, f.reviewID)
			if r.Status != "changes_requested" || r.Round != before.Round || r.SentHead != before.SentHead {
				t.Errorf("review must be untouched, got %q round %d sent_head %s (was round %d sent_head %s)", r.Status, r.Round, r.SentHead, before.Round, before.SentHead)
			}
			as, _ := f.s.db.ListAssignmentsForReview(ctx, f.orgID, f.reviewID)
			if len(as) != 1 || as[0].Reviewer != f.rev1 || as[0].Status != "changes_requested" {
				t.Errorf("assignments must be untouched, got %+v", as)
			}
			if h := f.head(); h != headBefore {
				t.Errorf("a refused send must not commit: HEAD moved %s -> %s", headBefore, h)
			}
			if tc.name == "body edit" && !strings.Contains(f.publishedFile(), "author text") {
				t.Errorf("the author's edit must still be on main, got %q", f.publishedFile())
			}
		})
	}
}

// The documented path: the author edits in the review, main is untouched, so
// the resubmit goes through and publish keeps both edits.
func TestReviewChangesRequestedAuthorEditsInReviewResubmits(t *testing.T) {
	f := newOpenFixture(t, "cr-inreview")
	f.reviewerEdit(f.rev1, "reviewer text\n")
	f.decide(f.rev1, "changes_requested")
	f.authorEditReview("reviewer text\nauthor text\n")

	rec, err := f.send(f.rev2)
	f.expectNewRound(rec, err)
	f.approve(f.rev1)
	f.approve(f.rev2)
	f.mustMerge()
	got := f.publishedFile()
	if !strings.Contains(got, "reviewer text") || !strings.Contains(got, "author text") {
		t.Errorf("published file should carry both edits, got %q", got)
	}
}

func TestReviewChangesRequestedReviewerEditOnlyResubmits(t *testing.T) {
	f := newOpenFixture(t, "cr-reviewer")
	f.reviewerEdit(f.rev1, "reviewer text\n")
	f.decide(f.rev1, "changes_requested")

	rec, err := f.send(f.rev2)
	f.expectNewRound(rec, err)
	f.approve(f.rev1)
	f.approve(f.rev2)
	f.mustMerge()
	if got := f.publishedFile(); !strings.Contains(got, "reviewer text") {
		t.Errorf("published file should carry the reviewer edit, got %q", got)
	}
}

func TestReviewChangesRequestedNoBranchMainEditResubmits(t *testing.T) {
	f := newOpenFixture(t, "cr-nobranch")
	f.decide(f.rev1, "changes_requested")
	f.putBody("author text\n")

	rec, err := f.send(f.rev2)
	f.expectNewRound(rec, err)
	f.approve(f.rev1)
	f.approve(f.rev2)
	f.mustMerge()
	if got := f.publishedFile(); !strings.Contains(got, "author text") {
		t.Errorf("published file should keep the author edit, got %q", got)
	}
}

// The document responses tell the Documents page which changes_requested
// banner to show: whether the reviewer edited the review and whether the
// document changed since it was sent.
func TestDocumentResponsesCarryChangesRequestedReviewState(t *testing.T) {
	cases := []struct {
		name           string
		setup          func(f *approvedFixture)
		changed, edits bool
	}{
		{"reviewer edit only", func(f *approvedFixture) {}, false, true},
		{"reviewer edit and main edit", func(f *approvedFixture) { f.putBody("author text\n") }, true, true},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newOpenFixture(t, "cr-dstate-"+strconv.Itoa(i))
			f.reviewerEdit(f.rev1, "reviewer text\n")
			f.decide(f.rev1, "changes_requested")
			tc.setup(f)
			f.expectDocFlags(tc.changed, tc.edits)
		})
	}
	t.Run("no review branch", func(t *testing.T) {
		f := newOpenFixture(t, "cr-dstate-nb")
		f.decide(f.rev1, "changes_requested")
		f.putBody("author text\n")
		f.expectDocFlags(true, false)
	})
}

func (f *approvedFixture) expectDocFlags(changed, edits bool) {
	f.t.Helper()
	for name, got := range f.docResponses() {
		if got["active_review_status"] != "changes_requested" {
			f.t.Fatalf("%s: active_review_status = %v", name, got["active_review_status"])
		}
		if got["active_review_changed_since_sent"] != changed {
			f.t.Errorf("%s: active_review_changed_since_sent = %v, want %v", name, got["active_review_changed_since_sent"], changed)
		}
		if got["active_review_has_reviewer_edits"] != edits {
			f.t.Errorf("%s: active_review_has_reviewer_edits = %v, want %v", name, got["active_review_has_reviewer_edits"], edits)
		}
	}
}

// GET /reviews/:id carries the same pair for the Reviews page, only for the
// statuses where a resubmit or publish is decided on it.
func (f *approvedFixture) reviewJSON() map[string]any {
	f.t.Helper()
	id := strconv.Itoa(f.reviewID)
	c, rec := reviewCtxForPath(f.orgID, http.MethodGet, "/api/v1/reviews/"+id, "", "admin", f.admin)
	c.SetParamNames("id")
	c.SetParamValues(id)
	if err := f.s.handleGetReview(c); err != nil {
		f.t.Fatalf("handleGetReview: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		f.t.Fatalf("decoding review: %v", err)
	}
	return got
}

func TestGetReviewCarriesChangesRequestedState(t *testing.T) {
	cases := []struct {
		name                 string
		branchEdit, mainEdit bool
	}{
		{"reviewer edit only", true, false},
		{"reviewer edit and main edit", true, true},
		{"no review branch, main edit", false, true},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newOpenFixture(t, "cr-getrev-"+strconv.Itoa(i))
			if tc.branchEdit {
				f.reviewerEdit(f.rev1, "reviewer text\n")
			}
			f.decide(f.rev1, "changes_requested")
			if tc.mainEdit {
				f.putBody("author text\n")
			}
			got := f.reviewJSON()
			if got["changed_since_sent"] != tc.mainEdit {
				t.Errorf("changed_since_sent = %v, want %v", got["changed_since_sent"], tc.mainEdit)
			}
			if got["has_reviewer_edits"] != tc.branchEdit {
				t.Errorf("has_reviewer_edits = %v, want %v", got["has_reviewer_edits"], tc.branchEdit)
			}
		})
	}
	t.Run("open review omits both", func(t *testing.T) {
		f := newOpenFixture(t, "cr-getrev-open")
		got := f.reviewJSON()
		for _, k := range []string{"changed_since_sent", "has_reviewer_edits"} {
			if _, ok := got[k]; ok {
				t.Errorf("%s must be absent for an open review", k)
			}
		}
	})
}
