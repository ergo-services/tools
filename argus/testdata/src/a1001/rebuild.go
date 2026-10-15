package a1001

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Halves struct {
	A []int
	B []int
}

type MessageIn struct {
	H Halves
}

type MessageOut struct {
	H Halves
}

// Rebuild forwards a struct with some of its fields replaced. Each field is what was
// last written into it, and the struct it was copied from only for the rest.
type Rebuild struct {
	act.Actor
	buf  []int
	peer gen.PID
}

func (r *Rebuild) HandleMessage(from gen.PID, message any) error {
	in := message.(MessageIn)
	r.buf = append(r.buf, 1)

	// B is this actor's buffer, which it keeps appending to
	h := in.H
	h.B = r.buf
	r.Send(r.peer, MessageOut{H: h}) // want `\[tier1\] \[A1001\] .*MessageOut -> H a1001.Halves -> B \[\]int; .*r.buf, mutated in place`

	// A is rebuilt fresh, so what is still shared is B, which arrived in the message
	g := in.H
	g.A = []int{1}
	r.Send(r.peer, MessageOut{H: g}) // want `\[tier2\] \[A1001\] .*MessageOut -> H a1001.Halves -> B \[\]int; .*arrived in message`

	// both rebuilt: nothing of anyone else's is left in it
	k := in.H
	k.A = []int{1}
	k.B = []int{2}
	r.Send(r.peer, MessageOut{H: k})

	// a write on one branch only does not replace the field on the other
	j := in.H
	j.A = []int{1}
	if len(j.A) > 0 {
		j.B = []int{2}
	}
	r.Send(r.peer, MessageOut{H: j}) // want `\[tier2\] \[A1001\] .*MessageOut -> H a1001.Halves -> B \[\]int; .*arrived in message`
	return nil
}
