package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDoc writes a minimal valid document under documents/<folder>/<name>.
// folder may be nested, e.g. "iso27001/clauses".
func writeDoc(t *testing.T, root, folder, name, id, title string) {
	t.Helper()
	dir := filepath.Join(root, "documents", folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\n" +
		"document_id: " + id + "\n" +
		"title: " + title + "\n" +
		"status: approved\n" +
		"version: \"1\"\n" +
		"---\n\n# " + title + "\n\nbody line\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
}

func docItem(m Model, id string) (treeItem, bool) {
	for _, it := range m.items {
		if !it.isFolder && it.docID == id {
			return it, true
		}
	}
	return treeItem{}, false
}

func folderItem(m Model, label string) (treeItem, bool) {
	for _, it := range m.items {
		if it.isFolder && it.label == label {
			return it, true
		}
	}
	return treeItem{}, false
}

// TestNewLoadsLocalCloneDocuments verifies the TUI reads the local clone off disk
// (#126) into the tree — document metadata + body cached for offline read.
func TestNewLoadsLocalCloneDocuments(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "iso27001", "4-1.md", "iso27001-4-1", "Context")

	m := New(root)

	d, ok := docItem(m, "iso27001-4-1")
	if !ok {
		t.Fatalf("expected the document in the tree; items=%+v", m.items)
	}
	if d.status != "approved" || d.version != "1" {
		t.Errorf("doc metadata not loaded from frontmatter: %+v", d)
	}
	if !strings.Contains(d.body, "body line") {
		t.Errorf("doc body not cached for offline read: %q", d.body)
	}
}

// TestNestedTreeMirrorsDirs is the fix for the "folders aren't under ISO 27001"
// report: the tree must mirror the real documents/<template>/<subfolder>/ layout,
// not flatten everything under the top folder.
func TestNestedTreeMirrorsDirs(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "iso27001/clauses", "4-1.md", "iso27001-4-1", "Context")
	writeDoc(t, root, "iso27001/annex-a", "a-5-1.md", "iso27001-a-5-1", "Policies")

	m := New(root)

	top, ok := folderItem(m, "iso27001")
	if !ok || top.depth != 0 {
		t.Fatalf("iso27001 should be a depth-0 folder; got %+v ok=%v", top, ok)
	}
	clauses, ok := folderItem(m, "clauses")
	if !ok || clauses.depth != 1 {
		t.Fatalf("clauses should nest under iso27001 at depth 1; got %+v ok=%v", clauses, ok)
	}
	annex, ok := folderItem(m, "annex-a")
	if !ok || annex.depth != 1 {
		t.Fatalf("annex-a should nest at depth 1; got %+v ok=%v", annex, ok)
	}
	d, ok := docItem(m, "iso27001-4-1")
	if !ok || d.depth != 2 {
		t.Fatalf("the doc should sit under clauses at depth 2; got %+v ok=%v", d, ok)
	}

	// Lone top folder opens, but its subfolders stay collapsed — so the visible
	// rows are the template + its two subfolders, NOT the docs.
	rows := m.visibleRows()
	if len(rows) != 3 {
		t.Fatalf("expected template + 2 subfolders visible (subfolders collapsed), got %d rows", len(rows))
	}
	for _, r := range rows {
		if !m.items[r].isFolder {
			t.Fatalf("no doc rows should show while subfolders are collapsed: %+v", m.items[r])
		}
	}
}

// TestDocRowShowsTitleNotID verifies rows label by the frontmatter title (like the
// web viewer), falling back to the id only when there is no title.
func TestDocRowShowsTitleNotID(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "iso27001", "4-1.md", "iso27001-4-1", "Understanding the organization")

	m := New(root)
	d, _ := docItem(m, "iso27001-4-1")
	if d.display() != "Understanding the organization" {
		t.Errorf("doc should display its title, got %q", d.display())
	}
	d.label = ""
	if d.display() != "iso27001-4-1" {
		t.Errorf("empty title should fall back to id, got %q", d.display())
	}
}

