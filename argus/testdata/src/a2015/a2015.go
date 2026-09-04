package a2015

import (
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessageTick struct{}

type MessageWork struct{ ID int64 }

type MessageRetry struct{}

// The correct idiom: Init arms once, the tick handler re-arms one successor, and there
// is always exactly one chain.
type Ticker struct {
	act.Actor
}

func (t *Ticker) Init(args ...any) error {
	t.SendAfter(t.PID(), MessageTick{}, time.Second)
	return nil
}

func (t *Ticker) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageTick:
		t.SendAfter(t.PID(), MessageTick{}, time.Second)
	}
	return nil
}

// The retry path: the work handler arms the tick chain that the tick handler already
// re-arms, so every failed unit of work adds a parallel chain.
type Retrying struct {
	act.Actor
	failures int
}

func (r *Retrying) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageTick:
		r.SendAfter(r.PID(), MessageTick{}, time.Second)
	case MessageWork:
		r.failures++
		r.SendAfter(r.PID(), MessageTick{}, time.Millisecond*100) // want `\[tier2\] \[A2015\] this arms a MessageTick chain that HandleMessage already re-arms on every delivery`
	}
	return nil
}

// Two armings as separate statements of the tick handler's own case: the chain doubles
// on every delivery.
type Doubling struct {
	act.Actor
}

func (d *Doubling) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageTick:
		d.SendAfter(d.PID(), MessageTick{}, time.Second)
		d.SendAfter(d.PID(), MessageTick{}, time.Second*2) // want `A2015.*this arms a MessageTick chain`
	}
	return nil
}

// Two armings in two arms of one branch: exactly one runs, so the chain stays single.
type Branched struct {
	act.Actor
	fast bool
}

func (b *Branched) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageTick:
		if b.fast {
			b.SendAfter(b.PID(), MessageTick{}, time.Millisecond*100)
		} else {
			b.SendAfter(b.PID(), MessageTick{}, time.Second)
		}
	}
	return nil
}

// Keeping the cancel is the stop mechanism, so the shape is no longer unstoppable.
type Cancellable struct {
	act.Actor
	stop gen.CancelFunc
}

func (c *Cancellable) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageTick:
		cancel, _ := c.SendAfter(c.PID(), MessageTick{}, time.Second)
		c.stop = cancel
	case MessageRetry:
		if c.stop != nil {
			c.stop()
		}
		cancel, _ := c.SendAfter(c.PID(), MessageTick{}, time.Millisecond*100)
		c.stop = cancel
	}
	return nil
}

// Two different messages are two different chains, each with one arming.
type Separate struct {
	act.Actor
}

func (s *Separate) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageTick:
		s.SendAfter(s.PID(), MessageTick{}, time.Second)
	case MessageRetry:
		s.SendAfter(s.PID(), MessageRetry{}, time.Second*5)
	}
	return nil
}

// A chain nobody re-arms is a plain deferred message, however many places arm it.
type Deferred struct {
	act.Actor
}

func (d *Deferred) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageWork:
		d.SendAfter(d.PID(), MessageRetry{}, time.Second)
	case MessageTick:
		d.SendAfter(d.PID(), MessageRetry{}, time.Second*2)
	}
	return nil
}

// A timer aimed at somebody else is their mailbox, not a self chain.
type Remote struct {
	act.Actor
	peer gen.PID
}

func (r *Remote) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageTick:
		r.SendAfter(r.PID(), MessageTick{}, time.Second)
	case MessageWork:
		r.SendAfter(r.peer, MessageTick{}, time.Second)
	}
	return nil
}

// A periodic send is a different defect: its discarded cancel is A2006's.
type Periodic struct {
	act.Actor
}

func (p *Periodic) Init(args ...any) error {
	p.SendEvery(p.PID(), MessageTick{}, time.Second)
	return nil
}

func (p *Periodic) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageTick:
		p.SendEvery(p.PID(), MessageTick{}, time.Second)
	}
	return nil
}

// The registered name is the same mailbox as the PID.
type Named struct {
	act.Actor
}

func (n *Named) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageTick:
		n.SendAfter(n.Name(), MessageTick{}, time.Second)
	case MessageWork:
		n.SendAfter(n.Name(), MessageTick{}, time.Millisecond*100) // want `A2015.*this arms a MessageTick chain`
	}
	return nil
}

// A recorded exception is silent.
type Recorded struct {
	act.Actor
}

func (r *Recorded) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageTick:
		r.SendAfter(r.PID(), MessageTick{}, time.Second)
	case MessageWork:
		//argus:allow A2015 the tick handler drops a tick it did not expect
		r.SendAfter(r.PID(), MessageTick{}, time.Millisecond*100)
	}
	return nil
}
