package a2022

import (
	"errors"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

var (
	ErrRegistered = errors.New("registered")
	ErrMissing    = errors.New("missing")
)

type Response struct {
	Error error
}

type Worker struct {
	act.Actor
	peer gen.PID
}

func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return Response{Error: ErrMissing}, nil // want `\[tier2\] \[A2022\] a2022.ErrMissing is returned as the reply to a request`
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	// The registered one keeps its identity through the error cache.
	w.Send(w.peer, Response{Error: ErrRegistered})
	// gen.Errorf preserves the operand, so the operand is what needs the entry.
	w.Send(w.peer, Response{Error: gen.Errorf("read: %w", ErrMissing)}) // want `A2022.*a2022.ErrMissing is sent as a message`
	w.Send(w.peer, Response{Error: gen.Errorf("read: %w", ErrRegistered)})
	// A framework sentinel is registered by the framework.
	w.Send(w.peer, Response{Error: gen.ErrTimeout})
	// A value with no identity of its own has nothing to register.
	w.Send(w.peer, Response{Error: errors.New("ad hoc")})
	return nil
}

func (w *Worker) Terminate(reason error) {
	//argus:allow A2022 the peer matches on the text for this one
	w.Send(w.peer, Response{Error: ErrMissing})
}

func register(node gen.Node) {
	node.Network().RegisterErrors([]error{ErrRegistered})
}
