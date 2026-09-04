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

		witness := s.Witness
		if witness == "" {
			witness = typeName(t)
		}
		m.Report(pass, site.Payload.Pos(),
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindShape, Tier: 1,
				ID: ergomodel.TypeID(t), Witness: witness},
			"%s is sent as a message and shares memory with the sender: %s -> %s; local delivery does not copy",
			typeName(t), typeName(t), witness)
	}
	return nil, nil
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
