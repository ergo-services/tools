package a2021

import (
	"time"

	"ergo.services/ergo/gen"
)

// A named string is a distinct type from string, so registration needs its own
// entry for it exactly as it does for a struct.
type Status string

type Fee struct {
	Amount int64
	Kind   Status
}

type Inner struct {
	Value int64
}

type Order struct {
	ID      int64
	Name    string
	When    time.Time
	Who     gen.PID
	Status  Status
	Fees    []Fee
	Inner   Inner
	Payload map[Status]int64
	Secret  Status `edf:"-"`
}

// A marshaler pair short circuits the field walk, so its fields need nothing.
type Opaque struct {
	raw []byte
}

func (o Opaque) MarshalEDF(b []byte) ([]byte, error) { return b, nil }
func (o *Opaque) UnmarshalEDF(b []byte) error        { return nil }

type Envelope struct {
	Body Opaque
}

func register(node gen.Node) {
	node.Network().RegisterTypes([]any{
		Order{}, // want `\[tier2\] \[A2021\] a2021.Order is registered here but a2021.Status` `A2021.*a2021.Order is registered here but a2021.Fee`
		Inner{},
		Envelope{},
	})
}

// A type nobody registers says nothing about the wire.
type NotRegistered struct {
	Status Status
}
