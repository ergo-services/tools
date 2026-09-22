package ergomodel

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

type Provenance int

const (
	ProvTransferred Provenance = iota
	ProvReceived
	ProvUnknown
	ProvOwned
)

func (p Provenance) String() string {
	switch p {
	case ProvTransferred:
		return "transferred"
	case ProvReceived:
		return "received"
	case ProvOwned:
		return "owned"
	}
	return "unknown"
}

type Origin struct {
	Prov    Provenance
	Witness string
	Pos     token.Pos

	Param   int
	Mutated bool
}

type ResultKind uint8

const (
	ResultUnknown ResultKind = iota
	ResultFromParam
	ResultFresh
)

type ResultOrigin struct {
	Kind  ResultKind
	Param int
}

type bind struct {
	expr  ast.Expr
	index int
}

type fieldWrite struct {
	expr ast.Expr
	pos  token.Pos
}

type flow struct {
	recv     string
	owner    string
	params   map[string]int
	binds    map[string][]bind
	fields   map[string]map[string][]fieldWrite
	ranges   map[string]ast.Expr
	writes   map[string][]ast.Expr
	opaque   map[string]bool
	retained map[string]string
}

func (m *Model) flowOf(decl *ast.FuncDecl) *flow {
	if f, ok := m.flows[decl]; ok {
		return f
	}
	f := m.buildFlow(decl)
	m.flows[decl] = f
	return f
}

func (m *Model) buildFlow(decl *ast.FuncDecl) *flow {
	f := &flow{
		recv:     receiverName(decl),
		params:   map[string]int{},
		binds:    map[string][]bind{},
		fields:   map[string]map[string][]fieldWrite{},
		ranges:   map[string]ast.Expr{},
		writes:   map[string][]ast.Expr{},
		opaque:   map[string]bool{},
		retained: map[string]string{},
	}
	if named, ok := receiverType(m.pass.TypesInfo, decl); ok {
		f.owner = typePath(named)
	}
	if decl.Type.Params != nil {
		index := 0
		for _, field := range decl.Type.Params.List {
			for _, name := range field.Names {
				f.params[name.Name] = index
				index++
			}
			if len(field.Names) == 0 {
				index++
			}
		}
	}

	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.RangeStmt:
			noteRange(f, x.Key, x.X)
			noteRange(f, x.Value, x.X)
		case *ast.TypeSwitchStmt:
			noteTypeSwitch(f, x)
		case *ast.AssignStmt:
			m.noteAssign(f, x)
		case *ast.DeclStmt:
			noteDecl(f, x)
		case *ast.CallExpr:
			m.noteBuiltinCopy(f, x)
		}
		return true
	})
	return f
}

func noteRange(f *flow, target ast.Expr, src ast.Expr) {
	id, ok := target.(*ast.Ident)
	if ok == false || id.Name == "_" {
		return
	}
	f.ranges[id.Name] = src
}

func noteTypeSwitch(f *flow, ts *ast.TypeSwitchStmt) {
	as, ok := ts.Assign.(*ast.AssignStmt)
	if ok == false || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return
	}
	id, isIdent := as.Lhs[0].(*ast.Ident)
	assert, isAssert := as.Rhs[0].(*ast.TypeAssertExpr)
	if isIdent == false || isAssert == false {
		return
	}
	f.binds[id.Name] = append(f.binds[id.Name], bind{expr: assert.X, index: -1})
}

func noteDecl(f *flow, ds *ast.DeclStmt) {
	gd, ok := ds.Decl.(*ast.GenDecl)
	if ok == false || gd.Tok != token.VAR {
		return
	}
	for _, spec := range gd.Specs {
		vs, isVar := spec.(*ast.ValueSpec)
		if isVar == false || len(vs.Values) == 0 {
			continue
		}
		if len(vs.Values) != len(vs.Names) {
			for _, name := range vs.Names {
				f.opaque[name.Name] = true
			}
			continue
		}
		for i, name := range vs.Names {
			f.binds[name.Name] = append(f.binds[name.Name], bind{expr: vs.Values[i], index: -1})
		}
	}
}

