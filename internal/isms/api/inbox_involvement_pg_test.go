package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays green
// without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...
//
// Regression for #205: the Inbox showed the whole organization's work. The
// list endpoints take `involving=me` and scope the rows to the caller in SQL.
// A request without it must keep returning exactly what it did before.

// inboxCtx builds an echo context for GET target, acting as email with role.
func inboxCtx(orgID int, email, role, target string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("org_id", orgID)
	c.Set("user_role", role)
	c.Set("user_email", email)
	return c, rec
}

// listRows runs a list handler and returns its `data` rows as raw JSON objects,
// so a test can tell an absent key from a false one.
func listRows(t *testing.T, h func(echo.Context) error, c echo.Context, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	if err := h(c); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body.String(), err)
	}
	return body.Data
}

func rowByID(rows []map[string]any, id int64) map[string]any {
	for _, r := range rows {
		if n, ok := r["id"].(float64); ok && int64(n) == id {
			return r
		}
	}
	return nil
}

func TestInboxInvolvingParam(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "inbox-involving")

	admin := "admin@inbox-involving.test"
	manager := "manager@inbox-involving.test"
	contributor := "contributor@inbox-involving.test"
	contractTestUser(t, s, orgID, admin, "admin")
	contractTestUser(t, s, orgID, manager, "manager")
	contractTestUser(t, s, orgID, contributor, "contributor")

	task := &db.Task{Title: "T1", TaskType: "general", Assignee: contributor, CreatedBy: admin, Status: "open", Priority: "medium"}
	if err := s.db.CreateTask(ctx, orgID, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	review := &db.Review{DocumentID: "inbox-doc", DocumentType: "policy", Title: "Inbox doc", Version: "1",
		RequestedBy: manager, Status: "open"}
	if err := s.db.CreateReview(ctx, orgID, review); err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if err := s.db.AddReviewAssignment(ctx, orgID, &db.ReviewAssignment{ReviewID: review.ID, Reviewer: contributor, Status: "pending"}); err != nil {
		t.Fatalf("AddReviewAssignment: %v", err)
	}

	t.Run("only me is accepted", func(t *testing.T) {
		handlers := map[string]func(echo.Context) error{
			"/reviews":       s.handleListReviews,
			"/tasks":         s.handleListTasks,
			"/changes":       s.handleListChanges,
			"/suggestions":   s.handleListEntitySuggestions,
			"/comments/open": s.handleAllOpenComments,
		}
		for path, h := range handlers {
			for _, v := range []string{"someone@else.test", contributor, "ME", "all"} {
				c, _ := inboxCtx(orgID, contributor, "contributor", path+"?involving="+v)
				err := h(c)
				var he *echo.HTTPError
				if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
					t.Errorf("GET %s?involving=%s: err = %v, want a 400", path, v, err)
				}
			}
		}
	})

	t.Run("reviews", func(t *testing.T) {
		c, rec := inboxCtx(orgID, contributor, "contributor", "/reviews?involving=me")
		row := rowByID(listRows(t, s.handleListReviews, c, rec), int64(review.ID))
		if row == nil {
			t.Fatal("scoped list is missing the review assigned to the caller")
		}
		if row["needs_action"] != true || row["inbox_group"] != "to_review" {
			t.Errorf("scoped row: needs_action=%v inbox_group=%v, want true/to_review", row["needs_action"], row["inbox_group"])
		}
		if r, ok := row["reviewers"].([]any); !ok || len(r) != 1 || r[0] != contributor {
			t.Errorf("reviewers = %v, want [%s]", row["reviewers"], contributor)
		}

		// The same row without the parameter carries neither computed key.
		c, rec = inboxCtx(orgID, contributor, "contributor", "/reviews")
		row = rowByID(listRows(t, s.handleListReviews, c, rec), int64(review.ID))
		if row == nil {
			t.Fatal("unscoped list is missing the review")
		}
		for _, k := range []string{"needs_action", "inbox_group"} {
			if _, present := row[k]; present {
				t.Errorf("unscoped row has %q, want it absent", k)
			}
		}
	})

	t.Run("tasks", func(t *testing.T) {
		c, rec := inboxCtx(orgID, contributor, "contributor", "/tasks?involving=me")
		row := rowByID(listRows(t, s.handleListTasks, c, rec), task.ID)
		if row == nil {
			t.Fatal("scoped list is missing the task assigned to the caller")
		}
		if row["needs_action"] != true || row["inbox_group"] != "assigned" {
			t.Errorf("scoped row: needs_action=%v inbox_group=%v, want true/assigned", row["needs_action"], row["inbox_group"])
		}

		c, rec = inboxCtx(orgID, contributor, "contributor", "/tasks")
		row = rowByID(listRows(t, s.handleListTasks, c, rec), task.ID)
		if row == nil {
			t.Fatal("unscoped list is missing the task")
		}
		for _, k := range []string{"needs_action", "inbox_group"} {
			if _, present := row[k]; present {
				t.Errorf("unscoped row has %q, want it absent", k)
			}
		}

		// The creator sees it as delegated work that waits on someone else.
		c, rec = inboxCtx(orgID, admin, "admin", "/tasks?involving=me")
		row = rowByID(listRows(t, s.handleListTasks, c, rec), task.ID)
		if row == nil || row["inbox_group"] != "delegated" {
			t.Errorf("creator's row = %v, want inbox_group delegated", row)
		} else if _, present := row["needs_action"]; present {
			t.Error("creator's open delegated task is flagged needs_action, want it only shown")
		}
	})

	t.Run("an uninvolved user gets empty lists", func(t *testing.T) {
		reader := "reader@inbox-involving.test"
		contractTestUser(t, s, orgID, reader, "reader")
		for path, h := range map[string]func(echo.Context) error{
			"/reviews":       s.handleListReviews,
			"/tasks":         s.handleListTasks,
			"/changes":       s.handleListChanges,
			"/suggestions":   s.handleListEntitySuggestions,
			"/comments/open": s.handleAllOpenComments,
		} {
			c, rec := inboxCtx(orgID, reader, "reader", path+"?involving=me")
			if rows := listRows(t, h, c, rec); len(rows) != 0 {
				t.Errorf("GET %s?involving=me returned %d rows for an uninvolved reader, want 0", path, len(rows))
			}
		}
	})

	t.Run("CLI inbox includes the assigned review", func(t *testing.T) {
		c, rec := inboxCtx(orgID, contributor, "contributor", "/inbox")
		rows := listRows(t, s.handleInbox, c, rec)
		var found map[string]any
		for _, r := range rows {
			if r["type"] == "review" && int64(r["id"].(float64)) == int64(review.ID) {
				found = r
			}
		}
		if found == nil {
			t.Fatalf("GET /inbox rows = %v, want the review assigned to the caller", rows)
		}
		if found["role"] != "reviewer" {
			t.Errorf("role = %v, want reviewer", found["role"])
		}

		// A reader with nothing assigned gets an empty inbox, not the org's comments.
		reader := "reader@inbox-involving.test"
		c, rec = inboxCtx(orgID, reader, "reader", "/inbox")
		if rows := listRows(t, s.handleInbox, c, rec); len(rows) != 0 {
			t.Errorf("reader GET /inbox = %v, want none", rows)
		}
	})
}
