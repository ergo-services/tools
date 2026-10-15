package sendreason

import (
	"go/ast"
	"go/constant"
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
request and its timeout variants, and monitor and demonitor. The failure of each one is
information about that peer, and a monitor is the sharpest case, because it fails exactly
when the peer died between the spawn and the watch - the moment the watcher exists to
survive.

Both forms count, and the guarded one is the form that ships the defect:

    return a.Send(peer, result)

    if err := a.MonitorPID(pid); err != nil {
        return err
    }

Not on the list, because the failure is not news about a peer:

    - an operation on this process's own name or PID. RegisterName fails because the name
      is taken, which is a real reason to refuse to start, and a send to the actor's own PID
      is about this process.
    - SendEvent. It publishes to this process's own registration: it fails only when the
      process is not running, the node is stopping, or the event or its token is not this
      process's to use. A missing subscriber is not an error.
    - an operation on a meta this process spawned. The meta belongs to the process and is
      terminated with it, so its absence is this process's own state.
    - an event obtained from the registrar. The node owns it and never unregisters it.
    - link and unlink, unless the process traps exits. Without the trap a link already ties
      this process's liveness to the peer, so returning a failed link is the link's own
      outcome. With SetTrapExit(true) the peer's exit is only a message, and the returned
      error is what makes the absence fatal, so the link family is reported there.
    - Init. Its error is a spawn refusal returned to the spawner, not a termination reason,
      and refusing to start without a dependency is legitimate.

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

Each site is reported once, under the first callback that reaches it; a helper that is
itself a callback is analysed as its own root. The finding key names the callback, the
helper chain, the operation and the target expression, so a baseline entry accepts that
operation on that target and not any other site in the same callback.

The fix is to decide what the failure means instead of forwarding it: log it and return nil,
or handle the absence - drop the entry, monitor the peer, answer somebody else.

Source: actors.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2029",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

var peerMethods = map[string]bool{
	"Send": true, "SendPID": true, "SendProcessID": true, "SendAlias": true,
	"SendWithPriority": true, "SendImportant": true,
	"SendResponse": true, "SendResponseError": true,
	"SendResponseImportant": true, "SendResponseErrorImportant": true,

	"MonitorPID": true, "MonitorProcessID": true, "MonitorAlias": true, "MonitorEvent": true,
	"DemonitorPID": true, "DemonitorProcessID": true, "DemonitorAlias": true,
	"DemonitorEvent": true,

	"Call": true, "CallWithTimeout": true, "CallWithPriority": true, "CallImportant": true,
	"CallPID": true, "CallProcessID": true, "CallAlias": true,
}

var linkMethods = map[string]bool{
	"LinkPID": true, "LinkProcessID": true, "LinkAlias": true, "LinkEvent": true,
	"UnlinkPID": true, "UnlinkProcessID": true, "UnlinkAlias": true, "UnlinkEvent": true,
}

const ergoPrefix = "ergo.services/ergo"

type finder struct {
	pass  *analysis.Pass
	m     *ergomodel.Model
	owner *types.Named
	traps bool
	roots map[*types.Func]bool
	seen  map[*types.Func]bool
}

type hit struct {
	pos    token.Pos
	method string
	target string
	chain  []string
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)
	trapping := trappingBehaviors(pass)

	var analysed []*ergomodel.Callback
	roots := map[*types.Func]bool{}
	for _, cb := range m.Callbacks {
		if cb.Meta || cb.Behavior == nil || cb.Kind == ergomodel.CBInit {
			continue
		}
		analysed = append(analysed, cb)
		if fn, ok := pass.TypesInfo.Defs[cb.Decl.Name].(*types.Func); ok {
			roots[fn] = true
		}
	}

	reported := map[token.Pos]bool{}
	for _, cb := range analysed {
		f := &finder{pass: pass, m: m, owner: cb.Behavior, traps: trapping[cb.Behavior],
			roots: roots, seen: map[*types.Func]bool{}}
		for _, h := range f.in(cb.Decl) {
			if reported[h.pos] == true {
				continue
			}
			reported[h.pos] = true

			path := append(append([]string{}, h.chain...), h.method)
			id := ergomodel.CallbackID(cb) + ":" + strings.Join(path, ">") + "@" + h.target

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
				if f.onPeer(decl, call) {
					out = append(out, hit{pos: call.Pos(), method: method, target: targetKey(call)})
				}
				continue
			}
			if ownerOf(callee) != f.owner || f.seen[callee] == true || f.roots[callee] == true {
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
		bound, ok := f.m.BoundBefore(decl, x, at)
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
	if fn.Pkg() == nil || strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
		return ""
	}
	if peerMethods[fn.Name()] == false && (linkMethods[fn.Name()] == false || f.traps == false) {
		return ""
	}
	return fn.Name() + "()"
}

func (f *finder) onPeer(decl *ast.FuncDecl, call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return true
	}
	switch target := ast.Unparen(call.Args[0]).(type) {
	case *ast.CallExpr:
		sel, ok := target.Fun.(*ast.SelectorExpr)
		return ok == false || sel.Sel.Name != "PID"
	case *ast.Ident:
		bound, ok := f.m.BoundBefore(decl, target, call.Pos())
		if ok == false {
			return true
		}
		src, ok := bound.(*ast.CallExpr)
		if ok == false {
			return true
		}
		fn := f.callee(src)
		if fn == nil || fn.Pkg() == nil || strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
			return true
		}
		switch fn.Name() {
		case "SpawnMeta":
			return f.ownHandle(decl, src) == false
		case "Event":
			return isRegistrar(ownerOf(fn)) == false
		}
	}
	return true
}

