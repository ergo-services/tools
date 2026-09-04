package a2012

import (
	"encoding/json"
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessageWork struct{ ID int64 }

type MessageIdle struct{}

type RequestState struct{}

type Worker struct {
	act.Actor
	peer  gen.PID
	store gen.PID
	token gen.Ref
	count int
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	// The canonical form: a transient send failure logged and then returned, which
	// ends the process and makes the supervisor restart it.
	if err := w.Send(w.peer, message); err != nil {
		w.Log().Error("send to %s failed: %s", w.peer, err)
		return err // want `\[tier2\] \[A2012\] HandleMessage logs this failure at Error level and then returns it as a termination reason \(Send\)`
	}

	// A bare return with no log is the documented let-it-crash, and stays silent.
	if err := w.Send(w.store, message); err != nil {
		return err
	}

	// Returning nil after logging is the fix this rule recommends. Firing here would
	// be fatal to adoption.
	if err := w.Send(w.store, MessageIdle{}); err != nil {
		w.Log().Error("send failed: %s", err)
		return nil
	}

	// The wrong log level is deliberately silent: a project that logs terminations at
	// Warning is not spammed.
	if err := w.Send(w.peer, MessageIdle{}); err != nil {
		w.Log().Warning("send failed: %s", err)
		return err
	}

	// One statement between the log and the return breaks the adjacency gate.
	if err := w.Send(w.peer, MessageIdle{}); err != nil {
		w.Log().Error("send failed: %s", err)
		w.count++
		return err
	}

	// Unknown provenance: there is no evidence the process could have continued.
	var v map[string]any
	if err := json.Unmarshal([]byte("{}"), &v); err != nil {
		w.Log().Error("decode failed: %s", err)
		return err
	}

	// A multi-result call still resolves the error slot.
	if _, err := w.Call(w.store, RequestState{}); err != nil {
		w.Log().Error("store unreachable: %s", err)
		return err // want `A2012.*termination reason \(Call\)`
	}

	// The wrapping form.
	if err := w.Send(w.peer, MessageWork{}); err != nil {
		w.Log().Error("forward failed: %s", err)
		return fmt.Errorf("forward: %w", err) // want `A2012.*termination reason \(Send\)`
	}

	// A local assigned more than once: which assignment reaches the return is
	// dataflow, so the rule stays silent.
	var err error
	if from.ID == 0 {
		err = w.Send(w.peer, MessageIdle{})
	} else {
		err = w.Send(w.store, MessageIdle{})
	}
	if err != nil {
		w.Log().Error("send failed: %s", err)
		return err
	}

	// A helper that performs the send keeps the transient property in its own frame,
	// which is out of scope in v1.
	if err := w.forward(message); err != nil {
		w.Log().Error("forward failed: %s", err)
		return err
	}

	// A deliberate stop is not a double report: the runtime does not log those.
	w.Log().Error("shutting down on request")
	return gen.TerminateReasonNormal
}

// The type switch case body is a CaseClause, not a BlockStmt, and this is the most
// common written form of the defect. It is the regression guard for the statement list
// walk.
func (w *Worker) HandleMessageName(name gen.Atom, from gen.PID, message any) error {
	switch message.(type) {
	case MessageWork:
		err := w.Send(w.peer, message)
		w.Log().Error("cannot forward: %s", err)
		return err // want `A2012.*HandleMessageName logs this failure at Error level`
	case MessageIdle:
		// The sentinel provenance path, with no local variable at all.
		w.Log().Error("peer gone")
		return gen.ErrNoConnection // want `A2012.*termination reason \(gen.ErrNoConnection\)`
	}
	return nil
}

// The event callback and the SendEvent surface.
func (w *Worker) HandleEvent(event gen.MessageEvent) error {
	if err := w.SendEvent("ticks", w.token, event.Message); err != nil {
		w.Log().Error("republish failed: %s", err)
		return err // want `A2012.*HandleEvent logs this failure at Error level`
	}
	return nil
}

// A2012 does not own the two-result surface: A1008 does, and the one-result signature
// gate excludes it structurally.
func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if err := w.Send(w.peer, request); err != nil {
		w.Log().Error("send failed: %s", err)
		return nil, err
	}
	return nil, nil
}

// A function literal has its own results, so its return is not the callback's.
func (w *Worker) HandleInspect(item ...string) map[string]string {
	fn := func() error {
		if err := w.Send(w.peer, MessageIdle{}); err != nil {
			w.Log().Error("send failed: %s", err)
			return err
		}
		return nil
	}
	_ = fn
	return nil
}

func (w *Worker) forward(message any) error { return w.Send(w.peer, message) }

// A supervisor's death takes its whole subtree with it, which is a different sentence.
type Sup struct {
	act.Supervisor
	peer gen.PID
}

func (s *Sup) HandleMessage(from gen.PID, message any) error {
	if err := s.Send(s.peer, message); err != nil {
		s.Log().Error("send failed: %s", err)
		return err // want `A2012.*which ends this supervisor and, one child-exit hop later, every child under it`
	}
	return nil
}

// A router declares RouteMessage, which returns gen.Atom and can carry no error at
// all, but the model maps it onto the HandleMessage kind. The name gate is what keeps
// it out.
type Router struct {
	act.Actor
	peer gen.PID
}

func (r *Router) RouteMessage(from gen.PID, message any) gen.Atom {
	if err := r.Send(r.peer, message); err != nil {
		r.Log().Error("send failed: %s", err)
		return ""
	}
	return "worker"
}

// A meta terminates only itself and the runtime logs it at Trace, so the double report
// discriminator does not hold there.
type Conn struct {
	gen.MetaProcess
	peer gen.PID
}

func (c *Conn) Init(process gen.MetaProcess) error { return nil }

func (c *Conn) Start() error { return nil }

func (c *Conn) HandleMessage(from gen.PID, message any) error {
	if err := c.Send(c.peer, message); err != nil {
		c.Log().Error("send failed: %s", err)
		return err
	}
	return nil
}

func (c *Conn) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (c *Conn) Terminate(reason error) {}

// A recorded exception is silent.
type Recorded struct {
	act.Actor
	peer gen.PID
}

func (r *Recorded) HandleMessage(from gen.PID, message any) error {
	if err := r.Send(r.peer, message); err != nil {
		r.Log().Error("send failed: %s", err)
		//argus:allow A2012 losing the link really is fatal for this actor
		return err
	}
	return nil
}
