package a1004

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Worker struct {
	act.Actor
	counter int
	pid     gen.PID
}

func (w *Worker) Init(args ...any) error {
	// touching a field from a goroutine races the actor loop
	go func() {
		w.counter++ // want `\[tier1\] \[A1004\] goroutine started in Init captures actor state \(w.counter\)`
	}()
	return nil
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	// the framework handle is actor goroutine only, so this is not exempt
	go func() {
		w.Log().Info("from a goroutine") // want `A1004.*captures actor state \(w.Log\)`
	}()

	// passing the receiver whole is the same defect
	go work(w) // want `A1004.*captures actor state \(w\)`

	// a goroutine that captures only locals is silent
	n := w.counter
	go func() {
		compute(n)
	}()

	// a recorded exception is silent
	//argus:allow A1004 counter is only read here and the value is stale by design
	go func() {
		_ = w.counter
	}()
	return nil
}

func (w *Worker) Terminate(reason error) {}

func work(w *Worker) {}
func compute(n int)  {}
