package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"isms.sh/internal/isms/db"
)

// Regression for #366: audits and audit findings were linked under two ids —
// AUDIT-<id> / FIND-<id> from the create-from-finding flow and the delete
// guard, the bare row id from the Links tabs — so links made one way were
// invisible the other way and the finding delete guard missed bare-id links.
// Every write path must now store the prefixed form whichever was sent.
//
// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays
// green without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...

func newRefTestAudit(t *testing.T, s *Server, orgID int, title string) *db.Audit {
	t.Helper()
	a := &db.Audit{Title: title, Status: "planned"}
	if err := s.db.CreateAudit(context.Background(), orgID, a); err != nil {
		t.Fatalf("CreateAudit: %v", err)
	}
	return a
}

func newRefTestFinding(t *testing.T, s *Server, orgID, auditID int, title string) *db.AuditFinding {
	t.Helper()
	f := &db.AuditFinding{AuditID: auditID, FindingType: "minor_nc", Status: "open", Title: title}
	if err := s.db.AddAuditFinding(context.Background(), orgID, f); err != nil {
		t.Fatalf("AddAuditFinding: %v", err)
	}
	return f
}

func newRefTestCA(t *testing.T, s *Server, orgID int, title string) *db.CorrectiveAction {
	t.Helper()
	ca := &db.CorrectiveAction{Title: title, CreatedBy: "admin@refs-audit.test"}
	applyCorrectiveActionDefaults(ca)
	if err := s.db.CreateCorrectiveAction(context.Background(), orgID, ca); err != nil {
		t.Fatalf("CreateCorrectiveAction: %v", err)
	}
	return ca
}

// listReferences calls GET /references?type=…&id=… and returns its data rows.
func listReferences(t *testing.T, s *Server, orgID int, entityType, id string) []map[string]any {
	t.Helper()
	path := "/api/v1/references?type=" + entityType + "&id=" + id
	c, rec := ctxForPath(orgID, http.MethodGet, path, "", "admin")
	if err := s.handleListReferences(c); err != nil {
		t.Fatalf("handleListReferences(%s %s): %v", entityType, id, err)
	}
	var out struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding response body %q: %v", rec.Body.String(), err)
	}
	return out.Data
}

func TestCreateReferenceCanonicalisesAuditFinding(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "refs-audit-finding")

	audit := newRefTestAudit(t, s, orgID, "audit for reference canonicalisation test")
	finding := newRefTestFinding(t, s, orgID, audit.ID, "finding linked both ways")
	ca := newRefTestCA(t, s, orgID, "ca linked to finding both ways")

	post := func(sourceType, sourceID, targetType, targetID string) map[string]any {
		t.Helper()
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
			t.Fatalf("post(%s %s -> %s %s): %v", sourceType, sourceID, targetType, targetID, err)
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding response body %q: %v", rec.Body.String(), err)
		}
		return out
	}

	findingRowID := strconv.FormatInt(finding.ID, 10)
	findingID := "FIND-" + findingRowID

	// The create-from-finding flow sends FIND-<id>; the finding's Links tab
	// used to send the bare row id. Both are the same pair and must land on
	// the same two rows.
	if got := post("corrective_action", ca.Identifier, "audit_finding", findingID)["target_id"]; got != findingID {
		t.Errorf("post(ca -> %s): response target_id = %v, want %q", findingID, got, findingID)
	}
	if got := post("audit_finding", findingRowID, "corrective_action", ca.Identifier)["source_id"]; got != findingID {
		t.Errorf("post(%s -> ca): response source_id = %v, want %q", findingRowID, got, findingID)
	}
	refs, err := s.db.ListAllReferencesForEntity(ctx, orgID, "audit_finding", findingID)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity(audit_finding, %s): %v", findingID, err)
	}
	if len(refs) != 2 {
		t.Fatalf("finding %s has %d reference rows after both spellings, want 2 (forward + reverse, deduplicated)", findingID, len(refs))
	}

	// The Links tab finds the corrective action under either spelling.
	for _, id := range []string{findingRowID, findingID} {
		if got := listReferences(t, s, orgID, "audit_finding", id); len(got) != 1 {
			t.Errorf("?type=audit_finding&id=%s: got %d rows, want 1", id, len(got))
		}
	}

	// A corrective action linked from the Links tab (bare row id) now blocks
	// deleting the finding, as one linked through create-from-finding does.
	guarded := newRefTestFinding(t, s, orgID, audit.ID, "finding linked from its Links tab")
	guardedCA := newRefTestCA(t, s, orgID, "ca linked from the Links tab")
	post("audit_finding", strconv.FormatInt(guarded.ID, 10), "corrective_action", guardedCA.Identifier)
	if err := s.db.SoftDeleteAuditFinding(ctx, orgID, guarded.ID); err == nil {
		t.Errorf("SoftDeleteAuditFinding(%d) succeeded with %s still linked, want refusal", guarded.ID, guardedCA.Identifier)
	}

	// Audits: AUDIT-<id> whichever side and spelling.
	risk := newRefTestRisk(t, s, orgID, "risk linked to audit both ways")
	auditRowID := strconv.Itoa(audit.ID)
	auditID := "AUDIT-" + auditRowID
	if got := post("audit", auditRowID, "risk", risk.Identifier)["source_id"]; got != auditID {
		t.Errorf("post(audit %s -> risk): response source_id = %v, want %q", auditRowID, got, auditID)
	}
	if got := post("risk", risk.Identifier, "audit", auditID)["target_id"]; got != auditID {
		t.Errorf("post(risk -> %s): response target_id = %v, want %q", auditID, got, auditID)
	}
	refs, err = s.db.ListAllReferencesForEntity(ctx, orgID, "audit", auditID)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity(audit, %s): %v", auditID, err)
	}
	if len(refs) != 2 {
		t.Fatalf("audit %s has %d reference rows after both spellings, want 2", auditID, len(refs))
	}
	if got := listReferences(t, s, orgID, "audit", auditRowID); len(got) != 1 {
		t.Errorf("?type=audit&id=%s: got %d rows, want 1", auditRowID, len(got))
	}
}

