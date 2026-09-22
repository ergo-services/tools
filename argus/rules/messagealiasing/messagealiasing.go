package messagealiasing

import (
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A1001"

var Analyzer = &analysis.Analyzer{
	Name: "argusA1001",
	Doc: `A1001: a message must not share mutable memory with its sender.

Local delivery does not copy, so a payload carrying a slice, map, pointer, channel
or function hands the receiving actor a reference to memory the sender still owns.
The same value sent to a remote node is safe, because serialization copies it, so
the rule takes the worst case and requires the payload to be safe as if delivery
were local.

Sharing is not a defect by itself: handing the receiver a buffer built for it and
then forgetting it is a transfer of ownership, and it is what keeps an actor system
off the allocator. What makes it a defect is a second reference. So the type is only
the filter, and the verdict comes from provenance of the payload at this site, which
the model computes per send:

  transferred  allocated in this callback and unreachable from the actor afterwards.
               Nothing is shared, so there is no finding at all.
  received     the payload is memory that arrived in a message and is forwarded on.
               Ownership stays with whoever built it, and the question belongs at
               that sender, so this reports at tier 2.
  unknown      the payload came from a parameter this package cannot resolve, or from
               a call whose result origin is not known. Tier 2.
  owned        the payload is reachable from a field of the sending actor. Tier 1 when
               the actor writes into that field somewhere, which is the race; tier 2
               when it only ever replaces the whole value, where the receiver reads a
               value nobody mutates and only the type fails to say so.

The verdict is narrowed by guardedness of the referenced type: a pointee that
synchronizes its own state (a mutex field, all atomic fields, sync.Map) is
deliberately shared and is not reported. On a production corpus nine in ten shared
pointees were guarded, so without this gate the rule buries its own findings.

A payload that is, or carries, a field of the sending actor's own receiver belongs to
A1006 instead. Both rules ask the model the same question, so exactly one of them
reports the site: the hazard is identical and the fix is not, since there the sender is
the owner and copying at the send site papers over the sharing rather than ending it.

Fix by sending a value, by generating a copy method, or by recording the exception
with //argus:allow A1001 <reason>.

Source: actors.md`,
	URL:      "https://docs.ergo.services/tools/argus#A1001",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, site := range m.SendSites {
		t := pass.TypesInfo.TypeOf(site.Payload)
		if t == nil {
			continue
		}

		if types.IsInterface(types.Unalias(t)) {
			continue
		}
		if _, ok := m.AllowedType(t); ok {
			continue
		}

		if isInspectReply(site.Method, t) {
			continue
		}

		if _, own := m.OwnStateSend(site); own {
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

		if site.Origin.Prov == ergomodel.ProvTransferred {
			continue
		}

		witness := s.Witness
		if witness == "" {
			witness = typeName(t)
		}
		tier, clause := verdict(site.Origin)

		m.Report(pass, site.Payload.Pos(),
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindShape, Tier: tier,
				ID: ergomodel.TypeID(t), Witness: witness},
			"%s is sent as a message and shares memory with the sender: %s -> %s; local delivery does not copy, and %s",
			typeName(t), typeName(t), witness, clause)
	}
	return nil, nil
}

func verdict(origin ergomodel.Origin) (int, string) {
	switch origin.Prov {
	case ergomodel.ProvOwned:
		if origin.Mutated {
			return 1, "this payload reaches " + origin.Witness +
				": the receiver reads that memory while this actor goes on writing into it. " +
				"Send a value, or a copy the receiver owns"
		}
		return 2, "this payload reaches " + origin.Witness +
			": the actor never writes into that memory, only swaps the whole value, so the " +
			"sharing holds today and nothing in the type says it has to. Send a value, or a copy"

	case ergomodel.ProvReceived:
		return 2, "this payload is memory that " + origin.Witness +
			" and is forwarded on: ownership stayed with whoever built it, so the receiver " +
			"now shares with a third process this one cannot speak for"
	}
	return 2, "the origin of this payload does not resolve here (" + origin.Witness +
		"), so the sender may still hold it"
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

func isInspectReply(method string, t types.Type) bool {
	switch method {
	case "SendResponse", "SendResponseImportant":
	default:
		return false
	}
	return types.TypeString(types.Unalias(t), nil) == "map[string]string"
}
