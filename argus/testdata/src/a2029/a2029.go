package a2029

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type done struct{}

type Registry struct {
	act.Actor
	waiting map[string]gen.PID
	owner   gen.PID
}

// the shape that emptied a shared registry: the peer was gone, the reply failed, and the
// process stopped holding everyone else's entries
func (r *Registry) HandleMessage(from gen.PID, message any) error {
	return r.Send(r.owner, done{}) // want `\[tier2\] \[A2029\] HandleMessage returns the error of Send\(\) as its termination reason`
}

// the same defect written one frame away
func (r *Registry) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, r.answer(from)
}

func (r *Registry) answer(to gen.PID) error {
	return r.SendWithPriority(to, done{}, gen.MessagePriorityHigh) // want `A2029.*HandleCall through answer returns the error of SendWithPriority\(\)`
}

// deciding what the absence means is the fix
type Careful struct {
	act.Actor
	owner gen.PID
}

func (c *Careful) HandleMessage(from gen.PID, message any) error {
	if err := c.Send(c.owner, done{}); err != nil {
		c.Log().Warning("nobody is listening: %s", err)
	}
	return nil
}

// a send to this process's own PID is about this process, not about a peer
type Ticker struct {
	act.Actor
}

func (t *Ticker) HandleMessage(from gen.PID, message any) error {
	return t.Send(t.PID(), done{})
}

// a send that is not the returned value carries no verdict
type Plain struct {
	act.Actor
	peer gen.PID
}

func (p *Plain) HandleMessage(from gen.PID, message any) error {
	p.Send(p.peer, done{})
	return nil
}

// a recorded exception is silent
type Recorded struct {
	act.Actor
	peer gen.PID
}

func (r *Recorded) HandleMessage(from gen.PID, message any) error {
	//argus:allow A2029 this process exists only to feed that peer and has nothing to do without it
	return r.Send(r.peer, done{})
}

type Watcher struct {
	act.Actor
	peer gen.PID
	name gen.Atom
}

func (w *Watcher) HandleMessage(from gen.PID, message any) error {
	if err := w.Send(w.peer, done{}); err != nil { // want `\[tier2\] \[A2029\] HandleMessage returns the error of Send\(\) as its termination reason`
		return err
	}
	return nil
}

func (w *Watcher) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	err := w.MonitorPID(w.peer) // want `A2029.*HandleCall returns the error of MonitorPID\(\)`
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (w *Watcher) HandleEvent(event gen.MessageEvent) error {
	return w.MonitorPID(w.peer) // want `A2029.*HandleEvent returns the error of MonitorPID\(\)`
}

type Asker struct {
	act.Actor
	peer gen.PID
	name gen.Atom
}

func (a *Asker) HandleMessage(from gen.PID, message any) error {
	if _, err := a.CallWithTimeout(a.peer, done{}, 5); err != nil { // want `A2029.*HandleMessage returns the error of CallWithTimeout\(\)`
		return err
	}
	return nil
}

func (a *Asker) Init(args ...any) error {
	if err := a.RegisterName(a.name); err != nil {
		return err
	}
	if err := a.MonitorPID(a.peer); err != nil {
		a.Log().Warning("peer already gone: %s", err)
	}
	return nil
}

// A shadowed err is a different variable. The error returned here is the outer one,
// and the failed send is only logged.
type Shadowed struct {
	act.Actor
	peer gen.PID
}

func (s *Shadowed) validate(message any) error { return nil }

func (s *Shadowed) HandleMessage(from gen.PID, message any) error {
	err := s.validate(message)
	if err != nil {
		if err := s.Send(s.peer, done{}); err != nil {
			s.Log().Warning("peer gone: %s", err)
		}
		return err
	}
	return nil
}

func (s *Shadowed) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	var err error
	if request != nil {
		if err := s.Send(s.peer, done{}); err != nil {
			s.Log().Warning("peer gone: %s", err)
		}
	}
	return nil, err
}
