package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays green
// without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...
//
// Regression for #196: the incident, corrective-action and legal update
// handlers took their "before" snapshot after applying the request onto the
// loaded record, so the diff was always empty and no history was written; the
// two status endpoints never wrote a changelog row at all. The risk, supplier,
// system and asset cases are controls that already worked.
func TestRegisterUpdatesWriteChangelogRow(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "register-changelog")
	const actor = "admin@id-resolution.test"

	inc := &db.Incident{Title: "inc before", Severity: "low", Status: "open",
		IncidentType: "incident", Source: "internal",
		AuthorityNotified: "not_required", SubjectsNotified: "not_required",
		Reporter: actor, DetectedAt: db.NewEpoch(time.Now())}
	if err := s.db.CreateIncident(ctx, orgID, inc); err != nil {
		t.Fatalf("CreateIncident: %v", err)
	}
	ca := &db.CorrectiveAction{Title: "ca before", CreatedBy: actor}
	applyCorrectiveActionDefaults(ca)
	if err := s.db.CreateCorrectiveAction(ctx, orgID, ca); err != nil {
		t.Fatalf("CreateCorrectiveAction: %v", err)
	}
	legal := &db.LegalRequirement{Title: "legal before", Jurisdiction: "EU", Category: "privacy", Status: "open"}
	if err := s.db.CreateLegalRequirement(ctx, orgID, legal); err != nil {
		t.Fatalf("CreateLegalRequirement: %v", err)
	}
	likelihood, impact := 2, 3
	risk := &db.Risk{
		Title:             "risk before",
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
	supplier := &db.Supplier{Name: "supplier before", SupplierType: "cloud", Criticality: "low", Status: "active"}
	if err := s.db.CreateSupplier(ctx, orgID, supplier); err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	system := &db.System{Name: "system before", Classification: "internal", Criticality: "low", Status: "active"}
	if err := s.db.CreateSystem(ctx, orgID, system); err != nil {
		t.Fatalf("CreateSystem: %v", err)
	}
	asset := newTestAsset(t, s, orgID, "asset before")

	// Order matters: the two incident status cases share one incident.
	cases := []struct {
		name       string
		entityType string
		id         int64
		handler    func(echo.Context) error
		body       string
		field      string
		oldValue   string
		newValue   string
	}{
		{"incident PUT", "incident", inc.ID, s.handleUpdateIncident, `{"title":"inc after"}`, "title", "inc before", "inc after"},
		{"incident status", "incident", inc.ID, s.handleUpdateIncidentStatus, `{"status":"investigating"}`, "status", "open", "investigating"},
		{"incident status root cause", "incident", inc.ID, s.handleUpdateIncidentStatus, `{"status":"investigating","root_cause":"rc"}`, "root_cause", "", "rc"},
		{"CA PUT", "corrective_action", ca.ID, s.handleUpdateCorrectiveAction, `{"title":"ca after"}`, "title", "ca before", "ca after"},
		{"CA status", "corrective_action", ca.ID, s.handleUpdateCorrectiveActionStatus, `{"status":"assessment"}`, "status", "todo", "assessment"},
		{"legal PUT", "legal_requirement", legal.ID, s.handleUpdateLegal, `{"title":"legal after"}`, "title", "legal before", "legal after"},
		{"risk PUT (control)", "risk", risk.ID, s.handleUpdateRisk, `{"notes":"n"}`, "notes", "", "n"},
		{"supplier PUT (control)", "supplier", supplier.ID, s.handleUpdateSupplier, `{"notes":"n"}`, "notes", "", "n"},
		{"system PUT (control)", "system", system.ID, s.handleUpdateSystem, `{"notes":"n"}`, "notes", "", "n"},
		{"asset PUT (control)", "asset", asset.ID, s.handleUpdateAsset, `{"notes":"n"}`, "notes", "", "n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := ctxFor(orgID, http.MethodPut, fmt.Sprintf("%d", tc.id), tc.body)
			c.Request().URL.RawQuery = "reason=test-196"
			if err := tc.handler(c); err != nil {
				t.Fatalf("handler: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
			}

			entries, err := s.db.ListEntityChangelog(ctx, orgID, tc.entityType, tc.id)
			if err != nil {
				t.Fatalf("ListEntityChangelog: %v", err)
			}
			var found *db.ChangelogEntry
			for i := range entries {
				if entries[i].Action == "update" && entries[i].Field == tc.field {
					found = &entries[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("no update changelog row for field %q on %s %d (got %d rows)", tc.field, tc.entityType, tc.id, len(entries))
			}
			if found.OldValue == nil || *found.OldValue != tc.oldValue {
				t.Errorf("old_value = %v, want %q", found.OldValue, tc.oldValue)
			}
			if found.NewValue == nil || *found.NewValue != tc.newValue {
				t.Errorf("new_value = %v, want %q", found.NewValue, tc.newValue)
			}
			if found.Reason != "test-196" {
				t.Errorf("reason = %q, want %q", found.Reason, "test-196")
			}
			if found.ChangedBy != actor {
				t.Errorf("changed_by = %q, want %q", found.ChangedBy, actor)
			}
		})
	}

	t.Run("incident status keeps root cause when omitted", func(t *testing.T) {
		c, rec := ctxFor(orgID, http.MethodPut, fmt.Sprintf("%d", inc.ID), `{"status":"contained"}`)
		if err := s.handleUpdateIncidentStatus(c); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
		}
		got, err := s.db.GetIncident(ctx, orgID, inc.ID)
		if err != nil {
			t.Fatalf("GetIncident: %v", err)
		}
		if got.RootCause != "rc" {
			t.Errorf("root_cause = %q, want %q", got.RootCause, "rc")
		}
	})

	t.Run("open corrective action blocks resolve and logs nothing", func(t *testing.T) {
		inc2 := &db.Incident{Title: "inc2", Severity: "low", Status: "open",
			IncidentType: "incident", Source: "internal",
			AuthorityNotified: "not_required", SubjectsNotified: "not_required",
			Reporter: actor, DetectedAt: db.NewEpoch(time.Now())}
		if err := s.db.CreateIncident(ctx, orgID, inc2); err != nil {
			t.Fatalf("CreateIncident: %v", err)
		}
		ca2 := &db.CorrectiveAction{Title: "ca2", CreatedBy: actor}
		applyCorrectiveActionDefaults(ca2)
		if err := s.db.CreateCorrectiveAction(ctx, orgID, ca2); err != nil {
			t.Fatalf("CreateCorrectiveAction: %v", err)
		}
		if err := s.db.CreateReference(ctx, orgID, &db.EntityReference{
			SourceType: "corrective_action", SourceID: ca2.Identifier,
			TargetType: "incident", TargetID: inc2.Identifier, CreatedBy: actor,
		}); err != nil {
			t.Fatalf("CreateReference: %v", err)
		}

		c, _ := ctxFor(orgID, http.MethodPut, fmt.Sprintf("%d", inc2.ID), `{"status":"resolved"}`)
		err := s.handleUpdateIncidentStatus(c)
		var he *echo.HTTPError
		if !errors.As(err, &he) || he.Code != http.StatusConflict {
			t.Fatalf("err = %v, want HTTP 409", err)
		}
		entries, err := s.db.ListEntityChangelog(ctx, orgID, "incident", inc2.ID)
		if err != nil {
			t.Fatalf("ListEntityChangelog: %v", err)
		}
		for _, e := range entries {
			if e.Action == "update" {
				t.Errorf("unexpected update changelog row for %q after a rejected request", e.Field)
			}
		}
	})
}

// Regression for #377: PUT /corrective-actions/:id returned 200 but never stored
// a new due_date, because the transactional UPDATE had no due_date column.
func TestCorrectiveActionPutStoresDueDate(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "ca-due-date")
	const actor = "admin@id-resolution.test"

	ca := &db.CorrectiveAction{Title: "ca due date", CreatedBy: actor}
	applyCorrectiveActionDefaults(ca)
	if err := s.db.CreateCorrectiveAction(ctx, orgID, ca); err != nil {
		t.Fatalf("CreateCorrectiveAction: %v", err)
	}

	assertDueDate := func(t *testing.T, want string) {
		t.Helper()
		got, err := s.db.GetCorrectiveAction(ctx, orgID, ca.ID)
		if err != nil {
			t.Fatalf("GetCorrectiveAction: %v", err)
		}
		if got.DueDate == nil || got.DueDate.Time.UTC().Format("2006-01-02") != want {
			t.Fatalf("due_date = %v, want %s", got.DueDate, want)
		}
	}

	t.Run("PUT stores due_date and logs history", func(t *testing.T) {
		c, rec := ctxFor(orgID, http.MethodPut, fmt.Sprintf("%d", ca.ID), `{"due_date":"2027-01-15"}`)
		if err := s.handleUpdateCorrectiveAction(c); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
		}
		assertDueDate(t, "2027-01-15")

		// History must record it too (#196 re-reads the stored row).
		entries, err := s.db.ListEntityChangelog(ctx, orgID, "corrective_action", ca.ID)
		if err != nil {
			t.Fatalf("ListEntityChangelog: %v", err)
		}
		found := false
		for _, e := range entries {
			if e.Action == "update" && e.Field == "due_date" {
				found = true
			}
		}
		if !found {
			t.Errorf("no due_date changelog row")
		}
	})

	t.Run("status change keeps due_date", func(t *testing.T) {
		// The status endpoint goes through the same UPDATE statement.
		c, rec := ctxFor(orgID, http.MethodPut, fmt.Sprintf("%d", ca.ID), `{"status":"assessment"}`)
		if err := s.handleUpdateCorrectiveActionStatus(c); err != nil {
			t.Fatalf("status handler: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
		}
		assertDueDate(t, "2027-01-15")
	})

	t.Run("PUT without due_date keeps it", func(t *testing.T) {
		c, rec := ctxFor(orgID, http.MethodPut, fmt.Sprintf("%d", ca.ID), `{"title":"ca due date renamed"}`)
		if err := s.handleUpdateCorrectiveAction(c); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
		}
		assertDueDate(t, "2027-01-15")
	})
}
