package store

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/crypto/ssh"
)

// ErrConflict is returned when a concurrent modification is detected (HEAD changed).
var ErrConflict = fmt.Errorf("conflict: document was modified by another user")

// NewBare opens a bare git repository and returns a Store that reads
// files directly from git objects (HEAD commit tree) instead of the filesystem.
func NewBare(repoPath string) (*Store, error) {
	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("opening bare repo %s: %w", repoPath, err)
	}
	return &Store{root: repoPath, repo: r}, nil
}

// headTree returns the tree object for the HEAD commit.
func (s *Store) headTree() (*object.Tree, error) {
	ref, err := s.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("resolving HEAD: %w", err)
	}
	commit, err := s.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("reading HEAD commit: %w", err)
	}
	return commit.Tree()
}

// relPath converts an absolute path to a path relative to the store root.
// For bare repos, paths like /data/org.git/folder/foo.md become folder/foo.md.
func (s *Store) relPath(path string) string {
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return path
	}
	// go-git uses forward slashes
	return filepath.ToSlash(rel)
}

// readFile reads a file either from the git tree (bare mode) or the filesystem.
func (s *Store) readFile(path string) ([]byte, error) {
	if s.repo == nil {
		return os.ReadFile(path)
	}
	tree, err := s.headTree()
	if err != nil {
		return nil, err
	}
	f, err := tree.File(s.relPath(path))
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	rc, err := f.Reader()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// dirEntry is a minimal fs.DirEntry for git tree entries.
type dirEntry struct {
	name  string
	isDir bool
}

func (d dirEntry) Name() string { return d.name }
func (d dirEntry) IsDir() bool  { return d.isDir }
func (d dirEntry) Type() fs.FileMode {
	if d.isDir {
		return fs.ModeDir
	}
	return 0
}
func (d dirEntry) Info() (fs.FileInfo, error) { return nil, fmt.Errorf("not supported") }

// readDir lists entries in a directory from the git tree or filesystem.
func (s *Store) readDir(path string) ([]fs.DirEntry, error) {
	if s.repo == nil {
		return os.ReadDir(path)
	}
	tree, err := s.headTree()
	if err != nil {
		return nil, err
	}
	rel := s.relPath(path)

	// If rel is "." we use the root tree; otherwise navigate to subtree.
	var target *object.Tree
	if rel == "." {
		target = tree
	} else {
		target, err = tree.Tree(rel)
		if err != nil {
			return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
		}
	}

	var entries []fs.DirEntry
	for _, e := range target.Entries {
		entries = append(entries, dirEntry{
			name:  e.Name,
			isDir: e.Mode.IsFile() == false,
		})
	}
	return entries, nil
}

// walkDir walks a directory tree, calling fn for each entry.
// In bare repo mode it walks the git tree; otherwise uses filepath.WalkDir.
func (s *Store) walkDir(root string, fn fs.WalkDirFunc) error {
	if s.repo == nil {
		return filepath.WalkDir(root, fn)
	}
	tree, err := s.headTree()
	if err != nil {
		return err
	}
	rel := s.relPath(root)

	var target *object.Tree
	if rel == "." {
		target = tree
	} else {
		target, err = tree.Tree(rel)
		if err != nil {
			// Directory doesn't exist — call fn with the error like filepath.WalkDir does.
			return fn(root, nil, &os.PathError{Op: "open", Path: root, Err: os.ErrNotExist})
		}
	}

	return s.walkGitTree(target, root, fn)
}

// walkGitTree recursively walks a git tree, calling fn for each entry.
func (s *Store) walkGitTree(tree *object.Tree, base string, fn fs.WalkDirFunc) error {
	// First call fn for the directory itself.
	de := dirEntry{name: filepath.Base(base), isDir: true}
	if err := fn(base, de, nil); err != nil {
		if err == filepath.SkipDir {
			return nil
		}
		return err
	}

	for _, entry := range tree.Entries {
		fullPath := filepath.Join(base, entry.Name)
		if entry.Mode.IsFile() {
			de := dirEntry{name: entry.Name, isDir: false}
			if err := fn(fullPath, de, nil); err != nil {
				if err == filepath.SkipDir || err == filepath.SkipAll {
					return nil
				}
				return err
			}
		} else {
			subtree, err := tree.Tree(entry.Name)
			if err != nil {
				// Report the error via fn.
				if err2 := fn(fullPath, nil, err); err2 != nil {
					return err2
				}
				continue
			}
			if err := s.walkGitTree(subtree, fullPath, fn); err != nil {
				if err == filepath.SkipDir {
					continue
				}
				return err
			}
		}
	}
	return nil
}

// ReadFile is the exported version of readFile for use by API handlers
// that need to read raw file content through the store.
func (s *Store) ReadFile(path string) ([]byte, error) {
	return s.readFile(path)
}

// ReadDir is the exported version of readDir for use by API handlers.
func (s *Store) ReadDir(path string) ([]fs.DirEntry, error) {
	return s.readDir(path)
}

// WalkDir is the exported version of walkDir for use by API handlers.
func (s *Store) WalkDir(root string, fn fs.WalkDirFunc) error {
	return s.walkDir(root, fn)
}

// ValidateRepoContents walks the committed git tree and enforces repo policy:
// - Only documents/**/*.md, documents/**/.title, and README.md are allowed
// - No files larger than maxSize bytes
// - No symlinks
// - No executable files
// This reads the actual committed content (git tree objects), not the filesystem,
// which is critical for bare repos where there is no working tree.
func (s *Store) ValidateRepoContents(maxSize int64) error {
	tree, err := s.headTree()
	if err != nil {
		return fmt.Errorf("reading HEAD tree: %w", err)
	}
	return s.validateTree(tree, "", maxSize)
}

func (s *Store) validateTree(tree *object.Tree, prefix string, maxSize int64) error {
	for _, entry := range tree.Entries {
		rel := entry.Name
		if prefix != "" {
			rel = prefix + "/" + entry.Name
		}

		switch entry.Mode {
		case filemode.Dir:
			subtree, err := tree.Tree(entry.Name)
			if err != nil {
				return fmt.Errorf("reading subtree %s: %w", rel, err)
			}
			if err := s.validateTree(subtree, rel, maxSize); err != nil {
				return err
			}
			continue
		case filemode.Symlink:
			return fmt.Errorf("symlink not allowed: %s", rel)
		case filemode.Executable:
			return fmt.Errorf("executable file not allowed: %s", rel)
		case filemode.Submodule:
			return fmt.Errorf("submodule not allowed: %s", rel)
		}

		// Path allowlist
		allowed := false
		if strings.HasPrefix(rel, "documents/") && strings.HasSuffix(rel, ".md") {
			allowed = true
		} else if strings.HasPrefix(rel, "documents/") && filepath.Base(rel) == ".title" {
			allowed = true
		} else if rel == "README.md" {
			allowed = true
		}
		if !allowed {
			return fmt.Errorf("file not allowed by repo policy: %s", rel)
		}

		// Size check via blob object
		blob, err := s.repo.BlobObject(entry.Hash)
		if err != nil {
			return fmt.Errorf("reading blob for %s: %w", rel, err)
		}
		if blob.Size > maxSize {
			return fmt.Errorf("file too large (%d bytes, max %d): %s", blob.Size, maxSize, rel)
		}
	}
	return nil
}

// ValidateUniqueDocumentIDs scans all .md files under documents/ and returns
// any duplicate document_id values. Returns nil if all IDs are unique.
func (s *Store) ValidateUniqueDocumentIDs() map[string][]string {
	docsRoot := s.DocsRoot()
	seen := make(map[string][]string) // document_id -> list of file paths

	s.walkDir(docsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		pf, loadErr := s.LoadDocument(path)
		if loadErr != nil || pf == nil || pf.Frontmatter.DocumentID == "" {
			return nil
		}
		relPath, _ := filepath.Rel(docsRoot, path)
		seen[pf.Frontmatter.DocumentID] = append(seen[pf.Frontmatter.DocumentID], relPath)
		return nil
	})

	// Filter to only duplicates
	dupes := make(map[string][]string)
	for id, paths := range seen {
		if len(paths) > 1 {
			dupes[id] = paths
		}
	}
	if len(dupes) == 0 {
		return nil
	}
	return dupes
}

