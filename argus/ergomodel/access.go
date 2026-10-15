package ergomodel

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

func (m *Model) fieldSteps(sel *ast.SelectorExpr) (string, bool) {
	s, ok := m.pass.TypesInfo.Selections[sel]
	if ok == false || s.Kind() != types.FieldVal {
		return "", false
	}
	var b strings.Builder
	t := s.Recv()
	for _, index := range s.Index() {
		if p, isPointer := t.Underlying().(*types.Pointer); isPointer {
			b.WriteString("*")
			t = p.Elem()
		}
		st, isStruct := t.Underlying().(*types.Struct)
		if isStruct == false {
			return "", false
		}
		field := st.Field(index)
		b.WriteString("." + field.Name())
		t = field.Type()
	}
	return b.String(), true
}

func indexStep(t types.Type) string {
	if t == nil {
		return "[]"
	}
	switch u := t.Underlying().(type) {
	case *types.Array:
		return "[#]"
	case *types.Pointer:
		if _, isArray := u.Elem().Underlying().(*types.Array); isArray {
			return "*[#]"
		}
	}
	return "[]"
}

func (m *Model) recvPath(e ast.Expr, recv types.Object) (string, bool) {
	if recv == nil {
		return "", false
	}
	info := m.pass.TypesInfo
	switch x := e.(type) {
	case *ast.ParenExpr:
		return m.recvPath(x.X, recv)

	case *ast.Ident:
		return "", info.Uses[x] == recv

	case *ast.SelectorExpr:
		steps, isField := m.fieldSteps(x)
		if isField == false {
			return "", false
		}
		base, ok := m.recvPath(x.X, recv)
		if ok == false {
			return "", false
		}
		if base == "" {
			steps = strings.TrimPrefix(steps, "*")
		}
		return base + steps, true

	case *ast.IndexExpr:
		base, ok := m.recvPath(x.X, recv)
		if ok == false {
			return "", false
		}
		return base + indexStep(info.TypeOf(x.X)), true

	case *ast.StarExpr:
		base, ok := m.recvPath(x.X, recv)
		if ok == false || base == "" {
			return base, ok
		}
		return base + "*", true

	case *ast.SliceExpr:
		base, ok := m.recvPath(x.X, recv)
		if ok == false {
			return "", false
		}
		if _, isPointer := info.TypeOf(x.X).Underlying().(*types.Pointer); isPointer {
			return base + "*", true
		}
		return base, true

	case *ast.TypeAssertExpr:
		return m.recvPath(x.X, recv)

	case *ast.CallExpr:
		if m.isConversionCall(x) && len(x.Args) == 1 && isStringConversion(x) == false {
			return m.recvPath(x.Args[0], recv)
		}
	}
	return "", false
}

func renderPath(path string) string {
	return strings.NewReplacer("*", "", "[#]", "[]").Replace(path)
}

func under(path string, prefix string) (string, bool) {
	if strings.HasPrefix(path, prefix) == false {
		return "", false
	}
	rest := path[len(prefix):]
	if rest != "" && strings.IndexByte(".[*", rest[0]) < 0 {
		return "", false
	}
	return rest, true
}

func throughShared(steps string) bool {
	return strings.Contains(steps, "[]") || strings.Contains(steps, "*")
}

func reaches(write string, held string, addressed bool) bool {
	if rest, ok := under(write, held); ok {
		return addressed || throughShared(rest)
	}
	if addressed {
		if rest, ok := under(held, write); ok {
			return throughShared(rest) == false
		}
	}
	return false
}

func (m *Model) buildMutatedFields(decls []*ast.FuncDecl) {
	m.mutated = map[string]bool{}
	m.mutatedInside = map[string]bool{}
	init := m.initOnly(decls, m.buildCallGraph(decls))

	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		named, ok := receiverType(m.pass.TypesInfo, decl)
		if ok == false {
			continue
		}
		recv := receiverObject(m.pass.TypesInfo, decl)
		if recv == nil {
			continue
		}
		owner := typePath(named)

		var visit func(root ast.Node, free bool)
		note := func(e ast.Expr, suffix string, free bool) {
			if _, rebinding := ast.Unparen(e).(*ast.Ident); rebinding && suffix == "" {
				return
			}
			path, isState := m.recvPath(e, recv)
			if isState == false {
				return
			}
			key := owner + "/" + path + suffix
			if init[decl] && free == false {
				m.mutatedInside[key] = true
				return
			}
			m.mutated[key] = true
		}
		visit = func(root ast.Node, free bool) {
			ast.Inspect(root, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.FuncLit:
					visit(x.Body, true)
					return false
				case *ast.GoStmt:
					visit(x.Call, true)
					return false
				case *ast.AssignStmt:
					for i, lhs := range x.Lhs {
						note(lhs, "", free)
						if len(x.Lhs) == len(x.Rhs) && m.appendsTo(lhs, x.Rhs[i], recv) {
							note(lhs, "[]", free)
						}
					}
				case *ast.RangeStmt:
					if x.Tok == token.ASSIGN {
						note(x.Key, "", free)
						note(x.Value, "", free)
					}
				case *ast.IncDecStmt:
					note(x.X, "", free)
				case *ast.CallExpr:
					if len(x.Args) == 0 || m.isBuiltin(x, "delete", "clear", "copy") == false {
						return true
					}
					note(x.Args[0], "[]", free)
				}
				return true
			})
		}
		visit(decl.Body, false)
	}
}

