package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays green
// without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...
//
// Regression for #295: every readings endpoint returned 201 while its
// entity_changelog row was rejected by entity_changelog_action_check and
// dropped by logChange, so a reading left no trace in the entity's history.
func TestReadingsWriteChangelogRow(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "readings-changelog")
	const actor = "manager@readings-changelog.test"

	postReading := func(id int64, body string) (echo.Context, *httptest.ResponseRecorder) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", id))
		c.Set("org_id", orgID)
		c.Set("user_role", "manager")
		c.Set("user_email", actor)
		return c, rec
	}

	likelihood, impact := 2, 3
	risk := &db.Risk{
		Title:             "risk for changelog test",
		RiskType:          db.RiskTypes[0],
		Origin:            db.RiskOrigins[0],
		Status:            db.RiskStatuses[0],
		Treatment:         "mitigate",
		CurrentLikelihood: &likelihood,
		CurrentImpact:     &impact,
	}
	if err := s.db.CreateRisk(ctx, orgID, risk); err != nil {
		t.Fatalf("CreateRisk: %v", err)
	}
	asset := newTestAsset(t, s, orgID, "asset for changelog test")
	legal := &db.LegalRequirement{Title: "legal for changelog test", Jurisdiction: "EU", Category: "privacy", Status: "open"}
	if err := s.db.CreateLegalRequirement(ctx, orgID, legal); err != nil {
		t.Fatalf("CreateLegalRequirement: %v", err)
	}
	supplier := &db.Supplier{Name: "supplier for changelog test", SupplierType: "cloud", Criticality: "low", Status: "active"}
	if err := s.db.CreateSupplier(ctx, orgID, supplier); err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	system := &db.System{Name: "system for changelog test", Classification: "internal", Criticality: "low", Status: "active"}
	if err := s.db.CreateSystem(ctx, orgID, system); err != nil {
		t.Fatalf("CreateSystem: %v", err)
	}

	cases := []struct {
		entityType string
		id         int64
		body       string
		handler    func(echo.Context) error
	}{
		{"risk", risk.ID, `{"current_likelihood":2,"current_impact":3}`, s.handleCreateRiskReading},
		{"asset", asset.ID, `{"confidentiality":2,"integrity":2,"availability":2}`, s.handleCreateAssetReading},
		{"legal_requirement", legal.ID, `{"current_likelihood":3,"current_impact":2}`, s.handleCreateLegalReading},
		{"supplier", supplier.ID, `{"confidentiality":4,"integrity":3,"availability":5}`, s.handleCreateSupplierReading},
		{"system", system.ID, `{"confidentiality":2,"integrity":2,"availability":2}`, s.handleCreateSystemReading},
	}
	for _, tc := range cases {
		t.Run(tc.entityType, func(t *testing.T) {
			c, rec := postReading(tc.id, tc.body)
			if err := tc.handler(c); err != nil {
				t.Fatalf("handler: %v", err)
			}
			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body.String())
			}

			entries, err := s.db.ListEntityChangelog(ctx, orgID, tc.entityType, tc.id)
			if err != nil {
				t.Fatalf("ListEntityChangelog: %v", err)
			}
			var found *db.ChangelogEntry
			for i := range entries {
				if entries[i].Action == "reading" {
					found = &entries[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("no changelog row with action \"reading\" for %s %d (got %d rows)", tc.entityType, tc.id, len(entries))
			}
			if found.ChangedBy != actor {
				t.Errorf("changed_by = %q, want %q", found.ChangedBy, actor)
			}
			if !strings.HasPrefix(found.Reason, "Reading #") {
				t.Errorf("reason = %q, want it to start with \"Reading #\"", found.Reason)
			}
		})
	}
}
