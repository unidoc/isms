package api

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestNotificationKeyParamsMatchesCallSites derives NotificationKeyParams from
// the source and asserts the hand-written table equals it.
//
// This closes the last direction of the param guard. The catalogue checks in
// notification_keys_test.go both run *through* the table:
// TestNotificationKeyParamsIsExhaustive asks whether every wire key has an
// entry, and TestFramesOnlyUseTheirOwnKeysParams asks whether a frame's slots
// are named in that entry. Neither asks whether the entry is true. A call site
// that dropped a param left all of them green and rendered "Alip approved "
// with the slot silently empty — in English as well as every translation, which
// is the same invisible failure the whole key mechanism exists to prevent,
// arriving through the one door nothing watched.
//
// Equality, not subset, in either direction. The table over-declaring was
// previously called a fail-safe — it only widens what a frame may ask for —
// but "fails safe" and "is checked" are different properties, and the frame
// check is only as good as the table it reads. Equality also makes every
// weakness in the derivation below loud: over-derive or under-derive and this
// test fails rather than quietly relaxing.
//
// The table stays hand-written on purpose. It is the readable spec, it is what
// TestFramesOnlyUseTheirOwnKeysParams indexes, and its per-key comments carry
// reasoning no derivation can express. What changes is its standing: it used to
// be an assertion about the code, and is now a claim the code is checked
// against. On disagreement the source wins.
func TestNotificationKeyParamsMatchesCallSites(t *testing.T) {
	derived := deriveNotificationKeyParams(t)

	for key, want := range derived {
		got := sortedSet(setOf(NotificationKeyParams[key]))
		if _, declared := NotificationKeyParams[key]; !declared {
			t.Errorf("wire key %q is written with params %v but has no entry in NotificationKeyParams",
				key, sortedSet(want))
			continue
		}
		if !reflect.DeepEqual(got, sortedSet(want)) {
			t.Errorf("NotificationKeyParams[%q] does not match its call sites\n"+
				"  declared: %v\n"+
				"  call sites send: %v\n"+
				"A name in the declaration only is a slot no site fills, so a frame may "+
				"interpolate it and render empty. A name in the call sites only is a param "+
				"no frame is allowed to use. Fix whichever is wrong; the source is the "+
				"authority for this table.", key, got, sortedSet(want))
		}
	}
	for key := range NotificationKeyParams {
		if _, ok := derived[key]; !ok {
			t.Errorf("NotificationKeyParams declares %q, which no notification write site emits\n"+
				"Either the write was removed and the entry is stale, or the key reaches "+
				"db.NotificationContent through a shape this test cannot read — in which "+
				"case teach it the shape rather than deleting the entry.", key)
		}
	}
}

// ── derivation ───────────────────────────────────────────────────────────────
//
// Two resolution rules, and the difference matters enough to state plainly:
// the derived set for a key is the params that can reach the write site *when
// that key is the one being written*.
//
//   - A key named directly in the composite literal (TitleKey:
//     NotifyKeyReviewForwarded) reaches the write on every path through the
//     function, so it takes the function-wide union — including the `note` that
//     only an `if req.Message != ""` branch adds.
//   - A key held in a variable (BodyKey: bodyKey) reaches the write only on the
//     paths where that variable was last assigned it, so it takes only the
//     params assigned in blocks that enclose that assignment. This is what
//     separates the three `_with_note` bodies from their plain siblings: the
//     branch that sets `bodyKey = …BodyWithNote` is the same branch that sets
//     `params["note"]`, and the plain assignment sits outside it.
//
// A key held in a *function parameter* (notifyMentions, notifySuggestionResolved)
// is resolved one level out, per call site, so each caller's key is paired with
// that same caller's param map rather than with the union of all of them.
//
// Everything here is deliberately shape-specific. An unrecognised shape fails
// the test instead of being guessed at: guessing is how a hand-maintained table
// became indistinguishable from a derived one in the first place.

// notifWriteTypes are the structs a stored notification is written through.
// Both carry the key/param triple: NotificationContent resolves a recipient by
// email, Notification takes a recipient id. Reading only the first missed the
// incident_new write entirely and derived nothing for it.
var notifWriteTypes = map[string]bool{
	"NotificationContent": true,
	"Notification":        true,
}

