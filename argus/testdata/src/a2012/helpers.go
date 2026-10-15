package a2012

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// The pair one frame away is the same defect: the callback returns the helper's error, so
// the helper's return is the termination reason.
type Responder struct {
	act.Actor
	peer gen.PID
}

func (r *Responder) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageWork:
		return r.answer(from, message)
	case MessageIdle:
		if err := r.notify(); err != nil {
			return err
		}
	}
	return r.HandleEvent(gen.MessageEvent{})
}

// a helper shared with another callback is reported once
func (r *Responder) HandleMessageName(name gen.Atom, from gen.PID, message any) error {
	return r.answer(from, message)
}

func (r *Responder) answer(to gen.PID, message any) error {
	if err := r.SendResponse(to, gen.Ref{}, message); err != nil {
		r.Log().Error("caller gone: %s", err)
		return err // want `\[tier2\] \[A2012\] HandleMessage through answer logs this failure at Error level and then returns it as a termination reason \(SendResponse\)`
	}
	return nil
}

func (r *Responder) notify() error {
	return r.forward()
}

func (r *Responder) forward() error {
	if _, err := r.Call(r.peer, MessageIdle{}); err != nil {
		r.Log().Error("peer unreachable: %s", err)
		return err // want `A2012.*HandleMessage through notify -> forward logs this failure`
	}
	return nil
}

// a callback reached from another callback is its own root, reported under its own name
func (r *Responder) HandleEvent(event gen.MessageEvent) error {
	if err := r.Send(r.peer, event); err != nil {
		r.Log().Error("forward failed: %s", err)
		return err // want `A2012.*\] HandleEvent logs this failure`
	}
	return nil
}

// A helper whose error the callback does not return carries no verdict, and neither does a
// helper that logs below Error.
type Quiet struct {
	act.Actor
	peer gen.PID
}

func (q *Quiet) HandleMessage(from gen.PID, message any) error {
	q.answer(message)
	return q.warn(message)
}

func (q *Quiet) answer(message any) error {
	if err := q.Send(q.peer, message); err != nil {
		q.Log().Error("send failed: %s", err)
		return err
	}
	return nil
}

func (q *Quiet) warn(message any) error {
	if err := q.Send(q.peer, message); err != nil {
		q.Log().Warning("send failed: %s", err)
		return err
	}
	return nil
}
