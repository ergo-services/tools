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

The same guardedness test applies, to each part of the argument on its own: a
pointee that synchronizes its own state is deliberately shared and is not reported,
and neither is an allowlisted type, while the rest of the argument still counts. An
interface field is never exempt, because the object behind it is whatever the parent
put there. The finding names the field that is shared. The tier follows A1001: a
parent that writes into the memory the argument holds is a race with the child, and
a write made only in Init is over before a spawn from any other callback. A fresh
container of copies taken from the parent's state shares what the copies reference,
not the container they were copied out of.

A spawn in a test file is not reported either: the child is a mock and the arguments
are fixture data, and a real sharing defect shows up at the production spawn of the
same factory.

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
		for i, arg := range site.Args {
			t := pass.TypesInfo.TypeOf(arg)
			if t == nil {
				continue
			}
			origin := ergomodel.Origin{Prov: ergomodel.ProvUnknown, Witness: "not resolved"}
			if i < len(site.Origins) {
				origin = site.Origins[i]
			}

			if types.IsInterface(types.Unalias(t)) {
				continue
			}

			exposure, exposed := m.Exposed(t)
			if exposed == false {
				continue
			}

			if _, isFunc := types.Unalias(exposure.Ref).(*types.Signature); isFunc {
				m.Report(pass, arg.Pos(),
					ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindShape, Tier: 2,
						ID: ergomodel.TypeID(t), Witness: exposure.Witness},
					"%s is passed to %s and carries a func (%s), which cannot be serialized: the child is pinned to this node and a remote spawn of the same factory fails",
					typeName(t), site.Method, exposure.Witness)
				continue
			}

			if origin.Prov == ergomodel.ProvTransferred {
				continue
			}

			witness := m.WitnessVia(t, origin.Via)
			if witness == "" {
				witness = typeName(t)
			}
			tier, clause := verdict(origin)
			m.Report(pass, arg.Pos(),
				ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindShape, Tier: tier,
					ID: ergomodel.TypeID(t), Witness: exposure.Witness},
				"%s is passed to %s and shares unsynchronized memory with the parent: %s; the child runs on its own goroutine, and %s",
				typeName(t), site.Method, witness, clause)
		}
	}
	return nil, nil
}

func verdict(origin ergomodel.Origin) (int, string) {
	switch origin.Prov {
	case ergomodel.ProvOwned:
		if origin.Mutated {
			return 1, "this argument reaches " + origin.Witness +
				": the child reads that memory while the parent goes on writing into it. " +
				"Pass a copy, or a type that guards itself"
		}
		return 2, "this argument reaches " + origin.Witness +
			": the parent never writes into that memory, only swaps the whole value, so the " +
			"sharing holds today and nothing in the type says it has to"

	case ergomodel.ProvReceived:
		return 2, "this argument is memory that " + origin.Witness +
			": ownership stayed with whoever built it, so the child now shares with a third " +
			"process this one cannot speak for"
	}
	return 2, "the origin of this argument does not resolve here (" + origin.Witness +
		"), so the parent may still hold it"
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
