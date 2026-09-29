// Package workers exercises the cross package path of A1010: nothing here is a
// callback, so no goroutine in it is reported here, and whether each function starts
// one with no boundary has to reach the calling package as a fact.
package workers

import "time"

// Start starts a goroutine with no boundary directly.
func Start() { go loop() }

// Run reaches one through a helper of its own.
func Run() { start() }

func start() { go loop() }

// Later starts one through time.AfterFunc, with a literal that defers nothing.
func Later() {
	time.AfterFunc(time.Second, func() { loop() })
}

// Safe installs the boundary in the goroutine body.
func Safe() {
	go func() {
		defer func() { _ = recover() }()
		loop()
	}()
}

// Accepted records the missing boundary as a decision, which does not travel.
func Accepted() {
	//argus:allow A1010 the loop is panic free by construction
	go loop()
}

// Go runs whatever the caller passes, so the verdict belongs at the caller.
func Go(f func()) { go f() }

// Pool is reached through a method, which carries the fact the same way.
type Pool struct{}

func (p *Pool) Spawn() { go loop() }

func loop() {}