// FindDocumentByID returns the absolute file path for a document_id.
// Uses a cached index (document_id → path) built from git tree, invalidated on HEAD change.
func (s *Store) FindDocumentByID(docID string) string {
	idx := s.getDocIndex()
	return idx[strings.ToLower(docID)]
}

// HasDocumentID returns true if a document with this ID already exists (case-insensitive).
func (s *Store) HasDocumentID(docID string) bool {
	return s.FindDocumentByID(docID) != ""
}

// docIndex caches document_id → absolute path mappings.
// Rebuilt when HEAD changes.
func (s *Store) getDocIndex() map[string]string {
	s.docIndexMu.Lock()
	defer s.docIndexMu.Unlock()

	currentHead := ""
	if h, err := s.HeadHash(); err == nil {
		currentHead = h
	}

	if s.docIndex != nil && s.docIndexHead == currentHead {
		return s.docIndex
	}

	// Rebuild index
	idx := make(map[string]string)
	docsRoot := s.DocsRoot()
	needle := []byte("document_id:")

	s.walkDir(docsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		raw, readErr := s.readFile(path)
		if readErr != nil || len(raw) == 0 {
			return nil
		}
		// Extract document_id from frontmatter without full YAML parse
		// Find "document_id:" line in first 500 bytes
		end := len(raw)
		if end > 500 {
			end = 500
		}
		chunk := raw[:end]
		pos := bytes.Index(chunk, needle)
		if pos < 0 {
			return nil
		}
		// Extract value after "document_id: "
		lineStart := pos + len(needle)
		lineEnd := bytes.IndexByte(chunk[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(chunk) - lineStart
		}
		val := strings.TrimSpace(string(chunk[lineStart : lineStart+lineEnd]))
		val = strings.Trim(val, "\"' ")
		if val != "" {
			idx[strings.ToLower(val)] = path
		}
		return nil
	})

	s.docIndex = idx
	s.docIndexHead = currentHead
	return idx
}

// HeadHash returns the HEAD commit hash as a hex string.
func (s *Store) HeadHash() (string, error) {
	if s.repo == nil {
		return "", fmt.Errorf("not a bare repo store")
	}
	ref, err := s.repo.Head()
	if err != nil {
		return "", fmt.Errorf("resolving HEAD: %w", err)
	}
	return ref.Hash().String(), nil
}

// ListRefHashes returns every ref in the repo as full-name → commit-hash. Used
// to snapshot refs around a push so server-managed refs (review/*) and history
// rewrites can be detected and reverted.
func (s *Store) ListRefHashes() (map[string]string, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("not a bare repo store")
	}
	out := map[string]string{}
	iter, err := s.repo.References()
	if err != nil {
		return nil, err
	}
	err = iter.ForEach(func(r *plumbing.Reference) error {
		if r.Type() == plumbing.HashReference {
			out[r.Name().String()] = r.Hash().String()
		}
		return nil
	})
	return out, err
}

// SetRefHash points a ref at a commit hash (creating it if absent).
func (s *Store) SetRefHash(name, hash string) error {
	if s.repo == nil {
		return fmt.Errorf("not a bare repo store")
	}
	return s.repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(name), plumbing.NewHash(hash)))
}

// DeleteRef removes a ref.
func (s *Store) DeleteRef(name string) error {
	if s.repo == nil {
		return fmt.Errorf("not a bare repo store")
	}
	return s.repo.Storer.RemoveReference(plumbing.ReferenceName(name))
}

// IsAncestor reports whether `ancestor` is an ancestor of `descendant` — i.e. the
// update old→new is a fast-forward (no history rewrite).
func (s *Store) IsAncestor(ancestor, descendant string) (bool, error) {
	if s.repo == nil {
		return false, fmt.Errorf("not a bare repo store")
	}
	a, err := s.repo.CommitObject(plumbing.NewHash(ancestor))
	if err != nil {
		return false, err
	}
	d, err := s.repo.CommitObject(plumbing.NewHash(descendant))
	if err != nil {
		return false, err
	}
	return a.IsAncestor(d)
}

// RefHash resolves a ref (branch name, tag, commit hash, HEAD~N) to its commit
// hash. Used to anchor a review's proposed-revision commit in the decision log
// so the per-event diff can be reconstructed later (even once the branch ref is
// archived out of refs/heads).
func (s *Store) RefHash(ref string) (string, error) {
	commit, err := s.resolveRef(ref)
	if err != nil {
		return "", err
	}
	return commit.Hash.String(), nil
}

// HeadCommit returns the HEAD commit object.
func (s *Store) HeadCommit() (*object.Commit, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("not a bare repo store")
	}
	ref, err := s.repo.Head()
	if err != nil {
		return nil, err
	}
	return s.repo.CommitObject(ref.Hash())
}

// ResetToCommit resets HEAD to a specific commit (used to revert rejected pushes).
func (s *Store) ResetToCommit(hash string) error {
	if s.repo == nil {
		return fmt.Errorf("not a bare repo store")
	}
	h := plumbing.NewHash(hash)
	ref := plumbing.NewHashReference(plumbing.HEAD, h)
	// For bare repos, update the branch that HEAD points to
	headRef, err := s.repo.Head()
	if err != nil {
		return s.repo.Storer.SetReference(ref)
	}
	branchRef := plumbing.NewHashReference(headRef.Name(), h)
	return s.repo.Storer.SetReference(branchRef)
}

// ChangedFile describes a file changed in a recent commit.
type ChangedFile struct {
	Path       string `json:"path"` // relative to documents/
	DocumentID string `json:"document_id"`
	Title      string `json:"title"`
	Folder     string `json:"folder"`
	CommitHash string `json:"commit_hash"`
	CommitMsg  string `json:"commit_message"`
	CommitTime string `json:"commit_time"`
	Author     string `json:"author"`
}

// RecentlyChanged returns documents under documents/ that were modified in the
// last N commits. Uses go-git commit log + tree diff — no CLI.
func (s *Store) RecentlyChanged(maxCommits int) []ChangedFile {
	if s.repo == nil {
		return nil
	}

	ref, err := s.repo.Head()
	if err != nil {
		return nil
	}

	logIter, err := s.repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil
	}
	defer logIter.Close()

	seen := make(map[string]bool)
	var results []ChangedFile
	count := 0

	logIter.ForEach(func(c *object.Commit) error {
		if count >= maxCommits {
			return fmt.Errorf("done") // stop iteration
		}
		count++

		// Get parent tree (nil for root commit)
		var parentTree *object.Tree
		if c.NumParents() > 0 {
			parent, err := c.Parent(0)
			if err == nil {
				parentTree, _ = parent.Tree()
			}
		}

		currentTree, err := c.Tree()
		if err != nil {
			return nil
		}

		changes, err := object.DiffTreeWithOptions(context.TODO(), parentTree, currentTree, &object.DiffTreeOptions{DetectRenames: false})
		if err != nil {
			return nil
		}

		for _, change := range changes {
			name := change.To.Name
			if name == "" {
				name = change.From.Name
			}
			// Only documents/ markdown files
			if !strings.HasPrefix(name, "documents/") || !strings.HasSuffix(name, ".md") {
				continue
			}
			relPath := strings.TrimPrefix(name, "documents/")
			if seen[relPath] {
				continue
			}
			seen[relPath] = true

			parts := strings.SplitN(relPath, "/", 2)
			folder := ""
			if len(parts) > 0 {
				folder = parts[0]
			}

			// Parse frontmatter
			fullPath := filepath.Join(s.root, name)
			pf, _ := s.LoadDocument(fullPath)
			var docID, title string
			if pf != nil {
				docID = pf.Frontmatter.DocumentID
				title = pf.Frontmatter.Title
			}

			results = append(results, ChangedFile{
				Path:       relPath,
				DocumentID: docID,
				Title:      title,
				Folder:     folder,
				CommitHash: c.Hash.String()[:8],
				CommitMsg:  strings.SplitN(c.Message, "\n", 2)[0],
				CommitTime: c.Author.When.Format("2006-01-02 15:04"),
				Author:     c.Author.Email,
			})
		}
		return nil
	})

	return results
}

