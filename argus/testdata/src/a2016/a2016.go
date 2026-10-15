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
// by name and its event is unknown. Each kind says what fails for it, and only that.
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
		if err := d.RegisterName("worker"); err != nil { // want `\[tier2\] \[A2016\] HandleMessage registers a name, deferred by the self send in Init at line \d+: Spawn has handed this process to its parent before this message is handled, and until then a send or call addressed to the name fails with gen.ErrProcessUnknown. Register it in Init, or name the process at spawn with SpawnRegister, after which RegisterName returns gen.ErrTaken$`
			return err
		}
		token, err := d.RegisterEvent("ticks", gen.EventOptions{}) // want `\[tier2\] \[A2016\] HandleMessage registers an event, deferred by the self send in Init at line \d+: Spawn has handed this process to its parent before this message is handled, and until then LinkEvent and MonitorEvent on the event fail with gen.ErrEventUnknown. Call RegisterEvent in Init and defer only the slow work$`
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

// The registered name is the same mailbox as the PID, so the deferral is the same. A
// deferred alias fails nobody outside, since nobody can address it before it exists,
// so it is only a hint about the ordering the handlers rely on.
type ByName struct {
	act.Actor
}

func (b *ByName) Init(args ...any) error {
	b.Send(b.Name(), MessageSetup{})
	return nil
}

func (b *ByName) HandleMessage(from gen.PID, message any) error {
	alias, err := b.CreateAlias() // want `\[tier3\] \[A2016\] HandleMessage creates an alias, deferred by the self send in Init at line \d+: nobody can address the alias before it exists, so nothing outside fails`
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
