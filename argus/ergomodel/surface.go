package ergomodel

import (
	"go/ast"
	"go/types"
	"strings"
)

const ergoPrefix = "ergo.services/ergo/"

type CallbackKind string

const (
	CBInit          CallbackKind = "Init"
	CBHandleMessage CallbackKind = "HandleMessage"
	CBHandleCall    CallbackKind = "HandleCall"
	CBHandleEvent   CallbackKind = "HandleEvent"
	CBHandleInspect CallbackKind = "HandleInspect"
	CBHandleLog     CallbackKind = "HandleLog"
	CBTerminate     CallbackKind = "Terminate"
	CBMetaStart     CallbackKind = "Start"
)

var callbackKinds = map[string]CallbackKind{
	"Init":                 CBInit,
	"HandleMessage":        CBHandleMessage,
	"HandleMessageName":    CBHandleMessage,
	"HandleMessageAlias":   CBHandleMessage,
	"HandleCall":           CBHandleCall,
	"HandleCallName":       CBHandleCall,
	"HandleCallAlias":      CBHandleCall,
	"HandleEvent":          CBHandleEvent,
	"HandleInspect":        CBHandleInspect,
	"HandleLog":            CBHandleLog,
	"HandleSpan":           CBHandleLog,
	"RouteMessage":         CBHandleMessage,
	"RouteCall":            CBHandleCall,
	"HandleChildStart":     CBHandleMessage,
	"HandleChildTerminate": CBHandleMessage,
	"Terminate":            CBTerminate,
	"Load":                 CBInit,

	"Start": CBMetaStart,
}

func defaultCallbacks() []CallbackSurface {
	return []CallbackSurface{
		{
			Recv: "ergo.services/ergo/gen.ProcessBehavior",
			Methods: []string{
				"Init", "HandleMessage", "HandleCall", "HandleEvent", "HandleInspect",
				"HandleLog", "Terminate", "HandleMessageName", "HandleMessageAlias",
				"HandleCallName", "HandleCallAlias", "HandleSpan", "RouteMessage",
				"RouteCall", "HandleChildStart", "HandleChildTerminate",
			},
		},
		{
			Recv:    "ergo.services/ergo/gen.MetaBehavior",
			Methods: []string{"Init", "Start", "HandleMessage", "HandleCall", "HandleInspect", "Terminate"},
		},

		{
			Recv:    "ergo.services/ergo/gen.ApplicationBehavior",
			Methods: []string{"Load", "Start", "Terminate"},
		},
	}
}

func defaultSenders() []SenderSurface {
	const proc = "ergo.services/ergo/gen.Process"
	return []SenderSurface{
		{Recv: proc, Method: "Send", Param: 1},
		{Recv: proc, Method: "SendAlias", Param: 1},
		{Recv: proc, Method: "SendPID", Param: 1},
		{Recv: proc, Method: "SendProcessID", Param: 1},
		{Recv: proc, Method: "SendWithPriority", Param: 1},
		{Recv: proc, Method: "SendImportant", Param: 1},
		{Recv: proc, Method: "SendAfter", Param: 1},
		{Recv: proc, Method: "SendWithPriorityAfter", Param: 1},
		{Recv: proc, Method: "SendEvery", Param: 1},
		{Recv: proc, Method: "SendWithPriorityEvery", Param: 1},
		{Recv: proc, Method: "SendEvent", Param: 2},
		{Recv: proc, Method: "SendResponse", Param: 2},
		{Recv: proc, Method: "SendResponseImportant", Param: 2},
		{Recv: proc, Method: "Call", Param: 1},
		{Recv: proc, Method: "CallWithTimeout", Param: 1},
		{Recv: proc, Method: "CallWithPriority", Param: 1},
		{Recv: proc, Method: "CallImportant", Param: 1},
		{Recv: proc, Method: "CallPID", Param: 1},
		{Recv: proc, Method: "CallProcessID", Param: 1},
		{Recv: proc, Method: "CallAlias", Param: 1},
	}
}

func (m *Model) senderIndex() map[string]int {
	if m.senders != nil {
		return m.senders
	}
	m.senders = make(map[string]int, len(m.cfg.Senders))
	for _, s := range m.cfg.Senders {
		m.senders[s.Method] = s.Param
	}
	return m.senders
}

