package uncheckedresult

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2001a"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2001a",
	Doc: `A2001a: a discarded result that hides a certain failure.

Two shapes, and the rule is deliberately narrow: the framework's own tree discards
about 180 timer results, almost all of them correctly, so a rule that reported every
discard would be deleted by its first reader.

Reported. A discarded error from a gated call in a place where the gate makes the
failure certain: a cleanup loop in Terminate whose every call returns ErrNotAllowed
does nothing at all, and the discard is what hides it. And a discarded CancelFunc
from SendEvery or SendWithPriorityEvery, which repeats until the process dies, so
without the cancel the ticker cannot be stopped for the process lifetime.

Not reported. A discarded CancelFunc from a one shot timer: it fires once and
accumulates nothing, so keeping it is optional. A discarded event buffer from
LinkEvent or MonitorEvent, whose severity belongs to A2013 and depends on whether
the producer set a buffer at all.

Both the blanked assignment and the bare expression statement are matched, since the
bare form is legal at every arity and is the more common way to write it.`,
	URL:      "https://docs.ergo.services/tools/argus#A2001a",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		surface := ergomodel.SurfaceOf(cb)
		certain := (cb.Kind == ergomodel.CBTerminate && cb.Meta == false) ||
			(cb.Kind == ergomodel.CBInit && cb.Meta)

		ast.Inspect(cb.Decl.Body, func(n ast.Node) bool {
			call, discarded := discardedCall(n)
			if discarded == false {
				return true
			}
			gate, ok := m.GatedCall(call, surface)
			if ok == false {
				return true
			}

			switch gate.Result {
			case ergomodel.ResultCancelPeriodic:
				m.Report(pass, call.Pos(),
					ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
						ID: ergomodel.CallbackID(cb) + ":" + gate.Method},
					"the CancelFunc from %s is discarded, so the ticker repeats for the whole process lifetime with no way to stop it (see A2006)",
					gate.Method)

			case ergomodel.ResultError:
				if certain == false || gate.Observable() == false {
					return true
				}
				if gate.ForbidTerminated == false && gate.ForbidMetaPreStart == false {
					return true
				}
				m.Report(pass, call.Pos(),
					ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
						ID: ergomodel.CallbackID(cb) + ":" + gate.Method},
					"the error from %s is discarded in %s, where the state makes the call fail every time, so this line does nothing",
					gate.Method, cb.Name)
			}
			return true
		})
	}
	return nil, nil
}

func discardedCall(n ast.Node) (*ast.CallExpr, bool) {
	switch x := n.(type) {
	case *ast.ExprStmt:
		call, ok := x.X.(*ast.CallExpr)
		return call, ok

	case *ast.AssignStmt:
		if len(x.Rhs) != 1 {
			return nil, false
		}
		call, ok := x.Rhs[0].(*ast.CallExpr)
		if ok == false {
			return nil, false
		}

		for _, lhs := range x.Lhs {
			id, isIdent := lhs.(*ast.Ident)
			if isIdent == false || id.Name != "_" {
				return nil, false
			}
		}
		return call, true
	}
	return nil, false
}
