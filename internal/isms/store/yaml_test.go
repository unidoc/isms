package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"isms.sh/internal/isms/model"
)

func TestParseFrontmatter_Valid(t *testing.T) {
	input := `---
document_id: ISO27001-4-1
title: Context of the Organization
status: draft
---
# Context of the Organization

This document describes...`

	fm, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(fm, "document_id: ISO27001-4-1") {
		t.Errorf("frontmatter should contain document_id, got: %s", fm)
	}
	if !strings.Contains(fm, "title: Context of the Organization") {
		t.Errorf("frontmatter should contain title, got: %s", fm)
	}
	if !strings.Contains(body, "# Context of the Organization") {
		t.Errorf("body should contain heading, got: %s", body)
	}
	if !strings.Contains(body, "This document describes...") {
		t.Errorf("body should contain content, got: %s", body)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	input := `# Just a markdown file

No frontmatter here.`

	_, _, err := parseFrontmatter(input)
	if err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
	if !strings.Contains(err.Error(), "no frontmatter found") {
		t.Errorf("error should mention no frontmatter, got: %v", err)
	}
}

func TestParseFrontmatter_EmptyFrontmatter(t *testing.T) {
	input := `---
---
Some body content.`

	fm, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fm != "" {
		t.Errorf("frontmatter should be empty, got: %q", fm)
	}
	if !strings.Contains(body, "Some body content.") {
		t.Errorf("body should contain content, got: %s", body)
	}
}

func TestParseFrontmatter_EmptyBody(t *testing.T) {
	input := `---
document_id: test-doc
title: Test
status: draft
---`

	fm, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(fm, "document_id: test-doc") {
		t.Errorf("frontmatter should contain document_id, got: %s", fm)
	}
	if body != "" {
		t.Errorf("body should be empty, got: %q", body)
	}
}

func TestParseFrontmatter_EmptyInput(t *testing.T) {
	_, _, err := parseFrontmatter("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseFrontmatter_MultilineBody(t *testing.T) {
	input := `---
title: Test
---
Line 1
Line 2
Line 3`

	_, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 body lines, got %d: %v", len(lines), lines)
	}
}

func TestParseFrontmatter_DashesInBody(t *testing.T) {
	// Dashes in the body (not at line start) should not confuse the parser
	input := `---
title: Test
---
Some text with --- in it
And another --- line`

	_, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(body, "Some text with --- in it") {
		t.Errorf("body should preserve dashes, got: %s", body)
	}
}

func TestDocumentIDNormalization(t *testing.T) {
	// Test that document_id is normalized to lowercase on load
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "documents")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `---
document_id: ISO27001-A-5-1
title: Test Policy
status: draft
---
# Test`

	docPath := filepath.Join(docsDir, "test.md")
	if err := os.WriteFile(docPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(dir)
	doc, err := s.LoadDocument(docPath)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}

	if doc.Frontmatter.DocumentID != "iso27001-a-5-1" {
		t.Errorf("document_id should be lowercase, got: %q", doc.Frontmatter.DocumentID)
	}
}

func TestDocumentRoundTrip(t *testing.T) {
	dir := t.TempDir()

	original := &DocumentFile{
		Path: filepath.Join(dir, "test.md"),
		Frontmatter: model.DocumentFrontmatter{
			DocumentID:     "test-doc-001",
			Title:          "Test Document",
			Version:        "1.0",
			Status:         "draft",
			Author:         "alice@example.com",
			Owner:          "bob@example.com",
			Classification: "internal",
		},
		Body: "\n# Test Document\n\nThis is a test.",
	}

	s := New(dir)

	// Save
	if err := s.SaveDocument(original); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}

	// Load back
	loaded, err := s.LoadDocument(original.Path)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}

	// Compare frontmatter fields (should be exact)
	if loaded.Frontmatter.DocumentID != original.Frontmatter.DocumentID {
		t.Errorf("DocumentID = %q, want %q", loaded.Frontmatter.DocumentID, original.Frontmatter.DocumentID)
	}
	if loaded.Frontmatter.Title != original.Frontmatter.Title {
		t.Errorf("Title = %q, want %q", loaded.Frontmatter.Title, original.Frontmatter.Title)
	}
	if loaded.Frontmatter.Version != original.Frontmatter.Version {
		t.Errorf("Version = %q, want %q", loaded.Frontmatter.Version, original.Frontmatter.Version)
	}
	if loaded.Frontmatter.Status != original.Frontmatter.Status {
		t.Errorf("Status = %q, want %q", loaded.Frontmatter.Status, original.Frontmatter.Status)
	}
	if loaded.Frontmatter.Author != original.Frontmatter.Author {
		t.Errorf("Author = %q, want %q", loaded.Frontmatter.Author, original.Frontmatter.Author)
	}
	if loaded.Frontmatter.Owner != original.Frontmatter.Owner {
		t.Errorf("Owner = %q, want %q", loaded.Frontmatter.Owner, original.Frontmatter.Owner)
	}
	if loaded.Frontmatter.Classification != original.Frontmatter.Classification {
		t.Errorf("Classification = %q, want %q", loaded.Frontmatter.Classification, original.Frontmatter.Classification)
	}

	// Body: parseFrontmatter prepends "\n" to non-empty body, so the round-trip
	// adds one leading newline. Verify content is preserved (trimmed comparison).
	trimmedOriginal := strings.TrimSpace(original.Body)
	trimmedLoaded := strings.TrimSpace(loaded.Body)
	if trimmedLoaded != trimmedOriginal {
		t.Errorf("Body content mismatch:\n  got:  %q\n  want: %q", trimmedLoaded, trimmedOriginal)
	}
}

