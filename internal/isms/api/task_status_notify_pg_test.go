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
// without a database.
//
// #205: a manager who delegates a task had no way to learn the assignee started
// or finished it short of reopening the task page. Changing a task's status
// now leaves an in-app notification for its creator, unless the creator made
// the change themselves or the status did not move.

// taskStatusCtx builds an echo context for PUT /tasks/:id/status.
func taskStatusCtx(orgID int, taskID int64, email, role, status string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/tasks/%d/status", taskID),
		strings.NewReader(fmt.Sprintf(`{"status":%q}`, status)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", taskID))
	c.Set("org_id", orgID)
	c.Set("user_role", role)
	c.Set("user_email", email)
	return c, rec
}

func taskStatusNotifications(t *testing.T, s *Server, orgID int, email string) []db.Notification {
	t.Helper()
	var out []db.Notification
	for _, n := range notificationsFor(t, s, orgID, email) {
		if n.TitleKey == NotifyKeyTaskStatusChanged {
			out = append(out, n)
		}
	}
	return out
}

func TestTaskStatusChangeNotifiesTheCreator(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "task-status-notify")

	creator := "creator@task-status-notify.test"
	assignee := "assignee@task-status-notify.test"
	contractTestUser(t, s, orgID, creator, "admin")
	contractTestUser(t, s, orgID, assignee, "contributor")

	task := &db.Task{Title: "Rotate the keys", TaskType: "general", Assignee: assignee, CreatedBy: creator,
		Status: "open", Priority: "medium"}
	if err := s.db.CreateTask(ctx, orgID, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	move := func(email, role, status string) {
		t.Helper()
		c, rec := taskStatusCtx(orgID, task.ID, email, role, status)
		if err := s.handleUpdateTaskStatus(c); err != nil {
			t.Fatalf("%s moves task to %s: %v", email, status, err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
		}
	}

	// The assignee starts the task: the creator is told, the assignee is not.
	move(assignee, "contributor", "in_progress")
	got := taskStatusNotifications(t, s, orgID, creator)
	if len(got) != 1 {
		t.Fatalf("creator has %d task notifications, want 1", len(got))
	}
	n := got[0]
	if n.BodyKey != NotifyKeyTaskStatusChangedBody || n.Link != "/inbox/tasks" {
		t.Errorf("body_key=%q link=%q, want %q and /inbox/tasks", n.BodyKey, n.Link, NotifyKeyTaskStatusChangedBody)
	}
	if n.Params["actor"] != assignee || n.Params["status"] != "in_progress" || n.Params["title"] != "Rotate the keys" {
		t.Errorf("params = %v, want actor=%s status=in_progress title=Rotate the keys", n.Params, assignee)
	}
	if n.Title != "Task in progress: Rotate the keys" {
		t.Errorf("English fallback title = %q", n.Title)
	}
	if len(taskStatusNotifications(t, s, orgID, assignee)) != 0 {
		t.Error("the assignee was notified about their own change")
	}

	// Sending the same status again is not a change.
	move(assignee, "contributor", "in_progress")
	if got := taskStatusNotifications(t, s, orgID, creator); len(got) != 1 {
		t.Errorf("creator has %d task notifications after a repeated status, want still 1", len(got))
	}

	// The creator moving their own task does not notify themselves.
	move(creator, "admin", "done")
	if got := taskStatusNotifications(t, s, orgID, creator); len(got) != 1 {
		t.Errorf("creator has %d task notifications after their own change, want still 1", len(got))
	}

	// A later change by the assignee is a new event.
	move(assignee, "contributor", "open")
	if got := taskStatusNotifications(t, s, orgID, creator); len(got) != 2 {
		t.Errorf("creator has %d task notifications after a second change, want 2", len(got))
	}
}
