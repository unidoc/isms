package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Requires a migrated Postgres; skipped otherwise.
//
// #203: the assignee of a corrective action could not work it, and the assignee
// of a task could not write its notes. These tests pin the role matrix for the
// narrow routes: manager/admin on anything, a contributor only on their own
// item, everyone else refused.

type assignmentFixture struct {
	s     *Server
	orgID int
	owner string
	other string
	mgr   string
	rdr   string
}

func newAssignmentFixture(t *testing.T, name string) *assignmentFixture {
	t.Helper()
	s := testServer(t)
	orgID := newTestOrg(t, s, name)
	f := &assignmentFixture{
		s: s, orgID: orgID,
		owner: "owner@" + name + ".test",
		other: "other@" + name + ".test",
		mgr:   "mgr@" + name + ".test",
		rdr:   "rdr@" + name + ".test",
	}
	contractTestUser(t, s, orgID, "boss@"+name+".test", "admin")
	contractTestUser(t, s, orgID, f.owner, "contributor")
	contractTestUser(t, s, orgID, f.other, "contributor")
	contractTestUser(t, s, orgID, f.mgr, "manager")
	contractTestUser(t, s, orgID, f.rdr, "reader")
	return f
}

// call runs handler as email/role with a JSON body against item id.
func (f *assignmentFixture) call(h func(echo.Context) error, id int64, email, role, body string) (error, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/x/%d", id), strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", id))
	c.Set("org_id", f.orgID)
	c.Set("user_role", role)
	c.Set("user_email", email)
	return h(c), rec
}

func (f *assignmentFixture) wantOK(t *testing.T, err error, rec *httptest.ResponseRecorder) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
}

func (f *assignmentFixture) newCA(t *testing.T, assignee string) *db.CorrectiveAction {
	t.Helper()
	ca := &db.CorrectiveAction{Title: "ca " + assignee, CreatedBy: f.mgr, Assignee: assignee}
	applyCorrectiveActionDefaults(ca)
	if err := f.s.db.CreateCorrectiveAction(context.Background(), f.orgID, ca); err != nil {
		t.Fatalf("CreateCorrectiveAction: %v", err)
	}
	return ca
}

