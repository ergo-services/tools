package a1002

import (
	"context"
	"sync"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Worker struct {
	act.Actor
	mu    sync.Mutex
	wg    sync.WaitGroup
	ready chan struct{}
	pid   gen.PID
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	// the naked constructs
	time.Sleep(time.Second) // want `\[tier1\] \[A1002\] sleep in HandleMessage waits with no bound`
	w.mu.Lock()             // want `A1002.*mutex in HandleMessage waits with no bound`
	w.mu.Unlock()
	w.wg.Wait() // want `A1002.*waitgroup in HandleMessage waits with no bound`

	// a bare channel receive
	<-w.ready // want `A1002.*a channel receive in HandleMessage waits with no bound`

	// a bare channel send
	w.ready <- struct{}{} // want `A1002.*a channel send in HandleMessage waits with no bound`
	return nil
}

func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	// a select with a default cannot wait at all
	select {
	case <-w.ready:
	default:
	}

	// a timeout arm bounds it
	select {
	case <-w.ready:
	case <-time.After(time.Second):
	}

	// so does a context-done arm
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case <-w.ready:
	case <-ctx.Done():
	}

	// TryLock cannot wait
	if w.mu.TryLock() {
		w.mu.Unlock()
	}

	// a framework Call is always bounded, by DefaultRequestTimeout when unset. This
	// is the case that made the blocking fact carry a bound.
	w.Call(w.pid, request)
	w.CallWithTimeout(w.pid, request, 3)
	return nil, nil
}

// A helper two frames down is reported at the call that reaches it.
func (w *Worker) HandleEvent(event gen.MessageEvent) error {
	w.drain() // want `A1002.*HandleEvent reaches drain, which waits with no bound`
	return nil
}

func (w *Worker) drain() {
	<-w.ready
}

// A goroutine is a different mailbox, so its wait is not this rule's business.
func (w *Worker) Terminate(reason error) {
	go func() {
		time.Sleep(time.Second)
	}()
}

// A recorded exception is silent.
func (w *Worker) HandleInspect(item ...string) map[string]string {
	//argus:allow A1002 the lock is held for a single map read and never contended
	w.mu.Lock()
	w.mu.Unlock()
	return nil
}

// A meta run loop is expected to block, so Start is excluded.
type Conn struct {
	gen.MetaProcess
	ready chan struct{}
}

func (c *Conn) Init(process gen.MetaProcess) error { return nil }

func (c *Conn) Start() error {
	for {
		<-c.ready
	}
}

func (c *Conn) HandleMessage(from gen.PID, message any) error { return nil }

func (c *Conn) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (c *Conn) Terminate(reason error) {}