func (m *Model) isBuiltin(call *ast.CallExpr, names ...string) bool {
	id, isIdent := ast.Unparen(call.Fun).(*ast.Ident)
	if isIdent == false {
		return false
	}
	if _, builtin := m.pass.TypesInfo.Uses[id].(*types.Builtin); builtin == false {
		return false
	}
	for _, name := range names {
		if id.Name == name {
			return true
		}
	}
	return false
}

func (m *Model) appendsTo(lhs ast.Expr, rhs ast.Expr, recv types.Object) bool {
	_, ok := m.selfAppend(lhs, rhs, recv)
	return ok
}

func (m *Model) selfAppend(lhs ast.Expr, rhs ast.Expr, recv types.Object) ([]ast.Expr, bool) {
	call, isCall := rhs.(*ast.CallExpr)
	if isCall == false || len(call.Args) == 0 || m.isBuiltin(call, "append") == false {
		return nil, false
	}
	target, isState := m.recvPath(lhs, recv)
	if isState == false {
		return nil, false
	}
	first, isState := m.recvPath(call.Args[0], recv)
	if isState == false || first != target {
		return nil, false
	}
	return call.Args[1:], true
}

func (m *Model) written(owner string, held string, addressed bool, init bool) bool {
	if writesInto(m.mutated, owner, held, addressed) {
		return true
	}
	return init && writesInto(m.mutatedInside, owner, held, addressed)
}

func writesInto(table map[string]bool, owner string, held string, addressed bool) bool {
	prefix := owner + "/"
	for key := range table {
		if strings.HasPrefix(key, prefix) && reaches(key[len(prefix):], held, addressed) {
			return true
		}
	}
	return false
}

func (m *Model) OwnFieldMutated(cb *Callback, field string) bool {
	return m.OwnStateMutated(cb, OwnStateShare{Field: field})
}

func (m *Model) OwnStateMutated(cb *Callback, share OwnStateShare) bool {
	if cb == nil || cb.Behavior == nil {
		return false
	}
	path := share.path
	if path == "" {
		path = "." + bareField(share.Field)
	}
	return m.written(typePath(cb.Behavior), path, share.Addressed, cb.Kind == CBInit)
}

func bareField(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

type callSite struct {
	call *ast.CallExpr
	decl *ast.FuncDecl
	cb   *Callback
	free bool
}

type callGraph struct {
	callers map[*types.Func][]callSite
	escaped map[*types.Func]bool
	dynamic map[string]bool
}

func (m *Model) buildCallGraph(decls []*ast.FuncDecl) *callGraph {
	info := m.pass.TypesInfo
	g := &callGraph{
		callers: map[*types.Func][]callSite{},
		escaped: map[*types.Func]bool{},
		dynamic: map[string]bool{},
	}
	byDecl := map[*ast.FuncDecl]*Callback{}
	for _, cb := range m.Callbacks {
		byDecl[cb.Decl] = cb
	}
	asCall := map[*ast.Ident]bool{}

	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		decl, cb := decl, byDecl[decl]
		var visit func(root ast.Node, free bool)
		visit = func(root ast.Node, free bool) {
			ast.Inspect(root, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.FuncLit:
					visit(x.Body, true)
					return false
				case *ast.GoStmt:
					visit(x.Call, true)
					return false
				case *ast.CallExpr:
					fn, isFunc := calleeOf(info, x)
					if isFunc == false {
						return true
					}
					switch fun := ast.Unparen(x.Fun).(type) {
					case *ast.Ident:
						asCall[fun] = true
					case *ast.SelectorExpr:
						asCall[fun.Sel] = true
					}
					if fn.Pkg() == m.pass.Pkg {
						g.callers[fn] = append(g.callers[fn], callSite{call: x, decl: decl, cb: cb, free: free})
					}
				}
				return true
			})
		}
		visit(decl.Body, false)
	}

	for id, obj := range info.Uses {
		fn, isFunc := obj.(*types.Func)
		if isFunc == false {
			continue
		}
		if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil && types.IsInterface(sig.Recv().Type()) {
			g.dynamic[fn.Name()] = true
		}
		if fn.Pkg() == m.pass.Pkg && asCall[id] == false {
			g.escaped[fn] = true
		}
	}
	return g
}

func (m *Model) initOnly(decls []*ast.FuncDecl, g *callGraph) map[*ast.FuncDecl]bool {
	out := map[*ast.FuncDecl]bool{}
	for _, cb := range m.Callbacks {
		if cb.Kind == CBInit {
			out[cb.Decl] = true
		}
	}

	type candidate struct {
		decl *ast.FuncDecl
		obj  *types.Func
	}
	var candidates []candidate
	for _, decl := range decls {
		if decl.Body == nil || out[decl] {
			continue
		}
		obj, _ := m.pass.TypesInfo.Defs[decl.Name].(*types.Func)
		if obj == nil || obj.Exported() || g.escaped[obj] || g.dynamic[obj.Name()] || len(g.callers[obj]) == 0 {
			continue
		}
		out[decl] = true
		candidates = append(candidates, candidate{decl: decl, obj: obj})
	}

	for changed := true; changed; {
		changed = false
		for _, c := range candidates {
			if out[c.decl] == false {
				continue
			}
			for _, site := range g.callers[c.obj] {
				if site.free || out[site.decl] == false {
					out[c.decl] = false
					changed = true
					break
				}
			}
		}
	}
	return out
}
