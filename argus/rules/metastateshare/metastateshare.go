package metastateshare

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A1005"

var Analyzer = &analysis.Analyzer{
	Name: "argusA1005",
	Doc: `A1005: meta state mutated from both of a meta's two goroutines.

A meta has two goroutines by design. Start runs the meta's own loop, and a separate
goroutine drains the mailbox into HandleMessage, HandleCall, HandleInspect and
Terminate. A field mutated from both sides is a data race, and it is invisible in
review because neither side looks concurrent on its own.

The trigger is a mutating access on each side, not reachability. Reachability alone
reports every meta that exists, including the ones written correctly and including the
idiomatic fix, which is what made the first version of this rule useless.

Init is on neither side. It runs on the spawning actor's goroutine before the start
goroutine exists, so an Init write followed by a Start read is ordered and needs no
synchronization; this is also what excuses the embedded handle, the options and
everything else set up once. Terminate counts on the mailbox side only, so a field
touched nowhere else is never reported against itself.

A mutating access is an assignment to the field or through it, an increment, taking its
address, or calling a method on it when its type is a pointer, an interface, a map, a
slice or a channel. That last clause is the one the rule exists for: the motivating
defect assigned the field on neither side. Both sides only read it and mutated what it
pointed at, so a model that looked for assignments would have found nothing.

A field whose type is safe to share is not shared state, and the model already answers
that question: the configured allowlist, which carries net.Conn, net.PacketConn,
net.Listener and sync.Pool because each of those documents its own methods as callable
from several goroutines, plus the guardedness oracle for a pointee that synchronizes
itself. This is one gate, shared with A1001, rather than a second list.

An accessor is a read. Addr, LocalAddr, RemoteAddr, String, Name and Len return
something already fixed, so calling one from the other side observes nothing that moves.

A lifecycle method is not a mutating access, and this one is load-bearing rather than a
convenience. Close, Kill, Wait, Stop, Cancel and the deadline setters exist precisely to
be called from a goroutine other than the one blocked in the loop: closing the socket is
how a meta's mailbox side unblocks a Start stuck in Read, and it is what every meta in
the framework does. Without this clause the rule reports all of them, which is the same
failure as reporting reachability, one step further in.

What remains as a mutating call is the data path: Write, Flush, Encode, Set and their
kin on a shared buffer or codec, which is the motivating defect exactly. There both
sides wrote through one gzip writer.

Four exemptions, each checkable at field granularity:

A channel field that is never reassigned after the meta starts, and whose accesses on
both sides are only sends, receives and closes. That is the single-writer handoff the
framework itself uses, and the channel is the synchronization.

An integer field whose every access on both sides goes through an atomic operation on
its address, which is how the framework writes counters rather than with atomic.Uint64.

A field whose every access on both sides is preceded, in its own function, by a Lock or
RLock on a mutex field of the same receiver. This replaces "protected by a declared
mutex", which is not a property of a field at all.

A field written only in Init or outside any callback, which follows from the trigger
rather than being tested separately.

The fix that scales is the one the framework's own metas use: make the mailbox
goroutine the only writer and have Start hand work to it, rather than adding a mutex to
a hot path on both sides.

Source: meta.md`,
	URL:      "https://docs.ergo.services/tools/argus#A1005",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

type side int

const (
	sideNone side = iota
	sideStart
	sideMailbox
)

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	type key struct {
		behavior *types.Named
		field    types.Object
	}
	collected := map[key][]access{}

	for _, cb := range m.Callbacks {
		if cb.Meta == false || cb.Recv == "" {
			continue
		}
		s := sideOf(cb)
		if s == sideNone {
			continue
		}
		for _, a := range accesses(pass, cb, s) {
			k := key{behavior: cb.Behavior, field: a.field}
			collected[k] = append(collected[k], a)
		}
	}

	reassigned := reassignedFields(pass, m)

	type report struct {
		pos     token.Pos
		field   string
		cb      *ergomodel.Callback
		other   string
		otherAt int
	}
	var out []report

	for k, list := range collected {
		startMut, mailboxMut := (*access)(nil), (*access)(nil)
		for i := range list {
			if list[i].mutating == false {
				continue
			}
			if list[i].side == sideStart && startMut == nil {
				startMut = &list[i]
			}
			if list[i].side == sideMailbox && mailboxMut == nil {
				mailboxMut = &list[i]
			}
		}
		if startMut == nil || mailboxMut == nil {
			continue
		}
		if shareable(m, k.field.Type()) {
			continue
		}
		if exempt(k.field, list, reassigned) {
			continue
		}
		out = append(out, report{
			pos: startMut.pos, field: k.field.Name(), cb: startMut.cb,
			other: mailboxMut.cb.Name, otherAt: pass.Fset.Position(mailboxMut.pos).Line,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].pos < out[j].pos })

	for _, r := range out {
		m.Report(pass, r.pos,
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindShape, Tier: 1,
				ID: ergomodel.TypeID(r.cb.Behavior) + ":" + r.field, Witness: r.field,
			},
			"Start mutates %s, and %s mutates it too at line %d: a meta runs its Start loop and its mailbox on two goroutines, so this is a data race with no synchronization on the field. Make the mailbox goroutine the only writer and have Start hand the work to it, or guard every access on both sides",
			r.field, r.other, r.otherAt)
	}
	return nil, nil
}

