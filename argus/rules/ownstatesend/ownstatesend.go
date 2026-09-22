package ownstatesend

import (
	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A1006"

var Analyzer = &analysis.Analyzer{
	Name: "argusA1006",
	Doc: `A1006: sending a field of the actor's own state.

The hazard is A1001's, and the fix is not, which is why this is a separate rule
rather than a witness variant. A1001 says a message type shares memory with whoever
sent it, and the answers are to send a value or to copy. Here the sender is the
owner: the field keeps being mutated by this actor's own callbacks while the receiver
reads it, and copying at the send site papers over a design in which two actors share
one variable. What the receiver needs is a message built out of the parts it uses.

The site is reported by exactly one of the two rules. Both ask the model the same
question, so A1001 stays silent wherever this fires and neither ever describes one
line twice with two different fixes.

Two payload forms count: the field itself, and a field of a composite literal payload.
A slice expression or an index on a receiver field is deliberately not one, because
that is A1007's question about memory escaping the owner, and a nested literal's own
fields belong to that type rather than to this send. A field reached through a local
is not a form this rule can see either, and it does not have to be: provenance carries
the verdict to A1001, which reports that site with the same tier.

The tier is whether the owner writes into that memory. A field the actor assigns into
(f[k] = v, f = append(f, x), delete, clear, a write through an element) is a live race
with the receiver and reports at tier 1. A field only ever replaced whole is a value
nobody mutates once it is out, so it reports at tier 2: the sharing holds today, and
only the type fails to say that it has to.

The guardedness gate is the same as A1001's: a pointee that synchronizes its own
state was shared on purpose. Spawn arguments are not this rule's surface at all,
because sharing through a spawn argument is what A2011 exists for and its population
is entirely different.

Source: actors.md`,
	URL:      "https://docs.ergo.services/tools/argus#A1006",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, site := range m.SendSites {
		share, ok := m.OwnStateSend(site)
		if ok == false {
			continue
		}
		payload := "this message"
		if share.Direct {
			payload = "the payload"
		}

		tier, clause := verdict(m.OwnFieldMutated(site.In, share.Field), share.Field)

		m.Report(pass, share.Pos,
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindShape, Tier: tier,
				ID:      ergomodel.CallbackID(site.In) + ":" + share.Field,
				Witness: share.Field,
			},
			"%s puts %s into %s, and %s is this actor's own state: the receiver reads it through %s, and local delivery does not copy. %s",
			site.In.Name, share.Field, payload, share.Field, share.Witness, clause)
	}
	return nil, nil
}

func verdict(mutated bool, field string) (int, string) {
	if mutated {
		return 1, "This actor writes into " + field +
			" elsewhere, so the receiver is reading memory that moves under it. Build a message" +
			" out of what the receiver needs rather than handing over the field"
	}
	return 2, "This actor only ever replaces " + field +
		" whole, so what the receiver holds is stable today and nothing but this reading says so." +
		" Build a message out of what the receiver needs rather than handing over the field"
}
