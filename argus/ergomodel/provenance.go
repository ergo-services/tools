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
	Recv    bool
	Via     []*types.Var

	pending   string
	held      string
	addressed bool
}

type bind struct {
	expr  ast.Expr
	index int
	pos   token.Pos
	obj   types.Object
}

type fieldWrite struct {
	expr ast.Expr
	pos  token.Pos
	stmt ast.Stmt
}

type retain struct {
	path  string
	shown string
	stmt  token.Pos
	obj   types.Object
	exact bool
}

type flow struct {
	info     *types.Info
	body     *ast.BlockStmt
	labels   bool
	recv     string
	recvObj  types.Object
	owner    string
	params   map[string]int
	binds    map[string][]bind
	fields   map[string]map[string][]fieldWrite
	ranges   map[string][]bind
	writes   map[string][]ast.Expr
	copies   map[string][]ast.Expr
	opaque   map[string]bool
	retained map[string][]retain
}

func (m *Model) flowOf(decl *ast.FuncDecl) *flow {
	if f, ok := m.flows[decl]; ok {
		return f
	}
	f := m.buildFlow(decl)
	m.flows[decl] = f
	return f
}

func (m *Model) BoundBefore(decl *ast.FuncDecl, id *ast.Ident, at token.Pos) (ast.Expr, bool) {
	f, ok := m.flows[decl]
	if ok == false {
		return nil, false
	}
	obj := m.pass.TypesInfo.Uses[id]
	if obj == nil {
		return nil, false
	}
	var out ast.Expr
	best := token.NoPos
	for _, b := range f.binds[id.Name] {
		if b.obj != obj || b.pos >= at || b.pos < best {
			continue
		}
		out, best = b.expr, b.pos
	}
	return out, out != nil
}

