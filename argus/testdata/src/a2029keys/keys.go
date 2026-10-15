// Package a2029keys pins the A2029 finding key: callback, helper chain and operation
// joined by a separator, and the target expression as the per-site part, so two sites in
// one callback are two baseline entries and a line move changes neither.
package a2029keys

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Pair struct {
	act.Actor
	left  gen.PID
	right gen.PID
}

func (p *Pair) HandleMessage(from gen.PID, message any) error {
	if err := p.Send(p.left, message); err != nil { // want `\[argus-key: A2029 callback a2029keys\.Pair\.HandleMessage:Send\(\)@p\.left\]`
		return err
	}
	if err := p.Send(p.right, message); err != nil { // want `\[argus-key: A2029 callback a2029keys\.Pair\.HandleMessage:Send\(\)@p\.right\]`
		return err
	}
	return p.connect(gen.Event{Name: "ticks", Node: "a@b"})
}

func (p *Pair) connect(event gen.Event) error {
	if _, err := p.MonitorEvent(event); err != nil { // want `\[argus-key: A2029 callback a2029keys\.Pair\.HandleMessage:connect>MonitorEvent\(\)@event\]`
		return err
	}
	return p.Send(p.left, gen.Event{Name: "ticks", Node: "a@b"}) // want `\[argus-key: A2029 callback a2029keys\.Pair\.HandleMessage:connect>Send\(\)@p\.left\]`
}
