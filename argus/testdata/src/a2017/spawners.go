package a2017

import (
	"errors"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

var errTaken = errors.New("taken")

// Every visible spawner decodes the sentinel, so the convention is one the package keeps
// and the declaration is not reported. This was reported before.
type Claim struct {
	act.Actor
}

func (c *Claim) Init(args ...any) error {
	if err := c.RegisterName("claim"); err != nil {
		if err == errTaken {
			return gen.TerminateReasonNormal
		}
		return err
	}
	return nil
}

func factoryClaim() gen.ProcessBehavior { return &Claim{} }

type Hub struct {
	act.Actor
}

func (h *Hub) HandleMessage(from gen.PID, message any) error {
	if _, err := h.Spawn(factoryClaim, gen.ProcessOptions{}); err != nil {
		if errors.Is(err, gen.TerminateReasonNormal) {
			return nil
		}
		h.Log().Error("spawn failed: %s", err)
	}
	return nil
}

// A SimpleOneForOne spec is a template and starts nothing: StartChild does, so the
// function around each StartChild is where the sentinel is decoded.
type Ticket struct {
	act.Actor
}

func (t *Ticket) Init(args ...any) error {
	if err := t.RegisterName("ticket"); err != nil {
		return gen.TerminateReasonNormal
	}
	return nil
}

func factoryTicket() gen.ProcessBehavior { return &Ticket{} }

type Desk struct {
	act.Supervisor
}

func (d *Desk) Init(args ...any) (act.SupervisorSpec, error) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeSimpleOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "ticket", Factory: factoryTicket},
		},
	}
	return spec, nil
}

func (d *Desk) HandleMessage(from gen.PID, message any) error {
	d.open()
	return nil
}

func (d *Desk) open() {
	if err := d.StartChild("ticket"); err != nil && err != gen.TerminateReasonNormal {
		d.Log().Error("cannot open a ticket: %s", err)
	}
}

// One StartChild decodes it and another does not, so the declaration names the blind one.
type Seat struct {
	act.Actor
}

func (s *Seat) Init(args ...any) error {
	if err := s.RegisterName("seat"); err != nil {
		return gen.TerminateReasonNormal // want `\[tier2\] \[A2017\] Init returns gen.TerminateReasonNormal as its error, which is the spawn failure channel rather than a termination reason, and the StartChild in Hall.refill does not decode it`
	}
	return nil
}

func factorySeat() gen.ProcessBehavior { return &Seat{} }

type Hall struct {
	act.Supervisor
}

func (h *Hall) Init(args ...any) (act.SupervisorSpec, error) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeSimpleOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "seat", Factory: factorySeat},
			{Name: "other", Factory: factoryTicket},
		},
	}
	return spec, nil
}

func (h *Hall) HandleMessage(from gen.PID, message any) error {
	if err := h.StartChild("seat"); errors.Is(err, gen.TerminateReasonNormal) {
		return nil
	}
	h.refill()
	return nil
}

func (h *Hall) refill() {
	h.StartChild("seat")
}

// A StartChild is matched to the child it names: the two "seat" calls above are not
// spawners of Ticket, and this decoding one is.
func (h *Hall) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if err := h.StartChild("other"); err != nil && errors.Is(err, gen.TerminateReasonNormal) == false {
		return nil, err
	}
	return nil, nil
}

// A spawner in the same package that never mentions the sentinel.
type Lane struct {
	act.Actor
}

func (l *Lane) Init(args ...any) error {
	if err := l.RegisterName("lane"); err != nil {
		return gen.TerminateReasonShutdown // want `A2017.*Init returns gen.TerminateReasonShutdown as its error.*and the Spawn in Pool.HandleMessage does not decode it`
	}
	return nil
}

func factoryLane() gen.ProcessBehavior { return &Lane{} }

type Pool struct {
	act.Actor
}

func (p *Pool) HandleMessage(from gen.PID, message any) error {
	p.Spawn(factoryLane, gen.ProcessOptions{})
	return nil
}

// Refusing bad arguments with the sentinel is a failure disguised as a normal outcome, and
// a spawner that decodes the sentinel as benign is exactly the one that hides it, so it is
// reported whatever the spawners do.
type Badge struct {
	act.Actor
	owner gen.PID
}

type badgeArgs struct{ owner gen.PID }

func (b *Badge) Init(args ...any) error {
	a, ok := args[0].(badgeArgs)
	if ok == false {
		return gen.TerminateReasonNormal // want `A2017.*Init returns gen.TerminateReasonNormal when it rejects its arguments, which disguises a spawn failure as a normal outcome`
	}
	b.owner = a.owner
	return nil
}

func factoryBadge() gen.ProcessBehavior { return &Badge{} }

func (h *Hub) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	_, err := h.Spawn(factoryBadge, gen.ProcessOptions{}, badgeArgs{owner: from})
	if errors.Is(err, gen.TerminateReasonNormal) {
		return nil, nil
	}
	return nil, err
}

// A condition on an operation's result is not a condition on the arguments, even when the
// operation was given a value derived from them.
type Named struct {
	act.Actor
}

func (n *Named) Init(args ...any) error {
	name := gen.Atom(args[0].(string))
	if err := n.RegisterName(name); err != nil {
		return gen.TerminateReasonNormal // want `A2017.*Init returns gen.TerminateReasonNormal as its error.*no spawn point in this package shows that they do`
	}
	return nil
}
