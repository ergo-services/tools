package terminateio

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2014"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2014",
	Doc: `A2014: a round trip in Terminate, which nothing waits for.

Terminate looks like the place for a final write, and the runtime's ordering says
otherwise. On the normal path the run loop calls unregisterProcess before
ProcessTerminate, and unregisterProcess is where waitprocesses.Done happens, so
node.Wait returns while Terminate bodies are still running: a program whose main
returns after Wait abandons this call mid flight. The same ordering has already
deleted the process from the registry, released its name and aliases and sent the
down signal, so a supervisor may have restarted the child before this Terminate
finishes, and a slow write from the dying incarnation can land after the new one's.

Both outcomes are the opposite of the intent. The write was added so the failure
would be recorded, and what it produces instead is a record that sometimes exists,
sometimes does not, and sometimes overwrites a newer one.

The surface is a configured list, not a built-in one, and every entry is a call whose
cost is the round trip rather than the syscall. That distinction is the rule's
soundness argument: matching I/O by receiver type reports bufio.Writer.Flush on an
in-memory buffer, and telling an in-memory sink from a socket is dataflow. A round
trip cannot be in-memory. It is also why the surface excludes the calls that have
nowhere else to go: File.Sync is a durability barrier, os.Remove cleans up a resource
the callback itself owns, and a meta buffering to the connection it is closing has no
later hook at all, since it reaches Terminate from both the Start return and the
mailbox exit path.

The verdict propagates through the project's own helpers, so a Terminate calling a
repository method that eventually issues the query is reported with the chain named.
It does not propagate through the standard library or a dependency.

The fix is to do the write while the process is still alive, in the handler that
decided to stop, and to return gen.TerminateReasonNormal afterwards. Where the record
must survive the process, hand it to something that outlives it: a supervised writer
with its own mailbox, or the caller.

Source: process.md, messages.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2014",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		if cb.Name != "Terminate" {
			continue
		}
		for _, site := range roundTrips(pass, m, cb.Decl) {
			consequence := "node.Wait has already returned by then, because unregisterProcess calls waitprocesses.Done before Terminate runs, so the call is abandoned when main exits, and the supervisor has already been told this process is down, so a late write can land after the next incarnation's"
			if cb.Meta {
				consequence = "the meta is already unregistered by then, and it reaches Terminate from both the Start return and the mailbox exit path, so this call runs with nothing waiting for it"
			}
			m.Report(pass, site.pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
					ID: ergomodel.CallbackID(cb), Witness: site.why,
				},
				"Terminate performs %s%s; %s. Do the write in the handler that decided to stop, then return gen.TerminateReasonNormal, or hand it to something that outlives this process",
				site.why, site.through, consequence)
		}
	}
	return nil, nil
}

type roundTrip struct {
	pos     token.Pos
	why     string
	through string
}

func roundTrips(pass *analysis.Pass, m *ergomodel.Model, decl *ast.FuncDecl) []roundTrip {
	if decl.Body == nil {
		return nil
	}
	var out []roundTrip
	seen := map[token.Pos]bool{}

	ast.Inspect(decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok == false {
			return true
		}
		fn := calleeFunc(pass, call)
		if fn == nil || seen[call.Pos()] {
			return true
		}

		why, direct := m.RoundTripSurface(fn)
		through := ""
		if direct == false {
			why = m.Behavior(fn).RoundTrip
			if why == "" {
				return true
			}

			through = ", through " + fn.Name()
		}
		seen[call.Pos()] = true
		out = append(out, roundTrip{pos: call.Pos(), why: why, through: through})
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