// resolveRef resolves a ref string (commit hash, branch, tag, HEAD, HEAD~N)
// to a commit object.
func (s *Store) resolveRef(ref string) (*object.Commit, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("not a bare repo store")
	}

	// Try resolving as a revision (handles HEAD, HEAD~1, branch names, tags).
	hash, err := s.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		// Try as a raw hex hash.
		h := plumbing.NewHash(ref)
		commit, err2 := s.repo.CommitObject(h)
		if err2 != nil {
			return nil, fmt.Errorf("resolving ref %q: %w", ref, err)
		}
		return commit, nil
	}
	return s.repo.CommitObject(*hash)
}

// ReadFileAtRef reads a file at a specific git ref (commit hash, branch name, tag).
func (s *Store) ReadFileAtRef(ref, path string) ([]byte, error) {
	commit, err := s.resolveRef(ref)
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("reading tree for ref %q: %w", ref, err)
	}
	f, err := tree.File(path)
	if err != nil {
		return nil, fmt.Errorf("file %q not found at ref %q: %w", path, ref, err)
	}
	rc, err := f.Reader()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// DiffFiles returns a unified diff for a file between two refs.
// If fromRef is empty, the parent of toRef is used.
func (s *Store) DiffFiles(fromRef, toRef, filePath string) (string, error) {
	if s.repo == nil {
		return "", fmt.Errorf("not a bare repo store")
	}

	// Resolve "to" commit.
	toCommit, err := s.resolveRef(toRef)
	if err != nil {
		return "", fmt.Errorf("resolving to ref: %w", err)
	}

	// If no fromRef, use the parent of toCommit.
	var fromCommit *object.Commit
	if fromRef == "" {
		parents := toCommit.ParentHashes
		if len(parents) == 0 {
			// Initial commit — diff against empty.
			fromCommit = nil
		} else {
			fromCommit, err = s.repo.CommitObject(parents[0])
			if err != nil {
				return "", fmt.Errorf("resolving parent commit: %w", err)
			}
		}
	} else {
		fromCommit, err = s.resolveRef(fromRef)
		if err != nil {
			return "", fmt.Errorf("resolving from ref: %w", err)
		}
	}

	// Read file content at each ref.
	var fromContent string
	if fromCommit != nil {
		data, err := s.readFileFromCommit(fromCommit, filePath)
		if err == nil {
			fromContent = string(data)
		}
		// If file doesn't exist at fromRef, treat as empty (new file).
	}

	toData, err := s.readFileFromCommit(toCommit, filePath)
	if err != nil {
		// If file doesn't exist at toRef, treat as empty (deleted file).
		toData = nil
	}
	toContent := string(toData)

	if fromContent == toContent {
		return "", nil
	}

	// Produce unified diff using go-diff.
	dmp := diffmatchpatch.New()
	a, b, c := dmp.DiffLinesToChars(fromContent, toContent)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, c)

	// Convert to unified diff format.
	return formatUnifiedDiff(filePath, fromContent, toContent, diffs), nil
}

// DiffDocumentBodies returns a unified diff between two refs, stripping YAML frontmatter
// from both versions so the diff only covers document body content.
func (s *Store) DiffDocumentBodies(fromRef, toRef, filePath string) (string, error) {
	if s.repo == nil {
		return "", fmt.Errorf("not a bare repo store")
	}

	var fromContent, toContent string

	if fromRef != "" {
		if data, err := s.ReadFileAtRef(fromRef, filePath); err == nil {
			fromContent = StripFrontmatter(string(data))
		}
	}

	toCommit, err := s.resolveRef(toRef)
	if err != nil {
		return "", fmt.Errorf("resolving to ref: %w", err)
	}
	toData, err := s.readFileFromCommit(toCommit, filePath)
	if err != nil {
		return "", fmt.Errorf("reading file at to ref: %w", err)
	}
	toContent = StripFrontmatter(string(toData))

	if fromContent == toContent {
		return "", nil
	}

	dmp := diffmatchpatch.New()
	a, b, c := dmp.DiffLinesToChars(fromContent, toContent)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, c)

	return formatUnifiedDiff(filePath, fromContent, toContent, diffs), nil
}

// readFileFromCommit reads a file from a specific commit.
func (s *Store) readFileFromCommit(commit *object.Commit, path string) ([]byte, error) {
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	f, err := tree.File(path)
	if err != nil {
		return nil, err
	}
	rc, err := f.Reader()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// formatUnifiedDiff converts go-diff output to a unified diff format.
func formatUnifiedDiff(path, oldText, newText string, diffs []diffmatchpatch.Diff) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- a/%s\n", path))
	b.WriteString(fmt.Sprintf("+++ b/%s\n", path))

	oldLine := 1
	newLine := 1

	for _, d := range diffs {
		lines := strings.Split(d.Text, "\n")
		// Remove trailing empty string from split if text ends with newline.
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		count := len(lines)

		switch d.Type {
		case diffmatchpatch.DiffEqual:
			for _, l := range lines {
				b.WriteString(fmt.Sprintf("@@ -%d,1 +%d,1 @@\n", oldLine, newLine))
				b.WriteString(" " + l + "\n")
				oldLine++
				newLine++
			}
		case diffmatchpatch.DiffDelete:
			b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,0 @@\n", oldLine, count, newLine))
			for _, l := range lines {
				b.WriteString("-" + l + "\n")
				oldLine++
			}
		case diffmatchpatch.DiffInsert:
			b.WriteString(fmt.Sprintf("@@ -%d,0 +%d,%d @@\n", oldLine, newLine, count))
			for _, l := range lines {
				b.WriteString("+" + l + "\n")
				newLine++
			}
		}
	}

	return b.String()
}

// FileLastCommit returns the most recent commit hash and time for a specific file path.
// The filePath should be relative to the repo root (e.g. "folder/doc.md").
// fileCommitInfo holds cached last-commit info for a file.
type fileCommitInfo struct {
	Hash       string
	CommitTime time.Time
	Message    string
	Author     string
}

