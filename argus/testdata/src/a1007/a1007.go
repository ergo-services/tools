package a1007

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"

	"shared"
)

type MessageFrame struct {
	Seq     int64
	Payload []byte
}

type Worker struct {
	act.Actor
}

func factoryWorker() gen.ProcessBehavior { return &Worker{} }

type Reader struct {
	act.Actor
	buf *shared.Buffer
	pid gen.PID
}

func (r *Reader) HandleMessage(from gen.PID, message any) error {
	// the sibling that does not honour the invariant: a window into memory the
	// buffer keeps mutating goes out as a message
	r.Send(r.pid, MessageFrame{Payload: r.buf.Peek(16)}) // want `\[tier1\] \[A1007\] Peek returns memory derived from its receiver's data field`

	// the consumer that copies is the fix, and it is silent
	r.Send(r.pid, MessageFrame{Payload: r.buf.Copy(16)})

	// directly in the payload position rather than inside a literal
	r.Send(r.pid, r.buf.Peek(16)) // want `A1007.*Peek returns memory derived from its receiver's data field`

	// the address of an element is the same escape by pointer
	r.Send(r.pid, r.buf.Head()) // want `A1007.*Head returns memory derived from its receiver's data field`

	// a value result carries no alias
	r.Send(r.pid, MessageFrame{Seq: int64(r.buf.Len())})

	// a conversion preserves the alias
	r.Send(r.pid, string(r.buf.Peek(16))) // want `A1007.*Peek returns memory derived`

	// a spawn argument crosses the same boundary
	r.Spawn(factoryWorker, gen.ProcessOptions{}, r.buf.Peek(16)) // want `A1007.*Peek returns memory derived`
	return nil
}

// The one bounded dataflow exception: a local assigned exactly once. The diagnostic
// points at the escape rather than at the send, which is where the copy belongs.
func (r *Reader) HandleEvent(event gen.MessageEvent) error {
	frame := r.buf.Peek(16) // want `A1007.*Peek returns memory derived`
	r.Send(r.pid, MessageFrame{Payload: frame})
	return nil
}

// A local assigned twice is not resolvable, so the rule stays quiet rather than
// guessing which assignment reached the send.
func (r *Reader) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	frame := r.buf.Copy(16)
	frame = r.buf.Peek(16)
	r.Send(r.pid, MessageFrame{Payload: frame})
	return nil, nil
}

// A buffer allocated and abandoned in this callback owns nothing the sender keeps,
// so handing over its bytes is fine.
func (r *Reader) Terminate(reason error) {
	var local shared.Buffer
	r.Send(r.pid, MessageFrame{Payload: local.Peek(16)})
}

// A recorded exception is silent.
type Recorded struct {
	act.Actor
	buf *shared.Buffer
	pid gen.PID
}

func (r *Recorded) HandleMessage(from gen.PID, message any) error {
	//argus:allow A1007 the receiver copies before the next Advance
	r.Send(r.pid, MessageFrame{Payload: r.buf.Peek(16)})
	return nil
}
