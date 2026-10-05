package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

func legalBody(t *testing.T, m map[string]any) string {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func wantBadRequest(t *testing.T, what string, err error) {
	t.Helper()
	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
		t.Errorf("%s: err = %v, want a 400 HTTPError", what, err)
	}
}

// TestLegalCategoriesSaveThroughAPI is the live-Postgres regression for #269:
// every category the server lists must be storable (so the migration and
// db.LegalCategories agree), and the dropped web-only values and an empty
// category must be rejected with a 400 rather than reaching the CHECK.
//
// Requires a migrated Postgres; skipped when ISMS_TEST_DATABASE_URL is unset.
func TestLegalCategoriesSaveThroughAPI(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "legal-categories")
	ctx := t.Context()

	create := func(category string) (db.LegalRequirement, error) {
		body := legalBody(t, map[string]any{"title": "cat " + category, "jurisdiction": "EU", "category": category})
		c, rec := ctxForPath(orgID, http.MethodPost, "/api/v1/legal", body, "admin")
		var lr db.LegalRequirement
		if err := s.handleCreateLegal(c); err != nil {
			return lr, err
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &lr); err != nil {
			t.Fatalf("decode create response: %v", err)
		}
		return lr, nil
	}
	update := func(id int64, body string) error {
		c, _ := ctxForPath(orgID, http.MethodPut, "/api/v1/legal/"+strconv.FormatInt(id, 10), body, "admin")
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(id, 10))
		return s.handleUpdateLegal(c)
	}
	stored := func(id int64) string {
		t.Helper()
		lr, err := s.db.GetLegalRequirement(ctx, orgID, id)
		if err != nil {
			t.Fatalf("GetLegalRequirement: %v", err)
		}
		return lr.Category
	}

	// 1. Every category creates.
	for _, cat := range db.LegalCategories {
		lr, err := create(cat)
		if err != nil {
			t.Errorf("create with category %q: %v", cat, err)
			continue
		}
		if got := stored(lr.ID); got != cat {
			t.Errorf("create with category %q stored %q", cat, got)
		}
	}

	row, err := create("privacy")
	if err != nil {
		t.Fatalf("create base row: %v", err)
	}

	// 2. Every category can be set by update.
	for _, cat := range db.LegalCategories {
		if err := update(row.ID, legalBody(t, map[string]any{"category": cat})); err != nil {
			t.Errorf("update to category %q: %v", cat, err)
			continue
		}
		if got := stored(row.ID); got != cat {
			t.Errorf("update to category %q stored %q", cat, got)
		}
	}

	// 3. The dropped web-only values are rejected on create and update.
	if err := update(row.ID, legalBody(t, map[string]any{"category": "corporate"})); err != nil {
		t.Fatalf("reset to corporate: %v", err)
	}
	for _, bad := range []string{"data_protection", "regulatory"} {
		_, err := create(bad)
		wantBadRequest(t, "create with category "+bad, err)
		wantBadRequest(t, "update to category "+bad, update(row.ID, legalBody(t, map[string]any{"category": bad})))
		if got := stored(row.ID); got != "corporate" {
			t.Errorf("rejected update to %q changed the row to %q", bad, got)
		}
	}

	// 4. An empty category is a 400, not a database error, and changes nothing.
	err = update(row.ID, `{"category": ""}`)
	wantBadRequest(t, "update with empty category", err)
	var he *echo.HTTPError
	if errors.As(err, &he) && strings.Contains(strings.ToLower(he.Error()), "constraint") {
		t.Errorf("empty category surfaced a database error: %v", he)
	}
	if got := stored(row.ID); got != "corporate" {
		t.Errorf("rejected empty update changed the row to %q", got)
	}
}