// buildFileCommitCache walks git log once and returns the last commit that touched each file.
// Much faster than calling FileLastCommit per file (O(commits) vs O(files * commits)).
func (s *Store) buildFileCommitCache() map[string]*fileCommitInfo {
	s.fileCommitMu.Lock()
	defer s.fileCommitMu.Unlock()

	// Check if cache is still valid (same HEAD)
	ref, err := s.repo.Head()
	if err != nil {
		return nil
	}
	headHash := ref.Hash().String()
	if s.fileCommitHead == headHash && s.fileCommitCache != nil {
		return s.fileCommitCache
	}

	cache := map[string]*fileCommitInfo{}
	logIter, err := s.repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil
	}
	defer logIter.Close()

	logIter.ForEach(func(c *object.Commit) error {
		currentTree, tErr := c.Tree()
		if tErr != nil {
			return nil
		}
		// Collect all file hashes in this commit's tree
		currentFiles := flattenTree(currentTree, "")

		// Get parent tree files
		var parentFiles map[string]plumbing.Hash
		if c.NumParents() > 0 {
			parent, pErr := c.Parent(0)
			if pErr == nil {
				pt, ptErr := parent.Tree()
				if ptErr == nil {
					parentFiles = flattenTree(pt, "")
				}
			}
		}

		// Find changed files by comparing hashes (much faster than DiffTree)
		for path, hash := range currentFiles {
			if _, exists := cache[path]; exists {
				continue // already found a newer commit for this file
			}
			parentHash, inParent := parentFiles[path]
			if !inParent || parentHash != hash {
				cache[path] = &fileCommitInfo{
					Hash:       c.Hash.String(),
					CommitTime: c.Author.When,
					Message:    strings.SplitN(c.Message, "\n", 2)[0],
					Author:     c.Author.Email,
				}
			}
		}
		// Files deleted in this commit (in parent but not current)
		for path := range parentFiles {
			if _, exists := cache[path]; exists {
				continue
			}
			if _, inCurrent := currentFiles[path]; !inCurrent {
				cache[path] = &fileCommitInfo{
					Hash:       c.Hash.String(),
					CommitTime: c.Author.When,
					Message:    strings.SplitN(c.Message, "\n", 2)[0],
					Author:     c.Author.Email,
				}
			}
		}
		return nil
	})

	s.fileCommitCache = cache
	s.fileCommitHead = headHash
	return cache
}

// FileLastCommit returns the last commit that touched a file. Uses cached bulk lookup.
func (s *Store) FileLastCommit(filePath string) (hash string, commitTime time.Time, message string, author string, err error) {
	if s.repo == nil {
		err = fmt.Errorf("not a bare repo store")
		return
	}
	cache := s.buildFileCommitCache()
	if cache == nil {
		err = fmt.Errorf("failed to build commit cache")
		return
	}
	info, ok := cache[filePath]
	if !ok {
		err = fmt.Errorf("no commits found for %s", filePath)
		return
	}
	return info.Hash, info.CommitTime, info.Message, info.Author, nil
}

// CommitsSince returns commit messages for a file since a given commit hash.
func (s *Store) CommitsSince(filePath, sinceHash string) []string {
	if s.repo == nil {
		return nil
	}

	ref, err := s.repo.Head()
	if err != nil {
		return nil
	}

	logIter, err := s.repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil
	}
	defer logIter.Close()

	var messages []string
	logIter.ForEach(func(c *object.Commit) error {
		// Stop if we've reached the approved commit
		if c.Hash.String() == sinceHash {
			return fmt.Errorf("done")
		}

		var parentTree *object.Tree
		if c.NumParents() > 0 {
			parent, pErr := c.Parent(0)
			if pErr == nil {
				parentTree, _ = parent.Tree()
			}
		}
		currentTree, tErr := c.Tree()
		if tErr != nil {
			return nil
		}

		changes, dErr := object.DiffTreeWithOptions(context.TODO(), parentTree, currentTree, &object.DiffTreeOptions{DetectRenames: false})
		if dErr != nil {
			return nil
		}

		for _, change := range changes {
			name := change.To.Name
			if name == "" {
				name = change.From.Name
			}
			if name == filePath {
				messages = append(messages, strings.SplitN(c.Message, "\n", 2)[0])
				break
			}
		}
		return nil
	})

	return messages
}

// ValidateFilePath checks that a file path is allowed by repo policy.
func ValidateFilePath(relPath string) error {
	relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")
	if strings.Contains(relPath, "..") {
		return fmt.Errorf("path traversal not allowed: %s", relPath)
	}
	if strings.HasPrefix(relPath, "documents/") && strings.HasSuffix(relPath, ".md") {
		return nil
	}
	if strings.HasPrefix(relPath, "documents/") && filepath.Base(relPath) == ".title" {
		return nil
	}
	if relPath == "README.md" {
		return nil
	}
	return fmt.Errorf("file not allowed by repo policy: %s", relPath)
}

// ValidateFileSize checks that content doesn't exceed the 2 MB limit.
func ValidateFileSize(content []byte) error {
	const maxSize = 2 * 1024 * 1024
	if len(content) > maxSize {
		return fmt.Errorf("file too large (%d bytes, max %d)", len(content), maxSize)
	}
	return nil
}

// CommitFile writes a file to the bare repo by creating a new blob, updating the tree,
// and creating a new commit. This is the foundation for web editing and metadata updates.
func (s *Store) CommitFile(filePath string, content []byte, authorName, authorEmail, message string, expectedHead ...string) (string, error) {
	if s.repo == nil {
		return "", fmt.Errorf("not a bare repo store")
	}
	// Validate path and size
	rel := s.relPath(filePath)
	if err := ValidateFilePath(rel); err != nil {
		return "", err
	}
	if err := ValidateFileSize(content); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commitFileUnlocked(filePath, content, authorName, authorEmail, message, expectedHead...)
}

// FileEntry represents a file to commit in a batch.
type FileEntry struct {
	Path    string
	Content []byte
}

// CommitFiles commits multiple files in a single commit.
func (s *Store) CommitFiles(files []FileEntry, authorName, authorEmail, message string) (string, error) {
	if s.repo == nil {
		return "", fmt.Errorf("not a bare repo store")
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no files to commit")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store all blobs
	type blobEntry struct {
		relPath string
		hash    plumbing.Hash
	}
	var blobs []blobEntry
	for _, f := range files {
		h, err := storeBlob(s.repo.Storer, f.Content)
		if err != nil {
			return "", fmt.Errorf("storing blob for %s: %w", f.Path, err)
		}
		var rel string
		if filepath.IsAbs(f.Path) {
			rel = s.relPath(f.Path)
		} else {
			rel = f.Path
		}
		rel = strings.ReplaceAll(rel, string(filepath.Separator), "/")
		blobs = append(blobs, blobEntry{relPath: rel, hash: h})
	}

	// Build tree
	ref, headErr := s.repo.Head()
	var parentHashes []plumbing.Hash
	var treeHash plumbing.Hash

	if headErr != nil {
		// Initial commit — build tree from scratch with all files
		tree, err := buildTreeFromScratch(s.repo, blobs[0].relPath, blobs[0].hash)
		if err != nil {
			return "", fmt.Errorf("building initial tree: %w", err)
		}
		// Apply remaining files
		treeHash = tree
		for i := 1; i < len(blobs); i++ {
			t, treeErr := s.repo.TreeObject(treeHash)
			if treeErr != nil {
				return "", fmt.Errorf("reading intermediate tree for %s: %w", blobs[i].relPath, treeErr)
			}
			treeHash, err = replaceFileInTree(s.repo, t, blobs[i].relPath, blobs[i].hash)
			if err != nil {
				return "", fmt.Errorf("adding file %s: %w", blobs[i].relPath, err)
			}
		}
	} else {
		headCommit, err := s.repo.CommitObject(ref.Hash())
		if err != nil {
			return "", fmt.Errorf("reading HEAD commit: %w", err)
		}
		currentTree, err := headCommit.Tree()
		if err != nil {
			return "", fmt.Errorf("reading HEAD tree: %w", err)
		}
		// Apply all files to tree — chain replaceFileInTree, re-reading each intermediate tree
		treeHash = currentTree.Hash
		for _, b := range blobs {
			t, treeErr := s.repo.TreeObject(treeHash)
			if treeErr != nil {
				return "", fmt.Errorf("reading intermediate tree for %s: %w", b.relPath, treeErr)
			}
			treeHash, err = replaceFileInTree(s.repo, t, b.relPath, b.hash)
			if err != nil {
				return "", fmt.Errorf("updating file %s: %w", b.relPath, err)
			}
		}
		parentHashes = []plumbing.Hash{ref.Hash()}
	}

	// Create commit
	now := time.Now()
	committerName := authorName
	committerEmail := authorEmail
	if s.signing != nil && s.signing.CommitterName != "" {
		committerName = s.signing.CommitterName
		committerEmail = s.signing.CommitterEmail
	}

	commit := &object.Commit{
		Author:    object.Signature{Name: authorName, Email: authorEmail, When: now},
		Committer: object.Signature{Name: committerName, Email: committerEmail, When: now},
		Message:   message,
		TreeHash:  treeHash,
	}
	commit.ParentHashes = parentHashes

	if s.signing != nil && s.signing.KeyPath != "" {
		if sig, signErr := s.signCommit(commit); signErr == nil {
			commit.PGPSignature = sig
		}
	}

	obj := &plumbing.MemoryObject{}
	obj.SetType(plumbing.CommitObject)
	if err := commit.Encode(obj); err != nil {
		return "", fmt.Errorf("encoding commit: %w", err)
	}
	commitHash, err := s.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return "", fmt.Errorf("storing commit: %w", err)
	}

	refName := plumbing.Main
	if ref != nil {
		refName = ref.Name()
	}
	newRef := plumbing.NewHashReference(refName, commitHash)
	if err := s.repo.Storer.SetReference(newRef); err != nil {
		return "", fmt.Errorf("updating ref: %w", err)
	}
	// Ensure HEAD points to the branch (needed for initial commit on bare repo)
	if ref == nil {
		headRef := plumbing.NewSymbolicReference(plumbing.HEAD, refName)
		_ = s.repo.Storer.SetReference(headRef)
	}

	s.fileCommitCache = nil
	return commitHash.String(), nil
}

