package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Postgres-gated regression tests for #301: POST /reviews/:id/approve silently
// no-op'd on a review with zero assignments — it wrote an approval row, returned
// 200, and left the review status "open" forever, because the status
// transition is derived entirely from assignment rows. Requires a migrated
// Postgres — see id_resolution_pg_test.go's testServer for setup. Skipped when
// ISMS_TEST_DATABASE_URL is unset.
//
// Reuses the #299 harness from review_create_pg_test.go: testServer, newTestOrg,
// seedRepoForOrg, seedReviewUser, reviewCtxForPath.

func TestApproveRejectsReviewWithNoAssignments(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "approve-noassign")
	seedRepoForOrg(t, s, orgID, "test-doc-noassign")
	seedReviewUser(t, s, orgID, "author@approve-noassign.test", "contributor")
	seedReviewUser(t, s, orgID, "mgr@approve-noassign.test", "manager")

	ctx := context.Background()

	// #299 (POST /reviews) and the equivalent guard on handleReviewSend both
	// reject an empty reviewers list, so the only way left to reach this
	// state is to create the review directly at the db layer.
	r := &db.Review{
		DocumentID:  "test-doc-noassign",
		Title:       "T",
		Version:     "1.0",
		Status:      "open",
		RequestedBy: "author@approve-noassign.test",
	}
	if err := s.db.CreateReview(ctx, orgID, r); err != nil {
		t.Fatalf("CreateReview: %v", err)
	}

	body := `{"decision":"approved","comment":"lgtm"}`
	c, _ := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews/"+strconv.Itoa(r.ID)+"/approve", body, "manager", "mgr@approve-noassign.test")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(r.ID))

	err := s.handleReviewApprove(c)
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
	}
	if he.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %v", he.Code, he.Message)
	}

	approvals, aerr := s.db.ApprovalsForReview(ctx, orgID, r.ID)
	if aerr != nil {
		t.Fatalf("ApprovalsForReview: %v", aerr)
	}
	if len(approvals) != 0 {
		t.Fatalf("expected no dangling approval rows, got %d", len(approvals))
	}

	got, gerr := s.db.GetReview(ctx, orgID, r.ID)
	if gerr != nil {
		t.Fatalf("GetReview: %v", gerr)
	}
	if got.Status != "open" {
		t.Fatalf("expected review status to remain open, got %q", got.Status)
	}
}

func TestRequestChangesRejectsReviewWithNoAssignments(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "changes-noassign")
	seedRepoForOrg(t, s, orgID, "test-doc-noassign2")
	seedReviewUser(t, s, orgID, "author@changes-noassign.test", "contributor")
	seedReviewUser(t, s, orgID, "mgr@changes-noassign.test", "manager")

	ctx := context.Background()

	r := &db.Review{
		DocumentID:  "test-doc-noassign2",
		Title:       "T",
		Version:     "1.0",
		Status:      "open",
		RequestedBy: "author@changes-noassign.test",
	}
	if err := s.db.CreateReview(ctx, orgID, r); err != nil {
		t.Fatalf("CreateReview: %v", err)
	}

	body := `{"decision":"changes_requested","comment":"needs work"}`
	c, _ := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews/"+strconv.Itoa(r.ID)+"/approve", body, "manager", "mgr@changes-noassign.test")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(r.ID))

	err := s.handleReviewApprove(c)
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
	}
	if he.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %v", he.Code, he.Message)
	}

	got, gerr := s.db.GetReview(ctx, orgID, r.ID)
	if gerr != nil {
		t.Fatalf("GetReview: %v", gerr)
	}
	if got.Status != "open" {
		t.Fatalf("expected review status to remain open, got %q", got.Status)
	}
}

