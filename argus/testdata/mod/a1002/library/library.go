// Package library is a requirement of the analysed module. Whether its channel
// operations can block rests on invariants of the library, so they do not travel to a
// caller in another module; a configured surface such as time.Sleep does.
package library

import "time"

type Pool struct {
	idle    chan int
	waiters []chan error
}

func (p *Pool) Put(conn int) { p.idle <- conn }

func (p *Pool) Get() int { return <-p.idle }

func (p *Pool) Notify() {
	for _, w := range p.waiters {
		w <- nil
	}
}

func (p *Pool) Close() {
	p.Notify()
	close(p.idle)
}

func Backoff() { time.Sleep(time.Second) }