// scanRoots mirrors TestKeyedWritesUseDeclaredConstants: the next write site
// need not land in this package — the MCP server writes notifications too.
var scanRoots = []string{"../../../internal", "../../../cmd"}

func deriveNotificationKeyParams(t *testing.T) map[string]map[string]bool {
	t.Helper()
	values := notifyKeyConstValues(t)
	files := parseServerSources(t)
	calls := collectCalls(files)

	derived := map[string]map[string]bool{}
	sites := 0
	for _, f := range files {
		for _, decl := range f.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			a := analyzeFunc(f.path, fn)
			for _, w := range a.writes {
				sites++
				for _, field := range []struct {
					name string
					expr ast.Expr
				}{{"TitleKey", w.titleKey}, {"BodyKey", w.bodyKey}} {
					if field.expr == nil {
						continue // deliberately untranslatable, or org-authored body
					}
					for key, params := range a.resolve(t, values, calls, field.expr, w) {
						if derived[key] == nil {
							derived[key] = map[string]bool{}
						}
						for p := range params {
							derived[key][p] = true
						}
					}
				}
			}
		}
	}
	if sites == 0 {
		t.Fatalf("found no notification composite literals (%v) under %v — the write shape "+
			"changed and this test is now vacuous", sortedSet(notifWriteTypes), scanRoots)
	}
	if len(derived) == 0 {
		t.Fatal("resolved no wire keys from the write sites — key resolution is broken " +
			"and this test is now vacuous")
	}
	return derived
}

// ── per-function analysis ────────────────────────────────────────────────────

type writeSite struct {
	titleKey ast.Expr
	bodyKey  ast.Expr
	params   ast.Expr
	pos      token.Pos
	chain    []ast.Node
}

// keyBinding is one assignment of a wire-key constant to a local variable,
// carried with the block chain it happened in.
type keyBinding struct {
	constName string
	chain     []ast.Node
	pos       token.Pos
}

// paramBinding is one param name made available to a map variable, carried with
// the block chain that made it available. isDecl marks the map literal that
// (re)declared the variable, which is what bounds one generation of it: a
// handler that sends two notifications declares `params` twice, and merging the
// two derived doc_id onto a key whose sites never send it.
type paramBinding struct {
	name   string
	chain  []ast.Node
	pos    token.Pos
	isDecl bool
}

type funcAnalysis struct {
	path   string
	fn     *ast.FuncDecl
	writes []writeSite

	// keyBindings maps a local variable name to the wire-key constants assigned
	// to it. paramBindings maps a map variable name to the param names it can
	// hold. absorbs maps a map variable name to the function parameters whose
	// contents it copies in wholesale (the `for k, v := range src` shape).
	keyBindings   map[string][]keyBinding
	paramBindings map[string][]paramBinding
	absorbs       map[string][]string
	paramNames    map[string]int // function parameter name -> position
}

func analyzeFunc(path string, fn *ast.FuncDecl) *funcAnalysis {
	a := &funcAnalysis{
		path:          path,
		fn:            fn,
		keyBindings:   map[string][]keyBinding{},
		paramBindings: map[string][]paramBinding{},
		absorbs:       map[string][]string{},
		paramNames:    map[string]int{},
	}
	pos := 0
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			for _, name := range field.Names {
				a.paramNames[name.Name] = pos
				pos++
			}
		}
	}

	var stack []ast.Node
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		chain := blockChain(stack)

		switch v := n.(type) {
		case *ast.CompositeLit:
			if isNotifWriteLit(v) {
				w := notifWrite(v)
				w.chain = chain
				a.writes = append(a.writes, w)
			}
		case *ast.AssignStmt:
			a.recordAssign(v, chain)
		case *ast.RangeStmt:
			a.recordRangeCopy(v)
		}
		return true
	})
	return a
}

