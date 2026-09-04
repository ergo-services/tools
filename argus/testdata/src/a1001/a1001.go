package a1001

import (
	"sync"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"

	"shared"
)

type MessageValues struct {
	ID    int64
	Name  string
	When  time.Time
	Where gen.PID
}

type MessageSlice struct {
	ID    int64
	Items []string
}

type MessageMap struct {
	Meta map[string]string
}

type MessageNested struct {
	ID    int64
	Inner Inner
}

type Inner struct {
	Tags []string
}

type MessagePointer struct {
	Cache *shared.Unguarded
}

type MessageGuarded struct {
	Cache *shared.Guarded
}

type MessageSyncMap struct {
	Conns *sync.Map
}

type MessageLeaky struct {
	Store *shared.Leaky
}

type MessageBytes struct {
	Payload []byte
}

type MessageChan struct {
	Done chan struct{}
}

type Worker struct {
	act.Actor
	pid gen.PID
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	// value only payloads are silent
	w.Send(w.pid, MessageValues{ID: 1, Name: "x"})

	// a slice hands over the backing array
	w.Send(w.pid, MessageSlice{ID: 1}) // want `\[tier1\] \[A1001\] a1001.MessageSlice is sent as a message and shares memory`

	// a map is shared wholesale
	w.Send(w.pid, MessageMap{}) // want `A1001.*MessageMap`

	// the reference is one level down
	w.Send(w.pid, MessageNested{}) // want `A1001.*MessageNested`

	// a pointer to a type with plain maps and no lock
	w.Send(w.pid, MessagePointer{}) // want `A1001.*MessagePointer`

	// a channel can never be shared safely
	w.Send(w.pid, MessageChan{}) // want `A1001.*MessageChan`

	// a mutex bearing pointee is deliberately shared, so it stays silent
	w.Send(w.pid, MessageGuarded{})

	// sync.Map is safe by construction
	w.Send(w.pid, MessageSyncMap{})

	// a mutex does not cover an exported reference field reached directly
	w.Send(w.pid, MessageLeaky{}) // want `A1001.*MessageLeaky`

	// []byte is allowlisted as a payload by convention
	w.Send(w.pid, MessageBytes{})

	// a recorded exception is silent
	//argus:allow A1001 the receiver owns the slice from here on
	w.Send(w.pid, MessageSlice{ID: 2})

	// a blanket ignore is silent too
	w.Send(w.pid, MessageMap{}) //argus:ignore migrating this path
	return nil
}

func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	// a send outside the payload position must not be mistaken for one
	w.SendResponse(from, ref, MessageValues{ID: 2})
	return MessageValues{ID: 3}, nil
}
