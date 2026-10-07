package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"isms.sh/internal/isms/db"
)

// Requires a migrated Postgres; skipped otherwise, same as change_approval_pg_test.go:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...
//
// Regression for #423: a change request could move to any status regardless
// of its current one — "proposed" straight to "implemented" with nobody ever
// approving it. Both status-change entry points (the dedicated status
// endpoint and the edit form's PUT) and the suggestion-apply path must all
// refuse the same disallowed jumps, because all three ultimately write
// through db.ChangeStatusTransitionAllowed.

func newProposedChange(t *testing.T, s *Server, orgID int, actor string) *db.ChangeRequest {
	t.Helper()
	contractTestUser(t, s, orgID, actor, "admin")
	cr := &db.ChangeRequest{
		Title: "transition guard change", Description: "original", Justification: "because",
		Priority: "medium", Category: "other", RiskLevel: "low", RollbackPlan: "revert",
		RequestedBy: actor, Status: "proposed",
	}
	if err := s.db.CreateChangeRequest(context.Background(), orgID, cr); err != nil {
		t.Fatalf("CreateChangeRequest: %v", err)
	}
	return cr
}

func wantChangeInvalidTransition(t *testing.T, err error, from, to string) {
	t.Helper()
	he := wantHTTPStatus(t, err, http.StatusConflict)
	eb, ok := he.Message.(errorBody)
	if !ok || eb.Code != CodeChangeInvalidTransition {
		t.Fatalf("expected %s, got %#v", CodeChangeInvalidTransition, he.Message)
	}
	if eb.Params["status"] != from || eb.Params["value"] != to {
		t.Fatalf("params = %#v, want status=%q value=%q", eb.Params, from, to)
	}
}

// The exact bug reported in #423: proposed straight to implemented, skipping
// approval entirely, through the dedicated status endpoint.
func TestChangeStatusEndpointRejectsSkippingApproval(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-skip-status")
	cr := newProposedChange(t, s, orgID, "admin@change-transition-skip-status.test")

	c, _ := ctxFor(orgID, http.MethodPut, itoa(cr.ID), `{"status":"implemented"}`)
	err := s.handleUpdateChangeStatus(c)
	wantChangeInvalidTransition(t, err, "proposed", "implemented")

	got, gerr := s.db.GetChangeRequest(context.Background(), orgID, cr.ID)
	if gerr != nil {
		t.Fatalf("GetChangeRequest: %v", gerr)
	}
	if got.Status != "proposed" {
		t.Errorf("status = %q, want proposed (rejected transition must not write)", got.Status)
	}
}

// The same jump, attempted through the edit form's PUT instead of the
// dedicated status endpoint — both entry points share prepareChangeUpdate, so
// both must refuse it (#423, #200's "PUT and apply accept exactly the same
// fields" guarantee extends to accepting the same transitions).
func TestChangeEditFormRejectsSkippingApproval(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-skip-form")
	cr := newProposedChange(t, s, orgID, "admin@change-transition-skip-form.test")

	c, _ := ctxFor(orgID, http.MethodPut, itoa(cr.ID), `{"status":"implemented"}`)
	err := s.handleUpdateChange(c)
	wantChangeInvalidTransition(t, err, "proposed", "implemented")
}

// A rejected change is terminal: nothing in the API, including a fresh
// approval attempt, moves it anywhere else.
func TestChangeStatusEndpointRejectsLeavingRejected(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-rejected")
	cr := newProposedChange(t, s, orgID, "admin@change-transition-rejected.test")
	cr = setChangeStatus(t, s, orgID, cr.ID, "rejected")

	c, _ := ctxFor(orgID, http.MethodPut, itoa(cr.ID), `{"status":"approved"}`)
	err := s.handleUpdateChangeStatus(c)
	wantChangeInvalidTransition(t, err, "rejected", "approved")
}

// A closed change is equally terminal — reopening it, even back to its own
// immediate predecessor status, is refused. There is deliberately no admin
// override for this: a mis-clicked close means filing a new change request.
func TestChangeStatusEndpointRejectsReopeningClosed(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-closed")
	cr := newProposedChange(t, s, orgID, "admin@change-transition-closed.test")
	cr = setChangeStatus(t, s, orgID, cr.ID, "approved")
	cr = setChangeStatus(t, s, orgID, cr.ID, "implemented")
	cr = setChangeStatus(t, s, orgID, cr.ID, "closed")

	c, _ := ctxFor(orgID, http.MethodPut, itoa(cr.ID), `{"status":"implemented"}`)
	err := s.handleUpdateChangeStatus(c)
	wantChangeInvalidTransition(t, err, "closed", "implemented")
}