// DeleteFile removes a file from the repo and commits the change.
func (s *Store) DeleteFile(filePath string, authorName, authorEmail, message string) (string, error) {
	if s.repo == nil {
		return "", fmt.Errorf("not a bare repo store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ref, err := s.repo.Head()
	if err != nil {
		return "", fmt.Errorf("resolving HEAD: %w", err)
	}
	headCommit, err := s.repo.CommitObject(ref.Hash())
	if err != nil {
		return "", fmt.Errorf("reading HEAD commit: %w", err)
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return "", fmt.Errorf("reading HEAD tree: %w", err)
	}

	relPath := s.relPath(filePath)
	relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

	newTreeHash, err := removeEntryFromTree(s.repo, headTree, relPath)
	if err != nil {
		return "", fmt.Errorf("removing file: %w", err)
	}

	now := time.Now()
	commit := &object.Commit{
		Author:    object.Signature{Name: authorName, Email: authorEmail, When: now},
		Committer: object.Signature{Name: authorName, Email: authorEmail, When: now},
		Message:   message,
		TreeHash:  newTreeHash,
	}
	commit.ParentHashes = []plumbing.Hash{ref.Hash()}

	obj := &plumbing.MemoryObject{}
	obj.SetType(plumbing.CommitObject)
	if err := commit.Encode(obj); err != nil {
		return "", fmt.Errorf("encoding commit: %w", err)
	}
	commitHash, err := s.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return "", fmt.Errorf("storing commit: %w", err)
	}

	newRef := plumbing.NewHashReference(ref.Name(), commitHash)
	if err := s.repo.Storer.SetReference(newRef); err != nil {
		return "", fmt.Errorf("updating ref: %w", err)
	}

	s.fileCommitCache = nil
	return commitHash.String(), nil
}

// signCommit signs a commit using SSH key (same format as git -S).
func (s *Store) signCommit(commit *object.Commit) (string, error) {
	keyData, err := os.ReadFile(s.signing.KeyPath)
	if err != nil {
		return "", fmt.Errorf("reading signing key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return "", fmt.Errorf("parsing signing key: %w", err)
	}

	// Build the commit content to sign (same as what git signs)
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("tree %s\n", commit.TreeHash))
	for _, parent := range commit.ParentHashes {
		buf.WriteString(fmt.Sprintf("parent %s\n", parent))
	}
	buf.WriteString(fmt.Sprintf("author %s <%s> %d %s\n",
		commit.Author.Name, commit.Author.Email,
		commit.Author.When.Unix(), commit.Author.When.Format("-0700")))
	buf.WriteString(fmt.Sprintf("committer %s <%s> %d %s\n",
		commit.Committer.Name, commit.Committer.Email,
		commit.Committer.When.Unix(), commit.Committer.When.Format("-0700")))
	buf.WriteString("\n")
	buf.WriteString(commit.Message)

	content := []byte(buf.String())

	// Sign with SSH
	sig, err := signer.Sign(cryptorand.Reader, content)
	if err != nil {
		return "", fmt.Errorf("signing: %w", err)
	}

	// Format as SSH signature (armored)
	sshSig := ssh.Marshal(sig)
	encoded := base64.StdEncoding.EncodeToString(sshSig)

	var armored strings.Builder
	armored.WriteString("-----BEGIN SSH SIGNATURE-----\n")
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		armored.WriteString(encoded[i:end])
		armored.WriteString("\n")
	}
	armored.WriteString("-----END SSH SIGNATURE-----")

	return armored.String(), nil
}

// commitFileUnlocked is the internal version without locking (caller must hold mu).
func (s *Store) commitFileUnlocked(filePath string, content []byte, authorName, authorEmail, message string, expectedHead ...string) (string, error) {
	// Create new blob
	blobObj := s.repo.Storer
	blobHash, err := storeBlob(blobObj, content)
	if err != nil {
		return "", fmt.Errorf("storing blob: %w", err)
	}

	relPath := s.relPath(filePath)
	relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

	// Get current HEAD — if none exists, this is the initial commit
	ref, headErr := s.repo.Head()
	var parentHashes []plumbing.Hash
	var newTree plumbing.Hash

	if headErr != nil {
		// No HEAD — initial commit. Build tree from scratch.
		tree, err := buildTreeFromScratch(s.repo, relPath, blobHash)
		if err != nil {
			return "", fmt.Errorf("building initial tree: %w", err)
		}
		newTree = tree
		// No parent for initial commit
	} else {
		// Normal case: HEAD exists
		if len(expectedHead) > 0 && expectedHead[0] != "" {
			if ref.Hash().String() != expectedHead[0] {
				return "", ErrConflict
			}
		}
		headCommit, err := s.repo.CommitObject(ref.Hash())
		if err != nil {
			return "", fmt.Errorf("reading HEAD commit: %w", err)
		}
		headTree, err := headCommit.Tree()
		if err != nil {
			return "", fmt.Errorf("reading HEAD tree: %w", err)
		}
		newTree, err = replaceFileInTree(s.repo, headTree, relPath, blobHash)
		if err != nil {
			return "", fmt.Errorf("updating tree: %w", err)
		}
		parentHashes = []plumbing.Hash{ref.Hash()}
	}

	// Create commit
	now := time.Now()
	committerName := authorName
	committerEmail := authorEmail
	if s.signing != nil && s.signing.CommitterName != "" {
		committerName = s.signing.CommitterName
		committerEmail = s.signing.CommitterEmail
	}

	commit := &object.Commit{
		Author: object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  now,
		},
		Committer: object.Signature{
			Name:  committerName,
			Email: committerEmail,
			When:  now,
		},
		Message:  message,
		TreeHash: newTree,
	}
	commit.ParentHashes = parentHashes

	// Sign commit with SSH key if configured
	if s.signing != nil && s.signing.KeyPath != "" {
		if sig, signErr := s.signCommit(commit); signErr == nil {
			commit.PGPSignature = sig
		}
	}

	obj := &plumbing.MemoryObject{}
	obj.SetType(plumbing.CommitObject)
	err = commit.Encode(obj)
	if err != nil {
		return "", fmt.Errorf("encoding commit: %w", err)
	}
	commitHash, err := s.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return "", fmt.Errorf("storing commit: %w", err)
	}

	// Update HEAD ref
	refName := plumbing.Main
	if ref != nil {
		refName = ref.Name()
	}
	newRef := plumbing.NewHashReference(refName, commitHash)
	if err := s.repo.Storer.SetReference(newRef); err != nil {
		return "", fmt.Errorf("updating ref: %w", err)
	}
	// Ensure HEAD points to the branch (needed for initial commit on bare repo)
	if ref == nil {
		headRef := plumbing.NewSymbolicReference(plumbing.HEAD, refName)
		_ = s.repo.Storer.SetReference(headRef)
	}

	return commitHash.String(), nil
}

