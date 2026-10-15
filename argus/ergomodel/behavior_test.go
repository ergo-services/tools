package ergomodel_test

import (
	"go/types"
	"reflect"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"ergo.tools/argus/ergomodel"
)

func TestEscapesIgnoreReturnsInLiterals(t *testing.T) {
	got := map[string][]ergomodel.Escape{}
	probe := &analysis.Analyzer{
		Name:     "escapes",
		Doc:      "reads the escapes the model computes for each method",
		Requires: []*analysis.Analyzer{ergomodel.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			m := ergomodel.From(pass)
			cache := pass.Pkg.Scope().Lookup("Cache").Type().(*types.Named)
			for i := 0; i < cache.NumMethods(); i++ {
				fn := cache.Method(i)
				got[fn.Name()] = m.Behavior(fn).Escapes
			}
			return nil, nil
		},
	}
	analysistest.Run(t, testdata(t), probe, "escapes")

	want := map[string][]ergomodel.Escape{
		"Items":  {{Result: 0, Field: "items"}},
		"Count":  nil,
		"Lookup": nil,
		"Visit":  {{Result: 0, Field: "index"}},
	}
	for name, escapes := range want {
		if reflect.DeepEqual(got[name], escapes) == false {
			t.Errorf("%s: escapes = %+v, want %+v", name, got[name], escapes)
		}
	}
}
