package db

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestLegalCategoriesMatchMigrations keeps LegalCategories and the
// legal_requirements.category CHECK constraint from drifting apart (#269). It
// reads the migrations in filename order and takes the last definition of the
// constraint, falling back to the inline CHECK in the initial schema.
func TestLegalCategoriesMatchMigrations(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "migrations")
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found in %s: %v", dir, err)
	}
	sort.Strings(files)

	named := regexp.MustCompile(`legal_requirements_category_check\s+CHECK\s*\(\s*category\s+IN\s*\(([^)]*)\)`)
	inline := regexp.MustCompile(`(?s)CREATE TABLE[^;]*?legal_requirements\s*\(.*?category\s+TEXT[^,]*?CHECK\s*\(\s*category\s+IN\s*\(([^)]*)\)`)

	var list, source string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		if m := named.FindAllStringSubmatch(text, -1); len(m) > 0 {
			list, source = m[len(m)-1][1], filepath.Base(f)
		} else if source == "" {
			if m := inline.FindStringSubmatch(text); m != nil {
				list, source = m[1], filepath.Base(f)
			}
		}
	}
	if source == "" {
		t.Fatal("found no legal_requirements category CHECK in any migration")
	}

	inDB := map[string]bool{}
	for _, m := range regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(list, -1) {
		inDB[m[1]] = true
	}
	inGo := map[string]bool{}
	for _, c := range LegalCategories {
		inGo[c] = true
	}

	var missing, extra []string // missing: in Go but not the constraint
	for c := range inGo {
		if !inDB[c] {
			missing = append(missing, c)
		}
	}
	for c := range inDB {
		if !inGo[c] {
			extra = append(extra, c)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Errorf("LegalCategories and the CHECK constraint (last defined in %s) differ:\n  in LegalCategories but not the constraint: %s\n  in the constraint but not LegalCategories: %s",
			source, strings.Join(missing, ", "), strings.Join(extra, ", "))
	}
}
