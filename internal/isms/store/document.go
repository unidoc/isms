package store

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"isms.sh/internal/isms/model"
	"gopkg.in/yaml.v3"
)

// DocumentFile represents a policy markdown file with parsed frontmatter.
type DocumentFile struct {
	Path        string                  `json:"path"`
	Frontmatter model.DocumentFrontmatter `json:"frontmatter"`
	Body        string                  `json:"body"` // markdown body after frontmatter
}

// LoadDocument reads a document file and parses its frontmatter.
func (s *Store) LoadDocument(path string) (*DocumentFile, error) {
	data, err := s.readFile(path)
	if err != nil {
		return nil, err
	}

	fm, body, err := parseFrontmatter(string(data))
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err)
	}

	var pf DocumentFile
	pf.Path = path
	pf.Body = body
	if err := yaml.Unmarshal([]byte(fm), &pf.Frontmatter); err != nil {
		return nil, fmt.Errorf("unmarshalling frontmatter: %w", err)
	}
	// Normalize document_id to lowercase — case-insensitive uniqueness
	pf.Frontmatter.DocumentID = strings.ToLower(pf.Frontmatter.DocumentID)
	return &pf, nil
}

// SaveDocument writes a document file with frontmatter and body.
func (s *Store) SaveDocument(pf *DocumentFile) error {
	if err := os.MkdirAll(filepath.Dir(pf.Path), 0o755); err != nil {
		return err
	}

	fmBytes, err := yaml.Marshal(&pf.Frontmatter)
	if err != nil {
		return err
	}

	content := "---\n" + string(fmBytes) + "---\n" + pf.Body
	return os.WriteFile(pf.Path, []byte(content), 0o644)
}

// ListDocFolders returns the names of top-level directories under the documents
// root, excluding hidden directories. The result is dynamic — whatever
// folders exist in the repo are returned.
func (s *Store) ListDocFolders() []string {
	entries, err := s.readDir(s.DocsRoot())
	if err != nil {
		return nil
	}
	var folders []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		folders = append(folders, e.Name())
	}
	return folders
}

// LoadDocumentsFromDir reads all markdown files recursively from a named
// subdirectory under the documents root.
func (s *Store) LoadDocumentsFromDir(folder string) ([]DocumentFile, error) {
	dir := filepath.Join(s.Root(), "documents", folder)
	return s.loadMarkdownRecursive(dir)
}

// loadMarkdownRecursive walks a directory tree and loads all .md files with frontmatter.
func (s *Store) loadMarkdownRecursive(root string) ([]DocumentFile, error) {
	var results []DocumentFile

	err := s.walkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		pf, err := s.LoadDocument(path)
		if err != nil {
			return fmt.Errorf("loading %s: %w", path, err)
		}
		results = append(results, *pf)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// parseFrontmatter splits a markdown file into YAML frontmatter and body.
func parseFrontmatter(content string) (string, string, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))

	// Expect opening ---
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return "", content, fmt.Errorf("no frontmatter found")
	}

	var fmLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		fmLines = append(fmLines, line)
	}

	// Rest is body
	var bodyLines []string
	for scanner.Scan() {
		bodyLines = append(bodyLines, scanner.Text())
	}

	fm := strings.Join(fmLines, "\n")
	body := strings.Join(bodyLines, "\n")
	if body != "" {
		body = "\n" + body
	}

	return fm, body, nil
}
