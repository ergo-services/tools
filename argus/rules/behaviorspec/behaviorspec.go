package behaviorspec

import (
	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2025"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2025",
	Doc: `A2025: router or pool options the framework rejects at init.

A2005 does this for a supervisor spec. A router and a pool are configured the same
way, validated the same way and fail the same way, and nothing was checking them.

Router. ProcessInit walks RouterOptions.Routes and refuses a route with an empty
Name, a nil Factory, or a name already used by an earlier route. Each of those
returns an error out of Init, so the router never starts and everything routed
through it goes with it.

Pool. ProcessInit spawns PoolSize workers from WorkerFactory before it returns, and
a nil factory makes the spawn answer ErrIncorrect, so the pool fails to start. A
zero PoolSize is not a defect: it normalizes to three.

The spec is resolved rather than pattern matched, so a literal and a sequence of
field assignments are the same input, exactly as in A2005. An options value that
escapes into a call this pass cannot resolve stays silent instead of being guessed
at.

Source: router.md, pool.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2025",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, spec := range m.BehaviorSpecs() {
		if spec.Resolved == false {
			continue
		}
		id := m.DeclID(spec.Decl)

		if spec.Kind == "pool" {
			factory := spec.Field("WorkerFactory")
			if factory.Set && factory.IsNil == false {
				continue
			}
			at := spec.Pos
			if factory.Set {
				at = factory.Pos
			}
			m.Report(pass, at,
				finding(id, "WorkerFactory"),
				"PoolOptions has no WorkerFactory, and the pool spawns its workers from it before Init returns, so the spawn answers gen.ErrIncorrect and the pool never starts")
			continue
		}

		if spec.ChildrenUnresolved {
			continue
		}
		seen := map[string]bool{}
		for i, route := range spec.Children {
			at := spec.ChildrenPos[i]

			name := route["Name"]
			switch {
			case name.Set == false:
				m.Report(pass, at, finding(id, "Name"),
					"route %d has no Name, which router init rejects", i)
			case name.HasStr && name.Str == "":
				m.Report(pass, name.Pos, finding(id, "Name"),
					"route %d has an empty Name, which router init rejects", i)
			case name.HasStr:
				if seen[name.Str] {
					m.Report(pass, name.Pos, finding(id, "Name"),
						"route name %q is already used by an earlier route, which router init rejects",
						name.Str)
				}
				seen[name.Str] = true
			}

			factory := route["Factory"]
			switch {
			case factory.Set == false:
				m.Report(pass, at, finding(id, "Factory"),
					"route %d has no Factory, which router init rejects", i)
			case factory.IsNil:
				m.Report(pass, factory.Pos, finding(id, "Factory"),
					"route %d has a nil Factory, which router init rejects", i)
			}
		}
	}
	return nil, nil
}

func finding(id, check string) ergomodel.Finding {
	return ergomodel.Finding{
		Rule: ruleID, Kind: ergomodel.KindSpec, Tier: 2,
		ID: id + ":" + check, Witness: check,
	}
}
