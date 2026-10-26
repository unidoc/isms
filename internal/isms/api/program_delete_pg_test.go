package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Regression for #367: deleting a program only soft-deleted the program row,
// so its objectives stayed live in lists, display-id lookups and overdue
// check-in reminders, and a create racing the delete could add a fresh orphan.
//
// Requires a migrated Postgres; skipped otherwise.

const programDeleteActor = "admin@program-delete.test"

func programDeleteCtx(orgID int, method, path, id, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if id != "" {
		c.SetParamNames("id")
		c.SetParamValues(id)
	}
	c.Set("org_id", orgID)
	c.Set("user_role", "admin")
	c.Set("user_email", programDeleteActor)
	return c, rec
}

func programDeleteHTTPCode(t *testing.T, err error) int {
	t.Helper()
	var he *echo.HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("error = %v, want an *echo.HTTPError", err)
	}
	return he.Code
}

func newDeleteTestObjective(t *testing.T, s *Server, orgID int, programID int64, title string) *db.Objective {
	t.Helper()
	o := &db.Objective{ProgramID: programID, Title: title}
	if err := s.db.CreateObjective(context.Background(), orgID, o); err != nil {
		t.Fatalf("CreateObjective %q: %v", title, err)
	}
	return o
}

func deleteRows(t *testing.T, s *Server, orgID int, entityType string, id int64) []db.ChangelogEntry {
	t.Helper()
	rows, err := s.db.ListEntityChangelog(context.Background(), orgID, entityType, id)
	if err != nil {
		t.Fatalf("ListEntityChangelog %s %d: %v", entityType, id, err)
	}
	var out []db.ChangelogEntry
	for _, r := range rows {
		if r.Action == "delete" {
			out = append(out, r)
		}
	}
	return out
}