// recordAssign reads the two assignment shapes the write sites use: binding a
// wire-key constant or a map literal to a variable, and adding a single param
// to an existing map by literal index.
func (a *funcAnalysis) recordAssign(as *ast.AssignStmt, chain []ast.Node) {
	if as.Tok != token.ASSIGN && as.Tok != token.DEFINE {
		return
	}
	for i, lhs := range as.Lhs {
		if i >= len(as.Rhs) {
			return // multi-value assignment; no shape here binds a key that way
		}
		rhs := as.Rhs[i]
		switch target := lhs.(type) {
		case *ast.Ident:
			if name, ok := notifyKeyConstName(rhs); ok {
				a.keyBindings[target.Name] = append(a.keyBindings[target.Name],
					keyBinding{constName: name, chain: chain, pos: as.Pos()})
				continue
			}
			if lit, ok := rhs.(*ast.CompositeLit); ok && isStringKeyedMap(lit) {
				keys := mapLiteralKeys(lit)
				if len(keys) == 0 {
					// An empty map literal still opens a generation, and losing
					// it would let the previous one leak across.
					a.paramBindings[target.Name] = append(a.paramBindings[target.Name],
						paramBinding{chain: chain, pos: as.Pos(), isDecl: true})
				}
				for _, name := range keys {
					a.paramBindings[target.Name] = append(a.paramBindings[target.Name],
						paramBinding{name: name, chain: chain, pos: as.Pos(), isDecl: true})
				}
			}
		case *ast.IndexExpr:
			// params["note"] = req.Message
			ident, ok := target.X.(*ast.Ident)
			if !ok {
				continue
			}
			if key, ok := stringLit(target.Index); ok {
				a.paramBindings[ident.Name] = append(a.paramBindings[ident.Name],
					paramBinding{name: key, chain: chain, pos: as.Pos()})
			}
		}
	}
}

// recordRangeCopy reads the one wholesale-copy shape in the tree:
//
//	for k, v := range bodyParams { params[k] = v }
//
// The index is an identifier, not a string literal, so a literal-key scan
// derives {action} for the three suggestion keys and misses everything the
// caller supplied. Only this exact shape is recognised — copying from a
// function parameter, one statement, both loop variables used verbatim — and
// only so the caller's map can be resolved at the call site. Anything looser
// would be a guess.
func (a *funcAnalysis) recordRangeCopy(rs *ast.RangeStmt) {
	src, ok := rs.X.(*ast.Ident)
	if !ok {
		return
	}
	if _, isParam := a.paramNames[src.Name]; !isParam {
		return
	}
	k, kok := rs.Key.(*ast.Ident)
	v, vok := rs.Value.(*ast.Ident)
	if !kok || !vok || rs.Body == nil || len(rs.Body.List) != 1 {
		return
	}
	as, ok := rs.Body.List[0].(*ast.AssignStmt)
	if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return
	}
	idx, ok := as.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return
	}
	dst, ok := idx.X.(*ast.Ident)
	if !ok || !identNamed(idx.Index, k.Name) || !identNamed(as.Rhs[0], v.Name) {
		return
	}
	a.absorbs[dst.Name] = append(a.absorbs[dst.Name], src.Name)
}

// ── resolution ───────────────────────────────────────────────────────────────

