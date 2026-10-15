package a2002

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type RequestState struct{}

type MessageReply struct{ Data string }

// No reply is possible for the requests it handles: every path is (nil, nil), ref is
// never used, no method of the type replies, and yet the request is read. Every
// caller of RequestState waits out its full timeout.
type Silent struct {
	act.Actor
	asked int
}

func (s *Silent) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) { // want `\[tier2\] \[A2002\] HandleCall reads the request but returns \(nil, nil\) on every path, never uses ref`
	switch request.(type) {
	case RequestState:
		s.asked++
	}
	return nil, nil
}

// Logging the ref is not keeping it, so a handler that reads the request and only logs
// the ref is the same defect.
type LogsRef struct {
	act.Actor
}

func (l *LogsRef) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) { // want `A2002.*HandleCall reads the request but returns \(nil, nil\) on every path`
	l.Log().Debug("dropping request %s from %s", ref, from)
	if _, ok := request.(RequestState); ok {
		l.Log().Info("state requested")
	}
	return nil, nil
}

// The request is the last parameter whatever it is called.
type Renamed struct {
	act.Actor
	asked int
}

func (r *Renamed) HandleCall(from gen.PID, ref gen.Ref, req any) (any, error) { // want `A2002.*HandleCall reads the request`
	if _, ok := req.(RequestState); ok {
		r.asked++
	}
	return nil, nil
}

// A stub that never reads its request is the framework's own fallback written out:
// act.Actor's HandleCall logs and returns (nil, nil) too, and the caller times out the
// same way whether the stub is written or inherited. Logging the request, even its
// type, is not reading it.
type Stub struct {
	act.Actor
}

func (s *Stub) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	s.Log().Error("unhandled request %T from %s", request, from)
	return nil, nil
}

// The same stub with the ref logged and the request unnamed.
type Unnamed struct {
	act.Actor
}

func (u *Unnamed) HandleCall(from gen.PID, ref gen.Ref, _ any) (any, error) {
	u.Log().Debug("dropping request %s from %s", ref, from)
	return nil, nil
}

// A meta behavior has no fallback to inherit, so the stub is how it says it takes no
// calls; the framework's port, TCP, UDP and web meta processes are written exactly so.
type Reader struct {
	gen.MetaProcess
}

func (r *Reader) Init(process gen.MetaProcess) error { return nil }

func (r *Reader) Start() error { return nil }

func (r *Reader) HandleMessage(from gen.PID, message any) error { return nil }

func (r *Reader) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (r *Reader) Terminate(reason error) {}

// The deferred reply: the token is stored and something later replies. This is the
// legitimate use of two nils and must stay silent.
type Deferred struct {
	act.Actor
	pending map[gen.Ref]gen.PID
}

func (d *Deferred) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	d.pending[ref] = from
	return nil, nil
}

// A type that replies elsewhere implements deferred replies, so its silent HandleCall
// is not a defect.
type RepliesLater struct {
	act.Actor
	waiting gen.PID
	token   gen.Ref
}

func (r *RepliesLater) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (r *RepliesLater) HandleMessage(from gen.PID, message any) error {
	return r.SendResponse(r.waiting, r.token, MessageReply{})
}

// Declining with the Important variant is still a reply. This is the case that would
// have been reported before SendResponseErrorImportant joined the detection.
type DeclinesImportant struct {
	act.Actor
	waiting gen.PID
	token   gen.Ref
}

func (d *DeclinesImportant) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (d *DeclinesImportant) HandleMessage(from gen.PID, message any) error {
	return d.SendResponseErrorImportant(d.waiting, d.token, gen.ErrUnsupported)
}

// A real reply on any path is not this rule's business.
type Answers struct {
	act.Actor
}

func (a *Answers) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request.(type) {
	case RequestState:
		return MessageReply{Data: "ok"}, nil
	}
	return nil, nil
}

// An error in the reason slot is A1008's surface, and the all-paths-nil gate keeps
// A2002 off it so the two can never both fire.
type Terminates struct {
	act.Actor
}

func (t *Terminates) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, gen.ErrUnsupported
}

// The explicit decline, which is what the diagnostic recommends.
type Declines struct {
	act.Actor
}

func (d *Declines) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	d.SendResponseError(from, ref, gen.ErrUnsupported)
	return nil, nil
}

// A named result written in the body carries the reply without a return expression.
type NamedResult struct {
	act.Actor
}

func (n *NamedResult) HandleCall(from gen.PID, ref gen.Ref, request any) (result any, reason error) {
	result = MessageReply{Data: "ok"}
	return
}

// A recorded exception is silent.
type Recorded struct {
	act.Actor
}

//argus:allow A2002 the caller is fire-and-forget by contract
func (r *Recorded) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	_ = request.(RequestState)
	return nil, nil
}
