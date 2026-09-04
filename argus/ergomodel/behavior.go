package ergomodel

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"time"
)

type FuncBehavior struct {
	Blocks  bool
	Bounded bool
	Why     string

	Timeout     int
	Spawns      bool
	Unrecovered bool
	Recovers    bool
	Replies     bool
	RoundTrip   string

	External   string
	Escapes    []Escape
	Sender     []int
	NodeSender []int
	NodeMethod string

	FmtWrapped bool
}

type GoSite struct {
	Stmt      *ast.GoStmt
	In        *Callback
	Enclosing *types.Func
	Recovered bool
	Capture   string
	CaptureAt token.Pos
}

type BlockSite struct {
	Pos       token.Pos
	Why       string
	In        *Callback
	Enclosing *types.Func
}

type SpawnSite struct {
	Call   *ast.CallExpr
	Method string
	Args   []ast.Expr
	In     *Callback
}

var spawnMethods = map[string]int{
	"Spawn":                     2,
	"SpawnRegister":             3,
	"SpawnMeta":                 1,
	"RemoteSpawn":               3,
	"RemoteSpawnRegister":       4,
	"ApplicationStart":          1,
	"ApplicationStartTemporary": 1,
}

func (m *Model) buildBehavior(decls []*ast.FuncDecl, byDecl map[*ast.FuncDecl]*Callback) {
	info := m.pass.TypesInfo

	type node struct {
		decl     *ast.FuncDecl
		obj      *types.Func
		behavior FuncBehavior
		callees  []*types.Func
	}
	nodes := make([]*node, 0, len(decls))
	byObj := make(map[*types.Func]*node, len(decls))

	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		obj, _ := info.Defs[decl.Name].(*types.Func)
		if obj == nil {
			continue
		}
		n := &node{decl: decl, obj: obj}
		n.behavior, n.callees = m.localBehavior(decl, byDecl[decl], obj)
		nodes = append(nodes, n)
		byObj[obj] = n
	}

	for changed := true; changed; {
		changed = false
		for _, n := range nodes {
			for _, callee := range n.callees {
				var from FuncBehavior
				if target, ok := byObj[callee]; ok {
					from = target.behavior
				} else {
					from = m.importedBehavior(callee)
				}
				if merge(&n.behavior, from) {
					changed = true
				}
			}
		}
	}

	m.funcs = make(map[*types.Func]FuncBehavior, len(nodes))
	for _, n := range nodes {
		m.funcs[n.obj] = n.behavior
		m.publishBehavior(n.obj, n.behavior)
	}

	m.underCallback = map[*types.Func]bool{}
	edges := map[*types.Func][]*types.Func{}
	for _, n := range nodes {
		edges[n.obj] = n.callees
	}
	var mark func(fn *types.Func)
	mark = func(fn *types.Func) {
		if fn == nil || m.underCallback[fn] {
			return
		}
		m.underCallback[fn] = true
		for _, callee := range edges[fn] {
			mark(callee)
		}
	}
	for _, cb := range m.Callbacks {
		if obj, _ := info.Defs[cb.Decl.Name].(*types.Func); obj != nil {
			mark(obj)
		}
	}
}

func merge(into *FuncBehavior, from FuncBehavior) bool {
	changed := false
	if from.Blocks {

		switch {
		case into.Blocks == false:
			into.Blocks, into.Bounded, into.Why = true, from.Bounded, from.Why
			changed = true
		case into.Bounded && from.Bounded == false:
			into.Bounded, into.Why = false, from.Why
			changed = true
		}
	}
	if from.Timeout > into.Timeout {
		into.Timeout = from.Timeout
		changed = true
	}
	if from.Spawns && into.Spawns == false {
		into.Spawns, changed = true, true
	}
	if from.Unrecovered && into.Unrecovered == false {
		into.Unrecovered, changed = true, true
	}
	if from.Replies && into.Replies == false {
		into.Replies, changed = true, true
	}
	if from.RoundTrip != "" && into.RoundTrip == "" {
		into.RoundTrip, changed = from.RoundTrip, true
	}
	if from.External != "" && into.External == "" {
		into.External, changed = from.External, true
	}
	return changed
}