func (f *finder) ownHandle(decl *ast.FuncDecl, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false || decl.Recv == nil || len(decl.Recv.List) == 0 || len(decl.Recv.List[0].Names) == 0 {
		return false
	}
	x := sel.X
	for {
		inner, isSel := x.(*ast.SelectorExpr)
		if isSel == false {
			break
		}
		field, isVar := f.pass.TypesInfo.Uses[inner.Sel].(*types.Var)
		if isVar == false || field.Embedded() == false {
			return false
		}
		x = inner.X
	}
	id, ok := x.(*ast.Ident)
	if ok == false {
		return false
	}
	recv := f.pass.TypesInfo.Defs[decl.Recv.List[0].Names[0]]
	return recv != nil && f.pass.TypesInfo.Uses[id] == recv
}

func isRegistrar(named *types.Named) bool {
	if named == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == ergoPrefix+"/gen" && named.Obj().Name() == "Registrar"
}

func targetKey(call *ast.CallExpr) string {
	if len(call.Args) == 0 {
		return "-"
	}
	return strings.Join(strings.Fields(types.ExprString(call.Args[0])), "")
}

func trappingBehaviors(pass *analysis.Pass) map[*types.Named]bool {
	out := map[*types.Named]bool{}
	for _, file := range pass.Files {
		for _, d := range file.Decls {
			decl, ok := d.(*ast.FuncDecl)
			if ok == false || decl.Recv == nil || decl.Body == nil {
				continue
			}
			fn, ok := pass.TypesInfo.Defs[decl.Name].(*types.Func)
			if ok == false {
				continue
			}
			owner := ownerOf(fn)
			if owner == nil {
				continue
			}
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if ok == false || len(call.Args) != 1 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if ok == false || sel.Sel.Name != "SetTrapExit" {
					return true
				}
				callee, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
				if ok == false || callee.Pkg() == nil || strings.HasPrefix(callee.Pkg().Path(), ergoPrefix) == false {
					return true
				}
				tv := pass.TypesInfo.Types[call.Args[0]]
				if tv.Value != nil && tv.Value.Kind() == constant.Bool && constant.BoolVal(tv.Value) == false {
					return true
				}
				out[owner] = true
				return true
			})
		}
	}
	return out
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
