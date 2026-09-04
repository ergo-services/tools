package ergomodel

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

type WireErrorSite struct {
	Expr  ast.Expr
	Pos   token.Pos
	What  string
	Field string
	In    *Callback
	Decl  *ast.FuncDecl
}

func (m *Model) WireErrorSites() []*WireErrorSite { return m.wireErrorSites }

var responseErrorMethods = map[string]bool{
	"SendResponseError":          true,
	"SendResponseErrorImportant": true,
}

func (m *Model) buildWireErrorSites(decls []*ast.FuncDecl) {
	byDecl := map[*ast.FuncDecl]*Callback{}
	for _, cb := range m.Callbacks {
		byDecl[cb.Decl] = cb
	}

	seen := map[ast.Expr]bool{}
	add := func(e ast.Expr, what, field string, cb *Callback, decl *ast.FuncDecl) {
		if e == nil || seen[e] {
			return
		}
		seen[e] = true
		m.wireErrorSites = append(m.wireErrorSites, &WireErrorSite{
			Expr: e, Pos: e.Pos(), What: what, Field: field, In: cb, Decl: decl,
		})
	}

	collect := func(payload ast.Expr, singles map[string]ast.Expr, what string, cb *Callback, decl *ast.FuncDecl) {
		if payload == nil {
			return
		}
		if isErrorType(m.pass.TypesInfo.TypeOf(payload)) {
			add(payload, what, "", cb, decl)
			return
		}
		lit := m.literalOf(payload, singles)
		if lit == nil {
			return
		}
		for _, f := range m.errorFields(lit) {
			add(f.value, what, f.name, cb, decl)
		}
	}

	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		cb := byDecl[decl]
		singles := m.singleAssignedLiterals(decl.Body)

		for _, site := range m.SendSites {
			if contains(decl.Body, site.Payload.Pos()) == false {
				continue
			}
			collect(site.Payload, singles, "sent as a message", cb, decl)
		}

		ast.Inspect(decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok == false {
				return true
			}
			fn, isFunc := m.calleeFunc(call)
			if isFunc == false {
				return true
			}
			if responseErrorMethods[fn.Name()] && fn.Pkg() != nil &&
				strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) && len(call.Args) > 2 {
				add(call.Args[2], "answered to a request with "+fn.Name(), "", cb, decl)
				return true
			}

			for _, idx := range m.Behavior(fn).Sender {
				if idx < len(call.Args) {
					collect(call.Args[idx], singles, "handed to "+fn.Name()+", which sends it", cb, decl)
				}
			}
			return true
		})

		if cb == nil || cb.Kind != CBHandleCall {
			continue
		}
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			if _, isLit := n.(*ast.FuncLit); isLit {
				return false
			}
			ret, isRet := n.(*ast.ReturnStmt)
			if isRet == false || len(ret.Results) != 2 {
				return true
			}
			collect(ret.Results[0], singles, "returned as the reply to a request", cb, decl)
			return true
		})
	}
}

func (m *Model) literalOf(e ast.Expr, singles map[string]ast.Expr) *ast.CompositeLit {
	switch x := unwrap(e).(type) {
	case *ast.CompositeLit:
		return x
	case *ast.Ident:
		if rhs, ok := singles[x.Name]; ok {
			if lit, isLit := unwrap(rhs).(*ast.CompositeLit); isLit {
				return lit
			}
		}
	}
	return nil
}

type errorField struct {
	name  string
	value ast.Expr
}

func (m *Model) errorFields(lit *ast.CompositeLit) []errorField {
	st, ok := deref(m.pass.TypesInfo.TypeOf(lit)).Underlying().(*types.Struct)
	if ok == false {
		return nil
	}
	fields := map[string]types.Type{}
	for i := 0; i < st.NumFields(); i++ {
		fields[st.Field(i).Name()] = st.Field(i).Type()
	}
	var out []errorField
	for _, elt := range lit.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if isKV == false {
			continue
		}
		key, isIdent := kv.Key.(*ast.Ident)
		if isIdent == false {
			continue
		}
		if isErrorType(fields[key.Name]) == false {
			continue
		}
		out = append(out, errorField{name: key.Name, value: kv.Value})
	}
	return out
}

func isErrorType(t types.Type) bool {
	if t == nil {
		return false
	}
	return types.Unalias(t).String() == "error"
}

func contains(body *ast.BlockStmt, pos token.Pos) bool {
	return body != nil && pos >= body.Pos() && pos <= body.End()
}

func (m *Model) WireReachable(named *types.Named) []*types.Named {
	var out []*types.Named
	seen := map[types.Type]bool{}
	var walk func(t types.Type, top bool)
	walk = func(t types.Type, top bool) {
		if t == nil {
			return
		}
		t = types.Unalias(t)
		if seen[t] {
			return
		}
		seen[t] = true

		if n, isNamed := t.(*types.Named); isNamed {
			if top == false {
				if m.builtinWireType(n) == false {
					out = append(out, n)

					return
				}
				return
			}
			if m.hasMarshalerPair(n) {
				return
			}
			walk(n.Underlying(), true)
			return
		}

		switch u := t.(type) {
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				f := u.Field(i)
				if tagOf(u, i) == "-" {
					continue
				}
				walk(f.Type(), false)
			}
		case *types.Slice:
			walk(u.Elem(), false)
		case *types.Array:
			walk(u.Elem(), false)
		case *types.Pointer:
			walk(u.Elem(), false)
		case *types.Map:
			walk(u.Key(), false)
			walk(u.Elem(), false)
		}
	}
	walk(named, true)
	return out
}

func (m *Model) builtinWireType(n *types.Named) bool {
	path := typePath(n)
	if frameworkValueTypes[path] || path == "error" {
		return true
	}
	if strings.HasPrefix(path, ergoPrefix) || strings.HasPrefix(path, "ergo.services/ergo.") {
		return true
	}
	if _, ok := m.cfg.AllowTypes[path]; ok {
		return true
	}
	return m.hasMarshalerPair(n)
}

func tagOf(st *types.Struct, i int) string {
	tag := st.Tag(i)
	const key = `edf:"`
	at := strings.Index(tag, key)
	if at < 0 {
		return ""
	}
	rest := tag[at+len(key):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}