func (m *Model) importedBehavior(fn *types.Func) FuncBehavior {
	var out FuncBehavior
	var blocks BlocksFact
	if m.pass.ImportObjectFact(fn, &blocks) {
		out.Blocks, out.Bounded, out.Why = true, blocks.Bounded, blocks.Why
		out.Timeout = int(blocks.Timeout / time.Second)
	}
	var spawns SpawnsFact
	if m.pass.ImportObjectFact(fn, &spawns) {
		out.Spawns = true
	}
	var unrec UnrecoveredSpawnFact
	if m.pass.ImportObjectFact(fn, &unrec) {
		out.Unrecovered = true
	}
	var recovers RecoversFact
	if m.pass.ImportObjectFact(fn, &recovers) {
		out.Recovers = true
	}
	var replies RepliesFact
	if m.pass.ImportObjectFact(fn, &replies) {
		out.Replies = true
	}
	var escapes EscapesFact
	if m.pass.ImportObjectFact(fn, &escapes) {
		out.Escapes = escapes.Escapes
	}
	var roundtrip RoundTripFact
	if m.pass.ImportObjectFact(fn, &roundtrip) {
		out.RoundTrip = roundtrip.Why
	}
	var external ExternalRoundTripFact
	if m.pass.ImportObjectFact(fn, &external) {
		out.External = external.Why
	}
	var nodeSender NodeSenderFact
	if m.pass.ImportObjectFact(fn, &nodeSender) {
		out.NodeSender, out.NodeMethod = nodeSender.Params, nodeSender.Method
	}
	var sender SenderFact
	if m.pass.ImportObjectFact(fn, &sender) {
		out.Sender = sender.Params
	}
	var wrapped FmtWrappedFact
	if m.pass.ImportObjectFact(fn, &wrapped) {
		out.FmtWrapped = true
	}
	return out
}

func (m *Model) publishBehavior(fn *types.Func, b FuncBehavior) {

	if b.Blocks && m.ownedCode() {
		m.pass.ExportObjectFact(fn, &BlocksFact{Why: b.Why, Bounded: b.Bounded})
	}
	if b.Spawns {
		m.pass.ExportObjectFact(fn, &SpawnsFact{})
	}
	if b.Unrecovered {
		m.pass.ExportObjectFact(fn, &UnrecoveredSpawnFact{})
	}
	if b.Recovers {
		m.pass.ExportObjectFact(fn, &RecoversFact{})
	}
	if b.Replies {
		m.pass.ExportObjectFact(fn, &RepliesFact{})
	}
	if len(b.Escapes) > 0 {
		m.pass.ExportObjectFact(fn, &EscapesFact{Escapes: b.Escapes})
	}
	if len(b.Sender) > 0 {
		m.pass.ExportObjectFact(fn, &SenderFact{Params: b.Sender})
	}

	if b.RoundTrip != "" && m.ownedCode() {
		m.pass.ExportObjectFact(fn, &RoundTripFact{Why: b.RoundTrip})
	}
	if b.External != "" && m.ownedCode() {
		m.pass.ExportObjectFact(fn, &ExternalRoundTripFact{Why: b.External})
	}
	if len(b.NodeSender) > 0 {
		m.pass.ExportObjectFact(fn,
			&NodeSenderFact{Params: b.NodeSender, Method: b.NodeMethod})
	}
	if b.FmtWrapped {
		m.pass.ExportObjectFact(fn, &FmtWrappedFact{})
	}
}

func (m *Model) ownedCode() bool {
	return m.pkgAllowed == false && m.stdlib() == false
}

