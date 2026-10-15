package ergomodel

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strings"
)

type FieldValue struct {
	Set bool
	Pos token.Pos

	Expr   ast.Expr
	Int    int64
	HasInt bool
	Str    string
	HasStr bool
	Bool   bool
	Sel    string
	IsNil  bool
}

type SupervisorSpec struct {
	In       *Callback
	Decl     *ast.FuncDecl
	Pos      token.Pos
	Var      string
	Resolved bool
	Why      string

	Kind string

	fields    map[string]FieldValue
	listField string

	Children    []map[string]FieldValue
	ChildrenPos []token.Pos

	ChildrenUnresolved bool
}

func (s *SupervisorSpec) Field(path string) FieldValue { return s.fields[path] }

func (m *Model) Specs() []*SupervisorSpec { return m.specs }

func (m *Model) BehaviorSpecs() []*SupervisorSpec { return m.behaviorSpecs }

func (m *Model) buildSpecs(decls []*ast.FuncDecl) {
	byDecl := map[*ast.FuncDecl]*Callback{}
	for _, cb := range m.Callbacks {
		byDecl[cb.Decl] = cb
	}
	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		for _, shape := range specShapes {
			for _, spec := range m.resolveSpecs(decl, shape) {
				spec.In = byDecl[decl]
				if shape.kind == "supervisor" {
					m.specs = append(m.specs, spec)
					continue
				}
				m.behaviorSpecs = append(m.behaviorSpecs, spec)
			}
		}
	}
}

const specTypePath = "ergo.services/ergo/act.SupervisorSpec"

type specShape struct {
	path string
	list string
	kind string
}

var specShapes = []specShape{
	{specTypePath, "Children", "supervisor"},
	{"ergo.services/ergo/act.RouterOptions", "Routes", "router"},
	{"ergo.services/ergo/act.PoolOptions", "", "pool"},
}

func (m *Model) resolveSpecs(decl *ast.FuncDecl, shape specShape) []*SupervisorSpec {
	if spec := m.resolveSpec(decl, shape); spec != nil {
		return []*SupervisorSpec{spec}
	}
	return m.resolveReturnedSpecs(decl, shape)
}

func (m *Model) resolveReturnedSpecs(decl *ast.FuncDecl, shape specShape) []*SupervisorSpec {
	var out []*SupervisorSpec
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if _, isLit := n.(*ast.FuncLit); isLit {
			return false
		}
		ret, isRet := n.(*ast.ReturnStmt)
		if isRet == false {
			return true
		}
		for i, result := range ret.Results {
			lit, isLit := unwrap(result).(*ast.CompositeLit)
			if isLit == false || isSpecType(m.pass.TypesInfo.TypeOf(lit), shape.path) == false {
				continue
			}
			if othersNil(ret.Results, i) == false {
				continue
			}
			spec := &SupervisorSpec{Decl: decl, fields: map[string]FieldValue{},
				Resolved: true, Kind: shape.kind, listField: shape.list,
				Pos: lit.Pos()}
			m.foldLiteral(spec, lit, "")
			out = append(out, spec)
		}
		return true
	})
	return out
}

func othersNil(results []ast.Expr, skip int) bool {
	for i, r := range results {
		if i == skip {
			continue
		}
		id, isIdent := r.(*ast.Ident)
		if isIdent == false || id.Name != "nil" {
			return false
		}
	}
	return true
}

