package metastart

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2027"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2027",
	Doc: `A2027: a meta Start that returns immediately, which ends the meta.

Start is the meta's run loop, not a setup hook. The framework spawns the mailbox
goroutine and then calls Start on its own; the moment Start returns, the meta is
marked terminated, its alias is deleted from the node and Terminate is invoked. So
a Start that does not block is not "a meta with no loop": it is a meta that is gone
before its parent can send it anything, and the alias SpawnMeta just handed back is
already dead.

What it looks like afterwards is a Send answering gen.ErrMetaUnknown, or nothing at
all, from a process that was constructed successfully and logged nothing.

Reported only for the shape that cannot possibly block: a body with no loop, no
select, no channel operation and no call other than logging. Anything that calls
out is left alone, because the callee may be exactly the blocking client this meta
exists to host, and answering that question needs the callee's own package.

The idiom this rule protects is the framework's own: block on a channel that
Terminate closes, so the mailbox side can stop the loop.

Source: meta.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2027",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		if cb.Meta == false || cb.Kind != ergomodel.CBMetaStart {
			continue
		}
		if cb.Decl.Body == nil || canBlock(pass, m, cb.Decl.Body) {
			continue
		}
		m.Report(pass, cb.Decl.Name.Pos(),
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
				ID: ergomodel.CallbackID(cb), Witness: "returns immediately",
			},
			"Start returns without blocking, and a meta ends when Start returns: the framework deletes the alias and calls Terminate straight away, so the parent holds an alias that is already gone and every Send to it answers gen.ErrMetaUnknown. Block until the meta should stop, on a channel Terminate closes")
	}
	return nil, nil
}

func canBlock(pass *analysis.Pass, m *ergomodel.Model, body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.ForStmt, *ast.RangeStmt, *ast.SelectStmt, *ast.SendStmt:
			found = true
		case *ast.UnaryExpr:
			if x.Op.String() == "<-" {
				found = true
			}
		case *ast.CallExpr:
			if isLog(pass, x) == false {
				found = true
			}
		}
		return true
	})
	return found
}

func isLog(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false {
		return false
	}
	inner, isCall := sel.X.(*ast.CallExpr)
	if isCall == false {
		return false
	}
	logSel, isSel := inner.Fun.(*ast.SelectorExpr)
	return isSel && logSel.Sel.Name == "Log"
}
