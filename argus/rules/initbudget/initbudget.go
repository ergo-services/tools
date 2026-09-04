package initbudget

import (
	"fmt"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A1011"

const (
	defaultBudget  = 5
	defaultRequest = 5
	cappedBudget   = 15
)

var Analyzer = &analysis.Analyzer{
	Name: "argusA1011",
	Doc: `A1011: a blocking request in Init against the init budget.

Init is bounded by ProcessOptions.InitTimeout, which defaults to DefaultRequestTimeout,
five seconds. The default request timeout is also five seconds. So an Init that makes a
Call with defaults has an inner budget equal to its outer budget, and the request cannot
reliably finish before the spawner gives up on it.

What happens then is worth stating exactly, because it is not simply a failed spawn.
Init runs on its own goroutine while the spawner waits in a select on the deadline. At
expiry the spawner kills the process and returns ErrTimeout, while the abandoned Init
keeps running to completion; that goroutine later invokes Terminate on a receiver that
never finished initializing. Meanwhile the spawner was blocked for the whole budget,
which matters most when it is a supervisor on a restart path.

The rule needs the transitive fact more than any other: on a production corpus a Call
reaches Init at twenty one sites and exactly one of them is syntactically visible, the
rest arriving through a library helper two frames down. The verdict is computed in the
factory's own package and exported on the factory object, because the spawning package
names the factory and nothing else.

Units differ and the diagnostic says which: ProcessOptions.InitTimeout and
CallWithTimeout are ints in seconds, while context.WithTimeout and
ApplicationSpec.InitTimeout are durations.

The budget is uncapped for supervisor children and for node and process spawns, but an
application group member is capped at three times DefaultRequestTimeout. Past the cap
the advice is to restructure rather than to raise the budget, because raising it past
the ceiling aborts the whole application start (A2009).`,
	URL:      "https://docs.ergo.services/tools/argus#A1011",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, point := range m.SpawnPoints() {
		if point.Factory == nil {
			continue
		}
		obj, info, ok := m.FactoryOf(point.Factory)
		if ok == false || info.InitBlocks == false {
			continue
		}

		inner := info.InitTimeout
		innerText := fmt.Sprintf("%ds", inner)
		if inner == 0 {
			inner = defaultRequest
			innerText = fmt.Sprintf("the default %ds", defaultRequest)
		}

		outer := defaultBudget
		outerText := fmt.Sprintf("the default %ds", defaultBudget)
		if point.Timeout.HasInt && point.Timeout.Int > 0 {
			outer = int(point.Timeout.Int)
			outerText = fmt.Sprintf("%ds", outer)
		}

		if inner < outer {
			continue
		}

		advice := fmt.Sprintf("raise InitTimeout above %ds on this %s, or move the request out of Init",
			inner, point.What)
		if point.Capped && inner >= cappedBudget {
			advice = fmt.Sprintf("an application group member cannot go above %ds, so this one has to be restructured: move the request out of Init and let the process fetch what it needs after it is running",
				cappedBudget)
		}

		m.Report(pass, point.Pos,
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindSpec, Tier: 1,
				ID:      m.DeclID(point.In) + ":" + obj.Name(),
				Witness: info.Chain,
			},
			"%s waits in Init (%s, via %s) with an inner budget of %s against an init budget of %s, so the spawner kills the process and returns ErrTimeout while Init keeps running; %s",
			obj.Name(), info.InitWhy, info.Chain, innerText, outerText, advice)
	}
	return nil, nil
}
