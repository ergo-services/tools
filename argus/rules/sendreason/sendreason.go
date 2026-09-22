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
	Doc: `A2029: a termination reason that is a failed send.

A callback returns the error of a send to another process, directly or through its own
helpers. The callback's error is the termination reason, so the actor's own liveness is
made to depend on whether a peer is still reachable - and a peer that is gone is the
ordinary condition in a distributed system, not a reason to stop.

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
or handle the absence - drop the entry, monitor the peer, answer somebody else. A send to the
actor's own PID is exempt: that failure is about this process, not about a peer.

Source: actors.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2029",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

var sendMethods = map[string]bool{
	"Send": true, "SendPID": true, "SendProcessID": true, "SendAlias": true,
	"SendWithPriority": true, "SendImportant": true, "SendEvent": true,
	"SendResponse": true, "SendResponseError": true,
	"SendResponseImportant": true, "SendResponseErrorImportant": true,
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
		call, ok := ret.Results[len(ret.Results)-1].(*ast.CallExpr)
		if ok == false {
			return true
		}
		callee := f.callee(call)
		if callee == nil {
			return true
		}

		if method := f.sendMethod(callee); method != "" {
			if f.toSelf(call) == false {
				out = append(out, hit{pos: call.Pos(), method: method})
			}
			return true
		}
		if ownerOf(callee) != f.owner || f.seen[callee] == true {
			return true
		}
		f.seen[callee] = true
		for _, deep := range f.in(f.m.DeclOf(callee)) {
			deep.chain = append([]string{callee.Name()}, deep.chain...)
			out = append(out, deep)
		}
		return true
	})
	return out
}

func (f *finder) sendMethod(fn *types.Func) string {
	if sendMethods[fn.Name()] == false || fn.Pkg() == nil {
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
