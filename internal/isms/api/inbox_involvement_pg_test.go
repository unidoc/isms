package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
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

// #205 follow-up: `isms inbox list` and `dump` ran their own narrower queries,
// so the terminal disagreed with the web Inbox (an approved review waiting to be
// merged and a finished delegated task were missing, and a thread the viewer
// wrote last was listed as work). Both now call the web's queries: list keeps
// only the rows that need the caller, dump carries every involved row flagged.
func TestInboxCLIEndpointsMatchTheWebQueries(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "inbox-cli")

	// handleInboxDump reads the org's repo; an empty bare one is enough.
	repoDir := t.TempDir()
	if _, err := git.PlainInit(repoDir, true); err != nil {
		t.Fatalf("initialising repo: %v", err)
	}
	st, err := store.NewBare(repoDir)
	if err != nil {
		t.Fatalf("opening repo: %v", err)
	}
	s.stores.Store(orgID, st)

	admin := "admin@inbox-cli.test"
	manager := "manager@inbox-cli.test"
	contributor := "contributor@inbox-cli.test"
	reader := "reader@inbox-cli.test"
	contractTestUser(t, s, orgID, admin, "admin")
	contractTestUser(t, s, orgID, manager, "manager")
	contractTestUser(t, s, orgID, contributor, "contributor")
	contractTestUser(t, s, orgID, reader, "reader")

	mkReview := func(doc, status string) *db.Review {
		t.Helper()
		r := &db.Review{DocumentID: doc, DocumentType: "policy", Title: doc, Version: "1", RequestedBy: manager, Status: "open"}
		if err := s.db.CreateReview(ctx, orgID, r); err != nil {
			t.Fatalf("CreateReview %s: %v", doc, err)
		}
		if status != "open" {
			if err := s.db.UpdateReviewStatus(ctx, orgID, r.ID, status); err != nil {
				t.Fatalf("setting %s to %s: %v", doc, status, err)
			}
		}
		return r
	}
	approved := mkReview("cli-approved", "approved") // the manager has to merge it
	waiting := mkReview("cli-waiting", "open")       // the manager is only waiting on reviewers
	if err := s.db.AddReviewAssignment(ctx, orgID, &db.ReviewAssignment{ReviewID: waiting.ID, Reviewer: contributor, Status: "pending"}); err != nil {
		t.Fatalf("AddReviewAssignment: %v", err)
	}

	// The manager wrote this document-level comment last, so it is theirs but
	// waits on nobody.
	own := &db.Comment{DocumentID: "cli-doc", Author: manager, Body: "my own note"}
	if err := s.db.AddComment(ctx, orgID, own); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	// The admin delegated a task to the contributor, who finished it.
	done := &db.Task{Title: "Delegated and done", TaskType: "general", Assignee: contributor, CreatedBy: admin, Status: "open", Priority: "medium"}
	if err := s.db.CreateTask(ctx, orgID, done); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := s.db.UpdateTaskStatus(ctx, orgID, done.ID, "done"); err != nil {
		t.Fatalf("finishing task: %v", err)
	}

	listItems := func(email, role string) map[string]map[string]any {
		t.Helper()
		c, rec := inboxCtx(orgID, email, role, "/inbox")
		out := map[string]map[string]any{}
		for _, r := range listRows(t, s.handleInbox, c, rec) {
			out[fmt.Sprintf("%v:%d", r["type"], int(r["id"].(float64)))] = r
		}
		return out
	}
	type dumpRow struct {
		ID          int    `json:"id"`
		NeedsAction bool   `json:"needs_action"`
		Role        string `json:"role"`
		InboxGroup  string `json:"inbox_group"`
	}
	var dump struct {
		Reviews  []dumpRow `json:"reviews"`
		Comments []dumpRow `json:"comments"`
		Tasks    []dumpRow `json:"tasks"`
	}
	getDump := func(email, role string) {
		t.Helper()
		dump.Reviews, dump.Comments, dump.Tasks = nil, nil, nil
		c, rec := inboxCtx(orgID, email, role, "/inbox/dump")
		if err := s.handleInboxDump(c); err != nil {
			t.Fatalf("dump as %s: %v", email, err)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &dump); err != nil {
			t.Fatalf("decoding dump: %v; body %s", err, rec.Body.String())
		}
	}
	find := func(rows []dumpRow, id int) *dumpRow {
		for i := range rows {
			if rows[i].ID == id {
				return &rows[i]
			}
		}
		return nil
	}

	t.Run("the manager sees the approved review to merge", func(t *testing.T) {
		list := listItems(manager, "manager")
		item, ok := list[fmt.Sprintf("review:%d", approved.ID)]
		if !ok {
			t.Fatalf("list = %v, want the approved review", list)
		}
		if item["role"] != "author" || item["needs_action"] != true {
			t.Errorf("approved review item = %v, want role author and needs_action", item)
		}
		if _, ok := list[fmt.Sprintf("review:%d", waiting.ID)]; ok {
			t.Error("list carries a review the manager is only waiting on")
		}
		if _, ok := list[fmt.Sprintf("comment:%d", own.ID)]; ok {
			t.Error("list carries a comment the manager wrote last")
		}

		getDump(manager, "manager")
		if r := find(dump.Reviews, approved.ID); r == nil || !r.NeedsAction || r.InboxGroup != "sent" || r.Role != "author" {
			t.Errorf("dump approved review = %+v, want sent/author/needs_action", r)
		}
		if r := find(dump.Reviews, waiting.ID); r == nil || r.NeedsAction {
			t.Errorf("dump waiting review = %+v, want present with needs_action=false", r)
		}
		if cm := find(dump.Comments, int(own.ID)); cm == nil || cm.NeedsAction {
			t.Errorf("dump own comment = %+v, want present with needs_action=false", cm)
		}
	})

	t.Run("a finished delegated task is shown but not counted", func(t *testing.T) {
		// The creator was notified when the assignee finished it, so it is not
		// an action item: absent from the list, present in the dump unflagged.
		if item, ok := listItems(admin, "admin")[fmt.Sprintf("task:%d", done.ID)]; ok {
			t.Errorf("list carries the finished delegated task: %v", item)
		}
		getDump(admin, "admin")
		if r := find(dump.Tasks, int(done.ID)); r == nil || r.InboxGroup != "delegated" || r.NeedsAction {
			t.Errorf("dump task = %+v, want delegated with needs_action=false", r)
		}
	})

	t.Run("the assigned reviewer has a review to do", func(t *testing.T) {
		item, ok := listItems(contributor, "contributor")[fmt.Sprintf("review:%d", waiting.ID)]
		if !ok || item["role"] != "reviewer" {
			t.Errorf("list item = %v (present %v), want the review with role reviewer", item, ok)
		}
	})

	t.Run("an uninvolved reader has an empty inbox", func(t *testing.T) {
		if list := listItems(reader, "reader"); len(list) != 0 {
			t.Errorf("list = %v, want none", list)
		}
		getDump(reader, "reader")
		if len(dump.Reviews)+len(dump.Comments)+len(dump.Tasks) != 0 {
			t.Errorf("dump = %+v, want empty", dump)
		}
	})
}