// migrationSection366 returns the #366 statements from the release migration
// that carries them, so the test runs exactly what ships.
func migrationSection366(t *testing.T) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.sql"))
	if err != nil {
		t.Fatalf("listing migrations: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		src := string(b)
		start := strings.Index(src, "\n-- #366:")
		if start < 0 {
			continue
		}
		section := src[start:]
		if next := strings.Index(section[1:], "\n-- #"); next >= 0 {
			section = section[:next+1]
		}
		return section
	}
	t.Fatal("no migration carries a -- #366: section")
	return ""
}

func TestMigrationRewritesBareAuditReferenceIDs(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "refs-audit-migration")

	audit := newRefTestAudit(t, s, orgID, "audit with legacy bare-id links")
	finding := newRefTestFinding(t, s, orgID, audit.ID, "finding with legacy bare-id links")
	dupFinding := newRefTestFinding(t, s, orgID, audit.ID, "finding linked under both spellings")
	ca := newRefTestCA(t, s, orgID, "ca linked by bare finding id")
	risk := newRefTestRisk(t, s, orgID, "risk linked by bare audit id")
	legal := &db.LegalRequirement{Title: "legal with a numeric-looking id", Jurisdiction: "EU", Category: "privacy", Status: "open"}
	if err := s.db.CreateLegalRequirement(ctx, orgID, legal); err != nil {
		t.Fatalf("CreateLegalRequirement: %v", err)
	}

	fRow := strconv.FormatInt(finding.ID, 10)
	dupRow := strconv.FormatInt(dupFinding.ID, 10)
	aRow := strconv.Itoa(audit.ID)
	seed := func(st, sid, tt, tid string) {
		t.Helper()
		r := &db.EntityReference{SourceType: st, SourceID: sid, TargetType: tt, TargetID: tid, CreatedBy: "seed@refs-audit.test"}
		if err := s.db.CreateReference(ctx, orgID, r); err != nil {
			t.Fatalf("seeding %s %s -> %s %s: %v", st, sid, tt, tid, err)
		}
	}
	// What the finding's Links tab wrote before the fix.
	seed("audit_finding", fRow, "corrective_action", ca.Identifier)
	seed("corrective_action", ca.Identifier, "audit_finding", fRow)
	// The same pair under both spellings: the bare twin must collapse.
	seed("audit_finding", dupRow, "corrective_action", ca.Identifier)
	seed("corrective_action", ca.Identifier, "audit_finding", dupRow)
	seed("audit_finding", "FIND-"+dupRow, "corrective_action", ca.Identifier)
	seed("corrective_action", ca.Identifier, "audit_finding", "FIND-"+dupRow)
	// What the audit's Links tab wrote.
	seed("audit", aRow, "risk", risk.Identifier)
	seed("risk", risk.Identifier, "audit", aRow)
	// A bare number on another type is #348's business, not this rewrite's.
	seed("risk", risk.Identifier, "legal_requirement", "8")

	sql := migrationSection366(t)
	for run := 1; run <= 2; run++ { // the second run must find nothing to do
		if _, err := s.db.Pool().Exec(ctx, sql); err != nil {
			t.Fatalf("running #366 migration (run %d): %v", run, err)
		}
	}

	count := func(st, sid, tt, tid string) int {
		t.Helper()
		var n int
		err := s.db.Pool().QueryRow(ctx, `SELECT count(*) FROM entity_references
			WHERE organization_id = $1 AND source_type = $2 AND source_id = $3 AND target_type = $4 AND target_id = $5`,
			orgID, st, sid, tt, tid).Scan(&n)
		if err != nil {
			t.Fatalf("counting %s %s -> %s %s: %v", st, sid, tt, tid, err)
		}
		return n
	}
	for _, c := range []struct {
		st, sid, tt, tid string
		want             int
	}{
		{"audit_finding", "FIND-" + fRow, "corrective_action", ca.Identifier, 1},
		{"corrective_action", ca.Identifier, "audit_finding", "FIND-" + fRow, 1},
		{"audit_finding", fRow, "corrective_action", ca.Identifier, 0},
		{"audit_finding", "FIND-" + dupRow, "corrective_action", ca.Identifier, 1},
		{"corrective_action", ca.Identifier, "audit_finding", "FIND-" + dupRow, 1},
		{"audit_finding", dupRow, "corrective_action", ca.Identifier, 0},
		{"corrective_action", ca.Identifier, "audit_finding", dupRow, 0},
		{"audit", "AUDIT-" + aRow, "risk", risk.Identifier, 1},
		{"risk", risk.Identifier, "audit", "AUDIT-" + aRow, 1},
		{"audit", aRow, "risk", risk.Identifier, 0},
		{"risk", risk.Identifier, "legal_requirement", "8", 1},
	} {
		if got := count(c.st, c.sid, c.tt, c.tid); got != c.want {
			t.Errorf("%s %s -> %s %s: %d rows after migration, want %d", c.st, c.sid, c.tt, c.tid, got, c.want)
		}
	}

	// The rewritten link now blocks deleting the finding.
	if err := s.db.SoftDeleteAuditFinding(ctx, orgID, finding.ID); err == nil {
		t.Errorf("SoftDeleteAuditFinding(%d) succeeded with %s linked by a migrated row, want refusal", finding.ID, ca.Identifier)
	}
}