func (m *Model) localBehavior(decl *ast.FuncDecl, cb *Callback, obj *types.Func) (FuncBehavior, []*types.Func) {
	var out FuncBehavior
	var callees []*types.Func

	recv := receiverName(decl)
	selectDepth := 0
	guardedSelect := 0

	var walk func(n ast.Node) bool
	walk = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectStmt:

			selectDepth++
			if selectHasEscape(m.pass.TypesInfo, x) {
				guardedSelect++
			}
			for _, stmt := range x.Body.List {
				ast.Inspect(stmt, walk)
			}
			if selectHasEscape(m.pass.TypesInfo, x) {
				guardedSelect--
			}
			selectDepth--
			return false

		case *ast.GoStmt:
			out.Spawns = true
			site := &GoSite{
				Stmt:      x,
				In:        cb,
				Enclosing: obj,
				Recovered: m.bodyRecovers(x),
			}
			site.Capture, site.CaptureAt = receiverUse(x, recv)
			if site.Recovered == false {
				out.Unrecovered = true
			}
			m.goSites = append(m.goSites, site)

			return false

		case *ast.SendStmt:
			if guardedSelect == 0 {
				out.Blocks, out.Bounded, out.Why = true, false, "channel send"
				m.blockSites = append(m.blockSites, &BlockSite{
					Pos: x.Pos(), Why: "a channel send", In: cb, Enclosing: obj,
				})
			}

		case *ast.UnaryExpr:
			if x.Op == token.ARROW && guardedSelect == 0 {
				out.Blocks, out.Bounded, out.Why = true, false, "channel receive"
				m.blockSites = append(m.blockSites, &BlockSite{
					Pos: x.Pos(), Why: "a channel receive", In: cb, Enclosing: obj,
				})
			}

		case *ast.ReturnStmt:
			if recv != "" {
				for i, result := range x.Results {
					if field, ok := receiverDerived(result, recv); ok {
						out.Escapes = append(out.Escapes, Escape{Result: i, Field: field})
					}
				}
			}

		case *ast.CallExpr:
			fn, ok := m.calleeFunc(x)
			if ok == false {
				if id, isIdent := x.Fun.(*ast.Ident); isIdent {
					if plain, isFunc := m.pass.TypesInfo.Uses[id].(*types.Func); isFunc {
						fn, ok = plain, true
					}
				}
			}
			if ok == false {
				return true
			}
			if fn.Pkg() == m.pass.Pkg {
				callees = append(callees, fn)
			} else {
				callees = append(callees, fn)
			}
			if seconds, isRequest := m.requestTimeout(fn, x); isRequest && seconds > out.Timeout {
				out.Timeout = seconds
			}

			if method, idx, isNodeSend := m.nodeSendThroughParam(decl, x); isNodeSend {
				out.NodeSender = appendUnique(out.NodeSender, idx)
				if out.NodeMethod == "" {
					out.NodeMethod = method
				}
			}
			if why, isRoundTrip := m.roundTripSurface(fn); isRoundTrip && out.RoundTrip == "" {
				out.RoundTrip = why
			}
			if why, isExternal := m.externalRoundTrip(fn); isExternal && out.External == "" {
				out.External = why
			}
			if why, bounded, transitive, isBlocking := m.blockingSurface(fn); isBlocking {

				if bounded == false {
					m.blockSites = append(m.blockSites, &BlockSite{
						Pos: x.Pos(), Why: why, In: cb, Enclosing: obj,
					})
				}
				if transitive || bounded {
					if out.Blocks == false || (out.Bounded && bounded == false) {
						out.Blocks, out.Bounded, out.Why = true, bounded, why
					}
				}
			}
			switch fn.Name() {

			case "SendResponse", "SendResponseImportant",
				"SendResponseError", "SendResponseErrorImportant":
				if fn.Pkg() != nil && strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) {
					out.Replies = true
				}
			}

			if payload, _, isSend := m.sendPayload(x); isSend {
				if idx, ok := paramIndex(decl, payload); ok {
					out.Sender = appendUnique(out.Sender, idx)
				}
			}
		}
		return true
	}
	ast.Inspect(decl.Body, walk)

	out.Recovers = callsRecover(decl.Body, m.pass.TypesInfo)
	out.FmtWrapped = m.returnsFmtWrapped(decl)
	return out, callees
}

func (m *Model) returnsFmtWrapped(decl *ast.FuncDecl) bool {
	if decl.Body == nil || returnsError(m.pass.TypesInfo, decl) == false {
		return false
	}
	found := false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		if _, isLit := n.(*ast.FuncLit); isLit {
			return false
		}
		ret, isRet := n.(*ast.ReturnStmt)
		if isRet == false {
			return true
		}
		for _, r := range ret.Results {
			if IsFmtErrorfWrap(m.pass.TypesInfo, r) {
				found = true
			}
		}
		return true
	})
	return found
}

