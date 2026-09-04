// Package probe exercises the model surfaces without asserting any diagnostic, so
// a probe analyzer can read the model without inheriting another rule's want set.
package probe

import (
	"sync"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessageValue struct{ ID int64 }
type MessageSlice struct{ Items []string }
type MessageGuarded struct{ Conns *sync.Map }

type Worker struct {
	act.Actor
	pid   gen.PID
	items []string
}

func (w *Worker) Init(args ...any) error {
	w.Send(w.pid, MessageValue{ID: 1})
	return nil
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	w.Send(w.pid, MessageValue{ID: 2})
	w.Send(w.pid, MessageSlice{Items: w.items})
	w.Send(w.pid, MessageGuarded{})
	w.SendEvent("ev", gen.Ref{}, MessageValue{ID: 3})
	w.Call(w.pid, MessageValue{ID: 4})
	return nil
}

func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	w.SendResponse(from, ref, MessageValue{ID: 5})
	return MessageValue{ID: 6}, nil
}

func (w *Worker) Terminate(reason error) {}