// storeBlob creates a blob object in the repo.
func storeBlob(storer interface {
	SetEncodedObject(plumbing.EncodedObject) (plumbing.Hash, error)
}, content []byte) (plumbing.Hash, error) {
	obj := &plumbing.MemoryObject{}
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(int64(len(content)))
	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	w.Write(content)
	w.Close()
	return storer.SetEncodedObject(obj)
}

// flattenTree returns a map of path → blob hash for all files in a tree (recursive).
func flattenTree(tree *object.Tree, prefix string) map[string]plumbing.Hash {
	files := map[string]plumbing.Hash{}
	if tree == nil {
		return files
	}
	for _, entry := range tree.Entries {
		path := entry.Name
		if prefix != "" {
			path = prefix + "/" + entry.Name
		}
		if entry.Mode.IsFile() {
			files[path] = entry.Hash
		}
	}
	// Recurse into subtrees
	for _, entry := range tree.Entries {
		if entry.Mode == filemode.Dir {
			subtree, err := tree.Tree(entry.Name)
			if err != nil {
				continue
			}
			path := entry.Name
			if prefix != "" {
				path = prefix + "/" + entry.Name
			}
			for k, v := range flattenTree(subtree, path) {
				files[k] = v
			}
		}
	}
	return files
}

// buildTreeFromScratch creates a tree with a single file (for initial commits on empty repos).
func buildTreeFromScratch(repo *git.Repository, path string, blobHash plumbing.Hash) (plumbing.Hash, error) {
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 1 {
		// Single file at root
		tree := &object.Tree{
			Entries: []object.TreeEntry{
				{Name: parts[0], Mode: filemode.Regular, Hash: blobHash},
			},
		}
		obj := &plumbing.MemoryObject{}
		obj.SetType(plumbing.TreeObject)
		if err := tree.Encode(obj); err != nil {
			return plumbing.ZeroHash, err
		}
		return repo.Storer.SetEncodedObject(obj)
	}

	// Nested: build subtree first, then parent
	subHash, err := buildTreeFromScratch(repo, parts[1], blobHash)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	tree := &object.Tree{
		Entries: []object.TreeEntry{
			{Name: parts[0], Mode: filemode.Dir, Hash: subHash},
		},
	}
	obj := &plumbing.MemoryObject{}
	obj.SetType(plumbing.TreeObject)
	if err := tree.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return repo.Storer.SetEncodedObject(obj)
}

// replaceFileInTree creates a new tree with one file replaced.
// Handles nested paths by recursively rebuilding subtrees.
func replaceFileInTree(repo *git.Repository, tree *object.Tree, path string, blobHash plumbing.Hash) (plumbing.Hash, error) {
	parts := strings.SplitN(path, "/", 2)

	var entries []object.TreeEntry

	// Copy existing entries, replacing the target
	found := false
	for _, entry := range tree.Entries {
		if entry.Name == parts[0] {
			found = true
			if len(parts) == 1 {
				// This is the file — replace blob hash
				entries = append(entries, object.TreeEntry{
					Name: entry.Name,
					Mode: entry.Mode,
					Hash: blobHash,
				})
			} else {
				// This is a subtree — recurse
				subTree, err := repo.TreeObject(entry.Hash)
				if err != nil {
					return plumbing.ZeroHash, err
				}
				newSubHash, err := replaceFileInTree(repo, subTree, parts[1], blobHash)
				if err != nil {
					return plumbing.ZeroHash, err
				}
				entries = append(entries, object.TreeEntry{
					Name: entry.Name,
					Mode: entry.Mode,
					Hash: newSubHash,
				})
			}
		} else {
			entries = append(entries, entry)
		}
	}

	if !found {
		if len(parts) == 1 {
			// New file — add it to the tree
			entries = append(entries, object.TreeEntry{
				Name: parts[0],
				Mode: filemode.Regular,
				Hash: blobHash,
			})
		} else {
			// New subdirectory — recurse with empty tree (no existing entries)
			newSubHash, err := replaceFileInTree(repo, &object.Tree{}, parts[1], blobHash)
			if err != nil {
				return plumbing.ZeroHash, err
			}
			entries = append(entries, object.TreeEntry{
				Name: parts[0],
				Mode: filemode.Dir,
				Hash: newSubHash,
			})
		}
	}

	// Sort entries — go-git requires tree entries in sorted order
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	// Create new tree object
	newTree := &object.Tree{Entries: entries}
	treeObj := &plumbing.MemoryObject{}
	treeObj.SetType(plumbing.TreeObject)
	if err := newTree.Encode(treeObj); err != nil {
		return plumbing.ZeroHash, err
	}
	return repo.Storer.SetEncodedObject(treeObj)
}

// removeEntryFromTree creates a new tree with one entry (file or subtree) removed.
// For nested paths, it recursively rebuilds subtrees.
func removeEntryFromTree(repo *git.Repository, tree *object.Tree, path string) (plumbing.Hash, error) {
	parts := strings.SplitN(path, "/", 2)

	var entries []object.TreeEntry
	found := false
	for _, entry := range tree.Entries {
		if entry.Name == parts[0] {
			found = true
			if len(parts) == 1 {
				// Skip this entry (remove it)
				continue
			}
			// Recurse into subtree
			subTree, err := repo.TreeObject(entry.Hash)
			if err != nil {
				return plumbing.ZeroHash, err
			}
			newSubHash, err := removeEntryFromTree(repo, subTree, parts[1])
			if err != nil {
				return plumbing.ZeroHash, err
			}
			entries = append(entries, object.TreeEntry{
				Name: entry.Name,
				Mode: entry.Mode,
				Hash: newSubHash,
			})
		} else {
			entries = append(entries, entry)
		}
	}

	if !found {
		return plumbing.ZeroHash, fmt.Errorf("entry not found in tree: %s", parts[0])
	}

	newTree := &object.Tree{Entries: entries}
	treeObj := &plumbing.MemoryObject{}
	treeObj.SetType(plumbing.TreeObject)
	if err := newTree.Encode(treeObj); err != nil {
		return plumbing.ZeroHash, err
	}
	return repo.Storer.SetEncodedObject(treeObj)
}

