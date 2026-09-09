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
	derived, _ := deriveNotificationKeyParams(t)

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

// TestFramesOnlySpendGuaranteedParams is the source-based half of the frame
// check, and the half that sees what a table cannot.
//
// TestFramesOnlyUseTheirOwnKeysParams asks whether a frame's slots are named in
// NotificationKeyParams. For a key written from one site that is the same
// question as this one. For a key written from several it is weaker, because the
// table entry is the union: suggestion_resolved's shared title frame is emitted
// for applied and for rejected, and the union entry names `entity`, `id` and
// `reason` even though no single emission sends all three. A `{reason}` there
// would render empty on every applied suggestion, in English and in every
// translation, and the table-based check would stay green.
//
// So a frame may only spend params guaranteed at every emission of its key.
// Still subset, not equality — a frame is free to use fewer than it is offered.
func TestFramesOnlySpendGuaranteedParams(t *testing.T) {
	_, guaranteed := deriveNotificationKeyParams(t)
	wireKeys := wireKeyForCatalogKey(t)
	for _, locale := range localesWithNotifications(t) {
		for key, frame := range loadNotificationLeaves(t, locale) {
			wireKey, ok := wireKeys[key]
			if !ok {
				continue // stale frame; TestENNotificationFramesAreEmitted owns it
			}
			sure, derived := guaranteed[wireKey]
			if !derived {
				continue // no resolvable write site; the equality check above reports it
			}
			for name := range placeholdersIn(frame) {
				if sure[name] {
					continue
				}
				t.Errorf("[%s] frame %q interpolates {%s}, which not every emission of %q sends\n"+
					"  frame: %q\n  sent by every site: %v\n"+
					"At least one write site reaches this frame without that param, so the "+
					"slot renders empty there. Either drop the slot, split the frame per "+
					"site, or make every site send the param.",
					locale, key, name, wireKey, frame, sortedSet(sure))
			}
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

// deriveNotificationKeyParams returns two sets per wire key.
//
// union is every param any emission of the key can send — what the table must
// equal, because a name in the table that no site sends is a slot a frame may
// interpolate empty.
//
// guaranteed is the intersection over emissions: the params present *whatever*
// path wrote the key. The two differ wherever one key is written from more than
// one site, and suggestion_resolved is exactly that — one shared title frame
// emitted for applied (`entity`, `id`) and rejected (`reason`). Against the
// union alone, a `{reason}` in that title passes every check and renders empty
// on the applied path, which is the failure this file exists to catch. Frames
// are held to the intersection; see TestFramesOnlySpendGuaranteedParams.
func deriveNotificationKeyParams(t *testing.T) (union, guaranteed map[string]map[string]bool) {
	t.Helper()
	values := notifyKeyConstValues(t)
	files := parseServerSources(t)
	calls := collectCalls(files)

	union = map[string]map[string]bool{}
	guaranteed = map[string]map[string]bool{}
	sites := 0
	for _, f := range files {
		for _, decl := range f.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			a := analyzeFunc(f.path, f.file.Name.Name, fn)
			for _, w := range a.writes {
				sites++
				for _, field := range []struct {
					name string
					expr ast.Expr
				}{{"TitleKey", w.titleKey}, {"BodyKey", w.bodyKey}} {
					if field.expr == nil {
						continue // deliberately untranslatable, or org-authored body
					}
					for _, e := range a.resolve(t, values, calls, field.expr, w) {
						if union[e.key] == nil {
							union[e.key] = map[string]bool{}
							guaranteed[e.key] = copySet(e.params)
						} else {
							intersect(guaranteed[e.key], e.params)
						}
						for p := range e.params {
							union[e.key][p] = true
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
	if len(union) == 0 {
		t.Fatal("resolved no wire keys from the write sites — key resolution is broken " +
			"and this test is now vacuous")
	}
	return union, guaranteed
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
// two generations would attribute the resubmitted site's doc_id to a key whose
// sites never send it.
//
// unsupported marks a write into the map this test cannot read — a dynamic
// index, `params[paramName] = value`. Its name is unknown, so the binding
// carries no name and instead fails the test if it can reach a write site:
// deriving short here would let a call site add a runtime param invisibly.
type paramBinding struct {
	name        string
	chain       []ast.Node
	pos         token.Pos
	isDecl      bool
	unsupported bool
}

type funcAnalysis struct {
	path   string
	pkg    string
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

	// recognised holds the assignments a more specific shape reader already
	// accepted, so the general reader does not see them as unsupported. The
	// wholesale-copy shape is a dynamic index assignment by construction.
	recognised map[*ast.AssignStmt]bool
}

func analyzeFunc(path, pkg string, fn *ast.FuncDecl) *funcAnalysis {
	a := &funcAnalysis{
		path:          path,
		pkg:           pkg,
		fn:            fn,
		keyBindings:   map[string][]keyBinding{},
		paramBindings: map[string][]paramBinding{},
		absorbs:       map[string][]string{},
		paramNames:    map[string]int{},
		recognised:    map[*ast.AssignStmt]bool{},
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
			if isNotifWriteLit(v, a.pkg) {
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
//
// A map write it cannot read — a dynamic index, or a literal whose keys are not
// string constants — is recorded as unsupported rather than skipped. Skipping
// it would derive the param set short and leave that site's real parameters
// unchecked, which is the failure this whole test exists to catch.
func (a *funcAnalysis) recordAssign(as *ast.AssignStmt, chain []ast.Node) {
	if as.Tok != token.ASSIGN && as.Tok != token.DEFINE {
		return
	}
	if a.recognised[as] {
		return // the wholesale-copy shape, already read by recordRangeCopy
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
				keys, unreadable := mapLiteralKeys(lit)
				if unreadable {
					a.paramBindings[target.Name] = append(a.paramBindings[target.Name],
						paramBinding{chain: chain, pos: as.Pos(), isDecl: true, unsupported: true})
				}
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
			key, ok := stringLit(target.Index)
			if !ok {
				// params[paramName] = value — the name is not in the source.
				a.paramBindings[ident.Name] = append(a.paramBindings[ident.Name],
					paramBinding{chain: chain, pos: as.Pos(), unsupported: true})
				continue
			}
			a.paramBindings[ident.Name] = append(a.paramBindings[ident.Name],
				paramBinding{name: key, chain: chain, pos: as.Pos()})
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
	a.recognised[as] = true
	a.absorbs[dst.Name] = append(a.absorbs[dst.Name], src.Name)
}

// ── resolution ───────────────────────────────────────────────────────────────

// emission is one resolved (write site, wire key) pair: the params that reach
// that one write when that one key is the one written. Kept separate rather
// than merged per key so the intersection over a key's emissions is available
// — merging first is what let the shared suggestion_resolved title frame claim
// a param only one of its two callers sends.
type emission struct {
	key    string
	params map[string]bool
}

// resolve returns one emission per wire key the given key expression can carry
// on a distinct path to this write site.
func (a *funcAnalysis) resolve(
	t *testing.T,
	values map[string]string,
	calls map[string][]*ast.CallExpr,
	keyExpr ast.Expr,
	w writeSite,
) []emission {
	t.Helper()
	var out []emission

	// Case 1 — the key is named directly, so it is written on every path.
	if name, ok := notifyKeyConstName(keyExpr); ok {
		return a.emissionsFor(t, calls, w, nil, a.wireValue(t, values, name))
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
			out = append(out, a.emissionsFor(t, calls, w, b.chain, a.wireValue(t, values, b.constName))...)
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
		out = append(out, emission{
			key:    a.wireValue(t, values, name),
			params: a.paramsReaching(t, map[string][]*ast.CallExpr{a.fn.Name.Name: {call}}, w, nil),
		})
	}
	return out
}

// emissionsFor turns one resolved key at one write site into its emissions.
//
// Usually that is one. It is one *per caller* when the params map absorbs a
// function parameter wholesale, because then each caller supplies a different
// map to the same write: notifySuggestionResolved names its title key directly,
// yet the applied and rejected callers pass `{entity,id}` and `{reason}`. Union
// those into one emission and the intersection over the key becomes the union,
// which is precisely the blind spot the intersection was added to remove.
func (a *funcAnalysis) emissionsFor(
	t *testing.T,
	calls map[string][]*ast.CallExpr,
	w writeSite,
	keyChain []ast.Node,
	key string,
) []emission {
	t.Helper()
	ident, isIdent := w.params.(*ast.Ident)
	if !isIdent || len(a.absorbs[ident.Name]) == 0 {
		return []emission{{key: key, params: a.paramsReaching(t, calls, w, keyChain)}}
	}
	sites := calls[a.fn.Name.Name]
	if len(sites) == 0 {
		t.Errorf("%s: %s copies its Params from a function parameter but has no resolvable "+
			"callers\nThe params of the keys it writes cannot be read, so they go unchecked.",
			a.path, a.fn.Name.Name)
		return nil
	}
	out := make([]emission, 0, len(sites))
	for _, call := range sites {
		out = append(out, emission{
			key:    key,
			params: a.paramsReaching(t, map[string][]*ast.CallExpr{a.fn.Name.Name: {call}}, w, keyChain),
		})
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
		keys, unreadable := mapLiteralKeys(lit)
		if unreadable {
			t.Errorf("%s: %s builds Params from a map literal with a key this test cannot "+
				"read\nA param whose name is not a string literal in the source cannot be "+
				"checked against the table; give it a literal key.", a.path, a.fn.Name.Name)
		}
		for _, name := range keys {
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
		if b.unsupported {
			t.Errorf("%s: %s writes into %q through a shape this test cannot read, and "+
				"that write reaches a notification write site\n"+
				"The param it adds is invisible to the derivation, so the key's set would "+
				"derive short and the site go unchecked. Use a string-literal key, or teach "+
				"this test the shape.", a.path, a.fn.Name.Name, ident.Name)
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
			keys, unreadable := mapLiteralKeys(lit)
			if unreadable {
				t.Errorf("%s: a call to %s passes %q as a map literal with a key this test "+
					"cannot read\nThe param it carries would derive short.",
					a.path, a.fn.Name.Name, src)
			}
			for _, name := range keys {
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

// isNotifWriteLit matches the write structs by type name, qualified
// (db.NotificationContent) or bare, and skips package db itself.
//
// Skipping db skips its unqualified re-packings — CreateNotification builds a
// NotificationContent purely to reach keyedColumns, and it carries no constant.
// That exclusion is safe by construction rather than by luck: the NotifyKey*
// constants live in package api, db cannot import api without a cycle, and a
// bare string is forbidden by TestKeyedWritesUseDeclaredConstants. A keyed
// write therefore cannot originate in db.
//
// Outside db, a bare name is matched too, so a local type alias or a dot-import
// of the write struct is read rather than skipped. Matching on the name alone
// would need go/types to be exact; a same-named unrelated struct here would
// widen a derived set and fail loudly, which is the right way round.
func isNotifWriteLit(lit *ast.CompositeLit, pkg string) bool {
	switch typ := lit.Type.(type) {
	case *ast.SelectorExpr:
		return notifWriteTypes[typ.Sel.Name]
	case *ast.Ident:
		return pkg != "db" && notifWriteTypes[typ.Name]
	}
	return false
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

// mapLiteralKeys returns the string-literal keys of a map literal, and whether
// it holds an entry this test cannot read. A non-literal key is a param name
// absent from the source: adding `map[string]any{paramName: value}` to a
// notification map would otherwise add a runtime param without changing the
// derived set, and this guard would stay green over it. The caller fails.
func mapLiteralKeys(lit *ast.CompositeLit) ([]string, bool) {
	var out []string
	unreadable := false
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			unreadable = true
			continue
		}
		key, ok := stringLit(kv.Key)
		if !ok {
			unreadable = true
			continue
		}
		out = append(out, key)
	}
	return out, unreadable
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

func copySet(s map[string]bool) map[string]bool {
	out := make(map[string]bool, len(s))
	for n := range s {
		out[n] = true
	}
	return out
}

// intersect narrows dst to the names src also holds.
func intersect(dst, src map[string]bool) {
	for n := range dst {
		if !src[n] {
			delete(dst, n)
		}
	}
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