func TestDocumentRoundTrip_WithChangelog(t *testing.T) {
	dir := t.TempDir()

	original := &DocumentFile{
		Path: filepath.Join(dir, "changelog-test.md"),
		Frontmatter: model.DocumentFrontmatter{
			DocumentID: "changelog-doc",
			Title:      "Changelog Test",
			Version:    "2.0",
			Status:     "approved",
			Changelog: []model.DocumentChange{
				{Version: "1.0", Date: "2025-01-01", Author: "alice@example.com", Description: "Initial version"},
				{Version: "2.0", Date: "2025-06-01", Author: "bob@example.com", ApprovedBy: "carol@example.com", Description: "Major update"},
			},
		},
		Body: "\n# Content here",
	}

	s := New(dir)

	if err := s.SaveDocument(original); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}

	loaded, err := s.LoadDocument(original.Path)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}

	if len(loaded.Frontmatter.Changelog) != 2 {
		t.Fatalf("Changelog length = %d, want 2", len(loaded.Frontmatter.Changelog))
	}

	cl := loaded.Frontmatter.Changelog
	if cl[0].Version != "1.0" || cl[0].Author != "alice@example.com" {
		t.Errorf("Changelog[0] = %+v, want version 1.0 by alice", cl[0])
	}
	if cl[1].Version != "2.0" || cl[1].ApprovedBy != "carol@example.com" {
		t.Errorf("Changelog[1] = %+v, want version 2.0 approved by carol", cl[1])
	}

	// Body preserved (content check, trimmed)
	if strings.TrimSpace(loaded.Body) != strings.TrimSpace(original.Body) {
		t.Errorf("Body mismatch: got %q, want %q", loaded.Body, original.Body)
	}
}

func TestDocumentMissingFields(t *testing.T) {
	// Only document_id and title; all optional fields omitted
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "documents")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `---
document_id: minimal-doc
title: Minimal
status: draft
---
Body text.`

	docPath := filepath.Join(docsDir, "minimal.md")
	if err := os.WriteFile(docPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(dir)
	doc, err := s.LoadDocument(docPath)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}

	if doc.Frontmatter.DocumentID != "minimal-doc" {
		t.Errorf("DocumentID = %q, want %q", doc.Frontmatter.DocumentID, "minimal-doc")
	}
	if doc.Frontmatter.Title != "Minimal" {
		t.Errorf("Title = %q, want %q", doc.Frontmatter.Title, "Minimal")
	}
	// Optional fields should be zero values
	if doc.Frontmatter.Version != "" {
		t.Errorf("Version should be empty, got %q", doc.Frontmatter.Version)
	}
	if doc.Frontmatter.Author != "" {
		t.Errorf("Author should be empty, got %q", doc.Frontmatter.Author)
	}
	if doc.Frontmatter.Owner != "" {
		t.Errorf("Owner should be empty, got %q", doc.Frontmatter.Owner)
	}
	if doc.Frontmatter.ReviewCycle != 0 {
		t.Errorf("ReviewCycle should be 0, got %d", doc.Frontmatter.ReviewCycle)
	}
	if doc.Frontmatter.Changelog != nil {
		t.Errorf("Changelog should be nil, got %v", doc.Frontmatter.Changelog)
	}
}

func TestDocumentIDCaseInsensitive(t *testing.T) {
	// Verify various cases all normalize to lowercase
	tests := []struct {
		input string
		want  string
	}{
		{"ISO27001-4-1", "iso27001-4-1"},
		{"iso27001-4-1", "iso27001-4-1"},
		{"Iso27001-A-5-1", "iso27001-a-5-1"},
		{"UPPERCASE", "uppercase"},
		{"MiXeD-CaSe", "mixed-case"},
		{"already-lower", "already-lower"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			dir := t.TempDir()
			content := "---\ndocument_id: " + tt.input + "\ntitle: Test\nstatus: draft\n---\nBody"
			docPath := filepath.Join(dir, "test.md")
			if err := os.WriteFile(docPath, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}

			s := New(dir)
			doc, err := s.LoadDocument(docPath)
			if err != nil {
				t.Fatalf("LoadDocument: %v", err)
			}

			if doc.Frontmatter.DocumentID != tt.want {
				t.Errorf("DocumentID = %q, want %q", doc.Frontmatter.DocumentID, tt.want)
			}
		})
	}
}

func TestSaveDocument_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	nestedPath := filepath.Join(dir, "a", "b", "c", "doc.md")

	s := New(dir)
	doc := &DocumentFile{
		Path: nestedPath,
		Frontmatter: model.DocumentFrontmatter{
			DocumentID: "nested-doc",
			Title:      "Nested",
			Status:     "draft",
		},
		Body: "\nContent",
	}

	if err := s.SaveDocument(doc); err != nil {
		t.Fatalf("SaveDocument should create parent dirs: %v", err)
	}

	if _, err := os.Stat(nestedPath); os.IsNotExist(err) {
		t.Error("file should exist after SaveDocument")
	}
}

func TestSaveDocument_Format(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "format-test.md")

	s := New(dir)
	doc := &DocumentFile{
		Path: docPath,
		Frontmatter: model.DocumentFrontmatter{
			DocumentID: "fmt-doc",
			Title:      "Format Test",
			Status:     "draft",
		},
		Body: "\n# Heading\n\nParagraph",
	}

	if err := s.SaveDocument(doc); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(raw)

	// Should start with ---
	if !strings.HasPrefix(content, "---\n") {
		t.Error("saved document should start with ---")
	}

	// Should have closing ---
	parts := strings.SplitN(content, "---\n", 3)
	if len(parts) < 3 {
		t.Error("saved document should have opening and closing ---")
	}

	// Body should follow
	if !strings.Contains(content, "# Heading") {
		t.Error("saved document should contain body content")
	}
}
