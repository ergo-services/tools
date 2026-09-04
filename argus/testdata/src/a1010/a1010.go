package a1010

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Worker struct {
	act.Actor
	pid gen.PID
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	// no boundary: a panic here takes the whole node down
	go func() { // want `\[tier1\] \[A1010\] goroutine started in HandleMessage has no recover`
		doWork()
	}()

	// an inline deferred recover is the accepted suppression
	go func() {
		defer func() {
			if r := recover(); r != nil {
				_ = r
			}
		}()
		doWork()
	}()

	// a conditionally deferred recover counts, which is how the framework and the
	// extra library both write it
	go func() {
		if recoverEnabled() {
			defer func() {
				_ = recover()
			}()
		}
		doWork()
	}()

	// a deferred call to a function that recovers directly is also a boundary
	go func() {
		defer guard()
		doWork()
	}()

	// go f() where f installs its own boundary
	go guarded()

	// go f() where f does not
	go unguarded() // want `A1010.*goroutine started in HandleMessage has no recover`

	// a recover one frame deeper does not protect this frame
	go func() { // want `A1010.*goroutine started in HandleMessage has no recover`
		guarded()
	}()
	return nil
}

// A helper two frames down is still reachable from a callback, so the rule applies
// through the call graph rather than only at the callback body.
func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	w.schedule()
	return nil, nil
}

func (w *Worker) schedule() {
	go func() { // want `A1010.*goroutine started in a path reachable from a callback has no recover`
		doWork()
	}()
}

// A meta Start is a run loop, and the goroutines it starts are still application
// goroutines with no boundary of their own.
type Conn struct {
	gen.MetaProcess
}

func (c *Conn) Init(process gen.MetaProcess) error { return nil }

func (c *Conn) Start() error {
	go func() { // want `A1010.*goroutine started in Start has no recover`
		doWork()
	}()
	return nil
}

func (c *Conn) HandleMessage(from gen.PID, message any) error { return nil }

func (c *Conn) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (c *Conn) Terminate(reason error) {}

// A bootstrap path is not reported: the program failing to start is arguably the
// correct outcome there, and nothing reaches this from a callback.
func bootstrap() {
	go doWork()
}

// Two deliberate exceptions on one statement take two rule ids, because A1004 and
// A1010 are independent consequences of the same construct.
func (w *Worker) Terminate(reason error) {
	//argus:allow A1004,A1010 the goroutine only reads an immutable snapshot
	go func() {
		_ = w.pid
		doWork()
	}()
}

func guard() { _ = recover() }

func guarded() {
	defer guard()
	doWork()
}

func unguarded() { doWork() }

func doWork() {}

func recoverEnabled() bool { return true }
