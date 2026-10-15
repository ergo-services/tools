package ergomodel_test

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"ergo.tools/argus/ergomodel"
)

func TestExposureSkipsAcceptedParts(t *testing.T) {
	want := map[string]string{
		"RawThenItems":   "Items []int",
		"GuardThenItems": "Items []int",
		"OnlyAccepted":   "",
		"Nested":         "Inner exposure.RawThenItems -> Items []int",
		"GuardedValue":   "Cache exposure.Guarded -> data map[string]int",
		"Values":         "",
	}
	seen := map[string]bool{}
	probe := &analysis.Analyzer{
		Name:     "exposure",
		Doc:      "reads the exposed part of every named type in the package",
		Requires: []*analysis.Analyzer{ergomodel.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			m := ergomodel.From(pass)
			scope := pass.Pkg.Scope()
			for _, name := range scope.Names() {
				tn, ok := scope.Lookup(name).(*types.TypeName)
				if ok == false {
					continue
				}
				witness, tracked := want[name]
				if tracked == false {
					continue
				}
				seen[name] = true
				e, exposed := m.Exposed(tn.Type())
				if exposed != (witness != "") {
					t.Errorf("%s: exposed = %v, want %v (witness %q)", name, exposed, witness != "", e.Witness)
					continue
				}
				if e.Witness != witness {
					t.Errorf("%s: witness = %q, want %q", name, e.Witness, witness)
				}
			}
			return nil, nil
		},
	}
	analysistest.Run(t, testdata(t), probe, "exposure")
	for name := range want {
		if seen[name] == false {
			t.Errorf("%s was never probed", name)
		}
	}
}
