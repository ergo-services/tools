package appinitleak

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2023"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2023",
	Doc: `A2023: an application Init that returns an error after acquiring a resource.

The application lifecycle is not the process lifecycle, and this is the one place
the difference bites. When a process Init fails the framework still calls
ProcessTerminate with that error, so whatever the half built receiver holds is
handed back to the code that knows how to release it. When an application Init
fails, Terminate is not called at all: the start is aborted and the callback that
closes the pool never runs.

So a pool opened in Init and abandoned on a later failure is held for the rest of
the process, and every retried start opens another one. What makes this easy to
write is that the code reads as if it were symmetric: Init opens, Terminate closes,
and the failure path in between quietly is not covered by either.

The rule uses Terminate as the specification rather than a list of names. A field
Terminate releases is a resource by the author's own account, and an early return
in Init positioned after that field is assigned, in a block that does nothing else,
is the path that skips the release. A block that calls anything before returning is
taken as handling it, which is what a.release() or pool.Close() looks like.

Only an application behavior is reported. The same shape in a process Init is not a
defect, because ProcessTerminate runs there.

Source: application.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2023",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	inits := map[*types.Named]*ergomodel.Callback{}
	terminates := map[*types.Named]*ast.FuncDecl{}
	for _, cb := range m.Callbacks {
		if cb.Application == false || cb.Recv == "" {
			continue
		}
		switch cb.Name {
		case "Init":
			inits[cb.Behavior] = cb
		case "Terminate":
			terminates[cb.Behavior] = cb.Decl
		}
	}

	for behavior, cb := range inits {
		terminate := terminates[behavior]
		if terminate == nil {
			continue
		}
		released := releasedFields(pass, terminate)
		if len(released) == 0 {
			continue
		}
		acquired, first := acquisitions(pass, cb, released)
		if len(acquired) == 0 {
			continue
		}
		for _, ret := range leakingReturns(cb.Decl, first) {
			m.Report(pass, ret.Pos(),
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
					ID: ergomodel.CallbackID(cb), Witness: acquired[0].Name(),
				},
				"this returns an error after Init acquired %s, and an application whose Init fails never gets its Terminate called, so nothing closes it for the rest of the process; release what Init already opened on this path, the way Terminate does",
				acquired[0].Name())
		}
	}
	return nil, nil
}

func releasedFields(pass *analysis.Pass, decl *ast.FuncDecl) map[types.Object]bool {
	out := map[types.Object]bool{}
	if decl.Body == nil {
		return out
	}
	recv := receiverName(decl)
	if recv == "" {
		return out
	}
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok == false {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if isSel == false {
			return true
		}
		inner, isInner := sel.X.(*ast.SelectorExpr)
		if isInner == false {
			return true
		}
		id, isIdent := inner.X.(*ast.Ident)
		if isIdent == false || id.Name != recv {
			return true
		}
		if obj := pass.TypesInfo.Uses[inner.Sel]; obj != nil {
			if v, isVar := obj.(*types.Var); isVar && v.IsField() {
				out[obj] = true
			}
		}
		return true
	})
	return out
}

func acquisitions(pass *analysis.Pass, cb *ergomodel.Callback,
	released map[types.Object]bool) ([]types.Object, token.Pos) {

	var out []types.Object
	first := token.NoPos
	if cb.Decl.Body == nil {
		return out, first
	}
	ast.Inspect(cb.Decl.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if ok == false {
			return true
		}
		for _, lhs := range as.Lhs {
			sel, isSel := lhs.(*ast.SelectorExpr)
			if isSel == false {
				continue
			}
			id, isIdent := sel.X.(*ast.Ident)
			if isIdent == false || id.Name != cb.Recv {
				continue
			}
			obj := pass.TypesInfo.Uses[sel.Sel]
			if obj == nil || released[obj] == false {
				continue
			}
			out = append(out, obj)
			if first == token.NoPos || as.Pos() < first {
				first = as.Pos()
			}
		}
		return true
	})
	return out, first
}

func leakingReturns(decl *ast.FuncDecl, after token.Pos) []*ast.ReturnStmt {
	var out []*ast.ReturnStmt
	if decl.Body == nil || after == token.NoPos || len(decl.Body.List) == 0 {
		return out
	}
	last := decl.Body.List[len(decl.Body.List)-1]

	var walk func(list []ast.Stmt)
	walk = func(list []ast.Stmt) {
		handled := false
		for _, stmt := range list {
			switch x := stmt.(type) {
			case *ast.ExprStmt, *ast.DeferStmt, *ast.GoStmt:
				handled = true
			case *ast.AssignStmt:

				for _, rhs := range x.Rhs {
					if _, isCall := rhs.(*ast.CallExpr); isCall {
						handled = true
					}
				}
			case *ast.ReturnStmt:
				if handled || x.Pos() < after || stmt == last {
					continue
				}
				if len(x.Results) != 1 || isNil(x.Results[0]) {
					continue
				}
				out = append(out, x)
			case *ast.IfStmt:
				walk(x.Body.List)
				if block, isBlock := x.Else.(*ast.BlockStmt); isBlock {
					walk(block.List)
				}
			case *ast.BlockStmt:
				walk(x.List)
			case *ast.SwitchStmt:
				walkClauses(x.Body, walk)
			case *ast.TypeSwitchStmt:
				walkClauses(x.Body, walk)
			}
		}
	}
	walk(decl.Body.List)
	return out
}

func walkClauses(body *ast.BlockStmt, walk func([]ast.Stmt)) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		if clause, ok := stmt.(*ast.CaseClause); ok {
			walk(clause.Body)
		}
	}
}

func isNil(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

func receiverName(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 || len(decl.Recv.List[0].Names) == 0 {
		return ""
	}
	return decl.Recv.List[0].Names[0].Name
}
