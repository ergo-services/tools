package a3001

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// A payload with no marker is checked only where it is sent.
type MessageUnmarked struct { // want `\[tier3\] \[A3001\] MessageUnmarked is sent as a message but carries no //argus:message marker`
	ID int64
}

// A marked one is already in definition site checking.
//
//argus:message
type MessageMarked struct {
	ID int64
}

// So is one marked local.
//
//argus:message local
type MessageLocal struct {
	conn chan int
}

// Prose saying the type is local does not exempt it from A3001: until the marker is
// there the tool cannot act on the prose. A3006 is the rule that offers to convert it.
//
// MessageProse carries a handle that is same node only.
type MessageProse struct { // want `\[tier3\] \[A3001\] MessageProse is sent as a message but carries no`
	Handle chan int
}

// A type nobody sends is not a message, so A3001 says nothing about it.
type NotSent struct {
	ID int64
}

type Worker struct {
	act.Actor
	pid gen.PID
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	w.Send(w.pid, MessageUnmarked{})
	w.Send(w.pid, MessageMarked{})
	w.Send(w.pid, MessageLocal{})
	w.Send(w.pid, MessageProse{})

	// the same unmarked type sent twice is one finding, at its declaration
	w.Send(w.pid, MessageUnmarked{ID: 2})
	return nil
}
