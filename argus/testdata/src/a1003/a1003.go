package a1003

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type RequestState struct{}

type MessageTick struct{}

type Worker struct {
	act.Actor
	peer  gen.PID
	other gen.Atom
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	// The registered name form: routed to this process's own mailbox while this
	// process waits for the reply.
	w.Call(w.Name(), RequestState{}) // want `\[tier2\] \[A1003\] this request is addressed to w.Name\(\), which is this process's own registered name`

	// The same through the priority and timeout variants.
	w.CallWithTimeout(w.Name(), RequestState{}, 3) // want `A1003.*own registered name`

	// A ProcessID built from own name and own node.
	w.Call(gen.ProcessID{Name: w.Name(), Node: w.Node().Name()}, RequestState{}) // want `A1003.*own ProcessID`

	// The other way of naming own node.
	w.CallProcessID(gen.ProcessID{Name: w.Name(), Node: w.PID().Node}, RequestState{}, 3) // want `A1003.*own ProcessID`

	// A same-named counterpart on a peer node is an ordinary remote call.
	w.Call(gen.ProcessID{Name: w.Name(), Node: "peer@localhost"}, RequestState{})

	// A ProcessID with no Node is remote too, and fails with a connection error
	// rather than timing out, so it is not this rule's business.
	w.Call(gen.ProcessID{Name: w.Name()}, RequestState{})

	// Another process's name is the normal case.
	w.Call(w.other, RequestState{})

	// A self send is a documented idiom: it defers work to the next mailbox pass
	// instead of waiting for it.
	w.Send(w.Name(), MessageTick{})
	w.Send(w.PID(), MessageTick{})

	// A request to a peer is what the API is for.
	w.Call(w.peer, RequestState{})
	return nil
}

func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	// The alias form, through the accessor.
	w.CallAlias(w.Aliases()[0], RequestState{}, 3) // want `A1003.*own alias`

	// And through a local with exactly one assignment.
	alias, _ := w.CreateAlias()
	w.CallAlias(alias, RequestState{}, 3) // want `A1003.*own alias`
	return nil, nil
}

// A name the receiver registers is deliberately NOT reported: ownership may be
// dynamic, and a follower calling that name is correct code.
const workerName gen.Atom = "worker"

func (w *Worker) Init(args ...any) error {
	w.RegisterName(workerName)
	w.Call(workerName, RequestState{})
	return nil
}

// A goroutine body is a different frame, and A1004 and A1010 own it.
func (w *Worker) Terminate(reason error) {
	go func() {
		w.Call(w.Name(), RequestState{})
	}()
}

// A recorded exception is silent.
func (w *Worker) HandleEvent(event gen.MessageEvent) error {
	//argus:allow A1003 the name is rebound to a sibling before this runs
	w.Call(w.Name(), RequestState{})
	return nil
}