func returnsError(info *types.Info, decl *ast.FuncDecl) bool {
	obj, ok := info.Defs[decl.Name].(*types.Func)
	if ok == false {
		return false
	}
	sig, ok := obj.Type().(*types.Signature)
	if ok == false {
		return false
	}
	for i := 0; i < sig.Results().Len(); i++ {
		if isErrorType(sig.Results().At(i).Type()) {
			return true
		}
	}
	return false
}

func IsFmtErrorfWrap(info *types.Info, e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if ok == false || len(call.Args) < 2 {
		return false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if isSel == false || sel.Sel.Name != "Errorf" {
		return false
	}
	fn, isFunc := info.Uses[sel.Sel].(*types.Func)
	if isFunc == false || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" {
		return false
	}
	tv, hasType := info.Types[call.Args[0]]
	if hasType == false || tv.Value == nil {
		return false
	}
	return strings.Contains(tv.Value.String(), "%w")
}

func (m *Model) blockingCall(fn *types.Func) (why string, bounded bool, ok bool) {
	why, bounded, _, ok = m.blockingSurface(fn)
	return why, bounded, ok
}

func (m *Model) RequestTimeout(fn *types.Func, call *ast.CallExpr) (int, bool) {
	if fn == nil || call == nil {
		return 0, false
	}
	return m.requestTimeout(fn, call)
}

func (m *Model) requestTimeout(fn *types.Func, call *ast.CallExpr) (int, bool) {
	if fn.Pkg() == nil || strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
		return 0, false
	}
	switch fn.Name() {
	case "Call", "CallImportant", "CallWithPriority":

		return 0, true
	case "CallWithTimeout", "CallPID", "CallProcessID", "CallAlias":

		if len(call.Args) >= 3 {
			if v := m.resolveValue(call.Args[2]); v.HasInt {
				return int(v.Int), true
			}
		}
		return 0, true
	}
	return 0, false
}

func (m *Model) blockingSurface(fn *types.Func) (string, bool, bool, bool) {
	if fn.Pkg() != nil && strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) {
		switch fn.Name() {
		case "Call", "CallWithTimeout", "CallWithPriority", "CallImportant",
			"CallPID", "CallProcessID", "CallAlias":
			return "request", true, true, true
		}
	}
	recv := recvTypePath(fn)
	full := ""
	if fn.Pkg() != nil {
		full = fn.Pkg().Path() + "." + fn.Name()
	}
	for _, s := range m.cfg.Blocking {
		if s.Func != "" && s.Func == full && recv == "" {
			return s.Why, false, s.Transitive, true
		}
		if s.Method != "" && s.Method == fn.Name() && s.Recv == recv {
			return s.Why, false, s.Transitive, true
		}
	}
	return "", false, false, false
}

var nodeRoutingMethods = map[string]bool{
	"Send": true, "SendWithPriority": true, "SendEvent": true, "SendExit": true,
	"Call": true, "CallWithTimeout": true, "CallWithPriority": true,
	"CallImportant": true, "CallPID": true, "CallProcessID": true, "CallAlias": true,
}

func NodeRoutingMethod(name string) bool { return nodeRoutingMethods[name] }

func (m *Model) nodeSendThroughParam(decl *ast.FuncDecl, call *ast.CallExpr) (string, int, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false || nodeRoutingMethods[sel.Sel.Name] == false {
		return "", 0, false
	}
	if isNodeHandle(m.pass.TypesInfo.TypeOf(sel.X)) == false {
		return "", 0, false
	}
	id, isIdent := sel.X.(*ast.Ident)
	if isIdent == false {
		return "", 0, false
	}
	obj := m.pass.TypesInfo.Uses[id]
	if obj == nil || decl.Type.Params == nil {
		return "", 0, false
	}
	index := 0
	for _, field := range decl.Type.Params.List {
		for _, name := range field.Names {
			if m.pass.TypesInfo.Defs[name] == obj {
				return sel.Sel.Name, index, true
			}
			index++
		}
		if len(field.Names) == 0 {
			index++
		}
	}
	return "", 0, false
}

func isNodeHandle(t types.Type) bool {
	if t == nil {
		return false
	}
	named, ok := types.Unalias(t).(*types.Named)
	if ok == false || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == "ergo.services/ergo/gen" && named.Obj().Name() == "Node"
}