func sideOf(cb *ergomodel.Callback) side {
	switch cb.Kind {
	case ergomodel.CBMetaStart:
		return sideStart
	case ergomodel.CBHandleMessage, ergomodel.CBHandleCall,
		ergomodel.CBHandleInspect, ergomodel.CBTerminate:
		return sideMailbox
	}
	return sideNone
}

type access struct {
	field    types.Object
	pos      token.Pos
	side     side
	cb       *ergomodel.Callback
	mutating bool
	chanOp   bool
	atomicOp bool
	locked   bool
}

func accesses(pass *analysis.Pass, cb *ergomodel.Callback, s side) []access {
	if cb.Decl.Body == nil {
		return nil
	}
	locks := lockPositions(pass, cb.Decl)
	var out []access

	consumed := map[token.Pos]bool{}
	add := func(e ast.Expr, pos token.Pos, mutating, chanOp, atomicOp bool) {
		obj, ok := receiverField(pass, e, cb.Recv)
		if ok == false {
			return
		}

		if consumed[e.Pos()] {
			return
		}
		consumed[e.Pos()] = true
		out = append(out, access{
			field: obj, pos: pos, side: s, cb: cb, mutating: mutating,
			chanOp: chanOp, atomicOp: atomicOp, locked: lockedAt(locks, pos),
		})
	}

	ast.Inspect(cb.Decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range x.Lhs {
				add(base(lhs), lhs.Pos(), true, false, false)
			}

		case *ast.IncDecStmt:
			add(base(x.X), x.Pos(), true, false, false)

		case *ast.SendStmt:
			add(base(x.Chan), x.Pos(), true, true, false)

		case *ast.UnaryExpr:
			switch x.Op {
			case token.AND:
				add(base(x.X), x.Pos(), true, false, false)
			case token.ARROW:
				add(base(x.X), x.Pos(), true, true, false)
			}

		case *ast.CallExpr:
			classifyCall(pass, cb, x, add)

		case *ast.SelectorExpr:
			add(x, x.Pos(), false, false, false)
		}
		return true
	})
	return out
}

type adder func(e ast.Expr, pos token.Pos, mutating, chanOp, atomicOp bool)

