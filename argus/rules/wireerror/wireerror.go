package wireerror

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2020"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2020",
	Doc: `A2020: an fmt.Errorf value with %w put on the wire, which drops the identity.

EDF preserves an error in exactly two ways. A *gen.Error is encoded field by field,
including its exported Wrapped slice, so every cause is carried and each of them
gets its own chance at the error cache. Any other error value is looked up in that
cache by identity; a hit becomes a two byte id, and a miss is flattened to the text
of Error().

fmt.Errorf keeps its %w operand in an unexported field. It is never in the cache,
because it is a fresh value, so it is always the second case: the peer decodes a
plain error carrying the right words and nothing else. errors.Is against the
sentinel answers false on the other side of the hop while the message still reads
correctly, which is why this survives review and testing and only shows up as a
caller silently falling through to a generic branch.

gen.Errorf is the drop-in replacement: same formatting, same %w, and the causes end
up somewhere EDF can reach them.

The surface is where the value reaches another process, not where it is built: the
payload of a framework send, the error argument of SendResponseError, the result
slot of a HandleCall, and an argument handed to a function whose own package
published a sender fact for that parameter, which is what a respond() wrapper is. A
value built and kept inside one process is not reported, since nothing encodes it.

A helper returning the value directly is followed through a fact, so
"return responseFor(err)" is reported at the send with the helper named.

Source: edf.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2020",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, site := range m.WireErrorSites() {
		if ergomodel.IsFmtErrorfWrap(pass.TypesInfo, site.Expr) {
			m.Report(pass, site.Pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindShape, Tier: 2,
					ID: m.DeclID(site.Decl) + ":" + where(site), Witness: "fmt.Errorf",
				},
				"this error is %s and it is built with fmt.Errorf(%%w), whose operand lives in an unexported field EDF cannot reach: the peer decodes the text and errors.Is stops matching the sentinel on the other side of the hop. Use gen.Errorf, which carries the causes in an exported field",
				site.What)
			continue
		}
		fn := calleeFunc(pass, site.Expr)
		if fn == nil || m.Behavior(fn).FmtWrapped == false {
			continue
		}
		m.Report(pass, site.Pos,
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindShape, Tier: 2,
				ID: m.DeclID(site.Decl) + ":" + fn.Name(), Witness: fn.Name(),
			},
			"this error is %s and %s builds it with fmt.Errorf(%%w), whose operand lives in an unexported field EDF cannot reach: the peer decodes the text and errors.Is stops matching the sentinel on the other side of the hop. Use gen.Errorf in %s",
			site.What, fn.Name(), fn.Name())
	}
	return nil, nil
}

func where(site *ergomodel.WireErrorSite) string {
	if site.Field != "" {
		return site.Field
	}
	return "error"
}

func calleeFunc(pass *analysis.Pass, e ast.Expr) *types.Func {
	call, ok := e.(*ast.CallExpr)
	if ok == false {
		return nil
	}
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		fn, _ := pass.TypesInfo.Uses[fun].(*types.Func)
		return fn
	case *ast.SelectorExpr:
		fn, _ := pass.TypesInfo.Uses[fun.Sel].(*types.Func)
		return fn
	}
	return nil
}
