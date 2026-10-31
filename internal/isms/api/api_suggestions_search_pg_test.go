package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"isms.sh/internal/isms/db"
)

// Regression for #500: a record created or changed by applying a suggestion was
// missing from search (Cmd/Ctrl+K, the Links picker, # mentions) until the org's
// index was next rebuilt. The HTTP create/update handlers patch the index
// directly; the suggestion apply handlers never did. The index here is built
// BEFORE the applies and stays fresh (searchIndexTTL), so the only way a record
// becomes findable is an apply-time upsert — the same condition the bug shows
// under in production.
//
// Requires a migrated Postgres; skipped otherwise, same as
// change_followup_task_pg_test.go:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...

// searchFinds asserts exactly one entry of the given type matches q and returns it.
func searchFinds(t *testing.T, s *Server, orgID int, entryType, q string) SearchEntry {
	t.Helper()
	got := s.searchIndex.Search(orgID, q, func() { s.buildSearchIndex(orgID) })
	if len(got) != 1 {
		t.Fatalf("search %q after apply: %d results (%v), want exactly 1 %s", q, len(got), got, entryType)
	}
	if got[0].Type != entryType {
		t.Fatalf("search %q: type %q, want %s", q, got[0].Type, entryType)
	}
	return got[0]
}

// searchFindsNothing asserts nothing matches q.
func searchFindsNothing(t *testing.T, s *Server, orgID int, q string) {
	t.Helper()
	got := s.searchIndex.Search(orgID, q, func() { s.buildSearchIndex(orgID) })
	if len(got) != 0 {
		t.Fatalf("search %q: %d results (%v), want none", q, len(got), got)
	}
}

func TestSuggestionApplyUpdatesSearchIndex(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "suggestion-search")

	// A record created the normal way, so the update case has an indexed target.
	ctrl := newTestRisk(t, s, orgID, "Quartz leaks secrets")
	// Fresh org: one build so the index exists and is fresh — as if a user had
	// just searched before the suggestion arrived.
	s.buildSearchIndex(orgID)

	// Create through a suggestion: findable by title and identifier right away.
	sgCreate := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:     "risk",
		SuggestionType: "create",
		Payload: json.RawMessage(`{"title":"Zephyr ambush","risk_type":"` + db.RiskTypes[0] +
			`","origin":"` + db.RiskOrigins[0] + `","status":"` + db.RiskStatuses[0] +
			`","treatment":"mitigate","current_likelihood":2,"current_impact":3}`),
	})
	applySuggestion(t, s, orgID, sgCreate.ID)
	entry := searchFinds(t, s, orgID, "risk", "zephyr ambush")
	if !strings.HasPrefix(entry.ID, "RISK-") {
		t.Errorf("entry.ID = %q, want the RISK-n identifier", entry.ID)
	}

	// Rename through an update suggestion: the new title finds the record, the
	// old one no longer does.
	sgRename := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:     "risk",
		EntityID:       ctrl.Identifier,
		SuggestionType: "update",
		Payload:        json.RawMessage(`{"fields":{"title":"Obsidian exfiltration"}}`),
	})
	applySuggestion(t, s, orgID, sgRename.ID)
	searchFinds(t, s, orgID, "risk", "obsidian exfiltration")
	searchFindsNothing(t, s, orgID, "quartz leaks secrets")

	// A second type with its own builder: supplier create via suggestion.
	sgSupplier := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:     "supplier",
		SuggestionType: "create",
		Payload:        json.RawMessage(`{"name":"Halcyon cloud"}`),
	})
	applySuggestion(t, s, orgID, sgSupplier.ID)
	searchFinds(t, s, orgID, "supplier", "halcyon cloud")
}