// DeleteDirectory removes an entire directory from the bare repo and commits the change.
func (s *Store) DeleteDirectory(dirPath, authorName, authorEmail, message string) (string, error) {
	if s.repo == nil {
		return "", fmt.Errorf("not a bare repo store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ref, err := s.repo.Head()
	if err != nil {
		return "", fmt.Errorf("resolving HEAD: %w", err)
	}
	headCommit, err := s.repo.CommitObject(ref.Hash())
	if err != nil {
		return "", fmt.Errorf("reading HEAD commit: %w", err)
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return "", fmt.Errorf("reading HEAD tree: %w", err)
	}

	relPath := s.relPath(dirPath)
	relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

	newTreeHash, err := removeEntryFromTree(s.repo, headTree, relPath)
	if err != nil {
		return "", fmt.Errorf("removing directory: %w", err)
	}

	now := time.Now()
	committerName := authorName
	committerEmail := authorEmail
	if s.signing != nil && s.signing.CommitterName != "" {
		committerName = s.signing.CommitterName
		committerEmail = s.signing.CommitterEmail
	}

	commit := &object.Commit{
		Author: object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  now,
		},
		Committer: object.Signature{
			Name:  committerName,
			Email: committerEmail,
			When:  now,
		},
		Message:  message,
		TreeHash: newTreeHash,
	}
	commit.ParentHashes = []plumbing.Hash{ref.Hash()}

	if s.signing != nil && s.signing.KeyPath != "" {
		if sig, signErr := s.signCommit(commit); signErr == nil {
			commit.PGPSignature = sig
		}
	}

	obj := &plumbing.MemoryObject{}
	obj.SetType(plumbing.CommitObject)
	err = commit.Encode(obj)
	if err != nil {
		return "", fmt.Errorf("encoding commit: %w", err)
	}
	commitHash, err := s.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return "", fmt.Errorf("storing commit: %w", err)
	}

	newRef := plumbing.NewHashReference(ref.Name(), commitHash)
	if err := s.repo.Storer.SetReference(newRef); err != nil {
		return "", fmt.Errorf("updating ref: %w", err)
	}

	return commitHash.String(), nil
}

// UpdateDocumentMetadata updates a frontmatter field in a document and commits the change.
func (s *Store) UpdateDocumentMetadata(docPath, field, value, authorName, authorEmail string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Read current content
	raw, err := s.readFile(docPath)
	if err != nil {
		return "", fmt.Errorf("reading document: %w", err)
	}

	content := string(raw)
	lines := strings.Split(content, "\n")

	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return "", fmt.Errorf("document has no frontmatter")
	}

	// Find and update the field in frontmatter
	endIdx := -1
	fieldFound := false
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
		// Match "field: value" or "field: " pattern
		if strings.HasPrefix(strings.TrimSpace(lines[i]), field+":") {
			lines[i] = field + ": " + quoteYAMLValue(value)
			fieldFound = true
		}
	}

	if endIdx < 0 {
		return "", fmt.Errorf("malformed frontmatter")
	}

	// If field not found, add it before the closing ---
	if !fieldFound {
		newLine := field + ": " + quoteYAMLValue(value)
		lines = append(lines[:endIdx], append([]string{newLine}, lines[endIdx:]...)...)
	}

	newContent := strings.Join(lines, "\n")

	// Extract document_id from frontmatter for a professional commit message
	docID := filepath.Base(docPath)
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		if strings.HasPrefix(strings.TrimSpace(l), "document_id:") {
			docID = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "document_id:"))
			docID = strings.Trim(docID, "\"' ")
			break
		}
	}
	message := fmt.Sprintf("docs(%s): set %s to %q", docID, field, value)

	return s.commitFileUnlocked(docPath, []byte(newContent), authorName, authorEmail, message)
}

// quoteYAMLValue quotes a value if it contains special characters.
func quoteYAMLValue(v string) string {
	if strings.ContainsAny(v, ":{}[]|>&*!%#`@,") || strings.Contains(v, " #") {
		return "\"" + strings.ReplaceAll(v, "\"", "\\\"") + "\""
	}
	return v
}

// UpdateDocumentMetadataMulti atomically updates multiple frontmatter fields in a single commit.
// This avoids the TOCTOU race of calling UpdateDocumentMetadata multiple times.
func (s *Store) UpdateDocumentMetadataMulti(docPath string, fields map[string]string, authorName, authorEmail string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := s.readFile(docPath)
	if err != nil {
		return "", fmt.Errorf("reading document: %w", err)
	}

	content := string(raw)
	lines := strings.Split(content, "\n")

	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return "", fmt.Errorf("document has no frontmatter")
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return "", fmt.Errorf("malformed frontmatter")
	}

	// Track which fields have been found and updated
	remaining := make(map[string]string)
	for k, v := range fields {
		remaining[k] = v
	}

	for i := 1; i < endIdx; i++ {
		trimmed := strings.TrimSpace(lines[i])
		for field, value := range remaining {
			if strings.HasPrefix(trimmed, field+":") {
				lines[i] = field + ": " + quoteYAMLValue(value)
				delete(remaining, field)
				break
			}
		}
	}

	// Add any fields not found in existing frontmatter
	for field, value := range remaining {
		newLine := field + ": " + quoteYAMLValue(value)
		lines = append(lines[:endIdx], append([]string{newLine}, lines[endIdx:]...)...)
		endIdx++ // adjust for inserted line
	}

	newContent := strings.Join(lines, "\n")

	// Extract document_id for commit message
	docID := filepath.Base(docPath)
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		if strings.HasPrefix(strings.TrimSpace(l), "document_id:") {
			docID = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "document_id:"))
			docID = strings.Trim(docID, "\"' ")
			break
		}
	}

	// Build commit message listing changed fields
	var fieldNames []string
	for k := range fields {
		fieldNames = append(fieldNames, k)
	}
	message := fmt.Sprintf("docs(%s): update %s", docID, strings.Join(fieldNames, ", "))

	return s.commitFileUnlocked(docPath, []byte(newContent), authorName, authorEmail, message)
}

// CreateSuggestion writes content to a suggestion branch for a document.
// Branch name format: suggestions/<docId>/<userId>
// Creates the branch from HEAD, then commits the file to that branch.
func (s *Store) CreateSuggestion(docPath, branchName string, content []byte, authorName, authorEmail, message string) (string, error) {
	if s.repo == nil {
		return "", fmt.Errorf("not a bare repo store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Resolve base: use branch tip if exists, otherwise HEAD
	ref, err := s.repo.Head()
	if err != nil {
		return "", fmt.Errorf("resolving HEAD: %w", err)
	}
	baseHash := ref.Hash()
	branchRef, branchErr := s.repo.Reference(plumbing.ReferenceName("refs/heads/"+branchName), true)
	if branchErr == nil && branchRef != nil {
		baseHash = branchRef.Hash()
	}

	baseCommit, err := s.repo.CommitObject(baseHash)
	if err != nil {
		return "", fmt.Errorf("reading base commit: %w", err)
	}
	baseTree, err := baseCommit.Tree()
	if err != nil {
		return "", fmt.Errorf("reading base tree: %w", err)
	}

	// Create new blob with the suggestion content
	blobHash, err := storeBlob(s.repo.Storer, content)
	if err != nil {
		return "", fmt.Errorf("storing blob: %w", err)
	}

	// Build new tree from base (branch tip or HEAD)
	relPath := s.relPath(docPath)
	relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

	newTreeHash, err := replaceFileInTree(s.repo, baseTree, relPath, blobHash)
	if err != nil {
		return "", fmt.Errorf("updating tree: %w", err)
	}

	// Create commit — parent is baseHash (branch tip or HEAD, resolved above)
	now := time.Now()
	commit := &object.Commit{
		Author: object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  now,
		},
		Committer: object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  now,
		},
		Message:  message,
		TreeHash: newTreeHash,
	}
	commit.ParentHashes = []plumbing.Hash{baseHash}

	obj := &plumbing.MemoryObject{}
	obj.SetType(plumbing.CommitObject)
	if err := commit.Encode(obj); err != nil {
		return "", fmt.Errorf("encoding commit: %w", err)
	}
	commitHash, err := s.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return "", fmt.Errorf("storing commit: %w", err)
	}

	// Create or update the suggestion branch reference
	newBranchRef := plumbing.NewHashReference(plumbing.ReferenceName("refs/heads/"+branchName), commitHash)
	if err := s.repo.Storer.SetReference(newBranchRef); err != nil {
		return "", fmt.Errorf("setting branch ref: %w", err)
	}

	return commitHash.String(), nil
}