// resolve pairs each wire key the given key expression can carry with the
// params that can reach the write site when that key is the one written.
func (a *funcAnalysis) resolve(
	t *testing.T,
	values map[string]string,
	calls map[string][]*ast.CallExpr,
	keyExpr ast.Expr,
	w writeSite,
) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}

	// Case 1 — the key is named directly, so it is written on every path.
	if name, ok := notifyKeyConstName(keyExpr); ok {
		out[a.wireValue(t, values, name)] = a.paramsReaching(t, calls, w, nil)
		return out
	}

	ident, ok := keyExpr.(*ast.Ident)
	if !ok {
		t.Errorf("%s: %s at %d is neither a NotifyKey* constant nor a variable (%T)\n"+
			"Every wire key must be resolvable, or its params go unchecked.",
			a.path, a.fn.Name.Name, a.fn.Pos(), keyExpr)
		return out
	}

	// Case 2 — the key is a local variable, so it is written only on the paths
	// where it was last assigned that constant. Only bindings that can reach
	// THIS write count: a handler that sends two notifications reuses the name
	// `bodyKey`, and the other one's assignments sit in a sibling block.
	if all := a.keyBindings[ident.Name]; len(all) > 0 {
		var reaching []keyBinding
		for _, b := range all {
			if b.pos < w.pos && reaches(w.chain, b.chain) {
				reaching = append(reaching, b)
			}
		}
		if len(reaching) == 0 {
			t.Errorf("%s: %s writes %q as a wire key but no assignment to it reaches the "+
				"write site\nThe resolution shape changed; teach this test the new one.",
				a.path, a.fn.Name.Name, ident.Name)
			return out
		}
		for _, b := range reaching {
			key := a.wireValue(t, values, b.constName)
			if out[key] == nil {
				out[key] = map[string]bool{}
			}
			for p := range a.paramsReaching(t, calls, w, b.chain) {
				out[key][p] = true
			}
		}
		return out
	}

	// Case 3 — the key is a function parameter, so each caller's key pairs with
	// that same caller's params.
	argPos, isParam := a.paramNames[ident.Name]
	if !isParam {
		t.Errorf("%s: %s uses %q as a wire key but never assigns it a NotifyKey* constant "+
			"and it is not a parameter\nThe resolution shape changed; teach this test the "+
			"new one rather than leaving the key unchecked.", a.path, a.fn.Name.Name, ident.Name)
		return out
	}
	sites := calls[a.fn.Name.Name]
	if len(sites) == 0 {
		t.Errorf("%s: %s takes its wire key as a parameter but has no resolvable callers\n"+
			"Its keys and params cannot be paired, so they go unchecked.", a.path, a.fn.Name.Name)
		return out
	}
	for _, call := range sites {
		if argPos >= len(call.Args) {
			continue // variadic or reshaped signature; the caller scan is name-based
		}
		name, ok := notifyKeyConstName(call.Args[argPos])
		if !ok {
			t.Errorf("%s: a call to %s passes a wire key that is not a NotifyKey* constant\n"+
				"TestKeyedWritesUseDeclaredConstants forbids the literal; this forbids "+
				"anything else unresolvable.", a.path, a.fn.Name.Name)
			continue
		}
		key := a.wireValue(t, values, name)
		if out[key] == nil {
			out[key] = map[string]bool{}
		}
		for p := range a.paramsReaching(t, map[string][]*ast.CallExpr{a.fn.Name.Name: {call}}, w, nil) {
			out[key][p] = true
		}
	}
	return out
}

