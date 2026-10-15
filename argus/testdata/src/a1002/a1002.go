package a1002

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/app"
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

// A channel made in the same function with room for every send cannot block on them,
// which is how a semaphore or a one shot reply channel starts out. Anything beyond the
// room, or anything after the channel is shared, is the general case again.
type Fresh struct {
	act.Actor
	size int
	out  chan int
	mu   chan struct{}
}

func (f *Fresh) HandleMessage(from gen.PID, message any) error {
	done := make(chan error, 1)
	done <- nil

	pair := make(chan int, 2)
	pair <- 1
	pair <- 2
	pair <- 3 // want `A1002.*a channel send in HandleMessage waits with no bound`

	shared := make(chan int, 1)
	f.out = shared
	shared <- 1 // want `A1002.*a channel send in HandleMessage waits with no bound`

	looped := make(chan int, 1)
	for i := 0; i < 2; i++ {
		looped <- i // want `A1002.*a channel send in HandleMessage waits with no bound`
	}

	bare := make(chan int)
	bare <- 1 // want `A1002.*a channel send in HandleMessage waits with no bound`

	sized := make(chan int, f.size)
	sized <- 1 // want `A1002.*a channel send in HandleMessage waits with no bound`

	captured := make(chan int, 1)
	go func() { <-captured }()
	captured <- 1 // want `A1002.*a channel send in HandleMessage waits with no bound`

	handed := make(chan int, 1)
	handed <- f.keep(handed) // want `A1002.*a channel send in HandleMessage waits with no bound`

	// the helper only fills a fresh channel of its own, so calling it waits for nothing
	f.handshake()
	return nil
}

func (f *Fresh) keep(ch chan int) int {
	f.out = ch
	return 0
}

func (f *Fresh) handshake() {
	mu := make(chan struct{}, 1)
	mu <- struct{}{}
	f.mu = mu
}

// A func literal that is only stored runs later, on whoever calls it, so its body is
// not the callback's. A literal called in place, deferred, bound to a local that is only
// called, or handed to a caller that runs it before returning, is.
type Stored struct {
	act.Actor
	ready    chan struct{}
	onClose  func()
	handlers map[string]func()
	items    []int
	seen     sync.Map
}

func (s *Stored) Init(args ...any) error {
	s.onClose = func() {
		<-s.ready
		time.Sleep(time.Second)
	}
	s.handlers["drain"] = func() { s.drain() }
	s.register(func() { s.ready <- struct{}{} })
	time.AfterFunc(time.Second, func() { s.ready <- struct{}{} })
	s.arm()

	func() { <-s.ready }() // want `A1002.*a channel receive in Init waits with no bound`
	defer func() {
		s.ready <- struct{}{} // want `A1002.*a channel send in Init waits with no bound`
	}()

	wait := func() { <-s.ready } // want `A1002.*a channel receive in Init waits with no bound`
	wait()

	sort.Slice(s.items, func(i, j int) bool {
		s.drain() // want `A1002.*Init reaches drain, which waits with no bound`
		return false
	})
	slices.SortFunc(s.items, func(a, b int) int {
		time.Sleep(time.Millisecond) // want `A1002.*sleep in Init waits with no bound`
		return 0
	})
	s.seen.Range(func(k, v any) bool {
		<-s.ready // want `A1002.*a channel receive in Init waits with no bound`
		return true
	})
	return nil
}

func (s *Stored) register(handler func()) { s.onClose = handler }

// arm stores a closure and returns, so a callback calling it is not waiting.
func (s *Stored) arm() {
	s.onClose = func() { <-s.ready }
}

func (s *Stored) drain() {
	<-s.ready
}

// A select is bounded by its escape arm, and the arm is recognized by its type: any
// channel of time.Time, a Timer's or a Ticker's C included, and a context's Done. A
// method that only happens to be called Done is not a deadline.
type Timed struct {
	act.Actor
	ready  chan struct{}
	values chan int
	stop   stopper
}

type stopper struct{ ch chan struct{} }

func (s stopper) Done() <-chan struct{} { return s.ch }

func (t *Timed) HandleMessage(from gen.PID, message any) error {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-t.ready:
	case <-timer.C:
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	select {
	case v := <-t.values:
		_ = v
	case now := <-ticker.C:
		_ = now
	}

	var deadline <-chan time.Time = time.After(time.Second)
	select {
	case <-t.ready:
	case <-deadline:
	}

	select {
	case <-t.ready: // want `A1002.*a channel receive in HandleMessage waits with no bound`
	case <-t.stop.Done(): // want `A1002.*a channel receive in HandleMessage waits with no bound`
	}
	return nil
}

// An escape arm bounds the communication of a clause and nothing else. Its operands
// are evaluated on entering the select, and its body runs once the select has chosen,
// so a wait in either is ordinary code.
type Clauses struct {
	act.Actor
	in    chan int
	out   chan int
	ready chan struct{}
}

func (c *Clauses) HandleMessage(from gen.PID, message any) error {
	select {
	case v := <-c.in:
		c.out <- v // want `A1002.*a channel send in HandleMessage waits with no bound`
		select {
		case <-c.ready:
		default:
		}
	case <-time.After(time.Second):
		<-c.ready // want `A1002.*a channel receive in HandleMessage waits with no bound`
	}

	select {
	case c.out <- <-c.in: // want `A1002.*a channel receive in HandleMessage waits with no bound`
	default:
	}
	return nil
}

// An application has no mailbox: Init and Start run on a goroutine of their own under
// deadlines of their own, and Terminate runs as the application unwinds. None of it
// can stop a mailbox.
type Service struct {
	app.Application
	ready chan struct{}
}

func (s *Service) Init(ref gen.Ref, mode gen.ApplicationMode) error {
	<-s.ready
	s.connect()
	return nil
}

func (s *Service) Start(ref gen.Ref, mode gen.ApplicationMode) {
	time.Sleep(time.Second)
}

func (s *Service) Terminate(reason error) {
	s.ready <- struct{}{}
	s.connect()
}

func (s *Service) connect() {
	<-s.ready
}
