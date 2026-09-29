package ergomodel_test

import (
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"ergo.tools/argus/ergomodel"
)

func testdata(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../testdata")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestModelIsConcurrencySafe(t *testing.T) {
	probe := &analysis.Analyzer{
		Name:     "probe",
		Doc:      "hammers the frozen model from many goroutines",
		Requires: []*analysis.Analyzer{ergomodel.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			m := ergomodel.From(pass)

			var payloads []types.Type
			for _, s := range m.SendSites {
				if tv := pass.TypesInfo.TypeOf(s.Payload); tv != nil {
					payloads = append(payloads, tv)
				}
			}
			if len(payloads) == 0 {
				t.Error("probe: no send sites collected, the fixture is not exercising the model")
			}

			const readers = 16
			var wg sync.WaitGroup
			for i := 0; i < readers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for _, tv := range payloads {
						_ = m.Shape(tv)
						_ = m.Guarded(tv)
						_, _ = m.AllowedType(tv)
					}
					for _, cb := range m.Callbacks {
						_ = m.Suppressed(cb.Decl.Pos(), "A1001")
					}
				}()
			}
			wg.Wait()
			return nil, nil
		},
	}
	analysistest.Run(t, testdata(t), probe, "probe")
}

func TestModelCollectsSurfaces(t *testing.T) {
	var callbacks, sends, metas int
	probe := &analysis.Analyzer{
		Name:     "surfaces",
		Doc:      "counts the classified surfaces",
		Requires: []*analysis.Analyzer{ergomodel.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			m := ergomodel.From(pass)
			callbacks += len(m.Callbacks)
			sends += len(m.SendSites)
			for _, cb := range m.Callbacks {
				if cb.Meta {
					metas++
				}
			}
			return nil, nil
		},
	}
	analysistest.Run(t, testdata(t), probe, "probe")

	if callbacks < 4 {
		t.Errorf("callbacks classified = %d, want at least 4 (Init, HandleMessage, HandleCall, Terminate)", callbacks)
	}
	if sends < 6 {
		t.Errorf("send sites classified = %d, want at least 6", sends)
	}
	if metas != 0 {
		t.Errorf("meta callbacks = %d, want 0 in this fixture", metas)
	}
}

func TestFactTypesAreComplete(t *testing.T) {
	declared := []analysis.Fact{
		(*ergomodel.ShapeFact)(nil),
		(*ergomodel.SenderFact)(nil),
		(*ergomodel.BlocksFact)(nil),
		(*ergomodel.SpawnsFact)(nil),
		(*ergomodel.UnrecoveredSpawnFact)(nil),
		(*ergomodel.RecoversFact)(nil),
		(*ergomodel.EscapesFact)(nil),
		(*ergomodel.ResultsFact)(nil),
		(*ergomodel.RepliesFact)(nil),
		(*ergomodel.FactoryFact)(nil),
		(*ergomodel.InitBudgetFact)(nil),
		(*ergomodel.InitSentinelFact)(nil),
		(*ergomodel.RoundTripFact)(nil),
		(*ergomodel.NodeSenderFact)(nil),
		(*ergomodel.FmtWrappedFact)(nil),
		(*ergomodel.ExternalRoundTripFact)(nil),
		(*ergomodel.ForksFact)(nil),
	}

	registered := map[reflect.Type]bool{}
	for _, f := range ergomodel.Analyzer.FactTypes {
		registered[reflect.TypeOf(f)] = true
	}
	for _, f := range declared {
		if registered[reflect.TypeOf(f)] == false {
			t.Errorf("%T is declared but missing from Analyzer.FactTypes", f)
		}
	}
	if len(ergomodel.Analyzer.FactTypes) != len(declared) {
		t.Errorf("FactTypes has %d entries, declared list has %d: keep them in step",
			len(ergomodel.Analyzer.FactTypes), len(declared))
	}
}

