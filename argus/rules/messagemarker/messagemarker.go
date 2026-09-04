package messagemarker

import (
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A3001"

var Analyzer = &analysis.Analyzer{
	Name: "argusA3001",
	Doc: `A3001: a message type with no marker.

A1001 and A2004 have real coverage at a type's declaration, but only for a type they
know is a message. Marking one with //argus:message moves it into definition site
checking, where the verdict is computed once for the type rather than rediscovered at
every send.

The rule looks at send sites and reports the declaration, which is deliberate: "a type
only ever used as a send payload" is a whole program quantifier and not answerable,
while "this expression is a payload here" is a fact. Only a type declared in the
analyzed package is reported, because only there can the marker be added.

It is tier 3 and off by default. Its job is to keep tier 1 coverage from decaying as
new message types appear. A3006 is the other end of the same migration path.`,
	URL:      "https://docs.ergo.services/tools/argus#A3001",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	reported := map[*types.TypeName]bool{}
	for _, site := range m.SendSites {
		t := pass.TypesInfo.TypeOf(site.Payload)
		if t == nil {
			continue
		}
		named, ok := namedOf(t)
		if ok == false {
			continue
		}
		tn := named.Obj()

		if tn == nil || tn.Pkg() != pass.Pkg || reported[tn] {
			continue
		}
		if marked, _ := m.MessageMarked(tn); marked {
			continue
		}
		if _, allowed := m.AllowedType(named); allowed {
			continue
		}
		reported[tn] = true
		m.Report(pass, tn.Pos(),
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindShape, Tier: 3,
				ID: ergomodel.TypeID(named),
			},
			"%s is sent as a message but carries no //argus:message marker, so it is checked only where it is sent rather than once at its declaration",
			tn.Name())
	}
	return nil, nil
}

func namedOf(t types.Type) (*types.Named, bool) {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	named, ok := t.(*types.Named)
	return named, ok
}
