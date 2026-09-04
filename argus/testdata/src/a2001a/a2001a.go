package a2001a

import (
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessageTick struct{}

type MessageBye struct{}

type RequestFlush struct{}

type Worker struct {
	act.Actor
	pid   gen.PID
	event gen.Event
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	// a one shot cancel fires once and accumulates nothing, so dropping it is fine
	w.SendAfter(w.pid, MessageTick{}, time.Second)
	_, _ = w.SendAfter(w.pid, MessageTick{}, time.Second)

	// a periodic cancel is the only way to stop the ticker
	w.SendEvery(w.pid, MessageTick{}, time.Second) // want `\[tier2\] \[A2001a\] the CancelFunc from SendEvery is discarded`

	// blanking both results is the same discard written differently
	_, _ = w.SendWithPriorityEvery(w.pid, MessageTick{}, gen.MessagePriorityHigh, time.Second) // want `A2001a.*the CancelFunc from SendWithPriorityEvery is discarded`

	// keeping the cancel is the point
	cancel, err := w.SendEvery(w.pid, MessageTick{}, time.Second)
	if err == nil {
		defer cancel()
	}

	// an event buffer discard belongs to A2013, whose premise is the producer's
	// buffer size, so this rule stays out of it
	w.MonitorEvent(w.event)

	// an ordinary send in a running callback can fail for ordinary reasons, which is
	// not what this rule is about
	w.Send(w.pid, MessageBye{})
	return nil
}

// Terminate is where a discarded error hides a call that cannot work at all.
func (w *Worker) Terminate(reason error) {
	// the classic dead cleanup loop
	w.DemonitorEvent(w.event) // want `A2001a.*the error from DemonitorEvent is discarded in Terminate`
	w.RegisterName("late")    // want `A2001a.*the error from RegisterName is discarded in Terminate`

	// Send works on the terminate path, so discarding its error is ordinary
	w.Send(w.pid, MessageBye{})

	// SetEnv is gated but returns nothing, so there is no discarded result to see:
	// A2001 reports the call itself instead
	w.SetEnv("phase", "done")

	// checking the error is the fix, and it is silent
	if _, err := w.Call(w.pid, RequestFlush{}); err != nil {
		w.Log().Error("flush refused: %s", err)
	}

	// a recorded exception is silent
	//argus:allow A2001a the node is going down anyway
	w.RegisterName("recorded")
}
