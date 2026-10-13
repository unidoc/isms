package db

import (
	"context"
	"testing"
)

// Requires a migrated Postgres — see notifications_roundtrip_test.go's header.
// Skipped when ISMS_TEST_DATABASE_URL is unset.
//
// #436: ListAllAuditFindings feeds the universal search index. It must span
// every audit in the org, skip soft-deleted findings, and never return another
// org's rows.
func TestListAllAuditFindings(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "all-findings-a")
	otherOrgID, _ := newTestOrgUser(t, d, "all-findings-b")

	newAudit := func(org int, title string) int {
		a := Audit{Title: title, AuditType: "internal", Status: "planned"}
		if err := d.CreateAudit(ctx, org, &a); err != nil {
			t.Fatalf("CreateAudit %s: %v", title, err)
		}
		return a.ID
	}
	newFinding := func(org, auditID int, title string) int64 {
		f := AuditFinding{AuditID: auditID, FindingType: "minor_nc", Title: title, Description: "d", Status: "open"}
		if err := d.AddAuditFinding(ctx, org, &f); err != nil {
			t.Fatalf("AddAuditFinding %s: %v", title, err)
		}
		return f.ID
	}

	a1 := newAudit(orgID, "audit one")
	a2 := newAudit(orgID, "audit two")
	keep1 := newFinding(orgID, a1, "keep one")
	keep2 := newFinding(orgID, a2, "keep two")
	gone := newFinding(orgID, a1, "soft deleted")
	other := newFinding(otherOrgID, newAudit(otherOrgID, "other audit"), "other org")

	if err := d.SoftDeleteAuditFinding(ctx, orgID, gone); err != nil {
		t.Fatalf("SoftDeleteAuditFinding: %v", err)
	}

	got, err := d.ListAllAuditFindings(ctx, orgID)
	if err != nil {
		t.Fatalf("ListAllAuditFindings: %v", err)
	}
	ids := map[int64]bool{}
	for _, f := range got {
		ids[f.ID] = true
	}
	if len(got) != 2 || !ids[keep1] || !ids[keep2] {
		t.Errorf("got %d findings %v, want exactly the two live ones across both audits (%d, %d)", len(got), ids, keep1, keep2)
	}
	if ids[gone] {
		t.Errorf("soft-deleted finding %d was returned", gone)
	}
	if ids[other] {
		t.Errorf("another org's finding %d was returned", other)
	}
}