func TestProgramDeleteCascadesToObjectives(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "program-delete")
	contractTestUser(t, s, orgID, programDeleteActor, "admin")

	p := &db.Program{Key: "PDA", Title: "Program to delete"}
	if err := s.db.CreateProgram(ctx, orgID, p); err != nil {
		t.Fatalf("CreateProgram P: %v", err)
	}
	q := &db.Program{Key: "PDB", Title: "Program to keep"}
	if err := s.db.CreateProgram(ctx, orgID, q); err != nil {
		t.Fatalf("CreateProgram Q: %v", err)
	}
	o1 := newDeleteTestObjective(t, s, orgID, p.ID, "live")
	o2 := newDeleteTestObjective(t, s, orgID, p.ID, "archived")
	o3 := newDeleteTestObjective(t, s, orgID, q.ID, "other program")
	if err := s.db.ArchiveObjective(ctx, orgID, o2.ID); err != nil {
		t.Fatalf("ArchiveObjective: %v", err)
	}

	pid := strconv.FormatInt(p.ID, 10)
	c, rec := programDeleteCtx(orgID, http.MethodDelete, "/api/v1/programs/"+pid, pid, "")
	if err := s.handleDeleteProgram(c); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}

	t.Run("objectives of the program are gone, the other program's is not", func(t *testing.T) {
		live, err := s.db.ListObjectives(ctx, orgID, 0, "")
		if err != nil {
			t.Fatalf("ListObjectives: %v", err)
		}
		got := map[string]bool{}
		for _, o := range live {
			got[o.DisplayID] = true
		}
		for _, o := range []*db.Objective{o1, o2} {
			if got[o.DisplayID] {
				t.Errorf("%s still listed after its program was deleted", o.DisplayID)
			}
			if _, err := s.db.GetObjectiveByDisplayID(ctx, orgID, o.DisplayID); err == nil {
				t.Errorf("GetObjectiveByDisplayID(%s) found it after its program was deleted", o.DisplayID)
			}
		}
		if !got[o3.DisplayID] {
			t.Errorf("%s (other program) missing from the list", o3.DisplayID)
		}
		if _, err := s.db.GetObjectiveByDisplayID(ctx, orgID, o3.DisplayID); err != nil {
			t.Errorf("GetObjectiveByDisplayID(%s): %v", o3.DisplayID, err)
		}
	})

	t.Run("history records the program and each objective", func(t *testing.T) {
		rows := deleteRows(t, s, orgID, "program", p.ID)
		if len(rows) != 1 || rows[0].ChangedBy != programDeleteActor {
			t.Fatalf("program delete rows = %+v, want one by %s", rows, programDeleteActor)
		}
		for _, o := range []*db.Objective{o1, o2} {
			rows := deleteRows(t, s, orgID, "objective", o.ID)
			if len(rows) != 1 {
				t.Errorf("%s delete rows = %+v, want exactly one", o.DisplayID, rows)
				continue
			}
			if rows[0].Reason != "program PDA deleted" {
				t.Errorf("%s reason = %q, want %q", o.DisplayID, rows[0].Reason, "program PDA deleted")
			}
			if rows[0].ChangedBy != programDeleteActor {
				t.Errorf("%s changed_by = %q, want %q", o.DisplayID, rows[0].ChangedBy, programDeleteActor)
			}
		}
		if rows := deleteRows(t, s, orgID, "objective", o3.ID); len(rows) != 0 {
			t.Errorf("other program's objective has delete rows: %+v", rows)
		}
	})

	t.Run("activity names the program key", func(t *testing.T) {
		acts, err := s.db.RecentActivity(ctx, orgID, 20)
		if err != nil {
			t.Fatalf("RecentActivity: %v", err)
		}
		for _, a := range acts {
			if a.Action == "program_deleted" {
				if a.Detail != "Program PDA deleted" {
					t.Errorf("detail = %q, want %q", a.Detail, "Program PDA deleted")
				}
				return
			}
		}
		t.Errorf("no program_deleted activity in %+v", acts)
	})

	t.Run("deleting again is a 404 and logs nothing", func(t *testing.T) {
		c, _ := programDeleteCtx(orgID, http.MethodDelete, "/api/v1/programs/"+pid, pid, "")
		err := s.handleDeleteProgram(c)
		if err == nil {
			t.Fatal("second delete succeeded, want 404")
		}
		if code := programDeleteHTTPCode(t, err); code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", code)
		}
		if rows := deleteRows(t, s, orgID, "program", p.ID); len(rows) != 1 {
			t.Errorf("program delete rows = %d after repeat, want still 1", len(rows))
		}
		for _, o := range []*db.Objective{o1, o2} {
			if rows := deleteRows(t, s, orgID, "objective", o.ID); len(rows) != 1 {
				t.Errorf("%s delete rows = %d after repeat, want still 1", o.DisplayID, len(rows))
			}
		}
	})

	t.Run("a missing program is a 404", func(t *testing.T) {
		c, _ := programDeleteCtx(orgID, http.MethodDelete, "/api/v1/programs/999999999", "999999999", "")
		err := s.handleDeleteProgram(c)
		if err == nil {
			t.Fatal("delete of missing program succeeded, want 404")
		}
		if code := programDeleteHTTPCode(t, err); code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", code)
		}
	})

	t.Run("no objective can be added to the deleted program", func(t *testing.T) {
		o := &db.Objective{ProgramID: p.ID, Title: "late"}
		if err := s.db.CreateObjective(ctx, orgID, o); err == nil {
			t.Error("CreateObjective on a deleted program succeeded")
		}
		body := `{"program_id":` + pid + `,"title":"late"}`
		c, _ := programDeleteCtx(orgID, http.MethodPost, "/api/v1/objectives", "", body)
		err := s.handleCreateObjective(c)
		if err == nil {
			t.Fatal("handleCreateObjective on a deleted program succeeded, want 400")
		}
		if code := programDeleteHTTPCode(t, err); code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", code)
		}
	})
}
