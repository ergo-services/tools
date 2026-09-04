package eventbuffer

import (
	"fmt"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2013"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2013",
	Doc: `A2013: the buffered events a subscription returns are discarded.

LinkEvent and MonitorEvent return the producer's ring buffer, so a late or reconnecting
subscriber catches up on what it missed. Dropping it is data loss precisely when the
producer set Buffer above zero, and nothing at all when it did not: an unbuffered
producer returns nil.

So the premise has to be established, not assumed, and it is decidable in only two
cases. The framework's own gen.CoreEvent is registered by every node with Buffer 1000
and its name cannot be taken by user code, so a discard there loses up to a thousand
lifecycle records. And a producer in the same package as the subscriber is visible
directly, through its RegisterEvent options.

Everything else is silent, and that is the design's own conclusion rather than a
shortcut: for most subscriptions the producer lives in a package the subscriber does not
import, and for a remote event the buffer arrives over the wire, so the size is not a
property of the call site at all. A cross-package buffer would need the producer to
publish it as a fact on the name constant; that is legal but not built, and the rule
prefers silence to a guess.

The inspect_ family is in the framework table specifically so it is NOT reported: it is
buffered at one and re-published every period, so a discard delays a snapshot rather
than losing one.

A behavior with no HandleEvent of its own is also silent. A subscription taken purely
for the unregister or down signal has no consumer to feed a buffer to.

Keeping the result is the fix, and the compiler already proves it: Go rejects an unused
local, so a named result is handled somewhere by construction. Only a bare call
statement or a blanked first result is a discard.`,
	URL:      "https://docs.ergo.services/tools/argus#A2013",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, sub := range m.EventSubscriptions() {
		if sub.Discarded == false {
			continue
		}

		if m.ConsumesEvents(sub.Behavior) == false {
			continue
		}
		if sub.Name.HasStr == false {
			continue
		}
		size, source, reportable, ok := m.EventBuffer(sub.Name.Str)
		if ok == false || reportable == false || size <= 0 {
			continue
		}

		id := sub.Name.Str
		if sub.In != nil {
			id = ergomodel.CallbackID(sub.In) + ":" + sub.Name.Str
		}
		m.Report(pass, sub.Pos,
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindEvent, Tier: 2,
				ID: id, Witness: fmt.Sprintf("buffer=%d", size),
			},
			"the event buffer returned by %s is discarded, but %q is %s buffering %d messages, so everything published before this subscription is lost; keep the result and feed it through the same path as HandleEvent",
			sub.Method, sub.Name.Str, source, size)
	}
	return nil, nil
}
