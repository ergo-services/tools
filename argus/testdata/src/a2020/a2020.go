package a2020

import (
	"errors"
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

var ErrNotFound = errors.New("not found")

type Response struct {
	ID    int64
	Error error
}

type Worker struct {
	act.Actor
	peer gen.PID
}

// The reply slot of a HandleCall is sent by the runtime itself.
func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return Response{Error: fmt.Errorf("read %d: %w", request, ErrNotFound)}, nil // want `\[tier2\] \[A2020\] this error is returned as the reply to a request and it is built with fmt.Errorf`
}

// gen.Errorf keeps the operand where EDF can reach it.
func (w *Worker) HandleMessage(from gen.PID, message any) error {
	w.Send(w.peer, Response{Error: gen.Errorf("read: %w", ErrNotFound)})
	w.Send(w.peer, Response{Error: fmt.Errorf("read: %w", ErrNotFound)}) // want `A2020.*this error is sent as a message`
	// No %w verb, so nothing is wrapped and nothing is lost.
	w.Send(w.peer, Response{Error: fmt.Errorf("read failed")})
	// The bare sentinel keeps its identity through the error cache.
	w.Send(w.peer, Response{Error: ErrNotFound})
	return nil
}

// The error argument of an explicit decline.
func (w *Worker) HandleMessageName(name gen.Atom, from gen.PID, message any) error {
	w.SendResponseError(from, gen.Ref{}, fmt.Errorf("decline: %w", ErrNotFound)) // want `A2020.*this error is answered to a request with SendResponseError`
	return nil
}

// A helper that sends is a send one frame away: the model publishes which
// parameter lands in the payload position.
func (w *Worker) respond(from gen.PID, ref gen.Ref, message any) {
	w.SendResponse(from, ref, message)
}

func (w *Worker) HandleMessageAlias(alias gen.Alias, from gen.PID, message any) error {
	w.respond(from, gen.Ref{}, Response{Error: fmt.Errorf("late: %w", ErrNotFound)}) // want `A2020.*this error is handed to respond, which sends it`
	return nil
}

// A helper that builds the value is followed through its fact.
func readError(err error) error {
	return fmt.Errorf("read: %w", err)
}

func (w *Worker) HandleEvent(message gen.MessageEvent) error {
	w.Send(w.peer, Response{Error: readError(ErrNotFound)}) // want `A2020.*readError builds it with fmt.Errorf`
	return nil
}

// A value that never leaves this process is nobody's business.
func local() error {
	return fmt.Errorf("local: %w", ErrNotFound)
}

// A recorded exception is silent.
func (w *Worker) Terminate(reason error) {
	//argus:allow A2020 the peer only logs this one
	w.Send(w.peer, Response{Error: fmt.Errorf("bye: %w", ErrNotFound)})
}
