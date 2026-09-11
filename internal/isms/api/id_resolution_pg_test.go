package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// #201: the defect this file exists for cannot be reproduced inside a single
// organization. Primary keys come from a per-TABLE `GENERATED ALWAYS AS
// IDENTITY` column; display identifiers come from a per-(org, entity type)
// counter in identifier_sequences. In one org the two march in lockstep — and
// deletes do not rewind the sequence — so the old arithmetic strip ("ASSET-3" →
// primary key 3) landed on the right row by luck and any single-org test passes
// against the bug.
//
// With two orgs they diverge on the very first row: org A's asset is PK n and
// "ASSET-1"; org B's asset is PK n+1 and also "ASSET-1", from B's own counter.
// Acting as org B, "ASSET-1" must reach PK n+1. The old parseID sent it to PK n
// — a row org B cannot even see — so this is the assertion that tells the fix
// from the bug.
//
// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays green
// without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...
func testServer(t *testing.T) *Server {
	t.Helper()
	url := os.Getenv("ISMS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ISMS_TEST_DATABASE_URL not set — skipping identifier resolution database test")
	}
	d, err := db.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}
	t.Cleanup(d.Close)
	return &Server{db: d, searchIndex: NewSearchIndex()}
}

// newTestOrg creates a throwaway organization and returns its id. Everything it
// creates is removed on cleanup so the suite can run repeatedly.
func newTestOrg(t *testing.T, s *Server, name string) int {
	t.Helper()
	ctx := context.Background()
	slug := fmt.Sprintf("%s-%d", name, os.Getpid())

	org := &db.Organization{Name: slug, Slug: slug, RepoPath: "/tmp/none"}
	if err := s.db.CreateOrganization(ctx, org); err != nil {
		t.Fatalf("creating org %s: %v", name, err)
	}
	// Soft delete; the slug/domain unique indexes filter on deleted_at, so the
	// suite can run repeatedly against the same database.
	t.Cleanup(func() {
		_ = s.db.DeleteOrganization(context.Background(), org.ID)
	})
	return org.ID
}

func newTestAsset(t *testing.T, s *Server, orgID int, name string) *db.Asset {
	t.Helper()
	a := &db.Asset{Name: name, AssetType: db.AssetTypes[0], Status: db.AssetStatuses[0]}
	if err := s.db.CreateAsset(context.Background(), orgID, a); err != nil {
		t.Fatalf("creating asset in org %d: %v", orgID, err)
	}
	return a
}

// ctxFor builds an echo context carrying the org, role and user the handlers read.
func ctxFor(orgID int, method, param, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/api/v1/assets/"+param, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(param)
	c.Set("org_id", orgID)
	c.Set("user_role", "admin")
	c.Set("user_email", "admin@id-resolution.test")
	return c, rec
}