func TestSuggestionApplyTaskSearchVisibility(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "suggestion-search-task")
	// tasks.assignee_id is NOT NULL and defaults to the applier, so the acting
	// manager must exist in this org.
	contractTestUser(t, s, orgID, suggestionChangelogActor, "manager")
	s.buildSearchIndex(orgID)

	// A public task created through a suggestion is searchable straight away...
	sgPublic := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:     "task",
		SuggestionType: "create",
		Payload:        json.RawMessage(`{"title":"Publish menace roster"}`),
	})
	applySuggestion(t, s, orgID, sgPublic.ID)
	searchFinds(t, s, orgID, "task", "publish menace roster")

	// ...while a private task never enters the shared index (#178).
	sgPrivate := newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:     "task",
		SuggestionType: "create",
		Payload:        json.RawMessage(`{"title":"Classified inventory","private":true}`),
	})
	applySuggestion(t, s, orgID, sgPrivate.ID)
	searchFindsNothing(t, s, orgID, "classified inventory")
}

func applySuggestionID(t *testing.T, s *Server, orgID int, sg *db.Suggestion) string {
	t.Helper()
	sg = newTestSuggestion(t, s, orgID, sg)
	c, rec := suggestionCtx(orgID, http.MethodPost, sg.ID)
	if err := s.handleApplyEntitySuggestion(c); err != nil {
		t.Fatalf("apply %s:%s: %v", sg.EntityType, sg.SuggestionType, err)
	}
	var out struct {
		Status string `json:"status"`
		ID     string `json:"applied_entity_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Status != "applied" {
		t.Fatalf("apply %s:%s: %d %s", sg.EntityType, sg.SuggestionType, rec.Code, rec.Body.String())
	}
	return out.ID
}

func searchIndexEntry(s *Server, orgID int, typ, id string) (SearchEntry, bool) {
	s.searchIndex.mu.RLock()
	defer s.searchIndex.mu.RUnlock()
	for _, e := range s.searchIndex.entries[orgID] {
		if e.Type == typ && e.ID == id {
			return e, true
		}
	}
	return SearchEntry{}, false
}

func TestSuggestionApplySearchParityAllTypes(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "suggestion-search-all")
	contractTestUser(t, s, orgID, suggestionChangelogActor, "manager")

	ap := &db.AuditProgramme{Title: "search programme", Year: 2026, Status: "draft", CreatedBy: suggestionChangelogActor}
	if err := s.db.CreateAuditProgramme(ctx, orgID, ap); err != nil {
		t.Fatalf("CreateAuditProgramme: %v", err)
	}
	audit := &db.Audit{ProgrammeID: &ap.ID, Title: "search audit", AuditType: "internal", Status: "planned", Auditor: suggestionChangelogActor}
	if err := s.db.CreateAudit(ctx, orgID, audit); err != nil {
		t.Fatalf("CreateAudit: %v", err)
	}

	s.buildSearchIndex(orgID)

	type tc struct {
		entity, typ, create, renameField string
	}
	cases := []tc{
		{"risk", "risk", `{"title":"Alpha risk","description":"alpha body","risk_type":"` + db.RiskTypes[0] + `","origin":"` + db.RiskOrigins[0] + `","status":"` + db.RiskStatuses[0] + `","treatment":"mitigate","current_likelihood":2,"current_impact":3}`, "title"},
		{"incident", "incident", `{"title":"Alpha incident","description":"alpha body"}`, "title"},
		{"supplier", "supplier", `{"name":"Alpha supplier","notes":"alpha body"}`, "name"},
		{"legal_requirement", "legal", `{"title":"Alpha legal","description":"alpha body"}`, "title"},
		{"change_request", "change", `{"title":"Alpha change","description":"alpha body"}`, "title"},
		{"corrective_action", "corrective_action", `{"title":"Alpha ca","description":"alpha body"}`, "title"},
		{"task", "task", `{"title":"Alpha task","description":"alpha body"}`, "title"},
		{"program", "program", `{"key":"srch","title":"Alpha program","description":"alpha body"}`, ""},
		{"objective", "objective", `{"title":"Alpha objective","description":"alpha body","program_key":"SRCH"}`, "title"},
		{"system", "system", `{"name":"Alpha system","description":"alpha body"}`, "name"},
		{"asset", "asset", `{"name":"Alpha asset","description":"alpha body"}`, "name"},
		{"audit_finding", "audit_finding", fmt.Sprintf(`{"audit_id":%d,"title":"Alpha finding","description":"alpha body"}`, audit.ID), "title"},
	}

	upserted := map[string]SearchEntry{}
	for _, c := range cases {
		t.Run(c.entity, func(t *testing.T) {
			id := applySuggestionID(t, s, orgID, &db.Suggestion{EntityType: c.entity, SuggestionType: "create", Payload: json.RawMessage(c.create)})
			e, ok := searchIndexEntry(s, orgID, c.typ, id)
			if !ok {
				t.Fatalf("after create: no %s entry with id %q", c.typ, id)
			}
			if c.renameField != "" {
				newName := "Omega " + strings.ReplaceAll(c.entity, "_", " ")
				applySuggestionID(t, s, orgID, &db.Suggestion{EntityType: c.entity, EntityID: id, SuggestionType: "update",
					Payload: json.RawMessage(fmt.Sprintf(`{"fields":{%q:%q}}`, c.renameField, newName))})
				e, ok = searchIndexEntry(s, orgID, c.typ, id)
				if !ok || e.Title != newName {
					t.Fatalf("after rename: entry %+v ok=%v, want title %q", e, ok, newName)
				}
			}
			upserted[c.typ+"|"+id] = e
		})
	}

	// Rebuild from the database and compare entry by entry.
	s.buildSearchIndex(orgID)
	for k, want := range upserted {
		typ, id, _ := strings.Cut(k, "|")
		got, ok := searchIndexEntry(s, orgID, typ, id)
		if !ok {
			t.Errorf("%s: missing after rebuild", k)
			continue
		}
		g := fmt.Sprintf("%s | %s | %s | %q", got.Type, got.ID, got.Title, got.Search)
		w := fmt.Sprintf("%s | %s | %s | %q", want.Type, want.ID, want.Title, want.Search)
		if g != w {
			t.Errorf("%s: rebuild and apply-upsert disagree\n  rebuild: %s\n  upsert:  %s", k, g, w)
		}
	}
}

// Task visibility flips through update suggestions: private -> public enters the
// index, public -> private leaves it, a rename while private stays out.
func TestSuggestionApplyTaskPrivacyFlip(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "suggestion-search-flip")
	contractTestUser(t, s, orgID, suggestionChangelogActor, "manager")
	s.buildSearchIndex(orgID)

	state := func(id string) string {
		e, ok := searchIndexEntry(s, orgID, "task", id)
		if !ok {
			return "absent"
		}
		return "present:" + e.Title
	}
	upd := func(id, fields string) {
		applySuggestionID(t, s, orgID, &db.Suggestion{EntityType: "task", EntityID: id, SuggestionType: "update", Payload: json.RawMessage(`{"fields":` + fields + `}`)})
	}
	id := applySuggestionID(t, s, orgID, &db.Suggestion{EntityType: "task", SuggestionType: "create", Payload: json.RawMessage(`{"title":"Secret start","private":true}`)})
	var got []string
	got = append(got, "created-private="+state(id))
	upd(id, `{"title":"Secret renamed"}`)
	got = append(got, "renamed-private="+state(id))
	upd(id, `{"private":false}`)
	got = append(got, "made-public="+state(id))
	upd(id, `{"title":"Public renamed"}`)
	got = append(got, "renamed-public="+state(id))
	upd(id, `{"private":true}`)
	got = append(got, "made-private="+state(id))
	want := "created-private=absent | renamed-private=absent | made-public=present:Secret renamed | renamed-public=present:Public renamed | made-private=absent"
	if g := strings.Join(got, " | "); g != want {
		t.Fatalf("\n got: %s\nwant: %s", g, want)
	}
}