func (m *Model) resolveSpec(decl *ast.FuncDecl, shape specShape) *SupervisorSpec {
	info := m.pass.TypesInfo

	spec := &SupervisorSpec{Decl: decl, fields: map[string]FieldValue{},
		Resolved: true, Kind: shape.kind, listField: shape.list}
	found := false

	for _, stmt := range decl.Body.List {
		switch st := stmt.(type) {
		case *ast.DeclStmt:
			gd, ok := st.Decl.(*ast.GenDecl)
			if ok == false || gd.Tok != token.VAR {
				continue
			}
			for _, s := range gd.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if ok == false || len(vs.Names) == 0 {
					continue
				}
				if isSpecType(info.TypeOf(vs.Type), shape.path) == false {
					continue
				}
				spec.Var, spec.Pos, found = vs.Names[0].Name, vs.Pos(), true
				if len(vs.Values) == 1 {
					m.foldLiteral(spec, vs.Values[0], "")
				}
			}
		case *ast.AssignStmt:
			if len(st.Lhs) != 1 || len(st.Rhs) != 1 {
				continue
			}
			id, ok := st.Lhs[0].(*ast.Ident)
			if ok == false {
				continue
			}
			if isSpecType(info.TypeOf(st.Rhs[0]), shape.path) == false {
				continue
			}
			if found && id.Name != spec.Var {
				continue
			}
			if found == false {
				spec.Var, spec.Pos, found = id.Name, st.Pos(), true
			}
			m.foldLiteral(spec, st.Rhs[0], "")
		}
		if found {
			break
		}
	}
	if found == false {
		return nil
	}

	for _, stmt := range decl.Body.List {
		switch st := stmt.(type) {
		case *ast.AssignStmt:
			m.foldAssign(spec, st)
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			if m.touchesSpec(stmt, spec.Var) {
				spec.Resolved = false
				spec.Why = "a field is assigned in a branch or a loop"
			}
		case *ast.ExprStmt:

			if call, ok := st.X.(*ast.CallExpr); ok && m.passesSpec(call, spec.Var) {
				spec.Resolved = false
				spec.Why = "the spec escapes into a call"
			}
		}
	}
	return spec
}

func isSpecType(t types.Type, path string) bool {
	named, ok := deref(t).(*types.Named)
	if ok == false {
		return false
	}
	return typePath(named) == path
}

func (m *Model) foldAssign(spec *SupervisorSpec, st *ast.AssignStmt) {
	if len(st.Lhs) != 1 || len(st.Rhs) != 1 {
		return
	}

	if id, ok := st.Lhs[0].(*ast.Ident); ok && id.Name == spec.Var {
		call, isCall := st.Rhs[0].(*ast.CallExpr)
		if isCall == false {
			return
		}
		if m.passesSpec(call, spec.Var) == false {
			spec.Resolved = false
			spec.Why = "the spec is replaced by a value we cannot resolve"
			return
		}
		if m.foldHook(spec, call) == false {
			spec.Resolved = false
			spec.Why = "the spec passes through a hook we cannot resolve"
		}
		return
	}

	path, ok := selectorPath(st.Lhs[0], spec.Var)
	if ok == false {
		return
	}
	if spec.listField != "" && path == spec.listField {
		m.foldChildren(spec, st.Rhs[0])
		return
	}
	if spec.listField != "" && strings.HasPrefix(path, spec.listField+"[") {
		spec.ChildrenUnresolved = true
		return
	}
	m.setField(spec, path, st.Rhs[0], st.Pos())
}

func (m *Model) foldHook(spec *SupervisorSpec, call *ast.CallExpr) bool {
	fn, ok := m.calleeFunc(call)
	if ok == false {
		if id, isIdent := call.Fun.(*ast.Ident); isIdent {
			fn, ok = m.pass.TypesInfo.Uses[id].(*types.Func)
		}
	}
	if ok == false || fn.Pkg() != m.pass.Pkg {
		return false
	}
	decl, ok := m.declOf[fn]
	if ok == false || decl.Body == nil {
		return false
	}

	param := ""
	if decl.Type.Params != nil {
		for _, field := range decl.Type.Params.List {
			if isSpecType(m.pass.TypesInfo.TypeOf(field.Type), specTypePath) && len(field.Names) > 0 {
				param = field.Names[0].Name
			}
		}
	}
	if param == "" {
		return false
	}
	inner := &SupervisorSpec{Var: param, fields: spec.fields, Resolved: true,
		Kind: spec.Kind, listField: spec.listField}
	for _, stmt := range decl.Body.List {
		if st, ok := stmt.(*ast.AssignStmt); ok {
			m.foldAssign(inner, st)
		}
	}
	if inner.Resolved == false {
		return false
	}
	spec.Children, spec.ChildrenPos = inner.Children, inner.ChildrenPos
	spec.ChildrenUnresolved = inner.ChildrenUnresolved
	return true
}