func (m *Model) buildFlow(decl *ast.FuncDecl) *flow {
	f := &flow{
		info:     m.pass.TypesInfo,
		body:     decl.Body,
		recv:     receiverName(decl),
		recvObj:  receiverObject(m.pass.TypesInfo, decl),
		params:   map[string]int{},
		binds:    map[string][]bind{},
		fields:   map[string]map[string][]fieldWrite{},
		ranges:   map[string][]bind{},
		writes:   map[string][]ast.Expr{},
		copies:   map[string][]ast.Expr{},
		opaque:   map[string]bool{},
		retained: map[string][]retain{},
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
		case *ast.LabeledStmt:
			f.labels = true
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
	f.ranges[id.Name] = append(f.ranges[id.Name],
		bind{expr: src, index: -1, pos: id.Pos(), obj: f.info.ObjectOf(id)})
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
	f.binds[id.Name] = append(f.binds[id.Name],
		bind{expr: assert.X, index: -1, pos: id.Pos(), obj: f.info.ObjectOf(id)})
}

func noteDecl(f *flow, ds *ast.DeclStmt) {
	gd, ok := ds.Decl.(*ast.GenDecl)
	if ok == false || gd.Tok != token.VAR {
		return
	}
	for _, spec := range gd.Specs {
		vs, isVar := spec.(*ast.ValueSpec)
		if isVar == false {
			continue
		}
		switch {
		case len(vs.Values) == 0:
			for _, name := range vs.Names {
				f.binds[name.Name] = append(f.binds[name.Name],
					bind{index: -1, pos: name.Pos(), obj: f.info.ObjectOf(name)})
			}
		case len(vs.Values) == len(vs.Names):
			for i, name := range vs.Names {
				f.binds[name.Name] = append(f.binds[name.Name],
					bind{expr: vs.Values[i], index: -1, pos: name.Pos(), obj: f.info.ObjectOf(name)})
			}
		default:
			call, isCall := vs.Values[0].(*ast.CallExpr)
			for i, name := range vs.Names {
				if isCall && len(vs.Values) == 1 {
					f.binds[name.Name] = append(f.binds[name.Name],
						bind{expr: call, index: i, pos: name.Pos(), obj: f.info.ObjectOf(name)})
					continue
				}
				f.opaque[name.Name] = true
			}
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

		if _, isIdent := lhs.(*ast.Ident); isIdent == false {
			if path, isState := m.recvPath(lhs, f.recvObj); isState {
				m.noteStore(f, lhs, path, rhs, as)
				continue
			}
		}

		switch target := lhs.(type) {
		case *ast.Ident:
			if target.Name == "_" {
				continue
			}
			if values, isSelfAppend := appendInto(target.Name, rhs); isSelfAppend {
				spread := rhs.(*ast.CallExpr).Ellipsis.IsValid()
				for i, value := range values {
					if spread && i == len(values)-1 {
						f.copies[target.Name] = append(f.copies[target.Name], value)
						continue
					}
					f.writes[target.Name] = append(f.writes[target.Name], value)
				}
				continue
			}
			f.binds[target.Name] = append(f.binds[target.Name],
				bind{expr: rhs, index: -1, pos: target.Pos(), obj: f.info.ObjectOf(target)})

		case *ast.IndexExpr:
			if id, isIdent := target.X.(*ast.Ident); isIdent {
				f.writes[id.Name] = append(f.writes[id.Name], rhs)
			}

		case *ast.SelectorExpr:
			base, path, isPath := fieldPath(target)
			if isPath == false || base == f.recv {
				continue
			}
			if f.fields[base] == nil {
				f.fields[base] = map[string][]fieldWrite{}
			}
			f.fields[base][path] = append(f.fields[base][path],
				fieldWrite{expr: rhs, pos: target.Pos(), stmt: as})
		}
	}
}

func (m *Model) noteStore(f *flow, lhs ast.Expr, path string, rhs ast.Expr, as *ast.AssignStmt) {
	shown := f.recv + renderPath(path)
	if values, isSelfAppend := m.selfAppend(lhs, rhs, f.recvObj); isSelfAppend {
		spread := rhs.(*ast.CallExpr).Ellipsis.IsValid()
		for i, value := range values {
			if spread && i == len(values)-1 && m.elemExposed(m.pass.TypesInfo.TypeOf(value)) == false {
				continue
			}
			m.noteRetained(f, path+"[]", shown+"[]", value, as.Pos(), false, 0)
		}
		return
	}
	m.noteRetained(f, path, shown, rhs, as.Pos(), true, 0)
}

func noteMultiValue(f *flow, as *ast.AssignStmt) {
	if len(as.Lhs) == 2 {
		switch src := as.Rhs[0].(type) {
		case *ast.TypeAssertExpr:
			bindIdent(f, as.Lhs[0], bind{expr: src.X, index: -1, pos: as.Lhs[0].Pos()})
			return
		case *ast.IndexExpr:
			bindIdent(f, as.Lhs[0], bind{expr: src, index: -1, pos: as.Lhs[0].Pos()})
			return
		}
	}
	if call, isCall := as.Rhs[0].(*ast.CallExpr); isCall {
		for i, lhs := range as.Lhs {
			bindIdent(f, lhs, bind{expr: call, index: i, pos: lhs.Pos()})
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
	if len(call.Args) != 2 || m.isBuiltin(call, "copy") == false {
		return
	}
	dst, isIdent := call.Args[0].(*ast.Ident)
	if isIdent == false {
		return
	}
	if m.elemAliases(m.pass.TypesInfo.TypeOf(call.Args[1])) == false {
		return
	}
	f.copies[dst.Name] = append(f.copies[dst.Name], call.Args[1])
}

func bindIdent(f *flow, lhs ast.Expr, b bind) {
	id, ok := lhs.(*ast.Ident)
	if ok == false || id.Name == "_" {
		return
	}
	b.obj = f.info.ObjectOf(id)
	f.binds[id.Name] = append(f.binds[id.Name], b)
}

func (m *Model) noteRetained(f *flow, path string, shown string, rhs ast.Expr, stmt token.Pos, exact bool, depth int) {
	if rhs == nil || depth > 6 {
		return
	}
	if t := m.pass.TypesInfo.TypeOf(rhs); t != nil && m.Shape(t).Aliasing == false {
		return
	}
	switch x := rhs.(type) {
	case *ast.Ident:
		obj := f.info.Uses[x]
		if obj != nil && obj == f.recvObj {
			return
		}
		f.retained[x.Name] = append(f.retained[x.Name],
			retain{path: path, shown: shown, stmt: stmt, obj: obj, exact: exact})

	case *ast.ParenExpr:
		m.noteRetained(f, path, shown, x.X, stmt, exact, depth+1)

	case *ast.StarExpr:
		m.noteRetained(f, path, shown, x.X, stmt, false, depth+1)

	case *ast.SliceExpr:
		m.noteRetained(f, path, shown, x.X, stmt, false, depth+1)

	case *ast.IndexExpr:
		m.noteRetained(f, path, shown, x.X, stmt, false, depth+1)

	case *ast.SelectorExpr:
		m.noteRetained(f, path, shown, x.X, stmt, false, depth+1)

	case *ast.TypeAssertExpr:
		m.noteRetained(f, path, shown, x.X, stmt, false, depth+1)

	case *ast.UnaryExpr:
		if x.Op == token.AND {
			m.noteRetained(f, path, shown, x.X, stmt, false, depth+1)
		}

	case *ast.CompositeLit:
		for _, elt := range x.Elts {
			value := elt
			if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
				value = kv.Value
			}
			m.noteRetained(f, path, shown, value, stmt, false, depth+1)
		}

	case *ast.CallExpr:
		switch {
		case m.isConversionCall(x) && len(x.Args) == 1 && isStringConversion(x) == false:
			m.noteRetained(f, path, shown, x.Args[0], stmt, exact, depth+1)
		case m.isBuiltin(x, "append"):
			for i, arg := range x.Args {
				if i == 0 && isEmptyOrNil(arg) {
					continue
				}
				if x.Ellipsis.IsValid() && i == len(x.Args)-1 && m.elemExposed(m.pass.TypesInfo.TypeOf(arg)) == false {
					continue
				}
				m.noteRetained(f, path, shown, arg, stmt, false, depth+1)
			}
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

type walkKey struct {
	expr      ast.Expr
	pending   string
	addressed bool
	field     *types.Var
}

type provWalk struct {
	m         *Model
	f         *flow
	inbound   bool
	init      bool
	at        token.Pos
	pending   string
	addressed bool
	via       []*types.Var
	lost      int
	seen      map[walkKey]bool
	roots     []Origin
}

func (m *Model) walkerFor(decl *ast.FuncDecl, inbound bool, at token.Pos, init bool) *provWalk {
	return &provWalk{m: m, f: m.flowOf(decl), inbound: inbound, init: init, at: at, seen: map[walkKey]bool{}}
}

func (w *provWalk) verdict() Origin { return verdict(w.roots) }

func verdict(roots []Origin) Origin {
	out := Origin{Prov: ProvTransferred, Param: -1,
		Witness: "built here, nothing else reaches it"}
	for _, r := range roots {
		if rank(r) >= rank(out) {
			out = r
		}
	}
	return out
}

func rank(o Origin) int {
	r := int(o.Prov) * 2
	if o.Mutated {
		r++
	}
	return r
}

func (w *provWalk) trail() []*types.Var {
	if len(w.via) == 0 {
		return nil
	}
	return append([]*types.Var(nil), w.via...)
}

func (w *provWalk) enter(field *types.Var) bool {
	if field == nil || w.lost > 0 || w.pending != "" {
		return false
	}
	w.via = append(w.via, field)
	return true
}

func (w *provWalk) leave(entered bool) {
	if entered {
		w.via = w.via[:len(w.via)-1]
	}
}

func (w *provWalk) add(p Provenance, witness string, pos token.Pos) {
	w.roots = append(w.roots, Origin{Prov: p, Witness: witness, Pos: pos, Param: -1, Via: w.trail()})
}

func (w *provWalk) addParam(name string, index int, pos token.Pos) {
	o := Origin{Prov: ProvUnknown, Witness: "parameter " + name, Pos: pos, Param: index,
		Via: w.trail(), pending: w.pending, addressed: w.addressed}
	if w.inbound {
		o.Prov, o.Witness = ProvReceived, "arrived in "+name
	}
	w.roots = append(w.roots, o)
}

func (w *provWalk) owned(held string, deep bool, shown string, pos token.Pos, addressed bool) {
	addressed = addressed || w.addressed
	mutated := w.m.written(w.f.owner, held, addressed, w.init)
	witness := shown + ", only ever replaced whole"
	switch {
	case mutated && deep == false:
		witness = shown + ", mutated in place"
	case mutated:
		witness = shown + ", written through"
	}
	w.roots = append(w.roots, Origin{Prov: ProvOwned, Witness: witness, Pos: pos, Param: -1,
		Mutated: mutated, Recv: true, Via: w.trail(), held: held, addressed: addressed})
}

func (w *provWalk) walk(e ast.Expr, depth int) {
	if e == nil || depth > 8 {
		return
	}
	key := walkKey{expr: e, pending: w.pending, addressed: w.addressed}
	if len(w.via) > 0 {
		key.field = w.via[len(w.via)-1]
	}
	if w.seen[key] {
		return
	}
	w.seen[key] = true

	t := w.m.pass.TypesInfo.TypeOf(e)
	if t != nil && w.shares(t) == false {
		return
	}

	switch x := e.(type) {
	case *ast.CompositeLit:
		w.literal(x, t, depth)

	case *ast.ParenExpr:
		w.walk(x.X, depth+1)

	case *ast.StarExpr:
		w.project("*", x.X, depth+1)

	case *ast.SliceExpr:
		w.slice(x, depth+1)

	case *ast.IndexExpr:
		w.project(indexStep(w.m.pass.TypesInfo.TypeOf(x.X)), x.X, depth+1)

	case *ast.TypeAssertExpr:
		w.walk(x.X, depth+1)

	case *ast.UnaryExpr:
		if x.Op == token.AND {
			w.address(x.X, depth+1)
			return
		}
		w.add(ProvUnknown, short(t), x.Pos())

	case *ast.SelectorExpr:
		w.selector(x, depth)

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

func (w *provWalk) shares(t types.Type) bool {
	if w.addressed {
		return true
	}
	if w.lost == 0 && w.pending == "" {
		return w.m.exposes(t)
	}
	return w.m.Shape(t).Aliasing
}

func (w *provWalk) walkAt(pending string, e ast.Expr, depth int) {
	saved := w.pending
	w.pending = pending
	w.walk(e, depth)
	w.pending = saved
}

func (w *provWalk) project(steps string, e ast.Expr, depth int) {
	w.lost++
	w.walkAt(steps+w.pending, e, depth)
	w.lost--
}

func (w *provWalk) detached(e ast.Expr, depth int) {
	w.lost++
	w.walkAt("", e, depth)
	w.lost--
}

func (w *provWalk) elements(e ast.Expr, depth int) {
	if w.m.elemExposed(w.m.pass.TypesInfo.TypeOf(e)) == false {
		return
	}
	steps := "[]" + w.pending
	if strings.HasPrefix(w.pending, "[]") {
		steps = w.pending
	}
	w.lost++
	w.walkAt(steps, e, depth)
	w.lost--
}

func (w *provWalk) literal(lit *ast.CompositeLit, t types.Type, depth int) {
	var st *types.Struct
	if t != nil {
		st, _ = types.Unalias(t).Underlying().(*types.Struct)
	}
	head, rest := firstStep(w.pending)
	addressed := w.addressed
	w.addressed = false
	defer func() { w.addressed = addressed }()

	for i, elt := range lit.Elts {
		value := elt
		var field *types.Var
		if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
			value = kv.Value
			if key, isIdent := kv.Key.(*ast.Ident); isIdent && st != nil {
				field = fieldNamed(st, key.Name)
			}
		} else if st != nil && i < st.NumFields() {
			field = st.Field(i)
		}

		if w.pending != "" {
			switch {
			case field != nil && head == "."+field.Name():
				w.walkAt(rest, value, depth+1)
			case st == nil && (head == "[]" || head == "[#]"):
				w.walkAt(rest, value, depth+1)
			}
			continue
		}
		entered := w.enter(field)
		w.walk(value, depth+1)
		w.leave(entered)
	}
}

func typeAt(t types.Type, steps string) types.Type {
	for t != nil && steps != "" {
		head, rest := firstStep(steps)
		switch u := t.Underlying().(type) {
		case *types.Pointer:
			if head != "*" {
				return nil
			}
			t = u.Elem()
		case *types.Slice:
			t = u.Elem()
		case *types.Map:
			t = u.Elem()
		case *types.Array:
			t = u.Elem()
		case *types.Chan:
			t = u.Elem()
		case *types.Struct:
			field := fieldNamed(u, strings.TrimPrefix(head, "."))
			if field == nil {
				return nil
			}
			t = field.Type()
		default:
			return nil
		}
		steps = rest
	}
	return t
}

func fieldNamed(st *types.Struct, name string) *types.Var {
	for i := 0; i < st.NumFields(); i++ {
		if st.Field(i).Name() == name {
			return st.Field(i)
		}
	}
	return nil
}

func firstStep(path string) (string, string) {
	switch {
	case path == "":
		return "", ""
	case strings.HasPrefix(path, "*"):
		return "*", path[1:]
	case strings.HasPrefix(path, "[#]"):
		return "[#]", path[3:]
	case strings.HasPrefix(path, "[]"):
		return "[]", path[2:]
	}
	end := strings.IndexAny(path[1:], ".[*")
	if end < 0 {
		return path, ""
	}
	return path[:end+1], path[end+1:]
}

func (w *provWalk) slice(x *ast.SliceExpr, depth int) {
	if t := w.m.pass.TypesInfo.TypeOf(x.X); t != nil {
		if _, isArray := t.Underlying().(*types.Array); isArray {
			w.address(x.X, depth)
			return
		}
	}
	w.walk(x.X, depth)
}

func (w *provWalk) address(e ast.Expr, depth int) {
	if strings.HasPrefix(w.pending, "*") {
		w.walkAt(w.pending[1:], e, depth)
		return
	}
	if w.pending != "" {
		w.walk(e, depth)
		return
	}
	if path, isState := w.m.recvPath(e, w.f.recvObj); isState && path != "" {
		w.owned(path, false, "&"+w.f.recv+renderPath(path), e.Pos(), true)
		return
	}
	addressed := w.addressed
	w.addressed = true
	w.walk(e, depth)
	w.addressed = addressed
}

func (w *provWalk) selector(x *ast.SelectorExpr, depth int) {
	steps, isField := w.m.fieldSteps(x)
	if isField == false {
		w.add(ProvUnknown, types.ExprString(x), x.Pos())
		return
	}
	if id, isIdent := x.X.(*ast.Ident); isIdent && w.f.recvObj != nil && w.m.pass.TypesInfo.Uses[id] == w.f.recvObj {
		held := strings.TrimPrefix(steps, "*") + w.pending
		w.owned(held, w.pending != "", w.f.recv+renderPath(held), x.Pos(), false)
		return
	}
	w.project(steps, x.X, depth+1)
}

func (w *provWalk) ident(id *ast.Ident, depth int) {
	if id.Name == "nil" || id.Name == "_" {
		return
	}
	obj := w.m.pass.TypesInfo.Uses[id]
	if obj != nil && obj == w.f.recvObj {
		held := w.pending
		_, isPointer := obj.Type().Underlying().(*types.Pointer)
		if isPointer {
			held = strings.TrimPrefix(held, "*")
		}
		w.owned(held, held != "", w.f.recv+renderPath(held), id.Pos(), isPointer && w.pending == "")
		return
	}
	if w.addressed && throughShared(w.pending) == false {
		if part := typeAt(w.m.pass.TypesInfo.TypeOf(id), w.pending); part != nil && w.m.Shape(part).Aliasing == false {
			return
		}
		w.addressed = false
		defer func() { w.addressed = true }()
	}
	for _, r := range w.f.retained[id.Name] {
		if r.obj != nil && obj != nil && r.obj != obj {
			continue
		}
		if w.f.body != nil && w.f.labels == false && w.at.IsValid() && unreachable(w.f.body, w.at, r.stmt) {
			continue
		}
		held := r.path
		if r.exact {
			held += w.pending
		}
		w.owned(held, w.pending != "", r.shown+" = "+id.Name, id.Pos(), false)
	}
	if fields := w.f.fields[id.Name]; len(fields) > 0 {
		w.rebuilt(id, fields, depth)
		return
	}
	w.sources(id, depth)
}

func (w *provWalk) sources(id *ast.Ident, depth int) {
	obj := w.f.info.ObjectOf(id)
	binds := sameVariable(w.f.binds[id.Name], obj)
	ranges := sameVariable(w.f.ranges[id.Name], obj)
	writes := w.f.writes[id.Name]
	copies := w.f.copies[id.Name]
	index, isParam := w.f.params[id.Name]

	if isParam {
		w.addParam(id.Name, index, id.Pos())
	} else if len(binds) == 0 && len(ranges) == 0 && len(writes) == 0 && len(copies) == 0 {
		w.add(ProvUnknown, id.Name, id.Pos())
		return
	}
	if w.f.opaque[id.Name] {
		w.add(ProvUnknown, id.Name, id.Pos())
	}
	for _, b := range binds {
		w.walkBind(b, depth)
	}
	for _, r := range ranges {
		w.project(indexStep(w.m.pass.TypesInfo.TypeOf(r.expr)), r.expr, depth+1)
	}
	head, rest := firstStep(w.pending)
	for _, value := range writes {
		switch {
		case w.pending == "":
			w.walk(value, depth+1)
		case head == "[]" || head == "[#]":
			w.walkAt(rest, value, depth+1)
		default:
			w.detached(value, depth+1)
		}
	}
	for _, src := range copies {
		w.elements(src, depth+1)
	}
}

func sameVariable(binds []bind, obj types.Object) []bind {
	if obj == nil {
		return binds
	}
	out := binds[:0:0]
	for _, b := range binds {
		if b.obj == nil || b.obj == obj {
			out = append(out, b)
		}
	}
	return out
}

func (w *provWalk) walkBind(b bind, depth int) {
	call, isCall := b.expr.(*ast.CallExpr)
	if b.index < 0 || isCall == false {
		w.walk(b.expr, depth+1)
		return
	}
	w.callResult(call, b.index, depth+1)
}

func (w *provWalk) rebuilt(id *ast.Ident, fields map[string][]fieldWrite, depth int) {
	t := w.m.pass.TypesInfo.TypeOf(id)
	var st *types.Struct
	if t != nil {
		st, _ = types.Unalias(t).Underlying().(*types.Struct)
	}
	if st == nil || w.f.labels || insideClosure(ancestors(w.f.body, id.Pos())) {
		w.sources(id, depth)
		for _, list := range fields {
			for _, write := range list {
				w.detached(write.expr, depth+1)
			}
		}
		return
	}

	consumed := map[string]bool{}
	w.rebuildFrom(id, st, "", fields, consumed, depth)
	for path, list := range fields {
		if consumed[path] {
			continue
		}
		for _, write := range list {
			if write.pos < id.Pos() {
				w.detached(write.expr, depth+1)
			}
		}
	}
}

func (w *provWalk) rebuildFrom(id *ast.Ident, st *types.Struct, prefix string,
	fields map[string][]fieldWrite, consumed map[string]bool, depth int) {

	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		if w.shares(field.Type()) == false {
			continue
		}
		path := field.Name()
		if prefix != "" {
			path = prefix + "." + path
		}
		rel, overlaps := relative(w.pending, "."+path)
		if overlaps == false {
			continue
		}
		entered := w.enter(field)
		w.rebuildField(id, field, path, rel, fields, consumed, depth)
		w.leave(entered)
	}
}

func (w *provWalk) rebuildField(id *ast.Ident, field *types.Var, path string, rel string,
	fields map[string][]fieldWrite, consumed map[string]bool, depth int) {

	use := id.Pos()
	writes := fields[path]
	consumed[path] = true

	last := -1
	for i, write := range writes {
		if write.pos >= use || dominates(w.f.body, write.stmt, use) == false {
			continue
		}
		if last < 0 || write.pos > writes[last].pos {
			last = i
		}
	}

	if last >= 0 {
		from := writes[last].pos
		w.walkAt(rel, writes[last].expr, depth+1)
		for _, write := range writes {
			if write.pos > from && write.pos < use {
				w.walkAt(rel, write.expr, depth+1)
			}
		}
		obj := w.m.pass.TypesInfo.Uses[id]
		for _, b := range w.f.binds[id.Name] {
			if b.pos > from && b.pos < use && (obj == nil || b.obj == obj) {
				w.lost++
				saved := w.pending
				w.pending = "." + path + rel
				w.walkBind(b, depth)
				w.pending = saved
				w.lost--
			}
		}
		return
	}

	for _, write := range writes {
		if write.pos < use {
			w.walkAt(rel, write.expr, depth+1)
		}
	}
	if nested, isStruct := types.Unalias(field.Type()).Underlying().(*types.Struct); isStruct && writtenUnder(fields, path) {
		w.rebuildFrom(id, nested, path, fields, consumed, depth)
		return
	}
	w.lost++
	saved := w.pending
	w.pending = "." + path + rel
	w.sources(id, depth)
	w.pending = saved
	w.lost--
}

func writtenUnder(fields map[string][]fieldWrite, path string) bool {
	for key := range fields {
		if strings.HasPrefix(key, path+".") {
			return true
		}
	}
	return false
}

func relative(pending string, path string) (string, bool) {
	if pending == "" {
		return "", true
	}
	if rest, ok := under(pending, path); ok {
		return rest, true
	}
	if _, ok := under(path, pending); ok {
		return "", true
	}
	return "", false
}

func (w *provWalk) callResult(call *ast.CallExpr, index int, depth int) {
	if w.m.isConversionCall(call) && len(call.Args) == 1 {
		if isStringConversion(call) {
			return
		}
		w.walk(call.Args[0], depth+1)
		return
	}

	if id, isIdent := ast.Unparen(call.Fun).(*ast.Ident); isIdent {
		if _, builtin := w.m.pass.TypesInfo.Uses[id].(*types.Builtin); builtin {
			if id.Name == "append" {
				w.appended(call, depth)
			}
			return
		}
	}

	if isCloneCall(call) && len(call.Args) == 1 {
		if w.m.elemAliases(w.m.pass.TypesInfo.TypeOf(call.Args[0])) {
			w.elements(call.Args[0], depth+1)
		}
		return
	}

	fn, isFunc := w.m.callee(call)
	if isFunc == false {
		w.add(ProvUnknown, callName(call), call.Pos())
		return
	}

	if index == 0 && w.m.isRequest(fn) {
		w.add(ProvReceived, "came back from "+fn.Name()+"()", call.Pos())
		return
	}

	switch origin := w.m.ResultOrigin(fn, index); origin.Kind {
	case ResultFresh:
		return
	case ResultFromParam:
		if w.derived(call, fn, origin, depth) {
			return
		}
	}

	for _, escape := range w.m.Behavior(fn).Escapes {
		if escape.Result == index && w.m.ResultAliases(fn, index) {
			w.add(ProvOwned, fn.Name()+" hands back its receiver's "+escape.Field, call.Pos())
			return
		}
	}
	w.add(ProvUnknown, callName(call), call.Pos())
}

func (w *provWalk) appended(call *ast.CallExpr, depth int) {
	if len(call.Args) == 0 {
		return
	}
	if isEmptyOrNil(call.Args[0]) == false {
		w.walk(call.Args[0], depth+1)
	}
	head, rest := firstStep(w.pending)
	element := ""
	if head == "[]" {
		element = rest
	}
	for i, arg := range call.Args[1:] {
		if call.Ellipsis.IsValid() && i == len(call.Args)-2 {
			if w.m.elemAliases(w.m.pass.TypesInfo.TypeOf(arg)) {
				w.elements(arg, depth+1)
			}
			continue
		}
		w.walkAt(element, arg, depth+1)
	}
}

func (w *provWalk) derived(call *ast.CallExpr, fn *types.Func, origin ResultOrigin, depth int) bool {
	var recv ast.Expr
	if sel, isSel := ast.Unparen(call.Fun).(*ast.SelectorExpr); isSel {
		if s, isMethod := w.m.pass.TypesInfo.Selections[sel]; isMethod {
			if s.Kind() != types.MethodVal {
				return false
			}
			recv = sel.X
		}
	}
	if origin.Recv && recv == nil {
		return false
	}

	type source struct {
		arg   ast.Expr
		steps string
	}
	var from []source
	if len(origin.Params) > 0 {
		sig, isSig := fn.Type().(*types.Signature)
		if isSig == false {
			return false
		}
		for _, p := range origin.Params {
			switch {
			case sig.Variadic() && p.Index == sig.Params().Len()-1 && p.Index <= len(call.Args):
				for _, arg := range call.Args[p.Index:] {
					from = append(from, source{arg: arg, steps: p.Steps})
				}
			case p.Index < len(call.Args) && len(call.Args) == sig.Params().Len():
				from = append(from, source{arg: call.Args[p.Index], steps: p.Steps})
			default:
				return false
			}
		}
	}

	w.lost++
	for _, s := range from {
		w.walkAt(s.steps, s.arg, depth+1)
	}
	if origin.Recv {
		w.walkAt(origin.RecvSteps, recv, depth+1)
	}
	w.lost--
	if origin.Received {
		w.add(ProvReceived, "came back from a request inside "+callName(call), call.Pos())
	}
	return true
}

func (m *Model) ResultAliases(fn *types.Func, index int) bool {
	sig, ok := fn.Type().(*types.Signature)
	if ok == false || index < 0 || index >= sig.Results().Len() {
		return true
	}
	return m.Shape(sig.Results().At(index).Type()).Aliasing
}

func (m *Model) isRequest(fn *types.Func) bool {
	if fn.Pkg() == nil || strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
		return false
	}
	if strings.HasPrefix(fn.Name(), "Call") == false {
		return false
	}
	_, ok := m.senderParam(fn)
	return ok
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

func (m *Model) buildProvenance(decls []*ast.FuncDecl) {
	r := &resolver{m: m, decls: decls, active: map[*ast.FuncDecl]bool{}}
	r.init = m.initOnly(decls, r.calls())

	for _, site := range m.spawnSites {
		if site.Decl == nil || site.Decl.Body == nil {
			continue
		}
		site.Origins = make([]Origin, len(site.Args))
		for i, arg := range site.Args {
			site.Origins[i] = r.origin(site.Decl, site.In, arg, site.Call.Pos())
		}
	}

	for _, site := range m.SendSites {
		if site.Decl == nil || site.Decl.Body == nil {
			site.Origin = Origin{Prov: ProvUnknown, Witness: "no enclosing body", Param: -1}
			continue
		}
		site.Origin = r.origin(site.Decl, site.In, site.Payload, site.Call.Pos())
	}
}

const forwardDepth = 4

type resolver struct {
	m      *Model
	decls  []*ast.FuncDecl
	graph  *callGraph
	init   map[*ast.FuncDecl]bool
	active map[*ast.FuncDecl]bool
}

func (r *resolver) calls() *callGraph {
	if r.graph == nil {
		r.graph = r.m.buildCallGraph(r.decls)
	}
	return r.graph
}

func (r *resolver) origin(decl *ast.FuncDecl, cb *Callback, e ast.Expr, at token.Pos) Origin {
	w := r.m.walkerFor(decl, cb != nil, at, r.init[decl])
	w.walk(e, 0)
	roots := w.roots
	if cb == nil {
		roots = r.resolve(decl, roots, 0)
	}
	return verdict(roots)
}

func (r *resolver) resolve(decl *ast.FuncDecl, roots []Origin, depth int) []Origin {
	out := make([]Origin, 0, len(roots))
	for _, root := range roots {
		if root.Prov == ProvUnknown && root.Param >= 0 {
			if merged, ok := r.param(decl, root, depth); ok {
				out = append(out, merged)
				continue
			}
		}
		out = append(out, root)
	}
	return out
}

func (r *resolver) param(decl *ast.FuncDecl, root Origin, depth int) (Origin, bool) {
	if depth >= forwardDepth || r.active[decl] {
		return Origin{}, false
	}
	obj, _ := r.m.pass.TypesInfo.Defs[decl.Name].(*types.Func)
	if obj == nil || obj.Exported() {
		return Origin{}, false
	}
	g := r.calls()
	if g.escaped[obj] || len(g.callers[obj]) == 0 {
		return Origin{}, false
	}
	sig, _ := obj.Type().(*types.Signature)
	variadic := sig != nil && sig.Variadic() && root.Param == sig.Params().Len()-1

	r.active[decl] = true
	defer delete(r.active, decl)

	merged := Origin{Prov: ProvTransferred, Param: -1, Witness: "every caller builds it fresh"}
	for _, caller := range g.callers[obj] {
		if caller.decl == decl || caller.decl.Body == nil {
			return Origin{}, false
		}
		args := caller.call.Args
		pending := root.pending
		addressed := root.addressed
		switch {
		case variadic && root.Param <= len(args):
			if caller.call.Ellipsis.IsValid() == false {
				pending, addressed = "", false
			}
			args = args[root.Param:]
		case sig != nil && len(args) == sig.Params().Len() && root.Param < len(args):
			args = args[root.Param : root.Param+1]
		default:
			return Origin{}, false
		}

		w := r.m.walkerFor(caller.decl, caller.cb != nil, caller.call.Pos(), r.init[caller.decl])
		w.pending, w.addressed = pending, addressed
		for _, arg := range args {
			w.walk(arg, 0)
		}
		roots := w.roots
		if caller.cb == nil {
			roots = r.resolve(caller.decl, roots, depth+1)
		}
		if at := verdict(roots); rank(at) >= rank(merged) {
			merged = at
		}
	}
	merged.Witness = root.Witness + ", and at its callers: " + merged.Witness
	merged.Param = -1
	merged.Via = root.Via
	merged.pending, merged.addressed = "", false
	return merged, true
}

func (m *Model) elemExposed(t types.Type) bool {
	if t == nil {
		return true
	}
	switch u := types.Unalias(t).Underlying().(type) {
	case *types.Slice:
		return m.exposes(u.Elem())
	case *types.Map:
		return m.exposes(u.Elem()) || m.exposes(u.Key())
	case *types.Array:
		return m.exposes(u.Elem())
	case *types.Pointer:
		return m.exposes(u.Elem())
	}
	return true
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
