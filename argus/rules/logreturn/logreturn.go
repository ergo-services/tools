package logreturn

import (
	"go/ast"
	"go/token"
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
or request in the same function, or an fmt.Errorf with %w wrapping one of those. An
error of unknown provenance is left alone: there is no evidence the process could have
continued. SendEvent is not a transient source: it fails only on this process's own
registration or token, or because the node is stopping, so ignoring the failure would
leave a producer that publishes nothing.

The pair is matched in the callback and in the helpers of the same behavior whose error
the callback returns, directly or through a local bound to the helper's call, the way
A2029 follows them. A helper that is itself one of these callbacks is analysed as its
own root, and a helper reached from several callbacks is reported once.

Do not gate on the restart strategy. An abnormal reason restarts a Permanent and a
Transient child alike, and a Temporary child that is not restarted is a worse outcome
rather than an exempt one, so the message states what each strategy does instead of
guessing which one the parent uses.

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

var ownStateSenders = map[string]bool{
	"SendEvent": true,
}

type site struct {
	ret     *ast.ReturnStmt
	witness string
	chain   []string
}

type walker struct {
	pass  *analysis.Pass
	m     *ergomodel.Model
	owner *types.Named
	roots map[*types.Func]bool
	seen  map[*types.Func]bool
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	var analysed []*ergomodel.Callback
	roots := map[*types.Func]bool{}
	for _, cb := range m.Callbacks {
		if callbackNames[cb.Name] == false || cb.Meta {
			continue
		}

		if singleErrorResult(pass, cb.Decl) == false {
			continue
		}
		analysed = append(analysed, cb)
		if fn, ok := pass.TypesInfo.Defs[cb.Decl.Name].(*types.Func); ok {
			roots[fn] = true
		}
	}

	reported := map[token.Pos]bool{}
	for _, cb := range analysed {
		supervisor := embedsSupervisor(cb.Behavior)
		w := &walker{pass: pass, m: m, owner: cb.Behavior, roots: roots, seen: map[*types.Func]bool{}}

		for _, s := range w.sites(cb.Decl, nil) {
			if reported[s.ret.Pos()] == true {
				continue
			}
			reported[s.ret.Pos()] = true

			tail := "which ends the process abnormally; the runtime then logs the same failure again as \"process terminated abnormally\", and under a supervisor a Permanent or Transient child is restarted for it, burning restart intensity, while a Temporary one stays down"
			if supervisor {
				tail = "which ends this supervisor and, one child-exit hop later, every child under it"
			}
			id := ergomodel.CallbackID(cb)
			if len(s.chain) > 0 {
				id += ":" + strings.Join(s.chain, ">")
			}
			m.Report(pass, s.ret.Pos(),
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
					ID: id, Witness: s.witness,
				},
				"%s%s logs this failure at Error level and then returns it as a termination reason (%s), %s; return nil after logging, or gen.TerminateReasonNormal to stop on purpose",
				cb.Name, via(s.chain), s.witness, tail)
		}
	}
	return nil, nil
}

func via(chain []string) string {
	if len(chain) == 0 {
		return ""
	}
	return " through " + strings.Join(chain, " -> ")
}

func (w *walker) sites(decl *ast.FuncDecl, chain []string) []site {
	if decl == nil || decl.Body == nil {
		return nil
	}
	var out []site

	sends := sendResultLocals(w.pass, w.m, decl.Body)
	for _, list := range statementLists(decl.Body) {
		for i := 0; i+1 < len(list); i++ {
			if isErrorLog(w.pass, list[i]) == false {
				continue
			}
			ret, ok := list[i+1].(*ast.ReturnStmt)
			if ok == false || len(ret.Results) != 1 {
				continue
			}
			witness, ok := transientProvenance(w.pass, ret.Results[0], sends)
			if ok == false {
				continue
			}
			out = append(out, site{ret: ret, witness: witness, chain: chain})
		}
	}

	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if ok == false || len(ret.Results) != 1 {
			return true
		}
		callee := w.returnedHelper(decl, ret.Results[0], ret.Pos())
		if callee == nil || w.seen[callee] == true || w.roots[callee] == true {
			return true
		}
		helper := w.m.DeclOf(callee)
		if helper == nil || singleErrorResult(w.pass, helper) == false {
			return true
		}
		w.seen[callee] = true
		deeper := append(append([]string{}, chain...), callee.Name())
		out = append(out, w.sites(helper, deeper)...)
		return true
	})
	return out
}

func (w *walker) returnedHelper(decl *ast.FuncDecl, result ast.Expr, at token.Pos) *types.Func {
	var call *ast.CallExpr
	switch x := result.(type) {
	case *ast.CallExpr:
		call = x
	case *ast.Ident:
		bound, ok := w.m.BoundBefore(decl, x, at)
		if ok == false {
			return nil
		}
		call, _ = bound.(*ast.CallExpr)
	}
	if call == nil {
		return nil
	}
	var fn *types.Func
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		fn, _ = w.pass.TypesInfo.Uses[fun.Sel].(*types.Func)
	case *ast.Ident:
		fn, _ = w.pass.TypesInfo.Uses[fun].(*types.Func)
	}
	if fn == nil || ownerOf(fn) != w.owner {
		return nil
	}
	return fn
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
	if strings.HasPrefix(fn.Pkg().Path(), "ergo.services/ergo/") == false || ownStateSenders[fn.Name()] == true {
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

func ownerOf(fn *types.Func) *types.Named {
	sig, ok := fn.Type().(*types.Signature)
	if ok == false || sig.Recv() == nil {
		return nil
	}
	t := sig.Recv().Type()
	if ptr, ok := t.(*types.Pointer); ok == true {
		t = ptr.Elem()
	}
	named, _ := t.(*types.Named)
	return named
}
