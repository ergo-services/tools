package a1001

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Tally struct {
	Total int
	Seen  []string
}

// Addresses hands out pointers. A pointer into this actor's own memory sees every
// write into that storage, a value field included; a pointer into a local copy sees
// only what the copy references.
type Addresses struct {
	act.Actor
	state *Tally
	copy  Tally
	peer  gen.PID
}

func (a *Addresses) count() {
	a.state.Total++
	a.copy.Total++
}

func (a *Addresses) HandleMessage(from gen.PID, message any) error {
	state := a.state
	a.Send(a.peer, &state.Total) // want `\[tier1\] \[A1001\] \*int .*a.state.Total, written through`

	a.Send(a.peer, &a.copy.Total) // want `\[tier1\] \[A1001\] \*int .*&a.copy.Total, mutated in place`

	local := a.copy
	a.Send(a.peer, &local.Total)
	a.Send(a.peer, &local.Seen) // want `\[tier2\] \[A1001\] \*\[\]string .*a.copy.Seen, only ever replaced whole`
	return nil
}
