package a3003

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessageRegistered struct{ ID int64 }

// Sent and never registered: it works locally and fails the first time it crosses a
// node boundary.
type MessageForgotten struct{ ID int64 } // want `\[tier3\] \[A3003\] MessageForgotten is used as a message here but is not in this package's registration list`

// Declared local by its author, so it is not supposed to be in the list.
//
//argus:message local
type MessageLocalOnly struct{ conn chan int }

type Worker struct {
	act.Actor
	pid gen.PID
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	w.Send(w.pid, MessageRegistered{})
	w.Send(w.pid, MessageForgotten{})
	w.Send(w.pid, MessageLocalOnly{})
	return nil
}

// The registration list this package writes. Its existence is what makes the rule
// speak at all: a package that registers nothing has its list elsewhere.
func registerTypes(node gen.Node) {
	node.Network().RegisterTypes([]any{MessageRegistered{}})
}
