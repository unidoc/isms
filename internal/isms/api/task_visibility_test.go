package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// TestCanViewTask covers the single by-id visibility check (GetTask paths, which
// aren't SQL-filtered). It must mirror db.TaskViewer exactly: public → everyone;
// private → only manager/admin, the assignee, or the creator. A false positive
// here leaks a private task to an unrelated reader/contributor (incl. agents).
func TestCanViewTask(t *testing.T) {
	e := echo.New()
	ctx := func(role, email string) echo.Context {
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
		c.Set("user_role", role)
		c.Set("user_email", email)
		return c
	}
	pub := &db.Task{Private: false, Assignee: "a@x.io", CreatedBy: "c@x.io"}
	priv := &db.Task{Private: true, Assignee: "a@x.io", CreatedBy: "c@x.io"}

	cases := []struct {
		name  string
		role  string
		email string
		task  *db.Task
		want  bool
	}{
		{"public visible to unrelated reader", "reader", "z@x.io", pub, true},
		{"private visible to manager", "manager", "z@x.io", priv, true},
		{"private visible to admin", "admin", "z@x.io", priv, true},
		{"private visible to assignee", "contributor", "a@x.io", priv, true},
		{"private visible to creator", "reader", "c@x.io", priv, true},
		{"private hidden from unrelated reader", "reader", "z@x.io", priv, false},
		{"private hidden from unrelated contributor (agent)", "contributor", "z@x.io", priv, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canViewTask(ctx(tc.role, tc.email), tc.task); got != tc.want {
				t.Errorf("canViewTask(role=%s, email=%s, private=%v) = %v, want %v",
					tc.role, tc.email, tc.task.Private, got, tc.want)
			}
		})
	}
}