func (m *Model) passesSpec(call *ast.CallExpr, name string) bool {
	for _, arg := range call.Args {
		if id, ok := arg.(*ast.Ident); ok && id.Name == name {
			return true
		}
	}
	return false
}

func (m *Model) touchesSpec(n ast.Node, name string) bool {
	found := false
	ast.Inspect(n, func(node ast.Node) bool {
		if found {
			return false
		}
		as, ok := node.(*ast.AssignStmt)
		if ok == false {
			return true
		}
		for _, lhs := range as.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
				found = true
			}
			if _, ok := selectorPath(lhs, name); ok {
				found = true
			}
		}
		return true
	})
	return found
}

func (m *Model) foldLiteral(spec *SupervisorSpec, e ast.Expr, prefix string) {
	lit, ok := unwrap(e).(*ast.CompositeLit)
	if ok == false {
		return
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if ok == false {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if ok == false {
			continue
		}
		path := key.Name
		if prefix != "" {
			path = prefix + "." + key.Name
		}
		if spec.listField != "" && path == spec.listField {
			m.foldChildren(spec, kv.Value)
			continue
		}
		if _, isLit := unwrap(kv.Value).(*ast.CompositeLit); isLit {
			m.foldLiteral(spec, kv.Value, path)
			continue
		}
		if isStructValue(m.pass.TypesInfo.TypeOf(kv.Value)) && m.foldValue(spec, kv.Value, path) {
			continue
		}
		m.setField(spec, path, kv.Value, kv.Pos())
	}
}

func isStructValue(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Struct)
	return ok
}

func (m *Model) foldValue(spec *SupervisorSpec, e ast.Expr, prefix string) bool {
	if _, isLit := unwrap(e).(*ast.CompositeLit); isLit {
		m.foldLiteral(spec, e, prefix)
		return true
	}
	id, ok := ast.Unparen(e).(*ast.Ident)
	if ok == false {
		return false
	}
	v, ok := m.pass.TypesInfo.Uses[id].(*types.Var)
	if ok == false || v.Pkg() != m.pass.Pkg {
		return false
	}
	if v.Parent() == m.pass.Pkg.Scope() {
		if v.Exported() {
			return false
		}
		roots := make([]ast.Node, 0, len(m.pass.Files))
		for _, file := range m.pass.Files {
			roots = append(roots, file)
		}
		writes, ok := m.writesTo(v, roots)
		if ok == false || len(writes) != 1 || writes[0].define == false {
			return false
		}
		return m.applyWrites(spec, writes, prefix)
	}
	decl := m.enclosingDecl(id.Pos())
	if decl == nil {
		return false
	}
	writes, ok := m.writesTo(v, []ast.Node{decl})
	if ok == false {
		return false
	}
	return m.foldLocal(spec, decl, id, writes, prefix)
}

type varWrite struct {
	stmt   ast.Node
	define bool
	path   string
	value  ast.Expr
	pos    token.Pos
}