// paramsReaching returns the param names the params expression can hold at the
// given write site. When keyChain is non-nil only bindings from the block that
// assigned that key (or an enclosing one) count — the path-sensitive half that
// separates a _with_note body from its plain sibling. calls supplies the
// callers used to resolve an absorbed parameter.
func (a *funcAnalysis) paramsReaching(
	t *testing.T,
	calls map[string][]*ast.CallExpr,
	w writeSite,
	keyChain []ast.Node,
) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	if w.params == nil {
		return got
	}

	// An inline map literal at the write site: nothing conditional to resolve.
	if lit, ok := w.params.(*ast.CompositeLit); ok {
		if !isStringKeyedMap(lit) {
			t.Errorf("%s: %s builds Params from a composite literal this test cannot read",
				a.path, a.fn.Name.Name)
			return got
		}
		for _, name := range mapLiteralKeys(lit) {
			got[name] = true
		}
		return got
	}

	ident, ok := w.params.(*ast.Ident)
	if !ok {
		t.Errorf("%s: %s builds Params from a %T, which this test cannot read\n"+
			"Params must be an inline map literal or a variable built in the function.",
			a.path, a.fn.Name.Name, w.params)
		return got
	}

	all := a.paramBindings[ident.Name]
	absorbed := a.absorbs[ident.Name]
	if len(all) == 0 && len(absorbed) == 0 {
		t.Errorf("%s: %s passes %q as Params but never builds it\n"+
			"The param set for its keys would silently derive as empty.",
			a.path, a.fn.Name.Name, ident.Name)
		return got
	}

	// Pick the generation of the variable this write reads: the most specific
	// declaration whose block encloses the write, latest first. Without this,
	// handleReviewSend's two `params := …` maps merge and the resubmitted
	// site's doc_id is attributed to review_requested as well.
	var gen *paramBinding
	for i := range all {
		b := all[i]
		if !b.isDecl || !encloses(b.chain, w.chain) || b.pos > w.pos {
			continue
		}
		if gen == nil || len(b.chain) > len(gen.chain) ||
			(len(b.chain) == len(gen.chain) && b.pos > gen.pos) {
			gen = &all[i]
		}
	}
	if gen == nil && len(absorbed) == 0 {
		t.Errorf("%s: %s reads %q at a write site no declaration of it reaches\n"+
			"The resolution shape changed; teach this test the new one.",
			a.path, a.fn.Name.Name, ident.Name)
		return got
	}

	for _, b := range all {
		if gen != nil && b.pos < gen.pos {
			continue // an earlier generation of the same name
		}
		if b.pos > w.pos {
			continue // see the position note on reaches
		}
		if !reaches(w.chain, b.chain) {
			continue
		}
		if keyChain != nil && !encloses(b.chain, keyChain) {
			continue
		}
		if b.name != "" {
			got[b.name] = true
		}
	}

	for _, src := range absorbed {
		pos, isParam := a.paramNames[src]
		if !isParam {
			continue
		}
		for _, call := range calls[a.fn.Name.Name] {
			if pos >= len(call.Args) {
				continue
			}
			lit, ok := call.Args[pos].(*ast.CompositeLit)
			if !ok || !isStringKeyedMap(lit) {
				t.Errorf("%s: a call to %s passes %q as something other than a map literal\n"+
					"Its keys cannot be read, so the params of the wire key it carries "+
					"would derive short.", a.path, a.fn.Name.Name, src)
				continue
			}
			for _, name := range mapLiteralKeys(lit) {
				got[name] = true
			}
		}
	}
	return got
}

func (a *funcAnalysis) wireValue(t *testing.T, values map[string]string, constName string) string {
	t.Helper()
	value, ok := values[constName]
	if !ok {
		t.Fatalf("%s: %s writes %s, which is not a constant in %s",
			a.path, a.fn.Name.Name, constName, notifyKeyDeclFile)
	}
	return value
}

// ── source reading ───────────────────────────────────────────────────────────

const notifyKeyDeclFile = "notification_keys.go"

type parsedFile struct {
	path string
	file *ast.File
}

func parseServerSources(t *testing.T) []parsedFile {
	t.Helper()
	var out []parsedFile
	for _, root := range scanRoots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if perr != nil {
				return fmt.Errorf("parsing %s: %w", path, perr)
			}
			out = append(out, parsedFile{path: path, file: f})
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if len(out) == 0 {
		t.Fatalf("parsed no Go files under %v — the source layout changed and this test "+
			"is now vacuous", scanRoots)
	}
	return out
}

// notifyKeyConstValues maps each NotifyKey* constant name to its wire value.
// Parsed rather than reflected because Go has no reflection over package-level
// constants; the same reason TestNotificationKeysIsExhaustive reads the source.
func notifyKeyConstValues(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), notifyKeyDeclFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", notifyKeyDeclFile, err)
	}
	values := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "NotifyKey") || i >= len(vs.Values) {
					continue
				}
				if value, ok := stringLit(vs.Values[i]); ok {
					values[name.Name] = value
				}
			}
		}
	}
	if len(values) == 0 {
		t.Fatalf("found no NotifyKey* constants in %s — the declaration shape changed",
			notifyKeyDeclFile)
	}
	return values
}

// collectCalls indexes every call by callee name. Name-based rather than
// type-resolved: the two helpers that take a wire key as a parameter are
// unexported methods with distinctive names, and go/types would pull the whole
// module's type-checking into a unit test for no extra precision here. A
// same-named method elsewhere would widen a derived set and so fail loudly.
func collectCalls(files []parsedFile) map[string][]*ast.CallExpr {
	calls := map[string][]*ast.CallExpr{}
	for _, f := range files {
		ast.Inspect(f.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				calls[fun.Name] = append(calls[fun.Name], call)
			case *ast.SelectorExpr:
				calls[fun.Sel.Name] = append(calls[fun.Sel.Name], call)
			}
			return true
		})
	}
	return calls
}

