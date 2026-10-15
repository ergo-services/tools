package a1001

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Cell struct {
	X string
}

type Frame struct {
	P *Cell
}

type MessageFrame struct {
	F Frame
}

// Relay hands a frame through two unexported helpers before it is sent. Every call
// to them is in this package, so the parameter resolves at the callers' callers.
type Relay struct {
	act.Actor
	current *Cell
	peer    gen.PID
}

func (r *Relay) move(x string) { r.current.X = x }

func (r *Relay) HandleMessage(from gen.PID, message any) error {
	r.relay(Frame{P: &Cell{X: "a"}})
	r.relayKept(Frame{P: r.current})
	return nil
}

func (r *Relay) relay(f Frame) { r.emit(f) }

func (r *Relay) emit(f Frame) {
	r.Send(r.peer, MessageFrame{F: f})
}

func (r *Relay) relayKept(f Frame) { r.emitKept(f) }

func (r *Relay) emitKept(f Frame) {
	r.Send(r.peer, MessageFrame{F: f}) // want `\[tier1\] \[A1001\] a1001.MessageFrame .*parameter f, and at its callers: parameter f, and at its callers: r.current, mutated in place`
}
