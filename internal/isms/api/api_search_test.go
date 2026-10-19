package api

import (
	"strings"
	"testing"

	"isms.sh/internal/isms/db"
)

// #171: objectives + programs join the search index. These lock in that the
// SearchEntry uses the IDENTIFIER form (display_id / key), not the numeric id —
// which is what resolveEntityTitle and the deep-link routes resolve against.

func TestObjectiveSearchEntries(t *testing.T) {
	got := objectiveSearchEntries([]db.Objective{
		{ID: 42, DisplayID: "ISMS-1", Title: "Reduce phishing", Description: "click rate"},
	})
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	e := got[0]
	if e.Type != "objective" {
		t.Errorf("Type=%q, want objective", e.Type)
	}
	if e.ID != "ISMS-1" {
		t.Errorf("ID=%q, want the display_id ISMS-1 (not the numeric id 42)", e.ID)
	}
	if e.Title != "Reduce phishing" {
		t.Errorf("Title=%q, want Reduce phishing", e.Title)
	}
	if !strings.Contains(e.Search, "reduce phishing") || !strings.Contains(e.Search, "isms-1") {
		t.Errorf("Search should carry the lowercased display_id + title: %q", e.Search)
	}
}

func TestProgramSearchEntries(t *testing.T) {
	got := programSearchEntries([]db.Program{
		{ID: 7, Key: "ISMS", Title: "Security Programme", Description: "annual"},
	})
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	e := got[0]
	if e.Type != "program" {
		t.Errorf("Type=%q, want program", e.Type)
	}
	if e.ID != "ISMS" {
		t.Errorf("ID=%q, want the key ISMS (not the numeric id 7)", e.ID)
	}
	if e.Title != "Security Programme" {
		t.Errorf("Title=%q, want Security Programme", e.Title)
	}
	if !strings.Contains(e.Search, "security programme") || !strings.Contains(e.Search, "isms") {
		t.Errorf("Search should carry the lowercased key + title: %q", e.Search)
	}
}

func TestSearchEntriesEmpty(t *testing.T) {
	if got := objectiveSearchEntries(nil); len(got) != 0 {
		t.Errorf("nil objectives → %d entries, want 0", len(got))
	}
	if got := programSearchEntries(nil); len(got) != 0 {
		t.Errorf("nil programs → %d entries, want 0", len(got))
	}
}

// #436: audit programmes, audits and findings join the search index. The Type
// strings and display-id forms must match what the references API stores
// (audit_programme/AP-<id>, audit/AUDIT-<id>, audit_finding/FIND-<id>) so a
// result can be handed straight to createReference.

func TestAuditProgrammeSearchEntries(t *testing.T) {
	got := auditProgrammeSearchEntries([]db.AuditProgramme{
		{ID: 7, Title: "Annual Audit Programme", Year: 2026, Description: "ISO surveillance cycle"},
	})
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	e := got[0]
	if e.Type != "audit_programme" {
		t.Errorf("Type=%q, want audit_programme", e.Type)
	}
	if e.ID != "AP-7" {
		t.Errorf("ID=%q, want AP-7 (display id, not the bare row id)", e.ID)
	}
	if e.Title != "Annual Audit Programme" {
		t.Errorf("Title=%q, want Annual Audit Programme", e.Title)
	}
	for _, want := range []string{"ap-7", "annual audit programme", "iso surveillance cycle", "2026"} {
		if !strings.Contains(e.Search, want) {
			t.Errorf("Search %q should contain %q", e.Search, want)
		}
	}
}

func TestAuditSearchEntries(t *testing.T) {
	got := auditSearchEntries([]db.Audit{
		{ID: 3, Title: "Supplier Management Audit", Scope: "Clause 8 and Annex A.5.19"},
	})
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	e := got[0]
	if e.Type != "audit" {
		t.Errorf("Type=%q, want audit", e.Type)
	}
	if e.ID != "AUDIT-3" {
		t.Errorf("ID=%q, want AUDIT-3 (display id, not the bare row id)", e.ID)
	}
	if e.Title != "Supplier Management Audit" {
		t.Errorf("Title=%q, want Supplier Management Audit", e.Title)
	}
	for _, want := range []string{"audit-3", "supplier management audit", "clause 8 and annex a.5.19"} {
		if !strings.Contains(e.Search, want) {
			t.Errorf("Search %q should contain %q", e.Search, want)
		}
	}
}

