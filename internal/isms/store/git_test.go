package store

import (
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"

	"isms.sh/internal/isms/model"
)

// initBareStore creates a bare git repo in dir and returns a Store.
func initBareStore(t *testing.T, dir string) *Store {
	t.Helper()
	repoPath := filepath.Join(dir, "test.git")
	if _, err := git.PlainInit(repoPath, true); err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	st, err := NewBare(repoPath)
	if err != nil {
		t.Fatalf("NewBare: %v", err)
	}
	return st
}

// commitTestDoc is a helper that commits a document with frontmatter to a bare repo.
// Returns the commit hash.
func commitTestDoc(t *testing.T, st *Store, relPath, docID, title, body string) string {
	t.Helper()
	content := "---\ndocument_id: " + docID + "\ntitle: " + title + "\nstatus: draft\n---\n" + body
	absPath := filepath.Join(st.Root(), relPath)
	hash, err := st.CommitFile(absPath, []byte(content), "Test User", "test@example.com", "add "+docID)
	if err != nil {
		t.Fatalf("CommitFile %s: %v", relPath, err)
	}
	return hash
}

func TestNewBare(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "test.git")

	// PlainInit creates the bare repo
	_, err := git.PlainInit(repoPath, true)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	st, err := NewBare(repoPath)
	if err != nil {
		t.Fatalf("NewBare: %v", err)
	}
	if st.Root() != repoPath {
		t.Errorf("Root() = %q, want %q", st.Root(), repoPath)
	}
	if st.DocsRoot() != filepath.Join(repoPath, "documents") {
		t.Errorf("DocsRoot() = %q, want %q", st.DocsRoot(), filepath.Join(repoPath, "documents"))
	}
}

func TestNewBare_NonExistent(t *testing.T) {
	dir := t.TempDir()
	_, err := NewBare(filepath.Join(dir, "nonexistent.git"))
	if err == nil {
		t.Fatal("expected error opening non-existent repo")
	}
}

func TestCommitFileAndReadFile(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	content := []byte("# Hello\nThis is a test file.\n")
	absPath := filepath.Join(st.Root(), "README.md")

	// Initial commit
	hash, err := st.CommitFile(absPath, content, "Test User", "test@example.com", "initial commit")
	if err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	if len(hash) != 40 {
		t.Errorf("expected 40-char hash, got %d chars: %q", len(hash), hash)
	}

	// Read back
	got, err := st.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("ReadFile content mismatch:\ngot:  %q\nwant: %q", got, content)
	}
}

func TestCommitFile_SecondCommit(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	abs := filepath.Join(st.Root(), "documents", "policies", "test.md")

	// First commit
	c1 := "---\ndocument_id: test-1\ntitle: Test\nstatus: draft\n---\nv1 body\n"
	h1, err := st.CommitFile(abs, []byte(c1), "A", "a@test.com", "first")
	if err != nil {
		t.Fatalf("first commit: %v", err)
	}

	// Second commit updates the file
	c2 := "---\ndocument_id: test-1\ntitle: Test\nstatus: draft\n---\nv2 body\n"
	h2, err := st.CommitFile(abs, []byte(c2), "B", "b@test.com", "second")
	if err != nil {
		t.Fatalf("second commit: %v", err)
	}
	if h1 == h2 {
		t.Error("second commit should have a different hash")
	}

	got, err := st.ReadFile(abs)
	if err != nil {
		t.Fatalf("ReadFile after second commit: %v", err)
	}
	if string(got) != c2 {
		t.Errorf("got %q, want %q", got, c2)
	}
}

func TestCommitFile_PathValidation(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	// Need an initial commit so HEAD exists
	abs := filepath.Join(st.Root(), "README.md")
	if _, err := st.CommitFile(abs, []byte("# Init"), "A", "a@test.com", "init"); err != nil {
		t.Fatalf("init commit: %v", err)
	}

	tests := []struct {
		name    string
		relPath string
		wantErr bool
	}{
		{"allowed README", "README.md", false},
		{"allowed doc", "documents/policies/test.md", false},
		{"allowed .title", "documents/policies/.title", false},
		{"disallowed root file", "config.yaml", true},
		{"disallowed binary", "documents/policies/test.pdf", true},
		{"disallowed path traversal", "../etc/passwd", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			absPath := filepath.Join(st.Root(), tc.relPath)
			_, err := st.CommitFile(absPath, []byte("test"), "A", "a@test.com", "test")
			if tc.wantErr && err == nil {
				t.Errorf("expected error for path %q", tc.relPath)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for path %q: %v", tc.relPath, err)
			}
		})
	}
}

func TestSaveAndLoadDocument(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	docPath := filepath.Join(st.Root(), "documents", "policies", "access-control.md")
	body := "\n# Access Control Policy\n\nThis policy governs access.\n"

	commitTestDoc(t, st, "documents/policies/access-control.md", "pol-ac-01", "Access Control Policy", "# Access Control Policy\n\nThis policy governs access.\n")

	// Load it back
	doc, err := st.LoadDocument(docPath)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}

	if doc.Frontmatter.DocumentID != "pol-ac-01" {
		t.Errorf("DocumentID = %q, want %q", doc.Frontmatter.DocumentID, "pol-ac-01")
	}
	if doc.Frontmatter.Title != "Access Control Policy" {
		t.Errorf("Title = %q, want %q", doc.Frontmatter.Title, "Access Control Policy")
	}
	if doc.Frontmatter.Status != "draft" {
		t.Errorf("Status = %q, want %q", doc.Frontmatter.Status, "draft")
	}
	if !strings.Contains(doc.Body, "This policy governs access.") {
		t.Errorf("Body should contain policy text, got: %q", doc.Body)
	}
	if doc.Path != docPath {
		t.Errorf("Path = %q, want %q", doc.Path, docPath)
	}
	_ = body // used for clarity
}