// TestFolderHeaderUsesDotTitle verifies folder labels honor a .title file's
// display name (#130 review), not the raw directory name.
func TestFolderHeaderUsesDotTitle(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "iso27001", "4-1.md", "iso27001-4-1", "Context")
	if err := os.WriteFile(filepath.Join(root, "documents", "iso27001", ".title"),
		[]byte("ISO 27001:2022\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(root)
	if _, ok := folderItem(m, "ISO 27001:2022"); !ok {
		t.Errorf("folder should use .title display name; items=%+v", m.items)
	}
}

// TestFoldersCollapsedByDefault verifies multiple top-level folders open closed.
func TestFoldersCollapsedByDefault(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "iso27001", "4-1.md", "iso27001-4-1", "Context")
	writeDoc(t, root, "policies", "acc.md", "policies-access", "Access Policy")

	m := New(root)
	rows := m.visibleRows()
	if len(rows) != 2 {
		t.Fatalf("collapsed tree should show only 2 folder rows, got %d", len(rows))
	}
	for _, r := range rows {
		if !m.items[r].isFolder {
			t.Fatalf("no document rows should be visible while collapsed: %+v", m.items[r])
		}
	}
}

// TestLoneFolderExpanded verifies a single top-level folder opens by default.
func TestLoneFolderExpanded(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "iso27001", "4-1.md", "iso27001-4-1", "Context")
	m := New(root)
	if !m.items[0].isFolder || !m.items[0].expanded {
		t.Errorf("a lone top folder should start expanded; got %+v", m.items[0])
	}
}

// TestFilterShowsMatchingDocs verifies the filter surfaces matching docs and
// their ancestor folders, across nesting.
func TestFilterShowsMatchingDocs(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "iso27001/clauses", "4-1.md", "iso27001-4-1", "Context of the organization")
	writeDoc(t, root, "iso27001/clauses", "5-1.md", "iso27001-5-1", "Leadership")

	m := New(root)
	m.filter = "leadership"
	rows := m.visibleRows()
	// iso27001 + clauses + the one matching doc
	if len(rows) != 3 {
		t.Fatalf("filter should show ancestors + 1 match, got %d rows", len(rows))
	}
	last := m.items[rows[len(rows)-1]]
	if last.isFolder || last.docID != "iso27001-5-1" {
		t.Errorf("filter matched the wrong row: %+v", last)
	}
}

// TestDocPathPopulated verifies each doc carries its working-tree path so it can
// be opened in $EDITOR.
func TestDocPathPopulated(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "iso27001", "4-1.md", "iso27001-4-1", "Context")
	m := New(root)
	d, _ := docItem(m, "iso27001-4-1")
	want := filepath.Join(root, "documents", "iso27001", "4-1.md")
	if d.path != want {
		t.Errorf("doc path = %q, want %q", d.path, want)
	}
}

// TestEditNoEditorIsNoOp verifies pressing edit without $EDITOR set does nothing
// destructive — no exec command, and a hint is surfaced.
func TestEditNoEditorIsNoOp(t *testing.T) {
	t.Setenv("EDITOR", "")
	root := t.TempDir()
	writeDoc(t, root, "iso27001", "4-1.md", "iso27001-4-1", "Context")
	m := New(root)
	m.cursor = 1 // the doc (lone folder is expanded, folder at row 0)
	nm, cmd := m.editCurrent()
	if cmd != nil {
		t.Error("no $EDITOR → no exec command")
	}
	if !strings.Contains(nm.(Model).loadErr, "EDITOR") {
		t.Errorf("expected an $EDITOR hint, got %q", nm.(Model).loadErr)
	}
}

// TestBadDocSurfacesError verifies a malformed doc surfaces a load error instead
// of silently hiding the folder (#130 review).
func TestBadDocSurfacesError(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "iso27001", "4-1.md", "iso27001-4-1", "Context")
	bad := filepath.Join(root, "documents", "iso27001", "4-2.md")
	if err := os.WriteFile(bad, []byte("just text, no frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(root)
	if !strings.Contains(m.loadErr, "Failed to load") {
		t.Errorf("expected a surfaced load error, got loadErr=%q", m.loadErr)
	}
}
