package wiresentinel

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2022"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2022",
	Doc: `A2022: a sentinel put on the wire that this node does not register.

An error keeps its identity across a hop only through the error cache, and the
cache is filled by RegisterError. A sentinel that is not in it is encoded as its
text and decoded as a fresh value, so errors.Is answers false on the receiving side
while the message reads exactly right. Both nodes have to register it: the sender
to get an id onto the wire, the receiver to map that id back onto its own value.

The failure is invisible to unit tests by construction. Every test builds the
sentinel locally and compares it to itself, so it passes whether or not the
registration exists; only the cluster hop can tell the difference, and what it
produces is a caller falling through to a generic branch rather than an error.

Reported: an error valued expression naming a package level sentinel that reaches
the wire, when this package declares an error registration list and the sentinel is
not in it. The wire surface is A2020's: a send payload, the error argument of
SendResponseError, the reply slot of a HandleCall, and an argument handed to a
sender helper. A sentinel wrapped with gen.Errorf is read through the wrapper,
because the wrapper preserves the operand and the operand is what needs the entry.

Silent for a package that registers no errors at all, and for one whose list is a
call rather than a literal, because membership is then undecidable and the answer
would be "everything is missing".

Source: edf.md, messages.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2022",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	if m.RegistersErrors() == false || m.ErrorListOpaque() {
		return nil, nil
	}
	registered := m.RegisteredErrors()

	for _, site := range m.WireErrorSites() {
		for _, obj := range sentinels(pass, site.Expr) {
			if registered[obj] {
				continue
			}
			m.Report(pass, site.Pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindRegistration, Tier: 2,
					ID: m.DeclID(site.Decl) + ":" + qualified(obj), Witness: obj.Name(),
				},
				"%s is %s but it is not in this package's error registration list: an unregistered sentinel is encoded as its text, so errors.Is answers false on the other side of the hop while the message still reads correctly. Add it to RegisterErrors",
				qualified(obj), site.What)
		}
	}
	return nil, nil
}

func sentinels(pass *analysis.Pass, e ast.Expr) []types.Object {
	if obj := sentinelObject(pass, e); obj != nil {
		return []types.Object{obj}
	}
	call, ok := e.(*ast.CallExpr)
	if ok == false || isGenErrorf(pass, call) == false {
		return nil
	}
	var out []types.Object
	for _, arg := range call.Args[1:] {
		if obj := sentinelObject(pass, arg); obj != nil {
			out = append(out, obj)
		}
	}
	return out
}

func sentinelObject(pass *analysis.Pass, e ast.Expr) types.Object {
	var id *ast.Ident
	switch x := e.(type) {
	case *ast.Ident:
		id = x
	case *ast.SelectorExpr:
		id = x.Sel
	default:
		return nil
	}
	obj := pass.TypesInfo.Uses[id]
	v, isVar := obj.(*types.Var)
	if isVar == false || v.Parent() == nil || v.Pkg() == nil {
		return nil
	}
	if v.Parent() != v.Pkg().Scope() {
		return nil
	}

	if strings.HasPrefix(v.Pkg().Path(), "ergo.services/ergo/") {
		return nil
	}
	if isErrorType(v.Type()) == false {
		return nil
	}
	return obj
}

func isGenErrorf(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) < 2 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false || sel.Sel.Name != "Errorf" {
		return false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if isFunc == false || fn.Pkg() == nil {
		return false
	}
	return fn.Pkg().Path() == "ergo.services/ergo/gen"
}

func isErrorType(t types.Type) bool {
	return t != nil && types.Unalias(t).String() == "error"
}

func qualified(obj types.Object) string {
	if obj.Pkg() == nil {
		return obj.Name()
	}
	return obj.Pkg().Name() + "." + obj.Name()
}
