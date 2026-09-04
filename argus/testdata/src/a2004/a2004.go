package a2004

import (
	"fmt"
	"time"

	"ergo.services/ergo/gen"
)

// The shapes registration accepts.

type MessageWireOK struct {
	ID   int64
	Name string
	When time.Time
	Who  gen.PID
}

type MessageAny struct {
	ID      int64
	Payload any // any has a built in encoder
}

type MessageErr struct {
	Reason error // so does error
}

type MessageTagged struct {
	ID     int64
	secret string `edf:"-"` // cut from the wire graph explicitly
}

// The shapes registration refuses.

type MessageUnexported struct { // want `\[tier2\] \[A2004\] MessageUnexported is used as a wire message but registration rejects its shape`
	ID     int64
	secret string
}

type MessageChan struct { // want `A2004.*MessageChan is used as a wire message`
	Done chan struct{}
}

type MessageIface struct { // want `A2004.*MessageIface is used as a wire message`
	Printer fmt.Stringer
}

type MessageDoublePointer struct { // want `A2004.*MessageDoublePointer is used as a wire message`
	P **int
}

type MessageRecursive struct { // want `A2004.*MessageRecursive is used as a wire message`
	ID   int64
	Next *MessageRecursive
}

// A marshaler pair short circuits registration before any field walk, so the
// unexported field is not a defect here.
type MessageMarshaler struct {
	raw []byte
}

func (m MessageMarshaler) MarshalEDF(b []byte) ([]byte, error) { return b, nil }
func (m *MessageMarshaler) UnmarshalEDF(b []byte) error        { return nil }

// A type declared local by its author is exempt: axis W asks what registration
// would do, and nobody registers this one.
//
//argus:message local
type MessageLocalOnly struct {
	conn chan int
}

// A bare marker opts a type into definition site checking even though this package
// never registers it.
//
//argus:message
type MessageMarkedBad struct { // want `A2004.*MessageMarkedBad is used as a wire message`
	Build func() int
}

// An unregistered, unmarked type says nothing about the wire, so it stays quiet
// even though its shape would be refused.
type NotAMessage struct {
	Done chan struct{}
}

// A recorded exception is silent.
type MessageRecorded struct { // argus:allow A2004 the peer decodes it with a custom proto
	Done chan struct{}
}

func registerTypes(node gen.Node) {
	node.Network().RegisterTypes([]any{
		MessageWireOK{},
		MessageAny{},
		MessageErr{},
		MessageTagged{},
		MessageUnexported{},
		MessageChan{},
		MessageIface{},
		MessageDoublePointer{},
		MessageRecursive{},
		MessageMarshaler{},
		MessageRecorded{},
	})
}
