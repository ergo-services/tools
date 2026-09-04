package a2018

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessageTick struct{ N int }

// Unaware sets Notify and handles neither notification, so it cannot tell whether
// anyone is listening.
type Unaware struct {
	act.Actor
}

func (u *Unaware) Init(args ...any) error {
	u.RegisterEvent("ticks", gen.EventOptions{Notify: true}) // want `\[tier2\] \[A2018\] Notify is set for ticks`
	return nil
}

// Aware handles both notifications, which is what the option is for.
type Aware struct {
	act.Actor
}

func (a *Aware) Init(args ...any) error {
	a.RegisterEvent("aware", gen.EventOptions{Notify: true})
	return nil
}

func (a *Aware) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case gen.MessageEventStart:
	case gen.MessageEventStop:
	}
	return nil
}

// PartiallyAware handles only the start, which is enough: the producer can observe
// the transition, and demanding both would be a style rule rather than a defect.
type PartiallyAware struct {
	act.Actor
}

func (p *PartiallyAware) Init(args ...any) error {
	p.RegisterEvent("partial", gen.EventOptions{Notify: true})
	return nil
}

func (p *PartiallyAware) HandleMessage(from gen.PID, message any) error {
	if _, ok := message.(gen.MessageEventStart); ok {
		return nil
	}
	return nil
}

// Quiet never sets Notify, so there is nothing to observe and nothing to report.
type Quiet struct {
	act.Actor
}

func (q *Quiet) Init(args ...any) error {
	q.RegisterEvent("quiet", gen.EventOptions{Buffer: 10})
	return nil
}

// Recorded documents the exception instead of handling the messages.
type Recorded struct {
	act.Actor
}

func (r *Recorded) Init(args ...any) error {
	//argus:allow A2018 the operator watches the subscriber gauge instead
	r.RegisterEvent("recorded", gen.EventOptions{Notify: true})
	return nil
}
