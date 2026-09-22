package ergomodel

import (
	"go/ast"
	"go/types"
	"strconv"
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
	const (
		proc = "ergo.services/ergo/gen.Process"
		meta = "ergo.services/ergo/gen.MetaProcess"
		node = "ergo.services/ergo/gen.Node"
		nreg = "ergo.services/ergo/gen.NodeRegistrar"
		conn = "ergo.services/ergo/gen.Connection"
		core = "ergo.services/ergo/gen.Core"
		ctm  = "ergo.services/ergo/gen.CoreTargetManager"
	)
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

		{Recv: meta, Method: "Send", Param: 1},
		{Recv: meta, Method: "SendWithPriority", Param: 1},
		{Recv: meta, Method: "SendAfter", Param: 1},
		{Recv: meta, Method: "SendWithPriorityAfter", Param: 1},
		{Recv: meta, Method: "SendEvery", Param: 1},
		{Recv: meta, Method: "SendWithPriorityEvery", Param: 1},
		{Recv: meta, Method: "SendResponse", Param: 2},

		{Recv: node, Method: "Send", Param: 1},
		{Recv: node, Method: "SendWithPriority", Param: 1},
		{Recv: node, Method: "SendEvent", Param: 3},
		{Recv: node, Method: "Call", Param: 1},
		{Recv: node, Method: "CallWithTimeout", Param: 1},
		{Recv: node, Method: "CallWithPriority", Param: 1},
		{Recv: node, Method: "CallImportant", Param: 1},
		{Recv: node, Method: "CallPID", Param: 1},
		{Recv: node, Method: "CallProcessID", Param: 1},
		{Recv: node, Method: "CallAlias", Param: 1},
		{Recv: nreg, Method: "SendEvent", Param: 3},

		{Recv: conn, Method: "SendPID", Param: 3},
		{Recv: conn, Method: "SendProcessID", Param: 3},
		{Recv: conn, Method: "SendAlias", Param: 3},
		{Recv: conn, Method: "SendEvent", Param: 2},
		{Recv: conn, Method: "SendResponse", Param: 3},
		{Recv: conn, Method: "CallPID", Param: 3},
		{Recv: conn, Method: "CallProcessID", Param: 3},
		{Recv: conn, Method: "CallAlias", Param: 3},

		{Recv: core, Method: "RouteSendPID", Param: 3},
		{Recv: core, Method: "RouteSendProcessID", Param: 3},
		{Recv: core, Method: "RouteSendAlias", Param: 3},
		{Recv: core, Method: "RouteSendEvent", Param: 3},
		{Recv: core, Method: "RouteSendResponse", Param: 3},
		{Recv: core, Method: "RouteCallPID", Param: 3},
		{Recv: core, Method: "RouteCallProcessID", Param: 3},
		{Recv: core, Method: "RouteCallAlias", Param: 3},

		{Recv: ctm, Method: "RouteSendPID", Param: 3},
		{Recv: ctm, Method: "RouteSendEventMessages", Param: 3},
		{Recv: ctm, Method: "RouteSendExitMessages", Param: 2},
	}
}

type senderKey struct {
	recv   string
	method string
}

type surfaceTable struct {
	surfaces []SenderSurface
	exact    map[senderKey]int
	shapes   map[senderKey]int
}

func newSurfaceTable(surfaces []SenderSurface) *surfaceTable {
	t := &surfaceTable{
		surfaces: surfaces,
		exact:    make(map[senderKey]int, len(surfaces)),
		shapes:   map[senderKey]int{},
	}
	for _, surface := range surfaces {
		if surface.Recv == "" {
			continue
		}
		t.exact[senderKey{recv: surface.Recv, method: surface.Method}] = surface.Param
	}
	return t
}

func (m *Model) param(t *surfaceTable, fn *types.Func) (int, bool) {
	if idx, ok := t.exact[senderKey{recv: recvTypePath(fn), method: fn.Name()}]; ok {
		return idx, true
	}
	sig, isSig := fn.Type().(*types.Signature)
	if isSig == false {
		return 0, false
	}

	key := senderKey{recv: strconv.Itoa(sig.Params().Len()), method: fn.Name()}
	if param, ok := t.shapes[key]; ok {
		return param, param >= 0
	}

	byArity := map[int]bool{}
	byName := map[int]bool{}
	for _, surface := range t.surfaces {
		if surface.Method != fn.Name() {
			continue
		}
		byName[surface.Param] = true
		if m.surfaceArity(surface) == sig.Params().Len() {
			byArity[surface.Param] = true
		}
	}

	param := -1
	if len(byArity) == 1 {
		param = onlyKey(byArity)
	} else if len(byName) == 1 {
		param = onlyKey(byName)
	}
	t.shapes[key] = param
	return param, param >= 0
}

func (m *Model) surfaceArity(surface SenderSurface) int {
	if surface.Recv == "" {
		return -1
	}
	iface := m.lookupInterface(surface.Recv)
	if iface == nil {
		return -1
	}
	for i := 0; i < iface.NumMethods(); i++ {
		method := iface.Method(i)
		if method.Name() != surface.Method {
			continue
		}
		if sig, ok := method.Type().(*types.Signature); ok {
			return sig.Params().Len()
		}
	}
	return -1
}

func onlyKey(set map[int]bool) int {
	for value := range set {
		return value
	}
	return -1
}

func (m *Model) senderTable() *surfaceTable {
	if m.senders == nil {
		m.senders = newSurfaceTable(m.cfg.Senders)
	}
	return m.senders
}

var errorResponders = []SenderSurface{
	{Recv: "ergo.services/ergo/gen.Process", Method: "SendResponseError", Param: 2},
	{Recv: "ergo.services/ergo/gen.Process", Method: "SendResponseErrorImportant", Param: 2},
	{Recv: "ergo.services/ergo/gen.MetaProcess", Method: "SendResponseError", Param: 2},
	{Recv: "ergo.services/ergo/gen.Connection", Method: "SendResponseError", Param: 3},
	{Recv: "ergo.services/ergo/gen.Core", Method: "RouteSendResponseError", Param: 3},
}

func (m *Model) errorResponderTable() *surfaceTable {
	if m.responders == nil {
		m.responders = newSurfaceTable(errorResponders)
	}
	return m.responders
}

func (m *Model) errorResponseArg(fn *types.Func) (int, bool) {
	if fn.Pkg() == nil || strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
		return 0, false
	}
	return m.param(m.errorResponderTable(), fn)
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
	idx, ok := m.senderParam(fn)
	if ok == false {
		return nil, "", false
	}
	if idx >= len(call.Args) {
		return nil, "", false
	}
	return call.Args[idx], fn.Name(), true
}

func (m *Model) senderParam(fn *types.Func) (int, bool) {
	t := m.senderTable()
	if idx, ok := t.exact[senderKey{recv: recvTypePath(fn), method: fn.Name()}]; ok {
		return idx, true
	}
	if m.fromSenderSurface(fn) == false {
		return 0, false
	}
	return m.param(t, fn)
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
