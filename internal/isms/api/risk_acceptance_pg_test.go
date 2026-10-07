package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// #414: "accepted" was wired through the schema (accepted_at/accepted_by_id,
// chk_risk_accepted), the API's provenance logic in prepareRiskUpdate and the
// CLI's `risk treat --decision accept`, but was never in db.RiskStatuses or the
// risks_status_check constraint, so none of it could ever run — every attempt
// 400'd with "invalid status". Requires a migrated Postgres (the new
// risks_status_check that allows 'accepted'); skipped when
// ISMS_TEST_DATABASE_URL is unset — see id_resolution_pg_test.go's testServer.

func TestRiskAcceptanceSetsProvenanceAndReopenClearsIt(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "risk-accept")

	risk := &db.Risk{
		Title: "Accept me", RiskType: "threat", Origin: "internal",
		Owner: "admin@custom-fields.test", Status: "open", Treatment: "accept",
	}
	if err := s.db.CreateRisk(ctx, orgID, risk); err != nil {
		t.Fatalf("CreateRisk: %v", err)
	}
	if risk.AcceptedAt != nil || risk.AcceptedByID != nil {
		t.Fatalf("newly created open risk must start unaccepted, got AcceptedAt=%v AcceptedByID=%v", risk.AcceptedAt, risk.AcceptedByID)
	}

	// Accept it — a real PUT through handleUpdateRisk, the same path the web
	// edit form and `isms risk treat --decision accept` both use.
	c, rec := ctxForPath(orgID, http.MethodPut, "/api/v1/risks/"+risk.Identifier, `{"status":"accepted"}`, "admin")
	c.SetParamNames("id")
	c.SetParamValues(risk.Identifier)
	if err := s.handleUpdateRisk(c); err != nil {
		t.Fatalf("handleUpdateRisk (accept): %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}

	accepted, err := s.db.GetRisk(ctx, orgID, risk.ID)
	if err != nil {
		t.Fatalf("GetRisk after accept: %v", err)
	}
	if accepted.Status != "accepted" {
		t.Fatalf("status = %q, want accepted", accepted.Status)
	}
	if accepted.AcceptedAt == nil {
		t.Fatal("accepted_at not set on transition to accepted")
	}
	if accepted.AcceptedByID == nil {
		t.Fatal("accepted_by_id not set on transition to accepted")
	}
	if accepted.AcceptedBy != "admin@custom-fields.test" {
		t.Errorf("accepted_by (resolved email) = %q, want admin@custom-fields.test", accepted.AcceptedBy)
	}

	// Reopen it — accepted_at/accepted_by_id must clear, not linger from the
	// prior acceptance (an operator editing an "accepted" risk's details would
	// otherwise see a stale acceptance survive an unrelated reopen).
	c2, rec2 := ctxForPath(orgID, http.MethodPut, "/api/v1/risks/"+risk.Identifier, `{"status":"open"}`, "admin")
	c2.SetParamNames("id")
	c2.SetParamValues(risk.Identifier)
	if err := s.handleUpdateRisk(c2); err != nil {
		t.Fatalf("handleUpdateRisk (reopen): %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec2.Code, rec2.Body.String())
	}

	reopened, err := s.db.GetRisk(ctx, orgID, risk.ID)
	if err != nil {
		t.Fatalf("GetRisk after reopen: %v", err)
	}
	if reopened.Status != "open" {
		t.Fatalf("status = %q, want open", reopened.Status)
	}
	if reopened.AcceptedAt != nil {
		t.Errorf("accepted_at = %v, want nil after reopen", reopened.AcceptedAt)
	}
	if reopened.AcceptedByID != nil {
		t.Errorf("accepted_by_id = %v, want nil after reopen", reopened.AcceptedByID)
	}
	if reopened.AcceptedBy != "" {
		t.Errorf("accepted_by = %q, want empty after reopen", reopened.AcceptedBy)
	}
}

// Before #414, "accepted" was rejected by db.RiskStatuses the same as any
// other invalid string. This pins the still-rejected case so a future change
// to RiskStatuses can't silently widen it to anything.
func TestRiskUpdateRejectsUnknownStatus(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "risk-bad-status")

	risk := &db.Risk{Title: "R", RiskType: "threat", Origin: "internal", Owner: "admin@custom-fields.test", Status: "open"}
	if err := s.db.CreateRisk(ctx, orgID, risk); err != nil {
		t.Fatalf("CreateRisk: %v", err)
	}

	c, _ := ctxForPath(orgID, http.MethodPut, "/api/v1/risks/"+risk.Identifier, `{"status":"treating"}`, "admin")
	c.SetParamNames("id")
	c.SetParamValues(risk.Identifier)
	err := s.handleUpdateRisk(c)
	if err == nil {
		t.Fatal("expected an error for status=treating (never a real risk status)")
	}
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Errorf("err = %v, want *echo.HTTPError 400", err)
	}
}
