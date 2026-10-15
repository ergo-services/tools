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
receive that is not lexically inside a select with an escape hatch. An escape hatch is
a default, a receive from a channel of time.Time, which is what time.After, a Timer
and a Ticker hand out, or a context's Done.

Capacity is ignored in general, because a send on a full buffered channel blocks just
as hard and the capacity is not in the channel type. The exception is the one the
source decides: a channel the function has just made with a constant buffer of N, sent
to at most N times before it is used in any other way, has room for each of those
sends. A send in a loop is never counted, since the loop count is not in the source.

Not reported: a select with an escape hatch; TryLock; and every framework Call, which
is always bounded, by DefaultRequestTimeout when no timeout is given. The bound is why
the blocking fact carries one: without it an ordinary Call in HandleMessage would be a
tier 1 finding.

Only code that runs while the callback runs is in scope. A go statement is another
goroutine, and so is the body of a func literal that is stored, registered as a handler
or given to time.AfterFunc: it runs later, somewhere else. A literal called in place,
deferred, bound to a local that is only ever called, or handed to a caller that runs it
before returning (sort.Slice, slices.SortFunc, sync.Map.Range, sync.Once.Do and a few
more) is part of the callback.

A wait reached through a call is reported at the call. A channel operation inside
another module does not travel that way, just as a mutex does not: whether a
library's send can block rests on invariants of the library that its source does not
state, so every caller of a library that uses channels internally would be reported.
The configured surfaces, time.Sleep and WaitGroup.Wait, travel from any module.

A meta Start is a run loop and is expected to block, so it is excluded. A meta Init
runs on the spawning actor's goroutine and is in scope. Application callbacks are
excluded: an application has no mailbox, its Init, Start and Stop run on goroutines of
their own under deadlines of their own, and Terminate runs as the application unwinds.

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
		if site.In == nil || inScope(site.In) == false {
			continue
		}
		m.Report(pass, site.Pos,
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 1,
				ID: ergomodel.CallbackID(site.In), Witness: site.Why},
			"%s in %s waits with no bound and stops the whole mailbox; use a select with a timeout or a default, or move the wait into a meta process",
			site.Why, site.In.Name)
	}

	for _, cb := range m.Callbacks {
		if inScope(cb) == false {
			continue
		}
		m.InspectExecuted(cb.Decl.Body, func(n ast.Node) bool {
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

func inScope(cb *ergomodel.Callback) bool {
	if cb.Kind == ergomodel.CBMetaStart && cb.Meta {
		return false
	}
	return cb.Application == false
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
