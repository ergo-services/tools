package a2016

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessageSetup struct{}

type MessageTick struct{}

type MessageRename struct{ Name gen.Atom }

// The canonical form: Init defers to itself and the handler establishes the identity,
// so between Spawn returning and that message being handled the process is unreachable
// by name and its event is unknown.
type Deferred struct {
	act.Actor
	token gen.Ref
}

func (d *Deferred) Init(args ...any) error {
	d.Send(d.PID(), MessageSetup{})
	return nil
}

func (d *Deferred) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageSetup:
		if err := d.RegisterName("worker"); err != nil { // want `\[tier2\] \[A2016\] HandleMessage establishes a registered name, and Init deferred to this handler with the self send at line \d+`
			return err
		}
		token, err := d.RegisterEvent("ticks", gen.EventOptions{}) // want `A2016.*establishes an event`
		if err != nil {
			return err
		}
		d.token = token
	}
	return nil
}

// Doing it in Init is the fix.
type Correct struct {
	act.Actor
	token gen.Ref
}

func (c *Correct) Init(args ...any) error {
	if err := c.RegisterName("worker_correct"); err != nil {
		return err
	}
	token, err := c.RegisterEvent("beats", gen.EventOptions{})
	if err != nil {
		return err
	}
	c.token = token
	// Deferring the slow work is fine: the identity is already established.
	c.Send(c.PID(), MessageSetup{})
	return nil
}

func (c *Correct) HandleMessage(from gen.PID, message any) error {
	return nil
}

// A dynamic rename on an operator command is deliberate, has no Init self send, and
// stays silent.
type Renamable struct {
	act.Actor
}

func (r *Renamable) Init(args ...any) error {
	return r.RegisterName("renamable")
}

func (r *Renamable) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case MessageRename:
		return r.RegisterName(m.Name)
	}
	return nil
}

// Registering an event when the first consumer asks is dynamic too.
type Lazy struct {
	act.Actor
	token gen.Ref
}

func (l *Lazy) HandleMessage(from gen.PID, message any) error {
	if l.token == (gen.Ref{}) {
		token, err := l.RegisterEvent("lazy", gen.EventOptions{})
		if err != nil {
			return err
		}
		l.token = token
	}
	return nil
}

// The registered name is the same mailbox as the PID, so the deferral is the same.
type ByName struct {
	act.Actor
}

func (b *ByName) Init(args ...any) error {
	b.Send(b.Name(), MessageSetup{})
	return nil
}

func (b *ByName) HandleMessage(from gen.PID, message any) error {
	alias, err := b.CreateAlias() // want `A2016.*establishes an alias`
	if err != nil {
		return err
	}
	_ = alias
	return nil
}

// A send to somebody else is not a deferral of this process's own setup.
type Notifier struct {
	act.Actor
	parent gen.PID
}

func (n *Notifier) Init(args ...any) error {
	n.Send(n.parent, MessageSetup{})
	return nil
}

func (n *Notifier) HandleMessage(from gen.PID, message any) error {
	return n.RegisterName("notifier")
}

// A recorded exception is silent.
type Recorded struct {
	act.Actor
}

func (r *Recorded) Init(args ...any) error {
	r.Send(r.PID(), MessageSetup{})
	return nil
}

func (r *Recorded) HandleMessage(from gen.PID, message any) error {
	//argus:allow A2016 the parent waits for a ready notification before publishing this name
	return r.RegisterName("recorded")
}
