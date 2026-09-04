package callbackgoroutine

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A1004"

var Analyzer = &analysis.Analyzer{
	Name: "argusA1004",
	Doc: `A1004: a goroutine started in a callback must not touch actor state.

An actor's fields need no locking precisely because one goroutine touches them. A
goroutine that closes over the receiver breaks that premise, and the resulting race
is silent until production because the race detector only finds it if a test happens
to interleave the two.

Note that the framework's own process handle is actor-goroutine only, so reading
p.Log() or calling p.Send() from the goroutine is reported too, not exempted: hoist
the handle, PID and logger into locals in the callback and reference only those.

A goroutine with no recover boundary is a separate finding (A1010) on the same
construct, because a race free goroutine can still take the whole node down.`,
	URL:      "https://docs.ergo.services/tools/argus#A1004",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {

		if cb.Kind == ergomodel.CBMetaStart && cb.Meta {
			continue
		}
		if cb.Recv == "" {
			continue
		}
		ast.Inspect(cb.Decl.Body, func(n ast.Node) bool {
			g, ok := n.(*ast.GoStmt)
			if ok == false {
				return true
			}

			if m.Suppressed(g.Pos(), ruleID) {
				return true
			}
			if use, pos := receiverUse(g, cb.Recv); use != "" {
				m.Report(pass, pos,
					ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 1,
						ID: ergomodel.CallbackID(cb), Witness: use},
					"goroutine started in %s captures actor state (%s); actor fields are unsynchronized by design, so pass the data in by value",
					cb.Name, use)
			}
			return true
		})
	}
	return nil, nil
}

func receiverUse(g *ast.GoStmt, recv string) (string, token.Pos) {
	found := ""
	at := g.Pos()
	ast.Inspect(g, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if ok == false {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if ok == false || id.Name != recv {
			return true
		}
		found = recv + "." + sel.Sel.Name
		at = sel.Pos()
		return false
	})
	if found != "" {
		return found, at
	}

	ast.Inspect(g, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		id, ok := n.(*ast.Ident)
		if ok && id.Name == recv {
			found = recv
			at = id.Pos()
			return false
		}
		return true
	})
	return found, at
}