// TestApproveByUnassignedManagerWithAssignmentsIsRecorded guards an
// intentional behaviour: an unassigned admin/manager approving a review that
// DOES have assignments is NOT an error. Their approval row still counts
// toward the merge approval policy (db/approval_policies.go's approvedBy
// set, built from approval records, not assignment records) even though the
// review itself does not advance to "approved" until the assigned reviewer
// also approves. This is deliberate — do not "fix" it into a 403/409.
func TestApproveByUnassignedManagerWithAssignmentsIsRecorded(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "approve-bypass")
	seedRepoForOrg(t, s, orgID, "test-doc-bypass")
	seedReviewUser(t, s, orgID, "author-mgr@approve-bypass.test", "manager")
	seedReviewUser(t, s, orgID, "reviewer@approve-bypass.test", "contributor")
	seedReviewUser(t, s, orgID, "mgr2@approve-bypass.test", "manager")

	createBody := `{"document_id":"test-doc-bypass","reviewers":["reviewer@approve-bypass.test"]}`
	cCreate, recCreate := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews", createBody, "manager", "author-mgr@approve-bypass.test")
	if err := s.handleCreateReview(cCreate); err != nil {
		t.Fatalf("handleCreateReview: %v", err)
	}
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", recCreate.Code, recCreate.Body.String())
	}
	var review db.Review
	if err := json.Unmarshal(recCreate.Body.Bytes(), &review); err != nil {
		t.Fatalf("decoding create response: %v", err)
	}

	// Approve as a second manager, who is not assigned as a reviewer.
	approveBody := `{"decision":"approved","comment":"lgtm from unassigned manager"}`
	cApprove, recApprove := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews/"+strconv.Itoa(review.ID)+"/approve", approveBody, "manager", "mgr2@approve-bypass.test")
	cApprove.SetParamNames("id")
	cApprove.SetParamValues(strconv.Itoa(review.ID))

	if err := s.handleReviewApprove(cApprove); err != nil {
		t.Fatalf("handleReviewApprove: %v", err)
	}
	if recApprove.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recApprove.Code, recApprove.Body.String())
	}

	ctx := context.Background()
	approvals, aerr := s.db.ApprovalsForReview(ctx, orgID, review.ID)
	if aerr != nil {
		t.Fatalf("ApprovalsForReview: %v", aerr)
	}
	if len(approvals) != 1 {
		t.Fatalf("expected exactly 1 approval row, got %d", len(approvals))
	}
	if approvals[0].ApprovedBy != "mgr2@approve-bypass.test" {
		t.Errorf("expected approval by mgr2@approve-bypass.test, got %s", approvals[0].ApprovedBy)
	}

	var resp struct {
		ReviewStatus     string   `json:"review_status"`
		PendingReviewers []string `json:"pending_reviewers"`
	}
	if err := json.Unmarshal(recApprove.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding approve response: %v", err)
	}
	if resp.ReviewStatus != "open" {
		t.Errorf("expected review to remain open (unassigned manager approval does not by itself advance it), got %q", resp.ReviewStatus)
	}
	found := false
	for _, p := range resp.PendingReviewers {
		if p == "reviewer@approve-bypass.test" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected pending_reviewers to still include the assigned reviewer, got %v", resp.PendingReviewers)
	}

	got, gerr := s.db.GetReview(ctx, orgID, review.ID)
	if gerr != nil {
		t.Fatalf("GetReview: %v", gerr)
	}
	if got.Status != "open" {
		t.Fatalf("expected review status open, got %q", got.Status)
	}
}