func TestAnalyzerIsValid(t *testing.T) {
	if err := analysis.Validate([]*analysis.Analyzer{ergomodel.Analyzer}); err != nil {
		t.Fatal(err)
	}
	if ergomodel.Analyzer.ResultType == nil {
		t.Fatal("the model analyzer must publish a ResultType, otherwise rules cannot consume it")
	}
}

func shapeProbe(t *testing.T, check func(t *testing.T, name string, s ergomodel.Shape)) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "shapes",
		Doc:      "reads the shape of every named type in the package",
		Requires: []*analysis.Analyzer{ergomodel.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			m := ergomodel.From(pass)
			scope := pass.Pkg.Scope()
			for _, name := range scope.Names() {
				tn, ok := scope.Lookup(name).(*types.TypeName)
				if ok == false {
					continue
				}
				check(t, name, m.Shape(tn.Type()))
			}
			return nil, nil
		},
	}
}

func TestShapesOfRecursiveTypes(t *testing.T) {
	want := map[string]struct{ aliasing, wire bool }{
		"Tree":   {aliasing: true, wire: false},
		"Linked": {aliasing: true, wire: true},
		"Chain":  {aliasing: true, wire: false},
		"Holder": {aliasing: true, wire: false},
		"Flat":   {aliasing: true, wire: false},
		"Plain":  {aliasing: false, wire: true},
	}
	seen := map[string]bool{}
	probe := shapeProbe(t, func(t *testing.T, name string, s ergomodel.Shape) {
		w, ok := want[name]
		if ok == false {
			return
		}
		seen[name] = true
		if s.Aliasing != w.aliasing {
			t.Errorf("%s: Aliasing = %v, want %v (witness %q)", name, s.Aliasing, w.aliasing, s.Witness)
		}
		if s.WireOK != w.wire {
			t.Errorf("%s: WireOK = %v, want %v (witness %q)", name, s.WireOK, w.wire, s.Witness)
		}
	})
	analysistest.Run(t, testdata(t), probe, "shapes")
	for name := range want {
		if seen[name] == false {
			t.Errorf("%s was never probed", name)
		}
	}
	if got, want := len(seen), len(want); got != want {
		t.Errorf("probed %d types, want %d", got, want)
	}
}

func TestShapesOfADenseCycleAreLinear(t *testing.T) {
	const n = 80
	var src strings.Builder
	src.WriteString("package dense\n\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&src, "type T%d struct {\n\tA *T%d\n\tB []T%d\n\tC map[string]*T%d\n\tD *T%d\n}\n\n",
			i, (i+1)%n, (i*7+3)%n, (i*13+5)%n, (i*29+11)%n)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src", "dense"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "dense", "dense.go"), []byte(src.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	probed := 0
	probe := shapeProbe(t, func(t *testing.T, name string, s ergomodel.Shape) {
		probed++
		if s.Aliasing == false || s.WireOK {
			t.Errorf("%s: Aliasing = %v, WireOK = %v, want a recursive shape (witness %q)",
				name, s.Aliasing, s.WireOK, s.Witness)
		}
	})
	start := time.Now()
	analysistest.Run(t, dir, probe, "dense")
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("shapes of %d mutually recursive types took %s; the computation must stay linear in the type graph", n, elapsed)
	}
	if probed != n {
		t.Errorf("probed %d types, want %d", probed, n)
	}
}

func TestDefaultConfigTiers(t *testing.T) {
	cfg := ergomodel.DefaultConfig()
	if got := cfg.Severity(1); got != ergomodel.SeverityError {
		t.Errorf("tier 1 severity = %v, want error", got)
	}
	if got := cfg.Severity(3); got != ergomodel.SeverityOff {
		t.Errorf("tier 3 severity = %v, want off by default", got)
	}
	if got := cfg.Severity(9); got != ergomodel.SeverityOff {
		t.Errorf("an unconfigured tier = %v, want off", got)
	}
	if _, ok := cfg.AllowTypes["[]byte"]; ok == false {
		t.Error("[]byte must be allowlisted by default, it is the documented payload exception")
	}
}
