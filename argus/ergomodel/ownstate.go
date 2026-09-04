package ergomodel

import (
	"go/ast"
	"go/token"
	"go/types"
)

type OwnStateShare struct {
	Pos token.Pos

	Field   string
	Ref     types.Type
	Witness string
	Direct  bool
}

func (m *Model) OwnStateSend(site *SendSite) (OwnStateShare, bool) {
	if site.In == nil || site.In.Recv == "" {
		return OwnStateShare{}, false
	}
	for _, cand := range receiverFields(site.Payload, site.In.Recv) {
		t := m.pass.TypesInfo.TypeOf(cand.expr)
		if t == nil || types.IsInterface(types.Unalias(t)) {
			continue
		}
		if _, allowed := m.AllowedType(t); allowed {
			continue
		}
		s := m.Shape(t)
		if s.Aliasing == false {
			continue
		}
		ref := s.Ref
		if ref == nil {
			ref = t
		}
		if _, allowed := m.AllowedType(ref); allowed {
			continue
		}
		if m.Guarded(ref) == GuardGuarded {
			continue
		}
		witness := s.Witness
		if witness == "" {
			witness = types.TypeString(types.Unalias(t),
				func(p *types.Package) string { return p.Name() })
		}
		return OwnStateShare{
			Pos: cand.expr.Pos(), Field: cand.text, Ref: ref,
			Witness: witness, Direct: cand.direct,
		}, true
	}
	return OwnStateShare{}, false
}

type candidate struct {
	expr   ast.Expr
	text   string
	direct bool
}

func receiverFields(payload ast.Expr, recv string) []candidate {
	var out []candidate

	if c, ok := receiverSelector(payload, recv); ok {
		return []candidate{{expr: payload, text: c, direct: true}}
	}
	lit, ok := unwrapLiteral(payload).(*ast.CompositeLit)
	if ok == false {
		return nil
	}
	for _, elt := range lit.Elts {
		value := elt
		if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
			value = kv.Value
		}
		if c, isField := receiverSelector(value, recv); isField {
			out = append(out, candidate{expr: value, text: c})
		}
	}
	return out
}

func receiverSelector(e ast.Expr, recv string) (string, bool) {
	sel, ok := unwrapLiteral(e).(*ast.SelectorExpr)
	if ok == false {
		return "", false
	}
	id, isIdent := sel.X.(*ast.Ident)
	if isIdent == false || id.Name != recv {
		return "", false
	}
	return recv + "." + sel.Sel.Name, true
}

func unwrapLiteral(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return unwrapLiteral(x.X)
	case *ast.UnaryExpr:
		if x.Op == token.AND {
			return unwrapLiteral(x.X)
		}
	}
	return e
}
