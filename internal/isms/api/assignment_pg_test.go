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
// item, everyone else refused. #409 does the same for incidents.

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

// ---- Incidents (#409) ----

func (f *assignmentFixture) newIncident(t *testing.T, assignee string) *db.Incident {
	t.Helper()
	inc := &db.Incident{Title: "inc " + assignee, Reporter: f.mgr, Assignee: assignee}
	f.s.applyIncidentDefaults(context.Background(), f.orgID, inc, f.mgr)
	if err := f.s.db.CreateIncident(context.Background(), f.orgID, inc); err != nil {
		t.Fatalf("CreateIncident: %v", err)
	}
	return inc
}

func (f *assignmentFixture) incident(t *testing.T, id int64) *db.Incident {
	t.Helper()
	got, err := f.s.db.GetIncident(context.Background(), f.orgID, id)
	if err != nil || got == nil {
		t.Fatalf("GetIncident: %v", err)
	}
	return got
}

func TestAssigneeIncidentStatus(t *testing.T) {
	f := newAssignmentFixture(t, "assign-inc-status")
	inc := f.newIncident(t, f.owner)
	h := f.s.handleUpdateIncidentStatus

	for _, st := range []string{"investigating", "resolved", "closed"} {
		err, rec := f.call(h, inc.ID, f.owner, "contributor", caStatusBody(st))
		f.wantOK(t, err, rec)
	}
	got := f.incident(t, inc.ID)
	if got.Status != "closed" || got.ClosedAt == nil || got.ClosedAt.IsZero() {
		t.Fatalf("status/closed_at = %q/%v, want closed with closed_at set", got.Status, got.ClosedAt)
	}
	err, rec := f.call(h, inc.ID, f.owner, "contributor", caStatusBody("investigating"))
	f.wantOK(t, err, rec)
	got = f.incident(t, inc.ID)
	if got.Status != "investigating" || (got.ClosedAt != nil && !got.ClosedAt.IsZero()) {
		t.Fatalf("after reopen status/closed_at = %q/%v, want investigating with closed_at cleared", got.Status, got.ClosedAt)
	}

	inc2 := f.newIncident(t, f.owner)
	err, _ = f.call(h, inc2.ID, f.other, "contributor", caStatusBody("investigating"))
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, _ = f.call(h, inc2.ID, f.rdr, "reader", caStatusBody("investigating"))
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, rec = f.call(h, inc2.ID, f.mgr, "manager", caStatusBody("investigating"))
	f.wantOK(t, err, rec)
}

func TestAssigneeCannotResolveIncidentWithOpenLinkedCA(t *testing.T) {
	f := newAssignmentFixture(t, "assign-inc-open-ca")
	inc := f.newIncident(t, f.owner)
	ca := f.newCA(t, f.owner)
	ref := &db.EntityReference{SourceType: "corrective_action", SourceID: ca.Identifier,
		TargetType: "incident", TargetID: inc.Identifier, CreatedBy: f.mgr}
	if err := f.s.db.CreateReference(context.Background(), f.orgID, ref); err != nil {
		t.Fatalf("CreateReference: %v", err)
	}
	h := f.s.handleUpdateIncidentStatus
	err, _ := f.call(h, inc.ID, f.owner, "contributor", caStatusBody("resolved"))
	wantHTTPStatus(t, err, http.StatusConflict)
	err, _ = f.call(h, inc.ID, f.owner, "contributor", caStatusBody("closed"))
	wantHTTPStatus(t, err, http.StatusConflict)
	err, _ = f.call(h, inc.ID, f.mgr, "manager", caStatusBody("resolved"))
	wantHTTPStatus(t, err, http.StatusConflict)
}

func TestAssigneeIncidentProgress(t *testing.T) {
	f := newAssignmentFixture(t, "assign-inc-progress")
	ctx := context.Background()
	inc := f.newIncident(t, f.owner)
	h := f.s.handleUpdateIncidentProgress

	err, rec := f.call(h, inc.ID, f.owner, "contributor", `{"root_cause":"stale credential","lessons_learned":"rotate keys","notes":"rotated"}`)
	f.wantOK(t, err, rec)
	var body db.Incident
	if jerr := json.Unmarshal(rec.Body.Bytes(), &body); jerr != nil {
		t.Fatalf("decoding response: %v", jerr)
	}
	if body.RootCause != "stale credential" || body.LessonsLearned != "rotate keys" || body.Notes != "rotated" || body.Status != inc.Status {
		t.Fatalf("response = %+v", body)
	}
	got := f.incident(t, inc.ID)
	if got.RootCause != "stale credential" || got.LessonsLearned != "rotate keys" || got.Notes != "rotated" || got.Status != inc.Status {
		t.Fatalf("stored = root_cause %q lessons %q notes %q status %q", got.RootCause, got.LessonsLearned, got.Notes, got.Status)
	}
	log, err := f.s.db.ListEntityChangelog(ctx, f.orgID, "incident", inc.ID)
	if err != nil {
		t.Fatalf("ListEntityChangelog: %v", err)
	}
	fields := map[string]string{}
	for _, e := range log {
		fields[e.Field] = e.ChangedBy
	}
	for _, field := range []string{"root_cause", "lessons_learned", "notes"} {
		if fields[field] != f.owner {
			t.Errorf("changelog for %s by %q, want %s (all: %v)", field, fields[field], f.owner, fields)
		}
	}

	err, rec = f.call(h, inc.ID, f.owner, "contributor", `{"notes":"second note"}`)
	f.wantOK(t, err, rec)
	got = f.incident(t, inc.ID)
	if got.RootCause != "stale credential" || got.LessonsLearned != "rotate keys" || got.Notes != "second note" {
		t.Fatalf("after notes-only: root_cause %q lessons %q notes %q", got.RootCause, got.LessonsLearned, got.Notes)
	}

	err, _ = f.call(h, inc.ID, f.owner, "contributor", `{}`)
	wantHTTPStatus(t, err, http.StatusBadRequest)
	err, _ = f.call(h, inc.ID, f.other, "contributor", `{"notes":"x"}`)
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, _ = f.call(h, inc.ID, f.rdr, "reader", `{"notes":"x"}`)
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, rec = f.call(h, inc.ID, f.mgr, "manager", `{"notes":"mgr note"}`)
	f.wantOK(t, err, rec)
}