// TestZeroAssignmentReviewRecoversAfterReviewerAdded closes the loop end to
// end: a review stuck in the zero-assignment state (409 on every decision)
// is not a dead end. The documented recovery path — POST /reviews/:id/forward
// (handleForwardReview) — assigns a reviewer to the existing review, and a
// decision from that reviewer then advances it normally.
func TestZeroAssignmentReviewRecoversAfterReviewerAdded(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "recover-noassign")
	seedRepoForOrg(t, s, orgID, "test-doc-recover")
	seedReviewUser(t, s, orgID, "author@recover-noassign.test", "contributor")
	seedReviewUser(t, s, orgID, "mgr@recover-noassign.test", "manager")
	seedReviewUser(t, s, orgID, "reviewer@recover-noassign.test", "contributor")

	ctx := context.Background()

	r := &db.Review{
		DocumentID:  "test-doc-recover",
		Title:       "T",
		Version:     "1.0",
		Status:      "open",
		RequestedBy: "author@recover-noassign.test",
	}
	if err := s.db.CreateReview(ctx, orgID, r); err != nil {
		t.Fatalf("CreateReview: %v", err)
	}

	// 1. Confirm the review is stuck: a decision on it 409s.
	stuckBody := `{"decision":"approved","comment":"lgtm"}`
	cStuck, _ := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews/"+strconv.Itoa(r.ID)+"/approve", stuckBody, "manager", "mgr@recover-noassign.test")
	cStuck.SetParamNames("id")
	cStuck.SetParamValues(strconv.Itoa(r.ID))
	err := s.handleReviewApprove(cStuck)
	if err == nil {
		t.Fatalf("expected the pre-recovery decision to error, got nil")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusConflict {
		t.Fatalf("expected 409 echo.HTTPError before recovery, got %v", err)
	}

	// 2. Recover via the real dedicated endpoint: POST /reviews/:id/forward.
	forwardBody := `{"reviewers":["reviewer@recover-noassign.test"],"message":"adding you as reviewer"}`
	cForward, recForward := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews/"+strconv.Itoa(r.ID)+"/forward", forwardBody, "manager", "mgr@recover-noassign.test")
	cForward.SetParamNames("id")
	cForward.SetParamValues(strconv.Itoa(r.ID))
	if err := s.handleForwardReview(cForward); err != nil {
		t.Fatalf("handleForwardReview: %v", err)
	}
	if recForward.Code != http.StatusOK {
		t.Fatalf("expected 200 from forward, got %d: %s", recForward.Code, recForward.Body.String())
	}

	assignments, aerr := s.db.ListAssignmentsForReview(ctx, orgID, r.ID)
	if aerr != nil {
		t.Fatalf("ListAssignmentsForReview: %v", aerr)
	}
	if len(assignments) != 1 || assignments[0].Reviewer != "reviewer@recover-noassign.test" {
		t.Fatalf("expected exactly 1 assignment for reviewer@recover-noassign.test, got %+v", assignments)
	}

	// 3. Approve as the newly-added reviewer.
	approveBody := `{"decision":"approved","comment":"lgtm now that I am assigned"}`
	cApprove, recApprove := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews/"+strconv.Itoa(r.ID)+"/approve", approveBody, "contributor", "reviewer@recover-noassign.test")
	cApprove.SetParamNames("id")
	cApprove.SetParamValues(strconv.Itoa(r.ID))
	if err := s.handleReviewApprove(cApprove); err != nil {
		t.Fatalf("handleReviewApprove after recovery: %v", err)
	}
	if recApprove.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recApprove.Code, recApprove.Body.String())
	}

	// 4. Assert the review reached "approved" and an approval row exists.
	got, gerr := s.db.GetReview(ctx, orgID, r.ID)
	if gerr != nil {
		t.Fatalf("GetReview: %v", gerr)
	}
	if got.Status != "approved" {
		t.Fatalf("expected review status approved after recovery, got %q", got.Status)
	}

	approvals, aperr := s.db.ApprovalsForReview(ctx, orgID, r.ID)
	if aperr != nil {
		t.Fatalf("ApprovalsForReview: %v", aperr)
	}
	if len(approvals) != 1 {
		t.Fatalf("expected exactly 1 approval row, got %d", len(approvals))
	}
	if approvals[0].ApprovedBy != "reviewer@recover-noassign.test" {
		t.Errorf("expected approval by reviewer@recover-noassign.test, got %s", approvals[0].ApprovedBy)
	}
}

// TestReviewSendRejectsEmptyReviewers is modeled on
// TestCreateReviewRejectsEmptyReviewers (review_create_pg_test.go), scoped to
// handleReviewSend (POST /documents/:docId/reviews) instead of
// handleCreateReview (POST /reviews). This is the entry point that #301's fix
// closed at api_collab.go — a fresh review sent with no reviewers used to
// reach the zero-assignment state that made handleReviewApprove get stuck
// (see the other tests in this file). Both shapes of an empty reviewers list
// must be rejected: the field absent entirely, and an explicit empty array.
func TestReviewSendRejectsEmptyReviewers(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "field absent", body: `{}`},
		{name: "empty array", body: `{"reviewers":[]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer(t)
			orgName := "reviewsend-empty-" + strings.ReplaceAll(tc.name, " ", "-")
			orgID := newTestOrg(t, s, orgName)
			seedRepoForOrg(t, s, orgID, "test-doc-send-empty")
			seedReviewUser(t, s, orgID, "admin@"+orgName+".test", "admin")

			c, rec := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/documents/test-doc-send-empty/reviews", tc.body, "admin", "admin@"+orgName+".test")
			c.SetParamNames("docId")
			c.SetParamValues("test-doc-send-empty")

			err := s.handleReviewSend(c)
			if err == nil {
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
				}
			} else {
				he, ok := err.(*echo.HTTPError)
				if !ok || he.Code != http.StatusBadRequest {
					t.Fatalf("expected 400 echo.HTTPError, got %v", err)
				}
			}

			existing, lerr := s.db.GetOpenReviewForDocument(context.Background(), orgID, "test-doc-send-empty")
			if lerr != nil {
				t.Fatalf("GetOpenReviewForDocument: %v", lerr)
			}
			if existing != nil {
				t.Fatalf("expected no review created, found review %d", existing.ID)
			}
		})
	}
}
