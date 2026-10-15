package ergomodel

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"
)

type ResultKind uint8

const (
	ResultUnknown ResultKind = iota
	ResultFromParam
	ResultFresh
)

type ResultOrigin struct {
	Kind      ResultKind
	Params    []ParamPath
	Recv      bool
	RecvSteps string
	Received  bool
}

type ParamPath struct {
	Index int
	Steps string
}

func (m *Model) ResultOrigin(fn *types.Func, index int) ResultOrigin {
	if fn == nil {
		return ResultOrigin{}
	}
	if fact, ok := m.results[fn]; ok {
		if index < len(fact.Results) {
			return fact.Results[index]
		}
		return ResultOrigin{}
	}
	var fact ResultsFact
	if m.pass.ImportObjectFact(fn, &fact) && index < len(fact.Results) {
		return fact.Results[index]
	}
	return ResultOrigin{}
}

func (m *Model) buildResultOrigins(decls []*ast.FuncDecl) {
	m.results = map[*types.Func]ResultsFact{}

	type entry struct {
		decl *ast.FuncDecl
		obj  *types.Func
		n    int
	}
	var entries []entry

	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		obj, _ := m.pass.TypesInfo.Defs[decl.Name].(*types.Func)
		if obj == nil {
			continue
		}
		sig, ok := obj.Type().(*types.Signature)
		if ok == false || sig.Results().Len() == 0 {
			continue
		}
		m.results[obj] = ResultsFact{Results: freshResults(sig.Results().Len())}
		entries = append(entries, entry{decl: decl, obj: obj, n: sig.Results().Len()})
	}
	closed := m.closedInterfaceMethods()
	for _, method := range closed {
		m.results[method.obj] = ResultsFact{Results: freshResults(method.n)}
	}

	for round := 0; round < 8; round++ {
		changed := false
		for _, e := range entries {
			next := m.computeResults(e.decl, e.n)
			if resultsDiffer(m.results[e.obj].Results, next) {
				m.results[e.obj] = ResultsFact{Results: next}
				changed = true
			}
		}
		for _, method := range closed {
			next := m.implementersResult(method)
			if resultsDiffer(m.results[method.obj].Results, next) {
				m.results[method.obj] = ResultsFact{Results: next}
				changed = true
			}
		}
		if changed == false {
			break
		}
	}

	for _, e := range entries {
		m.pass.ExportObjectFact(e.obj, &ResultsFact{Results: m.results[e.obj].Results})
	}
	for _, method := range closed {
		m.pass.ExportObjectFact(method.obj, &ResultsFact{Results: m.results[method.obj].Results})
	}
}

func freshResults(n int) []ResultOrigin {
	out := make([]ResultOrigin, n)
	for i := range out {
		out[i] = ResultOrigin{Kind: ResultFresh}
	}
	return out
}

func unknownResults(n int) []ResultOrigin {
	return make([]ResultOrigin, n)
}

func (m *Model) computeResults(decl *ast.FuncDecl, n int) []ResultOrigin {
	out := freshResults(n)
	named := resultNames(decl)

	ast.Inspect(decl.Body, func(node ast.Node) bool {
		if _, isLit := node.(*ast.FuncLit); isLit {
			return false
		}
		ret, isRet := node.(*ast.ReturnStmt)
		if isRet == false {
			return true
		}
		switch {
		case len(ret.Results) == n:
			for i, result := range ret.Results {
				out[i] = worstResult(out[i], m.originOfResult(decl, ret.Pos(), result, -1))
			}
		case len(ret.Results) == 1 && n > 1:
			call, isCall := ret.Results[0].(*ast.CallExpr)
			for i := range out {
				next := ResultOrigin{}
				if isCall {
					next = m.originOfResult(decl, ret.Pos(), call, i)
				}
				out[i] = worstResult(out[i], next)
			}
		case len(ret.Results) == 0 && len(named) == n:
			for i, name := range named {
				if name.Name == "_" {
					continue
				}
				out[i] = worstResult(out[i], m.originOfResult(decl, ret.Pos(), name, -1))
			}
		default:
			for i := range out {
				out[i] = worstResult(out[i], ResultOrigin{})
			}
		}
		return true
	})
	return out
}

func resultNames(decl *ast.FuncDecl) []*ast.Ident {
	if decl.Type.Results == nil {
		return nil
	}
	var out []*ast.Ident
	for _, field := range decl.Type.Results.List {
		if len(field.Names) == 0 {
			return nil
		}
		out = append(out, field.Names...)
	}
	return out
}

