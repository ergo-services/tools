package wiretypeclosure

import (
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2021"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2021",
	Doc: `A2021: a type reachable from a registered one that nobody registers.

Registration is eager, not lazy. Registering a struct walks its wire visible fields
and asks for an encoder for each one; a named type the registry has never seen
answers "no encoder for type X" and the whole registration fails. A named string is
a distinct type from string, so an enum needs its own entry exactly as a struct
does, and the framework registers even its own gen.LogLevel and gen.ProcessKind by
hand for that reason.

What makes the omission expensive is where it surfaces. The list usually still
registers cleanly on the node that declares it, because a sibling list happens to
carry the missing type; the node that does not have that sibling panics at start,
or resolves nothing and never completes a handshake. Both are far from the edit
that caused them.

The rule compares two things that are visible together in exactly one place: the
types this package hands to registration, and the types reachable through their
fields. An entry cut with edf:"-", a type with a marshaler pair, the framework's
own value types and anything on the configured allowlist are all resolved without a
registration of their own and are not reported.

A dependency's type is named but not descended into: its own fields are its own
package's business, and the fix here is one line in this list either way.

The rule is silent for a package that registers nothing, which is the same choice
A3003 makes: a centralized registration package is invisible from here and guessing
would make the rule useless.

Source: edf.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2021",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	registered := m.RegisteredTypes()
	if len(registered) == 0 {
		return nil, nil
	}

	reported := map[string]bool{}
	for _, entry := range registered {
		for _, missing := range m.WireReachable(entry.Named) {
			if m.Registered(missing) {
				continue
			}
			if _, allowed := m.AllowedType(missing); allowed {
				continue
			}
			key := ergomodel.TypeID(entry.Named) + "->" + ergomodel.TypeID(missing)
			if reported[key] {
				continue
			}
			reported[key] = true

			m.Report(pass, entry.Pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindRegistration, Tier: 2,
					ID: key, Witness: ergomodel.TypeID(missing),
				},
				"%s is registered here but %s, which registration reaches through its fields, is not: registration resolves the field graph eagerly and answers \"no encoder for type %s\", so the node that does not get this type from some other list refuses to start. Add %s to the same list",
				name(entry.Named), name(missing), types.TypeString(missing, nil), name(missing))
		}
	}
	return nil, nil
}

func name(n *types.Named) string {
	obj := n.Obj()
	if obj == nil {
		return "this type"
	}
	if obj.Pkg() != nil {
		return obj.Pkg().Name() + "." + obj.Name()
	}
	return obj.Name()
}
