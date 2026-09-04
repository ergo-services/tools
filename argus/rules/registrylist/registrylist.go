package registrylist

import (
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A3003"

var Analyzer = &analysis.Analyzer{
	Name: "argusA3003",
	Doc: `A3003: a message type declared here but missing from the registration list.

The rule compares two things that are visible in exactly one place together: the types a
package declares as messages, and the registration list that package writes. A type
that is sent and never registered works locally and fails the moment it crosses a node
boundary, which is the worst possible time to find out.

A package that registers nothing is silent rather than reporting everything. That is
the deliberate choice for a codebase with a centralized registration package: the rule
cannot see that list from here and guessing would make it useless.

A type marked //argus:message local is not supposed to be in the list, so it is not
reported: its author already said it does not cross the wire.`,
	URL:      "https://docs.ergo.services/tools/argus#A3003",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	registered := m.RegisteredNames()

	if len(registered) > 0 {
		for tn := range m.MessageTypes() {
			named, ok := tn.Type().(*types.Named)
			if ok == false {
				continue
			}

			if _, local := m.MessageMarked(tn); local {
				continue
			}
			path := ergomodel.TypeID(named)
			if registered[path] {
				continue
			}
			m.Report(pass, tn.Pos(),
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindRegistration, Tier: 3,
					ID: path,
				},
				"%s is used as a message here but is not in this package's registration list, so it works locally and fails the first time it crosses a node boundary; register it or mark it //argus:message local",
				tn.Name())
		}
	}

	return nil, nil
}
