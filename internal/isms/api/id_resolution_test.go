package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #201: one :id param used to have three resolution strategies. The arithmetic
// strip (parseID) was the dangerous one — it turned "ASSET-3" into primary key 3
// without checking that the identifier belonged to an asset, or to anything, so
// a write could land on an unrelated row with a 200 and a changelog entry. These
// tests guard the two halves of the fix that need no database: the shape check
// that decides 400-vs-404, and the absence of any surviving strip.

func TestHasIdentifierShape(t *testing.T) {
	for _, tc := range []struct {
		param string
		want  bool
		why   string
	}{
		{"ASSET-3", true, "the ordinary register identifier"},
		{"LEGAL-1", true, "a prefix parseID never carried — used to 400"},
		{"ISMS2026-3", true, "objective display ID: program key with digits"},
		{"2026-3", true, "a program key may be all digits — only uppercased on create"},
		{"A-1", true, "shortest real shape"},
		{"ASSET-3-extra", true, "trailing segments are the lookup's problem, not the shape's"},
		{"garbage", false, "no dash at all"},
		{"", false, "empty param"},
		{"-3", false, "empty prefix"},
		{"ASSET-", false, "empty remainder"},
	} {
		if got := hasIdentifierShape(tc.param); got != tc.want {
			t.Errorf("hasIdentifierShape(%q) = %v, want %v (%s)", tc.param, got, tc.want, tc.why)
		}
	}
}

// TestNoArithmeticIDStripSurvives is the drift guard. parseID was deleted, but
// the failure mode is someone reintroducing the same shortcut under a new name:
// strip an identifier prefix, treat the remainder as a primary key. It matches
// both spellings that can do that here — strings.TrimPrefix and the package's own
// stripPrefix, which this change made the sanctioned idiom at three sites.
//
// "FIND-" and "AUDIT-" are the only allowed literals: those display ids ARE the
// primary key (db.SoftDeleteAuditFinding mints "FIND-<id>" from the row id) and
// neither table has an identifier column, so there is nothing to look up.
// Allow-listing the literals rather than the files means a new arithmetic strip
// in api_audit.go or api_references.go is still caught — which is how the CR-
// and TASK- strips in resolveEntityTitle were found.
func TestNoArithmeticIDStripSurvives(t *testing.T) {
	// The audit module is the exception: AUDIT- and FIND- are built from the row
	// id, so there is no identifier column to look up.
	auditPrefixes := map[string]bool{"FIND-": true, "AUDIT-": true}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var name string
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				if pkg, ok := fn.X.(*ast.Ident); ok && pkg.Name == "strings" {
					name = "strings." + fn.Sel.Name
				}
			case *ast.Ident:
				name = fn.Name
			}
			if name != "strings.TrimPrefix" && name != "stripPrefix" {
				return true
			}
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				v := strings.Trim(lit.Value, `"`)
				if auditPrefixes[v] {
					continue
				}
				// An identifier prefix is uppercase and ends in a dash.
				if len(v) > 1 && strings.HasSuffix(v, "-") && v == strings.ToUpper(v) {
					t.Errorf("%s: %s(..., %q) — identifier prefixes must be resolved by "+
						"lookup (resolve*ID), not stripped to a primary key (#201)",
						fset.Position(lit.Pos()), name, v)
				}
			}
			return true
		})
	}
}

// TestEntityIDResolverKeysAreNameable: every type in entityIDResolvers is passed
// to Entity() by handleEntityChangelog, and the client renders that by looking
// it up in common.entity_inline.*. A type with no key there produces a broken
// error message, which no other test would catch.
func TestEntityIDResolverKeysAreNameable(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", "locales", "en", "common.json"))
	if err != nil {
		t.Skipf("locale file unavailable: %v", err)
	}
	var common struct {
		EntityInline map[string]string `json:"entity_inline"`
	}
	if err := json.Unmarshal(raw, &common); err != nil {
		t.Fatalf("parsing common.json: %v", err)
	}
	for entityType := range entityIDResolvers {
		if entityType == "change" {
			continue // MCP alias for change_request; never rendered
		}
		if _, ok := common.EntityInline[entityType]; !ok {
			t.Errorf("entityIDResolvers has %q but common.entity_inline has no key for it — "+
				"the error message would render with a missing translation", entityType)
		}
	}
}