func (m *Model) noteAssign(f *flow, as *ast.AssignStmt) {
	if len(as.Lhs) > 1 && len(as.Rhs) == 1 {
		noteMultiValue(f, as)
		return
	}
	if len(as.Lhs) != len(as.Rhs) {
		return
	}

	for i, lhs := range as.Lhs {
		rhs := as.Rhs[i]

		switch target := lhs.(type) {
		case *ast.Ident:
			if target.Name == "_" {
				continue
			}
			if values, isSelfAppend := appendInto(target.Name, rhs); isSelfAppend {
				f.writes[target.Name] = append(f.writes[target.Name], values...)
				continue
			}
			f.binds[target.Name] = append(f.binds[target.Name], bind{expr: rhs, index: -1})

		case *ast.IndexExpr:
			if id, isIdent := target.X.(*ast.Ident); isIdent {
				f.writes[id.Name] = append(f.writes[id.Name], rhs)
			}
			if field, isField := receiverSelector(target.X, f.recv); isField {
				m.noteRetained(f, field, rhs, 0)
			}

		case *ast.SelectorExpr:
			if field, isField := receiverSelector(target, f.recv); isField {
				m.noteRetained(f, field, rhs, 0)
				continue
			}
			base, path, isPath := fieldPath(target)
			if isPath == false || base == f.recv {
				continue
			}
			if f.fields[base] == nil {
				f.fields[base] = map[string][]fieldWrite{}
			}
			f.fields[base][path] = append(f.fields[base][path],
				fieldWrite{expr: rhs, pos: target.Pos()})
		}
	}
}

func noteMultiValue(f *flow, as *ast.AssignStmt) {
	if len(as.Lhs) == 2 {
		switch src := as.Rhs[0].(type) {
		case *ast.TypeAssertExpr:
			bindIdent(f, as.Lhs[0], bind{expr: src.X, index: -1})
			return
		case *ast.IndexExpr:
			bindIdent(f, as.Lhs[0], bind{expr: src, index: -1})
			return
		}
	}
	if call, isCall := as.Rhs[0].(*ast.CallExpr); isCall {
		for i, lhs := range as.Lhs {
			bindIdent(f, lhs, bind{expr: call, index: i})
		}
		return
	}
	for _, lhs := range as.Lhs {
		if id, isIdent := lhs.(*ast.Ident); isIdent {
			f.opaque[id.Name] = true
		}
	}
}

func fieldPath(e ast.Expr) (string, string, bool) {
	sel, isSel := e.(*ast.SelectorExpr)
	if isSel == false {
		return "", "", false
	}
	switch x := sel.X.(type) {
	case *ast.Ident:
		return x.Name, sel.Sel.Name, true
	case *ast.SelectorExpr:
		base, path, ok := fieldPath(x)
		if ok == false {
			return "", "", false
		}
		return base, path + "." + sel.Sel.Name, true
	}
	return "", "", false
}

func (m *Model) noteBuiltinCopy(f *flow, call *ast.CallExpr) {
	id, isIdent := call.Fun.(*ast.Ident)
	if isIdent == false || id.Name != "copy" || len(call.Args) != 2 {
		return
	}
	dst, isIdent := call.Args[0].(*ast.Ident)
	if isIdent == false {
		return
	}
	if m.elemAliases(m.pass.TypesInfo.TypeOf(call.Args[1])) == false {
		return
	}
	f.writes[dst.Name] = append(f.writes[dst.Name], call.Args[1])
}

func bindIdent(f *flow, lhs ast.Expr, b bind) {
	id, ok := lhs.(*ast.Ident)
	if ok == false || id.Name == "_" {
		return
	}
	f.binds[id.Name] = append(f.binds[id.Name], b)
}

func (m *Model) noteRetained(f *flow, field string, rhs ast.Expr, depth int) {
	if rhs == nil || depth > 6 {
		return
	}
	if t := m.pass.TypesInfo.TypeOf(rhs); t != nil && m.Shape(t).Aliasing == false {
		return
	}
	switch x := rhs.(type) {
	case *ast.Ident:
		f.retained[x.Name] = field

	case *ast.ParenExpr:
		m.noteRetained(f, field, x.X, depth+1)

	case *ast.StarExpr:
		m.noteRetained(f, field, x.X, depth+1)

	case *ast.SliceExpr:
		m.noteRetained(f, field, x.X, depth+1)

	case *ast.IndexExpr:
		m.noteRetained(f, field, x.X, depth+1)

	case *ast.SelectorExpr:
		m.noteRetained(f, field, x.X, depth+1)

	case *ast.UnaryExpr:
		if x.Op == token.AND {
			m.noteRetained(f, field, x.X, depth+1)
		}

	case *ast.CompositeLit:
		for _, elt := range x.Elts {
			value := elt
			if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
				value = kv.Value
			}
			m.noteRetained(f, field, value, depth+1)
		}

	case *ast.CallExpr:
		if m.isConversionCall(x) && len(x.Args) == 1 && isStringConversion(x) == false {
			m.noteRetained(f, field, x.Args[0], depth+1)
		}
	}
}

