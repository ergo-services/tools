package wireshape

import (
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2004"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2004",
	Doc: `A2004: a message type that cannot be serialized.

EDF registration resolves every field eagerly and refuses a shape it cannot encode,
so the failure is a node startup error or a dropped frame between two nodes rather
than a compile error. This rule turns it into compile time feedback.

Rejected shapes: an unexported field without an edf:"-" tag; a named pointer type or
a pointer to a pointer; a chan or func anywhere in the wire visible field graph; an
interface typed field other than any or error; a cycle in that graph; uintptr or
unsafe.Pointer. A type implementing the marshaler pair short circuits all of this.

Axis W means "would registration accept this shape", so a type nobody registers
cannot fail. The rule therefore fires on a type that is registered in this package
or carries an //argus:message marker. Mark a type that never crosses the wire with
//argus:message local and it is exempt.

Source: edf.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2004",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)
	scope := pass.Pkg.Scope()

	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if ok == false {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if ok == false {
			continue
		}
		if named.TypeParams() != nil && named.TypeParams().Len() > 0 {
			continue
		}

		marked, local := m.MessageMarked(tn)
		if local {
			continue
		}
		if marked == false && m.Registered(named) == false {
			continue
		}

		s := m.Shape(named)
		if s.WireOK {
			continue
		}
		m.Report(pass, tn.Pos(),
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindShape, Tier: 2,
				ID: ergomodel.TypeID(named), Witness: s.Witness},
			"%s is used as a wire message but registration rejects its shape: %s; fix the field or mark it //argus:message local",
			name, witness(s, named))
	}
	return nil, nil
}

func witness(s ergomodel.Shape, named *types.Named) string {
	if s.Witness != "" {
		return s.Witness
	}
	return types.TypeString(named, func(p *types.Package) string { return p.Name() })
}
