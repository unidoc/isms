// Package scaffold provides ISMS template scaffolding.
// Templates are loaded from a directory on disk (ISMS_TEMPLATE_PATH).
package scaffold

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"isms.sh/internal/isms/store"

	"gopkg.in/yaml.v3"
)

// TemplatePath returns the template directory path from env.
func TemplatePath() string {
	return os.Getenv("ISMS_TEMPLATE_PATH")
}

// TemplateMeta holds metadata from a template's meta.yaml.
type TemplateMeta struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Version     string `yaml:"version" json:"version"`
	Maintainer  string `yaml:"maintainer" json:"maintainer"`
}

// ListTemplates returns all available templates.
func ListTemplates() ([]TemplateMeta, error) {
	base := TemplatePath()
	if base == "" {
		return nil, fmt.Errorf("ISMS_TEMPLATE_PATH not set")
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, fmt.Errorf("reading templates: %w", err)
	}

	var templates []TemplateMeta
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		meta, err := loadMeta(filepath.Join(base, e.Name()))
		if err != nil {
			continue
		}
		templates = append(templates, meta)
	}
	return templates, nil
}

// IsValidTemplate checks if a template exists.
func IsValidTemplate(tmpl string) bool {
	base := TemplatePath()
	if base == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(base, tmpl, "meta.yaml"))
	return err == nil
}

func loadMeta(dir string) (TemplateMeta, error) {
	var meta TemplateMeta
	raw, err := os.ReadFile(filepath.Join(dir, "meta.yaml"))
	if err != nil {
		return meta, err
	}
	if err := yaml.Unmarshal(raw, &meta); err != nil {
		return meta, err
	}
	if meta.ID == "" {
		meta.ID = filepath.Base(dir)
	}
	return meta, nil
}

// Init scaffolds a new ISMS repository at the given root directory.
func Init(root, template string) error {
	base := TemplatePath()
	if base == "" {
		return fmt.Errorf("ISMS_TEMPLATE_PATH not set")
	}

	templateDir := filepath.Join(base, template)
	if _, err := os.Stat(filepath.Join(templateDir, "meta.yaml")); err != nil {
		return fmt.Errorf("template %q not found", template)
	}

	// Write README.md as repo marker
	readme := fmt.Sprintf("# ISMS Repository\n\nTemplate: %s\n\nAll configuration is managed via the web interface.\n", template)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(readme), 0o644); err != nil {
		return fmt.Errorf("writing README.md: %w", err)
	}

	// Copy template into documents/<template>/
	docsRoot := filepath.Join(root, "documents", template)
	return copyDir(templateDir, docsRoot)
}

// ScaffoldToRepo scaffolds a template into a bare git repo.
func ScaffoldToRepo(st *store.Store, template, authorName, authorEmail string) error {
	base := TemplatePath()
	if base == "" {
		return fmt.Errorf("ISMS_TEMPLATE_PATH not set")
	}

	templateDir := filepath.Join(base, template)
	if _, err := os.Stat(filepath.Join(templateDir, "meta.yaml")); err != nil {
		return fmt.Errorf("template %q not found", template)
	}

	// Collect all files, then commit in one batch
	var files []store.FileEntry
	err := filepath.WalkDir(templateDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		rel, _ := filepath.Rel(templateDir, path)
		if rel == "meta.yaml" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		gitPath := filepath.Join("documents", template, rel)
		files = append(files, store.FileEntry{Path: gitPath, Content: content})
		return nil
	})
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	_, commitErr := st.CommitFiles(files, authorName, authorEmail,
		fmt.Sprintf("chore: scaffold %s template (%d files)", template, len(files)))
	return commitErr
}

// LoadTemplateFile reads a file from a template directory.
func LoadTemplateFile(template, path string) ([]byte, error) {
	base := TemplatePath()
	if base == "" {
		return nil, fmt.Errorf("ISMS_TEMPLATE_PATH not set")
	}
	return os.ReadFile(filepath.Join(base, template, path))
}

// WalkTemplateFiles walks all .md files in a template, calling fn for each.
func WalkTemplateFiles(template string, fn func(relPath string, content []byte) error) error {
	base := TemplatePath()
	if base == "" {
		return fmt.Errorf("ISMS_TEMPLATE_PATH not set")
	}
	templateDir := filepath.Join(base, template)
	return filepath.WalkDir(templateDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		rel, _ := filepath.Rel(templateDir, path)
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return fn(rel, content)
	})
}

// ValidTemplates returns all valid template directory names.
func ValidTemplates() []string {
	templates, err := ListTemplates()
	if err != nil {
		return nil
	}
	var names []string
	for _, t := range templates {
		names = append(names, t.ID)
	}
	return names
}

// ---------------------------------------------------------------------------
// Frontmatter parsing helpers
// ---------------------------------------------------------------------------

// parseFrontmatter reads a markdown file and extracts YAML frontmatter + body.
func parseFrontmatter(path string) (map[string]string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	fm := map[string]string{}

	// Expect opening "---"
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return fm, "", nil
	}

	// Read frontmatter lines until closing "---"
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		// Simple key: "value" parsing
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			val = strings.Trim(val, "\"")
			fm[key] = val
		}
	}

	// Read body
	var body strings.Builder
	for scanner.Scan() {
		body.WriteString(scanner.Text())
		body.WriteString("\n")
	}
	return fm, body.String(), nil
}

// copyDir copies all files from src to dest, skipping meta.yaml.
func copyDir(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if rel == "." || rel == "meta.yaml" {
			return nil
		}
		destPath := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		os.MkdirAll(filepath.Dir(destPath), 0o755)
		return os.WriteFile(destPath, content, 0o644)
	})
}
