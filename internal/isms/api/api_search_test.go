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
