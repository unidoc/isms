package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Regression for #437: the audit programme create, update and delete handlers
// only wrote the activity feed, so a programme's History tab was always empty
// (and entity_changelog's entity_type CHECK refused 'audit_programme' anyway).
//
// Requires a migrated Postgres; skipped otherwise.

const programmeChangelogActor = "admin@programme-changelog.test"

func programmeCtx(orgID int, method, id, query, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/api/v1/audit/programmes/"+id+query, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if id != "" {
		c.SetParamNames("id")
		c.SetParamValues(id)
	}
	c.Set("org_id", orgID)
	c.Set("user_role", "admin")
	c.Set("user_email", programmeChangelogActor)
	return c, rec
}

func programmeHistory(t *testing.T, s *Server, orgID, id int) []db.ChangelogEntry {
	t.Helper()
	rows, err := s.db.ListEntityChangelog(context.Background(), orgID, "audit_programme", int64(id))
	if err != nil {
		t.Fatalf("ListEntityChangelog: %v", err)
	}
	return rows
}

func TestAuditProgrammeWritesChangelog(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "programme-changelog")

	c, rec := programmeCtx(orgID, http.MethodPost, "", "", `{"title":"Programme before","year":2026,"status":"draft"}`)
	if err := s.handleCreateAuditProgramme(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body %s", rec.Code, rec.Body.String())
	}
	var created db.AuditProgramme
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decoding create response %q: %v", rec.Body.String(), err)
	}
	id := created.ID
	sid := strconv.Itoa(id)

	rows := programmeHistory(t, s, orgID, id)
	if len(rows) != 1 || rows[0].Action != "create" {
		t.Fatalf("after create: got %d rows %+v, want exactly one create row", len(rows), rows)
	}
	if rows[0].ChangedBy != programmeChangelogActor {
		t.Errorf("create changed_by = %q, want %q", rows[0].ChangedBy, programmeChangelogActor)
	}

	t.Run("update logs one row per changed field", func(t *testing.T) {
		c, rec := programmeCtx(orgID, http.MethodPut, sid, "?reason=test-437", `{"title":"Programme after","status":"active"}`)
		if err := s.handleUpdateAuditProgramme(c); err != nil {
			t.Fatalf("update: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
		}
		got := map[string]db.ChangelogEntry{}
		for _, r := range programmeHistory(t, s, orgID, id) {
			if r.Action == "update" {
				got[r.Field] = r
			}
		}
		if len(got) != 2 {
			t.Fatalf("got update rows for %d fields (%v), want 2 (title, status)", len(got), got)
		}
		for field, want := range map[string][2]string{
			"title":  {"Programme before", "Programme after"},
			"status": {"draft", "active"},
		} {
			r, ok := got[field]
			if !ok {
				t.Errorf("no update row for field %q", field)
				continue
			}
			if r.OldValue == nil || *r.OldValue != want[0] {
				t.Errorf("%s old_value = %v, want %q", field, r.OldValue, want[0])
			}
			if r.NewValue == nil || *r.NewValue != want[1] {
				t.Errorf("%s new_value = %v, want %q", field, r.NewValue, want[1])
			}
			if r.Reason != "test-437" {
				t.Errorf("%s reason = %q, want %q", field, r.Reason, "test-437")
			}
			if r.ChangedBy != programmeChangelogActor {
				t.Errorf("%s changed_by = %q, want %q", field, r.ChangedBy, programmeChangelogActor)
			}
		}
	})

	t.Run("update with identical values logs nothing", func(t *testing.T) {
		before := len(programmeHistory(t, s, orgID, id))
		c, rec := programmeCtx(orgID, http.MethodPut, sid, "", `{"title":"Programme after","status":"active"}`)
		if err := s.handleUpdateAuditProgramme(c); err != nil {
			t.Fatalf("update: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
		}
		if after := len(programmeHistory(t, s, orgID, id)); after != before {
			t.Errorf("history rows = %d after a no-op update, want %d", after, before)
		}
	})

	t.Run("update and delete of a missing id answer 404 and log nothing", func(t *testing.T) {
		missing := id + 1000000
		c, _ := programmeCtx(orgID, http.MethodPut, strconv.Itoa(missing), "", `{"title":"ghost"}`)
		wantHTTPStatus(t, s.handleUpdateAuditProgramme(c), http.StatusNotFound)
		c, _ = programmeCtx(orgID, http.MethodDelete, strconv.Itoa(missing), "", "")
		wantHTTPStatus(t, s.handleDeleteAuditProgramme(c), http.StatusNotFound)
		if rows := programmeHistory(t, s, orgID, missing); len(rows) != 0 {
			t.Errorf("missing programme has %d history rows, want 0: %+v", len(rows), rows)
		}
	})

	t.Run("delete refused while audits exist logs nothing", func(t *testing.T) {
		withAudit := newRefTestProgramme(t, s, orgID, "programme with audit")
		a := &db.Audit{Title: "audit in programme", Status: "planned", ProgrammeID: &withAudit.ID}
		if err := s.db.CreateAudit(context.Background(), orgID, a); err != nil {
			t.Fatalf("CreateAudit: %v", err)
		}
		c, _ := programmeCtx(orgID, http.MethodDelete, strconv.Itoa(withAudit.ID), "", "")
		wantHTTPStatus(t, s.handleDeleteAuditProgramme(c), http.StatusConflict)
		if rows := programmeHistory(t, s, orgID, withAudit.ID); len(rows) != 0 {
			t.Errorf("refused delete left %d history rows: %+v", len(rows), rows)
		}
	})

	t.Run("delete of an empty programme logs a delete row", func(t *testing.T) {
		c, rec := programmeCtx(orgID, http.MethodDelete, sid, "", "")
		if err := s.handleDeleteAuditProgramme(c); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
		}
		var deletes int
		for _, r := range programmeHistory(t, s, orgID, id) {
			if r.Action == "delete" {
				deletes++
			}
		}
		if deletes != 1 {
			t.Errorf("got %d delete rows, want 1", deletes)
		}
	})
}
