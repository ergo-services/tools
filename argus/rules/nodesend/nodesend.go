package nodesend

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A1012"

var Analyzer = &analysis.Analyzer{
	Name: "argusA1012",
	Doc: `A1012: routing a message through the node handle from inside an actor.

Node().Send and Node().Call work from a callback, and that is the problem. The node
routes on its own behalf: the request is made by the core, so the calling actor never
enters WaitResponse. process.waitResponse is what sets that state, and the node path
never touches it, so an actor blocked for the full request timeout reports itself as
Running to inspection, to health probes and to the observer. That is precisely the
state stuck-process detection exists to find, and this is the one way to be stuck
while looking healthy.

That argument is about the request family, and the rule tiers accordingly: a
Node().Call is tier 1 because of the state, and a Node().Send is tier 2 because it does
not block at all, so what it loses is attribution rather than visibility.

Three more things are lost with it, all of them visible in the node's own code.
Priority is fixed: callWithOptions builds MessageOptions with PriorityNormal, so an
actor running at high priority silently drops to normal for this one message. Tracing
is attributed to the core, which the node states outright by setting the span behavior
to "core", so the message appears in a trace with no relation to the actor that sent
it. And the send is not counted against this process, because the counters belong to
whoever routes.

An application behavior is excluded, and not as a concession. An application has no
process of its own: no PID, no mailbox, no state that could be misreported, and no
process API at all. gen.Node is the only handle it is given, so Node().Send is the only
way an application can send anything and there is nothing to recommend instead. The
framework's own tests are where this showed up.

Detection is not "a callback that touches a node". gen.Node has well over a hundred
methods and only eleven route a message, so a helper taking a node to read its name is
ordinary code. Two forms are reported: the routing method called directly on the
result of Node(), and a call passing a node handle at a parameter index that the
callee is known to route through, which is what the NodeSenderFact carries across
packages.

The fix is the process API: Send, Call and their variants on the actor itself, which
set the state, keep the priority, attribute the trace and count the message. Where a
helper genuinely needs to route, pass it the process rather than the node.

Source: node.md, actors.md`,
	URL:      "https://docs.ergo.services/tools/argus#A1012",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		if cb.Decl.Body == nil || cb.Application {
			continue
		}
		ast.Inspect(cb.Decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok == false {
				return true
			}
			if method, isDirect := directRoute(pass, call); isDirect {
				m.Report(pass, call.Pos(),
					ergomodel.Finding{
						Rule: ruleID, Kind: ergomodel.KindCallback, Tier: tierOf(method),
						ID: ergomodel.CallbackID(cb), Witness: "Node()." + method,
					},
					"%s routes this message with Node().%s, so the node does it on its own behalf: %s; use %s on the process itself",
					cb.Name, method, consequence(method), method)
				return true
			}
			if arg, method, ok := handedToRouter(pass, m, call); ok {
				m.Report(pass, arg,
					ergomodel.Finding{
						Rule: ruleID, Kind: ergomodel.KindCallback, Tier: tierOf(method),
						ID: ergomodel.CallbackID(cb), Witness: "handle to " + calleeName(pass, call),
					},
					"%s hands the node handle to %s, which routes through it with %s: the message is then sent by the node rather than by this process, so %s; pass the process instead and let %s send through it",
					cb.Name, calleeName(pass, call), method,
					consequence(method), calleeName(pass, call))
			}
			return true
		})
	}
	return nil, nil
}

func waits(method string) bool {
	return strings.HasPrefix(method, "Call")
}

func tierOf(method string) int {
	if waits(method) {
		return 1
	}
	return 2
}

func consequence(method string) string {
	if waits(method) {
		return "this process never enters WaitResponse and so reports itself as Running for the whole wait, which is exactly the state stuck-process detection looks for, the priority is forced to normal, the span is attributed to the core and the request is not counted against this process"
	}
	return "the priority is forced to normal, the span is attributed to the core and the message is not counted against this process"
}

func directRoute(pass *analysis.Pass, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false || ergomodel.NodeRoutingMethod(sel.Sel.Name) == false {
		return "", false
	}
	if ergomodel.IsNodeHandle(pass.TypesInfo.TypeOf(sel.X)) == false {
		return "", false
	}

	inner, isCall := sel.X.(*ast.CallExpr)
	if isCall == false {
		return "", false
	}
	accessor, isSel := inner.Fun.(*ast.SelectorExpr)
	if isSel == false || accessor.Sel.Name != "Node" {
		return "", false
	}
	return sel.Sel.Name, true
}

func handedToRouter(pass *analysis.Pass, m *ergomodel.Model, call *ast.CallExpr) (token.Pos, string, bool) {
	fn := calleeFunc(pass, call)
	if fn == nil {
		return 0, "", false
	}
	behavior := m.Behavior(fn)
	if len(behavior.NodeSender) == 0 {
		return 0, "", false
	}
	for _, idx := range behavior.NodeSender {
		if idx >= len(call.Args) {
			continue
		}
		arg := call.Args[idx]
		if ergomodel.IsNodeHandle(pass.TypesInfo.TypeOf(arg)) == false {
			continue
		}
		method := behavior.NodeMethod
		if method == "" {
			method = "a node routing method"
		}
		return arg.Pos(), method, true
	}
	return 0, "", false
}

func calleeName(pass *analysis.Pass, call *ast.CallExpr) string {
	if fn := calleeFunc(pass, call); fn != nil {
		return fn.Name()
	}
	return "a helper"
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
