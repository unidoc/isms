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

func TestRiskSearchEntrySeededHeading(t *testing.T) {
	seed := riskDescriptionSeedHeading + "\n\n\n\n" + riskConsequencesSeedHeading + "\n\n"
	search := func(title, desc string) string {
		return riskSearchEntry(&db.Risk{Identifier: "RISK-9", Title: title, Description: desc, Category: "operational"}).Search
	}

	e := riskSearchEntry(&db.Risk{Identifier: "RISK-9", Title: "Lost laptop", Description: seed, Category: "operational"})
	if e.Type != "risk" || e.ID != "RISK-9" || e.Title != "Lost laptop" {
		t.Errorf("entry=%+v, want type risk, ID RISK-9, Title Lost laptop", e)
	}

	got := search("Lost laptop", seed)
	for _, bad := range []string{"description", "potential consequences", "consequences"} {
		if strings.Contains(got, bad) {
			t.Errorf("seeded-only Search %q should not contain %q", got, bad)
		}
	}
	for _, want := range []string{"risk-9", "lost laptop", "operational"} {
		if !strings.Contains(got, want) {
			t.Errorf("seeded-only Search %q should contain %q", got, want)
		}
	}

	got = search("Lost laptop", riskDescriptionSeedHeading+"\n\nDevice stolen from a car\n\n"+riskConsequencesSeedHeading+"\n\nData exposure")
	for _, want := range []string{"device stolen from a car", "data exposure"} {
		if !strings.Contains(got, want) {
			t.Errorf("user text under heading: Search=%q should contain %q", got, want)
		}
	}
	if strings.Contains(got, "potential consequences") || strings.Contains(got, "description") {
		t.Errorf("headings should be stripped: Search=%q", got)
	}

	if got := search("Description of potential consequences", seed); !strings.Contains(got, "description of potential consequences") {
		t.Errorf("title must stay searchable: Search=%q", got)
	}

	if got := search("t", "  ## Potential consequences  \n  ## Description\nx"); strings.Contains(got, "consequences") || strings.Contains(got, "description") {
		t.Errorf("padded headings should be stripped: Search=%q", got)
	}

	if got := search("t", "A long description of the potential consequences"); !strings.Contains(got, "a long description of the potential consequences") {
		t.Errorf("prose containing the words must stay searchable: Search=%q", got)
	}
}

func TestSystemSearchEntrySeededHeading(t *testing.T) {
	seed := systemPurposeSeedHeading + "\n\n"
	search := func(name, desc string) string {
		return systemSearchEntry(&db.System{Identifier: "SYS-4", Name: name, Description: desc}).Search
	}

	e := systemSearchEntry(&db.System{Identifier: "SYS-4", Name: "Payroll", Description: seed})
	if e.Type != "system" || e.ID != "SYS-4" || e.Title != "Payroll" {
		t.Errorf("entry=%+v, want type system, ID SYS-4, Title Payroll", e)
	}

	got := search("Payroll", seed)
	if strings.Contains(got, "purpose") {
		t.Errorf("seeded-only Search %q should not contain purpose", got)
	}
	for _, want := range []string{"sys-4", "payroll"} {
		if !strings.Contains(got, want) {
			t.Errorf("seeded-only Search %q should contain %q", got, want)
		}
	}

	got = search("Payroll", seed+"Runs monthly salaries")
	if !strings.Contains(got, "runs monthly salaries") || strings.Contains(got, "purpose") {
		t.Errorf("user text under heading: Search=%q", got)
	}

	if got := search("Purpose built CRM", seed); !strings.Contains(got, "purpose built crm") {
		t.Errorf("name must stay searchable: Search=%q", got)
	}

	if got := search("t", "  ## Purpose  \n\nx"); strings.Contains(got, "purpose") {
		t.Errorf("padded heading should be stripped: Search=%q", got)
	}

	if got := search("t", "Purpose: hosting payroll"); !strings.Contains(got, "purpose: hosting payroll") {
		t.Errorf("prose containing the word must stay searchable: Search=%q", got)
	}
}

func TestSupplierSearchEntrySeededHeading(t *testing.T) {
	seed := supplierServicesSeedHeading + "\n\n"
	search := func(name, notes string) string {
		return supplierSearchEntry(&db.Supplier{Identifier: "SUP-2", Name: name, Notes: notes}).Search
	}

	e := supplierSearchEntry(&db.Supplier{Identifier: "SUP-2", Name: "Acme", Notes: seed})
	if e.Type != "supplier" || e.ID != "SUP-2" || e.Title != "Acme" {
		t.Errorf("entry=%+v, want type supplier, ID SUP-2, Title Acme", e)
	}

	got := search("Acme", seed)
	if strings.Contains(got, "services") {
		t.Errorf("seeded-only Search %q should not contain services", got)
	}
	for _, want := range []string{"sup-2", "acme"} {
		if !strings.Contains(got, want) {
			t.Errorf("seeded-only Search %q should contain %q", got, want)
		}
	}

	got = search("Acme", seed+"Payroll processing")
	if !strings.Contains(got, "payroll processing") || strings.Contains(got, "services") {
		t.Errorf("user text under heading: Search=%q", got)
	}

	if got := search("Cloud Services Ltd", seed); !strings.Contains(got, "cloud services ltd") {
		t.Errorf("name must stay searchable: Search=%q", got)
	}

	if got := search("t", "  ## Services  \n\nx"); strings.Contains(got, "services") {
		t.Errorf("padded heading should be stripped: Search=%q", got)
	}

	if got := search("t", "Hosting services for payroll"); !strings.Contains(got, "hosting services for payroll") {
		t.Errorf("prose containing the word must stay searchable: Search=%q", got)
	}
}

// Existing rows hold these exact bytes; if a default drifts, the stored seed no
// longer equals the heading constants and stops being stripped (#474).
func TestSeededDefaultsBytes(t *testing.T) {
	var r db.Risk
	applyRiskDefaults(&r, "a@b.c")
	if want := "## Description\n\n\n\n## Potential consequences\n\n"; r.Description != want {
		t.Errorf("risk description=%q, want %q", r.Description, want)
	}
	var sys db.System
	applySystemDefaults(&sys, "a@b.c")
	if want := "## Purpose\n\n"; sys.Description != want {
		t.Errorf("system description=%q, want %q", sys.Description, want)
	}
	if want := "## Access control\n\n"; sys.Notes != want {
		t.Errorf("system notes=%q, want %q", sys.Notes, want)
	}
	var sup db.Supplier
	applySupplierDefaults(&sup, "a@b.c")
	if want := "## Services\n\n"; sup.Notes != want {
		t.Errorf("supplier notes=%q, want %q", sup.Notes, want)
	}
}
