package a2009

import (
	"errors"

	"a2009/wire"

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

var errSentinel = errors.New("sentinel")

// The package level entry points work from init and nothing fails at startup, so they
// are a deprecation hint rather than a failure: EDF alone learns the type, which only
// matters on a node that runs another wire protocol beside ENP.
func init() {
	edf.RegisterTypeOf(MessageOK{}) // want `\[tier3\] \[A2009\] edf.RegisterTypeOf is deprecated and prints a deprecation notice on first use; it registers with EDF alone, so on a node that runs another wire protocol beside ENP only EDF knows the type; list it in ApplicationSpec.Network.RegisterTypes, or call Network\(\).RegisterType in an application Load`
	edf.RegisterError(errSentinel)  // want `\[tier3\] \[A2009\] edf.RegisterError is deprecated .* only EDF knows the error; list it in ApplicationSpec.Network.RegisterErrors, or call Network\(\).RegisterError`

	// what the table refuses is still a startup failure, whichever entry point
	edf.RegisterTypeOf(&MessageOK{}) // want `\[tier2\] \[A2009\] a pointer type is not supported by registration` `\[tier3\] \[A2009\] edf.RegisterTypeOf is deprecated`
}

type App struct{}

func (a *App) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
	// an application Load is the documented place
	node.Network().RegisterType(MessageOK{})
	// and so it is for a helper Load calls, here or in another package
	RegisterTypes(node)
	wire.Register(node)
	(&codec{}).install(node)
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
	edf.RegisterTypeOf(MessageOK{})              // want `\[tier2\] \[A2009\] edf.RegisterTypeOf in method Init runs after a peer may already have connected` `\[tier3\] \[A2009\] edf.RegisterTypeOf is deprecated`
	return nil
}

// through a helper it is the same defect, reported where the callback calls it
func (w *Worker) HandleMessage(from gen.PID, message any) error {
	registerTypes(w.Node()) // want `\[tier2\] \[A2009\] registerTypes, called from method HandleMessage, registers types through Network\(\).RegisterType after a peer may already have connected`
	setupAll(w.Node())      // want `A2009.*setupAll, called from method HandleMessage, registers types through Network\(\).RegisterType`
	wire.Register(w.Node()) // want `A2009.*Register, called from method HandleMessage, registers types through Network\(\).RegisterTypes`
	RegisterTypes(w.Node()) // want `A2009.*RegisterTypes, called from method HandleMessage`
	wire.Describe(w.Node())
	return nil
}

// A helper's body is not judged: whoever calls it decides when it runs, so it
// carries a fact to its call sites instead, whatever its name.
func registerTypes(node gen.Node) { // want registerTypes:"registers through Network\\(\\).RegisterType"
	node.Network().RegisterType(MessageOK{})
}

// RegisterTypes is the exported form of the same helper.
func RegisterTypes(node gen.Node) error { // want RegisterTypes:"registers through Network\\(\\).RegisterTypes"
	return node.Network().RegisterTypes([]any{MessageOK{}})
}

// and a helper that only calls another one carries it too
func setupAll(node gen.Node) { // want setupAll:"registers through Network\\(\\).RegisterType"
	registerTypes(node)
}

type codec struct{}

func (c *codec) install(node gen.Node) { // want install:"registers through Network\\(\\).RegisterType"
	node.Network().RegisterType(MessageOK{})
}