func (m *Model) originOfResult(decl *ast.FuncDecl, at token.Pos, e ast.Expr, index int) ResultOrigin {
	w := m.walkerFor(decl, false, at, false)
	call, isCall := e.(*ast.CallExpr)
	switch {
	case index < 0:
		w.walk(e, 0)
	case isCall:
		tuple, isTuple := m.pass.TypesInfo.TypeOf(call).(*types.Tuple)
		if isTuple && index < tuple.Len() && m.Shape(tuple.At(index).Type()).Aliasing == false {
			return ResultOrigin{Kind: ResultFresh}
		}
		w.callResult(call, index, 0)
	}

	pointer := ""
	if recv := receiverObject(m.pass.TypesInfo, decl); recv != nil {
		if _, isPointer := recv.Type().Underlying().(*types.Pointer); isPointer {
			pointer = "*"
		}
	}
	out := ResultOrigin{Kind: ResultFresh}
	for _, root := range w.roots {
		next := ResultOrigin{}
		switch {
		case root.Param >= 0 && root.Prov == ProvUnknown:
			steps := root.pending
			if root.addressed {
				steps = ""
			}
			next = ResultOrigin{Kind: ResultFromParam, Params: []ParamPath{{Index: root.Param, Steps: steps}}}
		case root.Recv:
			next = ResultOrigin{Kind: ResultFromParam, Recv: true, RecvSteps: pointer + root.held}
			if root.addressed {
				next.RecvSteps = ""
			}
		case root.Prov == ProvReceived:
			next = ResultOrigin{Kind: ResultFromParam, Received: true}
		}
		out = worstResult(out, next)
	}
	return out
}

func worstResult(a ResultOrigin, b ResultOrigin) ResultOrigin {
	switch {
	case a.Kind == ResultUnknown || b.Kind == ResultFresh:
		return a
	case b.Kind == ResultUnknown || a.Kind == ResultFresh:
		return b
	}
	out := ResultOrigin{Kind: ResultFromParam, Params: mergeParams(a.Params, b.Params),
		Recv: a.Recv || b.Recv, Received: a.Received || b.Received}
	switch {
	case a.Recv && b.Recv:
		out.RecvSteps = commonSteps(a.RecvSteps, b.RecvSteps)
	case a.Recv:
		out.RecvSteps = a.RecvSteps
	case b.Recv:
		out.RecvSteps = b.RecvSteps
	}
	return out
}

func mergeParams(a []ParamPath, b []ParamPath) []ParamPath {
	if len(b) == 0 {
		return a
	}
	byIndex := map[int]string{}
	for _, p := range a {
		byIndex[p.Index] = p.Steps
	}
	for _, p := range b {
		if steps, seen := byIndex[p.Index]; seen {
			byIndex[p.Index] = commonSteps(steps, p.Steps)
			continue
		}
		byIndex[p.Index] = p.Steps
	}
	out := make([]ParamPath, 0, len(byIndex))
	for index, steps := range byIndex {
		out = append(out, ParamPath{Index: index, Steps: steps})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

func commonSteps(a string, b string) string {
	out := ""
	for a != "" && b != "" {
		headA, restA := firstStep(a)
		headB, restB := firstStep(b)
		if headA != headB {
			break
		}
		out += headA
		a, b = restA, restB
	}
	return out
}

func resultsDiffer(a []ResultOrigin, b []ResultOrigin) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if sameResult(a[i], b[i]) == false {
			return true
		}
	}
	return false
}

func sameResult(a ResultOrigin, b ResultOrigin) bool {
	if a.Kind != b.Kind || a.Recv != b.Recv || a.RecvSteps != b.RecvSteps ||
		a.Received != b.Received || len(a.Params) != len(b.Params) {
		return false
	}
	for i := range a.Params {
		if a.Params[i] != b.Params[i] {
			return false
		}
	}
	return true
}

type closedMethod struct {
	obj   *types.Func
	iface *types.Interface
	n     int
}

func (m *Model) closedInterfaceMethods() []closedMethod {
	scope := m.pass.Pkg.Scope()
	var out []closedMethod
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if ok == false || tn.IsAlias() {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if ok == false || named.TypeParams().Len() > 0 {
			continue
		}
		iface, ok := named.Underlying().(*types.Interface)
		if ok == false || closedInterface(iface) == false {
			continue
		}
		for i := 0; i < iface.NumMethods(); i++ {
			fn := iface.Method(i)
			sig, isSig := fn.Type().(*types.Signature)
			if isSig == false || fn.Pkg() != m.pass.Pkg || sig.Results().Len() == 0 {
				continue
			}
			out = append(out, closedMethod{obj: fn, iface: iface, n: sig.Results().Len()})
		}
	}
	return out
}

func closedInterface(iface *types.Interface) bool {
	for i := 0; i < iface.NumMethods(); i++ {
		if iface.Method(i).Exported() == false {
			return true
		}
	}
	return false
}

func (m *Model) implementersResult(method closedMethod) []ResultOrigin {
	out := freshResults(method.n)
	scope := m.pass.Pkg.Scope()

	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if ok == false || tn.IsAlias() {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if ok == false || types.IsInterface(named) {
			continue
		}
		if named.TypeParams().Len() > 0 {
			for i := 0; i < named.NumMethods(); i++ {
				if named.Method(i).Name() == method.obj.Name() {
					return unknownResults(method.n)
				}
			}
			continue
		}
		ptr := types.NewPointer(named)
		if types.Implements(ptr, method.iface) == false {
			continue
		}
		sel := types.NewMethodSet(ptr).Lookup(method.obj.Pkg(), method.obj.Name())
		if sel == nil {
			continue
		}
		impl, ok := sel.Obj().(*types.Func)
		if ok == false || impl == method.obj {
			continue
		}
		if sig, isSig := impl.Type().(*types.Signature); isSig && sig.Recv() != nil && types.IsInterface(sig.Recv().Type()) {
			return unknownResults(method.n)
		}
		for i := range out {
			out[i] = worstResult(out[i], m.ResultOrigin(impl, i))
		}
	}
	return out
}
