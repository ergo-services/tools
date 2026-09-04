package initsentinel

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2017"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2017",
	Doc: `A2017: Init returning a framework sentinel as control flow.

Init's error is the spawn failure channel, not a termination reason. Returning
gen.TerminateReasonNormal from Init to mean "another process already owns this name"
invents a convention, and then every caller has to know it: the spawn reports an error,
the error is a sentinel that means success, and whoever forgets to decode it logs a
failure that did not happen.

On a production corpus twelve Init implementations did this, and two sibling supervisors
disagreed about decoding it, so one of them produced error logs and inflated failure
counters on an ordinary duplicate spawn.

Two halves. At the declaration: an Init whose error result is a framework sentinel.
At the spawn site: a factory whose Init signals through a sentinel, spawned by code that
never mentions that sentinel, which is the caller that will misread a normal outcome as
a failure. The second half is why the verdict travels on the factory object.

The fix is to make the outcome explicit rather than encoded: let Init succeed and have
the process decide what to do, or return a distinct error type the caller matches on
deliberately.`,
	URL:      "https://docs.ergo.services/tools/argus#A2017",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		if cb.Kind != ergomodel.CBInit || cb.Meta {
			continue
		}
		sentinel, pos := returnedSentinel(pass, cb.Decl)
		if sentinel == "" {
			continue
		}
		m.Report(pass, pos,
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
				ID: ergomodel.CallbackID(cb), Witness: sentinel,
			},
			"Init returns %s as its error, which is the spawn failure channel rather than a termination reason, so every caller has to know that this particular error means success",
			sentinel)
	}

	for _, point := range m.SpawnPoints() {
		if point.Factory == nil || point.In == nil {
			continue
		}
		obj, info, ok := m.FactoryOf(point.Factory)
		if ok == false || info.Sentinel == "" {
			continue
		}

		if m.Factories()[obj].Sentinel != "" {
			continue
		}
		if mentions(pass, point.In, info.Sentinel) {
			continue
		}
		m.Report(pass, point.Pos,
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindSpec, Tier: 2,
				ID:      m.DeclID(point.In) + ":" + obj.Name(),
				Witness: info.Sentinel,
			},
			"%s signals a normal outcome by returning %s from Init, and this %s never mentions it, so an ordinary outcome is reported here as a spawn failure",
			obj.Name(), info.Sentinel, point.What)
	}
	return nil, nil
}

func returnedSentinel(pass *analysis.Pass, decl *ast.FuncDecl) (string, token.Pos) {
	name, pos := "", decl.Pos()
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if name != "" {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if ok == false || len(ret.Results) != 1 {
			return true
		}
		if s := sentinelName(pass, ret.Results[0]); s != "" {
			name, pos = s, ret.Pos()
			return false
		}
		return true
	})
	return name, pos
}

func sentinelName(pass *analysis.Pass, e ast.Expr) string {
	sel, ok := e.(*ast.SelectorExpr)
	if ok == false {
		return ""
	}
	obj := pass.TypesInfo.Uses[sel.Sel]
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "ergo.services/ergo/gen" {
		return ""
	}

	if strings.HasPrefix(sel.Sel.Name, "TerminateReason") {
		return "gen." + sel.Sel.Name
	}
	return ""
}

func mentions(pass *analysis.Pass, decl *ast.FuncDecl, sentinel string) bool {
	want := strings.TrimPrefix(sentinel, "gen.")
	found := false
	ast.Inspect(decl, func(n ast.Node) bool {
		if found {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if ok == false || sel.Sel.Name != want {
			return true
		}
		obj := pass.TypesInfo.Uses[sel.Sel]
		if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "ergo.services/ergo/gen" {
			found = true
		}
		return true
	})
	return found
}
