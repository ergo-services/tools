package a1010

import (
	"sync"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"golang.org/x/sync/errgroup"

	"workers"
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
	time.AfterFunc(time.Second, func() { doWork() })
	workers.Start()
}

// A goroutine is also started by anything that runs a function argument on one of its
// own, and the boundary then has to be in that argument.
type Timers struct {
	act.Actor
	job    func()
	runner runner
}

type runner interface{ Run() }

func (t *Timers) HandleMessage(from gen.PID, message any) error {
	time.AfterFunc(time.Second, func() { // want `\[tier1\] \[A1010\] HandleMessage hands this func literal to time.AfterFunc, which runs it on a goroutine of its own, and this func literal defers no recover`
		doWork()
	})
	time.AfterFunc(time.Second, func() {
		defer guard()
		doWork()
	})
	time.AfterFunc(time.Second, unguarded) // want `\[tier1\] \[A1010\] HandleMessage hands unguarded to time.AfterFunc`
	time.AfterFunc(time.Second, guarded)

	var wg sync.WaitGroup
	wg.Go(func() { doWork() }) // want `\[tier1\] \[A1010\] HandleMessage hands this func literal to WaitGroup.Go`

	var g errgroup.Group
	g.Go(func() error { return nil }) // want `\[tier1\] \[A1010\] HandleMessage hands this func literal to Group.Go`
	g.TryGo(func() error {
		defer guard()
		return nil
	})

	// a func value whose body does not resolve here is unknown rather than known bad
	time.AfterFunc(time.Second, t.job) // want `\[tier2\] \[A1010\] HandleMessage hands this func value to time.AfterFunc`
	fn := t.job
	time.AfterFunc(time.Second, fn)           // want `\[tier2\] \[A1010\] HandleMessage hands this func value to time.AfterFunc`
	time.AfterFunc(time.Second, t.runner.Run) // want `\[tier2\] \[A1010\] HandleMessage hands Run to time.AfterFunc`

	// a helper of this package that does go f() with its own parameter is checked
	// on what each caller passes, and so is one that forwards to it
	spawn(func() { doWork() }) // want `\[tier1\] \[A1010\] HandleMessage hands this func literal to spawn`
	spawn(guarded)
	spawnLater(unguarded) // want `\[tier1\] \[A1010\] HandleMessage hands unguarded to spawnLater`

	// the same verdict arrives from another package as a fact
	workers.Go(func() { doWork() }) // want `\[tier1\] \[A1010\] HandleMessage hands this func literal to workers.Go`
	workers.Go(guarded)
	return nil
}

func spawn(f func()) {
	go f()
}

func spawnLater(f func()) { spawn(f) }

// A call into another package that starts a goroutine with no boundary is reported at
// the call, because the go statement itself is outside this package and no callback
// there reaches it.
type Client struct {
	act.Actor
	pool *workers.Pool
}

func (c *Client) HandleMessage(from gen.PID, message any) error {
	workers.Start() // want `\[tier1\] \[A1010\] HandleMessage calls workers.Start, which starts a goroutine with no recover \(workers.go:9\)`
	workers.Run()   // want `\[tier1\] \[A1010\] HandleMessage calls workers.Run, which reaches, through workers.start, a goroutine with no recover`
	workers.Later() // want `\[tier1\] \[A1010\] HandleMessage calls workers.Later, which starts a goroutine with no recover`
	c.pool.Spawn()  // want `\[tier1\] \[A1010\] HandleMessage calls Pool.Spawn, which starts a goroutine with no recover`
	workers.Safe()
	workers.Accepted()
	c.relay()
	return nil
}

// A helper of this package reaching the same call is reported there, as a path
// reachable from a callback.
func (c *Client) relay() {
	workers.Start() // want `\[tier1\] \[A1010\] a path reachable from a callback calls workers.Start`
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
