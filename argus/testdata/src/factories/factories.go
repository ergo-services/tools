// Package factories declares behaviors whose Init properties have to travel to the
// package that spawns them. A1011 and A2017 both compute their verdict here and
// export it on the factory object, because a spawning package names the factory and
// nothing else: gen.ProcessFactory is func() ProcessBehavior, so the operand's static
// type is the same for every factory in existence.
package factories

import (
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"

	"example.com/requests"
)

type RequestSeed struct{ ID int64 }

// Quiet does nothing in Init, so nothing is exported about it.
type Quiet struct {
	act.Actor
}

func (q *Quiet) Init(args ...any) error { return nil }

func FactoryQuiet() gen.ProcessBehavior { return &Quiet{} }

// Direct makes a request in Init with the default timeout. This is the one shape a
// syntactic search finds.
type Direct struct {
	act.Actor
	seed gen.PID
}

func (d *Direct) Init(args ...any) error {
	d.Call(d.seed, RequestSeed{})
	return nil
}

func FactoryDirect() gen.ProcessBehavior { return &Direct{} }

// Indirect reaches the request two frames down through a helper, which is the shape
// twenty of twenty one corpus sites had.
type Indirect struct {
	act.Actor
	seed gen.PID
}

func (i *Indirect) Init(args ...any) error {
	return i.warmUp()
}

func (i *Indirect) warmUp() error {
	return i.fetchSeed()
}

func (i *Indirect) fetchSeed() error {
	i.Call(i.seed, RequestSeed{})
	return nil
}

func FactoryIndirect() gen.ProcessBehavior { return &Indirect{} }

// Patient asks for a long inner timeout, which is worse rather than better: the
// spawner still gives up at the init budget.
type Patient struct {
	act.Actor
	seed gen.PID
}

func (p *Patient) Init(args ...any) error {
	p.CallWithTimeout(p.seed, RequestSeed{}, 30)
	return nil
}

func FactoryPatient() gen.ProcessBehavior { return &Patient{} }

// Brief keeps its request well inside the budget, which is the correct shape.
type Brief struct {
	act.Actor
	seed gen.PID
}

func (b *Brief) Init(args ...any) error {
	b.CallWithTimeout(b.seed, RequestSeed{}, 1)
	return nil
}

func FactoryBrief() gen.ProcessBehavior { return &Brief{} }

// Sleepy waits on something that is not a request at all, so it has no inner budget
// to set against the init budget.
type Sleepy struct {
	act.Actor
}

func (s *Sleepy) Init(args ...any) error {
	time.Sleep(time.Second)
	return nil
}

func FactorySleepy() gen.ProcessBehavior { return &Sleepy{} }

// Waiting waits on a channel in Init, which is not a request either.
type Waiting struct {
	act.Actor
	ready chan struct{}
}

func (w *Waiting) Init(args ...any) error {
	<-w.ready
	return nil
}

func FactoryWaiting() gen.ProcessBehavior { return &Waiting{} }

// Mixed waits on a channel and makes a request; the request is what has a budget.
type Mixed struct {
	act.Actor
	ready chan struct{}
	seed  gen.PID
}

func (x *Mixed) Init(args ...any) error {
	<-x.ready
	x.Call(x.seed, RequestSeed{})
	return nil
}

func FactoryMixed() gen.ProcessBehavior { return &Mixed{} }

// Delegated reaches a thirty second request in a helper of another package, and
// DelegatedBrief a one second one. The timeout travels with the blocking fact.
type Delegated struct {
	act.Actor
	seed gen.PID
}

func (d *Delegated) Init(args ...any) error {
	return requests.Slow(&d.Actor, d.seed)
}

func FactoryDelegated() gen.ProcessBehavior { return &Delegated{} }

type DelegatedBrief struct {
	act.Actor
	seed gen.PID
}

func (d *DelegatedBrief) Init(args ...any) error {
	return requests.Brief(&d.Actor, d.seed)
}

func FactoryDelegatedBrief() gen.ProcessBehavior { return &DelegatedBrief{} }

// Hidden makes a three second request and a default one. The budget is the longest
// of the two, so the short one does not hide the default.
type Hidden struct {
	act.Actor
	seed gen.PID
}

func (h *Hidden) Init(args ...any) error {
	h.CallWithTimeout(h.seed, RequestSeed{}, 3)
	h.Call(h.seed, RequestSeed{})
	return nil
}

func FactoryHidden() gen.ProcessBehavior { return &Hidden{} }

// DelegatedBoth reaches the same pair through a helper of another package.
type DelegatedBoth struct {
	act.Actor
	seed gen.PID
}

func (d *DelegatedBoth) Init(args ...any) error {
	return requests.Both(&d.Actor, d.seed)
}

func FactoryDelegatedBoth() gen.ProcessBehavior { return &DelegatedBoth{} }

// Configured takes its timeout from a field, which does not resolve; Zero passes 0,
// which the framework replaces with DefaultRequestTimeout.
type Configured struct {
	act.Actor
	seed    gen.PID
	timeout int
}

func (c *Configured) Init(args ...any) error {
	c.CallWithTimeout(c.seed, RequestSeed{}, c.timeout)
	return nil
}

func FactoryConfigured() gen.ProcessBehavior { return &Configured{} }

type Zero struct {
	act.Actor
	seed gen.PID
}

func (z *Zero) Init(args ...any) error {
	z.CallWithTimeout(z.seed, RequestSeed{}, 0)
	return nil
}

func FactoryZero() gen.ProcessBehavior { return &Zero{} }

// Retrying only stores its request for later, here and through a helper, so Init
// itself waits for nothing.
type Retrying struct {
	act.Actor
	seed  gen.PID
	retry func()
	again func()
}

func (r *Retrying) Init(args ...any) error {
	r.retry = func() { r.Call(r.seed, RequestSeed{}) }
	r.again = requests.Later(&r.Actor, r.seed)
	return nil
}

func FactoryRetrying() gen.ProcessBehavior { return &Retrying{} }

// Owned signals "somebody else already owns this name" by returning a termination
// reason from Init, which is the convention every caller then has to decode.
type Owned struct {
	act.Actor
}

func (o *Owned) Init(args ...any) error { // want `\[tier2\] \[A2017\] Init returns gen.TerminateReasonNormal as its error`
	if o.Name() != "" {
		return gen.TerminateReasonNormal
	}
	return nil
}

func FactoryOwned() gen.ProcessBehavior { return &Owned{} }

// Wrapped builds its behavior next to a literal that returns another one; the
// literal's return is the literal's result, not the factory's.
func FactoryWrapped() gen.ProcessBehavior {
	fallback := func() gen.ProcessBehavior { return &Quiet{} }
	_ = fallback
	return &Direct{}
}

// A package level var of func type is how a generated supervisor is wired, and the
// link has to work through it too.
var FactoryOwnedVar gen.ProcessFactory = func() gen.ProcessBehavior { return &Owned{} }
