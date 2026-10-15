package ergomodel

import (
	"go/ast"
	"go/types"
	"strings"
)

func (m *Model) SelfTarget(site *SendSite) bool {
	if site == nil || site.Decl == nil || len(site.Call.Args) < 2 {
		return false
	}
	named, ok := receiverType(m.pass.TypesInfo, site.Decl)
	if ok == false || m.processBehavior(named) == false {
		return false
	}
	recv := receiverObject(m.pass.TypesInfo, site.Decl)
	if recv == nil {
		return false
	}
	fn, ok := m.calleeFunc(site.Call)
	if ok == false {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if ok == false || sig.Params().Len() < 2 || sig.Params().At(0).Name() != "to" {
		return false
	}
	sel := site.Call.Fun.(*ast.SelectorExpr)
	if m.isSelf(sel.X, recv) == false {
		return false
	}
	return m.selfAddress(site.Decl, site.Call.Args[0], recv)
}

func (m *Model) processBehavior(named *types.Named) bool {
	for _, cb := range m.Callbacks {
		if cb.Behavior == named {
			return cb.Meta == false
		}
	}
	return false
}

func receiverObject(info *types.Info, decl *ast.FuncDecl) types.Object {
	if decl.Recv == nil || len(decl.Recv.List) == 0 || len(decl.Recv.List[0].Names) == 0 {
		return nil
	}
	return info.Defs[decl.Recv.List[0].Names[0]]
}

func (m *Model) isSelf(e ast.Expr, recv types.Object) bool {
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident:
		return m.pass.TypesInfo.Uses[x] == recv
	case *ast.SelectorExpr:
		s, ok := m.pass.TypesInfo.Selections[x]
		if ok == false || s.Kind() != types.FieldVal {
			return false
		}
		field, isVar := s.Obj().(*types.Var)
		return isVar && field.Anonymous() && m.isSelf(x.X, recv)
	}
	return false
}

func (m *Model) selfAddress(decl *ast.FuncDecl, e ast.Expr, recv types.Object) bool {
	switch x := ast.Unparen(e).(type) {
	case *ast.CallExpr:
		return m.selfCall(x, recv, -1)
	case *ast.Ident:
		f, ok := m.flows[decl]
		if ok == false || f.opaque[x.Name] || len(f.writes[x.Name]) > 0 {
			return false
		}
		if _, isParam := f.params[x.Name]; isParam {
			return false
		}
		if len(f.ranges[x.Name]) > 0 {
			return false
		}
		obj := m.pass.TypesInfo.Uses[x]
		found := false
		for _, b := range f.binds[x.Name] {
			if b.obj != obj {
				continue
			}
			call, isCall := b.expr.(*ast.CallExpr)
			if isCall == false || m.selfCall(call, recv, b.index) == false {
				return false
			}
			found = true
		}
		return found
	}
	return false
}

func (m *Model) selfCall(call *ast.CallExpr, recv types.Object, index int) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false || m.isSelf(sel.X, recv) == false {
		return false
	}
	fn, ok := m.pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if ok == false || fn.Pkg() == nil || strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
		return false
	}
	switch fn.Name() {
	case "PID", "Name":
		return index < 0
	case "CreateAlias":
		return index == 0
	}
	return false
}
