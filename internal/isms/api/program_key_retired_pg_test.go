package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"isms.sh/internal/isms/db"
)

// Regression for #367: a deleted program's key stayed reusable, but objective
// display ids are <key>-<seq> and UNIQUE(organization_id, display_id) counts
// deleted rows, so the replacement program's first objective always collided.
// A key is retired once a deleted program with that key has any objective row.
//
// Requires a migrated Postgres; skipped otherwise.

func createProgramVia(t *testing.T, s *Server, orgID int, key string) (int, error, string) {
	t.Helper()
	body := `{"key":"` + key + `","title":"Program ` + key + `"}`
	c, rec := programDeleteCtx(orgID, http.MethodPost, "/api/v1/programs", "", body)
	err := s.handleCreateProgram(c)
	return rec.Code, err, rec.Body.String()
}

func deleteProgramVia(t *testing.T, s *Server, orgID int, id int64) {
	t.Helper()
	sid := strconv.FormatInt(id, 10)
	c, _ := programDeleteCtx(orgID, http.MethodDelete, "/api/v1/programs/"+sid, sid, "")
	if err := s.handleDeleteProgram(c); err != nil {
		t.Fatalf("delete program %d: %v", id, err)
	}
}

func wantKeyRetired(t *testing.T, err error, key string) {
	t.Helper()
	he := wantHTTPStatus(t, err, http.StatusConflict)
	eb, ok := he.Message.(errorBody)
	if !ok || eb.Code != CodeProgramKeyRetired {
		t.Fatalf("expected %s, got %#v", CodeProgramKeyRetired, he.Message)
	}
	if eb.Params[ParamRef] != key {
		t.Errorf("params = %#v, want ref=%q", eb.Params, key)
	}
}

func liveProgramCount(t *testing.T, s *Server, orgID int, key string) int {
	t.Helper()
	ps, err := s.db.ListPrograms(context.Background(), orgID)
	if err != nil {
		t.Fatalf("ListPrograms: %v", err)
	}
	n := 0
	for _, p := range ps {
		if p.Key == key {
			n++
		}
	}
	return n
}

func TestProgramKeyRetiredAfterDeleteWithObjectives(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "program-key-retired")
	contractTestUser(t, s, orgID, programDeleteActor, "admin")

	p := &db.Program{Key: "RKA", Title: "Retires its key"}
	if err := s.db.CreateProgram(ctx, orgID, p); err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	newDeleteTestObjective(t, s, orgID, p.ID, "keeps the key retired")
	deleteProgramVia(t, s, orgID, p.ID)

	t.Run("create over HTTP is refused, also after normalising the key", func(t *testing.T) {
		for _, key := range []string{"RKA", " rka "} {
			_, err, _ := createProgramVia(t, s, orgID, key)
			if err == nil {
				t.Fatalf("create with key %q succeeded, want 409", key)
			}
			wantKeyRetired(t, err, "RKA")
		}
		if n := liveProgramCount(t, s, orgID, "RKA"); n != 0 {
			t.Errorf("%d live programs with the retired key, want 0", n)
		}
	})

	t.Run("create by suggestion is refused and creates nothing", func(t *testing.T) {
		sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
			EntityType:     "program",
			SuggestionType: "create",
			Payload:        json.RawMessage(`{"key":"rka","title":"By suggestion"}`),
		})
		contractTestUser(t, s, orgID, suggestionChangelogActor, "manager")
		c, _ := suggestionCtx(orgID, http.MethodPost, sg.ID)
		err := s.handleApplyEntitySuggestion(c)
		if err == nil {
			t.Fatal("apply succeeded, want 409")
		}
		wantKeyRetired(t, err, "RKA")
		if n := liveProgramCount(t, s, orgID, "RKA"); n != 0 {
			t.Errorf("%d live programs with the retired key, want 0", n)
		}
	})
}

func TestProgramKeyRetiredWhenObjectiveDeletedFirst(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "program-key-retired-obj")
	contractTestUser(t, s, orgID, programDeleteActor, "admin")

	p := &db.Program{Key: "RKB", Title: "Objective deleted first"}
	if err := s.db.CreateProgram(ctx, orgID, p); err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	o := newDeleteTestObjective(t, s, orgID, p.ID, "deleted on its own")
	oid := strconv.FormatInt(o.ID, 10)
	c, _ := programDeleteCtx(orgID, http.MethodDelete, "/api/v1/objectives/"+oid, oid, "")
	if err := s.handleDeleteObjective(c); err != nil {
		t.Fatalf("delete objective: %v", err)
	}
	deleteProgramVia(t, s, orgID, p.ID)

	_, err, _ := createProgramVia(t, s, orgID, "RKB")
	if err == nil {
		t.Fatal("create succeeded, want 409")
	}
	wantKeyRetired(t, err, "RKB")
}

func TestProgramKeyFreeWhenDeletedProgramHadNoObjectives(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "program-key-free")
	contractTestUser(t, s, orgID, programDeleteActor, "admin")

	p := &db.Program{Key: "FK0", Title: "Never had objectives"}
	if err := s.db.CreateProgram(ctx, orgID, p); err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	deleteProgramVia(t, s, orgID, p.ID)

	code, err, body := createProgramVia(t, s, orgID, "FK0")
	if err != nil || code != http.StatusCreated {
		t.Fatalf("recreate: err %v, status %d, body %s; want 201", err, code, body)
	}
	var again db.Program
	if err := json.Unmarshal([]byte(body), &again); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	o := newDeleteTestObjective(t, s, orgID, again.ID, "first")
	if o.DisplayID != "FK0-1" {
		t.Errorf("first objective = %q, want FK0-1", o.DisplayID)
	}
}

func TestProgramKeyHeldByLiveProgramIsRefusedWithItsOwnCode(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "program-key-live")
	contractTestUser(t, s, orgID, programDeleteActor, "admin")

	p := &db.Program{Key: "LVK", Title: "Live"}
	if err := s.db.CreateProgram(ctx, orgID, p); err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	wantTaken := func(t *testing.T, err error) {
		t.Helper()
		he := wantHTTPStatus(t, err, http.StatusConflict)
		eb, ok := he.Message.(errorBody)
		if !ok || eb.Code != CodeProgramKeyTaken {
			t.Fatalf("expected %s, got %#v", CodeProgramKeyTaken, he.Message)
		}
		if eb.Params[ParamRef] != "LVK" {
			t.Errorf("params = %#v, want ref=LVK", eb.Params)
		}
		if want := "a program with the key LVK already exists"; eb.Message != want {
			t.Errorf("message = %q, want %q", eb.Message, want)
		}
	}

	t.Run("HTTP", func(t *testing.T) {
		_, err, _ := createProgramVia(t, s, orgID, "lvk")
		wantTaken(t, err)
	})
	t.Run("suggestion", func(t *testing.T) {
		sg := newTestSuggestion(t, s, orgID, &db.Suggestion{
			EntityType:     "program",
			SuggestionType: "create",
			Payload:        json.RawMessage(`{"key":"LVK","title":"By suggestion"}`),
		})
		contractTestUser(t, s, orgID, suggestionChangelogActor, "manager")
		c, _ := suggestionCtx(orgID, http.MethodPost, sg.ID)
		wantTaken(t, s.handleApplyEntitySuggestion(c))
	})
}