// TestUpdateByIdentifierHitsTheRightRowAcrossOrgs is the regression that matters:
// under the old arithmetic strip this wrote to another organization's asset (or
// 404'd), silently and with a 200.
func TestUpdateByIdentifierHitsTheRightRowAcrossOrgs(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	orgA := newTestOrg(t, s, "id-res-a")
	orgB := newTestOrg(t, s, "id-res-b")

	assetA := newTestAsset(t, s, orgA, "org A first asset")
	assetB := newTestAsset(t, s, orgB, "org B first asset")

	if assetA.Identifier != assetB.Identifier {
		t.Fatalf("precondition: both orgs' first asset should be %q; got %q and %q",
			"ASSET-1", assetA.Identifier, assetB.Identifier)
	}
	if assetA.ID == assetB.ID {
		t.Fatalf("precondition: primary keys must differ across orgs; both are %d", assetA.ID)
	}

	// Act as org B, addressing the asset by the identifier it shares with org A's.
	c, rec := ctxFor(orgB, http.MethodPut, assetB.Identifier, `{"notes":"written via identifier"}`)
	if err := s.handleUpdateAsset(c); err != nil {
		t.Fatalf("handleUpdateAsset(%s) as org B: %v", assetB.Identifier, err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}

	var got db.Asset
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.ID != assetB.ID {
		t.Errorf("write landed on asset id %d, want %d — the identifier resolved to the wrong row", got.ID, assetB.ID)
	}

	after, err := s.db.GetAsset(ctx, orgB, assetB.ID)
	if err != nil {
		t.Fatalf("reading org B asset back: %v", err)
	}
	if after.Notes != "written via identifier" {
		t.Errorf("org B asset notes = %q, want the written value", after.Notes)
	}

	// Org A's asset — the one the arithmetic strip could have reached — must be untouched.
	untouched, err := s.db.GetAsset(ctx, orgA, assetA.ID)
	if err != nil {
		t.Fatalf("reading org A asset back: %v", err)
	}
	if untouched.Notes != "" {
		t.Errorf("org A asset was modified by a write addressed to org B (notes = %q)", untouched.Notes)
	}
}

// TestUpdateRejectsMismatchedAndMalformedIdentifiers: the strip accepted any
// prefix in its list and never checked the identifier belonged to an asset, so
// PUT /assets/RISK-<n> wrote to the asset whose PRIMARY KEY was n.
//
// The suffix used below is deliberately another asset's primary key in the SAME
// org — that is what makes this discriminating. With a suffix that happens to
// match no row, the old code 404s by luck and the test passes against the bug.
func TestUpdateRejectsMismatchedAndMalformedIdentifiers(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "id-res-reject")

	victim := newTestAsset(t, s, orgID, "the row a mismatched prefix would reach")
	target := newTestAsset(t, s, orgID, "the row being addressed")
	pk := fmt.Sprint(victim.ID)

	for _, tc := range []struct {
		param      string
		wantStatus int
		why        string
	}{
		{"RISK-" + pk, http.StatusNotFound, "a risk identifier must not reach an asset"},
		{"SUPPLIER-" + pk, http.StatusNotFound, "nor a supplier identifier"},
		{"FIND-" + pk, http.StatusNotFound, "nor an audit-finding identifier"},
		{"AST-" + pk, http.StatusNotFound, "nor the prefix retired in #173"},
		{"garbage", http.StatusBadRequest, "not identifier-shaped at all"},
	} {
		t.Run(tc.param, func(t *testing.T) {
			c, rec := ctxFor(orgID, http.MethodPut, tc.param, `{"notes":"should never land"}`)
			err := s.handleUpdateAsset(c)
			if err == nil {
				t.Fatalf("PUT /assets/%s returned %d — %s", tc.param, rec.Code, tc.why)
			}
			he, ok := err.(*echo.HTTPError)
			if !ok {
				t.Fatalf("error is %T, want *echo.HTTPError: %v", err, err)
			}
			if he.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d (%s)", he.Code, tc.wantStatus, tc.why)
			}
		})
	}

	// Neither asset may have been touched: not the one addressed, and above all
	// not the one whose primary key the stripped suffix pointed at.
	for _, a := range []*db.Asset{victim, target} {
		after, err := s.db.GetAsset(ctx, orgID, a.ID)
		if err != nil {
			t.Fatalf("reading asset %d back: %v", a.ID, err)
		}
		if after.Notes != "" {
			t.Errorf("asset %d (%s) was written by a request that should have been rejected (notes = %q)",
				a.ID, a.Identifier, after.Notes)
		}
	}
}

