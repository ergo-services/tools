package a2003

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type RequestState struct{}

type ResponseState struct{ Data string }

type Worker struct {
	act.Actor
	pending map[gen.Ref]gen.PID
}

// The reply and the returned result are alternatives. Doing both sends two responses
// for one ref.
func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	w.SendResponse(from, ref, ResponseState{Data: "first"})
	return ResponseState{Data: "second"}, nil // want `\[tier2\] \[A2003\] HandleCall already replied to this request with SendResponse at line 20`
}

// TerminateReasonNormal with a non-nil result also replies, so it is the same defect.
type ReplyThenStop struct {
	act.Actor
}

func (r *ReplyThenStop) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	r.SendResponseError(from, ref, gen.ErrUnsupported)
	return ResponseState{}, gen.TerminateReasonNormal // want `A2003.*already replied to this request with SendResponseError`
}

// A deferred reply runs after the return, which is still two responses.
type Deferred struct {
	act.Actor
}

func (d *Deferred) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	defer d.SendResponse(from, ref, ResponseState{Data: "late"})
	return ResponseState{Data: "early"}, nil // want `A2003.*already replied to this request with SendResponse`
}

// The reply plus two nils is the documented deferred form, and A2002's population.
type Async struct {
	act.Actor
}

func (a *Async) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	a.SendResponse(from, ref, ResponseState{})
	return nil, nil
}

// Any other non-nil reason makes the runtime suppress the reply, so there is no second
// response to report.
type ReplyThenFail struct {
	act.Actor
}

func (r *ReplyThenFail) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	r.SendResponse(from, ref, ResponseState{})
	return ResponseState{}, gen.ErrUnsupported
}

// Mutually exclusive branches are correct code: one arm replies, another returns.
type Branches struct {
	act.Actor
}

func (b *Branches) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request.(type) {
	case RequestState:
		b.SendResponse(from, ref, ResponseState{Data: "explicit"})
		return nil, nil
	}
	return ResponseState{Data: "returned"}, nil
}

// A conditional reply followed by an unconditional return is the same shape one level
// down, and equally correct.
type Conditional struct {
	act.Actor
}

func (c *Conditional) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == nil {
		c.SendResponseError(from, ref, gen.ErrUnsupported)
		return nil, nil
	}
	return ResponseState{Data: "ok"}, nil
}

// Answering an earlier pending request and returning this one's result is the flush
// pattern: the reply goes to a different ref.
type Flush struct {
	act.Actor
	waiting gen.PID
	token   gen.Ref
}

func (f *Flush) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	f.SendResponse(f.waiting, f.token, ResponseState{Data: "earlier"})
	return ResponseState{Data: "this one"}, nil
}

// A result whose nil-ness needs dataflow is never reported.
type Unknown struct {
	act.Actor
	cached *ResponseState
}

func (u *Unknown) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	u.SendResponse(from, ref, ResponseState{})
	return u.cached, nil
}

// A local assigned once from the parameter is still the same ref.
type Aliased struct {
	act.Actor
}

func (a *Aliased) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	caller := from
	token := ref
	a.SendResponse(caller, token, ResponseState{})
	return ResponseState{}, nil // want `A2003.*already replied to this request with SendResponse`
}

// A reply from a goroutine has different timing and belongs to A1004 and A1010.
type Spawned struct {
	act.Actor
}

func (s *Spawned) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	go func() {
		s.SendResponse(from, ref, ResponseState{})
	}()
	return ResponseState{}, nil
}

// A recorded exception is silent.
type Recorded struct {
	act.Actor
}

func (r *Recorded) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	r.SendResponse(from, ref, ResponseState{})
	//argus:allow A2003 the peer tolerates the duplicate and we want the fast path
	return ResponseState{}, nil
}
