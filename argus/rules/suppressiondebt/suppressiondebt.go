package suppressiondebt

import (
	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
	"ergo.tools/argus/rules/alwaysfails"
	"ergo.tools/argus/rules/appinitleak"
	"ergo.tools/argus/rules/behaviorspec"
	"ergo.tools/argus/rules/callbackblocking"
	"ergo.tools/argus/rules/callbackgoroutine"
	"ergo.tools/argus/rules/callbackroundtrip"
	"ergo.tools/argus/rules/callbudget"
	"ergo.tools/argus/rules/eventnotify"
	"ergo.tools/argus/rules/goroutinerecover"
	"ergo.tools/argus/rules/handlecallreason"
	"ergo.tools/argus/rules/internalescape"
	"ergo.tools/argus/rules/messagealiasing"
	"ergo.tools/argus/rules/metastart"
	"ergo.tools/argus/rules/registration"
	"ergo.tools/argus/rules/spawnargs"
	"ergo.tools/argus/rules/stategate"
	"ergo.tools/argus/rules/supervisorspec"
	"ergo.tools/argus/rules/timermisuse"
	"ergo.tools/argus/rules/uncheckedresult"
	"ergo.tools/argus/rules/wireerror"
	"ergo.tools/argus/rules/wiresentinel"
	"ergo.tools/argus/rules/wireshape"
	"ergo.tools/argus/rules/wiretypeclosure"
)

const ruleID = "A3005"

var Analyzer = &analysis.Analyzer{
	Name: "argusA3005",
	Doc: `A3005: suppression debt.

Every exception is a decision someone made once and nobody revisits. This rule keeps
the record honest so the history starts on day one rather than after the first year of
accumulated directives.

Reported per directive: an allow or ignore with no reason, which is indistinguishable
from silencing the tool; a placeholder reason left exactly as the offered fix emitted
it; and a directive naming an identifier that is not a rule in this build, which is
either a typo suppressing nothing or a reference to a withdrawn rule. The same for a
baseline entry naming an unknown rule.

Reported per package: the debt metric. Suppressed findings counted by rule and by
which mechanism suppressed them, with the effective severity of the tier each rule
emits, because ten suppressions of a rule that is off and ten of a blocking one are
not the same number.

This rule is exempt from every form of suppression, and that is deliberate: excusing
a package would hide its debt, a bare ignore with no reason would silence the rule
that exists to find it, and baselining the metric would defeat the point of having
one.

It requires every other rule, purely for ordering: the metric counts what they
suppressed, so it has to run after them.`,
	URL: "https://docs.ergo.services/tools/argus#A3005",
	Requires: []*analysis.Analyzer{
		ergomodel.Analyzer,

		messagealiasing.Analyzer,
		callbackblocking.Analyzer,
		callbackgoroutine.Analyzer,
		internalescape.Analyzer,
		handlecallreason.Analyzer,
		goroutinerecover.Analyzer,
		stategate.Analyzer,
		uncheckedresult.Analyzer,
		wireshape.Analyzer,
		supervisorspec.Analyzer,
		timermisuse.Analyzer,
		registration.Analyzer,
		spawnargs.Analyzer,
		eventnotify.Analyzer,
		wireerror.Analyzer,
		wiretypeclosure.Analyzer,
		wiresentinel.Analyzer,
		appinitleak.Analyzer,
		callbackroundtrip.Analyzer,
		behaviorspec.Analyzer,
		alwaysfails.Analyzer,
		callbudget.Analyzer,
		metastart.Analyzer,
	},
	Run: run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, entry := range m.Debt() {
		if entry.Reason == "" {
			form := "an allow"
			if entry.Blanket {
				form = "an ignore"
			}
			m.Report(pass, entry.Pos, debtFinding("no-reason"),
				"%s directive has no reason, so nobody can tell later whether it is still true",
				form)
			continue
		}
		if entry.Reason == "<reason>" {
			m.Report(pass, entry.Pos, debtFinding("placeholder"),
				"the reason is still the placeholder the offered fix emitted; replace it with why this case is safe")
			continue
		}
		for _, id := range entry.Rules {
			if ergomodel.KnownRuleIDs[id] {
				continue
			}
			m.Report(pass, entry.Pos, debtFinding("unknown-rule/"+id),
				"%s is not a rule in this build, so this directive suppresses nothing", id)
		}
	}

	if path := m.BaselinePath(); path != "" && len(pass.Files) > 0 {
		for _, entry := range ergomodel.UnknownRuleEntries(path) {
			m.Report(pass, pass.Files[0].Pos(), debtFinding("baseline-unknown-rule/"+entry.Rule),
				"the baseline records %s %s %s, but %s is not a rule in this build, so the entry suppresses nothing",
				entry.Rule, entry.Kind, entry.ID, entry.Rule)
		}
	}

	if line, ok := m.DebtLine(); ok && len(pass.Files) > 0 {
		m.Report(pass, pass.Files[0].Pos(), debtFinding("metric"),
			"suppression debt in this package: %s", line)
	}
	return nil, nil
}

func debtFinding(what string) ergomodel.Finding {
	return ergomodel.Finding{
		Rule: ruleID, Kind: ergomodel.KindDirective, Tier: 3, ID: what,
	}
}
