package handlecallreason

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A1008"

var Analyzer = &analysis.Analyzer{
	Name: "argusA1008",
	Doc: `A1008: HandleCall must not return an error in the reason slot.

The second result of HandleCall is the process termination reason, not the reply.
"return nil, gen.ErrUnsupported" therefore kills the process and sends no reply,
so the caller also waits out its full timeout. Any peer sending an unrecognized
request type can terminate the callee, which makes this remotely triggerable.

To reject a request, put the error in the first result ("return err, nil") or call
SendResponseError. Returning gen.TerminateReasonNormal or TerminateReasonShutdown
in the second slot is the idiomatic reply-then-stop form and is not reported.

Source: messages.md, meta.md, actors.md`,
	URL:      "https://docs.ergo.services/tools/argus#A1008",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		if cb.Kind != ergomodel.CBHandleCall {
			continue
		}

		if resultCount(cb.Decl) != 2 {
			continue
		}
		ast.Inspect(cb.Decl.Body, func(n ast.Node) bool {

			if _, ok := n.(*ast.FuncLit); ok {
				return false
			}
			ret, ok := n.(*ast.ReturnStmt)
			if ok == false || len(ret.Results) != 2 {
				return true
			}
			if isNilIdent(ret.Results[0]) == false {
				return true
			}
			reason := ret.Results[1]
			if isNilIdent(reason) {
				return true
			}
			if isTerminateReason(pass.TypesInfo, reason) {
				return true
			}
			if implementsError(pass.TypesInfo.TypeOf(reason)) == false {
				return true
			}

			m.ReportFix(pass, ret.Pos(), ret.End(),
				ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 1,
					ID: ergomodel.CallbackID(cb)},
				swapFix(pass, ret, reason),
				"%s returns an error in the termination reason slot, which kills the process and sends no reply; put the error in the result instead",
				cb.Name)
			return true
		})
	}
	return nil, nil
}

func resultCount(fn *ast.FuncDecl) int {
	if fn.Type.Results == nil {
		return 0
	}
	n := 0
	for _, f := range fn.Type.Results.List {
		if len(f.Names) == 0 {
			n++
			continue
		}
		n += len(f.Names)
	}
	return n
}

func isNilIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

func isTerminateReason(info *types.Info, e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if ok == false {
		return false
	}
	switch sel.Sel.Name {
	case "TerminateReasonNormal", "TerminateReasonShutdown":
	default:
		return false
	}
	obj := info.Uses[sel.Sel]
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == "ergo.services/ergo/gen"
}

func implementsError(t types.Type) bool {
	if t == nil {
		return false
	}
	if types.Unalias(t).String() == "error" {
		return true
	}
	errIface, ok := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	if ok == false {
		return false
	}
	return types.Implements(t, errIface) || types.Implements(types.NewPointer(t), errIface)
}

func swapFix(pass *analysis.Pass, ret *ast.ReturnStmt, reason ast.Expr) []analysis.SuggestedFix {
	var buf bytes.Buffer
	if err := format.Node(&buf, pass.Fset, reason); err != nil {
		return nil
	}
	return []analysis.SuggestedFix{{
		Message: "return the error as the result",
		TextEdits: []analysis.TextEdit{{
			Pos:     ret.Results[0].Pos(),
			End:     ret.Results[1].End(),
			NewText: append(buf.Bytes(), []byte(", nil")...),
		}},
	}}
}