// D3: once closed, the assignee can't rewrite the findings until they reopen it.
func TestClosedIncidentIsReadOnlyForAssignee(t *testing.T) {
	f := newAssignmentFixture(t, "assign-inc-closed")
	inc := f.newIncident(t, f.owner)
	err, rec := f.call(f.s.handleUpdateIncidentStatus, inc.ID, f.mgr, "manager", caStatusBody("closed"))
	f.wantOK(t, err, rec)
	before := f.incident(t, inc.ID)

	err, _ = f.call(f.s.handleUpdateIncidentProgress, inc.ID, f.owner, "contributor", `{"notes":"late"}`)
	wantHTTPStatus(t, err, http.StatusConflict)
	err, _ = f.call(f.s.handleUpdateIncidentStatus, inc.ID, f.owner, "contributor", `{"status":"closed","lessons_learned":"x"}`)
	wantHTTPStatus(t, err, http.StatusConflict)
	got := f.incident(t, inc.ID)
	if got.Notes != before.Notes || got.LessonsLearned != before.LessonsLearned || got.Status != "closed" {
		t.Fatalf("row changed: notes %q lessons %q status %q", got.Notes, got.LessonsLearned, got.Status)
	}

	err, rec = f.call(f.s.handleUpdateIncidentProgress, inc.ID, f.mgr, "manager", `{"notes":"mgr"}`)
	f.wantOK(t, err, rec)
	err, rec = f.call(f.s.handleUpdateIncidentStatus, inc.ID, f.owner, "contributor", caStatusBody("investigating"))
	f.wantOK(t, err, rec)
	err, rec = f.call(f.s.handleUpdateIncidentProgress, inc.ID, f.owner, "contributor", `{"notes":"after reopen"}`)
	f.wantOK(t, err, rec)
	if got := f.incident(t, inc.ID); got.Notes != "after reopen" {
		t.Fatalf("notes = %q, want %q", got.Notes, "after reopen")
	}
}

func TestReassignedIncidentRefusesFormerAssignee(t *testing.T) {
	f := newAssignmentFixture(t, "assign-inc-reassign")
	inc := f.newIncident(t, f.owner)
	err, rec := f.call(f.s.handleUpdateIncident, inc.ID, f.mgr, "manager", fmt.Sprintf(`{"assignee":%q}`, f.other))
	f.wantOK(t, err, rec)
	before := f.incident(t, inc.ID)

	err, _ = f.call(f.s.handleUpdateIncidentProgress, inc.ID, f.owner, "contributor", `{"notes":"late"}`)
	wantHTTPStatus(t, err, http.StatusForbidden)
	err, _ = f.call(f.s.handleUpdateIncidentStatus, inc.ID, f.owner, "contributor", caStatusBody("investigating"))
	wantHTTPStatus(t, err, http.StatusForbidden)
	got := f.incident(t, inc.ID)
	if got.Assignee != f.other || got.Status != before.Status || got.Notes != before.Notes || got.RootCause != before.RootCause {
		t.Fatalf("row changed: assignee %q status %q notes %q", got.Assignee, got.Status, got.Notes)
	}
}

// A narrow write must leave every field it was not given as the manager last
// saved it, the regulatory notification fields included.
func TestIncidentProgressKeepsFieldsItWasNotGiven(t *testing.T) {
	f := newAssignmentFixture(t, "assign-inc-keep")
	inc := f.newIncident(t, f.owner)
	err, rec := f.call(f.s.handleUpdateIncident, inc.ID, f.mgr, "manager", `{"title":"Renamed by manager","severity":"high","authority_notified":"pending"}`)
	f.wantOK(t, err, rec)
	before := f.incident(t, inc.ID)
	err, rec = f.call(f.s.handleUpdateIncidentProgress, inc.ID, f.owner, "contributor", `{"root_cause":"x"}`)
	f.wantOK(t, err, rec)
	got := f.incident(t, inc.ID)
	if got.Title != "Renamed by manager" || got.Severity != "high" || got.AuthorityNotified != "pending" ||
		got.Assignee != f.owner || got.Status != before.Status || got.RootCause != "x" {
		t.Fatalf("stored = title %q severity %q authority %q assignee %q status %q root_cause %q",
			got.Title, got.Severity, got.AuthorityNotified, got.Assignee, got.Status, got.RootCause)
	}
}

func TestIncidentAssignmentEmailCaseInsensitive(t *testing.T) {
	f := newAssignmentFixture(t, "assign-inc-case")
	inc := f.newIncident(t, f.owner)
	err, rec := f.call(f.s.handleUpdateIncidentStatus, inc.ID, strings.ToUpper(f.owner), "contributor", caStatusBody("investigating"))
	f.wantOK(t, err, rec)
}
