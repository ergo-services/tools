package callbudget

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2028"

const defaultRequestTimeout = 5

var Analyzer = &analysis.Analyzer{
	Name: "argusA2028",
	Doc: `A2028: a request in HandleCall with no budget left for it.

Whoever called this handler is sitting in their own request, waiting on their own
timeout. That timeout defaults to DefaultRequestTimeout, five seconds. So a handler
that answers by making a request of its own, also on the default, has an inner
budget equal to the outer one: if the inner request ever uses its full budget the
caller has already given up, and the reply is delivered to nobody. The handler can
never actually spend the time it asked for.

That is the same arithmetic A1011 does for Init against the init budget, one layer
up, and it is decidable the same way: the two numbers are both in the source.

What turns the arithmetic into an outage is that an actor handles one message at a
time. A handler that blocks for T serves at most one caller per T, so the second
caller waits behind the first and the k-th waits k*T. The concurrency in front of
it does not help and hides the problem instead: a pool of workers all calling one
named process looks parallel, funnels into a single mailbox, and every request past
the first blows its budget waiting to be looked at. The observer hit exactly this,
with twenty five HTTP workers in front of one session actor whose HandleCall made a
five second request; a browser opening eight panels at once timed out on all eight,
including panels whose own answer takes thirty microseconds.

Reported: a framework request inside a HandleCall family callback whose timeout is
not smaller than the caller's default budget. A resolvable timeout below the default
is the fixed form and is not reported, because a handler that asks for two seconds
can still answer within the caller's five.

The other fix is to stop blocking: keep the caller's PID and ref, return (nil, nil),
and answer later with SendResponse once the inner request comes back. The runtime
sends nothing for a nil result precisely so that this works, and the handler is free
for the next caller immediately.

HandleMessage is out of scope. Its sender is not waiting on anything, so the harm
there is mailbox latency rather than a reply nobody can receive, and that argument
belongs to A2024.

Source: actors.md, messages.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2028",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		if cb.Kind != ergomodel.CBHandleCall || cb.Decl.Body == nil {
			continue
		}
		for _, site := range requests(pass, m, cb.Decl) {
			m.Report(pass, site.pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
					ID: ergomodel.CallbackID(cb) + ":" + site.id, Witness: site.witness,
				},
				"%s answers by making a request of its own%s with a budget of %s, and the caller waiting for this reply has the default %ds: the inner wait can outlast the outer one, so the reply arrives after the caller gave up. An actor also handles one message at a time, so every other caller queues behind this wait. Give the inner request a timeout smaller than the caller's, or keep from and ref, return (nil, nil) and answer with SendResponse when it comes back",
				cb.Name, site.through, site.budget, defaultRequestTimeout)
		}
	}
	return nil, nil
}

type request struct {
	pos     token.Pos
	budget  string
	through string
	id      string
	witness string
}

func requests(pass *analysis.Pass, m *ergomodel.Model, decl *ast.FuncDecl) []request {
	var out []request
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

		if timeout, isRequest := m.RequestTimeout(fn, call); isRequest {
			if timeout > 0 && timeout < defaultRequestTimeout {
				return true
			}
			seen[call.Pos()] = true
			out = append(out, request{
				pos: call.Pos(), budget: budgetOf(timeout), id: ergomodel.FuncID(fn),
				witness: fn.Name(),
			})
			return true
		}

		b := m.Behavior(fn)
		if b.RoundTrip != frameworkRequest {
			return true
		}
		if b.Timeout > 0 && b.Timeout < defaultRequestTimeout {
			return true
		}
		seen[call.Pos()] = true
		out = append(out, request{
			pos: call.Pos(), budget: budgetOf(b.Timeout), id: ergomodel.FuncID(fn),
			through: ", through " + fn.Name(), witness: fn.Name(),
		})
		return true
	})
	return out
}

const frameworkRequest = "a request awaiting a reply"

func budgetOf(timeout int) string {
	if timeout == 0 {
		return "the default 5s"
	}
	return strconv.Itoa(timeout) + "s"
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
