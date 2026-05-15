// Package store provides YAML file-based storage for documents.
// Registers (assets, risks, suppliers, systems, etc.) live in Postgres (db package).
package store

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	git "github.com/go-git/go-git/v5"
	"gopkg.in/yaml.v3"
)

// SigningConfig holds SSH signing configuration for git commits.
type SigningConfig struct {
	KeyPath        string // path to SSH private key (ed25519)
	CommitterName  string // e.g. "isms.sh"
	CommitterEmail string // e.g. "git@isms.sh"
}

// Store reads and writes ISMS data from YAML files in the repo.
type Store struct {
	root    string          // root directory of the ISMS repo
	repo    *git.Repository // nil = filesystem mode, non-nil = bare repo mode
	mu      sync.Mutex      // protects concurrent git write operations
	signing *SigningConfig   // SSH signing config (nil = unsigned commits)

	// Document ID index cache (invalidated on HEAD change)
	docIndexMu   sync.Mutex
	docIndex     map[string]string // document_id → absolute path
	docIndexHead string            // HEAD hash when index was built

	// File last-commit cache (invalidated on HEAD change)
	fileCommitMu    sync.Mutex
	fileCommitCache map[string]*fileCommitInfo // git path → last commit info
	fileCommitHead  string                     // HEAD hash when cache was built
}

// SetSigning configures SSH commit signing.
func (s *Store) SetSigning(cfg *SigningConfig) {
	s.signing = cfg
}

// WriteLock acquires the Store's write mutex. Used by API handlers that need
// to serialize external git operations (e.g. receive-pack) with go-git writes.
func (s *Store) WriteLock() { s.mu.Lock() }

// WriteUnlock releases the Store's write mutex.
func (s *Store) WriteUnlock() { s.mu.Unlock() }

// SearchResult is returned by Search.
type SearchResult struct {
	DocumentID string `json:"document_id"`
	Title      string `json:"title"`
	Folder     string `json:"folder"`
	Path       string `json:"path"`
	Snippet    string `json:"snippet"`
}

// Search searches all documents under documents/ for query.
// For bare repos uses `git grep` (native C, very fast on packfiles).
// For filesystem mode falls back to walking and reading files.
func (s *Store) Search(query string, limit int) []SearchResult {
	query = strings.TrimSpace(query)
	if query == "" || len(query) < 2 {
		return nil
	}

	// Same implementation for both bare repo and filesystem — walkDir handles both
	return s.walkSearch(query, limit)
}

// walkSearch searches documents by walking the tree and checking content.
// Works on both bare repos (go-git) and filesystem.
func (s *Store) walkSearch(query string, limit int) []SearchResult {
	lowerQuery := strings.ToLower(query)
	docsRoot := s.DocsRoot()
	var results []SearchResult

	s.walkDir(docsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") || len(results) >= limit {
			return nil
		}
		raw, readErr := s.readFile(path)
		if readErr != nil {
			return nil
		}
		content := string(raw)
		idx := strings.Index(strings.ToLower(content), lowerQuery)
		if idx < 0 {
			return nil
		}

		relPath, _ := filepath.Rel(docsRoot, path)
		parts := strings.SplitN(relPath, string(filepath.Separator), 2)
		folder := ""
		if len(parts) > 0 {
			folder = parts[0]
		}

		pf, _ := s.LoadDocument(path)
		var docID, title string
		if pf != nil {
			docID = pf.Frontmatter.DocumentID
			title = pf.Frontmatter.Title
		}

		// Extract snippet
		start := idx - 60
		if start < 0 {
			start = 0
		}
		end := idx + len(query) + 60
		if end > len(content) {
			end = len(content)
		}
		snippet := strings.TrimSpace(content[start:end])
		snippet = strings.ReplaceAll(snippet, "---", "")

		results = append(results, SearchResult{
			DocumentID: docID,
			Title:      title,
			Folder:     folder,
			Path:       relPath,
			Snippet:    strings.TrimSpace(snippet),
		})
		return nil
	})
	return results
}

// New creates a new Store rooted at the given directory.
func New(root string) *Store {
	return &Store{root: root}
}

// Root returns the ISMS root directory.
func (s *Store) Root() string {
	return s.root
}

// DocsRoot returns the documents root directory.
func (s *Store) DocsRoot() string {
	return filepath.Join(s.root, "documents")
}

// --- Helpers ---

func (s *Store) readYAML(path string, out interface{}) error {
	data, err := s.readFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}

func (s *Store) writeYAML(path string, data interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

