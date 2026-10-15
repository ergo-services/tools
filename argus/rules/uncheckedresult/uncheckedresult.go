package uncheckedresult

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
	"ergo.tools/argus/rules/stategate"
)

const ruleID = "A2001a"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2001a",
	Doc: `A2001a: a discarded result that hides a certain failure.

Two shapes, and the rule is deliberately narrow: the framework's own tree discards
about 180 timer results, almost all of them correctly, so a rule that reported every
discard would be deleted by its first reader.

Reported. A discarded error from a gated call in a place where the gate makes the
failure certain, such as a cleanup in a goroutine started from Terminate, whose call
returns ErrNotAllowed every time. A call that A2001 already reports is left to A2001,
which says the same thing about the same line, so in practice this shape is what A2001
does not see. And a discarded CancelFunc from SendEvery or SendWithPriorityEvery where
the call can run more than once per process start - in a handler, in a loop or a
closure, or in a helper reached from a handler - so every run adds a ticker that nothing
can stop; or where the same tick is armed again by another periodic timer of the same
behavior, so the discarded one cannot be stopped when the other replaces it.

Not reported. A single SendEvery armed once per process start, in Init or in a helper
reached only from Init: it is the lifetime ticker, and the framework stops it when the
process dies. A discarded CancelFunc from a one shot timer: it fires once and
accumulates nothing, so keeping it is optional. A discarded event buffer from LinkEvent
or MonitorEvent, whose severity belongs to A2013 and depends on whether the producer
set a buffer at all.

Both the blanked assignment and the bare expression statement are matched, since the
bare form is legal at every arity and is the more common way to write it.`,
	URL:      "https://docs.ergo.services/tools/argus#A2001a",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

const ergoPrefix = "ergo.services/ergo"

var periodic = map[string]bool{"SendEvery": true, "SendWithPriorityEvery": true}

type tick struct {
	call   *ast.CallExpr
	method string
	owner  *types.Named

	cb     *ergomodel.Callback
	chain  []string
	repeat bool
}

type walker struct {
	pass  *analysis.Pass
	m     *ergomodel.Model
	cb    *ergomodel.Callback
	owned map[*ast.CallExpr]bool
	roots map[*types.Func]bool
	seen  map[seenKey]bool
	ticks map[token.Pos]*tick
	order *[]*tick
}

type seenKey struct {
	fn     *types.Func
	repeat bool
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	roots := map[*types.Func]bool{}
	for _, cb := range m.Callbacks {
		if fn, ok := pass.TypesInfo.Defs[cb.Decl.Name].(*types.Func); ok {
			roots[fn] = true
		}
	}
	ticks := map[token.Pos]*tick{}
	var order []*tick

	for _, cb := range m.Callbacks {
		surface := ergomodel.SurfaceOf(cb)
		certain := (cb.Kind == ergomodel.CBTerminate && cb.Meta == false) ||
			(cb.Kind == ergomodel.CBInit && cb.Meta)
		owned := stategate.Reported(pass, m, cb)

		ast.Inspect(cb.Decl.Body, func(n ast.Node) bool {
			call, discarded := discardedCall(n)
			if discarded == false || owned[call] == true {
				return true
			}
			gate, ok := m.GatedCall(call, surface)
			if ok == false || gate.Result != ergomodel.ResultError {
				return true
			}
			if certain == false || gate.Observable() == false {
				return true
			}
			if gate.ForbidTerminated == false && gate.ForbidMetaPreStart == false {
				return true
			}
			m.Report(pass, call.Pos(),
				ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
					ID: ergomodel.CallbackID(cb) + ":" + gate.Method},
				"the error from %s is discarded in %s, where the state makes the call fail every time, so this line does nothing",
				gate.Method, cb.Name)
			return true
		})

		if cb.Kind == ergomodel.CBTerminate {
			continue
		}
		once := cb.Kind == ergomodel.CBInit || (cb.Meta && cb.Kind == ergomodel.CBMetaStart)
		w := &walker{pass: pass, m: m, cb: cb, owned: owned, roots: roots,
			seen: map[seenKey]bool{}, ticks: ticks, order: &order}
		w.visit(cb.Decl, cb.Decl.Body, once == false, nil)
	}

	arms := periodicArms(pass)
	for _, t := range order {
		path := strings.Join(append(append([]string{}, t.chain...), t.method), ">")
		finding := ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
			ID: ergomodel.CallbackID(t.cb) + ":" + path}

		if t.repeat {
			m.Report(pass, t.call.Pos(), finding,
				"the CancelFunc from %s is discarded in %s%s, which can run more than once per process start, so every run adds a ticker that nothing can stop and all of them repeat until the process dies (see A2006)",
				t.method, t.cb.Name, via(t.chain))
			continue
		}
		if other := rearmed(pass, t, arms[t.owner]); other != nil {
			m.Report(pass, t.call.Pos(), finding,
				"the CancelFunc from %s is discarded, and the same tick is armed again by the %s in %s, so this ticker cannot be stopped when that one replaces it and both repeat until the process dies (see A2006)",
				t.method, other.method, other.in)
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

func (w *walker) visit(decl *ast.FuncDecl, node ast.Node, repeat bool, chain []string) {
	if node == nil {
		return
	}
	surface := ergomodel.SurfaceOf(w.cb)

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			if repeat == false {
				w.visit(decl, x.Body, true, chain)
				return false
			}
		case *ast.ForStmt, *ast.RangeStmt:
			if repeat == false {
				w.visit(decl, x, true, chain)
				return false
			}
		}

		if call, discarded := discardedCall(n); discarded && w.owned[call] == false {
			if gate, ok := w.m.GatedCall(call, surface); ok && gate.Result == ergomodel.ResultCancelPeriodic {
				w.note(call, gate.Method, repeat, chain)
			}
		}

		call, ok := n.(*ast.CallExpr)
		if ok == false {
			return true
		}
		callee := calleeOf(w.pass, call)
		if callee == nil || ownerOf(callee) == nil || ownerOf(callee) != w.cb.Behavior || w.roots[callee] == true {
			return true
		}
		key := seenKey{fn: callee, repeat: repeat}
		if w.seen[key] == true {
			return true
		}
		w.seen[key] = true
		if helper := w.m.DeclOf(callee); helper != nil && helper.Body != nil {
			w.visit(helper, helper.Body, repeat, append(append([]string{}, chain...), callee.Name()))
		}
		return true
	})
}