func appendInto(name string, rhs ast.Expr) ([]ast.Expr, bool) {
	call, ok := rhs.(*ast.CallExpr)
	if ok == false {
		return nil, false
	}
	id, isIdent := call.Fun.(*ast.Ident)
	if isIdent == false || id.Name != "append" || len(call.Args) == 0 {
		return nil, false
	}
	first, isIdent := call.Args[0].(*ast.Ident)
	if isIdent == false || first.Name != name {
		return nil, false
	}
	return call.Args[1:], true
}

type provWalk struct {
	m       *Model
	f       *flow
	inbound bool
	inside  int
	seen    map[ast.Expr]bool
	roots   []Origin
}

func (m *Model) walkerFor(decl *ast.FuncDecl, inbound bool) *provWalk {
	return &provWalk{m: m, f: m.flowOf(decl), inbound: inbound, seen: map[ast.Expr]bool{}}
}

func (w *provWalk) verdict() Origin {
	out := Origin{Prov: ProvTransferred, Param: -1,
		Witness: "built here, nothing else reaches it"}
	for _, r := range w.roots {
		if r.Prov >= out.Prov {
			out = r
		}
	}
	return out
}

func (w *provWalk) add(p Provenance, witness string, pos token.Pos) {
	w.roots = append(w.roots, Origin{Prov: p, Witness: witness, Pos: pos, Param: -1})
}

func (w *provWalk) addParam(name string, index int, pos token.Pos) {
	if w.inbound {
		w.roots = append(w.roots,
			Origin{Prov: ProvReceived, Witness: "arrived in " + name, Pos: pos, Param: index})
		return
	}
	w.roots = append(w.roots,
		Origin{Prov: ProvUnknown, Witness: "parameter " + name, Pos: pos, Param: index})
}

func (w *provWalk) addOwned(field string, pos token.Pos, note string) {
	mutated := w.m.FieldMutated(w.f.owner, field, w.inside > 0)
	witness := field + note + ", only ever replaced whole"
	switch {
	case mutated && w.inside > 0:
		witness = field + note + ", written through"
	case mutated:
		witness = field + note + ", mutated in place"
	}
	w.roots = append(w.roots,
		Origin{Prov: ProvOwned, Witness: witness, Pos: pos, Param: -1, Mutated: mutated})
}

func (w *provWalk) walk(e ast.Expr, depth int) {
	if e == nil || depth > 8 || w.seen[e] {
		return
	}
	w.seen[e] = true

	t := w.m.pass.TypesInfo.TypeOf(e)
	if t != nil && w.m.Shape(t).Aliasing == false {
		return
	}

	switch x := e.(type) {
	case *ast.CompositeLit:
		for _, elt := range x.Elts {
			value := elt
			if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
				value = kv.Value
			}
			w.walk(value, depth+1)
		}

	case *ast.ParenExpr:
		w.walk(x.X, depth+1)

	case *ast.StarExpr:
		w.walk(x.X, depth+1)

	case *ast.SliceExpr:
		w.deeper(x.X, depth+1)

	case *ast.IndexExpr:
		w.deeper(x.X, depth+1)

	case *ast.TypeAssertExpr:
		w.walk(x.X, depth+1)

	case *ast.UnaryExpr:
		if x.Op == token.AND {
			w.walk(x.X, depth+1)
			return
		}
		w.add(ProvUnknown, short(t), x.Pos())

	case *ast.SelectorExpr:
		if field, isField := receiverSelector(x, w.f.recv); isField {
			w.addOwned(field, x.Pos(), "")
			return
		}
		w.walk(x.X, depth+1)

	case *ast.Ident:
		w.ident(x, depth)

	case *ast.CallExpr:
		w.callResult(x, 0, depth)

	case *ast.FuncLit:
		w.add(ProvUnknown, "closure", x.Pos())

	default:
		w.add(ProvUnknown, short(t), e.Pos())
	}
}

