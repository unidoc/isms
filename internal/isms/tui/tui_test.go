package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewLoadsLocalCloneDocuments verifies the TUI reads the local clone off disk
// (#126) — a folder header plus the document, with body + metadata cached so the
// reader is fully offline (no API).
func TestNewLoadsLocalCloneDocuments(t *testing.T) {
	root := t.TempDir()
	docDir := filepath.Join(root, "documents", "iso27001")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\n" +
		"document_id: iso27001-4-1\n" +
		"title: Context\n" +
		"status: approved\n" +
		"version: \"2\"\n" +
		"---\n\n# Context\n\nbody line\n"
	if err := os.WriteFile(filepath.Join(docDir, "4-1.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(root)

	var folderHdr, docRow *item
	for i := range m.items {
		it := &m.items[i]
		if it.isFolder && it.title == "ISO27001" {
			folderHdr = it
		}
		if it.docID == "iso27001-4-1" {
			docRow = it
		}
	}
	if folderHdr == nil {
		t.Errorf("expected a folder header for iso27001; got items=%+v", m.items)
	}
	if docRow == nil {
		t.Fatalf("expected the document row; got items=%+v", m.items)
	}
	if docRow.title != "Context" || docRow.status != "approved" || docRow.version != "2" {
		t.Errorf("doc metadata not loaded from frontmatter: %+v", *docRow)
	}
	if !strings.Contains(docRow.body, "body line") {
		t.Errorf("doc body not cached for offline read: %q", docRow.body)
	}
}

// TestFolderHeaderUsesDotTitle verifies the folder header honors a .title file's
// display name (as the API-backed tree did), not the raw directory name (#130 review).
func TestFolderHeaderUsesDotTitle(t *testing.T) {
	root := t.TempDir()
	docDir := filepath.Join(root, "documents", "iso27001")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docDir, ".title"), []byte("ISO 27001:2022\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "---\ndocument_id: iso27001-4-1\ntitle: Context\nstatus: approved\nversion: \"1\"\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(docDir, "4-1.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(root)

	want := strings.ToUpper("ISO 27001:2022")
	var found bool
	for _, it := range m.items {
		if it.isFolder && it.title == want {
			found = true
		}
	}
	if !found {
		t.Errorf("folder header should use .title display name %q; got items=%+v", want, m.items)
	}
}

// TestBadDocSurfacesError verifies a malformed doc surfaces a load error instead
// of silently zeroing the folder with a misleading "no documents" message (#130 review).
func TestBadDocSurfacesError(t *testing.T) {
	root := t.TempDir()
	docDir := filepath.Join(root, "documents", "iso27001")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatal(err)
	}
	good := "---\ndocument_id: iso27001-4-1\ntitle: Context\nstatus: approved\nversion: \"1\"\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(docDir, "4-1.md"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	// No frontmatter delimiters → LoadDocumentsFromDir errors for the folder.
	if err := os.WriteFile(filepath.Join(docDir, "4-2.md"), []byte("just text, no frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(root)

	if !strings.Contains(m.loadErr, "Failed to load") || !strings.Contains(m.loadErr, "iso27001") {
		t.Errorf("expected a surfaced load error naming the folder, got loadErr=%q", m.loadErr)
	}
}