// ── AST helpers ──────────────────────────────────────────────────────────────

// isNotifWriteLit matches a qualified literal only (db.NotificationContent),
// which skips the unqualified re-packings inside package db itself —
// CreateNotification builds a NotificationContent purely to reach
// keyedColumns, and it carries no constant. That exclusion is safe by
// construction rather than by luck: the NotifyKey* constants live in package
// api, db cannot import api without a cycle, and a bare string is forbidden by
// TestKeyedWritesUseDeclaredConstants. A keyed write therefore cannot originate
// in db. If one ever needs to, this is the line to widen.
func isNotifWriteLit(lit *ast.CompositeLit) bool {
	sel, ok := lit.Type.(*ast.SelectorExpr)
	return ok && notifWriteTypes[sel.Sel.Name]
}

func notifWrite(lit *ast.CompositeLit) writeSite {
	w := writeSite{pos: lit.Pos()}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "TitleKey":
			w.titleKey = kv.Value
		case "BodyKey":
			w.bodyKey = kv.Value
		case "Params":
			w.params = kv.Value
		}
	}
	return w
}

func notifyKeyConstName(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.Ident:
		if strings.HasPrefix(v.Name, "NotifyKey") {
			return v.Name, true
		}
	case *ast.SelectorExpr:
		if strings.HasPrefix(v.Sel.Name, "NotifyKey") {
			return v.Sel.Name, true
		}
	}
	return "", false
}

func isStringKeyedMap(lit *ast.CompositeLit) bool {
	mt, ok := lit.Type.(*ast.MapType)
	if !ok {
		return false
	}
	ident, ok := mt.Key.(*ast.Ident)
	return ok && ident.Name == "string"
}

func mapLiteralKeys(lit *ast.CompositeLit) []string {
	var out []string
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := stringLit(kv.Key); ok {
			out = append(out, key)
		}
	}
	return out
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

func identNamed(e ast.Expr, name string) bool {
	ident, ok := e.(*ast.Ident)
	return ok && ident.Name == name
}

// blockChain reduces a node stack to the enclosing blocks, which is the only
// part of it that decides reachability.
func blockChain(stack []ast.Node) []ast.Node {
	var chain []ast.Node
	for _, n := range stack {
		if _, ok := n.(*ast.BlockStmt); ok {
			chain = append(chain, n)
		}
	}
	return append([]ast.Node(nil), chain...)
}

// encloses reports whether every block in outer also encloses inner — that is,
// whether outer is an ancestor of (or the same as) inner's block.
func encloses(outer, inner []ast.Node) bool {
	if len(outer) > len(inner) {
		return false
	}
	for i := range outer {
		if outer[i] != inner[i] {
			return false
		}
	}
	return true
}

// reaches reports whether an assignment made in block chain `from` is on the
// same path as a write in block chain `at`.
//
// Callers pair it with a position bound: an assignment textually after the
// write does not reach it, and counting one would over-derive and blame a call
// site for a param it does not send. That is exact for the shapes here and an
// approximation in general — a variable reassigned after the write inside a
// loop body would reach the *next* iteration, and the bound would drop it. No
// site does that today, and dropping it fails loudly rather than deriving a
// wrong set quietly, which is the intended response to a shape this test has
// not seen. Teach it the shape if one appears.
//
// True when the two chains are on one path through the
// function, in either direction. Both directions occur and both are needed:
// `params := …` at function level is an ancestor of a write nested in a loop
// and an `if`, while `params["note"] = …` sits in a branch nested inside the
// write's own block. Chains on neither side of each other are sibling branches,
// which is what keeps a handler that sends two different notifications from
// attributing one's params to the other.
func reaches(at, from []ast.Node) bool {
	return encloses(at, from) || encloses(from, at)
}

func setOf(names []string) map[string]bool {
	out := map[string]bool{}
	for _, n := range names {
		out[n] = true
	}
	return out
}

func sortedSet(s map[string]bool) []string {
	out := make([]string, 0, len(s))
	for n := range s {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
