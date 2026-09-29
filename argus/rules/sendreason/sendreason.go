package sendreason

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2029"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2029",
	Doc: `A2029: a termination reason that is the failure of an operation on a peer.

A callback returns the error of an operation whose subject is another process, directly or
through its own helpers. The callback's error is the termination reason, so the actor's own
liveness is made to depend on whether a peer is still reachable - and a peer that is gone is
the ordinary condition in a distributed system, not a reason to stop.

The surface is every operation that asks something of another process: the send family, a
request and its timeout variants, and monitor, demonitor, link and unlink. The failure of
each one is information about that peer, and a monitor is the sharpest case, because it
fails exactly when the peer died between the spawn and the watch - the moment the watcher
exists to survive.

Both forms count, and the guarded one is the form that ships the defect:

    return a.Send(peer, result)

    if err := a.MonitorPID(pid); err != nil {
        return err
    }

An operation on this process's own name is not on this list. RegisterName fails because the
name is taken, which is a real reason to refuse to start, and a send to the actor's own PID
is about this process rather than about a peer.

The failure is rarely where it is written. An actor that answers its caller with
"return a.Send(caller, result)" dies the first time a caller went away before the answer
was ready: an ephemeral requester, a handler process, a caller restarted under a new PID.
A shared actor holding in-memory state - a registry of open gates, a pending table, a
subscription list - takes that state with it, so one absent peer costs every other peer's
work, and a Transient restart brings the process back empty with nobody told what was lost.

On a production corpus the send was a reply to an expired request whose alias the requester
had already deleted. The runtime answered ErrProcessUnknown, the callback returned it, and
an approvals process serving many tenants restarted empty; the timers it held were gone with
it and the requests waiting on them never finished.

The fix is to decide what the failure means instead of forwarding it: log it and return nil,
or handle the absence - drop the entry, monitor the peer, answer somebody else.

Source: actors.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2029",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

var peerMethods = map[string]bool{
	"Send": true, "SendPID": true, "SendProcessID": true, "SendAlias": true,
	"SendWithPriority": true, "SendImportant": true, "SendEvent": true,
	"SendResponse": true, "SendResponseError": true,
	"SendResponseImportant": true, "SendResponseErrorImportant": true,

	"MonitorPID": true, "MonitorProcessID": true, "MonitorAlias": true, "MonitorEvent": true,
	"DemonitorPID": true, "DemonitorProcessID": true, "DemonitorAlias": true,
	"DemonitorEvent": true,

	"LinkPID": true, "LinkProcessID": true, "LinkAlias": true, "LinkEvent": true,
	"UnlinkPID": true, "UnlinkProcessID": true, "UnlinkAlias": true, "UnlinkEvent": true,

	"Call": true, "CallWithTimeout": true, "CallWithPriority": true, "CallImportant": true,
	"CallPID": true, "CallProcessID": true, "CallAlias": true,
}

type finder struct {
	pass  *analysis.Pass
	m     *ergomodel.Model
	owner *types.Named
	seen  map[*types.Func]bool
}

type hit struct {
	pos    token.Pos
	method string
	chain  []string
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)
	reported := map[string]bool{}

	for _, cb := range m.Callbacks {
		if cb.Meta || cb.Behavior == nil {
			continue
		}
		f := &finder{pass: pass, m: m, owner: cb.Behavior, seen: map[*types.Func]bool{}}
		for _, h := range f.in(cb.Decl) {
			id := ergomodel.CallbackID(cb) + ":" + strings.Join(h.chain, ">") + h.method
			if reported[id] == true {
				continue
			}
			reported[id] = true

			m.Report(pass, h.pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
					ID: id, Witness: h.method,
				},
				"%s%s returns the error of %s as its termination reason, so this process stops whenever that peer is gone; decide what the absence means instead of forwarding it",
				cb.Name, via(h.chain), h.method)
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

func (f *finder) in(decl *ast.FuncDecl) []hit {
	if decl == nil || decl.Body == nil {
		return nil
	}
	out := []hit{}

	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if ok == false || len(ret.Results) == 0 {
			return true
		}
		for _, call := range f.returned(decl, ret.Results[len(ret.Results)-1], ret.Pos()) {
			callee := f.callee(call)
			if callee == nil {
				continue
			}

			if method := f.peerMethod(callee); method != "" {
				if f.toSelf(call) == false {
					out = append(out, hit{pos: call.Pos(), method: method})
				}
				continue
			}
			if ownerOf(callee) != f.owner || f.seen[callee] == true {
				continue
			}
			f.seen[callee] = true
			for _, deep := range f.in(f.m.DeclOf(callee)) {
				deep.chain = append([]string{callee.Name()}, deep.chain...)
				out = append(out, deep)
			}
		}
		return true
	})
	return out
}

func (f *finder) returned(decl *ast.FuncDecl, result ast.Expr, at token.Pos) []*ast.CallExpr {
	switch x := result.(type) {
	case *ast.CallExpr:
		return []*ast.CallExpr{x}
	case *ast.Ident:
		bound, ok := f.m.BoundBefore(decl, x.Name, at)
		if ok == false {
			return nil
		}
		if call, isCall := bound.(*ast.CallExpr); isCall {
			return []*ast.CallExpr{call}
		}
	}
	return nil
}

func (f *finder) peerMethod(fn *types.Func) string {
	if peerMethods[fn.Name()] == false || fn.Pkg() == nil {
		return ""
	}
	if strings.HasPrefix(fn.Pkg().Path(), "ergo.services/ergo") == false {
		return ""
	}
	return fn.Name() + "()"
}

func (f *finder) toSelf(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	inner, ok := call.Args[0].(*ast.CallExpr)
	if ok == false {
		return false
	}
	sel, ok := inner.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "PID"
}

func (f *finder) callee(call *ast.CallExpr) *types.Func {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		fn, _ := f.pass.TypesInfo.Uses[fun.Sel].(*types.Func)
		return fn
	case *ast.Ident:
		fn, _ := f.pass.TypesInfo.Uses[fun].(*types.Func)
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
