package stategate

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2001"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2001",
	Doc: `A2001: a state gated API called from a place where its state forbids it.

Most of the process API is gated on the process state, and two callbacks run in a
state that rejects a large part of it.

Terminate, on the normal termination path, runs with the state already Terminated.
Send, SendPID, SendProcessID, SendAlias, SendWithPriority, SendExit and SendExitMeta
still work there. Everything else does not: Call, Spawn, SendEvent, SendResponse, the
timer family, link and monitor, event registration and the setters all return
ErrNotAllowed. A cleanup loop written there does nothing, and because most call sites
discard the error the code reads as if it worked.

This is phrased as a contract violation rather than a guaranteed return value on
purpose: when Init fails or times out the runtime leaves the state at Init, so the
same Terminate body succeeds on that path. The call is wrong either way, because its
effect depends on how the process died.

A meta Init runs before Start with the state still zero, and no MetaState constant
covers it. SendResponse, SendResponseError, SetSendPriority and SetCompression all
require Running and fail there, while Send, SendWithPriority and Spawn work.

One case here is not about state at all and is reported in every callback: a request to
the caller's own PID. callPID refuses it before routing and RouteCallPID has the same
guard, so the line can never succeed no matter which state the process is in. It lives
in this rule because the design's precedence section puts the PID form of a self
directed request in the A2001 family; the name, ProcessID and alias forms, which are
routed and do burn the timeout, are A1003.

Source: actors.md, meta.md, messages.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2001",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func selfPIDCall(pass *analysis.Pass, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false || len(call.Args) == 0 {
		return "", false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if ok == false {
		return "", false
	}
	switch fn.Name() {
	case "Call", "CallWithTimeout", "CallWithPriority", "CallImportant", "CallPID":
	default:
		return "", false
	}
	if fn.Pkg() == nil || strings.HasPrefix(fn.Pkg().Path(), "ergo.services/ergo/") == false {
		return "", false
	}

	inner, ok := call.Args[0].(*ast.CallExpr)
	if ok == false {
		return "", false
	}
	pidSel, ok := inner.Fun.(*ast.SelectorExpr)
	if ok == false || pidSel.Sel.Name != "PID" {
		return "", false
	}
	pidFn, ok := pass.TypesInfo.Uses[pidSel.Sel].(*types.Func)
	if ok == false || pidFn.Pkg() == nil {
		return "", false
	}
	if strings.HasPrefix(pidFn.Pkg().Path(), "ergo.services/ergo/") == false {
		return "", false
	}
	if handleOf(sel.X) == "" || handleOf(sel.X) != handleOf(pidSel.X) {
		return "", false
	}
	return handleOf(pidSel.X) + ".PID()", true
}

func handleOf(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		inner := handleOf(x.X)
		if inner == "" {
			return ""
		}
		return inner + "." + x.Sel.Name
	}
	return ""
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		surface := ergomodel.SurfaceOf(cb)

		terminating := cb.Kind == ergomodel.CBTerminate && cb.Meta == false
		metaPreStart := cb.Kind == ergomodel.CBInit && cb.Meta

		ast.Inspect(cb.Decl.Body, func(n ast.Node) bool {

			if _, ok := n.(*ast.GoStmt); ok {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if ok == false {
				return true
			}

			if target, ok := selfPIDCall(pass, call); ok {
				m.Report(pass, call.Pos(),
					ergomodel.Finding{
						Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
						ID: ergomodel.CallbackID(cb) + ":selfPID", Witness: "own PID",
					},
					"a request to %s always returns ErrNotAllowed: the runtime refuses a call to the caller's own PID before routing it, so this line can never succeed",
					target)
				return true
			}

			gate, ok := m.GatedCall(call, surface)
			if ok == false {
				return true
			}

			switch {
			case terminating == false && metaPreStart == false:

			case terminating && gate.ForbidTerminated:
				detail := "returns ErrNotAllowed"
				if gate.Observable() == false {
					detail = "is silently ignored, and it returns nothing so the failure is invisible"
				}
				m.Report(pass, call.Pos(),
					ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
						ID: ergomodel.CallbackID(cb) + ":" + gate.Method},
					"%s %s on the normal termination path, where the state is already Terminated; Send and SendExit are the ones that still work here",
					gate.Method, detail)

			case metaPreStart && gate.ForbidMetaPreStart:
				m.Report(pass, call.Pos(),
					ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
						ID: ergomodel.CallbackID(cb) + ":" + gate.Method},
					"%s requires the meta to be Running, but Init runs before Start; Send, SendWithPriority and Spawn are the ones that work here",
					gate.Method)
			}
			return true
		})
	}
	return nil, nil
}