func (m *Model) writesTo(v *types.Var, roots []ast.Node) ([]varWrite, bool) {
	info := m.pass.TypesInfo
	var out []varWrite
	ok := true

	rooted := func(e ast.Expr) (string, bool) {
		root := ast.Unparen(e)
		for {
			switch x := root.(type) {
			case *ast.SelectorExpr:
				root = ast.Unparen(x.X)
				continue
			case *ast.IndexExpr:
				root = ast.Unparen(x.X)
				continue
			case *ast.StarExpr:
				root = ast.Unparen(x.X)
				continue
			case *ast.Ident:
				if info.Uses[x] != v && info.Defs[x] != v {
					return "", false
				}
				if x == ast.Unparen(e) {
					return "", true
				}
				path, isPath := selectorPath(ast.Unparen(e), x.Name)
				if isPath == false {
					ok = false
				}

				if strings.Contains(path, "[") {
					return "", false
				}
				return path, true
			}
			return "", false
		}
	}

	for _, root := range roots {
		ast.Inspect(root, func(n ast.Node) bool {
			if ok == false {
				return false
			}
			switch x := n.(type) {
			case *ast.ValueSpec:
				for i, name := range x.Names {
					if info.Defs[name] != v {
						continue
					}
					w := varWrite{stmt: x, define: true, pos: x.Pos()}
					switch {
					case len(x.Values) == len(x.Names):
						w.value = x.Values[i]
					case len(x.Values) != 0:
						ok = false
					}
					out = append(out, w)
				}
			case *ast.AssignStmt:
				for i, lhs := range x.Lhs {
					path, hit := rooted(lhs)
					if hit == false {
						continue
					}
					if len(x.Lhs) != len(x.Rhs) || (x.Tok != token.ASSIGN && x.Tok != token.DEFINE) {
						ok = false
						return false
					}
					id, _ := lhs.(*ast.Ident)
					out = append(out, varWrite{
						stmt: x, define: id != nil && info.Defs[id] == v,
						path: path, value: x.Rhs[i], pos: x.Pos(),
					})
				}
			case *ast.IncDecStmt:
				if _, hit := rooted(x.X); hit {
					ok = false
				}
			case *ast.UnaryExpr:
				if _, hit := rooted(x.X); hit && x.Op == token.AND {
					ok = false
				}
			case *ast.RangeStmt:
				for _, e := range []ast.Expr{x.Key, x.Value} {
					if e == nil {
						continue
					}
					if _, hit := rooted(e); hit {
						ok = false
					}
				}
			case *ast.SelectorExpr:
				if sel := info.Selections[x]; sel != nil && sel.Kind() == types.MethodVal {
					if _, hit := rooted(x.X); hit && pointerMethod(sel.Obj()) {
						ok = false
					}
				}
			}
			return true
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pos < out[j].pos })
	return out, ok
}

func pointerMethod(obj types.Object) bool {
	fn, ok := obj.(*types.Func)
	if ok == false {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if ok == false || sig.Recv() == nil {
		return false
	}
	_, isPtr := sig.Recv().Type().(*types.Pointer)
	return isPtr
}

func (m *Model) foldLocal(spec *SupervisorSpec, decl *ast.FuncDecl, id *ast.Ident, writes []varWrite, prefix string) bool {
	if len(writes) == 0 || writes[0].define == false {
		return false
	}
	path := pathTo(decl.Body, id)
	if path == nil {
		return false
	}
	def := writes[0].stmt
	onPath := map[ast.Node]bool{}
	for _, n := range path {
		onPath[n] = true
		switch n.(type) {
		case *ast.FuncLit:
			if encloses(n, def) == false && len(writes) > 1 {
				return false
			}
		case *ast.ForStmt, *ast.RangeStmt:
			if encloses(n, def) {
				continue
			}
			for _, w := range writes {
				if encloses(n, w.stmt) {
					return false
				}
			}
		}
	}

	before := writes[:1]
	for _, w := range writes[1:] {
		if w.pos >= id.Pos() || encloses(w.stmt, id) {
			continue
		}
		switch parent := parentOf(decl.Body, w.stmt); parent.(type) {
		case *ast.BlockStmt, *ast.CaseClause, *ast.CommClause:
			if onPath[parent] == false {
				return false
			}
		default:
			return false
		}
		before = append(before, w)
	}
	return m.applyWrites(spec, before, prefix)
}

func (m *Model) applyWrites(spec *SupervisorSpec, writes []varWrite, prefix string) bool {
	join := func(path string) string {
		switch {
		case prefix == "":
			return path
		case path == "":
			return prefix
		}
		return prefix + "." + path
	}
	for _, w := range writes {
		target := join(w.path)
		clearUnder(spec.fields, target)
		if w.value == nil {
			continue
		}
		if w.path != "" && isStructValue(m.pass.TypesInfo.TypeOf(w.value)) == false {
			m.setField(spec, target, w.value, w.pos)
			continue
		}
		if m.foldValue(spec, w.value, target) == false {
			if w.path == "" {
				return false
			}
			m.setField(spec, target, w.value, w.pos)
		}
	}
	return true
}

func clearUnder(fields map[string]FieldValue, path string) {
	for key := range fields {
		if path == "" || key == path || strings.HasPrefix(key, path+".") {
			delete(fields, key)
		}
	}
}

func (m *Model) enclosingDecl(pos token.Pos) *ast.FuncDecl {
	for _, file := range m.pass.Files {
		if pos < file.Pos() || pos >= file.End() {
			continue
		}
		for _, d := range file.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil && fd.Pos() <= pos && pos < fd.End() {
				return fd
			}
		}
	}
	return nil
}

func encloses(outer, inner ast.Node) bool {
	return outer.Pos() <= inner.Pos() && inner.End() <= outer.End()
}

func pathTo(root, target ast.Node) []ast.Node {
	var stack, found []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if encloses(n, target) == false {
			return false
		}
		stack = append(stack, n)
		if n == target {
			found = append([]ast.Node(nil), stack...)
			return false
		}
		return true
	})
	return found
}

