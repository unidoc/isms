package api

import (
	"context"
	"net/http"
	"testing"
)

// Postgres-gated tests for #484: a send to an already-open review that names
// no reviewers added nobody, yet answered 200, logged an empty "Added
// reviewers" activity row and fanned it out to Slack/Matrix. Resubmits keep
// accepting an empty list because they reuse the existing assignments.

func (f *approvedFixture) reviewersAdded() int {
	f.t.Helper()
	acts, err := f.s.db.ActivityForReview(context.Background(), f.orgID, f.reviewID, 100)
	if err != nil {
		f.t.Fatalf("ActivityForReview: %v", err)
	}
	n := 0
	for _, a := range acts {
		if a.Action == "reviewers_added" {
			n++
		}
	}
	return n
}

func TestReviewSendOpenReviewWithNoReviewersIsRefused(t *testing.T) {
	f := newOpenFixture(t, "open-noreviewers")

	rec, err := f.send()
	if status, _ := outcome(t, rec, err); status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (err=%v body=%s)", status, err, rec.Body.String())
	}
	as, _ := f.s.db.ListAssignmentsForReview(context.Background(), f.orgID, f.reviewID)
	if len(as) != 1 || as[0].Reviewer != f.rev1 {
		t.Errorf("assignments changed: %+v", as)
	}
	if n := f.reviewersAdded(); n != 0 {
		t.Errorf("%d reviewers_added activity rows, want 0", n)
	}
}

// Control: naming a new reviewer on an open review still works.
func TestReviewSendOpenReviewAddsNamedReviewer(t *testing.T) {
	f := newOpenFixture(t, "open-addreviewer")

	rec, err := f.send(f.rev2)
	if status, _ := outcome(t, rec, err); status != http.StatusOK {
		t.Fatalf("expected 200, got %d (err=%v body=%s)", status, err, rec.Body.String())
	}
	as, _ := f.s.db.ListAssignmentsForReview(context.Background(), f.orgID, f.reviewID)
	found := false
	for _, a := range as {
		found = found || a.Reviewer == f.rev2
	}
	if !found {
		t.Errorf("rev2 not assigned: %+v", as)
	}
	if n := f.reviewersAdded(); n != 1 {
		t.Errorf("%d reviewers_added activity rows, want 1", n)
	}
}

// Control: a resubmit after changes were requested may omit reviewers.
func TestReviewSendResubmitWithNoReviewersStillAllowed(t *testing.T) {
	f := newOpenFixture(t, "resubmit-noreviewers")
	f.decide(f.rev1, "changes_requested")

	rec, err := f.send()
	if status, _ := outcome(t, rec, err); status != http.StatusOK {
		t.Fatalf("expected 200, got %d (err=%v body=%s)", status, err, rec.Body.String())
	}
	r, _ := f.s.db.GetReview(context.Background(), f.orgID, f.reviewID)
	if r.Status != "open" || r.Round != 2 {
		t.Errorf("expected open round 2, got %q round %d", r.Status, r.Round)
	}
}