func (f *assignmentFixture) newTask(t *testing.T, assignee string, private bool) *db.Task {
	t.Helper()
	task := &db.Task{Title: "task " + assignee, TaskType: "general", Assignee: assignee, CreatedBy: f.mgr,
		Status: "open", Priority: "medium", Private: private}
	if err := f.s.db.CreateTask(context.Background(), f.orgID, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task
}

func caStatusBody(status string) string { return fmt.Sprintf(`{"status":%q}`, status) }

func TestAssigneeCorrectiveActionStatus(t *testing.T) {
	f := newAssignmentFixture(t, "assign-ca-status")
	ca := f.newCA(t, f.owner)
	h := f.s.handleUpdateCorrectiveActionStatus

	err, rec := f.call(h, ca.ID, f.owner, "contributor", caStatusBody("implementation"))
	f.wantOK(t, err, rec)
	err, rec = f.call(h, ca.ID, f.owner, "contributor", caStatusBody("resolved"))
	f.wantOK(t, err, rec)
	got, _ := f.s.db.GetCorrectiveAction(context.Background(), f.orgID, ca.ID)
	if got.Status != "resolved" || got.ResolvedBy != f.owner {
		t.Fatalf("status/resolved_by = %q/%q, want resolved/%s", got.Status, got.ResolvedBy, f.owner)
	}

	ca2 := f.newCA(t, f.owner)
	err, _ = f.call(h, ca2.ID, f.other, "contributor", caStatusBody("implementation"))
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, _ = f.call(h, ca2.ID, f.rdr, "reader", caStatusBody("implementation"))
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, rec = f.call(h, ca2.ID, f.mgr, "manager", caStatusBody("implementation"))
	f.wantOK(t, err, rec)
}

func TestAssigneeCannotResolveWithOpenLinkedTask(t *testing.T) {
	f := newAssignmentFixture(t, "assign-ca-open")
	ca := f.newCA(t, f.owner)
	link := &db.Task{Title: "Follow up " + ca.Identifier, TaskType: "ca_followup", Assignee: f.owner,
		CreatedBy: f.mgr, Status: "open", Priority: "medium"}
	if err := f.s.db.CreateTask(context.Background(), f.orgID, link); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	err, _ := f.call(f.s.handleUpdateCorrectiveActionStatus, ca.ID, f.owner, "contributor", caStatusBody("resolved"))
	wantHTTPStatus(t, err, http.StatusConflict)
}

func TestAssigneeCorrectiveActionProgress(t *testing.T) {
	f := newAssignmentFixture(t, "assign-ca-progress")
	ctx := context.Background()
	ca := f.newCA(t, f.owner)
	h := f.s.handleUpdateCorrectiveActionProgress

	err, rec := f.call(h, ca.ID, f.owner, "contributor", `{"root_cause":"stale credential","notes":"rotated"}`)
	f.wantOK(t, err, rec)
	var body db.CorrectiveAction
	if jerr := json.Unmarshal(rec.Body.Bytes(), &body); jerr != nil {
		t.Fatalf("decoding response: %v", jerr)
	}
	if body.RootCause != "stale credential" || body.Notes != "rotated" || body.Status != ca.Status {
		t.Fatalf("response = %+v", body)
	}
	got, _ := f.s.db.GetCorrectiveAction(ctx, f.orgID, ca.ID)
	if got.RootCause != "stale credential" || got.Notes != "rotated" || got.Status != "todo" {
		t.Fatalf("stored = root_cause %q notes %q status %q", got.RootCause, got.Notes, got.Status)
	}
	log, err := f.s.db.ListEntityChangelog(ctx, f.orgID, "corrective_action", ca.ID)
	if err != nil {
		t.Fatalf("ListEntityChangelog: %v", err)
	}
	fields := map[string]string{}
	for _, e := range log {
		fields[e.Field] = e.ChangedBy
	}
	for _, field := range []string{"root_cause", "notes"} {
		if fields[field] != f.owner {
			t.Errorf("changelog for %s by %q, want %s (all: %v)", field, fields[field], f.owner, fields)
		}
	}

	// Only notes: root cause is kept.
	err, rec = f.call(h, ca.ID, f.owner, "contributor", `{"notes":"second note"}`)
	f.wantOK(t, err, rec)
	got, _ = f.s.db.GetCorrectiveAction(ctx, f.orgID, ca.ID)
	if got.RootCause != "stale credential" || got.Notes != "second note" {
		t.Fatalf("after notes-only: root_cause %q notes %q", got.RootCause, got.Notes)
	}

	err, _ = f.call(h, ca.ID, f.owner, "contributor", `{}`)
	wantHTTPStatus(t, err, http.StatusBadRequest)
	err, _ = f.call(h, ca.ID, f.other, "contributor", `{"notes":"x"}`)
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, _ = f.call(h, ca.ID, f.rdr, "reader", `{"notes":"x"}`)
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, rec = f.call(h, ca.ID, f.mgr, "manager", `{"notes":"mgr note"}`)
	f.wantOK(t, err, rec)
}

func TestAssigneeTaskNotes(t *testing.T) {
	f := newAssignmentFixture(t, "assign-task-notes")
	ctx := context.Background()
	task := f.newTask(t, f.owner, false)
	h := f.s.handleUpdateTaskNotes

	err, rec := f.call(h, task.ID, f.owner, "contributor", `{"notes":"half done"}`)
	f.wantOK(t, err, rec)
	got, _ := f.s.db.GetTask(ctx, f.orgID, task.ID)
	if got.Notes != "half done" || got.Status != "open" {
		t.Fatalf("stored notes %q status %q", got.Notes, got.Status)
	}
	log, _ := f.s.db.ListEntityChangelog(ctx, f.orgID, "task", task.ID)
	found := false
	for _, e := range log {
		if e.Field == "notes" && e.ChangedBy == f.owner {
			found = true
		}
	}
	if !found {
		t.Errorf("no notes changelog entry by owner: %+v", log)
	}

	err, _ = f.call(h, task.ID, f.other, "contributor", `{"notes":"x"}`)
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, _ = f.call(h, task.ID, f.rdr, "reader", `{"notes":"x"}`)
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, _ = f.call(h, task.ID, f.owner, "contributor", `{}`)
	wantHTTPStatus(t, err, http.StatusBadRequest)
	err, rec = f.call(h, task.ID, f.mgr, "manager", `{"notes":"mgr note"}`)
	f.wantOK(t, err, rec)

	// A private task the caller cannot see looks absent, before ownership.
	priv := f.newTask(t, f.owner, true)
	err, _ = f.call(h, priv.ID, f.other, "contributor", `{"notes":"x"}`)
	wantHTTPStatus(t, err, http.StatusNotFound)
	err, rec = f.call(h, priv.ID, f.owner, "contributor", `{"notes":"mine"}`)
	f.wantOK(t, err, rec)
}

func TestTaskStatusHidesPrivateTaskFromOthers(t *testing.T) {
	f := newAssignmentFixture(t, "assign-task-private")
	h := f.s.handleUpdateTaskStatus
	priv := f.newTask(t, f.owner, true)
	err, _ := f.call(h, priv.ID, f.other, "contributor", `{"status":"in_progress"}`)
	wantHTTPStatus(t, err, http.StatusNotFound)
	pub := f.newTask(t, f.owner, false)
	err, _ = f.call(h, pub.ID, f.other, "contributor", `{"status":"in_progress"}`)
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, rec := f.call(h, priv.ID, f.owner, "contributor", `{"status":"in_progress"}`)
	f.wantOK(t, err, rec)
}

func TestAssignmentEmailCaseInsensitive(t *testing.T) {
	f := newAssignmentFixture(t, "assign-email-case")
	upper := strings.ToUpper(f.owner)
	task := f.newTask(t, f.owner, false)
	err, rec := f.call(f.s.handleUpdateTaskStatus, task.ID, upper, "contributor", `{"status":"in_progress"}`)
	f.wantOK(t, err, rec)
	ca := f.newCA(t, f.owner)
	err, rec = f.call(f.s.handleUpdateCorrectiveActionStatus, ca.ID, upper, "contributor", caStatusBody("implementation"))
	f.wantOK(t, err, rec)
}

// The assignee check runs on the locked row being written, so a corrective
// action reassigned away from the caller is refused on both narrow routes.
func TestReassignedCorrectiveActionRefusesFormerAssignee(t *testing.T) {
	f := newAssignmentFixture(t, "assign-ca-reassign")
	ctx := context.Background()
	ca := f.newCA(t, f.owner)
	err, rec := f.call(f.s.handleUpdateCorrectiveAction, ca.ID, f.mgr, "manager", fmt.Sprintf(`{"assignee":%q}`, f.other))
	f.wantOK(t, err, rec)
	before, _ := f.s.db.GetCorrectiveAction(ctx, f.orgID, ca.ID)

	err, _ = f.call(f.s.handleUpdateCorrectiveActionProgress, ca.ID, f.owner, "contributor", `{"notes":"late"}`)
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, _ = f.call(f.s.handleUpdateCorrectiveActionStatus, ca.ID, f.owner, "contributor", caStatusBody("implementation"))
	wantHTTPStatus(t, err, http.StatusForbidden)
	got, _ := f.s.db.GetCorrectiveAction(ctx, f.orgID, ca.ID)
	if got.Assignee != f.other || got.Notes != before.Notes || got.Status != before.Status || got.RootCause != before.RootCause {
		t.Fatalf("row changed: assignee %q notes %q status %q", got.Assignee, got.Notes, got.Status)
	}
}

// A narrow write must leave every field it was not given exactly as the
// manager last saved it.
func TestProgressKeepsFieldsItWasNotGiven(t *testing.T) {
	f := newAssignmentFixture(t, "assign-ca-keep")
	ctx := context.Background()
	ca := f.newCA(t, f.owner)
	err, rec := f.call(f.s.handleUpdateCorrectiveAction, ca.ID, f.mgr, "manager", `{"title":"Renamed by manager","severity":"major_nc"}`)
	f.wantOK(t, err, rec)
	err, rec = f.call(f.s.handleUpdateCorrectiveActionProgress, ca.ID, f.owner, "contributor", `{"notes":"owner note"}`)
	f.wantOK(t, err, rec)
	got, _ := f.s.db.GetCorrectiveAction(ctx, f.orgID, ca.ID)
	if got.Title != "Renamed by manager" || got.Severity != "major_nc" || got.Notes != "owner note" || got.Assignee != f.owner {
		t.Fatalf("stored = title %q severity %q notes %q assignee %q", got.Title, got.Severity, got.Notes, got.Assignee)
	}
}

// A private task assigned to a contributor stays visible and writable for them
// when the session email arrives in a different case.
func TestMixedCaseEmailOnPrivateTask(t *testing.T) {
	f := newAssignmentFixture(t, "assign-private-case")
	upper := strings.ToUpper(f.owner)
	task := f.newTask(t, f.owner, true)
	err, rec := f.call(f.s.handleUpdateTaskNotes, task.ID, upper, "contributor", `{"notes":"mixed"}`)
	f.wantOK(t, err, rec)
	err, rec = f.call(f.s.handleUpdateTaskStatus, task.ID, upper, "contributor", `{"status":"in_progress"}`)
	f.wantOK(t, err, rec)
	err, rec = f.call(f.s.handleGetTask, task.ID, upper, "contributor", ``)
	f.wantOK(t, err, rec)
}