func (w *walker) note(call *ast.CallExpr, method string, repeat bool, chain []string) {
	t, ok := w.ticks[call.Pos()]
	if ok == false {
		t = &tick{call: call, method: method, owner: w.cb.Behavior, cb: w.cb, chain: chain, repeat: repeat}
		w.ticks[call.Pos()] = t
		*w.order = append(*w.order, t)
		return
	}
	if repeat && t.repeat == false {
		t.cb, t.chain, t.repeat = w.cb, chain, true
	}
}

type arm struct {
	call   *ast.CallExpr
	method string
	in     string
}

func periodicArms(pass *analysis.Pass) map[*types.Named][]arm {
	out := map[*types.Named][]arm{}
	for _, file := range pass.Files {
		for _, d := range file.Decls {
			decl, ok := d.(*ast.FuncDecl)
			if ok == false || decl.Recv == nil || decl.Body == nil {
				continue
			}
			fn, ok := pass.TypesInfo.Defs[decl.Name].(*types.Func)
			if ok == false || ownerOf(fn) == nil {
				continue
			}
			owner := ownerOf(fn)
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if ok == false || len(call.Args) < 2 {
					return true
				}
				callee := calleeOf(pass, call)
				if callee == nil || periodic[callee.Name()] == false || callee.Pkg() == nil ||
					strings.HasPrefix(callee.Pkg().Path(), ergoPrefix) == false {
					return true
				}
				out[owner] = append(out[owner], arm{call: call, method: callee.Name(),
					in: owner.Obj().Name() + "." + fn.Name()})
				return true
			})
		}
	}
	return out
}

func rearmed(pass *analysis.Pass, t *tick, arms []arm) *arm {
	if len(t.call.Args) < 2 {
		return nil
	}
	for i := range arms {
		if arms[i].call == t.call {
			continue
		}
		if sameTick(pass.TypesInfo, t.call.Args[1], arms[i].call.Args[1]) {
			return &arms[i]
		}
	}
	return nil
}

func sameTick(info *types.Info, a ast.Expr, b ast.Expr) bool {
	ta, tb := info.Types[a], info.Types[b]
	if ta.Type == nil || tb.Type == nil {
		return false
	}
	if ta.Value != nil && tb.Value != nil {
		return constant.Compare(ta.Value, token.EQL, tb.Value)
	}
	return types.Identical(ta.Type, tb.Type)
}

func calleeOf(pass *analysis.Pass, call *ast.CallExpr) *types.Func {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		fn, _ := pass.TypesInfo.Uses[fun.Sel].(*types.Func)
		return fn
	case *ast.Ident:
		fn, _ := pass.TypesInfo.Uses[fun].(*types.Func)
		return fn
	}
	return nil
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

func discardedCall(n ast.Node) (*ast.CallExpr, bool) {
	switch x := n.(type) {
	case *ast.ExprStmt:
		call, ok := x.X.(*ast.CallExpr)
		return call, ok

	case *ast.AssignStmt:
		if len(x.Rhs) != 1 {
			return nil, false
		}
		call, ok := x.Rhs[0].(*ast.CallExpr)
		if ok == false {
			return nil, false
		}

		for _, lhs := range x.Lhs {
			id, isIdent := lhs.(*ast.Ident)
			if isIdent == false || id.Name != "_" {
				return nil, false
			}
		}
		return call, true
	}
	return nil, false
}
