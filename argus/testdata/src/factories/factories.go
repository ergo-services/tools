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

// Sleepy waits on something that is not a request at all.
type Sleepy struct {
	act.Actor
}

func (s *Sleepy) Init(args ...any) error {
	time.Sleep(time.Second)
	return nil
}

func FactorySleepy() gen.ProcessBehavior { return &Sleepy{} }

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

// A package level var of func type is how a generated supervisor is wired, and the
// link has to work through it too.
var FactoryOwnedVar gen.ProcessFactory = func() gen.ProcessBehavior { return &Owned{} }
