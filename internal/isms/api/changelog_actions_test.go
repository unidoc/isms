package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// #295: the readings handlers wrote Action "reading", which
// entity_changelog_action_check did not allow. logChange only logs a failed
// write, so every row was dropped behind a 201 for five releases and no test
// noticed. This drift guard needs no database, so it runs in the CI unit job:
// every Action a db.ChangelogEntry literal is built with, anywhere under
// internal/isms, must be in the constraint as the migrations leave it.
func TestChangelogActionsAllowedBySchema(t *testing.T) {
	allowed := changelogActionsInSchema(t)

	used := map[string][]string{} // action → positions
	root := filepath.Join("..")   // internal/isms
	fset := token.NewFileSet()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			var elts []ast.Expr
			if isChangelogEntryType(lit.Type) {
				elts = lit.Elts
			} else if arr, ok := lit.Type.(*ast.ArrayType); ok && isChangelogEntryType(arr.Elt) {
				// []db.ChangelogEntry{{...}}: the element literals carry no type.
				for _, e := range lit.Elts {
					if inner, ok := e.(*ast.CompositeLit); ok && inner.Type == nil {
						elts = append(elts, inner.Elts...)
					}
				}
			}
			for _, e := range elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); !ok || key.Name != "Action" {
					continue
				}
				pos := fset.Position(kv.Pos()).String()
				bl, ok := kv.Value.(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					t.Errorf("%s: ChangelogEntry.Action is not a string literal, so this guard cannot check it against the schema", pos)
					continue
				}
				v, _ := strconv.Unquote(bl.Value)
				used[v] = append(used[v], pos)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(used) == 0 {
		t.Fatal("found no ChangelogEntry literals — the scan is broken, not the code")
	}

	for action, positions := range used {
		if !allowed[action] {
			t.Errorf("changelog action %q (%s) is not allowed by entity_changelog_action_check; widen it in the current release migration",
				action, strings.Join(positions, ", "))
		}
	}
}

func isChangelogEntryType(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "ChangelogEntry"
	case *ast.SelectorExpr:
		return x.Sel.Name == "ChangelogEntry"
	}
	return false
}

var (
	changelogTableRe = regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS entity_changelog \((.*?)\n\);`)
	inlineActionRe   = regexp.MustCompile(`(?s)CHECK \(action IN \((.*?)\)\)`)
	namedActionRe    = regexp.MustCompile(`(?s)ADD CONSTRAINT entity_changelog_action_check\s+CHECK \(action IN \((.*?)\)\)`)
	sqlStringRe      = regexp.MustCompile(`'([^']*)'`)
)

// changelogActionsInSchema replays the migrations in filename order (the order
// the runner applies them) and returns the action set the last definition of
// entity_changelog_action_check allows.
func changelogActionsInSchema(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	sort.Strings(files)

	var list string
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if m := changelogTableRe.FindSubmatch(src); m != nil {
			if c := inlineActionRe.FindSubmatch(m[1]); c != nil {
				list = string(c[1])
			}
		}
		for _, m := range namedActionRe.FindAllSubmatch(src, -1) {
			list = string(m[1])
		}
	}
	if list == "" {
		t.Fatal("could not find the entity_changelog action CHECK in migrations/")
	}
	allowed := map[string]bool{}
	for _, m := range sqlStringRe.FindAllStringSubmatch(list, -1) {
		allowed[m[1]] = true
	}
	return allowed
}
