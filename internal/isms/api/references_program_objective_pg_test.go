package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"isms.sh/internal/isms/db"
)

// Regression for #350: programs and objectives can be addressed either by row
// id or by key / display id, and every write path must store the key /
// display id regardless of which form it was sent, so the two spellings
// never create two links to the same row.
//
// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays
// green without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...

func newRefTestProgram(t *testing.T, s *Server, orgID int, key, title string) *db.Program {
	t.Helper()
	p := &db.Program{Key: key, Title: title}
	if err := s.db.CreateProgram(context.Background(), orgID, p); err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	return p
}

func newRefTestObjective(t *testing.T, s *Server, orgID int, programID int64, title string) *db.Objective {
	t.Helper()
	o := &db.Objective{ProgramID: programID, Title: title}
	if err := s.db.CreateObjective(context.Background(), orgID, o); err != nil {
		t.Fatalf("CreateObjective: %v", err)
	}
	return o
}

func newRefTestRisk(t *testing.T, s *Server, orgID int, title string) *db.Risk {
	t.Helper()
	r := &db.Risk{Title: title, RiskType: db.RiskTypes[0], Origin: db.RiskOrigins[0], Status: db.RiskStatuses[0]}
	if err := s.db.CreateRisk(context.Background(), orgID, r); err != nil {
		t.Fatalf("CreateRisk: %v", err)
	}
	return r
}

func TestCreateReferenceCanonicalisesProgramObjective(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "refs-program-objective")

	risk := newRefTestRisk(t, s, orgID, "risk for program/objective reference test")
	program := newRefTestProgram(t, s, orgID, "REGCAN", "program for reference canonicalisation test")
	objective := newRefTestObjective(t, s, orgID, program.ID, "objective for reference canonicalisation test")

	post := func(sourceType, sourceID, targetType, targetID string) (map[string]any, error) {
		body, err := json.Marshal(map[string]string{
			"source_type": sourceType,
			"source_id":   sourceID,
			"target_type": targetType,
			"target_id":   targetID,
		})
		if err != nil {
			t.Fatalf("marshaling request body: %v", err)
		}
		c, rec := ctxForCreateReference(orgID, string(body))
		if err := s.handleCreateReference(c); err != nil {
			return nil, err
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding response body %q: %v", rec.Body.String(), err)
		}
		return out, nil
	}

	// risk -> program, first by row id, then by key: both must land on the
	// same pair, canonicalised to the key.
	programRowID := strconv.FormatInt(program.ID, 10)
	for _, targetID := range []string{programRowID, program.Key} {
		out, err := post("risk", risk.Identifier, "program", targetID)
		if err != nil {
			t.Fatalf("post(risk -> program %q): %v", targetID, err)
		}
		if got := out["target_id"]; got != program.Key {
			t.Errorf("post(risk -> program %q): response target_id = %v, want %q", targetID, got, program.Key)
		}
	}
	refs, err := s.db.ListAllReferencesForEntity(ctx, orgID, "program", program.Key)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity(program, %s): %v", program.Key, err)
	}
	if len(refs) != 2 {
		t.Fatalf("program %s has %d reference rows after both spellings, want 2 (forward + reverse, deduplicated)", program.Key, len(refs))
	}
	for _, r := range refs {
		if r.SourceType == "program" && r.SourceID != program.Key {
			t.Errorf("forward row: SourceID = %q, want %q", r.SourceID, program.Key)
		}
		if r.TargetType == "program" && r.TargetID != program.Key {
			t.Errorf("row targeting program: TargetID = %q, want %q", r.TargetID, program.Key)
		}
	}

	// risk -> objective, first by row id, then by display id.
	objectiveRowID := strconv.FormatInt(objective.ID, 10)
	for _, targetID := range []string{objectiveRowID, objective.DisplayID} {
		out, err := post("risk", risk.Identifier, "objective", targetID)
		if err != nil {
			t.Fatalf("post(risk -> objective %q): %v", targetID, err)
		}
		if got := out["target_id"]; got != objective.DisplayID {
			t.Errorf("post(risk -> objective %q): response target_id = %v, want %q", targetID, got, objective.DisplayID)
		}
	}
	refs, err = s.db.ListAllReferencesForEntity(ctx, orgID, "objective", objective.DisplayID)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity(objective, %s): %v", objective.DisplayID, err)
	}
	if len(refs) != 2 {
		t.Fatalf("objective %s has %d reference rows after both spellings, want 2 (forward + reverse, deduplicated)", objective.DisplayID, len(refs))
	}

	// A program as the source, addressed by its row id, must also be stored
	// as its key — a program or objective can be either side of a reference.
	newProgram := newRefTestProgram(t, s, orgID, "REGSRC", "program as reference source test")
	newProgramRowID := strconv.FormatInt(newProgram.ID, 10)
	out, err := post("program", newProgramRowID, "risk", risk.Identifier)
	if err != nil {
		t.Fatalf("post(program %q -> risk): %v", newProgramRowID, err)
	}
	if got := out["source_id"]; got != newProgram.Key {
		t.Errorf("post(program %q -> risk): response source_id = %v, want %q", newProgramRowID, got, newProgram.Key)
	}
	refs, err = s.db.ListAllReferencesForEntity(ctx, orgID, "program", newProgram.Key)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity(program, %s): %v", newProgram.Key, err)
	}
	if len(refs) != 2 {
		t.Fatalf("program %s has %d reference rows, want 2 (forward + reverse)", newProgram.Key, len(refs))
	}

	// A risk (a sequence-identifier type) target is stored unchanged: only
	// programs and objectives get a canonical rewrite.
	asset := &db.Asset{Name: "asset for unchanged-target test", AssetType: db.AssetTypes[0], Status: db.AssetStatuses[0]}
	if err := s.db.CreateAsset(ctx, orgID, asset); err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	out, err = post("asset", asset.Identifier, "risk", risk.Identifier)
	if err != nil {
		t.Fatalf("post(asset -> risk): %v", err)
	}
	if got := out["target_id"]; got != risk.Identifier {
		t.Errorf("post(asset -> risk): response target_id = %v, want %q (unchanged)", got, risk.Identifier)
	}
}

