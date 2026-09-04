package callbackroundtrip

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2024"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2024",
	Doc: `A2024: a round trip inside an actor callback, which parks the mailbox.

An actor handles one message at a time, so the cost of a callback is not paid by
the callback: it is paid by everything queued behind it. A round trip to a database
or an HTTP peer is measured in the same units as a request timeout, so a handler
that makes one holds the whole mailbox for that long, and a burst of them turns
into a queue nobody can see from the code.

This is the half A1002 does not ship. A1002 asks whether a wait is unbounded, and
refuses to guess whether a deadline set in another frame bounds a read. This rule
asks a different and decidable question: whether the call waits for an answer from
outside this process at all. A round trip cannot be served from memory, which is
what makes the surface checkable without dataflow, and a deadline does not change
the verdict: a bounded thirty second wait is still thirty seconds of mailbox.

The surface is configured, not built in. Only the entries whose cost is the round
trip itself are shipped, net/http and database/sql, because matching I/O by
receiver type reports a Flush on an in-memory buffer. Real code reaches its
database through an interface, so a project gets value here by naming its own
repository methods under surfaces.roundtrip in argus.yml; the receiver of an
interface method resolves exactly like a concrete one.

The verdict travels through the project's own helpers, so a handler calling a
service method that eventually issues the query is reported with the chain named.

A request to another process is not reported. It is a round trip, and A2014 reports
it in Terminate for that reason, but between two actors it is the framework working
as designed and bounded by the request timeout. What this rule is about is the wait
on something outside the cluster.

A meta process is exactly the answer this rule points at: it has its own goroutine,
its own lifecycle and an alias the actor can monitor, so a run loop there is where a
blocking client belongs. A meta Start is not reported for that reason.

Source: actors.md, meta.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2024",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		if inScope(cb) == false || cb.Decl.Body == nil {
			continue
		}
		for _, site := range roundTrips(pass, m, cb.Decl) {
			m.Report(pass, site.pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
					ID: ergomodel.CallbackID(cb) + ":" + site.id, Witness: site.why,
				},
				"%s performs %s%s, and an actor handles one message at a time, so every message queued behind this one waits for the answer too; move the call into a meta process and let it send the result back as a message",
				cb.Name, site.why, site.through)
		}
	}
	return nil, nil
}

func inScope(cb *ergomodel.Callback) bool {
	if cb.Meta && cb.Kind == ergomodel.CBMetaStart {
		return false
	}
	if cb.Application {
		return false
	}
	switch cb.Kind {
	case ergomodel.CBInit, ergomodel.CBHandleMessage,
		ergomodel.CBHandleCall, ergomodel.CBHandleEvent:
		return true
	}
	return false
}

type roundTrip struct {
	pos     token.Pos
	why     string
	through string
	id      string
}

func roundTrips(pass *analysis.Pass, m *ergomodel.Model, decl *ast.FuncDecl) []roundTrip {
	var out []roundTrip
	seen := map[token.Pos]bool{}

	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if _, isGo := n.(*ast.GoStmt); isGo {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if ok == false {
			return true
		}
		fn := calleeFunc(pass, call)
		if fn == nil || seen[call.Pos()] {
			return true
		}
		why, direct := m.ExternalRoundTripSurface(fn)
		through := ""
		if direct == false {
			why = m.Behavior(fn).External
			if why == "" {
				return true
			}
			through = ", through " + fn.Name()
		}
		seen[call.Pos()] = true
		out = append(out, roundTrip{
			pos: call.Pos(), why: why, through: through, id: ergomodel.FuncID(fn),
		})
		return true
	})
	return out
}

func calleeFunc(pass *analysis.Pass, call *ast.CallExpr) *types.Func {
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		fn, _ := pass.TypesInfo.Uses[fun].(*types.Func)
		return fn
	case *ast.SelectorExpr:
		fn, _ := pass.TypesInfo.Uses[fun.Sel].(*types.Func)
		return fn
	}
	return nil
}