func (w *provWalk) walkSpread(e ast.Expr, depth int) {
	if w.m.elemAliases(w.m.pass.TypesInfo.TypeOf(e)) == false {
		return
	}
	w.deeper(e, depth+1)
}

func (w *provWalk) deeper(e ast.Expr, depth int) {
	w.inside++
	w.walk(e, depth)
	w.inside--
}

func (w *provWalk) walkBind(b bind, depth int) {
	call, isCall := b.expr.(*ast.CallExpr)
	if b.index < 0 || isCall == false {
		w.walk(b.expr, depth+1)
		return
	}
	w.callResult(call, b.index, depth+1)
}

func (w *provWalk) ident(id *ast.Ident, depth int) {
	if id.Name == "nil" || id.Name == "_" {
		return
	}
	if field, isRetained := w.f.retained[id.Name]; isRetained {
		w.addOwned(field, id.Pos(), " = "+id.Name)
		return
	}
	if index, isParam := w.f.params[id.Name]; isParam {
		w.addParam(id.Name, index, id.Pos())
		return
	}

	if w.rebuilt(id, depth) {
		return
	}

	binds := w.f.binds[id.Name]
	rng, ranged := w.f.ranges[id.Name]
	writes := w.f.writes[id.Name]

	if len(binds) == 0 && ranged == false && len(writes) == 0 {
		w.add(ProvUnknown, id.Name, id.Pos())
		return
	}
	if w.f.opaque[id.Name] {
		w.add(ProvUnknown, id.Name, id.Pos())
	}
	for _, b := range binds {
		w.walkBind(b, depth)
	}
	if ranged {
		w.deeper(rng, depth+1)
	}
	for _, value := range writes {
		w.walk(value, depth+1)
	}
}

func (w *provWalk) rebuilt(id *ast.Ident, depth int) bool {
	writes := w.f.fields[id.Name]
	if len(writes) == 0 {
		return false
	}
	st, isStruct := deref(w.m.pass.TypesInfo.TypeOf(id)).Underlying().(*types.Struct)
	if isStruct == false {
		return false
	}

	sources, ok := w.rebuiltFrom(st, "", writes, id.Pos(), 0)
	if ok == false || len(sources) == 0 {
		return false
	}
	for _, source := range sources {
		w.walk(source, depth+1)
	}
	return true
}

func (w *provWalk) rebuiltFrom(st *types.Struct, prefix string,
	writes map[string][]fieldWrite, before token.Pos, depth int) ([]ast.Expr, bool) {

	if depth > 4 {
		return nil, false
	}
	var sources []ast.Expr

	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		if w.m.Shape(field.Type()).Aliasing == false {
			continue
		}
		path := field.Name()
		if prefix != "" {
			path = prefix + "." + field.Name()
		}

		if fresh := latestBefore(writes[path], before); fresh != nil {
			sources = append(sources, fresh)
			continue
		}
		nested, isStruct := deref(field.Type()).Underlying().(*types.Struct)
		if isStruct == false {
			return nil, false
		}
		inner, ok := w.rebuiltFrom(nested, path, writes, before, depth+1)
		if ok == false {
			return nil, false
		}
		sources = append(sources, inner...)
	}
	return sources, true
}

func latestBefore(writes []fieldWrite, before token.Pos) ast.Expr {
	var out ast.Expr
	for _, write := range writes {
		if write.pos < before {
			out = write.expr
		}
	}
	return out
}

func (w *provWalk) callResult(call *ast.CallExpr, index int, depth int) {
	if w.m.isConversionCall(call) && len(call.Args) == 1 {
		if isStringConversion(call) {
			return
		}
		w.walk(call.Args[0], depth+1)
		return
	}

	if id, isIdent := call.Fun.(*ast.Ident); isIdent {
		switch id.Name {
		case "make", "new", "copy", "len", "cap":
			return
		case "append":
			from := 0
			if len(call.Args) > 0 && isEmptyOrNil(call.Args[0]) {
				from = 1
			}
			for i, arg := range call.Args[from:] {
				if call.Ellipsis.IsValid() && from+i == len(call.Args)-1 {
					w.walkSpread(arg, depth)
					continue
				}
				w.walk(arg, depth+1)
			}
			return
		}
	}

	if isCloneCall(call) && len(call.Args) == 1 {
		if w.m.elemAliases(w.m.pass.TypesInfo.TypeOf(call.Args[0])) {
			w.deeper(call.Args[0], depth+1)
		}
		return
	}

	fn, isFunc := w.m.callee(call)
	if isFunc == false {
		w.add(ProvUnknown, callName(call), call.Pos())
		return
	}

	for _, escape := range w.m.Behavior(fn).Escapes {
		if escape.Result == index {
			w.add(ProvOwned, fn.Name()+" hands back its receiver's "+escape.Field, call.Pos())
			return
		}
	}

	switch origin := w.m.ResultOrigin(fn, index); origin.Kind {
	case ResultFresh:
		return
	case ResultFromParam:
		if origin.Param >= 0 && origin.Param < len(call.Args) {
			w.walk(call.Args[origin.Param], depth+1)
			return
		}
	}
	w.add(ProvUnknown, callName(call), call.Pos())
}