func parentOf(root, target ast.Node) ast.Node {
	path := pathTo(root, target)
	if len(path) < 2 {
		return nil
	}
	return path[len(path)-2]
}

func (m *Model) foldChildren(spec *SupervisorSpec, e ast.Expr) {
	lit, ok := unwrap(e).(*ast.CompositeLit)
	if ok == false {
		spec.ChildrenUnresolved = true
		return
	}
	spec.Children, spec.ChildrenPos = nil, nil
	spec.ChildrenUnresolved = false
	for _, elt := range lit.Elts {
		child := &SupervisorSpec{fields: map[string]FieldValue{}}
		if _, isLit := unwrap(elt).(*ast.CompositeLit); isLit == false {
			spec.ChildrenUnresolved = true
			return
		}
		m.foldLiteral(child, elt, "")
		spec.Children = append(spec.Children, child.fields)
		spec.ChildrenPos = append(spec.ChildrenPos, elt.Pos())
	}
}

func (m *Model) setField(spec *SupervisorSpec, path string, e ast.Expr, pos token.Pos) {
	fv := m.resolveValue(e)
	fv.Pos = pos
	spec.fields[path] = fv
}

func (m *Model) resolveValue(e ast.Expr) FieldValue {
	fv := FieldValue{Set: true, Pos: e.Pos(), Expr: e}

	if sel, ok := unwrap(e).(*ast.SelectorExpr); ok {
		fv.Sel = sel.Sel.Name
	}
	if id, ok := unwrap(e).(*ast.Ident); ok {
		switch id.Name {
		case "nil":
			fv.IsNil = true
			return fv
		default:
			fv.Sel = id.Name
		}
	}
	if tv, ok := m.pass.TypesInfo.Types[e]; ok && tv.Value != nil {
		switch tv.Value.Kind() {
		case constant.Int:
			if i, exact := constant.Int64Val(tv.Value); exact {
				fv.Int, fv.HasInt = i, true
			}
		case constant.String:
			fv.Str, fv.HasStr = constant.StringVal(tv.Value), true
		case constant.Bool:
			fv.Bool = constant.BoolVal(tv.Value)
		}
	}

	return fv
}

func selectorPath(e ast.Expr, base string) (string, bool) {
	var parts []string
	cur := e
	for {
		switch x := cur.(type) {
		case *ast.SelectorExpr:
			parts = append([]string{x.Sel.Name}, parts...)
			cur = x.X
		case *ast.IndexExpr:
			inner, ok := selectorPath(x.X, base)
			if ok == false {
				return "", false
			}
			return fmt.Sprintf("%s[]%s", inner, strings.Join(parts, ".")), true
		case *ast.Ident:
			if x.Name != base || len(parts) == 0 {
				return "", false
			}
			return strings.Join(parts, "."), true
		default:
			return "", false
		}
	}
}

func unwrap(e ast.Expr) ast.Expr {
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.UnaryExpr:
			if x.Op != token.AND {
				return e
			}
			e = x.X
		default:
			return e
		}
	}
}