// Sending the status a change already has is a no-op, not a transition — it
// must succeed even where the adjacency table lists nothing for that status
// (e.g. closed -> closed), so a client that always echoes the current status
// back (the web edit form does) never 409s on an unrelated field edit.
func TestChangeStatusEndpointSameStatusIsNoOp(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-noop")
	cr := newProposedChange(t, s, orgID, "admin@change-transition-noop.test")
	cr = setChangeStatus(t, s, orgID, cr.ID, "approved")
	cr = setChangeStatus(t, s, orgID, cr.ID, "implemented")
	cr = setChangeStatus(t, s, orgID, cr.ID, "closed")

	got := setChangeStatus(t, s, orgID, cr.ID, "closed")
	if got.Status != "closed" {
		t.Errorf("status = %q, want closed", got.Status)
	}
}

// The full documented path succeeds end to end through the dedicated status
// endpoint: proposed -> approved -> in_progress -> implemented -> closed.
func TestChangeStatusEndpointAllowsDocumentedPath(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-happy-path")
	cr := newProposedChange(t, s, orgID, "admin@change-transition-happy-path.test")

	for _, next := range []string{"approved", "in_progress", "implemented", "closed"} {
		cr = setChangeStatus(t, s, orgID, cr.ID, next)
		if cr.Status != next {
			t.Fatalf("status = %q, want %q", cr.Status, next)
		}
	}
}

// Approving straight to implemented is explicitly allowed — not every change
// needs an in_progress step in between.
func TestChangeStatusEndpointAllowsSkippingInProgress(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-skip-inprogress")
	cr := newProposedChange(t, s, orgID, "admin@change-transition-skip-inprogress.test")
	cr = setChangeStatus(t, s, orgID, cr.ID, "approved")

	got := setChangeStatus(t, s, orgID, cr.ID, "implemented")
	if got.Status != "implemented" {
		t.Errorf("status = %q, want implemented", got.Status)
	}
}

// Rewriting content on an approved change withdraws the approval by sending
// it back to proposed (#197) — a server-initiated revocation, not the user's
// own requested transition, so it must succeed even though it happens by
// rewriting req.Status out from under a request that did not ask for any
// status change at all. If the transition guard ran on the post-override
// target instead of (or as well as) the user's actual request, this would
// still pass since approved -> proposed is itself allowed; what this pins is
// that the override is never second-guessed by the guard (#423).
func TestChangeWithdrawalNotBlockedByTransitionGuard(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-withdrawal")
	cr := newApprovedChange(t, s, orgID)

	got := putChange(t, s, orgID, cr.ID, `{"description":"rewritten substantially","risk_level":"critical"}`)
	if got.Status != "proposed" {
		t.Errorf("status = %q, want proposed (withdrawal)", got.Status)
	}
}

// The suggestion-apply path shares prepareChangeUpdate with the PUT handler
// (#200), so it must refuse the same disallowed jump rather than applying it
// or, worse, turning the refusal into a 500 somewhere inside WithOrgTx.
func TestChangeSuggestionApplyRejectsSkippingApproval(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-suggestion")
	cr := newProposedChange(t, s, orgID, "admin@change-transition-suggestion.test")

	sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:     "change_request",
		EntityID:       cr.Identifier,
		SuggestionType: "update",
		Payload:        json.RawMessage(`{"fields":{"status":"implemented"}}`),
	})

	c, _ := suggestionCtx(orgID, http.MethodPost, sg.ID)
	err := s.handleApplyEntitySuggestion(c)
	wantChangeInvalidTransition(t, err, "proposed", "implemented")

	got, gerr := s.db.GetChangeRequest(context.Background(), orgID, cr.ID)
	if gerr != nil {
		t.Fatalf("GetChangeRequest: %v", gerr)
	}
	if got.Status != "proposed" {
		t.Errorf("status = %q, want proposed (rejected transition must not write, nor partially apply other fields)", got.Status)
	}

	// The suggestion must stay open — it was never applied.
	reloaded, serr := s.db.GetSuggestion(context.Background(), orgID, sg.ID)
	if serr != nil {
		t.Fatalf("GetSuggestion: %v", serr)
	}
	if reloaded.Status != "open" && reloaded.Status != "in_review" {
		t.Errorf("suggestion status = %q, want still open/in_review", reloaded.Status)
	}
}