func (m *Model) ResultOrigin(fn *types.Func, index int) ResultOrigin {
	if fn == nil {
		return ResultOrigin{Param: -1}
	}
	if fact, ok := m.results[fn]; ok {
		if index < len(fact.Results) {
			return fact.Results[index]
		}
		return ResultOrigin{Param: -1}
	}
	var fact ResultsFact
	if m.pass.ImportObjectFact(fn, &fact) && index < len(fact.Results) {
		return fact.Results[index]
	}
	return ResultOrigin{Param: -1}
}

func (m *Model) buildFuncAliases() {
	m.funcAliases = map[types.Object]*types.Func{}

	for _, file := range m.pass.Files {
		for _, decl := range file.Decls {
			gd, isGen := decl.(*ast.GenDecl)
			if isGen == false || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, isVar := spec.(*ast.ValueSpec)
				if isVar == false || len(vs.Names) != len(vs.Values) {
					continue
				}
				for i, name := range vs.Names {
					target := m.funcValue(vs.Values[i])
					if target == nil {
						continue
					}
					if obj := m.pass.TypesInfo.Defs[name]; obj != nil {
						m.funcAliases[obj] = target
					}
				}
			}
		}
	}
}

func (m *Model) funcValue(e ast.Expr) *types.Func {
	switch x := e.(type) {
	case *ast.Ident:
		fn, _ := m.pass.TypesInfo.Uses[x].(*types.Func)
		return fn
	case *ast.SelectorExpr:
		fn, _ := m.pass.TypesInfo.Uses[x.Sel].(*types.Func)
		return fn
	}
	return nil
}

func (m *Model) callee(call *ast.CallExpr) (*types.Func, bool) {
	if fn, ok := calleeOf(m.pass.TypesInfo, call); ok {
		return fn, true
	}
	var obj types.Object
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		obj = m.pass.TypesInfo.Uses[fun]
	case *ast.SelectorExpr:
		obj = m.pass.TypesInfo.Uses[fun.Sel]
	}
	if obj == nil {
		return nil, false
	}
	fn, ok := m.funcAliases[obj]
	return fn, ok
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
		n := sig.Results().Len()
		results := make([]ResultOrigin, n)
		for i := range results {
			results[i] = ResultOrigin{Kind: ResultFresh, Param: -1}
		}
		m.results[obj] = ResultsFact{Results: results}
		entries = append(entries, entry{decl: decl, obj: obj, n: n})
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
		if changed == false {
			break
		}
	}

	for _, e := range entries {
		m.pass.ExportObjectFact(e.obj, &ResultsFact{Results: m.results[e.obj].Results})
	}
}

func (m *Model) computeResults(decl *ast.FuncDecl, n int) []ResultOrigin {
	out := make([]ResultOrigin, n)
	for i := range out {
		out[i] = ResultOrigin{Kind: ResultFresh, Param: -1}
	}

	ast.Inspect(decl.Body, func(node ast.Node) bool {
		if _, isLit := node.(*ast.FuncLit); isLit {
			return false
		}
		ret, isRet := node.(*ast.ReturnStmt)
		if isRet == false {
			return true
		}
		if len(ret.Results) != n {
			for i := range out {
				out[i] = worstResult(out[i], ResultOrigin{Kind: ResultUnknown, Param: -1})
			}
			return true
		}
		for i, result := range ret.Results {
			out[i] = worstResult(out[i], m.originOfResult(decl, result))
		}
		return true
	})
	return out
}

