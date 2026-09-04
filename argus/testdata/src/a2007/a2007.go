package a2007

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

const eventTicks gen.Atom = "ticks"

type MessageTick struct{}

// The canonical defect: the token is thrown away at registration and the behavior
// publishes anyway.
type Mute struct {
	act.Actor
	token gen.Ref
}

func (m *Mute) Init(args ...any) error {
	if _, err := m.RegisterEvent(eventTicks, gen.EventOptions{Buffer: 10}); err != nil { // want `\[tier2\] \[A2007\] this registration of "ticks" discards the token RegisterEvent returns, and HandleMessage publishes that event`
		return err
	}
	return nil
}

func (m *Mute) HandleMessage(from gen.PID, message any) error {
	return m.SendEvent(eventTicks, m.token, MessageTick{})
}

// Keeping the token is the fix, and the rule has to be silent on it.
type Loud struct {
	act.Actor
	token gen.Ref
}

func (l *Loud) Init(args ...any) error {
	token, err := l.RegisterEvent("beats", gen.EventOptions{Buffer: 10})
	if err != nil {
		return err
	}
	l.token = token
	return nil
}

func (l *Loud) HandleMessage(from gen.PID, message any) error {
	return l.SendEvent("beats", l.token, MessageTick{})
}

// Open switches the token check off, so discarding it is deliberate and publishing
// with a zero Ref is correct.
type Open struct {
	act.Actor
}

func (o *Open) Init(args ...any) error {
	o.RegisterEvent("open_event", gen.EventOptions{Open: true})
	return nil
}

func (o *Open) HandleMessage(from gen.PID, message any) error {
	return o.SendEvent("open_event", gen.Ref{}, MessageTick{})
}

// A registration nobody in this behavior publishes says nothing about the token: the
// producer may hand it to a child, or the event may exist only to be subscribed to.
type Registrar struct {
	act.Actor
}

func (r *Registrar) Init(args ...any) error {
	r.RegisterEvent("delegated", gen.EventOptions{})
	return nil
}

// The zero literal at the publish site, on an event this package registers without
// Open.
type Zero struct {
	act.Actor
	token gen.Ref
}

func (z *Zero) Init(args ...any) error {
	token, err := z.RegisterEvent("zeros", gen.EventOptions{})
	if err != nil {
		return err
	}
	z.token = token
	return nil
}

func (z *Zero) HandleMessage(from gen.PID, message any) error {
	return z.SendEvent("zeros", gen.Ref{}, MessageTick{}) // want `A2007.*this publish of "zeros" passes gen.Ref\{\}`
}

// A field of type gen.Ref that this package never writes holds the zero value.
type NeverAssigned struct {
	act.Actor
	stale gen.Ref
}

func (n *NeverAssigned) Init(args ...any) error {
	token, err := n.RegisterEvent("cold", gen.EventOptions{})
	if err != nil {
		return err
	}
	_ = token
	return nil
}

func (n *NeverAssigned) HandleMessage(from gen.PID, message any) error {
	return n.SendEvent("cold", n.stale, MessageTick{}) // want `A2007.*passes the never assigned stale`
}

// An exported field can be written by an importer, so it is left alone.
type Exported struct {
	act.Actor
	Token gen.Ref
}

func (e *Exported) Init(args ...any) error {
	token, err := e.RegisterEvent("exported", gen.EventOptions{})
	if err != nil {
		return err
	}
	_ = token
	return nil
}

func (e *Exported) HandleMessage(from gen.PID, message any) error {
	return e.SendEvent("exported", e.Token, MessageTick{})
}

// Publishing an event this package does not register says nothing: the producer's
// options are not visible here, and Open is a real possibility.
type Foreign struct {
	act.Actor
	token gen.Ref
}

func (f *Foreign) HandleMessage(from gen.PID, message any) error {
	return f.SendEvent("somebody_elses", gen.Ref{}, MessageTick{})
}

// Two registrations of one name in one statement list: the second gets ErrTaken.
type Twice struct {
	act.Actor
	first  gen.Ref
	second gen.Ref
}

func (t *Twice) Init(args ...any) error {
	first, err := t.RegisterEvent("dup", gen.EventOptions{Buffer: 10})
	if err != nil {
		return err
	}
	t.first = first
	second, err := t.RegisterEvent("dup", gen.EventOptions{Buffer: 20}) // want `A2007.*"dup" is already registered at line \d+ of this same block`
	if err != nil {
		return err
	}
	t.second = second
	return nil
}

// Unregistering first makes a second registration of the same name legitimate.
type Recycled struct {
	act.Actor
	token gen.Ref
}

func (r *Recycled) HandleMessage(from gen.PID, message any) error {
	r.UnregisterEvent("cycle")
	token, err := r.RegisterEvent("cycle", gen.EventOptions{})
	if err != nil {
		return err
	}
	r.token = token
	token, err = r.RegisterEvent("cycle", gen.EventOptions{})
	if err != nil {
		return err
	}
	r.token = token
	return nil
}

// Two registrations in mutually exclusive branches are two statement lists, and only
// one of them runs.
type Branched struct {
	act.Actor
	token gen.Ref
}

func (b *Branched) Init(args ...any) error {
	var (
		token gen.Ref
		err   error
	)
	if len(args) > 0 {
		token, err = b.RegisterEvent("branch", gen.EventOptions{Buffer: 10})
	} else {
		token, err = b.RegisterEvent("branch", gen.EventOptions{Buffer: 1})
	}
	if err != nil {
		return err
	}
	b.token = token
	return nil
}

// A recorded exception is silent.
type Recorded struct {
	act.Actor
	token gen.Ref
}

func (r *Recorded) Init(args ...any) error {
	//argus:allow A2007 this event is published by the child that gets the token
	r.RegisterEvent("recorded", gen.EventOptions{})
	return nil
}

func (r *Recorded) HandleMessage(from gen.PID, message any) error {
	return r.SendEvent("recorded", r.token, MessageTick{})
}
