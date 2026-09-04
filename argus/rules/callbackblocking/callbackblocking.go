package callbackblocking

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A1002"

var Analyzer = &analysis.Analyzer{
	Name: "argusA1002",
	Doc: `A1002: an unbounded wait inside an actor callback.

An actor processes one message at a time. A callback that waits with no bound stops
the whole mailbox: inspection reports the process as Running, health probes see a live
node, and the sender of the next message simply waits. The rule is "no unbounded
blocking", not "no blocking".

Reported: time.Sleep, a naked Lock, RLock or WaitGroup.Wait, and a channel send or
receive that is not lexically inside a select with an escape hatch. Capacity is
deliberately ignored, because a send on a full buffered channel blocks just as hard
and the capacity is not in the channel type.

Not reported: a select with a default, a timeout arm or a context-done arm; TryLock;
and every framework Call, which is always bounded, by DefaultRequestTimeout when no
timeout is given. The bound is why the blocking fact carries one: without it an
ordinary Call in HandleMessage would be a tier 1 finding.

A meta Start is a run loop and is expected to block, so it is excluded. A meta Init
runs on the spawning actor's goroutine and is in scope.

The I/O half of this rule is not shipped: deciding whether a deadline established in
another frame bounds a read needs dataflow, and guessing it either way produces the
wrong answer. What ships is the naked construct set, as a regression guard.

Source: actors.md`,
	URL:      "https://docs.ergo.services/tools/argus#A1002",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, site := range m.BlockSites() {
		if site.In == nil {
			continue
		}
		if site.In.Kind == ergomodel.CBMetaStart && site.In.Meta {
			continue
		}
		m.Report(pass, site.Pos,
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 1,
				ID: ergomodel.CallbackID(site.In), Witness: site.Why},
			"%s in %s waits with no bound and stops the whole mailbox; use a select with a timeout or a default, or move the wait into a meta process",
			site.Why, site.In.Name)
	}

	for _, cb := range m.Callbacks {
		if cb.Kind == ergomodel.CBMetaStart && cb.Meta {
			continue
		}
		ast.Inspect(cb.Decl.Body, func(n ast.Node) bool {
			if _, ok := n.(*ast.GoStmt); ok {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if ok == false {
				return true
			}
			callee := calleeFunc(pass, call)
			if callee == nil {
				return true
			}

			if _, _, direct := m.BlockingCall(callee); direct {
				return true
			}
			b := m.Behavior(callee)
			if b.Blocks == false || b.Bounded {
				return true
			}
			m.Report(pass, call.Pos(),
				ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 1,
					ID: ergomodel.CallbackID(cb) + ":" + ergomodel.FuncID(callee), Witness: b.Why},
				"%s reaches %s, which waits with no bound (%s), and that stops the whole mailbox",
				cb.Name, callee.Name(), b.Why)
			return true
		})
	}
	return nil, nil
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