func (m *Model) originOfResult(decl *ast.FuncDecl, e ast.Expr) ResultOrigin {
	w := m.walkerFor(decl, false)
	w.walk(e, 0)

	out := ResultOrigin{Kind: ResultFresh, Param: -1}
	for _, root := range w.roots {
		next := ResultOrigin{Kind: ResultUnknown, Param: -1}
		if root.Param >= 0 {
			next = ResultOrigin{Kind: ResultFromParam, Param: root.Param}
		}
		out = worstResult(out, next)
	}
	return out
}

func worstResult(a ResultOrigin, b ResultOrigin) ResultOrigin {
	if a.Kind == ResultFromParam && b.Kind == ResultFromParam && a.Param != b.Param {
		return ResultOrigin{Kind: ResultUnknown, Param: -1}
	}
	if b.Kind < a.Kind {
		return b
	}
	return a
}

func resultsDiffer(a []ResultOrigin, b []ResultOrigin) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

func (m *Model) buildProvenance(decls []*ast.FuncDecl) {
	for _, site := range m.spawnSites {
		if site.Decl == nil || site.Decl.Body == nil {
			continue
		}
		site.Origins = make([]Origin, len(site.Args))
		for i, arg := range site.Args {
			w := m.walkerFor(site.Decl, site.In != nil)
			w.walk(arg, 0)
			site.Origins[i] = w.verdict()
		}
	}

	for _, site := range m.SendSites {
		if site.Decl == nil || site.Decl.Body == nil {
			site.Origin = Origin{Prov: ProvUnknown, Witness: "no enclosing body", Param: -1}
			continue
		}
		w := m.walkerFor(site.Decl, site.In != nil)
		w.walk(site.Payload, 0)
		site.Origin = w.verdict()
	}
	m.resolveForwarded(decls)
}

type callSite struct {
	args []ast.Expr
	decl *ast.FuncDecl
	cb   *Callback
}

func (m *Model) resolveForwarded(decls []*ast.FuncDecl) {
	pending := false
	for _, site := range m.SendSites {
		if site.In == nil && site.Origin.Prov == ProvUnknown && site.Origin.Param >= 0 {
			pending = true
			break
		}
	}
	if pending == false {
		return
	}

	byDecl := map[*ast.FuncDecl]*Callback{}
	for _, cb := range m.Callbacks {
		byDecl[cb.Decl] = cb
	}

	callers := map[*types.Func][]callSite{}
	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		enclosing, cb := decl, byDecl[decl]
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if isCall == false {
				return true
			}
			fn, isFunc := calleeOf(m.pass.TypesInfo, call)
			if isFunc == false || fn.Pkg() != m.pass.Pkg {
				return true
			}
			callers[fn] = append(callers[fn], callSite{args: call.Args, decl: enclosing, cb: cb})
			return true
		})
	}

	for _, site := range m.SendSites {
		if site.In != nil || site.Origin.Prov != ProvUnknown || site.Origin.Param < 0 {
			continue
		}
		obj, _ := m.pass.TypesInfo.Defs[site.Decl.Name].(*types.Func)
		if obj == nil || obj.Exported() {
			continue
		}
		calls := callers[obj]
		if len(calls) == 0 {
			continue
		}

		merged := Origin{Prov: ProvTransferred, Param: -1, Witness: "every caller builds it fresh"}
		resolved := true
		for _, caller := range calls {
			if site.Origin.Param >= len(caller.args) || caller.decl == site.Decl {
				resolved = false
				break
			}
			w := m.walkerFor(caller.decl, caller.cb != nil)
			w.walk(caller.args[site.Origin.Param], 0)
			if at := w.verdict(); at.Prov >= merged.Prov {
				merged = at
			}
		}
		if resolved == false {
			continue
		}
		merged.Witness = site.Origin.Witness + ", and at its callers: " + merged.Witness
		merged.Param = -1
		site.Origin = merged
	}
}

