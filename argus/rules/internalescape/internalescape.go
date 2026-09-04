package internalescape

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A1007"

var Analyzer = &analysis.Analyzer{
	Name: "argusA1007",
	Doc: `A1007: internal memory escaping across an actor boundary.

A method returns memory derived from a field of its receiver: a reslice of an internal
buffer, the address of an element, or a reference field returned directly. The owner
keeps mutating that memory under its own lock, and the receiving actor now holds a
slice into it and reads without that lock. Nothing about the message type shows this,
which is why no shape rule can see it.

This is the rule that found the only real defect in a production corpus sweep: a
buffer type documented the invariant in prose, one consumer honoured it with an
explicit copy and a comment, and its sibling did not.

Guardedness does not suppress this rule and neither does the []byte allowlist: the
corpus defect lives inside a properly locked type and is precisely a slice into a
byte buffer. What does suppress it is a copy at the send site, and append to a nil
slice, slices.Clone, bytes.Clone, maps.Clone and a string conversion are all
recognized as copies.

A receiver allocated and abandoned in the same callback is not reported, so handing
over the bytes of a local buffer stays quiet.`,
	URL:      "https://docs.ergo.services/tools/argus#A1007",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	type target struct {
		expr ast.Expr
		what string
		in   *ergomodel.Callback
	}
	var targets []target
	for _, site := range m.SendSites {
		targets = append(targets, target{site.Payload, site.Method, site.In})
	}
	for _, site := range m.SpawnSites() {
		for _, arg := range site.Args {
			targets = append(targets, target{arg, site.Method, site.In})
		}
	}

	for _, t := range targets {
		enclosing := enclosingBody(pass, t.expr)
		locals := localsAssignedIn(enclosing)
		singles := singleAssignments(enclosing)

		for _, call := range escapingCalls(pass, m, t.expr, singles) {

			if receiverIsLocal(call, locals) {
				continue
			}
			fn, ok := calleeOf(pass, call)
			if ok == false {
				continue
			}
			escapes := m.Behavior(fn).Escapes
			if len(escapes) == 0 {
				continue
			}
			m.Report(pass, call.Pos(),
				ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindEscape, Tier: 1,
					ID: ergomodel.FuncID(fn), Witness: escapes[0].Field},
				"%s returns memory derived from its receiver's %s field, and it is handed to %s: the owner keeps mutating it while the receiving actor reads it, so copy before sending",
				fn.Name(), escapes[0].Field, t.what)
		}
	}
	return nil, nil
}

func escapingCalls(pass *analysis.Pass, m *ergomodel.Model, e ast.Expr, singles map[string]ast.Expr) []*ast.CallExpr {
	var out []*ast.CallExpr
	seen := map[ast.Expr]bool{}

	var walk func(e ast.Expr, depth int)
	walk = func(e ast.Expr, depth int) {
		if e == nil || depth > 4 || seen[e] {
			return
		}
		seen[e] = true

		switch x := e.(type) {
		case *ast.CallExpr:

			if isConversion(pass, x) {
				if len(x.Args) == 1 {
					walk(x.Args[0], depth+1)
				}
				return
			}
			out = append(out, x)

		case *ast.CompositeLit:
			for _, elt := range x.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					walk(kv.Value, depth+1)
					continue
				}
				walk(elt, depth+1)
			}

		case *ast.UnaryExpr:
			walk(x.X, depth+1)

		case *ast.ParenExpr:
			walk(x.X, depth+1)

		case *ast.Ident:
			if rhs, ok := singles[x.Name]; ok {
				walk(rhs, depth+1)
			}
		}
	}
	walk(e, 0)
	return out
}

func isConversion(pass *analysis.Pass, call *ast.CallExpr) bool {
	if _, ok := call.Fun.(*ast.ArrayType); ok {
		return true
	}
	if id, ok := call.Fun.(*ast.Ident); ok {
		if _, isType := pass.TypesInfo.Uses[id].(*types.TypeName); isType {
			return true
		}
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if _, isType := pass.TypesInfo.Uses[sel.Sel].(*types.TypeName); isType {
			return true
		}
	}
	return false
}

func calleeOf(pass *analysis.Pass, call *ast.CallExpr) (*types.Func, bool) {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		fn, ok := pass.TypesInfo.Uses[fun.Sel].(*types.Func)
		return fn, ok
	case *ast.Ident:
		fn, ok := pass.TypesInfo.Uses[fun].(*types.Func)
		return fn, ok
	}
	return nil, false
}

func receiverIsLocal(call *ast.CallExpr, locals map[string]bool) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false {
		return false
	}
	base := sel.X
	for {
		switch x := base.(type) {
		case *ast.ParenExpr:
			base = x.X
		case *ast.StarExpr:
			base = x.X
		case *ast.UnaryExpr:
			base = x.X
		default:
			id, isIdent := base.(*ast.Ident)
			return isIdent && locals[id.Name]
		}
	}
}

func enclosingBody(pass *analysis.Pass, e ast.Expr) *ast.BlockStmt {
	for _, file := range pass.Files {
		if e.Pos() < file.Pos() || e.Pos() > file.End() {
			continue
		}
		var found *ast.BlockStmt
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if ok == false || fn.Body == nil {
				return true
			}
			if e.Pos() >= fn.Body.Pos() && e.Pos() <= fn.Body.End() {
				found = fn.Body
			}
			return true
		})
		return found
	}
	return nil
}

func localsAssignedIn(body *ast.BlockStmt) map[string]bool {
	out := map[string]bool{}
	if body == nil {
		return out
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok.String() != ":=" {
				return true
			}
			for _, lhs := range x.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					out[id.Name] = true
				}
			}
		case *ast.DeclStmt:
			gd, ok := x.Decl.(*ast.GenDecl)
			if ok == false {
				return true
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if ok == false {
					continue
				}
				for _, name := range vs.Names {
					out[name.Name] = true
				}
			}
		}
		return true
	})
	return out
}

func singleAssignments(body *ast.BlockStmt) map[string]ast.Expr {
	if body == nil {
		return nil
	}
	out := map[string]ast.Expr{}
	count := map[string]int{}

	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if ok == false || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		id, ok := as.Lhs[0].(*ast.Ident)
		if ok == false {
			return true
		}
		count[id.Name]++
		out[id.Name] = as.Rhs[0]
		return true
	})
	for name, n := range count {
		if n > 1 {
			delete(out, name)
		}
	}
	return out
}