func IsNodeHandle(t types.Type) bool { return isNodeHandle(t) }

func (m *Model) ExternalRoundTripSurface(fn *types.Func) (string, bool) {
	if fn == nil {
		return "", false
	}
	return m.externalRoundTrip(fn)
}

func (m *Model) externalRoundTrip(fn *types.Func) (string, bool) {
	recv := recvTypePath(fn)
	full := ""
	if fn.Pkg() != nil {
		full = fn.Pkg().Path() + "." + fn.Name()
	}
	for _, s := range m.cfg.RoundTrips {
		if s.Func != "" && s.Func == full && recv == "" {
			return s.Why, true
		}
		if s.Method != "" && s.Method == fn.Name() && s.Recv == recv {
			return s.Why, true
		}
	}
	return "", false
}

func (m *Model) RoundTripSurface(fn *types.Func) (string, bool) {
	if fn == nil {
		return "", false
	}
	return m.roundTripSurface(fn)
}

func (m *Model) roundTripSurface(fn *types.Func) (string, bool) {
	if fn.Pkg() != nil && strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) {
		switch fn.Name() {
		case "Call", "CallWithTimeout", "CallWithPriority", "CallImportant",
			"CallPID", "CallProcessID", "CallAlias":
			return "a request awaiting a reply", true
		}
	}
	recv := recvTypePath(fn)
	full := ""
	if fn.Pkg() != nil {
		full = fn.Pkg().Path() + "." + fn.Name()
	}
	for _, s := range m.cfg.RoundTrips {
		if s.Func != "" && s.Func == full && recv == "" {
			return s.Why, true
		}
		if s.Method != "" && s.Method == fn.Name() && s.Recv == recv {
			return s.Why, true
		}
	}
	return "", false
}

func selectHasEscape(info *types.Info, sel *ast.SelectStmt) bool {
	for _, stmt := range sel.Body.List {
		clause, ok := stmt.(*ast.CommClause)
		if ok == false {
			continue
		}
		if clause.Comm == nil {
			return true
		}
		found := false
		ast.Inspect(clause.Comm, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok == false {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				switch fun.Sel.Name {
				case "After", "Tick", "Done":
					found = true
				}
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func (m *Model) bodyRecovers(g *ast.GoStmt) bool {
	if lit, ok := g.Call.Fun.(*ast.FuncLit); ok {
		return m.establishesBoundary(lit.Body)
	}
	fn, ok := m.plainCallee(g.Call)
	if ok == false {
		return false
	}
	if fn.Pkg() == m.pass.Pkg {
		if decl, ok := m.declOf[fn]; ok {
			return m.establishesBoundary(decl.Body)
		}
		return false
	}

	var f RecoversFact
	return m.pass.ImportObjectFact(fn, &f)
}

func (m *Model) establishesBoundary(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}
	found := false
	var walk func(n ast.Node) bool
	walk = func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.GoStmt:
			return false
		case *ast.DeferStmt:
			if inner, ok := x.Call.Fun.(*ast.FuncLit); ok {
				if callsRecover(inner.Body, m.pass.TypesInfo) {
					found = true
				}
				return false
			}
			if fn, ok := m.plainCallee(x.Call); ok && m.recoversDirectly(fn) {
				found = true
			}
			return false
		}
		return true
	}
	ast.Inspect(body, walk)
	return found
}

func (m *Model) plainCallee(call *ast.CallExpr) (*types.Func, bool) {
	if fn, ok := m.calleeFunc(call); ok {
		return fn, true
	}
	if id, ok := call.Fun.(*ast.Ident); ok {
		fn, ok := m.pass.TypesInfo.Uses[id].(*types.Func)
		return fn, ok
	}
	return nil, false
}

func (m *Model) recoversDirectly(fn *types.Func) bool {
	if fn.Pkg() == m.pass.Pkg {
		if decl, ok := m.declOf[fn]; ok {
			return callsRecover(decl.Body, m.pass.TypesInfo)
		}
		return false
	}
	var f RecoversFact
	return m.pass.ImportObjectFact(fn, &f)
}

func callsRecover(body *ast.BlockStmt, info *types.Info) bool {
	if body == nil {
		return false
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if ok == false {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if ok == false || id.Name != "recover" {
			return true
		}
		if builtin, ok := info.Uses[id].(*types.Builtin); ok && builtin.Name() == "recover" {
			found = true
		}
		return true
	})
	return found
}

func receiverDerived(e ast.Expr, recv string) (string, bool) {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok && id.Name == recv {
			return x.Sel.Name, true
		}
		return receiverDerived(x.X, recv)
	case *ast.SliceExpr:
		return receiverDerived(x.X, recv)
	case *ast.IndexExpr:
		return receiverDerived(x.X, recv)
	case *ast.StarExpr:
		return receiverDerived(x.X, recv)
	case *ast.UnaryExpr:
		if x.Op == token.AND {
			return receiverDerived(x.X, recv)
		}
	case *ast.ParenExpr:
		return receiverDerived(x.X, recv)
	case *ast.CallExpr:
		if isCopyCall(x) {
			return "", false
		}

		if len(x.Args) == 1 {
			if _, isSel := x.Fun.(*ast.SelectorExpr); isSel {
				return receiverDerived(x.Args[0], recv)
			}
			if _, isArray := x.Fun.(*ast.ArrayType); isArray {
				return receiverDerived(x.Args[0], recv)
			}
		}
	}
	return "", false
}

