package export

import (
	"reflect"
	"testing"

	"isms.sh/internal/isms/model"
)

func TestDraft(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"approved", false},
		{"Approved", false},
		{"APPROVED", false},
		{"draft", true},
		{"in_review", true},
		{"retired", true},
		{"", true},
		{"approved ", true}, // not trimmed: anything but exactly approved is a draft
	}
	for _, c := range cases {
		d := ExportDoc{Meta: model.DocumentFrontmatter{Status: c.status}}
		if got := d.Draft(); got != c.want {
			t.Errorf("Draft(%q) = %v, want %v", c.status, got, c.want)
		}
	}
}

func TestRevisionRows(t *testing.T) {
	changes := []model.DocumentChange{
		{Version: "1.0", Date: "2026-01-01", Author: "a", Description: "first"},
		{Version: "1.1", Date: "2026-02-01", Author: "b", ApprovedBy: "c", Description: "second"},
	}
	d := ExportDoc{Meta: model.DocumentFrontmatter{Changelog: changes}}
	rows := d.RevisionRows()
	if !reflect.DeepEqual(rows, changes) {
		t.Fatalf("rows = %+v, want file order %+v", rows, changes)
	}
	// A copy: callers cannot reorder the document's own changelog.
	rows[0].Version = "x"
	if d.Meta.Changelog[0].Version != "1.0" {
		t.Error("RevisionRows aliases the changelog")
	}
	if got := (ExportDoc{}).RevisionRows(); len(got) != 0 {
		t.Errorf("empty changelog -> %v", got)
	}
}