// GetSuggestion reads the content of a file from a suggestion branch.
func (s *Store) GetSuggestion(docPath, branchName string) ([]byte, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("not a bare repo store")
	}

	relPath := s.relPath(docPath)
	relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

	return s.ReadFileAtRef(branchName, relPath)
}

// ListSuggestionBranches returns all suggestion branch names matching a prefix.
// Prefix format: "suggestions/<docId>/" — returns full branch names.
func (s *Store) ListSuggestionBranches(prefix string) ([]string, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("not a bare repo store")
	}

	refs, err := s.repo.References()
	if err != nil {
		return nil, fmt.Errorf("listing references: %w", err)
	}
	defer refs.Close()

	var branches []string
	refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()
		branchName := strings.TrimPrefix(name, "refs/heads/")
		if strings.HasPrefix(branchName, prefix) {
			branches = append(branches, branchName)
		}
		return nil
	})

	return branches, nil
}

// MergeSuggestion reads the file content from a suggestion branch and commits it to HEAD (main).
// If baseCommit is provided and non-empty, it verifies HEAD hasn't moved since the suggestion was created.
func (s *Store) MergeSuggestion(docPath, branchName string, authorName, authorEmail string, baseCommit ...string) (string, error) {
	if s.repo == nil {
		return "", fmt.Errorf("not a bare repo store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if the specific file was modified since baseCommit.
	// This is file-level conflict detection, not repo-level — changes to
	// other documents don't block merge.
	if len(baseCommit) > 0 && baseCommit[0] != "" {
		relPath := s.relPath(docPath)
		relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")
		// Read file content at baseCommit
		oldContent, oldErr := s.ReadFileAtRef(baseCommit[0], relPath)
		// Read file content at current HEAD
		curContent, curErr := s.readFile(docPath)
		// If both readable and different, the file was modified → conflict
		if oldErr == nil && curErr == nil && string(oldContent) != string(curContent) {
			return "", ErrConflict
		}
	}

	// Read file content from the suggestion branch
	content, err := s.GetSuggestion(docPath, branchName)
	if err != nil {
		return "", fmt.Errorf("reading suggestion content: %w", err)
	}

	// Extract document_id for professional commit message
	docID := filepath.Base(docPath)
	if pf, err := s.LoadDocument(docPath); err == nil && pf != nil && pf.Frontmatter.DocumentID != "" {
		docID = pf.Frontmatter.DocumentID
	}
	// Extract suggestion author from branch name: suggestions/<docId>/<email>
	sugAuthor := branchName
	if parts := strings.Split(branchName, "/"); len(parts) >= 3 {
		sugAuthor = parts[len(parts)-1]
	}
	message := fmt.Sprintf("docs(%s): accept suggestion from %s", docID, sugAuthor)
	commitHash, err := s.commitFileUnlocked(docPath, content, authorName, authorEmail, message)
	if err != nil {
		return "", fmt.Errorf("committing merge: %w", err)
	}

	// Delete the suggestion branch
	if delErr := s.DeleteSuggestionBranch(branchName); delErr != nil {
		// Non-fatal — log but don't fail
		fmt.Printf("warning: failed to delete suggestion branch %s: %v\n", branchName, delErr)
	}

	return commitHash, nil
}

// DeleteSuggestionBranch removes a suggestion branch reference.
func (s *Store) DeleteSuggestionBranch(branchName string) error {
	if s.repo == nil {
		return fmt.Errorf("not a bare repo store")
	}

	refName := plumbing.ReferenceName("refs/heads/" + branchName)
	return s.repo.Storer.RemoveReference(refName)
}

// DiffSuggestion returns a unified diff between the HEAD version and the suggestion branch version of a file.
func (s *Store) DiffSuggestion(docPath, branchName string) (string, error) {
	if s.repo == nil {
		return "", fmt.Errorf("not a bare repo store")
	}

	relPath := s.relPath(docPath)
	relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

	// Read current content from HEAD
	currentContent, err := s.readFile(docPath)
	if err != nil {
		return "", fmt.Errorf("reading current content: %w", err)
	}

	// Read suggested content from branch
	suggestedContent, err := s.ReadFileAtRef(branchName, relPath)
	if err != nil {
		return "", fmt.Errorf("reading suggestion content: %w", err)
	}

	fromText := StripFrontmatter(string(currentContent))
	toText := StripFrontmatter(string(suggestedContent))
	if fromText == toText {
		return "", nil
	}

	dmp := diffmatchpatch.New()
	a, b, c := dmp.DiffLinesToChars(fromText, toText)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, c)

	return formatUnifiedDiff(relPath, fromText, toText, diffs), nil
}

// BlameLine holds authorship info for a single line.
type BlameLine struct {
	Author string    `json:"author"`
	Date   time.Time `json:"date"`
	Hash   string    `json:"hash"`
	Text   string    `json:"text"`
}

// BlameFile returns per-line blame info for a file, stripping frontmatter.
// Optional atRef parameter specifies a branch or commit to blame at (default: HEAD).
func (s *Store) BlameFile(filePath string, atRef ...string) ([]BlameLine, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("not a bare repo store")
	}
	var commit *object.Commit
	if len(atRef) > 0 && atRef[0] != "" {
		c, err := s.resolveRef(atRef[0])
		if err != nil {
			return nil, fmt.Errorf("resolving ref %s: %w", atRef[0], err)
		}
		commit = c
	} else {
		ref, err := s.repo.Head()
		if err != nil {
			return nil, fmt.Errorf("resolving HEAD: %w", err)
		}
		c, err := s.repo.CommitObject(ref.Hash())
		if err != nil {
			return nil, fmt.Errorf("reading HEAD commit: %w", err)
		}
		commit = c
	}

	relPath := s.relPath(filePath)
	relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

	result, err := git.Blame(commit, relPath)
	if err != nil {
		return nil, fmt.Errorf("blame: %w", err)
	}

	// Find where frontmatter ends
	bodyStart := 0
	if len(result.Lines) > 0 && strings.TrimSpace(result.Lines[0].Text) == "---" {
		for i := 1; i < len(result.Lines); i++ {
			if strings.TrimSpace(result.Lines[i].Text) == "---" {
				bodyStart = i + 1
				break
			}
		}
	}

	var lines []BlameLine
	for i := bodyStart; i < len(result.Lines); i++ {
		l := result.Lines[i]
		lines = append(lines, BlameLine{
			Author: l.Author,
			Date:   l.Date,
			Hash:   l.Hash.String()[:8],
			Text:   l.Text,
		})
	}
	return lines, nil
}

// stripFrontmatterContent removes YAML frontmatter from markdown content.
// Exported for use by API handlers that read raw content via ReadFile.
func StripFrontmatter(content string) string {
	lines := strings.SplitN(content, "\n", -1)
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return content
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return content
}
