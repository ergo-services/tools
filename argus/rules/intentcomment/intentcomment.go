package intentcomment

import (
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A3006"

var Analyzer = &analysis.Analyzer{
	Name: "argusA3006",
	Doc: `A3006: prose that should be a marker.

Real code already writes scope intent in prose. A comment saying a type is local, same
node only, or never sent over the wire is exactly what //argus:message local records,
and converting it is mechanical. Until it is converted the tool cannot act on it, so
A2004 keeps asking about a wire shape the author already decided is not a wire type.

This is the migration path, and the other end of it is A3001.`,
	URL:      "https://docs.ergo.services/tools/argus#A3006",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

var localPhrases = []string{
	"same node only",
	"same node",
	"local only",
	"never sent over the wire",
	"not sent over the wire",
	"does not cross the wire",
	"in-process only",
	"in process only",
	"node local",
	"node-local",
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)
	scope := pass.Pkg.Scope()

	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if ok == false {
			continue
		}
		if marked, _ := m.MessageMarked(tn); marked {
			continue
		}
		doc := strings.ToLower(m.DocOf(tn))
		if doc == "" {
			continue
		}
		phrase := ""
		for _, p := range localPhrases {
			if strings.Contains(doc, p) {
				phrase = p
				break
			}
		}
		if phrase == "" {
			continue
		}
		m.Report(pass, tn.Pos(),
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindShape, Tier: 3,
				ID: pass.Pkg.Path() + "." + name, Witness: phrase,
			},
			"the doc comment on %s already says %q, which is what //argus:message local records; adding the marker turns the prose into something the tool can act on",
			name, phrase)
	}
	return nil, nil
}
