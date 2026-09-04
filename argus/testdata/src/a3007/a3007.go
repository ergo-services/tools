package a3007

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Worker struct {
	act.Actor
}

func factoryWorker() gen.ProcessBehavior { return &Worker{} }

// A spawn literal below the framework floor is applied verbatim, so it is a performance
// note rather than an error.
type Sup struct {
	act.Actor
}

func (s *Sup) Init(args ...any) error {
	s.Spawn(factoryWorker, gen.ProcessOptions{
		Compression: gen.Compression{
			Enable:    true,
			Threshold: 256, // want `\[tier3\] \[A3007\] a compression threshold of 256 is below the framework floor of 1024`
		},
	})

	// at or above the floor is the intended configuration
	s.Spawn(factoryWorker, gen.ProcessOptions{
		Compression: gen.Compression{Enable: true, Threshold: 4096},
	})
	s.Spawn(factoryWorker, gen.ProcessOptions{
		Compression: gen.Compression{Enable: true, Threshold: 1024},
	})

	// a threshold computed at runtime is not a literal and stays quiet
	s.Spawn(factoryWorker, gen.ProcessOptions{
		Compression: gen.Compression{Enable: true, Threshold: dynamicFloor()},
	})
	return nil
}

func dynamicFloor() int { return 512 }