func classifyCall(pass *analysis.Pass, cb *ergomodel.Callback, call *ast.CallExpr, add adder) {
	if isAtomicCall(pass, call) && len(call.Args) > 0 {
		if addr, ok := call.Args[0].(*ast.UnaryExpr); ok && addr.Op == token.AND {
			add(base(addr.X), call.Pos(), true, false, true)
			return
		}
	}
	if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "close" && len(call.Args) == 1 {
		add(base(call.Args[0]), call.Pos(), true, true, false)
		return
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if isSel == false {
		return
	}
	receiver := base(sel.X)
	obj, isField := receiverField(pass, receiver, cb.Recv)
	if isField == false {
		return
	}

	mutating := throughReference(obj.Type()) &&
		lockMethod(sel.Sel.Name) == false &&
		lifecycleMethod(sel.Sel.Name) == false &&
		accessorMethod(sel.Sel.Name) == false
	add(receiver, call.Pos(), mutating, false, false)
}

func exempt(field types.Object, list []access, reassigned map[types.Object]bool) bool {
	_, isChan := field.Type().Underlying().(*types.Chan)
	allChan, allAtomic, allLocked := isChan, true, true
	for _, a := range list {
		if a.chanOp == false {
			allChan = false
		}
		if a.atomicOp == false {
			allAtomic = false
		}
		if a.locked == false {
			allLocked = false
		}
	}

	if allChan && reassigned[field] == false {
		return true
	}
	if allAtomic {
		return true
	}
	return allLocked
}

func reassignedFields(pass *analysis.Pass, m *ergomodel.Model) map[types.Object]bool {
	out := map[types.Object]bool{}
	for _, cb := range m.Callbacks {
		if cb.Meta == false || cb.Recv == "" || cb.Kind == ergomodel.CBInit {
			continue
		}
		if cb.Decl.Body == nil {
			continue
		}
		ast.Inspect(cb.Decl.Body, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if ok == false {
				return true
			}
			for _, lhs := range as.Lhs {
				sel, isSel := lhs.(*ast.SelectorExpr)
				if isSel == false {
					continue
				}
				if obj, isField := receiverField(pass, sel, cb.Recv); isField {
					out[obj] = true
				}
			}
			return true
		})
	}
	return out
}

func receiverField(pass *analysis.Pass, e ast.Expr, recv string) (types.Object, bool) {
	sel, ok := e.(*ast.SelectorExpr)
	if ok == false {
		return nil, false
	}
	id, isIdent := sel.X.(*ast.Ident)
	if isIdent == false || id.Name != recv {
		return nil, false
	}
	obj := pass.TypesInfo.Uses[sel.Sel]
	v, isVar := obj.(*types.Var)
	if isVar == false || v.IsField() == false {
		return nil, false
	}
	return obj, true
}

func base(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return base(x.X)
	case *ast.IndexExpr:
		return base(x.X)
	case *ast.SliceExpr:
		return base(x.X)
	case *ast.StarExpr:
		return base(x.X)
	case *ast.SelectorExpr:

		if inner, ok := x.X.(*ast.SelectorExpr); ok {
			return base(inner)
		}
		return x
	}
	return e
}

func throughReference(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Map, *types.Slice, *types.Chan:
		return true
	}
	return false
}

func shareable(m *ergomodel.Model, t types.Type) bool {
	if _, ok := m.AllowedType(t); ok {
		return true
	}
	if p, isPtr := types.Unalias(t).(*types.Pointer); isPtr {
		if _, ok := m.AllowedType(p.Elem()); ok {
			return true
		}
	}
	return m.Guarded(t) == ergomodel.GuardGuarded
}

func accessorMethod(name string) bool {
	switch name {
	case "Addr", "LocalAddr", "RemoteAddr", "String", "Name", "Len", "Cap", "Size":
		return true
	}
	return false
}

func lifecycleMethod(name string) bool {
	switch name {
	case "Close", "CloseRead", "CloseWrite", "CloseSend",
		"Kill", "Wait", "Stop", "Shutdown", "Cancel", "Interrupt", "Signal", "Done",
		"SetDeadline", "SetReadDeadline", "SetWriteDeadline":
		return true
	}
	return false
}

func lockMethod(name string) bool {
	switch name {
	case "Lock", "Unlock", "RLock", "RUnlock":
		return true
	}
	return false
}

func lockPositions(pass *analysis.Pass, decl *ast.FuncDecl) []token.Pos {
	var out []token.Pos
	if decl.Body == nil {
		return out
	}
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok == false {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if isSel == false {
			return true
		}
		if sel.Sel.Name != "Lock" && sel.Sel.Name != "RLock" {
			return true
		}
		if isMutex(pass.TypesInfo.TypeOf(sel.X)) {
			out = append(out, call.Pos())
		}
		return true
	})
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func lockedAt(locks []token.Pos, pos token.Pos) bool {
	for _, at := range locks {
		if at < pos {
			return true
		}
	}
	return false
}

func isMutex(t types.Type) bool {
	if t == nil {
		return false
	}
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	if ok == false || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	if named.Obj().Pkg().Path() != "sync" {
		return false
	}
	return named.Obj().Name() == "Mutex" || named.Obj().Name() == "RWMutex"
}

func isAtomicCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false {
		return false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if isFunc == false || fn.Pkg() == nil {
		return false
	}
	return strings.HasPrefix(fn.Pkg().Path(), "sync/atomic")
}
