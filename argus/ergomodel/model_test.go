package ergomodel_test

import (
	"go/types"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

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
		(*ergomodel.RepliesFact)(nil),
		(*ergomodel.FactoryFact)(nil),
		(*ergomodel.InitBudgetFact)(nil),
		(*ergomodel.InitSentinelFact)(nil),
		(*ergomodel.RoundTripFact)(nil),
		(*ergomodel.NodeSenderFact)(nil),
		(*ergomodel.FmtWrappedFact)(nil),
		(*ergomodel.ExternalRoundTripFact)(nil),
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
