package spawnargs

import (
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2011"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2011",
	Doc: `A2011: a spawn argument that shares state with the parent.

Spawn arguments are handed to the child by reference, so an argument struct carrying
a repository, a store, a hub or a bare map gives parent and child the same object
with no synchronization between two goroutines that are supposed to be isolated. This
is where dependency-style sharing concentrates in real code, which is why it owns the
spawn surface on its own: A1001 and A1006 cover the send surface and stay silent here.

The same guardedness test applies: a pointee that synchronizes its own state is
deliberately shared and is not reported. A spawn in a test file is not reported
either: the child is a mock and the arguments are fixture data, and a real sharing
defect shows up at the production spawn of the same factory.

A func typed argument is called out separately. It cannot be serialized, so it also
pins the child to this node: a remote spawn of the same factory will fail.`,
	URL:      "https://docs.ergo.services/tools/argus#A2011",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, site := range m.SpawnSites() {

		if strings.HasSuffix(pass.Fset.Position(site.Call.Pos()).Filename, "_test.go") {
			continue
		}
		for _, arg := range site.Args {
			t := pass.TypesInfo.TypeOf(arg)
			if t == nil {
				continue
			}

			if types.IsInterface(types.Unalias(t)) {
				continue
			}
			if _, ok := m.AllowedType(t); ok {
				continue
			}

			s := m.Shape(t)
			if s.Aliasing == false {
				continue
			}
			ref := s.Ref
			if ref == nil {
				ref = t
			}
			if _, ok := m.AllowedType(ref); ok {
				continue
			}
			if m.Guarded(ref) == ergomodel.GuardGuarded {
				continue
			}

			if _, isFunc := types.Unalias(ref).(*types.Signature); isFunc {
				m.Report(pass, arg.Pos(),
					ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindShape, Tier: 2,
						ID: ergomodel.TypeID(t), Witness: s.Witness},
					"%s is passed to %s and carries a func (%s), which cannot be serialized: the child is pinned to this node and a remote spawn of the same factory fails",
					typeName(t), site.Method, s.Witness)
				continue
			}

			m.Report(pass, arg.Pos(),
				ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindShape, Tier: 2,
					ID: ergomodel.TypeID(t), Witness: s.Witness},
				"%s is passed to %s and shares unsynchronized memory with the parent: %s; the child runs on its own goroutine, so pass a copy or a type that guards itself",
				typeName(t), site.Method, witness(s, t))
		}
	}
	return nil, nil
}

func witness(s ergomodel.Shape, t types.Type) string {
	if s.Witness != "" {
		return s.Witness
	}
	return typeName(t)
}

func typeName(t types.Type) string {
	if named, ok := types.Unalias(t).(*types.Named); ok {
		obj := named.Obj()
		if obj.Pkg() != nil {
			return obj.Pkg().Name() + "." + obj.Name()
		}
		return obj.Name()
	}
	return types.TypeString(types.Unalias(t), func(p *types.Package) string { return p.Name() })
}
