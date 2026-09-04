package a2028

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Request struct{ ID int64 }

type Worker struct {
	act.Actor
	backend gen.PID
}

// The shape the rule exists for: the caller is waiting on its own default budget
// and this handler asks for the same one.
func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return w.Call(w.backend, request) // want `\[tier2\] \[A2028\] HandleCall answers by making a request of its own with a budget of the default 5s`
}

type Named struct {
	act.Actor
	backend gen.PID
}

// An explicit timeout that is not smaller than the caller's is the same defect
// spelled out.
func (n *Named) HandleCallName(name gen.Atom, from gen.PID, ref gen.Ref, request any) (any, error) {
	return n.CallWithTimeout(n.backend, request, 30) // want `A2028.*HandleCallName answers by making a request of its own with a budget of 30s`
}

// A budget smaller than the caller's leaves room for the reply to arrive.
type Bounded struct {
	act.Actor
	backend gen.PID
}

func (b *Bounded) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return b.CallWithTimeout(b.backend, request, 2)
}

// The verdict travels through the project's own helpers.
type Delegating struct {
	act.Actor
	backend gen.PID
}

func (d *Delegating) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return d.fetch(request) // want `A2028.*HandleCall answers by making a request of its own, through fetch`
}

func (d *Delegating) fetch(request any) (any, error) {
	return d.Call(d.backend, request)
}

// The deferred reply: keep from and ref, answer later, never block the mailbox.
type Deferred struct {
	act.Actor
	backend gen.PID
	pending map[gen.Ref]gen.PID
}

func (d *Deferred) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	d.pending[ref] = from
	d.Send(d.backend, Request{ID: 1})
	return nil, nil
}

func (d *Deferred) HandleMessage(from gen.PID, message any) error {
	for ref, to := range d.pending {
		d.SendResponse(to, ref, message)
		delete(d.pending, ref)
	}
	return nil
}

// HandleMessage is out of scope: nobody is waiting on a reply there.
type Async struct {
	act.Actor
	backend gen.PID
}

func (a *Async) HandleMessage(from gen.PID, message any) error {
	a.Call(a.backend, message)
	return nil
}

// A recorded exception is silent.
type Excused struct {
	act.Actor
	backend gen.PID
}

func (e *Excused) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	//argus:allow A2028 the only caller uses CallWithTimeout(30)
	return e.Call(e.backend, request)
}
