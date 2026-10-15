package ergomodel

import (
	"go/ast"
	"go/token"
	"go/types"
)

type OwnStateShare struct {
	Pos token.Pos

	Field     string
	Ref       types.Type
	Witness   string
	Direct    bool
	Addressed bool

	path string
}

func (m *Model) OwnStateSend(site *SendSite) (OwnStateShare, bool) {
	if site.In == nil || site.In.Recv == "" || m.SelfTarget(site) {
		return OwnStateShare{}, false
	}
	for _, cand := range m.receiverFields(site.Payload, site.In.Recv) {
		t := m.pass.TypesInfo.TypeOf(cand.expr)
		if t == nil || types.IsInterface(types.Unalias(t)) {
			continue
		}
		exp, exposed := m.Exposed(t)
		if exposed == false {
			continue
		}
		witness := exp.Witness
		if witness == "" {
			witness = types.TypeString(types.Unalias(t),
				func(p *types.Package) string { return p.Name() })
		}
		path, _ := m.recvPath(cand.field, receiverObject(m.pass.TypesInfo, site.In.Decl))
		return OwnStateShare{
			Pos: cand.expr.Pos(), Field: cand.text, Ref: exp.Ref,
			Witness: witness, Direct: cand.direct, Addressed: m.takesAddress(cand.expr),
			path: path,
		}, true
	}
	return OwnStateShare{}, false
}

type candidate struct {
	expr   ast.Expr
	field  *ast.SelectorExpr
	text   string
	direct bool
}

func (m *Model) receiverFields(payload ast.Expr, recv string) []candidate {
	var out []candidate

	if field, ok := m.receiverField(payload, recv); ok {
		return []candidate{{expr: payload, field: field, text: recv + "." + field.Sel.Name, direct: true}}
	}
	lit, ok := m.unwrapPayload(payload).(*ast.CompositeLit)
	if ok == false {
		return nil
	}
	for _, elt := range lit.Elts {
		value := elt
		if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
			value = kv.Value
		}
		if field, isField := m.receiverField(value, recv); isField {
			out = append(out, candidate{expr: value, field: field, text: recv + "." + field.Sel.Name})
		}
	}
	return out
}

func (m *Model) receiverField(e ast.Expr, recv string) (*ast.SelectorExpr, bool) {
	sel, ok := m.unwrapPayload(e).(*ast.SelectorExpr)
	if ok == false {
		return nil, false
	}
	id, isIdent := sel.X.(*ast.Ident)
	if isIdent == false || id.Name != recv {
		return nil, false
	}
	return sel, true
}

func (m *Model) unwrapPayload(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return m.unwrapPayload(x.X)
	case *ast.UnaryExpr:
		if x.Op == token.AND {
			return m.unwrapPayload(x.X)
		}
	case *ast.CallExpr:
		if m.isConversionCall(x) && len(x.Args) == 1 && m.copyingConversion(x) == false {
			return m.unwrapPayload(x.Args[0])
		}
	}
	return e
}

func (m *Model) takesAddress(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return m.takesAddress(x.X)
	case *ast.UnaryExpr:
		return x.Op == token.AND
	case *ast.CallExpr:
		if m.isConversionCall(x) && len(x.Args) == 1 {
			return m.takesAddress(x.Args[0])
		}
	}
	return false
}

func (m *Model) copyingConversion(call *ast.CallExpr) bool {
	for _, t := range []types.Type{m.pass.TypesInfo.TypeOf(call), m.pass.TypesInfo.TypeOf(call.Args[0])} {
		if t == nil {
			return true
		}
		if b, isBasic := t.Underlying().(*types.Basic); isBasic && b.Info()&types.IsString != 0 {
			return true
		}
	}
	return false
}