// TestNumericAndIdentifierFormsAgree is the sweep: for every register whose
// routes this change touched, both addressing forms must reach the same row on
// both the read and the write handler. That the two forms agreed on GET but not
// on PUT is precisely how #201 stayed invisible — the UI reads by identifier and
// writes by numeric id, so nothing in the browser ever exercised the broken half.
func TestNumericAndIdentifierFormsAgree(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "id-res-agree")
	ctx := context.Background()

	type entity struct {
		name    string
		id      int64
		ident   string
		get     func(echo.Context) error
		update  func(echo.Context) error
		reread  func(int64) (string, error)
		updBody string
	}

	asset := newTestAsset(t, s, orgID, "sweep asset")

	risk := &db.Risk{Title: "sweep risk", RiskType: db.RiskTypes[0], Origin: db.RiskOrigins[0], Status: db.RiskStatuses[0]}
	if err := s.db.CreateRisk(ctx, orgID, risk); err != nil {
		t.Fatalf("creating risk: %v", err)
	}
	system := &db.System{Name: "sweep system", Classification: db.SystemClassifications[0],
		Criticality: db.CriticalityLevels[0], Status: db.SystemStatuses[0]}
	if err := s.db.CreateSystem(ctx, orgID, system); err != nil {
		t.Fatalf("creating system: %v", err)
	}
	supplier := &db.Supplier{Name: "sweep supplier", SupplierType: db.SupplierTypes[0],
		Criticality: db.CriticalityLevels[0], Status: db.SupplierStatuses[0]}
	if err := s.db.CreateSupplier(ctx, orgID, supplier); err != nil {
		t.Fatalf("creating supplier: %v", err)
	}

	entities := []entity{
		{
			name: "asset", id: asset.ID, ident: asset.Identifier,
			get: s.handleGetAsset, update: s.handleUpdateAsset,
			updBody: `{"notes":"%s"}`,
			reread: func(id int64) (string, error) {
				a, err := s.db.GetAsset(ctx, orgID, id)
				if err != nil {
					return "", err
				}
				return a.Notes, nil
			},
		},
		{
			name: "risk", id: risk.ID, ident: risk.Identifier,
			get: s.handleGetRisk, update: s.handleUpdateRisk,
			updBody: `{"description":"%s"}`,
			reread: func(id int64) (string, error) {
				r, err := s.db.GetRisk(ctx, orgID, id)
				if err != nil {
					return "", err
				}
				return r.Description, nil
			},
		},
		{
			name: "system", id: system.ID, ident: system.Identifier,
			get: s.handleGetSystem, update: s.handleUpdateSystem,
			updBody: `{"description":"%s"}`,
			reread: func(id int64) (string, error) {
				sys, err := s.db.GetSystem(ctx, orgID, id)
				if err != nil {
					return "", err
				}
				return sys.Description, nil
			},
		},
		{
			name: "supplier", id: supplier.ID, ident: supplier.Identifier,
			get: s.handleGetSupplier, update: s.handleUpdateSupplier,
			updBody: `{"notes":"%s"}`,
			reread: func(id int64) (string, error) {
				sup, err := s.db.GetSupplier(ctx, orgID, id)
				if err != nil {
					return "", err
				}
				return sup.Notes, nil
			},
		},
	}

	for _, ent := range entities {
		t.Run(ent.name, func(t *testing.T) {
			for _, form := range []struct{ kind, param string }{
				{"numeric", fmt.Sprint(ent.id)},
				{"identifier", ent.ident},
			} {
				c, rec := ctxFor(orgID, http.MethodGet, form.param, "")
				if err := ent.get(c); err != nil {
					t.Fatalf("GET by %s (%s): %v", form.kind, form.param, err)
				}
				var got struct {
					ID int64 `json:"id"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decoding GET by %s: %v", form.kind, err)
				}
				if got.ID != ent.id {
					t.Errorf("GET by %s returned id %d, want %d", form.kind, got.ID, ent.id)
				}

				want := fmt.Sprintf("written by %s form", form.kind)
				c, rec = ctxFor(orgID, http.MethodPut, form.param, fmt.Sprintf(ent.updBody, want))
				if err := ent.update(c); err != nil {
					t.Fatalf("PUT by %s (%s): %v", form.kind, form.param, err)
				}
				if rec.Code != http.StatusOK {
					t.Fatalf("PUT by %s: status %d, body %s", form.kind, rec.Code, rec.Body.String())
				}
				after, err := ent.reread(ent.id)
				if err != nil {
					t.Fatalf("re-reading %s %d: %v", ent.name, ent.id, err)
				}
				if after != want {
					t.Errorf("PUT by %s did not reach %s %d (%s): field = %q, want %q",
						form.kind, ent.name, ent.id, ent.ident, after, want)
				}
			}
		})
	}
}
