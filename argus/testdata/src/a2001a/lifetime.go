package a2001a

import (
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessageReconcile struct{}

type MessagePoll struct{}

type MessageBeat struct{}

// One SendEvery armed in Init is the lifetime ticker: Init runs once per process start
// and the framework stops the ticker when the process dies. This used to be reported.
type Lifetime struct {
	act.Actor
	pid   gen.PID
	peers []gen.PID
}

func (l *Lifetime) Init(args ...any) error {
	_, _ = l.SendEvery(l.PID(), MessageReconcile{}, time.Second)
	l.arm()

	// a loop arms one ticker per iteration
	for _, peer := range l.peers {
		l.SendEvery(peer, MessageBeat{}, time.Second) // want `A2001a.*the CancelFunc from SendEvery is discarded in Init, which can run more than once per process start`
	}
	return nil
}

// reached only from Init, so it runs once as well
func (l *Lifetime) arm() {
	l.SendWithPriorityEvery(l.PID(), MessagePoll{}, gen.MessagePriorityHigh, time.Second)
}

// A helper reached from a handler runs once per message.
type Rearm struct {
	act.Actor
	pid gen.PID
}

func (r *Rearm) Init(args ...any) error {
	r.start()
	return nil
}

func (r *Rearm) HandleMessage(from gen.PID, message any) error {
	r.start()
	return nil
}

func (r *Rearm) start() {
	r.SendEvery(r.pid, MessagePoll{}, time.Second) // want `A2001a.*the CancelFunc from SendEvery is discarded in HandleMessage through start, which can run more than once`
}

// Armed once, but the same tick is armed again elsewhere, so the discarded one cannot be
// stopped when the other replaces it.
type Replaced struct {
	act.Actor
	pid    gen.PID
	cancel gen.CancelFunc
}

func (r *Replaced) Init(args ...any) error {
	r.SendEvery(r.pid, MessageReconcile{}, time.Second) // want `A2001a.*the CancelFunc from SendEvery is discarded, and the same tick is armed again by the SendEvery in Replaced.HandleCall`
	return nil
}

func (r *Replaced) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	cancel, err := r.SendEvery(r.pid, MessageReconcile{}, 2*time.Second)
	if err != nil {
		return nil, err
	}
	r.cancel = cancel
	return nil, nil
}