func TestAuditFindingSearchEntries(t *testing.T) {
	got := auditFindingSearchEntries([]db.AuditFinding{
		{ID: 12, Title: "Missing Access Review", Description: "Quarterly review not evidenced"},
	})
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	e := got[0]
	if e.Type != "audit_finding" {
		t.Errorf("Type=%q, want audit_finding", e.Type)
	}
	if e.ID != "FIND-12" {
		t.Errorf("ID=%q, want FIND-12 (display id, not the bare row id)", e.ID)
	}
	if e.Title != "Missing Access Review" {
		t.Errorf("Title=%q, want Missing Access Review", e.Title)
	}
	for _, want := range []string{"find-12", "missing access review", "quarterly review not evidenced"} {
		if !strings.Contains(e.Search, want) {
			t.Errorf("Search %q should contain %q", e.Search, want)
		}
	}
}

func TestAuditSearchEntriesEmpty(t *testing.T) {
	if got := auditProgrammeSearchEntries(nil); len(got) != 0 {
		t.Errorf("nil programmes → %d entries, want 0", len(got))
	}
	if got := auditSearchEntries(nil); len(got) != 0 {
		t.Errorf("nil audits → %d entries, want 0", len(got))
	}
	if got := auditFindingSearchEntries(nil); len(got) != 0 {
		t.Errorf("nil findings → %d entries, want 0", len(got))
	}
}

func TestLegalSearchEntry(t *testing.T) {
	lr := db.LegalRequirement{Identifier: "LEGAL-7", Title: "Data Protection Act", Description: "Privacy law", Jurisdiction: "IS"}
	e := legalSearchEntry(&lr)
	if e.Type != "legal" {
		t.Errorf("Type=%q, want legal (the type GlobalSearch routes on, not legal_requirement)", e.Type)
	}
	if e.ID != "LEGAL-7" {
		t.Errorf("ID=%q, want LEGAL-7", e.ID)
	}
	if e.Title != "Data Protection Act" {
		t.Errorf("Title=%q, want Data Protection Act", e.Title)
	}
	if want := legalSearchText(&lr); e.Search != want {
		t.Errorf("Search=%q, want %q", e.Search, want)
	}
}

func TestAuditFindingSearchEntrySeededHeading(t *testing.T) {
	seed := auditFindingSeedHeading + "\n\n"
	search := func(title, desc string) string {
		return auditFindingSearchEntry(&db.AuditFinding{ID: 3, Title: title, Description: desc}).Search
	}

	got := search("Missing review", seed)
	for _, bad := range []string{"corrective"} {
		if strings.Contains(got, bad) {
			t.Errorf("seeded-only Search %q should not contain %q", got, bad)
		}
	}
	for _, want := range []string{"find-3", "missing review"} {
		if !strings.Contains(got, want) {
			t.Errorf("seeded-only Search %q should contain %q", got, want)
		}
	}

	got = search("Missing review", seed+"Rotate the leaked keys")
	if !strings.Contains(got, "rotate the leaked keys") || strings.Contains(got, "corrective action") {
		t.Errorf("user text under heading: Search=%q", got)
	}

	if got := search("Corrective action overdue", seed); !strings.Contains(got, "corrective action") {
		t.Errorf("title must stay searchable: Search=%q", got)
	}

	if got := search("t", "  ## Corrective Action  \n\nx"); strings.Contains(got, "corrective") {
		t.Errorf("padded heading should be stripped: Search=%q", got)
	}

	if got := search("t", "Raise a corrective action with IT"); !strings.Contains(got, "raise a corrective action with it") {
		t.Errorf("prose containing the words must stay searchable: Search=%q", got)
	}
}
