package ergomodel

import (
	"reflect"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name:       "argusmodel",
	Doc:        "builds the Ergo surface model: callbacks, send sites, message shapes",
	URL:        "https://docs.ergo.services/tools/argus#model",
	Requires:   []*analysis.Analyzer{inspect.Analyzer},
	FactTypes:  factTypes,
	ResultType: reflect.TypeFor[*Model](),
	Run:        run,
}

func init() { BindFlags(&Analyzer.Flags) }

func run(pass *analysis.Pass) (any, error) {
	anchor := ""
	if len(pass.Files) > 0 {
		anchor = pass.Fset.Position(pass.Files[0].Pos()).Filename
	}
	cfg, err := LoadConfig(anchor)
	if err != nil {
		return nil, err
	}

	b, err := loadBaseline(resolveBaselinePath(cfg))
	if err != nil {
		return nil, err
	}

	m := newModel(pass, cfg)
	m.baseline = b
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	m.build(insp)
	m.precompute()
	m.freeze()

	return m, nil
}

func From(pass *analysis.Pass) *Model {
	return pass.ResultOf[Analyzer].(*Model)
}
