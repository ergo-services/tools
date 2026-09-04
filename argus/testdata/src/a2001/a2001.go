package a2001

import (
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessageBye struct{}

type RequestFlush struct{}

type Worker struct {
	act.Actor
	pid   gen.PID
	event gen.Event
	token gen.Ref
}

// Terminate runs with the state already Terminated on the normal path.
func (w *Worker) Terminate(reason error) {
	// the send family still works there
	w.Send(w.pid, MessageBye{})
	w.SendPID(w.pid, MessageBye{})
	w.SendWithPriority(w.pid, MessageBye{}, gen.MessagePriorityHigh)
	w.SendExit(w.pid, reason)

	// everything else does not
	w.Call(w.pid, RequestFlush{})                 // want `\[tier2\] \[A2001\] Call returns ErrNotAllowed on the normal termination path`
	w.SendEvent("ticks", w.token, MessageBye{})   // want `A2001.*SendEvent returns ErrNotAllowed`
	w.DemonitorEvent(w.event)                     // want `A2001.*DemonitorEvent returns ErrNotAllowed`
	w.SendAfter(w.pid, MessageBye{}, time.Second) // want `A2001.*SendAfter returns ErrNotAllowed`
	w.RegisterName("late")                        // want `A2001.*RegisterName returns ErrNotAllowed`
	w.SendResponse(w.pid, w.token, MessageBye{})  // want `A2001.*SendResponse returns ErrNotAllowed`
	w.SpawnMeta(&Conn{}, gen.MetaOptions{})       // want `A2001.*SpawnMeta returns ErrNotAllowed`

	// gated and silent: the caller cannot even see it fail
	w.SetEnv("phase", "done") // want `A2001.*SetEnv is silently ignored, and it returns nothing`
}

// A running callback is the state these methods exist for, so nothing is reported.
func (w *Worker) HandleMessage(from gen.PID, message any) error {
	w.Call(w.pid, RequestFlush{})
	w.SendEvent("ticks", w.token, MessageBye{})
	w.RegisterName("worker")
	w.SetEnv("phase", "running")
	return nil
}

// A recorded exception is silent.
func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

// Conn is a meta process. Its Init runs before Start, with the state still zero.
type Conn struct {
	gen.MetaProcess
	parent gen.PID
	ref    gen.Ref
}

func (c *Conn) Init(process gen.MetaProcess) error {
	c.MetaProcess = process

	// these work before Start
	c.Send(c.parent, MessageBye{})
	c.SendWithPriority(c.parent, MessageBye{}, gen.MessagePriorityHigh)
	c.Spawn(&Conn{}, gen.MetaOptions{})

	// these require Running
	c.SendResponse(c.parent, c.ref, MessageBye{})        // want `A2001.*SendResponse requires the meta to be Running`
	c.SendResponseError(c.parent, c.ref, gen.ErrUnknown) // want `A2001.*SendResponseError requires the meta to be Running`
	c.SetSendPriority(gen.MessagePriorityHigh)           // want `A2001.*SetSendPriority requires the meta to be Running`
	c.SetCompression(true)                               // want `A2001.*SetCompression requires the meta to be Running`
	return nil
}

// By the time Start runs the meta is Running, so the same calls are fine.
func (c *Conn) Start() error {
	c.SendResponse(c.parent, c.ref, MessageBye{})
	c.SetCompression(true)
	return nil
}

func (c *Conn) HandleMessage(from gen.PID, message any) error { return nil }

func (c *Conn) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (c *Conn) Terminate(reason error) {}

// A recorded exception on a terminate-path call is silent.
type Recorded struct {
	act.Actor
	event gen.Event
}

func (r *Recorded) Terminate(reason error) {
	//argus:allow A2001 the node is going down anyway and the log line is enough
	r.DemonitorEvent(r.event)
}