// TestCreateLegalCanonicalisesProgramReference confirms that references sent
// with a create request go through the same canonicalisation as POST
// /references directly: validateReferenceInputs is shared by all ten create
// handlers, so testing one (legal_requirement) covers the choke point.
func TestCreateLegalCanonicalisesProgramReference(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "refs-create-legal-program")

	program := newRefTestProgram(t, s, orgID, "REGLEG", "program for legal create-reference test")

	body, err := json.Marshal(map[string]any{
		"title":        "legal for program canonicalisation test",
		"jurisdiction": "EU",
		"category":     "privacy",
		"references":   []map[string]string{{"type": "program", "id": strconv.FormatInt(program.ID, 10)}},
	})
	if err != nil {
		t.Fatalf("marshaling request body: %v", err)
	}
	c, _ := ctxForPath(orgID, http.MethodPost, "/api/v1/legal", string(body), "admin")
	if err := s.handleCreateLegal(c); err != nil {
		t.Fatalf("handleCreateLegal: %v", err)
	}

	refs, err := s.db.ListAllReferencesForEntity(ctx, orgID, "program", program.Key)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity(program, %s): %v", program.Key, err)
	}
	if len(refs) != 2 {
		t.Fatalf("program %s has %d reference rows, want 2 (forward + reverse)", program.Key, len(refs))
	}
	for _, r := range refs {
		if r.SourceType == "program" && r.SourceID != program.Key {
			t.Errorf("forward row: SourceID = %q, want %q", r.SourceID, program.Key)
		}
		if r.TargetType == "program" && r.TargetID != program.Key {
			t.Errorf("row targeting program: TargetID = %q, want %q", r.TargetID, program.Key)
		}
	}
}

// TestHandleListReferencesCanonicalisesProgramID is the read-side regression:
// a caller asking for a program's references by row id (as MCP's
// get_entity_links and the pre-#350 program page do) must find rows stored
// under the program's key, and an id that never resolves still lists its
// literal rows rather than 500ing or returning nothing.
func TestHandleListReferencesCanonicalisesProgramID(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "refs-list-program")

	program := newRefTestProgram(t, s, orgID, "REGLIST", "program for list canonicalisation test")
	risk := newRefTestRisk(t, s, orgID, "risk for list canonicalisation test")

	fwd := &db.EntityReference{SourceType: "risk", SourceID: risk.Identifier, TargetType: "program", TargetID: program.Key, CreatedBy: "seed@refs-list.test"}
	if err := s.db.CreateReference(context.Background(), orgID, fwd); err != nil {
		t.Fatalf("seeding forward reference: %v", err)
	}
	rev := &db.EntityReference{SourceType: "program", SourceID: program.Key, TargetType: "risk", TargetID: risk.Identifier, CreatedBy: "seed@refs-list.test"}
	if err := s.db.CreateReference(context.Background(), orgID, rev); err != nil {
		t.Fatalf("seeding reverse reference: %v", err)
	}

	list := func(id string) []map[string]any {
		t.Helper()
		path := "/api/v1/references?type=program&id=" + id
		c, rec := ctxForPath(orgID, http.MethodGet, path, "", "admin")
		if err := s.handleListReferences(c); err != nil {
			t.Fatalf("handleListReferences(id=%s): %v", id, err)
		}
		var out struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding response body %q: %v", rec.Body.String(), err)
		}
		return out.Data
	}

	// The numeric row id must find the key-form rows.
	byRowID := list(strconv.FormatInt(program.ID, 10))
	if len(byRowID) != 1 {
		t.Fatalf("?id=%d: got %d rows, want 1", program.ID, len(byRowID))
	}

	// The key itself is unchanged behaviour.
	byKey := list(program.Key)
	if len(byKey) != 1 {
		t.Fatalf("?id=%s: got %d rows, want 1", program.Key, len(byKey))
	}

	// An id that resolves to nothing still returns its literal rows (a
	// soft-deleted entity, or a legacy raw-id row), rather than erroring.
	legacyProgramID := "999999"
	legacyRisk := newRefTestRisk(t, s, orgID, "risk for legacy literal-id test")
	legacyFwd := &db.EntityReference{SourceType: "risk", SourceID: legacyRisk.Identifier, TargetType: "program", TargetID: legacyProgramID, CreatedBy: "seed@refs-list.test"}
	if err := s.db.CreateReference(context.Background(), orgID, legacyFwd); err != nil {
		t.Fatalf("seeding legacy raw-id reference: %v", err)
	}
	byLegacyID := list(legacyProgramID)
	if len(byLegacyID) != 1 {
		t.Fatalf("?id=%s (unresolvable): got %d rows, want 1 (the literal-id row)", legacyProgramID, len(byLegacyID))
	}
}
