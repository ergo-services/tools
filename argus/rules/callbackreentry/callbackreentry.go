package callbackreentry

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A1013"

var Analyzer = &analysis.Analyzer{
	Name: "argusA1013",
	Doc: `A1013: a callback that re-enters itself.

A cycle among the behavior's own methods, reachable from a callback: the callback calls a
helper that calls the callback's path back. Recursion that terminates on actor state is
bounded only by that state, and a helper that re-enters on "nothing happened" does not
change it, so the loop is unbounded whenever the guard is a condition the recursion cannot
clear.

The fix is not that a self send terminates where recursion does not: a guard that never
clears spins either way. What changes is what the spin costs. A Go stack overflow is a
fatal runtime error, not a panic: recover does not see it, Terminate does not run, the
supervisor is never told, and the node goes down with every other process on it. A self
send returns from the callback first, so the actor comes back to its mailbox between
rounds: an exit signal, a cancel, a monitor, node.Stop and the supervisor all still reach
it, inspect shows one hot process, and the fault is where supervision can act on it.

So this rule trades an uncontainable node kill for a contained process fault. That is the
whole of it, and it is the contract every other actor rule here rests on.

On a production corpus the guard was "no work was dispatched this round, so decide again"
against a decision function that is pure in its input: an empty batch re-entered forever.

Bounded recursion over the payload is a different shape - the depth is the data, not the
state - and belongs in a baseline entry.

Source: actors.md`,
	URL:      "https://docs.ergo.services/tools/argus#A1013",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

const (
	unseen = iota
	onStack
	done
)

type walk struct {
	pass     *analysis.Pass
	m        *ergomodel.Model
	owner    *types.Named
	cb       *ergomodel.Callback
	state    map[*types.Func]int
	reported map[string]bool
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)
	reported := map[string]bool{}

	for _, cb := range m.Callbacks {
		if cb.Meta || cb.Behavior == nil {
			continue
		}
		entry := m.EnclosingFunc(cb.Decl)
		if entry == nil {
			continue
		}
		w := &walk{
			pass: pass, m: m, owner: cb.Behavior, cb: cb,
			state: map[*types.Func]int{}, reported: reported,
		}
		w.from(entry)
	}
	return nil, nil
}

func (w *walk) from(fn *types.Func) {
	decl := w.m.DeclOf(fn)
	if decl == nil || decl.Body == nil {
		w.state[fn] = done
		return
	}
	w.state[fn] = onStack

	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.FuncLit, *ast.GoStmt:
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if ok == false {
			return true
		}
		callee := calleeFunc(w.pass, call)
		if callee == nil || ownerOf(callee) != w.owner {
			return true
		}

		switch w.state[callee] {
		case onStack:
			w.report(call, fn, callee)
		case unseen:
			w.from(callee)
		}
		return true
	})

	w.state[fn] = done
}

func (w *walk) report(call *ast.CallExpr, from, to *types.Func) {
	id := ergomodel.FuncID(from) + "->" + ergomodel.FuncID(to)
	if w.reported[id] == true {
		return
	}
	w.reported[id] = true

	w.m.Report(w.pass, call.Pos(),
		ergomodel.Finding{
			Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 1,
			ID:      id,
			Witness: ergomodel.FuncID(to),
		},
		"%s calls %s, which is already running on this callback's stack, so %s re-enters itself; a stack overflow is a fatal runtime error the supervisor never sees, so send a message to self instead of calling back",
		from.Name(), to.Name(), w.cb.Name)
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

func calleeFunc(pass *analysis.Pass, call *ast.CallExpr) *types.Func {
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
