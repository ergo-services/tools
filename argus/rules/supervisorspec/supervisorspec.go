package supervisorspec

import (
	"go/token"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2005"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2005",
	Doc: `A2005: a supervisor spec the framework rejects at init.

Every one of these makes ProcessInit return an error, so the supervisor never starts
and its whole subtree never starts with it. They are all decidable from the resolved
spec, which is why the rule exists: the failure is deterministic and the message the
runtime prints does not say which child spec produced it.

Rejected outright:
  - an empty Children list, for every supervisor type including SimpleOneForOne,
    which reads Children as its template list
  - a child with an empty Name or a nil Factory
  - a per child Restart.Intensity with AllForOne or RestForOne
  - Restart.Period without Restart.Intensity, per child
  - Restart.OnExceed set to Disable without Restart.Intensity
  - Options.PreserveMailbox with AllForOne or RestForOne
  - duplicate child names
  - an unknown supervisor type or restart strategy

Reported as information, because the value is silently ignored rather than rejected:
Significant on a SimpleOneForOne spec or under the Permanent strategy, and KeepOrder
on OneForOne or SimpleOneForOne.

A zero Intensity or Period at supervisor level is not a defect: both normalize to 5.
Per child, a zero Intensity selects the supervisor's shared counter and is not
normalized, which is why a Period alone is rejected.

Source: supervision.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2005",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

const (
	typeOneForOne       = 0
	typeAllForOne       = 1
	typeRestForOne      = 2
	typeSimpleOneForOne = 3

	strategyInherit   = 0
	strategyPermanent = 3

	onExceedDisable = 1
)

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, spec := range m.Specs() {
		if spec.Resolved == false {
			continue
		}

		specID := m.DeclID(spec.Decl)
		supType := spec.Field("Type")
		checkEnum(pass, m, specID, supType, spec.Pos, "supervisor type")
		checkEnum(pass, m, specID, spec.Field("Restart.Strategy"), spec.Pos, "supervisor restart strategy")

		allOrRest := supType.Int == typeAllForOne || supType.Int == typeRestForOne

		if keep := spec.Field("Restart.KeepOrder"); keep.Bool {
			switch supType.Int {
			case typeOneForOne, typeSimpleOneForOne:
				m.Report(pass, keep.Pos,
					specFinding(specID, "KeepOrder", 3),
					"Restart.KeepOrder is ignored for %s", typeName(supType.Int))
			}
		}

		if spec.ChildrenUnresolved {
			continue
		}

		if len(spec.Children) == 0 {
			m.Report(pass, spec.Pos,
				specFinding(specID, "children-empty", 2),
				"the Children list is empty, which every supervisor type rejects at init, including SimpleOneForOne, which reads Children as its template list")
			continue
		}

		seen := map[string]token.Pos{}
		for i, child := range spec.Children {
			at := spec.ChildrenPos[i]
			field := func(path string) ergomodel.FieldValue { return child[path] }

			name := field("Name")
			switch {
			case name.Set == false:
				m.Report(pass, at,
					specFinding(specID, "child", 2), "child spec %d has no Name, which init rejects", i)
			case name.HasStr && name.Str == "":
				m.Report(pass, name.Pos,
					specFinding(specID, "Name", 2), "child spec %d has an empty Name, which init rejects", i)
			case name.HasStr:
				if prev, dup := seen[name.Str]; dup {
					_ = prev
					m.Report(pass, name.Pos,
						specFinding(specID, "Name", 2),
						"child name %q is already used by an earlier child spec, which init rejects with ErrSupervisorChildDuplicate",
						name.Str)
				}
				seen[name.Str] = name.Pos
			}

			factory := field("Factory")
			if factory.Set == false {
				m.Report(pass, at,
					specFinding(specID, "child", 2), "child spec %d has no Factory, which init rejects", i)
			} else if factory.IsNil {
				m.Report(pass, factory.Pos,
					specFinding(specID, "Factory", 2), "child spec %d has a nil Factory, which init rejects", i)
			}

			checkEnum(pass, m, specID, field("Restart.Strategy"), at, "child restart strategy")

			intensity := field("Restart.Intensity")
			period := field("Restart.Period")
			onExceed := field("Restart.OnExceed")

			if intensity.Int == 0 && period.Int > 0 {
				m.Report(pass, period.Pos,
					specFinding(specID, "Restart.Period", 2),
					"Restart.Period requires Restart.Intensity > 0 on the same child spec, which init rejects")
			}
			if onExceed.Int == onExceedDisable && intensity.Int == 0 {
				m.Report(pass, onExceed.Pos,
					specFinding(specID, "Restart.OnExceed", 2),
					"Restart.OnExceed=Disable requires Restart.Intensity > 0, which init rejects")
			}
			if intensity.Int > 0 && allOrRest {
				m.Report(pass, intensity.Pos,
					specFinding(specID, "Restart.Intensity", 2),
					"a per child Restart.Intensity is not supported for %s, which init rejects", typeName(supType.Int))
			}

			if preserve := field("Options.PreserveMailbox"); preserve.Bool && allOrRest {
				m.Report(pass, preserve.Pos,
					specFinding(specID, "Options.PreserveMailbox", 2),
					"Options.PreserveMailbox is not supported for %s, which init rejects", typeName(supType.Int))
			}

			if significant := field("Significant"); significant.Bool {
				effective := field("Restart.Strategy")
				strategy := effective.Int
				if effective.Set == false || strategy == strategyInherit {
					strategy = spec.Field("Restart.Strategy").Int
				}
				switch {
				case supType.Int == typeSimpleOneForOne:
					m.Report(pass, significant.Pos,
						specFinding(specID, "Significant", 3),
						"Significant is ignored for SimpleOneForOne")
				case strategy == strategyPermanent:
					m.Report(pass, significant.Pos,
						specFinding(specID, "Significant", 3),
						"Significant is ignored under the Permanent restart strategy")
				}
			}
		}
	}
	return nil, nil
}

func checkEnum(pass *analysis.Pass, m *ergomodel.Model, specID string, v ergomodel.FieldValue, fallback token.Pos, what string) {
	if v.Set == false || v.HasInt == false {
		return
	}
	if v.Int >= 0 && v.Int <= 3 {
		return
	}
	at := v.Pos
	if at == token.NoPos {
		at = fallback
	}
	m.Report(pass, at,
		specFinding(specID, "child", 2), "%d is not a known %s, which init rejects", v.Int, what)
}

func specFinding(specID, check string, tier int) ergomodel.Finding {
	return ergomodel.Finding{
		Rule: ruleID, Kind: ergomodel.KindSpec, Tier: tier,
		ID: specID + ":" + check, Witness: check,
	}
}

func typeName(v int64) string {
	switch v {
	case typeOneForOne:
		return "OneForOne"
	case typeAllForOne:
		return "AllForOne"
	case typeRestForOne:
		return "RestForOne"
	case typeSimpleOneForOne:
		return "SimpleOneForOne"
	}
	return "this supervisor type"
}
