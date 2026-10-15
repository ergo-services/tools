package a2029

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// SendEvent publishes to this process's own registration. It fails only when the process
// is not running, the node is stopping, or the token is not this process's, and a missing
// subscriber is not an error, so there is no peer in its failure.
type Producer struct {
	act.Actor
	token gen.Ref
}

func (p *Producer) HandleMessage(from gen.PID, message any) error {
	return p.relay(message)
}

func (p *Producer) relay(message any) error {
	if err := p.SendEvent("ticks", p.token, message); err != nil {
		return err
	}
	return nil
}

// Without the trap a link already ties this process to the peer, so a failed link is the
// link's own outcome rather than a second, separate death.
type Bound struct {
	act.Actor
	peer gen.PID
}

func (b *Bound) HandleMessage(from gen.PID, message any) error {
	return b.LinkPID(b.peer)
}

func (b *Bound) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if err := b.UnlinkPID(b.peer); err != nil {
		return nil, err
	}
	return nil, nil
}

// With the trap the peer's exit is only a message, so the returned error is what makes
// its absence fatal.
type Trapper struct {
	act.Actor
	peer gen.PID
}

func (t *Trapper) Init(args ...any) error {
	t.SetTrapExit(true)
	return nil
}

func (t *Trapper) HandleMessage(from gen.PID, message any) error {
	return t.LinkPID(t.peer) // want `A2029.*HandleMessage returns the error of LinkPID\(\)`
}

// A meta this process spawned belongs to it and is terminated with it, so watching it is
// about this process's own state. An alias that arrived from elsewhere is a peer.
type Owner struct {
	act.Actor
	conn   gen.Alias
	remote gen.Alias
}

func (o *Owner) HandleMessage(from gen.PID, message any) error {
	return o.connect()
}

func (o *Owner) connect() error {
	alias, err := o.SpawnMeta(nil, gen.MetaOptions{})
	if err != nil {
		return err
	}
	if err := o.MonitorAlias(alias); err != nil {
		return err
	}
	o.conn = alias
	return nil
}

func (o *Owner) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	alias := o.remote
	if err := o.MonitorAlias(alias); err != nil { // want `A2029.*HandleCall returns the error of MonitorAlias\(\)`
		return nil, err
	}
	return nil, nil
}

// The registrar's event is registered by the node and never unregistered, so monitoring it
// is not an operation on a peer.
type Discovery struct {
	act.Actor
	registrar gen.Registrar
}

func (d *Discovery) HandleMessage(from gen.PID, message any) error {
	event, err := d.registrar.Event()
	if err != nil {
		return err
	}
	if _, err := d.MonitorEvent(event); err != nil {
		return err
	}
	return nil
}

// Init's error is a spawn refusal handed back to the spawner, not a termination reason:
// a process that cannot start without its peer may say so.
type Starter struct {
	act.Actor
	peer gen.PID
}

func (s *Starter) Init(args ...any) error {
	return s.MonitorPID(s.peer)
}

// A callback reached from another callback is analysed as its own root, so its site is
// reported once and under its own name, and a helper shared by two callbacks is reported
// once as well.
type Relay struct {
	act.Actor
	peer gen.PID
}

func (r *Relay) HandleMessage(from gen.PID, message any) error {
	if err := r.forward(message); err != nil {
		return err
	}
	return r.HandleEvent(gen.MessageEvent{})
}

func (r *Relay) HandleEvent(event gen.MessageEvent) error {
	return r.Send(r.peer, event) // want `A2029.*\] HandleEvent returns the error of Send\(\)`
}

func (r *Relay) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, r.forward(request)
}

func (r *Relay) forward(message any) error {
	return r.SendImportant(r.peer, message) // want `A2029.*HandleMessage through forward returns the error of SendImportant\(\)`
}

// Two sites in one callback with the same operation are two findings, not one.
type Pair struct {
	act.Actor
	left  gen.PID
	right gen.PID
}

func (p *Pair) HandleMessage(from gen.PID, message any) error {
	if err := p.Send(p.left, message); err != nil { // want `A2029.*HandleMessage returns the error of Send\(\)`
		return err
	}
	if err := p.Send(p.right, message); err != nil { // want `A2029.*HandleMessage returns the error of Send\(\)`
		return err
	}
	return nil
}
