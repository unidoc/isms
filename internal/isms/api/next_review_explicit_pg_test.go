package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
// Regression for #202: the risk, supplier, system and legal-requirement create
// and update paths accepted a next_review date, returned 2xx, and then
// recalculated it in the database write. A caller-supplied date must be stored
// as sent; an absent or null one still means "work it out". Legal requirements
// must also count their review cycle from last_review, or a backdated
// last_review can never make one overdue.

// explicitReviewDate is far enough out that no review cycle could produce it.
const explicitReviewDate = "2031-02-14"

// ctxForCreate builds an echo context for a POST with a JSON body and no path
// parameter. ctxFor (id_resolution_pg_test.go) covers the PUT handlers.
func ctxForCreate(orgID int, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("org_id", orgID)
	c.Set("user_role", "admin")
	c.Set("user_email", "admin@id-resolution.test")
	return c, rec
}

func utcDate(e *db.Epoch) string {
	if e == nil {
		return "<nil>"
	}
	return e.Time.UTC().Format("2006-01-02")
}

func TestExplicitNextReviewIsStored(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "next-review-explicit")
	// The risk create handler defaults the owner to the caller and requires that
	// caller to be a member of the organization.
	contractTestUser(t, s, orgID, "admin@id-resolution.test", "admin")

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
	legal := &db.LegalRequirement{Title: "legal before", Jurisdiction: "EU", Category: "privacy", Status: "open"}
	if err := s.db.CreateLegalRequirement(ctx, orgID, legal); err != nil {
		t.Fatalf("CreateLegalRequirement: %v", err)
	}

	t.Run("update keeps an explicit date", func(t *testing.T) {
		body := fmt.Sprintf(`{"next_review":%q}`, explicitReviewDate)
		cases := []struct {
			name    string
			id      int64
			handler func(echo.Context) error
			stored  func() (*db.Epoch, error)
		}{
			{"risk", risk.ID, s.handleUpdateRisk, func() (*db.Epoch, error) {
				r, err := s.db.GetRisk(ctx, orgID, risk.ID)
				if err != nil {
					return nil, err
				}
				return r.NextReview, nil
			}},
			{"supplier", supplier.ID, s.handleUpdateSupplier, func() (*db.Epoch, error) {
				r, err := s.db.GetSupplier(ctx, orgID, supplier.ID)
				if err != nil {
					return nil, err
				}
				return r.NextReview, nil
			}},
			{"system", system.ID, s.handleUpdateSystem, func() (*db.Epoch, error) {
				r, err := s.db.GetSystem(ctx, orgID, system.ID)
				if err != nil {
					return nil, err
				}
				return r.NextReview, nil
			}},
			{"legal", legal.ID, s.handleUpdateLegal, func() (*db.Epoch, error) {
				r, err := s.db.GetLegalRequirement(ctx, orgID, legal.ID)
				if err != nil {
					return nil, err
				}
				return r.NextReview, nil
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				c, rec := ctxFor(orgID, http.MethodPut, fmt.Sprintf("%d", tc.id), body)
				if err := tc.handler(c); err != nil {
					t.Fatalf("handler: %v", err)
				}
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
				}
				got, err := tc.stored()
				if err != nil {
					t.Fatalf("read back: %v", err)
				}
				if utcDate(got) != explicitReviewDate {
					t.Fatalf("next_review = %s, want %s", utcDate(got), explicitReviewDate)
				}
			})
		}
	})

	t.Run("create keeps an explicit date", func(t *testing.T) {
		cases := []struct {
			name    string
			body    string
			handler func(echo.Context) error
			stored  func(id int64) (*db.Epoch, error)
		}{
			{
				"risk",
				`{"title":"risk create","risk_type":"threat","origin":"internal","status":"open","treatment":"mitigate","current_likelihood":2,"current_impact":3,"next_review":"2031-02-14"}`,
				s.handleAddRisk,
				func(id int64) (*db.Epoch, error) {
					r, err := s.db.GetRisk(ctx, orgID, id)
					if err != nil {
						return nil, err
					}
					return r.NextReview, nil
				},
			},
			{
				"supplier",
				`{"name":"supplier create","supplier_type":"saas","criticality":"low","data_access":false,"status":"active","next_review":"2031-02-14"}`,
				s.handleAddSupplier,
				func(id int64) (*db.Epoch, error) {
					r, err := s.db.GetSupplier(ctx, orgID, id)
					if err != nil {
						return nil, err
					}
					return r.NextReview, nil
				},
			},
			{
				"system",
				`{"name":"system create","classification":"internal","criticality":"low","status":"active","next_review":"2031-02-14"}`,
				s.handleCreateSystem,
				func(id int64) (*db.Epoch, error) {
					r, err := s.db.GetSystem(ctx, orgID, id)
					if err != nil {
						return nil, err
					}
					return r.NextReview, nil
				},
			},
			{
				"legal",
				`{"title":"legal create","jurisdiction":"EU","category":"privacy","status":"open","next_review":"2031-02-14"}`,
				s.handleCreateLegal,
				func(id int64) (*db.Epoch, error) {
					r, err := s.db.GetLegalRequirement(ctx, orgID, id)
					if err != nil {
						return nil, err
					}
					return r.NextReview, nil
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				c, rec := ctxForCreate(orgID, tc.body)
				if err := tc.handler(c); err != nil {
					t.Fatalf("handler: %v", err)
				}
				if rec.Code != http.StatusCreated {
					t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body.String())
				}
				var created struct {
					ID int64 `json:"id"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == 0 {
					t.Fatalf("decode created id: %v; body %s", err, rec.Body.String())
				}
				got, err := tc.stored(created.ID)
				if err != nil {
					t.Fatalf("read back: %v", err)
				}
				if utcDate(got) != explicitReviewDate {
					t.Fatalf("next_review = %s, want %s", utcDate(got), explicitReviewDate)
				}
			})
		}
	})
}

// TestAbsentOrNullNextReviewRecalculates pins the other half of the contract:
// without a date in the request, update still works the date out, and it
// replaces a date that an earlier request set.
func TestAbsentOrNullNextReviewRecalculates(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "next-review-recalc")

	// A fixed last_review keeps the expected date independent of today's date.
	lastReview := db.NewEpoch(time.Date(2030, 3, 10, 0, 0, 0, 0, time.UTC))
	supplier := &db.Supplier{Name: "supplier recalc", SupplierType: "cloud", Criticality: "low", Status: "active", LastReview: &lastReview}
	if err := s.db.CreateSupplier(ctx, orgID, supplier); err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}

	expected := func(t *testing.T) string {
		t.Helper()
		stored, err := s.db.GetSupplier(ctx, orgID, supplier.ID)
		if err != nil {
			t.Fatalf("GetSupplier: %v", err)
		}
		want := *stored
		want.CalculateNextReview(s.db.SupplierReviewCycles(ctx, orgID))
		return utcDate(want.NextReview)
	}
	put := func(t *testing.T, body string) {
		t.Helper()
		c, rec := ctxFor(orgID, http.MethodPut, fmt.Sprintf("%d", supplier.ID), body)
		if err := s.handleUpdateSupplier(c); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
		}
	}
	storedDate := func(t *testing.T) string {
		t.Helper()
		stored, err := s.db.GetSupplier(ctx, orgID, supplier.ID)
		if err != nil {
			t.Fatalf("GetSupplier: %v", err)
		}
		return utcDate(stored.NextReview)
	}

	for _, tc := range []struct{ name, body string }{
		{"absent", `{"notes":"x"}`},
		{"null", `{"next_review":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			put(t, fmt.Sprintf(`{"next_review":%q}`, explicitReviewDate))
			if got := storedDate(t); got != explicitReviewDate {
				t.Fatalf("setup: next_review = %s, want %s", got, explicitReviewDate)
			}
			put(t, tc.body)
			got, want := storedDate(t), expected(t)
			if got == explicitReviewDate {
				t.Fatalf("next_review still %s after a request without a date", got)
			}
			if got != want {
				t.Fatalf("next_review = %s, want the calculated %s", got, want)
			}
		})
	}
}

// TestLegalBackdatedLastReviewBecomesOverdue is the B2 regression: the legal
// review date used to count from today, so GET /overdue never listed a legal
// requirement and the legal_review task could never be created.
func TestLegalBackdatedLastReviewBecomesOverdue(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "next-review-legal-overdue")

	legal := &db.LegalRequirement{Title: "legal overdue", Jurisdiction: "EU", Category: "privacy", Status: "open"}
	if err := s.db.CreateLegalRequirement(ctx, orgID, legal); err != nil {
		t.Fatalf("CreateLegalRequirement: %v", err)
	}

	c, rec := ctxFor(orgID, http.MethodPut, fmt.Sprintf("%d", legal.ID), `{"last_review":"2024-01-15"}`)
	if err := s.handleUpdateLegal(c); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	stored, err := s.db.GetLegalRequirement(ctx, orgID, legal.ID)
	if err != nil {
		t.Fatalf("GetLegalRequirement: %v", err)
	}
	// Unassessed, so the 12-month default applies.
	if got := utcDate(stored.NextReview); got != "2025-01-15" {
		t.Fatalf("next_review = %s, want 2025-01-15", got)
	}

	summary, err := s.db.GetOverdueSummary(ctx, orgID, db.TaskViewer{CanSeeAll: true})
	if err != nil {
		t.Fatalf("GetOverdueSummary: %v", err)
	}
	found := false
	for _, item := range summary.Legal {
		if item.EntityID == legal.Identifier {
			found = true
		}
	}
	if !found {
		t.Errorf("overdue legal = %+v, want an item for %s", summary.Legal, legal.Identifier)
	}
}

// TestUpdateSupplierNilRecalculates keeps the nil contract honest for the
// internal callers (the manager cron, the review paths): with no explicit date
// the stored value is the calculated one, whatever the struct carried in.
func TestUpdateSupplierNilRecalculates(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "next-review-nil")

	supplier := &db.Supplier{Name: "supplier nil", SupplierType: "cloud", Criticality: "low", Status: "active"}
	if err := s.db.CreateSupplier(ctx, orgID, supplier); err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	sup, err := s.db.GetSupplier(ctx, orgID, supplier.ID)
	if err != nil {
		t.Fatalf("GetSupplier: %v", err)
	}
	// A fixed last_review keeps the expected date independent of today's date.
	lastReview := db.NewEpoch(time.Date(2030, 3, 10, 0, 0, 0, 0, time.UTC))
	farOff := db.NewEpoch(time.Date(2031, 2, 14, 0, 0, 0, 0, time.UTC))
	sup.LastReview = &lastReview
	sup.NextReview = &farOff

	want := *sup
	want.CalculateNextReview(s.db.SupplierReviewCycles(ctx, orgID))

	if err := s.db.UpdateSupplier(ctx, orgID, sup, nil); err != nil {
		t.Fatalf("UpdateSupplier: %v", err)
	}
	stored, err := s.db.GetSupplier(ctx, orgID, supplier.ID)
	if err != nil {
		t.Fatalf("GetSupplier: %v", err)
	}
	if got := utcDate(stored.NextReview); got == explicitReviewDate || got != utcDate(want.NextReview) {
		t.Fatalf("next_review = %s, want the calculated %s", got, utcDate(want.NextReview))
	}
}
