package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
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

// createChangeCtx builds a POST /changes echo.Context, the create-path
// equivalent of ctxFor (which only covers PUT with an id param).
func createChangeCtx(orgID int, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/changes", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("org_id", orgID)
	c.Set("user_role", "admin")
	c.Set("user_email", "admin@id-resolution.test")
	return c, rec
}

// alip/F1: a change could be CREATED already implemented/approved/closed,
// skipping the whole guard this PR adds to the update paths — the exact #423
// symptom, just one request earlier. Both create entry points (POST /changes
// and a suggestion's create payload) must refuse it.
func TestChangeCreateRejectsSkippingApproval(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-create-skip")
	contractTestUser(t, s, orgID, "admin@id-resolution.test", "admin")

	for _, status := range []string{"approved", "in_progress", "implemented", "closed", "rejected"} {
		t.Run(status, func(t *testing.T) {
			c, _ := createChangeCtx(orgID, `{"title":"born `+status+`","description":"x","status":"`+status+`"}`)
			err := s.handleCreateChange(c)
			wantChangeInvalidTransition(t, err, "proposed", status)
		})
	}
}

// A create with no status, or an explicit "proposed", is unaffected —
// applyChangeDefaults already turns the empty case into "proposed" before
// validateChangeCreate ever runs.
func TestChangeCreateAllowsProposedOrDefault(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-create-ok")
	contractTestUser(t, s, orgID, "admin@id-resolution.test", "admin")

	c, rec := createChangeCtx(orgID, `{"title":"no status","description":"x"}`)
	if err := s.handleCreateChange(c); err != nil {
		t.Fatalf("no status: %v", err)
	}
	if got := decodeChange(t, rec.Body.Bytes()); got.Status != "proposed" {
		t.Errorf("status = %q, want proposed", got.Status)
	}

	c, rec = createChangeCtx(orgID, `{"title":"explicit proposed","description":"x","status":"proposed"}`)
	if err := s.handleCreateChange(c); err != nil {
		t.Fatalf("explicit proposed: %v", err)
	}
	if got := decodeChange(t, rec.Body.Bytes()); got.Status != "proposed" {
		t.Errorf("status = %q, want proposed", got.Status)
	}
}

// The same create-time guard applies to a suggestion's create payload — the
// MCP create_suggestion tool documents it as accepting the same fields as the
// REST POST, so an agent could otherwise mint an already-implemented change.
func TestChangeSuggestionCreateRejectsSkippingApproval(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-suggestion-create")
	contractTestUser(t, s, orgID, "admin@id-resolution.test", "admin")

	sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:     "change_request",
		SuggestionType: "create",
		Payload:        json.RawMessage(`{"title":"born via suggestion","description":"x","status":"implemented"}`),
	})

	c, _ := suggestionCtx(orgID, http.MethodPost, sg.ID)
	err := s.handleApplyEntitySuggestion(c)
	wantChangeInvalidTransition(t, err, "proposed", "implemented")

	reloaded, serr := s.db.GetSuggestion(context.Background(), orgID, sg.ID)
	if serr != nil {
		t.Fatalf("GetSuggestion: %v", serr)
	}
	if reloaded.Status != "open" && reloaded.Status != "in_review" {
		t.Errorf("suggestion status = %q, want still open/in_review (nothing created)", reloaded.Status)
	}
}

// alip/F2: execChangeStatus's "approved" branch unconditionally re-stamps
// approved_by/approved_at. Before the fix, a second manager re-sending the
// change's already-current "approved" status silently became the approver of
// record. The status endpoint's same-status short-circuit must preserve the
// original approver, not just avoid a 409.
func TestChangeStatusEndpointSameStatusPreservesOriginalApprover(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "change-transition-noop-approver")
	contractTestUser(t, s, orgID, "manager-a@change-transition-noop-approver.test", "manager")
	contractTestUser(t, s, orgID, "manager-b@change-transition-noop-approver.test", "manager")
	cr := newProposedChange(t, s, orgID, "admin@change-transition-noop-approver.test")

	approveAs := func(email string) *db.ChangeRequest {
		t.Helper()
		c, rec := ctxFor(orgID, http.MethodPut, itoa(cr.ID), `{"status":"approved"}`)
		c.Set("user_email", email)
		if err := s.handleUpdateChangeStatus(c); err != nil {
			t.Fatalf("approve as %s: %v", email, err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("approve as %s: code %d, body %s", email, rec.Code, rec.Body.String())
		}
		got, err := s.db.GetChangeRequest(context.Background(), orgID, cr.ID)
		if err != nil {
			t.Fatalf("GetChangeRequest: %v", err)
		}
		return got
	}

	first := approveAs("manager-a@change-transition-noop-approver.test")
	if first.ApprovedBy != "manager-a@change-transition-noop-approver.test" {
		t.Fatalf("approved_by = %q, want manager-a", first.ApprovedBy)
	}

	second := approveAs("manager-b@change-transition-noop-approver.test")
	if second.ApprovedBy != first.ApprovedBy {
		t.Errorf("approved_by = %q after a same-status re-send by a different user, want unchanged %q",
			second.ApprovedBy, first.ApprovedBy)
	}
	if second.ApprovedAt == nil || first.ApprovedAt == nil || second.ApprovedAt.Time.Unix() != first.ApprovedAt.Time.Unix() {
		t.Errorf("approved_at changed: %v -> %v, want unchanged", first.ApprovedAt, second.ApprovedAt)
	}
}
