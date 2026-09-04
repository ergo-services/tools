package a2017

import (
	"errors"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"

	"factories"
)

// The caller that does not know the convention. FactoryOwned signals a normal outcome
// by returning gen.TerminateReasonNormal from Init, this function never mentions it,
// so an ordinary duplicate spawn is logged here as a failure.
type Blind struct {
	act.Supervisor
}

func (b *Blind) Init(args ...any) error {
	if _, err := b.Spawn(factories.FactoryOwned, gen.ProcessOptions{}); err != nil { // want `\[tier2\] \[A2017\] FactoryOwned signals a normal outcome by returning gen.TerminateReasonNormal from Init`
		b.Log().Error("spawn failed: %s", err)
	}
	return nil
}

// The caller that does know it. Decoding the sentinel is what the convention demands,
// and a site that does it is not reported: the defect is the undecoded one.
type Aware struct {
	act.Supervisor
}

func (a *Aware) Init(args ...any) error {
	if _, err := a.Spawn(factories.FactoryOwned, gen.ProcessOptions{}); err != nil {
		if errors.Is(err, gen.TerminateReasonNormal) {
			a.Log().Info("another process already owns the name")
			return nil
		}
		return err
	}
	return nil
}

// A factory with no sentinel convention says nothing about its callers.
type Ordinary struct {
	act.Supervisor
}

func (o *Ordinary) Init(args ...any) error {
	o.Spawn(factories.FactoryQuiet, gen.ProcessOptions{})
	return nil
}

// The var-of-func-type form has to carry the verdict too, since that is how a
// generated supervisor is wired.
type ViaVar struct {
	act.Supervisor
}

func (v *ViaVar) Init(args ...any) error {
	v.Spawn(factories.FactoryOwnedVar, gen.ProcessOptions{}) // want `A2017.*FactoryOwnedVar signals a normal outcome`
	return nil
}

// A recorded exception is silent.
type Recorded struct {
	act.Supervisor
}

func (r *Recorded) Init(args ...any) error {
	//argus:allow A2017 the caller treats every spawn error as fatal on purpose
	r.Spawn(factories.FactoryOwned, gen.ProcessOptions{})
	return nil
}
