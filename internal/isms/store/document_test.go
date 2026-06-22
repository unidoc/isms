package store

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestDocumentBodyAtRef covers the body-comparison primitive behind the
// "changed since approval" fix (#3): a metadata-only frontmatter edit produces
// a new commit but must leave the stripped body identical, while a body edit
// must change it.
func TestDocumentBodyAtRef(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)
	absPath := filepath.Join(st.Root(), "documents", "test", "doc.md")

	v1 := "---\ndocument_id: d\ntitle: T\nstatus: approved\nowner: alice@test.com\n---\nReviewed body.\n"
	h1, err := st.CommitFile(absPath, []byte(v1), "A", "a@test.com", "initial")
	if err != nil {
		t.Fatalf("commit v1: %v", err)
	}

	// Metadata-only change (owner) — the stripped body must still match v1.
	v2 := "---\ndocument_id: d\ntitle: T\nstatus: approved\nowner: bob@test.com\n---\nReviewed body.\n"
	if _, err := st.CommitFile(absPath, []byte(v2), "A", "a@test.com", "owner change"); err != nil {
		t.Fatalf("commit v2: %v", err)
	}

	b1, err := st.DocumentBodyAtRef(h1, "documents/test/doc.md")
	if err != nil {
		t.Fatalf("body at h1: %v", err)
	}
	head, err := st.HeadHash()
	if err != nil {
		t.Fatalf("head hash: %v", err)
	}
	b2, err := st.DocumentBodyAtRef(head, "documents/test/doc.md")
	if err != nil {
		t.Fatalf("body at HEAD: %v", err)
	}
	if strings.TrimSpace(b1) != strings.TrimSpace(b2) {
		t.Errorf("metadata-only edit changed the body:\nh1:   %q\nHEAD: %q", b1, b2)
	}

	// Body change — the stripped bodies must now differ.
	v3 := "---\ndocument_id: d\ntitle: T\nstatus: draft\n---\nUpdated body.\n"
	if _, err := st.CommitFile(absPath, []byte(v3), "A", "a@test.com", "body change"); err != nil {
		t.Fatalf("commit v3: %v", err)
	}
	head2, err := st.HeadHash()
	if err != nil {
		t.Fatalf("head hash 2: %v", err)
	}
	b3, err := st.DocumentBodyAtRef(head2, "documents/test/doc.md")
	if err != nil {
		t.Fatalf("body at HEAD2: %v", err)
	}
	if strings.TrimSpace(b1) == strings.TrimSpace(b3) {
		t.Error("body change was not detected (bodies still equal)")
	}

	// A missing ref must error (the caller falls back to flagging on error).
	if _, err := st.DocumentBodyAtRef("0000000000000000000000000000000000000000", "documents/test/doc.md"); err == nil {
		t.Error("expected error for a non-existent ref, got nil")
	}
}