func isCopyCall(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		switch fun.Name {
		case "append":

			if len(call.Args) >= 1 {
				return isEmptyOrNil(call.Args[0])
			}
		case "string":
			return true
		}
	case *ast.SelectorExpr:
		if pkg, ok := fun.X.(*ast.Ident); ok {
			switch pkg.Name + "." + fun.Sel.Name {
			case "slices.Clone", "maps.Clone", "bytes.Clone":
				return true
			}
		}
	}
	return false
}

func isEmptyOrNil(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "nil"
	case *ast.CallExpr:

		if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "make" {
			return true
		}
		if _, ok := x.Fun.(*ast.ArrayType); ok && len(x.Args) == 1 {
			return isEmptyOrNil(x.Args[0])
		}
	case *ast.CompositeLit:
		return len(x.Elts) == 0
	}
	return false
}

func paramIndex(decl *ast.FuncDecl, e ast.Expr) (int, bool) {
	id, ok := e.(*ast.Ident)
	if ok == false || decl.Type.Params == nil {
		return 0, false
	}
	idx := 0
	for _, field := range decl.Type.Params.List {
		if len(field.Names) == 0 {
			idx++
			continue
		}
		for _, name := range field.Names {
			if name.Name == id.Name {
				return idx, true
			}
			idx++
		}
	}
	return 0, false
}

func appendUnique(list []int, v int) []int {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func receiverUse(g *ast.GoStmt, recv string) (string, token.Pos) {
	if recv == "" {
		return "", g.Pos()
	}
	found := ""
	at := g.Pos()
	ast.Inspect(g, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if ok == false {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if ok == false || id.Name != recv {
			return true
		}
		found = recv + "." + sel.Sel.Name
		at = sel.Pos()
		return false
	})
	if found != "" {
		return found, at
	}

	ast.Inspect(g, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		id, ok := n.(*ast.Ident)
		if ok && id.Name == recv {
			found = recv
			at = id.Pos()
			return false
		}
		return true
	})
	return found, at
}

func (m *Model) Behavior(fn *types.Func) FuncBehavior {
	if fn == nil {
		return FuncBehavior{}
	}
	if b, ok := m.funcs[fn]; ok {
		return b
	}
	return m.importedBehavior(fn)
}

func (m *Model) BlockSites() []*BlockSite { return m.blockSites }

func (m *Model) BlockingCall(fn *types.Func) (string, bool, bool) {
	if fn == nil {
		return "", false, false
	}
	return m.blockingCall(fn)
}

func (m *Model) GoSites() []*GoSite { return m.goSites }

func (m *Model) SpawnSites() []*SpawnSite { return m.spawnSites }

func (m *Model) UnderCallback(fn *types.Func) bool {
	return fn != nil && m.underCallback[fn]
}

func (m *Model) EnclosingFunc(decl *ast.FuncDecl) *types.Func {
	fn, _ := m.pass.TypesInfo.Defs[decl.Name].(*types.Func)
	return fn
}
