package housethreshold

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A3007"

const compressionFloor = 1024

var Analyzer = &analysis.Analyzer{
	Name: "argusA3007",
	Doc: `A3007: a compression threshold below the framework floor.

ProcessOptions.Compression.Threshold below 1024 in a spawn literal is accepted and
applied verbatim, so messages smaller than a packet get compressed and the CPU spent is
larger than the bytes saved. It is a performance note, not an error, which is why it is
tier 3.

Note the asymmetry with the setter, because it changes whose rule it is:
SetCompressionThreshold rejects a value below the floor with ErrIncorrect, so there the
value is never applied at all and the defect is the discarded error, which belongs to
A2001a.

The rule's other half from the design, a Call with no timeout to a pool or router where
a worker may be unavailable, is not shipped: it needs a name index mapping a target atom
to the behavior that registered it, which is the same capability A2010's pool half waits
on.`,
	URL:      "https://docs.ergo.services/tools/argus#A3007",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			kv, ok := n.(*ast.KeyValueExpr)
			if ok == false {
				return true
			}
			key, ok := kv.Key.(*ast.Ident)
			if ok == false || key.Name != "Threshold" {
				return true
			}

			owner := pass.TypesInfo.Uses[key]
			if owner == nil || owner.Pkg() == nil || owner.Pkg().Path() != "ergo.services/ergo/gen" {
				return true
			}
			tv, ok := pass.TypesInfo.Types[kv.Value]
			if ok == false || tv.Value == nil {
				return true
			}
			value, exact := constantInt(tv.Value.String())
			if exact == false || value >= compressionFloor {
				return true
			}
			m.Report(pass, kv.Value.Pos(),
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindSpec, Tier: 3,
					ID: m.DeclID(enclosing(file, kv.Pos())) + ":Compression.Threshold",
				},
				"a compression threshold of %d is below the framework floor of %d and a spawn literal applies it verbatim, so messages smaller than a packet get compressed and the CPU costs more than the bytes saved",
				value, compressionFloor)
			return true
		})
	}
	return nil, nil
}

func constantInt(s string) (int64, bool) {
	var v int64
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg, s = true, s[1:]
	}
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		v = v*10 + int64(r-'0')
	}
	if neg {
		v = -v
	}
	return v, true
}

func enclosing(file *ast.File, pos token.Pos) *ast.FuncDecl {
	var found *ast.FuncDecl
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if ok == false || fn.Body == nil {
			return true
		}
		if pos >= fn.Body.Pos() && pos <= fn.Body.End() {
			found = fn
		}
		return true
	})
	return found
}
