package eventnotify

import (
	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2018"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2018",
	Doc: `A2018: Notify is set but the producer handles neither start nor stop.

EventOptions.Notify makes the framework send MessageEventStart when the subscriber
count goes from zero to one and MessageEventStop when it returns to zero. A producer
that sets it and handles neither message has switched on a feature it cannot observe,
so it keeps publishing into an event nobody consumes and cannot tell the difference.

Either handle the two messages, gating publication on them, or drop the option.

Source: events reference`,
	URL:      "https://docs.ergo.services/tools/argus#A2018",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, reg := range m.Events() {
		if reg.Notify.Set == false || reg.Notify.Bool == false {
			continue
		}

		if m.HandlesEventLifecycle(reg.Behavior) {
			continue
		}
		name := reg.Name
		if name == "" {
			name = "the event"
		}
		m.Report(pass, reg.Notify.Pos,
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindEvent, Tier: 2,
				ID: eventID(reg)},
			"Notify is set for %s but neither gen.MessageEventStart nor gen.MessageEventStop is handled, so the producer cannot tell whether anyone is listening",
			name)
	}
	return nil, nil
}

func eventID(reg *ergomodel.EventRegistration) string {
	name := reg.Name
	if name == "" {
		name = "?"
	}
	if reg.In != nil {
		return ergomodel.CallbackID(reg.In) + ":" + name
	}
	return name
}
