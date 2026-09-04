package terminatehalfbuilt

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2014a"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2014a",
	Doc: `A2014a: Terminate dereferencing something Init may never have assigned.

Terminate does not only run after a working process stops. When Init returns an error
the spawn path calls cleanupProcess and then ProcessTerminate with that same error,
on a fresh goroutine, against the receiver Init abandoned. When Init exceeds its
timeout the other path does the same with TerminateReasonKill. So every field assigned
after the first thing in Init that can fail is nil in a Terminate that runs on those
paths, and a Terminate written for the healthy case dereferences it.

What that costs is worse than it looks. The failure path runs Terminate on its own
goroutine, and the panic boundary there is the same build-tag switch as everywhere
else: the recover is installed only when lib.Recover is on. Under -tags=norecover a
nil dereference in a Terminate reached this way is a panic on a bare goroutine, which
takes the node down. With recovery on it is a Panic log, and the original Init error
that caused all of this is what the spawner sees, so the log and the returned error
disagree about what went wrong.

A meta process is excluded, because the same claim is false there. A meta's Init is
called synchronously from SpawnMeta, and when it returns an error SpawnMeta returns
immediately: the "go m.start()" that would eventually invoke Terminate is never
reached, so a meta whose Init failed has no Terminate at all. The framework's own
meta/port.go is the shape this exclusion protects, and it reads like the defect: it
guards on cmd, which is assigned before anything can fail, while the three pipes it
closes are the fields that can be nil. For a meta that guard is redundant rather than
misplaced.

The predicate is deliberately narrow. A field is late if every assignment to it in
Init sits after the first statement in Init that can return early, since a field
assigned before that point is set on every path that reaches Terminate at all. A late
field is reported when Terminate dereferences it and the type can be nil: a call
through it, a selection through it, an index, a receive, or an explicit star. Passing
it to something is not a dereference, because a nil pointer argument is only a problem
in the callee's frame, and the callee is judged on its own.

Any nil comparison against the field anywhere in Terminate counts as a guard, without
asking whether the guard dominates the use. Getting that wrong in the quiet direction
costs recall on code that is already thinking about the problem, and getting it wrong
in the loud direction reports code that already has the fix.

A guard also covers every field Init assigns before the guarded one, and that is not
leniency: Init assigns in order, so if the later field is non nil the earlier one was
reached. Real code guards the last thing it built and then uses everything, which is
correct and would otherwise be reported. The direction matters, which is why the
implication runs only from a late guard: a guard on a field assigned before anything
can fail says nothing at all, since it is always true, and that is the shape a
misplaced guard has.

The fix is the guard, or assigning the field before anything in Init can fail.

Source: process.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2014a",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	inits := map[*types.Named]*ast.FuncDecl{}
	terminates := map[*types.Named]*ergomodel.Callback{}
	for _, cb := range m.Callbacks {

		if cb.Meta {
			continue
		}
		switch cb.Name {
		case "Init":
			inits[cb.Behavior] = cb.Decl
		case "Terminate":
			terminates[cb.Behavior] = cb
		}
	}

	for behavior, cb := range terminates {
		init := inits[behavior]
		if init == nil {
			continue
		}
		late := lateFields(pass, init)
		if len(late) == 0 {
			continue
		}
		guarded := guardedFields(pass, cb.Decl, late)

		for _, use := range dereferences(pass, cb.Decl) {
			if _, isLate := late[use.field]; isLate == false || guarded[use.field] {
				continue
			}
			m.Report(pass, use.pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
					ID: ergomodel.CallbackID(cb), Witness: use.field.Name(),
				},
				"Terminate dereferences %s, which Init assigns only after its first fallible step, and Terminate also runs when Init fails or times out, against exactly that half built receiver; guard the field, or assign it before anything in Init can return an error",
				use.field.Name())
		}
	}
	return nil, nil
}

func lateFields(pass *analysis.Pass, init *ast.FuncDecl) map[types.Object]token.Pos {
	if init.Body == nil {
		return nil
	}
	cut, found := firstFallible(init.Body)
	if found == false {

		return nil
	}

	early := map[types.Object]bool{}
	late := map[types.Object]token.Pos{}
	ast.Inspect(init.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if ok == false {
			return true
		}
		for _, lhs := range as.Lhs {
			sel, isSel := lhs.(*ast.SelectorExpr)
			if isSel == false {
				continue
			}
			obj := pass.TypesInfo.Uses[sel.Sel]
			if obj == nil {
				continue
			}
			if _, isField := obj.(*types.Var); isField == false {
				continue
			}
			if as.Pos() < cut {
				early[obj] = true
				continue
			}
			if prev, seen := late[obj]; seen == false || as.Pos() < prev {
				late[obj] = as.Pos()
			}
		}
		return true
	})

	for obj := range early {
		delete(late, obj)
	}
	return late
}

func firstFallible(body *ast.BlockStmt) (token.Pos, bool) {
	for i, stmt := range body.List {
		last := i == len(body.List)-1
		if returnsEarly(stmt) == false {
			continue
		}
		if last {

			if _, isReturn := stmt.(*ast.ReturnStmt); isReturn {
				continue
			}
		}
		return stmt.Pos(), true
	}
	return 0, false
}

func returnsEarly(stmt ast.Stmt) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		if found {
			return false
		}
		if _, isLit := n.(*ast.FuncLit); isLit {
			return false
		}
		if _, isReturn := n.(*ast.ReturnStmt); isReturn {
			found = true
		}
		return true
	})
	return found
}

type use struct {
	field types.Object
	pos   token.Pos
}

func dereferences(pass *analysis.Pass, decl *ast.FuncDecl) []use {
	if decl.Body == nil {
		return nil
	}
	var out []use
	note := func(e ast.Expr) {
		sel, ok := e.(*ast.SelectorExpr)
		if ok == false {
			return
		}
		obj := pass.TypesInfo.Uses[sel.Sel]
		if obj == nil || nilable(obj.Type()) == false {
			return
		}
		out = append(out, use{field: obj, pos: sel.Pos()})
	}

	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:

			note(x.X)
		case *ast.IndexExpr:
			note(x.X)
		case *ast.StarExpr:
			note(x.X)
		case *ast.UnaryExpr:
			if x.Op == token.ARROW {
				note(x.X)
			}
		case *ast.SendStmt:
			note(x.Chan)
		}
		return true
	})
	return out
}

func nilable(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Signature, *types.Chan:
		return true
	case *types.Map:

		return true
	case *types.Slice:

		return true
	default:
		_ = u
		return false
	}
}

func guardedFields(pass *analysis.Pass, decl *ast.FuncDecl,
	late map[types.Object]token.Pos) map[types.Object]bool {

	out := nilChecked(pass, decl)
	for guard := range out {
		at, isLate := late[guard]
		if isLate == false {
			continue
		}
		for field, pos := range late {
			if pos <= at {
				out[field] = true
			}
		}
	}
	return out
}

func nilChecked(pass *analysis.Pass, decl *ast.FuncDecl) map[types.Object]bool {
	out := map[types.Object]bool{}
	if decl.Body == nil {
		return out
	}
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if ok == false || (bin.Op != token.EQL && bin.Op != token.NEQ) {
			return true
		}
		for _, side := range []ast.Expr{bin.X, bin.Y} {
			sel, isSel := side.(*ast.SelectorExpr)
			if isSel == false {
				continue
			}
			if isNil(bin.X) == false && isNil(bin.Y) == false {
				continue
			}
			if obj := pass.TypesInfo.Uses[sel.Sel]; obj != nil {
				out[obj] = true
			}
		}
		return true
	})
	return out
}

func isNil(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}
