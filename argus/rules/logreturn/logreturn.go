package logreturn

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2012"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2012",
	Doc: `A2012: logging a transient failure and then returning it, which kills the process.

A non-nil return from HandleMessage or HandleEvent is a termination reason. One failed
Send becomes a dead process, the supervisor restarts it, the actor re-subscribes and
regenerates the same condition, and the loop burns restart intensity until the
supervisor gives up on its whole subtree. Nothing in the code says "terminate", which is
what makes it easy to write.

The shape is also a double report, and that is the discriminator this rule uses. The
runtime already logs the same failure: process_run.go unwraps the reason and, for
anything that is not Normal or Shutdown, writes "process terminated abnormally". An
Error level log immediately before the return says the same thing one line earlier, so
the pair is evidence that the author meant to report a problem rather than to die of it.

Narrowed hard, because a bare "return err" is the documented let-it-crash and reporting
it would be wrong. All of these must hold: the log is at Error level, on the framework's
own gen.Log; the log is the statement immediately before the return, in the same
statement list; and the returned error is demonstrably transient, meaning a framework
Err* sentinel from a fixed table, or a local assigned exactly once from a framework send
or request in this same callback, or an fmt.Errorf with %w wrapping one of those. An
error of unknown provenance is left alone: there is no evidence the process could have
continued.

Do not gate on the restart strategy. The strategy switch has no Permanent case, so every
reason reaches the restart path, and a Temporary child that is not restarted is a worse
outcome rather than an exempt one.

The fix is to return nil after logging, or gen.TerminateReasonNormal to stop on purpose.
No code fix is offered: "return nil" is right in the common case, but applying it
automatically would convert a deliberate death into survival.

Source: messages.md, pool.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2012",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

var callbackNames = map[string]bool{
	"HandleMessage":      true,
	"HandleMessageName":  true,
	"HandleMessageAlias": true,
	"HandleEvent":        true,
}

var transientSentinels = map[string]bool{
	"ErrTimeout":            true,
	"ErrNoConnection":       true,
	"ErrNoRoute":            true,
	"ErrNetworkStopped":     true,
	"ErrProcessUnknown":     true,
	"ErrProcessTerminated":  true,
	"ErrProcessIncarnation": true,
	"ErrProcessMailboxFull": true,
	"ErrMetaUnknown":        true,
	"ErrMetaMailboxFull":    true,
	"ErrBusy":               true,
	"ErrDiscarded":          true,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		if callbackNames[cb.Name] == false || cb.Meta {
			continue
		}

		if singleErrorResult(pass, cb.Decl) == false {
			continue
		}

		sends := sendResultLocals(pass, m, cb.Decl.Body)
		supervisor := embedsSupervisor(cb.Behavior)

		for _, list := range statementLists(cb.Decl.Body) {
			for i := 0; i+1 < len(list); i++ {
				if isErrorLog(pass, list[i]) == false {
					continue
				}
				ret, ok := list[i+1].(*ast.ReturnStmt)
				if ok == false || len(ret.Results) != 1 {
					continue
				}
				witness, ok := transientProvenance(pass, ret.Results[0], sends)
				if ok == false {
					continue
				}

				tail := "which ends the process; the runtime then logs the same failure again as \"process terminated abnormally\" and the supervisor restarts it, burning restart intensity, because the strategy switch has no Permanent case"
				if supervisor {
					tail = "which ends this supervisor and, one child-exit hop later, every child under it"
				}
				m.Report(pass, ret.Pos(),
					ergomodel.Finding{
						Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
						ID: ergomodel.CallbackID(cb), Witness: witness,
					},
					"%s logs this failure at Error level and then returns it as a termination reason (%s), %s; return nil after logging, or gen.TerminateReasonNormal to stop on purpose",
					cb.Name, witness, tail)
			}
		}
	}
	return nil, nil
}

func statementLists(body *ast.BlockStmt) [][]ast.Stmt {
	var out [][]ast.Stmt
	var walk func(n ast.Node) bool
	walk = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BlockStmt:
			out = append(out, x.List)
		case *ast.CaseClause:
			out = append(out, x.Body)
		case *ast.CommClause:
			out = append(out, x.Body)
		}
		return true
	}
	ast.Inspect(body, walk)
	return out
}

func isErrorLog(pass *analysis.Pass, stmt ast.Stmt) bool {
	expr, ok := stmt.(*ast.ExprStmt)
	if ok == false {
		return false
	}
	call, ok := expr.X.(*ast.CallExpr)
	if ok == false {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false || sel.Sel.Name != "Error" {
		return false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if ok == false || fn.Pkg() == nil {
		return false
	}
	if fn.Pkg().Path() != "ergo.services/ergo/gen" {
		return false
	}
	return recvName(fn) == "Log"
}

func transientProvenance(pass *analysis.Pass, e ast.Expr, sends map[types.Object]string) (string, bool) {
	switch x := e.(type) {
	case *ast.Ident:
		if x.Name == "nil" {
			return "", false
		}
		obj := pass.TypesInfo.Uses[x]
		if obj == nil {
			return "", false
		}
		if method, ok := sends[obj]; ok {
			return method, true
		}
	case *ast.SelectorExpr:
		obj := pass.TypesInfo.Uses[x.Sel]
		if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "ergo.services/ergo/gen" {
			return "", false
		}

		if strings.HasPrefix(x.Sel.Name, "TerminateReason") {
			return "", false
		}
		if transientSentinels[x.Sel.Name] {
			return "gen." + x.Sel.Name, true
		}
	case *ast.CallExpr:

		if isErrorfWithWrap(pass, x) == false {
			return "", false
		}
		for _, arg := range x.Args[1:] {
			if w, ok := transientProvenance(pass, arg, sends); ok {
				return w, true
			}
		}
	}
	return "", false
}

func isErrorfWithWrap(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false || sel.Sel.Name != "Errorf" || len(call.Args) < 2 {
		return false
	}
	tv, ok := pass.TypesInfo.Types[call.Args[0]]
	if ok == false || tv.Value == nil {
		return false
	}
	return strings.Contains(tv.Value.String(), "%w")
}

func sendResultLocals(pass *analysis.Pass, m *ergomodel.Model, body *ast.BlockStmt) map[types.Object]string {
	out := map[types.Object]string{}
	count := map[types.Object]int{}

	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if ok == false || len(as.Rhs) != 1 {
			return true
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if ok == false {
			return true
		}
		method, ok := frameworkSender(pass, m, call)
		for idx, lhs := range as.Lhs {
			id, isIdent := lhs.(*ast.Ident)
			if isIdent == false || id.Name == "_" {
				continue
			}
			obj := pass.TypesInfo.Defs[id]
			if obj == nil {
				obj = pass.TypesInfo.Uses[id]
			}
			if obj == nil {
				continue
			}
			count[obj]++

			if ok && idx == len(as.Lhs)-1 {
				out[obj] = method
			}
		}
		return true
	})

	for obj, n := range count {
		if n > 1 {
			delete(out, obj)
		}
	}
	return out
}

func frameworkSender(pass *analysis.Pass, m *ergomodel.Model, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false {
		return "", false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if ok == false || fn.Pkg() == nil {
		return "", false
	}
	if strings.HasPrefix(fn.Pkg().Path(), "ergo.services/ergo/") == false {
		return "", false
	}
	for _, s := range m.Config().Senders {
		if s.Method == fn.Name() {
			return fn.Name(), true
		}
	}
	return "", false
}

func singleErrorResult(pass *analysis.Pass, decl *ast.FuncDecl) bool {
	if decl.Type.Results == nil || len(decl.Type.Results.List) != 1 {
		return false
	}
	field := decl.Type.Results.List[0]
	if len(field.Names) > 1 {
		return false
	}
	t := pass.TypesInfo.TypeOf(field.Type)
	if t == nil {
		return false
	}
	return types.Unalias(t).String() == "error"
}

func embedsSupervisor(behavior *types.Named) bool {
	if behavior == nil {
		return false
	}
	st, ok := behavior.Underlying().(*types.Struct)
	if ok == false {
		return false
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Anonymous() == false {
			continue
		}
		named, ok := f.Type().(*types.Named)
		if ok == false {
			continue
		}
		obj := named.Obj()
		if obj.Pkg() != nil && obj.Pkg().Path() == "ergo.services/ergo/act" && obj.Name() == "Supervisor" {
			return true
		}
	}
	return false
}

func recvName(fn *types.Func) string {
	sig, ok := fn.Type().(*types.Signature)
	if ok == false || sig.Recv() == nil {
		return ""
	}
	t := sig.Recv().Type()
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	if ok == false {
		return ""
	}
	return named.Obj().Name()
}
