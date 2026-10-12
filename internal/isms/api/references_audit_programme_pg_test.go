package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"isms.sh/internal/isms/db"
)

// Regression for #394: an audit programme could be neither the source nor the
// target of a link. It resolves like audits and findings (#366): a bare row id
// or AP-<id>, always stored as AP-<id>.
//
// Requires a migrated Postgres; skipped otherwise.

func newRefTestProgramme(t *testing.T, s *Server, orgID int, title string) *db.AuditProgramme {
	t.Helper()
	p := &db.AuditProgramme{Title: title, Year: 2026, Status: "active", CreatedBy: "admin@refs-programme.test"}
	if err := s.db.CreateAuditProgramme(context.Background(), orgID, p); err != nil {
		t.Fatalf("CreateAuditProgramme: %v", err)
	}
	return p
}

func TestCreateReferenceCanonicalisesAuditProgramme(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "refs-programme")

	prog := newRefTestProgramme(t, s, orgID, "programme linked both ways")
	audit := newRefTestAudit(t, s, orgID, "audit linked to programme")
	risk := newRefTestRisk(t, s, orgID, "risk linked to programme")

	post := func(orgID int, sourceType, sourceID, targetType, targetID string) (map[string]any, error) {
		t.Helper()
		body, err := json.Marshal(map[string]string{
			"source_type": sourceType, "source_id": sourceID,
			"target_type": targetType, "target_id": targetID,
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
	mustPost := func(st, sid, tt, tid string) map[string]any {
		t.Helper()
		out, err := post(orgID, st, sid, tt, tid)
		if err != nil {
			t.Fatalf("post(%s %s -> %s %s): %v", st, sid, tt, tid, err)
		}
		return out
	}

	rowID := strconv.Itoa(prog.ID)
	progID := "AP-" + rowID

	// As target, bare id then prefixed: the same pair.
	if got := mustPost("risk", risk.Identifier, "audit_programme", rowID)["target_id"]; got != progID {
		t.Errorf("post(risk -> %s): target_id = %v, want %q", rowID, got, progID)
	}
	if got := mustPost("risk", risk.Identifier, "audit_programme", progID)["target_id"]; got != progID {
		t.Errorf("post(risk -> %s): target_id = %v, want %q", progID, got, progID)
	}
	// As source.
	if got := mustPost("audit_programme", rowID, "audit", strconv.Itoa(audit.ID))["source_id"]; got != progID {
		t.Errorf("post(%s -> audit): source_id = %v, want %q", rowID, got, progID)
	}

	refs, err := s.db.ListAllReferencesForEntity(ctx, orgID, "audit_programme", progID)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity: %v", err)
	}
	// risk pair (forward + reverse) + audit pair (forward + reverse) = 4 rows,
	// 2 of which have the programme on the listed side.
	if len(refs) != 4 {
		t.Fatalf("programme %s has %d reference rows, want 4 (two pairs, deduplicated)", progID, len(refs))
	}

	// Listed from the programme side under either spelling, with the other
	// side's title resolved.
	for _, id := range []string{rowID, progID} {
		got := listReferences(t, s, orgID, "audit_programme", id)
		if len(got) != 2 {
			t.Errorf("?type=audit_programme&id=%s: got %d rows, want 2", id, len(got))
		}
	}
	// And from the risk side, the programme's title and canonical id.
	var found bool
	for _, r := range listReferences(t, s, orgID, "risk", risk.Identifier) {
		if r["target_type"] == "audit_programme" || r["source_type"] == "audit_programme" {
			found = true
			if r["title"] != prog.Title {
				t.Errorf("risk-side row title = %v, want %q", r["title"], prog.Title)
			}
		}
	}
	if !found {
		t.Errorf("risk %s does not list the programme", risk.Identifier)
	}

	_, err = post(orgID, "risk", risk.Identifier, "audit_programme", "999999")
	wantHTTPStatus(t, err, http.StatusBadRequest) // nonexistent programme
	_, err = post(orgID, "risk", risk.Identifier, "audit_programme", "AP-999999")
	wantHTTPStatus(t, err, http.StatusBadRequest) // nonexistent AP- programme

	otherOrg := newTestOrg(t, s, "refs-programme-other")
	otherRisk := newRefTestRisk(t, s, otherOrg, "risk in another org")
	_, err = post(otherOrg, "risk", otherRisk.Identifier, "audit_programme", rowID)
	wantHTTPStatus(t, err, http.StatusBadRequest) // programme from another org
}

// A programme's Discussion tab posts entity_type audit_programme; the
// entity_comments CHECK used to reject it.
func TestEntityCommentOnAuditProgramme(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "comment-programme")
	seedReviewUser(t, s, orgID, "dee@comment-programme.test", "contributor")
	prog := newRefTestProgramme(t, s, orgID, "programme with a discussion")

	err, code, body := postEntityComment(t, s, orgID, "dee@comment-programme.test",
		`{"entity_type":"audit_programme","entity_id":"`+strconv.Itoa(prog.ID)+`","body":"hello"}`)
	if err != nil || code != http.StatusCreated {
		t.Fatalf("programme comment: expected 201, got err=%v code=%d: %s", err, code, body)
	}
}
