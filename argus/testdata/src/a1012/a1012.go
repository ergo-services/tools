package a1012

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessageTick struct{}

type RequestState struct{}

type Worker struct {
	act.Actor
	peer gen.PID
}

// The direct forms.
func (w *Worker) HandleMessage(from gen.PID, message any) error {
	w.Node().Send(w.peer, MessageTick{}) // want `\[tier2\] \[A1012\] HandleMessage routes this message with Node\(\).Send`

	w.Node().Call(w.peer, RequestState{}) // want `\[tier1\] \[A1012\].*routes this message with Node\(\).Call`

	w.Node().SendExit(w.peer, gen.TerminateReasonShutdown) // want `A1012.*Node\(\).SendExit`

	// The process API is the fix.
	w.Send(w.peer, MessageTick{})
	w.Call(w.peer, RequestState{})

	// Reading something from the node is ordinary code: only the routing methods act
	// on the node's own behalf.
	_ = w.Node().Name()
	_ = w.Node().Uptime()
	w.Node().Log().Info("node uptime %d", w.Node().Uptime())
	return nil
}

// A helper that routes through a node handle it was given, reported where the handle
// is handed over.
func (w *Worker) HandleEvent(event gen.MessageEvent) error {
	broadcast(w.Node(), w.peer, MessageTick{}) // want `A1012.*hands the node handle to broadcast, which routes through it with Send`
	return nil
}

func broadcast(node gen.Node, to gen.PID, message any) {
	node.Send(to, message)
}

// A helper that takes a node without routing through it is not a node sender, which is
// the whole reason this needs a fact rather than a signature test.
func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	describe(w.Node())
	return nil, nil
}

func describe(node gen.Node) string {
	return string(node.Name())
}

// An application has no process of its own, so the node handle is the only thing it
// can send through and there is nothing to recommend instead.
type App struct {
	collector gen.PID
	node      gen.Node
}

func (a *App) PreLoad(args ...any) error { return nil }

func (a *App) Load(args ...any) error { return nil }

func (a *App) Terminate(reason error) {
	a.Node().Send(a.collector, reason)
}

func (a *App) Node() gen.Node { return a.node }

// Outside a callback there is no actor to leave in the wrong state: this is how a
// node-level program is meant to send.
func bootstrap(node gen.Node, to gen.PID) error {
	return node.Send(to, MessageTick{})
}

// A recorded exception is silent.
type Recorded struct {
	act.Actor
	peer gen.PID
}

func (r *Recorded) HandleMessage(from gen.PID, message any) error {
	//argus:allow A1012 this notification must outlive the process that triggered it
	r.Node().Send(r.peer, MessageTick{})
	return nil
}
