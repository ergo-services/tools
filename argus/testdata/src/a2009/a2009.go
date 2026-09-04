package a2009

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/net/edf"
)

type MessageOK struct{ ID int }

// MessageHalfPair implements the marshal half only, which registration rejects
// with "must be a method of *T".
type MessageHalfPair struct{ ID int } // want `\[tier2\] \[A2009\] MessageHalfPair implements MarshalEDF without a pointer receiver UnmarshalEDF`

func (m MessageHalfPair) MarshalEDF(b []byte) ([]byte, error) { return b, nil }

// MessageValueUnmarshal puts Unmarshal on the value, so the decoder cannot write
// through it and registration refuses the type.
type MessageValueUnmarshal struct{ ID int } // want `A2009.*MessageValueUnmarshal.UnmarshalEDF has a value receiver`

func (m MessageValueUnmarshal) MarshalEDF(b []byte) ([]byte, error) { return b, nil }
func (m MessageValueUnmarshal) UnmarshalEDF(b []byte) error         { return nil }

// A type nobody registers never reaches the pair check, so its half pair is not a
// defect: the rule reports what registration would refuse, and this one is never
// offered to it.
type LocalHalfPair struct{ ID int }

func (m LocalHalfPair) MarshalEDF(b []byte) ([]byte, error) { return b, nil }

// MessageGoodPair is the shape registration accepts.
type MessageGoodPair struct{ ID int }

func (m MessageGoodPair) MarshalEDF(b []byte) ([]byte, error) { return b, nil }
func (m *MessageGoodPair) UnmarshalEDF(b []byte) error        { return nil }

func init() {
	edf.RegisterTypeOf(MessageOK{}) // want `A2009.*edf.RegisterTypeOf registers into the package level table`
}

type App struct{}

func (a *App) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
	// an application Load is the documented place
	node.Network().RegisterType(MessageOK{})
	// The marshaler pair is only a defect for a type someone registers, so the two
	// broken ones are registered here: that is what makes them reach the check.
	node.Network().RegisterTypes([]any{
		MessageOK{},
		MessageGoodPair{},
		MessageHalfPair{},
		MessageValueUnmarshal{},
	})

	// a pointer is not supported by registration
	node.Network().RegisterType(&MessageOK{})         // want `A2009.*a pointer type is not supported by registration`
	node.Network().RegisterTypes([]any{&MessageOK{}}) // want `A2009.*a pointer type is not supported by registration`
	return gen.ApplicationSpec{}, nil
}

// The application group members. The framework caps a member's init budget at three
// times DefaultRequestTimeout and refuses to start the whole application above it,
// rather than shortening the wait.
func (a *App) load(node gen.Node) gen.ApplicationSpec {
	return gen.ApplicationSpec{
		Name: "worker_app",
		Group: []gen.ApplicationMemberSpec{
			{Name: "fast", Factory: nil},
			{Name: "slow", Factory: nil, Options: gen.ProcessOptions{InitTimeout: 15}},
			{Name: "toolong", Factory: nil, Options: gen.ProcessOptions{
				InitTimeout: 30, // want `A2009.*Options.InitTimeout is 30 on application group member "toolong", above the ceiling of 15 seconds`
			}},
		},
	}
}

func (a *App) Start(mode gen.ApplicationMode) {}

func (a *App) Terminate(reason error) {}

type Worker struct {
	act.Actor
}

// registering from an actor callback runs after a peer may already be connected
func (w *Worker) Init(args ...any) error {
	w.Node().Network().RegisterType(MessageOK{}) // want `A2009.*RegisterType in method Init runs after a peer may already have connected`
	return nil
}

// a helper named for registration is accepted, which is how the one frame down
// case stays quiet without a call graph
func registerTypes(node gen.Node) {
	node.Network().RegisterType(MessageOK{})
}
