// Package service is the analysed module. A1002 has to tell a channel operation of
// its own, which travels to every callback that reaches it, from one inside a
// requirement, which does not, the same way a mutex does not.
package service

import (
	"example.com/library"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Worker struct {
	act.Actor
	pool  *library.Pool
	local chan int
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	w.pool.Put(1)
	_ = w.pool.Get()
	w.pool.Close()

	library.Backoff() // want `A1002.*HandleMessage reaches Backoff, which waits with no bound \(sleep\)`
	w.drain()         // want `A1002.*HandleMessage reaches drain, which waits with no bound \(channel receive\)`
	return nil
}

func (w *Worker) drain() { <-w.local }