func TestLoadDocument_CaseNormalization(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	// Commit a doc with mixed-case document_id
	content := "---\ndocument_id: ISO27001-A-5-1\ntitle: Test\nstatus: draft\n---\nBody\n"
	absPath := filepath.Join(st.Root(), "documents", "controls", "a51.md")
	if _, err := st.CommitFile(absPath, []byte(content), "A", "a@test.com", "add"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	doc, err := st.LoadDocument(absPath)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}
	if doc.Frontmatter.DocumentID != "iso27001-a-5-1" {
		t.Errorf("DocumentID should be lowercased, got %q", doc.Frontmatter.DocumentID)
	}
}

func TestFindDocumentByID(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	commitTestDoc(t, st, "documents/policies/info-sec.md", "pol-is-01", "InfoSec Policy", "Body\n")
	commitTestDoc(t, st, "documents/controls/encryption.md", "ctrl-enc-01", "Encryption", "Body\n")

	t.Run("exact match", func(t *testing.T) {
		path := st.FindDocumentByID("pol-is-01")
		if path == "" {
			t.Fatal("expected to find document by ID")
		}
		if !strings.HasSuffix(path, "documents/policies/info-sec.md") {
			t.Errorf("unexpected path: %q", path)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		path := st.FindDocumentByID("POL-IS-01")
		if path == "" {
			t.Fatal("expected case-insensitive match")
		}
	})

	t.Run("not found", func(t *testing.T) {
		path := st.FindDocumentByID("nonexistent")
		if path != "" {
			t.Errorf("expected empty path for missing ID, got %q", path)
		}
	})

	t.Run("HasDocumentID", func(t *testing.T) {
		if !st.HasDocumentID("ctrl-enc-01") {
			t.Error("HasDocumentID should return true")
		}
		if st.HasDocumentID("nonexistent") {
			t.Error("HasDocumentID should return false for missing ID")
		}
	})
}

func TestHeadHash(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	// Before any commits, HeadHash should error
	_, err := st.HeadHash()
	if err == nil {
		t.Fatal("expected error before any commits")
	}

	commitTestDoc(t, st, "documents/test/doc.md", "test-1", "Test", "body\n")

	hash, err := st.HeadHash()
	if err != nil {
		t.Fatalf("HeadHash: %v", err)
	}
	if len(hash) != 40 {
		t.Errorf("expected 40-char hex hash, got %d chars: %q", len(hash), hash)
	}

	// After another commit, hash should change
	commitTestDoc(t, st, "documents/test/doc2.md", "test-2", "Test2", "body2\n")
	hash2, err := st.HeadHash()
	if err != nil {
		t.Fatalf("HeadHash after second commit: %v", err)
	}
	if hash == hash2 {
		t.Error("HeadHash should change after a new commit")
	}
}

func TestListDocFolders(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	// Create docs in multiple folders
	commitTestDoc(t, st, "documents/policies/p1.md", "p1", "P1", "body\n")
	commitTestDoc(t, st, "documents/controls/c1.md", "c1", "C1", "body\n")
	commitTestDoc(t, st, "documents/procedures/pr1.md", "pr1", "PR1", "body\n")

	folders := st.ListDocFolders()
	if len(folders) < 3 {
		t.Fatalf("expected at least 3 folders, got %d: %v", len(folders), folders)
	}

	found := map[string]bool{}
	for _, f := range folders {
		found[f] = true
	}
	for _, want := range []string{"policies", "controls", "procedures"} {
		if !found[want] {
			t.Errorf("missing folder %q in %v", want, folders)
		}
	}
}

func TestLoadDocumentsFromDir(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	commitTestDoc(t, st, "documents/policies/p1.md", "p1", "Policy 1", "body 1\n")
	commitTestDoc(t, st, "documents/policies/p2.md", "p2", "Policy 2", "body 2\n")
	commitTestDoc(t, st, "documents/controls/c1.md", "c1", "Control 1", "body 3\n")

	docs, err := st.LoadDocumentsFromDir("policies")
	if err != nil {
		t.Fatalf("LoadDocumentsFromDir: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 documents in policies, got %d", len(docs))
	}

	ids := map[string]bool{}
	for _, d := range docs {
		ids[d.Frontmatter.DocumentID] = true
	}
	if !ids["p1"] || !ids["p2"] {
		t.Errorf("expected p1 and p2, got %v", ids)
	}
}

func TestLoadDocumentsFromDir_Empty(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	// Commit something in a different folder so HEAD exists
	commitTestDoc(t, st, "documents/other/x.md", "x", "X", "body\n")

	docs, err := st.LoadDocumentsFromDir("nonexistent")
	if err != nil {
		t.Fatalf("LoadDocumentsFromDir should not error on missing dir: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(docs))
	}
}

func TestDeleteFile(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	absPath := filepath.Join(st.Root(), "documents", "policies", "to-delete.md")
	content := "---\ndocument_id: del-1\ntitle: Delete Me\nstatus: draft\n---\nGoodbye\n"
	if _, err := st.CommitFile(absPath, []byte(content), "A", "a@test.com", "add"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	// Verify it exists
	if _, err := st.ReadFile(absPath); err != nil {
		t.Fatalf("file should exist before delete: %v", err)
	}

	// Delete
	_, err := st.DeleteFile(absPath, "A", "a@test.com", "delete file")
	if err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	// Verify it's gone
	_, err = st.ReadFile(absPath)
	if err == nil {
		t.Error("ReadFile should error after delete")
	}
}

func TestDeleteFile_NonExistent(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	// Need HEAD to exist
	commitTestDoc(t, st, "documents/test/keep.md", "keep", "Keep", "body\n")

	absPath := filepath.Join(st.Root(), "documents", "test", "ghost.md")
	_, err := st.DeleteFile(absPath, "A", "a@test.com", "delete ghost")
	if err == nil {
		t.Error("expected error deleting non-existent file")
	}
}

func TestBranchLifecycle(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	docPath := filepath.Join(st.Root(), "documents", "policies", "branching.md")
	origContent := "---\ndocument_id: br-1\ntitle: Branch Test\nstatus: draft\n---\nOriginal body\n"
	if _, err := st.CommitFile(docPath, []byte(origContent), "A", "a@test.com", "initial"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	branchName := "suggestions/br-1/user1"
	branchContent := []byte("---\ndocument_id: br-1\ntitle: Branch Test\nstatus: draft\n---\nModified body from suggestion\n")

	// Create suggestion branch
	commitHash, err := st.CreateSuggestion(docPath, branchName, branchContent, "User1", "user1@test.com", "suggest changes")
	if err != nil {
		t.Fatalf("CreateSuggestion: %v", err)
	}
	if len(commitHash) != 40 {
		t.Errorf("expected 40-char hash, got %q", commitHash)
	}

	// Read from branch
	got, err := st.GetSuggestion(docPath, branchName)
	if err != nil {
		t.Fatalf("GetSuggestion: %v", err)
	}
	if string(got) != string(branchContent) {
		t.Errorf("branch content mismatch:\ngot:  %q\nwant: %q", got, branchContent)
	}

	// List branches
	branches, err := st.ListSuggestionBranches("suggestions/br-1/")
	if err != nil {
		t.Fatalf("ListSuggestionBranches: %v", err)
	}
	if len(branches) != 1 || branches[0] != branchName {
		t.Errorf("ListSuggestionBranches = %v, want [%q]", branches, branchName)
	}

	// Main branch should still have original content
	mainContent, err := st.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile from main: %v", err)
	}
	if string(mainContent) != origContent {
		t.Error("main branch content should not change after creating suggestion")
	}

	// Delete branch
	if err := st.DeleteSuggestionBranch(branchName); err != nil {
		t.Fatalf("DeleteSuggestionBranch: %v", err)
	}
	branches2, err := st.ListSuggestionBranches("suggestions/br-1/")
	if err != nil {
		t.Fatalf("ListSuggestionBranches after delete: %v", err)
	}
	if len(branches2) != 0 {
		t.Errorf("expected 0 branches after delete, got %v", branches2)
	}
}

func TestMergeSuggestion(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	docPath := filepath.Join(st.Root(), "documents", "policies", "merge-test.md")
	origContent := "---\ndocument_id: mrg-1\ntitle: Merge Test\nstatus: draft\n---\nOriginal\n"
	if _, err := st.CommitFile(docPath, []byte(origContent), "A", "a@test.com", "initial"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	branchName := "suggestions/mrg-1/reviewer"
	newContent := []byte("---\ndocument_id: mrg-1\ntitle: Merge Test\nstatus: draft\n---\nImproved content after review\n")
	if _, err := st.CreateSuggestion(docPath, branchName, newContent, "Reviewer", "rev@test.com", "suggest improvements"); err != nil {
		t.Fatalf("CreateSuggestion: %v", err)
	}

	// Merge
	mergeHash, err := st.MergeSuggestion(docPath, branchName, "Manager", "mgr@test.com")
	if err != nil {
		t.Fatalf("MergeSuggestion: %v", err)
	}
	if len(mergeHash) != 40 {
		t.Errorf("expected 40-char hash from merge, got %q", mergeHash)
	}

	// Verify main now has the merged content
	got, err := st.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile after merge: %v", err)
	}
	if string(got) != string(newContent) {
		t.Errorf("after merge:\ngot:  %q\nwant: %q", got, newContent)
	}

	// Branch should be deleted after merge
	branches, _ := st.ListSuggestionBranches("suggestions/mrg-1/")
	if len(branches) != 0 {
		t.Errorf("suggestion branch should be deleted after merge, got %v", branches)
	}
}

func TestMergeSuggestion_ConflictDetection(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	docPath := filepath.Join(st.Root(), "documents", "policies", "conflict.md")
	origContent := "---\ndocument_id: cfl-1\ntitle: Conflict\nstatus: draft\n---\nOriginal\n"
	if _, err := st.CommitFile(docPath, []byte(origContent), "A", "a@test.com", "initial"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	baseHash, _ := st.HeadHash()

	// Create suggestion from this base
	branchName := "suggestions/cfl-1/user"
	sugContent := []byte("---\ndocument_id: cfl-1\ntitle: Conflict\nstatus: draft\n---\nSuggested change\n")
	if _, err := st.CreateSuggestion(docPath, branchName, sugContent, "User", "u@test.com", "suggest"); err != nil {
		t.Fatalf("CreateSuggestion: %v", err)
	}

	// Now modify the same file on main (simulating a concurrent edit)
	updatedContent := "---\ndocument_id: cfl-1\ntitle: Conflict\nstatus: draft\n---\nConcurrent edit on main\n"
	if _, err := st.CommitFile(docPath, []byte(updatedContent), "Other", "other@test.com", "concurrent edit"); err != nil {
		t.Fatalf("concurrent edit: %v", err)
	}

	// Merge should detect conflict when baseCommit is provided
	_, err := st.MergeSuggestion(docPath, branchName, "Mgr", "mgr@test.com", baseHash)
	if err != ErrConflict {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestValidateRepoContents(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	t.Run("valid contents", func(t *testing.T) {
		// Commit only allowed files
		commitTestDoc(t, st, "documents/policies/valid.md", "v1", "Valid", "body\n")

		err := st.ValidateRepoContents(2 * 1024 * 1024) // 2MB limit
		if err != nil {
			t.Errorf("expected valid repo, got: %v", err)
		}
	})

	t.Run("readme allowed", func(t *testing.T) {
		abs := filepath.Join(st.Root(), "README.md")
		if _, err := st.CommitFile(abs, []byte("# Readme"), "A", "a@test.com", "add readme"); err != nil {
			t.Fatalf("CommitFile: %v", err)
		}
		if err := st.ValidateRepoContents(2 * 1024 * 1024); err != nil {
			t.Errorf("README.md should be allowed: %v", err)
		}
	})

	t.Run("title file allowed", func(t *testing.T) {
		abs := filepath.Join(st.Root(), "documents", "policies", ".title")
		if _, err := st.CommitFile(abs, []byte("Policies"), "A", "a@test.com", "add .title"); err != nil {
			t.Fatalf("CommitFile: %v", err)
		}
		if err := st.ValidateRepoContents(2 * 1024 * 1024); err != nil {
			t.Errorf(".title should be allowed: %v", err)
		}
	})
}

func TestValidateRepoContents_SizeLimit(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	// Commit a file that's within the small limit
	smallContent := "---\ndocument_id: sz-1\ntitle: Small\nstatus: draft\n---\nSmall body\n"
	abs := filepath.Join(st.Root(), "documents", "test", "small.md")
	if _, err := st.CommitFile(abs, []byte(smallContent), "A", "a@test.com", "add small"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	// Validate with a very small limit that the file exceeds
	err := st.ValidateRepoContents(10) // 10 bytes
	if err == nil {
		t.Error("expected size limit error")
	}
	if err != nil && !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected 'too large' error, got: %v", err)
	}
}

func TestValidateFilePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"allowed doc", "documents/policies/test.md", false},
		{"allowed nested doc", "documents/controls/sub/test.md", false},
		{"allowed .title", "documents/policies/.title", false},
		{"allowed README", "README.md", false},
		{"disallowed root yaml", "config.yaml", true},
		{"disallowed binary", "documents/policies/test.pdf", true},
		{"disallowed traversal", "../etc/passwd", true},
		{"disallowed doc without ext", "documents/test/noext", true},
		{"disallowed bare .md in root", "test.md", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFilePath(tc.path)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %q", tc.path)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tc.path, err)
			}
		})
	}
}

func TestValidateFileSize(t *testing.T) {
	small := make([]byte, 100)
	if err := ValidateFileSize(small); err != nil {
		t.Errorf("100 bytes should pass: %v", err)
	}

	big := make([]byte, 3*1024*1024)
	if err := ValidateFileSize(big); err == nil {
		t.Error("3MB should fail")
	}
}

func TestBlameFile(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	content := "---\ndocument_id: blame-1\ntitle: Blame Test\nstatus: draft\n---\nLine one\nLine two\nLine three\n"
	absPath := filepath.Join(st.Root(), "documents", "test", "blame.md")
	if _, err := st.CommitFile(absPath, []byte(content), "Alice", "alice@test.com", "initial blame test"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	lines, err := st.BlameFile(absPath)
	if err != nil {
		t.Fatalf("BlameFile: %v", err)
	}

	// Blame strips frontmatter, so we should get the body lines
	if len(lines) == 0 {
		t.Fatal("expected at least one blame line")
	}

	// All lines should be attributed to Alice
	for i, l := range lines {
		if l.Author != "alice@test.com" {
			t.Errorf("line %d: author = %q, want %q", i, l.Author, "alice@test.com")
		}
		if len(l.Hash) != 8 {
			t.Errorf("line %d: hash should be 8 chars, got %q", i, l.Hash)
		}
	}

	// First body line should be "Line one"
	if len(lines) > 0 && strings.TrimSpace(lines[0].Text) != "Line one" {
		t.Errorf("first blame line text = %q, want %q", lines[0].Text, "Line one")
	}
}

func TestDiffFiles(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	absPath := filepath.Join(st.Root(), "documents", "test", "diff.md")

	// First commit
	v1 := "---\ndocument_id: diff-1\ntitle: Diff Test\nstatus: draft\n---\nOriginal line\n"
	h1, err := st.CommitFile(absPath, []byte(v1), "A", "a@test.com", "v1")
	if err != nil {
		t.Fatalf("first commit: %v", err)
	}

	// Second commit
	v2 := "---\ndocument_id: diff-1\ntitle: Diff Test\nstatus: draft\n---\nModified line\nNew line\n"
	h2, err := st.CommitFile(absPath, []byte(v2), "B", "b@test.com", "v2")
	if err != nil {
		t.Fatalf("second commit: %v", err)
	}

	diff, err := st.DiffFiles(h1, h2, "documents/test/diff.md")
	if err != nil {
		t.Fatalf("DiffFiles: %v", err)
	}

	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(diff, "---") || !strings.Contains(diff, "+++") {
		t.Error("diff should contain unified diff headers")
	}
	if !strings.Contains(diff, "-Original line") {
		t.Error("diff should show removed line")
	}
	if !strings.Contains(diff, "+Modified line") {
		t.Error("diff should show added line")
	}
}

func TestDiffFiles_EmptyFromRef(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	absPath := filepath.Join(st.Root(), "documents", "test", "diff2.md")

	v1 := "---\ndocument_id: d2\ntitle: D2\nstatus: draft\n---\nFirst\n"
	if _, err := st.CommitFile(absPath, []byte(v1), "A", "a@test.com", "v1"); err != nil {
		t.Fatalf("commit v1: %v", err)
	}

	v2 := "---\ndocument_id: d2\ntitle: D2\nstatus: draft\n---\nSecond\n"
	h2, err := st.CommitFile(absPath, []byte(v2), "B", "b@test.com", "v2")
	if err != nil {
		t.Fatalf("commit v2: %v", err)
	}

	// Empty fromRef should use parent of toRef
	diff, err := st.DiffFiles("", h2, "documents/test/diff2.md")
	if err != nil {
		t.Fatalf("DiffFiles with empty fromRef: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff when using parent as from")
	}
}

func TestDiffFiles_NoChange(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	absPath := filepath.Join(st.Root(), "documents", "test", "same.md")
	content := "---\ndocument_id: same\ntitle: Same\nstatus: draft\n---\nNo change\n"
	h1, err := st.CommitFile(absPath, []byte(content), "A", "a@test.com", "v1")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Commit same content again (tree will differ because of timestamp in commit)
	// Actually need another file to change HEAD
	abs2 := filepath.Join(st.Root(), "documents", "test", "other.md")
	h2, err := st.CommitFile(abs2, []byte("---\ndocument_id: oth\ntitle: Oth\nstatus: draft\n---\nother\n"), "A", "a@test.com", "add other")
	if err != nil {
		t.Fatalf("commit other: %v", err)
	}

	diff, err := st.DiffFiles(h1, h2, "documents/test/same.md")
	if err != nil {
		t.Fatalf("DiffFiles: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for unchanged file, got: %q", diff)
	}
}

func TestCommitFiles_Batch(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	files := []FileEntry{
		{Path: "documents/policies/batch1.md", Content: []byte("---\ndocument_id: b1\ntitle: B1\nstatus: draft\n---\nBatch 1\n")},
		{Path: "documents/policies/batch2.md", Content: []byte("---\ndocument_id: b2\ntitle: B2\nstatus: draft\n---\nBatch 2\n")},
		{Path: "documents/controls/batch3.md", Content: []byte("---\ndocument_id: b3\ntitle: B3\nstatus: draft\n---\nBatch 3\n")},
	}

	hash, err := st.CommitFiles(files, "A", "a@test.com", "batch commit")
	if err != nil {
		t.Fatalf("CommitFiles: %v", err)
	}
	if len(hash) != 40 {
		t.Errorf("expected 40-char hash, got %q", hash)
	}

	// Read all three files back
	for _, f := range files {
		abs := filepath.Join(st.Root(), f.Path)
		got, err := st.ReadFile(abs)
		if err != nil {
			t.Errorf("ReadFile %s: %v", f.Path, err)
			continue
		}
		if string(got) != string(f.Content) {
			t.Errorf("content mismatch for %s:\ngot:  %q\nwant: %q", f.Path, got, f.Content)
		}
	}
}

func TestCommitFile_ExpectedHead_Conflict(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	abs := filepath.Join(st.Root(), "documents", "test", "conflict.md")
	content := "---\ndocument_id: cf\ntitle: CF\nstatus: draft\n---\nBody\n"
	if _, err := st.CommitFile(abs, []byte(content), "A", "a@test.com", "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	oldHead, _ := st.HeadHash()

	// Make another commit to advance HEAD
	if _, err := st.CommitFile(abs, []byte(content+"Updated\n"), "B", "b@test.com", "update"); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Now try to commit with the old HEAD — should fail
	_, err := st.CommitFile(abs, []byte(content+"Stale\n"), "C", "c@test.com", "stale", oldHead)
	if err != ErrConflict {
		t.Errorf("expected ErrConflict, got: %v", err)
	}
}

func TestSearch(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	commitTestDoc(t, st, "documents/policies/search-test.md", "srch-1", "Search Test Policy", "This document covers data protection requirements.\n")
	commitTestDoc(t, st, "documents/controls/other.md", "srch-2", "Other Control", "Unrelated content about firewalls.\n")

	t.Run("finds matching document", func(t *testing.T) {
		results := st.Search("data protection", 10)
		if len(results) == 0 {
			t.Fatal("expected at least one search result")
		}
		found := false
		for _, r := range results {
			if r.DocumentID == "srch-1" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find srch-1 in results")
		}
	})

	t.Run("no match", func(t *testing.T) {
		results := st.Search("quantum entanglement", 10)
		if len(results) != 0 {
			t.Errorf("expected no results, got %d", len(results))
		}
	})

	t.Run("short query ignored", func(t *testing.T) {
		results := st.Search("a", 10)
		if len(results) != 0 {
			t.Errorf("single char query should return nil, got %d results", len(results))
		}
	})

	t.Run("empty query", func(t *testing.T) {
		results := st.Search("", 10)
		if results != nil {
			t.Errorf("empty query should return nil")
		}
	})
}

func TestStripFrontmatter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "with frontmatter",
			input: "---\ntitle: Test\n---\nBody content",
			want:  "Body content",
		},
		{
			name:  "no frontmatter",
			input: "Just plain text",
			want:  "Just plain text",
		},
		{
			name:  "unclosed frontmatter",
			input: "---\ntitle: Test\nNo closing separator",
			want:  "---\ntitle: Test\nNo closing separator",
		},
		{
			name:  "empty body",
			input: "---\ntitle: Test\n---\n",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StripFrontmatter(tc.input)
			if got != tc.want {
				t.Errorf("StripFrontmatter(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestReadFileAtRef(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	absPath := filepath.Join(st.Root(), "documents", "test", "versioned.md")

	v1 := "---\ndocument_id: ver\ntitle: V\nstatus: draft\n---\nVersion 1\n"
	h1, err := st.CommitFile(absPath, []byte(v1), "A", "a@test.com", "v1")
	if err != nil {
		t.Fatalf("commit v1: %v", err)
	}

	v2 := "---\ndocument_id: ver\ntitle: V\nstatus: draft\n---\nVersion 2\n"
	if _, err := st.CommitFile(absPath, []byte(v2), "A", "a@test.com", "v2"); err != nil {
		t.Fatalf("commit v2: %v", err)
	}

	// Read at old ref
	got, err := st.ReadFileAtRef(h1, "documents/test/versioned.md")
	if err != nil {
		t.Fatalf("ReadFileAtRef: %v", err)
	}
	if string(got) != v1 {
		t.Errorf("ReadFileAtRef(h1) = %q, want %q", got, v1)
	}

	// Read at HEAD
	head, _ := st.HeadHash()
	got2, err := st.ReadFileAtRef(head, "documents/test/versioned.md")
	if err != nil {
		t.Fatalf("ReadFileAtRef at HEAD: %v", err)
	}
	if string(got2) != v2 {
		t.Errorf("ReadFileAtRef(HEAD) = %q, want %q", got2, v2)
	}
}

func TestRecentlyChanged(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	commitTestDoc(t, st, "documents/policies/recent1.md", "rc-1", "Recent 1", "body 1\n")
	commitTestDoc(t, st, "documents/controls/recent2.md", "rc-2", "Recent 2", "body 2\n")

	changed := st.RecentlyChanged(10)
	if len(changed) < 2 {
		t.Fatalf("expected at least 2 changed files, got %d", len(changed))
	}

	// Verify fields are populated
	for _, c := range changed {
		if c.Path == "" {
			t.Error("ChangedFile.Path should not be empty")
		}
		if c.Folder == "" {
			t.Error("ChangedFile.Folder should not be empty")
		}
		if c.CommitHash == "" {
			t.Error("ChangedFile.CommitHash should not be empty")
		}
	}
}

func TestFileLastCommit(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	absPath := filepath.Join(st.Root(), "documents", "test", "last-commit.md")
	content := "---\ndocument_id: lc\ntitle: LC\nstatus: draft\n---\nbody\n"
	if _, err := st.CommitFile(absPath, []byte(content), "CommitAuthor", "author@test.com", "test last commit"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	hash, commitTime, message, author, err := st.FileLastCommit("documents/test/last-commit.md")
	if err != nil {
		t.Fatalf("FileLastCommit: %v", err)
	}
	if len(hash) != 40 {
		t.Errorf("hash should be 40 chars, got %q", hash)
	}
	if commitTime.IsZero() {
		t.Error("commitTime should not be zero")
	}
	if message != "test last commit" {
		t.Errorf("message = %q, want %q", message, "test last commit")
	}
	if author != "author@test.com" {
		t.Errorf("author = %q, want %q", author, "author@test.com")
	}
}

func TestDeleteDirectory(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	// Create a folder with multiple docs
	commitTestDoc(t, st, "documents/obsolete/doc1.md", "obs-1", "Obs 1", "body\n")
	commitTestDoc(t, st, "documents/obsolete/doc2.md", "obs-2", "Obs 2", "body\n")
	// Also keep a doc in a different folder
	commitTestDoc(t, st, "documents/active/doc3.md", "act-1", "Active", "body\n")

	// Delete the obsolete folder
	dirPath := filepath.Join(st.Root(), "documents", "obsolete")
	_, err := st.DeleteDirectory(dirPath, "Admin", "admin@test.com", "remove obsolete folder")
	if err != nil {
		t.Fatalf("DeleteDirectory: %v", err)
	}

	// Verify files are gone
	_, err = st.ReadFile(filepath.Join(st.Root(), "documents", "obsolete", "doc1.md"))
	if err == nil {
		t.Error("doc1.md should be gone after directory delete")
	}

	// Active folder should still exist
	got, err := st.ReadFile(filepath.Join(st.Root(), "documents", "active", "doc3.md"))
	if err != nil {
		t.Fatalf("active doc should still exist: %v", err)
	}
	if !strings.Contains(string(got), "act-1") {
		t.Error("active doc content should be intact")
	}
}

func TestUpdateDocumentMetadata(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	absPath := filepath.Join(st.Root(), "documents", "test", "meta.md")
	content := "---\ndocument_id: meta-1\ntitle: Original Title\nstatus: draft\n---\nBody text\n"
	if _, err := st.CommitFile(absPath, []byte(content), "A", "a@test.com", "init"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	// Update status
	_, err := st.UpdateDocumentMetadata(absPath, "status", "approved", "Admin", "admin@test.com")
	if err != nil {
		t.Fatalf("UpdateDocumentMetadata: %v", err)
	}

	// Read and verify
	doc, err := st.LoadDocument(absPath)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}
	if doc.Frontmatter.Status != "approved" {
		t.Errorf("status = %q, want %q", doc.Frontmatter.Status, "approved")
	}
	// Title should be unchanged
	if doc.Frontmatter.Title != "Original Title" {
		t.Errorf("title should be unchanged, got %q", doc.Frontmatter.Title)
	}
}

func TestUpdateDocumentMetadataMulti(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	absPath := filepath.Join(st.Root(), "documents", "test", "multi.md")
	content := "---\ndocument_id: mm-1\ntitle: Multi\nstatus: draft\n---\nBody\n"
	if _, err := st.CommitFile(absPath, []byte(content), "A", "a@test.com", "init"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	fields := map[string]string{
		"status": "approved",
		"owner":  "security-team",
	}
	_, err := st.UpdateDocumentMetadataMulti(absPath, fields, "Admin", "admin@test.com")
	if err != nil {
		t.Fatalf("UpdateDocumentMetadataMulti: %v", err)
	}

	doc, err := st.LoadDocument(absPath)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}
	if doc.Frontmatter.Status != "approved" {
		t.Errorf("status = %q, want %q", doc.Frontmatter.Status, "approved")
	}
	if doc.Frontmatter.Owner != "security-team" {
		t.Errorf("owner = %q, want %q", doc.Frontmatter.Owner, "security-team")
	}
}

func TestValidateUniqueDocumentIDs(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	t.Run("unique IDs", func(t *testing.T) {
		commitTestDoc(t, st, "documents/a/d1.md", "unique-1", "D1", "body\n")
		commitTestDoc(t, st, "documents/a/d2.md", "unique-2", "D2", "body\n")

		dupes := st.ValidateUniqueDocumentIDs()
		if dupes != nil {
			t.Errorf("expected no duplicates, got %v", dupes)
		}
	})

	t.Run("duplicate IDs", func(t *testing.T) {
		// Commit another doc with the same ID as an existing one
		commitTestDoc(t, st, "documents/b/dup.md", "unique-1", "Duplicate", "body\n")

		dupes := st.ValidateUniqueDocumentIDs()
		if dupes == nil {
			t.Fatal("expected duplicates to be detected")
		}
		if paths, ok := dupes["unique-1"]; !ok || len(paths) < 2 {
			t.Errorf("expected duplicate paths for unique-1, got %v", dupes)
		}
	})
}

func TestDiffSuggestion(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	docPath := filepath.Join(st.Root(), "documents", "test", "suggest-diff.md")
	origContent := "---\ndocument_id: sd-1\ntitle: Suggest Diff\nstatus: draft\n---\nOriginal body text\n"
	if _, err := st.CommitFile(docPath, []byte(origContent), "A", "a@test.com", "init"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	branchName := "suggestions/sd-1/reviewer"
	sugContent := []byte("---\ndocument_id: sd-1\ntitle: Suggest Diff\nstatus: draft\n---\nImproved body text with more detail\n")
	if _, err := st.CreateSuggestion(docPath, branchName, sugContent, "Rev", "rev@test.com", "suggest"); err != nil {
		t.Fatalf("CreateSuggestion: %v", err)
	}

	diff, err := st.DiffSuggestion(docPath, branchName)
	if err != nil {
		t.Fatalf("DiffSuggestion: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(diff, "-Original body text") || !strings.Contains(diff, "+Improved body text") {
		t.Errorf("diff should show body changes:\n%s", diff)
	}
}

func TestDiffDocumentBodies(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	absPath := filepath.Join(st.Root(), "documents", "test", "body-diff.md")

	v1 := "---\ndocument_id: bd-1\ntitle: Body Diff\nstatus: draft\n---\nOld body content\n"
	h1, err := st.CommitFile(absPath, []byte(v1), "A", "a@test.com", "v1")
	if err != nil {
		t.Fatalf("commit v1: %v", err)
	}

	v2 := "---\ndocument_id: bd-1\ntitle: Body Diff Updated\nstatus: approved\n---\nNew body content\n"
	h2, err := st.CommitFile(absPath, []byte(v2), "B", "b@test.com", "v2")
	if err != nil {
		t.Fatalf("commit v2: %v", err)
	}

	diff, err := st.DiffDocumentBodies(h1, h2, "documents/test/body-diff.md")
	if err != nil {
		t.Fatalf("DiffDocumentBodies: %v", err)
	}

	// Should show body changes but NOT frontmatter changes
	if strings.Contains(diff, "document_id") {
		t.Error("DiffDocumentBodies should strip frontmatter from diff")
	}
	if !strings.Contains(diff, "-Old body content") || !strings.Contains(diff, "+New body content") {
		t.Errorf("diff should show body changes:\n%s", diff)
	}
}

func TestSaveDocument_FilesystemMode(t *testing.T) {
	// SaveDocument writes to filesystem, not git — test with New() (filesystem mode)
	dir := t.TempDir()
	st := New(dir)

	docPath := filepath.Join(dir, "documents", "policies", "fs-test.md")
	doc := &DocumentFile{
		Path: docPath,
		Frontmatter: model.DocumentFrontmatter{
			DocumentID: "fs-1",
			Title:      "Filesystem Test",
			Status:     "draft",
		},
		Body: "\n# Test\n\nBody content.\n",
	}

	if err := st.SaveDocument(doc); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}

	// Load it back (filesystem mode)
	got, err := st.LoadDocument(docPath)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}
	if got.Frontmatter.DocumentID != "fs-1" {
		t.Errorf("DocumentID = %q, want %q", got.Frontmatter.DocumentID, "fs-1")
	}
	if got.Frontmatter.Title != "Filesystem Test" {
		t.Errorf("Title = %q, want %q", got.Frontmatter.Title, "Filesystem Test")
	}
	if !strings.Contains(got.Body, "Body content.") {
		t.Errorf("Body should contain content, got %q", got.Body)
	}
}

func TestHeadCommit(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	commitTestDoc(t, st, "documents/test/hc.md", "hc-1", "HC", "body\n")

	commit, err := st.HeadCommit()
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}
	if commit.Author.Name != "Test User" {
		t.Errorf("Author.Name = %q, want %q", commit.Author.Name, "Test User")
	}
	if commit.Author.Email != "test@example.com" {
		t.Errorf("Author.Email = %q, want %q", commit.Author.Email, "test@example.com")
	}
}

func TestResetToCommit(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	absPath := filepath.Join(st.Root(), "documents", "test", "reset.md")
	v1 := "---\ndocument_id: rst\ntitle: Reset\nstatus: draft\n---\nVersion 1\n"
	h1, err := st.CommitFile(absPath, []byte(v1), "A", "a@test.com", "v1")
	if err != nil {
		t.Fatalf("commit v1: %v", err)
	}

	v2 := "---\ndocument_id: rst\ntitle: Reset\nstatus: draft\n---\nVersion 2\n"
	if _, err := st.CommitFile(absPath, []byte(v2), "B", "b@test.com", "v2"); err != nil {
		t.Fatalf("commit v2: %v", err)
	}

	// Reset back to v1
	if err := st.ResetToCommit(h1); err != nil {
		t.Fatalf("ResetToCommit: %v", err)
	}

	// HEAD should now point to h1
	head, err := st.HeadHash()
	if err != nil {
		t.Fatalf("HeadHash: %v", err)
	}
	if head != h1 {
		t.Errorf("HEAD = %q after reset, want %q", head, h1)
	}

	// File content should be v1
	got, err := st.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile after reset: %v", err)
	}
	if string(got) != v1 {
		t.Errorf("content after reset = %q, want %q", got, v1)
	}
}

func TestCommitsSince(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	absPath := filepath.Join(st.Root(), "documents", "test", "since.md")
	v1 := "---\ndocument_id: snc\ntitle: Since\nstatus: draft\n---\nV1\n"
	h1, err := st.CommitFile(absPath, []byte(v1), "A", "a@test.com", "first version")
	if err != nil {
		t.Fatalf("commit v1: %v", err)
	}

	v2 := "---\ndocument_id: snc\ntitle: Since\nstatus: draft\n---\nV2\n"
	if _, err := st.CommitFile(absPath, []byte(v2), "B", "b@test.com", "second version"); err != nil {
		t.Fatalf("commit v2: %v", err)
	}

	v3 := "---\ndocument_id: snc\ntitle: Since\nstatus: draft\n---\nV3\n"
	if _, err := st.CommitFile(absPath, []byte(v3), "C", "c@test.com", "third version"); err != nil {
		t.Fatalf("commit v3: %v", err)
	}

	messages := st.CommitsSince("documents/test/since.md", h1)
	if len(messages) != 2 {
		t.Fatalf("expected 2 commits since h1, got %d: %v", len(messages), messages)
	}
	// Should include "second version" and "third version" but not "first version"
	for _, m := range messages {
		if m == "first version" {
			t.Error("should not include the 'since' commit itself")
		}
	}
}

func TestQuoteYAMLValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"has: colon", "\"has: colon\""},
		{"has {braces}", "\"has {braces}\""},
		{"plain text", "plain text"},
		{"with #comment", "\"with #comment\""},
	}

	for _, tc := range tests {
		got := quoteYAMLValue(tc.input)
		if got != tc.want {
			t.Errorf("quoteYAMLValue(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseFrontmatter(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		input := "---\ntitle: Hello\nstatus: draft\n---\nBody here"
		fm, body, err := parseFrontmatter(input)
		if err != nil {
			t.Fatalf("parseFrontmatter: %v", err)
		}
		if !strings.Contains(fm, "title: Hello") {
			t.Errorf("frontmatter should contain title, got %q", fm)
		}
		if !strings.Contains(body, "Body here") {
			t.Errorf("body should contain text, got %q", body)
		}
	})

	t.Run("no frontmatter", func(t *testing.T) {
		input := "Just plain markdown"
		_, _, err := parseFrontmatter(input)
		if err == nil {
			t.Error("expected error for content without frontmatter")
		}
	})

	t.Run("empty body", func(t *testing.T) {
		input := "---\ntitle: Test\n---\n"
		fm, body, err := parseFrontmatter(input)
		if err != nil {
			t.Fatalf("parseFrontmatter: %v", err)
		}
		if !strings.Contains(fm, "title: Test") {
			t.Errorf("frontmatter = %q", fm)
		}
		// Empty body after frontmatter, the trailing newline produces one empty scan
		_ = body
	})
}

func TestReadDir(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	commitTestDoc(t, st, "documents/policies/p1.md", "rd-1", "P1", "body\n")
	commitTestDoc(t, st, "documents/policies/p2.md", "rd-2", "P2", "body\n")

	entries, err := st.ReadDir(filepath.Join(st.Root(), "documents", "policies"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
		if e.IsDir() {
			t.Errorf("entry %q should not be a directory", e.Name())
		}
	}
	if !names["p1.md"] || !names["p2.md"] {
		t.Errorf("expected p1.md and p2.md, got %v", names)
	}
}

func TestReadDir_Root(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	commitTestDoc(t, st, "documents/test/doc.md", "rd-root", "Doc", "body\n")
	abs := filepath.Join(st.Root(), "README.md")
	if _, err := st.CommitFile(abs, []byte("# Readme"), "A", "a@test.com", "add readme"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	entries, err := st.ReadDir(st.Root())
	if err != nil {
		t.Fatalf("ReadDir root: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 root entries (documents + README.md), got %d", len(entries))
	}

	foundDocs := false
	foundReadme := false
	for _, e := range entries {
		if e.Name() == "documents" && e.IsDir() {
			foundDocs = true
		}
		if e.Name() == "README.md" && !e.IsDir() {
			foundReadme = true
		}
	}
	if !foundDocs {
		t.Error("expected 'documents' directory at root")
	}
	if !foundReadme {
		t.Error("expected 'README.md' at root")
	}
}

func TestMultipleBranches(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	docPath := filepath.Join(st.Root(), "documents", "test", "multi-branch.md")
	orig := "---\ndocument_id: mb-1\ntitle: Multi Branch\nstatus: draft\n---\nOriginal\n"
	if _, err := st.CommitFile(docPath, []byte(orig), "A", "a@test.com", "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create two suggestion branches for the same doc
	sug1 := []byte("---\ndocument_id: mb-1\ntitle: Multi Branch\nstatus: draft\n---\nSuggestion from user1\n")
	if _, err := st.CreateSuggestion(docPath, "suggestions/mb-1/user1", sug1, "U1", "u1@test.com", "sug1"); err != nil {
		t.Fatalf("CreateSuggestion 1: %v", err)
	}

	sug2 := []byte("---\ndocument_id: mb-1\ntitle: Multi Branch\nstatus: draft\n---\nSuggestion from user2\n")
	if _, err := st.CreateSuggestion(docPath, "suggestions/mb-1/user2", sug2, "U2", "u2@test.com", "sug2"); err != nil {
		t.Fatalf("CreateSuggestion 2: %v", err)
	}

	branches, err := st.ListSuggestionBranches("suggestions/mb-1/")
	if err != nil {
		t.Fatalf("ListSuggestionBranches: %v", err)
	}
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d: %v", len(branches), branches)
	}

	// Each branch should have its own content
	c1, _ := st.GetSuggestion(docPath, "suggestions/mb-1/user1")
	c2, _ := st.GetSuggestion(docPath, "suggestions/mb-1/user2")
	if string(c1) == string(c2) {
		t.Error("branch contents should differ")
	}
}

func TestCreateSuggestion_UpdateExisting(t *testing.T) {
	dir := t.TempDir()
	st := initBareStore(t, dir)

	docPath := filepath.Join(st.Root(), "documents", "test", "update-sug.md")
	orig := "---\ndocument_id: us-1\ntitle: Update Sug\nstatus: draft\n---\nOriginal\n"
	if _, err := st.CommitFile(docPath, []byte(orig), "A", "a@test.com", "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	branchName := "suggestions/us-1/editor"

	// First suggestion
	v1 := []byte("---\ndocument_id: us-1\ntitle: Update Sug\nstatus: draft\n---\nFirst draft of suggestion\n")
	if _, err := st.CreateSuggestion(docPath, branchName, v1, "E", "e@test.com", "first draft"); err != nil {
		t.Fatalf("first CreateSuggestion: %v", err)
	}

	// Update the suggestion (same branch)
	v2 := []byte("---\ndocument_id: us-1\ntitle: Update Sug\nstatus: draft\n---\nRevised suggestion\n")
	if _, err := st.CreateSuggestion(docPath, branchName, v2, "E", "e@test.com", "revised"); err != nil {
		t.Fatalf("second CreateSuggestion: %v", err)
	}

	// Should read the latest version
	got, err := st.GetSuggestion(docPath, branchName)
	if err != nil {
		t.Fatalf("GetSuggestion: %v", err)
	}
	if string(got) != string(v2) {
		t.Errorf("suggestion should be updated:\ngot:  %q\nwant: %q", got, v2)
	}
}