func (m *Model) callbackIndex() map[string]bool {
	if m.cbNames != nil {
		return m.cbNames
	}
	m.cbNames = map[string]bool{}
	for _, s := range m.cfg.Callbacks {
		for _, name := range s.Methods {
			m.cbNames[name] = true
		}
	}
	return m.cbNames
}

func (m *Model) isBehavior(t *types.Named) bool {
	ptr := types.NewPointer(t)
	for _, s := range m.cfg.Callbacks {
		iface := m.lookupInterface(s.Recv)
		if iface == nil {
			continue
		}
		if types.Implements(ptr, iface) || types.Implements(t, iface) {
			return true
		}
	}
	return embedsErgoType(t)
}

func (m *Model) lookupInterface(path string) *types.Interface {
	if iface, ok := m.ifaces[path]; ok {
		return iface
	}
	m.ifaces[path] = nil

	pkgPath, name, ok := cutLast(path, ".")
	if ok == false {
		return nil
	}
	pkg := m.findPackage(pkgPath)
	if pkg == nil {
		return nil
	}
	obj := pkg.Scope().Lookup(name)
	if obj == nil {
		return nil
	}
	iface, _ := obj.Type().Underlying().(*types.Interface)
	m.ifaces[path] = iface
	return iface
}

func (m *Model) findPackage(path string) *types.Package {
	seen := map[*types.Package]bool{}
	var walk func(p *types.Package) *types.Package
	walk = func(p *types.Package) *types.Package {
		if p == nil || seen[p] {
			return nil
		}
		seen[p] = true
		if p.Path() == path {
			return p
		}
		for _, imp := range p.Imports() {
			if found := walk(imp); found != nil {
				return found
			}
		}
		return nil
	}
	return walk(m.pass.Pkg)
}

func cutLast(s, sep string) (string, string, bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+len(sep):], true
}

func embedsErgoType(t types.Type) bool {
	named, ok := deref(t).(*types.Named)
	if ok == false {
		return false
	}
	st, ok := named.Underlying().(*types.Struct)
	if ok == false {
		return false
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Anonymous() == false {
			continue
		}
		if fromErgo(f.Type()) {
			return true
		}
		if embedsErgoType(f.Type()) {
			return true
		}
	}
	return false
}

func fromErgo(t types.Type) bool {
	named, ok := deref(t).(*types.Named)
	if ok == false {
		return false
	}
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return false
	}
	return strings.HasPrefix(pkg.Path(), ergoPrefix)
}

func deref(t types.Type) types.Type {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		return types.Unalias(p.Elem())
	}
	return t
}

func receiverType(info *types.Info, fn *ast.FuncDecl) (*types.Named, bool) {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return nil, false
	}
	t := info.TypeOf(fn.Recv.List[0].Type)
	if t == nil {
		return nil, false
	}
	named, ok := deref(t).(*types.Named)
	return named, ok
}

func receiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	names := fn.Recv.List[0].Names
	if len(names) == 0 {
		return ""
	}
	return names[0].Name
}

func (m *Model) sendPayload(call *ast.CallExpr) (ast.Expr, string, bool) {
	fn, ok := m.calleeFunc(call)
	if ok == false {
		return nil, "", false
	}
	idx, ok := m.senderIndex()[fn.Name()]
	if ok == false {
		return nil, "", false
	}
	if m.fromSenderSurface(fn) == false {
		return nil, "", false
	}
	if idx >= len(call.Args) {
		return nil, "", false
	}
	return call.Args[idx], fn.Name(), true
}

func (m *Model) fromSenderSurface(fn *types.Func) bool {
	if fn.Pkg() != nil && strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) {
		return true
	}
	recv := recvTypePath(fn)
	for _, s := range m.cfg.Senders {
		if s.Method == fn.Name() && s.Recv != "" && s.Recv == recv {
			return true
		}
	}
	return false
}

func (m *Model) calleeFunc(call *ast.CallExpr) (*types.Func, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false {
		return nil, false
	}
	fn, ok := m.pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	return fn, ok
}

func recvTypePath(fn *types.Func) string {
	sig, ok := fn.Type().(*types.Signature)
	if ok == false || sig.Recv() == nil {
		return ""
	}
	named, ok := deref(sig.Recv().Type()).(*types.Named)
	if ok == false {
		return ""
	}
	return typePath(named)
}
