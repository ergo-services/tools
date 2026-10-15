package a1001

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"

	"shared"
)

// The first reference of these types is one the rule accepts on its own, an
// allowlisted []byte or a pointee that guards itself. That says nothing about the
// fields after it.
type MessageRawThenItems struct {
	Raw   []byte
	Items []int
}

type MessageGuardThenItems struct {
	Cache *shared.Guarded
	Items []int
}

type MessageRawOnly struct {
	Raw   []byte
	Cache *shared.Guarded
}

type Parts struct {
	act.Actor
	items []int
	guard *shared.Guarded
	peer  gen.PID
}

func (p *Parts) push(n int) { p.items = append(p.items, n) }

func (p *Parts) HandleMessage(from gen.PID, message any) error {
	items := p.items
	p.Send(p.peer, MessageRawThenItems{Raw: []byte("x"), Items: items}) // want `\[tier1\] \[A1001\] .*MessageRawThenItems -> Items \[\]int; .*p.items, mutated in place`

	guard := p.guard
	p.Send(p.peer, MessageGuardThenItems{Cache: guard, Items: items}) // want `\[tier1\] \[A1001\] .*MessageGuardThenItems -> Items \[\]int; .*p.items, mutated in place`

	// what is left is accepted, so nothing is shared that the rule asks about
	p.Send(p.peer, MessageRawOnly{Raw: []byte("y"), Cache: guard})

	// and an accepted part that comes from state does not make a fresh one shared
	p.Send(p.peer, MessageGuardThenItems{Cache: guard, Items: []int{1}})

	// a field of the actor's own state behind a conversion belongs to A1006, not to
	// this rule
	p.Send(p.peer, MessageRawThenItems(MessageRawThenItems{Items: p.items}))
	return nil
}