func (m *Model) buildMutatedFields(decls []*ast.FuncDecl) {
	m.mutated = map[string]bool{}
	m.mutatedInside = map[string]bool{}

	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		named, ok := receiverType(m.pass.TypesInfo, decl)
		if ok == false {
			continue
		}
		recv := receiverName(decl)
		if recv == "" {
			continue
		}
		owner := typePath(named)

		note := func(e ast.Expr, inside bool) {
			field, isField := receiverSelector(e, recv)
			if isField == false {
				return
			}
			key := owner + "/" + bareField(field)
			m.mutated[key] = true
			if inside {
				m.mutatedInside[key] = true
			}
		}

		ast.Inspect(decl.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range x.Lhs {
					note(inPlaceTarget(lhs))
				}
				if len(x.Lhs) == len(x.Rhs) {
					for i, rhs := range x.Rhs {
						if _, isSelfAppend := appendIntoField(x.Lhs[i], rhs, recv); isSelfAppend {
							note(x.Lhs[i], false)
						}
					}
				}
			case *ast.IncDecStmt:
				note(inPlaceTarget(x.X))
			case *ast.CallExpr:
				id, isIdent := x.Fun.(*ast.Ident)
				if isIdent == false || len(x.Args) == 0 {
					return true
				}
				switch id.Name {
				case "delete", "clear", "copy":
					note(x.Args[0], false)
				}
			}
			return true
		})
	}
}

func (m *Model) FieldMutated(owner string, field string, inside bool) bool {
	key := owner + "/" + bareField(field)
	if inside {
		return m.mutatedInside[key]
	}
	return m.mutated[key]
}

func (m *Model) OwnFieldMutated(cb *Callback, field string) bool {
	if cb == nil || cb.Behavior == nil {
		return false
	}
	return m.FieldMutated(typePath(cb.Behavior), field, false)
}

func bareField(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

func inPlaceTarget(lhs ast.Expr) (ast.Expr, bool) {
	switch x := lhs.(type) {
	case *ast.IndexExpr:
		return inPlaceBase(x.X), false
	case *ast.StarExpr:
		return inPlaceBase(x.X), true
	case *ast.SelectorExpr:
		switch inner := x.X.(type) {
		case *ast.IndexExpr:
			return inPlaceBase(inner.X), true
		case *ast.SelectorExpr:
			return inner, true
		case *ast.StarExpr:
			return inPlaceBase(inner.X), true
		}
	}
	return nil, false
}

func inPlaceBase(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return inPlaceBase(x.X)
	case *ast.SelectorExpr:
		return x
	}
	return nil
}

func appendIntoField(lhs ast.Expr, rhs ast.Expr, recv string) ([]ast.Expr, bool) {
	field, isField := receiverSelector(lhs, recv)
	if isField == false {
		return nil, false
	}
	call, isCall := rhs.(*ast.CallExpr)
	if isCall == false {
		return nil, false
	}
	id, isIdent := call.Fun.(*ast.Ident)
	if isIdent == false || id.Name != "append" || len(call.Args) == 0 {
		return nil, false
	}
	first, isFirstField := receiverSelector(call.Args[0], recv)
	if isFirstField == false || first != field {
		return nil, false
	}
	return call.Args[1:], true
}

func (m *Model) elemAliases(t types.Type) bool {
	if t == nil {
		return true
	}
	switch u := types.Unalias(t).Underlying().(type) {
	case *types.Slice:
		return m.Shape(u.Elem()).Aliasing
	case *types.Map:
		return m.Shape(u.Elem()).Aliasing || m.Shape(u.Key()).Aliasing
	case *types.Array:
		return m.Shape(u.Elem()).Aliasing
	case *types.Pointer:
		return m.Shape(u.Elem()).Aliasing
	}
	return true
}

func (m *Model) isConversionCall(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.ArrayType:
		return true
	case *ast.Ident:
		_, isType := m.pass.TypesInfo.Uses[fun].(*types.TypeName)
		return isType
	case *ast.SelectorExpr:
		_, isType := m.pass.TypesInfo.Uses[fun.Sel].(*types.TypeName)
		return isType
	}
	return false
}

func isStringConversion(call *ast.CallExpr) bool {
	id, isIdent := call.Fun.(*ast.Ident)
	return isIdent && id.Name == "string"
}

func isCloneCall(call *ast.CallExpr) bool {
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if isSel == false {
		return false
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	if isIdent == false {
		return false
	}
	switch pkg.Name + "." + sel.Sel.Name {
	case "slices.Clone", "maps.Clone", "bytes.Clone":
		return true
	}
	return false
}

func calleeOf(info *types.Info, call *ast.CallExpr) (*types.Func, bool) {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		fn, ok := info.Uses[fun.Sel].(*types.Func)
		return fn, ok
	case *ast.Ident:
		fn, ok := info.Uses[fun].(*types.Func)
		return fn, ok
	}
	return nil, false
}

func callName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name + "()"
	case *ast.SelectorExpr:
		return fun.Sel.Name + "()"
	}
	return "a call"
}
